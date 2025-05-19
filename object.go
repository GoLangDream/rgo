package rgo

// Object 是所有 R 类型的基础接口
type Object interface {
	// ToString 返回对象的字符串表示
	ToString() string

	// Class 返回对象的类名
	Class() string

	// IsA 检查对象是否为指定类型
	IsA(className string) bool

	// Equal 比较两个对象是否相等
	Equal(other Object) bool
}

// BaseObject 提供 Object 接口的基本实现
type BaseObject struct {
	className string
}

// NewBaseObject 创建一个新的 BaseObject
func NewBaseObject(className string) BaseObject {
	return BaseObject{className: className}
}

// Class 返回对象的类名
func (b BaseObject) Class() string {
	return b.className
}

// IsA 检查对象是否为指定类型
func (b BaseObject) IsA(className string) bool {
	return b.className == className
}
