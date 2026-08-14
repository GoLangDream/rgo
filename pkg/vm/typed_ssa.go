package vm

import (
	"fmt"
	"math"
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// typedSSAEnabled controls the first genuinely typed in-process compiler tier.
// Register IR is still the compatibility representation; a typedSSAPlan is a
// compact, predecoded graph which runs without an EmeraldValue register file.
var typedSSAEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA") == ""
var typedSSAReferenceFunctionEnabled = os.Getenv("RGO_ENABLE_TYPED_SSA_REFERENCE_FUNCTION") != "" &&
	os.Getenv("RGO_DISABLE_TYPED_SSA_REFERENCE_FUNCTION") == ""
var typedSSAReferenceFunctionDebug = os.Getenv("RGO_DEBUG_TYPED_SSA_REFERENCE") != ""
var typedSSAIntegerStringConcatEnabled = os.Getenv("RGO_DISABLE_TYPED_INTEGER_STRING_CONCAT") == ""
var typedSSAIntegerStringBatchEnabled = os.Getenv("RGO_DISABLE_TYPED_INTEGER_STRING_BATCH") == ""

// typedSSABlockCallsEnabled admits only the side-effect-free block call graph
// proved by typedSSAPlanSafeForBlockCalls below. Keeping a switch makes A/B
// measurements and compatibility bisects cheap without weakening the normal
// VM fallback.
var typedSSABlockCallsEnabled = os.Getenv("RGO_DISABLE_TYPED_SSA_BLOCK_CALLS") == ""

type typedSSAValueKind uint8

const (
	typedSSAInvalid typedSSAValueKind = iota
	typedSSAInteger
	typedSSAFloat
	typedSSAString
	typedSSABool
	typedSSANil
	typedSSAReference
)

// typedSSAValue is deliberately small.  Integer, boolean and nil values never
// allocate while a compiled function is running; reference values retain the
// original Ruby pointer and are only admitted for operations whose semantics
// do not dispatch (self/ivar reads, truthiness, moves and returns).
type typedSSAValue struct {
	kind  typedSSAValueKind
	int   int64
	float float64
	str   string
	bool  bool
	ref   *object.EmeraldValue
}

type typedSSAOpKind uint8

const (
	typedSSAOpLoadParam typedSSAOpKind = iota
	typedSSAOpLoadLiteral
	typedSSAOpLoadInstanceVar
	typedSSAOpLoadSelf
	typedSSAOpLoadFree
	typedSSAOpLoadLocal
	typedSSAOpMove
	typedSSAOpSwap
	typedSSAOpBang
	typedSSAOpStoreLocal
	typedSSAOpEqual
	typedSSAOpCompare
	typedSSAOpBinary
	typedSSAOpJump
	typedSSAOpJumpTruthy
	typedSSAOpJumpNotTruthy
	typedSSAOpJumpNotNil
	typedSSAOpIndex
	typedSSAOpStoreInstanceVar
	typedSSAOpYield
	typedSSAOpReturn
	typedSSAOpCall
)

type typedSSAOp struct {
	kind     typedSSAOpKind
	implicit bool
	dst      uint8
	left     uint8
	right    uint8
	param    uint8
	argc     uint8
	args     [4]uint8
	splat    uint8
	target   int
	opcode   compiler.Opcode
	literal  typedSSAValue
	name     string
}

// typedSSAPlan is compiled once from Register IR.  No bytecode decoding,
// stack-depth bookkeeping or method lookup occurs on the steady-state path.
type typedSSAPlan struct {
	ops                          []typedSSAOp
	registers                    uint8
	locals                       int
	integerOps                   []compiler.Opcode
	hasFloat                     bool
	hasString                    bool
	hasReference                 bool
	hasInstanceStore             bool
	blockReturn                  bool
	hasYield                     bool
	integerKernel                typedSSAIntegerKernel
	effectfulIntegerKernel       typedSSAEffectfulIntegerKernel
	effectfulIntegerSafeChecked  bool
	effectfulIntegerSafe         bool
	primitiveIntegerStringKernel typedSSAPrimitiveIntegerStringKernel
	integerStringConcatKernel    typedSSAIntegerStringConcatKernel
}

type typedSSAIntegerKernelKind uint8

const (
	typedSSAIntegerKernelNone typedSSAIntegerKernelKind = iota
	typedSSAIntegerKernelCompareBinary
	typedSSAIntegerKernelClamp
)

// typedSSAIntegerKernel is a compact lowering for the common branch-shaped
// integer helper (`if x <op> literal; x +|- literal; else; x +|- literal`).
// The general typed SSA executor remains the fallback for all other graphs;
// this kernel removes the per-op switch from integer loop callbacks.
type typedSSAIntegerKernel struct {
	kind         typedSSAIntegerKernelKind
	compare      compiler.Opcode
	compareValue int64
	truthyOp     compiler.Opcode
	truthyValue  int64
	falsyOp      compiler.Opcode
	falsyValue   int64
	valueArg     uint8
	lowArg       uint8
	highArg      uint8
}

type typedSSAEffectfulIntegerKernelKind uint8

const (
	typedSSAEffectfulIntegerKernelNone typedSSAEffectfulIntegerKernelKind = iota
	typedSSAEffectfulIntegerKernelCompareToSStore
	typedSSAEffectfulIntegerKernelInstanceBinary
)

// typedSSAEffectfulIntegerKernel is the predecoded form of a common Ruby
// setter used inside counted loops: compare the Integer argument, format it
// with the built-in to_s on one branch, select a String literal on the other,
// then perform one terminal instance-variable write. The generic effectful
// executor remains the fallback for all other graphs.
type typedSSAEffectfulIntegerKernel struct {
	kind          typedSSAEffectfulIntegerKernelKind
	compare       compiler.Opcode
	compareValue  int64
	falseString   string
	instanceVar   string
	binary        compiler.Opcode
	parameterLeft bool
}

func typedSSAEffectfulIntegerKernelCondition(kernel typedSSAEffectfulIntegerKernel, argument int64) (bool, bool) {
	if kernel.kind != typedSSAEffectfulIntegerKernelCompareToSStore {
		return false, false
	}
	switch kernel.compare {
	case compiler.OpLessThan:
		return argument < kernel.compareValue, true
	case compiler.OpLessThanOrEqual:
		return argument <= kernel.compareValue, true
	case compiler.OpGreaterThan:
		return argument > kernel.compareValue, true
	case compiler.OpGreaterThanOrEqual:
		return argument >= kernel.compareValue, true
	default:
		return false, false
	}
}

type typedSSAPrimitiveIntegerStringKernelKind uint8

const (
	typedSSAPrimitiveIntegerStringKernelNone typedSSAPrimitiveIntegerStringKernelKind = iota
	typedSSAPrimitiveIntegerStringKernelCompareToS
)

// typedSSAPrimitiveIntegerStringKernel is the pure sibling of the
// compare/to_s/store kernel. It covers a helper such as
// `value > 3 ? value.to_s : ""`; the caller owns any outer assignment, so
// the kernel only needs to select and box the String result.
type typedSSAPrimitiveIntegerStringKernel struct {
	kind         typedSSAPrimitiveIntegerStringKernelKind
	compare      compiler.Opcode
	compareValue int64
	falseString  string
}

type typedSSAIntegerStringConcatKernelKind uint8

const (
	typedSSAIntegerStringConcatKernelNone typedSSAIntegerStringConcatKernelKind = iota
	typedSSAIntegerStringConcatKernelPrefix
	typedSSAIntegerStringConcatKernelSuffix
)

// typedSSAIntegerStringConcatKernel is the straight-line sibling of the
// branch-shaped primitive String kernel. It covers both `"prefix" +
// value.to_s` and `value.to_s + "suffix"`; the batch caller can format and
// box the final String directly into its backing region without creating the
// intermediate Integer#to_s string.
type typedSSAIntegerStringConcatKernel struct {
	kind    typedSSAIntegerStringConcatKernelKind
	literal string
}

const typedStringBatchMaxValues = 1 << 16

type typedSSAEntry struct {
	plan                *typedSSAPlan
	generation          uint64
	disabled            bool
	referencePlan       *typedSSAPlan
	referenceChecked    bool
	referencePlanFailed bool
}

type typedSSAReferenceCallKey struct {
	receiver *object.EmeraldValue
	name     string
	argc     uint8
}

type typedSSAReferenceClassCallKey struct {
	class *object.Class
	name  string
	argc  uint8
}

type typedSSAReferenceCallEntry struct {
	generation uint64
	method     *object.Method
	owner      *object.Class
	fn         *object.Function
	plan       *typedSSAPlan
	// nativeFn is the fixed-arity native ABI counterpart of fn/plan.  A
	// reference SSA block may call a native method without allocating a Ruby
	// frame, but only after the same public/owner/generation proof used for a
	// Ruby callee.  Keep this separate from plan so callers never mistake a
	// native edge for a Ruby bytecode graph.
	nativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
}

func typedSSAValueFromObject(value *object.EmeraldValue) typedSSAValue {
	if value == nil || value.Type == object.ValueNil {
		return typedSSAValue{kind: typedSSANil}
	}
	switch value.Type {
	case object.ValueBool:
		truthy, _ := value.Data.(bool)
		return typedSSAValue{kind: typedSSABool, bool: truthy}
	case object.ValueInteger:
		// A BigInt or Integer subclass can override arithmetic. Keep it as a
		// reference so a later arithmetic operation deopts before user code.
		if value.BigIntValue() == nil && (value.Class == nil || value.Class == core.R.Classes["Integer"]) {
			if number, ok := value.Data.(int64); ok {
				return typedSSAValue{kind: typedSSAInteger, int: number}
			}
		}
		return typedSSAValue{kind: typedSSAReference, ref: value}
	case object.ValueFloat:
		// Float subclasses and singleton overrides may replace arithmetic or
		// comparison methods. Keep those values boxed so the primitive tier only
		// executes exact built-in Float receivers.
		if (value.Class == nil || value.Class == core.R.Classes["Float"]) && core.AttachedSingletonClass(value) == nil {
			if number, ok := value.Data.(float64); ok {
				return typedSSAValue{kind: typedSSAFloat, float: number}
			}
		}
		return typedSSAValue{kind: typedSSAReference, ref: value}
	case object.ValueString:
		// Raw strings are admitted only for the immutable-compatible subset.
		// Mutable operations (`<<`, `[]=`, instance-variable writes), unusual
		// encodings and singleton methods keep the original object so a later
		// operation can preserve identity, mutation and coercion semantics.
		if (value.Class == nil || value.Class == core.R.Classes["String"]) &&
			core.AttachedSingletonClass(value) == nil && len(value.InstanceVars) == 0 &&
			!value.Frozen && !value.Chilled && (value.Encoding == "" || value.Encoding == "UTF-8") {
			if raw, ok := value.Data.(string); ok {
				return typedSSAValue{kind: typedSSAString, str: raw}
			}
		}
		return typedSSAValue{kind: typedSSAReference, ref: value}
	default:
		return typedSSAValue{kind: typedSSAReference, ref: value}
	}
}

// typedSSAValueFromObjectWithRef keeps the original boxed value alongside its
// raw representation. A typed callee may need the exact Ruby object for a
// nested native send or for an identity-preserving terminal store; creating a
// second box would both waste work and change observable object identity.
func typedSSAValueFromObjectWithRef(value *object.EmeraldValue) typedSSAValue {
	result := typedSSAValueFromObject(value)
	if result.kind != typedSSAReference && value != nil {
		result.ref = value
	}
	return result
}

func typedSSAExactIntegerValue(value *object.EmeraldValue) (int64, bool) {
	return typedSSAExactIntegerValueForClass(value, core.R.Classes["Integer"])
}

// typedSSAExactIntegerValueForClass is the steady-state guard used by a
// proven integer callback. ValueInteger is an immediate Ruby value in this
// runtime, so it cannot have an attached singleton class; after the exact
// class check there is no identity map to consult. Callers that already hold
// the stable Integer class avoid a string-keyed runtime class-map lookup on
// every array element.
func typedSSAExactIntegerValueForClass(value *object.EmeraldValue, integerClass *object.Class) (int64, bool) {
	if value == nil || value.Type != object.ValueInteger || value.BigIntValue() != nil ||
		value.Class != nil && value.Class != integerClass {
		return 0, false
	}
	result, ok := value.Data.(int64)
	return result, ok
}

func typedSSAValueToObject(value typedSSAValue) *object.EmeraldValue {
	if value.ref != nil {
		return value.ref
	}
	switch value.kind {
	case typedSSAInteger:
		return core.NewIntegerValue(value.int)
	case typedSSAFloat:
		return core.NewFloatValue(value.float)
	case typedSSAString:
		return core.NewStringValue(value.str)
	case typedSSABool:
		if value.bool {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	case typedSSANil:
		return core.R.NilVal
	case typedSSAReference:
		if value.ref != nil {
			return value.ref
		}
	}
	return core.R.NilVal
}

// typedSSAValueToObjectForVM is the batch-aware boxing boundary for a proven
// typed collection region. Primitive String results still get distinct Ruby
// objects, but their headers can live in one batch backing slice; all other
// value kinds retain the ordinary canonical boxing rules.
func (vm *VM) typedSSAValueToObjectForVM(value typedSSAValue) *object.EmeraldValue {
	if value.ref == nil && value.kind == typedSSAString && vm != nil && vm.typedStringValueBatch != nil {
		return vm.typedStringValueBatch.New(value.str)
	}
	return typedSSAValueToObject(value)
}

func typedSSATruthy(value typedSSAValue) bool {
	switch value.kind {
	case typedSSANil:
		return false
	case typedSSABool:
		return value.bool
	default:
		return true
	}
}

func typedSSAImmediateEqual(vm *VM, left, right typedSSAValue) (typedSSAValue, bool) {
	if left.kind != right.kind {
		// Float#== accepts an Integer receiver, so treating a mixed pair as a
		// proven false result would be incorrect (1.0 == 1 is true).  Let the
		// boxed dispatcher handle mixed numeric equality until both operands
		// have a dedicated conversion guard.
		if left.kind == typedSSAFloat || right.kind == typedSSAString || right.kind == typedSSAFloat || left.kind == typedSSAString {
			return typedSSAValue{}, false
		}
		return typedSSAValue{kind: typedSSABool, bool: false}, true
	}
	if left.kind != typedSSAInteger && left.kind != typedSSAFloat && left.kind != typedSSAString && left.kind != typedSSABool && left.kind != typedSSANil {
		return typedSSAValue{}, false
	}
	if left.kind == typedSSAInteger && !vm.fusedIntegerOperationAvailable(compiler.OpEqual) {
		return typedSSAValue{}, false
	}
	if left.kind == typedSSAFloat && !vm.fusedFloatOperationAvailable(compiler.OpEqual) {
		return typedSSAValue{}, false
	}
	if left.kind == typedSSAString && !vm.fusedStringOperationAvailable(compiler.OpEqual) {
		return typedSSAValue{}, false
	}
	equal := false
	switch left.kind {
	case typedSSAInteger:
		equal = left.int == right.int
	case typedSSAFloat:
		equal = left.float == right.float
	case typedSSAString:
		equal = left.str == right.str
	case typedSSABool:
		equal = left.bool == right.bool
	case typedSSANil:
		equal = true
	}
	return typedSSAValue{kind: typedSSABool, bool: equal}, true
}

func typedSSABinary(vm *VM, opcode compiler.Opcode, left, right typedSSAValue) (typedSSAValue, bool) {
	if left.kind == typedSSAString && right.kind == typedSSAString {
		if opcode != compiler.OpAdd || !vm.fusedStringOperationAvailable(opcode) {
			return typedSSAValue{}, false
		}
		return typedSSAValue{kind: typedSSAString, str: left.str + right.str}, true
	}
	if left.kind == typedSSAFloat && right.kind == typedSSAFloat {
		if !vm.fusedFloatOperationAvailable(opcode) {
			return typedSSAValue{}, false
		}
		leftValue, rightValue := left.float, right.float
		if opcode == compiler.OpMod {
			// Float#% raises on zero and has explicit NaN/Infinity rules.
			// Keep those edges in the boxed implementation; finite non-zero
			// operands are lowered with the same floor-based definition.
			if rightValue == 0 || math.IsNaN(leftValue) || math.IsInf(leftValue, 0) ||
				math.IsNaN(rightValue) || math.IsInf(rightValue, 0) {
				return typedSSAValue{}, false
			}
			result := leftValue - math.Floor(leftValue/rightValue)*rightValue
			if result == 0 && math.Signbit(leftValue) {
				return typedSSAValue{}, false
			}
			return typedSSAValue{kind: typedSSAFloat, float: result}, true
		}
		var result float64
		switch opcode {
		case compiler.OpAdd:
			result = leftValue + rightValue
		case compiler.OpSub:
			result = leftValue - rightValue
		case compiler.OpMul:
			result = leftValue * rightValue
		case compiler.OpDiv:
			result = leftValue / rightValue
		}
		return typedSSAValue{kind: typedSSAFloat, float: result}, true
	}
	if left.kind != typedSSAInteger || right.kind != typedSSAInteger || !vm.fusedIntegerOperationAvailable(opcode) {
		return typedSSAValue{}, false
	}
	var result int64
	var ok bool
	switch opcode {
	case compiler.OpAdd:
		result, ok = checkedIntegerAdd(left.int, right.int)
	case compiler.OpSub:
		result, ok = checkedIntegerSub(left.int, right.int)
	case compiler.OpMul:
		result, ok = checkedIntegerMul(left.int, right.int)
	case compiler.OpMod:
		if right.int != 0 {
			result = left.int % right.int
			if result != 0 && (result < 0) != (right.int < 0) {
				result += right.int
			}
			ok = true
		}
	case compiler.OpBitAnd:
		result, ok = left.int&right.int, true
	case compiler.OpBitOr:
		result, ok = left.int|right.int, true
	case compiler.OpBitXor:
		result, ok = left.int^right.int, true
	case compiler.OpBitLeftShift:
		if right.int < 0 || right.int >= 64 {
			return typedSSAValue{}, false
		}
		result = left.int << uint(right.int)
		if result>>uint(right.int) != left.int {
			return typedSSAValue{}, false
		}
		ok = true
	case compiler.OpBitRightShift:
		if right.int < 0 {
			return typedSSAValue{}, false
		}
		if right.int >= 64 {
			if left.int < 0 {
				result = -1
			} else {
				result = 0
			}
		} else {
			result = left.int >> uint(right.int)
		}
		ok = true
	default:
		return typedSSAValue{}, false
	}
	if !ok {
		return typedSSAValue{}, false
	}
	return typedSSAValue{kind: typedSSAInteger, int: result}, true
}

func typedSSACompare(vm *VM, opcode compiler.Opcode, left, right typedSSAValue) (typedSSAValue, bool) {
	if left.kind == typedSSAFloat && right.kind == typedSSAFloat {
		if !vm.fusedFloatOperationAvailable(opcode) {
			return typedSSAValue{}, false
		}
		var result bool
		switch opcode {
		case compiler.OpLessThan:
			result = left.float < right.float
		case compiler.OpLessThanOrEqual:
			result = left.float <= right.float
		case compiler.OpGreaterThan:
			result = left.float > right.float
		case compiler.OpGreaterThanOrEqual:
			result = left.float >= right.float
		}
		return typedSSAValue{kind: typedSSABool, bool: result}, true
	}
	if left.kind != typedSSAInteger || right.kind != typedSSAInteger || !vm.fusedIntegerOperationAvailable(opcode) {
		return typedSSAValue{}, false
	}
	var result bool
	switch opcode {
	case compiler.OpLessThan:
		result = left.int < right.int
	case compiler.OpLessThanOrEqual:
		result = left.int <= right.int
	case compiler.OpGreaterThan:
		result = left.int > right.int
	case compiler.OpGreaterThanOrEqual:
		result = left.int >= right.int
	default:
		return typedSSAValue{}, false
	}
	return typedSSAValue{kind: typedSSABool, bool: result}, true
}

func compileTypedSSAPlan(fn *object.Function) (*typedSSAPlan, bool) {
	return compileTypedSSAPlanMode(fn, false, false)
}

func compileTypedSSAReferencePlan(fn *object.Function) (*typedSSAPlan, bool) {
	return compileTypedSSAPlanMode(fn, false, true)
}

// compileTypedSSAPlanWithBlockReturn shares the method compiler with the
// block tier.  OpBlockReturn has the same value-producing data flow as an
// ordinary return, but it must never be admitted for a method because its
// unwind protocol is different.  Keeping the mode explicit prevents a block
// plan from accidentally entering the method cache.
func compileTypedSSAPlanWithBlockReturn(fn *object.Function, allowBlockReturn bool) (*typedSSAPlan, bool) {
	return compileTypedSSAPlanMode(fn, allowBlockReturn, false)
}

func compileTypedSSAPlanMode(fn *object.Function, allowBlockReturn, allowImplicit bool) (*typedSSAPlan, bool) {
	if fn == nil || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		fn.RejectKeywords || fn.RejectBlock || !simpleBlockParameterPatterns(fn) ||
		registerIRFunctionNeedsDefaultEvaluation(fn, len(fn.Params)) || fn.NumLocals > 64 {
		return nil, false
	}
	// The ordinary Register IR keeps mutable String constants behind the
	// constantValue materializer.  A typed plan can safely admit that form only
	// after proving each use stays in the immutable raw-string subset; the
	// typed executor creates a fresh boxed result at the boundary and rejects
	// mutation/encoding operations below.  Do not require a process-wide string
	// experiment flag for this narrow, per-plan proof.
	registerOptions := defaultRegisterIRCompileOptions()
	registerOptions.allowStringLiterals = true
	registerPlan, ok := compileRegisterIRWithOptions(fn, registerOptions)
	if !ok || registerPlan == nil || (!allowBlockReturn && registerPlan.blockReturn) || registerPlan.registers > 16 {
		return nil, false
	}
	if registerPlan.hasImplicitSends {
		if typedSSAPlanOnlyYields(registerPlan) {
			if !allowBlockReturn {
				return nil, false
			}
		} else if !allowImplicit || !typedSSAImplicitSendPlanSafe(fn, registerPlan) {
			return nil, false
		}
	}
	result := &typedSSAPlan{registers: registerPlan.registers, locals: fn.NumLocals, blockReturn: registerPlan.blockReturn}
	result.ops = make([]typedSSAOp, 0, len(registerPlan.instructions))
	for _, instruction := range registerPlan.instructions {
		op := typedSSAOp{dst: instruction.dst, left: instruction.left, right: instruction.right, param: instruction.param, target: instruction.target, opcode: instruction.opcode, name: instruction.name}
		switch instruction.op {
		case registerIRLoadParam:
			op.kind = typedSSAOpLoadParam
		case registerIRLoadLiteral:
			op.kind = typedSSAOpLoadLiteral
			op.literal = typedSSAValueFromObject(instruction.value)
			if op.literal.kind == typedSSAInvalid {
				return nil, false
			}
			if op.literal.kind == typedSSAReference {
				result.hasReference = true
			}
			if op.literal.kind == typedSSAFloat {
				result.hasFloat = true
			}
			if op.literal.kind == typedSSAString {
				result.hasString = true
			}
		case registerIRLoadConstantValue:
			// This is the mutable-string materialization form emitted for a
			// normal Ruby literal.  The raw tier treats it as a fresh value and
			// therefore preserves per-evaluation identity as long as no mutation
			// or encoding operation is admitted into the plan.
			op.kind = typedSSAOpLoadLiteral
			op.literal = typedSSAValueFromObject(instruction.value)
			if op.literal.kind == typedSSAInvalid || op.literal.kind == typedSSAReference {
				return nil, false
			}
			if op.literal.kind == typedSSAString {
				result.hasString = true
			}
		case registerIRLoadFrozenString, registerIRSetStringEncoding:
			// Frozen identity and encoding are observable at Ruby boundaries;
			// keep these plans on the ordinary Register IR/Frame path.
			return nil, false
		case registerIRLoadInstanceVar:
			op.kind = typedSSAOpLoadInstanceVar
			result.hasReference = true
		case registerIRLoadSelf:
			op.kind = typedSSAOpLoadSelf
			result.hasReference = true
		case registerIRLoadFree:
			if !allowBlockReturn {
				return nil, false
			}
			op.kind = typedSSAOpLoadFree
			result.hasReference = true
		case registerIRLoadLocal:
			if instruction.param >= 64 {
				return nil, false
			}
			op.kind = typedSSAOpLoadLocal
		case registerIRMove:
			op.kind = typedSSAOpMove
		case registerIRSwap:
			op.kind = typedSSAOpSwap
		case registerIRBang:
			op.kind = typedSSAOpBang
		case registerIRStoreLocal:
			if instruction.param >= 64 {
				return nil, false
			}
			op.kind = typedSSAOpStoreLocal
		case registerIREqual:
			op.kind = typedSSAOpEqual
			result.integerOps = append(result.integerOps, compiler.OpEqual)
		case registerIRCompare:
			op.kind = typedSSAOpCompare
			if instruction.opcode != compiler.OpLessThan && instruction.opcode != compiler.OpLessThanOrEqual && instruction.opcode != compiler.OpGreaterThan && instruction.opcode != compiler.OpGreaterThanOrEqual {
				return nil, false
			}
			result.integerOps = append(result.integerOps, instruction.opcode)
		case registerIRBinary:
			op.kind = typedSSAOpBinary
			switch instruction.opcode {
			case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod,
				compiler.OpBitAnd, compiler.OpBitOr, compiler.OpBitXor,
				compiler.OpBitLeftShift, compiler.OpBitRightShift:
				result.integerOps = append(result.integerOps, instruction.opcode)
			case compiler.OpDiv:
				// Integer division is intentionally not admitted by the raw
				// integer ABI yet; Float division is handled by typedSSABinary.
				// Keep the opcode in the generation list so an integer caller
				// cannot repeatedly attempt the unboxed ABI and miss every time.
				result.integerOps = append(result.integerOps, instruction.opcode)
			default:
				return nil, false
			}
		case registerIRJump:
			op.kind = typedSSAOpJump
		case registerIRJumpTruthy:
			op.kind = typedSSAOpJumpTruthy
		case registerIRJumpNotTruthy:
			op.kind = typedSSAOpJumpNotTruthy
		case registerIRJumpNotNil:
			op.kind = typedSSAOpJumpNotNil
		case registerIRIndex:
			if !allowBlockReturn {
				return nil, false
			}
			op.kind = typedSSAOpIndex
			result.hasReference = true
		case registerIRStoreInstanceVar:
			// Effectful typed methods may commit one terminal instance-variable
			// write. The caller must prove the store is immediately before the
			// method return; the compiler itself still records the operation so the
			// typed executor can preserve assignment identity and frozen errors.
			if allowBlockReturn || !registerIRDirectTerminalMutationAt(registerPlan, len(result.ops), instruction.left) {
				return nil, false
			}
			op.kind = typedSSAOpStoreInstanceVar
			result.hasReference = true
			result.hasInstanceStore = true
		case registerIRYield:
			if !allowBlockReturn {
				return nil, false
			}
			if instruction.splatIndex != 255 {
				return nil, false
			}
			op.kind = typedSSAOpYield
			op.argc = instruction.argc
			op.args = instruction.args
			op.splat = instruction.splatIndex
			result.hasReference = true
			result.hasYield = true
		case registerIRReturn:
			op.kind = typedSSAOpReturn
		case registerIRSend:
			// A nested call is admitted only as a conservative reference ABI.
			// The callee is resolved at execution time under the current method
			// generation, so redefinition/refinement/singleton changes still
			// deopt. Blocks, splats and keyword sends retain the full VM protocol.
			if instruction.opcode != compiler.OpSend || instruction.blockPresent || instruction.splatIndex != 255 || instruction.argc > 4 {
				return nil, false
			}
			op.kind = typedSSAOpCall
			if instruction.byteIP >= 0 && instruction.byteIP+3 < len(fn.Instructions) {
				op.implicit = fn.Instructions[instruction.byteIP+3] == 3
			}
			op.argc = instruction.argc
			op.args = instruction.args
			// Integer#to_s is a pure primitive edge when it has no
			// arguments. Keep it out of the reference bit so a raw Integer
			// caller can carry the result as a String; the executor still
			// falls back to the identity-preserving reference edge when the
			// actual receiver is not an exact Integer or the builtin guard
			// has been invalidated.
			if instruction.name != "to_s" || instruction.argc != 0 {
				result.hasReference = true
			}
		default:
			return nil, false
		}
		result.ops = append(result.ops, op)
	}
	if len(result.ops) == 0 || result.ops[len(result.ops)-1].kind != typedSSAOpReturn {
		return nil, false
	}
	result.integerKernel = detectTypedSSAIntegerKernel(result)
	result.effectfulIntegerKernel = detectTypedSSAEffectfulIntegerKernel(result)
	result.primitiveIntegerStringKernel = detectTypedSSAPrimitiveIntegerStringKernel(result)
	result.integerStringConcatKernel = detectTypedSSAIntegerStringConcatKernel(result)
	return result, true
}

// Implicit sends normally need the full Ruby visibility/block protocol.  The
// typed reference ABI only needs the closed-world zero-argument form; all
// other implicit sends remain on the compatibility path.
func typedSSAImplicitSendPlanSafe(fn *object.Function, plan *registerIRPlan) bool {
	if fn == nil || plan == nil || !plan.hasImplicitSends {
		return true
	}
	for _, instruction := range plan.instructions {
		if instruction.op == registerIRYield {
			return false
		}
		if instruction.op != registerIRSend || instruction.byteIP < 0 || instruction.byteIP+3 >= len(fn.Instructions) || fn.Instructions[instruction.byteIP+3] != 3 {
			continue
		}
		if instruction.argc != 0 || instruction.blockPresent || instruction.splatIndex != 255 {
			return false
		}
	}
	return true
}

func typedSSAPlanOnlyYields(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	foundYield := false
	for _, instruction := range plan.instructions {
		if instruction.op == registerIRYield {
			foundYield = true
			continue
		}
		// A block-argument send or an implicit `yield` encoded through send
		// needs the caller's full unwind protocol.  A typed block may only use
		// the dedicated registerIRYield operation as its implicit send.
		if instruction.op == registerIRSend {
			return false
		}
	}
	return foundYield
}

func detectTypedSSAIntegerKernel(plan *typedSSAPlan) typedSSAIntegerKernel {
	if plan == nil || plan.hasReference {
		return typedSSAIntegerKernel{}
	}
	// `clamp(value, low, high)` is a common branch-shaped callee in numeric
	// loops.  Lower the two comparisons and three returns to one straight Go
	// function so a hot region does not interpret fourteen SSA instructions on
	// every iteration.
	if len(plan.ops) == 14 {
		ops := plan.ops
		if ops[0].kind == typedSSAOpLoadParam && ops[0].param == 0 &&
			ops[1].kind == typedSSAOpLoadParam && ops[1].param == 1 &&
			ops[2].kind == typedSSAOpCompare && ops[2].opcode == compiler.OpLessThan &&
			ops[2].left == ops[0].dst && ops[2].right == ops[1].dst &&
			ops[3].kind == typedSSAOpJumpNotTruthy && ops[3].left == ops[2].dst && ops[3].target == 6 &&
			ops[4].kind == typedSSAOpLoadParam && ops[4].param == 1 &&
			ops[5].kind == typedSSAOpJump && ops[5].target == 13 &&
			ops[6].kind == typedSSAOpLoadParam && ops[6].param == 0 &&
			ops[7].kind == typedSSAOpLoadParam && ops[7].param == 2 &&
			ops[8].kind == typedSSAOpCompare && ops[8].opcode == compiler.OpGreaterThan &&
			ops[8].left == ops[6].dst && ops[8].right == ops[7].dst &&
			ops[9].kind == typedSSAOpJumpNotTruthy && ops[9].left == ops[8].dst && ops[9].target == 12 &&
			ops[10].kind == typedSSAOpLoadParam && ops[10].param == 2 &&
			ops[11].kind == typedSSAOpJump && ops[11].target == 13 &&
			ops[12].kind == typedSSAOpLoadParam && ops[12].param == 0 &&
			ops[13].kind == typedSSAOpReturn && ops[13].left == ops[12].dst {
			return typedSSAIntegerKernel{
				kind:     typedSSAIntegerKernelClamp,
				valueArg: 0, lowArg: 1, highArg: 2,
			}
		}
	}
	if len(plan.ops) != 12 {
		return typedSSAIntegerKernel{}
	}
	ops := plan.ops
	if ops[0].kind != typedSSAOpLoadParam || ops[0].param != 0 ||
		ops[1].kind != typedSSAOpLoadLiteral || ops[1].literal.kind != typedSSAInteger ||
		ops[2].kind != typedSSAOpCompare || ops[2].left != ops[0].dst || ops[2].right != ops[1].dst ||
		ops[3].kind != typedSSAOpJumpNotTruthy || ops[3].left != ops[2].dst || ops[3].target != 8 ||
		ops[4].kind != typedSSAOpLoadParam || ops[4].param != 0 ||
		ops[5].kind != typedSSAOpLoadLiteral || ops[5].literal.kind != typedSSAInteger ||
		ops[6].kind != typedSSAOpBinary || ops[6].left != ops[4].dst || ops[6].right != ops[5].dst ||
		ops[7].kind != typedSSAOpJump || ops[7].target != 11 ||
		ops[8].kind != typedSSAOpLoadParam || ops[8].param != 0 ||
		ops[9].kind != typedSSAOpLoadLiteral || ops[9].literal.kind != typedSSAInteger ||
		ops[10].kind != typedSSAOpBinary || ops[10].left != ops[8].dst || ops[10].right != ops[9].dst ||
		ops[11].kind != typedSSAOpReturn || ops[11].left != ops[6].dst || ops[10].dst != ops[6].dst {
		return typedSSAIntegerKernel{}
	}
	switch ops[2].opcode {
	case compiler.OpLessThan, compiler.OpLessThanOrEqual, compiler.OpGreaterThan, compiler.OpGreaterThanOrEqual:
	default:
		return typedSSAIntegerKernel{}
	}
	switch ops[6].opcode {
	case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod, compiler.OpBitAnd:
	default:
		return typedSSAIntegerKernel{}
	}
	if ops[10].opcode != compiler.OpAdd && ops[10].opcode != compiler.OpSub && ops[10].opcode != compiler.OpMul && ops[10].opcode != compiler.OpMod && ops[10].opcode != compiler.OpBitAnd {
		return typedSSAIntegerKernel{}
	}
	return typedSSAIntegerKernel{
		kind:         typedSSAIntegerKernelCompareBinary,
		compare:      ops[2].opcode,
		compareValue: ops[1].literal.int,
		truthyOp:     ops[6].opcode,
		truthyValue:  ops[5].literal.int,
		falsyOp:      ops[10].opcode,
		falsyValue:   ops[9].literal.int,
	}
}

func detectTypedSSAEffectfulIntegerKernel(plan *typedSSAPlan) typedSSAEffectfulIntegerKernel {
	if plan == nil || !plan.hasInstanceStore {
		return typedSSAEffectfulIntegerKernel{}
	}
	if len(plan.ops) == 5 {
		var loadInstance, loadParam, binary *typedSSAOp
		var store, result *typedSSAOp
		for index := range plan.ops {
			instruction := &plan.ops[index]
			switch instruction.kind {
			case typedSSAOpLoadInstanceVar:
				if loadInstance != nil {
					return typedSSAEffectfulIntegerKernel{}
				}
				loadInstance = instruction
			case typedSSAOpLoadParam:
				if loadParam != nil || instruction.param != 0 {
					return typedSSAEffectfulIntegerKernel{}
				}
				loadParam = instruction
			case typedSSAOpBinary:
				if binary != nil {
					return typedSSAEffectfulIntegerKernel{}
				}
				switch instruction.opcode {
				case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod, compiler.OpBitAnd:
				default:
					return typedSSAEffectfulIntegerKernel{}
				}
				binary = instruction
			case typedSSAOpStoreInstanceVar:
				if store != nil {
					return typedSSAEffectfulIntegerKernel{}
				}
				store = instruction
			case typedSSAOpReturn:
				if result != nil {
					return typedSSAEffectfulIntegerKernel{}
				}
				result = instruction
			default:
				return typedSSAEffectfulIntegerKernel{}
			}
		}
		if loadInstance == nil || loadParam == nil || binary == nil || store == nil || result == nil ||
			store.name != loadInstance.name || store.left != binary.dst || result.left != store.left {
			return typedSSAEffectfulIntegerKernel{}
		}
		parameterLeft := false
		switch {
		case binary.left == loadInstance.dst && binary.right == loadParam.dst:
		case binary.left == loadParam.dst && binary.right == loadInstance.dst:
			parameterLeft = true
		default:
			return typedSSAEffectfulIntegerKernel{}
		}
		return typedSSAEffectfulIntegerKernel{
			kind:          typedSSAEffectfulIntegerKernelInstanceBinary,
			instanceVar:   loadInstance.name,
			binary:        binary.opcode,
			parameterLeft: parameterLeft,
		}
	}
	if len(plan.ops) != 10 {
		return typedSSAEffectfulIntegerKernel{}
	}
	ops := plan.ops
	if ops[0].kind != typedSSAOpLoadParam || ops[0].param != 0 ||
		ops[1].kind != typedSSAOpLoadLiteral || ops[1].literal.kind != typedSSAInteger ||
		ops[2].kind != typedSSAOpCompare || ops[2].left != ops[0].dst || ops[2].right != ops[1].dst ||
		ops[3].kind != typedSSAOpJumpNotTruthy || ops[3].left != ops[2].dst || ops[3].target != 7 ||
		ops[4].kind != typedSSAOpLoadParam || ops[4].param != 0 ||
		ops[5].kind != typedSSAOpCall || ops[5].name != "to_s" || ops[5].argc != 0 || ops[5].implicit ||
		ops[5].left != ops[4].dst || ops[5].dst != ops[4].dst ||
		ops[6].kind != typedSSAOpJump || ops[6].target != 8 ||
		ops[7].kind != typedSSAOpLoadLiteral || ops[7].literal.kind != typedSSAString ||
		ops[7].dst != ops[5].dst ||
		ops[8].kind != typedSSAOpStoreInstanceVar || ops[8].left != ops[5].dst || ops[8].name == "" ||
		ops[9].kind != typedSSAOpReturn || ops[9].left != ops[8].left {
		return typedSSAEffectfulIntegerKernel{}
	}
	switch ops[2].opcode {
	case compiler.OpLessThan, compiler.OpLessThanOrEqual, compiler.OpGreaterThan, compiler.OpGreaterThanOrEqual:
	default:
		return typedSSAEffectfulIntegerKernel{}
	}
	return typedSSAEffectfulIntegerKernel{
		kind:         typedSSAEffectfulIntegerKernelCompareToSStore,
		compare:      ops[2].opcode,
		compareValue: ops[1].literal.int,
		falseString:  ops[7].literal.str,
		instanceVar:  ops[8].name,
	}
}

func detectTypedSSAPrimitiveIntegerStringKernel(plan *typedSSAPlan) typedSSAPrimitiveIntegerStringKernel {
	if plan == nil || len(plan.ops) != 9 || plan.hasReference || plan.hasInstanceStore {
		return typedSSAPrimitiveIntegerStringKernel{}
	}
	ops := plan.ops
	if ops[0].kind != typedSSAOpLoadParam || ops[0].param != 0 ||
		ops[1].kind != typedSSAOpLoadLiteral || ops[1].literal.kind != typedSSAInteger ||
		ops[2].kind != typedSSAOpCompare || ops[2].left != ops[0].dst || ops[2].right != ops[1].dst ||
		ops[3].kind != typedSSAOpJumpNotTruthy || ops[3].left != ops[2].dst || ops[3].target != 7 ||
		ops[4].kind != typedSSAOpLoadParam || ops[4].param != 0 ||
		ops[5].kind != typedSSAOpCall || ops[5].name != "to_s" || ops[5].argc != 0 || ops[5].implicit ||
		ops[5].left != ops[4].dst || ops[5].dst != ops[4].dst ||
		ops[6].kind != typedSSAOpJump || ops[6].target != 8 ||
		ops[7].kind != typedSSAOpLoadLiteral || ops[7].literal.kind != typedSSAString ||
		ops[7].dst != ops[5].dst ||
		ops[8].kind != typedSSAOpReturn || ops[8].left != ops[5].dst {
		return typedSSAPrimitiveIntegerStringKernel{}
	}
	switch ops[2].opcode {
	case compiler.OpLessThan, compiler.OpLessThanOrEqual, compiler.OpGreaterThan, compiler.OpGreaterThanOrEqual:
	default:
		return typedSSAPrimitiveIntegerStringKernel{}
	}
	return typedSSAPrimitiveIntegerStringKernel{
		kind:         typedSSAPrimitiveIntegerStringKernelCompareToS,
		compare:      ops[2].opcode,
		compareValue: ops[1].literal.int,
		falseString:  ops[7].literal.str,
	}
}

func detectTypedSSAIntegerStringConcatKernel(plan *typedSSAPlan) typedSSAIntegerStringConcatKernel {
	if plan == nil || len(plan.ops) != 5 || plan.hasReference || plan.hasInstanceStore {
		return typedSSAIntegerStringConcatKernel{}
	}
	ops := plan.ops
	if ops[0].kind != typedSSAOpLoadParam || ops[0].param != 0 ||
		ops[1].kind != typedSSAOpCall || ops[1].name != "to_s" || ops[1].argc != 0 || ops[1].implicit ||
		ops[1].left != ops[0].dst || ops[2].kind != typedSSAOpLoadLiteral ||
		ops[2].literal.kind != typedSSAString || ops[3].kind != typedSSAOpBinary ||
		ops[3].opcode != compiler.OpAdd || ops[4].kind != typedSSAOpReturn || ops[4].left != ops[3].dst {
		return typedSSAIntegerStringConcatKernel{}
	}
	if ops[3].left == ops[1].dst && ops[3].right == ops[2].dst {
		return typedSSAIntegerStringConcatKernel{kind: typedSSAIntegerStringConcatKernelSuffix, literal: ops[2].literal.str}
	}
	if ops[3].left == ops[2].dst && ops[3].right == ops[1].dst {
		return typedSSAIntegerStringConcatKernel{kind: typedSSAIntegerStringConcatKernelPrefix, literal: ops[2].literal.str}
	}
	return typedSSAIntegerStringConcatKernel{}
}

func (vm *VM) typedSSAIntegerOpsAvailable(plan *typedSSAPlan) bool {
	if plan == nil {
		return false
	}
	// A float literal proves that this graph is not eligible for the raw
	// integer ABI.  The general typed executor may still admit it and guard
	// each Float operation independently.
	if plan.hasFloat || plan.hasString {
		return false
	}
	for _, opcode := range plan.integerOps {
		if !vm.fusedIntegerOperationAvailable(opcode) {
			return false
		}
	}
	return true
}

// typedSSAUnboxedPlanGuardsAvailable is the broader admission check for raw
// Integer arguments whose result may be a primitive String. Integer operator
// guards are checked up front; String#+ and Integer#to_s are checked at their
// exact operation/call sites because a plan may contain either one.
func (vm *VM) typedSSAUnboxedPlanGuardsAvailable(plan *typedSSAPlan) bool {
	if vm == nil || plan == nil || plan.hasReference || plan.hasFloat {
		return false
	}
	for _, opcode := range plan.integerOps {
		if !vm.fusedIntegerOperationAvailable(opcode) {
			return false
		}
	}
	return true
}

func (vm *VM) executeTypedSSAPlan(plan *typedSSAPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, free ...[]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	return vm.executeTypedSSAPlanMode(plan, fn, receiver, args, nil, free...)
}

// executeTypedSSAEffectfulInstanceIntegerKernel lowers the common stateful
// integer update `@field = @field <op> value`. Both operands must be exact
// built-in Integers and the operation must still be fused by the runtime; a
// miss leaves the original typed/reference path responsible for Ruby's
// dynamic dispatch, BigInt promotion, and error behavior.
func (vm *VM) executeTypedSSAEffectfulInstanceIntegerKernel(kernel typedSSAEffectfulIntegerKernel, receiver *object.EmeraldValue, argument int64) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || kernel.kind != typedSSAEffectfulIntegerKernelInstanceBinary ||
		kernel.instanceVar == "" || !vm.fusedIntegerOperationAvailable(kernel.binary) {
		return nil, false
	}
	current, currentOK := typedSSAExactIntegerValueForClass(core.DynamicInstanceVar(receiver, kernel.instanceVar), core.R.Classes["Integer"])
	if !currentOK {
		return nil, false
	}
	left, right := current, argument
	if kernel.parameterLeft {
		left, right = right, left
	}
	var value int64
	var ok bool
	switch kernel.binary {
	case compiler.OpAdd:
		value, ok = checkedIntegerAdd(left, right)
	case compiler.OpSub:
		value, ok = checkedIntegerSub(left, right)
	case compiler.OpMul:
		value, ok = checkedIntegerMul(left, right)
	case compiler.OpMod:
		value, ok = checkedIntegerMod(left, right)
	case compiler.OpBitAnd:
		value = left & right
		ok = true
	default:
		return nil, false
	}
	if !ok {
		return nil, false
	}
	result := core.NewIntegerValue(value)
	if errorValue := core.SetDynamicInstanceVar(receiver, kernel.instanceVar, result); errorValue != nil {
		return errorValue, true
	}
	return result, true
}

// executeTypedSSAEffectfulIntegerPlan is the raw-argument entry for a typed
// method whose only changing input is an exact Integer value. The caller has
// already proved the plan cannot observe the Integer object identity, so it
// avoids allocating a boxed loop index before a primitive native edge such as
// Integer#to_s.
func (vm *VM) executeTypedSSAEffectfulIntegerPlan(plan *typedSSAPlan, fn *object.Function, receiver *object.EmeraldValue, argument int64, free ...[]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if plan != nil && plan.effectfulIntegerKernel.kind == typedSSAEffectfulIntegerKernelInstanceBinary {
		return vm.executeTypedSSAEffectfulInstanceIntegerKernel(plan.effectfulIntegerKernel, receiver, argument)
	}
	if plan != nil && plan.effectfulIntegerKernel.kind == typedSSAEffectfulIntegerKernelCompareToSStore {
		return vm.executeTypedSSAEffectfulIntegerKernel(plan.effectfulIntegerKernel, receiver, argument)
	}
	if typedSSAIntegerStringConcatEnabled && plan != nil && plan.integerStringConcatKernel.kind != typedSSAIntegerStringConcatKernelNone {
		return vm.executeTypedSSAIntegerStringConcatKernelTrusted(plan.integerStringConcatKernel, argument)
	}
	if plan != nil && plan.primitiveIntegerStringKernel.kind == typedSSAPrimitiveIntegerStringKernelCompareToS {
		return vm.executeTypedSSAPrimitiveIntegerStringKernel(plan.primitiveIntegerStringKernel, argument)
	}
	raw := typedSSAValue{kind: typedSSAInteger, int: argument}
	return vm.executeTypedSSAPlanMode(plan, fn, receiver, nil, &raw, free...)
}

// executeTypedSSAEffectfulIntegerObjectPlan is the boxed-argument sibling
// used by Array callbacks. Unlike the raw times entry it preserves the exact
// array element object for graphs that store or return the parameter; only the
// specialized compare/to_s/store kernel is allowed to erase that identity.
func (vm *VM) executeTypedSSAEffectfulIntegerObjectPlan(plan *typedSSAPlan, fn *object.Function, receiver *object.EmeraldValue, argument *object.EmeraldValue, integerClass *object.Class, free ...[]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	raw, exact := typedSSAExactIntegerValueForClass(argument, integerClass)
	if !exact {
		return nil, false
	}
	if plan != nil && plan.effectfulIntegerKernel.kind == typedSSAEffectfulIntegerKernelCompareToSStore {
		return vm.executeTypedSSAEffectfulIntegerKernel(plan.effectfulIntegerKernel, receiver, raw)
	}
	args := [1]*object.EmeraldValue{argument}
	return vm.executeTypedSSAPlan(plan, fn, receiver, args[:], free...)
}

// executeTypedSSAEffectfulIntegerKernelValue runs the pure value half of the
// predecoded setter graph. A proven Array batch can defer the terminal ivar
// write until its last element because this graph contains no user Ruby code
// or exception-producing operation between elements.
func (vm *VM) executeTypedSSAEffectfulIntegerKernelValue(kernel typedSSAEffectfulIntegerKernel, argument int64) (*object.EmeraldValue, bool) {
	if vm == nil || kernel.kind != typedSSAEffectfulIntegerKernelCompareToSStore {
		return nil, false
	}
	condition, ok := typedSSAEffectfulIntegerKernelCondition(kernel, argument)
	if !ok {
		return nil, false
	}
	var value *object.EmeraldValue
	if condition {
		if vm.typedStringValueBatch != nil {
			value = vm.typedStringValueBatch.NewInteger(argument)
		} else {
			value = vm.newTypedSSAStringValue(core.IntegerToSRawBuiltin(argument))
		}
	} else {
		value = vm.newTypedSSAStringValue(kernel.falseString)
	}
	return value, true
}

// executeTypedSSAEffectfulIntegerKernel runs the predecoded setter graph
// without allocating a typedSSAValue register file or rechecking the builtin
// method tables on every iteration. The caller has already guarded the
// Integer comparison/to_s implementations and the global method generation.
func (vm *VM) executeTypedSSAEffectfulIntegerKernel(kernel typedSSAEffectfulIntegerKernel, receiver *object.EmeraldValue, argument int64) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || kernel.kind != typedSSAEffectfulIntegerKernelCompareToSStore {
		return nil, false
	}
	value, executed := vm.executeTypedSSAEffectfulIntegerKernelValue(kernel, argument)
	if !executed {
		return nil, false
	}
	if vm.typedStringValueScratch != nil && vm.typedStringScratchStored &&
		receiver.Type == object.ValueObject && !receiver.Frozen {
		if _, ok := receiver.Data.(*object.Object); ok {
			return value, true
		}
	}
	if receiver.Type == object.ValueObject && !receiver.Frozen {
		if data, ok := receiver.Data.(*object.Object); ok && data != nil {
			// The first ordinary callback has already created this terminal
			// instance variable. Updating the existing map entry directly keeps
			// the normal representation and reflection semantics while avoiding
			// the dynamic receiver switch and repeated "first ivar" bookkeeping.
			if data.InstanceVars != nil {
				if _, exists := data.InstanceVars[kernel.instanceVar]; exists {
					data.InstanceVars[kernel.instanceVar] = value
					if vm.typedStringValueScratch != nil {
						vm.typedStringScratchStored = true
					}
					return value, true
				}
			}
			data.SetInstanceVar(kernel.instanceVar, value)
			if vm.typedStringValueScratch != nil {
				vm.typedStringScratchStored = true
			}
			return value, true
		}
	}
	if result := core.SetDynamicInstanceVar(receiver, kernel.instanceVar, value); result != nil {
		return result, true
	}
	return value, true
}

func (vm *VM) executeTypedSSAPrimitiveIntegerStringKernel(kernel typedSSAPrimitiveIntegerStringKernel, argument int64) (*object.EmeraldValue, bool) {
	if vm == nil || kernel.kind != typedSSAPrimitiveIntegerStringKernelCompareToS {
		return nil, false
	}
	var condition bool
	switch kernel.compare {
	case compiler.OpLessThan:
		condition = argument < kernel.compareValue
	case compiler.OpLessThanOrEqual:
		condition = argument <= kernel.compareValue
	case compiler.OpGreaterThan:
		condition = argument > kernel.compareValue
	case compiler.OpGreaterThanOrEqual:
		condition = argument >= kernel.compareValue
	default:
		return nil, false
	}
	if condition {
		return vm.newTypedSSAStringValue(core.IntegerToSRawBuiltin(argument)), true
	}
	return vm.newTypedSSAStringValue(kernel.falseString), true
}

// executeTypedSSAIntegerStringConcatKernelTrusted is the raw result edge for
// a proven `Integer#to_s` plus one literal String. The caller has already
// guarded Integer#to_s, String#+, the Integer arithmetic guard list and the
// current method generation; keeping those checks out of the per-element
// loop is the point of this kernel.
func (vm *VM) executeTypedSSAIntegerStringConcatKernelTrusted(kernel typedSSAIntegerStringConcatKernel, argument int64) (*object.EmeraldValue, bool) {
	if vm == nil || kernel.kind == typedSSAIntegerStringConcatKernelNone {
		return nil, false
	}
	if vm.typedStringValueBatch != nil {
		if kernel.kind == typedSSAIntegerStringConcatKernelPrefix {
			return vm.typedStringValueBatch.NewPrefixInteger(kernel.literal, argument), true
		}
		if kernel.kind == typedSSAIntegerStringConcatKernelSuffix {
			return vm.typedStringValueBatch.NewIntegerSuffix(argument, kernel.literal), true
		}
		return nil, false
	}
	raw := core.IntegerToSRawBuiltin(argument)
	switch kernel.kind {
	case typedSSAIntegerStringConcatKernelPrefix:
		return vm.newTypedSSAStringValue(kernel.literal + raw), true
	case typedSSAIntegerStringConcatKernelSuffix:
		return vm.newTypedSSAStringValue(raw + kernel.literal), true
	default:
		return nil, false
	}
}

func (vm *VM) newTypedSSAStringValue(value string) *object.EmeraldValue {
	if vm != nil && vm.typedStringValueBatch != nil {
		return vm.typedStringValueBatch.New(value)
	}
	if vm != nil && vm.typedStringValueScratch != nil {
		return vm.typedStringValueScratch.New(value)
	}
	return core.NewStringValue(value)
}

func (vm *VM) executeTypedSSAPlanMode(plan *typedSSAPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, rawFirst *typedSSAValue, free ...[]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || plan == nil || fn == nil || plan.registers > 16 || plan.locals > 64 ||
		rawFirst == nil && len(args) != len(fn.ParamLocalIndices) ||
		rawFirst != nil && (len(fn.ParamLocalIndices) != 1 || len(args) != 0) {
		return nil, false
	}
	var freeValues []*object.EmeraldValue
	if len(free) > 0 {
		freeValues = free[0]
	}
	var registers [16]typedSSAValue
	var locals [64]typedSSAValue
	for index, local := range fn.ParamLocalIndices {
		if local < 0 || local >= len(locals) || rawFirst == nil && index >= len(args) {
			return nil, false
		}
		if rawFirst != nil && index == 0 {
			locals[local] = *rawFirst
		} else {
			locals[local] = typedSSAValueFromObjectWithRef(args[index])
		}
	}
	for pc := 0; pc < len(plan.ops); pc++ {
		instruction := plan.ops[pc]
		switch instruction.kind {
		case typedSSAOpLoadParam:
			if rawFirst == nil && int(instruction.param) >= len(args) || rawFirst != nil && instruction.param != 0 {
				return nil, false
			}
			if rawFirst != nil && instruction.param == 0 {
				registers[instruction.dst] = *rawFirst
			} else {
				registers[instruction.dst] = typedSSAValueFromObjectWithRef(args[instruction.param])
			}
		case typedSSAOpLoadLiteral:
			registers[instruction.dst] = instruction.literal
		case typedSSAOpLoadInstanceVar:
			registers[instruction.dst] = typedSSAValueFromObjectWithRef(core.DynamicInstanceVar(receiver, instruction.name))
		case typedSSAOpLoadSelf:
			registers[instruction.dst] = typedSSAValue{kind: typedSSAReference, ref: receiver}
		case typedSSAOpLoadFree:
			if int(instruction.param) >= len(freeValues) {
				return nil, false
			}
			registers[instruction.dst] = typedSSAValueFromObjectWithRef(derefClosureValue(freeValues[instruction.param]))
		case typedSSAOpLoadLocal:
			if instruction.param >= 64 {
				return nil, false
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
				return nil, false
			}
			locals[instruction.param] = registers[instruction.left]
		case typedSSAOpEqual:
			value, ok := typedSSAImmediateEqual(vm, registers[instruction.left], registers[instruction.right])
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case typedSSAOpCompare:
			value, ok := typedSSACompare(vm, instruction.opcode, registers[instruction.left], registers[instruction.right])
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case typedSSAOpBinary:
			value, ok := typedSSABinary(vm, instruction.opcode, registers[instruction.left], registers[instruction.right])
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case typedSSAOpJump:
			if instruction.target < 0 || instruction.target >= len(plan.ops) {
				return nil, false
			}
			pc = instruction.target - 1
		case typedSSAOpJumpTruthy:
			if typedSSATruthy(registers[instruction.left]) {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return nil, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpJumpNotTruthy:
			if !typedSSATruthy(registers[instruction.left]) {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return nil, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpJumpNotNil:
			if registers[instruction.left].kind != typedSSANil {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return nil, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpIndex:
			left := typedSSAValueToObject(registers[instruction.left])
			right := typedSSAValueToObject(registers[instruction.right])
			value, ok := vm.executeRegisterIRDirectIndex(left, right)
			if !ok {
				return nil, false
			}
			if value == nil {
				value = core.R.NilVal
			}
			registers[instruction.dst] = typedSSAValueFromObjectWithRef(value)
		case typedSSAOpStoreInstanceVar:
			value := typedSSAValueToObject(registers[instruction.left])
			if result := core.SetDynamicInstanceVar(receiver, instruction.name, value); result != nil {
				return result, true
			}
			// A literal/raw result is boxed exactly once at the assignment
			// boundary; OpReturnValue below must return that same object.
			registers[instruction.left] = typedSSAValueFromObjectWithRef(value)
		case typedSSAOpCall:
			if value, executed := vm.executeTypedSSAPrimitiveCall(instruction, registers[:]); executed {
				registers[instruction.dst] = value
				break
			}
			value, executed := vm.executeTypedSSAReferenceCall(instruction, registers[:])
			if !executed {
				return nil, false
			}
			registers[instruction.dst] = typedSSAValueFromObjectWithRef(value)
		case typedSSAOpYield:
			block := vm.currentBlock
			if block == nil || instruction.argc > 4 {
				return nil, false
			}
			var yieldArgs [4]*object.EmeraldValue
			for index := 0; index < int(instruction.argc); index++ {
				yieldArgs[index] = typedSSAValueToObject(registers[instruction.args[index]])
			}
			result := vm.callBlock(block, yieldArgs[:int(instruction.argc)]...)
			if result == nil {
				result = core.R.NilVal
			}
			registers[instruction.dst] = typedSSAValueFromObjectWithRef(result)
			if result.Type == object.ValueException || core.LastBlockResult != nil {
				return result, true
			}
		case typedSSAOpReturn:
			return vm.typedSSAValueToObjectForVM(registers[instruction.left]), true
		default:
			return nil, false
		}
	}
	return nil, false
}

// executeTypedSSAPrimitiveCall handles the small set of native edges whose
// receiver remains a raw typed value. Keeping Integer#to_s raw avoids boxing
// the loop index solely to call a method that immediately formats it; the
// resulting String is boxed only if the surrounding graph observes it.
func (vm *VM) executeTypedSSAPrimitiveCall(instruction typedSSAOp, registers []typedSSAValue) (typedSSAValue, bool) {
	if vm == nil || int(instruction.left) >= len(registers) || instruction.argc != 0 {
		return typedSSAValue{}, false
	}
	receiver := registers[instruction.left]
	if receiver.kind != typedSSAInteger {
		return typedSSAValue{}, false
	}
	if receiver.ref != nil && (receiver.ref.Class != core.R.Classes["Integer"] || core.AttachedSingletonClass(receiver.ref) != nil) {
		return typedSSAValue{}, false
	}
	switch instruction.name {
	case "to_s":
		value, ok := core.IntegerToSRaw(receiver.int)
		if !ok {
			return typedSSAValue{}, false
		}
		return typedSSAValue{kind: typedSSAString, str: value}, true
	default:
		return typedSSAValue{}, false
	}
}

// executeTypedSSAReferenceCall is the nested edge of the typed call graph.
// It deliberately accepts only reference receivers/arguments: boxing a raw
// String/Float/Integer into a new object could change singleton identity or
// frozen-state observations. Primitive callers therefore keep using the
// allocation-free ABI, while object-oriented methods can remove one Ruby
// frame at a time under the same generation guard.
func (vm *VM) executeTypedSSAReferenceCall(instruction typedSSAOp, registers []typedSSAValue) (*object.EmeraldValue, bool) {
	if vm == nil || instruction.argc > 4 || vm.typedSSACallDepth >= 8 ||
		int(instruction.left) >= len(registers) {
		return nil, false
	}
	receiverValue := registers[instruction.left]
	if receiverValue.ref == nil {
		return nil, false
	}
	receiver := receiverValue.ref
	var args [4]*object.EmeraldValue
	for index := 0; index < int(instruction.argc); index++ {
		if int(instruction.args[index]) >= len(registers) {
			return nil, false
		}
		argument := registers[instruction.args[index]]
		if argument.ref == nil {
			return nil, false
		}
		args[index] = argument.ref
	}
	entry, ok := vm.cachedTypedSSAReferenceCallee(receiver, instruction, args[:int(instruction.argc)])
	if !ok {
		return nil, false
	}
	if entry.nativeFn != nil {
		// Native methods have no Ruby frame to unwind.  The exact-arity cache
		// entry above is the proof that this direct call is equivalent to the
		// public fixed-arity dispatch path; retain the generation check across
		// the call in case native code redefines a method or activates tracing.
		generation := entry.generation
		result := callNativeMethod(entry.nativeFn, receiver, args[:int(instruction.argc)])
		if result == nil {
			result = core.R.NilVal
		}
		if generation != object.CurrentMethodGeneration() {
			return nil, false
		}
		return result, true
	}
	callee, plan := entry.fn, entry.plan
	if callee == nil || plan == nil || typedSSAPlanCallCount(plan) > 0 {
		// Keep the first reference edge bounded. A deeper graph needs an
		// unwind-aware call-chain artifact; replay it through the ordinary VM
		// instead of recursively removing frames without a complete proof.
		return nil, false
	}
	generation := entry.generation
	vm.typedSSACallDepth++
	result, executed := vm.executeTypedSSAPlan(plan, callee, receiver, args[:int(instruction.argc)])
	vm.typedSSACallDepth--
	if !executed || result == nil || generation != object.CurrentMethodGeneration() {
		return nil, false
	}
	return result, true
}

func typedSSAPlanCallCount(plan *typedSSAPlan) int {
	if plan == nil {
		return 0
	}
	count := 0
	for _, instruction := range plan.ops {
		if instruction.kind == typedSSAOpCall {
			count++
		}
	}
	return count
}

// typedSSAReferenceCallPlanEligible admits the small object getter/wrapper
// graph used by the integer loop direct caller. It permits one nested public
// call plus primitive control flow and ivar reads, but no writes, yields,
// indexing, free variables, or dynamic block protocol.
func typedSSAReferenceCallPlanEligible(plan *typedSSAPlan) bool {
	return typedSSAReferencePlanEligible(plan, 1) && typedSSAPlanCallCount(plan) == 1
}

func typedSSAReferenceCallInstruction(plan *typedSSAPlan) (typedSSAOp, bool) {
	if plan == nil {
		return typedSSAOp{}, false
	}
	var call typedSSAOp
	found := false
	for _, instruction := range plan.ops {
		if instruction.kind != typedSSAOpCall {
			continue
		}
		if found {
			return typedSSAOp{}, false
		}
		call = instruction
		found = true
	}
	return call, found && call.name != ""
}

func typedSSAReferencePlanEligible(plan *typedSSAPlan, maxCalls int) bool {
	if plan == nil || plan.hasYield || plan.blockReturn || typedSSAPlanCallCount(plan) > maxCalls {
		return false
	}
	for _, instruction := range plan.ops {
		switch instruction.kind {
		case typedSSAOpLoadSelf, typedSSAOpLoadInstanceVar, typedSSAOpLoadParam,
			typedSSAOpLoadLocal, typedSSAOpMove, typedSSAOpSwap, typedSSAOpBang,
			typedSSAOpStoreLocal, typedSSAOpEqual, typedSSAOpCompare, typedSSAOpBinary,
			typedSSAOpJump, typedSSAOpJumpTruthy, typedSSAOpJumpNotTruthy, typedSSAOpJumpNotNil,
			typedSSAOpCall, typedSSAOpReturn:
		default:
			return false
		}
	}
	return true
}

// typedSSAPlanSafeForBlockCalls is the replay barrier for the block tier. A
// typed block may be retried by callBlockWithSelfArgsMode when a receiver or
// method-generation guard misses, so the speculative graph must not perform a
// mutation or an operation with its own unwind protocol before that point.
// Pure Ruby callees are checked again by cachedTypedSSAReferenceCallee; this
// first pass only rejects operations visible in the outer block graph.
func typedSSAPlanSafeForBlockCalls(plan *typedSSAPlan) bool {
	if plan == nil || !plan.blockReturn || plan.hasInstanceStore || plan.hasYield || typedSSAPlanCallCount(plan) == 0 {
		return false
	}
	for _, instruction := range plan.ops {
		switch instruction.kind {
		case typedSSAOpIndex, typedSSAOpStoreInstanceVar, typedSSAOpYield:
			return false
		case typedSSAOpCall:
			if instruction.name == "" || instruction.argc > 4 {
				return false
			}
		}
	}
	return true
}

func (vm *VM) cachedTypedSSAReferenceCallee(receiver *object.EmeraldValue, instruction typedSSAOp, args []*object.EmeraldValue) (typedSSAReferenceCallEntry, bool) {
	if vm == nil || receiver == nil || instruction.name == "" {
		return typedSSAReferenceCallEntry{}, false
	}
	generation := object.CurrentMethodGeneration()
	classCacheable := receiver.Class != nil && core.AttachedSingletonClass(receiver) == nil
	classKey := typedSSAReferenceClassCallKey{class: receiver.Class, name: instruction.name, argc: instruction.argc}
	if classCacheable {
		if entry, found := vm.typedSSAReferenceClassCallCache[classKey]; found && typedSSAReferenceCallEntryUsable(vm, entry, instruction, generation) {
			return entry, true
		}
	}
	key := typedSSAReferenceCallKey{receiver: receiver, name: instruction.name, argc: instruction.argc}
	if entry, found := vm.typedSSAReferenceCallCache[key]; found && typedSSAReferenceCallEntryUsable(vm, entry, instruction, generation) {
		return entry, true
	}
	methodObj, owner, fallback := vm.lookupMethodForSend(receiver, instruction.name, args, false, true)
	if fallback != nil || methodObj == nil || methodObj.Visibility == "undefined" ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") || methodObj.DispatchOwner != nil ||
		methodObj.Ruby2Keywords || methodUsesRefinements(methodObj) {
		return typedSSAReferenceCallEntry{}, false
	}
	if nativeFn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
		if vm.typedSSARequireTrustedNativeReferenceCalls && !registerIRTrustedNativeNoEscapeName(instruction.name) {
			return typedSSAReferenceCallEntry{}, false
		}
		// Variable-arity native methods still need Ruby's argument protocol
		// (keyword conversion, range checks, and often implicit defaults).  The
		// typed reference ABI admits only an explicitly fixed arity, which is
		// enough for methods such as Symbol#to_s and avoids changing errors.
		if methodObj.Arity < 0 || methodObj.Arity != int(instruction.argc) {
			return typedSSAReferenceCallEntry{}, false
		}
		entry := typedSSAReferenceCallEntry{generation: generation, method: methodObj, owner: owner, nativeFn: nativeFn}
		vm.typedSSAReferenceCallCache[key] = entry
		if classCacheable {
			vm.typedSSAReferenceClassCallCache[classKey] = entry
		}
		return entry, true
	}
	callee, ok := methodObj.Fn.(*object.Function)
	if !ok || callee == nil || len(callee.Params) != int(instruction.argc) || len(callee.ParamLocalIndices) != int(instruction.argc) ||
		callee.HasRestParam || callee.HasBlockParam || len(callee.KeywordParams) != 0 || callee.KeywordRestParam != "" ||
		callee.KeywordRestOnly || callee.RejectKeywords || callee.RejectBlock || registerIRFunctionNeedsDefaultEvaluation(callee, int(instruction.argc)) {
		return typedSSAReferenceCallEntry{}, false
	}
	for _, defaultValue := range callee.ParamDefaults {
		if defaultValue != nil {
			return typedSSAReferenceCallEntry{}, false
		}
	}
	plan, ok := vm.cachedTypedSSAPlan(callee)
	if !ok || plan == nil || typedSSAPlanCallCount(plan) > 0 || plan.hasInstanceStore || plan.hasYield {
		return typedSSAReferenceCallEntry{}, false
	}
	entry := typedSSAReferenceCallEntry{generation: generation, method: methodObj, owner: owner, fn: callee, plan: plan}
	vm.typedSSAReferenceCallCache[key] = entry
	if classCacheable {
		vm.typedSSAReferenceClassCallCache[classKey] = entry
	}
	return entry, true
}

func typedSSAReferenceCallEntryUsable(vm *VM, entry typedSSAReferenceCallEntry, instruction typedSSAOp, generation uint64) bool {
	return vm != nil && entry.generation == generation && entry.method != nil && (entry.plan != nil || entry.nativeFn != nil) &&
		(!vm.typedSSARequireTrustedNativeReferenceCalls || entry.nativeFn == nil || registerIRTrustedNativeNoEscapeName(instruction.name))
}

// executeTypedSSAReferencePrimitivePlan is the raw counterpart of
// executeTypedSSAPlan for a reference-oriented method. It keeps self/ivar
// pointers boxed (because their identity is observable), but arithmetic,
// comparisons, branches, and the one nested reference call remain raw. This
// is the missing bridge between object getters and the integer loop kernel.
func (vm *VM) executeTypedSSAReferencePrimitivePlan(plan *typedSSAPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue) (typedSSAValue, bool) {
	return vm.executeTypedSSAReferencePrimitivePlanWithLimit(plan, fn, receiver, args, 1)
}

func (vm *VM) executeTypedSSAReferencePrimitivePlanWithLimit(plan *typedSSAPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, maxCalls int) (typedSSAValue, bool) {
	if vm == nil || receiver == nil || !typedSSAReferencePlanEligible(plan, maxCalls) || fn == nil ||
		len(args) != len(fn.ParamLocalIndices) || len(args) > 16 || plan.registers > 16 || plan.locals > 64 {
		return typedSSAValue{}, false
	}
	var registers [16]typedSSAValue
	var locals [64]typedSSAValue
	for index, local := range fn.ParamLocalIndices {
		if local < 0 || local >= len(locals) || index >= len(args) {
			return typedSSAValue{}, false
		}
		locals[local] = typedSSAValueFromObjectWithRef(args[index])
	}
	for pc := 0; pc < len(plan.ops); pc++ {
		instruction := plan.ops[pc]
		switch instruction.kind {
		case typedSSAOpLoadSelf:
			registers[instruction.dst] = typedSSAValue{kind: typedSSAReference, ref: receiver}
		case typedSSAOpLoadInstanceVar:
			registers[instruction.dst] = typedSSAValueFromObjectWithRef(core.DynamicInstanceVar(receiver, instruction.name))
		case typedSSAOpLoadParam:
			if int(instruction.param) >= len(args) {
				return typedSSAValue{}, false
			}
			registers[instruction.dst] = typedSSAValueFromObjectWithRef(args[instruction.param])
		case typedSSAOpLoadLocal:
			if instruction.param >= 64 {
				return typedSSAValue{}, false
			}
			value := locals[instruction.param]
			if value.kind == typedSSAInvalid {
				value = typedSSAValue{kind: typedSSANil}
			}
			registers[instruction.dst] = value
		case typedSSAOpLoadLiteral:
			registers[instruction.dst] = instruction.literal
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
		case typedSSAOpCall:
			if vm.typedSSACallDepth >= 8 || int(instruction.left) >= len(registers) {
				return typedSSAValue{}, false
			}
			callReceiver := registers[instruction.left]
			if callReceiver.ref == nil {
				return typedSSAValue{}, false
			}
			var callArgs [4]*object.EmeraldValue
			for index := 0; index < int(instruction.argc); index++ {
				argument := registers[instruction.args[index]]
				if argument.ref == nil {
					return typedSSAValue{}, false
				}
				callArgs[index] = argument.ref
			}
			entry, ok := vm.cachedTypedSSAReferenceCallee(callReceiver.ref, instruction, callArgs[:int(instruction.argc)])
			if !ok {
				return typedSSAValue{}, false
			}
			if entry.nativeFn != nil {
				generation := entry.generation
				value := callNativeMethod(entry.nativeFn, callReceiver.ref, callArgs[:int(instruction.argc)])
				if generation != object.CurrentMethodGeneration() {
					return typedSSAValue{}, false
				}
				registers[instruction.dst] = typedSSAValueFromObjectWithRef(value)
				break
			}
			vm.typedSSACallDepth++
			value, executed := vm.executeTypedSSAReferencePrimitivePlan(entry.plan, entry.fn, callReceiver.ref, callArgs[:int(instruction.argc)])
			vm.typedSSACallDepth--
			if !executed {
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
			if value.kind == typedSSAInvalid {
				return typedSSAValue{}, false
			}
			return value, true
		default:
			return typedSSAValue{}, false
		}
	}
	return typedSSAValue{}, false
}

// executeTypedSSAUnboxedArgsPlan is the method-level typed hot-region entry.
// It accepts only primitive integer arguments and a plan with no reference
// operations, then keeps every register/local as an int/bool/nil SSA value.
// The compatibility executor is entered only when a guard or checked
// arithmetic operation misses.  In particular, no EmeraldValue is allocated
// for parameters, constants, arithmetic results, comparisons, or branches;
// the caller boxes exactly one final primitive result after a successful run.
func (vm *VM) executeTypedSSAUnboxedArgsPlan(plan *typedSSAPlan, fn *object.Function, arguments []int64) (typedSSAValue, bool) {
	return vm.executeTypedSSAUnboxedArgsPlanMode(plan, fn, arguments, true)
}

// executeTypedSSAUnboxedArgsPlanTrusted is used inside a region that already
// checks the method-generation token once per iteration.  Skipping the
// per-op fused-builtin map check keeps the hot loop at raw arithmetic cost;
// the caller must stop immediately when the generation changes.
func (vm *VM) executeTypedSSAUnboxedArgsPlanTrusted(plan *typedSSAPlan, fn *object.Function, arguments []int64) (typedSSAValue, bool) {
	return vm.executeTypedSSAUnboxedArgsPlanMode(plan, fn, arguments, false)
}

func (vm *VM) executeTypedSSAUnboxedArgsPlanMode(plan *typedSSAPlan, fn *object.Function, arguments []int64, checkIntegerGeneration bool) (typedSSAValue, bool) {
	if vm == nil || plan == nil || fn == nil || plan.hasReference || plan.hasYield ||
		len(arguments) != len(fn.ParamLocalIndices) || len(arguments) > 16 ||
		plan.registers > 16 || plan.locals > 64 || checkIntegerGeneration && !vm.typedSSAUnboxedPlanGuardsAvailable(plan) {
		return typedSSAValue{}, false
	}
	if kernel := plan.integerKernel; kernel.kind == typedSSAIntegerKernelClamp {
		if len(arguments) <= int(kernel.highArg) {
			return typedSSAValue{}, false
		}
		value := arguments[kernel.valueArg]
		if value < arguments[kernel.lowArg] {
			value = arguments[kernel.lowArg]
		} else if value > arguments[kernel.highArg] {
			value = arguments[kernel.highArg]
		}
		return typedSSAValue{kind: typedSSAInteger, int: value}, true
	}
	// The same compact lowering is valid for the one-argument branch-shaped
	// helper. Keep it in the unboxed ABI as well as the older integer-only
	// entry so callers that already hold the generation guard do not interpret
	// the graph on every iteration.
	if kernel := plan.integerKernel; kernel.kind == typedSSAIntegerKernelCompareBinary {
		if len(arguments) != 1 {
			return typedSSAValue{}, false
		}
		value, ok := executeTypedSSAIntegerKernel(kernel, arguments[0])
		if !ok {
			return typedSSAValue{}, false
		}
		return typedSSAValue{kind: typedSSAInteger, int: value}, true
	}
	var args [16]typedSSAValue
	for index, argument := range arguments {
		args[index] = typedSSAValue{kind: typedSSAInteger, int: argument}
	}
	var registers [16]typedSSAValue
	var locals [64]typedSSAValue
	for index, local := range fn.ParamLocalIndices {
		if local < 0 || local >= len(locals) || index >= len(arguments) {
			return typedSSAValue{}, false
		}
		locals[local] = args[index]
	}
	for pc := 0; pc < len(plan.ops); pc++ {
		instruction := plan.ops[pc]
		switch instruction.kind {
		case typedSSAOpLoadParam:
			if int(instruction.param) >= len(arguments) {
				return typedSSAValue{}, false
			}
			registers[instruction.dst] = args[instruction.param]
		case typedSSAOpLoadLiteral:
			if instruction.literal.kind == typedSSAInvalid || instruction.literal.kind == typedSSAReference {
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
		case typedSSAOpCall:
			// The raw ABI deliberately admits only primitive calls. At present
			// this is Integer#to_s; identity-preserving reference calls remain
			// on the boxed typed executor.
			value, ok := vm.executeTypedSSAPrimitiveCall(instruction, registers[:])
			if !ok {
				return typedSSAValue{}, false
			}
			registers[instruction.dst] = value
		case typedSSAOpReturn:
			value := registers[instruction.left]
			if value.kind == typedSSAInvalid || value.kind == typedSSAReference {
				return typedSSAValue{}, false
			}
			return value, true
		default:
			return typedSSAValue{}, false
		}
	}
	return typedSSAValue{}, false
}

// cachedTypedSSAPlan returns the generation-guarded plan without entering the
// Ruby call protocol.  Loop tiers use this during admission so they can keep
// integer arguments/results unboxed; ordinary calls use the same cache through
// tryExecuteTypedSSAFunction.
func (vm *VM) cachedTypedSSAPlan(fn *object.Function) (*typedSSAPlan, bool) {
	if vm == nil || !typedSSAEnabled || fn == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	entry, found := vm.typedSSAFunctions[fn]
	if !found {
		plan, compiled := compileTypedSSAPlan(fn)
		entry = typedSSAEntry{plan: plan, generation: generation, disabled: !compiled || plan == nil}
		vm.typedSSAFunctions[fn] = entry
	}
	if entry.disabled || entry.plan == nil {
		return nil, false
	}
	if entry.generation != generation {
		entry.generation = generation
		vm.typedSSAFunctions[fn] = entry
	}
	return entry.plan, true
}

// cachedTypedSSAMethodPlans shares the ordinary method-plan lookup with the
// reference fallback. Most functions are not reference candidates, so doing a
// second Function->plan map lookup in the dispatcher would cost more than the
// small frame it is meant to remove. Compile the reference companion only when
// the ordinary plan is known to be unavailable, then retain both decisions in
// the same cache entry.
func (vm *VM) cachedTypedSSAMethodPlans(fn *object.Function) (*typedSSAPlan, *typedSSAPlan, bool) {
	if vm == nil || !typedSSAEnabled || fn == nil {
		return nil, nil, false
	}
	generation := object.CurrentMethodGeneration()
	entry, found := vm.typedSSAFunctions[fn]
	dirty := false
	if !found {
		plan, compiled := compileTypedSSAPlan(fn)
		entry = typedSSAEntry{plan: plan, generation: generation, disabled: !compiled || plan == nil}
		dirty = true
	}
	if entry.generation != generation {
		entry.generation = generation
		dirty = true
	}
	if (entry.disabled || entry.plan == nil) && typedSSAReferenceFunctionEnabled && !entry.referenceChecked {
		plan, compiled := compileTypedSSAReferencePlan(fn)
		entry.referencePlan = plan
		entry.referencePlanFailed = !compiled || plan == nil
		entry.referenceChecked = true
		dirty = true
	}
	if dirty {
		vm.typedSSAFunctions[fn] = entry
	}
	if entry.disabled || entry.plan == nil {
		return nil, entry.referencePlan, false
	}
	return entry.plan, nil, true
}

// cachedTypedSSAReferencePlan is kept separate from the ordinary method tier:
// a zero-argument implicit send is safe only after its caller performs the
// receiver/method-generation/visibility proof.  Compiling such a plan into
// the general method cache would make every ordinary private or dynamic call
// pay a speculative typed-graph miss.
func (vm *VM) cachedTypedSSAReferencePlan(fn *object.Function) (*typedSSAPlan, bool) {
	if vm == nil || !typedSSAEnabled || fn == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	entry, found := vm.typedSSAReferenceFunctions[fn]
	if !found {
		plan, compiled := compileTypedSSAReferencePlan(fn)
		entry = typedSSAEntry{plan: plan, generation: generation, disabled: !compiled || plan == nil}
		vm.typedSSAReferenceFunctions[fn] = entry
	}
	if entry.disabled || entry.plan == nil {
		return nil, false
	}
	if entry.generation != generation {
		entry.generation = generation
		vm.typedSSAReferenceFunctions[fn] = entry
	}
	return entry.plan, true
}

// cachedTypedSSABlockPlan is deliberately separate from the method cache:
// the same Function shape can end in OpBlockReturn, which is valid only while
// executing a closure.  Both entries retain the global method-generation guard
// so redefining Integer operators immediately deoptimizes existing blocks.
func (vm *VM) cachedTypedSSABlockPlan(fn *object.Function) (*typedSSAPlan, bool) {
	if vm == nil || !typedSSAEnabled || fn == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	entry, found := vm.typedSSABlockFunctions[fn]
	if !found {
		plan, compiled := compileTypedSSAPlanWithBlockReturn(fn, true)
		entry = typedSSAEntry{plan: plan, generation: generation, disabled: !compiled || plan == nil}
		vm.typedSSABlockFunctions[fn] = entry
	}
	if entry.disabled || entry.plan == nil {
		return nil, false
	}
	if entry.generation != generation {
		entry.generation = generation
		vm.typedSSABlockFunctions[fn] = entry
	}
	return entry.plan, true
}

// executeTypedSSAIntegerPlan is the unboxed one-argument loop entry.  It is
// intentionally narrower than executeTypedSSAPlan: reference values would
// require object identity/dispatch guards, so callers admit only plans whose
// graph has no self/ivar/reference values and whose result is an Integer.
func (vm *VM) executeTypedSSAIntegerPlan(plan *typedSSAPlan, fn *object.Function, argument int64) (int64, bool) {
	if vm == nil || plan == nil || fn == nil || plan.hasReference || len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 ||
		plan.registers > 16 || plan.locals > 64 || !vm.typedSSAIntegerOpsAvailable(plan) {
		return 0, false
	}
	if kernel := plan.integerKernel; kernel.kind == typedSSAIntegerKernelCompareBinary {
		return executeTypedSSAIntegerKernel(kernel, argument)
	}
	var registers [16]typedSSAValue
	var locals [64]typedSSAValue
	local := fn.ParamLocalIndices[0]
	if local < 0 || local >= len(locals) {
		return 0, false
	}
	argumentValue := typedSSAValue{kind: typedSSAInteger, int: argument}
	locals[local] = argumentValue
	for pc := 0; pc < len(plan.ops); pc++ {
		instruction := plan.ops[pc]
		switch instruction.kind {
		case typedSSAOpLoadParam:
			if instruction.param != 0 {
				return 0, false
			}
			registers[instruction.dst] = argumentValue
		case typedSSAOpLoadLiteral:
			if instruction.literal.kind == typedSSAReference || instruction.literal.kind == typedSSAInvalid {
				return 0, false
			}
			registers[instruction.dst] = instruction.literal
		case typedSSAOpLoadLocal:
			if instruction.param >= 64 {
				return 0, false
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
				return 0, false
			}
			locals[instruction.param] = registers[instruction.left]
		case typedSSAOpEqual:
			value, ok := typedSSAImmediateEqual(vm, registers[instruction.left], registers[instruction.right])
			if !ok {
				return 0, false
			}
			registers[instruction.dst] = value
		case typedSSAOpCompare:
			value, ok := typedSSACompare(vm, instruction.opcode, registers[instruction.left], registers[instruction.right])
			if !ok {
				return 0, false
			}
			registers[instruction.dst] = value
		case typedSSAOpBinary:
			value, ok := typedSSABinary(vm, instruction.opcode, registers[instruction.left], registers[instruction.right])
			if !ok {
				return 0, false
			}
			registers[instruction.dst] = value
		case typedSSAOpJump:
			if instruction.target < 0 || instruction.target >= len(plan.ops) {
				return 0, false
			}
			pc = instruction.target - 1
		case typedSSAOpJumpTruthy:
			if typedSSATruthy(registers[instruction.left]) {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return 0, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpJumpNotTruthy:
			if !typedSSATruthy(registers[instruction.left]) {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return 0, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpJumpNotNil:
			if registers[instruction.left].kind != typedSSANil {
				if instruction.target < 0 || instruction.target >= len(plan.ops) {
					return 0, false
				}
				pc = instruction.target - 1
			}
		case typedSSAOpReturn:
			result := registers[instruction.left]
			if result.kind != typedSSAInteger {
				return 0, false
			}
			return result.int, true
		default:
			return 0, false
		}
	}
	return 0, false
}

func executeTypedSSAIntegerKernel(kernel typedSSAIntegerKernel, argument int64) (int64, bool) {
	var condition bool
	switch kernel.compare {
	case compiler.OpLessThan:
		condition = argument < kernel.compareValue
	case compiler.OpLessThanOrEqual:
		condition = argument <= kernel.compareValue
	case compiler.OpGreaterThan:
		condition = argument > kernel.compareValue
	case compiler.OpGreaterThanOrEqual:
		condition = argument >= kernel.compareValue
	default:
		return 0, false
	}
	opcode, value := kernel.falsyOp, kernel.falsyValue
	if condition {
		opcode, value = kernel.truthyOp, kernel.truthyValue
	}
	switch opcode {
	case compiler.OpAdd:
		return checkedIntegerAdd(argument, value)
	case compiler.OpSub:
		return checkedIntegerSub(argument, value)
	case compiler.OpMul:
		return checkedIntegerMul(argument, value)
	case compiler.OpMod:
		if value == 0 {
			return 0, false
		}
		result := argument % value
		if result != 0 && (result < 0) != (value < 0) {
			result += value
		}
		return result, true
	case compiler.OpBitAnd:
		return argument & value, true
	default:
		return 0, false
	}
}

func (vm *VM) tryExecuteTypedSSAFunction(methodObj *object.Method, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, parentMethod ...string) (*object.EmeraldValue, bool) {
	parent := ""
	if len(parentMethod) > 0 {
		parent = parentMethod[0]
	}
	if vm == nil || !typedSSAEnabled || methodObj == nil || fn == nil || receiver == nil ||
		methodObj.DispatchOwner != nil || methodObj.Ruby2Keywords ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public" &&
			(methodObj.Visibility != "private" || !vm.typedSSAPrivateAccessAllowed(receiver, parent))) ||
		methodObj.OriginalName == "method_missing" || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 ||
		vm.pendingReturnTargetID != 0 || vm.pendingBreakTargetID != 0 || methodUsesRefinements(methodObj) ||
		len(args) != len(fn.Params) || len(fn.ParamLocalIndices) != len(fn.Params) || len(args) > 16 {
		return nil, false
	}
	plan, referencePlan, ok := vm.cachedTypedSSAMethodPlans(fn)
	if !ok {
		if result, executed := vm.tryExecuteTypedSSAReferenceFunction(methodObj, fn, receiver, args, referencePlan); executed {
			return result, true
		}
		return nil, false
	}
	if plan.effectfulIntegerKernel.kind == typedSSAEffectfulIntegerKernelInstanceBinary && len(args) == 1 {
		if argument, exact := typedSSAExactIntegerValueForClass(args[0], core.R.Classes["Integer"]); exact {
			if result, executed := vm.executeTypedSSAEffectfulIntegerPlan(plan, fn, receiver, argument); executed {
				return result, true
			}
		}
	}
	// Prefer the unboxed hot-region executor for primitive arguments.  The
	// ordinary typed SSA path below is still required for reference values and
	// yield/index operations, but a pure integer/bool/nil graph can run without
	// constructing an EmeraldValue for every intermediate result.
	if !plan.hasReference {
		var argumentStorage [16]int64
		arguments := argumentStorage[:len(args)]
		primitive := true
		for index, argument := range args {
			if !typedIntegerArgument(argument) {
				primitive = false
				break
			}
			arguments[index] = argument.Data.(int64)
		}
		if primitive {
			if value, executed := vm.executeTypedSSAUnboxedArgsPlan(plan, fn, arguments); executed {
				return typedSSAValueToObject(value), true
			}
		}
	}
	return vm.executeTypedSSAPlan(plan, fn, receiver, args)
}

const typedSSAReferenceFunctionMaxCalls = 4

// tryExecuteTypedSSAReferenceFunction is the ordinary-method entry for the
// reference ABI. The original typed SSA method cache intentionally rejects
// implicit sends because they need a caller-aware lookup. This companion cache
// admits only a small, read-only call graph: the outer method has no writes,
// yield, indexing, or block protocol; native edges must be in the no-escape
// whitelist; Ruby edges are limited by cachedTypedSSAReferenceCallee to a
// pure callee with no nested call, store, or yield. A miss therefore happens
// before any user-visible mutation and the normal Frame path can replay it.
func (vm *VM) tryExecuteTypedSSAReferenceFunction(methodObj *object.Method, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, plan *typedSSAPlan) (*object.EmeraldValue, bool) {
	if vm == nil || !typedSSAReferenceFunctionEnabled || methodObj == nil || fn == nil || receiver == nil ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 || len(vm.rescueStack) != 0 ||
		vm.pendingReturnTargetID != 0 || vm.pendingBreakTargetID != 0 {
		return nil, false
	}
	if plan == nil || typedSSAPlanCallCount(plan) == 0 || plan.hasInstanceStore || plan.hasYield ||
		plan.blockReturn || !typedSSAReferencePlanEligible(plan, typedSSAReferenceFunctionMaxCalls) {
		return nil, false
	}
	previous := vm.typedSSARequireTrustedNativeReferenceCalls
	vm.typedSSARequireTrustedNativeReferenceCalls = true
	value, executed := vm.executeTypedSSAReferencePrimitivePlanWithLimit(
		plan, fn, receiver, args, typedSSAReferenceFunctionMaxCalls,
	)
	vm.typedSSARequireTrustedNativeReferenceCalls = previous
	if !executed {
		return nil, false
	}
	if typedSSAReferenceFunctionDebug {
		fmt.Fprintf(os.Stderr, "RGO_TYPED_SSA_REFERENCE %s:%d %s calls=%d\n", fn.SourcePath, fn.DefinitionLine, fn.Name, typedSSAPlanCallCount(plan))
	}
	return vm.typedSSAValueToObjectForVM(value), true
}

// tryExecuteTypedSSABlock is the closure counterpart of the method tier.
// Blocks have a distinct OpBlockReturn unwind protocol, so they use a
// separate cache and are admitted only for ordinary local returns. The
// compiler accepts scalar data flow (including self/ivar reads) and a small,
// side-effect-free call graph; other sends, stores, yields, allocation and
// control-flow escapes retain the existing block executor.
func (vm *VM) tryExecuteTypedSSABlock(fn *object.Function, closure *object.Closure, receiver *object.EmeraldValue, args []*object.EmeraldValue, isLambda bool) (*object.EmeraldValue, bool) {
	if vm == nil || !typedSSAEnabled || fn == nil || closure == nil || receiver == nil || isLambda ||
		closure.AutoSplat || closure.ReturnOwnerID > 0 || closure.BreakOwnerID > 0 ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		core.ObjectSpaceAllocationTracing() || len(vm.catchStack) != 0 || len(vm.activeRescues) != 0 ||
		len(vm.rescueStack) != 0 ||
		vm.pendingReturnTargetID != 0 || vm.pendingBreakTargetID != 0 ||
		fn.ForLoopCollectAsPair || closureUsesRefinements(closure) ||
		len(args) != len(fn.Params) || len(fn.ParamLocalIndices) != len(fn.Params) || len(args) > 16 {
		return nil, false
	}
	if !simpleBlockParameterPatterns(fn) || !vm.simpleBlockCallShape(fn) {
		return nil, false
	}
	plan, ok := vm.cachedTypedSSABlockPlan(fn)
	if !ok || plan == nil || !plan.blockReturn {
		return nil, false
	}
	if typedSSAPlanCallCount(plan) > 0 {
		// The call graph is admitted only after the outer replay barrier above.
		// Reference dispatch additionally rejects non-whitelisted native methods
		// and effectful/nested Ruby callees in cachedTypedSSAReferenceCallee.
		if !typedSSABlockCallsEnabled || !typedSSAPlanSafeForBlockCalls(plan) {
			return nil, false
		}
		previous := vm.typedSSARequireTrustedNativeReferenceCalls
		vm.typedSSARequireTrustedNativeReferenceCalls = true
		result, executed := vm.executeTypedSSAPlan(plan, fn, receiver, args, closure.Free)
		vm.typedSSARequireTrustedNativeReferenceCalls = previous
		return result, executed
	}
	if plan.hasYield && vm.currentBlock == nil {
		return nil, false
	}
	return vm.executeTypedSSAPlan(plan, fn, receiver, args, closure.Free)
}

// typedSSAPrivateAccessAllowed mirrors invokeMethod's already-established
// private visibility rule.  Typed execution is speculative, so a private
// method is admitted only after the same receiver/parent checks that the
// normal dispatcher applies; public_send therefore cannot accidentally turn a
// private call into a successful compiled call.
func (vm *VM) typedSSAPrivateAccessAllowed(receiver *object.EmeraldValue, parentMethod string) bool {
	if vm == nil || receiver == nil || parentMethod == "public_send" {
		return false
	}
	if vm.visibilityBypass || receiver == core.R.Main {
		return true
	}
	if vm.fp < 0 || vm.fp >= len(vm.frames) || vm.frames[vm.fp] == nil {
		return false
	}
	frame := vm.frames[vm.fp]
	if frame.Bp < 0 || frame.Bp >= len(vm.stack) {
		return false
	}
	return receiver == vm.stack[frame.Bp]
}
