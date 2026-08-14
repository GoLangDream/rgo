package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedIntegerMethodEnabled admits a method-level typed tier for pure integer
// functions.  The ordinary VM remains authoritative: the tier only accepts a
// bytecode proof containing parameter loads, immediate integer constants,
// checked scalar arithmetic and a local return.  Any miss (type, generation,
// overflow, or unsupported shape) re-enters the normal binder/frame path.
var typedIntegerMethodEnabled = os.Getenv("RGO_DISABLE_TYPED_INTEGER_METHOD") == ""

type typedIntegerMethodEntry struct {
	plan     *integerFunctionPlan
	calls    uint8
	disabled bool
}

// tryExecuteTypedIntegerMethod is the first in-process compiled-function tier.
// Unlike executeRegisterIRIntegerOnly it is called at the method boundary, so
// a hot pure callee avoids Ruby argument binding, Frame setup, EmeraldValue
// decoding inside the body, and the per-op boxed register file.  A small hot
// threshold avoids paying plan setup for one-shot methods and a failed shape
// is remembered for the lifetime of this VM.
func (vm *VM) tryExecuteTypedIntegerMethod(methodObj *object.Method, fn *object.Function, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !typedIntegerMethodEnabled || methodObj == nil || fn == nil ||
		methodObj.DispatchOwner != nil || methodObj.Ruby2Keywords ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") ||
		vm.currentBlock != nil || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || len(vm.catchStack) != 0 ||
		methodUsesRefinements(methodObj) || len(args) != len(fn.ParamLocalIndices) || len(args) > 16 {
		return nil, false
	}
	for _, argument := range args {
		if !typedIntegerArgument(argument) {
			return nil, false
		}
	}
	entry, found := vm.typedIntegerMethods[fn]
	if !found {
		plan, supported := buildIntegerFunctionPlan(fn)
		entry = typedIntegerMethodEntry{plan: plan, disabled: !supported || plan == nil}
		vm.typedIntegerMethods[fn] = entry
	}
	if entry.disabled || entry.plan == nil {
		return nil, false
	}
	if entry.calls < typedIntegerMethodHotThreshold {
		entry.calls++
		vm.typedIntegerMethods[fn] = entry
		return nil, false
	}
	for _, step := range entry.plan.steps {
		switch step.op {
		case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod,
			compiler.OpBitAnd, compiler.OpBitOr, compiler.OpBitXor,
			compiler.OpBitLeftShift, compiler.OpBitRightShift:
			if !vm.fusedIntegerOperationAvailable(step.op) {
				return nil, false
			}
		}
	}
	var values [16]int64
	for index, argument := range args {
		values[index] = argument.Data.(int64)
	}
	result, executed := vm.executeCachedIntegerFunctionPlan(fn, entry.plan, values[:len(args)])
	if !executed {
		// A pure plan can only miss because of a checked arithmetic overflow or
		// a generation guard. Keep the fallback armed for future BigInt/override
		// calls; the normal method then supplies Ruby's exact semantics.
		return nil, false
	}
	return core.NewIntegerValue(result), true
}

const typedIntegerMethodHotThreshold uint8 = 3

func typedIntegerArgument(value *object.EmeraldValue) bool {
	return typedIntegerArgumentClass(value, core.R.Classes["Integer"])
}

func typedIntegerArgumentClass(value *object.EmeraldValue, integerClass *object.Class) bool {
	return value != nil && value.Type == object.ValueInteger && value.BigIntValue() == nil &&
		(value.Class == nil || value.Class == integerClass)
}
