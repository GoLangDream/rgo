package rgo

import (
	"fmt"
	"reflect"
	"sync"
)

// MethodFunc 表示一个方法函数，统一接口
type MethodFunc func(self *RClass, args []any) any

// Method 表示一个方法
type Method struct {
	Name     string
	Function MethodFunc
	IsClass  bool // 是否为类方法
}

// RClass 表示一个 Ruby 风格的类
type RClass struct {
	name         string
	instanceVars map[string]any
	classVars    map[string]any
	parent       *RClass

	// 方法存储 - 使用简单的 map + RWMutex
	methods   map[string]*Method
	methodsMu sync.RWMutex

	// 方法查找缓存 - 简单的两级缓存
	methodCache   map[string]*Method
	cacheMu       sync.RWMutex
	cacheVersion  int64
	parentVersion int64

	// 元编程支持
	methodMissing  func(name string, args ...any) any
	beforeHooks    map[string][]func(args ...any)
	afterHooks     map[string][]func(result any, args ...any)
	aliasedMethods map[string]string

	// 变量访问锁
	varMu sync.RWMutex

	// 元类支持
	eigenClass *RClass
	eigenMu    sync.RWMutex

	// 对象池支持
	pooled bool
}

// 全局对象池
var classPool = sync.Pool{
	New: func() any {
		return &RClass{
			instanceVars:   make(map[string]any),
			methods:        make(map[string]*Method),
			methodCache:    make(map[string]*Method),
			beforeHooks:    make(map[string][]func(args ...any)),
			afterHooks:     make(map[string][]func(result any, args ...any)),
			aliasedMethods: make(map[string]string),
		}
	},
}

// Class 创建一个新的类
func Class(name string) *RClass {
	return &RClass{
		name:           name,
		instanceVars:   make(map[string]any),
		classVars:      make(map[string]any),
		methods:        make(map[string]*Method),
		methodCache:    make(map[string]*Method),
		beforeHooks:    make(map[string][]func(args ...any)),
		afterHooks:     make(map[string][]func(result any, args ...any)),
		aliasedMethods: make(map[string]string),
	}
}

// wrapMethod 将各种类型的函数包装为统一的 MethodFunc
func wrapMethod(fn any) MethodFunc {
	if fn == nil {
		panic("method function cannot be nil")
	}

	// 如果已经是 MethodFunc，直接返回
	if mf, ok := fn.(MethodFunc); ok {
		return mf
	}

	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		panic("method must be a function")
	}

	t := v.Type()
	numIn := t.NumIn()
	numOut := t.NumOut()

	// 预检查一些常见的函数签名，提供快速路径
	switch fn := fn.(type) {
	case func() any:
		return func(self *RClass, args []any) any {
			if len(args) != 0 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 0)", len(args)))
			}
			return fn()
		}
	case func() string:
		return func(self *RClass, args []any) any {
			if len(args) != 0 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 0)", len(args)))
			}
			return fn()
		}
	case func() int:
		return func(self *RClass, args []any) any {
			if len(args) != 0 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 0)", len(args)))
			}
			return fn()
		}
	case func(any) any:
		return func(self *RClass, args []any) any {
			if len(args) != 1 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 1)", len(args)))
			}
			return fn(args[0])
		}
	case func(any) string:
		return func(self *RClass, args []any) any {
			if len(args) != 1 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 1)", len(args)))
			}
			return fn(args[0])
		}
	case func(any, any) any:
		return func(self *RClass, args []any) any {
			if len(args) != 2 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 2)", len(args)))
			}
			return fn(args[0], args[1])
		}
	case func(*RClass) any:
		return func(self *RClass, args []any) any {
			if len(args) != 0 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 0)", len(args)))
			}
			return fn(self)
		}
	case func(*RClass) string:
		return func(self *RClass, args []any) any {
			if len(args) != 0 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 0)", len(args)))
			}
			return fn(self)
		}
	case func(*RClass, any) any:
		return func(self *RClass, args []any) any {
			if len(args) != 1 {
				panic(fmt.Sprintf("wrong number of arguments (given %d, expected 1)", len(args)))
			}
			return fn(self, args[0])
		}
	}

	// 通用反射实现
	return func(self *RClass, args []any) any {
		if numIn != len(args) {
			panic(fmt.Sprintf("wrong number of arguments (given %d, expected %d)", len(args), numIn))
		}

		// 准备参数
		in := make([]reflect.Value, numIn)
		for i, arg := range args {
			in[i] = reflect.ValueOf(arg)
		}

		// 调用函数
		out := v.Call(in)

		// 返回结果
		if numOut == 0 {
			return nil
		}
		return out[0].Interface()
	}
}

// DefineMethod 定义实例方法
func (c *RClass) DefineMethod(name string, fn any) *RClass {
	method := &Method{
		Name:     name,
		Function: wrapMethod(fn),
		IsClass:  false,
	}

	c.methodsMu.Lock()
	c.methods[name] = method
	c.cacheVersion++
	c.methodsMu.Unlock()

	c.invalidateCache()
	return c
}

// DefineClassMethod 定义类方法
func (c *RClass) DefineClassMethod(name string, fn any) *RClass {
	method := &Method{
		Name:     name,
		Function: wrapMethod(fn),
		IsClass:  true,
	}

	c.methodsMu.Lock()
	c.methods[name] = method
	c.cacheVersion++
	c.methodsMu.Unlock()

	c.invalidateCache()
	return c
}

// New 创建类的新实例
func (c *RClass) New() *RClass {
	var instance *RClass
	if c.pooled {
		instance = classPool.Get().(*RClass)
		// 清理状态
		instance.clearForReuse()
	} else {
		instance = &RClass{
			instanceVars:   make(map[string]any),
			methods:        make(map[string]*Method),
			methodCache:    make(map[string]*Method),
			beforeHooks:    make(map[string][]func(args ...any)),
			afterHooks:     make(map[string][]func(result any, args ...any)),
			aliasedMethods: make(map[string]string),
		}
	}

	instance.name = c.name
	instance.classVars = c.classVars // 共享类变量
	instance.parent = c
	instance.methodMissing = c.methodMissing

	return instance
}

// clearForReuse 清理实例以便重用
func (c *RClass) clearForReuse() {
	// 清理实例变量
	for k := range c.instanceVars {
		delete(c.instanceVars, k)
	}
	// 清理方法
	for k := range c.methods {
		delete(c.methods, k)
	}
	// 清理缓存
	for k := range c.methodCache {
		delete(c.methodCache, k)
	}
	// 清理钩子
	for k := range c.beforeHooks {
		delete(c.beforeHooks, k)
	}
	for k := range c.afterHooks {
		delete(c.afterHooks, k)
	}
	// 清理别名
	for k := range c.aliasedMethods {
		delete(c.aliasedMethods, k)
	}

	c.cacheVersion = 0
	c.parentVersion = 0
	c.eigenClass = nil
}

// Call 调用方法
func (c *RClass) Call(methodName string, args ...any) any {
	// 处理方法别名 - 在继承链中查找
	originalMethodName := methodName
	methodName = c.resolveAlias(methodName)

	// 执行 before hooks - 在继承链中查找
	c.executeBefore(originalMethodName, args...)

	// 查找并调用方法
	method := c.findMethod(methodName)
	if method == nil {
		if c.methodMissing != nil {
			result := c.methodMissing(methodName, args...)
			// 执行 after hooks
			c.executeAfter(originalMethodName, result, args...)
			return result
		}
		panic(fmt.Sprintf("undefined method '%s' for %s", methodName, c.name))
	}

	result := method.Function(c, args)

	// 执行 after hooks - 在继承链中查找
	c.executeAfter(originalMethodName, result, args...)

	return result
}

// resolveAlias 在继承链中解析别名
func (c *RClass) resolveAlias(methodName string) string {
	current := c
	for current != nil {
		if alias, exists := current.aliasedMethods[methodName]; exists {
			return alias
		}
		current = current.parent
	}
	return methodName
}

// executeBefore 在继承链中执行 before hooks
func (c *RClass) executeBefore(methodName string, args ...any) {
	current := c
	for current != nil {
		if hooks, exists := current.beforeHooks[methodName]; exists {
			for _, hook := range hooks {
				hook(args...)
			}
		}
		current = current.parent
	}
}

// executeAfter 在继承链中执行 after hooks
func (c *RClass) executeAfter(methodName string, result any, args ...any) {
	current := c
	for current != nil {
		if hooks, exists := current.afterHooks[methodName]; exists {
			for _, hook := range hooks {
				hook(result, args...)
			}
		}
		current = current.parent
	}
}

// FastCall 快速方法调用（跳过钩子）
func (c *RClass) FastCall(methodName string, args ...any) any {
	// 处理方法别名 - 在继承链中查找
	methodName = c.resolveAlias(methodName)

	method := c.findMethod(methodName)
	if method == nil {
		if c.methodMissing != nil {
			return c.methodMissing(methodName, args...)
		}
		panic(fmt.Sprintf("undefined method '%s' for %s", methodName, c.name))
	}

	return method.Function(c, args)
}

// findMethod 查找方法（带缓存）
func (c *RClass) findMethod(methodName string) *Method {
	// 检查缓存是否有效
	parentVer := int64(0)
	if c.parent != nil {
		c.parent.methodsMu.RLock()
		parentVer = c.parent.cacheVersion
		c.parent.methodsMu.RUnlock()
	}

	c.cacheMu.RLock()
	cacheValid := c.parentVersion == parentVer
	if cacheValid {
		if method, exists := c.methodCache[methodName]; exists {
			c.cacheMu.RUnlock()
			return method
		}
	}
	c.cacheMu.RUnlock()

	// 缓存无效或未命中，重新查找
	method := c.findMethodRecursive(methodName)

	// 更新缓存
	c.cacheMu.Lock()
	if !cacheValid {
		// 清空缓存
		c.methodCache = make(map[string]*Method)
		c.parentVersion = parentVer
	}
	c.methodCache[methodName] = method
	c.cacheMu.Unlock()

	return method
}

// findMethodRecursive 递归查找方法
func (c *RClass) findMethodRecursive(methodName string) *Method {
	// 首先在元类中查找（如果存在）
	c.eigenMu.RLock()
	if c.eigenClass != nil {
		eigenClass := c.eigenClass
		c.eigenMu.RUnlock()

		// 元类只查找自己的方法，不递归
		eigenClass.methodsMu.RLock()
		if method, exists := eigenClass.methods[methodName]; exists {
			eigenClass.methodsMu.RUnlock()
			return method
		}
		eigenClass.methodsMu.RUnlock()
	} else {
		c.eigenMu.RUnlock()
	}

	// 在当前类中查找
	c.methodsMu.RLock()
	if method, exists := c.methods[methodName]; exists {
		c.methodsMu.RUnlock()
		return method
	}
	c.methodsMu.RUnlock()

	// 在父类中查找
	if c.parent != nil {
		return c.parent.findMethodRecursive(methodName)
	}

	return nil
}

// invalidateCache 使缓存失效
func (c *RClass) invalidateCache() {
	c.cacheMu.Lock()
	c.methodCache = make(map[string]*Method)
	c.cacheVersion++
	c.cacheMu.Unlock()
}

// AttrAccessor 定义属性访问器
func (c *RClass) AttrAccessor(names ...string) *RClass {
	for _, name := range names {
		varName := name
		// getter
		c.DefineMethod(name, func(self *RClass) any {
			return self.GetInstanceVar(varName)
		})
		// setter
		c.DefineMethod(name+"=", func(self *RClass, value any) any {
			self.SetInstanceVar(varName, value)
			return value
		})
	}
	return c
}

// AttrReader 定义只读属性
func (c *RClass) AttrReader(names ...string) *RClass {
	for _, name := range names {
		varName := name
		c.DefineMethod(name, func(self *RClass) any {
			return self.GetInstanceVar(varName)
		})
	}
	return c
}

// AttrWriter 定义只写属性
func (c *RClass) AttrWriter(names ...string) *RClass {
	for _, name := range names {
		varName := name
		c.DefineMethod(name+"=", func(self *RClass, value any) any {
			self.SetInstanceVar(varName, value)
			return value
		})
	}
	return c
}

// SetInstanceVar 设置实例变量
func (c *RClass) SetInstanceVar(name string, value any) {
	c.varMu.Lock()
	c.instanceVars[name] = value
	c.varMu.Unlock()
}

// GetInstanceVar 获取实例变量
func (c *RClass) GetInstanceVar(name string) any {
	c.varMu.RLock()
	value := c.instanceVars[name]
	c.varMu.RUnlock()
	return value
}

// SetClassVar 设置类变量
func (c *RClass) SetClassVar(name string, value any) {
	c.varMu.Lock()
	if c.classVars == nil {
		c.classVars = make(map[string]any)
	}
	c.classVars[name] = value
	c.varMu.Unlock()
}

// GetClassVar 获取类变量
func (c *RClass) GetClassVar(name string) any {
	c.varMu.RLock()
	var value any
	if c.classVars != nil {
		value = c.classVars[name]
	}
	c.varMu.RUnlock()
	return value
}

// MethodMissing 设置方法缺失处理器
func (c *RClass) MethodMissing(handler func(name string, args ...any) any) *RClass {
	c.methodMissing = handler
	return c
}

// Inherit 继承另一个类
func (c *RClass) Inherit(parent *RClass) *RClass {
	c.parent = parent
	c.classVars = parent.classVars // 共享类变量
	c.invalidateCache()
	return c
}

// RespondTo 检查是否响应某个方法
func (c *RClass) RespondTo(methodName string) bool {
	return c.findMethod(methodName) != nil
}

// Methods 返回所有可用的方法名
func (c *RClass) Methods() []string {
	methodSet := make(map[string]bool)
	c.collectMethods(methodSet)

	names := make([]string, 0, len(methodSet))
	for name := range methodSet {
		names = append(names, name)
	}
	return names
}

// collectMethods 递归收集方法名
func (c *RClass) collectMethods(methodSet map[string]bool) {
	c.methodsMu.RLock()
	for name := range c.methods {
		methodSet[name] = true
	}
	c.methodsMu.RUnlock()

	if c.parent != nil {
		c.parent.collectMethods(methodSet)
	}
}

// ClassMethods 返回所有可用的类方法名
func (c *RClass) ClassMethods() []string {
	c.methodsMu.RLock()
	defer c.methodsMu.RUnlock()

	var names []string
	for name, method := range c.methods {
		if method.IsClass {
			names = append(names, name)
		}
	}
	return names
}

// InstanceVars 返回所有实例变量名
func (c *RClass) InstanceVars() []string {
	c.varMu.RLock()
	names := make([]string, 0, len(c.instanceVars))
	for name := range c.instanceVars {
		names = append(names, name)
	}
	c.varMu.RUnlock()
	return names
}

// ClassVars 返回所有类变量名
func (c *RClass) ClassVars() []string {
	c.varMu.RLock()
	var names []string
	if c.classVars != nil {
		names = make([]string, 0, len(c.classVars))
		for name := range c.classVars {
			names = append(names, name)
		}
	}
	c.varMu.RUnlock()
	return names
}

// Name 返回类名
func (c *RClass) Name() string {
	return c.name
}

// Parent 返回父类
func (c *RClass) Parent() *RClass {
	return c.parent
}

// IsA 检查是否为指定类的实例
func (c *RClass) IsA(className string) bool {
	if c.name == className {
		return true
	}
	if c.parent != nil {
		return c.parent.IsA(className)
	}
	return false
}

// Super 调用父类的同名方法
func (c *RClass) Super(methodName string, args ...any) any {
	// 找到当前方法定义的类
	currentClass := c.findClassDefining(methodName)
	if currentClass == nil || currentClass.parent == nil {
		panic("no superclass method '" + methodName + "'")
	}

	// 从当前方法定义类的父类开始查找
	method := currentClass.parent.findMethodRecursive(methodName)
	if method == nil {
		panic(fmt.Sprintf("undefined method '%s' for %s", methodName, currentClass.parent.name))
	}

	return method.Function(c, args)
}

// findClassDefining 找到定义指定方法的类
func (c *RClass) findClassDefining(methodName string) *RClass {
	// 从当前类开始向上查找
	current := c
	for current != nil {
		current.methodsMu.RLock()
		_, exists := current.methods[methodName]
		current.methodsMu.RUnlock()

		if exists {
			return current
		}

		current = current.parent
	}
	return nil
}

// === 元编程扩展方法 ===

// Alias 为方法创建别名
func (c *RClass) Alias(newName, oldName string) *RClass {
	c.aliasedMethods[newName] = oldName
	return c
}

// Before 添加方法调用前的钩子
func (c *RClass) Before(methodName string, hook func(args ...any)) *RClass {
	if c.beforeHooks[methodName] == nil {
		c.beforeHooks[methodName] = make([]func(args ...any), 0, 1)
	}
	c.beforeHooks[methodName] = append(c.beforeHooks[methodName], hook)
	return c
}

// After 添加方法调用后的钩子
func (c *RClass) After(methodName string, hook func(result any, args ...any)) *RClass {
	if c.afterHooks[methodName] == nil {
		c.afterHooks[methodName] = make([]func(result any, args ...any), 0, 1)
	}
	c.afterHooks[methodName] = append(c.afterHooks[methodName], hook)
	return c
}

// EigenClass 获取元类（单例类）
func (c *RClass) EigenClass() *RClass {
	c.eigenMu.RLock()
	if c.eigenClass != nil {
		defer c.eigenMu.RUnlock()
		return c.eigenClass
	}
	c.eigenMu.RUnlock()

	c.eigenMu.Lock()
	defer c.eigenMu.Unlock()

	// 双重检查
	if c.eigenClass != nil {
		return c.eigenClass
	}

	c.eigenClass = Class(c.name + ":EigenClass")
	c.eigenClass.parent = c
	return c.eigenClass
}

// DefineEigenMethod 为单个对象定义方法
func (c *RClass) DefineEigenMethod(name string, fn any) *RClass {
	eigenClass := c.EigenClass()
	eigenClass.DefineMethod(name, fn)
	// 重要：清空实例的方法缓存，因为元类方法优先级最高
	c.invalidateCache()
	return c
}

// EnablePooling 启用对象池
func (c *RClass) EnablePooling() *RClass {
	c.pooled = true
	return c
}

// ReturnToPool 将对象返回到池中
func (c *RClass) ReturnToPool() {
	if c.pooled {
		c.clearForReuse()
		classPool.Put(c)
	}
}

// GetMethodDirectly 直接获取方法
func (c *RClass) GetMethodDirectly(methodName string) *Method {
	return c.findMethod(methodName)
}

// CallDirect 直接调用方法
func (m *Method) CallDirect(self *RClass, args ...any) any {
	return m.Function(self, args)
}
