package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

var integerTimesLinearEnabled = os.Getenv("RGO_DISABLE_INTEGER_TIMES_LINEAR") == ""

// tryExecuteIntegerTimesLinearBlock is the batch entry for a pure arithmetic
// callback such as `n.times { |i| i * 3 + 1 }`.  The ordinary Integer#times
// implementation has to allocate a Ruby Integer and enter the block binder on
// every iteration even though this shape cannot observe either value.  Once
// Register IR proves that the callback is a straight-line integer expression,
// keep the induction value unboxed and execute the loop in Go.
//
// This tier is intentionally stricter than the per-element integer block
// entry.  It admits only a local block return with no sends, branches,
// captures, explicit returns, or other frame-dependent operations.  A failed
// arithmetic guard returns handled=false before any user-visible mutation;
// Integer#times then replays the original callback through the full Ruby path.
func (vm *VM) tryExecuteIntegerTimesLinearBlock(receiver *object.EmeraldValue, count int64, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !integerTimesLinearEnabled || receiver == nil || block == nil || block.Type != object.ValueClosure ||
		!registerIRBlockEnabled || !registerIRBatchBlockEnabled || vm.instructionLimit != 0 ||
		DevMode || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || vm.threadDepth > 0 {
		return nil, false
	}
	if count <= 0 {
		core.LastBlockResult = nil
		return receiver, true
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil ||
		closure.BreakOwnerID > 0 || closureUsesRefinements(closure) {
		return nil, false
	}
	fn := closure.Fn
	if closure.AutoSplat && blockWantsDestructuring(fn) {
		return nil, false
	}
	if len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		!simpleBlockParameterPatterns(fn) {
		return nil, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	plan, found := vm.cachedBlockLeafPlan(fn)
	if !found || plan.kind != leafMethodRegisterIR || plan.registerIR == nil {
		return nil, false
	}
	ir := plan.registerIR
	if !ir.integerOnly || !ir.integerLinear || !ir.blockReturn || ir.hasBranches ||
		ir.hasImplicitSends || ir.hasExplicitReturn || ir.requiresFrame || len(ir.instructions) == 0 {
		return nil, false
	}
	if !registerIRIntegerLinearInputMatchesParameter(ir, fn) {
		return nil, false
	}
	if ir.integerLinearKind != 1 && ir.integerLinearKind != 2 {
		return nil, false
	}
	if !vm.fusedIntegerOperationAvailable(ir.integerLinearOpA) ||
		(ir.integerLinearKind == 2 && !vm.fusedIntegerOperationAvailable(ir.integerLinearOpB)) {
		return nil, false
	}
	// Integer#times discards the value returned by a pure linear block.  Once
	// the plan has no sends, captures, branches or writes, executing every
	// iteration only to throw the result away is unnecessary.  Integer
	// overflow promotes to a Ruby Bignum rather than raising; modulo by zero
	// is the only exceptional operation in this restricted shape and is
	// rejected by integerLinearPlanNoThrow below.
	if integerLinearRangeSafe(ir, count) {
		core.LastBlockResult = nil
		return receiver, true
	}

	for index := int64(0); index < count; index++ {
		value, ok := applyRegisterIRIntegerLinearOpRaw(ir.integerLinearOpA, index, ir.integerLinearConstA)
		if ok && ir.integerLinearKind == 2 {
			value, ok = applyRegisterIRIntegerLinearOpRaw(ir.integerLinearOpB, value, ir.integerLinearConstB)
		}
		if !ok {
			// The body is pure, so no observable work has happened.  Returning a
			// miss lets intTimes replay from index zero with BigInt/error semantics.
			core.LastBlockResult = nil
			return nil, false
		}
	}
	core.LastBlockResult = nil
	return receiver, true
}

func integerLinearRangeSafe(plan *registerIRPlan, count int64) bool {
	if plan == nil || count <= 0 || !plan.integerLinear ||
		(plan.integerLinearKind != 1 && plan.integerLinearKind != 2) {
		return false
	}
	return integerLinearPlanNoThrow(plan)
}

// integerLinearPlanNoThrow is the side-effect/exception guard shared by
// discarded-result loops.  The integer-linear recognizer admits only add,
// subtract, multiply and modulo by an immutable integer.  The first three
// never raise in Ruby (overflow promotes to a Bignum); modulo raises only for
// a zero divisor.  A caller that discards the result may therefore omit the
// arithmetic once element type and this guard have been proven.
func integerLinearPlanNoThrow(plan *registerIRPlan) bool {
	if plan == nil || !plan.integerLinear || (plan.integerLinearKind != 1 && plan.integerLinearKind != 2) {
		return false
	}
	if plan.integerLinearOpA == compiler.OpMod && plan.integerLinearConstA == 0 {
		return false
	}
	if plan.integerLinearKind == 2 && plan.integerLinearOpB == compiler.OpMod && plan.integerLinearConstB == 0 {
		return false
	}
	return true
}

// integerLinearArrayNoopSafe proves that Array#each can discard a pure
// arithmetic callback.  Ruby still has to reject non-Integers before the
// callback would dispatch to Integer#+/-/*/%, so the elements are preflighted
// once.  No value is materialized and no callback frame is entered after the
// proof succeeds.  Overflow is not an exception in Ruby and is consequently
// irrelevant when the callback result is discarded.
func integerLinearArrayNoopSafe(plan *registerIRPlan, elems []*object.EmeraldValue) bool {
	if !integerLinearPlanNoThrow(plan) {
		return false
	}
	for _, elem := range elems {
		if !smallIntegerValue(elem) {
			return false
		}
	}
	return true
}
