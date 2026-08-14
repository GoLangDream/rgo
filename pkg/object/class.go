package object

import "os"

// CompactObjectLayouts is enabled only for the closed-world compiled mode.
// The compatibility VM keeps the historical map-backed object representation;
// compiled mode can use a class-proven inline ivar layout and still deopt to
// the map when a class exceeds the small inline budget.
var CompactObjectLayouts = os.Getenv("RGO_COMPACT_OBJECTS") != "" || os.Getenv("RGO_EXEC_MODE") == "compiled"

const inlineInstanceVarSlots = 4

// ActiveValueArena is installed by the compiled VM for the lifetime of a
// process/run. It batches ordinary user-object allocations into stable chunks
// so a hot object region does not perform one Go heap allocation per Ruby
// object. Compatibility mode leaves it nil and keeps normal GC ownership.
var ActiveValueArena *ValueArena

const valueArenaChunkSize = 4096

type ValueArena struct {
	pairs [][]objectValuePair
}

func NewValueArena() *ValueArena {
	return &ValueArena{}
}

func (a *ValueArena) NewObjectValue(class *Class) *EmeraldValue {
	if a == nil {
		return nil
	}
	chunkIndex := len(a.pairs) - 1
	if chunkIndex < 0 || len(a.pairs[chunkIndex]) >= valueArenaChunkSize {
		a.pairs = append(a.pairs, make([]objectValuePair, 0, valueArenaChunkSize))
		chunkIndex++
	}
	chunk := &a.pairs[chunkIndex]
	*chunk = append(*chunk, objectValuePair{})
	pair := &(*chunk)[len(*chunk)-1]
	objectValue := &pair.object
	value := &pair.value
	objectValue.Class = class
	if class == nil || !class.CompactInstanceVars {
		objectValue.InstanceVars = make(map[string]*EmeraldValue)
	}
	value.Type = ValueObject
	value.Data = objectValue
	value.Class = class
	return value
}

// FillObjectValues materializes a batch of ordinary user objects into one
// contiguous value/payload allocation. The returned values keep the backing
// pair slice alive through their Data pointers, so callers can use the result
// as a normal Ruby Array without retaining a second arena object. It is used
// only by closed-world compiled regions; the regular allocator remains
// NewObjectValue for compatibility paths.
func FillObjectValues(values []*EmeraldValue, class *Class) {
	if len(values) == 0 {
		return
	}
	for offset := 0; offset < len(values); {
		end := offset + valueArenaChunkSize
		if end > len(values) {
			end = len(values)
		}
		pairs := make([]objectValuePair, end-offset)
		for index := range pairs {
			pair := &pairs[index]
			pair.object.Class = class
			if class == nil || !class.CompactInstanceVars {
				pair.object.InstanceVars = make(map[string]*EmeraldValue)
			}
			pair.value.Type = ValueObject
			pair.value.Data = &pair.object
			pair.value.Class = class
			values[offset+index] = &pair.value
		}
		offset = end
	}
}

// independentObjectValuePair keeps one ObjectSpace-visible value header and
// its payload in one heap allocation. The value is the first field on purpose:
// weak.Make(value) then points at the allocation base instead of an interior
// address, while Data keeps the payload alive. This preserves the cheap weak
// lookup of the independent path without paying for a separate value header
// and a separate payload chunk.
type independentObjectValuePair struct {
	value  EmeraldValue
	object Object
}

// FillObjectValuesWithIndependentValues materializes a batch with one
// independent value/payload allocation per element. ObjectSpace can weakly
// point at each value allocation itself, avoiding Go's interior-pointer weak
// scan. It is used only by a proven compiled constructor batch.
func FillObjectValuesWithIndependentValues(values []*EmeraldValue, class *Class) {
	if len(values) == 0 {
		return
	}
	for index := range values {
		pair := &independentObjectValuePair{}
		pair.object.Class = class
		if class == nil || !class.CompactInstanceVars {
			pair.object.InstanceVars = make(map[string]*EmeraldValue)
		}
		pair.value.Type = ValueObject
		pair.value.Data = &pair.object
		pair.value.Class = class
		values[index] = &pair.value
	}
}

type Class struct {
	Name                string
	NameValue           *EmeraldValue
	TemporaryName       bool
	SuperClass          *Class
	SuperClassSet       bool
	Methods             map[string]*Method
	Constants           map[string]*EmeraldValue
	PrivateConstants    map[string]bool
	DeprecatedConstants map[string]bool
	Autoloads           map[string]string
	ConstantLocations   map[string]ConstantLocation
	AutoloadLocations   map[string]ConstantLocation
	ClassVars           map[string]*EmeraldValue
	ClassVarOrder       []string
	ClassMethods        map[string]*Method
	InstanceVars        map[string]*EmeraldValue
	IsSingleton         bool
	Frozen              bool
	SingletonOwner      *EmeraldValue
	SingletonClass      *Class
	IncludedModules     []*Module // Modules included via include
	PrependedModules    []*Module // Modules prepended via prepend
	UsedRefinements     []*EmeraldValue
	StructFields        []string
	StructKeywordInit   int
	DirectSubclasses    []*Class
	CompactInstanceVars bool
	InstanceVarSlots    map[string]uint8
}

func NewClass(name string) *Class {
	return &Class{
		Name:                name,
		Methods:             make(map[string]*Method),
		Constants:           make(map[string]*EmeraldValue),
		PrivateConstants:    make(map[string]bool),
		DeprecatedConstants: make(map[string]bool),
		Autoloads:           make(map[string]string),
		ConstantLocations:   make(map[string]ConstantLocation),
		AutoloadLocations:   make(map[string]ConstantLocation),
		ClassVars:           make(map[string]*EmeraldValue),
		ClassMethods:        make(map[string]*Method),
		InstanceVars:        make(map[string]*EmeraldValue),
	}
}

func (c *Class) DefineMethod(name string, method *Method) {
	c.Methods[name] = method
	if CompactObjectLayouts && method != nil {
		if _, rubyMethod := method.Fn.(*Function); rubyMethod {
			c.CompactInstanceVars = true
		}
	}
	BumpMethodGeneration()
}

func (c *Class) DefineClassMethod(name string, method *Method) {
	if c.ClassMethods == nil {
		c.ClassMethods = map[string]*Method{}
	}
	c.ClassMethods[name] = method
	if c.SingletonClass != nil {
		if c.SingletonClass.Methods == nil {
			c.SingletonClass.Methods = map[string]*Method{}
		}
		c.SingletonClass.Methods[name] = method
	}
	BumpMethodGeneration()
}

func (c *Class) GetMethod(name string) (*Method, bool) {
	method, _, ok := c.GetMethodWithOwner(name)
	return method, ok
}

func (c *Class) GetMethodWithOwner(name string) (*Method, *Class, bool) {
	// Check prepended modules first (highest priority)
	for _, mod := range c.PrependedModules {
		if method, ok := mod.GetMethod(name); ok {
			return method, c, true
		}
	}

	// Check class methods
	method, ok := c.Methods[name]
	if ok {
		return method, c, true
	}

	// Check included modules. Later includes have higher priority in Ruby's
	// ancestor chain.
	for i := len(c.IncludedModules) - 1; i >= 0; i-- {
		mod := c.IncludedModules[i]
		if method, ok := mod.GetMethod(name); ok {
			return method, c, true
		}
	}

	// Check superclass
	if c.SuperClass != nil {
		return c.SuperClass.GetMethodWithOwner(name)
	}

	return nil, nil, false
}

func (c *Class) DefineConstant(name string, value *EmeraldValue) {
	c.Constants[name] = value
}

func (c *Class) GetConstant(name string) (*EmeraldValue, bool) {
	val, ok := c.Constants[name]
	if !ok && c.SuperClass != nil {
		return c.SuperClass.GetConstant(name)
	}
	return val, ok
}

func (c *Class) SetInstanceVar(name string, value *EmeraldValue) {
	c.InstanceVars[name] = value
}

func (c *Class) GetInstanceVar(name string) *EmeraldValue {
	return c.InstanceVars[name]
}

func (c *Class) Include(module *Module) {
	for _, included := range c.IncludedModules {
		if included == module {
			return
		}
	}
	c.IncludedModules = append(c.IncludedModules, module)
	BumpMethodGeneration()
}

func (c *Class) Extend(module *Module) {
	for name, method := range module.Methods {
		c.DefineClassMethod(name, method)
	}
}

func (c *Class) Prepend(module *Module) {
	for _, prepended := range c.PrependedModules {
		if prepended == module {
			return
		}
	}
	c.PrependedModules = append([]*Module{module}, c.PrependedModules...)
	BumpMethodGeneration()
}

func (c *Class) NewInstance() *EmeraldValue {
	return NewObjectValue(c)
}

type Object struct {
	Class                  *Class
	InstanceVars           map[string]*EmeraldValue
	InstanceVarOrder       []string
	InlineInstanceVarNames [inlineInstanceVarSlots]string
	InlineInstanceVarCount uint8
	InlineInstanceVars     [inlineInstanceVarSlots]*EmeraldValue
	InlineInstanceVarMask  uint8
	// HotIntegerInstanceVars is a lazy, per-object scalar sidecar used by a
	// proven VM region. It keeps the current int64 out of the boxed/map ABI
	// while no Ruby code can observe the intermediate value. The historical
	// InstanceVars map remains coherent at the last committed value and is
	// materialized on the first generic read/write/reflection operation.
	HotIntegerInstanceVarNames   [inlineInstanceVarSlots]string
	HotIntegerInstanceVarValues  [inlineInstanceVarSlots]int64
	HotIntegerInstanceVarClasses [inlineInstanceVarSlots]*Class
	HotIntegerInstanceVarMask    uint8
	ClassVars                    map[string]*EmeraldValue
	SingletonMethods             map[string]*Method
	SingletonClass               *Class
	StructValues                 []*EmeraldValue
}

// objectValuePair keeps the Ruby value header and its ordinary object payload
// in one Go allocation. The historical path allocated the EmeraldValue and
// Object separately, which dominates tight `Class.new` loops once method
// frames and ObjectSpace bookkeeping are removed.
type objectValuePair struct {
	value  EmeraldValue
	object Object
}

func NewObject(class *Class) *Object {
	obj := &Object{Class: class}
	if class == nil || !class.CompactInstanceVars {
		obj.InstanceVars = make(map[string]*EmeraldValue)
	}
	return obj
}

func NewObjectValue(class *Class) *EmeraldValue {
	if CompactObjectLayouts && ActiveValueArena != nil {
		if value := ActiveValueArena.NewObjectValue(class); value != nil {
			return value
		}
	}
	pair := &objectValuePair{}
	pair.object.Class = class
	if class == nil || !class.CompactInstanceVars {
		pair.object.InstanceVars = make(map[string]*EmeraldValue)
	}
	pair.value.Type = ValueObject
	pair.value.Data = &pair.object
	pair.value.Class = class
	return &pair.value
}

// NewBareObject is used by allocation paths whose builtin initializer cannot
// observe an instance-variable map.  InstanceVars remains lazily creatable by
// SetInstanceVar/SetDynamicInstanceVar, while the common Object.new case avoids
// allocating an empty Go map for every object.
func NewBareObject(class *Class) *Object {
	return &Object{Class: class}
}

func (o *Object) SetSingletonMethod(name string, method *Method) {
	if o.SingletonMethods == nil {
		o.SingletonMethods = make(map[string]*Method)
	}
	o.SingletonMethods[name] = method
}

func (o *Object) SetClassVar(name string, value *EmeraldValue) {
	if o.ClassVars == nil {
		o.ClassVars = make(map[string]*EmeraldValue)
	}
	o.ClassVars[name] = value
}

func (o *Object) GetInstanceVar(name string) *EmeraldValue {
	if o == nil {
		return nil
	}
	o.flushHotIntegerInstanceVars()
	if o.InstanceVars != nil {
		if val, ok := o.InstanceVars[name]; ok {
			return val
		}
	}
	if o.Class != nil {
		if index, ok := o.Class.instanceVarSlot(name); ok && index < inlineInstanceVarSlots && o.InlineInstanceVarMask&(1<<index) != 0 {
			return o.InlineInstanceVars[index]
		}
	}
	return nil
}

func (o *Object) SetInstanceVar(name string, value *EmeraldValue) {
	if o == nil {
		return
	}
	o.flushHotIntegerInstanceVars()
	if o.Class != nil && CompactObjectLayouts && o.Class.CompactInstanceVars {
		if index, ok := o.Class.ensureInstanceVarSlot(name); ok && index < inlineInstanceVarSlots {
			bit := uint8(1 << index)
			if o.InlineInstanceVarMask&bit == 0 {
				if o.InstanceVarOrder != nil {
					o.InstanceVarOrder = append(o.InstanceVarOrder, name)
				} else if o.InlineInstanceVarCount < inlineInstanceVarSlots {
					o.InlineInstanceVarNames[o.InlineInstanceVarCount] = name
					o.InlineInstanceVarCount++
				}
			}
			o.InlineInstanceVars[index] = value
			o.InlineInstanceVarMask |= bit
			if o.InstanceVars != nil {
				o.InstanceVars[name] = value
			}
			return
		}
	}
	// A proven constructor batch may use the inline slots even when the
	// process-wide compact layout flag is off. Materialize before a generic
	// write so existing inline values are not lost when this object crosses
	// back into the compatibility map representation.
	if o.InstanceVars == nil && o.InlineInstanceVarMask != 0 {
		o.InstanceVarMap()
	}
	if o.InstanceVars == nil {
		o.InstanceVars = make(map[string]*EmeraldValue)
	}
	if _, exists := o.InstanceVars[name]; !exists {
		o.InstanceVarOrder = append(o.InstanceVarOrder, name)
	}
	o.InstanceVars[name] = value
}

// PrepareHotIntegerInstanceVar reserves a small per-object scalar slot for a
// proven VM kernel. Unlike the closed-world compact layout, this does not
// mutate the class or change the representation of any other instance.
func (o *Object) PrepareHotIntegerInstanceVar(name string) (int, bool) {
	if o == nil || name == "" {
		return 0, false
	}
	for index, existing := range o.HotIntegerInstanceVarNames {
		if existing == name {
			return index, true
		}
	}
	for index, existing := range o.HotIntegerInstanceVarNames {
		if existing == "" {
			o.HotIntegerInstanceVarNames[index] = name
			return index, true
		}
	}
	return 0, false
}

// HotIntegerInstanceVar reads a still-unboxed scalar from a prepared slot.
func (o *Object) HotIntegerInstanceVar(index int) (int64, bool) {
	if o == nil || index < 0 || index >= inlineInstanceVarSlots ||
		o.HotIntegerInstanceVarMask&(1<<index) == 0 {
		return 0, false
	}
	return o.HotIntegerInstanceVarValues[index], true
}

// SetHotIntegerInstanceVar publishes the current scalar into a prepared slot
// without allocating an EmeraldValue. The map is deliberately left at its
// last committed value; any generic object API flushes the scalar first.
func (o *Object) SetHotIntegerInstanceVar(index int, value int64, integerClass *Class) bool {
	if o == nil || index < 0 || index >= inlineInstanceVarSlots ||
		o.HotIntegerInstanceVarNames[index] == "" {
		return false
	}
	o.HotIntegerInstanceVarValues[index] = value
	o.HotIntegerInstanceVarClasses[index] = integerClass
	o.HotIntegerInstanceVarMask |= uint8(1 << index)
	return true
}

// FlushHotIntegerInstanceVars commits any lazy scalar slots to the ordinary
// boxed/map representation. It is exported for VM side exits; normal Ruby
// reads and writes reach the same operation through the Object methods above.
func (o *Object) FlushHotIntegerInstanceVars() {
	o.flushHotIntegerInstanceVars()
}

func (o *Object) flushHotIntegerInstanceVars() {
	if o == nil || o.HotIntegerInstanceVarMask == 0 {
		return
	}
	if o.InstanceVars == nil {
		o.InstanceVars = make(map[string]*EmeraldValue)
	}
	for index, name := range o.HotIntegerInstanceVarNames {
		bit := uint8(1 << index)
		if name == "" || o.HotIntegerInstanceVarMask&bit == 0 {
			continue
		}
		if _, exists := o.InstanceVars[name]; !exists {
			o.InstanceVarOrder = append(o.InstanceVarOrder, name)
		}
		o.InstanceVars[name] = &EmeraldValue{
			Type:  ValueInteger,
			Data:  o.HotIntegerInstanceVarValues[index],
			Class: o.HotIntegerInstanceVarClasses[index],
		}
		o.HotIntegerInstanceVarMask &^= bit
		o.HotIntegerInstanceVarClasses[index] = nil
	}
}

// SetInlineInstanceVar writes a previously resolved compact slot without
// repeating class/name lookup. It returns false for map-backed/overflow
// objects so callers can fall back to SetInstanceVar. The map mirror is kept
// coherent when a reflective API has already materialized it.
func (o *Object) SetInlineInstanceVar(index int, name string, value *EmeraldValue) bool {
	if o == nil || index < 0 || index >= inlineInstanceVarSlots || o.Class == nil ||
		!o.Class.CompactInstanceVars {
		return false
	}
	bit := uint8(1 << index)
	if o.InlineInstanceVarMask&bit == 0 {
		if o.InstanceVarOrder != nil {
			o.InstanceVarOrder = append(o.InstanceVarOrder, name)
		} else if o.InlineInstanceVarCount < inlineInstanceVarSlots {
			o.InlineInstanceVarNames[o.InlineInstanceVarCount] = name
			o.InlineInstanceVarCount++
		}
	}
	o.InlineInstanceVars[index] = value
	o.InlineInstanceVarMask |= bit
	if o.InstanceVars != nil {
		o.InstanceVars[name] = value
	}
	return true
}

// SetInlineInstanceVarFast is the constructor-batch sibling of
// SetInlineInstanceVar. The caller must already have proven that index is a
// class slot and that this object uses the compact layout. Keeping the proof
// out of the per-element method removes a class/slot map lookup from the hot
// allocation loop; a materialized map is still mirrored for reflective users.
func (o *Object) SetInlineInstanceVarFast(index int, name string, value *EmeraldValue) {
	if o == nil || index < 0 || index >= inlineInstanceVarSlots {
		return
	}
	bit := uint8(1 << index)
	if o.InlineInstanceVarMask&bit == 0 {
		if o.InstanceVarOrder != nil {
			o.InstanceVarOrder = append(o.InstanceVarOrder, name)
		} else if o.InlineInstanceVarCount < inlineInstanceVarSlots {
			o.InlineInstanceVarNames[o.InlineInstanceVarCount] = name
			o.InlineInstanceVarCount++
		}
	}
	o.InlineInstanceVars[index] = value
	o.InlineInstanceVarMask |= bit
	if o.InstanceVars != nil {
		o.InstanceVars[name] = value
	}
}

// InstanceVarMap materializes the compact slots only for APIs that require a
// map view (reflection, duplication, serializers). Normal VM ivar reads/writes
// stay on GetInstanceVar/SetInstanceVar and therefore do not pay this cost.
func (o *Object) InstanceVarMap() map[string]*EmeraldValue {
	if o == nil {
		return nil
	}
	o.flushHotIntegerInstanceVars()
	if o.InstanceVars == nil && o.InlineInstanceVarMask != 0 {
		o.InstanceVars = make(map[string]*EmeraldValue, len(o.InstanceVarOrder))
		if o.Class != nil {
			for name, index := range o.Class.InstanceVarSlots {
				if index < inlineInstanceVarSlots && o.InlineInstanceVarMask&(1<<index) != 0 {
					o.InstanceVars[name] = o.InlineInstanceVars[index]
				}
			}
		}
	}
	if o.InstanceVarOrder == nil && o.InlineInstanceVarCount != 0 {
		o.InstanceVarOrder = append([]string(nil), o.InlineInstanceVarNames[:o.InlineInstanceVarCount]...)
	}
	return o.InstanceVars
}

func (c *Class) instanceVarSlot(name string) (int, bool) {
	if c == nil || c.InstanceVarSlots == nil {
		return 0, false
	}
	index, ok := c.InstanceVarSlots[name]
	return int(index), ok
}

func (c *Class) ensureInstanceVarSlot(name string) (int, bool) {
	if c == nil || !c.CompactInstanceVars || name == "" {
		return 0, false
	}
	return c.ensureInstanceVarSlotForBatch(name)
}

func (c *Class) ensureInstanceVarSlotForBatch(name string) (int, bool) {
	if c == nil || !c.CompactInstanceVars || name == "" {
		return 0, false
	}
	if c.InstanceVarSlots == nil {
		c.InstanceVarSlots = make(map[string]uint8)
	}
	if index, ok := c.InstanceVarSlots[name]; ok {
		return int(index), true
	}
	if len(c.InstanceVarSlots) >= inlineInstanceVarSlots {
		return 0, false
	}
	index := uint8(len(c.InstanceVarSlots))
	c.InstanceVarSlots[name] = index
	return int(index), true
}

// EnsureInstanceVarSlot exposes the stable compact-layout slot allocator to
// the VM's constructor ABI. Existing slots never move; callers may cache the
// returned index for a class/function pair.
func (c *Class) EnsureInstanceVarSlot(name string) (int, bool) {
	return c.ensureInstanceVarSlot(name)
}

// PrepareBatchInstanceVarLayout reserves the small inline layout for a
// closed-world constructor batch. It is intentionally independent of the
// process-wide compact mode: a proven batch can use inline fields in default
// compatibility mode, while ordinary future allocations of the same class
// may still use maps. The operation is transactional with respect to the
// slot budget, so an oversized initializer remains on the map path.
func (c *Class) PrepareBatchInstanceVarLayout(names []string) bool {
	if c == nil {
		return false
	}
	needed := len(c.InstanceVarSlots)
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			return false
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		if _, exists := c.InstanceVarSlots[name]; !exists {
			needed++
		}
	}
	if needed > inlineInstanceVarSlots {
		return false
	}
	c.CompactInstanceVars = true
	for _, name := range names {
		if _, ok := c.ensureInstanceVarSlotForBatch(name); !ok {
			return false
		}
	}
	return true
}

func (o *Object) GetMethod(name string) (*Method, bool) {
	return o.Class.GetMethod(name)
}

func (o *Object) RespondTo(method string) bool {
	_, ok := o.GetMethod(method)
	return ok
}
