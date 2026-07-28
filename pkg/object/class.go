package object

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
	c.IncludedModules = append(c.IncludedModules, module)
	BumpMethodGeneration()
}

func (c *Class) Extend(module *Module) {
	for name, method := range module.Methods {
		c.DefineClassMethod(name, method)
	}
}

func (c *Class) Prepend(module *Module) {
	c.PrependedModules = append([]*Module{module}, c.PrependedModules...)
	BumpMethodGeneration()
}

func (c *Class) NewInstance() *EmeraldValue {
	return &EmeraldValue{
		Type:  ValueObject,
		Data:  NewObject(c),
		Class: c,
	}
}

type Object struct {
	Class            *Class
	InstanceVars     map[string]*EmeraldValue
	InstanceVarOrder []string
	ClassVars        map[string]*EmeraldValue
	SingletonMethods map[string]*Method
	SingletonClass   *Class
	StructValues     []*EmeraldValue
}

func NewObject(class *Class) *Object {
	return &Object{
		Class:            class,
		InstanceVars:     make(map[string]*EmeraldValue),
		ClassVars:        make(map[string]*EmeraldValue),
		SingletonMethods: make(map[string]*Method),
	}
}

func (o *Object) GetInstanceVar(name string) *EmeraldValue {
	if val, ok := o.InstanceVars[name]; ok {
		return val
	}
	return nil
}

func (o *Object) SetInstanceVar(name string, value *EmeraldValue) {
	if _, exists := o.InstanceVars[name]; !exists {
		o.InstanceVarOrder = append(o.InstanceVarOrder, name)
	}
	o.InstanceVars[name] = value
}

func (o *Object) GetMethod(name string) (*Method, bool) {
	return o.Class.GetMethod(name)
}

func (o *Object) RespondTo(method string) bool {
	_, ok := o.GetMethod(method)
	return ok
}
