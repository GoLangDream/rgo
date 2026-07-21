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

type EmeraldValue struct {
	Type            ValueType
	Data            interface{}
	Class           *Class
	Frozen          bool
	TemporaryLocked bool
	Chilled         bool
	Literal         bool
	BigInt          *big.Int
	Encoding        string
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
		if v.BigInt != nil {
			return v.BigInt.String()
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
		arr := v.Data.([]*EmeraldValue)
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
}

type Proc struct {
	Origin           *Proc
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
	AutoSplat        bool
	BreakOwnerID     int
	ReturnOwnerID    int
	FlipFlopStates   map[int]bool
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
	Default           *EmeraldValue
	DefaultProc       *EmeraldValue
	CompareByIdentity bool
	InstanceVars      map[string]*EmeraldValue
}

type RSymbol struct {
	Value string
}

type RRegexp struct {
	Pattern    string
	Options    string
	Timeout    float64
	HasTimeout bool
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

type RBinding struct {
	Self                    *EmeraldValue
	Block                   *EmeraldValue
	AllowAnonymousBlockPass bool
	EvalReturnTargetID      int
	Locals                  map[string]*EmeraldValue
	LocalNames              []string
	Constants               map[string]*EmeraldValue
	Method                  string
	BacktraceLabel          string
	InstanceVars            map[string]*EmeraldValue
	ClassStack              []*EmeraldValue
	ClassVarScope           *EmeraldValue
	Refinements             []*EmeraldValue
	Path                    string
	Line                    int64
	ShareAllLocals          bool
	Parent                  *RBinding
	SharedLocals            map[string]struct{}
}
