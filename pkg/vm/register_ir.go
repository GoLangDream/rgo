package vm

import (
	"math"
	"os"
	"sort"
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// registerIRFunctionNeedsDefaultEvaluation distinguishes the compiler's
// conservative EvaluateParamDefaults marker from an invocation that actually
// omits a positional default. The compiler currently sets that marker on every
// Ruby function, including zero-argument methods and calls supplying all
// positional arguments. Treating the marker as a blanket rejection kept the
// framed IR cache cold for nearly every gem method.
func registerIRFunctionNeedsDefaultEvaluation(fn *object.Function, suppliedPositional int) bool {
	if fn == nil || !fn.EvaluateParamDefaults || suppliedPositional >= len(fn.Params) {
		return false
	}
	if suppliedPositional < 0 {
		suppliedPositional = 0
	}
	for index := suppliedPositional; index < len(fn.ParamDefaults); index++ {
		if fn.ParamDefaults[index] != nil {
			return true
		}
	}
	return false
}

// registerIRUnsupportedOpcodeSummary is intentionally used only by the
// opt-in profile report.  It keeps the normal compilation path unchanged while
// making a rejected hot method actionable instead of guessing from its Ruby
// name.  The summary reports unique bytecode opcodes; a method can still be
// rejected for a semantic guard even when all of its opcodes are listed.
func registerIRUnsupportedOpcodeSummary(fn *object.Function) string {
	if fn == nil {
		return "nil"
	}
	seen := make(map[compiler.Opcode]bool)
	for position := 0; position < len(fn.Instructions); {
		op := compiler.Opcode(fn.Instructions[position])
		seen[op] = true
		definition, ok := compiler.Lookup(byte(op))
		if !ok {
			position++
			continue
		}
		width := 1
		for _, operandWidth := range definition.OperandWidths {
			width += operandWidth
		}
		if position+width > len(fn.Instructions) {
			break
		}
		position += width
	}
	names := make([]string, 0, len(seen))
	for op := range seen {
		if definition, ok := compiler.Lookup(byte(op)); ok {
			names = append(names, definition.Name)
		} else {
			names = append(names, "unknown")
		}
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func registerIROpcodeSequence(fn *object.Function) string {
	if fn == nil {
		return "nil"
	}
	names := make([]string, 0, len(fn.Instructions))
	for position := 0; position < len(fn.Instructions); {
		op := compiler.Opcode(fn.Instructions[position])
		if definition, ok := compiler.Lookup(byte(op)); ok {
			names = append(names, definition.Name)
			width := 1
			for _, operandWidth := range definition.OperandWidths {
				width += operandWidth
			}
			if position+width > len(fn.Instructions) {
				break
			}
			position += width
			continue
		}
		names = append(names, "unknown")
		position++
	}
	return strings.Join(names, ">")
}

var registerIREnabled = os.Getenv("RGO_DISABLE_REGISTER_IR") == ""
var registerIRSendCacheEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_SEND_CACHE") == ""

// The bytecode-level cache performs a receiver/context probe on every eligible
// send.  Repeated A/B runs on the current dispatcher show that its call-site
// generation guard now pays for itself on both long dynamic Gem workloads and
// ordinary Ruby call loops, so keep it on by default.  The explicit disable
// switch is useful for compatibility/performance bisects; the historical
// RGO_ENABLE_REGISTER_IR_BYTECODE_SEND_CACHE=1 remains harmless for scripts
// that used the old opt-in spelling.
var registerIRBytecodeSendCacheEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BYTECODE_SEND_CACHE") == ""

// After a bytecode send has entered the fixed-arity frame path successfully,
// its generation-scoped cache entry has already proven the static method
// shape.  The trusted entry skips those immutable shape probes on subsequent
// sends; dynamic VM guards remain in the callee.  It is kept opt-in because
// compatibility-mode Gem workloads can still benefit from the existing native
// and ordinary fixed-bytecode ladder more than this narrower entry.  The
// explicit enable/disable switches remain useful for bisects.
var registerIRTrustedBytecodeEntryEnabled = os.Getenv("RGO_ENABLE_REGISTER_IR_TRUSTED_BYTECODE_ENTRY") != "" &&
	os.Getenv("RGO_DISABLE_REGISTER_IR_TRUSTED_BYTECODE_ENTRY") == ""

var registerIRBytecodeBlockSendCacheEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BYTECODE_BLOCK_SEND_CACHE") == ""
var registerIRTrustedFramedNativeRegionEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_TRUSTED_FRAMED_NATIVE_REGION") == ""

// Batch callbacks such as Integer#times already reuse their Ruby Frame and
// have a generation-scoped Register IR send cache. The narrow batch-send
// entry below reuses either a cached native ABI or an exact fixed-arity Ruby
// bytecode entry across iterations; a miss stays on executeRegisterIRSend for
// the current instruction. Keep an explicit switch for low-risk bisects.
var registerIRBatchSendEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BATCH_SEND") == ""

// A direct no-frame probe is more expensive than the established framed leaf
// path when a method is called only a handful of times.  Require a modest
// warmup before probing so dynamic Gem helpers do not pay a failed speculative
// check on their short hot paths; genuinely hot methods still enter the tier
// early in steady state.
const registerIRDirectNoFrameWarmupCalls = 32

var registerIRKeywordSendCacheEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_KEYWORD_SEND_CACHE") == ""
var registerIRPolymorphicSendCacheEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_POLYMORPHIC_CACHE") == ""
var registerIRCollectionSendCacheEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_COLLECTION_SEND_CACHE") == ""

// A call site that misses the monomorphic/two-entry cache this many times in
// one method-generation epoch is predictably polymorphic.  Stop probing it;
// the ordinary dispatcher already preserves the complete Ruby semantics and
// method-generation invalidation re-enables the cache after redefinition.
const registerIRBytecodeSendCacheFailureLimit uint8 = 4

var registerIRArithmeticEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_ARITHMETIC") == ""
var registerIRComparisonEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_COMPARISON") == ""
var registerIRBlockEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BLOCKS") == ""
var registerIRBlockSendCacheEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BLOCK_SEND_CACHE") == ""
var registerIRInlineEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_INLINE") == ""

// Framed-send inline is conservative: the cache is populated only for a
// public exact-arity Ruby method whose Register IR plan has already passed the
// normal safety proof.  Keep a disable switch for compatibility/debugging,
// but make the proven path the default so Gem-heavy code does not repeatedly
// re-enter invokeMethod for the same callee.
var registerIRFramedSendInlineEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_FRAMED_SEND_INLINE") == ""

// Branch-bearing framed plans are admitted by default only for the Prawn
// source graph, where the cached bytecode loop has a measurable send/frame
// cost. Other Ruby programs keep the conservative bytecode choice; the
// environment switch enables the broader A/B comparison explicitly.
var registerIRBranchFrameInlineEnabled = os.Getenv("RGO_ENABLE_REGISTER_IR_BRANCH_FRAME_INLINE") != ""

func registerIRBranchFrameInlineAllowed(fn *object.Function) bool {
	if registerIRBranchFrameInlineEnabled {
		return true
	}
	if fn == nil {
		return false
	}
	return strings.Contains(fn.SourcePath, "/prawn-") || strings.Contains(fn.SourcePath, "/pdf-core-")
}

// A broad branch+send framed tier is slower on Prawn because large boxed
// methods pay Register IR branch bookkeeping without removing their dynamic
// work. Keep only a small diagnostic subset available for measurement; the
// default remains the conservative straight-line admission above.

// No-frame call-chain/direct execution is guarded by cached receiver/method
// proofs. Keep the established tiers enabled for hot, monomorphic helpers;
// their explicit disable switches remain useful for compatibility and A/B
// measurements on workloads where the guard cost dominates.
var registerIRCallChainEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_CALL_CHAIN_INLINE") == ""
var registerIRNoFrameEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_NOFRAME") == ""
var registerIRDirectNoFrameEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_DIRECT_NOFRAME") == ""

// Aggressive IR is the second execution mode's graph-inlining tier.  It is
// intentionally opt-in until the compatibility suite has covered methods
// that rely on rescue/ensure, yield, closures, or non-local control flow.  A
// plan admitted here runs its supported operations without allocating a Ruby
// Frame; an unproven send is dispatched once through the normal method cache
// rather than replaying a partially executed body.
// Compiled mode is allowed to use the frame-free aggressive ABI.  Ordinary
// `run` keeps the compatibility executor unless the explicit diagnostic flag
// is present; this preserves complete exception/backtrace behavior by
// default while making `rgo compiled` select the optimized execution tier.
var registerIRAggressiveEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_AGGRESSIVE") == "" &&
	(os.Getenv("RGO_ENABLE_REGISTER_IR_AGGRESSIVE") != "" || os.Getenv("RGO_EXEC_MODE") == "compiled")
var registerIRCaseDispatchBranchEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_CASE_BRANCH") == ""
var registerIRMemoizedReaderEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_MEMO_FAST") == ""
var registerIRIntegerStringFastEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_INTEGER_STRING_FAST") == ""

var registerIRConstantsEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_CONSTANTS") == ""
var registerIRScopedConstantsEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_SCOPED_CONSTANTS") == ""
var registerIRIndexAssignEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_INDEX_ASSIGN") == ""
var registerIRStringEncodingEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_STRING_ENCODING") == ""
var registerIRDefinedInstanceVarEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_DEFINED_INSTANCE_VAR") == ""
var registerIRSliceEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_SLICE") == ""
var registerIRFrozenStringLiteralsEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_FROZEN_STRING_LITERALS") == ""

// File-level frozen-string templates still widen the set of framed IR
// methods.  Keep that broader tier opt-in until its extra block/body coverage
// is independently proven neutral on all compatibility workloads.
var registerIRFrozenSourceStringLiteralsEnabled = os.Getenv("RGO_ENABLE_REGISTER_IR_FROZEN_SOURCE_STRINGS") != ""

// A string that is provably reachable only on a path ending in OpRaise can
// retain the normal constantValue allocation/freeze semantics without
// enabling the broad mutable/frozen string tier. This covers common argument
// validation helpers while keeping returned strings and mutation-heavy code
// on the compatibility interpreter path.
var registerIRColdRaiseStringLiteralsEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_COLD_RAISE_STRINGS") == ""

// String literals still require a broader allocation/encoding and setter
// deoptimisation audit in framed IR. Keep the implementation available for
// experiments, but do not let it change the default Gem compatibility tier.
var registerIRStringLiteralsEnabled = os.Getenv("RGO_ENABLE_REGISTER_IR_STRING_LITERALS") != ""

// Block bodies already execute with a real Ruby Frame in the framed tier, so
// mutable literals can retain constantValue's per-call allocation semantics
// without widening the no-frame/method tiers.  This closes a common IR gap in
// generated Gem callbacks while keeping an explicit kill switch for audits.
var registerIRBlockStringLiteralsEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BLOCK_STRING_LITERALS") == ""
var registerIRBitShiftEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BIT_SHIFT") == ""

// Closure-producing bytecode needs to preserve Ruby's binding, block and
// non-local control-flow metadata.  Keep this tier opt-in until the full
// closure/block regression set and real Gem workloads prove it is neutral.
var registerIRClosuresEnabled = os.Getenv("RGO_ENABLE_REGISTER_IR_CLOSURES") != ""

// Passing a closure through an IR send is a separate, stricter experiment:
// block unwinding and Proc conversion still have edge cases outside the
// straight-line block tier.  It therefore needs an explicit opt-in in
// addition to closure creation.
var registerIRClosureSendsEnabled = os.Getenv("RGO_ENABLE_REGISTER_IR_CLOSURE_SENDS") != ""

var registerIRLogicalAssignmentEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_LOGICAL_ASSIGNMENT") == ""
var registerIRRaiseEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_RAISE") == ""

// Framed block IR keeps the normal Ruby Frame/binding/unwind protocol while
// replacing the block's bytecode dispatch loop with the register executor.
// Although the frame remains present, Gem workloads exercise subtle block
// binding and unwind behavior. The block-return guard and full Gem gate now
// cover the default path; keep the explicit disable switch for diagnostics.
var registerIRFramedBlockEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_FRAMED_BLOCKS") == ""

// Fixed-argument framed blocks still create the normal Ruby Frame, but can
// use a direct positional slot binder after the call-site has completed all
// block arity/keyword checks.  Keep a kill switch because binding is part of
// Ruby's observable Binding semantics.
// Batch block IR only admits a pure, one-argument, no-frame plan. It does not
// replace the normal framed block protocol and can therefore be audited as a
// separate compatibility tier. Keep it enabled by default, with an explicit
// kill switch for bisecting Gem regressions.
var registerIRBatchBlockEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BATCH_BLOCKS") == ""
var registerIRTrustedArrayIndexEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_TRUSTED_ARRAY_INDEX") == ""
var registerIRTrustedArrayArithmeticEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_TRUSTED_ARRAY_ARITHMETIC") == ""
var registerIRBranchNoFrameBlockEnabled = os.Getenv("RGO_DISABLE_REGISTER_IR_BRANCH_NOFRAME_BLOCK") == ""

// Fixed positional block calls do not need the keyword/rest protocol probes;
// retain a kill switch so Gem compatibility and performance can be bisected
// independently of the broader block IR tiers.
var registerIRSimpleBlockProtocolEnabled = os.Getenv("RGO_DISABLE_SIMPLE_BLOCK_PROTOCOL") == ""

// Fixed positional Ruby methods can use the same proof to bypass the
// keyword/rest normalization helpers when the call site has no keyword
// syntax. The complete binder remains the fallback for every other shape.
var registerIRSimpleMethodProtocolEnabled = os.Getenv("RGO_DISABLE_SIMPLE_METHOD_PROTOCOL") == ""

// Optional-parameter defaults still execute with the normal Ruby Frame and
// binder-visible argument slots, but framed IR can cost more than the compact
// bytecode path on default-heavy calls. Keep this tier opt-in until a hotness
// threshold can select it per function; the flag is useful for targeted Gem
// workloads and compatibility/performance bisects.
var registerIROptionalDefaultsEnabled = os.Getenv("RGO_ENABLE_REGISTER_IR_OPTIONAL_DEFAULTS") != ""

type registerIROp uint8

const (
	registerIRLoadParam registerIROp = iota
	registerIRLoadLocal
	registerIRLoadLiteral
	registerIRLoadConstantValue
	registerIRLoadFrozenString
	registerIRLoadConstant
	registerIRLoadScopedConstant
	registerIRLoadCapture
	registerIRClosure
	registerIRLoadInstanceVar
	registerIRDefinedInstanceVar
	registerIRStoreInstanceVar
	registerIRSetStringEncoding
	registerIRArray
	registerIRSplatToArray
	registerIRHash
	registerIRHashMerge
	registerIRMarkKeywordHash
	registerIRRange
	registerIRBlockGiven
	registerIRLoadSelf
	registerIRLoadFree
	registerIRStoreFree
	registerIRMove
	registerIRSwap
	registerIRBang
	registerIRNotEqual
	registerIRNeg
	registerIRStoreLocal
	registerIREqual
	registerIRDynamicEqual
	registerIRBinary
	registerIRCompare
	registerIRDynamicCompare
	registerIRSend
	registerIRYield
	registerIRLogicalSendAssignment
	registerIRIndex
	registerIRSlice
	registerIRIndexAssign
	registerIRMultiAssignPrepare
	registerIRMultiAssignExtract
	registerIRMultiAssignCheckToAry
	registerIRJump
	registerIRJumpTruthy
	registerIRJumpNotTruthy
	registerIRJumpNotNil
	registerIRJumpLocalPresent
	registerIRRaise
	registerIRReturn
)

type registerIRCaptureKind uint8

const (
	registerIRCaptureLocal registerIRCaptureKind = iota
	registerIRCaptureOuter
	registerIRCaptureFree
	registerIRCaptureOuterFree
)

type registerIRInstruction struct {
	op           registerIROp
	dst          uint8
	left         uint8
	right        uint8
	param        uint8
	argc         uint8
	target       int
	value        *object.EmeraldValue
	name         string
	setter       string
	args         [4]uint8
	splatIndex   uint8
	block        uint8
	blockPresent bool
	// implicit marks Ruby's bare identifier send (the bytecode call-kind 3).
	// It is normally equivalent to a self send, but a missing method must be
	// converted to Ruby's "undefined local variable or method" NameError.
	implicit       bool
	byteIP         int
	opcode         compiler.Opcode
	explicitReturn bool
	cache          *registerIRSendCache
	fn             *object.Function
	captureKind    registerIRCaptureKind
}

// registerIRTrustedDirectEntry caches the method-level proof used by a
// trusted block region. The send cache already guards receiver identity/class
// and method generation; keeping the derived no-frame plan proof here avoids
// rescanning the callee's Register IR and constant environment on every
// callback iteration.
type registerIRTrustedDirectEntry struct {
	generation         uint64
	constantGeneration uint64
	vm                 *VM
	method             *object.Method
	leaf               *leafMethodPlan
	fn                 *object.Function
	plan               *registerIRPlan
	free               []*object.EmeraldValue
	allowConstants     bool
	allowCaseBranch    bool
	safe               bool
}

type registerIRSendCache struct {
	generation uint64
	// bytecodeProbeGeneration/Failures/Disabled belong only to the bytecode
	// call-site cache.  A polymorphic site can spend more time probing the
	// two-entry cache than it would spend in the ordinary dispatcher.  Keep a
	// small generation-scoped miss budget so such a site deopts to the normal
	// send path instead of paying that probe on every invocation.  Method
	// redefinition changes generation and re-arms the probe automatically.
	bytecodeProbeGeneration  uint64
	bytecodeProbeFailures    uint8
	bytecodeProbeDisabled    bool
	receiver                 *object.EmeraldValue
	class                    *object.Class
	method                   *object.Method
	owner                    *object.Class
	inlineLeaf               *leafMethodPlan
	inlineFn                 *object.Function
	inlinePlan               *registerIRPlan
	nativeFn                 func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	framedNativeFn           func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	bytecodeFixedArity       bool
	directIndex              bool
	secondReceiver           *object.EmeraldValue
	secondClass              *object.Class
	secondMethod             *object.Method
	secondOwner              *object.Class
	secondLeaf               *leafMethodPlan
	secondFn                 *object.Function
	secondInlinePlan         *registerIRPlan
	secondNativeFn           func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	secondFramedNativeFn     func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	secondBytecodeFixedArity bool
	secondDirectIndex        bool
	// aggressivePlan/aggressiveNativeFn are the already-admitted call-graph
	// edge used by executeRegisterIRAggressiveSend.  The ordinary method cache
	// above remembers only the Method object, which still forced the aggressive
	// tier to rediscover the Ruby plan (or native ABI) on every iteration.
	// Keep a separate entry because the framed and no-frame tiers have
	// different safety predicates.  The prepared bit is needed to distinguish
	// a proven unsupported method (nil plan/native) from a cold entry.
	aggressivePlan           *registerIRPlan
	aggressiveNativeFn       func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	aggressivePrepared       bool
	secondAggressivePlan     *registerIRPlan
	secondAggressiveNativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	secondAggressivePrepared bool
	trustedDirect            *registerIRTrustedDirectEntry
	secondTrustedDirect      *registerIRTrustedDirectEntry
}

type registerIRPlan struct {
	instructions        []registerIRInstruction
	registers           uint8
	sendCount           uint8
	hasBranches         bool
	hasSends            bool
	hasConstantLoads    bool
	hasFrozenStrings    bool
	integerOnly         bool
	integerLinear       bool
	integerLinearKind   uint8
	integerLinearOpA    compiler.Opcode
	integerLinearOpB    compiler.Opcode
	integerLinearConstA int64
	integerLinearConstB int64
	// Set when a string literal was admitted solely because every path after
	// it raises. It gates the narrow pre-binder entry in executor.go; ordinary
	// IR plans keep the full Ruby argument-normalization path.
	coldRaiseStringLiterals bool
	// These two predicates depend only on the decoded instruction shape. Keep
	// their result beside the plan so hot invokeMethod calls do not rescan every
	// instruction before entering the speculative tiers.
	directNoFrameChecked      bool
	directNoFrameSafe         bool
	directNoFrameBlockChecked bool
	directNoFrameBlockSafe    bool
	// The broad direct predicate has three independent admission switches
	// (block return, constants, case branches).  Hot block callers use the
	// combinations below repeatedly; cache the result as a small option bitset
	// instead of rescanning every Register IR instruction at each callback.
	directNoFrameOptionsChecked     uint8
	directNoFrameOptionsSafe        uint8
	typedHotArrayCallSafeChecked    bool
	typedHotArrayCallSafe           bool
	typedHotArrayCallSafeGeneration uint64
	noFrameInlineChecked            bool
	noFrameInlineSafe               bool
	mayDeoptChecked                 bool
	mayDeopt                        bool
	framedBlockChecked              bool
	framedBlockSafe                 bool
	caseDispatchFramedChecked       bool
	caseDispatchFramedSafe          bool
	framedBlockReturnChecked        bool
	framedBlockReturnSafe           bool
	framelessBlockChecked           bool
	framelessBlockSafe              bool
	arrayLiteralIndexFoldsChecked   bool
	arrayLiteralIndexFolds          []registerIRArrayLiteralIndexFold
	branchNoFrameBlockChecked       bool
	branchNoFrameBlockSafe          bool
	aggressiveMethodChecked         bool
	aggressiveMethodSafe            bool
	aggressiveMethodSideExitChecked bool
	aggressiveMethodSideExitSafe    bool
	aggressiveBlockChecked          bool
	aggressiveBlockSafe             bool
	// blockReturn is set when the source function ends in OpBlockReturn.
	// Such a plan is valid for a block frame, but must not be used as a
	// normal method return path because OpBlockReturn has different unwind
	// semantics from OpReturnValue.
	blockReturn       bool
	hasExplicitReturn bool
	requiresFrame     bool
	hasImplicitSends  bool
	noFrameGeneration uint64
	noFrameCalls      uint8
	noFrameDisabled   bool
	// Trusted block regions may execute the same top-level constant load many
	// thousands of times. Keep the resolved values beside the plan, scoped to
	// the VM and constant-generation epoch that produced them.
	trustedTopLevelConstantVM         *VM
	trustedTopLevelConstantGeneration uint64
	trustedTopLevelConstants          []*object.EmeraldValue
	// directFastKind identifies a small, shape-based speculative block.  The
	// block is admitted independently of the broad branch tier and deopts to
	// the original framed method on any cache/type miss.
	directFastKind                    uint8
	directFastFirstIvar               string
	directFastMemoIvar                string
	directFastSendA                   uint8
	directFastSendB                   uint8
	directFastKey                     uint8
	directFastStreamIvar              string
	directFastFilteredIvar            string
	directFastStreamLiteral           uint8
	directFastStreamSend              uint8
	directFastStreamBinary            uint8
	directFastReferenceDataIvar       string
	directFastReferenceStreamIvar     string
	directFastIntegerStringPrefixIvar string
	directFastIntegerStringFallback   uint8
	directFastIntegerStringKeyParam   uint8
	directFastIntegerStringValueParam uint8
	directFastIntegerStringThreshold  int64
	directFastIntegerStringMultiplier int64
}

type registerIRArrayLiteralIndexFold struct {
	valid        bool
	resultDst    uint8
	sourceOffset int8
}

const (
	registerIRDirectFastNone uint8 = iota
	registerIRDirectFastShortCircuitIndex
	registerIRDirectFastHashOption
	registerIRDirectFastStreamAppend
	registerIRDirectFastReferenceAppend
	registerIRDirectFastMemoizedIvar
	registerIRDirectFastIntegerStringConcat
)

type registerIRIntegerValue struct {
	value int64
	kind  uint8 // 0 = integer, 1 = false, 2 = true, 3 = nil
}

func registerIRPlanMayDeopt(plan *registerIRPlan) bool {
	if plan == nil {
		return true
	}
	if plan.mayDeoptChecked {
		return plan.mayDeopt
	}
	plan.mayDeoptChecked = true
	plan.mayDeopt = registerIRPlanMayDeoptUncached(plan)
	return plan.mayDeopt
}

func registerIRPlanMayDeoptUncached(plan *registerIRPlan) bool {
	if plan == nil {
		return true
	}
	for _, instruction := range plan.instructions {
		if instruction.op == registerIREqual || instruction.op == registerIRDynamicEqual || instruction.op == registerIRCompare {
			return true
		}
	}
	return false
}

func registerIRPlanSafeForActiveRescues(plan *registerIRPlan, vm *VM) bool {
	return plan == nil || !plan.hasImplicitSends || vm == nil || len(vm.activeRescues) == 0
}

// registerIRPlanSafeForFramedBlock is the conservative admission check for
// executing a block's Register IR after the block Frame has already been
// created.  A framed plan can perform normal sends and allocations, but an
// explicit/implicit block send still needs the bytecode loop's per-instruction
// pending break/next/return checks.  Keep those plans on the established
// interpreter path until there is an unwind-aware IR tier.
func registerIRPlanSafeForFramedBlock(plan *registerIRPlan, vm *VM) bool {
	if plan == nil || !registerIRPlanSafeForActiveRescues(plan, vm) {
		return false
	}
	if !plan.framedBlockChecked {
		plan.framedBlockChecked = true
		plan.framedBlockSafe = !plan.blockReturn
		for _, instruction := range plan.instructions {
			if instruction.blockPresent || instruction.op == registerIRLogicalSendAssignment {
				plan.framedBlockSafe = false
				break
			}
			if instruction.op == registerIRYield {
				plan.framedBlockSafe = false
				break
			}
		}
	}
	return plan.framedBlockSafe
}

func registerIRPlanSafeForFramedBlockUncached(plan *registerIRPlan) bool {
	if plan == nil || plan.blockReturn {
		return false
	}
	for _, instruction := range plan.instructions {
		if instruction.blockPresent || instruction.op == registerIRLogicalSendAssignment {
			return false
		}
	}
	return true
}

// Case bodies execute inside the method Frame that has already passed the
// normal argument/visibility checks. Unlike the collection-block tier, an
// ordinary block send in a case body is safe here: executeRegisterIRSend keeps
// the caller's currentBlock and full unwind protocol. The only operation that
// still needs a dedicated speculative tier is logical assignment, whose
// closure/setter replay can mutate the receiver before a later guard miss.
func registerIRPlanSafeForCaseDispatchFramed(plan *registerIRPlan) bool {
	if plan == nil || plan.blockReturn {
		return false
	}
	if plan.caseDispatchFramedChecked {
		return plan.caseDispatchFramedSafe
	}
	plan.caseDispatchFramedChecked = true
	plan.caseDispatchFramedSafe = true
	for _, instruction := range plan.instructions {
		if instruction.op == registerIRLogicalSendAssignment {
			plan.caseDispatchFramedSafe = false
			break
		}
		if instruction.op == registerIRSend && instruction.opcode != compiler.OpSend && instruction.opcode != compiler.OpSendWithKeywords && instruction.opcode != compiler.OpSendSetter {
			plan.caseDispatchFramedSafe = false
			break
		}
	}
	return plan.caseDispatchFramedSafe
}

// registerIRPlanSafeForFramedBlockReturn admits block bodies that retain the
// normal Ruby Frame/unwind protocol while replacing only the body dispatch
// loop. OpBlockReturn is a normal implicit block result. Bare identifier sends
// keep their call-kind marker in Register IR and are safe here; a missing bare
// method is converted back to Ruby's NameError at the send boundary. Local
// branches are handled by the Register IR executor and do not change the
// surrounding block owner or frame.
func registerIRPlanSafeForFramedBlockReturn(plan *registerIRPlan, vm *VM) bool {
	if plan == nil || !registerIRPlanSafeForActiveRescues(plan, vm) {
		return false
	}
	if !plan.framedBlockReturnChecked {
		plan.framedBlockReturnChecked = true
		// A block-return plan may branch locally; the frame executor preserves
		// block unwinding while the Register IR loop handles the branch targets.
		plan.framedBlockReturnSafe = plan.blockReturn && registerIRFramedBlockReturnInstructionsSafe(plan)
	}
	return plan.framedBlockReturnSafe
}

func registerIRPlanSafeForFramedBlockReturnUncached(plan *registerIRPlan) bool {
	if plan == nil || !plan.blockReturn {
		return false
	}
	return registerIRFramedBlockReturnInstructionsSafe(plan)
}

// registerIRPlanSafeForFramedExplicitBlockReturn is intentionally separate
// from the long-standing implicit block-return predicate.  Explicit returns
// carry a non-local owner and must only be admitted by a caller that can prove
// the owner is active and can propagate the pending return after the callback.
func registerIRPlanSafeForFramedExplicitBlockReturn(plan *registerIRPlan, vm *VM) bool {
	if plan == nil || !plan.hasExplicitReturn || !registerIRPlanSafeForActiveRescues(plan, vm) {
		return false
	}
	return registerIRFramedBlockReturnInstructionsSafe(plan)
}

func registerIRFramedBlockReturnInstructionsSafe(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	for index := range plan.instructions {
		instruction := &plan.instructions[index]
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadConstantValue, registerIRLoadFrozenString,
			registerIRLoadConstant, registerIRLoadScopedConstant,
			registerIRLoadInstanceVar, registerIRLoadSelf, registerIRLoadFree,
			registerIRMove, registerIRSwap, registerIRBang,
			registerIRNeg, registerIRNotEqual,
			registerIRStoreLocal, registerIRStoreFree, registerIRStoreInstanceVar,
			registerIRArray, registerIRHash, registerIRHashMerge,
			registerIRMultiAssignPrepare, registerIRMultiAssignExtract,
			registerIRMultiAssignCheckToAry,
			registerIRMarkKeywordHash, registerIRRange, registerIRSetStringEncoding,
			registerIREqual, registerIRBinary, registerIRCompare,
			registerIRDynamicCompare, registerIRIndex, registerIRSlice,
			registerIRIndexAssign, registerIRRaise,
			registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy,
			registerIRJumpNotNil, registerIRJumpLocalPresent,
			registerIRReturn:
		case registerIRSend:
			// A send without a block has no implicit block unwind to observe.
			// Keyword and setter sends are handled by the framed IR executor too;
			// unlike the no-frame tier they retain the full dispatcher semantics.
			if instruction.blockPresent {
				return false
			}
		default:
			return false
		}
	}
	return true
}

const (
	registerIRFramelessUnknown uint8 = iota
	registerIRFramelessInteger
	registerIRFramelessArray
	registerIRFramelessHash
	registerIRFramelessBool
)

// registerIRPlanSafeForFramelessBlock admits a side-effect-free block tier.
// Array/Hash literals are safe to allocate without a Frame when ObjectSpace
// allocation tracing is disabled. A direct [] on an unknown receiver is also
// admitted when no earlier operation can have escaped a side effect: the
// executor's exact Array/Hash guard then either completes the lookup or
// deopts before user code. Non-native sends use the same cached leaf proof as
// the no-frame call-chain tier; a cache/shape miss returns false before the
// ordinary framed block is replayed. Branches, stores and implicit block
// sends remain excluded because they need the caller's unwind protocol.
func registerIRPlanSafeForFramelessBlock(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	if plan.framelessBlockChecked {
		return plan.framelessBlockSafe
	}
	plan.framelessBlockChecked = true
	plan.framelessBlockSafe = registerIRPlanSafeForFramelessBlockUncached(plan)
	return plan.framelessBlockSafe
}

func registerIRPlanSafeForFramelessBlockUncached(plan *registerIRPlan) bool {
	if plan == nil || plan.hasBranches || plan.hasImplicitSends {
		return false
	}
	if plan.blockReturn {
		// The frameless executor returns the register result directly to the
		// collection caller, so an implicit block return is safe only for a
		// side-effect-free shape.  Captures/stores/branches and dynamic sends
		// remain rejected by the instruction checks below.
		if plan.sendCount > 1 {
			return false
		}
	}
	var kinds [16]uint8
	sideEffect := false
	dynamicSend := false
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadSelf, registerIRLoadInstanceVar:
			kinds[instruction.dst] = registerIRFramelessUnknown
		case registerIRLoadFree:
			// Pure blocks may read an immutable capture.  The batch caller
			// supplies the closure's current Free slice explicitly; stores and
			// control-flow-sensitive captures remain rejected below.
			kinds[instruction.dst] = registerIRFramelessUnknown
		case registerIRLoadLiteral:
			if instruction.value != nil && instruction.value.Type == object.ValueInteger && smallIntegerValue(instruction.value) {
				kinds[instruction.dst] = registerIRFramelessInteger
			} else {
				kinds[instruction.dst] = registerIRFramelessUnknown
			}
		case registerIRMove:
			kinds[instruction.dst] = kinds[instruction.left]
		case registerIRSwap:
			kinds[instruction.left], kinds[instruction.right] = kinds[instruction.right], kinds[instruction.left]
		case registerIRBang:
			kinds[instruction.dst] = registerIRFramelessBool
		case registerIRBinary:
			if sideEffect && (kinds[instruction.left] != registerIRFramelessInteger || kinds[instruction.right] != registerIRFramelessInteger) {
				return false
			}
			kinds[instruction.dst] = registerIRFramelessInteger
		case registerIRSend:
			if instruction.opcode != compiler.OpSend || instruction.blockPresent {
				return false
			}
			if !registerIRDirectNativeName(instruction.name) {
				// A non-native send is safe only in a straight-line plan with no
				// frame requirement. executeRegisterIRInlineSendNoFrame admits
				// only an already-proven native/accessor/Register-IR leaf and
				// returns false before invoking an unproven Ruby method.
				if plan.requiresFrame || plan.hasBranches {
					return false
				}
				dynamicSend = true
			}
			kinds[instruction.dst] = registerIRFramelessUnknown
		case registerIRArray:
			kinds[instruction.dst] = registerIRFramelessArray
			sideEffect = true
		case registerIRHash:
			kinds[instruction.dst] = registerIRFramelessHash
			sideEffect = true
		case registerIRIndex:
			switch kinds[instruction.left] {
			case registerIRFramelessArray:
				if kinds[instruction.right] != registerIRFramelessInteger {
					return false
				}
			case registerIRFramelessHash:
			case registerIRFramelessUnknown:
				// An unknown receiver may still be an exact built-in Array or
				// Hash (for example @glyph_table[r]). Do not allow the guard
				// after a dynamic send or literal mutation, where replaying the
				// framed block could duplicate an observable effect.
				if sideEffect || dynamicSend {
					return false
				}
			default:
				return false
			}
			kinds[instruction.dst] = registerIRFramelessUnknown
		case registerIRReturn:
			return true
		default:
			return false
		}
	}
	return false
}

// registerIRPlanTrustedArrayLiteralIndex proves the narrow shape needed by
// the repeated-block raw Array#[] path.  The indexed receiver must come from
// an Array literal produced by this plan and the key must be a small integer
// literal.  Keeping the proof local to the plan means the executor does not
// inspect Array#[]'s method generation for every callback element.
func registerIRPlanTrustedArrayLiteralIndex(plan *registerIRPlan) bool {
	if plan == nil || plan.hasBranches || plan.sendCount != 0 || plan.hasImplicitSends {
		return false
	}
	var arrays [16]bool
	var integers [16]bool
	sawIndex := false
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadInstanceVar,
			registerIRLoadSelf, registerIRLoadFree:
			arrays[instruction.dst] = false
			integers[instruction.dst] = false
		case registerIRLoadLiteral:
			arrays[instruction.dst] = false
			integers[instruction.dst] = instruction.value != nil && smallIntegerValue(instruction.value)
		case registerIRMove:
			arrays[instruction.dst] = arrays[instruction.left]
			integers[instruction.dst] = integers[instruction.left]
		case registerIRSwap:
			arrays[instruction.left], arrays[instruction.right] = arrays[instruction.right], arrays[instruction.left]
			integers[instruction.left], integers[instruction.right] = integers[instruction.right], integers[instruction.left]
		case registerIRBang, registerIREqual, registerIRBinary, registerIRCompare,
			registerIRDynamicCompare:
			arrays[instruction.dst] = false
			integers[instruction.dst] = false
		case registerIRArray:
			arrays[instruction.dst] = true
			integers[instruction.dst] = false
		case registerIRIndex:
			if !arrays[instruction.left] || !integers[instruction.right] {
				return false
			}
			sawIndex = true
			arrays[instruction.dst] = false
			integers[instruction.dst] = false
		case registerIRReturn:
			// The surrounding block executor owns block-return semantics.
		default:
			return false
		}
	}
	return sawIndex
}

func registerIRPlanSafeWithoutFrame(plan *registerIRPlan) bool {
	if plan == nil || plan.blockReturn || plan.requiresFrame || plan.hasSends {
		return false
	}
	for _, instruction := range plan.instructions {
		if instruction.op == registerIRBinary {
			return false
		}
	}
	return true
}

// registerIRPlanSafeForBranchNoFrameBlock admits a branch-bearing scalar block
// with no send, allocation, constant lookup, or unwind opcode. Local slots are
// private to the block and can be kept in the companion executor's small local
// array; they do not escape without a closure operation (which is rejected).
func registerIRPlanSafeForBranchNoFrameBlock(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	if plan.branchNoFrameBlockChecked {
		return plan.branchNoFrameBlockSafe
	}
	plan.branchNoFrameBlockChecked = true
	plan.branchNoFrameBlockSafe = registerIRPlanSafeForBranchNoFrameBlockUncached(plan)
	return plan.branchNoFrameBlockSafe
}

func registerIRPlanSafeForBranchNoFrameBlockUncached(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	planSafe := plan.hasBranches && !plan.hasSends && !plan.hasImplicitSends
	if planSafe {
		for _, instruction := range plan.instructions {
			switch instruction.op {
			case registerIRLoadParam, registerIRLoadSelf, registerIRLoadInstanceVar,
				registerIRLoadFree, registerIRLoadLocal, registerIRMove, registerIRBang,
				registerIRStoreLocal,
				registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy,
				registerIRJumpNotNil, registerIRReturn:
			case registerIRLoadLiteral:
				if !registerIRImmediateLiteral(instruction.value) {
					planSafe = false
				}
			default:
				planSafe = false
			}
			if !planSafe {
				break
			}
		}
	}
	return planSafe
}

func registerIRImmediateLiteral(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	switch value.Type {
	case object.ValueNil, object.ValueBool, object.ValueInteger, object.ValueFloat, object.ValueSymbol:
		return true
	default:
		return false
	}
}

// registerIRDirectFastShape recognizes short-circuit blocks whose successful
// path consists only of instance reads, proven accessor/native sends and an
// exact built-in index.  It deliberately describes instruction shape rather
// than Ruby names, so the same executor applies to SDK models, user classes
// and other generated code.  The normal framed plan remains the deopt path.
func registerIRDirectFastShape(plan *registerIRPlan) uint8 {
	if plan == nil || plan.blockReturn {
		return registerIRDirectFastNone
	}
	instructions := plan.instructions
	// `@value ||= expression` has a side-effect-free hot path once the memoized
	// ivar is truthy: Ruby returns the existing object without evaluating the
	// initializer.  Keep the initializer on the normal framed path (it may
	// allocate, send, or raise), but bypass that frame and binder for the common
	// already-initialized case.  The exact twelve-instruction shape below is the
	// compiler form for `@renderer ||= PDF::Core::Renderer.new(state)` and is
	// intentionally structural so equivalent memoized readers benefit too.
	if registerIRMemoizedReaderEnabled && len(instructions) == 12 {
		ivar := instructions[0]
		copy := instructions[1]
		branch := instructions[2]
		loadOuter := instructions[3]
		loadMiddle := instructions[4]
		loadInner := instructions[5]
		self := instructions[6]
		accessor := instructions[7]
		constructor := instructions[8]
		store := instructions[9]
		jump := instructions[10]
		result := instructions[11]
		if ivar.op == registerIRLoadInstanceVar && ivar.name != "" &&
			copy.op == registerIRMove && copy.left == ivar.dst &&
			branch.op == registerIRJumpTruthy && branch.left == copy.dst && branch.target == 11 &&
			loadOuter.op == registerIRLoadConstant && loadOuter.name != "" &&
			loadMiddle.op == registerIRLoadScopedConstant && loadMiddle.name != "" && loadMiddle.left == loadOuter.dst &&
			loadInner.op == registerIRLoadScopedConstant && loadInner.name != "" && loadInner.left == loadMiddle.dst &&
			self.op == registerIRLoadSelf &&
			accessor.op == registerIRSend && accessor.name != "" && accessor.left == self.dst && accessor.argc == 0 && !accessor.blockPresent &&
			constructor.op == registerIRSend && constructor.name == "new" && constructor.left == loadInner.dst && constructor.argc == 1 && !constructor.blockPresent && constructor.args[0] == accessor.dst &&
			store.op == registerIRStoreInstanceVar && store.name == ivar.name && store.left == constructor.dst &&
			jump.op == registerIRJump && jump.target == 11 &&
			result.op == registerIRReturn && result.left == ivar.dst {
			plan.directFastMemoIvar = ivar.name
			return registerIRDirectFastMemoizedIvar
		}
	}
	// `unless @data.is_a?(::Hash); raise ...; end; (@stream ||= Stream.new) << io`
	// is the hot PDF::Core::Reference append path.  The direct tier only takes
	// the exact built-in Hash/existing-stream success case; non-Hash values,
	// missing streams and custom dispatch replay the full framed method so its
	// exception and constructor semantics remain unchanged.
	if len(instructions) == 18 {
		data := instructions[0]
		hashClass := instructions[1]
		isA := instructions[2]
		dataBranch := instructions[3]
		raiseClass := instructions[4]
		raise := instructions[5]
		skipRaise := instructions[6]
		nilValue := instructions[7]
		stream := instructions[8]
		streamCopy := instructions[9]
		streamBranch := instructions[10]
		streamClass := instructions[11]
		streamNew := instructions[12]
		streamStore := instructions[13]
		streamEnd := instructions[14]
		argument := instructions[15]
		appendBinary := instructions[16]
		result := instructions[17]
		if data.op == registerIRLoadInstanceVar && data.name != "" &&
			hashClass.op == registerIRLoadConstant && strings.HasSuffix(hashClass.name, "Hash") &&
			isA.op == registerIRSend && isA.name == "is_a?" && isA.left == data.dst && isA.argc == 1 && isA.args[0] == hashClass.dst && !isA.blockPresent &&
			dataBranch.op == registerIRJumpTruthy && dataBranch.left == isA.dst && dataBranch.target == 7 &&
			raiseClass.op == registerIRLoadConstant && raiseClass.name != "" && raise.op == registerIRRaise && raise.left == raiseClass.dst &&
			skipRaise.op == registerIRJump && skipRaise.target == 8 &&
			nilValue.op == registerIRLoadLiteral && nilValue.value != nil && nilValue.value.Type == object.ValueNil &&
			stream.op == registerIRLoadInstanceVar && stream.name != "" && streamCopy.op == registerIRMove && streamCopy.left == stream.dst &&
			streamBranch.op == registerIRJumpTruthy && streamBranch.left == stream.dst && streamBranch.target == 15 &&
			streamClass.op == registerIRLoadConstant && strings.HasSuffix(streamClass.name, "Stream") &&
			streamNew.op == registerIRSend && streamNew.name == "new" && streamNew.left == streamClass.dst && streamNew.argc == 0 && !streamNew.blockPresent &&
			streamStore.op == registerIRStoreInstanceVar && streamStore.left == streamNew.dst && streamStore.name == stream.name &&
			streamEnd.op == registerIRJump && streamEnd.target == 15 &&
			argument.op == registerIRLoadParam && argument.param == 0 &&
			appendBinary.op == registerIRBinary && appendBinary.opcode == compiler.OpBitLeftShift && appendBinary.left == stream.dst && appendBinary.right == argument.dst &&
			result.op == registerIRReturn && result.left == appendBinary.dst {
			plan.directFastReferenceDataIvar = data.name
			plan.directFastReferenceStreamIvar = stream.name
			return registerIRDirectFastReferenceAppend
		}
	}
	// Keep the shape proof tolerant of register renumbering performed by a
	// future compiler revision.  The operation/name sequence is still exact;
	// runtime guards below only use the two instance-variable names.
	if len(instructions) == 18 &&
		instructions[0].op == registerIRLoadInstanceVar && instructions[0].name != "" &&
		instructions[1].op == registerIRLoadConstant && strings.HasSuffix(instructions[1].name, "Hash") &&
		instructions[2].op == registerIRSend && instructions[2].name == "is_a?" && instructions[2].argc == 1 && !instructions[2].blockPresent &&
		instructions[3].op == registerIRJumpTruthy && instructions[3].target == 7 &&
		instructions[4].op == registerIRLoadConstant && instructions[4].name != "" &&
		instructions[5].op == registerIRRaise && instructions[6].op == registerIRJump && instructions[6].target == 8 &&
		instructions[7].op == registerIRLoadLiteral && instructions[7].value != nil && instructions[7].value.Type == object.ValueNil &&
		instructions[8].op == registerIRLoadInstanceVar && instructions[8].name != "" &&
		instructions[9].op == registerIRMove && instructions[10].op == registerIRJumpTruthy && instructions[10].target == 15 &&
		instructions[11].op == registerIRLoadConstant && strings.HasSuffix(instructions[11].name, "Stream") &&
		instructions[12].op == registerIRSend && instructions[12].name == "new" && instructions[12].argc == 0 && !instructions[12].blockPresent &&
		instructions[13].op == registerIRStoreInstanceVar && instructions[13].name == instructions[8].name &&
		instructions[14].op == registerIRJump && instructions[14].target == 15 &&
		instructions[15].op == registerIRLoadParam && instructions[15].param == 0 &&
		instructions[16].op == registerIRBinary && instructions[16].opcode == compiler.OpBitLeftShift &&
		instructions[17].op == registerIRReturn {
		plan.directFastReferenceDataIvar = instructions[0].name
		plan.directFastReferenceStreamIvar = instructions[8].name
		return registerIRDirectFastReferenceAppend
	}
	// @stream ||= +''; @stream << io; @filtered_stream = nil; self
	//
	// This is the compact shape emitted for the common PDF::Core stream
	// appender.  It is deliberately structural: any class can use the same
	// sequence, while custom String subclasses, redefined String#+@/<< and
	// mutable literal modes deopt before user-visible mutation.
	if len(instructions) == 13 {
		stream := instructions[0]
		copyStream := instructions[1]
		streamBranch := instructions[2]
		literal := instructions[3]
		unary := instructions[4]
		storeStream := instructions[5]
		jumpEnd := instructions[6]
		argument := instructions[7]
		appendBinary := instructions[8]
		filteredNil := instructions[9]
		storeFiltered := instructions[10]
		self := instructions[11]
		result := instructions[12]
		frozenLiteral := literal.value != nil && literal.value.Type == object.ValueString &&
			(literal.op == registerIRLoadFrozenString || literal.op == registerIRLoadLiteral && literal.value.Frozen)
		if stream.op == registerIRLoadInstanceVar && stream.name != "" &&
			copyStream.op == registerIRMove && copyStream.left == stream.dst &&
			streamBranch.op == registerIRJumpTruthy && streamBranch.left == copyStream.dst && streamBranch.target == 7 &&
			frozenLiteral &&
			unary.op == registerIRSend && unary.left == literal.dst && unary.name == "+@" && unary.argc == 0 && !unary.blockPresent && unary.opcode == compiler.OpSend &&
			storeStream.op == registerIRStoreInstanceVar && storeStream.left == unary.dst && storeStream.name == stream.name &&
			jumpEnd.op == registerIRJump && jumpEnd.target == 7 &&
			argument.op == registerIRLoadParam && argument.param == 0 &&
			appendBinary.op == registerIRBinary && appendBinary.opcode == compiler.OpBitLeftShift && appendBinary.left == stream.dst && appendBinary.right == argument.dst && appendBinary.dst == stream.dst &&
			filteredNil.op == registerIRLoadLiteral && filteredNil.value != nil && filteredNil.value.Type == object.ValueNil &&
			storeFiltered.op == registerIRStoreInstanceVar && storeFiltered.left == filteredNil.dst && storeFiltered.name != "" && storeFiltered.name != stream.name &&
			self.op == registerIRLoadSelf && result.op == registerIRReturn && result.left == self.dst {
			plan.directFastStreamIvar = stream.name
			plan.directFastFilteredIvar = storeFiltered.name
			plan.directFastStreamLiteral = 3
			plan.directFastStreamSend = 4
			plan.directFastStreamBinary = 8
			return registerIRDirectFastStreamAppend
		}
	}
	// @first || (send && send[:literal])
	if len(instructions) == 12 &&
		instructions[0].op == registerIRLoadInstanceVar && instructions[0].name != "" &&
		instructions[1].op == registerIRMove && instructions[1].left == instructions[0].dst &&
		instructions[2].op == registerIRJumpTruthy && instructions[2].left == instructions[0].dst &&
		instructions[2].target == 11 &&
		instructions[3].op == registerIRMove && instructions[3].left == instructions[0].dst &&
		instructions[4].op == registerIRSend && instructions[4].left == instructions[0].dst && instructions[4].argc == 0 && !instructions[4].blockPresent &&
		instructions[5].op == registerIRMove && instructions[5].left == instructions[4].dst &&
		instructions[6].op == registerIRJumpNotTruthy && instructions[6].left == instructions[4].dst &&
		instructions[6].target == 11 &&
		instructions[7].op == registerIRMove && instructions[7].left == instructions[4].dst &&
		instructions[8].op == registerIRSend && instructions[8].left == instructions[4].dst && instructions[8].argc == 0 && !instructions[8].blockPresent &&
		instructions[9].op == registerIRLoadLiteral && instructions[9].value != nil &&
		instructions[10].op == registerIRIndex && instructions[10].left == instructions[8].dst &&
		instructions[10].right == instructions[9].dst && instructions[10].dst == instructions[8].dst &&
		instructions[11].op == registerIRReturn && instructions[11].left == instructions[0].dst {
		plan.directFastFirstIvar = instructions[0].name
		plan.directFastSendA = 4
		plan.directFastSendB = 8
		plan.directFastKey = 9
		return registerIRDirectFastShortCircuitIndex
	}
	// if options.key?(name); options[name]; else raise ... end
	// The false branch is intentionally not executed speculatively: the fast
	// block returns a miss and the framed plan recreates the exact Ruby error.
	if len(instructions) == 14 &&
		instructions[0].op == registerIRLoadParam &&
		instructions[1].op == registerIRLoadParam &&
		instructions[2].op == registerIRSend && instructions[2].name == "key?" &&
		instructions[2].argc == 1 && !instructions[2].blockPresent &&
		instructions[2].left == instructions[0].dst && instructions[2].args[0] == instructions[1].dst &&
		instructions[3].op == registerIRJumpNotTruthy && instructions[3].target == 8 &&
		instructions[4].op == registerIRLoadParam && instructions[5].op == registerIRLoadParam &&
		instructions[4].param == instructions[0].param && instructions[5].param == instructions[1].param &&
		instructions[6].op == registerIRIndex && instructions[6].left == instructions[4].dst &&
		instructions[6].right == instructions[5].dst && instructions[6].dst == instructions[4].dst &&
		instructions[7].op == registerIRJump &&
		instructions[7].target == 13 && instructions[8].op == registerIRLoadSelf &&
		instructions[9].op == registerIRLoadConstant && instructions[10].op == registerIRLoadLiteral &&
		instructions[11].op == registerIRSend && instructions[11].name == "raise" &&
		instructions[11].argc == 2 && !instructions[11].blockPresent &&
		instructions[11].left == instructions[8].dst && instructions[11].args[0] == instructions[9].dst &&
		instructions[11].args[1] == instructions[10].dst && instructions[12].op == registerIRRaise &&
		instructions[12].left == instructions[11].dst && instructions[13].op == registerIRReturn &&
		instructions[13].left == instructions[0].dst {
		plan.directFastSendA = 2
		plan.directFastKey = 1
		return registerIRDirectFastHashOption
	}
	if registerIRIntegerStringFastEnabled && registerIRDirectFastIntegerStringConcatShape(plan) {
		return registerIRDirectFastIntegerStringConcat
	}
	return registerIRDirectFastNone
}

// registerIRDirectFastIntegerStringConcatShape recognizes the structural
// formatter graph used by many generated Ruby helpers:
//
//	if value > integer
//	  @prefix + (key * integer + value).to_s
//	else
//	  "fallback"
//	end
//
// The fast executor still guards every dynamic boundary at runtime.  Keeping
// this proof structural makes it reusable for SDK/Gem classes without tying
// it to a class or source-file name.
func registerIRDirectFastIntegerStringConcatShape(plan *registerIRPlan) bool {
	if plan == nil || len(plan.instructions) != 15 || plan.blockReturn {
		return false
	}
	instructions := plan.instructions
	conditionValue := instructions[0]
	conditionLiteral := instructions[1]
	condition := instructions[2]
	branch := instructions[3]
	prefix := instructions[4]
	key := instructions[5]
	multiplier := instructions[6]
	product := instructions[7]
	value := instructions[8]
	total := instructions[9]
	toString := instructions[10]
	concat := instructions[11]
	join := instructions[12]
	fallback := instructions[13]
	result := instructions[14]
	if conditionValue.op != registerIRLoadParam ||
		conditionLiteral.op != registerIRLoadLiteral || !smallIntegerValue(conditionLiteral.value) ||
		condition.op != registerIRCompare || condition.opcode != compiler.OpGreaterThan ||
		condition.left != conditionValue.dst || condition.right != conditionLiteral.dst ||
		branch.op != registerIRJumpNotTruthy || branch.left != condition.dst || branch.target != 13 ||
		prefix.op != registerIRLoadInstanceVar || prefix.name == "" ||
		key.op != registerIRLoadParam || key.param == conditionValue.param ||
		multiplier.op != registerIRLoadLiteral || !smallIntegerValue(multiplier.value) ||
		product.op != registerIRBinary || product.opcode != compiler.OpMul ||
		product.left != key.dst || product.right != multiplier.dst ||
		value.op != registerIRLoadParam || value.param != conditionValue.param ||
		total.op != registerIRBinary || total.opcode != compiler.OpAdd ||
		total.left != product.dst || total.right != value.dst ||
		toString.op != registerIRSend || toString.name != "to_s" || toString.argc != 0 ||
		toString.opcode != compiler.OpSend || toString.blockPresent || toString.splatIndex != 255 ||
		toString.left != total.dst ||
		concat.op != registerIRBinary || concat.opcode != compiler.OpAdd ||
		concat.left != prefix.dst || concat.right != toString.dst ||
		join.op != registerIRJump || join.target != 14 ||
		(fallback.op != registerIRLoadConstantValue && fallback.op != registerIRLoadLiteral && fallback.op != registerIRLoadFrozenString) ||
		fallback.value == nil || fallback.value.Type != object.ValueString ||
		fallback.dst != concat.dst || result.op != registerIRReturn || result.left != concat.dst {
		return false
	}
	plan.directFastIntegerStringPrefixIvar = prefix.name
	plan.directFastIntegerStringFallback = 13
	plan.directFastIntegerStringKeyParam = key.param
	plan.directFastIntegerStringValueParam = conditionValue.param
	plan.directFastIntegerStringThreshold = conditionLiteral.value.Data.(int64)
	plan.directFastIntegerStringMultiplier = multiplier.value.Data.(int64)
	return true
}

// registerIRConstructorPlan recognizes the closed-world initializer shape
// emitted for ordinary value objects, for example
// `def initialize(v); @v = v; @tag = 1; end`.  It has no sends, branches,
// allocations, or captures: the only effects are deterministic ivar writes
// followed by the method's final return.  Such a method can run through the
// direct ABI immediately; waiting for the general no-frame warmup would make
// object-heavy constructors pay a full Ruby Frame for every allocation.
func registerIRConstructorPlan(plan *registerIRPlan) bool {
	if plan == nil || plan.hasBranches || plan.sendCount != 0 || plan.hasImplicitSends || plan.blockReturn || len(plan.instructions) == 0 {
		return false
	}
	stores := 0
	returned := false
	for index, instruction := range plan.instructions {
		if returned {
			return false
		}
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadSelf, registerIRMove, registerIRSwap:
		case registerIRLoadLiteral:
			if !registerIRImmutableLiteral(instruction.value) {
				return false
			}
		case registerIRStoreInstanceVar:
			if instruction.name == "" {
				return false
			}
			stores++
		case registerIRReturn:
			if index != len(plan.instructions)-1 || stores == 0 {
				return false
			}
			returned = true
		default:
			return false
		}
	}
	return returned
}

// registerIRPlanSafeForDirectNoFrame is the conservative predicate for a
// framed plan that can still be executed without a Ruby Frame. The only
// operations admitted here are reads, private scalar local slots, control
// flow, exact built-in indexing, and sends that are proven pure by an
// already-hot native/accessor cache. The one intentional side-effecting
// exception is registerIRConstructorPlan: its complete graph is a sequence of
// ivar writes, so there is no later speculative guard that could replay a
// partially-mutated receiver.
func registerIRPlanSafeForDirectNoFrame(plan *registerIRPlan) bool {
	if plan != nil && plan.directNoFrameChecked {
		return plan.directNoFrameSafe
	}
	return registerIRPlanSafeForDirectNoFrameUncached(plan)
}

func registerIRPlanSafeForDirectNoFrameUncached(plan *registerIRPlan) bool {
	return registerIRPlanSafeForDirectNoFrameWithOptionsUncached(plan, false, false, false)
}

// registerIRDirectNoFrameUnsupportedOp is used only by the opt-in plan
// profiler.  Keeping the first rejected operation visible makes it possible
// to widen the speculative tier from actual gem hot spots instead of adding
// name-specific fast paths blindly.
func registerIRDirectNoFrameUnsupportedOp(plan *registerIRPlan) string {
	if plan == nil {
		return "nil"
	}
	for _, instruction := range plan.instructions {
		supported := false
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLiteral, registerIRLoadInstanceVar,
			registerIRLoadSelf, registerIRMove, registerIRSwap, registerIRBang,
			registerIRSend, registerIRIndex, registerIRJump, registerIRJumpTruthy,
			registerIRJumpNotTruthy, registerIRJumpNotNil, registerIRReturn,
			registerIRBinary, registerIRCompare, registerIREqual, registerIRLoadLocal,
			registerIRStoreLocal, registerIRArray, registerIRHash,
			registerIRDefinedInstanceVar, registerIRDynamicCompare, registerIRSlice,
			registerIRSetStringEncoding, registerIRStoreInstanceVar, registerIRStoreFree:
			supported = true
		}
		if !supported {
			if definition, ok := compiler.Lookup(byte(instruction.opcode)); ok {
				return definition.Name
			}
			return "register_ir"
		}
	}
	return "none"
}

// registerIRPlanSafeForDirectNoFrameBlock is the block-only companion to the
// method direct tier. A block's OpBlockReturn can be returned directly to its
// collection caller when the plan has no implicit block send; methods must
// continue rejecting that terminator because it has different unwind rules.
// Constants are admitted only in this block tier, where the caller validates
// that lexical scopes do not shadow the top-level constant lookup.
func registerIRPlanSafeForDirectNoFrameBlock(plan *registerIRPlan) bool {
	if plan != nil && plan.directNoFrameBlockChecked {
		return plan.directNoFrameBlockSafe
	}
	return registerIRPlanSafeForDirectNoFrameWithOptions(plan, true, true)
}

func registerIRPlanSafeForDirectNoFrameWithOptions(plan *registerIRPlan, allowBlockReturn, allowConstants bool, caseBranch ...bool) bool {
	allowCaseBranch := len(caseBranch) > 0 && caseBranch[0]
	if plan == nil {
		return false
	}
	optionKey := uint8(1)
	if allowBlockReturn {
		optionKey |= 2
	}
	if allowConstants {
		optionKey |= 4
	}
	if allowCaseBranch {
		optionKey |= 8
	}
	if plan.directNoFrameOptionsChecked&optionKey != 0 {
		return plan.directNoFrameOptionsSafe&optionKey != 0
	}
	safe := registerIRPlanSafeForDirectNoFrameWithOptionsUncached(plan, allowBlockReturn, allowConstants, allowCaseBranch)
	plan.directNoFrameOptionsChecked |= optionKey
	if safe {
		plan.directNoFrameOptionsSafe |= optionKey
	}
	return safe
}

func registerIRPlanSafeForDirectNoFrameWithOptionsUncached(plan *registerIRPlan, allowBlockReturn, allowConstants, allowCaseBranch bool) bool {
	constructor := registerIRConstructorPlan(plan)
	if plan == nil || (!allowBlockReturn && plan.blockReturn) || !plan.hasSends && !allowCaseBranch && !constructor {
		return false
	}
	if plan.directFastKind != registerIRDirectFastNone {
		return true
	}
	for index, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLiteral, registerIRLoadInstanceVar,
			registerIRLoadSelf, registerIRMove, registerIRSwap, registerIRBang,
			registerIRSend, registerIRIndex, registerIRJump, registerIRJumpTruthy,
			registerIRJumpNotTruthy, registerIRJumpNotNil, registerIRReturn,
			registerIRBinary, registerIRCompare, registerIREqual, registerIRLoadLocal,
			registerIRStoreLocal, registerIRArray, registerIRHash,
			registerIRDefinedInstanceVar, registerIRDynamicCompare, registerIRSlice:
			if instruction.op == registerIRSend && (instruction.opcode != compiler.OpSend || instruction.blockPresent) {
				return false
			}
			if instruction.op == registerIRSend && instruction.name == "raise" {
				// `raise(...)` is the bytecode-side half of a cold validation
				// branch. Do not execute it speculatively; the following
				// registerIRRaise is the deopt boundary that restores the normal
				// exception/backtrace protocol.
				if index+1 >= len(plan.instructions) || plan.instructions[index+1].op != registerIRRaise ||
					!registerIRDirectRaisePrefixSafe(plan, index) {
					return false
				}
			}
			// A case branch is entered speculatively after its predicate has
			// matched. Plain sends may still be admitted when their runtime cache
			// proves a no-frame leaf (including another Register IR method); the
			// executor rejects block/keyword/setter sends above because those need
			// the caller's full binding and unwind protocol.
		case registerIRLoadConstant, registerIRLoadScopedConstant:
			if !allowConstants {
				return false
			}
		case registerIRLoadConstantValue:
			// Direct blocks can materialize a fresh String literal without a
			// Ruby Frame.  The value is cloned per invocation, matching
			// constantValue's mutable-literal semantics; frozen literals are
			// compiled as registerIRLoadLiteral instead.
			if instruction.value == nil || instruction.value.Type != object.ValueString {
				return false
			}
		case registerIRSetStringEncoding:
			// Encoding is a value-preserving mutation of the result string. It is
			// safe without a Frame when it is terminal or immediately follows a
			// proven builtin String#+ result. The latter is a fresh temporary that
			// has not escaped; allowing it removes a frame from typed string-building
			// blocks while a branch or an older user value still side-exits.
			if !registerIRDirectTerminalMutationAt(plan, index, instruction.left) &&
				!registerIRFreshStringEncodingSafe(plan, index, instruction.left) {
				return false
			}
		case registerIRStoreInstanceVar:
			// The same rule applies to a final ivar assignment. The normal
			// executor already returns the frozen-object exception from the store
			// primitive, so the direct tier can preserve that result. A complete
			// constructor graph is safe even when it has more than one store;
			// there is no cache/type guard between those stores.
			if !constructor && !registerIRDirectTerminalMutationAt(plan, index, instruction.left) {
				return false
			}
		case registerIRStoreFree:
			// A captured assignment is safe only as the final observable
			// operation. If a later native cache misses, replaying the current
			// block would otherwise apply the capture twice.
			if !allowBlockReturn || !registerIRDirectTerminalMutationAt(plan, index, instruction.left) {
				return false
			}
		case registerIRRaise:
			// A cold validation branch may raise after only pure guards. The
			// direct executor deopts when it reaches the raise, so admit it only
			// when every preceding operation is replay-safe; otherwise a later
			// cache miss could duplicate an observable mutation.
			if !registerIRDirectRaisePrefixSafe(plan, index) {
				return false
			}
		case registerIRLoadFree:
			if !allowBlockReturn {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func registerIRFreshStringEncodingSafe(plan *registerIRPlan, index int, resultRegister uint8) bool {
	if plan == nil || plan.hasBranches || index <= 0 || index >= len(plan.instructions) {
		return false
	}
	previous := plan.instructions[index-1]
	return previous.op == registerIRBinary && previous.opcode == compiler.OpAdd && previous.dst == resultRegister
}

// registerIRAggressivePlanSafe describes the larger, frame-free graph tier
// used by the explicit fast mode.  Unlike the deoptimising direct tier, this
// executor does not replay a body after an unproven send: it dispatches that
// send through the normal VM method cache and keeps executing the graph.  The
// proof therefore only excludes operations whose semantics fundamentally
// depend on a Ruby Frame (yield, closure creation, rescue/ensure, and
// non-local control flow).  Dynamic calls and ordinary allocations are safe
// here because they complete exactly once and return their Ruby value.
func registerIRAggressivePlanSafe(plan *registerIRPlan, allowBlockReturn bool) bool {
	if plan == nil || core.ObjectSpaceAllocationTracing() || plan.hasImplicitSends || plan.hasExplicitReturn || plan.blockReturn && !allowBlockReturn {
		return false
	}
	for index, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadConstantValue, registerIRLoadFrozenString,
			registerIRLoadConstant, registerIRLoadScopedConstant,
			registerIRLoadInstanceVar, registerIRDefinedInstanceVar,
			registerIRLoadSelf, registerIRLoadFree, registerIRMove,
			registerIRSwap, registerIRBang, registerIRStoreLocal,
			registerIRStoreInstanceVar, registerIRSetStringEncoding,
			registerIRArray, registerIRHash, registerIRIndex,
			registerIRIndexAssign, registerIREqual, registerIRBinary,
			registerIRCompare, registerIRDynamicCompare, registerIRSend, registerIRJump,
			registerIRJumpTruthy, registerIRJumpNotTruthy, registerIRJumpNotNil,
			registerIRRaise, registerIRReturn:
			if instruction.op == registerIRLoadConstantValue && (instruction.value == nil || instruction.value.Type != object.ValueString) {
				return false
			}
			if instruction.op == registerIRLoadFree && !allowBlockReturn {
				return false
			}
			if (instruction.op == registerIRLoadLocal || instruction.op == registerIRStoreLocal) && instruction.param >= 64 {
				return false
			}
			if instruction.op == registerIRSend && (instruction.blockPresent || instruction.splatIndex != 255 || instruction.opcode != compiler.OpSend) {
				return false
			}
		case registerIRStoreFree:
			// A captured write is compatible with the frame-free graph only
			// when it is the terminal mutation; otherwise a later graph miss
			// could replay an already-visible write.
			if !allowBlockReturn || !registerIRDirectTerminalMutationAt(plan, index, instruction.left) {
				return false
			}
		case registerIRLoadCapture, registerIRClosure,
			registerIRSplatToArray, registerIRMarkKeywordHash, registerIRHashMerge,
			registerIRRange, registerIRSlice,
			registerIRBlockGiven, registerIRYield, registerIRLogicalSendAssignment,
			registerIRJumpLocalPresent:
			return false
		default:
			return false
		}
	}
	return len(plan.instructions) > 0
}

func (vm *VM) aggressiveIRPlanForFunction(fn *object.Function, allowBlockReturn bool) (*registerIRPlan, bool) {
	if vm == nil || fn == nil || !registerIRAggressiveEnabled {
		return nil, false
	}
	plan, found := vm.aggressiveIRCache[fn]
	if !found {
		var compiled bool
		plan, compiled = compileRegisterIR(fn)
		if !compiled || plan == nil {
			vm.aggressiveIRCache[fn] = nil
			return nil, false
		}
		vm.aggressiveIRCache[fn] = plan
	}
	if plan == nil {
		return nil, false
	}
	if allowBlockReturn {
		if !plan.aggressiveBlockChecked {
			plan.aggressiveBlockSafe = registerIRAggressivePlanSafe(plan, true)
			plan.aggressiveBlockChecked = true
		}
		if !plan.aggressiveBlockSafe {
			return nil, false
		}
	} else {
		if !plan.aggressiveMethodChecked {
			plan.aggressiveMethodSafe = registerIRAggressivePlanSafe(plan, false)
			plan.aggressiveMethodChecked = true
		}
		if !plan.aggressiveMethodSafe {
			return nil, false
		}
	}
	return plan, true
}

// registerIRDirectRaisePrefixSafe proves that a raise instruction can only be
// reached after operations whose replay has no user-visible side effect. It
// intentionally uses a small whitelist instead of treating every native send
// as pure: names such as push/sub! are native too, but replaying them after a
// later guard miss would be incorrect. The direct executor itself returns a
// miss at registerIRRaise; the framed path then performs the real exception.
func registerIRDirectRaisePrefixSafe(plan *registerIRPlan, raiseIndex int) bool {
	if plan == nil || raiseIndex < 0 || raiseIndex >= len(plan.instructions) {
		return false
	}
	for index := 0; index < raiseIndex; index++ {
		instruction := plan.instructions[index]
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadConstantValue, registerIRLoadFrozenString,
			registerIRLoadConstant, registerIRLoadScopedConstant,
			registerIRLoadInstanceVar, registerIRLoadSelf, registerIRLoadFree,
			registerIRMove, registerIRSwap, registerIRBang,
			registerIREqual, registerIRCompare, registerIRDynamicCompare,
			registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy,
			registerIRJumpNotNil:
		case registerIRSend:
			if instruction.opcode != compiler.OpSend || instruction.blockPresent {
				return false
			}
			if instruction.name == "raise" {
				if index+1 >= len(plan.instructions) || plan.instructions[index+1].op != registerIRRaise {
					return false
				}
				continue
			}
			if !registerIRDirectRaisePureSendName(instruction.name) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func registerIRDirectRaisePureSendName(name string) bool {
	switch name {
	case "is_a?", "kind_of?", "instance_of?", "nil?", "frozen?", "class",
		"respond_to?", "==", "!=", "eql?", "hash", "to_sym":
		return true
	default:
		return false
	}
}

func registerIRDirectTerminalMutationAt(plan *registerIRPlan, index int, resultRegister uint8) bool {
	if plan == nil || index != len(plan.instructions)-2 || index < 0 || index+1 >= len(plan.instructions) {
		return false
	}
	return plan.instructions[index+1].op == registerIRReturn && plan.instructions[index+1].left == resultRegister
}

// registerIRDirectConstantsSafe proves that every constant read in a direct
// method/block entry can use the same top-level lookup as the existing
// frameless executor. Lexical constants are deliberately rejected here: the
// normal resolver carries more context (autoload, const_missing, and eval
// state) than a direct entry does.
func registerIRDirectConstantsSafe(vm *VM, closure *object.Closure, plan *registerIRPlan) bool {
	if vm == nil || plan == nil {
		return false
	}
	if !plan.hasConstantLoads {
		return true
	}
	var constants [16]*object.EmeraldValue
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadSelf, registerIRLoadInstanceVar,
			registerIRLoadLiteral, registerIRLoadLocal, registerIRLoadFree,
			registerIRSend, registerIRIndex, registerIRSlice, registerIRBinary,
			registerIRCompare, registerIREqual, registerIRDynamicCompare,
			registerIRBang, registerIRArray, registerIRHash:
			if int(instruction.dst) < len(constants) {
				constants[instruction.dst] = nil
			}
		case registerIRLoadScopedConstant:
			if instruction.name == "" || int(instruction.left) >= len(constants) || constants[instruction.left] == nil {
				return false
			}
			value, ok := directConstantValue(constants[instruction.left], instruction.name)
			if !ok || value == nil || value.Type == object.ValueException {
				return false
			}
			constants[instruction.dst] = value
		case registerIRLoadConstant:
			if instruction.name == "" || strings.Contains(instruction.name, "::") {
				return false
			}
			value, ok := vm.topLevelConstantValue(instruction.name)
			if !ok || value == nil || value.Type == object.ValueException {
				return false
			}
			if closure != nil {
				for _, scope := range closure.ClassStack {
					if scope == nil || scope.Type != object.ValueClass && scope.Type != object.ValueModule {
						continue
					}
					if _, shadowed := lexicalDirectConstantValue(scope, instruction.name); shadowed {
						return false
					}
				}
			}
			constants[instruction.dst] = value
		case registerIRMove:
			if int(instruction.dst) < len(constants) && int(instruction.left) < len(constants) {
				constants[instruction.dst] = constants[instruction.left]
			}
		case registerIRSwap:
			if int(instruction.left) < len(constants) && int(instruction.right) < len(constants) {
				constants[instruction.left], constants[instruction.right] = constants[instruction.right], constants[instruction.left]
			}
		}
	}
	return true
}

func (vm *VM) prepareRegisterIRTrustedTopLevelConstants(plan *registerIRPlan) bool {
	if vm == nil || plan == nil || !plan.hasConstantLoads {
		return true
	}
	hasTopLevel := false
	for _, instruction := range plan.instructions {
		if instruction.op == registerIRLoadConstant {
			hasTopLevel = true
			break
		}
	}
	if !hasTopLevel {
		return true
	}
	generation := object.CurrentConstantGeneration()
	if plan.trustedTopLevelConstantVM == vm && plan.trustedTopLevelConstantGeneration == generation &&
		len(plan.trustedTopLevelConstants) == len(plan.instructions) {
		return true
	}
	values := make([]*object.EmeraldValue, len(plan.instructions))
	for index, instruction := range plan.instructions {
		if instruction.op != registerIRLoadConstant {
			continue
		}
		value, ok := vm.topLevelConstantValue(instruction.name)
		if !ok || value == nil || value.Type == object.ValueException {
			return false
		}
		values[index] = value
	}
	plan.trustedTopLevelConstantVM = vm
	plan.trustedTopLevelConstantGeneration = generation
	plan.trustedTopLevelConstants = values
	return true
}

// registerIRPlanSafeForNoFrameInline is the stricter callee predicate used by
// send-site call-chain inlining.  A no-frame callee may contain sends, but it
// must not contain an operation whose failure would need to replay an earlier
// side effect (arithmetic/comparison guards, locals/captures, constants,
// allocation, or dynamic indexing).  Such methods can be executed by the
// existing no-frame register loop without manufacturing a nested Ruby Frame.
func registerIRPlanSafeForNoFrameInline(plan *registerIRPlan) bool {
	if plan != nil && plan.noFrameInlineChecked {
		return plan.noFrameInlineSafe
	}
	return registerIRPlanSafeForNoFrameInlineUncached(plan)
}

func registerIRPlanSafeForNoFrameInlineUncached(plan *registerIRPlan) bool {
	if plan == nil || plan.blockReturn {
		return false
	}
	// The scalar tier already proves that every operation is a side-effect-free
	// immediate operation (including private locals and ivar reads). It has its
	// own unboxed executor, so the general no-frame predicate's arithmetic and
	// local restrictions do not apply to this proven subset.
	if plan.integerOnly {
		return true
	}
	// A leaf with no sends is already proven side-effect free by the
	// no-frame predicate above.  Allow it here as a nested leaf too: callers
	// such as `items.map { |item| item.value }` otherwise pay invokeMethod and
	// a second method-plan lookup for every element even though the callee is
	// just an ivar/literal return.  Plans with sends still need the stricter
	// shape below so a cache miss can deopt before user-visible effects.
	if !plan.hasSends {
		return registerIRPlanSafeWithoutFrame(plan)
	}
	if plan.requiresFrame {
		return false
	}
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLiteral, registerIRLoadInstanceVar,
			registerIRLoadSelf, registerIRMove, registerIRSwap, registerIRBang,
			registerIRSend, registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy,
			registerIRJumpNotNil, registerIRReturn:
			if instruction.op == registerIRSend && (instruction.opcode != compiler.OpSend || instruction.blockPresent) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func registerIRClosureTierEnabled() bool {
	return registerIRClosuresEnabled && registerIRClosureSendsEnabled
}

// registerIRClosureControlFlowSafe limits the first closure tier to blocks
// whose completion is local to the block call.  break/next/redo, non-local
// return, throw and yield all need the caller's unwind protocol; executing
// those from a speculative IR send could leave a pending target after the
// enclosing method frame has already been cleaned up.  Such methods stay on
// the bytecode path until a dedicated unwind-aware tier exists.
func registerIRClosureControlFlowSafe(fn *object.Function) bool {
	if fn == nil {
		return false
	}
	for position := 0; position < len(fn.Instructions); {
		op := compiler.Opcode(fn.Instructions[position])
		switch op {
		case compiler.OpBreak, compiler.OpBreakValue, compiler.OpNext, compiler.OpRedo,
			compiler.OpReturn, compiler.OpThrow, compiler.OpReraise,
			compiler.OpYield, compiler.OpYieldWithValue, compiler.OpYieldWithSplat,
			compiler.OpCatch, compiler.OpEndCatch:
			return false
		case compiler.OpReturnValue:
			// The compiler emits this local-return terminator for the synthetic
			// constant-assignment thunk. It has no user-visible non-local return
			// target; ordinary blocks/Procs remain conservative here.
			if fn.Name != "__scoped_const_rhs__" {
				return false
			}
		}
		definition, ok := compiler.Lookup(byte(op))
		if !ok {
			return false
		}
		width := 1
		for _, operandWidth := range definition.OperandWidths {
			width += operandWidth
		}
		if position+width > len(fn.Instructions) {
			return false
		}
		position += width
	}
	return true
}

// compileRegisterIR converts a supported stack-bytecode method into explicit
// virtual-register operations. Unsupported methods remain on the full Ruby VM.
// registerIRStringLiteralOnlyRaises proves that every control-flow path
// after a string literal reaches OpRaise before returning normally. Such a
// literal is cold by construction (it is an error message in practice), so
// loading it with constantValue cannot affect the successful return path.
// The proof is deliberately conservative: malformed bytecode, loops and
// unsupported terminal operations reject the literal and preserve the full
// interpreter path.
func registerIRStringLiteralOnlyRaises(fn *object.Function, position int) bool {
	if fn == nil || position < 0 || position >= len(fn.Instructions) || compiler.Opcode(fn.Instructions[position]) != compiler.OpConstant {
		return false
	}
	definition, ok := compiler.Lookup(byte(compiler.OpConstant))
	if !ok {
		return false
	}
	next := position + 1
	for _, width := range definition.OperandWidths {
		next += width
	}
	if next > len(fn.Instructions) {
		return false
	}
	state := make(map[int]uint8)
	var reachesRaise func(int) bool
	reachesRaise = func(ip int) bool {
		if ip < 0 || ip >= len(fn.Instructions) {
			return false
		}
		switch state[ip] {
		case 1:
			// A back-edge could execute user code indefinitely; do not treat a
			// cycle as a proof of an exception-only path.
			return false
		case 2:
			return true
		case 3:
			return false
		}
		state[ip] = 1
		op := compiler.Opcode(fn.Instructions[ip])
		if op == compiler.OpRaise {
			state[ip] = 2
			return true
		}
		if op == compiler.OpReturn || op == compiler.OpReturnValue || op == compiler.OpBlockReturn || op == compiler.OpNonLocalReturnValue {
			state[ip] = 3
			return false
		}
		definition, ok := compiler.Lookup(byte(op))
		if !ok {
			state[ip] = 3
			return false
		}
		width := 1
		for _, operandWidth := range definition.OperandWidths {
			width += operandWidth
		}
		if width <= 0 || ip+width > len(fn.Instructions) {
			state[ip] = 3
			return false
		}
		fallthroughIP := ip + width
		result := false
		switch op {
		case compiler.OpJump:
			if len(definition.OperandWidths) != 1 || definition.OperandWidths[0] != 2 {
				state[ip] = 3
				return false
			}
			target := int(fn.Instructions[ip+1])<<8 | int(fn.Instructions[ip+2])
			result = reachesRaise(target)
		case compiler.OpJumpNotTruthy, compiler.OpJumpTruthy, compiler.OpJumpNotNil:
			if len(definition.OperandWidths) != 1 || definition.OperandWidths[0] != 2 {
				state[ip] = 3
				return false
			}
			target := int(fn.Instructions[ip+1])<<8 | int(fn.Instructions[ip+2])
			result = reachesRaise(fallthroughIP) && reachesRaise(target)
		case compiler.OpJumpLocalPresent:
			if len(definition.OperandWidths) != 2 || definition.OperandWidths[1] != 2 {
				state[ip] = 3
				return false
			}
			target := int(fn.Instructions[ip+2])<<8 | int(fn.Instructions[ip+3])
			result = reachesRaise(fallthroughIP) && reachesRaise(target)
		default:
			result = reachesRaise(fallthroughIP)
		}
		if result {
			state[ip] = 2
		} else {
			state[ip] = 3
		}
		return result
	}
	return reachesRaise(next)
}

type registerIRCompileOptions struct {
	allowStringLiterals    bool
	allowClosures          bool
	allowOptionalDefaults  bool
	allowStringEncoding    bool
	allowLogicalAssignment bool
}

func defaultRegisterIRCompileOptions() registerIRCompileOptions {
	return registerIRCompileOptions{
		allowStringLiterals:    registerIRStringLiteralsEnabled,
		allowClosures:          registerIRClosureTierEnabled(),
		allowOptionalDefaults:  registerIROptionalDefaultsEnabled,
		allowStringEncoding:    registerIRStringEncodingEnabled,
		allowLogicalAssignment: registerIRLogicalAssignmentEnabled,
	}
}

func compileRegisterIR(fn *object.Function) (*registerIRPlan, bool) {
	return compileRegisterIRWithOptions(fn, defaultRegisterIRCompileOptions())
}

func compileRegisterIRWithOptions(fn *object.Function, options registerIRCompileOptions) (*registerIRPlan, bool) {
	// Ordinary positional rest parameters are frame-bound below, so they can
	// use the same IR body as fixed-arity methods.  Keyword/block forwarding,
	// anonymous rest and destructuring still need the complete argument
	// protocol and remain on bytecode.  In particular, do not let this wider
	// compiler admission imply that the direct/no-frame tiers may skip binding.
	if fn == nil || fn.HasBlockParam || fn.AnonymousRestParam ||
		len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly {
		return nil, false
	}
	if !options.allowOptionalDefaults {
		for _, value := range fn.ParamDefaults {
			if value != nil {
				return nil, false
			}
		}
	}
	plan := &registerIRPlan{}
	stackDepth := 0
	maxStackDepth := 0
	fallthroughReachable := true
	byteToIR := make(map[int]int)
	incomingDepth := make(map[int]int)
	byteDepth := make(map[int]int)
	whileEndTargets := make([]int, 0, 2)
	implicitFallthroughReturn := false
	// A parameter load can use the argument array only while the parameter is
	// immutable.  Once a parameter is assigned, later OpGetLocalFast bytecode
	// must observe the frame slot (and any captured cell), not the original
	// argument.  Pre-scan assignments so branches and loop back-edges remain
	// conservative and correct.
	assignedLocals := make(map[byte]bool)
	for scanPosition := 0; scanPosition < len(fn.Instructions); {
		scanOp := compiler.Opcode(fn.Instructions[scanPosition])
		if scanOp == compiler.OpSetLocal && scanPosition+1 < len(fn.Instructions) {
			assignedLocals[fn.Instructions[scanPosition+1]] = true
		}
		definition, ok := compiler.Lookup(byte(scanOp))
		if !ok {
			scanPosition++
			continue
		}
		scanWidth := 1
		for _, operandWidth := range definition.OperandWidths {
			scanWidth += operandWidth
		}
		if scanPosition+scanWidth > len(fn.Instructions) {
			return nil, false
		}
		scanPosition += scanWidth
	}
	pushSlot := func() (uint8, bool) {
		if stackDepth >= 16 {
			return 0, false
		}
		result := uint8(stackDepth)
		stackDepth++
		if stackDepth > maxStackDepth {
			maxStackDepth = stackDepth
		}
		return result, true
	}
	recordIncomingDepth := func(target, depth int) bool {
		if existing, ok := incomingDepth[target]; ok {
			return existing == depth
		}
		incomingDepth[target] = depth
		return true
	}
	paramForLocal := func(local int) (uint8, bool) {
		for index, candidate := range fn.ParamLocalIndices {
			if candidate == local && index < 256 {
				return uint8(index), true
			}
		}
		return 0, false
	}

	instructions := fn.Instructions
	for position := 0; position < len(instructions); {
		for len(whileEndTargets) > 0 && whileEndTargets[len(whileEndTargets)-1] <= position {
			whileEndTargets = whileEndTargets[:len(whileEndTargets)-1]
		}
		byteToIR[position] = len(plan.instructions)
		if expected, ok := incomingDepth[position]; ok {
			if !fallthroughReachable {
				stackDepth = expected
				fallthroughReachable = true
			} else if expected != stackDepth {
				return nil, false
			}
		}
		if !fallthroughReachable {
			op := compiler.Opcode(instructions[position])
			definition, ok := compiler.Lookup(byte(op))
			if !ok {
				position++
				continue
			}
			// The compiler emits a trailing ReturnValue after a Raise so the
			// framed IR plan still has a normal terminal instruction. It is
			// unreachable in bytecode, but unlike ordinary dead jump scaffolding
			// it is part of the plan shape and must be lowered.
			if op == compiler.OpReturnValue || op == compiler.OpBlockReturn {
				fallthroughReachable = true
			} else {
				width := 1
				for _, operandWidth := range definition.OperandWidths {
					width += operandWidth
				}
				if position+width > len(instructions) {
					return nil, false
				}
				position += width
				continue
			}
		}
		byteDepth[position] = stackDepth
		op := compiler.Opcode(instructions[position])
		switch op {
		case compiler.OpGetLocal, compiler.OpGetLocalFast:
			if position+1 >= len(instructions) {
				return nil, false
			}
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			local := int(instructions[position+1])
			if param, isParam := paramForLocal(local); isParam && !assignedLocals[byte(local)] {
				plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadParam, dst: dst, param: param})
			} else {
				plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadLocal, dst: dst, param: uint8(local)})
				plan.requiresFrame = true
			}
			position += 2
		case compiler.OpTrue, compiler.OpFalse, compiler.OpNil:
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			value := core.R.NilVal
			if op == compiler.OpTrue {
				value = core.R.TrueVal
			} else if op == compiler.OpFalse {
				value = core.R.FalseVal
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadLiteral, dst: dst, value: value})
			position++
		case compiler.OpConstant:
			if position+2 >= len(instructions) {
				return nil, false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
				return nil, false
			}
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			constant := fn.Constants[index]
			if registerIRImmutableLiteral(constant) {
				plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadLiteral, dst: dst, value: constant})
			} else if registerIRFrozenStringLiteralsEnabled && registerIRFrozenSourceStringLiteralsEnabled && constant.Type == object.ValueString && fn.StringLiteralModeSet && fn.FreezeStringLiterals {
				// A file-level frozen-string directive makes every literal in this
				// function immutable, even though the compiler constant itself is
				// still a template shared by constantValue.  Materialize it once
				// through the VM's frozen-string intern table, then rewrite this
				// instruction to a direct literal load for subsequent calls.  This
				// keeps cross-file identity and encoding semantics while avoiding
				// the per-call mutable-string allocation tier.
				plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadFrozenString, dst: dst, value: constant})
				plan.hasFrozenStrings = true
				plan.requiresFrame = true
			} else if constant.Type == object.ValueString && registerIRColdRaiseStringLiteralsEnabled && registerIRStringLiteralOnlyRaises(fn, position) {
				// Cold error strings are materialized at the point where the
				// interpreter would execute OpConstant. The defining Frame is still
				// available, so mutable/frozen/chilled source modes remain intact.
				plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadConstantValue, dst: dst, value: constant})
				plan.requiresFrame = true
				plan.hasSends = true
				plan.coldRaiseStringLiterals = true
			} else if (options.allowStringLiterals || (registerIRBlockStringLiteralsEnabled && fn.Name == "__block__")) && constant.Type == object.ValueString {
				// String literals must still pass through constantValue so frozen,
				// chilled and mutable source modes keep their allocation/encoding
				// semantics. This is a framed safepoint, unlike immediate literals
				// which can be shared directly.
				plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadConstantValue, dst: dst, value: constant})
				plan.requiresFrame = true
				plan.hasSends = true
			} else {
				return nil, false
			}
			position += 3
		case compiler.OpGetConstant:
			if !registerIRConstantsEnabled {
				return nil, false
			}
			if position+2 >= len(instructions) {
				return nil, false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
				return nil, false
			}
			name, ok := fn.Constants[index].Data.(string)
			if !ok || name == "" {
				return nil, false
			}
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadConstant, dst: dst, name: name, byteIP: position})
			plan.requiresFrame = true
			position += 3
		case compiler.OpGetScopedConstant:
			if !registerIRScopedConstantsEnabled {
				return nil, false
			}
			if position+2 >= len(instructions) || stackDepth < 1 {
				return nil, false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
				return nil, false
			}
			name, ok := fn.Constants[index].Data.(string)
			if !ok || name == "" {
				return nil, false
			}
			receiver := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadScopedConstant, dst: receiver, left: receiver, name: name, byteIP: position})
			plan.requiresFrame = true
			position += 3
		case compiler.OpGetLocalCell, compiler.OpGetOuterCell, compiler.OpGetFreeCell, compiler.OpGetOuterFreeCell:
			if !options.allowClosures || position+1 >= len(instructions) {
				return nil, false
			}
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			captureKind := registerIRCaptureLocal
			switch op {
			case compiler.OpGetOuterCell:
				captureKind = registerIRCaptureOuter
			case compiler.OpGetFreeCell:
				captureKind = registerIRCaptureFree
			case compiler.OpGetOuterFreeCell:
				captureKind = registerIRCaptureOuterFree
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRLoadCapture, dst: dst, param: instructions[position+1], captureKind: captureKind,
				byteIP: position,
			})
			plan.requiresFrame = true
			// Materializing a local cell mutates the live frame.  Treat it as a
			// safepoint so a later guard never replays the capture from entry.
			plan.hasSends = true
			position += 2
		case compiler.OpClosure:
			if !options.allowClosures || position+3 >= len(instructions) {
				return nil, false
			}
			fnIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
			numFree := int(instructions[position+3])
			if fnIndex < 0 || fnIndex >= len(fn.Constants) || numFree < 0 || numFree > 15 || stackDepth < numFree {
				return nil, false
			}
			constant := fn.Constants[fnIndex]
			if constant == nil || constant.Type != object.ValueFunction {
				return nil, false
			}
			closureFn, ok := constant.Data.(*object.Function)
			if !ok || closureFn == nil || !registerIRClosureControlFlowSafe(closureFn) {
				return nil, false
			}
			dst := stackDepth - numFree
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRClosure, dst: uint8(dst), argc: uint8(numFree), fn: closureFn, byteIP: position,
			})
			stackDepth = dst + 1
			plan.requiresFrame = true
			plan.hasSends = true
			position += 4
		case compiler.OpGetInstanceVar:
			if position+2 >= len(instructions) {
				return nil, false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
				return nil, false
			}
			name, ok := fn.Constants[index].Data.(string)
			if !ok {
				return nil, false
			}
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadInstanceVar, dst: dst, name: name})
			position += 3
		case compiler.OpDefinedInstanceVar:
			if !registerIRDefinedInstanceVarEnabled || position+2 >= len(instructions) {
				return nil, false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
				return nil, false
			}
			name, ok := fn.Constants[index].Data.(string)
			if !ok || name == "" {
				return nil, false
			}
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRDefinedInstanceVar, dst: dst, name: name, byteIP: position})
			// Keep the operation framed: DynamicInstanceVar has receiver-specific
			// storage rules, and the result is a newly allocated frozen String on
			// the defined branch.  This also keeps later deoptimization from
			// replaying the allocation outside a caller frame.
			plan.requiresFrame = true
			plan.hasSends = true
			position += 3
		case compiler.OpSelf:
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadSelf, dst: dst})
			position++
		case compiler.OpGetFree:
			if position+1 >= len(instructions) {
				return nil, false
			}
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRLoadFree, dst: dst, param: instructions[position+1]})
			plan.requiresFrame = true
			position += 2
		case compiler.OpDup:
			if stackDepth < 1 {
				return nil, false
			}
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRMove, dst: dst, left: uint8(stackDepth - 2)})
			position++
		case compiler.OpSwap:
			if stackDepth < 2 {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRSwap, left: uint8(stackDepth - 2), right: uint8(stackDepth - 1)})
			position++
		case compiler.OpBang:
			if stackDepth < 1 {
				return nil, false
			}
			top := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRBang, dst: top, left: top})
			position++
		case compiler.OpNeg, compiler.OpNegate:
			if stackDepth < 1 {
				return nil, false
			}
			top := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRNeg, dst: top, left: top, byteIP: position})
			plan.requiresFrame = true
			plan.hasSends = true
			position++
		case compiler.OpPop:
			if stackDepth < 1 {
				return nil, false
			}
			stackDepth--
			position++
		case compiler.OpSetLocal:
			if position+1 >= len(instructions) || stackDepth < 1 {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRStoreLocal, left: uint8(stackDepth - 1), param: instructions[position+1]})
			plan.requiresFrame = true
			// A local assignment can update a captured cell. Do not allow a later
			// speculative guard to deopt and replay that write from the method
			// entry.
			plan.hasSends = true
			position += 2
		case compiler.OpSetFree:
			if position+1 >= len(instructions) || stackDepth < 1 {
				return nil, false
			}
			// Free-slot writes are observable through the live closure. They are
			// valid in the framed executor, but never in a speculative no-frame
			// tier because a later guard could otherwise replay the write.
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRStoreFree, left: uint8(stackDepth - 1), param: instructions[position+1]})
			plan.requiresFrame = true
			plan.hasSends = true
			position += 2
		case compiler.OpSetInstanceVar:
			if position+2 >= len(instructions) || stackDepth < 1 {
				return nil, false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
				return nil, false
			}
			name, ok := fn.Constants[index].Data.(string)
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRStoreInstanceVar, left: uint8(stackDepth - 1), name: name, byteIP: position})
			plan.hasSends = true
			plan.requiresFrame = true
			position += 3
		case compiler.OpSetStringEncoding:
			if !options.allowStringEncoding {
				return nil, false
			}
			if position+2 >= len(instructions) || stackDepth < 1 {
				return nil, false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(fn.Constants) || fn.Constants[index] == nil {
				return nil, false
			}
			encoding, ok := fn.Constants[index].Data.(string)
			if !ok {
				return nil, false
			}
			if encoding == "" {
				// The bytecode emitter uses an empty encoding marker for the
				// ASCII-compatible interpolation path. Runtime execution leaves
				// the value untouched in that case, so the stack-preserving no-op
				// need not widen the Register IR plan.
				position += 3
				continue
			}
			top := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRSetStringEncoding, dst: top, left: top, name: encoding, byteIP: position})
			plan.hasSends = true
			plan.requiresFrame = true
			position += 3
		case compiler.OpArray:
			if position+4 >= len(instructions) {
				return nil, false
			}
			elements := int(instructions[position+1])<<24 | int(instructions[position+2])<<16 |
				int(instructions[position+3])<<8 | int(instructions[position+4])
			if elements < 0 || elements > 16 || stackDepth < elements {
				return nil, false
			}
			dst := stackDepth - elements
			if stackDepth == 16 && elements == 0 {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRArray, dst: uint8(dst), argc: uint8(elements)})
			stackDepth = dst + 1
			if stackDepth > maxStackDepth {
				maxStackDepth = stackDepth
			}
			plan.requiresFrame = true
			position += 5
		case compiler.OpSplatToArray:
			if stackDepth < 1 {
				return nil, false
			}
			top := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRSplatToArray, dst: top, left: top, byteIP: position})
			// `to_a` coercion can dispatch Ruby code and raise, so this operation
			// remains framed. The normal executor preserves the original
			// OpSplatToArray allocation/error protocol.
			plan.requiresFrame = true
			plan.hasSends = true
			position++
		case compiler.OpHash:
			if position+2 >= len(instructions) {
				return nil, false
			}
			pairs := int(instructions[position+1])<<8 | int(instructions[position+2])
			if pairs < 0 || pairs > 8 || stackDepth < pairs*2 {
				return nil, false
			}
			dst := stackDepth - pairs*2
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRHash, dst: uint8(dst), argc: uint8(pairs)})
			stackDepth = dst + 1
			if stackDepth > maxStackDepth {
				maxStackDepth = stackDepth
			}
			plan.requiresFrame = true
			position += 3
		case compiler.OpHashMerge:
			if stackDepth < 2 {
				return nil, false
			}
			target := uint8(stackDepth - 2)
			source := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRHashMerge, dst: target, left: target, right: source, byteIP: position,
			})
			stackDepth--
			// Hash merging may invoke to_hash and always mutates the target hash.
			// Keep it framed so errors and user-defined conversions use the normal
			// Ruby rescue/backtrace protocol.
			plan.requiresFrame = true
			plan.hasSends = true
			position++
		case compiler.OpMultiAssignPrepare:
			if stackDepth < 1 {
				return nil, false
			}
			top := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRMultiAssignPrepare, dst: top, left: top, byteIP: position,
			})
			// RHS preparation may call user-defined `to_ary` and may raise. It
			// therefore stays in the framed executor; the operation itself is
			// completed before the next IR instruction, so a later plan guard
			// never needs to replay it.
			plan.requiresFrame = true
			plan.hasSends = true
			position++
		case compiler.OpMultiAssignExtract:
			if position+4 >= len(instructions) || stackDepth < 1 {
				return nil, false
			}
			kind := instructions[position+1]
			index := instructions[position+2]
			preCount := instructions[position+3]
			postCount := instructions[position+4]
			top := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRMultiAssignExtract, dst: top, left: top,
				param: kind, argc: index, args: [4]uint8{preCount, postCount}, byteIP: position,
			})
			// Splat extraction allocates a fresh Array for the `*rest` form.
			// Keep all extraction forms framed so allocation and exception
			// backtraces retain the bytecode-visible method frame.
			plan.requiresFrame = true
			plan.hasSends = true
			position += 5
		case compiler.OpMultiAssignCheckToAry:
			if stackDepth < 1 {
				return nil, false
			}
			top := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRMultiAssignCheckToAry, dst: top, left: top, byteIP: position,
			})
			plan.requiresFrame = true
			plan.hasSends = true
			position++
		case compiler.OpMarkKeywordHash:
			if stackDepth < 1 {
				return nil, false
			}
			top := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRMarkKeywordHash, dst: top, left: top, byteIP: position,
			})
			plan.requiresFrame = true
			position++
		case compiler.OpEqual:
			if stackDepth < 2 {
				return nil, false
			}
			left := uint8(stackDepth - 2)
			right := uint8(stackDepth - 1)
			equalityOp := registerIREqual
			if plan.hasSends && !registerIRIntegerOnlyPrefix(plan) {
				// Equality after a send may dispatch to user Ruby code or a
				// container implementation. Keep it in the framed executor so a
				// dynamic receiver cannot invalidate an already-observed prefix.
				equalityOp = registerIRDynamicEqual
				plan.requiresFrame = true
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: equalityOp, dst: left, left: left, right: right, byteIP: position,
			})
			stackDepth--
			position++
		case compiler.OpNotEqual:
			if stackDepth < 2 {
				return nil, false
			}
			left := uint8(stackDepth - 2)
			right := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRNotEqual, dst: left, left: left, right: right, byteIP: position})
			stackDepth--
			plan.requiresFrame = true
			plan.hasSends = true
			position++
		case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod, compiler.OpBitLeftShift:
			if op == compiler.OpBitLeftShift && !registerIRBitShiftEnabled {
				return nil, false
			}
			if !registerIRArithmeticEnabled {
				return nil, false
			}
			if stackDepth < 2 {
				return nil, false
			}
			left := uint8(stackDepth - 2)
			right := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRBinary, opcode: op, dst: left, left: left, right: right, byteIP: position})
			stackDepth--
			// Ruby arithmetic can dispatch an overridden method and therefore is
			// both a safepoint and a possible side effect.
			plan.hasSends = true
			position++
		case compiler.OpRange:
			if position+3 >= len(instructions) || stackDepth < 2 {
				return nil, false
			}
			exclusive := instructions[position+1]
			startMissing := instructions[position+2]
			endMissing := instructions[position+3]
			if exclusive > 1 || startMissing > 1 || endMissing > 1 {
				return nil, false
			}
			left := uint8(stackDepth - 2)
			right := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRRange, dst: left, left: left, right: right,
				param: exclusive | startMissing<<1 | endMissing<<2, byteIP: position,
			})
			stackDepth--
			// Range construction allocates a Ruby object and carries endpoint
			// values into later sends; keep it in the framed tier so a failed
			// dynamic guard never replays an escaped endpoint.
			plan.requiresFrame = true
			plan.hasSends = true
			position += 4
		case compiler.OpBlockGiven:
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRBlockGiven, dst: dst, byteIP: position})
			plan.requiresFrame = true
			position++
		case compiler.OpGreaterThan, compiler.OpGreaterThanOrEqual, compiler.OpLessThan, compiler.OpLessThanOrEqual:
			if !registerIRComparisonEnabled || stackDepth < 2 {
				return nil, false
			}
			left := uint8(stackDepth - 2)
			right := uint8(stackDepth - 1)
			compareOp := registerIRCompare
			if plan.hasSends && !registerIRIntegerOnlyPrefix(plan) {
				// Once an earlier send has run, an integer guard miss cannot replay
				// the method from its entry.  Use the framed VM comparison helpers
				// instead; they retain custom <=>/comparison semantics in place.
				compareOp = registerIRDynamicCompare
				plan.requiresFrame = true
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: compareOp, opcode: op, dst: left, left: left, right: right, byteIP: position})
			stackDepth--
			position++
		case compiler.OpSend, compiler.OpSendWithKeywords, compiler.OpSendSetter:
			if position+5 >= len(instructions) {
				return nil, false
			}
			methodIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
			blockArg := instructions[position+3]
			numArgs := int(instructions[position+4])
			splatIndex := instructions[position+5]
			if methodIndex < 0 || methodIndex >= len(fn.Constants) || fn.Constants[methodIndex] == nil ||
				(blockArg != 0 && blockArg != 1 && blockArg != 3) || numArgs > 4 ||
				(splatIndex != 255 && int(splatIndex) >= numArgs) ||
				(blockArg == 1 && !options.allowClosures) || stackDepth < numArgs+1 ||
				(blockArg == 1 && stackDepth < numArgs+2) {
				return nil, false
			}
			methodName, ok := fn.Constants[methodIndex].Data.(string)
			if !ok || !registerIRSafePlainSendName(methodName) {
				return nil, false
			}
			receiverSlot := stackDepth - numArgs - 1
			if blockArg == 1 {
				receiverSlot--
			}
			instruction := registerIRInstruction{op: registerIRSend, dst: uint8(receiverSlot), left: uint8(receiverSlot), argc: uint8(numArgs), name: methodName, byteIP: position, opcode: op, cache: &registerIRSendCache{}, splatIndex: 255, implicit: blockArg == 3}
			if splatIndex != 255 {
				instruction.splatIndex = splatIndex
				plan.requiresFrame = true
			}
			for index := 0; index < numArgs; index++ {
				instruction.args[index] = uint8(receiverSlot + 1 + index)
			}
			plan.instructions = append(plan.instructions, instruction)
			if blockArg == 1 {
				instruction.block = uint8(stackDepth - 1)
				instruction.blockPresent = true
				plan.instructions[len(plan.instructions)-1] = instruction
				stackDepth -= numArgs + 1
				plan.requiresFrame = true
			} else {
				stackDepth -= numArgs
			}
			plan.hasSends = true
			if blockArg == 3 {
				plan.hasImplicitSends = true
			}
			if plan.sendCount < 255 {
				plan.sendCount++
			}
			position += 6
		case compiler.OpYield:
			dst, ok := pushSlot()
			if !ok {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRYield, dst: dst, argc: 0, splatIndex: 255, byteIP: position,
			})
			plan.requiresFrame = true
			plan.hasSends = true
			plan.hasImplicitSends = true
			position++
		case compiler.OpYieldWithValue:
			if position+1 >= len(instructions) {
				return nil, false
			}
			numArgs := int(instructions[position+1])
			if numArgs > 4 || stackDepth < numArgs {
				return nil, false
			}
			dst := uint8(stackDepth - numArgs)
			instruction := registerIRInstruction{op: registerIRYield, dst: dst, argc: uint8(numArgs), splatIndex: 255, byteIP: position}
			for index := 0; index < numArgs; index++ {
				instruction.args[index] = dst + uint8(index)
			}
			plan.instructions = append(plan.instructions, instruction)
			stackDepth = int(dst) + 1
			plan.requiresFrame = true
			plan.hasSends = true
			plan.hasImplicitSends = true
			position += 2
		case compiler.OpYieldWithSplat:
			if position+2 >= len(instructions) {
				return nil, false
			}
			numArgs := int(instructions[position+1])
			splatIndex := instructions[position+2]
			if numArgs > 4 || int(splatIndex) >= numArgs || stackDepth < numArgs {
				return nil, false
			}
			dst := uint8(stackDepth - numArgs)
			instruction := registerIRInstruction{op: registerIRYield, dst: dst, argc: uint8(numArgs), splatIndex: splatIndex, byteIP: position}
			for index := 0; index < numArgs; index++ {
				instruction.args[index] = dst + uint8(index)
			}
			plan.instructions = append(plan.instructions, instruction)
			stackDepth = int(dst) + 1
			plan.requiresFrame = true
			plan.hasSends = true
			plan.hasImplicitSends = true
			position += 3
		case compiler.OpLogicalSendAssignment:
			if !options.allowClosures || !options.allowLogicalAssignment || position+6 >= len(instructions) || stackDepth < 2 {
				return nil, false
			}
			getterIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
			setterIndex := int(instructions[position+3])<<8 | int(instructions[position+4])
			argc := int(instructions[position+5])
			mode := instructions[position+6]
			if argc > 4 || stackDepth < argc+2 || getterIndex < 0 || getterIndex >= len(fn.Constants) ||
				setterIndex < 0 || setterIndex >= len(fn.Constants) || fn.Constants[getterIndex] == nil || fn.Constants[setterIndex] == nil {
				return nil, false
			}
			getter, getterOK := fn.Constants[getterIndex].Data.(string)
			setter, setterOK := fn.Constants[setterIndex].Data.(string)
			if !getterOK || !setterOK || getter == "" || setter == "" {
				return nil, false
			}
			receiverSlot := stackDepth - argc - 2
			instruction := registerIRInstruction{
				op: registerIRLogicalSendAssignment, dst: uint8(receiverSlot), left: uint8(receiverSlot),
				argc: uint8(argc), block: uint8(stackDepth - 1), blockPresent: true,
				name: getter, setter: setter, param: mode, byteIP: position,
			}
			for index := 0; index < argc; index++ {
				instruction.args[index] = uint8(receiverSlot + 1 + index)
			}
			plan.instructions = append(plan.instructions, instruction)
			stackDepth -= argc + 1
			plan.hasSends = true
			plan.requiresFrame = true
			position += 7
		case compiler.OpIndex:
			if stackDepth < 2 {
				return nil, false
			}
			left := uint8(stackDepth - 2)
			right := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRIndex, dst: left, left: left, right: right, byteIP: position})
			stackDepth--
			// Exact built-in Array/Hash indexing can run without a frame. If the
			// receiver is a subclass or otherwise needs dynamic [] dispatch, the
			// direct guard fails before any user code runs and the whole method
			// falls back to the normal VM. A prior side effect still forces a
			// framed plan so deoptimization cannot replay it.
			if plan.hasSends || plan.requiresFrame {
				plan.requiresFrame = true
			}
			position++
		case compiler.OpSliceIndex:
			if !registerIRSliceEnabled || stackDepth < 3 {
				return nil, false
			}
			left := uint8(stackDepth - 3)
			start := uint8(stackDepth - 2)
			length := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRSlice, dst: left, left: left, right: start, args: [4]uint8{length}, byteIP: position,
			})
			stackDepth -= 2
			// String/subclass slices can dispatch through user Ruby code, so this
			// remains a framed safepoint even though built-in Array slices are
			// handled directly by the shared primitive.
			plan.hasSends = true
			plan.requiresFrame = true
			position++
		case compiler.OpIndexAssign:
			if !registerIRIndexAssignEnabled {
				return nil, false
			}
			if stackDepth < 3 {
				return nil, false
			}
			left := uint8(stackDepth - 3)
			index := uint8(stackDepth - 2)
			value := uint8(stackDepth - 1)
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRIndexAssign, dst: left, left: left, right: index, args: [4]uint8{value}, byteIP: position})
			stackDepth -= 2
			plan.hasSends = true
			plan.requiresFrame = true
			position++
		case compiler.OpSetWhileEnd:
			if position+2 >= len(instructions) {
				return nil, false
			}
			target := int(instructions[position+1])<<8 | int(instructions[position+2])
			if target <= position || target > len(instructions) {
				return nil, false
			}
			// The bytecode executor stores this target in Frame.WhileEnd so a
			// following OpBreak can route to the end of the current while loop.
			// Register IR keeps the real Frame, but lowers the control transfer
			// itself to a normal IR jump; no mutable WhileEnd side state is needed
			// while the plan is executing.
			whileEndTargets = append(whileEndTargets, target)
			plan.requiresFrame = true
			position += 3
		case compiler.OpBreak:
			if len(whileEndTargets) == 0 {
				// A break outside a local while loop targets the enclosing block
				// owner and needs the full non-local unwind protocol.
				return nil, false
			}
			target := whileEndTargets[len(whileEndTargets)-1]
			if !recordIncomingDepth(target, stackDepth) {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRJump, target: target})
			plan.hasBranches = true
			plan.requiresFrame = true
			fallthroughReachable = false
			position++
		case compiler.OpBreakValue:
			// A value-bearing break has a separate target/stack protocol. Keep it
			// on bytecode until the IR executor models the loop result explicitly.
			return nil, false
		case compiler.OpNext:
			if position+2 >= len(instructions) || stackDepth < 1 {
				return nil, false
			}
			target := int(instructions[position+1])<<8 | int(instructions[position+2])
			// A zero target is the foreach/block protocol, not a local while
			// back-edge. It must retain BlockNextVal and caller unwind semantics.
			if target <= 0 || target >= len(instructions) {
				return nil, false
			}
			// Bytecode OpNext pops the current value and pushes the next value
			// again, so its unreachable linear successor keeps the same depth.
			// The local while edge, however, starts at the loop header after that
			// value has been discarded. Keep both depths separate while compiling
			// the remaining unreachable bytecode after OpNext.
			nextDepth := stackDepth - 1
			if target < position {
				expected, ok := byteDepth[target]
				if !ok || expected != nextDepth {
					return nil, false
				}
			} else if !recordIncomingDepth(target, nextDepth) {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRJump, target: target})
			plan.hasBranches = true
			plan.requiresFrame = true
			fallthroughReachable = false
			position += 3
		case compiler.OpJumpLocalPresent:
			if position+3 >= len(instructions) {
				return nil, false
			}
			target := int(instructions[position+2])<<8 | int(instructions[position+3])
			if target <= position || target >= len(instructions) || !recordIncomingDepth(target, stackDepth) {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRJumpLocalPresent, param: instructions[position+1], target: target})
			plan.hasBranches = true
			plan.requiresFrame = true
			position += 4
		case compiler.OpJump, compiler.OpJumpTruthy, compiler.OpJumpNotTruthy, compiler.OpJumpNotNil:
			if position+2 >= len(instructions) || (op != compiler.OpJump && stackDepth < 1) {
				return nil, false
			}
			target := int(instructions[position+1])<<8 | int(instructions[position+2])
			if target < 0 || target > len(instructions) {
				return nil, false
			}
			if op != compiler.OpJump {
				stackDepth--
			}
			if target < position {
				expected, ok := byteDepth[target]
				if !ok || expected != stackDepth {
					return nil, false
				}
			} else if target == len(instructions) {
				// A block may leave its last expression on the stack and jump
				// past the bytecode terminator. Preserve that implicit block
				// result as an explicit IR return edge. The return instruction is
				// appended after the normal bytecode pass so this jump lands on it
				// instead of skipping over it.
				if stackDepth != 1 {
					return nil, false
				}
				implicitFallthroughReturn = true
			} else if !recordIncomingDepth(target, stackDepth) {
				return nil, false
			}
			plan.hasBranches = true
			irOp := registerIRJump
			if op == compiler.OpJumpTruthy {
				irOp = registerIRJumpTruthy
			} else if op == compiler.OpJumpNotTruthy {
				irOp = registerIRJumpNotTruthy
			} else if op == compiler.OpJumpNotNil {
				irOp = registerIRJumpNotNil
			}
			instruction := registerIRInstruction{op: irOp, target: target}
			if op != compiler.OpJump {
				instruction.left = uint8(stackDepth)
			}
			plan.instructions = append(plan.instructions, instruction)
			if op == compiler.OpJump {
				fallthroughReachable = false
			}
			position += 3
		case compiler.OpReturnValue, compiler.OpBlockReturn:
			if stackDepth != 1 {
				return nil, false
			}
			if op == compiler.OpBlockReturn && position != len(instructions)-1 {
				return nil, false
			}
			plan.blockReturn = op == compiler.OpBlockReturn
			if op == compiler.OpReturnValue {
				plan.hasExplicitReturn = true
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{
				op: registerIRReturn, left: 0, explicitReturn: op == compiler.OpReturnValue,
			})
			// A Ruby method may return from the middle of a conditional.  The
			// following linear bytecode is unreachable unless a later branch target
			// explicitly re-enters it; leave stack-depth reconciliation to the
			// incoming-target check above.
			fallthroughReachable = false
			position++
		case compiler.OpRaise:
			if !registerIRRaiseEnabled || stackDepth < 1 {
				return nil, false
			}
			plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRRaise, left: uint8(stackDepth - 1), byteIP: position})
			// OpRaise never resumes at the following bytecode instruction.  The
			// compiler still emits a trailing ReturnValue for the method body, so
			// keep a single synthetic result slot for that unreachable terminator
			// instead of rejecting an otherwise valid plan at ReturnValue.
			stackDepth = 1
			plan.hasSends = true
			plan.requiresFrame = true
			fallthroughReachable = false
			position++
		default:
			return nil, false
		}
	}
	if len(plan.instructions) == 0 || plan.instructions[len(plan.instructions)-1].op != registerIRReturn {
		return nil, false
	}
	if implicitFallthroughReturn {
		byteToIR[len(instructions)] = len(plan.instructions)
		plan.instructions = append(plan.instructions, registerIRInstruction{op: registerIRReturn, left: 0})
	} else {
		byteToIR[len(instructions)] = len(plan.instructions)
	}
	for index := range plan.instructions {
		instruction := &plan.instructions[index]
		switch instruction.op {
		case registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy, registerIRJumpNotNil, registerIRJumpLocalPresent:
			target, ok := byteToIR[instruction.target]
			if !ok {
				return nil, false
			}
			instruction.target = target
		}
	}
	plan.registers = uint8(maxStackDepth)
	for _, instruction := range plan.instructions {
		if instruction.op == registerIRLoadConstant || instruction.op == registerIRLoadScopedConstant {
			plan.hasConstantLoads = true
			break
		}
	}
	plan.integerOnly = registerIRIntegerOnlyPlan(plan)
	plan.integerLinear = registerIRIntegerLinearPlan(plan)
	if plan.integerLinear {
		plan.integerLinearKind, plan.integerLinearOpA, plan.integerLinearOpB, plan.integerLinearConstA, plan.integerLinearConstB = registerIRIntegerLinearShape(plan)
	}
	plan.directFastKind = registerIRDirectFastShape(plan)
	plan.directNoFrameChecked = true
	plan.directNoFrameSafe = registerIRPlanSafeForDirectNoFrameUncached(plan)
	plan.directNoFrameBlockChecked = true
	plan.directNoFrameBlockSafe = registerIRPlanSafeForDirectNoFrameWithOptions(plan, true, true)
	plan.noFrameInlineChecked = true
	plan.noFrameInlineSafe = registerIRPlanSafeForNoFrameInlineUncached(plan)
	plan.mayDeoptChecked = true
	plan.mayDeopt = registerIRPlanMayDeoptUncached(plan)
	plan.framedBlockChecked = true
	plan.framedBlockSafe = registerIRPlanSafeForFramedBlockUncached(plan)
	plan.framedBlockReturnChecked = true
	plan.framedBlockReturnSafe = registerIRPlanSafeForFramedBlockReturnUncached(plan)
	plan.framelessBlockChecked = true
	plan.framelessBlockSafe = registerIRPlanSafeForFramelessBlockUncached(plan)
	plan.branchNoFrameBlockChecked = true
	plan.branchNoFrameBlockSafe = registerIRPlanSafeForBranchNoFrameBlockUncached(plan)
	return plan, true
}

func registerIRIntegerOnlyPlan(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	hasIntegerOperation := false
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadInstanceVar, registerIRMove, registerIRStoreLocal, registerIRReturn,
			registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy, registerIRJumpNotNil:
		case registerIREqual, registerIRBinary, registerIRCompare:
			hasIntegerOperation = true
		default:
			return false
		}
	}
	return hasIntegerOperation
}

// registerIRIntegerOnlyCanUseFastArgs keeps scalar plans on their unboxed
// executor only while the call arguments are immediate integers.  OpAdd and
// OpBitLeftShift are also Ruby String#+/String#<<, so a method such as
// `def append(a, b); a << b; end` must enter the framed Register IR path for
// string arguments instead of falling all the way back to bytecode.
func registerIRIntegerOnlyCanUseFastArgs(plan *registerIRPlan, args []*object.EmeraldValue) bool {
	if plan == nil || !plan.integerOnly {
		return false
	}
	dynamicStringOp := false
	for _, instruction := range plan.instructions {
		if instruction.op == registerIRBinary && (instruction.opcode == compiler.OpAdd || instruction.opcode == compiler.OpBitLeftShift) {
			dynamicStringOp = true
			break
		}
	}
	if !dynamicStringOp {
		return true
	}
	for _, arg := range args {
		if !smallIntegerValue(arg) {
			return false
		}
	}
	return true
}

// registerIRIntegerLinearPlan identifies the straight-line arithmetic subset
// used by the tightest one-argument block loops.  A block parameter is
// normally represented as a LoadLocal rather than a LoadParam after Ruby's
// binder has assigned its local slot, so the shape admits one input local and
// leaves the caller to verify that it is the actual parameter slot.  Other
// locals, instance variables, comparisons, and branches stay out so the
// executor can operate on raw int64 registers without Ruby-value kind checks.
func registerIRIntegerLinearPlan(plan *registerIRPlan) bool {
	if plan == nil || !plan.integerOnly || plan.hasBranches {
		return false
	}
	hasBinary := false
	inputLocals := 0
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param != 0 {
				return false
			}
		case registerIRLoadLocal:
			inputLocals++
			if inputLocals > 1 {
				return false
			}
		case registerIRLoadLiteral:
			if instruction.value == nil || instruction.value.Type != object.ValueInteger || !smallIntegerValue(instruction.value) {
				return false
			}
		case registerIRBinary:
			switch instruction.opcode {
			case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod:
				hasBinary = true
			default:
				return false
			}
		case registerIRReturn:
		default:
			return false
		}
	}
	return hasBinary
}

func registerIRIntegerLinearShape(plan *registerIRPlan) (uint8, compiler.Opcode, compiler.Opcode, int64, int64) {
	if plan == nil || !plan.integerLinear {
		return 0, 0, 0, 0, 0
	}
	instructions := plan.instructions
	loadInput := func(instruction registerIRInstruction) (uint8, bool) {
		if instruction.op == registerIRLoadParam && instruction.param == 0 {
			return instruction.dst, true
		}
		if instruction.op == registerIRLoadLocal {
			return instruction.dst, true
		}
		return 0, false
	}
	loadLiteral := func(instruction registerIRInstruction) (uint8, int64, bool) {
		if instruction.op != registerIRLoadLiteral || instruction.value == nil || instruction.value.Type != object.ValueInteger || !smallIntegerValue(instruction.value) {
			return 0, 0, false
		}
		return instruction.dst, instruction.value.Data.(int64), true
	}
	validBinary := func(instruction registerIRInstruction, result, left, right uint8) bool {
		if instruction.op != registerIRBinary || instruction.dst != result || instruction.left != left || instruction.right != right {
			return false
		}
		switch instruction.opcode {
		case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod:
			return true
		default:
			return false
		}
	}
	if len(instructions) == 4 {
		param, paramOK := loadInput(instructions[0])
		literal, constant, literalOK := loadLiteral(instructions[1])
		binary := instructions[2]
		if paramOK && literalOK && validBinary(binary, param, param, literal) && instructions[3].op == registerIRReturn && instructions[3].left == param {
			return 1, binary.opcode, 0, constant, 0
		}
	}
	if len(instructions) == 6 {
		param, paramOK := loadInput(instructions[0])
		literalA, constantA, literalAOK := loadLiteral(instructions[1])
		binaryA := instructions[2]
		literalB, constantB, literalBOK := loadLiteral(instructions[3])
		binaryB := instructions[4]
		if paramOK && literalAOK && literalBOK && validBinary(binaryA, param, param, literalA) && validBinary(binaryB, param, param, literalB) &&
			instructions[3].dst == literalB && instructions[5].op == registerIRReturn && instructions[5].left == param {
			return 2, binaryA.opcode, binaryB.opcode, constantA, constantB
		}
	}
	return 0, 0, 0, 0, 0
}

// registerIRIntegerLinearInputMatchesParameter distinguishes the compiler's
// two representations of a block argument.  Methods may use LoadParam while
// blocks normally use LoadLocal after binding.  The raw linear executor has no
// local environment, so a LoadLocal is safe only when it is exactly the
// function's sole parameter slot.
func registerIRIntegerLinearInputMatchesParameter(plan *registerIRPlan, fn *object.Function) bool {
	if plan == nil || fn == nil || len(fn.ParamLocalIndices) != 1 {
		return false
	}
	inputLocal := -1
	inputCount := 0
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param != 0 {
				return false
			}
			inputCount++
		case registerIRLoadLocal:
			inputLocal = int(instruction.param)
			inputCount++
		}
		if inputCount > 1 {
			return false
		}
	}
	if inputCount != 1 {
		return false
	}
	return inputLocal < 0 || inputLocal == fn.ParamLocalIndices[0]
}

// registerIRIntegerOnlyPrefix reports whether all instructions compiled so
// far are part of the unboxed scalar tier. Arithmetic and local stores set
// plan.hasSends/plan.requiresFrame because their general Ruby forms can
// dispatch or update a captured cell, but those markers are conservative for
// an integer-only plan. Keeping this predicate separate lets a later branch
// use the direct comparison path without allowing a real user send to be
// replayed after a guard miss.
func registerIRIntegerOnlyPrefix(plan *registerIRPlan) bool {
	if plan == nil {
		return false
	}
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadInstanceVar, registerIRMove, registerIRStoreLocal,
			registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy,
			registerIRJumpNotNil, registerIRReturn, registerIREqual,
			registerIRBinary, registerIRCompare:
		default:
			return false
		}
	}
	return true
}

func registerIRSafePlainSendName(name string) bool {
	switch name {
	case "include", "alias_method", "eval", "binding", "local_variable_get", "local_variable_set",
		"send", "__send__", "public_send", "define_method", "def_delegators", "module_eval", "class_eval", "instance_eval",
		"instance_exec", "class_exec", "module_exec", "const_set", "remove_method", "undef_method", "singleton_class":
		return false
	default:
		return name != ""
	}
}

func registerIRImmutableLiteral(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	switch value.Type {
	case object.ValueNil, object.ValueBool, object.ValueInteger, object.ValueFloat, object.ValueSymbol:
		return true
	case object.ValueString:
		// A frozen-string-literal constant is immutable and intentionally shared
		// across evaluations.  It can be loaded by pointer just like a Symbol;
		// mutable strings still require the opt-in constant materialization tier.
		return registerIRFrozenStringLiteralsEnabled && value.Frozen
	case object.ValueRegexp:
		return value.Frozen
	default:
		return false
	}
}

func registerIRImmediateEqualityValue(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	switch value.Type {
	case object.ValueNil, object.ValueBool, object.ValueInteger, object.ValueFloat, object.ValueSymbol:
		return true
	default:
		return false
	}
}

func (vm *VM) executeRegisterIR(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, method string, methodObj *object.Method, methodOwner *object.Class, cachedFramed ...bool) (*object.EmeraldValue, bool) {
	if plan == nil || plan.registers > 16 || fn != nil && !simpleBlockParameterPatterns(fn) {
		return nil, false
	}
	if fn != nil && methodObj != nil {
		if result, executed := vm.tryExecuteTypedIntegerMethod(methodObj, fn, args); executed {
			return result, true
		}
		if result, executed := vm.tryExecuteTypedSSAFunction(methodObj, fn, receiver, args); executed {
			return result, true
		}
	}
	fastFramed := len(cachedFramed) > 0 && cachedFramed[0]
	hasRestParam := fn != nil && fn.HasRestParam
	// A simple Ruby initializer is already fully checked by invokeMethod's
	// argument/visibility protocol. Its straight-line ivar stores have no
	// dynamic safepoint, so execute them through the same direct ABI used by
	// nested hot callees instead of allocating and tearing down a Ruby Frame for
	// every `Class.new` call. Keep the admission narrow: no block, keywords,
	// rescues, tracing, or refinements may be active.
	if !fastFramed && method == "initialize" && methodObj != nil && fn != nil &&
		registerIRConstructorPlan(plan) && len(args) == len(fn.Params) && !fn.HasRestParam && !fn.HasBlockParam &&
		len(fn.KeywordParams) == 0 && fn.KeywordRestParam == "" && !fn.KeywordRestOnly &&
		vm.instructionLimit == 0 && !DevMode && !core.AnyTracePointActive() &&
		len(vm.catchStack) == 0 && len(vm.activeRescues) == 0 && len(vm.rescueStack) == 0 &&
		!methodUsesRefinements(methodObj) && registerIRDirectNoFrameEnabled {
		if result, executed := vm.executeRegisterIRDirectNoFrameWithFreeMode(plan, fn, receiver, args, object.CurrentMethodGeneration(), nil, false); executed {
			return result, true
		}
	}
	if fastFramed {
		// The call-site cache has already proved the exact public positional
		// shape and retained the decoded plan.  Keep the unwind/trace guards,
		// but do not repeat speculative no-frame proofs that already failed at
		// this site (or would only add work before the framed executor).
		if !registerIRPlanSafeForActiveRescues(plan, vm) || fn == nil || methodObj == nil ||
			len(vm.catchStack) > 0 || core.AnyTracePointActive() || DevMode || vm.instructionLimit != 0 {
			return nil, false
		}
	} else {
		if plan.integerOnly && !hasRestParam && registerIRIntegerOnlyCanUseFastArgs(plan, args) {
			return vm.executeRegisterIRIntegerOnly(plan, fn, receiver, args)
		}
		if !registerIRPlanSafeForActiveRescues(plan, vm) {
			return nil, false
		}
		directConstantsSafe := methodObj != nil && registerIRDirectConstantsSafe(vm, methodObj.Closure, plan)
		directPlanSafe := registerIRPlanSafeForDirectNoFrame(plan)
		if directConstantsSafe {
			directPlanSafe = registerIRPlanSafeForDirectNoFrameWithOptions(plan, false, true)
		}
		if !hasRestParam && registerIRDirectNoFrameEnabled && registerIRNoFrameEnabled && methodObj != nil &&
			plan.hasSends && !plan.hasImplicitSends && directPlanSafe &&
			len(vm.catchStack) == 0 && !core.AnyTracePointActive() && !DevMode &&
			vm.instructionLimit == 0 && !methodUsesRefinements(methodObj) {
			result, executed := vm.tryExecuteRegisterIRDirectNoFrame(plan, fn, receiver, args, false, directConstantsSafe)
			if executed {
				return result, true
			}
		}
		if !hasRestParam && registerIRNoFrameEnabled && plan.sendCount >= 3 && !plan.requiresFrame && !plan.hasBranches && methodObj != nil &&
			len(vm.catchStack) == 0 && !core.AnyTracePointActive() && !DevMode && vm.instructionLimit == 0 &&
			!methodUsesRefinements(methodObj) {
			if result, executed := vm.tryExecuteRegisterIRNoFrame(plan, receiver, args); executed {
				return result, true
			}
		}
		if !hasRestParam && !plan.hasSends && registerIRPlanSafeWithoutFrame(plan) {
			return vm.executeRegisterIRInstructions(plan, receiver, args, nil)
		}
	}
	if fn == nil || methodObj == nil || len(vm.catchStack) > 0 {
		return nil, false
	}

	var oldFrame *Frame
	if vm.fp >= 0 && vm.fp < len(vm.frames) {
		oldFrame = vm.frames[vm.fp]
	}
	bp := vm.sp
	vm.stack[vm.sp] = receiver
	vm.sp++
	if fn.HasRestParam {
		vm.bindRestParameterSlots(fn, args, bp, methodObj.Ruby2Keywords)
	} else {
		vm.bindPositionalParameterSlots(fn, args, bp)
	}
	minSp := bp + 1 + fn.NumLocals
	if vm.sp < minSp {
		for index := vm.sp; index < minSp; index++ {
			vm.stack[index] = nil
		}
		vm.sp = minSp
	}

	invocation := vm.buildInvocationMetadata(receiver, method, methodObj, methodOwner)
	frame := vm.pushReusableFrame()
	*frame = Frame{
		ID:                    vm.allocateFrameID(),
		Fn:                    fn,
		Ip:                    -1,
		Bp:                    bp,
		Closure:               methodObj.Closure,
		MethodName:            method,
		OriginalMethodName:    methodObj.OriginalName,
		SuperStart:            invocation.superStart,
		SuperModule:           invocation.superModule,
		SuperAfterClass:       invocation.superAfterClass,
		Args:                  args,
		Block:                 vm.currentBlock,
		DefinedByDefineMethod: methodObj.DefinedByDefineMethod,
		BlockBreakAddr:        -1,
		WhileStart:            -1,
		WhileEnd:              -1,
		TraceSelf:             receiver,
		BacktraceReceiver:     receiver,
		BacktraceMethod:       methodObj,
		BacktraceOwner:        methodOwner,
	}
	previousBlock := vm.currentBlock
	vm.currentBlock = nil
	previousClassStack := vm.classStack
	if methodObj.Closure != nil {
		vm.classStack = methodObj.Closure.ClassStack
	}

	result, executed := vm.executeRegisterIRInstructions(plan, receiver, args, frame, registerIRSendCacheEnabled && registerIRSendCacheContextSafe(methodObj))
	vm.currentBlock = previousBlock
	vm.classStack = previousClassStack
	vm.setStackPointer(bp)
	vm.endActiveRescuesForFrame(frame)
	frame.InstructionException = nil
	frame.InstructionSnapshotSet = false
	vm.frames = vm.frames[:vm.fp]
	vm.fp--
	if vm.fp >= 0 && oldFrame != nil {
		vm.frames[vm.fp] = oldFrame
	}
	return result, executed
}

func (vm *VM) executeRegisterIRNoFrame(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if plan == nil || plan.sendCount < 3 || plan.requiresFrame || vm == nil || !registerIRPlanSafeForActiveRescues(plan, vm) {
		return nil, false
	}
	return vm.executeRegisterIRInstructions(plan, receiver, args, nil, true)
}

func (vm *VM) tryExecuteRegisterIRNoFrame(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if plan == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	if plan.noFrameGeneration != generation {
		plan.noFrameGeneration = generation
		plan.noFrameCalls = 0
		plan.noFrameDisabled = false
	}
	if plan.noFrameCalls < 8 {
		plan.noFrameCalls++
		return nil, false
	}
	if plan.noFrameDisabled {
		return nil, false
	}
	if result, executed := vm.executeRegisterIRNoFrame(plan, receiver, args); executed {
		return result, true
	}
	// A miss is normally a stable shape failure (uncacheable receiver,
	// unsupported leaf, or a side-effectful native). Retrying it on every
	// subsequent call costs more than the frame it was meant to avoid. The
	// generation guard above re-arms the plan after a method redefinition.
	plan.noFrameDisabled = true
	return nil, false
}

func (vm *VM) tryExecuteRegisterIRDirectNoFrame(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, options ...bool) (*object.EmeraldValue, bool) {
	return vm.tryExecuteRegisterIRDirectNoFrameWithFree(plan, fn, receiver, args, nil, options...)
}

func (vm *VM) tryExecuteRegisterIRDirectNoFrameWithFree(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, free []*object.EmeraldValue, options ...bool) (*object.EmeraldValue, bool) {
	if plan == nil || vm == nil {
		return nil, false
	}
	allowBlockReturn := len(options) > 0 && options[0]
	allowConstants := len(options) > 1 && options[1]
	allowCaseBranch := len(options) > 2 && options[2]
	generation := object.CurrentMethodGeneration()
	if plan.noFrameGeneration != generation {
		plan.noFrameGeneration = generation
		plan.noFrameCalls = 0
		plan.noFrameDisabled = false
	}
	// The direct tier waits for a few framed calls before probing. Unlike the
	// broader no-frame tier it never speculates on an uncached Ruby callee, but
	// a single failed probe is still costly for short-lived methods.
	if plan.noFrameCalls < registerIRDirectNoFrameWarmupCalls {
		plan.noFrameCalls++
		return nil, false
	}
	if plan.noFrameDisabled {
		return nil, false
	}
	if plan.directFastKind != registerIRDirectFastNone {
		if result, executed := vm.executeRegisterIRDirectFast(plan, fn, receiver, args, generation); executed {
			return result, true
		}
		// A memoized reader legitimately misses while its ivar is still nil or
		// false; the framed initializer will populate it, so retain the probe for
		// later calls instead of permanently disabling the fast path.
		if plan.directFastKind != registerIRDirectFastMemoizedIvar && plan.directFastKind != registerIRDirectFastIntegerStringConcat {
			plan.noFrameDisabled = true
		}
		return nil, false
	}
	if result, executed := vm.executeRegisterIRDirectNoFrameWithFree(plan, fn, receiver, args, generation, free, allowBlockReturn, allowConstants, allowCaseBranch); executed {
		return result, true
	}
	// Direct chains are deliberately one-shot per method generation. A cache or
	// built-in type miss must not add another probe to every subsequent call.
	plan.noFrameDisabled = true
	return nil, false
}

func (vm *VM) executeRegisterIRDirectNoFrame(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, generation uint64, options ...bool) (*object.EmeraldValue, bool) {
	return vm.executeRegisterIRDirectNoFrameWithFree(plan, fn, receiver, args, generation, nil, options...)
}

// executeRegisterIRDirectNoFrame runs the small framed subset whose sends are
// already resolved to pure native/accessor leaves. It intentionally returns a
// miss instead of raising: the caller has not entered user code and can still
// execute the original framed plan with complete Ruby error/backtrace state.
func (vm *VM) executeRegisterIRDirectNoFrameWithFree(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, generation uint64, free []*object.EmeraldValue, options ...bool) (*object.EmeraldValue, bool) {
	return vm.executeRegisterIRDirectNoFrameWithFreeMode(plan, fn, receiver, args, generation, free, false, options...)
}

// executeRegisterIRDirectNoFrameWithFreeTrusted is the steady-state entry for
// a typed block loop after one invocation has crossed the normal direct-tier
// proof and generation guard.  The caller still supplies the current method
// generation; nested send caches retain their receiver/class checks, so a
// changed dynamic callee returns executed=false and the caller deopts the
// remaining suffix through the ordinary Ruby protocol.
func (vm *VM) executeRegisterIRDirectNoFrameWithFreeTrusted(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, generation uint64, free []*object.EmeraldValue, options ...bool) (*object.EmeraldValue, bool) {
	return vm.executeRegisterIRDirectNoFrameWithFreeMode(plan, fn, receiver, args, generation, free, true, options...)
}

func (vm *VM) executeRegisterIRDirectNoFrameWithFreeMode(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, generation uint64, free []*object.EmeraldValue, trusted bool, options ...bool) (resultValue *object.EmeraldValue, executed bool) {
	allowBlockReturn := len(options) > 0 && options[0]
	allowConstants := len(options) > 1 && options[1]
	allowCaseBranch := len(options) > 2 && options[2]
	aggressive := len(options) > 3 && options[3]
	// trustedRegion is used only by a loop that has already executed one
	// complete iteration and proved that every send in the body is a cached
	// native leaf.  It removes the repeated global-generation/cache-admission
	// work from the steady state while retaining the receiver/class guard at
	// each send.  A region miss is a clean side exit; the caller replays the
	// current suffix through the ordinary block protocol.
	trustedRegion := len(options) > 4 && options[4]
	if trustedRegion && !trusted {
		return nil, false
	}
	planSafe := trusted
	if !trusted {
		planSafe = registerIRPlanSafeForDirectNoFrame(plan)
		if allowConstants && !allowBlockReturn && !allowCaseBranch {
			planSafe = registerIRPlanSafeForDirectNoFrameWithOptions(plan, false, true)
		}
		if allowCaseBranch {
			planSafe = registerIRPlanSafeForDirectNoFrameWithOptions(plan, allowBlockReturn, allowConstants, true)
		}
		if allowBlockReturn {
			if allowCaseBranch {
				// Keep the branch-enabled proof when a block also has a
				// BlockReturn. The legacy cached block bit is intentionally
				// branch-free and would otherwise override this stronger proof.
				planSafe = registerIRPlanSafeForDirectNoFrameWithOptions(plan, true, allowConstants, true)
			} else {
				planSafe = registerIRPlanSafeForDirectNoFrameBlock(plan)
			}
		}
	}
	if vm == nil || plan == nil || !planSafe || !trustedRegion && !aggressive && generation != object.CurrentMethodGeneration() {
		return nil, false
	}
	if plan.directFastKind != registerIRDirectFastNone {
		if result, executed := vm.executeRegisterIRDirectFast(plan, fn, receiver, args, generation); executed {
			return result, true
		}
		// The fast block is speculative.  A miss must replay the complete framed
		// method so that custom []/key? implementations and raise paths retain
		// their normal Ruby semantics.
		return nil, false
	}
	var registers [16]*object.EmeraldValue
	// Most Ruby methods use only a handful of locals. Keep the speculative
	// direct tier's fixed storage small; methods with a larger local frame
	// deopt to the ordinary framed Register IR path below.
	var locals [64]*object.EmeraldValue
	if fn == nil {
		return nil, false
	}
	for index, local := range fn.ParamLocalIndices {
		if local < 0 || local >= len(locals) || index >= len(args) {
			return nil, false
		}
		locals[local] = args[index]
	}
	for pc := 0; pc < len(plan.instructions); {
		instruction := plan.instructions[pc]
		switch instruction.op {
		case registerIRLoadParam:
			if int(instruction.param) >= len(args) {
				return nil, false
			}
			registers[instruction.dst] = args[instruction.param]
		case registerIRLoadLiteral:
			registers[instruction.dst] = instruction.value
		case registerIRLoadFrozenString:
			// Frozen literals are immutable shared values; unlike mutable
			// constant strings they do not need a per-call clone.
			registers[instruction.dst] = instruction.value
		case registerIRLoadConstantValue:
			value, ok := vm.directRegisterIRConstantValue(instruction.value, fn)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRLoadInstanceVar:
			value := core.DynamicInstanceVar(receiver, instruction.name)
			if value == nil {
				value = core.R.NilVal
			}
			registers[instruction.dst] = value
		case registerIRSetStringEncoding:
			if !vm.executeRegisterIRSetStringEncoding(nil, instruction, &registers) {
				return nil, false
			}
		case registerIRStoreInstanceVar:
			value := registers[instruction.left]
			if value == nil {
				value = core.R.NilVal
			}
			if result := core.SetDynamicInstanceVar(receiver, instruction.name, value); result != nil {
				return result, true
			}
		case registerIRDefinedInstanceVar:
			value, ok := executeRegisterIRDefinedInstanceVar(receiver, instruction.name)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRLoadFree:
			if !allowBlockReturn || int(instruction.param) >= len(free) {
				return nil, false
			}
			registers[instruction.dst] = derefClosureValue(free[instruction.param])
		case registerIRLoadConstant:
			if !allowConstants {
				return nil, false
			}
			var value *object.EmeraldValue
			var ok bool
			if trustedRegion && plan.trustedTopLevelConstantVM == vm &&
				plan.trustedTopLevelConstantGeneration == object.CurrentConstantGeneration() &&
				pc >= 0 && pc < len(plan.trustedTopLevelConstants) {
				value = plan.trustedTopLevelConstants[pc]
				ok = value != nil
			} else {
				value, ok = vm.topLevelConstantValue(instruction.name)
			}
			if !ok || value == nil {
				if !aggressive {
					return nil, false
				}
				value = vm.missingConstantValue(instruction.name, false)
				if value == nil {
					value = core.NewNameError("uninitialized constant " + instruction.name)
				}
			}
			registers[instruction.dst] = value
			if value.Type == object.ValueException {
				if aggressive {
					core.LastException = value
				}
				return value, true
			}
		case registerIRLoadScopedConstant:
			if !allowConstants {
				return nil, false
			}
			container := registers[instruction.left]
			var value *object.EmeraldValue
			var ok bool
			value, ok = vm.scopedConstantValue(container, instruction.name)
			if !ok || value == nil {
				if !aggressive {
					return nil, false
				}
				if container != nil {
					value = vm.sendBypassVisibility(container, "const_missing", []*object.EmeraldValue{{Type: object.ValueSymbol, Data: instruction.name, Class: core.R.Classes["Symbol"]}})
				} else {
					value = core.NewNameError("uninitialized constant " + instruction.name)
				}
				if value == nil {
					value = core.NewNameError("uninitialized constant " + instruction.name)
				}
			}
			registers[instruction.dst] = value
			if value.Type == object.ValueException {
				if aggressive {
					core.LastException = value
				}
				return value, true
			}
		case registerIRLoadLocal:
			local := int(instruction.param)
			if local < 0 || local >= len(locals) {
				return nil, false
			}
			if locals[local] == nil {
				locals[local] = core.R.NilVal
			}
			registers[instruction.dst] = locals[local]
		case registerIRLoadSelf:
			registers[instruction.dst] = receiver
		case registerIRMove:
			registers[instruction.dst] = registers[instruction.left]
		case registerIRSwap:
			registers[instruction.left], registers[instruction.right] = registers[instruction.right], registers[instruction.left]
		case registerIRBang:
			registers[instruction.dst] = registerIRBangValue(registers[instruction.left])
		case registerIRStoreLocal:
			local := int(instruction.param)
			if local < 0 || local >= len(locals) {
				return nil, false
			}
			value := registers[instruction.left]
			if value == nil {
				value = core.R.NilVal
			}
			locals[local] = value
		case registerIRStoreFree:
			// A StoreFree admitted by registerIRPlanSafeForDirectNoFrameBlock
			// is the terminal mutation of the block.  It is safe to update the
			// live closure directly because no later speculative send can miss
			// and replay this write.
			if !allowBlockReturn || int(instruction.param) >= len(free) {
				return nil, false
			}
			value := registers[instruction.left]
			if value == nil {
				value = core.R.NilVal
			}
			setClosureValue(&free[instruction.param], value)
		case registerIREqual:
			result, ok := vm.executeRegisterIRNoFrameEqual(registers[instruction.left], registers[instruction.right])
			if !ok {
				if !aggressive {
					return nil, false
				}
				left, right := registers[instruction.left], registers[instruction.right]
				if left == nil {
					left = core.R.NilVal
				}
				if right == nil {
					right = core.R.NilVal
				}
				result = vm.equals(left, right)
			}
			if result == nil {
				result = core.R.NilVal
			}
			registers[instruction.dst] = result
			if result.Type == object.ValueException && aggressive {
				core.LastException = result
				return result, true
			}
		case registerIRDynamicCompare:
			result, ok := vm.executeRegisterIRIntegerComparison(registers[instruction.left], registers[instruction.right], instruction.opcode)
			if !ok {
				if !aggressive {
					return nil, false
				}
				result = vm.aggressiveRegisterIRCompare(registers[instruction.left], registers[instruction.right], instruction.opcode)
			}
			if result == nil {
				result = core.R.NilVal
			}
			registers[instruction.dst] = result
			if result.Type == object.ValueException && aggressive {
				core.LastException = result
				return result, true
			}
		case registerIRCompare:
			result, ok := vm.executeRegisterIRIntegerComparison(registers[instruction.left], registers[instruction.right], instruction.opcode)
			if !ok {
				if !aggressive {
					return nil, false
				}
				result = vm.aggressiveRegisterIRCompare(registers[instruction.left], registers[instruction.right], instruction.opcode)
			}
			if result == nil {
				result = core.R.NilVal
			}
			registers[instruction.dst] = result
			if result.Type == object.ValueException && aggressive {
				core.LastException = result
				return result, true
			}
		case registerIRBinary:
			result, ok := vm.executeRegisterIRNoFrameBinary(instruction, &registers)
			if !ok {
				if !aggressive {
					return nil, false
				}
				result = vm.aggressiveRegisterIRBinary(instruction, &registers)
			}
			if result == nil {
				result = core.R.NilVal
			}
			registers[instruction.dst] = result
			if result.Type == object.ValueException && aggressive {
				core.LastException = result
				return result, true
			}
		case registerIRSend:
			if instruction.name == "raise" {
				// A proven cold validation branch deopts before invoking raise;
				// the ordinary framed executor supplies the exception/backtrace.
				if !aggressive {
					return nil, false
				}
			}
			var result *object.EmeraldValue
			var ok bool
			if trustedRegion {
				result, ok = vm.executeRegisterIRTrustedDirectSend(instruction, &registers, allowCaseBranch)
			} else {
				result, ok = vm.executeRegisterIRInlineSendNoFrame(instruction, &registers, true, allowCaseBranch, aggressive)
			}
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRIndex:
			result, ok := vm.executeRegisterIRIndex(nil, instruction, &registers)
			if !ok {
				if !aggressive {
					return nil, false
				}
				left, index := registers[instruction.left], registers[instruction.right]
				if left == nil {
					left = core.R.NilVal
				}
				if index == nil {
					index = core.R.NilVal
				}
				result = vm.index(left, index)
			}
			if result == nil {
				result = core.R.NilVal
			}
			registers[instruction.dst] = result
			if result.Type == object.ValueException && aggressive {
				core.LastException = result
				return result, true
			}
		case registerIRIndexAssign:
			if !aggressive {
				return nil, false
			}
			left, index, value := registers[instruction.left], registers[instruction.right], registers[instruction.args[0]]
			if left == nil {
				left = core.R.NilVal
			}
			if index == nil {
				index = core.R.NilVal
			}
			if value == nil {
				value = core.R.NilVal
			}
			result := vm.indexAssign(left, index, value)
			if result != nil && result.Type == object.ValueException {
				core.LastException = result
				return result, true
			}
			registers[instruction.dst] = value
		case registerIRSlice:
			result, ok := vm.executeRegisterIRDirectSlice(&registers, instruction)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRArray:
			if !vm.executeRegisterIRArray(nil, instruction, &registers) {
				return nil, false
			}
		case registerIRHash:
			if !vm.executeRegisterIRHash(nil, instruction, &registers) {
				return nil, false
			}
		case registerIRJump:
			pc = instruction.target
			continue
		case registerIRJumpTruthy:
			if registers[instruction.left] != nil && registers[instruction.left].IsTruthy() {
				pc = instruction.target
				continue
			}
		case registerIRJumpNotTruthy:
			if registers[instruction.left] == nil || !registers[instruction.left].IsTruthy() {
				pc = instruction.target
				continue
			}
		case registerIRJumpNotNil:
			if registers[instruction.left] != nil && registers[instruction.left].Type != object.ValueNil {
				pc = instruction.target
				continue
			}
		case registerIRRaise:
			if !aggressive {
				return nil, false
			}
			exception := registers[instruction.left]
			if exception == nil || exception.Type != object.ValueException {
				if exception == nil {
					exception = core.R.NilVal
				}
				exception = core.RaiseValue(exception)
			}
			core.LastException = exception
			return exception, true
		case registerIRReturn:
			return registers[instruction.left], true
		default:
			return nil, false
		}
		pc++
	}
	return nil, false
}

// directRegisterIRConstantValue mirrors the mutable-literal branch of
// VM.constantValue for the no-frame block executor.  Register IR reaches this
// helper only with a String literal (frozen literals are shared immutable
// values and use registerIRLoadLiteral); cloning preserves Ruby's per-call
// object identity and prevents a later `<<` from modifying the function's
// constant template.
func (vm *VM) directRegisterIRConstantValue(value *object.EmeraldValue, fn *object.Function) (*object.EmeraldValue, bool) {
	if vm == nil || value == nil || value.Type != object.ValueString {
		return nil, false
	}
	mutable := *value
	mutable.Cold = value.CloneColdData()
	mutable.Frozen = false
	mutable.Chilled = vm.chillStringLiterals
	mutable.Literal = true
	encoding := vm.sourceEncoding
	if fn != nil && fn.SourceEncoding != "" {
		encoding = fn.SourceEncoding
	}
	if value.Encoding != "" {
		encoding = value.Encoding
	}
	result := &mutable
	if encoding != "" {
		core.SetStringEncoding(result, encoding)
	}
	return result, true
}

// executeRegisterIRDirectFast is the predecoded basic-block tier for the two
// short-circuit shapes admitted by registerIRDirectFastShape.  It performs no
// IR switch or frame setup on the successful path; each nested send is still
// guarded by the already-populated generation/class cache and deoptimizes
// before user-visible work when that proof is unavailable.
func (vm *VM) executeRegisterIRDirectFast(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, generation uint64) (*object.EmeraldValue, bool) {
	if vm == nil || plan == nil || generation != object.CurrentMethodGeneration() {
		return nil, false
	}
	switch plan.directFastKind {
	case registerIRDirectFastMemoizedIvar:
		if receiver == nil || len(args) != 0 {
			return nil, false
		}
		value := core.DynamicInstanceVar(receiver, plan.directFastMemoIvar)
		if value == nil || !value.IsTruthy() {
			// Nil/false must replay the initializer, including its sends,
			// allocation, exception and frozen-object behavior.
			return nil, false
		}
		return value, true
	case registerIRDirectFastReferenceAppend:
		if receiver == nil || len(args) != 1 {
			return nil, false
		}
		data := core.DynamicInstanceVar(receiver, plan.directFastReferenceDataIvar)
		// Restrict the speculative success path to an exact built-in Hash.  A
		// subclass or singleton override may change is_a? semantics, so it
		// re-enters the original framed method instead of guessing.
		if data == nil || data.Type != object.ValueHash || data.Class != core.R.Classes["Hash"] || core.AttachedSingletonClass(data) != nil {
			return nil, false
		}
		stream := core.DynamicInstanceVar(receiver, plan.directFastReferenceStreamIvar)
		if stream == nil || stream.Type == object.ValueNil {
			return nil, false
		}
		result, handled := core.AppendStringOneFast(stream, args[0])
		if !handled {
			result = vm.send(stream, "<<", args)
		}
		if result == nil {
			return nil, false
		}
		return result, true
	case registerIRDirectFastStreamAppend:
		if receiver == nil || len(args) != 1 || !core.StringAppendUsesBuiltinImplementation() {
			return nil, false
		}
		stream := core.DynamicInstanceVar(receiver, plan.directFastStreamIvar)
		if stream == nil || stream.Type == object.ValueNil {
			// The source literal is frozen, so +@ must produce a fresh mutable
			// String.  Keep that copy-on-write behavior even though the direct
			// executor has no Ruby Frame from which to call constantValue.
			var registers [16]*object.EmeraldValue
			literal := plan.instructions[plan.directFastStreamLiteral]
			var initialized *object.EmeraldValue
			var ok bool
			if literal.op == registerIRLoadFrozenString {
				initialized = core.MutableStringCopy(literal.value)
				ok = initialized != nil
			} else {
				registers[literal.dst] = literal.value
				initialized, ok = vm.executeRegisterIRInlineSendNoFrame(plan.instructions[plan.directFastStreamSend], &registers, true)
			}
			if !ok || initialized == nil || initialized.Type != object.ValueString || initialized.Class != core.R.Classes["String"] {
				return nil, false
			}
			if errVal := core.SetDynamicInstanceVar(receiver, plan.directFastStreamIvar, initialized); errVal != nil {
				return errVal, true
			}
			stream = initialized
		}
		// A custom String subclass may override <<.  The exact built-in class
		// guard keeps this tier side-effect free until the append primitive is
		// known to be the installed implementation.
		if stream == nil || stream.Type != object.ValueString || stream.Class != core.R.Classes["String"] {
			return nil, false
		}
		result, handled := core.AppendStringOneFast(stream, args[0])
		if !handled {
			result = core.AppendStringOne(stream, args[0])
		}
		if result != nil && result.Type == object.ValueException {
			return result, true
		}
		if errVal := core.SetDynamicInstanceVar(receiver, plan.directFastFilteredIvar, core.R.NilVal); errVal != nil {
			return errVal, true
		}
		return receiver, true
	case registerIRDirectFastShortCircuitIndex:
		first := core.DynamicInstanceVar(receiver, plan.directFastFirstIvar)
		if first == nil {
			first = core.R.NilVal
		}
		if first.IsTruthy() {
			return first, true
		}
		var registers [16]*object.EmeraldValue
		registers[plan.instructions[4].left] = receiver
		second, ok := vm.executeRegisterIRInlineSendNoFrame(plan.instructions[plan.directFastSendA], &registers, true)
		if !ok {
			return nil, false
		}
		if second == nil {
			return nil, false
		}
		if !second.IsTruthy() {
			return second, true
		}
		third, ok := vm.executeRegisterIRInlineSendNoFrame(plan.instructions[plan.directFastSendB], &registers, true)
		if !ok {
			return nil, false
		}
		if third == nil {
			return nil, false
		}
		if !third.IsTruthy() {
			return third, true
		}
		key := plan.instructions[plan.directFastKey].value
		return vm.executeRegisterIRDirectIndex(third, key)
	case registerIRDirectFastHashOption:
		if len(args) != 2 || args[0] == nil || args[1] == nil {
			return nil, false
		}
		var registers [16]*object.EmeraldValue
		// The compiler emits options as parameter 1 and name as parameter 0 for
		// this shape.  Keep the check tied to the decoded plan so a future
		// compiler stack layout cannot silently enter the fast path.
		if plan.instructions[0].param != 1 || plan.instructions[1].param != 0 ||
			plan.instructions[2].left != plan.instructions[0].dst ||
			plan.instructions[2].args[0] != plan.instructions[1].dst ||
			plan.instructions[4].param != 1 || plan.instructions[5].param != 0 {
			return nil, false
		}
		registers[plan.instructions[0].dst] = args[1]
		registers[plan.instructions[1].dst] = args[0]
		keyResult, ok := vm.executeRegisterIRInlineSendNoFrame(plan.instructions[plan.directFastSendA], &registers, true)
		if !ok || keyResult == nil || !keyResult.IsTruthy() {
			// A false key? result deliberately deopts: the original branch may
			// raise or perform another observable operation.
			return nil, false
		}
		return vm.executeRegisterIRDirectIndex(args[1], args[0])
	case registerIRDirectFastIntegerStringConcat:
		return vm.executeRegisterIRDirectIntegerStringConcat(plan, fn, receiver, args, generation)
	default:
		return nil, false
	}
}

func (vm *VM) executeRegisterIRDirectIntegerStringConcat(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, generation uint64) (*object.EmeraldValue, bool) {
	if vm == nil || plan == nil || receiver == nil || generation != object.CurrentMethodGeneration() ||
		len(args) <= int(plan.directFastIntegerStringKeyParam) || len(args) <= int(plan.directFastIntegerStringValueParam) ||
		!vm.fusedIntegerOperationAvailable(compiler.OpGreaterThan) ||
		!vm.fusedIntegerOperationAvailable(compiler.OpMul) ||
		!vm.fusedIntegerOperationAvailable(compiler.OpAdd) ||
		!vm.fusedIntegerToSAvailable() || !vm.fusedStringOperationAvailable(compiler.OpAdd) {
		return nil, false
	}
	value, ok := registerIRDirectFastExactInteger(args[plan.directFastIntegerStringValueParam])
	if !ok {
		return nil, false
	}
	if value <= plan.directFastIntegerStringThreshold {
		return vm.executeRegisterIRDirectIntegerStringFallback(plan, fn)
	}
	key, ok := registerIRDirectFastExactInteger(args[plan.directFastIntegerStringKeyParam])
	if !ok {
		return nil, false
	}
	prefix := core.DynamicInstanceVar(receiver, plan.directFastIntegerStringPrefixIvar)
	if prefix == nil || prefix.Type != object.ValueString || prefix.Class != core.R.Classes["String"] || core.AttachedSingletonClass(prefix) != nil {
		return nil, false
	}
	product, ok := checkedIntegerMul(key, plan.directFastIntegerStringMultiplier)
	if !ok {
		return nil, false
	}
	total, ok := checkedIntegerAdd(product, value)
	if !ok {
		return nil, false
	}
	if vm.typedStringValueBatch != nil {
		if result, handled := core.StringConcatIntegerBatch(vm.typedStringValueBatch, prefix, total); handled {
			return result, true
		}
	}
	return core.StringConcatIntegerRaw(prefix, total)
}

func (vm *VM) executeRegisterIRDirectIntegerStringFallback(plan *registerIRPlan, fn *object.Function) (*object.EmeraldValue, bool) {
	if vm == nil || plan == nil || int(plan.directFastIntegerStringFallback) >= len(plan.instructions) {
		return nil, false
	}
	instruction := plan.instructions[plan.directFastIntegerStringFallback]
	switch instruction.op {
	case registerIRLoadConstantValue:
		return vm.directRegisterIRConstantValue(instruction.value, fn)
	case registerIRLoadLiteral, registerIRLoadFrozenString:
		if instruction.value == nil || instruction.value.Type != object.ValueString {
			return nil, false
		}
		if instruction.value.Frozen {
			return instruction.value, true
		}
		return vm.directRegisterIRConstantValue(instruction.value, fn)
	default:
		return nil, false
	}
}

func registerIRDirectFastExactInteger(value *object.EmeraldValue) (int64, bool) {
	if !smallIntegerValue(value) || value.Class != nil && value.Class != core.R.Classes["Integer"] || core.AttachedSingletonClass(value) != nil {
		return 0, false
	}
	return value.Data.(int64), true
}

func (vm *VM) executeRegisterIRIntegerOnly(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if integerBlockSingleArgEnabled && len(args) == 1 && smallIntegerValue(args[0]) {
		if result, executed := vm.executeRegisterIRIntegerOnlySingleArg(plan, fn, receiver, args[0].Data.(int64)); executed {
			return result, true
		}
	}
	var registers [16]registerIRIntegerValue
	// A compact native local file is enough for the small scalar methods this
	// tier targets. Larger methods retain full Ruby locals and fall back before
	// any user-visible state is changed.
	var locals [64]registerIRIntegerValue
	var localSet [64]bool
	valueFromObject := func(value *object.EmeraldValue) (registerIRIntegerValue, bool) {
		if value == nil {
			return registerIRIntegerValue{kind: 3}, true
		}
		switch value.Type {
		case object.ValueInteger:
			if !smallIntegerValue(value) {
				return registerIRIntegerValue{}, false
			}
			return registerIRIntegerValue{value: value.Data.(int64)}, true
		case object.ValueBool:
			if value.Data.(bool) {
				return registerIRIntegerValue{kind: 2}, true
			}
			return registerIRIntegerValue{kind: 1}, true
		case object.ValueNil:
			return registerIRIntegerValue{kind: 3}, true
		default:
			return registerIRIntegerValue{}, false
		}
	}
	if fn != nil {
		for index, local := range fn.ParamLocalIndices {
			if local < 0 || local >= len(locals) || index >= len(args) {
				return nil, false
			}
			value, ok := valueFromObject(args[index])
			if !ok {
				return nil, false
			}
			locals[local] = value
			localSet[local] = true
		}
	}
	for pc := 0; pc < len(plan.instructions); {
		instruction := plan.instructions[pc]
		switch instruction.op {
		case registerIRLoadParam:
			if int(instruction.param) >= len(args) {
				return nil, false
			}
			value, ok := valueFromObject(args[instruction.param])
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRLoadLocal:
			local := int(instruction.param)
			if local < 0 || local >= len(locals) {
				return nil, false
			}
			if localSet[local] {
				registers[instruction.dst] = locals[local]
			} else {
				// Ruby locals read before assignment are nil. Keep an explicit
				// presence bit so the zero value (integer 0) is never confused
				// with an uninitialized slot without clearing 256 entries per call.
				registers[instruction.dst] = registerIRIntegerValue{kind: 3}
			}
		case registerIRLoadLiteral:
			value, ok := valueFromObject(instruction.value)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRLoadInstanceVar:
			value, ok := valueFromObject(core.DynamicInstanceVar(receiver, instruction.name))
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRMove:
			registers[instruction.dst] = registers[instruction.left]
		case registerIRStoreLocal:
			local := int(instruction.param)
			if local < 0 || local >= len(locals) {
				return nil, false
			}
			locals[local] = registers[instruction.left]
			localSet[local] = true
		case registerIREqual:
			left := registers[instruction.left]
			right := registers[instruction.right]
			if (left.kind == 0 || right.kind == 0) && !vm.fusedIntegerOperationAvailable(compiler.OpEqual) {
				return nil, false
			}
			if left.kind != right.kind || (left.kind == 0 && left.value != right.value) {
				registers[instruction.dst] = registerIRIntegerValue{kind: 1}
			} else {
				registers[instruction.dst] = registerIRIntegerValue{kind: 2}
			}
		case registerIRCompare:
			left := registers[instruction.left]
			right := registers[instruction.right]
			if left.kind != 0 || right.kind != 0 {
				return nil, false
			}
			if !vm.fusedIntegerOperationAvailable(instruction.opcode) {
				return nil, false
			}
			matched := false
			switch instruction.opcode {
			case compiler.OpGreaterThan:
				matched = left.value > right.value
			case compiler.OpGreaterThanOrEqual:
				matched = left.value >= right.value
			case compiler.OpLessThan:
				matched = left.value < right.value
			case compiler.OpLessThanOrEqual:
				matched = left.value <= right.value
			default:
				return nil, false
			}
			if matched {
				registers[instruction.dst] = registerIRIntegerValue{kind: 2}
			} else {
				registers[instruction.dst] = registerIRIntegerValue{kind: 1}
			}
		case registerIRBinary:
			left := registers[instruction.left]
			right := registers[instruction.right]
			if left.kind != 0 || right.kind != 0 {
				return nil, false
			}
			if !vm.fusedIntegerOperationAvailable(instruction.opcode) {
				return nil, false
			}
			var result int64
			switch instruction.opcode {
			case compiler.OpAdd:
				if (right.value > 0 && left.value > math.MaxInt64-right.value) || (right.value < 0 && left.value < math.MinInt64-right.value) {
					return nil, false
				}
				result = left.value + right.value
			case compiler.OpSub:
				if (right.value < 0 && left.value > math.MaxInt64+right.value) || (right.value > 0 && left.value < math.MinInt64+right.value) {
					return nil, false
				}
				result = left.value - right.value
			case compiler.OpMul:
				result = left.value * right.value
				if (left.value == -1 && right.value == math.MinInt64) || (right.value == -1 && left.value == math.MinInt64) || (left.value != 0 && result/left.value != right.value) {
					return nil, false
				}
			case compiler.OpMod:
				if right.value == 0 {
					return nil, false
				}
				result = left.value % right.value
				if result != 0 && (result < 0) != (right.value < 0) {
					result += right.value
				}
			default:
				return nil, false
			}
			registers[instruction.dst] = registerIRIntegerValue{value: result}
		case registerIRJumpTruthy:
			if registers[instruction.left].kind == 0 || registers[instruction.left].kind == 2 {
				pc = instruction.target
				continue
			}
		case registerIRJumpNotTruthy:
			if registers[instruction.left].kind == 1 || registers[instruction.left].kind == 3 {
				pc = instruction.target
				continue
			}
		case registerIRJumpNotNil:
			if registers[instruction.left].kind != 3 {
				pc = instruction.target
				continue
			}
		case registerIRJump:
			pc = instruction.target
			continue
		case registerIRReturn:
			value := registers[instruction.left]
			switch value.kind {
			case 0:
				return core.NewIntegerValue(value.value), true
			case 1:
				return core.R.FalseVal, true
			case 2:
				return core.R.TrueVal, true
			case 3:
				return core.R.NilVal, true
			}
			return nil, false
		default:
			return nil, false
		}
		pc++
	}
	return nil, false
}

// executeRegisterIRIntegerOnlySingleArg is the hot one-argument block entry.
// The caller has already proven the block shape and integer input; keeping the
// raw int64 in the parameter/local slot avoids materializing an argument slice
// and re-decoding the same EmeraldValue on every Array/Enumerable element.
// It deliberately mirrors the generic integer interpreter and returns false
// on every guard/overflow miss so the caller can replay the ordinary block.
func (vm *VM) executeRegisterIRIntegerOnlySingleArg(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, argument int64) (*object.EmeraldValue, bool) {
	if plan == nil || fn == nil || plan.registers > 16 || len(fn.ParamLocalIndices) != 1 || fn.ParamLocalIndices[0] < 0 || fn.ParamLocalIndices[0] >= 64 {
		return nil, false
	}
	if plan.integerLinear && registerIRIntegerLinearInputMatchesParameter(plan, fn) {
		if result, ok := vm.executeRegisterIRIntegerLinearSingleArg(plan, argument); ok {
			return core.NewIntegerValue(result), true
		}
	}
	var registers [16]registerIRIntegerValue
	var locals [64]registerIRIntegerValue
	var localSet [64]bool
	locals[fn.ParamLocalIndices[0]] = registerIRIntegerValue{value: argument}
	localSet[fn.ParamLocalIndices[0]] = true
	valueFromObject := func(value *object.EmeraldValue) (registerIRIntegerValue, bool) {
		if value == nil {
			return registerIRIntegerValue{kind: 3}, true
		}
		switch value.Type {
		case object.ValueInteger:
			if !smallIntegerValue(value) {
				return registerIRIntegerValue{}, false
			}
			return registerIRIntegerValue{value: value.Data.(int64)}, true
		case object.ValueBool:
			if value.Data.(bool) {
				return registerIRIntegerValue{kind: 2}, true
			}
			return registerIRIntegerValue{kind: 1}, true
		case object.ValueNil:
			return registerIRIntegerValue{kind: 3}, true
		default:
			return registerIRIntegerValue{}, false
		}
	}
	for pc := 0; pc < len(plan.instructions); {
		instruction := plan.instructions[pc]
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param != 0 {
				return nil, false
			}
			registers[instruction.dst] = registerIRIntegerValue{value: argument}
		case registerIRLoadLocal:
			local := int(instruction.param)
			if local < 0 || local >= len(locals) {
				return nil, false
			}
			if localSet[local] {
				registers[instruction.dst] = locals[local]
			} else {
				registers[instruction.dst] = registerIRIntegerValue{kind: 3}
			}
		case registerIRLoadLiteral:
			value, ok := valueFromObject(instruction.value)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRLoadInstanceVar:
			value, ok := valueFromObject(core.DynamicInstanceVar(receiver, instruction.name))
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRMove:
			registers[instruction.dst] = registers[instruction.left]
		case registerIRStoreLocal:
			local := int(instruction.param)
			if local < 0 || local >= len(locals) {
				return nil, false
			}
			locals[local] = registers[instruction.left]
			localSet[local] = true
		case registerIREqual:
			left := registers[instruction.left]
			right := registers[instruction.right]
			if (left.kind == 0 || right.kind == 0) && !vm.fusedIntegerOperationAvailable(compiler.OpEqual) {
				return nil, false
			}
			if left.kind != right.kind || (left.kind == 0 && left.value != right.value) {
				registers[instruction.dst] = registerIRIntegerValue{kind: 1}
			} else {
				registers[instruction.dst] = registerIRIntegerValue{kind: 2}
			}
		case registerIRCompare:
			left := registers[instruction.left]
			right := registers[instruction.right]
			if left.kind != 0 || right.kind != 0 || !vm.fusedIntegerOperationAvailable(instruction.opcode) {
				return nil, false
			}
			matched := false
			switch instruction.opcode {
			case compiler.OpGreaterThan:
				matched = left.value > right.value
			case compiler.OpGreaterThanOrEqual:
				matched = left.value >= right.value
			case compiler.OpLessThan:
				matched = left.value < right.value
			case compiler.OpLessThanOrEqual:
				matched = left.value <= right.value
			default:
				return nil, false
			}
			if matched {
				registers[instruction.dst] = registerIRIntegerValue{kind: 2}
			} else {
				registers[instruction.dst] = registerIRIntegerValue{kind: 1}
			}
		case registerIRBinary:
			left := registers[instruction.left]
			right := registers[instruction.right]
			if left.kind != 0 || right.kind != 0 || !vm.fusedIntegerOperationAvailable(instruction.opcode) {
				return nil, false
			}
			var result int64
			switch instruction.opcode {
			case compiler.OpAdd:
				if (right.value > 0 && left.value > math.MaxInt64-right.value) || (right.value < 0 && left.value < math.MinInt64-right.value) {
					return nil, false
				}
				result = left.value + right.value
			case compiler.OpSub:
				if (right.value < 0 && left.value > math.MaxInt64+right.value) || (right.value > 0 && left.value < math.MinInt64+right.value) {
					return nil, false
				}
				result = left.value - right.value
			case compiler.OpMul:
				result = left.value * right.value
				if (left.value == -1 && right.value == math.MinInt64) || (right.value == -1 && left.value == math.MinInt64) || (left.value != 0 && result/left.value != right.value) {
					return nil, false
				}
			case compiler.OpMod:
				if right.value == 0 {
					return nil, false
				}
				result = left.value % right.value
				if result != 0 && (result < 0) != (right.value < 0) {
					result += right.value
				}
			default:
				return nil, false
			}
			registers[instruction.dst] = registerIRIntegerValue{value: result}
		case registerIRJumpTruthy:
			if registers[instruction.left].kind == 0 || registers[instruction.left].kind == 2 {
				pc = instruction.target
				continue
			}
		case registerIRJumpNotTruthy:
			if registers[instruction.left].kind == 1 || registers[instruction.left].kind == 3 {
				pc = instruction.target
				continue
			}
		case registerIRJumpNotNil:
			if registers[instruction.left].kind != 3 {
				pc = instruction.target
				continue
			}
		case registerIRJump:
			pc = instruction.target
			continue
		case registerIRReturn:
			value := registers[instruction.left]
			switch value.kind {
			case 0:
				return core.NewIntegerValue(value.value), true
			case 1:
				return core.R.FalseVal, true
			case 2:
				return core.R.TrueVal, true
			case 3:
				return core.R.NilVal, true
			}
			return nil, false
		default:
			return nil, false
		}
		pc++
	}
	return nil, false
}

func (vm *VM) executeRegisterIRIntegerLinearSingleArg(plan *registerIRPlan, argument int64) (int64, bool) {
	if vm == nil || plan == nil || !plan.integerLinear {
		return 0, false
	}
	if plan.integerLinearKind == 1 {
		return applyRegisterIRIntegerLinearOp(vm, plan.integerLinearOpA, argument, plan.integerLinearConstA)
	}
	if plan.integerLinearKind == 2 {
		value, ok := applyRegisterIRIntegerLinearOp(vm, plan.integerLinearOpA, argument, plan.integerLinearConstA)
		if !ok {
			return 0, false
		}
		return applyRegisterIRIntegerLinearOp(vm, plan.integerLinearOpB, value, plan.integerLinearConstB)
	}
	var registers [16]int64
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param != 0 {
				return 0, false
			}
			registers[instruction.dst] = argument
		case registerIRLoadLiteral:
			if instruction.value == nil || instruction.value.Type != object.ValueInteger || !smallIntegerValue(instruction.value) {
				return 0, false
			}
			registers[instruction.dst] = instruction.value.Data.(int64)
		case registerIRBinary:
			if !vm.fusedIntegerOperationAvailable(instruction.opcode) {
				return 0, false
			}
			left := registers[instruction.left]
			right := registers[instruction.right]
			var result int64
			switch instruction.opcode {
			case compiler.OpAdd:
				if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
					return 0, false
				}
				result = left + right
			case compiler.OpSub:
				if (right < 0 && left > math.MaxInt64+right) || (right > 0 && left < math.MinInt64+right) {
					return 0, false
				}
				result = left - right
			case compiler.OpMul:
				result = left * right
				if (left == -1 && right == math.MinInt64) || (right == -1 && left == math.MinInt64) || (left != 0 && result/left != right) {
					return 0, false
				}
			case compiler.OpMod:
				if right == 0 {
					return 0, false
				}
				result = left % right
				if result != 0 && (result < 0) != (right < 0) {
					result += right
				}
			default:
				return 0, false
			}
			registers[instruction.dst] = result
		case registerIRReturn:
			return registers[instruction.left], true
		default:
			return 0, false
		}
	}
	return 0, false
}

func applyRegisterIRIntegerLinearOp(vm *VM, opcode compiler.Opcode, left, right int64) (int64, bool) {
	if vm == nil || !vm.fusedIntegerOperationAvailable(opcode) {
		return 0, false
	}
	return applyRegisterIRIntegerLinearOpRaw(opcode, left, right)
}

func applyRegisterIRIntegerLinearOpRaw(opcode compiler.Opcode, left, right int64) (int64, bool) {
	switch opcode {
	case compiler.OpAdd:
		if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
			return 0, false
		}
		return left + right, true
	case compiler.OpSub:
		if (right < 0 && left > math.MaxInt64+right) || (right > 0 && left < math.MinInt64+right) {
			return 0, false
		}
		return left - right, true
	case compiler.OpMul:
		result := left * right
		if (left == -1 && right == math.MinInt64) || (right == -1 && left == math.MinInt64) || (left != 0 && result/left != right) {
			return 0, false
		}
		return result, true
	case compiler.OpMod:
		if right == 0 {
			return 0, false
		}
		result := left % right
		if result != 0 && (result < 0) != (right < 0) {
			result += right
		}
		return result, true
	case compiler.OpBitAnd:
		return left & right, true
	default:
		return 0, false
	}
}

func (vm *VM) executeRegisterIRInstructions(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue, frame *Frame, allowSendCache ...bool) (*object.EmeraldValue, bool) {
	return vm.executeRegisterIRInstructionsWithFree(plan, receiver, args, frame, nil, allowSendCache...)
}

// executeRegisterIRInstructionsWithFree is the same Register IR executor with
// an optional closure-free value slice for the narrow frameless block tier.
// Supplying free values avoids manufacturing a synthetic Ruby Frame merely to
// read immutable captures; all other frame-dependent operations still reject.
func (vm *VM) executeRegisterIRInstructionsWithFree(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue, frame *Frame, free []*object.EmeraldValue, allowSendCache ...bool) (*object.EmeraldValue, bool) {
	return vm.executeRegisterIRInstructionsWithFreeMode(plan, receiver, args, frame, free, false, false, allowSendCache...)
}

// executeRegisterIRInstructionsWithFreeTrustedArrayIndex is used only by an
// already-admitted repeated Array callback.  The caller proves that the plan
// contains no Ruby send/control-flow edge, and that Array#[] is still the
// builtin implementation, so the per-element method-generation probe can be
// omitted while the exact integer index operation remains guarded.
func (vm *VM) executeRegisterIRInstructionsWithFreeTrustedArrayIndex(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue, frame *Frame, free []*object.EmeraldValue, allowSendCache ...bool) (*object.EmeraldValue, bool) {
	return vm.executeRegisterIRInstructionsWithFreeMode(plan, receiver, args, frame, free, true, false, allowSendCache...)
}

// executeRegisterIRInstructionsWithFreeTrustedNativeRegion is used by a
// reusable framed collection callback after its first element has populated
// every send edge. The region admits only pure/native query sends; a receiver
// or generation miss returns before a user-defined callee can run, so the
// collection helper can replay the current suffix through the normal path.
func (vm *VM) executeRegisterIRInstructionsWithFreeTrustedNativeRegion(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue, frame *Frame, free []*object.EmeraldValue, allowSendCache ...bool) (*object.EmeraldValue, bool) {
	if !registerIRTrustedFramedNativeRegionEnabled {
		return vm.executeRegisterIRInstructionsWithFree(plan, receiver, args, frame, free, allowSendCache...)
	}
	return vm.executeRegisterIRInstructionsWithFreeMode(plan, receiver, args, frame, free, false, true, allowSendCache...)
}

// executeRegisterIRInstructionsWithFreeBatchSend is used by a repeated
// callback after its outer Frame and method-generation guard are established.
// It only bypasses the generic send probe for an instruction whose first
// complete iteration proved a stable cached entry; all other instructions use
// the normal executor immediately, so a miss cannot replay a partial iteration.
func (vm *VM) executeRegisterIRInstructionsWithFreeBatchSend(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue, frame *Frame, free []*object.EmeraldValue, generation uint64, batchSendMask []bool, allowSendCache ...bool) (*object.EmeraldValue, bool) {
	if !registerIRBatchSendEnabled || generation == 0 {
		return vm.executeRegisterIRInstructionsWithFree(plan, receiver, args, frame, free, allowSendCache...)
	}
	return vm.executeRegisterIRInstructionsWithFreeModeBatch(plan, receiver, args, frame, free, false, false, generation, batchSendMask, allowSendCache...)
}

func (vm *VM) executeRegisterIRInstructionsWithFreeMode(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue, frame *Frame, free []*object.EmeraldValue, trustedArrayIndex bool, trustedNativeRegion bool, allowSendCache ...bool) (*object.EmeraldValue, bool) {
	return vm.executeRegisterIRInstructionsWithFreeModeBatch(plan, receiver, args, frame, free, trustedArrayIndex, trustedNativeRegion, 0, nil, allowSendCache...)
}

func (vm *VM) executeRegisterIRInstructionsWithFreeModeBatch(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue, frame *Frame, free []*object.EmeraldValue, trustedArrayIndex bool, trustedNativeRegion bool, batchGeneration uint64, batchSendMask []bool, allowSendCache ...bool) (*object.EmeraldValue, bool) {
	if frame == nil && plan != nil && plan.hasSends && !plan.hasBranches {
		return vm.executeRegisterIRNoFrameLinear(plan, receiver, args, free, trustedArrayIndex)
	}
	vm.prepareRegisterIRFrozenStrings(plan, frame)
	var registers [16]*object.EmeraldValue
	cacheSends := len(allowSendCache) > 0 && allowSendCache[0]
	noFrame := frame == nil && plan != nil && plan.hasSends
	if !plan.hasBranches {
		for pc := 0; pc < len(plan.instructions); pc++ {
			instruction := plan.instructions[pc]
			if trustedArrayIndex && instruction.op == registerIRArray {
				if dst, result, ok := executeRegisterIRArrayLiteralIndexFold(plan, pc, &registers); ok {
					registers[dst] = result
					pc += 2
					continue
				}
			}
			switch instruction.op {
			case registerIRLoadParam:
				if int(instruction.param) >= len(args) {
					return nil, false
				}
				registers[instruction.dst] = args[instruction.param]
			case registerIRLoadLocal:
				if !vm.executeRegisterIRLoadLocal(frame, instruction, &registers) {
					return nil, false
				}
			case registerIRLoadLiteral:
				registers[instruction.dst] = instruction.value
			case registerIRLoadConstantValue:
				if frame == nil {
					return nil, false
				}
				registers[instruction.dst] = vm.constantValue(instruction.value, frame)
			case registerIRLoadConstant:
				value, ok := vm.executeRegisterIRLoadConstant(frame, instruction)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = value
				if value != nil && value.Type == object.ValueException {
					return value, true
				}
			case registerIRLoadScopedConstant:
				value, ok := vm.executeRegisterIRLoadScopedConstant(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = value
				if value != nil && value.Type == object.ValueException {
					return value, true
				}
			case registerIRLoadCapture:
				value, ok := vm.executeRegisterIRLoadCapture(frame, instruction)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = value
			case registerIRClosure:
				value, ok := vm.executeRegisterIRClosure(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = value
			case registerIRLoadInstanceVar:
				value := core.DynamicInstanceVar(receiver, instruction.name)
				if value == nil {
					value = core.R.NilVal
				}
				registers[instruction.dst] = value
			case registerIRDefinedInstanceVar:
				value, ok := executeRegisterIRDefinedInstanceVar(receiver, instruction.name)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = value
			case registerIRSetStringEncoding:
				if !vm.executeRegisterIRSetStringEncoding(frame, instruction, &registers) {
					return nil, false
				}
			case registerIRStoreInstanceVar:
				result, ok := vm.executeRegisterIRStoreInstanceVar(frame, receiver, instruction, &registers)
				if !ok {
					return nil, false
				}
				if result != nil {
					return result, true
				}
			case registerIRArray:
				if !vm.executeRegisterIRArray(frame, instruction, &registers) {
					return nil, false
				}
			case registerIRSplatToArray:
				result, executed := vm.executeRegisterIRSplatToArray(frame, instruction, &registers)
				if !executed {
					return nil, false
				}
				if result != nil {
					return result, true
				}
			case registerIRHash:
				if !vm.executeRegisterIRHash(frame, instruction, &registers) {
					return nil, false
				}
			case registerIRHashMerge:
				result, ok := vm.executeRegisterIRHashMerge(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
				if result != nil && result.Type == object.ValueException {
					return result, true
				}
			case registerIRMultiAssignPrepare:
				result, ok := vm.executeRegisterIRMultiAssignPrepare(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
				if result != nil && result.Type == object.ValueException {
					return result, true
				}
			case registerIRMultiAssignExtract:
				result, ok := vm.executeRegisterIRMultiAssignExtract(instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
			case registerIRMultiAssignCheckToAry:
				result, ok := vm.executeRegisterIRMultiAssignCheckToAry(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				if result != nil && result.Type == object.ValueException {
					return result, true
				}
			case registerIRMarkKeywordHash:
				result, ok := vm.executeRegisterIRMarkKeywordHash(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
			case registerIRRange:
				result, ok := vm.executeRegisterIRRange(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
			case registerIRBlockGiven:
				if frame == nil {
					return nil, false
				}
				registers[instruction.dst] = core.R.FalseVal
				if vm.currentBlock != nil || frame.Block != nil {
					registers[instruction.dst] = core.R.TrueVal
				}
			case registerIRLoadSelf:
				registers[instruction.dst] = receiver
			case registerIRLoadFree:
				if frame == nil {
					if int(instruction.param) >= len(free) {
						return nil, false
					}
					registers[instruction.dst] = derefClosureValue(free[instruction.param])
				} else {
					if frame.Closure == nil || int(instruction.param) >= len(frame.Closure.Free) {
						return nil, false
					}
					registers[instruction.dst] = derefClosureValue(frame.Closure.Free[instruction.param])
				}
			case registerIRMove:
				registers[instruction.dst] = registers[instruction.left]
			case registerIRSwap:
				registers[instruction.left], registers[instruction.right] = registers[instruction.right], registers[instruction.left]
			case registerIRBang:
				registers[instruction.dst] = registerIRBangValue(registers[instruction.left])
			case registerIRNeg:
				result, stop := vm.executeRegisterIRNeg(frame, instruction, &registers)
				registers[instruction.dst] = result
				if stop {
					return result, true
				}
			case registerIRNotEqual:
				result, stop := vm.executeRegisterIRNotEqual(frame, instruction, &registers)
				registers[instruction.dst] = result
				if stop {
					return result, true
				}
			case registerIRStoreLocal:
				if !vm.executeRegisterIRStoreLocal(frame, instruction, &registers) {
					return nil, false
				}
			case registerIRStoreFree:
				if !vm.executeRegisterIRStoreFree(frame, instruction, &registers) {
					return nil, false
				}
			case registerIREqual:
				left := registers[instruction.left]
				right := registers[instruction.right]
				if !registerIRImmediateEqualityValue(left) || !registerIRImmediateEqualityValue(right) {
					return nil, false
				}
				registers[instruction.dst] = vm.equals(left, right)
			case registerIRDynamicEqual:
				result, stop := vm.executeRegisterIRDynamicEqual(frame, instruction, &registers)
				registers[instruction.dst] = result
				if stop {
					return result, true
				}
			case registerIRCompare:
				result, ok := vm.executeRegisterIRIntegerComparison(registers[instruction.left], registers[instruction.right], instruction.opcode)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
			case registerIRDynamicCompare:
				result, stop := vm.executeRegisterIRDynamicCompare(frame, instruction, &registers)
				if !stop {
					return nil, false
				}
				registers[instruction.dst] = result
				if result != nil && result.Type == object.ValueException {
					return result, true
				}
			case registerIRBinary:
				var result *object.EmeraldValue
				var stop bool
				if noFrame {
					var ok bool
					result, ok = vm.executeRegisterIRNoFrameBinary(instruction, &registers)
					if !ok {
						return nil, false
					}
				} else {
					result, stop = vm.executeRegisterIRBinary(frame, instruction, &registers)
				}
				registers[instruction.dst] = result
				if stop {
					return result, true
				}
			case registerIRSend:
				var result *object.EmeraldValue
				var stop bool
				if noFrame {
					var ok bool
					result, ok = vm.executeRegisterIRInlineSend(instruction, &registers)
					if !ok {
						return nil, false
					}
				} else if trustedNativeRegion {
					result, stop = vm.executeRegisterIRTrustedDirectSend(instruction, &registers, false)
					if !stop && result == nil {
						return nil, false
					}
				} else if batchGeneration != 0 && pc < len(batchSendMask) && batchSendMask[pc] {
					result, stop = vm.executeRegisterIRBatchSend(frame, instruction, &registers, batchGeneration, cacheSends)
				} else {
					result, stop = vm.executeRegisterIRSend(frame, instruction, &registers, cacheSends)
				}
				registers[instruction.dst] = result
				if stop {
					return result, true
				}
			case registerIRYield:
				result, stop := vm.executeRegisterIRYield(frame, instruction, &registers)
				if stop {
					return result, true
				}
				registers[instruction.dst] = result
			case registerIRLogicalSendAssignment:
				result, stop := vm.executeRegisterIRLogicalSendAssignment(frame, instruction, &registers)
				if !stop && result == nil {
					return nil, false
				}
				registers[instruction.dst] = result
				if stop {
					return result, true
				}
			case registerIRIndex:
				result, ok := vm.executeRegisterIRIndex(frame, instruction, &registers, trustedArrayIndex)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
				if result.Type == object.ValueException {
					return result, true
				}
			case registerIRSlice:
				result, ok := vm.executeRegisterIRSlice(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
				if result.Type == object.ValueException {
					return result, true
				}
			case registerIRIndexAssign:
				result, ok := vm.executeRegisterIRIndexAssign(frame, instruction, &registers)
				if !ok {
					return nil, false
				}
				registers[instruction.dst] = result
				if result != nil && result.Type == object.ValueException {
					return result, true
				}
			case registerIRRaise:
				return vm.executeRegisterIRRaise(frame, instruction, &registers)
			case registerIRReturn:
				result, stop := vm.executeRegisterIRReturn(frame, instruction, &registers)
				if stop {
					return result, true
				}
			default:
				return nil, false
			}
		}
		return nil, false
	}
	for pc := 0; pc < len(plan.instructions); {
		instruction := plan.instructions[pc]
		switch instruction.op {
		case registerIRLoadParam:
			if int(instruction.param) >= len(args) {
				return nil, false
			}
			registers[instruction.dst] = args[instruction.param]
		case registerIRLoadLocal:
			if !vm.executeRegisterIRLoadLocal(frame, instruction, &registers) {
				return nil, false
			}
		case registerIRLoadLiteral:
			registers[instruction.dst] = instruction.value
		case registerIRLoadConstantValue:
			if frame == nil {
				return nil, false
			}
			registers[instruction.dst] = vm.constantValue(instruction.value, frame)
		case registerIRLoadConstant:
			value, ok := vm.executeRegisterIRLoadConstant(frame, instruction)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
			if value != nil && value.Type == object.ValueException {
				return value, true
			}
		case registerIRLoadScopedConstant:
			value, ok := vm.executeRegisterIRLoadScopedConstant(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
			if value != nil && value.Type == object.ValueException {
				return value, true
			}
		case registerIRLoadCapture:
			value, ok := vm.executeRegisterIRLoadCapture(frame, instruction)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRClosure:
			value, ok := vm.executeRegisterIRClosure(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRLoadInstanceVar:
			value := core.DynamicInstanceVar(receiver, instruction.name)
			if value == nil {
				value = core.R.NilVal
			}
			registers[instruction.dst] = value
		case registerIRDefinedInstanceVar:
			value, ok := executeRegisterIRDefinedInstanceVar(receiver, instruction.name)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRSetStringEncoding:
			if !vm.executeRegisterIRSetStringEncoding(frame, instruction, &registers) {
				return nil, false
			}
		case registerIRStoreInstanceVar:
			result, ok := vm.executeRegisterIRStoreInstanceVar(frame, receiver, instruction, &registers)
			if !ok {
				return nil, false
			}
			if result != nil {
				return result, true
			}
		case registerIRArray:
			if !vm.executeRegisterIRArray(frame, instruction, &registers) {
				return nil, false
			}
		case registerIRSplatToArray:
			result, executed := vm.executeRegisterIRSplatToArray(frame, instruction, &registers)
			if !executed {
				return nil, false
			}
			if result != nil {
				return result, true
			}
		case registerIRHash:
			if !vm.executeRegisterIRHash(frame, instruction, &registers) {
				return nil, false
			}
		case registerIRHashMerge:
			result, ok := vm.executeRegisterIRHashMerge(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
		case registerIRMultiAssignPrepare:
			result, ok := vm.executeRegisterIRMultiAssignPrepare(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
		case registerIRMultiAssignExtract:
			result, ok := vm.executeRegisterIRMultiAssignExtract(instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRMultiAssignCheckToAry:
			result, ok := vm.executeRegisterIRMultiAssignCheckToAry(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
		case registerIRMarkKeywordHash:
			result, ok := vm.executeRegisterIRMarkKeywordHash(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRRange:
			result, ok := vm.executeRegisterIRRange(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRBlockGiven:
			if frame == nil {
				return nil, false
			}
			registers[instruction.dst] = core.R.FalseVal
			if vm.currentBlock != nil || frame.Block != nil {
				registers[instruction.dst] = core.R.TrueVal
			}
		case registerIRLoadSelf:
			registers[instruction.dst] = receiver
		case registerIRLoadFree:
			if frame == nil {
				if int(instruction.param) >= len(free) {
					return nil, false
				}
				registers[instruction.dst] = derefClosureValue(free[instruction.param])
			} else {
				if frame.Closure == nil || int(instruction.param) >= len(frame.Closure.Free) {
					return nil, false
				}
				registers[instruction.dst] = derefClosureValue(frame.Closure.Free[instruction.param])
			}
		case registerIRMove:
			registers[instruction.dst] = registers[instruction.left]
		case registerIRSwap:
			registers[instruction.left], registers[instruction.right] = registers[instruction.right], registers[instruction.left]
		case registerIRBang:
			registers[instruction.dst] = registerIRBangValue(registers[instruction.left])
		case registerIRNeg:
			result, stop := vm.executeRegisterIRNeg(frame, instruction, &registers)
			registers[instruction.dst] = result
			if stop {
				return result, true
			}
		case registerIRNotEqual:
			result, stop := vm.executeRegisterIRNotEqual(frame, instruction, &registers)
			registers[instruction.dst] = result
			if stop {
				return result, true
			}
		case registerIRStoreLocal:
			if !vm.executeRegisterIRStoreLocal(frame, instruction, &registers) {
				return nil, false
			}
		case registerIRStoreFree:
			if !vm.executeRegisterIRStoreFree(frame, instruction, &registers) {
				return nil, false
			}
		case registerIREqual:
			left := registers[instruction.left]
			right := registers[instruction.right]
			if !registerIRImmediateEqualityValue(left) || !registerIRImmediateEqualityValue(right) {
				return nil, false
			}
			registers[instruction.dst] = vm.equals(left, right)
		case registerIRDynamicEqual:
			result, stop := vm.executeRegisterIRDynamicEqual(frame, instruction, &registers)
			registers[instruction.dst] = result
			if stop {
				return result, true
			}
		case registerIRCompare:
			result, ok := vm.executeRegisterIRIntegerComparison(registers[instruction.left], registers[instruction.right], instruction.opcode)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRDynamicCompare:
			result, stop := vm.executeRegisterIRDynamicCompare(frame, instruction, &registers)
			if !stop {
				return nil, false
			}
			registers[instruction.dst] = result
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
		case registerIRBinary:
			var result *object.EmeraldValue
			var stop bool
			if noFrame {
				var ok bool
				result, ok = vm.executeRegisterIRNoFrameBinary(instruction, &registers)
				if !ok {
					return nil, false
				}
			} else {
				result, stop = vm.executeRegisterIRBinary(frame, instruction, &registers)
			}
			registers[instruction.dst] = result
			if stop {
				return result, true
			}
		case registerIRSend:
			var result *object.EmeraldValue
			var stop bool
			if noFrame {
				var ok bool
				result, ok = vm.executeRegisterIRInlineSend(instruction, &registers)
				if !ok {
					return nil, false
				}
			} else if trustedNativeRegion {
				result, stop = vm.executeRegisterIRTrustedDirectSend(instruction, &registers, false)
				if !stop && result == nil {
					return nil, false
				}
			} else if batchGeneration != 0 && pc < len(batchSendMask) && batchSendMask[pc] {
				result, stop = vm.executeRegisterIRBatchSend(frame, instruction, &registers, batchGeneration, cacheSends)
			} else {
				result, stop = vm.executeRegisterIRSend(frame, instruction, &registers, cacheSends)
			}
			registers[instruction.dst] = result
			if stop {
				return result, true
			}
		case registerIRYield:
			result, stop := vm.executeRegisterIRYield(frame, instruction, &registers)
			if stop {
				return result, true
			}
			registers[instruction.dst] = result
		case registerIRLogicalSendAssignment:
			result, stop := vm.executeRegisterIRLogicalSendAssignment(frame, instruction, &registers)
			if !stop && result == nil {
				return nil, false
			}
			registers[instruction.dst] = result
			if stop {
				return result, true
			}
		case registerIRIndex:
			result, ok := vm.executeRegisterIRIndex(frame, instruction, &registers, trustedArrayIndex)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
			if result.Type == object.ValueException {
				return result, true
			}
		case registerIRSlice:
			result, ok := vm.executeRegisterIRSlice(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
			if result.Type == object.ValueException {
				return result, true
			}
		case registerIRIndexAssign:
			result, ok := vm.executeRegisterIRIndexAssign(frame, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
			if result != nil && result.Type == object.ValueException {
				return result, true
			}
		case registerIRRaise:
			return vm.executeRegisterIRRaise(frame, instruction, &registers)
		case registerIRJump:
			pc = instruction.target
			continue
		case registerIRJumpTruthy:
			if registers[instruction.left].IsTruthy() {
				pc = instruction.target
				continue
			}
		case registerIRJumpNotTruthy:
			if !registers[instruction.left].IsTruthy() {
				pc = instruction.target
				continue
			}
		case registerIRJumpNotNil:
			if registers[instruction.left] != nil && registers[instruction.left].Type != object.ValueNil {
				pc = instruction.target
				continue
			}
		case registerIRJumpLocalPresent:
			if frame == nil {
				return nil, false
			}
			stackIndex := frame.Bp + 1 + int(instruction.param)
			if stackIndex >= 0 && stackIndex < len(vm.stack) && localSlotPresent(vm.stack[stackIndex]) {
				pc = instruction.target
				continue
			}
		case registerIRReturn:
			result, stop := vm.executeRegisterIRReturn(frame, instruction, &registers)
			if stop {
				return result, true
			}
		default:
			return nil, false
		}
		pc++
	}
	return nil, false
}

// registerIRBatchFixedBytecodeEligible proves the positional shape that
// executeCachedFixedArityRubyBytecodeTrusted is allowed to skip. The cached
// entry retains the callee's default prologue, so omitted optional parameters
// are safe as well; rest, keyword, destructuring and refinement protocols stay
// on the full path.
func registerIRBatchFixedBytecodeEligible(methodObj *object.Method, argc uint8) bool {
	if methodObj == nil || methodObj.DispatchOwner != nil || methodObj.Ruby2Keywords ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") || methodUsesRefinements(methodObj) {
		return false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	return ok && fn != nil && int(argc) <= len(fn.Params) && len(fn.ParamLocalIndices) == len(fn.Params) &&
		cachedPositionalMethodArgumentShape(fn)
}

func registerIRBatchCacheHasFastEntry(cache *registerIRSendCache, generation uint64, argc uint8) bool {
	if cache == nil || cache.generation != generation {
		return false
	}
	return cache.framedNativeFn != nil || cache.nativeFn != nil ||
		cache.secondFramedNativeFn != nil || cache.secondNativeFn != nil ||
		registerIRBatchFixedBytecodeEligible(cache.method, argc) ||
		registerIRBatchFixedBytecodeEligible(cache.secondMethod, argc)
}

// registerIRBatchSendMask snapshots native/fixed-bytecode candidates after a
// callback's first ordinary iteration has populated its call-site caches. A
// nil result is intentional: later iterations then use the old executor
// without paying a per-send batch probe when this body has no stable entries.
func registerIRBatchSendMask(plan *registerIRPlan, generation uint64) []bool {
	if plan == nil || generation == 0 || !plan.hasSends {
		return nil
	}
	hasCandidate := false
	for _, instruction := range plan.instructions {
		if instruction.op != registerIRSend || instruction.blockPresent || instruction.splatIndex != 255 ||
			instruction.opcode != compiler.OpSend || instruction.implicit || instruction.argc > 4 || instruction.cache == nil {
			continue
		}
		if registerIRBatchCacheHasFastEntry(instruction.cache, generation, instruction.argc) {
			hasCandidate = true
			break
		}
	}
	if !hasCandidate {
		return nil
	}
	mask := make([]bool, len(plan.instructions))
	for index, instruction := range plan.instructions {
		if instruction.op != registerIRSend || instruction.blockPresent || instruction.splatIndex != 255 ||
			instruction.opcode != compiler.OpSend || instruction.implicit || instruction.argc > 4 || instruction.cache == nil {
			continue
		}
		mask[index] = registerIRBatchCacheHasFastEntry(instruction.cache, generation, instruction.argc)
	}
	return mask
}

// prepareRegisterIRFrozenStrings resolves source-level frozen literals through
// the VM's normal interning/encoding path once per VM-local IR plan.  The plan
// cache is VM-local, so rewriting the instruction is safe and removes both the
// constantValue call and the allocation from every subsequent invocation.
func (vm *VM) prepareRegisterIRFrozenStrings(plan *registerIRPlan, frame *Frame) {
	if vm == nil || plan == nil || frame == nil || !plan.hasFrozenStrings {
		return
	}
	for index := range plan.instructions {
		instruction := &plan.instructions[index]
		if instruction.op != registerIRLoadFrozenString || instruction.value == nil {
			continue
		}
		value := vm.constantValue(instruction.value, frame)
		if value == nil {
			continue
		}
		instruction.op = registerIRLoadLiteral
		instruction.value = value
	}
}

// prepareRegisterIRArrayLiteralIndexFolds resolves every temporary Array
// followed immediately by a literal integer index once per immutable plan.
// The per-element executor then performs only a bounds-free metadata lookup
// and a register read; all shape/type checks remain at block admission.
func prepareRegisterIRArrayLiteralIndexFolds(plan *registerIRPlan) {
	if plan == nil || plan.arrayLiteralIndexFoldsChecked {
		return
	}
	plan.arrayLiteralIndexFoldsChecked = true
	plan.arrayLiteralIndexFolds = make([]registerIRArrayLiteralIndexFold, len(plan.instructions))
	for pc := 0; pc+2 < len(plan.instructions); pc++ {
		arrayInstruction := plan.instructions[pc]
		loadInstruction := plan.instructions[pc+1]
		indexInstruction := plan.instructions[pc+2]
		if arrayInstruction.op != registerIRArray || loadInstruction.op != registerIRLoadLiteral ||
			indexInstruction.op != registerIRIndex || indexInstruction.left != arrayInstruction.dst ||
			indexInstruction.right != loadInstruction.dst || arrayInstruction.argc > 16 ||
			!smallIntegerValue(loadInstruction.value) {
			continue
		}
		position := loadInstruction.value.Data.(int64)
		if position < 0 {
			position += int64(arrayInstruction.argc)
		}
		fold := registerIRArrayLiteralIndexFold{valid: true, resultDst: indexInstruction.dst, sourceOffset: -1}
		if position >= 0 && position < int64(arrayInstruction.argc) {
			fold.sourceOffset = int8(position)
		}
		plan.arrayLiteralIndexFolds[pc] = fold
	}
}

// executeRegisterIRArrayLiteralIndexFold removes the temporary Array object
// from the very narrow `[expr0, expr1, ...][constant_integer]` shape.  The
// surrounding caller has already proved that the plan has no Ruby send or
// control-flow edge and that exact Array#[] is builtin, so the allocation and
// lookup have no observable boundary between them.
func executeRegisterIRArrayLiteralIndexFold(plan *registerIRPlan, pc int, registers *[16]*object.EmeraldValue) (uint8, *object.EmeraldValue, bool) {
	if plan == nil || registers == nil || pc < 0 || pc >= len(plan.arrayLiteralIndexFolds) {
		return 0, nil, false
	}
	fold := plan.arrayLiteralIndexFolds[pc]
	if !fold.valid {
		return 0, nil, false
	}
	result := core.R.NilVal
	if fold.sourceOffset >= 0 {
		result = registers[int(plan.instructions[pc].dst)+int(fold.sourceOffset)]
	}
	return fold.resultDst, result, true
}

// executeRegisterIRNoFrameLinear is the hot straight-line form of Register IR.
// It deliberately has no frame-mode branches or variadic cache flag: callers
// reach it only after all frame-dependent operations have been excluded.
func (vm *VM) executeRegisterIRNoFrameLinear(plan *registerIRPlan, receiver *object.EmeraldValue, args []*object.EmeraldValue, free []*object.EmeraldValue, trustedArrayIndex ...bool) (*object.EmeraldValue, bool) {
	var registers [16]*object.EmeraldValue
	trustedArrayLiteralIndex := len(trustedArrayIndex) > 0 && trustedArrayIndex[0]
	for pc := 0; pc < len(plan.instructions); pc++ {
		instruction := plan.instructions[pc]
		if trustedArrayLiteralIndex && instruction.op == registerIRArray {
			if dst, result, ok := executeRegisterIRArrayLiteralIndexFold(plan, pc, &registers); ok {
				registers[dst] = result
				pc += 2
				continue
			}
		}
		switch instruction.op {
		case registerIRLoadParam:
			if int(instruction.param) >= len(args) {
				return nil, false
			}
			registers[instruction.dst] = args[instruction.param]
		case registerIRLoadLiteral:
			registers[instruction.dst] = instruction.value
		case registerIRLoadInstanceVar:
			value := core.DynamicInstanceVar(receiver, instruction.name)
			if value == nil {
				value = core.R.NilVal
			}
			registers[instruction.dst] = value
		case registerIRLoadSelf:
			registers[instruction.dst] = receiver
		case registerIRLoadFree:
			if int(instruction.param) >= len(free) {
				return nil, false
			}
			registers[instruction.dst] = derefClosureValue(free[instruction.param])
		case registerIRMove:
			registers[instruction.dst] = registers[instruction.left]
		case registerIRSwap:
			registers[instruction.left], registers[instruction.right] = registers[instruction.right], registers[instruction.left]
		case registerIRBang:
			registers[instruction.dst] = registerIRBangValue(registers[instruction.left])
		case registerIRBinary:
			result, ok := vm.executeRegisterIRNoFrameBinary(instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRSend:
			result, ok := vm.executeRegisterIRInlineSendNoFrame(instruction, &registers, false)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRArray:
			if !vm.executeRegisterIRArray(nil, instruction, &registers) {
				return nil, false
			}
		case registerIRHash:
			if !vm.executeRegisterIRHash(nil, instruction, &registers) {
				return nil, false
			}
		case registerIRRange:
			result, ok := vm.executeRegisterIRRange(nil, instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRIndex:
			result, ok := vm.executeRegisterIRIndex(nil, instruction, &registers, trustedArrayIndex...)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = result
		case registerIRReturn:
			return registers[instruction.left], true
		default:
			return nil, false
		}
	}
	return nil, false
}

func (vm *VM) executeRegisterIRIntegerComparison(left, right *object.EmeraldValue, opcode compiler.Opcode) (*object.EmeraldValue, bool) {
	if !smallIntegerValue(left) || !smallIntegerValue(right) {
		return nil, false
	}
	if !vm.fusedIntegerOperationAvailable(opcode) {
		return nil, false
	}
	l := left.Data.(int64)
	r := right.Data.(int64)
	matched := false
	switch opcode {
	case compiler.OpGreaterThan:
		matched = l > r
	case compiler.OpGreaterThanOrEqual:
		matched = l >= r
	case compiler.OpLessThan:
		matched = l < r
	case compiler.OpLessThanOrEqual:
		matched = l <= r
	default:
		return nil, false
	}
	if matched {
		return core.R.TrueVal, true
	}
	return core.R.FalseVal, true
}

func (vm *VM) executeRegisterIRDynamicCompare(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || frame == nil {
		return nil, false
	}
	left := registers[instruction.left]
	right := registers[instruction.right]
	if left == nil {
		left = core.R.NilVal
	}
	if right == nil {
		right = core.R.NilVal
	}
	frame.Ip = instruction.byteIP
	var result *object.EmeraldValue
	switch instruction.opcode {
	case compiler.OpGreaterThan:
		result = vm.greaterThan(left, right)
	case compiler.OpGreaterThanOrEqual:
		result = vm.greaterThanOrEqual(left, right)
		if result == nil {
			gt := vm.greaterThan(left, right)
			if gt != nil && gt.Type == object.ValueException {
				result = gt
				break
			}
			eq := vm.equals(left, right)
			if eq != nil && eq.Type == object.ValueException {
				result = eq
				break
			}
			if (gt != nil && gt.Type == object.ValueBool && gt.Data == true) || (eq != nil && eq.Type == object.ValueBool && eq.Data == true) {
				result = core.R.TrueVal
			} else {
				result = core.R.FalseVal
			}
		}
	case compiler.OpLessThan:
		result = vm.lessThan(left, right)
	case compiler.OpLessThanOrEqual:
		result = vm.lessThanOrEqual(left, right)
		if result == nil {
			lt := vm.lessThan(left, right)
			if lt != nil && lt.Type == object.ValueException {
				result = lt
				break
			}
			eq := vm.equals(left, right)
			if eq != nil && eq.Type == object.ValueException {
				result = eq
				break
			}
			if (lt != nil && lt.Type == object.ValueBool && lt.Data == true) || (eq != nil && eq.Type == object.ValueBool && eq.Data == true) {
				result = core.R.TrueVal
			} else {
				result = core.R.FalseVal
			}
		}
	default:
		return nil, false
	}
	if result == nil {
		return nil, false
	}
	if result.Type == object.ValueException {
		if vm.raiseException(frame, result) {
			return result, true
		}
		vm.returnUnhandledException(frame, result)
		return result, true
	}
	return result, true
}

func (vm *VM) executeRegisterIRNoFrameBinary(instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	left := registers[instruction.left]
	right := registers[instruction.right]
	if instruction.opcode == compiler.OpBitLeftShift && left != nil && right != nil &&
		left.Type == object.ValueString && left.Class == core.R.Classes["String"] && core.AttachedSingletonClass(left) == nil &&
		core.StringAppendUsesBuiltinImplementation() {
		result, handled := core.AppendStringOneFast(left, right)
		if !handled {
			result = core.AppendStringOne(left, right)
		}
		if result == nil || result.Type == object.ValueException {
			return nil, false
		}
		return result, true
	}
	if smallIntegerValue(left) && smallIntegerValue(right) {
		if !vm.fusedIntegerOperationAvailable(instruction.opcode) {
			return nil, false
		}
		value, ok := applyIntegerBinary(instruction.opcode, left.Data.(int64), right.Data.(int64))
		if !ok {
			return nil, false
		}
		return core.NewIntegerValue(value), true
	}
	// String concatenation is safe without a Ruby Frame only for the exact
	// built-in String class. Subclasses and non-String operands must retain the
	// normal dynamic + dispatch and therefore deopt before user code runs.
	if instruction.opcode == compiler.OpAdd && left != nil && right != nil &&
		left.Type == object.ValueString && right.Type == object.ValueString &&
		left.Class == core.R.Classes["String"] && right.Class == core.R.Classes["String"] &&
		core.AttachedSingletonClass(left) == nil &&
		core.StringPlusUsesBuiltinImplementation() {
		result := vm.add(left, right)
		if result == nil || result.Type == object.ValueException {
			return nil, false
		}
		return result, true
	}
	return nil, false
}

func (vm *VM) aggressiveRegisterIRCompare(left, right *object.EmeraldValue, opcode compiler.Opcode) *object.EmeraldValue {
	if left == nil {
		left = core.R.NilVal
	}
	if right == nil {
		right = core.R.NilVal
	}
	switch opcode {
	case compiler.OpGreaterThan:
		return vm.greaterThan(left, right)
	case compiler.OpGreaterThanOrEqual:
		if result := vm.greaterThanOrEqual(left, right); result != nil {
			return result
		}
		greater := vm.greaterThan(left, right)
		if greater != nil && greater.Type == object.ValueException {
			return greater
		}
		equal := vm.equals(left, right)
		if equal != nil && equal.Type == object.ValueException {
			return equal
		}
		if greater != nil && greater.Type == object.ValueBool && greater.Data.(bool) || equal != nil && equal.Type == object.ValueBool && equal.Data.(bool) {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	case compiler.OpLessThan:
		return vm.lessThan(left, right)
	case compiler.OpLessThanOrEqual:
		if result := vm.lessThanOrEqual(left, right); result != nil {
			return result
		}
		less := vm.lessThan(left, right)
		if less != nil && less.Type == object.ValueException {
			return less
		}
		equal := vm.equals(left, right)
		if equal != nil && equal.Type == object.ValueException {
			return equal
		}
		if less != nil && less.Type == object.ValueBool && less.Data.(bool) || equal != nil && equal.Type == object.ValueBool && equal.Data.(bool) {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	default:
		return core.R.NilVal
	}
}

func (vm *VM) aggressiveRegisterIRBinary(instruction registerIRInstruction, registers *[16]*object.EmeraldValue) *object.EmeraldValue {
	left := registers[instruction.left]
	right := registers[instruction.right]
	if left == nil {
		left = core.R.NilVal
	}
	if right == nil {
		right = core.R.NilVal
	}
	switch instruction.opcode {
	case compiler.OpAdd:
		return vm.add(left, right)
	case compiler.OpSub:
		return vm.sub(left, right)
	case compiler.OpMul:
		return vm.mul(left, right)
	case compiler.OpMod:
		return vm.mod(left, right)
	case compiler.OpBitLeftShift:
		return vm.send(left, "<<", []*object.EmeraldValue{right})
	default:
		return core.R.NilVal
	}
}

func (vm *VM) executeRegisterIRNoFrameEqual(left, right *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if left == nil {
		left = core.R.NilVal
	}
	if right == nil {
		right = core.R.NilVal
	}
	if left.Type == object.ValueString || right.Type == object.ValueString {
		if left.Type != object.ValueString || right.Type != object.ValueString ||
			left.Class != core.R.Classes["String"] || right.Class != core.R.Classes["String"] {
			return nil, false
		}
		if left.Data.(string) == right.Data.(string) {
			return core.R.TrueVal, true
		}
		return core.R.FalseVal, true
	}
	if !registerIRImmediateEqualityValue(left) || !registerIRImmediateEqualityValue(right) {
		return nil, false
	}
	if (left.Type == object.ValueInteger || right.Type == object.ValueInteger) && !vm.fusedIntegerOperationAvailable(compiler.OpEqual) {
		return nil, false
	}
	if left.Type != right.Type {
		return core.R.FalseVal, true
	}
	switch value := left.Data.(type) {
	case nil:
		return core.R.TrueVal, true
	case bool:
		return boolValue(value == right.Data.(bool)), true
	case int64:
		return boolValue(value == right.Data.(int64)), true
	case float64:
		return boolValue(value == right.Data.(float64)), true
	case string:
		return boolValue(value == right.Data.(string)), true
	default:
		return nil, false
	}
}

func registerIRBangValue(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil || !value.IsTruthy() {
		return core.R.TrueVal
	}
	return core.R.FalseVal
}

func (vm *VM) executeRegisterIRIndex(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue, trustedArrayIndex ...bool) (*object.EmeraldValue, bool) {
	left := registers[instruction.left]
	index := registers[instruction.right]
	if left == nil || index == nil {
		return nil, false
	}
	if frame == nil {
		return vm.executeRegisterIRDirectIndex(left, index, trustedArrayIndex...)
	}
	result := vm.index(left, index)
	if result == nil {
		return nil, false
	}
	return result, true
}

func (vm *VM) executeRegisterIRMultiAssignPrepare(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || frame == nil || registers == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	prepared, errVal := vm.prepareMultiAssignRHS(registers[instruction.left])
	if errVal != nil {
		if vm.raiseException(frame, errVal) {
			return errVal, true
		}
		vm.returnUnhandledException(frame, errVal)
		return errVal, true
	}
	if prepared == nil {
		return nil, false
	}
	return prepared, true
}

func (vm *VM) executeRegisterIRMultiAssignExtract(instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || registers == nil {
		return nil, false
	}
	array := registers[instruction.left]
	if array == nil || array.Type != object.ValueArray {
		return nil, false
	}
	values, ok := array.Data.([]*object.EmeraldValue)
	if !ok {
		return nil, false
	}
	kind := int(instruction.param)
	index := int(instruction.argc)
	preCount := int(instruction.args[0])
	postCount := int(instruction.args[1])
	switch kind {
	case 1:
		start := preCount
		if start > len(values) {
			start = len(values)
		}
		end := len(values) - postCount
		if end < start {
			end = start
		}
		return vm.arrayValue(append([]*object.EmeraldValue(nil), values[start:end]...)...), true
	case 2:
		start := len(values) - postCount
		if start < preCount {
			start = preCount
		}
		position := start + index
		if position >= 0 && position < len(values) {
			return values[position], true
		}
		return core.R.NilVal, true
	default:
		if index >= 0 && index < len(values) {
			return values[index], true
		}
		return core.R.NilVal, true
	}
}

func (vm *VM) executeRegisterIRMultiAssignCheckToAry(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || frame == nil || registers == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	if errVal := vm.checkMultiAssignToAry(registers[instruction.left]); errVal != nil {
		if vm.raiseException(frame, errVal) {
			return errVal, true
		}
		vm.returnUnhandledException(frame, errVal)
		return errVal, true
	}
	return nil, true
}

func executeRegisterIRDefinedInstanceVar(receiver *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	if receiver == nil || name == "" {
		return nil, false
	}
	if core.DynamicInstanceVar(receiver, name) == nil {
		return core.R.NilVal, true
	}
	return &object.EmeraldValue{
		Type:   object.ValueString,
		Data:   "instance-variable",
		Class:  core.R.Classes["String"],
		Frozen: true,
	}, true
}

func (vm *VM) executeRegisterIRSlice(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, false
	}
	left := registers[instruction.left]
	start := registers[instruction.right]
	length := registers[instruction.args[0]]
	if left == nil {
		left = core.R.NilVal
	}
	if start == nil {
		start = core.R.NilVal
	}
	if length == nil {
		length = core.R.NilVal
	}
	frame.Ip = instruction.byteIP
	result := vm.sliceIndex(left, start, length)
	if result == nil {
		return nil, false
	}
	if result.Type == object.ValueException {
		if vm.raiseException(frame, result) {
			return result, true
		}
		vm.returnUnhandledException(frame, result)
		return result, true
	}
	return result, true
}

// executeRegisterIRDirectSlice handles only exact built-in Array slices in the
// no-frame tier.  Ruby returns a fresh array for Array#slice; copy the selected
// elements instead of exposing the source backing store.  Strings stay on the
// framed path because character offsets depend on the runtime encoding.
func (vm *VM) executeRegisterIRDirectSlice(registers *[16]*object.EmeraldValue, instruction registerIRInstruction) (*object.EmeraldValue, bool) {
	if vm == nil || registers == nil {
		return nil, false
	}
	left := registers[instruction.left]
	start := registers[instruction.right]
	length := registers[instruction.args[0]]
	if left == nil || start == nil || length == nil || left.Type != object.ValueArray ||
		left.Class != core.R.Classes["Array"] || core.AttachedSingletonClass(left) != nil ||
		start.Type != object.ValueInteger || !smallIntegerValue(start) ||
		length.Type != object.ValueInteger || !smallIntegerValue(length) {
		return nil, false
	}
	data, ok := left.Data.([]*object.EmeraldValue)
	if !ok {
		return nil, false
	}
	s := int(start.Data.(int64))
	l := int(length.Data.(int64))
	if l < 0 {
		return core.R.NilVal, true
	}
	if s < 0 {
		s = len(data) + s
	}
	if s < 0 || s > len(data) {
		return core.R.NilVal, true
	}
	end := s + l
	if end < s {
		end = len(data)
	}
	if end > len(data) {
		end = len(data)
	}
	result := append([]*object.EmeraldValue(nil), data[s:end]...)
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: core.R.Classes["Array"]}, true
}

func (vm *VM) executeRegisterIRRange(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	left := registers[instruction.left]
	right := registers[instruction.right]
	if left == nil {
		left = core.R.NilVal
	}
	if right == nil {
		right = core.R.NilVal
	}
	startMissing := instruction.param&2 != 0
	endMissing := instruction.param&4 != 0
	startValue := left
	endValue := right
	var start, end int64
	var startRaw, endRaw float64
	startFloat := false
	endFloat := false
	if startMissing {
		startValue = nil
	}
	if endMissing {
		endValue = nil
	}
	if startValue != nil {
		switch value := startValue.Data.(type) {
		case int64:
			start = value
			startRaw = float64(value)
		case float64:
			start = int64(value)
			startFloat = true
			startRaw = value
		}
	}
	if endValue != nil {
		switch value := endValue.Data.(type) {
		case int64:
			end = value
			endRaw = float64(value)
		case float64:
			end = int64(value)
			endFloat = true
			endRaw = value
		}
	}
	if frame != nil {
		frame.Ip = instruction.byteIP
	}
	return &object.EmeraldValue{
		Type: object.ValueRange,
		Data: &object.RRange{
			Start: start, End: end, StartValue: startValue, EndValue: endValue,
			StartFloat: startFloat, EndFloat: endFloat, StartRaw: startRaw, EndRaw: endRaw,
			StartMissing: startMissing, EndMissing: endMissing, Exclusive: instruction.param&1 != 0,
		},
		Class: core.R.Classes["Range"], Frozen: true,
	}, true
}

func (vm *VM) executeRegisterIRIndexAssign(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	left := registers[instruction.left]
	index := registers[instruction.right]
	value := registers[instruction.args[0]]
	if left == nil {
		left = core.R.NilVal
	}
	if index == nil {
		index = core.R.NilVal
	}
	if value == nil {
		value = core.R.NilVal
	}
	result := vm.indexAssign(left, index, value)
	if result != nil && result.Type == object.ValueException {
		if !vm.raiseException(frame, result) {
			vm.returnUnhandledException(frame, result)
		}
		return result, true
	}
	return value, true
}

func (vm *VM) executeRegisterIRLogicalSendAssignment(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil || instruction.argc > 4 || !instruction.blockPresent {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	receiver := derefClosureValue(registers[instruction.left])
	if receiver == nil {
		receiver = core.R.NilVal
	}
	var argStorage [4]*object.EmeraldValue
	args := argStorage[:int(instruction.argc)]
	for index := range args {
		args[index] = registers[instruction.args[index]]
	}
	current := vm.send(receiver, instruction.name, args)
	if shouldPropagateExceptionValue(current) {
		if vm.raiseException(frame, current) {
			return current, true
		}
		vm.returnUnhandledException(frame, current)
		return current, true
	}
	assign := !current.IsTruthy()
	if instruction.param == 1 {
		assign = current.IsTruthy()
	}
	if !assign {
		return current, false
	}
	assigned := vm.callBlock(derefClosureValue(registers[instruction.block]))
	if shouldPropagateExceptionValue(assigned) {
		if vm.raiseException(frame, assigned) {
			return assigned, true
		}
		vm.returnUnhandledException(frame, assigned)
		return assigned, true
	}
	setterArgs := make([]*object.EmeraldValue, int(instruction.argc)+1)
	copy(setterArgs, args)
	setterArgs[len(args)] = assigned
	setterResult := vm.send(receiver, instruction.setter, setterArgs)
	if shouldPropagateExceptionValue(setterResult) {
		if vm.raiseException(frame, setterResult) {
			return setterResult, true
		}
		vm.returnUnhandledException(frame, setterResult)
		return setterResult, true
	}
	return assigned, false
}

func (vm *VM) executeRegisterIRRaise(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	exception := registers[instruction.left]
	previousException := core.LastException
	if exception == nil || exception.Type != object.ValueException {
		if exception == nil {
			exception = core.R.NilVal
		}
		exception = core.RaiseValue(exception)
	}
	cause := previousException
	canSetCause := len(vm.activeRescues) > 0
	if vm.ensureActive && len(vm.pendingEnsures) > 0 {
		pending := vm.pendingEnsures[len(vm.pendingEnsures)-1]
		if pending.Frame == frame && pending.Exception != nil {
			cause = pending.Exception
			canSetCause = true
		}
	}
	if canSetCause && cause != nil && cause != exception {
		if exc, ok := exception.Data.(*object.RException); ok && exc != nil && exc.Cause == nil {
			exc.Cause = cause
		}
	}
	if vm.raiseException(frame, exception) {
		return exception, true
	}
	vm.returnUnhandledException(frame, exception)
	return exception, true
}

func (vm *VM) executeRegisterIRDirectIndex(left, index *object.EmeraldValue, trustedArrayIndex ...bool) (*object.EmeraldValue, bool) {
	if left == nil || index == nil || left.Class == nil {
		return nil, false
	}
	if left.Type != object.ValueArray && left.Type != object.ValueHash {
		return nil, false
	}
	if (left.Type == object.ValueArray && left.Class != core.R.Classes["Array"]) ||
		(left.Type == object.ValueHash && left.Class != core.R.Classes["Hash"]) {
		return nil, false
	}
	if left.Type == object.ValueArray && len(trustedArrayIndex) > 0 && trustedArrayIndex[0] {
		// The caller has proved that this is a fresh exact Array literal and
		// that the callback contains no Ruby send/control-flow edge.  Keep the
		// same integer bounds as Array#[]; an out-of-range Integer deopts so
		// the normal framed path can preserve its error behavior.
		if index.Type != object.ValueInteger || !smallIntegerValue(index) {
			return nil, false
		}
		position, ok := index.Data.(int64)
		if !ok || position < -(int64(1)<<62) || position >= int64(1)<<62 {
			return nil, false
		}
		elements, ok := left.Data.([]*object.EmeraldValue)
		if !ok {
			return nil, false
		}
		if position < 0 {
			position += int64(len(elements))
		}
		if position < 0 || position >= int64(len(elements)) {
			return core.R.NilVal, true
		}
		return elements[int(position)], true
	}
	if (left.Type == object.ValueArray && !core.ArrayIndexUsesBuiltinImplementation()) ||
		(left.Type == object.ValueHash && !core.HashIndexUsesBuiltinImplementation()) {
		return nil, false
	}
	// Array#[] accepts ranges and other coercible values through Ruby
	// dispatch. The no-frame tier may only use the raw int64 path; rejecting
	// every other key before calling vm.index prevents it from re-entering a
	// user-defined/native [] implementation after an earlier direct send.
	if left.Type == object.ValueArray && index.Type != object.ValueInteger {
		return nil, false
	}
	if left.Type == object.ValueHash {
		return core.DirectHashIndex(left, index)
	}
	return vm.index(left, index), true
}

func (vm *VM) executeRegisterIRLoadLocal(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) bool {
	if frame == nil {
		return false
	}
	stackIndex := frame.Bp + 1 + int(instruction.param)
	if stackIndex < 0 || stackIndex >= len(vm.stack) {
		return false
	}
	registers[instruction.dst] = derefClosureValue(vm.stack[stackIndex])
	return true
}

func (vm *VM) executeRegisterIRLoadCapture(frame *Frame, instruction registerIRInstruction) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, false
	}
	cellAt := func(stackIndex int) (*object.EmeraldValue, bool) {
		if stackIndex < 0 || stackIndex >= len(vm.stack) {
			return nil, false
		}
		if current := vm.stack[stackIndex]; current != nil {
			if _, ok := current.Data.(*closureCell); ok {
				return current, true
			}
		}
		cell := &object.EmeraldValue{
			Type:  object.ValueObject,
			Data:  &closureCell{value: derefClosureValue(vm.stack[stackIndex])},
			Class: core.R.Classes["Object"],
		}
		vm.stack[stackIndex] = cell
		return cell, true
	}

	switch instruction.captureKind {
	case registerIRCaptureLocal:
		return cellAt(frame.Bp + 1 + int(instruction.param))
	case registerIRCaptureOuter:
		if vm.fp <= 0 || vm.fp-1 >= len(vm.frames) || vm.frames[vm.fp-1] == nil {
			return nil, false
		}
		outer := vm.frames[vm.fp-1]
		return cellAt(outer.Bp + 1 + int(instruction.param))
	case registerIRCaptureFree:
		if frame.Closure == nil || int(instruction.param) >= len(frame.Closure.Free) {
			return nil, false
		}
		return frame.Closure.Free[instruction.param], true
	case registerIRCaptureOuterFree:
		if vm.fp <= 0 || vm.fp-1 >= len(vm.frames) || vm.frames[vm.fp-1] == nil {
			return nil, false
		}
		outer := vm.frames[vm.fp-1]
		if outer.Closure == nil || int(instruction.param) >= len(outer.Closure.Free) {
			return nil, false
		}
		return outer.Closure.Free[instruction.param], true
	default:
		return nil, false
	}
}

func (vm *VM) executeRegisterIRClosure(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil || instruction.fn == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	numFree := int(instruction.argc)
	free := make([]*object.EmeraldValue, numFree)
	for index := 0; index < numFree; index++ {
		free[index] = snapshotClosureCapture(registers[int(instruction.dst)+index])
	}
	closureFn := functionWithConstants(instruction.fn, vm.frameConstants(frame))
	binding := vm.captureFrameBinding()
	closure := &object.Closure{
		Fn:        closureFn,
		Free:      free,
		Block:     vm.yieldBlock(),
		Binding:   binding,
		AutoSplat: true,
	}
	if binding != nil {
		closure.ClassStack = binding.ClassStack
	}
	if refinements, fixed := vm.currentFixedRefinements(); fixed {
		closure.Refinements = append([]*object.EmeraldValue(nil), refinements...)
		closure.RefinementsFixed = true
	}
	if closureFn.Name == "__block__" || closureFn.SingletonClassBody {
		closure.ReturnOwnerID = vm.lexicalReturnOwnerID(frame)
	}
	if closureFn.Name == "__scoped_const_rhs__" && frame.WhileEnd >= 0 {
		closure.BreakOwnerID = frame.ID
	}
	return &object.EmeraldValue{Type: object.ValueClosure, Data: closure, Class: core.R.Classes["Proc"]}, true
}

func (vm *VM) executeRegisterIRLoadConstant(frame *Frame, instruction registerIRInstruction) (*object.EmeraldValue, bool) {
	if frame == nil || instruction.name == "" {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	value := vm.resolveConstantRead(frame, instruction.name)
	if value == nil {
		return nil, false
	}
	if vm.raiseConstantException(frame, value) {
		return value, true
	}
	return value, true
}

func (vm *VM) executeRegisterIRLoadScopedConstant(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil || instruction.name == "" {
		return nil, false
	}
	receiver := registers[instruction.left]
	if receiver == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	value, ok := vm.scopedConstantValue(receiver, instruction.name)
	if !ok {
		value = vm.sendBypassVisibility(receiver, "const_missing", []*object.EmeraldValue{{Type: object.ValueSymbol, Data: instruction.name, Class: core.R.Classes["Symbol"]}})
	}
	if vm.raiseConstantException(frame, value) {
		return value, true
	}
	return value, true
}

func (vm *VM) executeRegisterIRStoreLocal(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) bool {
	if frame == nil {
		return false
	}
	stackIndex := frame.Bp + 1 + int(instruction.param)
	if stackIndex < 0 || stackIndex >= len(vm.stack) {
		return false
	}
	value := registers[instruction.left]
	if value == nil {
		value = core.R.NilVal
	}
	if current := vm.stack[stackIndex]; current != nil {
		if _, ok := current.Data.(*closureCell); ok {
			setClosureValue(&vm.stack[stackIndex], value)
		} else {
			vm.stack[stackIndex] = value
		}
	} else {
		vm.stack[stackIndex] = value
	}
	vm.updateCapturedBindingLocal(frame, int(instruction.param), value)
	return true
}

func (vm *VM) executeRegisterIRStoreFree(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) bool {
	if frame == nil || frame.Closure == nil || int(instruction.param) >= len(frame.Closure.Free) {
		return false
	}
	value := registers[instruction.left]
	if value == nil {
		value = core.R.NilVal
	}
	setClosureValue(&frame.Closure.Free[instruction.param], value)
	return true
}

func (vm *VM) executeRegisterIRSetStringEncoding(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) bool {
	value := registers[instruction.left]
	if value == nil || value.Type != object.ValueString || instruction.name == "" {
		return true
	}
	raw, _ := value.Data.(string)
	for index := 0; index < len(raw); index++ {
		if raw[index] >= 0x80 {
			return true
		}
	}
	core.SetStringEncoding(value, instruction.name)
	return true
}

func (vm *VM) executeRegisterIRStoreInstanceVar(frame *Frame, receiver *object.EmeraldValue, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil || receiver == nil {
		return nil, false
	}
	value := registers[instruction.left]
	if value == nil {
		value = core.R.NilVal
	}
	return core.SetDynamicInstanceVar(receiver, instruction.name, value), true
}

func (vm *VM) executeRegisterIRArray(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) bool {
	if frame == nil && core.ObjectSpaceAllocationTracing() {
		return false
	}
	count := int(instruction.argc)
	start := int(instruction.dst)
	if count < 0 || start < 0 || start+count > len(registers) {
		return false
	}
	elements := make([]*object.EmeraldValue, count)
	copy(elements, registers[start:start+count])
	registers[instruction.dst] = vm.trackObjectSpaceAllocation(&object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  elements,
		Class: core.R.Classes["Array"],
	}, frame)
	return true
}

// executeRegisterIRSplatToArray mirrors OpSplatToArray for the framed IR
// executor.  The conversion is intentionally never admitted to a no-frame
// plan: a non-Array value may dispatch user-defined `to_a`, and conversion
// errors must retain the caller's rescue/backtrace state.
func (vm *VM) executeRegisterIRSplatToArray(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || frame == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	value := registers[instruction.left]
	if value == nil {
		value = core.R.NilVal
	}
	elements, errVal := vm.toAForAssignmentSplat(value)
	if errVal != nil {
		if vm.raiseException(frame, errVal) {
			return errVal, true
		}
		vm.returnUnhandledException(frame, errVal)
		return errVal, true
	}
	registers[instruction.dst] = vm.trackObjectSpaceAllocation(vm.arrayValue(elements...), frame)
	return nil, true
}

func (vm *VM) executeRegisterIRHash(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) bool {
	if frame == nil && core.ObjectSpaceAllocationTracing() {
		return false
	}
	pairs := int(instruction.argc)
	start := int(instruction.dst)
	if pairs < 0 || start < 0 || start+2*pairs > len(registers) {
		return false
	}
	hash := &object.RHash{
		Pairs: make(map[*object.EmeraldValue]*object.EmeraldValue, pairs),
		Keys:  make([]*object.EmeraldValue, 0, pairs),
	}
	for index := 0; index < pairs; index++ {
		value := registers[start+2*index]
		key := hashLiteralKey(registers[start+2*index+1])
		if existing := hashLiteralExistingKey(hash, key); existing != nil {
			hash.Pairs[existing] = value
			continue
		}
		hash.Keys = append(hash.Keys, key)
		hash.Pairs[key] = value
	}
	registers[instruction.dst] = vm.trackObjectSpaceAllocation(&object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  hash,
		Class: core.R.Classes["Hash"],
	}, frame)
	return true
}

func (vm *VM) executeRegisterIRHashMerge(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	target := registers[instruction.left]
	source := registers[instruction.right]
	if source == nil || source.Type != object.ValueHash {
		if source != nil && core.ReceiverHasCallableMethod(source, "to_hash") {
			source = core.CallMethod(source, "to_hash")
		}
		if source != nil && source.Type == object.ValueException {
			if vm.raiseException(frame, source) {
				return source, true
			}
			vm.returnUnhandledException(frame, source)
			return source, true
		}
		if source == nil || source.Type != object.ValueHash {
			name := "nil"
			if source != nil {
				name = source.TypeName()
			}
			errVal := core.NewTypeError("no implicit conversion of " + name + " into Hash")
			if vm.raiseException(frame, errVal) {
				return errVal, true
			}
			vm.returnUnhandledException(frame, errVal)
			return errVal, true
		}
	}
	if target == nil || target.Type != object.ValueHash {
		errVal := core.NewTypeError("no implicit conversion into Hash")
		if vm.raiseException(frame, errVal) {
			return errVal, true
		}
		vm.returnUnhandledException(frame, errVal)
		return errVal, true
	}
	targetHash, targetOK := target.Data.(*object.RHash)
	sourceHash, sourceOK := source.Data.(*object.RHash)
	if !targetOK || !sourceOK || targetHash == nil || sourceHash == nil {
		errVal := core.NewTypeError("no implicit conversion into Hash")
		if vm.raiseException(frame, errVal) {
			return errVal, true
		}
		vm.returnUnhandledException(frame, errVal)
		return errVal, true
	}
	for _, key := range sourceHash.Keys {
		targetKey := hashLiteralExistingKey(targetHash, key)
		if targetKey == nil {
			targetKey = hashLiteralKey(key)
			targetHash.Keys = append(targetHash.Keys, targetKey)
		}
		value, ok := sourceHash.Pairs[key]
		if !ok {
			// Keys normally come directly from sourceHash.Keys.  Keep the
			// equality fallback for malformed/custom hash storage, but avoid
			// rescanning the whole map on the ordinary literal path.
			value = executorHashValue(source, key)
		}
		targetHash.Pairs[targetKey] = value
	}
	targetHash.Buckets = nil
	targetHash.BucketSize = 0
	return target, true
}

func (vm *VM) executeRegisterIRMarkKeywordHash(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, false
	}
	frame.Ip = instruction.byteIP
	value := registers[instruction.left]
	if value == nil {
		value = core.R.NilVal
	}
	core.MarkRuby2KeywordHash(value)
	return value, true
}

func (vm *VM) executeRegisterIRInlineSend(instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	receiver := registers[instruction.left]
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	identityCache := registerIRCacheableClassReceiver(receiver)
	if (!vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) && !identityCache) || instruction.cache == nil || instruction.cache.generation != object.CurrentMethodGeneration() {
		return nil, false
	}
	cache := instruction.cache
	var methodObj *object.Method
	var methodOwner *object.Class
	var leaf *leafMethodPlan
	var fn *object.Function
	var nativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	var directIndex bool
	if identityCache && cache.receiver == receiver || !identityCache && cache.receiver == nil && cache.class == receiver.Class {
		methodObj, methodOwner, leaf, fn, nativeFn, directIndex = cache.method, cache.owner, cache.inlineLeaf, cache.inlineFn, cache.nativeFn, cache.directIndex
	} else if registerIRPolymorphicSendCacheEnabled && (identityCache && cache.secondReceiver == receiver || !identityCache && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
		methodObj, methodOwner, leaf, fn, nativeFn, directIndex = cache.secondMethod, cache.secondOwner, cache.secondLeaf, cache.secondFn, cache.secondNativeFn, cache.secondDirectIndex
	} else {
		return nil, false
	}
	if methodObj == nil {
		return nil, false
	}
	if nativeFn != nil {
		// Keep the hot native ABI on explicit register arguments. Building a
		// temporary [4] slice before callNativeMethod made that backing array
		// escape once this method-level tier became the steady state, adding one
		// allocation per direct native send. The variadic native function still
		// owns Ruby's fixed-arity ABI, but no intermediate argument slice is
		// needed here.
		var result *object.EmeraldValue
		switch instruction.argc {
		case 0:
			result = nativeFn(receiver)
		case 1:
			result = nativeFn(receiver, registers[instruction.args[0]])
		case 2:
			result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]])
		case 3:
			result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]], registers[instruction.args[2]])
		case 4:
			result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]], registers[instruction.args[2]], registers[instruction.args[3]])
		default:
			return nil, false
		}
		if result == nil || result.Type == object.ValueException {
			return nil, false
		}
		return result, true
	}
	var args [4]*object.EmeraldValue
	for index := 0; index < int(instruction.argc); index++ {
		args[index] = registers[instruction.args[index]]
	}
	callArgs := args[:int(instruction.argc)]
	if directIndex && len(callArgs) == 1 {
		if result, executed := vm.executeRegisterIRCachedIndex(receiver, callArgs[0], true); executed {
			return result, true
		}
	}
	if leaf == nil || leaf.kind == leafMethodInstanceWriter || leaf.kind == leafMethodRegisterIR && fn == nil {
		return nil, false
	}
	result, executed := vm.executeRegisterIRInlineLeaf(leaf, fn, receiver, callArgs, methodObj, methodOwner)
	if !executed {
		return nil, false
	}
	return result, true
}

// executeRegisterIRInlineSendNoFrame is used after the enclosing plan has
// already checked the global method generation. Keeping that guard out of
// every send matters for short, frequently-called chains.
func (vm *VM) executeRegisterIRInlineSendNoFrame(instruction registerIRInstruction, registers *[16]*object.EmeraldValue, strictDirect bool, caseBranch ...bool) (*object.EmeraldValue, bool) {
	allowCaseBranch := len(caseBranch) > 0 && caseBranch[0]
	aggressive := len(caseBranch) > 1 && caseBranch[1]
	receiver := registers[instruction.left]
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	if instruction.cache == nil {
		if aggressive {
			return vm.executeRegisterIRAggressiveSend(instruction, registers)
		}
		return nil, false
	}
	cache := instruction.cache
	identityCache := registerIRCacheableClassReceiver(receiver)
	if aggressive && cache.generation != object.CurrentMethodGeneration() {
		return vm.executeRegisterIRAggressiveSend(instruction, registers)
	}
	if strictDirect {
		if !identityCache && !vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) {
			return nil, false
		}
		// The enclosing direct executor already checks the method generation and
		// each populated cache was admitted through the full receiver/singleton
		// guard.  Repeating that guard for every instruction defeats the point of
		// the direct tier; a generation change still deopts before user code.  A
		// direct plan can, however, create a fresh exact built-in collection and
		// send to it before the framed tier has ever warmed that nested call site.
		// Admit only the native leaf in that narrow case; Ruby-defined methods
		// still return false and replay through the complete framed dispatcher.
		if cache.generation != object.CurrentMethodGeneration() {
			if instruction.blockPresent || !vm.registerIRStrictNativeCacheReceiver(receiver, instruction.name) ||
				!vm.populateRegisterIRNoFrameCache(instruction, receiver) ||
				cache.generation != object.CurrentMethodGeneration() {
				return nil, false
			}
		}
	} else if !vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) && !identityCache {
		return nil, false
	}
	var methodObj *object.Method
	var methodOwner *object.Class
	var leaf *leafMethodPlan
	var fn *object.Function
	var nativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	var directIndex bool
	if identityCache && cache.receiver == receiver || !identityCache && cache.receiver == nil && cache.class == receiver.Class {
		methodObj, methodOwner, leaf, fn, nativeFn, directIndex = cache.method, cache.owner, cache.inlineLeaf, cache.inlineFn, cache.nativeFn, cache.directIndex
	} else if registerIRPolymorphicSendCacheEnabled && (identityCache && cache.secondReceiver == receiver || !identityCache && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
		methodObj, methodOwner, leaf, fn, nativeFn, directIndex = cache.secondMethod, cache.secondOwner, cache.secondLeaf, cache.secondFn, cache.secondNativeFn, cache.secondDirectIndex
	} else {
		if !vm.populateRegisterIRNoFrameCache(instruction, receiver) {
			return nil, false
		}
		if identityCache && cache.receiver == receiver || !identityCache && cache.receiver == nil && cache.class == receiver.Class {
			methodObj, methodOwner, leaf, fn, nativeFn, directIndex = cache.method, cache.owner, cache.inlineLeaf, cache.inlineFn, cache.nativeFn, cache.directIndex
		} else if registerIRPolymorphicSendCacheEnabled && (identityCache && cache.secondReceiver == receiver || !identityCache && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
			methodObj, methodOwner, leaf, fn, nativeFn, directIndex = cache.secondMethod, cache.secondOwner, cache.secondLeaf, cache.secondFn, cache.secondNativeFn, cache.secondDirectIndex
		} else {
			return nil, false
		}
	}
	if methodObj == nil {
		return nil, false
	}
	if strictDirect && nativeFn == nil && registerIRTrustedNativeNoEscapeName(instruction.name) {
		// The aggressive cache may have warmed only the Method pointer before
		// this direct tier runs.  Complete the native leaf entry once here so a
		// later trusted region does not keep rediscovering the same ABI through
		// executeRegisterIRAggressiveSend.
		if vm.populateRegisterIRNoFrameCache(instruction, receiver) {
			if identityCache && cache.receiver == receiver || !identityCache && cache.receiver == nil && cache.class == receiver.Class {
				methodObj, methodOwner, leaf, fn, nativeFn, directIndex = cache.method, cache.owner, cache.inlineLeaf, cache.inlineFn, cache.nativeFn, cache.directIndex
			} else if registerIRPolymorphicSendCacheEnabled && (identityCache && cache.secondReceiver == receiver || !identityCache && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
				methodObj, methodOwner, leaf, fn, nativeFn, directIndex = cache.secondMethod, cache.secondOwner, cache.secondLeaf, cache.secondFn, cache.secondNativeFn, cache.secondDirectIndex
			}
		}
	}
	if strictDirect && leaf == nil {
		leaf, fn = vm.registerIRInlineableLeafForDirect(methodObj, receiver, instruction.argc, allowCaseBranch)
		if leaf != nil {
			if identityCache && cache.receiver == receiver || !identityCache && cache.receiver == nil && cache.class == receiver.Class {
				cache.inlineLeaf = leaf
				cache.inlineFn = fn
			} else if registerIRPolymorphicSendCacheEnabled && (identityCache && cache.secondReceiver == receiver || !identityCache && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
				cache.secondLeaf = leaf
				cache.secondFn = fn
			}
		}
	}
	if nativeFn != nil {
		if instruction.name == "count" {
			for index := 0; index < int(instruction.argc); index++ {
				argument := registers[instruction.args[index]]
				if argument == nil || argument.Type != object.ValueString || argument.Class != core.R.Classes["String"] || core.AttachedSingletonClass(argument) != nil {
					return nil, false
				}
			}
		}
		// Keep the hot native ABI on explicit register arguments. Building a
		// temporary [4] slice before callNativeMethod makes that backing array
		// escape in the method-level direct tier, adding one allocation per
		// native send.
		var result *object.EmeraldValue
		switch instruction.argc {
		case 0:
			result = nativeFn(receiver)
		case 1:
			result = nativeFn(receiver, registers[instruction.args[0]])
		case 2:
			result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]])
		case 3:
			result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]], registers[instruction.args[2]])
		case 4:
			result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]], registers[instruction.args[2]], registers[instruction.args[3]])
		default:
			return nil, false
		}
		if result == nil || result.Type == object.ValueException {
			return nil, false
		}
		return result, true
	}
	var args [4]*object.EmeraldValue
	for index := 0; index < int(instruction.argc); index++ {
		args[index] = registers[instruction.args[index]]
	}
	callArgs := args[:int(instruction.argc)]
	if directIndex && len(callArgs) == 1 {
		if result, executed := vm.executeRegisterIRCachedIndex(receiver, callArgs[0], true); executed {
			return result, true
		}
	}
	if leaf == nil || leaf.kind == leafMethodInstanceWriter || leaf.kind == leafMethodRegisterIR && fn == nil {
		if aggressive {
			return vm.executeRegisterIRAggressiveSend(instruction, registers)
		}
		return nil, false
	}
	if strictDirect && leaf.kind == leafMethodRegisterIR {
		if aggressive {
			if nestedPlan, ok := vm.aggressiveIRPlanForFunction(fn, false); ok {
				var free []*object.EmeraldValue
				if methodObj != nil && methodObj.Closure != nil {
					free = methodObj.Closure.Free
				}
				result, executed := vm.executeRegisterIRDirectNoFrameWithFreeMode(nestedPlan, fn, receiver, callArgs, object.CurrentMethodGeneration(), free, true, true, true, true, true)
				if executed {
					return result, true
				}
			}
			return vm.executeRegisterIRAggressiveSend(instruction, registers)
		}
		planSafe := leaf.registerIR != nil && (leaf.registerIR.integerOnly || registerIRPlanSafeForDirectNoFrameWithOptions(leaf.registerIR, false, true, allowCaseBranch))
		if !planSafe {
			// A nested Ruby helper can be a pure no-send projection such as
			// `@objects[@root]`. The direct-send proof intentionally requires a
			// dynamic send or a closed mutation shape, but the existing no-frame
			// inline proof already admits these read/index-only helpers. Reuse that
			// proof here so a safe projection does not deopt its caller's whole
			// chain merely because its own send count is zero.
			if !aggressive && leaf.registerIR != nil && !leaf.registerIR.hasSends &&
				registerIRPlanSafeForNoFrameInline(leaf.registerIR) {
				if vm.registerIRInlineDepth >= 8 {
					return nil, false
				}
				vm.registerIRInlineDepth++
				result, executed := vm.executeRegisterIRInlineNoFramePlan(leaf.registerIR, fn, receiver, callArgs)
				vm.registerIRInlineDepth--
				if executed {
					return result, true
				}
			}
			if aggressive {
				if nestedPlan, ok := vm.aggressiveIRPlanForFunction(fn, false); ok {
					var free []*object.EmeraldValue
					if methodObj != nil && methodObj.Closure != nil {
						free = methodObj.Closure.Free
					}
					return vm.executeRegisterIRDirectNoFrameWithFreeMode(nestedPlan, fn, receiver, callArgs, object.CurrentMethodGeneration(), free, true, true, true, true, true)
				}
				return vm.executeRegisterIRAggressiveSend(instruction, registers)
			}
			return nil, false
		}
		if leaf.registerIR.integerOnly {
			return vm.executeRegisterIRInlineNoFramePlan(leaf.registerIR, fn, receiver, callArgs)
		}
		if vm.registerIRInlineDepth >= 8 {
			return nil, false
		}
		vm.registerIRInlineDepth++
		// The enclosing direct executor has already crossed its warmup and
		// generation/receiver proof.  Re-warming every nested Ruby leaf defeats
		// the typed block entry (and makes a nested module_function call deopt
		// the whole Array suffix), so use the trusted ABI entry here.  It still
		// checks the current generation and all runtime receiver guards.
		result, executed := vm.executeRegisterIRDirectNoFrameWithFreeTrusted(leaf.registerIR, fn, receiver, callArgs, object.CurrentMethodGeneration(), nil, false, true, allowCaseBranch)
		vm.registerIRInlineDepth--
		return result, executed
	}
	if strictDirect && leaf.kind != leafMethodInstanceReader && leaf.kind != leafMethodAttributeIntegerCompare && leaf.kind != leafMethodInstanceFallbackIndex && leaf.kind != leafMethodCaseDispatch {
		if aggressive {
			return vm.executeRegisterIRAggressiveSend(instruction, registers)
		}
		return nil, false
	}
	return vm.executeRegisterIRInlineLeaf(leaf, fn, receiver, callArgs, methodObj, methodOwner)
}

// executeRegisterIRTrustedNativeSend is the narrow steady-state ABI used by
// trusted block regions. The enclosing loop already checks the method
// generation once per element, so this function only retains the receiver
// class/identity guard needed for heterogeneous collections. Do not populate
// or walk a cache on a miss; the caller replays that element through Ruby's
// protocol.
func (vm *VM) executeRegisterIRTrustedNativeSend(instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || instruction.cache == nil || instruction.blockPresent || instruction.splatIndex != 255 ||
		instruction.opcode != compiler.OpSend || instruction.argc > 4 {
		return nil, false
	}
	receiver := registers[instruction.left]
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	cache := instruction.cache
	var nativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	if cache.receiver == nil {
		if cache.class == receiver.Class {
			nativeFn = cache.nativeFn
		} else if registerIRPolymorphicSendCacheEnabled && cache.secondReceiver == nil && cache.secondClass == receiver.Class {
			nativeFn = cache.secondNativeFn
		}
	} else if cache.receiver == receiver {
		nativeFn = cache.nativeFn
	} else if registerIRPolymorphicSendCacheEnabled && cache.secondReceiver == receiver {
		nativeFn = cache.secondNativeFn
	}
	if nativeFn == nil {
		return nil, false
	}
	if instruction.name == "length" && instruction.argc == 0 {
		if result, ok := core.StringLengthASCII(receiver); ok {
			return result, true
		}
	}
	var result *object.EmeraldValue
	switch instruction.argc {
	case 0:
		result = nativeFn(receiver)
	case 1:
		result = nativeFn(receiver, registers[instruction.args[0]])
	case 2:
		result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]])
	case 3:
		result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]], registers[instruction.args[2]])
	case 4:
		result = nativeFn(receiver, registers[instruction.args[0]], registers[instruction.args[1]], registers[instruction.args[2]], registers[instruction.args[3]])
	default:
		return nil, false
	}
	if result == nil {
		result = core.R.NilVal
	}
	return result, true
}

// executeRegisterIRTrustedDirectSend is the steady-state counterpart for a
// cached Ruby callee at the end of a typed callback. The first callback
// iteration has already populated the exact receiver/method/leaf cache; later
// iterations only retain that guard and enter the callee's direct plan. A
// mismatch returns before user code, allowing the outer collection loop to
// replay the current suffix through the complete Ruby protocol.
func (vm *VM) executeRegisterIRTrustedDirectSend(instruction registerIRInstruction, registers *[16]*object.EmeraldValue, allowCaseBranch bool) (*object.EmeraldValue, bool) {
	if vm == nil || instruction.cache == nil || instruction.blockPresent || instruction.splatIndex != 255 ||
		instruction.opcode != compiler.OpSend || instruction.argc > 4 {
		return nil, false
	}
	if registerIRTrustedNativeNoEscapeName(instruction.name) {
		return vm.executeRegisterIRTrustedNativeSend(instruction, registers)
	}
	receiver := registers[instruction.left]
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	cache := instruction.cache
	if cache.generation != object.CurrentMethodGeneration() {
		return nil, false
	}
	var methodObj *object.Method
	var methodOwner *object.Class
	var leaf *leafMethodPlan
	var fn *object.Function
	var trustedEntry **registerIRTrustedDirectEntry
	identityCache := registerIRCacheableClassReceiver(receiver)
	if !identityCache && !vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) {
		return nil, false
	}
	if identityCache && cache.receiver == receiver || !identityCache && cache.receiver == nil && cache.class == receiver.Class {
		methodObj, methodOwner, leaf, fn = cache.method, cache.owner, cache.inlineLeaf, cache.inlineFn
		trustedEntry = &cache.trustedDirect
	} else if registerIRPolymorphicSendCacheEnabled && (identityCache && cache.secondReceiver == receiver || !identityCache && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
		methodObj, methodOwner, leaf, fn = cache.secondMethod, cache.secondOwner, cache.secondLeaf, cache.secondFn
		trustedEntry = &cache.secondTrustedDirect
	} else {
		return nil, false
	}
	if methodObj == nil || leaf == nil {
		return nil, false
	}
	if _, native := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); native {
		// Mutating/native names are intentionally not admitted here. The pure
		// trusted-native path above handles query methods; a terminal native
		// mutation remains on the conservative direct path so an exception cannot
		// cause a side-effecting native call to be replayed.
		return nil, false
	}
	var argsStorage [4]*object.EmeraldValue
	for index := 0; index < int(instruction.argc); index++ {
		argsStorage[index] = registers[instruction.args[index]]
	}
	args := argsStorage[:int(instruction.argc)]
	if leaf.kind == leafMethodRegisterIR {
		if fn == nil || leaf.registerIR == nil || trustedEntry == nil || vm.registerIRInlineDepth >= 8 {
			return nil, false
		}
		constantGeneration := uint64(0)
		if leaf.registerIR.hasConstantLoads {
			constantGeneration = object.CurrentConstantGeneration()
		}
		entry := *trustedEntry
		if entry == nil || entry.generation != object.CurrentMethodGeneration() ||
			entry.constantGeneration != constantGeneration || entry.vm != vm || entry.method != methodObj ||
			entry.leaf != leaf || entry.fn != fn || entry.allowCaseBranch != allowCaseBranch {
			allowConstants := methodObj.Closure != nil && registerIRDirectConstantsSafe(vm, methodObj.Closure, leaf.registerIR)
			var free []*object.EmeraldValue
			if methodObj.Closure != nil {
				free = methodObj.Closure.Free
			}
			entry = &registerIRTrustedDirectEntry{
				generation:         object.CurrentMethodGeneration(),
				constantGeneration: constantGeneration,
				vm:                 vm,
				method:             methodObj,
				leaf:               leaf,
				fn:                 fn,
				plan:               leaf.registerIR,
				free:               free,
				allowConstants:     allowConstants,
				allowCaseBranch:    allowCaseBranch,
				safe:               registerIRPlanSafeForDirectNoFrameWithOptions(leaf.registerIR, false, allowConstants, allowCaseBranch),
			}
			*trustedEntry = entry
		}
		if !entry.safe {
			return nil, false
		}
		vm.registerIRInlineDepth++
		result, executed := vm.executeRegisterIRDirectNoFrameWithFreeMode(
			entry.plan, entry.fn, receiver, args, entry.generation, entry.free,
			true, false, entry.allowConstants, entry.allowCaseBranch, false, true,
		)
		vm.registerIRInlineDepth--
		return result, executed
	}
	return vm.executeRegisterIRInlineLeaf(leaf, fn, receiver, args, methodObj, methodOwner)
}

// registerIRTrustedDirectPlanReady proves that a cached Ruby callee can run
// in the trusted direct executor. It is the object-mutating extension of the
// pure native-region proof: only no-escape query sends may precede a terminal
// instance/free write, and every send must already have a native cache.
func registerIRTrustedDirectPlanReady(plan *registerIRPlan, generation uint64) bool {
	if plan == nil || plan.blockReturn || plan.hasImplicitSends || plan.sendCount == 0 {
		return false
	}
	readySend := false
	for index, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadFrozenString, registerIRLoadConstantValue,
			registerIRLoadConstant, registerIRLoadScopedConstant,
			registerIRLoadInstanceVar, registerIRDefinedInstanceVar,
			registerIRLoadSelf, registerIRLoadFree, registerIRMove,
			registerIRSwap, registerIRBang, registerIRStoreLocal,
			registerIREqual, registerIRCompare, registerIRDynamicCompare,
			registerIRBinary, registerIRIndex, registerIRSlice,
			registerIRReturn:
		case registerIRStoreInstanceVar, registerIRStoreFree, registerIRSetStringEncoding:
			if !registerIRDirectTerminalMutationAt(plan, index, instruction.left) {
				return false
			}
		case registerIRSend:
			if instruction.blockPresent || instruction.splatIndex != 255 || instruction.opcode != compiler.OpSend ||
				!registerIRTrustedNativeNoEscapeName(instruction.name) {
				return false
			}
			if instruction.name == "length" && instruction.argc == 0 && !core.StringLengthUsesBuiltinImplementation() {
				return false
			}
			if instruction.cache == nil || instruction.cache.generation != generation ||
				instruction.cache.nativeFn == nil && instruction.cache.secondNativeFn == nil {
				return false
			}
			readySend = true
		case registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy, registerIRJumpNotNil:
		default:
			return false
		}
	}
	return readySend
}

// registerIRTrustedArrayCallbackReady proves the send edges of an outer
// Array callback after its first element has populated the call-site caches.
// Native query edges use the compact native proof; Ruby-defined edges must
// point at a direct Register IR callee whose own nested sends are already
// native-cached and whose final mutation is terminal. This lets the outer
// callback skip the per-element cache/leaf admission work without guessing
// about a heterogeneous receiver.
func registerIRTrustedArrayCallbackReady(plan *registerIRPlan, generation uint64) bool {
	if plan == nil || !plan.blockReturn || plan.hasImplicitSends || plan.hasExplicitReturn || plan.sendCount == 0 {
		return false
	}
	readySend := false
	for index := range plan.instructions {
		instruction := &plan.instructions[index]
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadFrozenString, registerIRLoadConstantValue,
			registerIRLoadConstant, registerIRLoadScopedConstant,
			registerIRLoadInstanceVar, registerIRLoadSelf, registerIRLoadFree,
			registerIRMove, registerIRSwap, registerIRBang, registerIRStoreLocal,
			registerIRReturn, registerIRJump, registerIRJumpTruthy,
			registerIRJumpNotTruthy, registerIRJumpNotNil:
		case registerIRBinary:
			// No-frame binary execution can prove these operations without
			// invoking Ruby code. A miss (heterogeneous value, overflow, or
			// operator redefinition) returns before the terminal store, so the
			// outer Array loop can replay the current suffix safely.
			if !registerIRTrustedArrayArithmeticEnabled || !registerIRTrustedIntegerBinaryOpcode(instruction.opcode) {
				return false
			}
		case registerIRStoreInstanceVar, registerIRStoreFree:
			if !registerIRDirectTerminalMutationAt(plan, index, instruction.left) {
				return false
			}
		case registerIRSend:
			if instruction.blockPresent || instruction.splatIndex != 255 || instruction.opcode != compiler.OpSend || instruction.cache == nil ||
				instruction.cache.generation != generation {
				return false
			}
			if registerIRTrustedNativeNoEscapeName(instruction.name) {
				if instruction.name == "length" && instruction.argc == 0 && !core.StringLengthUsesBuiltinImplementation() {
					return false
				}
				if instruction.cache.nativeFn == nil && instruction.cache.secondNativeFn == nil {
					return false
				}
				readySend = true
				continue
			}
			readyLeaf := func(leaf *leafMethodPlan) bool {
				return leaf != nil && leaf.kind == leafMethodRegisterIR && leaf.registerIR != nil &&
					registerIRTrustedDirectPlanReady(leaf.registerIR, generation)
			}
			if !readyLeaf(instruction.cache.inlineLeaf) && !readyLeaf(instruction.cache.secondLeaf) {
				return false
			}
			readySend = true
		default:
			return false
		}
	}
	return readySend
}

func registerIRTrustedIntegerBinaryOpcode(opcode compiler.Opcode) bool {
	switch opcode {
	case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod, compiler.OpBitAnd:
		return true
	default:
		return false
	}
}

// registerIRTrustedNativeRegionReady admits a region made entirely of pure
// register operations and native, no-escape query sends.  A later collection
// element may take a different branch or class path; because every operation
// admitted here is non-mutating, replaying that element through the ordinary
// block path cannot duplicate an observable prefix.  Writes, allocations and
// calls into Ruby remain outside this tier.
func registerIRTrustedNativeRegionReady(plan *registerIRPlan, generation uint64) bool {
	if plan == nil || plan.sendCount == 0 || len(plan.instructions) == 0 {
		return false
	}
	readySend := false
	for index := range plan.instructions {
		instruction := &plan.instructions[index]
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadFrozenString, registerIRLoadConstantValue,
			registerIRLoadConstant, registerIRLoadScopedConstant,
			registerIRLoadInstanceVar, registerIRLoadSelf, registerIRLoadFree,
			registerIRMove, registerIRSwap, registerIRBang, registerIRStoreLocal,
			registerIRReturn:
		case registerIRStoreFree:
			// The capture write is admitted only as the final operation. A
			// native-send miss before it can therefore replay only pure work.
			if !registerIRDirectTerminalMutationAt(plan, index, instruction.left) {
				return false
			}
		case registerIRSend:
			if instruction.blockPresent || instruction.splatIndex != 255 || instruction.opcode != compiler.OpSend ||
				!registerIRTrustedNativeNoEscapeName(instruction.name) {
				return false
			}
			if instruction.name == "length" && instruction.argc == 0 && !core.StringLengthUsesBuiltinImplementation() {
				return false
			}
			if instruction.cache != nil && instruction.cache.generation == generation &&
				(instruction.cache.nativeFn != nil || instruction.cache.secondNativeFn != nil) {
				readySend = true
			}
		case registerIRJump, registerIRJumpTruthy, registerIRJumpNotTruthy, registerIRJumpNotNil:
		default:
			return false
		}
	}
	// At least one send must have warmed on the first element.  Other pure
	// sends may belong to an untaken branch; if that branch is later selected,
	// executeRegisterIRTrustedNativeSend returns a miss and the caller replays
	// the complete element through the framed executor.
	return readySend
}

// registerIRTrustedFramedNativeRegionReady is the frame-preserving sibling of
// registerIRTrustedNativeRegionReady. It allows ordinary value construction
// and control-flow operations because the caller still needs a real Frame,
// but disallows writes, Ruby calls, yields and exception-producing operations.
// Thus a trusted-send miss cannot replay an observable mutation from the
// current callback before the fallback path gets control.
func registerIRTrustedFramedNativeRegionReady(plan *registerIRPlan, generation uint64) bool {
	if plan == nil || len(plan.instructions) == 0 || plan.hasImplicitSends || plan.hasExplicitReturn {
		return false
	}
	readySend := false
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadFrozenString, registerIRLoadConstantValue,
			registerIRLoadConstant, registerIRLoadScopedConstant,
			registerIRLoadInstanceVar, registerIRDefinedInstanceVar,
			registerIRLoadSelf, registerIRLoadFree, registerIRMove,
			registerIRSwap, registerIRBang, registerIRStoreLocal,
			registerIRArray, registerIRHash, registerIRHashMerge,
			registerIRRange, registerIRReturn,
			registerIRJump, registerIRJumpTruthy,
			registerIRJumpNotTruthy, registerIRJumpNotNil:
		case registerIRSend:
			if instruction.blockPresent || instruction.splatIndex != 255 || instruction.opcode != compiler.OpSend || instruction.cache == nil ||
				instruction.cache.generation != generation || !registerIRTrustedNativeNoEscapeName(instruction.name) ||
				instruction.name == "length" && !core.StringLengthUsesBuiltinImplementation() ||
				instruction.cache.nativeFn == nil && instruction.cache.secondNativeFn == nil {
				return false
			}
			readySend = true
		default:
			return false
		}
	}
	return readySend
}

func registerIRTrustedNativeNoEscapeName(name string) bool {
	switch name {
	case "to_s", "to_str", "to_i", "to_f", "to_sym", "inspect",
		"abs", "zero?", "positive?", "negative?", "nil?", "frozen?",
		"class", "object_id", "hash", "size", "length", "empty?",
		"upcase", "downcase", "strip", "start_with?", "end_with?",
		"is_a?", "kind_of?", "instance_of?", "respond_to?":
		return true
	default:
		return false
	}
}

// executeRegisterIRAggressiveSend completes a send that the conservative
// direct tier could not prove as a leaf.  A fixed-arity public Ruby callee is
// recursively entered through the same frame-free IR graph; other methods are
// dispatched once through vm.send.  This is deliberately an execution path,
// not a speculative probe: it never returns a miss after user code has run.
func (vm *VM) executeRegisterIRAggressiveSend(instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !registerIRAggressiveEnabled || instruction.blockPresent || instruction.splatIndex != 255 || instruction.opcode != compiler.OpSend {
		return nil, false
	}
	receiver := registers[instruction.left]
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	var argsStorage [4]*object.EmeraldValue
	for index := 0; index < int(instruction.argc); index++ {
		argsStorage[index] = registers[instruction.args[index]]
	}
	args := argsStorage[:int(instruction.argc)]
	methodObj, methodOwner := vm.aggressiveSendCachedMethod(instruction, receiver)
	if methodObj == nil {
		var fallback *object.EmeraldValue
		methodObj, methodOwner, fallback = vm.lookupMethodForSend(receiver, instruction.name, args, false, true)
		if fallback != nil {
			return fallback, true
		}
		vm.aggressiveSendStoreMethod(instruction, receiver, methodObj, methodOwner)
	}
	// A cache hit now carries the complete admitted call-graph edge.  Do this
	// before the legacy method-type checks: those checks (and, previously, the
	// plan lookup below) were paid for by every iteration even though the
	// receiver class and method generation were already guarded above.
	if methodObj != nil {
		if plan, nativeFn, prepared := vm.aggressiveSendCachedDispatch(instruction, receiver); prepared {
			if nativeFn != nil {
				return callNativeMethod(nativeFn, receiver, args), true
			}
			generation := object.CurrentMethodGeneration()
			if plan != nil {
				fn, ruby := methodObj.Fn.(*object.Function)
				if !ruby || fn == nil {
					result := vm.send(receiver, instruction.name, args)
					if result == nil {
						result = core.R.NilVal
					}
					return result, true
				}
				if vm.registerIRInlineDepth >= 32 {
					result := vm.send(receiver, instruction.name, args)
					if result == nil {
						result = core.R.NilVal
					}
					return result, true
				}
				vm.registerIRInlineDepth++
				var free []*object.EmeraldValue
				if methodObj.Closure != nil {
					free = methodObj.Closure.Free
				}
				result, executed := vm.executeRegisterIRDirectNoFrameWithFreeMode(plan, fn, receiver, args, object.CurrentMethodGeneration(), free, false, true, true, true, true)
				vm.registerIRInlineDepth--
				if executed {
					return result, true
				}
			}
			// A prepared plan is allowed to side-exit before user code (for
			// example, after a runtime type guard).  Replay the ordinary send
			// exactly once; do not repeat the admission work in this call.
			result := vm.invokeAggressiveCachedSend(receiver, instruction.name, args, methodObj, methodOwner, generation)
			if result == nil {
				result = core.R.NilVal
			}
			return result, true
		}
	}
	if methodObj != nil && methodObj.DispatchOwner == nil &&
		(methodObj.Visibility == "" || methodObj.Visibility == "public") &&
		!methodObj.Ruby2Keywords && !methodUsesRefinements(methodObj) {
		if nativeFn, native := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); native {
			return callNativeMethod(nativeFn, receiver, args), true
		}
		if fn, ruby := methodObj.Fn.(*object.Function); ruby && fn != nil &&
			len(fn.Params) == len(args) && simpleBlockParameterPatterns(fn) && !fn.HasRestParam && !fn.HasBlockParam &&
			len(fn.KeywordParams) == 0 && fn.KeywordRestParam == "" && !fn.KeywordRestOnly &&
			!registerIRFunctionNeedsDefaultEvaluation(fn, len(args)) {
			if plan, ok := vm.aggressiveIRPlanForFunction(fn, false); ok {
				if vm.registerIRInlineDepth >= 32 {
					return vm.send(receiver, instruction.name, args), true
				}
				vm.registerIRInlineDepth++
				var free []*object.EmeraldValue
				if methodObj.Closure != nil {
					free = methodObj.Closure.Free
				}
				result, executed := vm.executeRegisterIRDirectNoFrameWithFreeMode(plan, fn, receiver, args, object.CurrentMethodGeneration(), free, false, true, true, true, true)
				vm.registerIRInlineDepth--
				if executed {
					return result, true
				}
			}
		}
	}
	result := vm.send(receiver, instruction.name, args)
	if result == nil {
		result = core.R.NilVal
	}
	return result, true
}

// invokeAggressiveCachedSend is the side-exit boundary for a prepared
// aggressive call-graph edge. The receiver/method cache has already proved
// the exact public method for the current generation, so re-entering send
// would repeat the hierarchy walk and generic lookup probes on every loop
// iteration. If a nested direct plan changed the method generation, the
// cached pointer is no longer valid and the complete send path is retained.
func (vm *VM) invokeAggressiveCachedSend(receiver *object.EmeraldValue, name string, args []*object.EmeraldValue, methodObj *object.Method, methodOwner *object.Class, generation uint64) *object.EmeraldValue {
	if vm == nil {
		return nil
	}
	if receiver == nil || methodObj == nil || generation != object.CurrentMethodGeneration() {
		return vm.send(receiver, name, args)
	}
	return vm.invokeMethod(receiver, name, name, args, methodObj, methodOwner, false)
}

// aggressiveSendCachedMethod is the missing call-graph edge in the old
// aggressive block tier.  The enclosing Register IR instruction already owns
// a generation/receiver cache, but executeRegisterIRAggressiveSend used to
// ignore it and repeat a full method lookup for every loop iteration.  Cache
// only after the normal VM method-cache context has proved that no lexical
// refinements or singleton receiver can affect dispatch; a cache miss remains
// the ordinary lookup path and method-generation changes invalidate it.
func (vm *VM) aggressiveSendCachedMethod(instruction registerIRInstruction, receiver *object.EmeraldValue) (*object.Method, *object.Class) {
	if vm == nil || instruction.cache == nil || receiver == nil ||
		!registerIRCacheableSendName(instruction.name) ||
		!vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) ||
		!aggressiveSendCacheContextSafe(vm) {
		return nil, nil
	}
	cache := instruction.cache
	generation := object.CurrentMethodGeneration()
	identity := registerIRCacheableClassReceiver(receiver)
	if cache.generation == generation {
		if identity && cache.receiver == receiver && cache.method != nil {
			return cache.method, cache.owner
		}
		if !identity && cache.receiver == nil && cache.class == receiver.Class && cache.method != nil {
			return cache.method, cache.owner
		}
		if registerIRPolymorphicSendCacheEnabled {
			if identity && cache.secondReceiver == receiver && cache.secondMethod != nil {
				return cache.secondMethod, cache.secondOwner
			}
			if !identity && cache.secondReceiver == nil && cache.secondClass == receiver.Class && cache.secondMethod != nil {
				return cache.secondMethod, cache.secondOwner
			}
		}
	}
	// Do not perform a second hierarchy walk here.  A previously warmed VM
	// method cache is safe under the same context guards; on a cold site the
	// caller performs the one required lookup and installs the result below.
	method, owner, found := vm.cachedPlainMethod(receiver, instruction.name)
	if !found || method == nil || method.Visibility == "undefined" {
		return nil, nil
	}
	if cache.generation != generation {
		*cache = registerIRSendCache{generation: generation}
	}
	if cache.method == nil || !registerIRPolymorphicSendCacheEnabled ||
		identity && cache.receiver == receiver || !identity && cache.receiver == nil && cache.class == receiver.Class {
		cache.class = receiver.Class
		if identity {
			cache.receiver = receiver
		}
		cache.method, cache.owner = method, owner
		cache.bytecodeFixedArity = false
	} else if cache.secondMethod == nil || identity && cache.secondReceiver == receiver || !identity && cache.secondReceiver == nil && cache.secondClass == receiver.Class {
		cache.secondClass = receiver.Class
		if identity {
			cache.secondReceiver = receiver
		}
		cache.secondMethod, cache.secondOwner = method, owner
		cache.secondBytecodeFixedArity = false
	}
	return method, owner
}

func (vm *VM) aggressiveSendStoreMethod(instruction registerIRInstruction, receiver *object.EmeraldValue, method *object.Method, owner *object.Class) {
	if vm == nil || instruction.cache == nil || receiver == nil || method == nil ||
		!registerIRCacheableSendName(instruction.name) ||
		!vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) ||
		!aggressiveSendCacheContextSafe(vm) {
		return
	}
	cache := instruction.cache
	generation := object.CurrentMethodGeneration()
	if cache.generation != generation {
		*cache = registerIRSendCache{generation: generation}
	}
	identity := registerIRCacheableClassReceiver(receiver)
	if cache.method == nil || !registerIRPolymorphicSendCacheEnabled {
		cache.class = receiver.Class
		if identity {
			cache.receiver = receiver
		}
		cache.method, cache.owner = method, owner
		cache.bytecodeFixedArity = false
		return
	}
	if cache.secondMethod == nil {
		cache.secondClass = receiver.Class
		if identity {
			cache.secondReceiver = receiver
		}
		cache.secondMethod, cache.secondOwner = method, owner
		cache.secondBytecodeFixedArity = false
	}
}

// aggressiveSendCachedDispatch returns the already-admitted no-frame entry
// for the receiver's monomorphic/polymorphic cache slot.  It deliberately
// prepares an entry lazily: the first aggressive execution may have obtained
// the Method from cachedPlainMethod before the caller had a chance to install
// the plan.  Once prepared, even a negative result is sticky until method
// generation changes, so unsupported methods do not repeatedly rescan their
// signature and Register IR body.
func (vm *VM) aggressiveSendCachedDispatch(instruction registerIRInstruction, receiver *object.EmeraldValue) (*registerIRPlan, func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue, bool) {
	if vm == nil || instruction.cache == nil || receiver == nil ||
		!registerIRCacheableSendName(instruction.name) ||
		!vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) ||
		!aggressiveSendCacheContextSafe(vm) {
		return nil, nil, false
	}
	cache := instruction.cache
	if cache.generation != object.CurrentMethodGeneration() {
		return nil, nil, false
	}
	identity := registerIRCacheableClassReceiver(receiver)
	if identity && cache.receiver == receiver || !identity && cache.receiver == nil && cache.class == receiver.Class {
		if cache.method == nil {
			return nil, nil, false
		}
		if !cache.aggressivePrepared {
			cache.aggressivePlan, cache.aggressiveNativeFn = vm.prepareAggressiveSendMethod(cache.method, instruction.argc)
			cache.aggressivePrepared = true
		}
		return cache.aggressivePlan, cache.aggressiveNativeFn, true
	}
	if registerIRPolymorphicSendCacheEnabled && (identity && cache.secondReceiver == receiver || !identity && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
		if cache.secondMethod == nil {
			return nil, nil, false
		}
		if !cache.secondAggressivePrepared {
			cache.secondAggressivePlan, cache.secondAggressiveNativeFn = vm.prepareAggressiveSendMethod(cache.secondMethod, instruction.argc)
			cache.secondAggressivePrepared = true
		}
		return cache.secondAggressivePlan, cache.secondAggressiveNativeFn, true
	}
	return nil, nil, false
}

func (vm *VM) prepareAggressiveSendMethod(methodObj *object.Method, argc uint8) (*registerIRPlan, func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue) {
	if vm == nil || methodObj == nil || methodObj.DispatchOwner != nil ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") ||
		methodObj.Ruby2Keywords || methodUsesRefinements(methodObj) {
		return nil, nil
	}
	if nativeFn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
		return nil, nativeFn
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || len(fn.Params) != int(argc) || !simpleBlockParameterPatterns(fn) ||
		fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) != 0 ||
		fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		registerIRFunctionNeedsDefaultEvaluation(fn, int(argc)) {
		return nil, nil
	}
	plan, ok := vm.aggressiveIRPlanForFunction(fn, false)
	if !ok {
		return nil, nil
	}
	return plan, nil
}

func aggressiveSendCacheContextSafe(vm *VM) bool {
	if vm == nil {
		return false
	}
	if refinements, fixed := vm.currentFixedRefinements(); fixed && len(refinements) > 0 {
		return false
	}
	for _, scope := range vm.classStack {
		if scope == nil {
			continue
		}
		switch scope.Type {
		case object.ValueClass:
			if class, ok := scope.Data.(*object.Class); ok && class != nil && len(class.UsedRefinements) > 0 {
				return false
			}
		case object.ValueModule:
			if module, ok := scope.Data.(*object.Module); ok && module != nil && len(module.UsedRefinements) > 0 {
				return false
			}
		}
	}
	return true
}

// populateRegisterIRNoFrameCache warms a no-frame send cache without invoking
// Ruby code. Besides direct native methods, a leaf plan proven safe by
// registerIRInlineableLeaf can be cached here. A failed proof returns false
// before user code, so the caller can replay the ordinary block path.
func (vm *VM) populateRegisterIRNoFrameCache(instruction registerIRInstruction, receiver *object.EmeraldValue) bool {
	if vm == nil || instruction.cache == nil || receiver == nil ||
		!registerIRCacheableSendName(instruction.name) ||
		!vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) {
		return false
	}
	methodObj, owner, ok := vm.cachedPlainMethod(receiver, instruction.name)
	if !ok && vm.registerIRStrictNativeCacheReceiver(receiver, instruction.name) {
		// A direct plan may be the first execution path for a method body.  In
		// that case the ordinary framed send has not necessarily populated the
		// VM-wide class method cache yet.  Perform the same lookup without
		// entering Ruby code so the exact native leaf can be admitted safely.
		var fallback *object.EmeraldValue
		methodObj, owner, fallback = vm.lookupMethodForSend(receiver, instruction.name, nil, false, true)
		ok = fallback == nil && methodObj != nil
	}
	if !ok {
		return false
	}
	methodObj, owner, ok = resolveVisibilityAliasMethod(instruction.name, methodObj, owner)
	if !ok || methodObj == nil || methodObj.Visibility == "undefined" {
		return false
	}
	var leaf *leafMethodPlan
	var fn *object.Function
	if registerIRInlineEnabled {
		// The direct cache has already proved the receiver's class/singleton
		// shape. Use the direct admission helper for ordinary public instance
		// methods as well as Class/Module identity receivers; otherwise a Ruby
		// leaf with mutable string literals (for example Reference#to_s) is
		// rejected by the normal framed-only leaf cache and every enclosing
		// typed block deopts back to a Frame.
		leaf, fn = vm.registerIRInlineableLeafForDirect(methodObj, receiver, instruction.argc, true)
	}
	nativeFn := registerIRDirectNativeFn(methodObj, instruction.name)
	directIndex := registerIRDirectIndexEligible(receiver, instruction.name, instruction.argc, methodObj, owner)
	if nativeFn == nil && leaf == nil && !directIndex {
		return false
	}
	generation := object.CurrentMethodGeneration()
	cache := instruction.cache
	if cache.generation != generation {
		*cache = registerIRSendCache{generation: generation}
	}
	if cache.class == nil || cache.class == receiver.Class || !registerIRPolymorphicSendCacheEnabled {
		identityReceiver := registerIRCacheableClassReceiver(receiver)
		cache.class = receiver.Class
		if identityReceiver {
			cache.receiver = receiver
		} else {
			cache.receiver = nil
		}
		cache.method = methodObj
		cache.owner = owner
		cache.bytecodeFixedArity = false
		cache.inlineLeaf = leaf
		cache.inlineFn = fn
		cache.nativeFn = nativeFn
		cache.directIndex = directIndex
		return true
	}
	if cache.secondClass == nil || cache.secondClass == receiver.Class {
		identityReceiver := registerIRCacheableClassReceiver(receiver)
		cache.secondClass = receiver.Class
		if identityReceiver {
			cache.secondReceiver = receiver
		} else {
			cache.secondReceiver = nil
		}
		cache.secondMethod = methodObj
		cache.secondOwner = owner
		cache.secondBytecodeFixedArity = false
		cache.secondLeaf = leaf
		cache.secondFn = fn
		cache.secondNativeFn = nativeFn
		cache.secondDirectIndex = directIndex
		return true
	}
	return false
}

func (vm *VM) executeRegisterIRBinary(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, true
	}
	frame.Ip = instruction.byteIP
	left := registers[instruction.left]
	right := registers[instruction.right]
	// `String#<<` shares OpBitLeftShift with numeric shifts.  Exact built-in
	// String receivers can use the core primitive without a second dynamic send;
	// subclasses, singleton overrides and redefined methods deopt below.
	if instruction.opcode == compiler.OpBitLeftShift && left != nil &&
		left.Type == object.ValueString && left.Class == core.R.Classes["String"] && core.AttachedSingletonClass(left) == nil &&
		core.StringAppendUsesBuiltinImplementation() {
		if right == nil {
			right = core.R.NilVal
		}
		result, handled := core.AppendStringOneFast(left, right)
		if !handled {
			result = core.AppendStringOne(left, right)
		}
		if result != nil && result.Type == object.ValueException {
			if !vm.raiseException(frame, result) {
				vm.returnUnhandledException(frame, result)
			}
			return result, true
		}
		return result, false
	}
	if smallIntegerValue(left) && smallIntegerValue(right) {
		l := left.Data.(int64)
		r := right.Data.(int64)
		switch instruction.opcode {
		case compiler.OpAdd:
			if vm.fusedIntegerBuiltinsAvailable() && !((r > 0 && l > math.MaxInt64-r) || (r < 0 && l < math.MinInt64-r)) {
				return core.NewIntegerValue(l + r), false
			}
		case compiler.OpSub:
			if vm.fusedIntegerOperationAvailable(compiler.OpSub) &&
				!((r < 0 && l > math.MaxInt64+r) || (r > 0 && l < math.MinInt64+r)) {
				return core.NewIntegerValue(l - r), false
			}
		case compiler.OpMul:
			product := l * r
			if vm.fusedIntegerOperationAvailable(compiler.OpMul) &&
				!((l == -1 && r == math.MinInt64) || (r == -1 && l == math.MinInt64)) && (l == 0 || product/l == r) {
				return core.NewIntegerValue(product), false
			}
		case compiler.OpMod:
			if vm.fusedIntegerOperationAvailable(compiler.OpMod) && r != 0 {
				result := l % r
				if result != 0 && (result < 0) != (r < 0) {
					result += r
				}
				return core.NewIntegerValue(result), false
			}
		case compiler.OpBitLeftShift:
			if r >= 0 && r < 64 {
				result := l << uint(r)
				if result>>uint(r) == l {
					return core.NewIntegerValue(result), false
				}
			}
		}
	}
	var result *object.EmeraldValue
	switch instruction.opcode {
	case compiler.OpAdd:
		result = vm.add(left, right)
	case compiler.OpSub:
		result = vm.sub(left, right)
	case compiler.OpMul:
		result = vm.mul(left, right)
	case compiler.OpMod:
		result = vm.mod(left, right)
	case compiler.OpBitLeftShift:
		var ok bool
		result, ok = integerShift(left, right, true)
		if !ok {
			result = vm.send(left, "<<", []*object.EmeraldValue{right})
		}
	default:
		return nil, true
	}
	if result != nil && result.Type == object.ValueException {
		if !vm.raiseException(frame, result) {
			vm.returnUnhandledException(frame, result)
		}
		return result, true
	}
	return result, false
}

// executeRegisterIRDynamicEqual keeps the equality operation inside the
// framed Register IR loop after an earlier send.  Unlike immediate equality,
// vm.equals may enter a Ruby/container implementation, so the current bytecode
// position and the normal exception protocol must remain visible.
func (vm *VM) executeRegisterIRDynamicEqual(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || frame == nil {
		return nil, false
	}
	left := registers[instruction.left]
	if left == nil {
		left = core.R.NilVal
	}
	right := registers[instruction.right]
	if right == nil {
		right = core.R.NilVal
	}
	frame.Ip = instruction.byteIP
	result := vm.equals(left, right)
	if result == nil {
		result = core.R.NilVal
	}
	if result.Type == object.ValueException {
		if !vm.raiseException(frame, result) {
			vm.returnUnhandledException(frame, result)
		}
		return result, true
	}
	return result, false
}

func (vm *VM) executeRegisterIRReturn(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	result := registers[instruction.left]
	if result == nil {
		result = core.R.NilVal
	}
	if !instruction.explicitReturn || frame == nil || frame.Fn == nil || frame.Fn.Name != "__block__" ||
		frame.DefinedByDefineMethod || frame.Fn.DefinedByDefineMethod || frame.Closure == nil ||
		frame.Closure.ReturnOwnerID <= 0 || frame.Closure.ReturnOwnerID == frame.ID {
		return result, true
	}
	if vm.threadDepth > 0 || !vm.frameIDActive(frame.Closure.ReturnOwnerID) {
		exception := core.NewLocalJumpErrorWithReturn(result)
		if vm.raiseException(frame, exception) {
			return exception, true
		}
		vm.returnUnhandledException(frame, exception)
		return exception, true
	}
	vm.pendingReturnTargetID = frame.Closure.ReturnOwnerID
	vm.pendingReturnValue = result
	vm.push(result)
	frame.Returned = true
	return result, true
}

func (vm *VM) executeRegisterIRNeg(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, true
	}
	frame.Ip = instruction.byteIP
	value := registers[instruction.left]
	if value == nil {
		value = core.R.NilVal
	}
	result := vm.negate(value)
	if result != nil && result.Type == object.ValueException {
		if !vm.raiseException(frame, result) {
			vm.returnUnhandledException(frame, result)
		}
		return result, true
	}
	return result, false
}

func (vm *VM) executeRegisterIRNotEqual(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, true
	}
	frame.Ip = instruction.byteIP
	left := registers[instruction.left]
	if left == nil {
		left = core.R.NilVal
	}
	right := registers[instruction.right]
	if right == nil {
		right = core.R.NilVal
	}
	if result, handled := core.EvaluateExpectationNotEqual(left, right); handled {
		return result, false
	}
	result := vm.send(left, "!=", []*object.EmeraldValue{right})
	if result == nil {
		result = core.R.NilVal
	}
	if result.Type == object.ValueException {
		if !vm.raiseException(frame, result) {
			vm.returnUnhandledException(frame, result)
		}
		return result, true
	}
	return result, false
}

// executeRegisterIRBatchSend is the narrow hot-send ABI for a repeated
// Register IR callback. The enclosing loop has already established a
// generation epoch and the instruction cache has already resolved the same
// receiver/class slot. When that slot contains a framed/native Go function,
// call it with fixed arguments directly; every other shape uses the complete
// send path for the current instruction. In particular, a miss never returns
// false after a preceding instruction has mutated user-visible state.
func (vm *VM) executeRegisterIRBatchSend(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue, generation uint64, allowCache bool) (*object.EmeraldValue, bool) {
	if vm == nil || frame == nil || !allowCache || generation == 0 || generation != object.CurrentMethodGeneration() ||
		instruction.blockPresent || instruction.splatIndex != 255 || instruction.opcode != compiler.OpSend || instruction.implicit ||
		instruction.argc > 4 || instruction.cache == nil || vm.currentBlock != nil || core.AnyTracePointActive() ||
		vm.pendingEscapedThrowHandler != nil {
		return vm.executeRegisterIRSend(frame, instruction, registers, allowCache)
	}
	receiver := registers[instruction.left]
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	if receiver == nil {
		receiver = core.R.NilVal
	}
	if receiver.Type == object.ValueProc && (instruction.name == "call" || instruction.name == "[]" || instruction.name == "yield") {
		return vm.executeRegisterIRSend(frame, instruction, registers, allowCache)
	}
	identityCache := registerIRCacheableClassReceiver(receiver)
	if !identityCache && !vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) {
		return vm.executeRegisterIRSend(frame, instruction, registers, allowCache)
	}
	cache := instruction.cache
	if cache.generation != generation {
		return vm.executeRegisterIRSend(frame, instruction, registers, allowCache)
	}
	var cachedMethod *object.Method
	var cachedOwner *object.Class
	var cachedInlineLeaf *leafMethodPlan
	var cachedInlineFn *object.Function
	var nativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	if identityCache && cache.receiver == receiver || !identityCache && cache.receiver == nil && cache.class == receiver.Class {
		cachedMethod = cache.method
		cachedOwner = cache.owner
		cachedInlineLeaf = cache.inlineLeaf
		cachedInlineFn = cache.inlineFn
		nativeFn = cache.framedNativeFn
		if nativeFn == nil {
			nativeFn = cache.nativeFn
		}
	} else if registerIRPolymorphicSendCacheEnabled &&
		(identityCache && cache.secondReceiver == receiver || !identityCache && cache.secondReceiver == nil && cache.secondClass == receiver.Class) {
		cachedMethod = cache.secondMethod
		cachedOwner = cache.secondOwner
		cachedInlineLeaf = cache.secondLeaf
		cachedInlineFn = cache.secondFn
		nativeFn = cache.secondFramedNativeFn
		if nativeFn == nil {
			nativeFn = cache.secondNativeFn
		}
	}
	if cachedMethod == nil && nativeFn == nil {
		return vm.executeRegisterIRSend(frame, instruction, registers, allowCache)
	}
	var argsStorage [4]*object.EmeraldValue
	args := argsStorage[:int(instruction.argc)]
	for index := range args {
		args[index] = registers[instruction.args[index]]
	}
	if cachedMethod != nil && (cachedMethod.Visibility == "" || cachedMethod.Visibility == "public") {
		// Keep the same source-identity native hooks that the ordinary cached
		// send checks before its native ABI. The batch entry must not bypass a
		// Prawn/PDF intrinsic merely because the Ruby method also has a fixed
		// bytecode shape.
		if nativePDFDispatchCandidateForMethod(receiver, cachedMethod) {
			if result, executed := vm.executeNativePDFDispatch(cachedMethod, receiver, args, false); executed {
				return vm.finishRegisterIRBatchNativeSend(frame, result)
			}
		}
		if instruction.name == "compute_width_of" || instruction.name == "kern" || instruction.name == "unscaled_width_of" {
			if result, executed := vm.executeNativePrawnTextMethod(cachedMethod, receiver, args); executed {
				return vm.finishRegisterIRBatchNativeSend(frame, result)
			}
		}
		if instruction.name == "width_of" {
			if result, executed := vm.executeNativePrawnFontMetricWidth(cachedMethod, receiver, args); executed {
				return vm.finishRegisterIRBatchNativeSend(frame, result)
			}
		}
		if instruction.name == "soft_hyphen" || instruction.name == "zero_width_space" || instruction.name == "whitespace" ||
			instruction.name == "break_chars" || instruction.name == "hyphen" || instruction.name == "tokenize" ||
			instruction.name == "add_fragment_to_line" || instruction.name == "fragment_finished" {
			if result, executed := vm.executeNativePrawnLineWrapMethod(cachedMethod, receiver, args); executed {
				return vm.finishRegisterIRBatchNativeSend(frame, result)
			}
		}
		if instruction.name == "process_text" {
			if result, executed := vm.executeNativePrawnFormattedProcessText(cachedMethod, receiver, args); executed {
				return vm.finishRegisterIRBatchNativeSend(frame, result)
			}
		}
	}
	if nativeFn == nil && cachedMethod != nil && !instruction.blockPresent &&
		registerIRBatchFixedBytecodeEligible(cachedMethod, instruction.argc) &&
		!core.AnyTracePointActive() {
		// A pure integer-only Ruby callee has no receiver/container mutation and
		// can be entered through the already-proven direct Register IR ABI. The
		// ordinary batch path used to send it straight to the fixed-bytecode
		// Frame entry, paying a second Ruby instruction loop even though the call
		// site had already cached its leaf plan. Keep this admission deliberately
		// narrower than the general framed-plan experiment: non-integer plans
		// retain the established bytecode path, while a direct-proof miss occurs
		// before the callee executes and falls through unchanged.
		if cachedInlineLeaf != nil && cachedInlineFn != nil &&
			cachedInlineLeaf.kind == leafMethodRegisterIR && cachedInlineLeaf.registerIR != nil &&
			cachedInlineLeaf.registerIR.integerOnly {
			if result, executed := vm.executeRegisterIRTrustedDirectSend(instruction, registers, false); executed {
				return vm.finishRegisterIRBatchNativeSend(frame, result)
			}
		}
		frame.Ip = instruction.byteIP
		if result, executed := vm.executeCachedFixedArityRubyBytecodeTrusted(receiver, instruction.name, args, cachedMethod, cachedOwner); executed {
			return vm.finishRegisterIRBatchNativeSend(frame, result)
		}
	}
	if nativeFn == nil {
		return vm.executeRegisterIRSend(frame, instruction, registers, allowCache)
	}
	frame.Ip = instruction.byteIP
	result := callRegisterIRBatchNative(nativeFn, receiver, instruction, registers)
	return vm.finishRegisterIRBatchNativeSend(frame, result)
}

func callRegisterIRBatchNative(fn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue, receiver *object.EmeraldValue, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) *object.EmeraldValue {
	switch instruction.argc {
	case 0:
		return fn(receiver)
	case 1:
		return fn(receiver, registers[instruction.args[0]])
	case 2:
		return fn(receiver, registers[instruction.args[0]], registers[instruction.args[1]])
	case 3:
		return fn(receiver, registers[instruction.args[0]], registers[instruction.args[1]], registers[instruction.args[2]])
	case 4:
		return fn(receiver, registers[instruction.args[0]], registers[instruction.args[1]], registers[instruction.args[2]], registers[instruction.args[3]])
	default:
		return nil
	}
}

func (vm *VM) finishRegisterIRBatchNativeSend(frame *Frame, result *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm.pendingEscapedThrowHandler != nil {
		handler := vm.pendingEscapedThrowHandler
		value := vm.pendingEscapedThrowValue
		vm.pendingEscapedThrowHandler = nil
		vm.pendingEscapedThrowValue = nil
		if !vm.routeThrowThroughEnsure(frame, handler, value) {
			vm.completeThrow(frame, handler, value)
		}
		return value, true
	}
	vm.consumeCompletedBreakMarker()
	if vm.threadDepth > 0 && core.IsThreadBlockedResult(result) {
		frame.Returned = true
		return result, true
	}
	if core.IsTerminationResult(result) || shouldPropagateExceptionValue(result) {
		if !vm.raiseException(frame, result) {
			vm.returnUnhandledException(frame, result)
		}
		return result, true
	}
	return result, false
}

func (vm *VM) executeRegisterIRSend(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue, allowCache bool) (*object.EmeraldValue, bool) {
	if frame == nil {
		return nil, true
	}
	frame.Ip = instruction.byteIP
	isKeywordSend := instruction.opcode == compiler.OpSendWithKeywords
	isSetterSend := instruction.opcode == compiler.OpSendSetter
	previousBlock := vm.currentBlock
	if instruction.blockPresent {
		block, blockErr := vm.normalizeBlockPass(derefClosureValue(registers[instruction.block]))
		if blockErr != nil {
			if vm.raiseException(frame, blockErr) {
				return blockErr, true
			}
			vm.returnUnhandledException(frame, blockErr)
			return blockErr, true
		}
		vm.currentBlock = block
	}
	var args []*object.EmeraldValue
	if instruction.argc > 0 {
		var argStorage [4]*object.EmeraldValue
		args = argStorage[:int(instruction.argc)]
		for index := range args {
			args[index] = registers[instruction.args[index]]
		}
	}
	if instruction.splatIndex != 255 {
		expanded, errVal := vm.expandMethodSplatArgs(args, int(instruction.splatIndex))
		if errVal != nil {
			if vm.raiseException(frame, errVal) {
				if instruction.blockPresent {
					vm.currentBlock = previousBlock
				}
				return errVal, true
			}
			vm.returnUnhandledException(frame, errVal)
			if instruction.blockPresent {
				vm.currentBlock = previousBlock
			}
			return errVal, true
		}
		args = expanded
	}
	if isKeywordSend && len(args) > 0 {
		last := args[len(args)-1]
		if last == nil || last.Type == object.ValueNil {
			args = args[:len(args)-1]
		} else if last.Type == object.ValueHash {
			if len(executorHashToMap(last)) == 0 {
				args = args[:len(args)-1]
			} else {
				last = copyKeywordHash(last)
				args[len(args)-1] = last
				core.MarkRuby2KeywordHash(last)
			}
		}
	}
	assignedValue := core.R.NilVal
	if isSetterSend && len(args) > 0 {
		assignedValue = args[len(args)-1]
	}
	receiver := registers[instruction.left]
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	var result *object.EmeraldValue
	// Keyword sends used to bypass the Register IR send cache entirely.  They
	// can reuse the method/owner cache as long as the final dispatch still goes
	// through invokeMethod with keywordSyntax=true; that preserves Ruby keyword
	// normalization, arity errors, visibility and exception behavior.
	identityCache := allowCache && registerIRCacheableClassReceiver(receiver)
	blockCache := allowCache && instruction.blockPresent && registerIRBlockSendCacheEnabled &&
		registerIRBlockSendCacheName(instruction.name) &&
		(vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) || vm.registerIRExactBuiltinCollectionReceiver(receiver))
	cacheable := allowCache && instruction.cache != nil && registerIRCacheableSendName(instruction.name) &&
		(!instruction.blockPresent && (vm.registerIRCacheableReceiverForMethod(receiver, instruction.name) || identityCache) || blockCache)
	var cachedMethod *object.Method
	var cachedOwner *object.Class
	var cachedInlineLeaf *leafMethodPlan
	var cachedInlineFn *object.Function
	var cachedInlinePlan *registerIRPlan
	var cachedNativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	var cachedFramedNativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	var cachedDirectIndex bool
	var cachedBytecodeFixedArity *bool
	var resolvedMethod *object.Method
	var resolvedOwner *object.Class
	var resolvedFallback *object.EmeraldValue
	resolvedFallbackReady := false
	if cacheable {
		if identityCache && instruction.cache.receiver == receiver || !identityCache && instruction.cache.receiver == nil && instruction.cache.class == receiver.Class {
			cachedMethod = instruction.cache.method
			cachedOwner = instruction.cache.owner
			cachedInlineLeaf = instruction.cache.inlineLeaf
			cachedInlineFn = instruction.cache.inlineFn
			cachedInlinePlan = instruction.cache.inlinePlan
			cachedNativeFn = instruction.cache.nativeFn
			cachedFramedNativeFn = instruction.cache.framedNativeFn
			cachedDirectIndex = instruction.cache.directIndex
			cachedBytecodeFixedArity = &instruction.cache.bytecodeFixedArity
		} else if registerIRPolymorphicSendCacheEnabled && (identityCache && instruction.cache.secondReceiver == receiver || !identityCache && instruction.cache.secondReceiver == nil && instruction.cache.secondClass == receiver.Class) {
			cachedMethod = instruction.cache.secondMethod
			cachedOwner = instruction.cache.secondOwner
			cachedInlineLeaf = instruction.cache.secondLeaf
			cachedInlineFn = instruction.cache.secondFn
			cachedInlinePlan = instruction.cache.secondInlinePlan
			cachedNativeFn = instruction.cache.secondNativeFn
			cachedFramedNativeFn = instruction.cache.secondFramedNativeFn
			cachedDirectIndex = instruction.cache.secondDirectIndex
			cachedBytecodeFixedArity = &instruction.cache.secondBytecodeFixedArity
		}
	}
	cacheHit := cachedMethod != nil && instruction.cache.generation == object.CurrentMethodGeneration()
	nativePDFExecuted := false
	if cacheHit && nativePDFDispatchCandidateForMethod(receiver, cachedMethod) &&
		(cachedMethod.Visibility == "" || cachedMethod.Visibility == "public" || instruction.implicit) {
		if nativeResult, executed := vm.executeNativePDFDispatch(cachedMethod, receiver, args, isKeywordSend); executed {
			result = nativeResult
			nativePDFExecuted = true
		}
	}
	nativePrawnTextExecuted := false
	if cacheHit && !instruction.blockPresent && !isKeywordSend &&
		(instruction.name == "compute_width_of" || instruction.name == "kern" || instruction.name == "unscaled_width_of") &&
		(cachedMethod.Visibility == "" || cachedMethod.Visibility == "public") {
		if nativeResult, executed := vm.executeNativePrawnTextMethod(cachedMethod, receiver, args); executed {
			result = nativeResult
			nativePrawnTextExecuted = true
		}
	}
	nativePrawnFontMetricExecuted := false
	if cacheHit && !instruction.blockPresent && instruction.name == "width_of" {
		if nativeResult, executed := vm.executeNativePrawnFontMetricWidth(cachedMethod, receiver, args); executed {
			result = nativeResult
			nativePrawnFontMetricExecuted = true
		}
	}
	nativePrawnLineWrapExecuted := false
	if cacheHit && !instruction.blockPresent && !isKeywordSend &&
		(instruction.name == "soft_hyphen" || instruction.name == "zero_width_space" || instruction.name == "whitespace" ||
			instruction.name == "break_chars" || instruction.name == "hyphen" || instruction.name == "tokenize" ||
			instruction.name == "add_fragment_to_line" || instruction.name == "fragment_finished") {
		if nativeResult, executed := vm.executeNativePrawnLineWrapMethod(cachedMethod, receiver, args); executed {
			result = nativeResult
			nativePrawnLineWrapExecuted = true
		}
	}
	nativeCachedExecuted := false
	if cacheHit && !instruction.blockPresent && !isKeywordSend && !cachedDirectIndex &&
		!core.AnyTracePointActive() &&
		(receiver == nil || receiver.Type != object.ValueProc || instruction.name != "call" && instruction.name != "[]" && instruction.name != "yield") {
		nativeFn := cachedFramedNativeFn
		if nativeFn == nil {
			nativeFn = cachedNativeFn
		}
		if nativeFn != nil {
			result = callNativeMethod(nativeFn, receiver, args)
			nativeCachedExecuted = true
		}
	}
	if nativePDFExecuted {
	} else if nativePrawnTextExecuted {
	} else if nativePrawnFontMetricExecuted {
	} else if nativePrawnLineWrapExecuted {
	} else if nativeCachedExecuted {
	} else if cacheHit {
		if !instruction.blockPresent && isKeywordSend {
			if fastResult, fastExecuted := vm.executeKeywordMethodNoSendFast(cachedMethod, receiver, args); fastExecuted {
				result = fastResult
			} else {
				result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, true)
			}
		} else if !instruction.blockPresent && cachedDirectIndex && len(args) == 1 {
			if directResult, directExecuted := vm.executeRegisterIRCachedIndex(receiver, args[0], true); directExecuted {
				result = directResult
			} else if cachedRubyBytecodeFrameEnabled {
				if frameResult, frameExecuted := vm.executeCachedFixedArityRubyBytecodeForSend(receiver, instruction.name, args, cachedMethod, cachedOwner, instruction.implicit, cachedBytecodeFixedArity); frameExecuted {
					result = frameResult
				} else {
					result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, false)
				}
			} else {
				result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, false)
			}
		} else if !instruction.blockPresent && (cachedFramedNativeFn != nil || cachedNativeFn != nil) &&
			(receiver == nil || receiver.Type != object.ValueProc || instruction.name != "call" && instruction.name != "[]" && instruction.name != "yield") {
			nativeFn := cachedFramedNativeFn
			if nativeFn == nil {
				nativeFn = cachedNativeFn
			}
			result = callNativeMethod(nativeFn, receiver, args)
		} else if !instruction.blockPresent && cachedInlineLeaf != nil && !isKeywordSend && (cachedInlineFn != nil || cachedInlineLeaf.kind == leafMethodInstanceReader || cachedInlineLeaf.kind == leafMethodInstanceWriter) {
			var cacheResult *object.EmeraldValue
			var cacheExecuted bool
			cacheResult, cacheExecuted = vm.executeRegisterIRInlineLeaf(cachedInlineLeaf, cachedInlineFn, receiver, args, cachedMethod, cachedOwner)
			result = cacheResult
			if !cacheExecuted {
				// A leaf proof is stronger than the framed plan, but its runtime
				// guard can still miss for a receiver-specific shape (for example
				// an object with an instance override).  The call site already
				// retained the exact-arity framed IR for this method; use it before
				// paying invokeMethod's full lookup, binder, and plan probes.
				if registerIRFramedSendInlineEnabled && cachedInlinePlan != nil && cachedInlineFn != nil && vm.currentBlock == nil {
					if framedResult, framedExecuted := vm.executeRegisterIR(cachedInlinePlan, cachedInlineFn, receiver, args, instruction.name, cachedMethod, cachedOwner, true); framedExecuted {
						result = framedResult
						cacheExecuted = true
					}
				}
				if !cacheExecuted {
					// A cached leaf/framed plan can reject a terminal-mutation
					// method even though its method-level direct plan is safe:
					// the latter is allowed to side-exit before the final ivar
					// write, while the framed plan retains the full bytecode
					// protocol. Reuse the already-proven method cache before
					// manufacturing another Ruby Frame.
					if hotResult, hotExecuted := vm.tryExecuteCachedTypedHotMethod(cachedMethod, receiver, args); hotExecuted {
						result = hotResult
						cacheExecuted = true
					}
				}
				if !cacheExecuted && cachedRubyBytecodeFrameEnabled {
					if frameResult, frameExecuted := vm.executeCachedFixedArityRubyBytecodeForSend(receiver, instruction.name, args, cachedMethod, cachedOwner, instruction.implicit, cachedBytecodeFixedArity); frameExecuted {
						result = frameResult
						cacheExecuted = true
					}
				}
				if !cacheExecuted {
					result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, false)
				}
			}
		} else if !instruction.blockPresent && registerIRFramedSendInlineEnabled && !isKeywordSend && cachedInlinePlan != nil && cachedInlineFn != nil && vm.currentBlock == nil {
			// Fixed positional Ruby methods that need a real Frame can still reuse
			// their predecoded Register IR.  The cache was populated only after a
			// public exact-arity method lookup, so this skips invokeMethod's second
			// plan lookup/binder probe while retaining the framed executor and its
			// normal send/exception behavior.  A guard miss falls back before the
			// method is user-visible.
			if cacheResult, cacheExecuted := vm.executeRegisterIR(cachedInlinePlan, cachedInlineFn, receiver, args, instruction.name, cachedMethod, cachedOwner, true); cacheExecuted {
				result = cacheResult
			} else if hotResult, hotExecuted := vm.tryExecuteCachedTypedHotMethod(cachedMethod, receiver, args); hotExecuted {
				result = hotResult
			} else if cachedRubyBytecodeFrameEnabled {
				if frameResult, frameExecuted := vm.executeCachedFixedArityRubyBytecodeForSend(receiver, instruction.name, args, cachedMethod, cachedOwner, instruction.implicit, cachedBytecodeFixedArity); frameExecuted {
					result = frameResult
				} else {
					result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, false)
				}
			} else {
				result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, false)
			}
		} else if instruction.blockPresent && !isKeywordSend && cachedFramedNativeFn != nil &&
			(receiver == nil || receiver.Type != object.ValueProc || instruction.name != "call" && instruction.name != "[]" && instruction.name != "yield") {
			// Built-in iterator sends are already guarded by the exact call-site
			// cache (receiver/class identity, method generation, visibility and
			// refinement state).  Native iterator bodies consume the current block
			// through the VM hook, so invoking the cached function directly keeps
			// block/break/next semantics while removing invokeMethod's repeated
			// visibility, binder and native dispatch probes.  Any non-native or
			// unsupported block-cache entry remains on the complete path below.
			result = callNativeMethod(cachedFramedNativeFn, receiver, args)
		} else if instruction.blockPresent && !isKeywordSend && registerIRFramedSendInlineEnabled &&
			cachedInlinePlan != nil && cachedInlineFn != nil && !core.AnyTracePointActive() {
			// A fixed-arity Ruby method that does not expose rest/keyword/block
			// binding can retain the block in the normal Frame and execute its
			// already-decoded IR directly.  executeRegisterIR keeps frame.Block,
			// yield, break/next and exception state intact; a failed plan guard
			// deopts to invokeMethod before any user-visible result is returned.
			if cacheResult, cacheExecuted := vm.executeRegisterIR(cachedInlinePlan, cachedInlineFn, receiver, args, instruction.name, cachedMethod, cachedOwner, true); cacheExecuted {
				result = cacheResult
			} else {
				result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, isKeywordSend)
			}
		} else if !instruction.blockPresent && !isKeywordSend {
			if hotResult, hotExecuted := vm.tryExecuteCachedTypedHotMethod(cachedMethod, receiver, args); hotExecuted {
				result = hotResult
			} else if cachedRubyBytecodeFrameEnabled {
				if frameResult, frameExecuted := vm.executeCachedFixedArityRubyBytecodeForSend(receiver, instruction.name, args, cachedMethod, cachedOwner, instruction.implicit, cachedBytecodeFixedArity); frameExecuted {
					result = frameResult
				} else {
					result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, isKeywordSend)
				}
			} else {
				result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, isKeywordSend)
			}
		} else {
			result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, cachedMethod, cachedOwner, isKeywordSend)
		}
		if isVisibilityNoMethodErrorFor(result, instruction.name) {
			if missingResult := vm.callMethodMissingForSend(receiver, instruction.name, args); missingResult != nil {
				result = missingResult
			}
		}
	} else {
		// Class/Module receivers need an identity key, but their first lookup
		// must still use the normal visibility and dynamic-missing protocol.
		// Direct hits are invoked exactly once and then cached; fallbacks remain
		// uncached because method_missing/accessor dispatch is receiver-specific.
		if identityCache && cacheable {
			candidate, owner, fallback := vm.lookupMethodForSend(receiver, instruction.name, args, false, true)
			if fallback == nil && candidate != nil && candidate.Visibility != "undefined" {
				resolvedMethod = candidate
				resolvedOwner = owner
			} else if fallback != nil {
				resolvedFallback = fallback
				resolvedFallbackReady = true
			}
		}
		if resolvedMethod != nil {
			result = vm.invokeMethod(receiver, instruction.name, instruction.name, args, resolvedMethod, resolvedOwner, isKeywordSend)
		} else if resolvedFallbackReady {
			if numberedParameterMethodNamePattern.MatchString(instruction.name) {
				markExceptionRaised(resolvedFallback)
				result = resolvedFallback
			} else if missingResult := vm.callMethodMissingForSend(receiver, instruction.name, args); missingResult != nil {
				result = missingResult
			} else {
				result = resolvedFallback
			}
		} else if isKeywordSend {
			result = vm.sendWithKeywords(receiver, instruction.name, args)
		} else {
			result = vm.send(receiver, instruction.name, args)
		}
		if cacheable {
			methodObj, owner, ok := resolvedMethod, resolvedOwner, resolvedMethod != nil
			if !ok {
				methodObj, owner, ok = vm.cachedPlainMethod(receiver, instruction.name)
			}
			if ok {
				if methodObj, owner, ok = resolveVisibilityAliasMethod(instruction.name, methodObj, owner); ok && methodObj.Visibility != "undefined" {
					generation := object.CurrentMethodGeneration()
					if instruction.cache.generation != generation {
						*instruction.cache = registerIRSendCache{generation: generation}
					}
					firstMatches := identityCache && instruction.cache.receiver == receiver || !identityCache && instruction.cache.receiver == nil && instruction.cache.class == receiver.Class
					if instruction.cache.method == nil || firstMatches || !registerIRPolymorphicSendCacheEnabled {
						if identityCache {
							instruction.cache.receiver = receiver
						} else {
							instruction.cache.receiver = nil
						}
						instruction.cache.class = receiver.Class
						instruction.cache.method = methodObj
						instruction.cache.owner = owner
						instruction.cache.bytecodeFixedArity = false
						if registerIRInlineEnabled {
							instruction.cache.inlineLeaf, instruction.cache.inlineFn = vm.registerIRInlineableLeaf(methodObj, instruction.argc, instruction.implicit)
							if registerIRFramedSendInlineEnabled {
								// Keep both tiers. A method can have a useful no-frame
								// leaf shape whose runtime guard misses (for example a
								// custom collection receiver) while its exact-arity
								// framed IR remains safe to reuse. Previously the framed
								// plan was cached only when the leaf proof failed at
								// compile time, so a failed leaf probe paid invokeMethod
								// on every hot call.
								var framedFn *object.Function
								instruction.cache.inlinePlan, framedFn = vm.registerIRBytecodeInlinePlan(methodObj, instruction.argc)
								if instruction.cache.inlineFn == nil {
									instruction.cache.inlineFn = framedFn
								}
							}
						}
						instruction.cache.nativeFn = registerIRDirectNativeFn(methodObj, instruction.name)
						instruction.cache.framedNativeFn = registerIRFramedNativeFn(methodObj, instruction.name)
						instruction.cache.directIndex = registerIRDirectIndexEligible(receiver, instruction.name, instruction.argc, methodObj, owner)
					} else {
						if identityCache {
							instruction.cache.secondReceiver = receiver
						} else {
							instruction.cache.secondReceiver = nil
						}
						instruction.cache.secondClass = receiver.Class
						instruction.cache.secondMethod = methodObj
						instruction.cache.secondOwner = owner
						instruction.cache.secondBytecodeFixedArity = false
						if registerIRInlineEnabled {
							instruction.cache.secondLeaf, instruction.cache.secondFn = vm.registerIRInlineableLeaf(methodObj, instruction.argc, instruction.implicit)
							if registerIRFramedSendInlineEnabled {
								var framedFn *object.Function
								instruction.cache.secondInlinePlan, framedFn = vm.registerIRBytecodeInlinePlan(methodObj, instruction.argc)
								if instruction.cache.secondFn == nil {
									instruction.cache.secondFn = framedFn
								}
							}
						}
						instruction.cache.secondNativeFn = registerIRDirectNativeFn(methodObj, instruction.name)
						instruction.cache.secondFramedNativeFn = registerIRFramedNativeFn(methodObj, instruction.name)
						instruction.cache.secondDirectIndex = registerIRDirectIndexEligible(receiver, instruction.name, instruction.argc, methodObj, owner)
					}
				}
			}
		}
	}
	if vm.pendingEscapedThrowHandler != nil {
		handler := vm.pendingEscapedThrowHandler
		value := vm.pendingEscapedThrowValue
		vm.pendingEscapedThrowHandler = nil
		vm.pendingEscapedThrowValue = nil
		if !vm.routeThrowThroughEnsure(frame, handler, value) {
			vm.completeThrow(frame, handler, value)
		}
		if instruction.blockPresent {
			vm.currentBlock = previousBlock
		}
		return value, true
	}
	vm.consumeCompletedBreakMarker()
	if vm.threadDepth > 0 && core.IsThreadBlockedResult(result) {
		frame.Returned = true
		if instruction.blockPresent {
			vm.currentBlock = previousBlock
		}
		return result, true
	}
	if instruction.implicit {
		var implicitIdentifierCause *object.EmeraldValue
		if len(vm.activeRescues) > 0 {
			implicitIdentifierCause = core.LastException
		}
		result = implicitIdentifierNameError(vm, result, receiver, instruction.name, implicitIdentifierCause)
	}
	if core.IsTerminationResult(result) || shouldPropagateExceptionValue(result) {
		if !vm.raiseException(frame, result) {
			vm.returnUnhandledException(frame, result)
		}
		if instruction.blockPresent {
			vm.currentBlock = previousBlock
		}
		return result, true
	}
	if isSetterSend {
		result = assignedValue
	}
	if instruction.blockPresent {
		vm.currentBlock = previousBlock
	}
	return result, false
}

func (vm *VM) executeRegisterIRYield(frame *Frame, instruction registerIRInstruction, registers *[16]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || frame == nil {
		return nil, true
	}
	frame.Ip = instruction.byteIP
	block := vm.yieldBlock()
	if block == nil {
		errVal := core.NewLocalJumpError("no block given")
		if vm.raiseException(frame, errVal) {
			return errVal, true
		}
		vm.returnUnhandledException(frame, errVal)
		return errVal, true
	}
	var argsStorage [4]*object.EmeraldValue
	args := argsStorage[:int(instruction.argc)]
	for index := range args {
		args[index] = registers[instruction.args[index]]
	}
	if instruction.splatIndex != 255 {
		expanded, errVal := vm.expandYieldSplatArgs(args, int(instruction.splatIndex))
		if errVal != nil {
			if vm.raiseException(frame, errVal) {
				return errVal, true
			}
			vm.returnUnhandledException(frame, errVal)
			return errVal, true
		}
		args = expanded
	}
	result := vm.callBlock(block, args...)
	if shouldPropagateExceptionValue(result) {
		if vm.raiseException(frame, result) {
			return result, true
		}
		vm.returnUnhandledException(frame, result)
		return result, true
	}
	return result, false
}

func registerIRDirectNativeFn(methodObj *object.Method, name string) func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue {
	if methodObj == nil || methodObj.DispatchOwner != nil || !registerIRDirectNativeName(name) ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public" && !(methodObj.Visibility == "private" && (name == "Array" || name == "Integer" || name == "String" || name == "format"))) {
		return nil
	}
	fn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue)
	if !ok {
		return nil
	}
	return fn
}

// registerIRFramedNativeFn is broader than the speculative no-frame native
// whitelist. A framed Register IR send already has the caller's unwind state
// and has completed normal method lookup/visibility checks, so it can call a
// cached public builtin directly. Keep the handful of invokeMethod wrappers
// that add backtrace or Proc keyword semantics on the ordinary path.
func registerIRFramedNativeFn(methodObj *object.Method, name string) func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue {
	if methodObj == nil || methodObj.DispatchOwner != nil ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") ||
		name == "sleep" || name == "tap" || methodObj.OriginalName == "tap" {
		return nil
	}
	fn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue)
	if !ok {
		return nil
	}
	return fn
}

func registerIRDirectIndexEligible(receiver *object.EmeraldValue, name string, argc uint8, methodObj *object.Method, owner *object.Class) bool {
	if receiver == nil || receiver.Class == nil || name != "[]" || argc != 1 || methodObj == nil || owner == nil ||
		methodObj.DispatchOwner != nil || (methodObj.Visibility != "" && methodObj.Visibility != "public") {
		return false
	}
	if _, native := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); !native {
		return false
	}
	switch receiver.Type {
	case object.ValueArray:
		return receiver.Class == core.R.Classes["Array"] && owner == core.R.Classes["Array"] && core.ArrayIndexUsesBuiltinImplementation()
	case object.ValueHash:
		return receiver.Class == core.R.Classes["Hash"] && owner == core.R.Classes["Hash"] && core.HashIndexUsesBuiltinImplementation()
	default:
		return false
	}
}

func (vm *VM) executeRegisterIRCachedIndex(receiver, index *object.EmeraldValue, direct bool) (*object.EmeraldValue, bool) {
	if !direct || receiver == nil || index == nil {
		return nil, false
	}
	if receiver.Type == object.ValueHash {
		return core.DirectHashIndex(receiver, index)
	}
	return vm.executeRegisterIRDirectIndex(receiver, index)
}

func registerIRDirectNativeName(name string) bool {
	switch name {
	case "to_s", "to_str", "to_i", "to_f", "to_sym", "inspect", "size", "length", "last", "empty?", "count", "nil?", "frozen?", "class", "object_id", "hash", "key?", "has_key?", "include?", "member?", "start_with?", "end_with?", "strip", "upcase", "downcase", "encode", "pack", "force_encoding", "sub!", "unpack1", "format", "+@", "String", "abs", "zero?", "positive?", "negative?", "is_a?", "kind_of?", "instance_of?", "respond_to?", "new", "==", "!=", "eql?", "last_match", "exist?", "push", "Array", "Integer":
		return true
	default:
		return false
	}
}

func (vm *VM) registerIRInlineableLeaf(methodObj *object.Method, argc uint8, allowImplicitNonPublic bool, caseBranch ...bool) (*leafMethodPlan, *object.Function) {
	allowCaseBranch := len(caseBranch) > 0 && caseBranch[0]
	nonPublic := methodObj != nil && methodObj.Visibility != "" && methodObj.Visibility != "public"
	if methodObj == nil || methodObj.DispatchOwner != nil || nonPublic && (!allowImplicitNonPublic || methodObj.Visibility != "private" && methodObj.Visibility != "protected") || methodUsesRefinements(methodObj) {
		return nil, nil
	}
	if methodObj.AttrReaderName != "" && argc == 0 {
		return &leafMethodPlan{kind: leafMethodInstanceReader, name: methodObj.AttrReaderName}, nil
	}
	if methodObj.AttrWriterName != "" && argc == 1 {
		return &leafMethodPlan{kind: leafMethodInstanceWriter, name: methodObj.AttrWriterName}, nil
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok {
		return nil, nil
	}
	plan, found := vm.leafMethodCache[fn]
	if !found {
		plan = buildLeafMethodPlan(fn)
		vm.leafMethodCache[fn] = plan
	}
	optionalPlan := plan.kind == leafMethodOptionalInstanceReader || plan.kind == leafMethodOptionalInstanceFallbackReader
	if optionalPlan {
		// Optional readers are invoked with no positional arguments; their
		// default prologue is already proven by the leaf matcher.  A caller
		// carrying a block still uses the normal path, so methods with an
		// implicit block parameter remain semantically visible there.
		if argc != 0 {
			return nil, nil
		}
	} else if plan.kind == leafMethodRescueLiteral {
		if argc != 1 {
			return nil, nil
		}
	} else if plan.kind == leafMethodRescueEncoding {
		if argc > 1 {
			return nil, nil
		}
	} else if plan.kind == leafMethodRescueEncodeLiteral {
		if argc != 1 {
			return nil, nil
		}
	} else if len(fn.Params) != int(argc) || fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly {
		return nil, nil
	}
	if plan.kind != leafMethodInstanceReader && plan.kind != leafMethodInstanceWriter && plan.kind != leafMethodAttributeIntegerCompare && plan.kind != leafMethodInstanceFallbackIndex &&
		plan.kind != leafMethodOptionalInstanceReader && plan.kind != leafMethodOptionalInstanceFallbackReader && plan.kind != leafMethodRescueLiteral && plan.kind != leafMethodRescueEncoding && plan.kind != leafMethodRescueEncodeLiteral && plan.kind != leafMethodRegisterIR && plan.kind != leafMethodCaseDispatch {
		return nil, nil
	}
	if plan.kind == leafMethodCaseDispatch && plan.caseDispatch == nil {
		return nil, nil
	}
	if plan.kind == leafMethodRegisterIR {
		if plan.registerIR == nil {
			return nil, nil
		}
		allowConstants := allowCaseBranch || registerIRDirectConstantsSafe(vm, methodObj.Closure, plan.registerIR)
		if plan.registerIR.integerOnly || registerIRPlanSafeForDirectNoFrameWithOptions(plan.registerIR, false, allowConstants, allowCaseBranch) {
			// Proven scalar callees use the unboxed executor even when the
			// broader Ruby callee-chain experiment remains disabled. Direct-safe
			// boxed plans are equally constrained to native/accessor/index leaves.
		} else if registerIRCallChainEnabled {
			if !registerIRPlanSafeForNoFrameInline(plan.registerIR) {
				return nil, nil
			}
		} else if !registerIRPlanSafeWithoutFrame(plan.registerIR) {
			return nil, nil
		}
	}
	copy := plan
	return &copy, fn
}

// registerIRInlineableLeafForDirect admits a private module-function leaf only
// when the receiver is the exact module that owns it.  Normal leaf caching
// stays public-only because it does not carry the caller's private visibility
// context; the direct case-branch tier has already proven the receiver
// identity and therefore can preserve Ruby's private self-call rule.
func (vm *VM) registerIRInlineableLeafForDirect(methodObj *object.Method, receiver *object.EmeraldValue, argc uint8, caseBranch ...bool) (*leafMethodPlan, *object.Function) {
	if methodObj == nil || receiver == nil || methodObj.Visibility != "" && methodObj.Visibility != "public" && methodObj.Visibility != "private" {
		return nil, nil
	}
	publicMethod := methodObj.Visibility == "" || methodObj.Visibility == "public"
	ownerMatches := methodObj.Owner == receiver
	if !ownerMatches && methodObj.Owner == nil && publicMethod && receiver.Class != nil {
		// Builtin methods are installed directly on a Class and historically do
		// not retain an Owner on the Method value.  The exact receiver-class
		// guard on the surrounding send cache is sufficient for these native
		// leaves (notably attr_reader/attr_writer descriptors).
		if _, native := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); native {
			ownerMatches = true
		}
	}
	if !ownerMatches && methodObj.Owner != nil && methodObj.Owner.Type == object.ValueClass {
		if ownerClass, ok := methodObj.Owner.Data.(*object.Class); ok && ownerClass != nil {
			// Public instance methods may be inlined when the resolved owner is
			// in the receiver's exact class ancestry.  The receiver/class and
			// method-generation guards are already part of the send cache, while
			// the public-only restriction preserves private/module-function
			// visibility semantics handled by the ordinary dispatcher.
			if publicMethod && receiver.Class != nil && valueHasClassInAncestry(receiver, ownerClass) {
				ownerMatches = true
			} else {
				ownerMatches = ownerClass.SingletonOwner == receiver
			}
		}
	}
	if !ownerMatches {
		return nil, nil
	}
	copy := *methodObj
	copy.Visibility = "public"
	leaf, fn := vm.registerIRInlineableLeaf(&copy, argc, false, caseBranch...)
	if leaf == nil && len(caseBranch) > 0 && caseBranch[0] {
		if candidate, ok := copy.Fn.(*object.Function); ok {
			plan := buildLeafMethodPlan(candidate)
			if plan.kind == leafMethodUnsupported {
				// A direct class/module call has an exact receiver identity and
				// an outer deopt boundary.  Compile mutable string constants into
				// per-call clones for this private direct cache only; the normal
				// leaf cache deliberately keeps the compatibility default (where
				// string-literal allocation still requires a Ruby Frame).
				options := defaultRegisterIRCompileOptions()
				options.allowStringLiterals = true
				if compiled, compiledOK := compileRegisterIRWithOptions(candidate, options); compiledOK {
					plan = leafMethodPlan{kind: leafMethodRegisterIR, registerIR: compiled}
				}
			}
			if plan.kind == leafMethodRegisterIR && plan.registerIR != nil && registerIRPlanSafeForDirectNoFrameWithOptions(plan.registerIR, false, true, true) {
				planCopy := plan
				return &planCopy, candidate
			}
		}
	}
	return leaf, fn
}

func (vm *VM) executeRegisterIRInlineLeaf(plan *leafMethodPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue, methodObj *object.Method, methodOwner *object.Class) (*object.EmeraldValue, bool) {
	if plan == nil {
		return nil, false
	}
	switch plan.kind {
	case leafMethodInstanceReader:
		if len(args) != 0 {
			return nil, false
		}
		if value := core.DynamicInstanceVar(receiver, plan.name); value != nil {
			return value, true
		}
		return core.R.NilVal, true
	case leafMethodInstanceWriter:
		if len(args) != 1 {
			return nil, false
		}
		if result := core.SetDynamicInstanceVar(receiver, plan.name, args[0]); result != nil {
			return result, true
		}
		return args[0], true
	case leafMethodInstanceFallbackIndex:
		if len(args) != 0 || receiver == nil || plan.indexKey == nil {
			return nil, false
		}
		primary := core.DynamicInstanceVar(receiver, plan.name)
		if primary != nil && primary.IsTruthy() {
			return primary, true
		}
		secondary := core.DynamicInstanceVar(receiver, plan.secondaryName)
		if secondary == nil {
			secondary = core.R.NilVal
		}
		if !secondary.IsTruthy() {
			return secondary, true
		}
		if value, executed := core.DirectHashIndex(secondary, plan.indexKey); executed {
			return value, true
		}
		return nil, false
	case leafMethodOptionalInstanceReader, leafMethodOptionalInstanceFallbackReader, leafMethodRescueLiteral, leafMethodRescueEncoding, leafMethodRescueEncodeLiteral:
		if fn == nil || methodObj == nil {
			return nil, false
		}
		// The caller already carries the generation-scoped leaf plan from the
		// bytecode/Register IR send cache. Reusing it avoids a second
		// Function->leaf-plan map lookup on every hot optional-reader/rescue
		// invocation while preserving the same runtime guards below.
		return vm.executeCachedLeafMethodPlan(*plan, fn, receiver, args, methodObj.Name, methodObj, methodOwner)
	case leafMethodAttributeIntegerCompare:
		if fn == nil {
			return nil, false
		}
		return vm.executeAttributeIntegerComparePlan(fn, *plan, receiver, args, methodObj)
	case leafMethodCaseDispatch:
		if fn == nil || plan.caseDispatch == nil || len(args) != len(fn.Params) {
			return nil, false
		}
		start, direct, ok := vm.caseDispatchStart(plan.caseDispatch, args)
		if !ok {
			return nil, false
		}
		if direct != nil {
			return direct, true
		}
		// The selected branch has its own no-frame proof. If that proof or a
		// nested send cache misses, return false before user-visible work so the
		// enclosing caller can replay the complete framed method.
		return vm.executeCaseDispatchBranchNoFrame(plan.caseDispatch, start, fn, receiver, args)
	case leafMethodRegisterIR:
		// General nested Ruby register-IR callees remain opt-in: only the
		// already-proven integer-only subset is safe by default. Other callees
		// can contain sends whose unwind/backtrace behavior needs a dedicated
		// no-frame proof.
		if plan.registerIR == nil {
			return nil, false
		}
		if fn == nil {
			return nil, false
		}
		if result, executed := vm.tryExecuteTypedIntegerMethod(methodObj, fn, args); executed {
			return result, true
		}
		if result, executed := vm.tryExecuteTypedSSAFunction(methodObj, fn, receiver, args); executed {
			return result, true
		}
		allowConstants := methodObj != nil && registerIRDirectConstantsSafe(vm, methodObj.Closure, plan.registerIR)
		if !plan.registerIR.integerOnly && registerIRPlanSafeForDirectNoFrameWithOptions(plan.registerIR, false, allowConstants) {
			if vm.registerIRInlineDepth >= 8 {
				return nil, false
			}
			vm.registerIRInlineDepth++
			result, executed := vm.tryExecuteRegisterIRDirectNoFrame(plan.registerIR, fn, receiver, args, false, allowConstants)
			vm.registerIRInlineDepth--
			return result, executed
		}
		if !registerIRCallChainEnabled && !plan.registerIR.integerOnly &&
			!registerIRPlanSafeWithoutFrame(plan.registerIR) {
			return nil, false
		}
		if !registerIRPlanSafeForNoFrameInline(plan.registerIR) {
			return nil, false
		}
		return vm.executeRegisterIRInlineNoFramePlan(plan.registerIR, fn, receiver, args)
	default:
		return nil, false
	}
}

// executeRegisterIRInlineNoFramePlan executes a nested Ruby callee without
// entering invokeMethod or allocating a second Frame.  The depth cap keeps
// recursive/self-referential send caches on the normal VM path.
func (vm *VM) executeRegisterIRInlineNoFramePlan(plan *registerIRPlan, fn *object.Function, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || !registerIRPlanSafeForNoFrameInline(plan) || vm.registerIRInlineDepth >= 8 {
		return nil, false
	}
	vm.registerIRInlineDepth++
	var result *object.EmeraldValue
	var executed bool
	if plan.integerOnly {
		result, executed = vm.executeRegisterIRIntegerOnly(plan, fn, receiver, args)
	} else if plan.hasBranches {
		result, executed = vm.executeRegisterIRInstructions(plan, receiver, args, nil, true)
	} else {
		result, executed = vm.executeRegisterIRNoFrameLinear(plan, receiver, args, nil)
	}
	vm.registerIRInlineDepth--
	return result, executed
}

func registerIRSendCacheContextSafe(method *object.Method) bool {
	return method != nil && !methodUsesRefinements(method)
}

func registerIRCacheableReceiver(receiver *object.EmeraldValue) bool {
	if receiver == nil || receiver.Class == nil {
		return false
	}
	switch receiver.Type {
	case object.ValueBool, object.ValueNil, object.ValueInteger, object.ValueFloat, object.ValueSymbol:
		return true
	case object.ValueObject:
		instance, ok := receiver.Data.(*object.Object)
		return ok && instance != nil && len(instance.SingletonMethods) == 0 && instance.SingletonClass == nil
	default:
		return false
	}
}

// Native-backed values still dispatch through their stable Class method table;
// they only need the same singleton/native-singleton guards as ordinary Ruby
// objects. Exact built-in collections can therefore cache every ordinary
// method name for the framed send tier; the separate direct-native whitelist
// below still controls speculative no-frame execution.
func (vm *VM) registerIRCacheableReceiverForMethod(receiver *object.EmeraldValue, name string) bool {
	if registerIRCacheableReceiver(receiver) {
		return true
	}
	if receiver == nil || receiver.Class == nil {
		return false
	}
	// Class and Module values have per-object singleton method tables.  They
	// are cacheable only with the receiver identity included in the cache key;
	// unlike ordinary instances, their shared Class/Module class pointer is not
	// sufficient to distinguish two unrelated module functions.
	if registerIRCacheableClassReceiver(receiver) {
		return registerIRCacheableSendName(name)
	}
	if core.AttachedSingletonClass(receiver) != nil {
		return false
	}
	if vm != nil && len(vm.nativeSingletonMethods[nativeSingletonKey(receiver)]) > 0 {
		return false
	}
	if receiver.Type == object.ValueObject {
		if instance, ok := receiver.Data.(*object.Object); ok && instance != nil &&
			(len(instance.SingletonMethods) > 0 || instance.SingletonClass != nil) {
			return false
		}
	}
	switch receiver.Type {
	case object.ValueArray, object.ValueHash, object.ValueString:
		if name == "" {
			return false
		}
		if receiver.Type == object.ValueArray {
			return receiver.Class == core.R.Classes["Array"]
		}
		if receiver.Type == object.ValueHash {
			return receiver.Class == core.R.Classes["Hash"]
		}
		return receiver.Class == core.R.Classes["String"]
	case object.ValueObject, object.ValueRegexp, object.ValueRange:
		return true
	default:
		return false
	}
}

// registerIRExactBuiltinCollectionReceiver admits only the concrete built-in
// collection classes for the framed IR send cache.  Subclasses and values with
// an attached/native singleton stay on the conservative path; method
// redefinition is still covered by the cache generation guard.
func (vm *VM) registerIRExactBuiltinCollectionReceiver(receiver *object.EmeraldValue) bool {
	if vm == nil || !registerIRCollectionSendCacheEnabled || receiver == nil || receiver.Class == nil ||
		core.AttachedSingletonClass(receiver) != nil || len(vm.nativeSingletonMethods[nativeSingletonKey(receiver)]) > 0 {
		return false
	}
	switch receiver.Type {
	case object.ValueArray:
		return receiver.Class == core.R.Classes["Array"]
	case object.ValueHash:
		return receiver.Class == core.R.Classes["Hash"]
	case object.ValueString:
		return receiver.Class == core.R.Classes["String"]
	default:
		return false
	}
}

// registerIRStrictNativeCacheReceiver is the admission guard used when a
// direct no-frame plan needs to warm a nested send itself.  Exact Class/Module
// receivers are identity-keyed, so they may also warm a Ruby leaf (not only a
// native leaf); this is what lets module_function helpers stay inside a typed
// block.  Collection/scalar receivers remain limited to the native whitelist.
func (vm *VM) registerIRStrictNativeCacheReceiver(receiver *object.EmeraldValue, name string) bool {
	if vm == nil || receiver == nil || !registerIRCacheableSendName(name) {
		return false
	}
	// Immediate scalar values have no per-object singleton method table.  Their
	// class dispatch is just as stable as the exact Array/Hash/String leaves,
	// but the old guard admitted collections only, making even harmless
	// `Symbol#to_s`/`is_a?` sends deopt every direct block on its first probe.
	switch receiver.Type {
	case object.ValueBool, object.ValueNil, object.ValueInteger, object.ValueFloat, object.ValueSymbol:
		return receiver.Class != nil && core.AttachedSingletonClass(receiver) == nil
	case object.ValueClass, object.ValueModule:
		// Class/module sends are keyed by exact receiver identity below.  This
		// is necessary for module_function helpers such as PDF::Core#real and
		// for native Kernel/Class methods; a shared Class/Module class key would
		// otherwise alias unrelated namespaces.
		return receiver.Class != nil
	default:
		// Ordinary objects, regexp/range values and other native-backed values
		// can safely use an inherited native/accessor leaf when their exact class
		// and singleton state are stable.  The subsequent cache population still
		// requires a proven leaf (or the native whitelist); restricting this guard
		// to Array/Hash/String made accessor-heavy Gem objects deopt typed blocks.
		return vm.registerIRCacheableReceiverForMethod(receiver, name)
	}
}

// Class and Module values have per-object singleton/class-method tables, so a
// class-key cache would alias unrelated receivers.  Their identity is stable
// for the lifetime of the EmeraldValue, while method-generation invalidation
// covers singleton/class method redefinition and prepend/include changes.
func registerIRCacheableClassReceiver(receiver *object.EmeraldValue) bool {
	return receiver != nil && receiver.Class != nil &&
		(receiver.Type == object.ValueClass || receiver.Type == object.ValueModule)
}

func registerIRCacheableSendName(name string) bool {
	switch name {
	case "resolve_feature_path", "send", "__send__", "public_send", "instance_eval", "class_eval", "module_eval",
		"instance_exec", "class_exec", "module_exec", "__exec_class_body__", "eval":
		return false
	default:
		return name != ""
	}
}

// Block-bearing sends use the same generation/receiver/refinement guards as
// ordinary sends.  Keep the name filter aligned with the normal cache filter:
// dangerous meta-dispatch methods (send/eval/instance_exec, etc.) are already
// excluded there, while every ordinary method may now reuse either a cached
// native iterator or an exact fixed-arity framed Ruby plan.  A plan miss still
// falls through to invokeMethod, so this widens only the cache coverage, not
// the semantic fast-path admission.
func registerIRBlockSendCacheName(name string) bool {
	return registerIRCacheableSendName(name)
}

// registerIRBytecodeSendContextSafe keeps the hot-path check intentionally
// local to the current closure. The first cache fill still uses
// cachedPlainMethod's complete refinement checks; a closure with a fixed
// refinement snapshot never becomes an ordinary call-site cache afterwards.
func registerIRBytecodeSendContextSafe(frame *Frame) bool {
	if frame == nil {
		return false
	}
	return frame.Closure == nil || len(frame.Closure.Refinements) == 0
}

// executeBytecodeSendCache handles the common positional send path used by
// ordinary bytecode loops. Block-bearing calls are admitted only when the
// dedicated block cache is enabled; the callee still proves whether it can
// ignore the caller block before using the fixed Ruby entry.
func (vm *VM) executeBytecodeSendCache(frame *Frame, ip int, receiver *object.EmeraldValue, method string, args []*object.EmeraldValue, blockArg int) (*object.EmeraldValue, bool) {
	return vm.executeBytecodeSendCacheWithSyntax(frame, ip, receiver, method, args, blockArg, false)
}

// executeBytecodeKeywordSendCache is the keyword-preserving counterpart of
// executeBytecodeSendCache.  The cache is still tied to the bytecode call site
// and method generation; a hit only skips lookup and invokes the normal method
// protocol with keywordSyntax=true.
func (vm *VM) executeBytecodeKeywordSendCache(frame *Frame, ip int, receiver *object.EmeraldValue, method string, args []*object.EmeraldValue, blockArg int) (*object.EmeraldValue, bool) {
	if !registerIRKeywordSendCacheEnabled {
		return nil, false
	}
	return vm.executeBytecodeSendCacheWithSyntax(frame, ip, receiver, method, args, blockArg, true)
}

func (vm *VM) executeBytecodeSendCacheWithSyntax(frame *Frame, ip int, receiver *object.EmeraldValue, method string, args []*object.EmeraldValue, blockArg int, keywordSyntax bool) (*object.EmeraldValue, bool) {
	if !registerIRSendCacheEnabled || !registerIRBytecodeSendCacheEnabled || vm == nil || frame == nil || frame.Fn == nil ||
		(blockArg != 0 && blockArg != 1 && blockArg != 2 && blockArg != 3) ||
		(blockArg != 0 && !registerIRBytecodeBlockSendCacheEnabled) ||
		(keywordSyntax && blockArg != 0) ||
		(blockArg == 3 && len(vm.activeRescues) > 0) ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() ||
		!registerIRCacheableSendName(method) ||
		(!registerIRCacheableReceiver(receiver) && !vm.registerIRCacheableReceiverForMethod(receiver, method)) ||
		!registerIRBytecodeSendContextSafe(frame) {
		return nil, false
	}
	receiver = derefClosureValue(receiver)
	if receiver == nil || receiver.Class == nil {
		return nil, false
	}
	cache := vm.bytecodeSendCacheAt(frame, ip)
	if cache == nil {
		return nil, false
	}
	generation := object.CurrentMethodGeneration()
	if cache.bytecodeProbeGeneration != generation {
		cache.bytecodeProbeGeneration = generation
		cache.bytecodeProbeFailures = 0
		cache.bytecodeProbeDisabled = false
	}
	if cache.bytecodeProbeDisabled {
		// This call site has already demonstrated that its receiver set is too
		// polymorphic for the tiny cache.  Returning cached=false is deliberate:
		// the caller immediately uses the normal send path, with no semantic
		// change and no duplicate user-code execution.
		return nil, false
	}
	cacheEntryReady := cache.generation == generation && cache.class != nil
	if cacheEntryReady {
		if result, hit := vm.executeBytecodeCachedSend(cache, receiver, method, args, keywordSyntax, blockArg); hit {
			cache.bytecodeProbeFailures = 0
			return result, true
		}
		if cache.bytecodeProbeFailures < registerIRBytecodeSendCacheFailureLimit {
			cache.bytecodeProbeFailures++
		}
		if cache.bytecodeProbeFailures >= registerIRBytecodeSendCacheFailureLimit {
			cache.bytecodeProbeDisabled = true
		}
	}

	result := vm.sendWithCallInfo(receiver, method, args, keywordSyntax)
	vm.populateBytecodeSendCache(cache, generation, receiver, method, uint8(len(args)))
	return result, true
}

func (vm *VM) executeBytecodeCachedSend(cache *registerIRSendCache, receiver *object.EmeraldValue, method string, args []*object.EmeraldValue, keywordSyntax bool, blockArg int) (*object.EmeraldValue, bool) {
	if cache == nil || cache.generation != object.CurrentMethodGeneration() {
		return nil, false
	}
	var methodObj *object.Method
	var methodOwner *object.Class
	var leaf *leafMethodPlan
	var fn *object.Function
	var inlinePlan *registerIRPlan
	var nativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	var framedNativeFn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue
	var bytecodeFixedArity *bool
	switch {
	case cache.class == receiver.Class && (!registerIRCacheableClassReceiver(receiver) || cache.receiver == receiver):
		methodObj, methodOwner, leaf, fn, inlinePlan, nativeFn, framedNativeFn = cache.method, cache.owner, cache.inlineLeaf, cache.inlineFn, cache.inlinePlan, cache.nativeFn, cache.framedNativeFn
		bytecodeFixedArity = &cache.bytecodeFixedArity
	case registerIRPolymorphicSendCacheEnabled && cache.secondClass == receiver.Class && (!registerIRCacheableClassReceiver(receiver) || cache.secondReceiver == receiver):
		methodObj, methodOwner, leaf, fn, inlinePlan, nativeFn, framedNativeFn = cache.secondMethod, cache.secondOwner, cache.secondLeaf, cache.secondFn, cache.secondInlinePlan, cache.secondNativeFn, cache.secondFramedNativeFn
		bytecodeFixedArity = &cache.secondBytecodeFixedArity
	default:
		return nil, false
	}
	if methodObj == nil {
		return nil, false
	}
	// Once this call site has successfully entered the exact fixed-arity Ruby
	// bytecode loop, do not repeat the native/typed/forwardable probe ladder on
	// every hit. The bit is generation-scoped by the enclosing cache; a runtime
	// guard miss clears it and continues through the complete ladder below.
	if registerIRTrustedBytecodeEntryEnabled && blockArg == 0 && !keywordSyntax && bytecodeFixedArity != nil && *bytecodeFixedArity {
		if result, executed := vm.executeCachedFixedArityRubyBytecodeTrusted(receiver, method, args, methodObj, methodOwner); executed {
			return result, true
		}
		*bytecodeFixedArity = false
	}
	if nativePDFDispatchCandidateForMethod(receiver, methodObj) && (methodObj.Visibility == "" || methodObj.Visibility == "public") {
		if result, executed := vm.executeNativePDFDispatch(methodObj, receiver, args, keywordSyntax); executed {
			return result, true
		}
	}
	if method == "width_of" {
		if result, executed := vm.executeNativePrawnFontMetricWidth(methodObj, receiver, args); executed {
			return result, true
		}
	}
	if !keywordSyntax && blockArg == 0 &&
		(method == "soft_hyphen" || method == "zero_width_space" || method == "whitespace" || method == "break_chars" || method == "hyphen" || method == "tokenize" || method == "add_fragment_to_line" || method == "fragment_finished") {
		if result, executed := vm.executeNativePrawnLineWrapMethod(methodObj, receiver, args); executed {
			return result, true
		}
	}
	if !keywordSyntax && blockArg == 0 && method == "process_text" {
		if result, executed := vm.executeNativePrawnFormattedProcessText(methodObj, receiver, args); executed {
			return result, true
		}
	}
	// Keyword syntax does not change the ABI of a proven public native method:
	// argument normalization has already happened before the call-site cache,
	// and the native whitelist excludes Proc/sleep/tap backtrace-sensitive
	// wrappers.  Keep this before the generic keyword binder so keyword-heavy
	// Gem code does not re-enter invokeMethod for every native send.
	if keywordSyntax && nativeFn != nil &&
		(receiver == nil || receiver.Type != object.ValueProc || method != "call" && method != "[]" && method != "yield") {
		return callNativeMethod(nativeFn, receiver, args), true
	}
	if !keywordSyntax && nativeFn != nil &&
		(receiver == nil || receiver.Type != object.ValueProc || method != "call" && method != "[]" && method != "yield") {
		return callNativeMethod(nativeFn, receiver, args), true
	}
	if !keywordSyntax &&
		(method == "compute_width_of" || method == "kern" || method == "unscaled_width_of") &&
		(methodObj.Visibility == "" || methodObj.Visibility == "public") {
		if result, executed := vm.executeNativePrawnTextMethod(methodObj, receiver, args); executed {
			return result, true
		}
	}
	if keywordSyntax {
		if result, executed := vm.executeKeywordMethodNoSendFast(methodObj, receiver, args); executed {
			return result, true
		}
		// Keyword callers must retain the complete Ruby binder/arity/keyword
		// normalization protocol.  The call-site cache only supplies the method
		// and owner; invokeMethod remains the single semantic entry point.
		return vm.invokeMethod(receiver, method, method, args, methodObj, methodOwner, true), true
	}
	if blockArg != 0 {
		// Native iterators consume VM.currentBlock directly. This is the same
		// framed-native ABI used by the Register IR block-send cache and keeps
		// block/next/break handling in the existing native implementation.
		if framedNativeFn != nil &&
			(receiver == nil || receiver.Type != object.ValueProc || method != "call" && method != "[]" && method != "yield") {
			return callNativeMethod(framedNativeFn, receiver, args), true
		}
		// A Ruby callee is allowed here only when its fixed bytecode proof says
		// that it does not observe the caller block. The ordinary entry retains
		// the block guard; methods that yield, use block_given?, or forward it
		// deopt to invokeMethod without having run user code.
		if bytecodeFixedArity != nil && *bytecodeFixedArity {
			if result, executed := vm.executeCachedFixedArityRubyBytecode(receiver, method, args, methodObj, methodOwner, false); executed {
				return result, true
			}
			*bytecodeFixedArity = false
		}
		if cachedRubyBytecodeFrameEnabled {
			if result, executed := vm.executeCachedFixedArityRubyBytecode(receiver, method, args, methodObj, methodOwner, false); executed {
				if bytecodeFixedArity != nil {
					*bytecodeFixedArity = true
				}
				return result, true
			}
		}
		return vm.invokeMethod(receiver, method, method, args, methodObj, methodOwner, false), true
	}
	// Once the call site has proven that this fixed-arity Ruby method needs the
	// ordinary bytecode loop, skip the repeated typed/leaf/forwardable probes.
	// The entry is generation-scoped by the surrounding cache; a runtime guard
	// miss (for example an active caller block or a temporary stack limit) clears
	// it and re-enters the complete speculative ladder below.
	if bytecodeFixedArity != nil && *bytecodeFixedArity {
		var result *object.EmeraldValue
		var executed bool
		if registerIRTrustedBytecodeEntryEnabled {
			result, executed = vm.executeCachedFixedArityRubyBytecodeTrusted(receiver, method, args, methodObj, methodOwner)
		} else {
			result, executed = vm.executeCachedFixedArityRubyBytecode(receiver, method, args, methodObj, methodOwner, false)
		}
		if executed {
			return result, true
		}
		*bytecodeFixedArity = false
	}
	if nativeFn != nil {
		result := callNativeMethod(nativeFn, receiver, args)
		if result != nil {
			return result, true
		}
	}
	// The bytecode cache also retains the broader framed-native proof.  These
	// methods are public fixed ABI entries, but are not in the conservative
	// no-frame name whitelist; calling them here avoids re-entering invokeMethod
	// for every native send in an otherwise unsupported Ruby method.  Preserve
	// the PDF ABI hook before the direct call and keep Proc call/yield wrappers
	// on the complete path.
	if framedNativeFn != nil && !keywordSyntax &&
		(receiver == nil || receiver.Type != object.ValueProc || method != "call" && method != "[]" && method != "yield") {
		if result, executed := vm.executeNativePDFDispatch(methodObj, receiver, args, false); executed {
			return result, true
		}
		return callNativeMethod(framedNativeFn, receiver, args), true
	}
	// The cached call site already proves the exact public method and arity. Try
	// the typed callee before the broader boxed Register IR leaf; otherwise a
	// branch-bearing integer method is consumed by the leaf executor and never
	// reaches the compiled typed graph.
	if inlinePlan == nil && fn != nil {
		if result, executed := vm.tryExecuteTypedIntegerMethod(methodObj, fn, args); executed {
			return result, true
		}
		if result, executed := vm.tryExecuteTypedSSAFunction(methodObj, fn, receiver, args); executed {
			return result, true
		}
	}
	if leaf != nil && (fn != nil || leaf.kind == leafMethodInstanceReader || leaf.kind == leafMethodInstanceWriter) {
		if result, executed := vm.executeRegisterIRInlineLeaf(leaf, fn, receiver, args, methodObj, methodOwner); executed {
			return result, true
		}
	}
	// Branch-heavy/dynamic Ruby methods are deliberately absent from the
	// exact framed-plan cache.  Once their fixed-arity body has warmed, the
	// method-level typed tier can still execute a side-exit-safe direct plan;
	// before warmup it returns false and leaves the established bytecode path
	// unchanged.
	if inlinePlan == nil && fn != nil {
		if result, executed := vm.tryExecuteTypedHotFunction(methodObj, fn, receiver, args); executed {
			return result, true
		}
	}
	if inlinePlan != nil && fn != nil {
		if result, executed := vm.executeRegisterIR(inlinePlan, fn, receiver, args, method, methodObj, methodOwner, true); executed {
			return result, true
		}
		// The cached framed plan can be too conservative for a method whose
		// successful path ends in one guarded instance-variable write. The
		// method-level direct tier can prove that shape side-exit-safe, so give it
		// the same chance before entering the fixed-arity bytecode loop.
		if result, executed := vm.tryExecuteCachedTypedHotMethod(methodObj, receiver, args); executed {
			return result, true
		}
	}
	if fn != nil && methodObj.DispatchOwner == nil &&
		(methodObj.Visibility == "" || methodObj.Visibility == "public") {
		if directAttributeAccessorEnabled {
			if result, executed := vm.executeDirectAttributeAccessor(fn, receiver, args, methodObj); executed {
				return result, true
			}
		}
		if directAttributeCompareEnabled {
			if result, executed := vm.executeEarlyAttributeComparePlan(fn, receiver, args, methodObj); executed {
				return result, true
			}
		}
	}
	// Forwardable's generated rest/block wrapper is already recognized by a
	// strict source-shape guard.  Cached bytecode sends can enter that helper
	// directly instead of falling through invokeMethod only to rediscover the
	// same delegator; all target lookup, visibility, and keyword semantics stay
	// inside executeForwardableDelegatorFast.
	if forwardableFastEnabled && fn != nil && methodObj.DispatchOwner == nil &&
		(methodObj.Visibility == "" || methodObj.Visibility == "public") {
		if result, executed := vm.executeForwardableDelegatorFast(fn, receiver, args, keywordSyntax); executed {
			return result, true
		}
	}
	// Some Ruby methods are intentionally outside the current Register IR
	// admission set (branches, dynamic sends, or unsupported opcodes).  They
	// still have a cheap, exact-arity bytecode entry: the cache has already
	// proved the method and owner, so avoid repeating invokeMethod's generic
	// lookup/binder probes before entering the normal bytecode loop.
	if cachedRubyBytecodeFrameEnabled {
		if result, executed := vm.executeCachedFixedArityRubyBytecode(receiver, method, args, methodObj, methodOwner, false); executed {
			if bytecodeFixedArity != nil {
				*bytecodeFixedArity = true
			}
			return result, true
		}
	}
	return vm.invokeMethod(receiver, method, method, args, methodObj, methodOwner, false), true
}

func keywordMethodNoSendPlanSafe(plan *registerIRPlan) bool {
	if plan == nil || plan.blockReturn || plan.hasBranches || plan.hasImplicitSends {
		return false
	}
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam, registerIRLoadLocal, registerIRLoadLiteral,
			registerIRLoadInstanceVar, registerIRLoadSelf, registerIRMove,
			registerIRSwap, registerIRBang, registerIRStoreLocal, registerIRBinary, registerIRReturn:
		case registerIRSend:
			return false
		default:
			return false
		}
	}
	return len(plan.instructions) > 0
}

func (vm *VM) cachedKeywordMethodNoSendPlan(fn *object.Function) (*registerIRPlan, bool) {
	if vm == nil || fn == nil {
		return nil, false
	}
	if plan, found := vm.keywordMethodCache[fn]; found {
		return plan, plan != nil
	}
	plan, ok := compileRegisterIR(fn)
	if !ok || !keywordMethodNoSendPlanSafe(plan) {
		vm.keywordMethodCache[fn] = nil
		return nil, false
	}
	vm.keywordMethodCache[fn] = plan
	return plan, true
}

// executeKeywordMethodNoSendFast handles the narrowest keyword callee: fixed
// positional/keyword parameters, no defaults/rest/block, and a body made only
// of local/ivar reads, checked primitive arithmetic, moves, stores, and a
// return. The caller has already passed the normal bytecode call-site cache
// guards; this function only runs after validating the exact keyword hash
// shape, so a miss can fall back to invokeMethod without replaying user-visible
// work.
func (vm *VM) executeKeywordMethodNoSendFast(methodObj *object.Method, receiver *object.EmeraldValue, args []*object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || methodObj == nil || receiver == nil || methodObj.DispatchOwner != nil ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") || methodUsesRefinements(methodObj) {
		return nil, false
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || fn == nil || len(fn.KeywordParams) == 0 || fn.HasRestParam || fn.HasBlockParam ||
		fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectKeywords || !simpleBlockParameterPatterns(fn) ||
		len(args) != len(fn.Params)+1 {
		return nil, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	for _, keyword := range fn.KeywordParams {
		if keyword.HasDefault || keyword.Name == "" {
			return nil, false
		}
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash || !core.Ruby2KeywordHash(last) {
		return nil, false
	}
	kwargs := executorHashToMap(last)
	if len(kwargs) != len(fn.KeywordParams) {
		return nil, false
	}
	keywordValues := make([]*object.EmeraldValue, len(fn.KeywordParams))
	for index, keyword := range fn.KeywordParams {
		value := keywordValueFromMap(kwargs, keyword.Name)
		if value == nil {
			return nil, false
		}
		keywordValues[index] = value
	}
	for key := range kwargs {
		if !keywordParamExists(fn, key) {
			return nil, false
		}
	}
	plan, found := vm.cachedKeywordMethodNoSendPlan(fn)
	if !found {
		return nil, false
	}
	var locals [64]*object.EmeraldValue
	for index, local := range fn.ParamLocalIndices {
		if index >= len(fn.Params) || local < 0 || local >= len(locals) || index >= len(args)-1 {
			return nil, false
		}
		locals[local] = args[index]
	}
	for index, keyword := range fn.KeywordParams {
		local, ok := fn.LocalNames[keyword.Name]
		if !ok || local < 0 || local >= len(locals) {
			return nil, false
		}
		locals[local] = keywordValues[index]
	}
	var registers [16]*object.EmeraldValue
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam:
			if int(instruction.param) >= len(args)-1 {
				return nil, false
			}
			registers[instruction.dst] = args[instruction.param]
		case registerIRLoadLocal:
			local := int(instruction.param)
			if local < 0 || local >= len(locals) {
				return nil, false
			}
			value := locals[local]
			if value == nil {
				value = core.R.NilVal
			}
			registers[instruction.dst] = value
		case registerIRLoadLiteral:
			registers[instruction.dst] = instruction.value
		case registerIRLoadInstanceVar:
			value := core.DynamicInstanceVar(receiver, instruction.name)
			if value == nil {
				value = core.R.NilVal
			}
			registers[instruction.dst] = value
		case registerIRLoadSelf:
			registers[instruction.dst] = receiver
		case registerIRMove:
			registers[instruction.dst] = registers[instruction.left]
		case registerIRSwap:
			registers[instruction.left], registers[instruction.right] = registers[instruction.right], registers[instruction.left]
		case registerIRBang:
			registers[instruction.dst] = registerIRBangValue(registers[instruction.left])
		case registerIRBinary:
			value, ok := vm.executeRegisterIRNoFrameBinary(instruction, &registers)
			if !ok {
				return nil, false
			}
			registers[instruction.dst] = value
		case registerIRStoreLocal:
			local := int(instruction.param)
			if local < 0 || local >= len(locals) {
				return nil, false
			}
			value := registers[instruction.left]
			if value == nil {
				value = core.R.NilVal
			}
			locals[local] = value
		case registerIRReturn:
			value := registers[instruction.left]
			if value == nil {
				value = core.R.NilVal
			}
			return value, true
		default:
			return nil, false
		}
	}
	return nil, false
}

func (vm *VM) bytecodeSendCacheAt(frame *Frame, ip int) *registerIRSendCache {
	if vm == nil || frame == nil || frame.Fn == nil || ip < 0 || ip >= len(frame.Fn.Instructions) {
		return nil
	}
	table := frame.bytecodeSendCacheTable
	if table == nil {
		if vm.bytecodeSendTables == nil {
			vm.bytecodeSendTables = make(map[*object.Function][]*registerIRSendCache)
		}
		table = vm.bytecodeSendTables[frame.Fn]
		if table == nil {
			table = make([]*registerIRSendCache, len(frame.Fn.Instructions))
			vm.bytecodeSendTables[frame.Fn] = table
		}
		frame.bytecodeSendCacheTable = table
	}
	if table[ip] == nil {
		table[ip] = &registerIRSendCache{}
	}
	return table[ip]
}

// registerIRBytecodeInlinePlan returns a framed Ruby plan for an exact
// positional call-site shape.  Unlike registerIRInlineableLeaf this does not
// require a no-frame proof: the caller still creates the normal Ruby Frame via
// executeRegisterIR, but skips invokeMethod's second dispatch/plan lookup.
// Optional/rest/keyword/pattern/block methods stay on the compatibility path
// because their argument protocol is observable before the body executes.
func (vm *VM) registerIRBytecodeInlinePlan(methodObj *object.Method, argc uint8) (*registerIRPlan, *object.Function) {
	if vm == nil || methodObj == nil || methodObj.DispatchOwner != nil ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") || methodObj.Ruby2Keywords || methodUsesRefinements(methodObj) {
		return nil, nil
	}
	fn, ok := methodObj.Fn.(*object.Function)
	if !ok || len(fn.Params) != int(argc) || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectBlock {
		return nil, nil
	}
	if registerIRFunctionNeedsDefaultEvaluation(fn, int(argc)) {
		return nil, nil
	}
	for _, pattern := range fn.ParamPatterns {
		if pattern != nil {
			return nil, nil
		}
	}
	// Framed Register IR is still semantically safe for an exact fixed-arity
	// method even when the body is too complex for a no-frame leaf (branches,
	// allocations, dynamic sends, and stores all retain the normal Frame and
	// unwind state).  Limiting this cache to leafMethodRegisterIR left most Gem
	// methods on invokeMethod, paying the binder/visibility/plan probes on every
	// bytecode send despite already having an exact call-site guard.
	plan, compiled := compileRegisterIR(fn)
	if !compiled || plan == nil || plan.blockReturn {
		return nil, nil
	}
	// A branch-bearing plan with dynamic sends still executes the same boxed
	// dispatch graph inside a newly managed Frame, but now also pays Register IR
	// branch/guard bookkeeping.  Measurements on Prawn show this shape slower
	// than the bytecode send cache. Keep framed inlining for straight-line
	// methods (and typed/integer plans) until a branch-aware typed executor can
	// remove that overhead; the ordinary cache remains the semantic fallback.
	if plan.hasBranches && plan.hasSends && !plan.integerOnly && !registerIRBranchFrameInlineAllowed(fn) {
		return nil, nil
	}
	return plan, fn
}

func (vm *VM) populateBytecodeSendCache(cache *registerIRSendCache, generation uint64, receiver *object.EmeraldValue, method string, argc uint8) {
	if vm == nil || cache == nil || receiver == nil || receiver.Class == nil ||
		!registerIRCacheableSendName(method) ||
		(!registerIRCacheableReceiver(receiver) && !vm.registerIRCacheableReceiverForMethod(receiver, method)) {
		return
	}
	methodObj, owner, ok := vm.cachedPlainMethod(receiver, method)
	if !ok && registerIRCacheableClassReceiver(receiver) {
		// Class/Module values have per-object singleton method tables and are
		// intentionally excluded from the class-keyed method cache. The
		// bytecode call site itself is identity-aware, however, so perform one
		// complete lookup while warming that site and retain the exact receiver
		// guard in the resulting entry.
		var fallback *object.EmeraldValue
		methodObj, owner, fallback = vm.lookupMethodForSend(receiver, method, nil, false, true)
		ok = fallback == nil && methodObj != nil
	}
	if !ok {
		return
	}
	methodObj, owner, ok = resolveVisibilityAliasMethod(method, methodObj, owner)
	if !ok || methodObj == nil || methodObj.DispatchOwner != nil ||
		(methodObj.Visibility != "" && methodObj.Visibility != "public") || methodUsesRefinements(methodObj) {
		return
	}
	if cache.generation != generation {
		*cache = registerIRSendCache{generation: generation}
	}
	primarySlot := cache.class == nil
	if !primarySlot && cache.class == receiver.Class {
		primarySlot = !registerIRCacheableClassReceiver(receiver) || cache.receiver == receiver
	}
	if primarySlot || !registerIRPolymorphicSendCacheEnabled {
		cache.class = receiver.Class
		cache.receiver = receiver
		cache.method = methodObj
		cache.owner = owner
		cache.bytecodeFixedArity = false
		if registerIRInlineEnabled {
			cache.inlineLeaf, cache.inlineFn = vm.registerIRInlineableLeaf(methodObj, argc, false)
		}
		if registerIRFramedSendInlineEnabled {
			var framedFn *object.Function
			cache.inlinePlan, framedFn = vm.registerIRBytecodeInlinePlan(methodObj, argc)
			if cache.inlineFn == nil {
				cache.inlineFn = framedFn
			}
		}
		cache.nativeFn = registerIRDirectNativeFn(methodObj, method)
		cache.framedNativeFn = registerIRFramedNativeFn(methodObj, method)
		cache.directIndex = registerIRDirectIndexEligible(receiver, method, argc, methodObj, owner)
		return
	}
	secondSlot := cache.secondClass == nil
	if !secondSlot && cache.secondClass == receiver.Class {
		secondSlot = registerIRCacheableClassReceiver(receiver) && cache.secondReceiver == receiver
	}
	if secondSlot {
		cache.secondClass = receiver.Class
		cache.secondReceiver = receiver
		cache.secondMethod = methodObj
		cache.secondOwner = owner
		cache.secondBytecodeFixedArity = false
		if registerIRInlineEnabled {
			cache.secondLeaf, cache.secondFn = vm.registerIRInlineableLeaf(methodObj, argc, false)
		}
		if registerIRFramedSendInlineEnabled {
			var framedFn *object.Function
			cache.secondInlinePlan, framedFn = vm.registerIRBytecodeInlinePlan(methodObj, argc)
			if cache.secondFn == nil {
				cache.secondFn = framedFn
			}
		}
		cache.secondNativeFn = registerIRDirectNativeFn(methodObj, method)
		cache.secondFramedNativeFn = registerIRFramedNativeFn(methodObj, method)
		cache.secondDirectIndex = registerIRDirectIndexEligible(receiver, method, argc, methodObj, owner)
	}
}
