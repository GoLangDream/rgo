package object

type Class struct {
	Name         string
	SuperClass   *Class
	Methods      map[string]*Method
	Constants    map[string]*EmeraldValue
	ClassMethods map[string]*Method
	InstanceVars map[string]*EmeraldValue
	IsSingleton  bool
}

func NewClass(name string) *Class {
	return &Class{
		Name:         name,
		Methods:      make(map[string]*Method),
		Constants:    make(map[string]*EmeraldValue),
		ClassMethods: make(map[string]*Method),
		InstanceVars: make(map[string]*EmeraldValue),
	}
}

func (c *Class) DefineMethod(name string, method *Method) {
	c.Methods[name] = method
}

func (c *Class) DefineClassMethod(name string, method *Method) {
	c.ClassMethods[name] = method
}

func (c *Class) GetMethod(name string) (*Method, bool) {
	method, ok := c.Methods[name]
	if !ok && c.SuperClass != nil {
		return c.SuperClass.GetMethod(name)
	}
	return method, ok
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

func (c *Class) NewInstance() *EmeraldValue {
	return &EmeraldValue{
		Type:  ValueObject,
		Data:  NewObject(c),
		Class: c,
	}
}

type Object struct {
	Class       *Class
	InstanceVars map[string]*EmeraldValue
}

func NewObject(class *Class) *Object {
	return &Object{
		Class:       class,
		InstanceVars: make(map[string]*EmeraldValue),
	}
}

func (o *Object) GetInstanceVar(name string) *EmeraldValue {
	if val, ok := o.InstanceVars[name]; ok {
		return val
	}
	return nil
}

func (o *Object) SetInstanceVar(name string, value *EmeraldValue) {
	o.InstanceVars[name] = value
}

func (o *Object) GetMethod(name string) (*Method, bool) {
	return o.Class.GetMethod(name)
}

func (o *Object) RespondTo(method string) bool {
	_, ok := o.GetMethod(method)
	return ok
}
