package rgo

import (
	"fmt"
	"reflect"
	"sync"
)

// MethodFunc 统一的方法函数类型
type MethodFunc func(receiver any, args ...any) any

// CachedMethod 缓存的方法信息
type CachedMethod struct {
	fn      MethodFunc
	numArgs int
}

// RClassOptimized 优化版本的 RClass
type RClassOptimized struct {
	name          string
	methods       map[string]*CachedMethod
	classMethods  map[string]*CachedMethod
	instanceVars  map[string]any
	classVars     map[string]any
	parent        *RClassOptimized
	methodMissing func(name string, args ...any) any
	mu            sync.RWMutex

	// 缓存优化
	methodCache map[string]*CachedMethod
	cacheMu     sync.RWMutex
}

// ClassOptimized 创建一个新的优化类
func ClassOptimized(name string) *RClassOptimized {
	return &RClassOptimized{
		name:         name,
		methods:      make(map[string]*CachedMethod),
		classMethods: make(map[string]*CachedMethod),
		instanceVars: make(map[string]any),
		classVars:    make(map[string]any),
		methodCache:  make(map[string]*CachedMethod),
	}
}

// DefineMethod 定义实例方法（优化版本）
func (c *RClassOptimized) DefineMethod(name string, method any) *RClassOptimized {
	c.mu.Lock()
	defer c.mu.Unlock()

	cached := c.wrapMethod(method)
	c.methods[name] = cached

	// 清除缓存
	c.clearCache()

	return c
}

// DefineClassMethod 定义类方法（优化版本）
func (c *RClassOptimized) DefineClassMethod(name string, method any) *RClassOptimized {
	c.mu.Lock()
	defer c.mu.Unlock()

	cached := c.wrapMethod(method)
	c.classMethods[name] = cached

	// 清除缓存
	c.clearCache()

	return c
}

// wrapMethod 包装方法以减少反射使用
func (c *RClassOptimized) wrapMethod(method any) *CachedMethod {
	methodValue := reflect.ValueOf(method)
	methodType := methodValue.Type()
	numArgs := methodType.NumIn()

	// 针对常见的方法签名进行优化
	switch fn := method.(type) {
	case func() any:
		return &CachedMethod{
			fn: func(receiver any, args ...any) any {
				return fn()
			},
			numArgs: 0,
		}
	case func(any) any:
		return &CachedMethod{
			fn: func(receiver any, args ...any) any {
				if len(args) != 1 {
					panic(fmt.Sprintf("wrong number of arguments (given %d, expected 1)", len(args)))
				}
				return fn(args[0])
			},
			numArgs: 1,
		}
	case func(any, any) any:
		return &CachedMethod{
			fn: func(receiver any, args ...any) any {
				if len(args) != 2 {
					panic(fmt.Sprintf("wrong number of arguments (given %d, expected 2)", len(args)))
				}
				return fn(args[0], args[1])
			},
			numArgs: 2,
		}
	case func(*RClassOptimized) any:
		return &CachedMethod{
			fn: func(receiver any, args ...any) any {
				return fn(receiver.(*RClassOptimized))
			},
			numArgs: 0,
		}
	case func(*RClassOptimized, any) any:
		return &CachedMethod{
			fn: func(receiver any, args ...any) any {
				if len(args) != 1 {
					panic(fmt.Sprintf("wrong number of arguments (given %d, expected 1)", len(args)))
				}
				return fn(receiver.(*RClassOptimized), args[0])
			},
			numArgs: 1,
		}
	default:
		// 回退到反射实现
		return &CachedMethod{
			fn: func(receiver any, args ...any) any {
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
			},
			numArgs: numArgs,
		}
	}
}

// New 创建类的新实例（优化版本）
func (c *RClassOptimized) New() *RClassOptimized {
	instance := &RClassOptimized{
		name:          c.name,
		methods:       make(map[string]*CachedMethod),
		classMethods:  c.classMethods,
		instanceVars:  make(map[string]any),
		classVars:     c.classVars,
		parent:        c,
		methodMissing: c.methodMissing,
		methodCache:   make(map[string]*CachedMethod),
	}

	// 复制父类的方法
	c.mu.RLock()
	for name, method := range c.methods {
		instance.methods[name] = method
	}
	c.mu.RUnlock()

	return instance
}

// Call 调用方法（优化版本）
func (c *RClassOptimized) Call(methodName string, args ...any) any {
	method := c.findMethodCached(methodName)
	if method == nil {
		if c.methodMissing != nil {
			return c.methodMissing(methodName, args...)
		}
		panic(fmt.Sprintf("undefined method '%s' for %s", methodName, c.name))
	}

	return method.fn(c, args...)
}

// findMethodCached 查找方法（带缓存）
func (c *RClassOptimized) findMethodCached(methodName string) *CachedMethod {
	// 先查缓存
	c.cacheMu.RLock()
	if method, exists := c.methodCache[methodName]; exists {
		c.cacheMu.RUnlock()
		return method
	}
	c.cacheMu.RUnlock()

	// 查找方法
	method := c.findMethod(methodName)

	// 缓存结果（包括 nil）
	c.cacheMu.Lock()
	c.methodCache[methodName] = method
	c.cacheMu.Unlock()

	return method
}

// findMethod 查找方法（优化版本）
func (c *RClassOptimized) findMethod(methodName string) *CachedMethod {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// 先在当前类中查找
	if method, exists := c.methods[methodName]; exists {
		return method
	}
	if method, exists := c.classMethods[methodName]; exists {
		return method
	}

	// 如果在当前类中没找到，且存在父类，则在父类中查找
	if c.parent != nil {
		return c.parent.findMethod(methodName)
	}

	return nil
}

// clearCache 清除方法缓存
func (c *RClassOptimized) clearCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	// 使用高效的 map 清空方式
	for k := range c.methodCache {
		delete(c.methodCache, k)
	}
}

// AttrAccessor 定义属性访问器（优化版本）
func (c *RClassOptimized) AttrAccessor(names ...string) *RClassOptimized {
	for _, name := range names {
		// 优化：使用闭包捕获变量，避免每次都创建新的函数
		varName := name
		c.DefineMethod(varName, func(self *RClassOptimized) any {
			return self.GetInstanceVar(varName)
		})
		c.DefineMethod(varName+"=", func(self *RClassOptimized, value any) any {
			self.SetInstanceVar(varName, value)
			return value
		})
	}
	return c
}

// AttrReader 定义只读属性（优化版本）
func (c *RClassOptimized) AttrReader(names ...string) *RClassOptimized {
	for _, name := range names {
		varName := name
		c.DefineMethod(varName, func(self *RClassOptimized) any {
			return self.GetInstanceVar(varName)
		})
	}
	return c
}

// AttrWriter 定义只写属性（优化版本）
func (c *RClassOptimized) AttrWriter(names ...string) *RClassOptimized {
	for _, name := range names {
		varName := name
		c.DefineMethod(varName+"=", func(self *RClassOptimized, value any) any {
			self.SetInstanceVar(varName, value)
			return value
		})
	}
	return c
}

// SetInstanceVar 设置实例变量（优化版本）
func (c *RClassOptimized) SetInstanceVar(name string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instanceVars[name] = value
}

// GetInstanceVar 获取实例变量（优化版本）
func (c *RClassOptimized) GetInstanceVar(name string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.instanceVars[name]
}

// SetClassVar 设置类变量（优化版本）
func (c *RClassOptimized) SetClassVar(name string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.classVars[name] = value
}

// GetClassVar 获取类变量（优化版本）
func (c *RClassOptimized) GetClassVar(name string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.classVars[name]
}

// MethodMissing 设置方法缺失处理器（优化版本）
func (c *RClassOptimized) MethodMissing(handler func(name string, args ...any) any) *RClassOptimized {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.methodMissing = handler
	return c
}

// Inherit 继承另一个类（优化版本）
func (c *RClassOptimized) Inherit(parent *RClassOptimized) *RClassOptimized {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.parent = parent

	// 复制父类的方法
	for name, method := range parent.methods {
		if _, exists := c.methods[name]; !exists {
			c.methods[name] = method
		}
	}

	// 清除缓存
	c.clearCache()
	return c
}

// RespondTo 检查是否响应某个方法（优化版本）
func (c *RClassOptimized) RespondTo(methodName string) bool {
	return c.findMethod(methodName) != nil
}

// Methods 返回所有可用的方法名（优化版本）
func (c *RClassOptimized) Methods() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getMethodNames(c.methods)
}

// ClassMethods 返回所有可用的类方法名（优化版本）
func (c *RClassOptimized) ClassMethods() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getMethodNames(c.classMethods)
}

// getMethodNames 获取方法名列表（优化版本）
func (c *RClassOptimized) getMethodNames(methods map[string]*CachedMethod) []string {
	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	return names
}

// InstanceVars 返回所有实例变量名（优化版本）
func (c *RClassOptimized) InstanceVars() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getVarNames(c.instanceVars)
}

// ClassVars 返回所有类变量名（优化版本）
func (c *RClassOptimized) ClassVars() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.getVarNames(c.classVars)
}

// getVarNames 获取变量名列表（优化版本）
func (c *RClassOptimized) getVarNames(vars map[string]any) []string {
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	return names
}

// Name 返回类名
func (c *RClassOptimized) Name() string {
	return c.name
}

// Parent 返回父类
func (c *RClassOptimized) Parent() *RClassOptimized {
	return c.parent
}

// IsA 检查是否为指定类的实例（优化版本）
func (c *RClassOptimized) IsA(className string) bool {
	if c.name == className {
		return true
	}
	if c.parent != nil {
		return c.parent.IsA(className)
	}
	return false
}

// Super 调用父类的同名方法（优化版本）
func (c *RClassOptimized) Super(methodName string, args ...any) any {
	if c.parent == nil {
		panic("no superclass method '" + methodName + "'")
	}

	method := c.parent.findMethod(methodName)
	if method == nil {
		panic(fmt.Sprintf("undefined method '%s' for %s", methodName, c.parent.name))
	}

	return method.fn(c, args...)
}

// 内存池优化（实验性）
var classPool = sync.Pool{
	New: func() interface{} {
		return &RClassOptimized{
			methods:      make(map[string]*CachedMethod),
			classMethods: make(map[string]*CachedMethod),
			instanceVars: make(map[string]any),
			classVars:    make(map[string]any),
			methodCache:  make(map[string]*CachedMethod),
		}
	},
}

// NewFromPool 从对象池创建实例（实验性优化）
func (c *RClassOptimized) NewFromPool() *RClassOptimized {
	instance := classPool.Get().(*RClassOptimized)
	instance.name = c.name
	instance.parent = c
	instance.methodMissing = c.methodMissing
	instance.classMethods = c.classMethods
	instance.classVars = c.classVars

	// 清空和复制方法
	for k := range instance.methods {
		delete(instance.methods, k)
	}
	for k := range instance.methodCache {
		delete(instance.methodCache, k)
	}
	for k := range instance.instanceVars {
		delete(instance.instanceVars, k)
	}

	for name, method := range c.methods {
		instance.methods[name] = method
	}

	return instance
}

// ReturnToPool 返回到对象池（实验性优化）
func (c *RClassOptimized) ReturnToPool() {
	classPool.Put(c)
}

// FastCall 快速方法调用（无锁版本，仅用于性能关键场景）
func (c *RClassOptimized) FastCall(methodName string, args ...any) any {
	// 警告：这个方法不是线程安全的，仅用于单线程性能测试
	if method, exists := c.methods[methodName]; exists {
		return method.fn(c, args...)
	}
	if method, exists := c.classMethods[methodName]; exists {
		return method.fn(c, args...)
	}
	if c.parent != nil {
		return c.parent.FastCall(methodName, args...)
	}

	if c.methodMissing != nil {
		return c.methodMissing(methodName, args...)
	}
	panic(fmt.Sprintf("undefined method '%s' for %s", methodName, c.name))
}

// GetMethodDirectly 直接获取方法指针（最高性能）
func (c *RClassOptimized) GetMethodDirectly(methodName string) *CachedMethod {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if method, exists := c.methods[methodName]; exists {
		return method
	}
	if method, exists := c.classMethods[methodName]; exists {
		return method
	}
	if c.parent != nil {
		return c.parent.GetMethodDirectly(methodName)
	}
	return nil
}

// CallDirect 直接调用方法（跳过查找）
func (method *CachedMethod) CallDirect(receiver any, args ...any) any {
	return method.fn(receiver, args...)
}
