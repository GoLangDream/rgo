package object

type Module struct {
	Name      string
	Methods   map[string]*Method
	Constants map[string]*EmeraldValue
	Parent    *Module
}

func NewModule(name string) *Module {
	return &Module{
		Name:      name,
		Methods:   make(map[string]*Method),
		Constants: make(map[string]*EmeraldValue),
	}
}

func (m *Module) DefineMethod(name string, method *Method) {
	m.Methods[name] = method
}

func (m *Module) GetMethod(name string) (*Method, bool) {
	method, ok := m.Methods[name]
	if !ok && m.Parent != nil {
		return m.Parent.GetMethod(name)
	}
	return method, ok
}

func (m *Module) DefineConstant(name string, value *EmeraldValue) {
	m.Constants[name] = value
}

func (m *Module) GetConstant(name string) (*EmeraldValue, bool) {
	val, ok := m.Constants[name]
	if !ok && m.Parent != nil {
		return m.Parent.GetConstant(name)
	}
	return val, ok
}

func (m *Module) Include(module *Module) {
	for name, method := range module.Methods {
		if _, ok := m.Methods[name]; !ok {
			m.Methods[name] = method
		}
	}
}

func (m *Module) Extend(module *Module) {
	for name, method := range module.Methods {
		m.Methods[name] = method
	}
}
