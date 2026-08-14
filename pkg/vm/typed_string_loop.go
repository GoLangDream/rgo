package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedStringLoopEnabled gates the first closed-world String call-loop tier.
// It is separate from typedSSAEnabled so benchmarks and bug reports can
// isolate the loop ABI without disabling typed plans for ordinary calls.
var typedStringLoopEnabled = os.Getenv("RGO_DISABLE_TYPED_STRING_LOOP") == ""

// A closed-world String#+ graph has no Ruby-visible side effects. In compiled
// mode, when the bytecode immediately pops its result, allocating that result
// only to throw it away dominates the loop while changing no value used by the
// program. Keep a switch for allocation-sensitive compatibility tests that
// inspect GC stats; the compatibility `run` mode retains result allocation.
var typedStringLoopDeadResultEnabled = os.Getenv("RGO_DISABLE_TYPED_STRING_DEAD_RESULT") == "" && os.Getenv("RGO_EXEC_MODE") == "compiled"

// executeTypedSSAStringPlan is kept as a small compatibility wrapper for
// existing String-loop tests. The actual implementation is the shared
// primitive ABI, which also handles Float and mixed primitive branches.
func (vm *VM) executeTypedSSAStringPlan(plan *typedSSAPlan, fn *object.Function, argument string) (string, bool) {
	value, ok := vm.executeTypedSSAPrimitivePlan(plan, fn, []typedSSAValue{{kind: typedSSAString, str: argument}})
	if !ok || value.kind != typedSSAString {
		return "", false
	}
	return value.str, true
}

func typedSSAStringPlanDiscardable(plan *typedSSAPlan, fn *object.Function) bool {
	return plan != nil && plan.hasString && typedSSAPrimitivePlanDiscardable(plan, fn)
}

// tryExecuteTypedStringCallLoop lowers the common shape
//
//	while i < limit
//	  pure_string_helper("literal")
//	  i += 1
//	end
//
// into a direct typed call loop.  The helper is resolved once and must compile
// to the closed String ABI above. No Ruby frame, Send lookup, EmeraldValue
// argument, or boxed result is created per iteration. The result is deliberately
// discarded only after the bytecode proves the call is followed by OpPop and
// the callee is a pure typed graph; the ordinary interpreter remains the
// fallback for every other call shape.
func (vm *VM) tryExecuteTypedStringCallLoop(frame *Frame, target, jumpPosition int) bool {
	if vm == nil || frame == nil || frame.Fn == nil || frame.Fn.Name != "__main__" ||
		!typedStringLoopEnabled || !typedSSAEnabled || !integerLoopOptimizationEnabled ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() || vm.currentBlock != nil ||
		vm.pendingReturnTargetID != 0 || vm.pendingBreakTargetID != 0 || len(vm.catchStack) != 0 ||
		len(vm.rescueStack) != 0 || len(vm.activeRescues) != 0 || len(vm.pendingEnsures) != 0 {
		return false
	}
	counterLocal, limitLocal, exitPosition, position, limit, ok := integerLoopHeader(frame, target, jumpPosition)
	if !ok {
		return false
	}
	instructions := frame.Fn.Instructions

	// The call receiver is self. A stable local/object receiver would require a
	// separate class/singleton guard and is intentionally left to the generic
	// integer-loop parser for now.
	if position >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSelf {
		return false
	}
	position++
	argument, position, ok := loopPrimitiveConstantAt(frame, position, jumpPosition)
	if !ok {
		return false
	}
	if position+5 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSend ||
		instructions[position+3] != 0 || instructions[position+4] != 1 || instructions[position+5] != 255 {
		return false
	}
	methodConstant := int(instructions[position+1])<<8 | int(instructions[position+2])
	if methodConstant < 0 || methodConstant >= len(frame.Fn.Constants) || frame.Fn.Constants[methodConstant] == nil {
		return false
	}
	methodName, ok := frame.Fn.Constants[methodConstant].Data.(string)
	if !ok || methodName == "" {
		return false
	}
	position += 6
	resultLocal := -1
	if position >= jumpPosition {
		return false
	}
	if compiler.Opcode(instructions[position]) == compiler.OpSetLocal {
		if position+1 >= jumpPosition {
			return false
		}
		resultLocal = int(instructions[position+1])
		if resultLocal == counterLocal {
			return false
		}
		position += 2
	}
	if compiler.Opcode(instructions[position]) != compiler.OpPop {
		return false
	}
	position++
	updateLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || updateLocal != counterLocal {
		return false
	}
	step, position, ok := loopIntegerConstantAt(frame, position, jumpPosition)
	if !ok || step <= 0 || position >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpAdd {
		return false
	}
	position++
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSetLocal ||
		int(instructions[position+1]) != counterLocal || compiler.Opcode(instructions[position+2]) != compiler.OpPop ||
		position+3 != jumpPosition {
		return false
	}

	// The literal is passed through the primitive ABI only when its exact
	// built-in representation is accepted by typedSSAValueFromObject.
	argumentValue := typedSSAValueFromObject(argument)
	if !typedSSAPrimitiveValue(argumentValue.kind) {
		return false
	}
	receiverIndex := frame.Bp
	if receiverIndex < 0 || receiverIndex >= len(vm.stack) || vm.stack[receiverIndex] == nil {
		return false
	}
	receiver := vm.stack[receiverIndex]
	methodObj, methodOwner, fallback := vm.lookupMethodForSend(receiver, methodName, nil, false, false)
	if fallback != nil || methodObj == nil || methodObj.Visibility == "undefined" || methodObj.Visibility == "protected" ||
		methodObj.DispatchOwner != nil || methodObj.Ruby2Keywords || methodUsesRefinements(methodObj) {
		return false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectKeywords || fn.RejectBlock ||
		len(fn.FreeVars) != 0 || methodOwner == nil {
		return false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return false
		}
	}
	plan, ok := vm.cachedTypedSSAPlan(fn)
	if !ok || plan == nil || plan.hasReference || plan.hasYield || plan.blockReturn || !typedSSAPrimitivePlanDiscardable(plan, fn) {
		return false
	}
	discardResult := resultLocal < 0 && typedStringLoopDeadResultEnabled
	if discardResult {
		// Validate the actual operand kinds once before eliding the discarded
		// result. This keeps unusual but reference-free plans conservative.
		if _, executed := vm.executeTypedSSAPrimitivePlan(plan, fn, []typedSSAValue{argumentValue}); !executed {
			return false
		}
	}

	counterValue, ok := typedStringLoopIntegerLocal(vm, frame, counterLocal)
	if !ok {
		return false
	}
	if limitLocal >= 0 {
		limitValue, limitOK := typedStringLoopIntegerLocal(vm, frame, limitLocal)
		if !limitOK {
			return false
		}
		limit = limitValue
	}
	if counterValue >= limit {
		frame.WhileEnd = exitPosition
		frame.BlockBreakAddr = exitPosition
		frame.Ip = exitPosition - 1
		return true
	}

	generation := object.CurrentMethodGeneration()
	counter := counterValue
	completed := true
	result := typedSSAValue{}
	resultReady := false
	for iterations := 0; counter < limit; iterations++ {
		if iterations == 1_000_000 || object.CurrentMethodGeneration() != generation {
			completed = false
			break
		}
		if resultLocal >= 0 {
			if !resultReady {
				var executed bool
				result, executed = vm.executeTypedSSAPrimitivePlan(plan, fn, []typedSSAValue{argumentValue})
				if !executed {
					return false
				}
				resultReady = true
			}
		} else if !discardResult {
			if _, executed := vm.executeTypedSSAPrimitivePlan(plan, fn, []typedSSAValue{argumentValue}); !executed {
				return false
			}
		}
		var stepOK bool
		counter, stepOK = checkedIntegerAdd(counter, step)
		if !stepOK {
			return false
		}
	}
	if resultLocal >= 0 && resultReady {
		commitTypedStringLoopLocal(vm, frame, resultLocal, typedSSAValueToObject(result))
	}
	last := vm.commitIntegerLoopLocal(frame, counterLocal, counter)
	vm.recordPoppedValue(last)
	frame.WhileEnd = exitPosition
	frame.BlockBreakAddr = exitPosition
	if completed {
		frame.Ip = exitPosition - 1
	} else {
		frame.Ip = target - 1
	}
	return true
}

func commitTypedStringLoopLocal(vm *VM, frame *Frame, local int, value *object.EmeraldValue) {
	if vm == nil || frame == nil || value == nil || local < 0 {
		return
	}
	index := frame.Bp + local + 1
	if index < 0 || index >= len(vm.stack) {
		return
	}
	vm.stack[index] = value
	if name, ok := vm.topLevelLocalName(frame, local); ok {
		if binding := vm.topLevelBindingData(); binding != nil {
			binding.Locals[name] = value
		}
	}
	vm.updateCapturedBindingLocal(frame, local, value)
}

func loopPrimitiveConstantAt(frame *Frame, position, end int) (*object.EmeraldValue, int, bool) {
	if frame == nil || frame.Fn == nil || position+2 >= end || compiler.Opcode(frame.Fn.Instructions[position]) != compiler.OpConstant {
		return nil, position, false
	}
	index := int(frame.Fn.Instructions[position+1])<<8 | int(frame.Fn.Instructions[position+2])
	if index < 0 || index >= len(frame.Fn.Constants) {
		return nil, position, false
	}
	value := frame.Fn.Constants[index]
	if value == nil || !typedSSAPrimitiveValue(typedSSAValueFromObject(value).kind) {
		return nil, position, false
	}
	return value, position + 3, true
}

func typedStringLoopIntegerLocal(vm *VM, frame *Frame, local int) (int64, bool) {
	if vm == nil || frame == nil || local < 0 {
		return 0, false
	}
	index := frame.Bp + local + 1
	if index < 0 || index >= len(vm.stack) {
		return 0, false
	}
	value := vm.stack[index]
	if value == nil || value.Type != object.ValueInteger || value.BigIntValue() != nil ||
		(value.Class != nil && value.Class != core.R.Classes["Integer"]) || core.AttachedSingletonClass(value) != nil {
		return 0, false
	}
	if _, cell := value.Data.(*closureCell); cell {
		return 0, false
	}
	result, ok := value.Data.(int64)
	return result, ok
}
