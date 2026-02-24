package rvm

import (
	"fmt"

	"github.com/GoLangDream/rgo/rvm/compiler"
)

const StackSize = 2048

type VM struct {
	constants []interface{}
	globals   []interface{}

	stack []interface{}
	sp    int

	instructions compiler.Instructions
	ip           int
}

func New(bytecode *compiler.Bytecode) *VM {
	return &VM{
		constants:    bytecode.Constants,
		globals:      make([]interface{}, 100),
		stack:        make([]interface{}, StackSize),
		sp:           0,
		instructions: bytecode.Instructions,
		ip:           -1,
	}
}

func (vm *VM) Run() error {
	for vm.ip < len(vm.instructions)-1 {
		vm.ip++

		op := compiler.Opcode(vm.instructions[vm.ip])

		err := vm.execute(op)
		if err != nil {
			return err
		}
	}

	return nil
}

func (vm *VM) execute(op compiler.Opcode) error {
	switch op {
	case compiler.OpConstant:
		idx := vm.readUint16()
		vm.push(vm.constants[idx])

	case compiler.OpTrue:
		vm.push(true)

	case compiler.OpFalse:
		vm.push(false)

	case compiler.OpNil:
		vm.push(nil)

	case compiler.OpPop:
		vm.pop()

	case compiler.OpAdd:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.add(left, right))

	case compiler.OpSub:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.sub(left, right))

	case compiler.OpMul:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.mul(left, right))

	case compiler.OpDiv:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.div(left, right))

	case compiler.OpMod:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.mod(left, right))

	case compiler.OpPow:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.pow(left, right))

	case compiler.OpMinus:
		val := vm.pop()
		vm.push(vm.negate(val))

	case compiler.OpBang:
		val := vm.pop()
		vm.push(vm.bang(val))

	case compiler.OpEqual:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.equals(left, right))

	case compiler.OpNotEqual:
		right := vm.pop()
		left := vm.pop()
		vm.push(!vm.equals(left, right))

	case compiler.OpGreaterThan:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.greaterThan(left, right))

	case compiler.OpGreaterThanOrEqual:
		right := vm.pop()
		left := vm.pop()
		vm.push(!vm.lessThan(left, right))

	case compiler.OpLessThan:
		right := vm.pop()
		left := vm.pop()
		vm.push(vm.lessThan(left, right))

	case compiler.OpLessThanOrEqual:
		right := vm.pop()
		left := vm.pop()
		vm.push(!vm.greaterThan(left, right))

	case compiler.OpJump:
		pos := vm.readUint16()
		vm.ip = pos - 1

	case compiler.OpJumpNotTruthy:
		pos := vm.readUint16()
		condition := vm.pop()
		if !vm.isTruthy(condition) {
			vm.ip = pos - 1
		}

	case compiler.OpArray:
		n := vm.readUint16()
		arr := make([]interface{}, n)
		for i := n - 1; i >= 0; i-- {
			arr[i] = vm.pop()
		}
		vm.push(arr)

	case compiler.OpHash:
		n := vm.readUint16()
		h := make(map[interface{}]interface{})
		for i := 0; i < int(n); i++ {
			value := vm.pop()
			key := vm.pop()
			h[key] = value
		}
		vm.push(h)

	case compiler.OpIndex:
		index := vm.pop()
		left := vm.pop()
		vm.push(vm.index(left, index))

	case compiler.OpIndexAssign:
		value := vm.pop()
		index := vm.pop()
		left := vm.pop()
		vm.push(vm.indexAssign(left, index, value))

	case compiler.OpGetGlobal:
		idx := vm.readUint16()
		vm.push(vm.globals[idx])

	case compiler.OpSetGlobal:
		idx := vm.readUint16()
		vm.globals[idx] = vm.peek(0)

	case compiler.OpGetLocal:
		idx := vm.readUint8()
		basePtr := 0
		vm.push(vm.stack[basePtr+idx])

	case compiler.OpSetLocal:
		idx := vm.readUint8()
		basePtr := 0
		vm.stack[basePtr+idx] = vm.peek(0)

	case compiler.OpSelf:
		vm.push(vm.stack[0])

	case compiler.OpReturn:
		vm.sp = 0

	case compiler.OpReturnValue:
		vm.sp = 0
		vm.push(vm.pop())

	case compiler.OpSend:
		methodNameIdx := vm.readUint16()
		block := vm.readUint8()
		numArgs := vm.readUint8()
		_ = block
		methodName := vm.constants[methodNameIdx].(string)

		args := make([]interface{}, 0)
		for i := 0; i < int(numArgs); i++ {
			args = append(args, vm.pop())
		}
		receiver := vm.pop()

		result := vm.send(receiver, methodName, args)
		vm.push(result)

	case compiler.OpBreak:
		return fmt.Errorf("unexpected break")

	case compiler.OpDefineMethod:
		_ = vm.readUint16()

	case compiler.OpDefineClassMethod:
		_ = vm.readUint16()

	case compiler.OpClass:
		_ = vm.readUint16()

	case compiler.OpModule:
		_ = vm.readUint16()

	case compiler.OpDup:
		vm.push(vm.peek(0))

	case compiler.OpLambda:
		_ = vm.readUint16()
		numFree := vm.readUint8()
		vm.push(numFree)

	case compiler.OpClosure:
		_ = vm.readUint16()
		numFree := vm.readUint8()

		free := make([]interface{}, numFree)
		for i := numFree - 1; i >= 0; i-- {
			free[i] = vm.pop()
		}
		vm.push(free)

	case compiler.OpNeg:
		val := vm.pop()
		vm.push(vm.negate(val))

	default:
		return fmt.Errorf("unknown opcode: %v", op)
	}

	return nil
}

func (vm *VM) push(val interface{}) {
	vm.stack[vm.sp] = val
	vm.sp++
}

func (vm *VM) pop() interface{} {
	vm.sp--
	return vm.stack[vm.sp]
}

func (vm *VM) peek(n int) interface{} {
	return vm.stack[vm.sp-1-n]
}

func (vm *VM) readUint16() int {
	vm.ip++
	high := int(vm.instructions[vm.ip])
	vm.ip++
	low := int(vm.instructions[vm.ip])
	return high<<8 | low
}

func (vm *VM) readUint8() int {
	vm.ip++
	return int(vm.instructions[vm.ip])
}

func (vm *VM) isTruthy(val interface{}) bool {
	switch v := val.(type) {
	case bool:
		return v
	case nil:
		return false
	default:
		return true
	}
}

func (vm *VM) add(left, right interface{}) interface{} {
	switch l := left.(type) {
	case int64:
		switch r := right.(type) {
		case int64:
			return l + r
		case float64:
			return float64(l) + r
		}
	case float64:
		switch r := right.(type) {
		case int64:
			return l + float64(r)
		case float64:
			return l + r
		}
	case string:
		switch r := right.(type) {
		case string:
			return l + r
		}
	}
	return fmt.Sprintf("%v%v", left, right)
}

func (vm *VM) sub(left, right interface{}) interface{} {
	switch l := left.(type) {
	case int64:
		switch r := right.(type) {
		case int64:
			return l - r
		case float64:
			return float64(l) - r
		}
	case float64:
		switch r := right.(type) {
		case int64:
			return l - float64(r)
		case float64:
			return l - r
		}
	}
	return nil
}

func (vm *VM) mul(left, right interface{}) interface{} {
	switch l := left.(type) {
	case int64:
		switch r := right.(type) {
		case int64:
			return l * r
		case float64:
			return float64(l) * r
		}
	case float64:
		switch r := right.(type) {
		case int64:
			return l * float64(r)
		case float64:
			return l * r
		}
	}
	return nil
}

func (vm *VM) div(left, right interface{}) interface{} {
	switch l := left.(type) {
	case int64:
		switch r := right.(type) {
		case int64:
			if r == 0 {
				return nil
			}
			return l / r
		case float64:
			if r == 0 {
				return nil
			}
			return float64(l) / r
		}
	case float64:
		switch r := right.(type) {
		case int64:
			if r == 0 {
				return nil
			}
			return l / float64(r)
		case float64:
			if r == 0 {
				return nil
			}
			return l / r
		}
	}
	return nil
}

func (vm *VM) mod(left, right interface{}) interface{} {
	switch l := left.(type) {
	case int64:
		switch r := right.(type) {
		case int64:
			if r == 0 {
				return nil
			}
			return l % r
		}
	}
	return nil
}

func (vm *VM) pow(left, right interface{}) interface{} {
	switch l := left.(type) {
	case int64:
		switch r := right.(type) {
		case int64:
			if r < 0 {
				return 1.0 / vm.powInt(l, -int(r))
			}
			return vm.powInt(l, int(r))
		case float64:
			return vm.mathPow(float64(l), r)
		}
	case float64:
		switch r := right.(type) {
		case int64:
			return vm.mathPow(l, float64(r))
		case float64:
			return vm.mathPow(l, r)
		}
	}
	return nil
}

func (vm *VM) powInt(base int64, exp int) int64 {
	result := int64(1)
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

func (vm *VM) mathPow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

func (vm *VM) negate(val interface{}) interface{} {
	switch v := val.(type) {
	case int64:
		return -v
	case float64:
		return -v
	}
	return nil
}

func (vm *VM) bang(val interface{}) interface{} {
	switch v := val.(type) {
	case bool:
		return !v
	case nil:
		return true
	default:
		return false
	}
}

func (vm *VM) equals(left, right interface{}) bool {
	switch l := left.(type) {
	case bool:
		r, ok := right.(bool)
		return ok && l == r
	case nil:
		return right == nil
	case int64:
		switch r := right.(type) {
		case int64:
			return l == r
		case float64:
			return float64(l) == r
		}
	case float64:
		switch r := right.(type) {
		case int64:
			return l == float64(r)
		case float64:
			return l == r
		}
	case string:
		r, ok := right.(string)
		return ok && l == r
	}
	return false
}

func (vm *VM) lessThan(left, right interface{}) bool {
	switch l := left.(type) {
	case int64:
		switch r := right.(type) {
		case int64:
			return l < r
		case float64:
			return float64(l) < r
		}
	case float64:
		switch r := right.(type) {
		case int64:
			return l < float64(r)
		case float64:
			return l < r
		}
	}
	return false
}

func (vm *VM) greaterThan(left, right interface{}) bool {
	return vm.lessThan(right, left)
}

func (vm *VM) index(left, index interface{}) interface{} {
	switch l := left.(type) {
	case []interface{}:
		switch i := index.(type) {
		case int64:
			if i < 0 {
				i = int64(len(l)) + i
			}
			if i < 0 || i >= int64(len(l)) {
				return nil
			}
			return l[i]
		}
	case map[interface{}]interface{}:
		return l[index]
	case string:
		switch i := index.(type) {
		case int64:
			if i < 0 {
				i = int64(len(l)) + i
			}
			if i < 0 || i >= int64(len(l)) {
				return nil
			}
			return string(l[i])
		}
	}
	return nil
}

func (vm *VM) indexAssign(left, index, value interface{}) interface{} {
	switch l := left.(type) {
	case []interface{}:
		switch i := index.(type) {
		case int64:
			if i >= 0 && i < int64(len(l)) {
				l[i] = value
			}
		}
	case map[interface{}]interface{}:
		l[index] = value
	}
	return value
}

func (vm *VM) send(receiver interface{}, method string, args []interface{}) interface{} {
	switch r := receiver.(type) {
	case int64:
		return vm.sendToInteger(r, method, args)
	case float64:
		return vm.sendToFloat(r, method, args)
	case string:
		return vm.sendToString(r, method, args)
	case []interface{}:
		return vm.sendToArray(r, method, args)
	case bool:
		return vm.sendToBool(r, method, args)
	default:
		return nil
	}
}

func (vm *VM) sendToInteger(val int64, method string, args []interface{}) interface{} {
	switch method {
	case "+":
		if len(args) == 1 {
			switch a := args[0].(type) {
			case int64:
				return val + a
			case float64:
				return float64(val) + a
			}
		}
	case "-":
		if len(args) == 1 {
			switch a := args[0].(type) {
			case int64:
				return val - a
			case float64:
				return float64(val) - a
			}
		}
	case "*":
		if len(args) == 1 {
			switch a := args[0].(type) {
			case int64:
				return val * a
			case float64:
				return float64(val) * a
			}
		}
	case "/":
		if len(args) == 1 {
			switch a := args[0].(type) {
			case int64:
				if a != 0 {
					return val / a
				}
			case float64:
				if a != 0 {
					return float64(val) / a
				}
			}
		}
	case "to_s":
		return fmt.Sprintf("%d", val)
	case "chr":
		return string(rune(val))
	case "odd?":
		return val%2 == 1
	case "even?":
		return val%2 == 0
	case "zero?":
		return val == 0
	case "abs":
		if val < 0 {
			return -val
		}
		return val
	}
	return nil
}

func (vm *VM) sendToFloat(val float64, method string, args []interface{}) interface{} {
	switch method {
	case "+":
		if len(args) == 1 {
			switch a := args[0].(type) {
			case int64:
				return val + float64(a)
			case float64:
				return val + a
			}
		}
	case "-":
		if len(args) == 1 {
			switch a := args[0].(type) {
			case int64:
				return val - float64(a)
			case float64:
				return val - a
			}
		}
	case "*":
		if len(args) == 1 {
			switch a := args[0].(type) {
			case int64:
				return val * float64(a)
			case float64:
				return val * a
			}
		}
	case "/":
		if len(args) == 1 {
			switch a := args[0].(type) {
			case int64:
				if a != 0 {
					return val / float64(a)
				}
			case float64:
				if a != 0 {
					return val / a
				}
			}
		}
	case "to_s":
		return fmt.Sprintf("%g", val)
	case "to_i":
		return int64(val)
	}
	return nil
}

func (vm *VM) sendToString(val string, method string, args []interface{}) interface{} {
	switch method {
	case "+":
		if len(args) == 1 {
			return val + args[0].(string)
		}
	case "length", "size":
		return int64(len(val))
	case "empty?":
		return len(val) == 0
	case "to_s":
		return val
	case "upcase":
		return vm.toUpper(val)
	case "downcase":
		return vm.toLower(val)
	}
	return nil
}

func (vm *VM) toUpper(s string) string {
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else {
			result += string(r)
		}
	}
	return result
}

func (vm *VM) toLower(s string) string {
	result := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	return result
}

func (vm *VM) sendToArray(val []interface{}, method string, args []interface{}) interface{} {
	switch method {
	case "length", "size":
		return int64(len(val))
	case "first":
		if len(val) > 0 {
			return val[0]
		}
		return nil
	case "last":
		if len(val) > 0 {
			return val[len(val)-1]
		}
		return nil
	case "push":
		if len(args) > 0 {
			return append(val, args[0])
		}
	case "pop":
		if len(val) > 0 {
			return val[len(val)-1]
		}
		return nil
	case "empty?":
		return len(val) == 0
	case "join":
		if len(args) > 0 {
			sep := args[0].(string)
			result := ""
			for i, v := range val {
				result += fmt.Sprintf("%v", v)
				if i < len(val)-1 {
					result += sep
				}
			}
			return result
		}
	case "reverse":
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[len(val)-1-i] = v
		}
		return result
	}
	return nil
}

func (vm *VM) sendToBool(val bool, method string, args []interface{}) interface{} {
	switch method {
	case "to_s":
		if val {
			return "true"
		}
		return "false"
	case "!", "not":
		return !val
	}
	return nil
}

func (vm *VM) LastPoppedStackElement() interface{} {
	return vm.stack[vm.sp]
}
