package vm

import (
	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/object"
)

// tryExecuteTypedSSAReferenceGetterLoop lowers the narrow but important
// object-read shape
//
//	sum += receiver.outer
//	counter += 1
//
// when outer is a closed-world zero-argument method that only calls another
// zero-argument method returning one instance variable.  The generic typed
// reference executor is semantically safe, but it still walks the SSA graph,
// checks eligibility, and resolves the nested call on every iteration.  This
// kernel resolves that call once and performs the stable object/map read in
// the loop itself.  Any guard miss returns before committing locals so the
// ordinary bytecode path replays the loop with full Ruby semantics.
func (vm *VM) tryExecuteTypedSSAReferenceGetterLoop(frame *Frame, exitPosition, counterLocal int, limit int64, steps []integerLoopStep, locals [256]int64) bool {
	if vm == nil || frame == nil || len(steps) != 11 || counterLocal < 0 {
		return false
	}
	if !isLocalLoadOpcode(steps[0].op) || steps[0].local == counterLocal ||
		steps[1].op != compiler.OpGetLocal && steps[1].op != compiler.OpGetLocalFast ||
		steps[1].receiverLocal < 0 || steps[2].op != compiler.OpSend ||
		!steps[2].typedReferenceCall || steps[2].typedPlan == nil || steps[2].fn == nil ||
		steps[2].argc != 0 || steps[3].op != compiler.OpAdd ||
		steps[4].op != compiler.OpSetLocal || steps[4].local != steps[0].local ||
		steps[5].op != compiler.OpPop || !isLocalLoadOpcode(steps[6].op) ||
		steps[6].local != counterLocal || steps[7].op != compiler.OpConstant ||
		steps[7].value <= 0 || steps[8].op != compiler.OpAdd ||
		steps[9].op != compiler.OpSetLocal || steps[9].local != counterLocal ||
		steps[10].op != compiler.OpPop {
		return false
	}

	receiverIndex := frame.Bp + steps[1].receiverLocal + 1
	if receiverIndex < 0 || receiverIndex >= len(vm.stack) {
		return false
	}
	receiver := derefClosureValue(vm.stack[receiverIndex])
	if receiver == nil || receiver.Type != object.ValueObject {
		return false
	}
	obj, ok := receiver.Data.(*object.Object)
	if !ok || obj == nil {
		return false
	}
	if cell, isCell := vm.stack[receiverIndex].Data.(*closureCell); isCell && cell != nil {
		return false
	}

	outerPlan := steps[2].typedPlan
	if !typedSSAReferenceCallPlanEligible(outerPlan) || len(outerPlan.ops) != 3 {
		return false
	}
	loadSelf, call, ret := outerPlan.ops[0], outerPlan.ops[1], outerPlan.ops[2]
	if loadSelf.kind != typedSSAOpLoadSelf || call.kind != typedSSAOpCall || call.argc != 0 ||
		ret.kind != typedSSAOpReturn || call.left != loadSelf.dst || ret.left != call.dst {
		return false
	}
	entry, ok := vm.cachedTypedSSAReferenceCallee(receiver, call, nil)
	if !ok || entry.fn == nil || entry.plan == nil || len(entry.plan.ops) != 2 {
		return false
	}
	innerLoad, innerReturn := entry.plan.ops[0], entry.plan.ops[1]
	if innerLoad.kind != typedSSAOpLoadInstanceVar || innerReturn.kind != typedSSAOpReturn ||
		innerReturn.left != innerLoad.dst || innerLoad.name == "" ||
		entry.generation != object.CurrentMethodGeneration() ||
		steps[2].typedGeneration != entry.generation {
		return false
	}

	sumLocal := steps[0].local
	counter := locals[counterLocal]
	sum := locals[sumLocal]
	if counter >= limit {
		return false
	}
	counterStep := steps[7].value
	for counter < limit {
		if object.CurrentMethodGeneration() != steps[2].typedGeneration {
			return false
		}
		value := (*object.EmeraldValue)(nil)
		value = obj.GetInstanceVar(innerLoad.name)
		typed := typedSSAValueFromObject(value)
		if typed.kind != typedSSAInteger {
			return false
		}
		var addOK bool
		sum, addOK = checkedIntegerAdd(sum, typed.int)
		if !addOK {
			return false
		}
		counter, addOK = checkedIntegerAdd(counter, counterStep)
		if !addOK {
			return false
		}
	}
	vm.commitIntegerLoopLocal(frame, sumLocal, sum)
	last := vm.commitIntegerLoopLocal(frame, counterLocal, counter)
	vm.recordPoppedValue(last)
	frame.WhileEnd = exitPosition
	frame.BlockBreakAddr = exitPosition
	frame.Ip = exitPosition - 1
	return true
}
