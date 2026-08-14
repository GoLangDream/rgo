package object

type Module struct {
	Name                string
	NameValue           *EmeraldValue
	TemporaryName       bool
	SyntheticNamespace  bool
	IsRefinement        bool
	RefinementTarget    *EmeraldValue
	RefinementOwner     *EmeraldValue
	Methods             map[string]*Method
	Constants           map[string]*EmeraldValue
	PrivateConstants    map[string]bool
	DeprecatedConstants map[string]bool
	Autoloads           map[string]string
	ConstantLocations   map[string]ConstantLocation
	AutoloadLocations   map[string]ConstantLocation
	ClassVars           map[string]*EmeraldValue
	ClassVarOrder       []string
	InstanceVars        map[string]*EmeraldValue
	Parent              *Module
	IncludedModules     []*Module
	PrependedModules    []*Module // Modules prepended via prepend
	SingletonClass      *Class
	UsedRefinements     []*EmeraldValue
}

func NewModule(name string) *Module {
	return &Module{
		Name:                name,
		Methods:             make(map[string]*Method),
		Constants:           make(map[string]*EmeraldValue),
		PrivateConstants:    make(map[string]bool),
		DeprecatedConstants: make(map[string]bool),
		Autoloads:           make(map[string]string),
		ConstantLocations:   make(map[string]ConstantLocation),
		AutoloadLocations:   make(map[string]ConstantLocation),
		ClassVars:           make(map[string]*EmeraldValue),
		InstanceVars:        make(map[string]*EmeraldValue),
	}
}

func (m *Module) DefineMethod(name string, method *Method) {
	m.Methods[name] = method
	BumpMethodGeneration()
}

func (m *Module) GetMethod(name string) (*Method, bool) {
	for _, prepended := range m.PrependedModules {
		if method, ok := prepended.GetMethod(name); ok {
			return method, true
		}
	}
	method, ok := m.Methods[name]
	if ok {
		return method, true
	}
	for i := len(m.IncludedModules) - 1; i >= 0; i-- {
		if method, ok := m.IncludedModules[i].GetMethod(name); ok {
			return method, true
		}
	}
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

func (m *Module) SetInstanceVar(name string, value *EmeraldValue) {
	m.InstanceVars[name] = value
}

func (m *Module) GetInstanceVar(name string) *EmeraldValue {
	return m.InstanceVars[name]
}

func (m *Module) Include(module *Module) {
	for _, included := range m.IncludedModules {
		if included == module {
			return
		}
	}
	m.IncludedModules = append(m.IncludedModules, module)
	for name, method := range module.Methods {
		if _, ok := m.Methods[name]; !ok {
			m.Methods[name] = method
		}
	}
	BumpMethodGeneration()
}

func (m *Module) Extend(module *Module) {
	for name, method := range module.Methods {
		m.Methods[name] = method
	}
	BumpMethodGeneration()
}

func (m *Module) Prepend(module *Module) {
	m.PrependedModules = append([]*Module{module}, m.PrependedModules...)
	BumpMethodGeneration()
}
