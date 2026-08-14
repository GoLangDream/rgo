package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedHotFunctionEnabled enables the method-level typed execution tier.  The
// ordinary interpreter remains the semantic authority: this tier only enters
// after a fixed-arity, public Ruby function has a Register IR plan whose direct
// executor can prove every operation side-effect-safe on a guard miss.
var typedHotFunctionEnabled = os.Getenv("RGO_DISABLE_TYPED_HOT_FUNCTION") == ""

// typedHotFunctionEntry stores the decoded plan separately from object.Function
// so the object model does not grow another runtime-only cache field.  The
// direct executor owns its warmup/disabled counters; generation is kept here to
// re-arm a plan after a nested method redefinition changes its send guards.
type typedHotFunctionEntry struct {
	plan           *registerIRPlan
	allowConstants bool
	generation     uint64
	disabled       bool
}

// tryExecuteTypedHotFunction is the method-level bridge from the normal Ruby
// dispatcher to the existing frame-free Register IR executor.  Previously that
// executor was reached mostly from block/leaf paths; ordinary Ruby methods
// therefore paid invokeMethod and a Ruby Frame even when their body already had
// a valid direct plan.  This bridge makes the tier reusable for Gem code while
// retaining a strict side-exit contract.
func (vm *VM) tryExecuteTypedHotFunction(methodObj *object.Method, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !typedHotFunctionEnabled || methodObj == nil || fn == nil || receiver == nil ||
		methodObj.DispatchOwner != nil || methodObj.Ruby2Keywords ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") ||
		methodObj.OriginalName == "method_missing" ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 ||
		vm.pendingReturnTargetID != 0 || vm.pendingBreakTargetID != 0 ||
		methodUsesRefinements(methodObj) || len(args) != len(fn.Params) ||
		len(fn.ParamLocalIndices) != len(fn.Params) || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		fn.RejectKeywords || fn.RejectBlock || !simpleBlockParameterPatterns(fn) ||
		registerIRFunctionNeedsDefaultEvaluation(fn, len(args)) || fn.NumLocals > 64 {
		return nil, false
	}
	if registerIRAggressiveEnabled {
		if result, executed := vm.tryExecuteAggressiveHotFunction(methodObj, fn, receiver, args); executed {
			return result, true
		}
	}

	generation := object.CurrentMethodGeneration()
	entry, found := vm.typedHotFunctions[fn]
	if !found {
		plan, compiled := compileRegisterIR(fn)
		if !compiled {
			// Ordinary framed Register IR keeps mutable String literals behind
			// its opt-in gate because constantValue allocates a fresh object.
			// The method-level direct tier already rejects ObjectSpace tracing
			// and admits LoadConstantValue only when every later mutation is
			// terminal, so it can safely compile this otherwise common branch
			// shape without widening the general framed cache.
			options := defaultRegisterIRCompileOptions()
			options.allowStringLiterals = true
			plan, compiled = compileRegisterIRWithOptions(fn, options)
		}
		allowConstants := plan != nil && registerIRDirectConstantsSafe(vm, methodObj.Closure, plan)
		if !compiled || plan == nil || plan.blockReturn || plan.hasImplicitSends ||
			plan.registers > 16 || !registerIRPlanSafeForActiveRescues(plan, vm) ||
			!registerIRPlanSafeForDirectNoFrameWithOptions(plan, false, allowConstants) {
			entry = typedHotFunctionEntry{disabled: true, generation: generation}
		} else {
			entry = typedHotFunctionEntry{plan: plan, allowConstants: allowConstants, generation: generation}
		}
		vm.typedHotFunctions[fn] = entry
	}
	if entry.disabled || entry.plan == nil {
		return nil, false
	}
	if entry.generation != generation {
		entry.generation = generation
		entry.plan.noFrameGeneration = 0
		entry.plan.noFrameCalls = 0
		entry.plan.noFrameDisabled = false
		vm.typedHotFunctions[fn] = entry
	}
	// A Ruby method called from a block inherits the caller's block in the
	// ordinary framed path, but the method itself must not observe that block
	// unless it declares/uses the block protocol.  The plan admission above
	// rejects block parameters, yields, block sends and block_given? operations,
	// so it is safe to clear the transient block while the frame-free region
	// runs.  This is the important bridge for Gem code: a helper called from an
	// Array/Hash callback can now use the same no-frame region as a top-level
	// helper instead of paying invokeMethod + a Ruby Frame for every element.
	previousBlock := vm.currentBlock
	previousClassStack := vm.classStack
	vm.currentBlock = nil
	if methodObj.Closure != nil && methodObj.Closure.ClassStack != nil {
		vm.classStack = methodObj.Closure.ClassStack
	}
	result, executed := vm.tryExecuteRegisterIRDirectNoFrame(entry.plan, fn, receiver, args, false, entry.allowConstants)
	vm.currentBlock = previousBlock
	vm.classStack = previousClassStack
	return result, executed
}

// tryExecuteCachedTypedHotMethod is the send-cache counterpart of
// tryExecuteTypedHotFunction.  Cached Register IR sends normally go straight
// to the fixed-arity bytecode fallback when their leaf/framed plan misses.
// That bypassed the method-level direct tier for ordinary Ruby methods whose
// body has a branch followed by one terminal ivar write. Keep the bridge small
// and side-effect-free on a non-Ruby/native method so callers can place it
// immediately before that fallback without repeating method lookup.
func (vm *VM) tryExecuteCachedTypedHotMethod(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if methodObj == nil {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil {
		return nil, false
	}
	return vm.tryExecuteTypedHotFunction(methodObj, fn, receiver, args)
}

// aggressiveMethodPlanSideExitSafe narrows the frame-free aggressive method
// tier to graphs that cannot mutate a receiver or a collection before a later
// proof miss.  Dynamic sends are intentionally allowed: the aggressive send
// ABI completes them exactly once and returns a value or exception.  Methods
// with state writes continue through the compatibility Frame path.
func aggressiveMethodPlanSideExitSafe(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	if plan.aggressiveMethodChecked && !plan.aggressiveMethodSafe {
		return false
	}
	if plan.aggressiveMethodChecked {
		if plan.aggressiveMethodSafe && plan.aggressiveMethodSideExitChecked {
			return plan.aggressiveMethodSideExitSafe
		}
	} else if !registerIRAggressivePlanSafe(plan, false) {
		plan.aggressiveMethodChecked = true
		plan.aggressiveMethodSafe = false
		return false
	}
	if plan.aggressiveMethodSideExitChecked {
		return plan.aggressiveMethodSideExitSafe
	}
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRStoreInstanceVar, registerIRIndexAssign, registerIRSetStringEncoding:
			plan.aggressiveMethodSideExitChecked = true
			plan.aggressiveMethodSideExitSafe = false
			return false
		}
	}
	plan.aggressiveMethodSideExitChecked = true
	plan.aggressiveMethodSideExitSafe = true
	return true
}

// tryExecuteAggressiveHotFunction is the method-level counterpart of the
// aggressive block executor.  It removes the Ruby Frame and bytecode loop for
// fixed-arity branch/send methods while preserving a clean compatibility
// fallback for writes, closures, yields, defaults, refinements, tracing and
// active unwind state.  This path is opt-in through RGO_ENABLE_REGISTER_IR_AGGRESSIVE
// because backtrace metadata for unusual exceptions belongs to the normal VM.
func (vm *VM) tryExecuteAggressiveHotFunction(methodObj *object.Method, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || methodObj == nil || fn == nil || receiver == nil ||
		methodObj.DispatchOwner != nil || methodObj.Ruby2Keywords ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") ||
		methodObj.OriginalName == "method_missing" || vm.currentBlock != nil ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 ||
		len(vm.activeRescues) != 0 || len(vm.rescueStack) != 0 ||
		vm.pendingReturnTargetID != 0 || vm.pendingBreakTargetID != 0 ||
		methodUsesRefinements(methodObj) || len(args) != len(fn.Params) ||
		len(fn.ParamLocalIndices) != len(fn.Params) || fn.HasRestParam ||
		fn.HasBlockParam || len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" ||
		fn.KeywordRestOnly || fn.RejectKeywords || fn.RejectBlock ||
		!simpleBlockParameterPatterns(fn) || registerIRFunctionNeedsDefaultEvaluation(fn, len(args)) {
		return nil, false
	}
	plan, ok := vm.aggressiveIRPlanForFunction(fn, false)
	if !ok || !aggressiveMethodPlanSideExitSafe(plan) ||
		!registerIRDirectConstantsSafe(vm, methodObj.Closure, plan) {
		return nil, false
	}
	var free []*object.EmeraldValue
	if methodObj.Closure != nil {
		free = methodObj.Closure.Free
	}
	return vm.executeRegisterIRDirectNoFrameWithFreeMode(
		plan, fn, receiver, args, object.CurrentMethodGeneration(), free,
		true, true, true, true, true,
	)
}
