package object

import (
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strings"
)

type ValueType int

const (
	ValueNil ValueType = iota
	ValueBool
	ValueInteger
	ValueFloat
	ValueString
	ValueArray
	ValueHash
	ValueSymbol
	ValueRegexp
	ValueRange
	ValueClass
	ValueModule
	ValueObject
	ValueFunction
	ValueBuiltin
	ValueClosure
	ValueProc
	ValueMethod
	ValueBinding
	ValueFiber
	ValueMatchData
	ValueIO
	ValueFile
	ValueException
)

// AllocationMetadata is cold ObjectSpace tracing state. Ordinary Ruby values
// do not need it; keeping it behind a lazy pointer avoids putting source
// locations and weak-tracing bookkeeping in every hot EmeraldValue header.
type AllocationMetadata struct {
	Owner      *EmeraldValue
	Traced     bool
	Generation int64
	SourceFile string
	SourceLine int64
	ClassPath  string
	MethodID   string
}

// ValueColdData keeps rarely-used arbitrary-precision and mutable-string
// state away from the hot EmeraldValue header. The sidecar is allocated only
// when a value actually needs one of these fields.
type ValueColdData struct {
	BigInt        *big.Int
	StringBuilder *strings.Builder
	Allocation    *AllocationMetadata
	LazyArray     *LazyArrayRegion
}

// LazyArrayRegion is a deliberately small escape hatch for a proven VM
// producer. The ordinary Array representation remains []*EmeraldValue; a
// region may postpone that materialization until an operation needs actual
// elements. Payload is owned by the producer and is opaque to object.
// Materialize must return the exact Ruby elements in order and must not be
// called more than once for the same EmeraldValue.
type LazyArrayRegion struct {
	Length      int
	Payload     any
	Materialize func() []*EmeraldValue
	// ElementAt may serve a single indexed read without committing the whole
	// region.  A nil function keeps the historical full-materialization path.
	ElementAt        func(index int) (*EmeraldValue, bool)
	MethodGeneration uint64
}

type EmeraldValue struct {
	Type            ValueType
	Data            interface{}
	Class           *Class
	Frozen          bool
	TemporaryLocked bool
	Chilled         bool
	Literal         bool
	Ruby2Keywords   bool
	Encoding        string
	InstanceVars    map[string]*EmeraldValue
	Cold            *ValueColdData
}

// CloneColdData copies the sidecar container while preserving the historical
// shallow-copy behavior of the values it contains.  EmeraldValue copies must
// not share the container itself: setters on one copy must not rewrite the
// other copy's BigInt or StringBuilder field.
func (v *EmeraldValue) CloneColdData() *ValueColdData {
	if v == nil || v.Cold == nil {
		return nil
	}
	clone := *v.Cold
	return &clone
}

func (v *EmeraldValue) BigIntValue() *big.Int {
	if v == nil || v.Cold == nil {
		return nil
	}
	return v.Cold.BigInt
}

func (v *EmeraldValue) SetBigInt(value *big.Int) {
	if v == nil {
		return
	}
	if value == nil && v.Cold == nil {
		return
	}
	if v.Cold == nil {
		v.Cold = &ValueColdData{}
	}
	v.Cold.BigInt = value
}

func (v *EmeraldValue) StringBuilderValue() *strings.Builder {
	if v == nil || v.Cold == nil {
		return nil
	}
	return v.Cold.StringBuilder
}

func (v *EmeraldValue) SetStringBuilder(value *strings.Builder) {
	if v == nil {
		return
	}
	if value == nil && v.Cold == nil {
		return
	}
	if v.Cold == nil {
		v.Cold = &ValueColdData{}
	}
	v.Cold.StringBuilder = value
}

func (v *EmeraldValue) AllocationMetadataValue() *AllocationMetadata {
	if v == nil || v.Cold == nil {
		return nil
	}
	return v.Cold.Allocation
}

func (v *EmeraldValue) SetAllocationMetadata(value *AllocationMetadata) {
	if v == nil {
		return
	}
	if value == nil && v.Cold == nil {
		return
	}
	if v.Cold == nil {
		v.Cold = &ValueColdData{}
	}
	v.Cold.Allocation = value
}

func (v *EmeraldValue) LazyArrayRegionValue() *LazyArrayRegion {
	if v == nil || v.Cold == nil {
		return nil
	}
	return v.Cold.LazyArray
}

func (v *EmeraldValue) SetLazyArrayRegion(region *LazyArrayRegion) {
	if v == nil {
		return
	}
	if region == nil && v.Cold == nil {
		return
	}
	if v.Cold == nil {
		v.Cold = &ValueColdData{}
	}
	v.Cold.LazyArray = region
}

// MaterializeLazyArray commits a deferred Array region to the ordinary slice
// representation. The caller can use the returned slice for generic Array
// operations; a false result means that the value was not a lazy Array.
func (v *EmeraldValue) MaterializeLazyArray() ([]*EmeraldValue, bool) {
	if v == nil || v.Type != ValueArray || v.Cold == nil || v.Cold.LazyArray == nil {
		return nil, false
	}
	region := v.Cold.LazyArray
	if region.Materialize == nil {
		return nil, false
	}
	elements := region.Materialize()
	v.Data = elements
	v.Cold.LazyArray = nil
	return elements, true
}

func NewValue(t ValueType, data interface{}, class *Class) *EmeraldValue {
	return &EmeraldValue{
		Type:  t,
		Data:  data,
		Class: class,
	}
}

func (v *EmeraldValue) Inspect() string {
	return v.inspectWithSeen(make(map[*EmeraldValue]bool))
}

func (v *EmeraldValue) InspectWithSeen(seen map[*EmeraldValue]bool) string {
	if v == nil {
		return "nil"
	}
	if seen == nil {
		seen = make(map[*EmeraldValue]bool)
	}
	return v.inspectWithSeen(seen)
}

func (v *EmeraldValue) inspectWithSeen(seen map[*EmeraldValue]bool) string {
	if v == nil {
		return "nil"
	}
	switch v.Type {
	case ValueNil:
		return "nil"
	case ValueBool:
		if v.Data.(bool) {
			return "true"
		}
		return "false"
	case ValueInteger:
		if bigInteger := v.BigIntValue(); bigInteger != nil {
			return bigInteger.String()
		}
		return fmt.Sprintf("%d", v.Data)
	case ValueFloat:
		f := v.Data.(float64)
		s := fmt.Sprintf("%g", f)
		if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") && !math.IsInf(f, 0) && !math.IsNaN(f) {
			s += ".0"
		}
		return s
	case ValueString:
		return inspectString(v.Data.(string))
	case ValueArray:
		if seen[v] {
			return "[...]"
		}
		seen[v] = true
		defer delete(seen, v)
		arr, ok := v.Data.([]*EmeraldValue)
		if !ok {
			arr, _ = v.MaterializeLazyArray()
		}
		str := "["
		for i, e := range arr {
			str += e.inspectElementWithSeen(seen)
			if i < len(arr)-1 {
				str += ", "
			}
		}
		str += "]"
		return str
	case ValueHash:
		if seen[v] {
			return "{...}"
		}
		seen[v] = true
		defer delete(seen, v)
		type pair struct {
			key   *EmeraldValue
			value *EmeraldValue
		}
		entries := make([]pair, 0)
		switch h := v.Data.(type) {
		case *RHash:
			for _, key := range h.Keys {
				if value, ok := h.Pairs[key]; ok {
					entries = append(entries, pair{key: key, value: value})
				}
			}
			for key, value := range h.Pairs {
				isKeyOrdered := false
				for _, candidate := range h.Keys {
					if key == candidate {
						isKeyOrdered = true
						break
					}
				}
				if isKeyOrdered {
					continue
				}
				entries = append(entries, pair{key: key, value: value})
			}
		default:
			for key, value := range v.Data.(map[*EmeraldValue]*EmeraldValue) {
				entries = append(entries, pair{key: key, value: value})
			}
		}
		hashLen := 0
		switch h := v.Data.(type) {
		case *RHash:
			hashLen = len(h.Pairs)
		default:
			hashLen = len(v.Data.(map[*EmeraldValue]*EmeraldValue))
		}
		str := "{"
		for i, entry := range entries {
			str += entry.key.inspectElementWithSeen(seen) + " => " + entry.value.inspectElementWithSeen(seen)
			if i < hashLen-1 {
				str += ", "
			}
		}
		str += "}"
		return str
	case ValueClass:
		if class, ok := v.Data.(*Class); ok && class != nil {
			if class.Name != "" {
				return class.Name
			}
			return fmt.Sprintf("#<Class:%p>", class)
		}
		return "#<Class:0x0>"
	case ValueModule:
		if module, ok := v.Data.(*Module); ok && module != nil {
			if module.Name != "" {
				return module.Name
			}
			return fmt.Sprintf("#<Module:%p>", module)
		}
		return "#<Module:0x0>"
	case ValueFunction:
		fn := v.Data.(*Function)
		return fmt.Sprintf("#<Function:%s>", fn.Name)
	case ValueBuiltin:
		fn := v.Data.(*BuiltinFunction)
		return fmt.Sprintf("#<BuiltinFunction:%s>", fn.Name)
	case ValueClosure:
		return "#<Closure>"
	case ValueProc:
		return "#<Proc>"
	case ValueMethod:
		m := v.Data.(*Method)
		return fmt.Sprintf("#<Method: %s>", m.Name)
	case ValueBinding:
		return "#<Binding>"
	case ValueSymbol:
		return inspectSymbol(v.Data.(string))
	case ValueRange:
		r := v.Data.(*RRange)
		op := ".."
		if r.Exclusive {
			op = "..."
		}
		return fmt.Sprintf("%d%s%d", r.Start, op, r.End)
	case ValueObject:
		if obj, ok := v.Data.(*Object); ok && obj != nil && obj.Class != nil && obj.Class.Name != "" {
			return fmt.Sprintf("#<%s:%p>", obj.Class.Name, v)
		}
		if obj, ok := v.Data.(*Object); ok && obj != nil {
			return fmt.Sprintf("#<Object:%p>", v)
		}
		return fmt.Sprintf("#<%v:%p>", reflect.TypeOf(v.Data), v)
	default:
		return fmt.Sprintf("#<%v>", v.Type)
	}
}

func inspectSymbol(name string) string {
	if isBareSymbolInspectName(name) {
		return ":" + name
	}
	return ":\"" + escapeSymbolInspectName(name) + "\""
}

func inspectString(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\a':
			out.WriteString(`\a`)
		case '\b':
			out.WriteString(`\b`)
		case '\t':
			out.WriteString(`\t`)
		case '\n':
			out.WriteString(`\n`)
		case '\v':
			out.WriteString(`\v`)
		case '\f':
			out.WriteString(`\f`)
		case '\r':
			out.WriteString(`\r`)
		case 0x1b:
			out.WriteString(`\e`)
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteByte(value[i])
		default:
			out.WriteByte(value[i])
		}
	}
	out.WriteByte('"')
	return out.String()
}

func isBareSymbolInspectName(name string) bool {
	if name == "" {
		return false
	}
	if strings.ContainsAny(name, " \t\r\n\"\\\x00") {
		return false
	}
	if isSingleSymbolOperator(name) {
		return true
	}
	first := name[0]
	if !((first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || first == '_' || first == '@' || first == '$') {
		return false
	}
	for i := 1; i < len(name); i++ {
		ch := name[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '@' || ch == '$' || ch == '?' || ch == '!' || ch == '=' {
			continue
		}
		return false
	}
	return true
}

func isSingleSymbolOperator(name string) bool {
	return len(name) == 1 && strings.ContainsAny(name, "+-*/%&|^~<>=")
}

func escapeSymbolInspectName(name string) string {
	var out strings.Builder
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		case 0:
			out.WriteString(`\x00`)
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteByte(name[i])
		default:
			out.WriteByte(name[i])
		}
	}
	return out.String()
}

func (v *EmeraldValue) InspectElement() string {
	return v.inspectElementWithSeen(make(map[*EmeraldValue]bool))
}

func (v *EmeraldValue) inspectElementWithSeen(seen map[*EmeraldValue]bool) string {
	if v == nil {
		return "nil"
	}
	if v.Type == ValueString {
		return inspectString(v.Data.(string))
	}
	return v.inspectWithSeen(seen)
}

func (v *EmeraldValue) TypeName() string {
	switch v.Type {
	case ValueNil:
		return "NilClass"
	case ValueBool:
		return "TrueClass"
	case ValueInteger:
		return "Integer"
	case ValueFloat:
		return "Float"
	case ValueString:
		return "String"
	case ValueArray:
		return "Array"
	case ValueHash:
		return "Hash"
	case ValueSymbol:
		return "Symbol"
	case ValueRegexp:
		return "Regexp"
	case ValueRange:
		return "Range"
	case ValueClass:
		return "Class"
	case ValueModule:
		return "Module"
	case ValueFunction:
		return "Function"
	case ValueBuiltin:
		return "Builtin"
	case ValueClosure:
		return "Closure"
	case ValueProc:
		return "Proc"
	case ValueMethod:
		return "Method"
	case ValueBinding:
		return "Binding"
	default:
		return "Unknown"
	}
}

func (v *EmeraldValue) IsTruthy() bool {
	if v == nil {
		return false
	}
	switch v.Type {
	case ValueNil:
		return false
	case ValueBool:
		if v.Data == nil {
			return false
		}
		return v.Data.(bool)
	default:
		return true
	}
}

func (v *EmeraldValue) Equals(other *EmeraldValue) bool {
	if v == nil || other == nil {
		return v == other
	}
	if v.Type != other.Type {
		return false
	}
	if v.Type != ValueArray && v.Type != ValueHash {
		return v.equals(other, nil)
	}
	return v.equals(other, make(map[[2]*EmeraldValue]bool))
}

func (v *EmeraldValue) equals(other *EmeraldValue, seen map[[2]*EmeraldValue]bool) bool {
	if v == nil || other == nil {
		return v == other
	}
	if v.Type != other.Type {
		return false
	}
	key := [2]*EmeraldValue{v, other}
	if seen[key] {
		return true
	}
	if v.Type != other.Type {
		return false
	}
	switch v.Type {
	case ValueNil:
		return true
	case ValueBool:
		return v.Data.(bool) == other.Data.(bool)
	case ValueInteger:
		return v.Data.(int64) == other.Data.(int64)
	case ValueFloat:
		return v.Data.(float64) == other.Data.(float64)
	case ValueString:
		return v.Data.(string) == other.Data.(string)
	case ValueSymbol:
		return v.Data.(string) == other.Data.(string)
	case ValueArray:
		left := v.Data.([]*EmeraldValue)
		right := other.Data.([]*EmeraldValue)
		if len(left) != len(right) {
			return false
		}
		seen[key] = true
		for i := range left {
			if !left[i].equals(right[i], seen) {
				return false
			}
		}
		delete(seen, key)
		return true
	case ValueHash:
		left := hashPairsForEquals(v)
		right := hashPairsForEquals(other)
		if len(left) != len(right) {
			return false
		}
		seen[key] = true
		for leftKey, leftValue := range left {
			found := false
			for rightKey, rightValue := range right {
				if leftKey.equals(rightKey, seen) {
					if !leftValue.equals(rightValue, seen) {
						delete(seen, key)
						return false
					}
					found = true
					break
				}
			}
			if !found {
				delete(seen, key)
				return false
			}
		}
		delete(seen, key)
		return true
	case ValueRange:
		r1 := v.Data.(*RRange)
		r2 := other.Data.(*RRange)
		return r1.Start == r2.Start && r1.End == r2.End && r1.Exclusive == r2.Exclusive
	case ValueClass:
		left, leftOK := v.Data.(*Class)
		right, rightOK := other.Data.(*Class)
		if !leftOK || !rightOK || left == nil || right == nil {
			return v.Data == other.Data
		}
		return left == right || (left.Name != "" && left.Name == right.Name)
	case ValueModule:
		left, leftOK := v.Data.(*Module)
		right, rightOK := other.Data.(*Module)
		if !leftOK || !rightOK || left == nil || right == nil {
			return v.Data == other.Data
		}
		return left == right || (left.Name != "" && left.Name == right.Name)
	case ValueMethod:
		left := v.Data.(*Method)
		right := other.Data.(*Method)
		return methodReceiverEqual(left.Receiver, right.Receiver) && methodImplementationEqual(left.Fn, right.Fn)
	default:
		return v == other
	}
}

func methodReceiverEqual(left *EmeraldValue, right *EmeraldValue) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left == right || left.Equals(right)
}

func methodImplementationEqual(left interface{}, right interface{}) bool {
	if left == nil || right == nil {
		return left == right
	}
	leftValue := reflect.ValueOf(left)
	rightValue := reflect.ValueOf(right)
	if leftValue.Type() != rightValue.Type() {
		return false
	}
	switch leftValue.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return leftValue.Pointer() == rightValue.Pointer()
	default:
		if leftValue.Type().Comparable() {
			return left == right
		}
		return false
	}
}

func hashPairsForEquals(value *EmeraldValue) map[*EmeraldValue]*EmeraldValue {
	if value == nil || value.Type != ValueHash {
		return nil
	}
	switch h := value.Data.(type) {
	case map[*EmeraldValue]*EmeraldValue:
		return h
	case *RHash:
		if h == nil {
			return nil
		}
		return h.Pairs
	default:
		return nil
	}
}

type KeywordParamInfo struct {
	Name       string
	HasDefault bool
	Default    *EmeraldValue
}

type ParameterPattern struct {
	Name      string
	Children  []*ParameterPattern
	Rest      *ParameterPattern
	RestIndex int
}

type ConstantLocation struct {
	Path string
	Line int64
}

type Function struct {
	Name                  string
	SourcePath            string
	SourceAbsolutePath    string
	EvalSource            bool
	EvalInheritedLocals   int
	SourceEncoding        string
	StringLiteralModeSet  bool
	FreezeStringLiterals  bool
	ChillStringLiterals   bool
	DefinitionLine        int64
	Params                []string
	ParamLocalIndices     []int
	ParamPatterns         []*ParameterPattern
	ParamDefaults         []*EmeraldValue
	EvaluateParamDefaults bool
	MethodBody            bool
	SingletonClassBody    bool
	KeywordParams         []KeywordParamInfo
	Body                  interface{}
	BlockLocals           []string
	FreeVars              []*EmeraldValue
	Instructions          []byte
	LineMap               map[int]int
	Constants             []*EmeraldValue
	NumLocals             int
	GlobalNames           map[string]int
	LocalNames            map[string]int
	FreeVarNames          []string
	HasRestParam          bool
	AnonymousRestParam    bool
	RestParamIndex        int
	RestParamName         string
	RejectKeywords        bool
	SingleDestructure     bool
	KeywordRestOnly       bool
	KeywordRestParam      string
	HasBlockParam         bool
	AnonymousBlockParam   bool
	BlockParamIndex       int
	RejectBlock           bool
	TrailingCommaParam    bool
	DefinedByDefineMethod bool
	ForLoopCollectAsPair  bool
	FlipFlopStates        map[int]bool
	ImplicitItParameter   bool
	NumberedParameters    bool
	DefinitionVisibility  string
	// SimpleArgumentShapeChecked/Eligible are immutable compiler-shape
	// metadata used by VM block/method fast paths. Keeping the proof on the
	// Function avoids a per-VM map lookup for every callback invocation.
	SimpleArgumentShapeChecked  bool
	SimpleArgumentShapeEligible bool
	// SimpleBlockPatternChecked/Eligible cache the immutable named-parameter
	// pattern proof used by the zero-frame and reusable block entries.
	SimpleBlockPatternChecked  bool
	SimpleBlockPatternEligible bool
	// CachedPositionalArgumentShapeChecked/Eligible are the corresponding proof
	// for the cached bytecode entry. Unlike the block shape, this proof permits
	// positional defaults; the normal bytecode prologue still evaluates any
	// omitted default expression.
	CachedPositionalArgumentShapeChecked  bool
	CachedPositionalArgumentShapeEligible bool
	// CallerBlockObservationChecked/Safe cache the bytecode proof used when a
	// fixed-arity method is invoked from inside another block. The instruction
	// stream is immutable after compilation, so repeating the opcode scan at
	// every cached send is unnecessary.
	CallerBlockObservationChecked bool
	CallerBlockObservationSafe    bool
	// ArrayBytecodeBlockReuseChecked/Eligible cache the immutable opcode proof
	// for the reusable Array callback entry. The VM may inspect the same block
	// thousands of times through Array#each/map; do not rescan its bytecode on
	// every native iterator call.
	ArrayBytecodeBlockReuseChecked  bool
	ArrayBytecodeBlockReuseEligible bool
}

type BuiltinFunction struct {
	Name  string
	Fn    func(args ...*EmeraldValue) *EmeraldValue
	Arity int
}

type Method struct {
	Name                  string
	OriginalName          string
	Fn                    interface{}
	Closure               *Closure
	Arity                 int
	Receiver              *EmeraldValue
	Owner                 *EmeraldValue
	DispatchOwner         *EmeraldValue
	Visibility            string
	VisibilityAliasStart  *Class
	Ruby2Keywords         bool
	DynamicMissing        bool
	EnforceArity          bool
	DefinedByDefineMethod bool
	AttrReaderName        string
	AttrWriterName        string
}

type Proc struct {
	Origin           *Proc
	CachedClosure    *Closure
	Fn               *Function
	Env              []*EmeraldValue
	Block            *EmeraldValue
	Binding          *RBinding
	ClassStack       []*EmeraldValue
	Refinements      []*EmeraldValue
	RefinementsFixed bool
	InstanceVars     map[string]*EmeraldValue
	Native           func(args ...*EmeraldValue) *EmeraldValue
	NativeArity      int
	HasNativeArity   bool
	AutoSplat        bool
	IsLambda         bool
	BreakOwnerID     int
	ReturnOwnerID    int
	FlipFlopStates   map[int]bool
	CurryTarget      *EmeraldValue
	CurryArgs        []*EmeraldValue
	CurryArity       int
	SymbolProc       bool
	SymbolName       string
	SourceMethod     *Method
}

type ControlFlow struct {
	Kind  string
	Value *EmeraldValue
}

type Closure struct {
	Fn               *Function
	Free             []*EmeraldValue
	Block            *EmeraldValue
	Binding          *RBinding
	ClassStack       []*EmeraldValue
	Refinements      []*EmeraldValue
	RefinementsFixed bool
	// Refinement checks are performed at many speculative block/method
	// call-sites.  The VM caches the result against the method hierarchy
	// generation; callers that mutate this view clear the generation before
	// the next probe.  Keeping the metadata on the closure avoids a VM-global
	// map lookup on every callback.
	RefinementCheckGeneration uint64
	RefinementCheckResult     bool
	RefinementCheckValid      bool
	AutoSplat                 bool
	BreakOwnerID              int
	ReturnOwnerID             int
	FlipFlopStates            map[int]bool
}

type RInteger struct {
	Value int64
}

type RFloat struct {
	Value float64
}

type RString struct {
	Value string
}

type RArray struct {
	Elements []*EmeraldValue
}

type RHash struct {
	Pairs             map[*EmeraldValue]*EmeraldValue
	Keys              []*EmeraldValue
	Hashes            map[*EmeraldValue]int64
	Buckets           map[int64][]*EmeraldValue
	Linear            *RHashLinearRegion
	BucketSize        int
	Default           *EmeraldValue
	DefaultProc       *EmeraldValue
	CompareByIdentity bool
	InstanceVars      map[string]*EmeraldValue
}

// RHashLinearRegion is a private lazy representation for a completed affine
// integer fill. It is valid only until a general Hash API materializes or
// mutates the ordinary pointer map.
type RHashLinearRegion struct {
	Start       int64
	Step        int64
	ValueOffset int64
	Count       int
}

type RSymbol struct {
	Value string
}

type RRegexp struct {
	Pattern    string
	Options    string
	Timeout    float64
	HasTimeout bool

	CompiledPattern string
	Compiled        any
}

type RRange struct {
	Start int64
	End   int64

	StartValue interface{}
	EndValue   interface{}

	StartFloat bool
	EndFloat   bool
	StartRaw   float64
	EndRaw     float64

	StartMissing bool
	EndMissing   bool

	Exclusive bool
}

type RException struct {
	Message            string
	MessageValue       *EmeraldValue
	Path               string
	Backtrace          []string
	BacktraceValue     *EmeraldValue
	Locations          []RBacktraceLocation
	LocationsValue     *EmeraldValue
	InstanceVars       map[string]*EmeraldValue
	SingletonClass     *Class
	Cause              *EmeraldValue
	Raised             bool
	Result             *EmeraldValue
	Status             *int64
	Errno              *int64
	NameErrorName      string
	NameErrorNameValue *EmeraldValue
	NameErrorReceiver  *EmeraldValue
	NoMethodArgs       []*EmeraldValue
	KeyErrorKey        *EmeraldValue
	UncaughtThrowTag   *EmeraldValue
	UncaughtThrowValue *EmeraldValue
	LocalJumpReason    string
	LocalJumpExitValue *EmeraldValue
}

type RBacktraceLocation struct {
	Path         string
	AbsolutePath string
	Line         int64
	Label        string
	EvalSource   bool
}

type RBindingExpanded struct {
	Locals         map[string]*EmeraldValue
	LocalNames     []string
	InstanceVars   map[string]*EmeraldValue
	SharedLocals   map[string]struct{}
	ShareAllLocals bool
}

type RBinding struct {
	*RBindingExpanded
	Self                    *EmeraldValue
	Block                   *EmeraldValue
	EvalReturnTargetID      int
	Constants               map[string]*EmeraldValue
	Method                  string
	BacktraceLabel          string
	ClassStack              []*EmeraldValue
	ClassVarScope           *EmeraldValue
	Refinements             []*EmeraldValue
	Path                    string
	Line                    int64
	Parent                  *RBinding
	CompactLocalIndices     map[string]int
	CompactLocalValues      []*EmeraldValue
	CompactFreeNames        []string
	CompactFreeValues       []*EmeraldValue
	AllowAnonymousBlockPass bool
	CompactImplicitIt       bool
	CompactNumberedParams   bool
	CompactLocals           bool
}

func (binding *RBinding) EnsureExpanded() *RBindingExpanded {
	if binding.RBindingExpanded == nil {
		binding.RBindingExpanded = &RBindingExpanded{}
	}
	return binding.RBindingExpanded
}

type BindingCell interface {
	BindingValue() *EmeraldValue
}

func DereferenceBindingValue(value *EmeraldValue) *EmeraldValue {
	seen := map[BindingCell]struct{}{}
	for value != nil {
		cell, ok := value.Data.(BindingCell)
		if !ok {
			return value
		}
		if _, cycle := seen[cell]; cycle {
			return nil
		}
		seen[cell] = struct{}{}
		value = cell.BindingValue()
	}
	return nil
}

func (binding *RBinding) MaterializeLocals() {
	if binding == nil || !binding.CompactLocals {
		return
	}
	binding.EnsureExpanded()
	capacity := len(binding.CompactLocalIndices) + len(binding.CompactFreeNames)
	if binding.Locals == nil {
		binding.Locals = make(map[string]*EmeraldValue, capacity)
	}
	ordered := make([]string, len(binding.CompactLocalValues))
	for name, index := range binding.CompactLocalIndices {
		if index >= 0 && index < len(ordered) {
			ordered[index] = name
		}
	}
	for index, name := range ordered {
		if binding.compactLocalHidden(name) || binding.compactFreeLocal(name) || index >= len(binding.CompactLocalValues) {
			continue
		}
		if value := binding.CompactLocalValues[index]; value != nil {
			binding.Locals[name] = value
			binding.LocalNames = append(binding.LocalNames, name)
		}
	}
	for index, name := range binding.CompactFreeNames {
		if name == "" || index >= len(binding.CompactFreeValues) {
			continue
		}
		if _, exists := binding.Locals[name]; exists {
			continue
		}
		binding.Locals[name] = binding.CompactFreeValues[index]
		binding.LocalNames = append(binding.LocalNames, name)
	}
	binding.CompactLocalIndices = nil
	binding.CompactLocalValues = nil
	binding.CompactFreeNames = nil
	binding.CompactFreeValues = nil
	binding.CompactLocals = false
}

func (binding *RBinding) compactLocalHidden(name string) bool {
	if name == "" || binding.CompactImplicitIt && name == "it" {
		return true
	}
	return binding.CompactNumberedParams && len(name) == 2 && name[0] == '_' && name[1] >= '1' && name[1] <= '9'
}

func (binding *RBinding) compactFreeLocal(name string) bool {
	for _, freeName := range binding.CompactFreeNames {
		if freeName == name {
			return true
		}
	}
	return false
}
