package vm

import (
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedSSAPrimitivePlanDiscardable proves that a plan stays entirely inside
// the primitive ABI.  It may branch and use locals, but it cannot load self,
// instance variables, free variables, indexed objects, yield, or send a Ruby
// method.  Such a plan is safe for a direct caller to execute without an
// EmeraldValue register file; the caller still performs the operation guards
// in typedSSAImmediateEqual/typedSSACompare/typedSSABinary.
func typedSSAPrimitivePlanDiscardable(plan *typedSSAPlan, fn *object.Function) bool {
	if plan == nil || fn == nil || plan.hasReference || plan.hasYield ||
		len(fn.ParamLocalIndices) != len(fn.Params) || len(plan.ops) == 0 || plan.registers > 16 || plan.locals > 64 {
		return false
	}
	returned := false
	for _, instruction := range plan.ops {
		switch instruction.kind {
		case typedSSAOpLoadParam, typedSSAOpLoadLiteral, typedSSAOpLoadLocal,
			typedSSAOpMove, typedSSAOpSwap, typedSSAOpBang, typedSSAOpStoreLocal,
			typedSSAOpEqual, typedSSAOpCompare, typedSSAOpBinary,
			typedSSAOpJump, typedSSAOpJumpTruthy, typedSSAOpJumpNotTruthy, typedSSAOpJumpNotNil:
		case typedSSAOpReturn:
			returned = true
		default:
			return false
		}
	}
	return returned
}

func typedSSAPrimitiveValue(kind typedSSAValueKind) bool {
	switch kind {
	case typedSSAInteger, typedSSAFloat, typedSSAString, typedSSABool, typedSSANil:
		return true
	default:
		return false
	}
}

// executeTypedSSAPrimitivePlan is the allocation-free executor shared by
// direct loop/call graph tiers.  It is intentionally separate from
// executeTypedSSAPlan: the latter accepts Ruby references and can box values
// at every operation, while this function accepts already-proven primitive
// values and returns one raw typedSSAValue at the boundary.
func (vm *VM) executeTypedSSAPrimitivePlan(plan *typedSSAPlan, fn *object.Function, arguments []typedSSAValue) (typedSSAValue, bool) {
	if vm == nil || !typedSSAPrimitivePlanDiscardable(plan, fn) ||
		len(arguments) != len(fn.ParamLocalIndices) || len(arguments) > 16 {
		return typedSSAValue{}, false
	}
	for _, argument := range arguments {
		if !typedSSAPrimitiveValue(argument.kind) {
			return typedSSAValue{}, false
		}
	}
	var registers [16]typedSSAValue
	var locals [64]typedSSAValue
	for index, local := range fn.ParamLocalIndices {
		if local < 0 || local >= len(locals) || index >= len(arguments) {
			return typedSSAValue{}, false
		}
		locals[local] = arguments[index]
	}
	for pc := 0; pc < len(plan.ops); pc++ {
		instruction := plan.ops[pc]
		switch instruction.kind {
		case typedSSAOpLoadParam:
			if int(instruction.param) >= len(arguments) {
				return typedSSAValue{}, false
			}
			registers[instruction.dst] = arguments[instruction.param]
		case typedSSAOpLoadLiteral:
			if !typedSSAPrimitiveValue(instruction.literal.kind) {
				return typedSSAValue{}, false
			}
			registers[instruction.dst] = instruction.literal
		case typedSSAOpLoadLocal:
			if instruction.param >= 64 {
				return typedSSAValue{}, false
			}
			value := locals[instruction.param]
			if value.kind == typedSSAInvalid {
				value = typedSSAValue{kind: typedSSANil}
			}
			registers[instruction.dst] = value
		case typedSSAOpMove:
			registers[instruction.dst] = registers[instruction.left]
		case typedSSAOpSwap:
			registers[instruction.left], registers[instruction.right] = registers[instruction.right], registers[instruction.left]
		case typedSSAOpBang:
			registers[instruction.dst] = typedSSAValue{kind: typedSSABool, bool: !typedSSATruthy(registers[instruction.left])}
		case typedSSAOpStoreLocal:
			if instruction.param >= 64 {
				return typedSSAValue{}, false
			}
			locals[instruction.param] = registers[instruction.left]
		case typedSSAOpEqual:
			value, ok := typedSSAImmediateEqual(vm, registers[instruction.left], registers[instruction.right])
			if !ok {
				return typedSSAValue{}, false
			}
			registers[instruction.dst] = value
		case typedSSAOpCompare:
			value, ok := typedSSACompare(vm, instruction.opcode, registers[instruction.left], registers[instruction.right])
			if !ok {
				return typedSSAValue{}, false
			}
			registers[instruction.dst] = value
		case typedSSAOpBinary:
			value, ok := typedSSABinary(vm, instruction.opcode, registers[instruction.left], registers[instruction.right])
			if !ok {
				return typedSSAValue{}, false
			}
			registers[instruction.dst] = value
		case typedSSAOpJump:
			if instruction.target < 0 || instruction.target >= len(plan.ops) {
				return typedSSAValue{}, false
			}
			pc = instruction.target - 1
		case typedSSAOpJumpTruthy:
			if typedSSATruthy(registers[instruction.left]) {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return typedSSAValue{}, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpJumpNotTruthy:
			if !typedSSATruthy(registers[instruction.left]) {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return typedSSAValue{}, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpJumpNotNil:
			if registers[instruction.left].kind != typedSSANil {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return typedSSAValue{}, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpReturn:
			value := registers[instruction.left]
			if !typedSSAPrimitiveValue(value.kind) {
				return typedSSAValue{}, false
			}
			return value, true
		default:
			return typedSSAValue{}, false
		}
	}
	return typedSSAValue{}, false
}
