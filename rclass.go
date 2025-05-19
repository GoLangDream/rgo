package rgo

import (
	"fmt"
	"reflect"
	"sync"
)

// RClass 表示一个 Ruby 风格的类
type RClass struct {
	name          string
	methods       map[string]interface{}
	instanceVars  map[string]interface{}
	classVars     map[string]interface{}
	parent        *RClass
	methodMissing func(name string, args ...interface{}) interface{}
	mu            sync.RWMutex
}

// RClassBuilder 用于构建新的类
func RClassBuilder(name string, block func(*RClass)) *RClass {
	class := &RClass{
		name:         name,
		methods:      make(map[string]interface{}),
		instanceVars: make(map[string]interface{}),
		classVars:    make(map[string]interface{}),
	}

	if block != nil {
		block(class)
	}

	return class
}

// RDefineMethod 定义实例方法
func RDefineMethod(c *RClass, name string, method interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methods[name] = method
}

// RDefineClassMethod 定义类方法
func RDefineClassMethod(c *RClass, name string, method interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methods[name] = method
}

// New 创建类的新实例
func (c *RClass) New() *RClass {
	instance := &RClass{
		name:          c.name,
		methods:       make(map[string]interface{}),
		instanceVars:  make(map[string]interface{}),
		classVars:     c.classVars,
		parent:        c,
		methodMissing: c.methodMissing, // 复制 methodMissing 处理器
	}

	// 复制父类的方法
	for name, method := range c.methods {
		instance.methods[name] = method
	}

	return instance
}

// Call 调用方法
func (c *RClass) Call(methodName string, args ...interface{}) interface{} {
	c.mu.RLock()
	method, exists := c.methods[methodName]
	c.mu.RUnlock()

	if !exists {
		if c.methodMissing != nil {
			return c.methodMissing(methodName, args...)
		}
		panic(fmt.Sprintf("undefined method '%s' for %s", methodName, c.name))
	}

	// 使用反射调用方法
	methodValue := reflect.ValueOf(method)
	methodType := methodValue.Type()

	// 检查参数数量
	if methodType.NumIn() != len(args) {
		panic(fmt.Sprintf("wrong number of arguments (given %d, expected %d)", len(args), methodType.NumIn()))
	}

	// 转换参数类型
	callArgs := make([]reflect.Value, len(args))
	for i, arg := range args {
		callArgs[i] = reflect.ValueOf(arg)
	}

	// 调用方法
	results := methodValue.Call(callArgs)

	// 处理返回值
	if len(results) == 0 {
		return nil
	}
	return results[0].Interface()
}

// SetInstanceVar 设置实例变量
func SetInstanceVar(c *RClass, name string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instanceVars[name] = value
}

// GetInstanceVar 获取实例变量
func GetInstanceVar(c *RClass, name string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.instanceVars[name]
}

// SetClassVar 设置类变量
func SetClassVar(c *RClass, name string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.classVars[name] = value
}

// GetClassVar 获取类变量
func GetClassVar(c *RClass, name string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.classVars[name]
}

// SetMethodMissing 设置方法缺失处理器
func SetMethodMissing(c *RClass, handler func(name string, args ...interface{}) interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methodMissing = handler
}

// Inherit 继承另一个类
func (c *RClass) Inherit(parent *RClass) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parent = parent

	// 复制父类的方法
	for name, method := range parent.methods {
		if _, exists := c.methods[name]; !exists {
			c.methods[name] = method
		}
	}
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
	methods := make([]string, 0, len(c.methods))
	for name := range c.methods {
		methods = append(methods, name)
	}
	return methods
}
