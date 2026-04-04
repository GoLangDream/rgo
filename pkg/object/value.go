package object

import (
	"fmt"
)

type ValueType int

const (
	ValueNil ValueType = iota
	ValueBool
	ValueInteger
	ValueFloat
	ValueString
	ValueArray
	ValueHash
	ValueSymbol
	ValueRegexp
	ValueRange
	ValueClass
	ValueModule
	ValueObject
	ValueFunction
	ValueBuiltin
	ValueClosure
	ValueProc
	ValueMethod
	ValueBinding
	ValueFiber
	ValueMatchData
	ValueIO
	ValueFile
	ValueException
)

type EmeraldValue struct {
	Type  ValueType
	Data  interface{}
	Class *Class
}

func NewValue(t ValueType, data interface{}, class *Class) *EmeraldValue {
	return &EmeraldValue{
		Type:  t,
		Data:  data,
		Class: class,
	}
}

func (v *EmeraldValue) Inspect() string {
	switch v.Type {
	case ValueNil:
		return "nil"
	case ValueBool:
		if v.Data.(bool) {
			return "true"
		}
		return "false"
	case ValueInteger:
		return fmt.Sprintf("%d", v.Data)
	case ValueFloat:
		return fmt.Sprintf("%g", v.Data)
	case ValueString:
		return v.Data.(string)
	case ValueArray:
		arr := v.Data.([]*EmeraldValue)
		str := "["
		for i, e := range arr {
			str += e.Inspect()
			if i < len(arr)-1 {
				str += ", "
			}
		}
		str += "]"
		return str
	case ValueHash:
		h := v.Data.(map[*EmeraldValue]*EmeraldValue)
		str := "{"
		i := 0
		for k, val := range h {
			str += k.Inspect() + " => " + val.Inspect()
			i++
			if i < len(h) {
				str += ", "
			}
		}
		str += "}"
		return str
	case ValueClass:
		if v.Data != nil {
			return v.Data.(*Class).Name
		}
		return "#<Class:...>"
	case ValueModule:
		if v.Data != nil {
			return v.Data.(*Module).Name
		}
		return "#<Module:...>"
	case ValueFunction:
		fn := v.Data.(*Function)
		return fmt.Sprintf("#<Function:%s>", fn.Name)
	case ValueBuiltin:
		fn := v.Data.(*BuiltinFunction)
		return fmt.Sprintf("#<BuiltinFunction:%s>", fn.Name)
	case ValueClosure:
		return "#<Closure>"
	case ValueProc:
		return "#<Proc>"
	case ValueMethod:
		m := v.Data.(*Method)
		return fmt.Sprintf("#<Method: %s>", m.Name)
	case ValueBinding:
		return "#<Binding>"
	default:
		return fmt.Sprintf("#<%v>", v.Type)
	}
}

func (v *EmeraldValue) TypeName() string {
	switch v.Type {
	case ValueNil:
		return "NilClass"
	case ValueBool:
		return "TrueClass"
	case ValueInteger:
		return "Integer"
	case ValueFloat:
		return "Float"
	case ValueString:
		return "String"
	case ValueArray:
		return "Array"
	case ValueHash:
		return "Hash"
	case ValueSymbol:
		return "Symbol"
	case ValueRegexp:
		return "Regexp"
	case ValueRange:
		return "Range"
	case ValueClass:
		return "Class"
	case ValueModule:
		return "Module"
	case ValueFunction:
		return "Function"
	case ValueBuiltin:
		return "Builtin"
	case ValueClosure:
		return "Closure"
	case ValueProc:
		return "Proc"
	case ValueMethod:
		return "Method"
	case ValueBinding:
		return "Binding"
	default:
		return "Unknown"
	}
}

func (v *EmeraldValue) IsTruthy() bool {
	if v == nil {
		return false
	}
	switch v.Type {
	case ValueNil:
		return false
	case ValueBool:
		if v.Data == nil {
			return false
		}
		return v.Data.(bool)
	default:
		return true
	}
}

func (v *EmeraldValue) Equals(other *EmeraldValue) bool {
	if v.Type != other.Type {
		return false
	}
	switch v.Type {
	case ValueNil:
		return true
	case ValueBool:
		return v.Data.(bool) == other.Data.(bool)
	case ValueInteger:
		return v.Data.(int64) == other.Data.(int64)
	case ValueFloat:
		return v.Data.(float64) == other.Data.(float64)
	case ValueString:
		return v.Data.(string) == other.Data.(string)
	case ValueClass:
		return v.Data == other.Data
	default:
		return v == other
	}
}

type KeywordParamInfo struct {
	Name       string
	HasDefault bool
	Default    *EmeraldValue
}

type Function struct {
	Name           string
	Params         []string
	KeywordParams  []KeywordParamInfo
	Body           interface{}
	FreeVars       []*EmeraldValue
	Instructions   []byte
	NumLocals      int
	HasRestParam   bool
	RestParamIndex int
}

type BuiltinFunction struct {
	Name  string
	Fn    func(args ...*EmeraldValue) *EmeraldValue
	Arity int
}

type Method struct {
	Name  string
	Fn    interface{}
	Arity int
}

type Proc struct {
	Fn       *Function
	Env      []*EmeraldValue
	IsLambda bool
}

type ControlFlow struct {
	Kind  string
	Value *EmeraldValue
}

type Closure struct {
	Fn   *Function
	Free []*EmeraldValue
}

type RInteger struct {
	Value int64
}

type RFloat struct {
	Value float64
}

type RString struct {
	Value string
}

type RArray struct {
	Elements []*EmeraldValue
}

type RHash struct {
	Pairs map[*EmeraldValue]*EmeraldValue
}

type RSymbol struct {
	Value string
}

type RRegexp struct {
	Pattern string
	Options string
}

type RRange struct {
	Start     int64
	End       int64
	Exclusive bool
}

type RException struct {
	Message   string
	Backtrace []string
}

type RBinding struct {
	Self      *EmeraldValue
	Locals    map[string]*EmeraldValue
	Constants map[string]*EmeraldValue
}
