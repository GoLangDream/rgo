package rgo

import (
	"fmt"
	"reflect"
	"sync"
)

// RClass 表示一个 Ruby 风格的类
type RClass struct {
	name          string
	methods       map[string]any
	classMethods  map[string]any
	instanceVars  map[string]any
	classVars     map[string]any
	parent        *RClass
	methodMissing func(name string, args ...any) any
	mu            sync.RWMutex
}

// Class 创建一个新的类
func Class(name string) *RClass {
	return &RClass{
		name:         name,
		methods:      make(map[string]any),
		classMethods: make(map[string]any),
		instanceVars: make(map[string]any),
		classVars:    make(map[string]any),
	}
}

// DefineMethod 定义实例方法
func (c *RClass) DefineMethod(name string, method any) *RClass {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methods[name] = method
	return c
}

// DefineClassMethod 定义类方法
func (c *RClass) DefineClassMethod(name string, method any) *RClass {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.classMethods[name] = method
	return c
}

// New 创建类的新实例
func (c *RClass) New() *RClass {
	instance := &RClass{
		name:          c.name,
		methods:       make(map[string]any),
		classMethods:  c.classMethods,
		instanceVars:  make(map[string]any),
		classVars:     c.classVars,
		parent:        c,
		methodMissing: c.methodMissing,
	}

	// 复制父类的方法
	for name, method := range c.methods {
		instance.methods[name] = method
	}

	return instance
}

// Call 调用方法
func (c *RClass) Call(methodName string, args ...any) any {
	method, exists := c.findMethod(methodName)
	if !exists {
		if c.methodMissing != nil {
			return c.methodMissing(methodName, args...)
		}
		panic(fmt.Sprintf("undefined method '%s' for %s", methodName, c.name))
	}

	return c.callMethod(method, args...)
}

// findMethod 查找方法
func (c *RClass) findMethod(methodName string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 先在当前类中查找
	method, exists := c.methods[methodName]
	if !exists {
		method, exists = c.classMethods[methodName]
	}

	// 如果在当前类中没找到，且存在父类，则在父类中查找
	if !exists && c.parent != nil {
		return c.parent.findMethod(methodName)
	}

	return method, exists
}

// callMethod 调用方法
func (c *RClass) callMethod(method any, args ...any) any {
	methodValue := reflect.ValueOf(method)
	methodType := methodValue.Type()

	if methodType.NumIn() != len(args) {
		panic(fmt.Sprintf("wrong number of arguments (given %d, expected %d)", len(args), methodType.NumIn()))
	}

	callArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		callArgs[i] = reflect.ValueOf(arg)
	}

	results := methodValue.Call(callArgs)
	if len(results) == 0 {
		return nil
	}
	return results[0].Interface()
}

// AttrAccessor 定义属性访问器
func (c *RClass) AttrAccessor(names ...string) *RClass {
	for _, name := range names {
		c.DefineMethod(name, func(self *RClass) any {
			return self.GetInstanceVar(name)
		})
		c.DefineMethod(name+"=", func(self *RClass, value any) any {
			self.SetInstanceVar(name, value)
			return value
		})
	}
	return c
}

// AttrReader 定义只读属性
func (c *RClass) AttrReader(names ...string) *RClass {
	for _, name := range names {
		c.DefineMethod(name, func(self *RClass) any {
			return self.GetInstanceVar(name)
		})
	}
	return c
}

// AttrWriter 定义只写属性
func (c *RClass) AttrWriter(names ...string) *RClass {
	for _, name := range names {
		c.DefineMethod(name+"=", func(self *RClass, value any) any {
			self.SetInstanceVar(name, value)
			return value
		})
	}
	return c
}

// SetInstanceVar 设置实例变量
func (c *RClass) SetInstanceVar(name string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instanceVars[name] = value
}

// GetInstanceVar 获取实例变量
func (c *RClass) GetInstanceVar(name string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.instanceVars[name]
}

// SetClassVar 设置类变量
func (c *RClass) SetClassVar(name string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.classVars[name] = value
}

// GetClassVar 获取类变量
func (c *RClass) GetClassVar(name string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.classVars[name]
}

// MethodMissing 设置方法缺失处理器
func (c *RClass) MethodMissing(handler func(name string, args ...any) any) *RClass {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methodMissing = handler
	return c
}

// Inherit 继承另一个类
func (c *RClass) Inherit(parent *RClass) *RClass {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parent = parent

	// 复制父类的方法
	for name, method := range parent.methods {
		if _, exists := c.methods[name]; !exists {
			c.methods[name] = method
		}
	}
	return c
}

// RespondTo 检查是否响应某个方法
func (c *RClass) RespondTo(methodName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.methods[methodName]
	return exists
}

// Methods 返回所有可用的方法名
func (c *RClass) Methods() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getMethodNames(c.methods)
}

// ClassMethods 返回所有可用的类方法名
func (c *RClass) ClassMethods() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getMethodNames(c.classMethods)
}

// getMethodNames 获取方法名列表
func (c *RClass) getMethodNames(methods map[string]any) []string {
	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	return names
}

// InstanceVars 返回所有实例变量名
func (c *RClass) InstanceVars() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getVarNames(c.instanceVars)
}

// ClassVars 返回所有类变量名
func (c *RClass) ClassVars() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getVarNames(c.classVars)
}

// getVarNames 获取变量名列表
func (c *RClass) getVarNames(vars map[string]any) []string {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
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
	if c.parent == nil {
		panic("no superclass method '" + methodName + "'")
	}

	c.mu.RLock()
	method, exists := c.parent.methods[methodName]
	c.mu.RUnlock()

	if !exists {
		panic(fmt.Sprintf("undefined method '%s' for %s", methodName, c.parent.name))
	}

	return c.callMethod(method, args...)
}
