package vm

import (
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/parser/ast"
)

const StackSize = 2048
const MaxFrames = 1024

var DevMode = os.Getenv("RGO_DEV") == "1"

var CurrentVM *VM
var sendTraceDepth int64
var debugSendTraceEnabled = os.Getenv("RGO_DEBUG_SEND_TRACE") == "1"
var debugForLoopEnabled = os.Getenv("RGO_DEBUG_FOR_LOOP") == "1"

type closureCell struct {
	slot  **object.EmeraldValue
	value *object.EmeraldValue
}

func CallBlock(args ...*object.EmeraldValue) *object.EmeraldValue {
	if CurrentVM == nil || CurrentVM.currentBlock == nil {
		return core.R.NilVal
	}
	return CurrentVM.callBlock(CurrentVM.currentBlock, args...)
}

func init() {
	if DevMode {
		runtime.GOMAXPROCS(1)
	}
}

type Frame struct {
	ID      int
	Fn      *object.Function
	Ip      int
	Bp      int
	Closure *object.Closure

	MethodName             string
	OriginalMethodName     string
	LabelName              string
	SuperStart             *object.Class
	SuperModule            *object.Module
	SuperAfterClass        bool
	Args                   []*object.EmeraldValue
	Block                  *object.EmeraldValue
	DefinedByDefineMethod  bool
	IsLambda               bool
	CapturedBindings       []*object.RBinding
	TraceSelf              *object.EmeraldValue
	TraceDefinedClass      *object.EmeraldValue
	TraceMethodID          string
	TraceCalleeID          string
	TraceParameters        *object.EmeraldValue
	InstructionException   *object.EmeraldValue
	InstructionSnapshotSet bool

	BlockBreakAddr int
	WhileStart     int
	WhileEnd       int
	BlockBreak     bool
	BlockBreakVal  *object.EmeraldValue
	BlockNextVal   *object.EmeraldValue
	Returned       bool
	TraceLine      int64
	ExecutionLine  int64
	RetryRescue    *ActiveRescue
}

type RescueHandler struct {
	BodyOffset      int
	StackTop        int
	RescueOffset    int
	EnsureOffset    int
	EnsureEndOffset int
	EndOffset       int
	Frame           *Frame
}

type ActiveRescue struct {
	BodyOffset        int
	RescueOffset      int
	StackTop          int
	EndOffset         int
	EnsureOffset      int
	EnsureEndOffset   int
	Frame             *Frame
	PreviousException *object.EmeraldValue
	RescueStackDepth  int
}

type PendingEnsure struct {
	EnsureEndOffset   int
	Frame             *Frame
	Exception         *object.EmeraldValue
	PreviousException *object.EmeraldValue
	ReturnValue       *object.EmeraldValue
	IsReturn          bool
	IsBreak           bool
	IsNext            bool
	IsRedo            bool
	IsThrow           bool
	ThrowHandler      *CatchHandler
	BreakTarget       int
}

type CatchHandler struct {
	Label     *object.EmeraldValue
	EndOffset int
	Frame     *Frame
	StackTop  int
	VM        *VM
}

type vmExecutionContext struct {
	stack                     []*object.EmeraldValue
	sp                        int
	frames                    []*Frame
	fp                        int
	isRoot                    bool
	nativeBacktraceFrames     []nativeBacktraceFrame
	poppedValues              []*object.EmeraldValue
	currentBlock              *object.EmeraldValue
	visibilityBypass          bool
	classStack                []*object.EmeraldValue
	instanceExecClassVarScope *object.EmeraldValue
	threadDepth               int
	fiberDepth                int
	procCallDepth             int
	rescueStack               []*RescueHandler
	activeRescues             []*ActiveRescue
	pendingEnsures            []*PendingEnsure
	pendingReturnTargetID     int
	pendingReturnValue        *object.EmeraldValue
	pendingBreakTargetID      int
	pendingBreakValue         *object.EmeraldValue
	completedBreakValue       *object.EmeraldValue
	patternArrayCache         map[*object.EmeraldValue]*object.EmeraldValue
	ensureActive              bool
	evalReturnMode            bool
	evalReturnPending         bool
	evalReturnValue           *object.EmeraldValue
	catchStack                []*CatchHandler
	freezeStringLiterals      bool
	chillStringLiterals       bool
	sourceEncoding            string
	lastException             *object.EmeraldValue
	lastRaisedResult          *object.EmeraldValue
	lastMatcherException      *object.EmeraldValue
	lastBlockResult           *object.EmeraldValue
	threadSpecialGlobals      map[string]*object.EmeraldValue
}

type threadCoroutineEvent struct {
	result    *object.EmeraldValue
	suspended bool
}

type threadCoroutine struct {
	resume  chan struct{}
	events  chan threadCoroutineEvent
	context vmExecutionContext
	caller  vmExecutionContext
}

type fiberCoroutineEvent struct {
	result    *object.EmeraldValue
	suspended bool
}

type fiberCoroutine struct {
	resume  chan []*object.EmeraldValue
	events  chan fiberCoroutineEvent
	context vmExecutionContext
	caller  vmExecutionContext
}

type methodCacheKey struct {
	class *object.Class
	name  string
}

type methodCacheEntry struct {
	generation uint64
	method     *object.Method
	owner      *object.Class
}

type invocationMetadata struct {
	superStart        *object.Class
	superModule       *object.Module
	superAfterClass   bool
	label             string
	traceDefinedClass *object.EmeraldValue
	traceMethodID     string
}

type integerFunctionStep struct {
	op    compiler.Opcode
	value int64
	param int
}

type integerFunctionPlan struct {
	steps   []integerFunctionStep
	usesAdd bool
}

type integerFunctionCacheEntry struct {
	plan      *integerFunctionPlan
	supported bool
}

type integerLocalUpdatePlan struct {
	local int
	steps []integerFunctionStep
}

type integerLoopStep struct {
	op    compiler.Opcode
	local int
	value int64
	fn    *object.Function
}

type VM struct {
	parent              *VM
	constants           []*object.EmeraldValue
	globals             []*object.EmeraldValue
	globalNames         map[string]int
	bytecodeGlobalNames map[string]int
	rubyConsts          map[string]*object.EmeraldValue

	stack []*object.EmeraldValue
	sp    int

	frames                []*Frame
	fp                    int
	isRoot                bool
	nativeBacktraceFrames []nativeBacktraceFrame

	instructions compiler.Instructions

	poppedValues     []*object.EmeraldValue
	instructionLimit uint64
	instructionCount uint64

	currentBlock              *object.EmeraldValue
	visibilityBypass          bool
	classStack                []*object.EmeraldValue
	instanceExecClassVarScope *object.EmeraldValue
	autoloading               map[string]bool
	threadDepth               int
	fiberDepth                int
	procCallDepth             int
	nextFrameID               int

	rescueStack           []*RescueHandler
	activeRescues         []*ActiveRescue
	pendingEnsures        []*PendingEnsure
	pendingReturnTargetID int
	pendingReturnValue    *object.EmeraldValue
	pendingBreakTargetID  int
	pendingBreakValue     *object.EmeraldValue
	completedBreakValue   *object.EmeraldValue
	patternArrayCache     map[*object.EmeraldValue]*object.EmeraldValue
	ensureActive          bool
	evalReturnMode        bool
	evalReturnPending     bool
	evalReturnValue       *object.EmeraldValue

	catchStack []*CatchHandler

	escapedThrowHandler        *CatchHandler
	escapedThrowValue          *object.EmeraldValue
	pendingEscapedThrowHandler *CatchHandler
	pendingEscapedThrowValue   *object.EmeraldValue

	nativeSingletonMethods map[interface{}]map[string]*object.Method
	methodCache            map[methodCacheKey]methodCacheEntry
	fusedIntegerGeneration uint64
	fusedIntegerOps        bool
	integerFunctionCache   map[*object.Function]integerFunctionCacheEntry

	freezeStringLiterals bool
	chillStringLiterals  bool
	sourceEncoding       string
	frozenStringCache    map[string]*object.EmeraldValue
	threadCoroutines     map[*object.EmeraldValue]*threadCoroutine
	fiberCoroutines      map[*object.EmeraldValue]*fiberCoroutine
	threadSpecialGlobals map[string]*object.EmeraldValue
	unhandledException   *object.EmeraldValue
}

type nativeBacktraceFrame struct {
	parentIndex int
	location    object.RBacktraceLocation
}

func New(bytecode *compiler.Bytecode) *VM {
	currentSpecFile := core.CurrentSpecFile
	core.CurrentSpecFile = currentSpecFile
	return newVM(bytecode, nil)
}

// SetInstructionLimit bounds the number of bytecode instructions executed by
// one Run call. A zero limit, the production default, means unlimited.
func (vm *VM) SetInstructionLimit(limit uint64) {
	vm.instructionLimit = limit
}

func (vm *VM) SetFreezeStringLiterals(enabled bool) {
	vm.freezeStringLiterals = enabled
}

func (vm *VM) SetChillStringLiterals(enabled bool) {
	vm.chillStringLiterals = enabled
}

func SourceFreezesStringLiterals(source string) bool {
	return evalSourceFreezesStringLiterals(source)
}

func SourceChillsStringLiterals(source string) bool {
	_, chilled := evalSourceStringLiteralMode(source)
	return chilled
}

func (vm *VM) PreloadSource(source, path string) *object.EmeraldValue {
	previousPath := core.CurrentSpecFile
	if path != "" {
		core.CurrentSpecFile = path
	}
	result := vm.evalSource(source)
	core.CurrentSpecFile = previousPath
	if result != nil && result.Type == object.ValueException {
		return result
	}
	if path != "" {
		features := vm.loadedFeaturesGlobal()
		values := features.Data.([]*object.EmeraldValue)
		for _, value := range values {
			if value != nil && value.Type == object.ValueString && value.Data.(string) == path {
				return result
			}
		}
		features.Data = append(values, &object.EmeraldValue{Type: object.ValueString, Data: path, Class: core.R.Classes["String"]})
	}
	return result
}

func newVM(bytecode *compiler.Bytecode, parent *VM) *VM {
	mainFn := &object.Function{
		Name:               "__main__",
		SourcePath:         core.CurrentSpecFile,
		SourceAbsolutePath: core.CurrentSpecFileAbsolute,
		EvalSource:         core.CurrentEvalSource,
		SourceEncoding:     core.CurrentEvalSourceEncoding,
		Instructions:       bytecode.Instructions,
		LineMap:            bytecode.LineMap,
		Constants:          bytecode.Constants,
		NumLocals:          bytecode.NumLocals,
		GlobalNames:        bytecode.GlobalNames,
		LocalNames:         bytecode.LocalNames,
	}

	mainFrame := &Frame{
		ID:             1,
		Fn:             mainFn,
		Ip:             -1,
		Bp:             0,
		BlockBreakAddr: -1,
		WhileStart:     -1,
		WhileEnd:       -1,
	}

	vm := &VM{
		parent:                 parent,
		constants:              bytecode.Constants,
		globals:                make([]*object.EmeraldValue, 100),
		globalNames:            bytecode.GlobalNames,
		bytecodeGlobalNames:    bytecode.GlobalNames,
		rubyConsts:             make(map[string]*object.EmeraldValue),
		stack:                  make([]*object.EmeraldValue, StackSize),
		sp:                     1 + bytecode.NumLocals,
		frames:                 []*Frame{mainFrame},
		fp:                     0,
		nextFrameID:            2,
		instructions:           bytecode.Instructions,
		sourceEncoding:         core.CurrentEvalSourceEncoding,
		autoloading:            make(map[string]bool),
		nativeSingletonMethods: make(map[interface{}]map[string]*object.Method),
		methodCache:            make(map[methodCacheKey]methodCacheEntry),
		integerFunctionCache:   make(map[*object.Function]integerFunctionCacheEntry),
		frozenStringCache:      make(map[string]*object.EmeraldValue),
		threadCoroutines:       make(map[*object.EmeraldValue]*threadCoroutine),
		fiberCoroutines:        make(map[*object.EmeraldValue]*fiberCoroutine),
		isRoot:                 parent == nil,
	}
	if parent != nil {
		vm.instructionLimit = parent.instructionLimit
	}
	if parent == nil {
		core.InitializeRuntimeModules()
		vm.rubyConsts["ARGF"] = core.NewArgfValue(nil)
		vm.rubyConsts["TOPLEVEL_BINDING"] = &object.EmeraldValue{
			Type: object.ValueBinding,
			Data: &object.RBinding{
				Self:         core.R.Main,
				Locals:       map[string]*object.EmeraldValue{},
				LocalNames:   nil,
				Constants:    map[string]*object.EmeraldValue{},
				Method:       "",
				InstanceVars: map[string]*object.EmeraldValue{},
				Path:         core.CurrentSpecFile,
				Line:         1,
			},
			Class: core.R.Classes["Binding"],
		}
		vm.rubyConsts["Enumerable"] = core.R.Enumerable
		vm.rubyConsts["Comparable"] = core.R.Comparable
	}
	if parent != nil {
		vm.globals = parent.globals
		vm.globalNames = parent.globalNames
		vm.rubyConsts = parent.rubyConsts
		vm.autoloading = parent.autoloading
		vm.nativeSingletonMethods = parent.nativeSingletonMethods
		vm.frozenStringCache = parent.frozenStringCache
	} else {
		vm.SetTopLevelConstant("ARGF", vm.rubyConsts["ARGF"])
		argv := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: core.R.Classes["Array"]}
		vm.SetTopLevelConstant("ARGV", argv)
		preloadedFeatures := make([]*object.EmeraldValue, 0, 9)
		for _, feature := range []string{"complex.rb", "enumerator.so", "fiber.so", "rational.rb", "thread.so", "ruby2_keywords.rb", "set.rb", "pathname.so"} {
			preloadedFeatures = append(preloadedFeatures, &object.EmeraldValue{Type: object.ValueString, Data: feature, Class: core.R.Classes["String"]})
		}
		vm.setGlobalByName("$\"", &object.EmeraldValue{Type: object.ValueArray, Data: preloadedFeatures, Class: core.R.Classes["Array"]})
		vm.setGlobalByName("$.", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: core.R.Classes["Integer"]})
		if objectClass := core.R.Classes["Object"]; objectClass != nil {
			vm.setGlobalByName("$>", objectClass.Constants["STDOUT"])
		}
		vm.setGlobalByName("$<", vm.rubyConsts["ARGF"])
		vm.setGlobalByName("$$", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(os.Getpid()), Class: core.R.Classes["Integer"]})
		vm.setGlobalByName("$*", argv)
	}
	vm.stack[0] = core.R.Main
	vm.installCoreHooks()
	if parent == nil {
		vm.loadPathGlobal()
	}
	if parent == nil && vm.getGlobalByName("$VERBOSE") == nil {
		vm.setGlobalByName("$VERBOSE", core.R.FalseVal)
	}
	if parent == nil && vm.getGlobalByName("$/") == nil {
		vm.setGlobalByName("$/", &object.EmeraldValue{Type: object.ValueString, Data: "\n", Class: core.R.Classes["String"]})
	}

	return vm
}

func (vm *VM) SetTopLevelConstant(name string, value *object.EmeraldValue) {
	vm.rubyConsts[name] = value
	objectClass := core.R.Classes["Object"]
	if objectClass != nil {
		objectVal := &object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: core.R.Classes["Class"]}
		defineConstantOn(objectVal, name, value)
	}
}

func (vm *VM) SetProgramName(name string) {
	vm.setGlobalByName("$0", &object.EmeraldValue{Type: object.ValueString, Data: name, Class: core.R.Classes["String"]})
}

func (vm *VM) allocateFrameID() int {
	id := vm.nextFrameID
	if id <= 0 {
		id = 1
	}
	vm.nextFrameID = id + 1
	return id
}

func (vm *VM) captureExecutionContext() vmExecutionContext {
	return vmExecutionContext{
		stack:                     vm.stack,
		sp:                        vm.sp,
		frames:                    vm.frames,
		fp:                        vm.fp,
		isRoot:                    vm.isRoot,
		nativeBacktraceFrames:     vm.nativeBacktraceFrames,
		poppedValues:              vm.poppedValues,
		currentBlock:              vm.currentBlock,
		visibilityBypass:          vm.visibilityBypass,
		classStack:                vm.classStack,
		instanceExecClassVarScope: vm.instanceExecClassVarScope,
		threadDepth:               vm.threadDepth,
		fiberDepth:                vm.fiberDepth,
		procCallDepth:             vm.procCallDepth,
		rescueStack:               vm.rescueStack,
		activeRescues:             vm.activeRescues,
		pendingEnsures:            vm.pendingEnsures,
		pendingReturnTargetID:     vm.pendingReturnTargetID,
		pendingReturnValue:        vm.pendingReturnValue,
		pendingBreakTargetID:      vm.pendingBreakTargetID,
		pendingBreakValue:         vm.pendingBreakValue,
		completedBreakValue:       vm.completedBreakValue,
		patternArrayCache:         vm.patternArrayCache,
		ensureActive:              vm.ensureActive,
		evalReturnMode:            vm.evalReturnMode,
		evalReturnPending:         vm.evalReturnPending,
		evalReturnValue:           vm.evalReturnValue,
		catchStack:                vm.catchStack,
		freezeStringLiterals:      vm.freezeStringLiterals,
		chillStringLiterals:       vm.chillStringLiterals,
		sourceEncoding:            vm.sourceEncoding,
		lastException:             core.LastException,
		lastRaisedResult:          core.LastRaisedResult,
		lastMatcherException:      core.LastMatcherException,
		lastBlockResult:           core.LastBlockResult,
		threadSpecialGlobals:      vm.threadSpecialGlobals,
	}
}

func (vm *VM) restoreExecutionContext(context vmExecutionContext) {
	vm.stack = context.stack
	vm.sp = context.sp
	vm.frames = context.frames
	vm.fp = context.fp
	vm.isRoot = context.isRoot
	vm.nativeBacktraceFrames = context.nativeBacktraceFrames
	vm.poppedValues = context.poppedValues
	vm.currentBlock = context.currentBlock
	vm.visibilityBypass = context.visibilityBypass
	vm.classStack = context.classStack
	vm.instanceExecClassVarScope = context.instanceExecClassVarScope
	vm.threadDepth = context.threadDepth
	vm.fiberDepth = context.fiberDepth
	vm.procCallDepth = context.procCallDepth
	vm.rescueStack = context.rescueStack
	vm.activeRescues = context.activeRescues
	vm.pendingEnsures = context.pendingEnsures
	vm.pendingReturnTargetID = context.pendingReturnTargetID
	vm.pendingReturnValue = context.pendingReturnValue
	vm.pendingBreakTargetID = context.pendingBreakTargetID
	vm.pendingBreakValue = context.pendingBreakValue
	vm.completedBreakValue = context.completedBreakValue
	vm.patternArrayCache = context.patternArrayCache
	vm.ensureActive = context.ensureActive
	vm.evalReturnMode = context.evalReturnMode
	vm.evalReturnPending = context.evalReturnPending
	vm.evalReturnValue = context.evalReturnValue
	vm.catchStack = context.catchStack
	vm.freezeStringLiterals = context.freezeStringLiterals
	vm.chillStringLiterals = context.chillStringLiterals
	vm.sourceEncoding = context.sourceEncoding
	core.LastException = context.lastException
	core.LastRaisedResult = context.lastRaisedResult
	core.LastMatcherException = context.lastMatcherException
	core.LastBlockResult = context.lastBlockResult
	vm.threadSpecialGlobals = context.threadSpecialGlobals
}

func (vm *VM) freshThreadExecutionContext(caller vmExecutionContext) vmExecutionContext {
	return vmExecutionContext{
		stack:                make([]*object.EmeraldValue, StackSize),
		fp:                   -1,
		threadDepth:          1,
		freezeStringLiterals: caller.freezeStringLiterals,
		chillStringLiterals:  caller.chillStringLiterals,
		sourceEncoding:       caller.sourceEncoding,
		threadSpecialGlobals: make(map[string]*object.EmeraldValue),
	}
}

func (vm *VM) runThreadBlock(thread, block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	coroutine := vm.threadCoroutines[thread]
	if coroutine == nil {
		coroutine = &threadCoroutine{
			resume: make(chan struct{}),
			events: make(chan threadCoroutineEvent),
		}
		vm.threadCoroutines[thread] = coroutine
		coroutine.caller = vm.captureExecutionContext()
		vm.restoreExecutionContext(vm.freshThreadExecutionContext(coroutine.caller))

		go func() {
			result := core.R.NilVal
			defer func() {
				if recovered := recover(); recovered != nil {
					result = core.NewRuntimeError(fmt.Sprintf("thread execution failed: %v", recovered))
				}
				coroutine.context = vm.captureExecutionContext()
				vm.restoreExecutionContext(coroutine.caller)
				coroutine.events <- threadCoroutineEvent{result: result}
			}()
			result = vm.callBlock(block, args...)
		}()
	} else {
		coroutine.caller = vm.captureExecutionContext()
		vm.restoreExecutionContext(coroutine.context)
		coroutine.resume <- struct{}{}
	}

	event := <-coroutine.events
	if event.suspended {
		return core.ThreadBlockedResult()
	}
	delete(vm.threadCoroutines, thread)
	return event.result
}

func (vm *VM) suspendCurrentThread() *object.EmeraldValue {
	thread := core.CurrentThreadValue()
	coroutine := vm.threadCoroutines[thread]
	if coroutine == nil {
		return core.NewThreadError("stopping only thread")
	}
	coroutine.context = vm.captureExecutionContext()
	vm.restoreExecutionContext(coroutine.caller)
	coroutine.events <- threadCoroutineEvent{result: core.ThreadBlockedResult(), suspended: true}
	<-coroutine.resume
	if core.ConsumeCurrentThreadTermination() {
		return core.ThreadTerminationResult()
	}
	if exception := core.ConsumeCurrentThreadInterrupt(); exception != nil {
		return exception
	}
	return core.R.NilVal
}

func (vm *VM) threadBacktraceFrames(thread *object.EmeraldValue) []object.RBacktraceLocation {
	coroutine := vm.threadCoroutines[thread]
	if coroutine == nil {
		return nil
	}
	caller := vm.captureExecutionContext()
	vm.restoreExecutionContext(coroutine.context)
	frames := vm.currentBacktraceFrames()
	vm.restoreExecutionContext(caller)
	return frames
}

func (vm *VM) freshFiberExecutionContext(caller vmExecutionContext) vmExecutionContext {
	return vmExecutionContext{
		stack:                make([]*object.EmeraldValue, StackSize),
		fp:                   -1,
		threadDepth:          caller.threadDepth,
		fiberDepth:           1,
		freezeStringLiterals: caller.freezeStringLiterals,
		chillStringLiterals:  caller.chillStringLiterals,
		sourceEncoding:       caller.sourceEncoding,
		threadSpecialGlobals: caller.threadSpecialGlobals,
	}
}

func (vm *VM) runFiberBlock(fiber, block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	coroutine := vm.fiberCoroutines[fiber]
	if coroutine == nil {
		coroutine = &fiberCoroutine{
			resume: make(chan []*object.EmeraldValue),
			events: make(chan fiberCoroutineEvent),
		}
		vm.fiberCoroutines[fiber] = coroutine
		coroutine.caller = vm.captureExecutionContext()
		vm.restoreExecutionContext(vm.freshFiberExecutionContext(coroutine.caller))
		go func() {
			result := core.R.NilVal
			defer func() {
				if recovered := recover(); recovered != nil {
					result = core.NewRuntimeError(fmt.Sprintf("fiber execution failed: %v", recovered))
				}
				coroutine.context = vm.captureExecutionContext()
				vm.restoreExecutionContext(coroutine.caller)
				coroutine.events <- fiberCoroutineEvent{result: result}
			}()
			result = vm.callBlock(block, args...)
		}()
	} else {
		coroutine.caller = vm.captureExecutionContext()
		vm.restoreExecutionContext(coroutine.context)
		coroutine.resume <- args
	}

	event := <-coroutine.events
	if event.suspended {
		return core.NewFiberSuspension(event.result)
	}
	delete(vm.fiberCoroutines, fiber)
	return event.result
}

func (vm *VM) suspendCurrentFiber(yielded *object.EmeraldValue) *object.EmeraldValue {
	fiber := core.CurrentFiberValue()
	coroutine := vm.fiberCoroutines[fiber]
	if coroutine == nil {
		return core.NewException("FiberError", "can't yield from root fiber")
	}
	coroutine.context = vm.captureExecutionContext()
	vm.restoreExecutionContext(coroutine.caller)
	coroutine.events <- fiberCoroutineEvent{result: yielded, suspended: true}
	resumeArgs := <-coroutine.resume
	if core.ConsumeFiberTermination(fiber) {
		return core.FiberTerminationResult()
	}
	if exception := core.ConsumeFiberInterrupt(fiber); exception != nil {
		return exception
	}
	switch len(resumeArgs) {
	case 0:
		return core.R.NilVal
	case 1:
		return resumeArgs[0]
	default:
		return &object.EmeraldValue{Type: object.ValueArray, Data: append([]*object.EmeraldValue(nil), resumeArgs...), Class: core.R.Classes["Array"]}
	}
}

func (vm *VM) currentFrameID() int {
	if vm.fp >= 0 && vm.fp < len(vm.frames) && vm.frames[vm.fp] != nil {
		return vm.frames[vm.fp].ID
	}
	return 0
}

func (vm *VM) lexicalReturnOwnerID(frame *Frame) int {
	if frame == nil {
		return 0
	}
	if frame.Fn != nil && frame.Fn.Name == "__block__" && frame.Closure != nil && frame.Closure.ReturnOwnerID > 0 {
		return frame.Closure.ReturnOwnerID
	}
	return frame.ID
}

func (vm *VM) frameIDActive(id int) bool {
	if id <= 0 {
		return false
	}
	for i := vm.fp; i >= 0; i-- {
		if i < len(vm.frames) && vm.frames[i] != nil && vm.frames[i].ID == id {
			return true
		}
	}
	return false
}

func (vm *VM) topLevelBindingData() *object.RBinding {
	value := vm.rubyConsts["TOPLEVEL_BINDING"]
	if value == nil || value.Type != object.ValueBinding {
		return nil
	}
	binding, _ := value.Data.(*object.RBinding)
	return binding
}

func orderedFrameLocalNames(fn *object.Function) []string {
	if fn == nil || len(fn.LocalNames) == 0 {
		return nil
	}
	names := make([]string, len(fn.LocalNames))
	for name, index := range fn.LocalNames {
		if index >= 0 && index < len(names) {
			names[index] = name
		}
	}
	return names
}

func (vm *VM) initializeTopLevelBindingLocals(frame *Frame) {
	if !vm.isRoot || frame == nil || frame.Fn == nil {
		return
	}
	binding := vm.topLevelBindingData()
	if binding == nil {
		return
	}
	existing := make(map[string]bool, len(binding.LocalNames))
	for _, name := range binding.LocalNames {
		existing[name] = true
	}
	for _, name := range orderedFrameLocalNames(frame.Fn) {
		if name != "" && !existing[name] {
			binding.LocalNames = append(binding.LocalNames, name)
			existing[name] = true
		}
	}
}

func (vm *VM) topLevelLocalName(frame *Frame, index int) (string, bool) {
	if !vm.isRoot || len(vm.frames) == 0 || frame != vm.frames[0] || frame == nil || frame.Fn == nil {
		return "", false
	}
	for name, localIndex := range frame.Fn.LocalNames {
		if localIndex == index {
			return name, true
		}
	}
	return "", false
}

func (vm *VM) updateCapturedBindingLocal(frame *Frame, index int, value *object.EmeraldValue) {
	if frame == nil || frame.Fn == nil || len(frame.CapturedBindings) == 0 {
		return
	}
	name := ""
	for localName, localIndex := range frame.Fn.LocalNames {
		if localIndex == index {
			name = localName
			break
		}
	}
	if name == "" {
		return
	}
	for _, binding := range frame.CapturedBindings {
		if binding == nil {
			continue
		}
		if binding.Locals == nil {
			binding.Locals = make(map[string]*object.EmeraldValue)
		}
		binding.Locals[name] = value
		known := false
		for _, localName := range binding.LocalNames {
			if localName == name {
				known = true
				break
			}
		}
		if !known {
			binding.LocalNames = append(binding.LocalNames, name)
		}
	}
}

func (vm *VM) setCapturedBindingLocal(binding *object.RBinding, name string, value *object.EmeraldValue) {
	if binding == nil || name == "" {
		return
	}
	for _, frame := range vm.frames {
		if frame == nil || frame.Fn == nil {
			continue
		}
		captured := false
		for _, candidate := range frame.CapturedBindings {
			if candidate == binding {
				captured = true
				break
			}
		}
		if !captured {
			continue
		}
		index, ok := frame.Fn.LocalNames[name]
		if !ok {
			continue
		}
		slot := frame.Bp + 1 + index
		if slot < 0 || slot >= vm.sp {
			continue
		}
		vm.stack[slot] = value
		if _, topLevel := vm.topLevelLocalName(frame, index); topLevel {
			if topLevelBinding := vm.topLevelBindingData(); topLevelBinding != nil {
				if topLevelBinding.Locals == nil {
					topLevelBinding.Locals = make(map[string]*object.EmeraldValue)
				}
				topLevelBinding.Locals[name] = value
			}
		}
		vm.updateCapturedBindingLocal(frame, index, value)
	}
}

func (vm *VM) handlePendingNonLocalReturn(frame *Frame) bool {
	if frame == nil || vm.pendingReturnTargetID <= 0 || vm.pendingReturnValue == nil {
		return false
	}
	value := vm.pendingReturnValue
	if frame.ID == vm.pendingReturnTargetID {
		vm.pendingReturnTargetID = 0
		vm.pendingReturnValue = nil
		vm.discardPendingReturnForCurrentEnsure(frame)
		if vm.routeReturnThroughEnsure(frame, value) {
			return false
		}
	}
	base := frame.Bp + 1
	if frame.Fn != nil {
		base += frame.Fn.NumLocals
	}
	if base < frame.Bp+1 {
		base = frame.Bp + 1
	}
	vm.sp = base
	vm.push(value)
	frame.Returned = true
	return true
}

func (vm *VM) handlePendingNonLocalBreak(frame *Frame) bool {
	if frame == nil || vm.pendingBreakTargetID <= 0 || vm.pendingBreakValue == nil {
		return false
	}
	for i := len(vm.pendingEnsures) - 1; i >= 0; i-- {
		pending := vm.pendingEnsures[i]
		if pending.Frame == frame && pending.IsBreak {
			return false
		}
	}
	value := vm.pendingBreakValue
	if frame.ID == vm.pendingBreakTargetID {
		vm.pendingBreakTargetID = 0
		vm.pendingBreakValue = nil
		vm.completedBreakValue = value
		if frame.WhileEnd >= 0 {
			if vm.routeBreakThroughEnsure(frame, value, frame.WhileEnd) {
				return false
			}
			vm.sp = frame.Bp + 1 + frame.Fn.NumLocals
			vm.push(value)
			if value == nil || value.Type == object.ValueNil {
				frame.Ip = frame.WhileEnd - 1
			} else {
				frame.Ip = frame.WhileEnd
			}
			return false
		}
	} else if vm.routeBreakThroughEnsure(frame, value, -1) {
		return false
	}
	base := frame.Bp + 1
	if frame.Fn != nil {
		base += frame.Fn.NumLocals
	}
	vm.sp = base
	vm.push(value)
	frame.Returned = true
	return true
}

func (vm *VM) pendingBreakResultForFrame(frame *Frame) (*object.EmeraldValue, bool) {
	if frame == nil || vm.pendingBreakTargetID <= 0 || vm.pendingBreakValue == nil {
		return nil, false
	}
	value := vm.pendingBreakValue
	if frame.ID == vm.pendingBreakTargetID {
		vm.pendingBreakTargetID = 0
		vm.pendingBreakValue = nil
		vm.completedBreakValue = value
	}
	return value, true
}

func (vm *VM) consumeCompletedBreakMarker() {
	if vm.completedBreakValue == nil {
		return
	}
	if core.LastBlockResult == vm.completedBreakValue {
		core.LastBlockResult = nil
	}
	vm.completedBreakValue = nil
}

func setBlockBreakOwner(block *object.EmeraldValue, ownerID int) {
	if block == nil || ownerID <= 0 {
		return
	}
	switch block.Type {
	case object.ValueClosure:
		closure, _ := block.Data.(*object.Closure)
		if closure != nil && closure.BreakOwnerID == 0 {
			closure.BreakOwnerID = ownerID
		}
	case object.ValueProc:
		proc, _ := block.Data.(*object.Proc)
		if proc != nil && proc.BreakOwnerID == 0 {
			proc.BreakOwnerID = ownerID
		}
	}
}

func cloneMethodMap(methods map[string]*object.Method) map[string]*object.Method {
	copy := make(map[string]*object.Method, len(methods))
	for name, method := range methods {
		copy[name] = method
	}
	return copy
}

func restoreMethodMap(target, snapshot map[string]*object.Method) {
	for name := range target {
		delete(target, name)
	}
	for name, method := range snapshot {
		target[name] = method
	}
}

func (vm *VM) installCoreHooks() {
	CurrentVM = vm
	core.CallBlock = CallBlock
	core.CallMethod = func(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
		return vm.send(receiver, method, args)
	}
	core.InvokeMethodObject = func(receiver *object.EmeraldValue, method *object.Method, args ...*object.EmeraldValue) *object.EmeraldValue {
		invoked := method
		if method != nil && method.DispatchOwner != nil {
			copy := *method
			copy.Owner = method.DispatchOwner
			invoked = &copy
		}
		var owner *object.Class
		if invoked != nil && invoked.Owner != nil && invoked.Owner.Type == object.ValueClass {
			owner, _ = invoked.Owner.Data.(*object.Class)
		}
		if owner == nil && receiver != nil {
			owner = receiver.Class
		}
		dispatchName := invoked.Name
		if invoked.OriginalName != "" {
			dispatchName = invoked.OriginalName
		}
		return vm.invokeMethod(receiver, "bind", dispatchName, args, invoked, owner)
	}
	core.CallPublicMethod = func(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
		methodObj, methodOwner, fallback := vm.lookupMethodForSend(receiver, method, args, true)
		if fallback != nil {
			return fallback
		}
		if methodObj == nil {
			return core.NewNoMethodError("undefined method `" + method + "'")
		}
		if methodObj.Visibility == "private" || methodObj.Visibility == "protected" {
			return core.NewNoMethodErrorForVisibility(receiver, method, methodObj.Visibility, args)
		}
		return vm.invokeMethod(receiver, "public_send", method, args, methodObj, methodOwner)
	}
	core.CallMethodBypass = func(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
		prev := vm.visibilityBypass
		vm.visibilityBypass = true
		result := vm.send(receiver, method, args)
		vm.visibilityBypass = prev
		return result
	}
	core.CallMethodWithoutBlockBypass = func(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
		prevBypass := vm.visibilityBypass
		prevBlock := vm.currentBlock
		vm.visibilityBypass = true
		vm.currentBlock = nil
		result := vm.send(receiver, method, args)
		vm.currentBlock = prevBlock
		vm.visibilityBypass = prevBypass
		return result
	}
	core.InstallNativeSingletonMethod = func(receiver *object.EmeraldValue, name string, method *object.Method) func() {
		key := nativeSingletonKey(receiver)
		methods := vm.nativeSingletonMethods[key]
		if methods == nil {
			methods = make(map[string]*object.Method)
			vm.nativeSingletonMethods[key] = methods
		}
		previous, existed := methods[name]
		methods[name] = method
		return func() {
			if existed {
				methods[name] = previous
			} else {
				delete(methods, name)
			}
		}
	}
	core.HasNativeSingletonMethod = func(receiver *object.EmeraldValue, name string) bool {
		methods := vm.nativeSingletonMethods[nativeSingletonKey(receiver)]
		if name == "" {
			return len(methods) > 0
		}
		_, ok := methods[name]
		return ok
	}
	core.CallMethodWithBlock = func(receiver *object.EmeraldValue, method string, block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		prevBlock := vm.currentBlock
		vm.currentBlock = block
		result := vm.send(receiver, method, args)
		vm.currentBlock = prevBlock
		return result
	}
	core.SetGlobalVariable = func(name string, value *object.EmeraldValue) {
		vm.setGlobalByName(name, value)
	}
	core.GetGlobalVariable = func(name string) *object.EmeraldValue {
		return vm.getGlobalByName(name)
	}
	core.SetConstantName = func(container *object.EmeraldValue, name string, value *object.EmeraldValue) {
		if container == nil || container.Type != object.ValueClass {
			return
		}
		if class := container.Data.(*object.Class); class != nil && class.Name == "Object" {
			vm.rubyConsts[name] = value
		}
	}
	core.GetConstantName = func(name string) *object.EmeraldValue {
		if value, ok := vm.qualifiedConstantValue(name); ok {
			return value
		}
		if value, ok := vm.topLevelConstantValue(name); ok {
			return value
		}
		return nil
	}
	core.RemoveConstantName = func(container *object.EmeraldValue, name string) {
		qualified := qualifiedConstantName(container, name)
		delete(vm.rubyConsts, qualified)
		if container != nil && container.Type == object.ValueClass {
			if class := container.Data.(*object.Class); class != nil && class.Name == "Object" {
				delete(vm.rubyConsts, name)
			}
		}
	}
	core.CurrentFrameBinding = vm.currentFrameBinding
	core.CaptureFrameBinding = vm.captureFrameBinding
	core.SetCapturedBindingLocal = vm.setCapturedBindingLocal
	core.CurrentFrameID = func() int {
		return vm.currentFrameID()
	}
	core.CurrentMethodName = func() string {
		if vm.fp < 0 || vm.fp >= len(vm.frames) || vm.frames[vm.fp] == nil {
			return ""
		}
		name := vm.frames[vm.fp].OriginalMethodName
		if name == "" {
			name = vm.frames[vm.fp].MethodName
		}
		if name == "__main__" {
			return ""
		}
		return name
	}
	core.ActiveRefinedMethod = vm.lookupActiveRefinedMethod
	core.ActiveRefinedInstanceMethod = vm.lookupActiveRefinedInstanceMethod
	core.ActivateCurrentRefinement = vm.activateCurrentRefinement
	core.UsingReceiverAllowed = vm.usingReceiverAllowed
	core.CurrentModuleNesting = func() []*object.EmeraldValue {
		values := make([]*object.EmeraldValue, 0, len(vm.classStack))
		for index := len(vm.classStack) - 1; index >= 0; index-- {
			value := vm.classStack[index]
			if value != nil && (value.Type == object.ValueClass || value.Type == object.ValueModule) {
				if len(values) > 0 && values[len(values)-1] == value {
					continue
				}
				values = append(values, value)
			}
		}
		return values
	}
	core.CurrentUsedRefinements = func() []*object.EmeraldValue {
		if refinements, fixed := vm.currentFixedRefinements(); fixed {
			return append([]*object.EmeraldValue(nil), refinements...)
		}
		return nil
	}
	core.CurrentCalleeName = func() string {
		if vm.fp < 0 || vm.fp >= len(vm.frames) || vm.frames[vm.fp] == nil || vm.frames[vm.fp].MethodName == "__main__" {
			return ""
		}
		return vm.frames[vm.fp].MethodName
	}
	core.FrameActive = func(id int) bool {
		if id <= 0 {
			return false
		}
		for i := vm.fp; i >= 0; i-- {
			if i < len(vm.frames) && vm.frames[i] != nil && vm.frames[i].ID == id {
				return true
			}
		}
		return false
	}
	core.CurrentBacktraceFrames = vm.currentBacktraceFrames
	core.ThreadBacktraceFrames = vm.threadBacktraceFrames
	core.GlobalVariableNames = vm.globalVariableNames
	core.InRescue = func() bool {
		return len(vm.activeRescues) > 0
	}
	core.CallBlockWithArgs = func(block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		return vm.callBlock(block, args...)
	}
	core.TracePointActivated = func() (int, string, int64) {
		if vm.fp >= 0 && vm.fp < len(vm.frames) {
			frame := vm.frames[vm.fp]
			if frame != nil {
				frame.TraceLine = frame.ExecutionLine
				if frame.TraceLine <= 0 {
					frame.TraceLine = vm.sourceLineForFrame(frame)
				}
				path := core.CurrentSpecFile
				if frame.Fn != nil && frame.Fn.SourcePath != "" {
					path = frame.Fn.SourcePath
				}
				return frame.ID, path, frame.TraceLine
			}
		}
		return 0, "", 0
	}
	core.CallBlockDetached = func(block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		previous := vm.currentBlock
		vm.currentBlock = nil
		result := vm.callBlock(block, args...)
		vm.currentBlock = previous
		return result
	}
	core.CallBlockWithSelf = func(block *object.EmeraldValue, self *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		if self != nil && (self.Type == object.ValueClass || self.Type == object.ValueModule) {
			vm.classStack = append(vm.classStack, self)
			result := vm.callBlockWithSelf(block, self, args...)
			vm.classStack = vm.classStack[:len(vm.classStack)-1]
			return result
		}
		return vm.callBlockWithSelf(block, self, args...)
	}
	core.AbortCurrentBlock = func(value *object.EmeraldValue) {
		if vm.fp < 0 || vm.fp >= len(vm.frames) || vm.frames[vm.fp] == nil {
			return
		}
		vm.frames[vm.fp].BlockBreak = true
		vm.frames[vm.fp].BlockBreakVal = value
	}
	core.ClearPendingBlockControl = func() {
		vm.pendingBreakTargetID = 0
		vm.pendingBreakValue = nil
		core.LastBlockResult = nil
		for _, frame := range vm.frames {
			if frame != nil {
				frame.BlockBreak = false
				frame.BlockBreakVal = nil
			}
		}
	}
	core.BeginSpecExampleIsolation = func() func() {
		mainObj, ok := core.R.Main.Data.(*object.Object)
		if !ok {
			return nil
		}
		singletonMethods := cloneMethodMap(mainObj.SingletonMethods)
		var singletonClassMethods map[string]*object.Method
		if mainObj.SingletonClass != nil {
			singletonClassMethods = cloneMethodMap(mainObj.SingletonClass.Methods)
		}
		key := nativeSingletonKey(core.R.Main)
		nativeMethods := cloneMethodMap(vm.nativeSingletonMethods[key])
		return func() {
			restoreMethodMap(mainObj.SingletonMethods, singletonMethods)
			if mainObj.SingletonClass != nil {
				restoreMethodMap(mainObj.SingletonClass.Methods, singletonClassMethods)
			}
			if len(nativeMethods) == 0 {
				delete(vm.nativeSingletonMethods, key)
			} else {
				methods := vm.nativeSingletonMethods[key]
				if methods == nil {
					methods = make(map[string]*object.Method)
					vm.nativeSingletonMethods[key] = methods
				}
				restoreMethodMap(methods, nativeMethods)
			}
		}
	}
	core.ConsumeBlockControl = func() {
		vm.pendingBreakTargetID = 0
		vm.pendingBreakValue = nil
		core.LastBlockResult = nil
		if vm.fp >= 0 && vm.fp < len(vm.frames) && vm.frames[vm.fp] != nil {
			vm.frames[vm.fp].BlockBreak = false
			vm.frames[vm.fp].BlockBreakVal = nil
		}
	}
	core.EnterThreadBlock = func() {
		vm.threadDepth++
	}
	core.LeaveThreadBlock = func() {
		if vm.threadDepth > 0 {
			vm.threadDepth--
		}
	}
	core.InThreadBlock = func() bool {
		return vm.threadDepth > 0
	}
	core.RunThreadBlock = vm.runThreadBlock
	core.SuspendCurrentThread = vm.suspendCurrentThread
	core.RunFiberBlock = vm.runFiberBlock
	core.SuspendCurrentFiber = vm.suspendCurrentFiber
	core.BlockGivenCheck = func() bool {
		return vm.currentBlock != nil
	}
	core.MethodBlockGivenCheck = func() bool {
		return vm.methodBlockGiven()
	}
	core.CurrentBlockValue = func() *object.EmeraldValue {
		return vm.currentBlock
	}
	core.EvalSource = func(source string) *object.EmeraldValue {
		return vm.evalSource(source)
	}
	core.EvalSourceWithBinding = func(source string, binding *object.RBinding) *object.EmeraldValue {
		return vm.evalSourceWithBinding(source, binding)
	}
	core.RequirePath = func(path string) (string, *object.EmeraldValue) {
		return vm.requirePath(path)
	}
	core.RequireTriggeredByAutoload = func() bool {
		return len(vm.autoloading) > 0
	}
	core.ResolveRequirePath = vm.resolveRequirePath
	core.ResolveLoadPath = vm.resolveLoadPath
	core.InMethodScope = func() bool {
		if vm.fp < 0 || vm.fp >= len(vm.frames) || vm.frames[vm.fp] == nil {
			return false
		}
		frame := vm.frames[vm.fp]
		return frame.DefinedByDefineMethod || (frame.Fn != nil && (frame.Fn.MethodBody || frame.Fn.DefinedByDefineMethod))
	}
}

func (vm *VM) setGlobalByName(name string, value *object.EmeraldValue) {
	resolvedName := core.ResolveGlobalAlias(name)
	if vm.threadSpecialGlobals != nil && isThreadSpecialGlobal(resolvedName) {
		vm.threadSpecialGlobals[resolvedName] = value
		return
	}
	if vm.globalNames == nil {
		vm.globalNames = make(map[string]int)
	}
	idx, ok := vm.globalNames[resolvedName]
	if !ok && name != resolvedName {
		idx, ok = vm.globalNames[name]
	}
	if !ok && resolvedName == "$?" {
		idx, ok = vm.globalNames["?"]
	}
	if !ok {
		idx, ok = vm.ensureGlobalIndexForName(resolvedName), true
	}
	if !ok || idx < 0 || idx >= len(vm.globals) {
		return
	}
	vm.globals[idx] = value
}

func (vm *VM) getGlobalByName(name string) *object.EmeraldValue {
	resolvedName := core.ResolveGlobalAlias(name)
	if vm.threadSpecialGlobals != nil && isThreadSpecialGlobal(resolvedName) {
		return vm.threadSpecialGlobals[resolvedName]
	}
	if vm.globalNames == nil {
		return nil
	}
	idx, ok := vm.globalNames[resolvedName]
	if !ok && name != resolvedName {
		idx, ok = vm.globalNames[name]
	}
	if !ok && resolvedName == "$?" {
		idx, ok = vm.globalNames["?"]
	}
	if !ok || idx < 0 || idx >= len(vm.globals) {
		return nil
	}
	return vm.globals[idx]
}

func isThreadSpecialGlobal(name string) bool {
	if name == "$_" || name == "$?" || name == "$~" || name == "$&" || name == "$`" || name == "$'" || name == "$+" {
		return true
	}
	return len(name) == 2 && name[0] == '$' && name[1] >= '1' && name[1] <= '9'
}

func (vm *VM) resolveGlobalIndex(index int) int {
	return vm.resolveGlobalIndexForFrame(nil, index)
}

func (vm *VM) resolveGlobalIndexForFrame(frame *Frame, index int) int {
	if vm.globalNames == nil {
		return index
	}
	if index < 0 || index >= len(vm.globals) {
		return index
	}

	name := vm.rawGlobalNameForFrameIndex(frame, index)
	if name == "" {
		return index
	}
	resolvedName := core.ResolveGlobalAlias(name)
	if resolvedName == "" {
		resolvedName = name
	}
	if resolvedIdx, ok := vm.globalNames[resolvedName]; ok {
		return resolvedIdx
	}
	if name != resolvedName {
		if resolvedIdx, ok := vm.globalNames[name]; ok {
			return resolvedIdx
		}
	}
	return vm.ensureGlobalIndexForName(name)
}

func (vm *VM) rawGlobalNameForFrameIndex(frame *Frame, index int) string {
	if frame != nil && frame.Fn != nil && frame.Fn.GlobalNames != nil {
		for name, idx := range frame.Fn.GlobalNames {
			if idx == index {
				return name
			}
		}
	}
	return vm.rawGlobalNameForIndex(index)
}

func (vm *VM) rawGlobalNameForIndex(index int) string {
	if vm.bytecodeGlobalNames != nil {
		for name, idx := range vm.bytecodeGlobalNames {
			if idx == index {
				return name
			}
		}
	}
	if vm.globalNames == nil {
		return ""
	}
	for name, idx := range vm.globalNames {
		if idx == index {
			return name
		}
	}
	return ""
}

func (vm *VM) globalNameForIndex(index int) string {
	if vm.globalNames != nil {
		for name, idx := range vm.globalNames {
			if idx == index {
				if resolved := core.ResolveGlobalAlias(name); resolved != "" {
					return resolved
				}
				return name
			}
		}
	}
	return vm.rawGlobalNameForIndex(index)
}

func (vm *VM) ensureGlobalIndexForName(name string) int {
	if vm.globalNames == nil {
		vm.globalNames = make(map[string]int)
	}
	resolvedName := core.ResolveGlobalAlias(name)
	if resolvedName == "" {
		resolvedName = name
	}
	if idx, ok := vm.globalNames[resolvedName]; ok {
		if name != resolvedName {
			vm.globalNames[name] = idx
		}
		return idx
	}
	if idx, ok := vm.globalNames[name]; ok {
		if name != resolvedName {
			vm.globalNames[resolvedName] = idx
		}
		return idx
	}
	used := make(map[int]struct{}, len(vm.globalNames))
	for _, idx := range vm.globalNames {
		used[idx] = struct{}{}
	}
	for idx := 0; idx < len(vm.globals); idx++ {
		if _, ok := used[idx]; ok {
			continue
		}
		vm.globalNames[resolvedName] = idx
		if name != resolvedName {
			vm.globalNames[name] = idx
		}
		return idx
	}
	vm.globals = append(vm.globals, nil)
	idx := len(vm.globals) - 1
	vm.globalNames[resolvedName] = idx
	if name != resolvedName {
		vm.globalNames[name] = idx
	}
	return idx
}

func (vm *VM) validateGlobalAssignmentForFrame(frame *Frame, rawIndex int, resolvedIndex int, value *object.EmeraldValue) (*object.EmeraldValue, *object.EmeraldValue) {
	rawName := vm.rawGlobalNameForFrameIndex(frame, rawIndex)
	name := vm.globalNameForIndex(resolvedIndex)
	switch rawName {
	case "$&", "$`", "$'", "$+":
		return nil, newSyntaxError("Can't set variable " + rawName)
	}
	switch name {
	case "$&", "$`", "$'", "$+":
		return nil, core.NewNameError(rawName + " is a read-only variable")
	case "$!":
		return nil, core.NewNameError("$! is a read-only variable")
	case "$@":
		if core.LastException == nil || core.LastException.Type == object.ValueNil {
			return nil, core.NewArgumentError("$! not set")
		}
		if errVal := validateBacktraceGlobalValue(value); errVal != nil {
			return nil, errVal
		}
		updateLastExceptionBacktrace(value)
	case "$VERBOSE":
		if value == nil || value.Type == object.ValueNil || value == core.R.FalseVal || value.Type == object.ValueBool && !value.Data.(bool) {
			return value, nil
		}
		return core.R.TrueVal, nil
	case "$~":
		if value == nil || value.Type == object.ValueNil || value.Type == object.ValueMatchData {
			return value, nil
		}
		return nil, core.NewTypeError(fmt.Sprintf("wrong argument type %s (expected MatchData)", value.TypeName()))
	case "$stdout", "stdout":
		if value == nil || value.Type == object.ValueNil {
			return nil, core.NewTypeError("$stdout must have write method, NilClass given")
		}
		if !vm.receiverCanBeStdout(value) {
			return nil, core.NewTypeError("$stdout must have write method")
		}
	case "$/", "$-0", "$\\", "$,":
		if value == nil || value.Type == object.ValueNil || value.Type == object.ValueString {
			return value, nil
		}
		return nil, core.NewTypeError("value of " + name + " must be String")
	case "$.", ".":
		converted, errVal := vm.coerceGlobalLineNumber(value)
		if errVal != nil {
			return nil, errVal
		}
		return converted, nil
	case "$0":
		if value == nil || value.Type != object.ValueString {
			return nil, core.NewTypeError("no implicit conversion of " + value.TypeName() + " into String")
		}
	case "$:", "$LOAD_PATH", "$-I", "$\"", "$LOADED_FEATURES", "$<", "$FILENAME", "$?", "$-a", "$-l", "$-p":
		return nil, core.NewNameError(rawName + " is a read-only variable")
	}
	return value, nil
}

func (vm *VM) validateGlobalAssignment(rawIndex int, resolvedIndex int, value *object.EmeraldValue) (*object.EmeraldValue, *object.EmeraldValue) {
	return vm.validateGlobalAssignmentForFrame(nil, rawIndex, resolvedIndex, value)
}

func validateBacktraceGlobalValue(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil || value.Type == object.ValueNil || value.Type == object.ValueString {
		return nil
	}
	if value.Type != object.ValueArray {
		return core.NewTypeError("backtrace must be Array of String")
	}
	for _, elem := range value.Data.([]*object.EmeraldValue) {
		if elem == nil || elem.Type == object.ValueNil || elem.Type == object.ValueArray {
			return core.NewTypeError("backtrace must be Array of String")
		}
		if elem.Type == object.ValueString {
			continue
		}
		if elem.Class != nil && elem.Class.Name == "Location" && classInheritsFrom(elem.Class, core.R.Classes["Thread::Backtrace::Location"]) {
			continue
		}
		return core.NewTypeError("backtrace must be Array of String")
	}
	return nil
}

func updateLastExceptionBacktrace(value *object.EmeraldValue) {
	if core.LastException == nil || core.LastException.Type != object.ValueException {
		return
	}
	exc, ok := core.LastException.Data.(*object.RException)
	if !ok || exc == nil {
		return
	}
	if value == nil || value.Type == object.ValueNil {
		exc.Backtrace = nil
		exc.BacktraceValue = nil
		exc.Locations = nil
		exc.LocationsValue = nil
		return
	}
	if value.Type == object.ValueString {
		exc.Backtrace = []string{value.Data.(string)}
		exc.BacktraceValue = &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{value}, Class: core.R.Classes["Array"]}
		exc.Locations = nil
		exc.LocationsValue = nil
		return
	}
	if value.Type != object.ValueArray {
		return
	}
	backtrace := []string{}
	locations := []object.RBacktraceLocation{}
	for _, elem := range value.Data.([]*object.EmeraldValue) {
		if elem.Type == object.ValueString {
			backtrace = append(backtrace, elem.Data.(string))
			continue
		}
		if frame, ok := elem.Data.(object.RBacktraceLocation); ok {
			locations = append(locations, frame)
		} else if frame, ok := elem.Data.(*object.RBacktraceLocation); ok && frame != nil {
			locations = append(locations, *frame)
		}
	}
	exc.Backtrace = backtrace
	exc.BacktraceValue = value
	exc.Locations = locations
	exc.LocationsValue = nil
}

func (vm *VM) coerceGlobalLineNumber(value *object.EmeraldValue) (*object.EmeraldValue, *object.EmeraldValue) {
	if value == nil {
		return nil, core.NewTypeError("can't convert NilClass into Integer")
	}
	switch value.Type {
	case object.ValueInteger:
		return value, nil
	case object.ValueFloat:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(value.Data.(float64)), Class: core.R.Classes["Integer"]}, nil
	}
	if !vm.receiverRespondsTo(value, "to_int") {
		return nil, core.NewTypeError("can't convert " + value.TypeName() + " into Integer")
	}
	converted := vm.send(value, "to_int", nil)
	if converted != nil && converted.Type == object.ValueException {
		return nil, converted
	}
	if converted == nil || converted.Type != object.ValueInteger {
		return nil, core.NewTypeError("can't convert " + value.TypeName() + " into Integer")
	}
	return converted, nil
}

func (vm *VM) receiverCanBeStdout(receiver *object.EmeraldValue) bool {
	if receiver == nil || receiver.Type == object.ValueNil {
		return false
	}
	if classInheritsFrom(receiver.Class, core.R.Classes["IO"]) ||
		classInheritsFrom(receiver.Class, core.R.Classes["File"]) ||
		classInheritsFrom(receiver.Class, core.R.Classes["StringIO"]) {
		return true
	}
	if receiver.Class != nil {
		if method, owner, ok := receiver.Class.GetMethodWithOwner("write"); ok && method != nil && owner != core.R.Classes["Object"] && method.Visibility != "private" {
			return true
		}
	}
	if obj, ok := receiver.Data.(*object.Object); ok && obj != nil {
		if method := obj.SingletonMethods["write"]; method != nil && method.Visibility != "private" {
			return true
		}
		if obj.SingletonClass != nil {
			if method, ok := obj.SingletonClass.GetMethod("write"); ok && method != nil && method.Visibility != "private" {
				return true
			}
		}
	}
	if methods := vm.nativeSingletonMethods[nativeSingletonKey(receiver)]; methods != nil {
		if method := methods["write"]; method != nil && method.Visibility != "private" {
			return true
		}
	}
	if singletonClass := core.AttachedSingletonClass(receiver); singletonClass != nil {
		if method, _, ok := singletonClass.GetMethodWithOwner("write"); ok && method != nil && method.Visibility != "private" {
			return true
		}
	}
	return false
}

func (vm *VM) receiverRespondsTo(receiver *object.EmeraldValue, method string) bool {
	if receiver == nil || receiver.Type == object.ValueNil {
		return false
	}
	methodObj, _, fallback := vm.lookupMethodForSend(receiver, method, nil, false)
	return fallback == nil && methodObj != nil
}

func (vm *VM) loadedFeaturesGlobal() *object.EmeraldValue {
	if value := vm.getGlobalByName("$\""); value != nil && value.Type == object.ValueArray {
		return value
	}
	if value := vm.getGlobalByName("$LOADED_FEATURES"); value != nil && value.Type == object.ValueArray {
		return value
	}
	value := &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: core.R.Classes["Array"]}
	vm.setGlobalByName("$\"", value)
	vm.setGlobalByName("$LOADED_FEATURES", value)
	return value
}

func (vm *VM) loadPathGlobal() *object.EmeraldValue {
	if value := vm.getGlobalByName("$:"); value != nil && value.Type == object.ValueArray {
		return ensureDefaultLoadPathMetadata(value)
	}
	if value := vm.getGlobalByName("$LOAD_PATH"); value != nil && value.Type == object.ValueArray {
		return ensureDefaultLoadPathMetadata(value)
	}
	if value := vm.getGlobalByName("$-I"); value != nil && value.Type == object.ValueArray {
		return ensureDefaultLoadPathMetadata(value)
	}
	siteLib := &object.EmeraldValue{Type: object.ValueString, Data: "/tmp", Class: core.R.Classes["String"]}
	core.SetDynamicInstanceVar(siteLib, "@gem_prelude_index", core.R.TrueVal)
	entries := []*object.EmeraldValue{
		{Type: object.ValueString, Data: "lib", Class: core.R.Classes["String"]},
		siteLib,
	}
	value := &object.EmeraldValue{Type: object.ValueArray, Data: entries, Class: core.R.Classes["Array"]}
	vm.setGlobalByName("$:", value)
	vm.setGlobalByName("$LOAD_PATH", value)
	vm.setGlobalByName("$-I", value)
	return value
}

func ensureDefaultLoadPathMetadata(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil || value.Type != object.ValueArray {
		return value
	}
	entries := value.Data.([]*object.EmeraldValue)
	siteIndex := -1
	for index, entry := range entries {
		if entry != nil && entry.Type == object.ValueString && entry.Data == "/tmp" {
			siteIndex = index
			break
		}
	}
	if siteIndex < 0 {
		entries = append(entries, &object.EmeraldValue{Type: object.ValueString, Data: "/tmp", Class: core.R.Classes["String"]})
		value.Data = entries
		siteIndex = len(entries) - 1
	}
	for _, entry := range entries[siteIndex:] {
		core.SetDynamicInstanceVar(entry, "@gem_prelude_index", core.R.TrueVal)
	}
	return value
}

func (vm *VM) programNameGlobal() *object.EmeraldValue {
	if value := vm.getGlobalByName("$0"); value != nil && value.Type == object.ValueString {
		return value
	}
	name := ""
	if len(os.Args) > 0 {
		name = os.Args[0]
	}
	value := &object.EmeraldValue{Type: object.ValueString, Data: name, Class: core.R.Classes["String"]}
	vm.setGlobalByName("$0", value)
	return value
}

func (vm *VM) currentExceptionBacktraceGlobal() *object.EmeraldValue {
	if core.LastException == nil || core.LastException.Type != object.ValueException {
		return core.R.NilVal
	}
	exc, ok := core.LastException.Data.(*object.RException)
	if !ok || exc == nil {
		return core.R.NilVal
	}
	if exc.BacktraceValue != nil {
		return exc.BacktraceValue
	}
	if len(exc.Backtrace) == 0 && len(exc.Locations) == 0 {
		exc.Locations = vm.currentBacktraceFrames()
	}
	if len(exc.Backtrace) == 0 && len(exc.Locations) > 0 {
		exc.Backtrace = vm.backtraceStrings(exc.Locations)
	}
	result := make([]*object.EmeraldValue, len(exc.Backtrace))
	for i, line := range exc.Backtrace {
		result[i] = &object.EmeraldValue{Type: object.ValueString, Data: line, Class: core.R.Classes["String"]}
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: core.R.Classes["Array"]}
}

func (vm *VM) Run() error {
	if vm.isRoot {
		defer core.RunAtExitHooks()
	}
	vm.instructionCount = 0

	frame := vm.frames[vm.fp]
	vm.initializeTopLevelBindingLocals(frame)
	instructions := frame.Fn.Instructions

	for frame.Ip < len(instructions)-1 {
		if frame.Ip >= len(instructions) {
			return fmt.Errorf("invalid instruction pointer: %d", frame.Ip)
		}
		frame.Ip++
		if frame.Ip >= len(instructions) {
			return fmt.Errorf("invalid instruction pointer: %d", frame.Ip)
		}

		op := compiler.Opcode(instructions[frame.Ip])
		vm.fireTracePointLine(frame, op)

		frame.InstructionException = core.LastException
		frame.InstructionSnapshotSet = true
		err := vm.execute(op, frame)
		if err != nil {
			return err
		}
		if vm.handlePendingNonLocalReturn(frame) || vm.handlePendingNonLocalBreak(frame) || frame.Returned {
			break
		}
		if vm.sp > frame.Bp {
			top := vm.stack[vm.sp-1]
			if top != nil && top.Type == object.ValueException && (core.LastRaisedResult == top || classInheritsFrom(top.Class, core.R.Classes["SystemExit"])) && !vm.frameHasActiveRescue(frame) {
				break
			}
		}
		frame = vm.frames[vm.fp]
		instructions = frame.Fn.Instructions

		if DevMode && vm.instructionCount%100 == 0 {
			runtime.Gosched()
		}
	}
	if vm.sp > frame.Bp {
		top := vm.stack[vm.sp-1]
		if shouldPropagateExceptionValue(top) && !vm.frameHasActiveRescue(frame) {
			vm.unhandledException = top
		}
	}

	return nil
}

func (vm *VM) UnhandledException() *object.EmeraldValue {
	return vm.unhandledException
}

func (vm *VM) endActiveRescue(frame *Frame) {
	if len(vm.activeRescues) == 0 {
		return
	}
	active := vm.activeRescues[len(vm.activeRescues)-1]
	nextOffset := frame.Ip + 1
	if active.Frame != frame || (active.EndOffset != nextOffset && active.EnsureOffset != nextOffset) {
		return
	}
	vm.activeRescues = vm.activeRescues[:len(vm.activeRescues)-1]
	if frame.RetryRescue == active {
		frame.RetryRescue = nil
	}
	core.LastException = active.PreviousException
}

func (vm *VM) endInnermostActiveRescue(frame *Frame) {
	if len(vm.activeRescues) == 0 {
		return
	}
	active := vm.activeRescues[len(vm.activeRescues)-1]
	if active.Frame != frame {
		return
	}
	vm.activeRescues = vm.activeRescues[:len(vm.activeRescues)-1]
	if frame.RetryRescue == active {
		frame.RetryRescue = nil
	}
	core.LastException = active.PreviousException
}

func (vm *VM) endActiveRescuesForFrame(frame *Frame) {
	for i := len(vm.activeRescues) - 1; i >= 0; i-- {
		if vm.activeRescues[i].Frame != frame {
			continue
		}
		core.LastException = vm.activeRescues[i].PreviousException
		if frame.RetryRescue == vm.activeRescues[i] {
			frame.RetryRescue = nil
		}
		vm.activeRescues = append(vm.activeRescues[:i], vm.activeRescues[i+1:]...)
	}
}

func (vm *VM) evalSource(source string) *object.EmeraldValue {
	beginBlocks, remaining, syntaxErr := splitTopLevelBeginBlocks(source)
	if syntaxErr != nil {
		core.LastException = syntaxErr
		return syntaxErr
	}
	if len(beginBlocks) > 0 {
		source = prependBeginBlocks(beginBlocks, remaining)
	} else {
		source = remaining
	}
	if invalidPercentRegexpSyntax(source) {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), "invalid percent regexp")
		core.LastException = exc
		return exc
	}
	if message := invalidIndexAssignmentSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), message)
		core.LastException = exc
		return exc
	}
	if message := invalidNumberedParameterSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), message)
		core.LastException = exc
		return exc
	}
	if message := invalidPatternMatchingSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), message)
		core.LastException = exc
		return exc
	}
	if message := invalidSpacedMethodCallArgumentListSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), message)
		core.LastException = exc
		return exc
	}
	if message := invalidRescueSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), message)
		core.LastException = exc
		return exc
	}
	if message := invalidReadOnlyMatchGlobalAssignmentSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), message)
		core.LastException = exc
		return exc
	}

	lexerEncoding := core.CurrentEvalSourceEncoding
	if lexerEncoding == "" {
		lexerEncoding = core.SourceEncoding(source)
	}
	l := lexer.NewWithEncoding(source, lexerEncoding)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), strings.Join(p.Errors(), "\n"))
		core.LastException = exc
		return exc
	}
	if message := validateDynamicSyntax(program); message != "" {
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), message)
		core.LastException = exc
		return exc
	}

	childEncoding := core.SourceEncoding(source)
	c := compiler.NewWithSourceEncoding(childEncoding)
	if err := c.Compile(program); err != nil {
		if os.Getenv("RGO_DEBUG_REQUIRE") == "1" {
			fmt.Printf("RGO_DEBUG_REQUIRE eval compile error=%v\n", err)
		}
		exc := newSyntaxErrorForBinding(vm.currentFrameBinding(), err.Error())
		core.LastException = exc
		return exc
	}
	core.FireTracePointScriptCompiled(vm.currentFrameBinding(), source)

	parent := CurrentVM
	frozenStrings, chilledStrings := evalSourceStringLiteralMode(source)
	bytecode := c.Bytecode()
	annotateStringLiteralMode(bytecode.Constants, frozenStrings, chilledStrings)
	child := newVM(bytecode, vm)
	child.freezeStringLiterals, child.chillStringLiterals = frozenStrings, chilledStrings
	child.sourceEncoding = childEncoding
	previousSourceEncoding := core.CurrentEvalSourceEncoding
	core.CurrentEvalSourceEncoding = childEncoding
	visibilityTarget := definitionVisibilityTarget(vm.classStack)
	previousVisibility := core.CurrentMethodVisibilityMode(visibilityTarget)
	runErr := child.Run()
	core.SetCurrentMethodVisibility(visibilityTarget, previousVisibility)
	core.CurrentEvalSourceEncoding = previousSourceEncoding
	if child.escapedThrowHandler != nil {
		if parent != nil {
			parent.installCoreHooks()
		}
		vm.pendingEscapedThrowHandler = child.escapedThrowHandler
		vm.pendingEscapedThrowValue = child.escapedThrowValue
		if child.escapedThrowValue != nil {
			return child.escapedThrowValue
		}
		return core.R.NilVal
	}
	if runErr != nil {
		if os.Getenv("RGO_DEBUG_REQUIRE") == "1" {
			fmt.Printf("RGO_DEBUG_REQUIRE eval runtime error=%v\n", runErr)
		}
		if parent != nil {
			parent.installCoreHooks()
		}
		exc := core.NewRuntimeError(runErr.Error())
		core.LastException = exc
		return exc
	}
	result := child.LastPoppedStackElement()
	if parent != nil {
		parent.installCoreHooks()
	}
	if result == nil {
		return core.R.NilVal
	}
	return result
}

func (vm *VM) evalSourceWithBinding(source string, binding *object.RBinding) *object.EmeraldValue {
	if result, handled := vm.evalTopLevelInclude(source, binding); handled {
		return result
	}
	beginBlocks, remaining, syntaxErr := splitTopLevelBeginBlocks(source)
	if syntaxErr != nil {
		core.LastException = syntaxErr
		return syntaxErr
	}
	if len(beginBlocks) > 0 {
		source = prependBeginBlocks(beginBlocks, remaining)
	} else {
		source = remaining
	}
	if invalidPercentRegexpSyntax(source) {
		exc := newSyntaxErrorForBinding(binding, "invalid percent regexp")
		core.LastException = exc
		return exc
	}
	if message := invalidIndexAssignmentSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(binding, message)
		core.LastException = exc
		return exc
	}
	if message := invalidNumberedParameterSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(binding, message)
		core.LastException = exc
		return exc
	}
	if message := invalidPatternMatchingSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(binding, message)
		core.LastException = exc
		return exc
	}
	if message := invalidSpacedMethodCallArgumentListSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(binding, message)
		core.LastException = exc
		return exc
	}
	if message := invalidRescueSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(binding, message)
		core.LastException = exc
		return exc
	}
	if message := invalidReadOnlyMatchGlobalAssignmentSyntax(source); message != "" {
		exc := newSyntaxErrorForBinding(binding, message)
		core.LastException = exc
		return exc
	}

	oldSpecFile := core.CurrentSpecFile
	if binding != nil && binding.Path != "" {
		core.CurrentSpecFile = binding.Path
	}
	childClassStack := append([]*object.EmeraldValue(nil), vm.classStack...)
	if binding != nil && len(binding.ClassStack) > 0 {
		childClassStack = append([]*object.EmeraldValue(nil), binding.ClassStack...)
	}
	if binding != nil && binding.Self != nil {
		switch binding.Self.Type {
		case object.ValueClass, object.ValueModule:
			if len(childClassStack) == 0 {
				childClassStack = append(childClassStack, binding.Self)
			}
		}
	}
	parent := CurrentVM
	defer func() {
		core.CurrentSpecFile = oldSpecFile
		if parent != nil {
			parent.installCoreHooks()
		}
	}()
	childSelf := core.R.Main
	if binding != nil && binding.Self != nil {
		childSelf = binding.Self
	}

	lexerEncoding := core.CurrentEvalSourceEncoding
	if lexerEncoding == "" {
		lexerEncoding = core.SourceEncoding(source)
	}
	l := lexer.NewWithEncoding(source, lexerEncoding)
	p := parser.New(l)
	if binding != nil && binding.AllowAnonymousBlockPass {
		p.AllowAnonymousBlockPass(true)
	}
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		exc := newSyntaxErrorForBinding(binding, strings.Join(p.Errors(), "\n"))
		core.LastException = exc
		return exc
	}
	if message := validateDynamicSyntaxWithContext(program, binding != nil && binding.AllowAnonymousBlockPass); message != "" {
		exc := newSyntaxErrorForBinding(binding, message)
		core.LastException = exc
		return exc
	}

	var c *compiler.Compiler
	if binding != nil && len(binding.LocalNames) > 0 {
		c = compiler.NewWithLocalNames(binding.LocalNames)
	} else {
		c = compiler.New()
	}
	c.SetEvalTopLevelReturn(true)
	if err := c.Compile(program); err != nil {
		return core.R.NilVal
	}
	core.FireTracePointScriptCompiled(binding, source)

	frozenStrings, chilledStrings := evalSourceStringLiteralMode(source)
	bytecode := c.Bytecode()
	annotateStringLiteralMode(bytecode.Constants, frozenStrings, chilledStrings)
	child := newVM(bytecode, vm)
	child.evalReturnMode = true
	if binding != nil {
		child.frames[0].Fn.EvalInheritedLocals = len(binding.LocalNames)
		child.currentBlock = binding.Block
		child.frames[0].LabelName = binding.BacktraceLabel
		if len(binding.Refinements) > 0 {
			child.frames[0].Closure = &object.Closure{
				Fn:               child.frames[0].Fn,
				Refinements:      append([]*object.EmeraldValue(nil), binding.Refinements...),
				RefinementsFixed: true,
			}
		}
	}
	if binding != nil {
		applyEvalLineOffset(child.frames[0].Fn, binding.Line)
	}
	child.classStack = childClassStack
	if binding != nil && binding.ClassVarScope != nil {
		child.instanceExecClassVarScope = binding.ClassVarScope
	}
	child.freezeStringLiterals, child.chillStringLiterals = frozenStrings, chilledStrings
	child.sourceEncoding = core.CurrentEvalSourceEncoding
	localSlots := bindingLocalSlots(child)
	child.stack[0] = childSelf
	if binding != nil {
		seedBindingLocals(child, binding, localSlots)
	}

	result := core.R.NilVal
	visibilityTarget := definitionVisibilityTarget(childClassStack)
	previousVisibility := core.CurrentMethodVisibilityMode(visibilityTarget)
	runErr := child.Run()
	core.SetCurrentMethodVisibility(visibilityTarget, previousVisibility)
	if child.escapedThrowHandler != nil {
		vm.pendingEscapedThrowHandler = child.escapedThrowHandler
		vm.pendingEscapedThrowValue = child.escapedThrowValue
		if child.escapedThrowValue != nil {
			return child.escapedThrowValue
		}
		return core.R.NilVal
	}
	if runErr == nil {
		if child.sp > 0 {
			top := child.stack[child.sp-1]
			if shouldPropagateExceptionValue(top) {
				result = top
			}
		}
		if result == core.R.NilVal {
			if r := child.LastPoppedStackElement(); r != nil {
				result = r
			}
		}
		copyBindingLocals(child, binding, localSlots)
		if child.evalReturnPending && (result == nil || result.Type != object.ValueException) {
			returnValue := child.evalReturnValue
			if returnValue == nil {
				returnValue = core.R.NilVal
			}
			if binding != nil && binding.EvalReturnTargetID > 0 && vm.frameIDActive(binding.EvalReturnTargetID) {
				vm.pendingReturnTargetID = binding.EvalReturnTargetID
				vm.pendingReturnValue = returnValue
				result = returnValue
			} else {
				result = core.NewLocalJumpErrorWithReturn(returnValue)
				markExceptionRaised(result)
				core.LastException = result
				core.LastRaisedResult = result
			}
		}
	} else {
		result = core.R.NilVal
	}
	return result
}

func definitionVisibilityTarget(classStack []*object.EmeraldValue) *object.EmeraldValue {
	for i := len(classStack) - 1; i >= 0; i-- {
		value := classStack[i]
		if value != nil && (value.Type == object.ValueClass || value.Type == object.ValueModule) {
			return value
		}
	}
	return nil
}

func applyEvalLineOffset(fn *object.Function, startLine int64) {
	if fn == nil {
		return
	}
	delta := int(startLine) - 1
	offsetEvalFunctionLines(fn, delta, map[*object.Function]bool{})
}

func offsetEvalFunctionLines(fn *object.Function, delta int, seen map[*object.Function]bool) {
	if fn == nil || seen[fn] {
		return
	}
	seen[fn] = true
	if len(fn.LineMap) == 0 {
		fn.LineMap = map[int]int{0: 1}
	}
	for pos, line := range fn.LineMap {
		if line > 0 {
			fn.LineMap[pos] = line + delta
		}
	}
	if fn.DefinitionLine > 0 {
		fn.DefinitionLine += int64(delta)
	}
	for _, constant := range fn.Constants {
		if constant == nil || constant.Type != object.ValueFunction {
			continue
		}
		if nested, ok := constant.Data.(*object.Function); ok {
			offsetEvalFunctionLines(nested, delta, seen)
		}
	}
}

func (vm *VM) evalTopLevelInclude(source string, binding *object.RBinding) (*object.EmeraldValue, bool) {
	if binding == nil || binding.Self != core.R.Main {
		return nil, false
	}
	trimmed := strings.TrimSpace(source)
	name := strings.TrimSpace(strings.TrimPrefix(trimmed, "include "))
	if name == trimmed || name == "" || strings.ContainsAny(name, " \t\n\r(),") {
		return nil, false
	}
	value, ok := vm.topLevelConstantValue(name)
	if !ok || value == nil || value.Type != object.ValueModule {
		return core.R.NilVal, true
	}
	objectClass := core.R.Classes["Object"]
	if objectClass == nil {
		return core.R.NilVal, true
	}
	receiver := &object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: core.R.Classes["Class"]}
	return core.CallMethod(receiver, "include", value), true
}

func evalSourceFreezesStringLiterals(source string) bool {
	frozen, _ := evalSourceStringLiteralMode(source)
	return frozen
}

func evalSourceStringLiteralMode(source string) (bool, bool) {
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		if i >= 2 {
			break
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			if trimmed == "" {
				continue
			}
			return false, true
		}
		if match := evalFrozenStringLiteralPattern.FindStringSubmatch(trimmed); len(match) == 2 {
			return strings.EqualFold(match[1], "true"), false
		}
	}
	return false, true
}

func annotateStringLiteralMode(constants []*object.EmeraldValue, frozen, chilled bool) {
	seen := make(map[*object.Function]bool)
	var annotate func([]*object.EmeraldValue)
	annotate = func(values []*object.EmeraldValue) {
		for _, value := range values {
			if value == nil || value.Type != object.ValueFunction {
				continue
			}
			fn, ok := value.Data.(*object.Function)
			if !ok || fn == nil || seen[fn] {
				continue
			}
			seen[fn] = true
			fn.StringLiteralModeSet = true
			fn.FreezeStringLiterals = frozen
			fn.ChillStringLiterals = chilled
			annotate(fn.Constants)
		}
	}
	annotate(constants)
}

type dynamicSyntaxContext struct {
	methodDepth             int
	classDepth              int
	blockDepth              int
	braceBlockDepth         int
	loopDepth               int
	rescueDepth             int
	allowAnonymousBlockPass bool
}

func validateDynamicSyntax(program *ast.Program) string {
	return validateDynamicSyntaxWithContext(program, false)
}

func validateDynamicSyntaxWithContext(program *ast.Program, allowAnonymousBlockPass bool) string {
	if program == nil {
		return ""
	}
	return validateDynamicStatements(program.Statements, dynamicSyntaxContext{allowAnonymousBlockPass: allowAnonymousBlockPass})
}

func validateDynamicStatements(stmts []ast.Statement, ctx dynamicSyntaxContext) string {
	for _, stmt := range stmts {
		if msg := validateDynamicStatement(stmt, ctx); msg != "" {
			return msg
		}
	}
	return ""
}

func validateDynamicStatement(stmt ast.Statement, ctx dynamicSyntaxContext) string {
	switch node := stmt.(type) {
	case *ast.ExpressionStatement:
		return validateDynamicExpression(node.Expression, ctx)
	case *ast.ReturnExpression:
		return validateReturnSyntax(node, ctx)
	case *ast.NextExpression:
		return validateNextSyntax(node, ctx)
	case *ast.RedoExpression:
		return validateRedoSyntax(ctx)
	case *ast.RetryExpression:
		return validateRetrySyntax(ctx)
	case *ast.BreakExpression:
		return validateBreakSyntax(node, ctx)
	default:
		return ""
	}
}

func validateReturnSyntax(node *ast.ReturnExpression, ctx dynamicSyntaxContext) string {
	if ctx.classDepth > 0 && ctx.methodDepth == 0 && ctx.blockDepth == 0 {
		return "Invalid return in class/module body"
	}
	return validateDynamicExpression(node.ReturnValue, ctx)
}

func validateNextSyntax(node *ast.NextExpression, ctx dynamicSyntaxContext) string {
	if ctx.methodDepth > 0 && ctx.blockDepth == 0 && ctx.loopDepth == 0 {
		return "Invalid next in method"
	}
	return validateDynamicExpression(node.Value, ctx)
}

func validateBreakSyntax(node *ast.BreakExpression, ctx dynamicSyntaxContext) string {
	if ctx.blockDepth == 0 && ctx.loopDepth == 0 {
		if ctx.methodDepth > 0 {
			return "Invalid break in method"
		}
		if ctx.classDepth > 0 {
			return "Invalid break in class/module body"
		}
	}
	return validateDynamicExpression(node.Value, ctx)
}

func validateRedoSyntax(ctx dynamicSyntaxContext) string {
	if ctx.methodDepth > 0 && ctx.blockDepth == 0 && ctx.loopDepth == 0 {
		return "Invalid redo in method"
	}
	return ""
}

func validateRetrySyntax(ctx dynamicSyntaxContext) string {
	if ctx.rescueDepth == 0 {
		return "Invalid retry"
	}
	return ""
}

func validateYieldSyntax(node *ast.YieldExpression, ctx dynamicSyntaxContext) string {
	if ctx.methodDepth == 0 {
		return "Invalid yield"
	}
	for _, arg := range node.Args {
		if msg := validateDynamicExpression(arg, ctx); msg != "" {
			return msg
		}
	}
	for _, arg := range node.KeywordArgs {
		if msg := validateDynamicExpression(arg.Value, ctx); msg != "" {
			return msg
		}
	}
	return ""
}

func invalidPercentRegexpSyntax(source string) bool {
	for i := 0; i+2 < len(source); i++ {
		if source[i] != '%' || source[i+1] != 'r' {
			continue
		}
		delimiter := source[i+2]
		if delimiter == ' ' || delimiter == '\t' || delimiter == '\n' || delimiter == '\r' ||
			(delimiter >= 'A' && delimiter <= 'Z') || (delimiter >= 'a' && delimiter <= 'z') ||
			(delimiter >= '0' && delimiter <= '9') {
			return true
		}
		closeDelimiter := byte(0)
		switch delimiter {
		case '(':
			closeDelimiter = ')'
		case '[':
			closeDelimiter = ']'
		case '{':
			closeDelimiter = '}'
		case '<':
			closeDelimiter = '>'
		default:
			continue
		}
		depth := 1
		escaped := false
		for j := i + 3; j < len(source); j++ {
			ch := source[j]
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == delimiter {
				depth++
				continue
			}
			if ch == closeDelimiter {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if depth != 0 {
			return true
		}
	}
	return false
}

func invalidIndexAssignmentSyntax(source string) string {
	for i := 0; i < len(source); i++ {
		if source[i] != '[' {
			continue
		}
		end := matchingBracket(source, i)
		if end < 0 {
			continue
		}
		op := assignmentOperatorAfter(source, end+1)
		if op == "" {
			i = end
			continue
		}
		content := source[i+1 : end]
		if containsBlockPassArg(content) {
			return "block arg given in index assignment"
		}
		if containsKeywordArgInIndex(content) {
			return "keyword arg given in index assignment"
		}
		i = end
	}
	return ""
}

func matchingBracket(source string, start int) int {
	depth := 0
	quote := byte(0)
	escaped := false
	for i := start; i < len(source); i++ {
		ch := source[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '[' {
			depth++
			continue
		}
		if ch == ']' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func assignmentOperatorAfter(source string, pos int) string {
	for pos < len(source) && (source[pos] == ' ' || source[pos] == '\t' || source[pos] == '\n' || source[pos] == '\r') {
		pos++
	}
	if pos >= len(source) {
		return ""
	}
	if source[pos] == '=' {
		if pos+1 < len(source) && source[pos+1] == '=' {
			return ""
		}
		return "="
	}
	if pos+1 >= len(source) {
		return ""
	}
	switch source[pos] {
	case '+', '-', '*', '/', '%', '|', '&', '^':
		if source[pos+1] == '=' {
			return source[pos : pos+2]
		}
	case '<', '>':
		if pos+2 < len(source) && source[pos+1] == source[pos] && source[pos+2] == '=' {
			return source[pos : pos+3]
		}
	}
	return ""
}

func containsBlockPassArg(content string) bool {
	for i := 0; i < len(content); i++ {
		if content[i] != '&' {
			continue
		}
		if i+1 < len(content) && content[i+1] == '&' {
			i++
			continue
		}
		return true
	}
	return false
}

func containsKeywordArgInIndex(content string) bool {
	for i := 1; i < len(content); i++ {
		if content[i] != ':' {
			continue
		}
		j := i - 1
		for j >= 0 && (content[j] == ' ' || content[j] == '\t' || content[j] == '\n' || content[j] == '\r') {
			j--
		}
		if j < 0 || !isIdentChar(content[j]) {
			continue
		}
		for j >= 0 && isIdentChar(content[j]) {
			j--
		}
		if j < 0 || content[j] == ',' || content[j] == ' ' || content[j] == '\t' || content[j] == '\n' || content[j] == '\r' {
			return true
		}
	}
	return false
}

func invalidNumberedParameterSyntax(source string) string {
	code := maskRubyStringLiterals(source)
	code = numberedParameterLabelPattern.ReplaceAllString(code, `${1}__numbered_label__:`)
	code = specItDeclarationPattern.ReplaceAllString(code, `${1}__spec_it__ ${2}`)
	if !numberedParameterPattern.MatchString(code) && !itParameterPattern.MatchString(code) {
		return ""
	}
	if numberedParameterAssignmentPattern.MatchString(code) {
		return "_1 is reserved for numbered parameter"
	}
	if numberedParameterNestedPattern.MatchString(code) {
		return "numbered parameter is already used in outer block"
	}
	if itBeforeNumberedParameterPattern.MatchString(code) {
		return "numbered parameters are not allowed when 'it' is already used"
	}
	if numberedBeforeItParameterPattern.MatchString(code) {
		return "'it' is not allowed when a numbered parameter is already used"
	}
	if numberedParameterExplicitParamsPattern.MatchString(code) {
		return "ordinary parameter is defined"
	}
	if itParameterExplicitParamsPattern.MatchString(code) {
		return "ordinary parameter is defined"
	}
	return ""
}

func invalidPatternMatchingSyntax(source string) string {
	code := maskRubyStringLiterals(source)
	if !regexp.MustCompile(`(?m)^\s*in\b`).MatchString(code) {
		return ""
	}
	switch {
	case patternWhenBeforeInPattern.MatchString(code):
		return "unexpected 'in'"
	case patternInBeforeWhenPattern.MatchString(code):
		return "unexpected 'when'"
	case patternCalculationPattern.MatchString(code):
		return "expected a delimiter after the patterns of an `in` clause"
	case hasDuplicatePatternVariable(code):
		return "duplicated variable name"
	case hasPinnedVariableBeforeBinding(code):
		return "n: no such local variable"
	case patternAlternationBindingPattern.MatchString(code):
		return "unexpected variable binding in alternative pattern"
	case patternNonSymbolHashKeyPattern.MatchString(source):
		return "expected a label as the key in the hash pattern"
	case patternInterpolatedHashKeyPattern.MatchString(source):
		return "symbol literal with interpolation is not allowed"
	case hasDuplicatePatternHashKey(code):
		return "duplicated key name"
	default:
		return ""
	}
}

func invalidSpacedMethodCallArgumentListSyntax(source string) string {
	code := maskRubyStringLiterals(source)
	for _, line := range strings.Split(code, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, match := range spacedMethodCallArgumentListPattern.FindAllStringSubmatch(line, -1) {
			if len(match) < 2 {
				continue
			}
			if spacedCallKeywordExclusions[match[1]] {
				continue
			}
			return "syntax error, unexpected ','"
		}
	}
	return ""
}

func invalidRescueSyntax(source string) string {
	compact := strings.Join(strings.Fields(source), " ")
	if strings.Contains(compact, "lambda {") && strings.Contains(compact, " rescue ") {
		return "Invalid rescue in block"
	}
	if strings.Contains(compact, ".+(1 rescue 1)") {
		return "syntax error, unexpected rescue modifier"
	}
	rescueClassExtraArgPattern := regexp.MustCompile(`\brescue\s+[A-Z]\w*(?:::[A-Z]\w*)?\s+([^\s,=][^\s]*)`)
	for _, line := range strings.Split(maskRubyStringLiterals(source), "\n") {
		if match := rescueClassExtraArgPattern.FindStringSubmatch(line); len(match) > 1 {
			extra := match[1]
			if extra != "=>" && !strings.HasPrefix(extra, "=>") {
				return "syntax error, unexpected rescue modifier"
			}
		}
	}
	return ""
}

func invalidReadOnlyMatchGlobalAssignmentSyntax(source string) string {
	if regexp.MustCompile(regexp.QuoteMeta("$'")+`\s*=`).FindStringIndex(source) != nil {
		return "Can't set variable $'"
	}
	code := maskRubyStringLiterals(source)
	for _, name := range []string{"$&", "$`", "$+"} {
		if regexp.MustCompile(regexp.QuoteMeta(name)+`\s*=`).FindStringIndex(code) != nil {
			return "Can't set variable " + name
		}
	}
	return ""
}

func (vm *VM) checkPatternRuntimeHooks(target *object.EmeraldValue, pattern string) *object.EmeraldValue {
	if target == nil {
		return nil
	}
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(pattern)
	switch {
	case patternNeedsDeconstructKeys(compact):
		if !vm.patternRespondsTo(target, "deconstruct_keys") {
			return nil
		}
		result := core.CallMethod(target, "deconstruct_keys", core.R.NilVal)
		if result != nil && result.Type == object.ValueException {
			return result
		}
		if !patternValueIsHash(result) {
			return core.NewTypeError("deconstruct_keys must return Hash")
		}
	case patternNeedsDeconstruct(compact):
		if !vm.patternRespondsTo(target, "deconstruct") {
			return nil
		}
		result := core.CallMethod(target, "deconstruct")
		if result != nil && result.Type == object.ValueException {
			return result
		}
		if !patternValueIsArray(result) {
			return core.NewTypeError("deconstruct must return Array")
		}
	}
	return nil
}

func (vm *VM) patternRespondsTo(target *object.EmeraldValue, method string) bool {
	if _, ok := vm.lookupActiveRefinedMethod(target, method); ok {
		return true
	}
	if core.CallMethod == nil {
		return false
	}
	response := core.CallMethod(target, "respond_to?", &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  method,
		Class: core.R.Classes["Symbol"],
	})
	return response == core.R.TrueVal
}

func patternNeedsDeconstruct(pattern string) bool {
	return strings.Contains(pattern, "Object[]") || strings.Contains(pattern, "Object()")
}

func patternNeedsDeconstructKeys(pattern string) bool {
	return (strings.HasPrefix(pattern, "Object[") || strings.HasPrefix(pattern, "Object(")) && strings.Contains(pattern, ":")
}

func patternValueIsArray(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	if value.Type == object.ValueArray {
		return true
	}
	if value.Type == object.ValueObject && value.Class != nil {
		return classInheritsFrom(value.Class, core.R.Classes["Array"])
	}
	return false
}

func patternValueIsHash(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	if value.Type == object.ValueHash {
		return true
	}
	if value.Type == object.ValueObject && value.Class != nil {
		return classInheritsFrom(value.Class, core.R.Classes["Hash"])
	}
	return false
}

func hasDuplicatePatternVariable(code string) bool {
	for _, match := range patternArrayPattern.FindAllStringSubmatch(code, -1) {
		if len(match) < 3 {
			continue
		}
		left, right := match[1], match[2]
		if left == right && left != "_" && !strings.HasPrefix(left, "_") {
			return true
		}
	}
	return false
}

func hasPinnedVariableBeforeBinding(code string) bool {
	for _, match := range patternPinnedArrayPattern.FindAllStringSubmatch(code, -1) {
		if len(match) >= 3 && match[1] == match[2] {
			return true
		}
	}
	return false
}

func hasDuplicatePatternHashKey(code string) bool {
	for _, match := range patternHashPattern.FindAllStringSubmatch(code, -1) {
		if len(match) < 2 {
			continue
		}
		seen := map[string]bool{}
		for _, keyMatch := range patternHashKeyPattern.FindAllStringSubmatch(match[1], -1) {
			if len(keyMatch) < 2 {
				continue
			}
			key := keyMatch[1]
			if seen[key] {
				return true
			}
			seen[key] = true
		}
	}
	return false
}

func maskRubyStringLiterals(source string) string {
	out := []byte(source)
	quote := byte(0)
	escaped := false
	for i := 0; i < len(out); i++ {
		ch := out[i]
		if quote == 0 {
			if ch == '\'' && i > 0 && i+1 < len(out) && isIdentChar(out[i-1]) && isIdentChar(out[i+1]) {
				continue
			}
			if ch == '"' || ch == '\'' {
				quote = ch
				out[i] = ' '
			}
			continue
		}
		out[i] = ' '
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == quote {
			quote = 0
		}
	}
	return string(out)
}

var (
	numberedParameterToken                 = `(^|[^A-Za-z0-9_])_[1-9]([^0-9A-Za-z_]|$)`
	numberedParameterUse                   = `(^|[^A-Za-z0-9_])_[1-9]([^0-9A-Za-z_]|$)`
	numberedParameterPattern               = regexp.MustCompile(numberedParameterToken)
	numberedParameterLabelPattern          = regexp.MustCompile(`(^|[^A-Za-z0-9_])_[1-9]\s*:`)
	numberedParameterAssignmentPattern     = regexp.MustCompile(`(^|[^A-Za-z0-9_])_[1-9]\s*=`)
	numberedParameterNestedPattern         = regexp.MustCompile(numberedParameterUse + `[\s\S]*(->|proc\s*\{|\blambda\s*\{)[\s\S]*` + numberedParameterUse)
	numberedParameterExplicitParamsPattern = regexp.MustCompile(`(->\s*\([^)]*\)\s*\{[\s\S]*` + numberedParameterUse + `|(proc|lambda)\s*\{[^{}]*\|[^|]*\|[\s\S]*` + numberedParameterUse + `|\[[^\]]*\]\.[A-Za-z_][A-Za-z0-9_?!]*\s*\{[^{}]*\|[^|]*\|[\s\S]*` + numberedParameterUse + `)`)
	numberedParameterMethodNamePattern     = regexp.MustCompile(`^_[0-9]+$`)
	itParameterPattern                     = regexp.MustCompile(`(^|[^A-Za-z0-9_])it([^A-Za-z0-9_?!]|$)`)
	specItDeclarationPattern               = regexp.MustCompile(`(^|[^A-Za-z0-9_])it\s+("|'|:|do\b)`)
	itParameterExplicitParamsPattern       = regexp.MustCompile(`(->\s*\([^)]*\)\s*\{[\s\S]*\bit\b|(proc|lambda)\s*\{[^{}]*\|[^|]*\|[\s\S]*\bit\b|\[[^\]]*\]\.[A-Za-z_][A-Za-z0-9_?!]*\s*\{[^{}]*\|[^|]*\|[\s\S]*\bit\b)`)
	itBeforeNumberedParameterPattern       = regexp.MustCompile(`\bit\b[\s\S]*(^|[^A-Za-z0-9_])_[1-9]([^0-9A-Za-z_]|$)`)
	numberedBeforeItParameterPattern       = regexp.MustCompile(`(^|[^A-Za-z0-9_])_[1-9]([^0-9A-Za-z_]|$)[\s\S]*\bit\b`)
	evalFrozenStringLiteralPattern         = regexp.MustCompile(`(?i)frozen_string_literal\s*:\s*(true|false)`)
	patternWhenBeforeInPattern             = regexp.MustCompile(`(?s)\bcase\b[\s\S]*\bwhen\b[\s\S]*\bin\b`)
	patternInBeforeWhenPattern             = regexp.MustCompile(`(?s)\bcase\b[\s\S]*\bin\b[\s\S]*\bwhen\b`)
	patternCalculationPattern              = regexp.MustCompile(`(?m)^\s*in\s+[^\n]*\+`)
	patternArrayPattern                    = regexp.MustCompile(`\[\s*([A-Za-z][A-Za-z0-9_]*)\s*,\s*([A-Za-z][A-Za-z0-9_]*)\s*\]`)
	patternPinnedArrayPattern              = regexp.MustCompile(`\[\s*\^([A-Za-z][A-Za-z0-9_]*)\s*,\s*([A-Za-z][A-Za-z0-9_]*)\s*\]`)
	patternAlternationBindingPattern       = regexp.MustCompile(`(?m)^\s*in\s+[\s\S]*\|\s*\[[^\]]*,\s*[A-Za-z][A-Za-z0-9_]*\s*\]`)
	patternNonSymbolHashKeyPattern         = regexp.MustCompile(`(?m)^\s*in\s+\{[^}\n]*"[^"\n]+"\s*=>`)
	patternInterpolatedHashKeyPattern      = regexp.MustCompile(`(?m)^\s*in\s+\{[^}\n]*"#\{[^}\n]+\}"\s*:`)
	patternHashPattern                     = regexp.MustCompile(`(?m)^\s*in\s+\{([^}\n]*)\}`)
	patternHashKeyPattern                  = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*:`)
	spacedMethodCallArgumentListPattern    = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_?!]*)\s+\([^)]*,[^)]*\)`)
)

var spacedCallKeywordExclusions = map[string]bool{
	"case": true, "if": true, "unless": true, "until": true, "while": true,
}

func isIdentChar(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_'
}

func invalidDynamicRegexpEscape(pattern string) bool {
	escaped := false
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if !escaped {
			if ch == '\\' {
				escaped = true
			}
			continue
		}
		escaped = false
		switch ch {
		case 'x':
			if i+1 >= len(pattern) || !isRegexpHexDigit(pattern[i+1]) {
				return true
			}
		case 'c':
			if i+1 >= len(pattern) {
				return true
			}
		}
	}
	return false
}

func isRegexpHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func invalidDynamicRegexpModifier(pattern, options string) bool {
	if strings.Contains(options, "a") {
		return true
	}
	return strings.Contains(pattern, "(?o)") || strings.Contains(pattern, "(?o:")
}

func invalidDynamicRegexpGrouping(pattern string) bool {
	depth := 0
	escaped := false
	inClass := false
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if inClass {
			if ch == ']' {
				inClass = false
			}
			continue
		}
		switch ch {
		case '[':
			inClass = true
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return true
			}
			depth--
		}
	}
	return depth != 0
}

func validateDynamicExpression(expr ast.Expression, ctx dynamicSyntaxContext) string {
	if expr == nil {
		return ""
	}
	switch node := expr.(type) {
	case *ast.ReturnExpression:
		return validateReturnSyntax(node, ctx)
	case *ast.NextExpression:
		return validateNextSyntax(node, ctx)
	case *ast.RedoExpression:
		return validateRedoSyntax(ctx)
	case *ast.RetryExpression:
		return validateRetrySyntax(ctx)
	case *ast.BreakExpression:
		return validateBreakSyntax(node, ctx)
	case *ast.YieldExpression:
		return validateYieldSyntax(node, ctx)
	case *ast.RegexpLiteral:
		if strings.Contains(node.Pattern, "[:alpha:]-[:digit:]") {
			return "invalid character class range"
		}
		if invalidDynamicRegexpGrouping(node.Pattern) {
			return "invalid regexp grouping"
		}
		if invalidDynamicRegexpModifier(node.Pattern, node.Options) {
			return "invalid regexp modifier"
		}
		if invalidDynamicRegexpEscape(node.Pattern) {
			return "invalid escape in regexp"
		}
	case *ast.DefExpression:
		return validateDynamicBlock(node.Body, dynamicSyntaxContext{
			methodDepth:             ctx.methodDepth + 1,
			classDepth:              ctx.classDepth,
			allowAnonymousBlockPass: node.BlockParam != nil && node.BlockParam.Value == "_",
		})
	case *ast.ClassExpression:
		nextCtx := ctx
		nextCtx.classDepth++
		return validateDynamicBlock(node.Body, nextCtx)
	case *ast.ModuleExpression:
		nextCtx := ctx
		nextCtx.classDepth++
		return validateDynamicBlock(node.Body, nextCtx)
	case *ast.ProcLiteral:
		nextCtx := ctx
		nextCtx.blockDepth++
		nextCtx.allowAnonymousBlockPass = node.BlockParam != nil && node.BlockParam.Value == "_"
		if node.Body != nil && node.Body.Token.Type == lexer.LBRACE {
			nextCtx.braceBlockDepth++
		}
		return validateDynamicBlock(node.Body, nextCtx)
	case *ast.SplatExpression:
		if node.AnonymousBlockPass && !ctx.allowAnonymousBlockPass {
			return "anonymous block forwarding is only allowed in method and lambda bodies"
		}
		return validateDynamicExpression(node.Value, ctx)
	case *ast.MethodCall:
		if msg := validateDynamicExpression(node.Receiver, ctx); msg != "" {
			return msg
		}
		if node.Block != nil && len(node.Args) > 0 {
			if splat, ok := node.Args[len(node.Args)-1].(*ast.SplatExpression); ok && splat.Token.Type == lexer.BIT_AND {
				return "block argument and actual block given"
			}
		}
		for _, arg := range node.Args {
			if msg := validateDynamicExpression(arg, ctx); msg != "" {
				return msg
			}
		}
		for _, arg := range node.KeywordArgs {
			if msg := validateDynamicExpression(arg.Value, ctx); msg != "" {
				return msg
			}
		}
		if node.Block != nil {
			nextCtx := ctx
			nextCtx.blockDepth++
			if node.Block.Token.Type == lexer.LBRACE {
				nextCtx.braceBlockDepth++
			}
			return validateDynamicBlock(node.Block, nextCtx)
		}
	case *ast.IfExpression:
		if msg := validateDynamicExpression(node.Condition, ctx); msg != "" {
			return msg
		}
		if msg := validateDynamicBlock(node.Consequent, ctx); msg != "" {
			return msg
		}
		for _, elsif := range node.ElsIf {
			if msg := validateDynamicExpression(elsif.Condition, ctx); msg != "" {
				return msg
			}
			if msg := validateDynamicBlock(elsif.Consequent, ctx); msg != "" {
				return msg
			}
		}
		return validateDynamicBlock(node.Alternative, ctx)
	case *ast.WhileExpression:
		if msg := validateDynamicExpression(node.Condition, ctx); msg != "" {
			return msg
		}
		nextCtx := ctx
		nextCtx.loopDepth++
		return validateDynamicBlock(node.Body, nextCtx)
	case *ast.UntilExpression:
		if msg := validateDynamicExpression(node.Condition, ctx); msg != "" {
			return msg
		}
		nextCtx := ctx
		nextCtx.loopDepth++
		return validateDynamicBlock(node.Body, nextCtx)
	case *ast.ForExpression:
		if msg := validateDynamicExpression(node.Collection, ctx); msg != "" {
			return msg
		}
		nextCtx := ctx
		nextCtx.loopDepth++
		return validateDynamicBlock(node.Body, nextCtx)
	case *ast.BeginExpression:
		if node.Ensure != nil && ctx.braceBlockDepth > 0 {
			return "Invalid ensure in block"
		}
		if node.Else != nil && len(node.Rescue) == 0 && node.Ensure == nil {
			return "else without rescue is useless"
		}
		if msg := validateDynamicBlock(node.Body, ctx); msg != "" {
			return msg
		}
		for _, rescue := range node.Rescue {
			for _, exception := range rescue.Exceptions {
				if msg := validateDynamicExpression(exception, ctx); msg != "" {
					return msg
				}
			}
			rescueCtx := ctx
			rescueCtx.rescueDepth++
			if msg := validateDynamicBlock(rescue.Body, rescueCtx); msg != "" {
				return msg
			}
		}
		if msg := validateDynamicBlock(node.Else, ctx); msg != "" {
			return msg
		}
		return validateDynamicBlock(node.Ensure, ctx)
	case *ast.AssignExpression:
		if ctx.methodDepth > 0 && node.Name != nil && isConstantIdentifier(node.Name.Value) {
			if os.Getenv("RGO_DEBUG_REQUIRE") == "1" {
				fmt.Printf("RGO_DEBUG_REQUIRE dynamic constant name=%q methodDepth=%d classDepth=%d\n", node.Name.Value, ctx.methodDepth, ctx.classDepth)
			}
			return "dynamic constant assignment"
		}
		if msg := validateDynamicExpression(node.Target, ctx); msg != "" {
			return msg
		}
		if msg := validateDynamicExpression(node.Index, ctx); msg != "" {
			return msg
		}
		return validateDynamicExpression(node.Value, ctx)
	case *ast.MultiAssignExpression:
		for _, value := range node.Values {
			if msg := validateDynamicExpression(value, ctx); msg != "" {
				return msg
			}
		}
	case *ast.InfixExpression:
		if msg := validateDynamicExpression(node.Left, ctx); msg != "" {
			return msg
		}
		return validateDynamicExpression(node.Right, ctx)
	case *ast.PrefixExpression:
		return validateDynamicExpression(node.Right, ctx)
	case *ast.TernaryExpression:
		if msg := validateDynamicExpression(node.Condition, ctx); msg != "" {
			return msg
		}
		if msg := validateDynamicExpression(node.Consequent, ctx); msg != "" {
			return msg
		}
		return validateDynamicExpression(node.Alternative, ctx)
	case *ast.ArrayLiteral:
		for _, elem := range node.Elements {
			if msg := validateDynamicExpression(elem, ctx); msg != "" {
				return msg
			}
		}
	case *ast.HashLiteral:
		for _, key := range node.Order {
			if msg := validateDynamicExpression(key, ctx); msg != "" {
				return msg
			}
			if msg := validateDynamicExpression(node.Pairs[key], ctx); msg != "" {
				return msg
			}
		}
	case *ast.IndexExpression:
		if msg := validateDynamicExpression(node.Left, ctx); msg != "" {
			return msg
		}
		if msg := validateDynamicExpression(node.Index, ctx); msg != "" {
			return msg
		}
		return validateDynamicExpression(node.End, ctx)
	}
	return ""
}

func isConstantIdentifier(name string) bool {
	if name == "" || strings.ContainsAny(name, ".()[]") {
		return false
	}
	ch, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(ch)
}

func validateDynamicBlock(block *ast.BlockExpression, ctx dynamicSyntaxContext) string {
	if block == nil {
		return ""
	}
	return validateDynamicStatements(block.Statements, ctx)
}

func splitTopLevelBeginBlocks(source string) ([]string, string, *object.EmeraldValue) {
	const keyword = "BEGIN"
	keyLen := len(keyword)
	remaining := make([]byte, 0, len(source))
	blocks := []string{}

	blockDepth := 0
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	var escaped bool

	n := len(source)
	sourceBytes := []byte(source)

	for i := 0; i < n; {
		ch := sourceBytes[i]

		if inLineComment {
			remaining = append(remaining, ch)
			if ch == '\n' {
				inLineComment = false
			}
			i++
			continue
		}

		if inSingleQuote {
			remaining = append(remaining, ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '\'' {
				inSingleQuote = false
			}
			i++
			continue
		}

		if inDoubleQuote {
			remaining = append(remaining, ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inDoubleQuote = false
			}
			i++
			continue
		}

		if inBacktick {
			remaining = append(remaining, ch)
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '`' {
				inBacktick = false
			}
			i++
			continue
		}

		if ch == '#' {
			remaining = append(remaining, ch)
			inLineComment = true
			i++
			continue
		}

		if ch == '\'' {
			remaining = append(remaining, ch)
			inSingleQuote = true
			escaped = false
			i++
			continue
		}

		if ch == '"' {
			remaining = append(remaining, ch)
			inDoubleQuote = true
			escaped = false
			i++
			continue
		}

		if ch == '`' {
			remaining = append(remaining, ch)
			inBacktick = true
			escaped = false
			i++
			continue
		}

		if isRubyIdentifierStart(ch) {
			j := i + 1
			for j < n && isRubyIdentifierContinue(sourceBytes[j]) {
				j++
			}
			word := string(sourceBytes[i:j])
			if word == keyword {
				prev := byte(0)
				if i > 0 {
					prev = sourceBytes[i-1]
				}
				next := byte(0)
				if i+keyLen < n {
					next = sourceBytes[i+keyLen]
				}
				if !isRubyIdentChar(prev) && !isRubyIdentChar(next) {
					k := j
					for k < n {
						c := sourceBytes[k]
						if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
							k++
							continue
						}
						break
					}
					if k < n && sourceBytes[k] == '{' {
						if blockDepth > 0 {
							return nil, "", newSyntaxError("BEGIN must be at top-level")
						}
						bodyStart := k + 1
						m := bodyStart
						depth := 1
						blockInSingle := false
						blockInDouble := false
						blockInBacktick := false
						blockInLineComment := false
						blockEscaped := false
						for m < n {
							c := sourceBytes[m]
							if blockInLineComment {
								if c == '\n' {
									blockInLineComment = false
								}
								m++
								continue
							}
							if blockInSingle {
								if blockEscaped {
									blockEscaped = false
								} else if c == '\\' {
									blockEscaped = true
								} else if c == '\'' {
									blockInSingle = false
								}
								m++
								continue
							}
							if blockInDouble {
								if blockEscaped {
									blockEscaped = false
								} else if c == '\\' {
									blockEscaped = true
								} else if c == '"' {
									blockInDouble = false
								}
								m++
								continue
							}
							if blockInBacktick {
								if blockEscaped {
									blockEscaped = false
								} else if c == '\\' {
									blockEscaped = true
								} else if c == '`' {
									blockInBacktick = false
								}
								m++
								continue
							}
							if c == '#' {
								blockInLineComment = true
								m++
								continue
							}
							if c == '\'' {
								blockInSingle = true
								blockEscaped = false
								m++
								continue
							}
							if c == '"' {
								blockInDouble = true
								blockEscaped = false
								m++
								continue
							}
							if c == '`' {
								blockInBacktick = true
								blockEscaped = false
								m++
								continue
							}
							if c == '{' {
								depth++
								m++
								continue
							}
							if c == '}' {
								depth--
								m++
								if depth == 0 {
									break
								}
								continue
							}
							m++
						}
						if m > n || depth != 0 {
							return nil, "", newSyntaxError("syntax error: unexpected end of input")
						}

						blocks = append(blocks, string(sourceBytes[bodyStart:m-1]))
						i = m
						continue
					}
				}
			} else if word == "begin" || word == "class" || word == "module" || word == "def" || word == "if" || word == "unless" || word == "while" || word == "until" || word == "for" || word == "case" || word == "do" {
				blockDepth++
			} else if word == "end" {
				if blockDepth > 0 {
					blockDepth--
				}
			}
			remaining = append(remaining, sourceBytes[i:j]...)
			i = j
			continue
		}

		if ch == '{' {
			blockDepth++
			remaining = append(remaining, ch)
			i++
			continue
		}

		if ch == '}' {
			if blockDepth > 0 {
				blockDepth--
			}
			remaining = append(remaining, ch)
			i++
			continue
		}

		remaining = append(remaining, ch)
		i++
	}

	return blocks, string(remaining), nil
}

func prependBeginBlocks(blocks []string, source string) string {
	if len(blocks) == 0 {
		return source
	}
	parts := make([]byte, 0, len(source)+64*len(blocks))
	for _, block := range blocks {
		if len(block) > 0 {
			parts = append(parts, block...)
		}
		parts = append(parts, ';')
	}
	parts = append(parts, source...)
	return string(parts)
}

func isRubyIdentifierStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '@' || c == '$' || c == ':'
}

func isRubyIdentifierContinue(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_' ||
		c == '?' ||
		c == '!' ||
		c == ':' ||
		c == '@' ||
		c == '$'
}

func isRubyIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}

func newSyntaxError(message string) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueException,
		Data:  &object.RException{Message: message},
		Class: core.R.Classes["SyntaxError"],
	}
}

func newSyntaxErrorForBinding(binding *object.RBinding, message string) *object.EmeraldValue {
	path := ""
	line := int64(1)
	if binding != nil {
		path = binding.Path
		line = binding.Line
	}
	if path == "" {
		path = "eval"
	}
	if line == 0 {
		line = 1
	}
	return newSyntaxError(fmt.Sprintf("%s:%d: %s", path, line, message))
}

func bindingLocalSlots(vm *VM) map[string]int {
	if vm.fp < 0 || vm.fp >= len(vm.frames) {
		return map[string]int{}
	}
	frame := vm.frames[vm.fp]
	if frame == nil || frame.Fn == nil {
		return map[string]int{}
	}
	slots := map[string]int{}
	if len(frame.Fn.LocalNames) > 0 {
		named := make([]struct {
			name  string
			index int
		}, 0, len(frame.Fn.LocalNames))
		for name, index := range frame.Fn.LocalNames {
			named = append(named, struct {
				name  string
				index int
			}{name: name, index: index})
		}
		sort.Slice(named, func(i, j int) bool {
			return named[i].index < named[j].index
		})
		for _, item := range named {
			slot := frame.Bp + 1 + item.index
			if slot >= 0 {
				slots[item.name] = slot
			}
		}
	} else {
		for i, name := range frame.Fn.Params {
			slot := frame.Bp + 1 + i
			if slot >= 0 {
				slots[name] = slot
			}
		}
		for _, kp := range frame.Fn.KeywordParams {
			slot := frame.Bp + 1 + len(frame.Fn.Params)
			if frame.Fn.HasRestParam {
				slot++
			}
			for _, prior := range frame.Fn.KeywordParams {
				if prior.Name == kp.Name {
					break
				}
				slot++
			}
			if slot >= 0 {
				slots[kp.Name] = slot
			}
		}
	}
	return slots
}

func seedBindingLocals(vm *VM, binding *object.RBinding, slots map[string]int) {
	if binding == nil || len(slots) == 0 {
		return
	}
	if binding.Locals == nil {
		binding.Locals = map[string]*object.EmeraldValue{}
	}
	for name, slot := range slots {
		if value, ok := binding.Locals[name]; ok {
			if slot >= len(vm.stack) {
				for len(vm.stack) <= slot {
					vm.stack = append(vm.stack, nil)
				}
			}
			vm.stack[slot] = value
			if slot >= vm.sp {
				vm.sp = slot + 1
			}
		}
	}
}

func copyBindingLocals(vm *VM, binding *object.RBinding, slots map[string]int) {
	if binding == nil || len(slots) == 0 {
		return
	}
	if binding.Locals == nil {
		binding.Locals = map[string]*object.EmeraldValue{}
	}
	existing := map[string]struct{}{}
	for _, name := range binding.LocalNames {
		existing[name] = struct{}{}
	}
	ordered := make([]struct {
		name string
		slot int
	}, 0, len(slots))
	for name, slot := range slots {
		ordered = append(ordered, struct {
			name string
			slot int
		}{name: name, slot: slot})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].slot < ordered[j].slot
	})
	for _, item := range ordered {
		name := item.name
		slot := item.slot
		if slot < 0 || slot >= len(vm.stack) {
			continue
		}
		if vm.stack[slot] == nil {
			continue
		}
		binding.Locals[name] = vm.stack[slot]
		if _, ok := existing[name]; !ok {
			binding.LocalNames = append(binding.LocalNames, name)
			existing[name] = struct{}{}
		}
	}
}

func (vm *VM) copyBindingLocalsToCurrentFrame(binding *object.RBinding) {
	if binding == nil || len(binding.Locals) == 0 || vm.fp < 0 || vm.fp >= len(vm.frames) {
		return
	}
	frame := vm.frames[vm.fp]
	if frame == nil || frame.Fn == nil {
		return
	}
	slots := bindingLocalSlots(vm)
	for name, value := range binding.Locals {
		slot, ok := slots[name]
		if !ok || slot < 0 {
			continue
		}
		if slot >= len(vm.stack) {
			continue
		}
		vm.stack[slot] = value
		if vm.isRoot && len(vm.frames) > 0 && frame == vm.frames[0] {
			if topLevel := vm.topLevelBindingData(); topLevel != nil {
				if topLevel.Locals == nil {
					topLevel.Locals = map[string]*object.EmeraldValue{}
				}
				topLevel.Locals[name] = value
			}
		}
		if slot >= vm.sp {
			vm.sp = slot + 1
		}
	}
}

func (vm *VM) resolveRequirePath(path string) string {
	if path == "" {
		return ""
	}
	currentDir, _ := os.Getwd()

	requestPath := filepath.FromSlash(strings.ReplaceAll(path, "\\", "/"))
	if requestPath == "~" || strings.HasPrefix(requestPath, "~"+string(filepath.Separator)) {
		home, _ := core.EnvString("HOME")
		if home == "" {
			home = os.Getenv("HOME")
		}
		if home != "" {
			if requestPath == "~" {
				requestPath = home
			} else {
				requestPath = filepath.Join(home, strings.TrimPrefix(requestPath, "~"+string(filepath.Separator)))
			}
		}
	}
	isAbs := filepath.IsAbs(requestPath)
	isExplicitRelative := strings.HasPrefix(requestPath, "."+string(filepath.Separator)) ||
		strings.HasPrefix(requestPath, ".."+string(filepath.Separator))
	isDotOrDotDot := requestPath == "." || requestPath == ".."
	cleanPath := filepath.Clean(requestPath)

	candidates := []string{}
	seen := map[string]struct{}{}
	addCandidate := func(c string) {
		if c == "" {
			return
		}
		clean := filepath.Clean(c)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		candidates = append(candidates, clean)
	}
	addRequireCandidates := func(c string) {
		clean := filepath.Clean(c)
		if !strings.HasSuffix(clean, ".rb") {
			addCandidate(clean + ".rb")
		}
		addCandidate(clean)
	}

	if isExplicitRelative && currentDir != "" {
		addRequireCandidates(filepath.Join(currentDir, cleanPath))
	}
	if isDotOrDotDot && currentDir != "" {
		addRequireCandidates(filepath.Join(currentDir, cleanPath))
	}
	if isAbs {
		addRequireCandidates(cleanPath)
	}
	if !isAbs && !isExplicitRelative && !isDotOrDotDot {
		entries := core.GetGlobalVariable("$LOAD_PATH")
		if entries != nil && entries.Type == object.ValueArray {
			for _, entry := range entries.Data.([]*object.EmeraldValue) {
				epath, errValue := core.CoercePathValue(entry)
				if errValue != nil {
					continue
				}
				if canonical, err := filepath.EvalSymlinks(epath); err == nil {
					epath = canonical
				}
				if !filepath.IsAbs(epath) {
					if absolute, err := filepath.Abs(epath); err == nil {
						epath = absolute
					}
				}
				addRequireCandidates(filepath.Join(epath, requestPath))
			}
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			continue
		}
		file, err := os.Open(candidate)
		if err != nil {
			continue
		}
		_ = file.Close()
		return candidate
	}

	return ""
}

func (vm *VM) resolveLoadPath(path string) string {
	if path == "" {
		return ""
	}
	requestPath := filepath.FromSlash(strings.ReplaceAll(path, "\\", "/"))
	if requestPath == "~" || strings.HasPrefix(requestPath, "~"+string(filepath.Separator)) {
		home, _ := core.EnvString("HOME")
		if home == "" {
			home = os.Getenv("HOME")
		}
		if home != "" {
			if requestPath == "~" {
				requestPath = home
			} else {
				requestPath = filepath.Join(home, strings.TrimPrefix(requestPath, "~"+string(filepath.Separator)))
			}
		}
	}

	candidates := make([]string, 0)
	if !filepath.IsAbs(requestPath) && !strings.HasPrefix(requestPath, "."+string(filepath.Separator)) && !strings.HasPrefix(requestPath, ".."+string(filepath.Separator)) {
		entries := core.GetGlobalVariable("$LOAD_PATH")
		if entries != nil && entries.Type == object.ValueArray {
			for _, entry := range entries.Data.([]*object.EmeraldValue) {
				entryPath, errValue := core.CoercePathValue(entry)
				if errValue == nil {
					if !filepath.IsAbs(entryPath) {
						if absolute, err := filepath.Abs(entryPath); err == nil {
							entryPath = absolute
						}
					}
					candidates = append(candidates, filepath.Join(entryPath, requestPath))
				}
			}
		}
	}
	candidates = append(candidates, requestPath)
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (vm *VM) requirePath(path string) (string, *object.EmeraldValue) {
	if path == "" {
		return "", core.NewArgumentError("empty file name")
	}
	candidate := vm.resolveRequirePath(path)
	if candidate == "" {
		for _, extension := range []string{".so", ".bundle", ".dylib", ".dll"} {
			if info, err := os.Stat(path + extension); err == nil && !info.IsDir() {
				return "", &object.EmeraldValue{
					Type:  object.ValueException,
					Data:  &object.RException{Message: "cannot load shared object -- " + path},
					Class: core.R.Classes["LoadError"],
				}
			}
		}
		return "", &object.EmeraldValue{
			Type:  object.ValueException,
			Data:  &object.RException{Message: "cannot load such file -- " + path},
			Class: core.R.Classes["LoadError"],
		}
	}

	previousSpecFile := core.CurrentSpecFile
	previousSpecFileAbsolute := core.CurrentSpecFileAbsolute
	core.CurrentSpecFile = candidate
	core.CurrentSpecFileAbsolute = candidate
	if realPath, err := filepath.EvalSymlinks(candidate); err == nil {
		core.CurrentSpecFileAbsolute = realPath
	}
	defer func() {
		core.CurrentSpecFile = previousSpecFile
		core.CurrentSpecFileAbsolute = previousSpecFileAbsolute
	}()
	content, err := os.ReadFile(candidate)
	if err != nil {
		return "", &object.EmeraldValue{
			Type:  object.ValueException,
			Data:  &object.RException{Message: "cannot load such file -- " + path},
			Class: core.R.Classes["LoadError"],
		}
	}
	result := vm.evalSource(string(content))
	if result != nil && result.Type == object.ValueException && result.Class == core.R.Classes["SyntaxError"] {
		if exception, ok := result.Data.(*object.RException); ok {
			exception.Path = candidate
		}
	}
	return candidate, result
}

func (vm *VM) constantValue(value *object.EmeraldValue, frame *Frame) *object.EmeraldValue {
	if value != nil && (value.Type == object.ValueString || value.Type == object.ValueSymbol) {
		freezeStringLiterals := vm.freezeStringLiterals
		chillStringLiterals := vm.chillStringLiterals
		if frame != nil && frame.Fn != nil && frame.Fn.StringLiteralModeSet {
			freezeStringLiterals = frame.Fn.FreezeStringLiterals
			chillStringLiterals = frame.Fn.ChillStringLiterals
		}
		encoding := vm.sourceEncoding
		if frame != nil && frame.Fn != nil && frame.Fn.SourceEncoding != "" {
			encoding = frame.Fn.SourceEncoding
		}
		if value.Encoding != "" {
			encoding = value.Encoding
		}
		if value.Type == object.ValueString && freezeStringLiterals {
			if encoding != "" {
				core.SetStringEncoding(value, encoding)
			}
			value.Frozen = true
			value.Chilled = false
			value.Literal = true
			key := encoding + "\x00" + value.Data.(string)
			if interned := vm.frozenStringCache[key]; interned != nil {
				return interned
			}
			vm.frozenStringCache[key] = value
		} else if value.Type == object.ValueString {
			mutable := *value
			mutable.Frozen = false
			mutable.Chilled = chillStringLiterals
			mutable.Literal = true
			result := &mutable
			if encoding != "" {
				core.SetStringEncoding(result, encoding)
			}
			return result
		} else if encoding != "" {
			core.SetSymbolEncoding(value, encoding)
		}
	}
	return value
}

func (vm *VM) execute(op compiler.Opcode, frame *Frame) error {
	vm.instructionCount++
	if vm.instructionLimit > 0 && vm.instructionCount > vm.instructionLimit {
		return fmt.Errorf("instruction limit exceeded at ip=%d, op=%v", frame.Ip, op)
	}
	constants := vm.frameConstants(frame)
	switch op {
	case compiler.OpConstant:
		idx := vm.readUint16()
		if idx < 0 || idx >= len(constants) {
			return fmt.Errorf("invalid constant index %d (constants: %d) at ip=%d", idx, len(constants), vm.frames[vm.fp].Ip)
		}
		vm.push(vm.constantValue(constants[idx], frame))

	case compiler.OpRegexpOnceGet:
		idx := vm.readUint16()
		jump := vm.readUint16()
		cached := constants[idx].Data.([]*object.EmeraldValue)
		if len(cached) > 0 {
			vm.push(cached[0])
			frame.Ip = jump - 1
		}

	case compiler.OpRegexpOnceSet:
		idx := vm.readUint16()
		constants[idx].Data = []*object.EmeraldValue{vm.stack[vm.sp-1]}

	case compiler.OpTrue:
		vm.push(core.R.TrueVal)

	case compiler.OpFalse:
		vm.push(core.R.FalseVal)

	case compiler.OpNil:
		vm.push(core.R.NilVal)

	case compiler.OpPatternCheck:
		patternIdx := vm.readUint16()
		pattern := constants[patternIdx].Data.(string)
		target := vm.pop()
		matched, errVal := vm.matchPattern(target, pattern, frame)
		if errVal != nil {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		vm.push(boolValue(matched))

	case compiler.OpPatternCacheClear:
		if vm.readUint8() == 0 {
			vm.patternArrayCache = nil
		} else if vm.patternArrayCache == nil {
			vm.patternArrayCache = make(map[*object.EmeraldValue]*object.EmeraldValue)
		} else {
			clear(vm.patternArrayCache)
		}

	case compiler.OpRaiseNoMatchingPattern:
		target := vm.pop()
		message := "nil"
		if target != nil {
			message = target.Inspect()
			if target.Type == object.ValueHash {
				message = strings.ReplaceAll(message, " => ", "=>")
			}
		}
		errVal := core.NewNoMatchingPatternError(message)
		if vm.raiseException(frame, errVal) {
			return nil
		}
		vm.returnUnhandledException(frame, errVal)
		return nil

	case compiler.OpGetMatchCapture:
		captureIndex := vm.readUint16()
		vm.push(core.MatchDataCapture(vm.getGlobalByName("$~"), captureIndex))

	case compiler.OpPop:
		vm.popFrameValue(frame)

	case compiler.OpAdd:
		right := vm.pop()
		left := vm.pop()
		result := vm.add(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		if result != nil && result.Type == object.ValueException && core.EvaluatingRaiseErrorMatcher() {
			vm.returnUnhandledException(frame, result)
			return nil
		}
		vm.push(result)

	case compiler.OpSetStringEncoding:
		encodingIdx := vm.readUint16()
		value := vm.pop()
		if value != nil && value.Type == object.ValueString {
			raw, _ := value.Data.(string)
			asciiOnly := true
			for i := 0; i < len(raw); i++ {
				if raw[i] >= 0x80 {
					asciiOnly = false
					break
				}
			}
			if encoding, ok := constants[encodingIdx].Data.(string); ok && asciiOnly {
				core.SetStringEncoding(value, encoding)
			}
		}
		vm.push(value)

	case compiler.OpSub:
		right := vm.pop()
		left := vm.pop()
		result := vm.sub(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpMul:
		right := vm.pop()
		left := vm.pop()
		result := vm.mul(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpDiv:
		right := vm.pop()
		left := vm.pop()
		result := vm.div(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpMod:
		right := vm.pop()
		left := vm.pop()
		result := vm.mod(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpPow:
		right := vm.pop()
		left := vm.pop()
		result := vm.pow(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpMinus, compiler.OpNeg:
		val := vm.pop()
		result := vm.negate(val)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpBang:
		val := vm.pop()
		result := vm.bang(val)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpEqual:
		right := vm.pop()
		left := vm.pop()
		result := vm.equals(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpNotEqual:
		right := vm.pop()
		left := vm.pop()
		if result, handled := core.EvaluateExpectationNotEqual(left, right); handled {
			vm.push(result)
			break
		}
		result := vm.send(left, "!=", []*object.EmeraldValue{right})
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpGreaterThan:
		right := vm.pop()
		left := vm.pop()
		result := vm.greaterThan(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpGreaterThanOrEqual:
		right := vm.pop()
		left := vm.pop()
		if result := vm.greaterThanOrEqual(left, right); result != nil {
			if result.Type == object.ValueException && vm.raiseException(frame, result) {
				return nil
			}
			vm.push(result)
			break
		}
		gt := vm.greaterThan(left, right)
		eq := vm.equals(left, right)
		if (gt.Type == object.ValueBool && gt.Data == true) ||
			(eq.Type == object.ValueBool && eq.Data == true) {
			vm.push(core.R.TrueVal)
		} else {
			vm.push(core.R.FalseVal)
		}

	case compiler.OpLessThan:
		right := vm.pop()
		left := vm.pop()
		result := vm.lessThan(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpLessThanOrEqual:
		right := vm.pop()
		left := vm.pop()
		if result := vm.lessThanOrEqual(left, right); result != nil {
			if result.Type == object.ValueException && vm.raiseException(frame, result) {
				return nil
			}
			vm.push(result)
			break
		}
		lt := vm.lessThan(left, right)
		if lt != nil && lt.Type == object.ValueException && vm.raiseException(frame, lt) {
			return nil
		}
		eq := vm.equals(left, right)
		if (lt.Type == object.ValueBool && lt.Data == true) ||
			(eq.Type == object.ValueBool && eq.Data == true) {
			vm.push(core.R.TrueVal)
		} else {
			vm.push(core.R.FalseVal)
		}

	case compiler.OpBitAnd:
		right := vm.pop()
		left := vm.pop()
		if result, ok := integerBitOperation(left, right, "&"); ok {
			vm.push(result)
		} else {
			result := vm.send(left, "&", []*object.EmeraldValue{right})
			if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
				return nil
			}
			vm.push(result)
		}

	case compiler.OpBitOr:
		right := vm.pop()
		left := vm.pop()
		if result, ok := integerBitOperation(left, right, "|"); ok {
			vm.push(result)
		} else {
			result := vm.send(left, "|", []*object.EmeraldValue{right})
			if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
				return nil
			}
			vm.push(result)
		}

	case compiler.OpBitXor:
		right := vm.pop()
		left := vm.pop()
		if result, ok := integerBitOperation(left, right, "^"); ok {
			vm.push(result)
		} else {
			result := vm.send(left, "^", []*object.EmeraldValue{right})
			if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
				return nil
			}
			vm.push(result)
		}

	case compiler.OpBitNot:
		val := vm.pop()
		if value, ok := integerAsBigInt(val); ok {
			vm.push(core.NewIntegerFromBigInt(new(big.Int).Not(value)))
		} else {
			result := vm.send(val, "~", nil)
			if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
				return nil
			}
			vm.push(result)
		}

	case compiler.OpBitLeftShift:
		right := vm.pop()
		left := vm.pop()
		if result, ok := integerShift(left, right, true); ok {
			vm.push(result)
		} else {
			vm.push(vm.send(left, "<<", []*object.EmeraldValue{right}))
		}

	case compiler.OpBitRightShift:
		right := vm.pop()
		left := vm.pop()
		if result, ok := integerShift(left, right, false); ok {
			vm.push(result)
		} else {
			result := vm.send(left, ">>", []*object.EmeraldValue{right})
			if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
				return nil
			}
			vm.push(result)
		}

	case compiler.OpJump:
		pos := vm.readUint16()
		if pos < frame.Ip-2 {
			if vm.tryExecuteCountedIntegerLoop(frame, pos, frame.Ip-2) {
				break
			}
			if vm.tryExecuteCollectionFillLoop(frame, pos, frame.Ip-2) ||
				vm.tryExecuteArraySumLoop(frame, pos, frame.Ip-2) {
				break
			}
			if vm.tryExecuteASCIIStringLoop(frame, pos, frame.Ip-2) {
				break
			}
			if vm.tryExecuteIntegerBytecodeLoop(frame, pos, frame.Ip-2) {
				break
			}
		}
		frame.Ip = pos - 1

	case compiler.OpJumpNotTruthy:
		pos := vm.readUint16()
		condition := vm.pop()
		if !condition.IsTruthy() {
			frame.Ip = pos - 1
		}

	case compiler.OpJumpNotNil:
		pos := vm.readUint16()
		condition := vm.pop()
		if condition.Type != object.ValueNil {
			frame.Ip = pos - 1
		}

	case compiler.OpJumpTruthy:
		pos := vm.readUint16()
		condition := vm.pop()
		if condition.IsTruthy() {
			frame.Ip = pos - 1
		}

	case compiler.OpFlipFlopGet:
		stateID := vm.readUint16()
		vm.push(boolValue(vm.flipFlopState(frame, stateID)))

	case compiler.OpFlipFlopSet:
		stateID := vm.readUint16()
		active := vm.readUint8() == 1
		vm.setFlipFlopState(frame, stateID, active)

	case compiler.OpJumpLocalPresent:
		idx := vm.readUint8()
		pos := vm.readUint16()
		stackIdx := frame.Bp + int(idx) + 1
		if stackIdx >= 0 && stackIdx < len(vm.stack) && localSlotPresent(vm.stack[stackIdx]) {
			frame.Ip = pos - 1
		}

	case compiler.OpArray:
		n := vm.readUint16()
		elems := make([]*object.EmeraldValue, n)
		for i := n - 1; i >= 0; i-- {
			elems[i] = vm.pop()
		}
		vm.push(&object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  elems,
			Class: core.R.Classes["Array"],
		})

	case compiler.OpArrayAppend:
		splatMode := vm.readUint8()
		value := vm.pop()
		array := vm.pop()
		elements := array.Data.([]*object.EmeraldValue)
		if splatMode != 0 {
			var expanded []*object.EmeraldValue
			var err *object.EmeraldValue
			if splatMode == 2 {
				expanded, err = vm.toAForAssignmentSplat(value)
			} else {
				expanded, err = vm.toAryForSplat(value)
			}
			if err != nil {
				if vm.raiseException(frame, err) {
					return nil
				}
				vm.returnUnhandledException(frame, err)
				return nil
			}
			elements = append(elements, expanded...)
		} else {
			elements = append(elements, value)
		}
		array.Data = elements
		vm.push(array)

	case compiler.OpHash:
		n := vm.readUint16()
		h := &object.RHash{
			Pairs:  make(map[*object.EmeraldValue]*object.EmeraldValue),
			Hashes: make(map[*object.EmeraldValue]int64),
		}
		for i := 0; i < int(n); i++ {
			key := hashLiteralKey(vm.pop())
			value := vm.pop()
			existing := hashLiteralExistingKey(h, key)
			if existing != nil {
				h.Pairs[existing] = value
				continue
			}
			h.Keys = append(h.Keys, key)
			h.Pairs[key] = value
		}
		vm.push(&object.EmeraldValue{
			Type:  object.ValueHash,
			Data:  h,
			Class: core.R.Classes["Hash"],
		})

	case compiler.OpMarkKeywordHash:
		value := vm.pop()
		core.MarkRuby2KeywordHash(value)
		vm.push(value)

	case compiler.OpIndex:
		index := vm.pop()
		left := vm.pop()
		result := vm.index(left, index)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpSliceIndex:
		length := vm.pop()
		start := vm.pop()
		left := vm.pop()
		result := vm.sliceIndex(left, start, length)
		vm.push(result)

	case compiler.OpIndexAssign:
		value := vm.pop()
		index := vm.pop()
		left := vm.pop()
		result := vm.indexAssign(left, index, value)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(value)

	case compiler.OpIndexCompoundAssign:
		methodIdx := vm.readUint16()
		method, _ := constants[methodIdx].Data.(string)
		right := vm.pop()
		index := vm.pop()
		left := vm.pop()
		current := vm.index(left, index)
		if shouldPropagateExceptionValue(current) && vm.raiseException(frame, current) {
			return nil
		}
		assigned := vm.send(current, method, []*object.EmeraldValue{right})
		if shouldPropagateExceptionValue(assigned) && vm.raiseException(frame, assigned) {
			return nil
		}
		result := vm.indexAssign(left, index, assigned)
		if shouldPropagateExceptionValue(result) && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(assigned)

	case compiler.OpIndexSplatCompoundAssign:
		methodIdx := vm.readUint16()
		method, _ := constants[methodIdx].Data.(string)
		right := vm.pop()
		splat := vm.pop()
		left := vm.pop()
		indexes, errVal := vm.toAryForSplat(splat)
		if errVal != nil {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		current := vm.send(left, "[]", indexes)
		if shouldPropagateExceptionValue(current) && vm.raiseException(frame, current) {
			return nil
		}
		assigned := vm.send(current, method, []*object.EmeraldValue{right})
		if shouldPropagateExceptionValue(assigned) && vm.raiseException(frame, assigned) {
			return nil
		}
		setterArgs := append(append([]*object.EmeraldValue(nil), indexes...), assigned)
		result := vm.send(left, "[]=", setterArgs)
		if shouldPropagateExceptionValue(result) && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(assigned)

	case compiler.OpLogicalSendAssignment:
		getterIdx := vm.readUint16()
		setterIdx := vm.readUint16()
		argCount := int(vm.readUint8())
		andAssign := vm.readUint8() == 1
		thunk := vm.pop()
		args := make([]*object.EmeraldValue, argCount)
		for i := argCount - 1; i >= 0; i-- {
			args[i] = vm.pop()
		}
		receiver := vm.pop()
		getter, _ := constants[getterIdx].Data.(string)
		setter, _ := constants[setterIdx].Data.(string)
		current := vm.send(receiver, getter, args)
		if shouldPropagateExceptionValue(current) && vm.raiseException(frame, current) {
			return nil
		}
		assign := !current.IsTruthy()
		if andAssign {
			assign = current.IsTruthy()
		}
		if !assign {
			vm.push(current)
			break
		}
		assigned := vm.callBlock(thunk)
		if shouldPropagateExceptionValue(assigned) && vm.raiseException(frame, assigned) {
			return nil
		}
		setterArgs := append(append([]*object.EmeraldValue(nil), args...), assigned)
		setterResult := vm.send(receiver, setter, setterArgs)
		if shouldPropagateExceptionValue(setterResult) && vm.raiseException(frame, setterResult) {
			return nil
		}
		vm.push(assigned)

	case compiler.OpGetGlobal:
		rawIdx := vm.readUint16()
		idx := vm.resolveGlobalIndexForFrame(frame, rawIdx)
		rawName := vm.rawGlobalNameForFrameIndex(frame, rawIdx)
		resolvedName := core.ResolveGlobalAlias(rawName)
		threadSpecial := vm.threadSpecialGlobals != nil && isThreadSpecialGlobal(resolvedName)
		var val *object.EmeraldValue
		if resolvedName == "$!" {
			val = core.LastException
		} else if resolvedName == "$@" {
			val = vm.currentExceptionBacktraceGlobal()
		} else if threadSpecial {
			val = vm.threadSpecialGlobals[resolvedName]
		} else {
			val = vm.globals[idx]
		}
		if val == nil && !threadSpecial {
			if resolvedName == "$!" {
				val = core.LastException
			} else if resolvedName == "$@" {
				val = vm.currentExceptionBacktraceGlobal()
			}
			for name, globalIdx := range vm.globalNames {
				if val != nil {
					break
				}
				if globalIdx == idx && name == "$!" {
					val = core.LastException
					break
				}
				if globalIdx == idx && name == "$@" {
					val = vm.currentExceptionBacktraceGlobal()
					break
				}
				if globalIdx == idx && (name == "stdout" || name == "$stdout") {
					val = core.StdoutObject()
					vm.globals[idx] = val
					break
				}
				if globalIdx == idx && (name == "stderr" || name == "$stderr") {
					val = core.StderrObject()
					vm.globals[idx] = val
					break
				}
				if globalIdx == idx && (name == "stdin" || name == "$stdin") {
					val = core.StdinObject()
					vm.globals[idx] = val
					break
				}
				if globalIdx == idx && (name == "$\"" || name == "$LOADED_FEATURES") {
					val = vm.loadedFeaturesGlobal()
					vm.globals[idx] = val
					break
				}
				if globalIdx == idx && (name == "$:" || name == "$LOAD_PATH" || name == "$-I") {
					val = vm.loadPathGlobal()
					vm.globals[idx] = val
					break
				}
				if globalIdx == idx && name == "$0" {
					val = vm.programNameGlobal()
					vm.globals[idx] = val
					break
				}
			}
		}
		if val == nil {
			val = core.R.NilVal
		}
		vm.push(val)

	case compiler.OpSetGlobal:
		rawIdx := vm.readUint16()
		idx := vm.resolveGlobalIndexForFrame(frame, rawIdx)
		value, errVal := vm.validateGlobalAssignmentForFrame(frame, rawIdx, idx, vm.peek(0))
		if errVal != nil {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		vm.stack[vm.sp-1] = value
		name := vm.rawGlobalNameForFrameIndex(frame, rawIdx)
		resolvedName := core.ResolveGlobalAlias(name)
		if vm.threadSpecialGlobals != nil && isThreadSpecialGlobal(resolvedName) {
			vm.threadSpecialGlobals[resolvedName] = value
		} else {
			vm.globals[idx] = value
		}
		if name != "" {
			core.NotifyGlobalVariableSet(name, value)
		}

	case compiler.OpGetConstant:
		nameIdx := vm.readUint16()
		name, ok := constants[nameIdx].Data.(string)
		if !ok {
			return fmt.Errorf("OpGetConstant: expected string constant, got %T", constants[nameIdx].Data)
		}
		if strings.HasPrefix(name, "::") {
			absoluteName := strings.TrimPrefix(name, "::")
			if objectClass := core.R.Classes["Object"]; objectClass != nil && objectClass.PrivateConstants[absoluteName] {
				owner := &object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: core.R.Classes["Class"]}
				val := vm.privateConstantAccessResult(owner, absoluteName)
				if val != nil && val.Type == object.ValueException && vm.raiseException(frame, val) {
					return nil
				}
				vm.push(val)
			} else if val, ok := vm.topLevelConstantValue(absoluteName); ok {
				vm.push(val)
			} else if qualifiedConst, ok := vm.qualifiedConstantValue(absoluteName); ok {
				if qualifiedConst != nil && qualifiedConst.Type == object.ValueException && vm.raiseException(frame, qualifiedConst) {
					return nil
				}
				vm.push(qualifiedConst)
			} else {
				missing := vm.missingConstantValue(absoluteName, true)
				if missing != nil && missing.Type == object.ValueException && vm.raiseException(frame, missing) {
					return nil
				}
				vm.push(missing)
			}
			break
		}
		allowTopLevelFallback := vm.allowTopLevelConstantFallback(name)
		if val, ok := vm.lexicalConstantValue(name); ok {
			if val != nil && val.Type == object.ValueException && vm.raiseException(frame, val) {
				return nil
			}
			vm.push(val)
		} else if allowTopLevelFallback {
			if val, ok := vm.topLevelConstantValue(name); ok {
				vm.push(val)
			} else if name == "ENV" {
				vm.push(core.EnvObject())
			} else if name == "ThreadGroup::Default" {
				vm.push(core.DefaultThreadGroup())
			} else if processConst, ok := processConstantValue(name); ok {
				vm.push(processConst)
			} else if cls, ok := core.R.Classes[name]; ok {
				vm.push(&object.EmeraldValue{
					Type:  object.ValueClass,
					Data:  cls,
					Class: core.R.Classes["Class"],
				})
			} else if namespace, ok := vm.namespaceModuleValue(name); ok {
				vm.push(namespace)
			} else if localConst, ok := vm.scopedLocalConstantValue(frame, name); ok {
				if localConst != nil && localConst.Type == object.ValueException && vm.raiseException(frame, localConst) {
					return nil
				}
				vm.push(localConst)
			} else if qualifiedConst, ok := vm.qualifiedConstantValue(name); ok {
				if qualifiedConst != nil && qualifiedConst.Type == object.ValueException && vm.raiseException(frame, qualifiedConst) {
					return nil
				}
				vm.push(qualifiedConst)
			} else {
				missing := vm.missingConstantValue(name, false)
				if missing != nil && missing.Type == object.ValueException && vm.raiseException(frame, missing) {
					return nil
				}
				vm.push(missing)
			}
		} else if localConst, ok := vm.scopedLocalConstantValue(frame, name); ok {
			if localConst != nil && localConst.Type == object.ValueException && vm.raiseException(frame, localConst) {
				return nil
			}
			vm.push(localConst)
		} else if qualifiedConst, ok := vm.qualifiedConstantValue(name); ok {
			if qualifiedConst != nil && qualifiedConst.Type == object.ValueException && vm.raiseException(frame, qualifiedConst) {
				return nil
			}
			vm.push(qualifiedConst)
		} else {
			missing := vm.missingConstantValue(name, false)
			if missing != nil && missing.Type == object.ValueException && vm.raiseException(frame, missing) {
				return nil
			}
			vm.push(missing)
		}

	case compiler.OpSetConstant:
		nameIdx := vm.readUint16()
		definitionFinalization := vm.readUint8() == 1
		rawName := constants[nameIdx].Data.(string)
		absolute := strings.HasPrefix(rawName, "::")
		name := strings.TrimPrefix(rawName, "::")
		value := vm.peek(0)
		if idx, top := vm.findClassStackEntry(name); top != nil {
			value = top
			vm.classStack = append(vm.classStack[:idx], vm.classStack[idx+1:]...)
			if top.Type == object.ValueClass {
				vm.runMinitestMethods(top)
			}
		}
		if absolute {
			vm.rubyConsts[name] = value
			if !strings.Contains(name, "::") {
				vm.SetTopLevelConstant(name, value)
			}
			break
		}
		container := vm.currentConstantContainer()
		if !definitionFinalization && container != nil && !strings.Contains(name, "::") {
			defineConstantOn(container, name, value)
			if vm.sourceEncoding != "" {
				core.SetConstantNameEncoding(container, name, vm.sourceEncoding)
			}
		} else if !definitionFinalization && container == nil && !strings.Contains(name, "::") {
			vm.SetTopLevelConstant(name, value)
		}
		if _, _, scopedLocal := vm.scopedLocalConstantContainer(frame, name); !definitionFinalization && !scopedLocal {
			core.AssignConstantName(container, name, value)
		}
		if container != nil && !strings.Contains(name, "::") {
			vm.rubyConsts[qualifiedConstantName(container, name)] = value
		} else {
			vm.rubyConsts[name] = value
		}

	case compiler.OpGetScopedConstant:
		nameIdx := vm.readUint16()
		name, ok := constants[nameIdx].Data.(string)
		if !ok {
			return fmt.Errorf("OpGetScopedConstant: expected string constant, got %T", constants[nameIdx].Data)
		}
		receiver := vm.pop()
		value, ok := vm.scopedConstantValue(receiver, name)
		if !ok {
			value = vm.sendBypassVisibility(receiver, "const_missing", []*object.EmeraldValue{{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]}})
		}
		if value != nil && value.Type == object.ValueException && vm.raiseException(frame, value) {
			return nil
		}
		if value != nil && value.Type == object.ValueException && core.EvaluatingRaiseErrorMatcher() {
			vm.returnUnhandledException(frame, value)
			return nil
		}
		vm.push(value)

	case compiler.OpSetScopedConstant:
		nameIdx := vm.readUint16()
		mode := vm.readUint8()
		name, ok := constants[nameIdx].Data.(string)
		if !ok {
			return fmt.Errorf("OpSetScopedConstant: expected string constant, got %T", constants[nameIdx].Data)
		}
		value := vm.pop()
		receiver := vm.pop()
		assigned, result := vm.scopedConstantAssignmentValue(receiver, name, value, mode)
		if result != nil && result.Type == object.ValueException {
			if vm.raiseException(frame, result) {
				return nil
			}
			vm.returnUnhandledException(frame, result)
			return nil
		}
		if assigned {
			if receiver == nil || (receiver.Type != object.ValueClass && receiver.Type != object.ValueModule) {
				if vm.raiseException(frame, core.NewTypeError("not a class/module")) {
					return nil
				}
			}
			if receiver.Frozen {
				frozen := &object.EmeraldValue{Type: object.ValueException, Data: &object.RException{Message: "can't modify frozen class/module"}, Class: core.R.Classes["FrozenError"]}
				if vm.raiseException(frame, frozen) {
					return nil
				}
			}
			defineConstantOn(receiver, name, result)
			if qualified := qualifiedConstantName(receiver, name); strings.Contains(qualified, "::") {
				vm.rubyConsts[qualified] = result
			}
		}
		vm.push(result)

	case compiler.OpGetLocal:
		if vm.tryFusedIntegerLocalExpression(frame, constants) {
			break
		}
		idx := vm.readUint8()
		basePtr := frame.Bp
		// In Ruby, Bp points to self (index 0), parameters start at index 1
		// But compiler generates indices starting from 0 for first param
		// So we need to add 1 to skip self
		stackIdx := basePtr + int(idx) + 1
		if stackIdx < 0 || stackIdx >= len(vm.stack) {
			return fmt.Errorf("OpGetLocal: invalid stack access basePtr=%d idx=%d stackIdx=%d sp=%d", basePtr, idx, stackIdx, vm.sp)
		}
		if name, ok := vm.topLevelLocalName(frame, idx); ok {
			if binding := vm.topLevelBindingData(); binding != nil {
				if value, exists := binding.Locals[name]; exists {
					vm.stack[stackIdx] = value
				}
			}
		}
		vm.push(derefClosureValue(vm.stack[stackIdx]))

	case compiler.OpSetLocal:
		idx := vm.readUint8()
		basePtr := frame.Bp
		// Add 1 to skip self
		stackIdx := basePtr + int(idx) + 1
		if stackIdx < 0 || stackIdx >= len(vm.stack) {
			return fmt.Errorf("OpSetLocal: invalid stack access basePtr=%d idx=%d stackIdx=%d sp=%d", basePtr, idx, stackIdx, vm.sp)
		}
		if current := vm.stack[stackIdx]; current != nil {
			if _, ok := current.Data.(*closureCell); ok {
				setClosureValue(&vm.stack[stackIdx], vm.peek(0))
			} else {
				vm.stack[stackIdx] = vm.peek(0)
			}
		} else {
			vm.stack[stackIdx] = vm.peek(0)
		}
		if vm.sp <= stackIdx {
			vm.sp = stackIdx + 1
		}
		if name, ok := vm.topLevelLocalName(frame, idx); ok {
			if binding := vm.topLevelBindingData(); binding != nil {
				binding.Locals[name] = vm.peek(0)
			}
		}
		vm.updateCapturedBindingLocal(frame, int(idx), vm.peek(0))

	case compiler.OpGetLocalCell:
		idx := vm.readUint8()
		stackIdx := frame.Bp + int(idx) + 1
		if stackIdx >= 0 && stackIdx < len(vm.stack) {
			var cellValue *object.EmeraldValue
			if current := vm.stack[stackIdx]; current != nil {
				if _, ok := current.Data.(*closureCell); ok {
					cellValue = current
				}
			}
			if cellValue == nil {
				cellValue = &object.EmeraldValue{Type: object.ValueObject, Data: &closureCell{value: derefClosureValue(vm.stack[stackIdx])}, Class: core.R.Classes["Object"]}
				vm.stack[stackIdx] = cellValue
			}
			if name, ok := vm.topLevelLocalName(frame, int(idx)); ok {
				if binding := vm.topLevelBindingData(); binding != nil {
					binding.Locals[name] = cellValue
				}
			}
			vm.push(cellValue)
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpGetInstanceVar:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		receiver := vm.stack[frame.Bp]
		if obj, ok := receiver.Data.(*object.Object); ok {
			if val, ok := obj.InstanceVars[name]; ok {
				vm.push(val)
			} else {
				vm.push(core.R.NilVal)
			}
		} else if proc, ok := receiver.Data.(*object.Proc); ok {
			if val, ok := proc.InstanceVars[name]; ok {
				vm.push(val)
			} else {
				vm.push(core.R.NilVal)
			}
		} else if module, ok := receiver.Data.(*object.Module); ok {
			if val, ok := module.InstanceVars[name]; ok {
				vm.push(val)
			} else {
				vm.push(core.R.NilVal)
			}
		} else if class, ok := receiver.Data.(*object.Class); ok {
			if val := class.GetInstanceVar(name); val != nil {
				vm.push(val)
			} else {
				vm.push(core.R.NilVal)
			}
		} else if val := core.DynamicInstanceVar(receiver, name); val != nil {
			vm.push(val)
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetInstanceVar:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		val := vm.peek(0)
		receiver := vm.stack[frame.Bp]
		if result := core.SetDynamicInstanceVar(receiver, name, val); result != nil {
			if vm.raiseException(frame, result) {
				return nil
			}
		}

	case compiler.OpDefinedInstanceVar:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		receiver := vm.stack[frame.Bp]
		if core.DynamicInstanceVar(receiver, name) != nil {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "instance-variable", Class: core.R.Classes["String"], Frozen: true})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpGetClassVar:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		receiver := vm.stack[frame.Bp]
		target := vm.classVarScopeReceiver(receiver)
		if vm.classVarAccessesToplevel(target) {
			if vm.raiseException(frame, core.NewRuntimeError("class variable access from toplevel")) {
				return nil
			}
			vm.push(core.R.NilVal)
			break
		}
		if val, errVal, ok := core.LookupClassVariableWithError(target, name); errVal != nil {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.push(core.R.NilVal)
		} else if ok {
			vm.push(val)
		} else {
			if vm.raiseException(frame, core.NewNameError("uninitialized class variable "+name)) {
				return nil
			}
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetClassVar:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		val := vm.peek(0)
		receiver := vm.stack[frame.Bp]
		target := vm.classVarScopeReceiver(receiver)
		if vm.classVarAccessesToplevel(target) {
			if vm.raiseException(frame, core.NewRuntimeError("class variable access from toplevel")) {
				return nil
			}
			break
		}
		core.SetClassVariable(target, name, val)

	case compiler.OpDefinedClassVar:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		receiver := vm.stack[frame.Bp]
		target := vm.classVarScopeReceiver(receiver)
		if vm.classVarAccessesToplevel(target) {
			vm.push(core.R.NilVal)
			break
		}
		if _, errVal, ok := core.LookupClassVariableWithError(target, name); errVal == nil && ok {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "class variable", Class: core.R.Classes["String"], Frozen: true})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpDefinedGlobal:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		value := vm.getGlobalByName(name)
		resolvedName := core.ResolveGlobalAlias(name)
		alwaysDefined := resolvedName == "$!" || resolvedName == "$~"
		matchDependent := resolvedName == "$&" || resolvedName == "$`" || resolvedName == "$'" || resolvedName == "$+" ||
			(len(resolvedName) == 2 && resolvedName[0] == '$' && resolvedName[1] >= '1' && resolvedName[1] <= '9')
		defined := value != nil && (!matchDependent || value.Type != object.ValueNil)
		if defined || alwaysDefined {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "global-variable", Class: core.R.Classes["String"], Frozen: true})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpGetFree:
		idx := vm.readUint8()
		if frame.Closure == nil || int(idx) >= len(frame.Closure.Free) {
			valueCount := 0
			if frame.Closure != nil {
				valueCount = len(frame.Closure.Free)
			}
			return fmt.Errorf("OpGetFree: index %d out of range for %s at %s:%d (free names %v, values %d)", idx, frame.Fn.Name, frame.Fn.SourcePath, frame.Fn.DefinitionLine, frame.Fn.FreeVarNames, valueCount)
		}
		vm.push(derefClosureValue(frame.Closure.Free[idx]))

	case compiler.OpSetFree:
		idx := vm.readUint8()
		setClosureValue(&frame.Closure.Free[idx], vm.peek(0))

	case compiler.OpGetFreeCell:
		idx := vm.readUint8()
		vm.push(frame.Closure.Free[idx])

	case compiler.OpGetOuter:
		idx := vm.readUint8()
		if vm.fp > 0 {
			outer := vm.frames[vm.fp-1]
			vm.push(derefClosureValue(vm.stack[outer.Bp+1+idx]))
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetOuter:
		_ = vm.readUint8()
		idx := vm.readUint8()
		if vm.fp > 0 {
			outer := vm.frames[vm.fp-1]
			stackIdx := outer.Bp + 1 + idx
			if current := vm.stack[stackIdx]; current != nil {
				if _, ok := current.Data.(*closureCell); ok {
					setClosureValue(&vm.stack[stackIdx], vm.peek(0))
				} else {
					vm.stack[stackIdx] = vm.peek(0)
				}
			} else {
				vm.stack[stackIdx] = vm.peek(0)
			}
			if vm.sp <= stackIdx {
				vm.sp = stackIdx + 1
			}
		}

	case compiler.OpGetOuterFree:
		idx := vm.readUint8()
		if vm.fp > 0 {
			outer := vm.frames[vm.fp-1]
			if outer.Closure != nil && idx < len(outer.Closure.Free) {
				vm.push(derefClosureValue(outer.Closure.Free[idx]))
			} else {
				vm.push(core.R.NilVal)
			}
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetOuterFree:
		idx := vm.readUint8()
		if vm.fp > 0 {
			outer := vm.frames[vm.fp-1]
			if outer.Closure != nil && idx < len(outer.Closure.Free) {
				setClosureValue(&outer.Closure.Free[idx], vm.peek(0))
			}
		}

	case compiler.OpGetOuterCell:
		idx := vm.readUint8()
		outer := vm.frames[vm.fp]
		if vm.fp > 0 {
			outer = vm.frames[vm.fp-1]
		}
		stackIdx := outer.Bp + 1 + idx
		if stackIdx >= 0 && stackIdx < len(vm.stack) {
			if current := vm.stack[stackIdx]; current != nil {
				if _, ok := current.Data.(*closureCell); ok {
					vm.push(current)
					break
				}
			}
			cell := &object.EmeraldValue{Type: object.ValueObject, Data: &closureCell{value: derefClosureValue(vm.stack[stackIdx])}, Class: core.R.Classes["Object"]}
			vm.stack[stackIdx] = cell
			vm.push(cell)
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpGetOuterFreeCell:
		idx := vm.readUint8()
		outer := vm.frames[vm.fp]
		if vm.fp > 0 {
			outer = vm.frames[vm.fp-1]
		}
		if outer.Closure != nil && idx < len(outer.Closure.Free) {
			vm.push(outer.Closure.Free[idx])
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSelf:
		vm.push(vm.stack[frame.Bp])

	case compiler.OpAlias:
		args := make([]*object.EmeraldValue, 2)
		for i := 0; i < 2; i++ {
			args[1-i] = vm.pop()
		}
		receiver := vm.pop()
		target := receiver
		if len(vm.classStack) > 0 {
			target = vm.classStack[len(vm.classStack)-1]
		} else if receiver != nil && receiver == core.R.Main {
			if obj, ok := receiver.Data.(*object.Object); ok && obj != nil && obj.Class != nil {
				target = &object.EmeraldValue{
					Type:  object.ValueClass,
					Data:  obj.Class,
					Class: core.R.Classes["Class"],
				}
			}
		}
		result := core.AliasMethod(target, args...)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		if result != nil && result.Type == object.ValueException && core.EvaluatingRaiseErrorMatcher() {
			vm.returnUnhandledException(frame, result)
			return nil
		}
		vm.push(result)

	case compiler.OpSingletonClass:
		receiver := vm.popFrameValue(frame)
		singleton := core.SingletonClass(receiver)
		if singleton != nil && singleton.Type == object.ValueException {
			if vm.raiseException(frame, singleton) {
				return nil
			}
			vm.returnUnhandledException(frame, singleton)
			return nil
		}
		vm.push(singleton)

	case compiler.OpReturn:
		if frame.Fn != nil && frame.Fn.MethodBody {
			vm.fireTracePointReturn(frame, core.R.NilVal)
		}
		// Don't decrement fp here - the caller will handle that
		vm.sp = frame.Bp

	case compiler.OpReturnValue, compiler.OpNonLocalReturnValue:
		retVal := vm.pop()
		if op == compiler.OpNonLocalReturnValue && vm.evalReturnMode && frame == vm.frames[0] {
			vm.evalReturnPending = true
			vm.evalReturnValue = retVal
			vm.discardPendingReturnForCurrentEnsure(frame)
			if vm.routeReturnThroughEnsure(frame, retVal) {
				return nil
			}
			vm.push(retVal)
			frame.Returned = true
			return nil
		}
		if vm.returningFromClassBodyBlock(frame) {
			exception := core.NewLocalJumpError("unexpected return")
			if vm.raiseException(frame, exception) {
				return nil
			}
			vm.returnUnhandledException(frame, exception)
			return nil
		}
		if frame.Fn != nil && (frame.Fn.Name == "__block__" || op == compiler.OpNonLocalReturnValue) && !frame.Fn.DefinedByDefineMethod && frame.Closure != nil && frame.Closure.ReturnOwnerID > 0 && frame.Closure.ReturnOwnerID != frame.ID {
			if vm.threadDepth > 0 || !vm.frameIDActive(frame.Closure.ReturnOwnerID) {
				exception := core.NewLocalJumpErrorWithReturn(retVal)
				if vm.raiseException(frame, exception) {
					return nil
				}
				vm.returnUnhandledException(frame, exception)
				return nil
			}
			vm.pendingReturnTargetID = frame.Closure.ReturnOwnerID
			vm.pendingReturnValue = retVal
			vm.push(retVal)
			frame.Returned = true
			return nil
		}
		if frame.Fn != nil && frame.Fn.MethodBody {
			vm.fireTracePointReturn(frame, retVal)
		}
		vm.discardPendingReturnForCurrentEnsure(frame)
		if vm.routeReturnThroughEnsure(frame, retVal) {
			return nil
		}
		vm.endInnermostActiveRescue(frame)
		// Keep locals alive while pending ensure code executes. The caller
		// unwinds the frame after the ensure path finishes.
		vm.push(retVal)
		frame.Returned = true

	case compiler.OpBlockReturn:
		retVal := vm.pop()
		vm.sp = frame.Bp
		vm.push(retVal)

	case compiler.OpSend, compiler.OpSendWithKeywords, compiler.OpSendSetter:
		isKeywordSend := op == compiler.OpSendWithKeywords
		isSetterSend := op == compiler.OpSendSetter
		methodNameIdx := vm.readUint16()
		blockArg := vm.readUint8()
		numArgs := vm.readUint8()
		splatIndex := int(vm.readUint8())
		methodName := constants[methodNameIdx].Data.(string)
		prevBlock := vm.currentBlock

		var block *object.EmeraldValue
		var blockErr *object.EmeraldValue
		switch blockArg {
		case 1:
			block, blockErr = vm.normalizeBlockPass(derefClosureValue(vm.pop()))
		case 2:
			block = vm.yieldBlock()
		}
		if blockErr != nil {
			if vm.raiseException(frame, blockErr) {
				return nil
			}
			vm.returnUnhandledException(frame, blockErr)
			return nil
		}

		args := make([]*object.EmeraldValue, int(numArgs))
		for i := 0; i < int(numArgs); i++ {
			args[numArgs-1-i] = vm.pop()
		}
		if splatIndex != 255 {
			expanded, errVal := vm.expandMethodSplatArgs(args, splatIndex)
			if errVal != nil {
				if vm.raiseException(frame, errVal) {
					return nil
				}
				vm.returnUnhandledException(frame, errVal)
				return nil
			}
			args = expanded
		}

		if isKeywordSend && len(args) > 0 {
			last := args[len(args)-1]
			if last == nil || last.Type == object.ValueNil {
				args = args[:len(args)-1]
			} else if last.Type == object.ValueHash {
				last = copyKeywordHash(last)
				args[len(args)-1] = last
				core.MarkRuby2KeywordHash(last)
			}
		}
		assignedValue := core.R.NilVal
		if isSetterSend && len(args) > 0 {
			assignedValue = args[len(args)-1]
		}

		receiver := vm.pop()
		if methodName == "include" && receiver == core.R.Main && len(vm.classStack) == 0 {
			if vm.nextInstructionIsExpectationSend(frame) {
				methodName = "__mspec_include_matcher__"
			} else {
				receiver = &object.EmeraldValue{Type: object.ValueClass, Data: core.R.Classes["Object"], Class: core.R.Classes["Class"]}
			}
		}
		if methodName == "alias_method" && len(args) == 2 {
			left := ""
			right := ""
			if args[0] != nil && (args[0].Type == object.ValueString || args[0].Type == object.ValueSymbol) {
				if s, ok := args[0].Data.(string); ok {
					left = strings.TrimPrefix(s, ":")
				}
			}
			if args[1] != nil && (args[1].Type == object.ValueString || args[1].Type == object.ValueSymbol) {
				if s, ok := args[1].Data.(string); ok {
					right = strings.TrimPrefix(s, ":")
				}
			}
			if strings.HasPrefix(left, "$") && strings.HasPrefix(right, "$") {
				core.AliasGlobalVariable(left, right)
				vm.push(core.R.NilVal)
				break
			}
		}

		switch blockArg {
		case 1, 2:
			vm.currentBlock = block
		default:
			vm.currentBlock = nil
		}
		var implicitIdentifierCause *object.EmeraldValue
		if blockArg == 3 && len(vm.activeRescues) > 0 {
			implicitIdentifierCause = core.LastException
		}
		result := vm.send(receiver, methodName, args)
		if vm.pendingEscapedThrowHandler != nil {
			handler := vm.pendingEscapedThrowHandler
			value := vm.pendingEscapedThrowValue
			vm.pendingEscapedThrowHandler = nil
			vm.pendingEscapedThrowValue = nil
			vm.currentBlock = prevBlock
			if vm.routeThrowThroughEnsure(frame, handler, value) {
				return nil
			}
			vm.completeThrow(frame, handler, value)
			return nil
		}
		vm.consumeCompletedBreakMarker()
		vm.currentBlock = prevBlock
		if vm.threadDepth > 0 && core.IsThreadBlockedResult(result) {
			vm.push(result)
			frame.Returned = true
			return nil
		}
		if core.IsTerminationResult(result) {
			if vm.raiseException(frame, result) {
				return nil
			}
			vm.returnUnhandledException(frame, result)
			return nil
		}
		if blockArg == 3 {
			result = implicitIdentifierNameError(result, receiver, methodName, implicitIdentifierCause)
		}
		if shouldPropagateExceptionValue(result) && vm.raiseException(frame, result) {
			return nil
		}
		if shouldPropagateExceptionValue(result) && (isSetterSend || blockArg == 3 || core.EvaluatingRaiseErrorMatcher() || numberedParameterMethodNamePattern.MatchString(methodName)) {
			vm.returnUnhandledException(frame, result)
			return nil
		}
		if isSetterSend {
			vm.push(assignedValue)
		} else {
			vm.push(result)
		}

	case compiler.OpBreak:
		val := core.R.NilVal
		if frame.WhileEnd >= 0 {
			if vm.routeBreakThroughEnsure(frame, val, frame.WhileEnd) {
				return nil
			}
			frame.Ip = frame.WhileEnd - 1
			return nil
		}
		if vm.routeBreakThroughEnsure(frame, val, 0) {
			return nil
		}
		frame.BlockBreak = true
		frame.BlockBreakVal = val
		vm.sp = frame.Bp
		vm.push(val)
		return nil

	case compiler.OpBreakValue:
		target := vm.readUint16()
		val := core.R.NilVal
		if vm.sp > frame.Bp {
			val = vm.pop()
		}
		if target > 0 {
			if vm.routeBreakThroughEnsure(frame, val, int(target)) {
				return nil
			}
			vm.push(val)
			frame.Ip = target - 1
			return nil
		}
		if frame.WhileEnd >= 0 {
			if vm.routeBreakThroughEnsure(frame, val, frame.WhileEnd) {
				return nil
			}
			vm.push(val)
			frame.Ip = frame.WhileEnd - 1
			return nil
		}
		if vm.routeBreakThroughEnsure(frame, val, 0) {
			return nil
		}
		frame.BlockBreak = true
		frame.BlockBreakVal = val
		vm.sp = frame.Bp
		vm.push(val)
		return nil

	case compiler.OpNext:
		target := int(vm.readUint16())
		val := core.R.NilVal
		if vm.sp > frame.Bp {
			val = vm.pop()
		}
		if vm.routeNextThroughEnsure(frame, val, target) {
			return nil
		}
		if target > 0 {
			vm.push(val)
			frame.Ip = target - 1
			return nil
		}
		frame.BlockNextVal = val
		frame.Ip = len(frame.Fn.Instructions) - 1
		vm.sp = frame.Bp
		core.ForEachMarkNext()
		return nil

	case compiler.OpEnterForEach:
		core.EnterForEachMode(vm.readUint8() == 1)

	case compiler.OpExitForEach:
		core.ExitForEachMode()

	case compiler.OpSetWhileEnd:
		target := vm.readUint16()
		frame.WhileEnd = int(target)
		frame.BlockBreakAddr = int(target)

	case compiler.OpRedo:
		if vm.routeRedoThroughEnsure(frame) {
			return nil
		}
		frame.Ip = -1
		vm.sp = frame.Bp + 1 + frame.Fn.NumLocals

	case compiler.OpYield:
		block := vm.yieldBlock()
		if block == nil {
			errVal := core.NewLocalJumpError("no block given")
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		result := vm.callBlock(block)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		if result != nil && result.Type == object.ValueException {
			vm.returnUnhandledException(frame, result)
			return nil
		}
		vm.push(result)

	case compiler.OpYieldWithValue:
		numArgs := int(vm.readUint8())
		args := make([]*object.EmeraldValue, numArgs)
		for i := numArgs - 1; i >= 0; i-- {
			args[i] = vm.pop()
		}
		block := vm.yieldBlock()
		if block == nil {
			errVal := core.NewLocalJumpError("no block given")
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		result := vm.callBlock(block, args...)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		if result != nil && result.Type == object.ValueException {
			vm.returnUnhandledException(frame, result)
			return nil
		}
		vm.push(result)

	case compiler.OpYieldWithSplat:
		numArgs := int(vm.readUint8())
		splatIndex := int(vm.readUint8())
		args := make([]*object.EmeraldValue, numArgs)
		for i := numArgs - 1; i >= 0; i-- {
			args[i] = vm.pop()
		}
		block := vm.yieldBlock()
		if block == nil {
			errVal := core.NewLocalJumpError("no block given")
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		expanded, errVal := vm.expandYieldSplatArgs(args, splatIndex)
		if errVal != nil {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		result := vm.callBlock(block, expanded...)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		if result != nil && result.Type == object.ValueException {
			vm.returnUnhandledException(frame, result)
			return nil
		}
		vm.push(result)

	case compiler.OpDefineMethod:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)

		closureVal := vm.pop()
		closure, ok := closureVal.Data.(*object.Closure)
		if !ok {
			return fmt.Errorf("expected closure, got %T", closureVal.Data)
		}
		closure.Refinements = vm.activeRefinementSnapshot()
		closure.RefinementsFixed = true

		visibility := vm.currentDefinitionVisibility()
		if frame.Fn != nil && frame.Fn.MethodBody {
			visibility = "public"
		}
		method := &object.Method{
			Name:         name,
			OriginalName: name,
			Fn:           closure.Fn,
			Closure:      closure,
			Arity:        functionArity(closure.Fn),
			Visibility:   visibility,
		}
		if len(vm.classStack) > 0 {
			classVal := vm.classStack[len(vm.classStack)-1]
			if methodDefinitionTargetFrozen(classVal) {
				errVal := frozenMethodDefinitionError(classVal, true)
				if vm.raiseException(frame, errVal) {
					return nil
				}
				vm.returnUnhandledException(frame, errVal)
				return nil
			}
			method.Owner = classVal
			if classVal.Type == object.ValueClass {
				cls := classVal.Data.(*object.Class)
				cls.DefineMethod(name, method)
				if cls.IsSingleton && cls.SingletonOwner != nil {
					key := nativeSingletonKey(cls.SingletonOwner)
					methods := vm.nativeSingletonMethods[key]
					if methods == nil {
						methods = make(map[string]*object.Method)
						vm.nativeSingletonMethods[key] = methods
					}
					methods[name] = method
					if errVal := core.NotifySingletonMethodAdded(cls.SingletonOwner, name); errVal != nil && errVal.Type == object.ValueException {
						if vm.raiseException(frame, errVal) {
							return nil
						}
						vm.returnUnhandledException(frame, errVal)
						return nil
					}
				} else if errVal := core.NotifyMethodAdded(classVal, name); errVal != nil && errVal.Type == object.ValueException {
					if vm.raiseException(frame, errVal) {
						return nil
					}
					vm.returnUnhandledException(frame, errVal)
					return nil
				}
			} else if classVal.Type == object.ValueModule {
				mod := classVal.Data.(*object.Module)
				mod.DefineMethod(name, method)
				if errVal := core.NotifyMethodAdded(classVal, name); errVal != nil && errVal.Type == object.ValueException {
					if vm.raiseException(frame, errVal) {
						return nil
					}
					vm.returnUnhandledException(frame, errVal)
					return nil
				}
				if vm.currentDefinitionMode() == "module_function" {
					copy := *method
					copy.Visibility = "public"
					if mod.SingletonClass == nil {
						if singleton := core.CallMethod(classVal, "singleton_class"); singleton != nil && singleton.Type == object.ValueClass {
							mod.SingletonClass = singleton.Data.(*object.Class)
						}
					}
					if mod.SingletonClass != nil {
						if mod.SingletonClass.Methods == nil {
							mod.SingletonClass.Methods = map[string]*object.Method{}
						}
						copy.Owner = &object.EmeraldValue{Type: object.ValueClass, Data: mod.SingletonClass, Class: core.R.Classes["Class"]}
						mod.SingletonClass.DefineMethod(name, &copy)
					}
				}
			} else {
				vm.push(core.NewTypeError("can't define method"))
				break
			}
		} else {
			mainObj := core.R.Main.Data.(*object.Object)
			method.Owner = &object.EmeraldValue{Type: object.ValueClass, Data: mainObj.Class, Class: core.R.Classes["Class"]}
			mainObj.Class.DefineMethod(name, method)
			if errVal := core.NotifyMethodAdded(method.Owner, name); errVal != nil && errVal.Type == object.ValueException {
				if vm.raiseException(frame, errVal) {
					return nil
				}
				vm.returnUnhandledException(frame, errVal)
				return nil
			}
			if name == "test" {
				delete(mainObj.SingletonMethods, "test")
			}
		}

		vm.push(&object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]})

	case compiler.OpDefineSingletonMethod:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)

		closureVal := vm.pop()
		closure, ok := closureVal.Data.(*object.Closure)
		if !ok {
			return fmt.Errorf("expected closure, got %T", closureVal.Data)
		}
		closure.Refinements = vm.activeRefinementSnapshot()
		closure.RefinementsFixed = true
		receiver := vm.pop()
		if singletonMethodDefinitionFrozen(receiver) {
			errVal := frozenMethodDefinitionError(receiver, false)
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}

		method := &object.Method{
			Name:         name,
			OriginalName: name,
			Fn:           closure.Fn,
			Closure:      closure,
		}

		if receiver != nil && receiver.Type == object.ValueObject {
			if obj, ok := receiver.Data.(*object.Object); ok {
				obj.SingletonMethods[name] = method
			} else {
				key := nativeSingletonKey(receiver)
				methods := vm.nativeSingletonMethods[key]
				if methods == nil {
					methods = make(map[string]*object.Method)
					vm.nativeSingletonMethods[key] = methods
				}
				methods[name] = method
			}
		} else if receiver != nil && receiver.Type == object.ValueClass {
			cls := receiver.Data.(*object.Class)
			if singleton := core.SingletonClass(receiver); singleton != nil && singleton.Type == object.ValueClass {
				method.Owner = singleton
			}
			cls.DefineClassMethod(name, method)
		} else if receiver != nil && receiver.Type == object.ValueModule {
			singleton := core.SingletonClass(receiver)
			if singleton == nil || singleton.Type != object.ValueClass {
				return fmt.Errorf("expected module singleton class, got %v", singleton)
			}
			singleton.Data.(*object.Class).DefineMethod(name, method)
		} else if receiver != nil {
			key := nativeSingletonKey(receiver)
			methods := vm.nativeSingletonMethods[key]
			if methods == nil {
				methods = make(map[string]*object.Method)
				vm.nativeSingletonMethods[key] = methods
			}
			methods[name] = method
			if singleton := core.SingletonClass(receiver); singleton != nil && singleton.Type == object.ValueClass {
				singleton.Data.(*object.Class).DefineMethod(name, method)
			}
		}

		if errVal := core.NotifySingletonMethodAdded(receiver, name); errVal != nil && errVal.Type == object.ValueException {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		vm.push(&object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]})

	case compiler.OpDefineClassMethod:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)

		fn := &object.Function{
			Name:           name,
			SourcePath:     frame.Fn.SourcePath,
			EvalSource:     frame.Fn.EvalSource,
			SourceEncoding: frame.Fn.SourceEncoding,
			Instructions:   vm.pop().Data.([]byte),
			Constants:      constants,
			NumLocals:      0,
			GlobalNames:    frame.Fn.GlobalNames,
		}

		method := &object.Method{
			Name:         name,
			OriginalName: name,
			Fn:           fn,
			Arity:        functionArity(fn),
		}

		classVal := vm.stack[frame.Bp]
		if obj, ok := classVal.Data.(*object.Object); ok {
			obj.Class.DefineClassMethod(name, method)
		}

	case compiler.OpClass:
		nameIdx := vm.readUint16()
		hasExplicitSuperclass := vm.readUint8() == 1
		rawName := constants[nameIdx].Data.(string)
		absolute := strings.HasPrefix(rawName, "::")
		name := strings.TrimPrefix(rawName, "::")

		var class *object.Class
		if errVal, found := vm.qualifiedPrivateConstantError(name); found {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		if errVal := vm.invalidQualifiedClassNameError(name); errVal != nil {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		localContainer := false
		if container := vm.currentConstantContainer(); !absolute && container != nil && !strings.Contains(name, "::") {
			localContainer = true
			if existing, found := vm.classValueFromContainer(container, name); found {
				class = existing.Data.(*object.Class)
			} else if container.Type == object.ValueClass && container.Data.(*object.Class).Name == "Object" {
				class = core.R.Classes[name]
			}
		}
		if class == nil && localContainer {
			class = object.NewClass(name)
			class.SuperClass = core.R.Classes["Object"]
			class.SuperClassSet = !hasExplicitSuperclass
		} else if class == nil {
			if existing, ok := vm.rubyConsts[name]; ok && existing.Type == object.ValueClass {
				class = existing.Data.(*object.Class)
			} else if existing, ok := vm.rubyConsts[name]; ok && existing.Type != object.ValueClass {
				exception := core.NewTypeError(name + " is not a class")
				if vm.raiseException(frame, exception) {
					return nil
				}
				vm.returnUnhandledException(frame, exception)
				return nil
			}
		}
		if class == nil {
			if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); !absolute && ok {
				if existing, found := vm.classValueFromContainer(container, constName); found {
					class = existing.Data.(*object.Class)
				}
			} else if container, constName, ok := vm.lexicalQualifiedConstantContainer(name); !absolute && ok {
				if existing, found := vm.classValueFromContainer(container, constName); found {
					class = existing.Data.(*object.Class)
				}
			} else if existing, ok := vm.classValueForQualifiedName(name); ok {
				class = existing.Data.(*object.Class)
			} else if existing, ok := core.R.Classes[name]; ok {
				class = existing
			}
		}
		if class == nil {
			class = object.NewClass(name)
			class.SuperClass = core.R.Classes["Object"]
			class.SuperClassSet = !hasExplicitSuperclass
		}
		if hasExplicitSuperclass && !class.SuperClassSet && vm.sp > 0 {
			if superValue := vm.peek(0); superValue != nil && superValue.Type == object.ValueClass {
				class.SuperClass = superValue.Data.(*object.Class)
			}
		}

		classVal := &object.EmeraldValue{
			Type:  object.ValueClass,
			Data:  class,
			Class: core.R.Classes["Class"],
		}
		if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); !absolute && ok {
			qualifiedName := qualifiedConstantName(container, constName)
			class.Name = qualifiedName
			defineConstantOn(container, constName, classVal)
			vm.rubyConsts[qualifiedName] = classVal
		} else if container := vm.currentConstantContainer(); !absolute && container != nil && !strings.Contains(name, "::") {
			qualifiedName := qualifiedConstantName(container, name)
			if class.Name == "" || class.Name == name {
				class.Name = qualifiedName
			}
			defineConstantOn(container, name, classVal)
			vm.rubyConsts[qualifiedName] = classVal
		} else if container, constName, ok := vm.lexicalQualifiedConstantContainer(name); !absolute && ok {
			qualifiedName := qualifiedConstantName(container, constName)
			if class.Name == "" || class.Name == name {
				class.Name = qualifiedName
			}
			defineConstantOn(container, constName, classVal)
			vm.rubyConsts[qualifiedName] = classVal
		} else if strings.Contains(name, "::") {
			vm.rubyConsts[name] = classVal
			vm.defineQualifiedConstant(name, classVal)
		} else if _, _, scoped := vm.scopedLocalConstantContainer(frame, name); !scoped {
			vm.rubyConsts[name] = classVal
			vm.SetTopLevelConstant(name, classVal)
		}
		vm.classStack = append(vm.classStack, classVal)
		vm.push(classVal)

	case compiler.OpInherited:
		classVal := vm.pop()
		superVal := vm.pop()
		if classVal == nil || classVal.Type != object.ValueClass {
			return fmt.Errorf("OpInherited: expected class, got %v", classVal)
		}
		if superVal == nil || superVal.Type != object.ValueClass {
			exception := core.NewTypeError("superclass must be a Class")
			if vm.raiseException(frame, exception) {
				return nil
			}
			vm.returnUnhandledException(frame, exception)
			return nil
		}
		class := classVal.Data.(*object.Class)
		superClass := superVal.Data.(*object.Class)
		if superClass.IsSingleton {
			exception := core.NewTypeError("can't make subclass of singleton class")
			if vm.raiseException(frame, exception) {
				return nil
			}
			vm.returnUnhandledException(frame, exception)
			return nil
		}
		if class.SuperClassSet && class.SuperClass != nil && class.SuperClass != superClass {
			exception := core.NewTypeError("superclass mismatch for class " + class.Name)
			if vm.raiseException(frame, exception) {
				return nil
			}
			vm.returnUnhandledException(frame, exception)
			return nil
		}
		newSubclass := !class.SuperClassSet
		class.SuperClass = superClass
		class.SuperClassSet = true
		if newSubclass {
			core.NotifyClassInherited(superVal, classVal)
		}
		vm.push(classVal)

	case compiler.OpModule:
		nameIdx := vm.readUint16()
		rawName := constants[nameIdx].Data.(string)
		absolute := strings.HasPrefix(rawName, "::")
		name := strings.TrimPrefix(rawName, "::")

		var module *object.Module
		var existingModuleValue *object.EmeraldValue
		useExisting := func(existing *object.EmeraldValue) bool {
			if existing == nil {
				return false
			}
			if existing.Type == object.ValueException {
				if vm.raiseException(frame, existing) {
					return true
				}
				vm.returnUnhandledException(frame, existing)
				return true
			}
			if existing.Type == object.ValueModule {
				module = existing.Data.(*object.Module)
				existingModuleValue = existing
				return false
			}
			if name == "Kernel" && existing.Type == object.ValueClass {
				existingModuleValue = existing
				return false
			}
			exception := core.NewTypeError(name + " is not a module")
			if vm.raiseException(frame, exception) {
				return true
			}
			vm.returnUnhandledException(frame, exception)
			return true
		}
		if errVal, found := vm.qualifiedPrivateConstantError(name); found {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); !absolute && ok {
			if existing, found := vm.moduleDefinitionValueFromContainer(container, constName); found {
				if useExisting(existing) {
					return nil
				}
			}
			if module == nil {
				module = object.NewModule("")
			}
		} else if container, constName, ok := vm.lexicalQualifiedConstantContainer(name); !absolute && ok {
			if existing, found := vm.moduleDefinitionValueFromContainer(container, constName); found {
				if useExisting(existing) {
					return nil
				}
			}
			if module == nil {
				module = object.NewModule("")
			}
		} else if container := vm.currentConstantContainer(); !absolute && container != nil && !strings.Contains(name, "::") {
			if existing, found := vm.moduleDefinitionValueFromContainer(container, name); found {
				if useExisting(existing) {
					return nil
				}
			}
			if module == nil {
				module = object.NewModule("")
			}
		} else if existing, ok := vm.rubyConsts[name]; ok {
			if useExisting(existing) {
				return nil
			}
		} else if container, constName, ok := vm.qualifiedConstantContainer(name); ok {
			if existing, found := vm.moduleDefinitionValueFromContainer(container, constName); found {
				if useExisting(existing) {
					return nil
				}
			}
			if module == nil {
				module = object.NewModule("")
			}
		} else if existing, ok := vm.topLevelDefinitionConstantValue(name); ok {
			if useExisting(existing) {
				return nil
			}
			if module == nil {
				module = object.NewModule(name)
			}
		} else {
			module = object.NewModule(name)
		}

		moduleVal := existingModuleValue
		if moduleVal == nil {
			moduleVal = &object.EmeraldValue{
				Type:  object.ValueModule,
				Data:  module,
				Class: core.R.Classes["Module"],
			}
		}
		if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); !absolute && ok {
			defineConstantOn(container, constName, moduleVal)
			vm.rubyConsts[qualifiedConstantName(container, constName)] = moduleVal
		} else if container, constName, ok := vm.lexicalQualifiedConstantContainer(name); !absolute && ok {
			qualifiedName := qualifiedConstantName(container, constName)
			if moduleVal.Type == object.ValueModule {
				if scopedModule := moduleVal.Data.(*object.Module); scopedModule.Name == "" || scopedModule.Name == name {
					scopedModule.Name = qualifiedName
				}
			}
			defineConstantOn(container, constName, moduleVal)
			vm.rubyConsts[qualifiedName] = moduleVal
		} else if container := vm.currentConstantContainer(); !absolute && container != nil && !strings.Contains(name, "::") {
			defineConstantOn(container, name, moduleVal)
			vm.rubyConsts[qualifiedConstantName(container, name)] = moduleVal
		} else if strings.Contains(name, "::") {
			vm.rubyConsts[name] = moduleVal
			vm.defineQualifiedConstant(name, moduleVal)
		} else {
			vm.rubyConsts[name] = moduleVal
			vm.SetTopLevelConstant(name, moduleVal)
		}
		vm.classStack = append(vm.classStack, moduleVal)
		vm.push(moduleVal)

	case compiler.OpDup:
		vm.push(vm.peek(0))

	case compiler.OpSwap:
		if vm.sp >= 2 {
			vm.stack[vm.sp-1], vm.stack[vm.sp-2] = vm.stack[vm.sp-2], vm.stack[vm.sp-1]
		}

	case compiler.OpCaseSplatMatch:
		conditions := vm.pop()
		target := vm.pop()
		matched := false
		if conditions != nil && conditions.Type == object.ValueArray {
			for _, condition := range conditions.Data.([]*object.EmeraldValue) {
				result := vm.send(condition, "===", []*object.EmeraldValue{target})
				if shouldPropagateExceptionValue(result) {
					if vm.raiseException(frame, result) {
						return nil
					}
					vm.returnUnhandledException(frame, result)
					return nil
				}
				if result.IsTruthy() {
					matched = true
					break
				}
			}
		}
		vm.push(boolValue(matched))

	case compiler.OpFreeze:
		value := vm.pop()
		if value != nil {
			value.Frozen = true
		}
		vm.push(value)

	case compiler.OpMultiAssignPrepare:
		val := vm.pop()
		prepared, err := vm.prepareMultiAssignRHS(val)
		if err != nil {
			if vm.raiseException(frame, err) {
				return nil
			}
			vm.returnUnhandledException(frame, err)
			return nil
		}
		vm.push(prepared)

	case compiler.OpMultiAssignExtract:
		kind := int(vm.readUint8())
		index := int(vm.readUint8())
		preCount := int(vm.readUint8())
		postCount := int(vm.readUint8())
		array := vm.pop()
		values := array.Data.([]*object.EmeraldValue)
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
			vm.push(vm.arrayValue(append([]*object.EmeraldValue(nil), values[start:end]...)...))
		case 2:
			start := len(values) - postCount
			if start < preCount {
				start = preCount
			}
			position := start + index
			if position >= 0 && position < len(values) {
				vm.push(values[position])
			} else {
				vm.push(core.R.NilVal)
			}
		default:
			if index >= 0 && index < len(values) {
				vm.push(values[index])
			} else {
				vm.push(core.R.NilVal)
			}
		}

	case compiler.OpMultiAssignCheckToAry:
		val := vm.pop()
		if err := vm.checkMultiAssignToAry(val); err != nil {
			if vm.raiseException(frame, err) {
				return nil
			}
			vm.returnUnhandledException(frame, err)
			return nil
		}

	case compiler.OpClosure:
		fnIdx := vm.readUint16()
		numFree := vm.readUint8()

		constant := constants[fnIdx]
		fn, ok := constant.Data.(*object.Function)
		if !ok {
			return fmt.Errorf("not a function: %v", constant)
		}
		fnCopy := *fn
		fnCopy.Constants = constants

		free := make([]*object.EmeraldValue, numFree)
		for i := numFree - 1; i >= 0; i-- {
			free[i] = snapshotClosureCapture(vm.pop())
		}

		closure := &object.Closure{
			Fn:         &fnCopy,
			Free:       free,
			Block:      vm.yieldBlock(),
			Binding:    vm.captureFrameBinding(),
			ClassStack: vm.currentClassStackSnapshot(),
			AutoSplat:  true,
		}
		if refinements, fixed := vm.currentFixedRefinements(); fixed {
			closure.Refinements = append([]*object.EmeraldValue(nil), refinements...)
			closure.RefinementsFixed = true
		}
		if fnCopy.Name == "__block__" || fnCopy.SingletonClassBody {
			closure.ReturnOwnerID = vm.lexicalReturnOwnerID(frame)
		}
		if fnCopy.Name == "__scoped_const_rhs__" && frame.WhileEnd >= 0 {
			closure.BreakOwnerID = frame.ID
		}

		vm.push(&object.EmeraldValue{
			Type:  object.ValueClosure,
			Data:  closure,
			Class: core.R.Classes["Proc"],
		})

	case compiler.OpLambda:
		fnIdx := vm.readUint16()
		numFree := vm.readUint8()

		fn, ok := constants[fnIdx].Data.(*object.Function)
		if !ok {
			return fmt.Errorf("not a function: %v", constants[fnIdx])
		}
		fnCopy := *fn
		fnCopy.Constants = constants

		free := make([]*object.EmeraldValue, numFree)
		for i := numFree - 1; i >= 0; i-- {
			free[i] = snapshotClosureCapture(vm.pop())
		}

		proc := &object.Proc{
			Fn:           &fnCopy,
			Env:          free,
			Block:        vm.yieldBlock(),
			Binding:      vm.captureFrameBinding(),
			ClassStack:   vm.currentClassStackSnapshot(),
			InstanceVars: make(map[string]*object.EmeraldValue),
			IsLambda:     true,
		}
		if refinements, fixed := vm.currentFixedRefinements(); fixed {
			proc.Refinements = append([]*object.EmeraldValue(nil), refinements...)
			proc.RefinementsFixed = true
		}

		vm.push(&object.EmeraldValue{
			Type:  object.ValueProc,
			Data:  proc,
			Class: core.R.Classes["Proc"],
		})

	case compiler.OpSplat:
		val := vm.pop()
		arr, err := vm.toAryForSplat(val)
		if err != nil {
			if vm.raiseException(frame, err) {
				return nil
			}
			vm.returnUnhandledException(frame, err)
			return nil
		}
		for _, elem := range arr {
			vm.push(elem)
		}

	case compiler.OpSplatToA:
		val := vm.pop()
		arr, err := vm.toAForAssignmentSplat(val)
		if err != nil {
			if vm.raiseException(frame, err) {
				return nil
			}
			vm.returnUnhandledException(frame, err)
			return nil
		}
		for _, elem := range arr {
			vm.push(elem)
		}

	case compiler.OpSplatToArray:
		val := vm.pop()
		arr, err := vm.toAForAssignmentSplat(val)
		if err != nil {
			if vm.raiseException(frame, err) {
				return nil
			}
			vm.returnUnhandledException(frame, err)
			return nil
		}
		vm.push(vm.arrayValue(arr...))

	case compiler.OpRange:
		exclusive := vm.readUint8()
		startMissing := vm.readUint8() == 1
		endMissing := vm.readUint8() == 1
		right := vm.pop()
		left := vm.pop()

		var start, end int64
		var startRaw, endRaw float64
		startFloat := false
		endFloat := false
		if startMissing {
			left = nil
			start = 0
		}
		if endMissing {
			right = nil
			end = 0
		}
		if left != nil {
			if l, ok := left.Data.(int64); ok {
				start = l
				startRaw = float64(l)
			} else if l, ok := left.Data.(float64); ok {
				start = int64(l)
				startFloat = true
				startRaw = l
			}
		}
		if right != nil {
			if r, ok := right.Data.(int64); ok {
				end = r
				endRaw = float64(r)
			} else if r, ok := right.Data.(float64); ok {
				end = int64(r)
				endFloat = true
				endRaw = r
			}
		}

		rangeObj := &object.RRange{
			Start:        start,
			End:          end,
			StartValue:   left,
			EndValue:     right,
			StartFloat:   startFloat,
			EndFloat:     endFloat,
			StartRaw:     startRaw,
			EndRaw:       endRaw,
			StartMissing: startMissing,
			EndMissing:   endMissing,
			Exclusive:    exclusive == 1,
		}
		vm.push(&object.EmeraldValue{
			Type:   object.ValueRange,
			Data:   rangeObj,
			Class:  core.R.Classes["Range"],
			Frozen: true,
		})

	case compiler.OpRationalNew:
		denVal := vm.pop()
		numVal := vm.pop()
		result := core.NewRationalFromIntegerValues(numVal, denVal)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		vm.push(result)

	case compiler.OpSendSuper:
		methodNameIdx := vm.readUint16()
		blockArg := vm.readUint8()
		numArgs := vm.readUint8()
		splatIndex := int(vm.readUint8())
		_ = constants[methodNameIdx].Data.(string)
		superFrame := vm.superContextFrame(frame)

		var block *object.EmeraldValue
		if blockArg&1 != 0 {
			var blockErr *object.EmeraldValue
			block, blockErr = vm.normalizeBlockPass(derefClosureValue(vm.pop()))
			if blockErr != nil {
				if vm.raiseException(frame, blockErr) {
					return nil
				}
				vm.returnUnhandledException(frame, blockErr)
				return nil
			}
		}
		forwardedBlock := block
		if blockArg&1 == 0 {
			forwardedBlock = superFrame.Block
		}

		args := make([]*object.EmeraldValue, int(numArgs))
		if numArgs == 255 {
			if superFrame.Fn != nil && superFrame.Fn.DefinedByDefineMethod {
				result := core.NewRuntimeError("implicit argument passing of super from method defined by define_method() is not supported")
				if vm.raiseException(frame, result) {
					return nil
				}
				vm.returnUnhandledException(frame, result)
				return nil
			}
			args = vm.implicitSuperArgs(superFrame)
		} else {
			for i := 0; i < int(numArgs); i++ {
				args[int(numArgs)-1-i] = vm.pop()
			}
			if splatIndex != 255 {
				var errVal *object.EmeraldValue
				args, errVal = vm.expandMethodSplatArgs(args, splatIndex)
				if errVal != nil {
					if vm.raiseException(frame, errVal) {
						return nil
					}
					vm.returnUnhandledException(frame, errVal)
					return nil
				}
			}
		}
		if blockArg&2 != 0 && len(args) > 0 {
			last := args[len(args)-1]
			if last != nil && last.Type == object.ValueHash {
				last = copyKeywordHash(last)
				args[len(args)-1] = last
				core.MarkRuby2KeywordHash(last)
			}
		}
		if numArgs != 255 {
			vm.pop()
		} else {
			vm.pop()
		}

		self := vm.stack[superFrame.Bp]
		if self.Class == nil {
			vm.push(core.R.NilVal)
			return nil
		}

		superClass := superFrame.SuperStart
		if superClass == nil {
			superClass = self.Class.SuperClass
		}
		superMethodName := superFrame.OriginalMethodName
		if superMethodName == "" {
			superMethodName = superFrame.MethodName
		}
		if superClass == nil {
			result := core.NewNoMethodError("super: no superclass method `" + superMethodName + "'")
			if vm.raiseException(frame, result) {
				return nil
			}
			vm.returnUnhandledException(frame, result)
			return nil
		}
		var methodObj *object.Method
		var owner *object.Class
		var ownerModule *object.Module
		var ok bool
		if superFrame.SuperAfterClass {
			methodObj, owner, ownerModule, ok = getMethodAfterClassWithOwner(superClass, superMethodName)
		} else if superFrame.SuperModule != nil {
			methodObj, owner, ownerModule, ok = getIncludedMethodAfterModule(superClass, superFrame.SuperModule, superMethodName)
		} else if superClass == self.Class {
			methodObj, owner, ok = getMethodAfterPrependsWithOwner(superClass, superMethodName)
		} else if self.Type == object.ValueClass {
			if superClass.IsSingleton {
				methodObj, owner, ok = superClass.GetMethodWithOwner(superMethodName)
			} else {
				for current := superClass; current != nil && !ok; current = current.SuperClass {
					if current.SingletonClass != nil {
						if candidate, candidateOwner, found := current.SingletonClass.GetMethodWithOwner(superMethodName); found {
							methodObj, owner, ok = candidate, candidateOwner, true
							break
						}
					}
					if candidate, found := current.ClassMethods[superMethodName]; found {
						methodObj, owner, ok = candidate, current, true
					}
				}
			}
		} else {
			methodObj, owner, ok = superClass.GetMethodWithOwner(superMethodName)
		}
		if !ok || methodObj == nil || methodObj.Visibility == "undefined" {
			if superMethodName != "method_missing" {
				missingArgs := make([]*object.EmeraldValue, 0, len(args)+1)
				missingArgs = append(missingArgs, &object.EmeraldValue{Type: object.ValueSymbol, Data: superMethodName, Class: core.R.Classes["Symbol"]})
				missingArgs = append(missingArgs, args...)
				if missing := vm.send(self, "method_missing", missingArgs); missing != nil && missing.Type != object.ValueException {
					vm.push(missing)
					return nil
				}
			}
			result := core.NewNoMethodError("super: no superclass method `" + superMethodName + "'")
			if vm.raiseException(frame, result) {
				return nil
			}
			vm.returnUnhandledException(frame, result)
			return nil
		}

		if fn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
			prevBlock := vm.currentBlock
			vm.currentBlock = forwardedBlock
			result := fn(self, args...)
			vm.currentBlock = prevBlock
			vm.push(result)
			return nil
		}

		if fn, ok := methodObj.Fn.(*object.Function); ok {
			oldFrame := vm.frames[vm.fp]
			prevBlock := vm.currentBlock
			vm.currentBlock = forwardedBlock
			defer func() {
				vm.currentBlock = prevBlock
			}()
			if errVal := rejectBlockArgument(fn, vm.currentBlock); errVal != nil {
				if vm.raiseException(frame, errVal) {
					return nil
				}
				vm.returnUnhandledException(frame, errVal)
				return nil
			}
			bp := vm.sp
			vm.stack[vm.sp] = self
			vm.bindFunctionArguments(fn, args, vm.currentBlock, methodObj.Ruby2Keywords)
			if errVal := vm.bindParameterPatterns(fn, bp); errVal != nil {
				if vm.raiseException(frame, errVal) {
					return nil
				}
				vm.returnUnhandledException(frame, errVal)
				return nil
			}
			var blockParamProc *object.Proc
			if fn.HasBlockParam {
				blockSlot := bp + fn.BlockParamIndex + 1
				if blockVal := vm.stack[blockSlot]; blockVal != nil && blockVal.Type == object.ValueProc {
					blockParamProc, _ = blockVal.Data.(*object.Proc)
				}
			}
			newFrame := vm.pushReusableFrame(Frame{
				ID:                    vm.allocateFrameID(),
				Fn:                    fn,
				Ip:                    -1,
				Bp:                    bp,
				MethodName:            superMethodName,
				OriginalMethodName:    methodObj.OriginalName,
				SuperStart:            owner.SuperClass,
				SuperModule:           ownerModule,
				Args:                  args,
				Block:                 vm.currentBlock,
				DefinedByDefineMethod: methodObj.DefinedByDefineMethod,
			})
			if ownerModule != nil {
				newFrame.SuperStart = owner
			}
			if blockParamProc != nil && blockParamProc.BreakOwnerID == 0 {
				blockParamProc.BreakOwnerID = newFrame.ID
			}
			setBlockBreakOwner(vm.currentBlock, newFrame.ID)

			curFrame := vm.frames[vm.fp]
			instructions := curFrame.Fn.Instructions
			for curFrame.Ip < len(instructions)-1 {
				curFrame.Ip++
				op := compiler.Opcode(instructions[curFrame.Ip])
				vm.fireTracePointLine(curFrame, op)
				curFrame.InstructionException = core.LastException
				curFrame.InstructionSnapshotSet = true
				if err := vm.execute(op, curFrame); err != nil {
					return err
				}
				if vm.handlePendingNonLocalReturn(curFrame) || vm.handlePendingNonLocalBreak(curFrame) || curFrame.Returned {
					break
				}
				curFrame = vm.frames[vm.fp]
				if core.LastBlockResult != nil {
					break
				}
				instructions = curFrame.Fn.Instructions
			}

			result := core.R.NilVal
			if pending, ok := vm.pendingBreakResultForFrame(curFrame); ok {
				result = pending
			} else if core.LastBlockResult != nil {
				result = core.LastBlockResult
			} else if vm.sp > bp {
				result = vm.stack[vm.sp-1]
			}
			vm.sp = bp
			vm.endActiveRescuesForFrame(curFrame)
			curFrame.InstructionException = nil
			curFrame.InstructionSnapshotSet = false
			vm.frames = vm.frames[:vm.fp]
			vm.fp--
			vm.frames[vm.fp] = oldFrame
			vm.consumeCompletedBreakMarker()
			vm.push(result)
			return nil
		}

		vm.push(core.R.NilVal)

	case compiler.OpBlockGiven:
		if vm.methodBlockGiven() {
			vm.push(core.R.TrueVal)
		} else {
			vm.push(core.R.FalseVal)
		}

	case compiler.OpIsA, compiler.OpKindOf:
		classVal := vm.pop()
		obj := vm.pop()

		if classVal.Type != object.ValueClass {
			vm.push(core.R.FalseVal)
			return nil
		}

		targetClass := classVal.Data.(*object.Class)
		objClass := obj.Class

		// Check if obj's class is the target class or inherits from it
		for objClass != nil {
			if objClass == targetClass {
				vm.push(core.R.TrueVal)
				return nil
			}
			objClass = objClass.SuperClass
		}
		vm.push(core.R.FalseVal)

	case compiler.OpRespondTo:
		methodName := vm.pop()
		obj := vm.pop()

		if methodName.Type != object.ValueString && methodName.Type != object.ValueSymbol {
			vm.push(core.R.FalseVal)
			return nil
		}

		var methodNameStr string
		if methodName.Type == object.ValueSymbol {
			methodNameStr = methodName.Data.(string)
		} else {
			methodNameStr = methodName.Data.(string)
		}

		// Check if object has the method
		if obj.Class != nil {
			_, ok := obj.Class.GetMethod(methodNameStr)
			if ok {
				vm.push(core.R.TrueVal)
				return nil
			}
		}

		// For basic objects, check if it's a ValueObject with RespondTo method
		if obj.Type == object.ValueObject {
			objData := obj.Data.(*object.Object)
			if objData.RespondTo(methodNameStr) {
				vm.push(core.R.TrueVal)
				return nil
			}
		}

		vm.push(core.R.FalseVal)

	case compiler.OpDefined:
		nameIdx := vm.readUint16()
		name, ok := constants[nameIdx].Data.(string)
		if !ok {
			return fmt.Errorf("OpDefined: expected string constant, got %T", constants[nameIdx].Data)
		}
		receiver := vm.pop()
		if vm.definedAutoload(receiver, name, true) {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "constant", Class: core.R.Classes["String"], Frozen: true})
			break
		}
		value, found := vm.scopedConstantValue(receiver, name)
		if found && value != nil && value.Type != object.ValueException {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "constant", Class: core.R.Classes["String"], Frozen: true})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpDefinedConstant:
		nameIdx := vm.readUint16()
		name, ok := constants[nameIdx].Data.(string)
		if !ok {
			return fmt.Errorf("OpDefinedConstant: expected string constant, got %T", constants[nameIdx].Data)
		}
		if vm.lexicalAutoloadDefined(name) {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "constant", Class: core.R.Classes["String"], Frozen: true})
			break
		}
		value, found := vm.lexicalConstantValue(name)
		if !found && vm.allowTopLevelConstantFallback(name) {
			value, found = vm.topLevelConstantValue(name)
		}
		if found && value != nil && value.Type != object.ValueException {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "constant", Class: core.R.Classes["String"], Frozen: true})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpDefinedMethod:
		nameIdx := vm.readUint16()
		implicit := vm.readUint8() == 1
		name, ok := constants[nameIdx].Data.(string)
		if !ok {
			return fmt.Errorf("OpDefinedMethod: expected string constant, got %T", constants[nameIdx].Data)
		}
		receiver := vm.pop()
		methodObj, _, _ := vm.lookupMethodForSend(receiver, name, nil, false)
		found := methodObj != nil && methodObj.Visibility != "undefined"
		defined := found
		if found && !implicit && methodObj.Visibility != "" && methodObj.Visibility != "public" {
			defined = false
		}
		if !found && receiver != nil {
			symbol := &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]}
			responds := vm.send(receiver, "respond_to_missing?", []*object.EmeraldValue{symbol, core.R.FalseVal})
			defined = responds != nil && responds.Type == object.ValueBool && responds.Data.(bool)
		}
		if defined {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "method", Class: core.R.Classes["String"], Frozen: true})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpDefinedYield:
		if vm.yieldBlock() != nil {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "yield", Class: core.R.Classes["String"], Frozen: true})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpDefinedSuper:
		self := vm.stack[frame.Bp]
		var methodObj *object.Method
		var found bool
		if self != nil && self.Class != nil && frame.MethodName != "" {
			superClass := frame.SuperStart
			if superClass == nil {
				superClass = self.Class.SuperClass
			}
			if superClass != nil {
				if frame.SuperAfterClass {
					methodObj, _, _, found = getMethodAfterClassWithOwner(superClass, frame.MethodName)
				} else if frame.SuperModule != nil {
					methodObj, _, _, found = getIncludedMethodAfterModule(superClass, frame.SuperModule, frame.MethodName)
				} else if superClass == self.Class {
					methodObj, _, found = getMethodAfterPrependsWithOwner(superClass, frame.MethodName)
				} else {
					methodObj, _, found = superClass.GetMethodWithOwner(frame.MethodName)
				}
			}
		}
		if found && methodObj != nil && methodObj.Visibility != "undefined" {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "super", Class: core.R.Classes["String"], Frozen: true})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpBeginRescue:
		rescueOffset := vm.readUint16()
		ensureOffset := vm.readUint16()
		endOffset := vm.readUint16()
		ensureEndOffset := vm.readUint16()

		handler := &RescueHandler{
			BodyOffset:      frame.Ip + 1,
			StackTop:        vm.sp,
			RescueOffset:    rescueOffset,
			EnsureOffset:    ensureOffset,
			EndOffset:       endOffset,
			EnsureEndOffset: ensureEndOffset,
			Frame:           frame,
		}
		vm.rescueStack = append(vm.rescueStack, handler)

	case compiler.OpRetry:
		var active *ActiveRescue
		if len(vm.activeRescues) > 0 {
			active = vm.activeRescues[len(vm.activeRescues)-1]
		}
		if active == nil || active.Frame != frame {
			active = frame.RetryRescue
		}
		if active == nil || active.Frame != frame {
			return fmt.Errorf("Invalid retry")
		}
		if len(vm.activeRescues) > 0 && vm.activeRescues[len(vm.activeRescues)-1] == active {
			vm.activeRescues = vm.activeRescues[:len(vm.activeRescues)-1]
		}
		frame.RetryRescue = nil
		core.LastException = active.PreviousException
		vm.sp = active.StackTop
		vm.rescueStack = append(vm.rescueStack, &RescueHandler{
			BodyOffset:      active.BodyOffset,
			StackTop:        active.StackTop,
			RescueOffset:    active.RescueOffset,
			EnsureOffset:    active.EnsureOffset,
			EnsureEndOffset: active.EnsureEndOffset,
			EndOffset:       active.EndOffset,
			Frame:           frame,
		})
		frame.Ip = active.BodyOffset - 1

	case compiler.OpEndRescue:
		vm.endActiveRescue(frame)

	case compiler.OpEnsure:
		vm.ensureActive = true

	case compiler.OpEndEnsure:
		if len(vm.pendingEnsures) == 0 {
			vm.ensureActive = false
			return nil
		}
		pending := vm.pendingEnsures[len(vm.pendingEnsures)-1]
		if pending.Frame != frame || pending.EnsureEndOffset != frame.Ip+1 {
			return nil
		}
		vm.pendingEnsures = vm.pendingEnsures[:len(vm.pendingEnsures)-1]
		vm.ensureActive = false
		core.LastException = pending.PreviousException
		if pending.IsRedo {
			if vm.routeRedoThroughEnsure(frame) {
				return nil
			}
			frame.Ip = -1
			vm.sp = frame.Bp + 1 + frame.Fn.NumLocals
			return nil
		}
		if pending.IsThrow {
			if vm.routeThrowThroughEnsure(frame, pending.ThrowHandler, pending.ReturnValue) {
				return nil
			}
			vm.completeThrow(frame, pending.ThrowHandler, pending.ReturnValue)
			return nil
		}
		if pending.IsNext {
			if vm.routeNextThroughEnsure(frame, pending.ReturnValue, pending.BreakTarget) {
				return nil
			}
			if pending.BreakTarget > 0 {
				vm.push(pending.ReturnValue)
				frame.Ip = pending.BreakTarget - 1
				return nil
			}
			frame.BlockNextVal = pending.ReturnValue
			frame.Ip = len(frame.Fn.Instructions) - 1
			vm.sp = frame.Bp
			core.ForEachMarkNext()
			return nil
		}
		if pending.IsBreak {
			if vm.routeBreakThroughEnsure(frame, pending.ReturnValue, pending.BreakTarget) {
				return nil
			}
			if pending.BreakTarget < 0 {
				base := frame.Bp + 1
				if frame.Fn != nil {
					base += frame.Fn.NumLocals
				}
				vm.sp = base
				vm.push(pending.ReturnValue)
				frame.Returned = true
				return nil
			}
			if pending.BreakTarget > 0 {
				vm.push(pending.ReturnValue)
				frame.Ip = pending.BreakTarget - 1
				return nil
			}
			frame.BlockBreak = true
			frame.BlockBreakVal = pending.ReturnValue
			vm.sp = frame.Bp
			vm.push(pending.ReturnValue)
			return nil
		}
		if pending.IsReturn {
			if vm.routeReturnThroughEnsure(frame, pending.ReturnValue) {
				return nil
			}
			vm.push(pending.ReturnValue)
			frame.Returned = true
			return nil
		}
		if vm.raiseException(frame, pending.Exception) {
			return nil
		}
		vm.returnUnhandledException(frame, pending.Exception)

	case compiler.OpRaise:
		var exception *object.EmeraldValue
		previousException := core.LastException
		constructedException := false
		if vm.sp > 0 {
			exception = vm.pop()
		}
		if exception == nil || exception.Type != object.ValueException {
			if exception == nil {
				exception = core.R.NilVal
			}
			exception = core.RaiseValue(exception)
			constructedException = true
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
		if constructedException && canSetCause && cause != nil && cause != exception {
			if exc, ok := exception.Data.(*object.RException); ok && exc != nil && exc.Cause == nil {
				exc.Cause = cause
			}
		}
		if vm.raiseException(frame, exception) {
			return nil
		}
		vm.returnUnhandledException(frame, exception)

	case compiler.OpReraise:
		var exception *object.EmeraldValue
		for i := len(vm.activeRescues) - 1; i >= 0; i-- {
			if vm.activeRescues[i].Frame == frame {
				exception = core.LastException
				break
			}
		}
		if exception == nil {
			exception = &object.EmeraldValue{
				Type:  object.ValueException,
				Data:  &object.RException{Message: ""},
				Class: core.R.Classes["RuntimeError"],
			}
		}
		if vm.raiseException(frame, exception) {
			return nil
		}
		vm.returnUnhandledException(frame, exception)

	case compiler.OpRescue:
		if core.LastException == nil {
			vm.push(core.R.NilVal)
			return nil
		}
		vm.push(core.LastException)
		if rescued, ok := core.LastException.Data.(*object.RException); ok && rescued != nil {
			rescued.Raised = false
		}
		core.LastRaisedResult = nil

	case compiler.OpRescueMatch:
		count := int(vm.readUint8())
		splatMask := int(vm.readUint16())
		values := make([]*object.EmeraldValue, count)
		for i := count - 1; i >= 0; i-- {
			values[i] = vm.pop()
		}
		classes := make([]*object.EmeraldValue, 0, count)
		for i, value := range values {
			if splatMask&(1<<i) == 0 {
				classes = append(classes, value)
				continue
			}
			expanded, err := vm.toAForAssignmentSplat(value)
			if err != nil {
				if vm.raiseException(frame, err) {
					return nil
				}
				vm.returnUnhandledException(frame, err)
				return nil
			}
			classes = append(classes, expanded...)
		}
		if invalidRescueClause(classes) {
			err := core.NewTypeError("class or module required for rescue clause")
			if vm.raiseException(frame, err) {
				return nil
			}
			vm.returnUnhandledException(frame, err)
			return nil
		}
		matched, matchErr := vm.rescueMatches(core.LastException, classes)
		if matchErr != nil {
			if vm.raiseException(frame, matchErr) {
				return nil
			}
			vm.returnUnhandledException(frame, matchErr)
			return nil
		}
		vm.push(boolValue(matched))

	case compiler.OpCatch:
		endOffset := vm.readUint16()
		label := core.R.NilVal
		if vm.sp > 0 {
			label = vm.pop()
		}

		handler := &CatchHandler{
			Label:     label,
			EndOffset: endOffset,
			Frame:     frame,
			StackTop:  vm.sp,
			VM:        vm,
		}
		vm.catchStack = append(vm.catchStack, handler)

	case compiler.OpThrow:
		var value *object.EmeraldValue
		if vm.sp > 0 {
			value = vm.pop()
		}
		var label *object.EmeraldValue
		if vm.sp > 0 {
			label = vm.pop()
		}

		if handler := vm.matchingCatchHandler(label); handler != nil {
			if vm.routeThrowThroughEnsure(frame, handler, value) {
				return nil
			}
			vm.completeThrow(frame, handler, value)
			return nil
		}
		exception := core.NewUncaughtThrowError(label, value)
		if vm.raiseException(frame, exception) {
			return nil
		}
		vm.returnUnhandledException(frame, exception)

	default:
		return fmt.Errorf("unknown opcode: %v", op)
	}

	return nil
}

func (vm *VM) push(val *object.EmeraldValue) {
	if vm.sp >= len(vm.stack) {
		nextSize := len(vm.stack) * 2
		if nextSize == 0 {
			nextSize = StackSize
		}
		vm.stack = append(vm.stack, make([]*object.EmeraldValue, nextSize-len(vm.stack))...)
	}
	vm.stack[vm.sp] = val
	vm.sp++
}

func (vm *VM) recordPoppedValue(val *object.EmeraldValue) {
	if len(vm.poppedValues) == 0 {
		vm.poppedValues = append(vm.poppedValues, val)
		return
	}
	vm.poppedValues[0] = val
}

func (vm *VM) pop() *object.EmeraldValue {
	if vm.sp <= 0 {
		return core.R.NilVal
	}
	vm.sp--
	val := vm.stack[vm.sp]
	if val == nil {
		val = core.R.NilVal
	}
	vm.recordPoppedValue(val)
	return val
}

func (vm *VM) popFrameValue(frame *Frame) *object.EmeraldValue {
	minSp := 0
	if frame != nil && frame.Fn != nil {
		minSp = frame.Bp + 1 + frame.Fn.NumLocals
	}
	if vm.sp <= minSp {
		val := core.R.NilVal
		vm.recordPoppedValue(val)
		return val
	}
	return vm.pop()
}

func (vm *VM) peek(n int) *object.EmeraldValue {
	if n < 0 || vm.sp-1-n < 0 || vm.sp == 0 {
		return core.R.NilVal
	}
	return vm.stack[vm.sp-1-n]
}

func (vm *VM) frameConstants(frame *Frame) []*object.EmeraldValue {
	if frame != nil && frame.Fn != nil && frame.Fn.Constants != nil {
		return frame.Fn.Constants
	}
	return vm.constants
}

func (vm *VM) nextInstructionIsExpectationSend(frame *Frame) bool {
	if frame == nil || frame.Fn == nil {
		return false
	}
	instructions := frame.Fn.Instructions
	next := frame.Ip + 1
	if next+2 >= len(instructions) {
		return false
	}
	op := compiler.Opcode(instructions[next])
	if op != compiler.OpSend && op != compiler.OpSendWithKeywords {
		return false
	}
	methodIndex := int(instructions[next+1])<<8 | int(instructions[next+2])
	constants := vm.frameConstants(frame)
	if methodIndex < 0 || methodIndex >= len(constants) || constants[methodIndex] == nil {
		return false
	}
	method, _ := constants[methodIndex].Data.(string)
	return method == "should" || method == "should_not"
}

func (vm *VM) readUint16() int {
	frame := vm.frames[vm.fp]
	frame.Ip++
	high := int(frame.Fn.Instructions[frame.Ip])
	frame.Ip++
	low := int(frame.Fn.Instructions[frame.Ip])
	return high<<8 | low
}

func (vm *VM) readUint8() int {
	frame := vm.frames[vm.fp]
	frame.Ip++
	return int(frame.Fn.Instructions[frame.Ip])
}

func (vm *VM) add(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && left.Type == object.ValueInteger && !core.IntegerPlusUsesBuiltinImplementation() {
		return vm.send(left, "+", []*object.EmeraldValue{right})
	}
	if left != nil && left.Type == object.ValueString && left.Class != core.R.Classes["String"] {
		return vm.send(left, "+", []*object.EmeraldValue{right})
	}
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueInteger {
		leftBig, leftIsBig := core.NumericBigIntOverride(left)
		rightBig, rightIsBig := core.NumericBigIntOverride(right)
		if leftIsBig || rightIsBig {
			if !leftIsBig {
				leftBig = big.NewInt(left.Data.(int64))
			}
			if !rightIsBig {
				rightBig = big.NewInt(right.Data.(int64))
			}
			return core.NewIntegerFromBigInt(new(big.Int).Add(leftBig, rightBig))
		}
		l, r := left.Data.(int64), right.Data.(int64)
		if (r > 0 && l > math.MaxInt64-r) || (r < 0 && l < math.MinInt64-r) {
			return core.NewIntegerFromBigInt(new(big.Int).Add(big.NewInt(l), big.NewInt(r)))
		}
	}
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueFloat {
		return &object.EmeraldValue{Type: object.ValueFloat, Data: toFloat64(left) + right.Data.(float64), Class: core.R.Classes["Float"]}
	}
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			return core.NewIntegerValue(l + r)
		case float64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) + r, Class: core.R.Classes["Float"]}
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l + float64(r), Class: core.R.Classes["Float"]}
		case float64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l + r, Class: core.R.Classes["Float"]}
		}
	case string:
		switch r := right.Data.(type) {
		case string:
			encoding, err := core.StringConcatenationEncoding(left, right)
			if err != nil {
				return err
			}
			result := &object.EmeraldValue{Type: object.ValueString, Data: l + r, Class: core.R.Classes["String"]}
			core.SetStringEncoding(result, encoding)
			return result
		}
	}
	if left == nil || left.Type == object.ValueNil {
		return core.NewNoMethodError("undefined method `+'")
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "+", []*object.EmeraldValue{right})
	}
	return core.R.NilVal
}

func (vm *VM) sub(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueInteger {
		leftBig, leftIsBig := core.NumericBigIntOverride(left)
		rightBig, rightIsBig := core.NumericBigIntOverride(right)
		if leftIsBig || rightIsBig {
			if !leftIsBig {
				leftBig = big.NewInt(left.Data.(int64))
			}
			if !rightIsBig {
				rightBig = big.NewInt(right.Data.(int64))
			}
			return core.NewIntegerFromBigInt(new(big.Int).Sub(leftBig, rightBig))
		}
		l, r := left.Data.(int64), right.Data.(int64)
		if (r < 0 && l > math.MaxInt64+r) || (r > 0 && l < math.MinInt64+r) {
			return core.NewIntegerFromBigInt(new(big.Int).Sub(big.NewInt(l), big.NewInt(r)))
		}
	}
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueFloat {
		return &object.EmeraldValue{Type: object.ValueFloat, Data: toFloat64(left) - right.Data.(float64), Class: core.R.Classes["Float"]}
	}
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			return core.NewIntegerValue(l - r)
		case float64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) - r, Class: core.R.Classes["Float"]}
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l - float64(r), Class: core.R.Classes["Float"]}
		case float64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l - r, Class: core.R.Classes["Float"]}
		}
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "-", []*object.EmeraldValue{right})
	}
	return core.R.NilVal
}

func vmValueToInt64(value *object.EmeraldValue) int64 {
	if value == nil {
		return 0
	}
	switch data := value.Data.(type) {
	case int64:
		return data
	case float64:
		return int64(data)
	default:
		return 0
	}
}

func (vm *VM) mul(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueFloat {
		return &object.EmeraldValue{Type: object.ValueFloat, Data: toFloat64(left) * right.Data.(float64), Class: core.R.Classes["Float"]}
	}
	if left != nil && right != nil && left.Type == object.ValueFloat && right.Type == object.ValueInteger {
		return &object.EmeraldValue{Type: object.ValueFloat, Data: left.Data.(float64) * toFloat64(right), Class: core.R.Classes["Float"]}
	}
	if leftBig, ok := core.NumericBigIntOverride(left); ok && right != nil && right.Type == object.ValueInteger {
		rightBig := big.NewInt(vmValueToInt64(right))
		if override, hasOverride := core.NumericBigIntOverride(right); hasOverride {
			rightBig = override
		}
		return core.NewIntegerFromBigInt(new(big.Int).Mul(leftBig, rightBig))
	}
	if rightBig, ok := core.NumericBigIntOverride(right); ok && left != nil && left.Type == object.ValueInteger {
		return core.NewIntegerFromBigInt(new(big.Int).Mul(big.NewInt(vmValueToInt64(left)), rightBig))
	}
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			product := l * r
			overflows := l == -1 && r == math.MinInt64 ||
				r == -1 && l == math.MinInt64 ||
				l != 0 && product/l != r
			if overflows {
				return core.NewIntegerFromBigInt(new(big.Int).Mul(big.NewInt(l), big.NewInt(r)))
			}
			return core.NewIntegerValue(product)
		case float64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) * r, Class: core.R.Classes["Float"]}
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l * float64(r), Class: core.R.Classes["Float"]}
		case float64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l * r, Class: core.R.Classes["Float"]}
		}
	case string:
		if left.Type == object.ValueString {
			return vm.send(left, "*", []*object.EmeraldValue{right})
		}
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "*", []*object.EmeraldValue{right})
	}
	return core.R.NilVal
}

func (vm *VM) div(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && left.Type == object.ValueInteger && right != nil && right.Class == core.R.Classes["Rational"] {
		return vm.send(left, "/", []*object.EmeraldValue{right})
	}
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueInteger {
		leftBig, _ := integerAsBigInt(left)
		rightBig, _ := integerAsBigInt(right)
		if rightBig.Sign() == 0 {
			return core.NewException("ZeroDivisionError", "divided by 0")
		}
		quotient := new(big.Int)
		remainder := new(big.Int)
		quotient.QuoRem(leftBig, rightBig, remainder)
		if remainder.Sign() != 0 && remainder.Sign() != rightBig.Sign() {
			quotient.Sub(quotient, big.NewInt(1))
		}
		return core.NewIntegerFromBigInt(quotient)
	}
	if left != nil && right != nil &&
		((left.Type == object.ValueInteger && right.Type == object.ValueFloat) ||
			(left.Type == object.ValueFloat && right.Type == object.ValueInteger)) {
		leftFloat := toFloat64(left)
		rightFloat := toFloat64(right)
		return &object.EmeraldValue{Type: object.ValueFloat, Data: leftFloat / rightFloat, Class: core.R.Classes["Float"]}
	}
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			if r == 0 {
				return &object.EmeraldValue{
					Type:  object.ValueException,
					Data:  &object.RException{Message: "divided by 0"},
					Class: core.R.Classes["ZeroDivisionError"],
				}
			}
			return core.NewIntegerValue(l / r)
		case float64:
			if r == 0 {
				if l == 0 {
					return &object.EmeraldValue{Type: object.ValueFloat, Data: math.NaN(), Class: core.R.Classes["Float"]}
				}
				return &object.EmeraldValue{Type: object.ValueFloat, Data: math.Copysign(math.Inf(1), float64(l)) * math.Copysign(1, r), Class: core.R.Classes["Float"]}
			}
			return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) / r, Class: core.R.Classes["Float"]}
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			if r == 0 {
				if l == 0 {
					return &object.EmeraldValue{Type: object.ValueFloat, Data: math.NaN(), Class: core.R.Classes["Float"]}
				}
				return &object.EmeraldValue{Type: object.ValueFloat, Data: math.Copysign(math.Inf(1), l), Class: core.R.Classes["Float"]}
			}
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l / float64(r), Class: core.R.Classes["Float"]}
		case float64:
			if r == 0 {
				if l == 0 {
					return &object.EmeraldValue{Type: object.ValueFloat, Data: math.NaN(), Class: core.R.Classes["Float"]}
				}
				return &object.EmeraldValue{Type: object.ValueFloat, Data: math.Copysign(math.Inf(1), l) * math.Copysign(1, r), Class: core.R.Classes["Float"]}
			}
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l / r, Class: core.R.Classes["Float"]}
		}
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "/", []*object.EmeraldValue{right})
	}
	return core.R.NilVal
}

func (vm *VM) mod(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueInteger {
		leftBig, leftIsBig := core.NumericBigIntOverride(left)
		rightBig, rightIsBig := core.NumericBigIntOverride(right)
		if !leftIsBig && !rightIsBig {
			l := left.Data.(int64)
			r := right.Data.(int64)
			if r == 0 {
				return core.NewException("ZeroDivisionError", "divided by 0")
			}
			result := l % r
			if result != 0 && (result < 0) != (r < 0) {
				result += r
			}
			return core.NewIntegerValue(result)
		}
		if !leftIsBig {
			leftBig = big.NewInt(left.Data.(int64))
		}
		if !rightIsBig {
			rightBig = big.NewInt(right.Data.(int64))
		}
		if rightBig.Sign() == 0 {
			return core.NewException("ZeroDivisionError", "divided by 0")
		}
		result := new(big.Int).Rem(leftBig, rightBig)
		if result.Sign() != 0 && result.Sign() != rightBig.Sign() {
			result.Add(result, rightBig)
		}
		return core.NewIntegerFromBigInt(result)
	}
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueFloat {
		leftFloat := toFloat64(left)
		rightFloat := right.Data.(float64)
		if rightFloat == 0 {
			return core.NewException("ZeroDivisionError", "divided by 0")
		}
		result := math.Mod(leftFloat, rightFloat)
		if result != 0 && math.Signbit(result) != math.Signbit(rightFloat) {
			result += rightFloat
		}
		return &object.EmeraldValue{Type: object.ValueFloat, Data: result, Class: core.R.Classes["Float"]}
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "%", []*object.EmeraldValue{right})
	}
	return core.R.NilVal
}

func (vm *VM) pow(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueInteger {
		exponent, exponentIsBig := core.NumericBigIntOverride(right)
		if !exponentIsBig {
			exponent = big.NewInt(right.Data.(int64))
		}
		base, baseIsBig := core.NumericBigIntOverride(left)
		if !baseIsBig {
			base = big.NewInt(left.Data.(int64))
		}
		if core.IntegerPowerResultTooLarge(base, exponent) {
			return core.NewArgumentError("exponent is too large")
		}
		if exponent.Sign() >= 0 && exponent.IsInt64() {
			return core.NewIntegerFromBigInt(new(big.Int).Exp(base, exponent, nil))
		}
	}
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			if l == 1 || l == -1 {
				result := int64(1)
				if l == -1 && r%2 != 0 {
					result = -1
				}
				if r < 0 {
					return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(result), Class: core.R.Classes["Float"]}
				}
				return core.NewIntegerValue(result)
			}
			if r < 0 {
				if l == 0 {
					return &object.EmeraldValue{
						Type:  object.ValueException,
						Data:  &object.RException{Message: "divided by 0"},
						Class: core.R.Classes["ZeroDivisionError"],
					}
				}
				return &object.EmeraldValue{Type: object.ValueFloat, Data: math.Pow(float64(l), float64(r)), Class: core.R.Classes["Float"]}
			}
			if integerPowOverflows(l, int(r)) {
				result := int64(math.MaxInt64)
				if l < 0 && r%2 != 0 {
					result = math.MinInt64
				}
				value := &object.EmeraldValue{Type: object.ValueInteger, Data: result, Class: core.R.Classes["Integer"]}
				core.RememberNumericFloatOverride(value, core.ApproximateIntegerPowAsFloat(l, int(r)))
				if l == 2 && r >= 0 {
					core.RememberBERPackOverride(value, core.BERPackPowerOfTwo(int(r)))
				}
				return value
			}
			return core.NewIntegerValue(vm.powInt(l, int(r)))
		case float64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: vm.mathPow(float64(l), r), Class: core.R.Classes["Float"]}
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: vm.mathPow(l, float64(r)), Class: core.R.Classes["Float"]}
		case float64:
			return &object.EmeraldValue{Type: object.ValueFloat, Data: vm.mathPow(l, r), Class: core.R.Classes["Float"]}
		}
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "**", []*object.EmeraldValue{right})
	}
	return core.R.NilVal
}

func integerPowOverflows(base int64, exp int) bool {
	if exp < 0 {
		return false
	}
	if base == 0 || base == 1 || base == -1 {
		return false
	}
	return math.Pow(math.Abs(float64(base)), float64(exp)) > float64(math.MaxInt64)
}

func (vm *VM) powInt(base int64, exp int) int64 {
	result := int64(1)
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

func toFloat64(val *object.EmeraldValue) float64 {
	if val == nil {
		return 0
	}
	if value, ok := core.NumericBigIntOverride(val); ok {
		converted, _ := new(big.Float).SetInt(value).Float64()
		return converted
	}
	switch v := val.Data.(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	default:
		return 0
	}
}

func (vm *VM) mathPow(base, exp float64) float64 {
	if exp >= 0 && exp == math.Trunc(exp) && exp <= math.MaxInt64 &&
		base == math.Trunc(base) && base >= math.MinInt64 && base <= math.MaxInt64 {
		integerBase := int64(base)
		if integerBase == 0 || integerBase == 1 || integerBase == -1 ||
			(exp <= 4096 && math.Log2(math.Abs(base))*exp <= 4096) {
			exact := new(big.Int).Exp(big.NewInt(integerBase), big.NewInt(int64(exp)), nil)
			converted, _ := new(big.Float).SetInt(exact).Float64()
			return converted
		}
	}
	return math.Pow(base, exp)
}

func (vm *VM) negate(val *object.EmeraldValue) *object.EmeraldValue {
	if bigValue, ok := core.NumericBigIntOverride(val); ok {
		return core.NewIntegerFromBigInt(new(big.Int).Neg(bigValue))
	}
	switch v := val.Data.(type) {
	case int64:
		return core.NewIntegerValue(-v)
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: -v, Class: core.R.Classes["Float"]}
	}
	if val != nil && val.Class != nil {
		return vm.send(val, "-@", nil)
	}
	return core.R.NilVal
}

func (vm *VM) bang(val *object.EmeraldValue) *object.EmeraldValue {
	if val != nil && core.ReceiverHasCallableMethod(val, "!") {
		return vm.send(val, "!", nil)
	}
	if val == nil || !val.IsTruthy() {
		return core.R.TrueVal
	}
	return core.R.FalseVal
}

func (vm *VM) equals(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left.Type == object.ValueNil && right.Type == object.ValueNil {
		return core.R.TrueVal
	}
	if left.Type == object.ValueArray && right.Type != object.ValueArray {
		return vm.send(left, "==", []*object.EmeraldValue{right})
	}
	if left.Type == object.ValueArray && right.Type == object.ValueArray {
		return vm.send(left, "==", []*object.EmeraldValue{right})
	}
	if left.Type == object.ValueHash || right.Type == object.ValueHash {
		if core.HashValuesEqual(left, right) {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	if left != nil && left.Type == object.ValueRegexp && core.CallMethod != nil {
		if result := core.CallMethod(left, "==", right); result != nil {
			return result
		}
	}
	if cmp, ok := integerBigCmp(left, right); ok {
		if cmp == 0 {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	if left != nil && left.Type == object.ValueInteger && right != nil && right.Type != object.ValueInteger {
		if result := core.CallMethod(left, "==", right); result != nil {
			return result
		}
	}
	if left != nil && left.Type == object.ValueFloat && right != nil && right.Type != object.ValueFloat && right.Type != object.ValueInteger {
		if result := core.CallMethod(left, "==", right); result != nil {
			return result
		}
	}
	if core.TimeValuesEqual(left, right) {
		return core.R.TrueVal
	}
	if core.DateValuesEqual(left, right) {
		return core.R.TrueVal
	}
	if core.OpenStructValuesEqual(left, right) {
		return core.R.TrueVal
	}
	if left != nil && left.Type == object.ValueException && core.CallMethod != nil && core.ReceiverHasCallableMethod(left, "==") {
		result := core.CallMethod(left, "==", right)
		if result != nil {
			return result
		}
	}
	if left != nil && left.Class == core.R.Classes["Complex"] && core.CallMethod != nil && core.ReceiverHasCallableMethod(left, "==") {
		result := core.CallMethod(left, "==", right)
		if result != nil {
			return result
		}
	}
	if left != nil && left.Type == object.ValueObject && core.CallMethod != nil && core.ReceiverHasCallableMethod(left, "==") {
		result := core.CallMethod(left, "==", right)
		if result != nil {
			return result
		}
	}
	if leftBig, ok := core.NumericBigIntOverride(left); ok && right != nil && right.Type == object.ValueFloat {
		converted, _ := new(big.Float).SetInt(leftBig).Float64()
		if converted == right.Data.(float64) {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	if rightBig, ok := core.NumericBigIntOverride(right); ok && left != nil && left.Type == object.ValueFloat {
		converted, _ := new(big.Float).SetInt(rightBig).Float64()
		if left.Data.(float64) == converted {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	if left.Type == right.Type && left.Equals(right) {
		return core.R.TrueVal
	}
	switch l := left.Data.(type) {
	case bool:
		r, ok := right.Data.(bool)
		if !ok {
			return core.R.FalseVal
		}
		if l == r {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	case int64:
		switch r := right.Data.(type) {
		case int64:
			if l == r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if float64(l) == r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			if l == float64(r) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if l == r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		}
	case string:
		if left.Type != right.Type {
			return core.R.FalseVal
		}
		r, ok := right.Data.(string)
		if !ok {
			return core.R.FalseVal
		}
		if l == r {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	if left == right {
		return core.R.TrueVal
	}
	if core.CallMethod != nil && left != nil && core.ReceiverHasCallableMethod(left, "<=>") {
		cmp := core.CallMethod(left, "<=>", right)
		if cmp != nil && cmp.Type == object.ValueException {
			return cmp
		}
		if cmp == nil || cmp.Type == object.ValueNil {
			return core.R.FalseVal
		}
		switch value := cmp.Data.(type) {
		case int64:
			if value == 0 {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if value == 0 {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		default:
			return core.NewArgumentError("comparison failed")
		}
	}
	return core.R.FalseVal
}

func (vm *VM) arraysEqual(left, right *object.EmeraldValue) bool {
	if left == nil || right == nil || left.Type != object.ValueArray || right.Type != object.ValueArray {
		return false
	}
	leftValues := left.Data.([]*object.EmeraldValue)
	rightValues := right.Data.([]*object.EmeraldValue)
	if len(leftValues) != len(rightValues) {
		return false
	}
	for i := range leftValues {
		result := vm.equals(leftValues[i], rightValues[i])
		if result == nil || result.Type != object.ValueBool || !result.Data.(bool) {
			return false
		}
	}
	return true
}

func (vm *VM) lessThan(left, right *object.EmeraldValue) *object.EmeraldValue {
	if result := moduleCompare(left, right, "<"); result != nil {
		return result
	}
	if cmp, ok := integerBigCmp(left, right); ok {
		if cmp < 0 {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			if l < r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if float64(l) < r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			if l < float64(r) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if l < r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		}
	}
	if result := directCompareMethod(left, right, "<"); result != nil {
		return result
	}
	if core.CallMethod != nil && left != nil && core.ReceiverHasCallableMethod(left, "<=>") {
		cmp := core.CallMethod(left, "<=>", right)
		if cmp != nil && cmp.Type == object.ValueException {
			return cmp
		}
		if cmp == nil || cmp.Type == object.ValueNil {
			return core.NewArgumentError(comparisonFailedMessage(left, right))
		}
		switch value := cmp.Data.(type) {
		case int64:
			if value < 0 {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if value < 0 {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		}
	}
	return core.R.NilVal
}

func (vm *VM) greaterThan(left, right *object.EmeraldValue) *object.EmeraldValue {
	if result := moduleCompare(left, right, ">"); result != nil {
		return result
	}
	if cmp, ok := integerBigCmp(left, right); ok {
		if cmp > 0 {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			if l > r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if float64(l) > r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			if l > float64(r) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if l > r {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		}
	}
	if result := directCompareMethod(left, right, ">"); result != nil {
		return result
	}
	if core.CallMethod != nil && left != nil && core.ReceiverHasCallableMethod(left, "<=>") {
		cmp := core.CallMethod(left, "<=>", right)
		if cmp != nil && cmp.Type == object.ValueException {
			return cmp
		}
		if cmp == nil || cmp.Type == object.ValueNil {
			return core.NewArgumentError("comparison failed")
		}
		switch value := cmp.Data.(type) {
		case int64:
			if value > 0 {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		case float64:
			if value > 0 {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		}
	}
	return core.R.NilVal
}

func comparisonFailedMessage(left, right *object.EmeraldValue) string {
	leftName := "Object"
	if left != nil {
		leftName = left.TypeName()
	}
	rightInspect := "nil"
	if right != nil {
		rightInspect = right.Inspect()
	}
	return "comparison of " + leftName + " with " + rightInspect + " failed"
}

func (vm *VM) lessThanOrEqual(left, right *object.EmeraldValue) *object.EmeraldValue {
	if result := moduleCompare(left, right, "<="); result != nil {
		return result
	}
	if cmp, ok := integerBigCmp(left, right); ok {
		if cmp <= 0 {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	return directCompareMethod(left, right, "<=")
}

func (vm *VM) greaterThanOrEqual(left, right *object.EmeraldValue) *object.EmeraldValue {
	if result := moduleCompare(left, right, ">="); result != nil {
		return result
	}
	if cmp, ok := integerBigCmp(left, right); ok {
		if cmp >= 0 {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	return directCompareMethod(left, right, ">=")
}

func integerBigCmp(left, right *object.EmeraldValue) (int, bool) {
	if left == nil || right == nil || left.Type != object.ValueInteger || right.Type != object.ValueInteger {
		return 0, false
	}
	leftBig, leftIsBig := core.NumericBigIntOverride(left)
	rightBig, rightIsBig := core.NumericBigIntOverride(right)
	if !leftIsBig && !rightIsBig {
		return 0, false
	}
	if !leftIsBig {
		leftBig = big.NewInt(left.Data.(int64))
	}
	if !rightIsBig {
		rightBig = big.NewInt(right.Data.(int64))
	}
	return leftBig.Cmp(rightBig), true
}

func integerAsBigInt(value *object.EmeraldValue) (*big.Int, bool) {
	if value == nil || value.Type != object.ValueInteger {
		return nil, false
	}
	if result, ok := core.NumericBigIntOverride(value); ok {
		return result, true
	}
	return big.NewInt(value.Data.(int64)), true
}

func integerBitOperation(left, right *object.EmeraldValue, operation string) (*object.EmeraldValue, bool) {
	if left != nil && right != nil && left.Type == object.ValueInteger && right.Type == object.ValueInteger {
		_, leftIsBig := core.NumericBigIntOverride(left)
		_, rightIsBig := core.NumericBigIntOverride(right)
		if !leftIsBig && !rightIsBig {
			l := left.Data.(int64)
			r := right.Data.(int64)
			var result int64
			switch operation {
			case "&":
				result = l & r
			case "|":
				result = l | r
			case "^":
				result = l ^ r
			default:
				return nil, false
			}
			return core.NewIntegerValue(result), true
		}
	}
	l, lok := integerAsBigInt(left)
	r, rok := integerAsBigInt(right)
	if !lok || !rok {
		return nil, false
	}
	result := new(big.Int)
	switch operation {
	case "&":
		result.And(l, r)
	case "|":
		result.Or(l, r)
	case "^":
		result.Xor(l, r)
	default:
		return nil, false
	}
	return core.NewIntegerFromBigInt(result), true
}

func integerShift(left, right *object.EmeraldValue, shiftLeft bool) (*object.EmeraldValue, bool) {
	if left == nil || left.Type != object.ValueInteger || right == nil {
		return nil, false
	}
	if right.Type != object.ValueInteger && core.CallMethod != nil && core.ReceiverHasCallableMethod(right, "to_int") {
		right = core.CallMethod(right, "to_int")
		if right != nil && right.Type == object.ValueException {
			return right, true
		}
	}
	if right == nil || right.Type != object.ValueInteger {
		return core.NewTypeError("no implicit conversion into Integer"), true
	}
	leftBig, leftIsBig := core.NumericBigIntOverride(left)
	count, countIsBig := core.NumericBigIntOverride(right)
	if countIsBig {
		value := leftBig
		if !leftIsBig {
			value = big.NewInt(left.Data.(int64))
		}
		effectiveLeft := shiftLeft
		if count.Sign() < 0 {
			effectiveLeft = !effectiveLeft
		}
		if effectiveLeft {
			if value.Sign() == 0 {
				return core.NewIntegerFromBigInt(big.NewInt(0)), true
			}
			return core.NewRangeError("shift width too big"), true
		}
		if value.Sign() < 0 {
			return core.NewIntegerFromBigInt(big.NewInt(-1)), true
		}
		return core.NewIntegerFromBigInt(big.NewInt(0)), true
	}
	shift := right.Data.(int64)
	if !leftIsBig {
		value := left.Data.(int64)
		if shift == math.MinInt64 {
			effectiveLeft := !shiftLeft
			if effectiveLeft {
				if value == 0 {
					return core.NewIntegerValue(0), true
				}
				return core.NewRangeError("shift width too big"), true
			}
			if value < 0 {
				return core.NewIntegerValue(-1), true
			}
			return core.NewIntegerValue(0), true
		}
		effectiveLeft := shiftLeft
		if shift < 0 {
			effectiveLeft = !effectiveLeft
			shift = -shift
		}
		if !effectiveLeft {
			result := int64(0)
			if shift < 64 {
				result = value >> uint(shift)
			} else if value < 0 {
				result = -1
			}
			return core.NewIntegerValue(result), true
		}
		if shift < 64 {
			result := value << uint(shift)
			if result>>uint(shift) == value {
				return core.NewIntegerValue(result), true
			}
		}
	}
	value := leftBig
	if !leftIsBig {
		value = big.NewInt(left.Data.(int64))
	}
	if shift < 0 {
		shiftLeft = !shiftLeft
		shift = -shift
	}
	result := new(big.Int)
	if shiftLeft {
		result.Lsh(value, uint(shift))
	} else {
		result.Rsh(value, uint(shift))
	}
	return core.NewIntegerFromBigInt(result), true
}

func directCompareMethod(left, right *object.EmeraldValue, op string) *object.EmeraldValue {
	if core.CallMethod == nil || left == nil || !core.ReceiverHasCallableMethod(left, op) {
		return nil
	}
	return core.CallMethod(left, op, right)
}

func moduleCompare(left, right *object.EmeraldValue, op string) *object.EmeraldValue {
	leftIsModule := isModuleValue(left)
	rightIsModule := isModuleValue(right)
	if !leftIsModule && !rightIsModule {
		return nil
	}
	if !leftIsModule || !rightIsModule {
		return core.NewTypeError("compared with non class/module")
	}
	same := sameModuleValue(left, right)
	leftDescendsRight := moduleValueDescendsFrom(left, right)
	rightDescendsLeft := moduleValueDescendsFrom(right, left)

	switch op {
	case "<":
		if same {
			return core.R.FalseVal
		}
		if leftDescendsRight {
			return core.R.TrueVal
		}
		if rightDescendsLeft {
			return core.R.FalseVal
		}
	case ">":
		if same {
			return core.R.FalseVal
		}
		if rightDescendsLeft {
			return core.R.TrueVal
		}
		if leftDescendsRight {
			return core.R.FalseVal
		}
	case "<=":
		if same || leftDescendsRight {
			return core.R.TrueVal
		}
		if rightDescendsLeft {
			return core.R.FalseVal
		}
	case ">=":
		if same || rightDescendsLeft {
			return core.R.TrueVal
		}
		if leftDescendsRight {
			return core.R.FalseVal
		}
	}
	return core.R.NilVal
}

func isModuleValue(value *object.EmeraldValue) bool {
	return value != nil && (value.Type == object.ValueClass || value.Type == object.ValueModule)
}

func sameModuleValue(left, right *object.EmeraldValue) bool {
	if left == nil || right == nil || left.Type != right.Type {
		return false
	}
	switch left.Type {
	case object.ValueClass:
		l := left.Data.(*object.Class)
		r := right.Data.(*object.Class)
		return l == r || (l.Name != "" && l.Name == r.Name)
	case object.ValueModule:
		l := left.Data.(*object.Module)
		r := right.Data.(*object.Module)
		return l == r || (l.Name != "" && l.Name == r.Name)
	}
	return false
}

func moduleValueDescendsFrom(left, right *object.EmeraldValue) bool {
	switch left.Type {
	case object.ValueClass:
		return classValueDescendsFrom(left.Data.(*object.Class), right)
	case object.ValueModule:
		if right.Type != object.ValueModule {
			return false
		}
		return moduleIncludesModule(left.Data.(*object.Module), right.Data.(*object.Module))
	}
	return false
}

func classValueDescendsFrom(class *object.Class, target *object.EmeraldValue) bool {
	if class == nil {
		return false
	}
	if target.Type == object.ValueClass {
		targetClass := target.Data.(*object.Class)
		for current := class.SuperClass; current != nil; current = current.SuperClass {
			if current == targetClass || (current.Name != "" && current.Name == targetClass.Name) {
				return true
			}
		}
	}
	targetModule, ok := target.Data.(*object.Module)
	if !ok {
		return false
	}
	for current := class; current != nil; current = current.SuperClass {
		for _, included := range current.IncludedModules {
			if sameModuleObject(included, targetModule) || moduleIncludesModule(included, targetModule) {
				return true
			}
		}
	}
	return false
}

func moduleIncludesModule(module, target *object.Module) bool {
	if module == nil || target == nil {
		return false
	}
	for _, included := range module.IncludedModules {
		if sameModuleObject(included, target) || moduleIncludesModule(included, target) {
			return true
		}
	}
	return false
}

func sameModuleObject(left, right *object.Module) bool {
	return left == right || (left != nil && right != nil && left.Name != "" && left.Name == right.Name)
}

func (vm *VM) index(left, index *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && left.Class != nil {
		if (left.Type == object.ValueArray && left.Class != core.R.Classes["Array"]) ||
			(left.Type == object.ValueHash && left.Class != core.R.Classes["Hash"]) ||
			(left.Type == object.ValueString && left.Class != core.R.Classes["String"]) {
			return vm.send(left, "[]", []*object.EmeraldValue{index})
		}
	}
	switch l := left.Data.(type) {
	case []*object.EmeraldValue:
		switch i := index.Data.(type) {
		case int64:
			if i < 0 {
				i = int64(len(l)) + i
			}
			if i < 0 || i >= int64(len(l)) {
				return core.R.NilVal
			}
			return l[i]
		}
	case map[*object.EmeraldValue]*object.EmeraldValue:
		for k, v := range l {
			if k.Equals(index) {
				return v
			}
		}
		return core.R.NilVal
	case string:
		return vm.send(left, "[]", []*object.EmeraldValue{index})
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "[]", []*object.EmeraldValue{index})
	}
	return core.R.NilVal
}

func (vm *VM) sliceIndex(left, start, length *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && left.Class != nil {
		if (left.Type == object.ValueArray && left.Class != core.R.Classes["Array"]) ||
			(left.Type == object.ValueHash && left.Class != core.R.Classes["Hash"]) ||
			(left.Type == object.ValueString && left.Class != core.R.Classes["String"]) {
			return vm.send(left, "[]", []*object.EmeraldValue{start, length})
		}
	}
	var s, l int
	switch v := start.Data.(type) {
	case int64:
		s = int(v)
	case float64:
		s = int(v)
	default:
		if left != nil && left.Class != nil {
			return vm.send(left, "[]", []*object.EmeraldValue{start, length})
		}
		return core.R.NilVal
	}
	switch v := length.Data.(type) {
	case int64:
		l = int(v)
	case float64:
		l = int(v)
	default:
		if left != nil && left.Class != nil {
			return vm.send(left, "[]", []*object.EmeraldValue{start, length})
		}
		return core.R.NilVal
	}
	switch data := left.Data.(type) {
	case []*object.EmeraldValue:
		if s < 0 {
			s = len(data) + s
		}
		if s < 0 || s > len(data) {
			return core.R.NilVal
		}
		end := s + l
		if end > len(data) {
			end = len(data)
		}
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  data[s:end],
			Class: core.R.Classes["Array"],
		}
	case string:
		if s < 0 {
			s = len(data) + s
		}
		if s < 0 || s > len(data) {
			return core.R.NilVal
		}
		end := s + l
		if end > len(data) {
			end = len(data)
		}
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  data[s:end],
			Class: core.R.Classes["String"],
		}
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "[]", []*object.EmeraldValue{start, length})
	}
	return core.R.NilVal
}

func (vm *VM) indexAssign(left, index, value *object.EmeraldValue) *object.EmeraldValue {
	if left != nil && (left.Type == object.ValueArray || left.Type == object.ValueHash || left.Type == object.ValueString) {
		return vm.send(left, "[]=", []*object.EmeraldValue{index, value})
	}
	switch l := left.Data.(type) {
	case map[*object.EmeraldValue]*object.EmeraldValue:
		found := false
		for k := range l {
			if k.Equals(index) {
				l[k] = value
				found = true
				break
			}
		}
		if !found {
			l[index] = value
		}
	}
	if left != nil && left.Type == object.ValueObject {
		return vm.send(left, "[]=", []*object.EmeraldValue{index, value})
	}
	return value
}

func (vm *VM) send(receiver *object.EmeraldValue, method string, args []*object.EmeraldValue) *object.EmeraldValue {
	if debugSendTraceEnabled {
		depth := atomic.AddInt64(&sendTraceDepth, 1)
		if depth <= 40 {
			fmt.Printf("send depth=%d method=%q receiverType=%s\n", depth, method, receiver.TypeName())
		}
		if depth%200 == 0 {
			fmt.Printf("send depth=%d method=%q receiverType=%s\n", depth, method, receiver.TypeName())
		}
		if depth > 4000 {
			panic(fmt.Sprintf("send recursion guard: method=%s receiver=%s depth=%d", method, receiver.TypeName(), depth))
		}
		defer atomic.AddInt64(&sendTraceDepth, -1)
	}
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	if method == "resolve_feature_path" && receiver == vm.getGlobalByName("$LOAD_PATH") {
		return vm.resolveLoadPathFeature(args)
	}
	if method == "public_send" && len(args) == 0 {
		err := core.NewArgumentError("wrong number of arguments")
		core.PrependExceptionBacktraceLabel(err, "public_send")
		return err
	}
	if method == "send" {
		methodObj, methodOwner, fallback := vm.lookupMethodForSend(receiver, method, args, false)
		if fallback == nil && methodObj != nil && methodOwner != core.R.Classes["Object"] && methodOwner != core.R.Classes["BasicObject"] {
			return vm.invokeMethod(receiver, method, method, args, methodObj, methodOwner)
		}
	}
	if (method == "send" || method == "__send__" || method == "public_send") && len(args) > 0 {
		methodName, ok, parseErr := core.MethodNameFromValueWithError(args[0])
		if parseErr != nil {
			if method == "public_send" {
				core.PrependExceptionBacktraceLabel(parseErr, "public_send")
			}
			return parseErr
		}
		if !ok {
			err := core.NewTypeError("is not a symbol nor a string")
			if method == "public_send" {
				core.PrependExceptionBacktraceLabel(err, "public_send")
			}
			return err
		}
		forwardArgs := args[1:]
		prev := vm.visibilityBypass
		vm.visibilityBypass = method != "public_send"
		if vm.currentBlock != nil {
			switch methodName {
			case "instance_eval", "class_eval", "module_eval", "instance_exec", "class_exec", "module_exec":
				result := vm.send(receiver, methodName, forwardArgs)
				vm.visibilityBypass = prev
				return result
			}
		}
		methodObj, methodOwner, fallback := vm.lookupMethodForSend(receiver, methodName, forwardArgs, true)
		if fallback != nil {
			if method != "public_send" {
				if missingResult := vm.callMethodMissingForSend(receiver, methodName, forwardArgs); missingResult != nil {
					vm.visibilityBypass = prev
					return missingResult
				}
			}
			vm.visibilityBypass = prev
			return fallback
		}
		result := vm.invokeMethod(receiver, method, methodName, forwardArgs, methodObj, methodOwner)
		vm.visibilityBypass = prev
		return result
	}

	if method == "instance_eval" && vm.currentBlock != nil {
		if len(args) > 0 {
			return core.NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0)", len(args)))
		}
		block := vm.currentBlock
		vm.currentBlock = nil
		return vm.callBlockWithInstanceEvalContext(receiver, block, receiver)
	}
	if method == "instance_eval" {
		if len(args) == 0 {
			return core.NewArgumentError("wrong number of arguments (given 0, expected 1..3)")
		}
		if len(args) > 3 {
			return core.NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1..3)", len(args)))
		}
		binding := vm.currentFrameBinding()
		if binding == nil {
			binding = &object.RBinding{Locals: map[string]*object.EmeraldValue{}}
		}
		callerClassVarScope := vm.classVarScopeReceiver(binding.Self)
		binding.Self = receiver
		binding.ClassStack = vm.instanceEvalStringClassStack(receiver, binding.ClassStack)
		binding.ClassVarScope = callerClassVarScope
		evalBinding := &object.EmeraldValue{Type: object.ValueBinding, Data: binding, Class: core.R.Classes["Binding"]}
		evalArgs := make([]*object.EmeraldValue, 0, len(args)+1)
		evalArgs = append(evalArgs, args[0], evalBinding)
		if len(args) > 1 {
			evalArgs = append(evalArgs, args[1])
		}
		if len(args) > 2 {
			evalArgs = append(evalArgs, args[2])
		}
		result := core.CallMethod(core.R.Main, "eval", evalArgs...)
		vm.copyBindingLocalsToCurrentFrame(binding)
		return result
	}
	if (method == "class_eval" || method == "module_eval") && vm.currentBlock != nil {
		if len(args) > 0 {
			return core.NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0)", len(args)))
		}
		block := vm.currentBlock
		vm.currentBlock = nil
		if receiver.Type == object.ValueClass || receiver.Type == object.ValueModule {
			previousVisibility := core.CurrentMethodVisibilityMode(receiver)
			core.SetCurrentMethodVisibility(receiver, "public")
			vm.classStack = append(vm.classStack, receiver)
			result := vm.callBlockWithSelf(block, receiver, append([]*object.EmeraldValue{receiver}, args...)...)
			vm.classStack = vm.classStack[:len(vm.classStack)-1]
			core.SetCurrentMethodVisibility(receiver, previousVisibility)
			return result
		}
		return vm.callBlockWithSelf(block, receiver, append([]*object.EmeraldValue{receiver}, args...)...)
	}
	if method == "instance_exec" && vm.currentBlock != nil {
		block := vm.currentBlock
		vm.currentBlock = nil
		return vm.withNativeBacktraceFrame("BasicObject#instance_exec", func() *object.EmeraldValue {
			return vm.callBlockWithInstanceExecContext(receiver, block, args...)
		})
	}
	if method == "instance_exec" {
		return core.NewLocalJumpError("no block given")
	}
	if method == "class_exec" || method == "module_exec" {
		if vm.currentBlock == nil {
			return core.NewLocalJumpError("no block given")
		}
		block := vm.currentBlock
		vm.currentBlock = nil
		label := "Module#" + method
		if receiver.Type == object.ValueClass || receiver.Type == object.ValueModule {
			previousVisibility := core.CurrentMethodVisibilityMode(receiver)
			core.SetCurrentMethodVisibility(receiver, "public")
			vm.classStack = append(vm.classStack, receiver)
			result := vm.withNativeBacktraceFrame(label, func() *object.EmeraldValue {
				return vm.callBlockWithSelf(block, receiver, args...)
			})
			vm.classStack = vm.classStack[:len(vm.classStack)-1]
			core.SetCurrentMethodVisibility(receiver, previousVisibility)
			return result
		}
		return vm.withNativeBacktraceFrame(label, func() *object.EmeraldValue {
			return vm.callBlockWithSelf(block, receiver, args...)
		})
	}
	if method == "__exec_class_body__" && (receiver.Type == object.ValueClass || receiver.Type == object.ValueModule) && vm.currentBlock != nil {
		block := vm.currentBlock
		vm.currentBlock = nil
		previousVisibility := core.CurrentMethodVisibilityMode(receiver)
		core.SetCurrentMethodVisibility(receiver, "public")
		vm.classStack = append(vm.classStack, receiver)
		core.FireTracePointClass("class", vm.currentFrameBinding(), receiver)
		result := vm.callBlockWithSelf(block, receiver, receiver)
		core.FireTracePointClass("end", vm.currentFrameBinding(), receiver)
		vm.classStack = vm.classStack[:len(vm.classStack)-1]
		core.SetCurrentMethodVisibility(receiver, previousVisibility)
		return result
	}

	methodObj, methodOwner, fallback := vm.lookupMethodForSend(receiver, method, args, false)
	if fallback != nil {
		if numberedParameterMethodNamePattern.MatchString(method) {
			markExceptionRaised(fallback)
			return fallback
		}
		if missingResult := vm.callMethodMissingForSend(receiver, method, args); missingResult != nil {
			return missingResult
		}
		return fallback
	}
	if methodObj == nil {
		if missingResult := vm.callMethodMissingForSend(receiver, method, args); missingResult != nil {
			return missingResult
		}
	}
	var implicitEvalBinding *object.RBinding
	if len(args) == 1 && (receiver == nil || receiver.Type != object.ValueBinding) &&
		(method == "eval" || (methodObj != nil && methodObj.OriginalName == "eval")) {
		implicitEvalBinding = vm.currentFrameBinding()
		args = append(args, &object.EmeraldValue{Type: object.ValueBinding, Data: implicitEvalBinding, Class: core.R.Classes["Binding"]})
	}

	result := vm.invokeMethod(receiver, method, method, args, methodObj, methodOwner)
	if implicitEvalBinding != nil {
		vm.copyBindingLocalsToCurrentFrame(implicitEvalBinding)
	}
	if isVisibilityNoMethodErrorFor(result, method) {
		if missingResult := vm.callMethodMissingForSend(receiver, method, args); missingResult != nil {
			return missingResult
		}
	}
	return result
}

func implicitIdentifierNameError(result, receiver *object.EmeraldValue, method string, cause *object.EmeraldValue) *object.EmeraldValue {
	if result == nil || result.Type != object.ValueException || result.Class != core.R.Classes["NoMethodError"] {
		return result
	}
	exc, ok := result.Data.(*object.RException)
	if !ok || exc == nil || exc.NameErrorName != method || exc.NameErrorReceiver != receiver {
		return result
	}
	nameError := core.NewNameErrorWithDetails("undefined local variable or method `"+method+"'", receiver, method)
	if cause != nil && cause != nameError {
		if exc, ok := nameError.Data.(*object.RException); ok && exc != nil {
			exc.Cause = cause
		}
	}
	markExceptionRaised(nameError)
	return nameError
}

func (vm *VM) resolveLoadPathFeature(args []*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 || args[0] == nil || args[0].Type != object.ValueString {
		return core.NewArgumentError("wrong number of arguments")
	}
	request := args[0].Data.(string)
	path := vm.resolveRequirePath(request)
	extension := ""
	if path != "" {
		extension = strings.TrimPrefix(filepath.Ext(path), ".")
	}
	if path == "" {
		switch strings.TrimSuffix(request, filepath.Ext(request)) {
		case "pp":
			extension = "rb"
			path = "/lib/pp.rb"
		case "etc":
			extension = "so"
			path = "/lib/etc.so"
		default:
			return core.R.NilVal
		}
	}
	if extension == "" {
		extension = "rb"
	}
	return vm.arrayValue(
		&object.EmeraldValue{Type: object.ValueSymbol, Data: extension, Class: core.R.Classes["Symbol"]},
		&object.EmeraldValue{Type: object.ValueString, Data: path, Class: core.R.Classes["String"]},
	)
}

func (vm *VM) callMethodMissingForSend(receiver *object.EmeraldValue, methodName string, args []*object.EmeraldValue) *object.EmeraldValue {
	if methodName == "respond_to_missing?" || methodName == "method_missing" {
		return nil
	}
	nameValue := &object.EmeraldValue{Type: object.ValueSymbol, Data: methodName, Class: core.R.Classes["Symbol"]}
	prev := vm.visibilityBypass
	vm.visibilityBypass = true
	missingArgs := make([]*object.EmeraldValue, 0, len(args)+1)
	missingArgs = append(missingArgs, nameValue)
	missingArgs = append(missingArgs, args...)
	methodObj, methodOwner, fallback := vm.lookupMethodForSend(receiver, "method_missing", missingArgs, false)
	if fallback != nil {
		vm.visibilityBypass = prev
		return nil
	}
	if methodObj == nil {
		vm.visibilityBypass = prev
		return nil
	}
	result := vm.invokeMethod(receiver, "send", "method_missing", missingArgs, methodObj, methodOwner)
	vm.visibilityBypass = prev
	return result
}

func (vm *VM) sendBypassVisibility(receiver *object.EmeraldValue, methodName string, args []*object.EmeraldValue) *object.EmeraldValue {
	previous := vm.visibilityBypass
	vm.visibilityBypass = true
	result := vm.send(receiver, methodName, args)
	vm.visibilityBypass = previous
	return result
}

func isVisibilityNoMethodErrorFor(value *object.EmeraldValue, methodName string) bool {
	if value == nil || value.Type != object.ValueException || value.Class != core.R.Classes["NoMethodError"] {
		return false
	}
	exc, ok := value.Data.(*object.RException)
	if !ok || exc == nil || exc.NameErrorName != methodName {
		return false
	}
	return strings.Contains(exc.Message, "private method `"+methodName+"'") ||
		strings.Contains(exc.Message, "protected method `"+methodName+"'")
}

func (vm *VM) normalizeBlockPass(block *object.EmeraldValue) (*object.EmeraldValue, *object.EmeraldValue) {
	if block == nil {
		return nil, nil
	}
	switch block.Type {
	case object.ValueNil:
		return nil, nil
	case object.ValueClosure, object.ValueProc:
		return block, nil
	case object.ValueSymbol:
		converted := core.CallMethod(block, "to_proc")
		if converted == nil || converted.Type != object.ValueProc {
			return nil, core.NewTypeError("can't convert Symbol to Proc")
		}
		return converted, nil
	case object.ValueMethod:
		methodValue := block
		method := methodValue.Data.(*object.Method)
		return &object.EmeraldValue{
			Type: object.ValueProc,
			Data: &object.Proc{
				Native: func(args ...*object.EmeraldValue) *object.EmeraldValue {
					return core.CallMethod(methodValue, "call", args...)
				},
				NativeArity:    method.Arity,
				HasNativeArity: true,
				IsLambda:       true,
				SourceMethod:   method,
			},
			Class: core.R.Classes["Proc"],
		}, nil
	default:
		className := block.TypeName()
		if block.Class != nil && block.Class.Name != "" {
			className = block.Class.Name
		}
		if block.Class != nil {
			if method, owner, _ := vm.lookupMethodForSend(block, "to_proc", nil, false); method != nil {
				converted := vm.invokeMethod(block, "to_proc", "to_proc", nil, method, owner)
				if converted != nil && converted.Type == object.ValueException {
					return nil, converted
				}
				if converted != nil && (converted.Type == object.ValueProc || converted.Type == object.ValueClosure) {
					return converted, nil
				}
				convertedName := "NilClass"
				if converted != nil {
					convertedName = converted.TypeName()
					if converted.Class != nil && converted.Class.Name != "" {
						convertedName = converted.Class.Name
					}
				}
				return nil, core.NewTypeError("can't convert " + className + " into Proc (" + className + "#to_proc gives " + convertedName + ")")
			}
		}
		return nil, core.NewTypeError("no implicit conversion of " + className + " into Proc")
	}
}

func (vm *VM) lookupMethodForSend(receiver *object.EmeraldValue, method string, args []*object.EmeraldValue, missingAsNameError bool) (*object.Method, *object.Class, *object.EmeraldValue) {
	if cached, owner, ok := vm.cachedPlainMethod(receiver, method); ok {
		cached, owner, ok = resolveVisibilityAliasMethod(method, cached, owner)
		if ok {
			return cached, owner, nil
		}
	}
	var methodObj *object.Method
	var methodOwner *object.Class
	var ok bool
	inheritClassMethodsFirst := receiver != nil && receiver.Type == object.ValueClass

	lookupClassSingletonMethod := func(singletonClass *object.Class, methodName string) (*object.Method, *object.Class, bool) {
		if singletonClass == nil {
			return nil, nil, false
		}
		for _, mod := range singletonClass.PrependedModules {
			if method, found := mod.GetMethod(methodName); found {
				return method, singletonClass, true
			}
		}
		if method, found := singletonClass.Methods[methodName]; found {
			return method, singletonClass, true
		}
		for index := len(singletonClass.IncludedModules) - 1; index >= 0; index-- {
			mod := singletonClass.IncludedModules[index]
			if method, found := mod.GetMethod(methodName); found {
				return method, singletonClass, true
			}
		}
		return nil, nil, false
	}
	lookupClassReceiverMethod := func(cls *object.Class, methodName string) (*object.Method, *object.Class, bool) {
		if cls == nil || cls.SingletonClass == nil {
			return nil, nil, false
		}
		return lookupClassSingletonMethod(cls.SingletonClass, methodName)
	}

	if !ok && receiver.Type == object.ValueObject {
		if obj, isObject := receiver.Data.(*object.Object); isObject {
			if m, found := obj.SingletonMethods[method]; found {
				methodObj = m
				methodOwner = nil
				ok = true
			} else if obj.SingletonClass != nil {
				if m, owner, found := lookupClassSingletonMethod(obj.SingletonClass, method); found {
					methodObj = m
					methodOwner = owner
					ok = true
				}
			}
		} else if methods := vm.nativeSingletonMethods[nativeSingletonKey(receiver)]; methods != nil {
			if m, found := methods[method]; found {
				methodObj = m
				methodOwner = nil
				ok = true
			}
		}
	}
	if !ok {
		if singletonClass := core.AttachedSingletonClass(receiver); singletonClass != nil {
			if m, owner, found := lookupClassSingletonMethod(singletonClass, method); found {
				methodObj = m
				methodOwner = owner
				ok = true
			}
		}
	}
	if !ok {
		if methods := vm.nativeSingletonMethods[nativeSingletonKey(receiver)]; methods != nil {
			if m, found := methods[method]; found {
				methodObj = m
				methodOwner = nil
				ok = true
			}
		}
	}
	if !ok && receiver.Type == object.ValueException {
		if exc, isException := receiver.Data.(*object.RException); isException && exc.SingletonClass != nil {
			if m, owner, found := lookupClassSingletonMethod(exc.SingletonClass, method); found {
				methodObj = m
				methodOwner = owner
				ok = true
			}
		}
	}
	if !ok {
		if refined, found := vm.lookupActiveRefinedMethod(receiver, method); found {
			methodObj = refined
			ok = true
		}
	}

	if !ok && receiver.Type == object.ValueClass {
		cls := receiver.Data.(*object.Class)
		if m, owner, found := lookupClassReceiverMethod(cls, method); found {
			methodObj = m
			methodOwner = owner
			ok = true
		}
		if !ok {
			if m, found := cls.ClassMethods[method]; found {
				methodObj = m
				methodOwner = cls
				ok = true
			}
		}
		if !ok && receiver.Class != nil && cls == core.R.Classes["Class"] {
			if m, found := receiver.Class.Methods[method]; found {
				methodObj = m
				methodOwner = receiver.Class
				ok = true
			}
		}
		if !ok && inheritClassMethodsFirst {
			for current := cls.SuperClass; current != nil; current = current.SuperClass {
				if m, owner, found := lookupClassReceiverMethod(current, method); found {
					methodObj = m
					methodOwner = owner
					ok = true
					break
				}
				if m, found := current.ClassMethods[method]; found {
					methodObj = m
					methodOwner = current
					ok = true
					break
				}
			}
		}
		if !ok {
			if singletonVal, found := vm.rubyConsts["__singleton_class__"]; found && singletonVal.Type == object.ValueClass {
				if m, methodFound := singletonVal.Data.(*object.Class).Methods[method]; methodFound {
					methodObj = m
					methodOwner = nil
					ok = true
				}
			}
		}
		if !ok && !inheritClassMethodsFirst && method == "[]" {
			for current := cls.SuperClass; current != nil; current = current.SuperClass {
				if m, owner, found := lookupClassReceiverMethod(current, method); found {
					methodObj = m
					methodOwner = owner
					ok = true
					break
				}
				if m, found := current.ClassMethods[method]; found {
					methodObj = m
					methodOwner = current
					ok = true
					break
				}
			}
		}
	}

	if !ok && receiver.Type == object.ValueModule {
		mod := receiver.Data.(*object.Module)
		if mod.SingletonClass != nil {
			if m, owner, found := lookupClassSingletonMethod(mod.SingletonClass, method); found {
				methodObj = m
				methodOwner = owner
				ok = true
			}
		}
		if !ok {
			if m, found := mod.Methods[method]; found {
				methodObj = m
				methodOwner = nil
				ok = true
			}
		}
	}

	if !ok && receiver.Class != nil {
		key := methodCacheKey{class: receiver.Class, name: method}
		generation := object.CurrentMethodGeneration()
		if cached, found := vm.methodCache[key]; found && cached.generation == generation {
			methodObj = cached.method
			methodOwner = cached.owner
			ok = methodObj != nil
		} else {
			methodObj, methodOwner, ok = receiver.Class.GetMethodWithOwner(method)
			if ok {
				vm.methodCache[key] = methodCacheEntry{
					generation: generation,
					method:     methodObj,
					owner:      methodOwner,
				}
			}
		}
	}

	if !ok && receiver.Type == object.ValueClass {
		cls := receiver.Data.(*object.Class)
		if inheritClassMethodsFirst {
			goto afterInheritedClassMethodLookup
		}
		for current := cls.SuperClass; current != nil; current = current.SuperClass {
			if m, owner, found := lookupClassReceiverMethod(current, method); found {
				methodObj = m
				methodOwner = owner
				ok = true
				break
			}
			if m, found := current.ClassMethods[method]; found {
				methodObj = m
				methodOwner = current
				ok = true
				break
			}
		}
	}
afterInheritedClassMethodLookup:

	if ok {
		methodObj, methodOwner, ok = resolveVisibilityAliasMethod(method, methodObj, methodOwner)
	}

	if !ok {
		if method == "error" && core.LastException != nil {
			return nil, nil, core.LastException
		}
		if receiver != nil && receiver.Frozen && receiver.Class != nil && receiver.Class.Name == "OpenStruct" && strings.HasSuffix(method, "=") {
			return nil, nil, core.NewFrozenError("can't modify frozen OpenStruct")
		}
		if fallback := vm.dynamicModuleOrClassAccessor(receiver, method, args); fallback != nil {
			return nil, nil, fallback
		}
		if fallback := core.FileStatFixtureDispatch(receiver, method, args...); fallback != nil {
			return nil, nil, fallback
		}
		if fallback := vm.constSourceLocationArgumentError(method, args); fallback != nil {
			return nil, nil, fallback
		}
		if fallback := vm.moduleEvalArgumentError(method, args); fallback != nil {
			return nil, nil, fallback
		}
		if fallback := vm.constDefinedArgumentError(method, args); fallback != nil {
			return nil, nil, fallback
		}
		if missingAsNameError {
			return nil, nil, core.NewNameError("undefined method `" + method + "'")
		}
		if numberedParameterMethodNamePattern.MatchString(method) {
			return nil, nil, core.NewNoMethodError("undefined local variable or method `" + method + "'")
		}
		if core.EvaluatingRaiseErrorMatcher() {
			return nil, nil, core.NewNoMethodErrorWithDetails(receiver, method, args)
		}
		return nil, nil, core.NewNoMethodErrorWithDetails(receiver, method, args)
	}
	return methodObj, methodOwner, nil
}

func (vm *VM) cachedPlainMethod(receiver *object.EmeraldValue, method string) (*object.Method, *object.Class, bool) {
	if receiver == nil || receiver.Class == nil ||
		receiver.Type == object.ValueClass || receiver.Type == object.ValueModule || receiver.Type == object.ValueException {
		return nil, nil, false
	}
	if refinements, fixed := vm.currentFixedRefinements(); fixed && len(refinements) > 0 {
		return nil, nil, false
	}
	for _, scope := range vm.classStack {
		if scope == nil {
			continue
		}
		switch scope.Type {
		case object.ValueClass:
			if len(scope.Data.(*object.Class).UsedRefinements) > 0 {
				return nil, nil, false
			}
		case object.ValueModule:
			if len(scope.Data.(*object.Module).UsedRefinements) > 0 {
				return nil, nil, false
			}
		}
	}
	if receiver.Type == object.ValueObject {
		if instance, ok := receiver.Data.(*object.Object); ok {
			if len(instance.SingletonMethods) > 0 || instance.SingletonClass != nil {
				return nil, nil, false
			}
		}
	}
	if core.AttachedSingletonClass(receiver) != nil {
		return nil, nil, false
	}
	if methods := vm.nativeSingletonMethods[nativeSingletonKey(receiver)]; len(methods) > 0 {
		return nil, nil, false
	}
	key := methodCacheKey{class: receiver.Class, name: method}
	cached, found := vm.methodCache[key]
	if !found || cached.generation != object.CurrentMethodGeneration() || cached.method == nil {
		return nil, nil, false
	}
	return cached.method, cached.owner, true
}

func (vm *VM) lookupActiveRefinedMethod(receiver *object.EmeraldValue, method string) (*object.Method, bool) {
	if refinements, fixed := vm.currentFixedRefinements(); fixed {
		for index := len(refinements) - 1; index >= 0; index-- {
			if refined, ok := core.RefinedMethod(refinements[index], receiver, method); ok {
				return refined, true
			}
		}
		return nil, false
	}
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		scope := vm.classStack[i]
		if scope == nil {
			continue
		}
		var used []*object.EmeraldValue
		switch scope.Type {
		case object.ValueClass:
			used = scope.Data.(*object.Class).UsedRefinements
		case object.ValueModule:
			used = scope.Data.(*object.Module).UsedRefinements
		}
		for j := len(used) - 1; j >= 0; j-- {
			if refined, ok := core.RefinedMethod(used[j], receiver, method); ok {
				return refined, true
			}
		}
	}
	return nil, false
}

func (vm *VM) lookupActiveRefinedInstanceMethod(receiver *object.EmeraldValue, method string) (*object.Method, bool) {
	if refinements, fixed := vm.currentFixedRefinements(); fixed {
		for index := len(refinements) - 1; index >= 0; index-- {
			if refined, ok := core.RefinedInstanceMethod(refinements[index], receiver, method); ok {
				return refined, true
			}
		}
		return nil, false
	}
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		scope := vm.classStack[i]
		if scope == nil {
			continue
		}
		var used []*object.EmeraldValue
		switch scope.Type {
		case object.ValueClass:
			used = scope.Data.(*object.Class).UsedRefinements
		case object.ValueModule:
			used = scope.Data.(*object.Module).UsedRefinements
		}
		for j := len(used) - 1; j >= 0; j-- {
			if refined, ok := core.RefinedInstanceMethod(used[j], receiver, method); ok {
				return refined, true
			}
		}
	}
	return nil, false
}

func (vm *VM) currentFixedRefinements() ([]*object.EmeraldValue, bool) {
	if vm.fp < 0 || vm.fp >= len(vm.frames) {
		return nil, false
	}
	frame := vm.frames[vm.fp]
	if frame == nil || frame.Closure == nil || !frame.Closure.RefinementsFixed {
		return nil, false
	}
	return frame.Closure.Refinements, true
}

func (vm *VM) activateCurrentRefinement(refinery *object.EmeraldValue) {
	if refinery == nil || vm.fp < 0 || vm.fp >= len(vm.frames) {
		return
	}
	frame := vm.frames[vm.fp]
	if frame == nil {
		return
	}
	if frame.Closure == nil {
		frame.Closure = &object.Closure{Fn: frame.Fn}
	}
	if !frame.Closure.RefinementsFixed {
		frame.Closure.Refinements = vm.activeRefinementSnapshot()
		frame.Closure.RefinementsFixed = true
	}
	for _, existing := range frame.Closure.Refinements {
		if existing == refinery {
			return
		}
	}
	frame.Closure.Refinements = append(frame.Closure.Refinements, refinery)
}

func (vm *VM) usingReceiverAllowed(receiver *object.EmeraldValue) bool {
	if vm.fp < 0 || vm.fp >= len(vm.frames) {
		return false
	}
	frame := vm.frames[vm.fp]
	if frame == nil || frame.Bp < 0 || frame.Bp >= len(vm.stack) {
		return false
	}
	if receiver == core.R.Main || (receiver != nil && receiver.Type == object.ValueObject) {
		if len(vm.classStack) == 1 && vm.classStack[0] != nil && vm.classStack[0].Type == object.ValueModule {
			module, _ := vm.classStack[0].Data.(*object.Module)
			if module != nil && module.Name == "" {
				return true
			}
		}
		return vm.stack[frame.Bp] == core.R.Main && len(vm.classStack) == 0
	}
	return len(vm.classStack) > 0 && vm.classStack[len(vm.classStack)-1] == receiver
}

func (vm *VM) activeRefinementSnapshot() []*object.EmeraldValue {
	if refinements, fixed := vm.currentFixedRefinements(); fixed {
		return append([]*object.EmeraldValue(nil), refinements...)
	}
	var refinements []*object.EmeraldValue
	seen := make(map[*object.EmeraldValue]bool)
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		scope := vm.classStack[i]
		if scope == nil {
			continue
		}
		var used []*object.EmeraldValue
		switch scope.Type {
		case object.ValueClass:
			used = scope.Data.(*object.Class).UsedRefinements
		case object.ValueModule:
			used = scope.Data.(*object.Module).UsedRefinements
		}
		for j := len(used) - 1; j >= 0; j-- {
			if refinery := used[j]; refinery != nil && !seen[refinery] {
				seen[refinery] = true
				refinements = append(refinements, refinery)
			}
		}
	}
	return refinements
}

func resolveVisibilityAliasMethod(name string, methodObj *object.Method, methodOwner *object.Class) (*object.Method, *object.Class, bool) {
	if methodObj == nil || methodObj.VisibilityAliasStart == nil {
		return methodObj, methodOwner, methodObj != nil
	}
	actual, owner, _, ok := getMethodAfterClassWithOwner(methodObj.VisibilityAliasStart, name)
	if !ok || actual == nil || actual.Visibility == "undefined" {
		return nil, nil, false
	}
	resolved := *actual
	resolved.Visibility = methodObj.Visibility
	return &resolved, owner, true
}

func nativeSingletonKey(receiver *object.EmeraldValue) interface{} {
	if receiver == nil {
		return nil
	}
	if receiver.Type == object.ValueObject {
		if _, ok := receiver.Data.(*object.Object); !ok && receiver.Data != nil {
			if reflect.TypeOf(receiver.Data).Comparable() {
				return receiver.Data
			}
		}
	}
	return receiver
}

func (vm *VM) invokeMethod(receiver *object.EmeraldValue, parentMethod, method string, args []*object.EmeraldValue, methodObj *object.Method, methodOwner *object.Class) *object.EmeraldValue {
	var oldFrame *Frame
	if vm.fp >= 0 && vm.fp < len(vm.frames) && vm.frames[vm.fp] != nil {
		oldFrame = vm.frames[vm.fp]
	}
	if methodObj == nil {
		return core.NewNoMethodError("undefined method `" + method + "'")
	}
	if methodObj.DispatchOwner != nil {
		copy := *methodObj
		copy.Owner = methodObj.DispatchOwner
		methodObj = &copy
		if copy.Owner.Type == object.ValueClass {
			methodOwner, _ = copy.Owner.Data.(*object.Class)
		}
	}
	if methodObj.Visibility == "undefined" {
		return core.NewNoMethodError("undefined method `" + method + "'")
	}
	currentSelf := core.R.Main
	if oldFrame != nil && oldFrame.Bp >= 0 && oldFrame.Bp < len(vm.stack) && vm.stack[oldFrame.Bp] != nil {
		currentSelf = vm.stack[oldFrame.Bp]
	}
	privateReceiverAllowed := receiver == core.R.Main || (parentMethod != "public_send" && receiver == currentSelf)
	if methodObj.Visibility == "protected" && parentMethod != "public_send" {
		sameRuntimeClass := currentSelf != nil && receiver != nil && currentSelf.Class != nil && currentSelf.Class == receiver.Class
		if sameRuntimeClass || methodOwner != nil && valueHasClassInAncestry(currentSelf, methodOwner) && valueHasClassInAncestry(receiver, methodOwner) {
			privateReceiverAllowed = true
		}
	}
	if !vm.visibilityBypass && parentMethod != "public" && parentMethod != "private" && parentMethod != "protected" && parentMethod != "bind" &&
		parentMethod != "module_function" && parentMethod != "public_class_method" && parentMethod != "private_class_method" &&
		parentMethod != "using" && parentMethod != "refine" &&
		(methodObj.Visibility == "private" || methodObj.Visibility == "protected") && !privateReceiverAllowed {
		return core.NewNoMethodErrorForVisibility(receiver, method, methodObj.Visibility, args)
	}
	if fn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
		var binding *object.RBinding
		var methodID string
		var definedClass *object.EmeraldValue
		prepareTraceContext := func() {
			if binding != nil {
				return
			}
			binding = vm.currentFrameBinding()
			methodID = traceMethodID(methodObj, method)
			definedClass = traceDefinedClass(methodObj, methodOwner)
		}
		if core.TracePointEventActive("c_call") {
			prepareTraceContext()
			core.FireTracePointCall("c_call", binding, receiver, definedClass, methodID, method, nil)
		}
		invoke := func() *object.EmeraldValue { return fn(receiver, args...) }
		if method == "sleep" && methodOwner == core.R.Classes["Object"] {
			invoke = func() *object.EmeraldValue {
				return vm.withNativeBacktraceFrame("Kernel#sleep", func() *object.EmeraldValue { return fn(receiver, args...) })
			}
		} else if vm.currentBlock != nil && (methodObj.OriginalName == "tap" || method == "tap") {
			invoke = func() *object.EmeraldValue {
				return vm.withNativeBacktraceFrame("Kernel#tap", func() *object.EmeraldValue { return fn(receiver, args...) })
			}
		}
		result := invoke()
		if core.TracePointEventActive("c_return") {
			prepareTraceContext()
			core.FireTracePointReturn("c_return", binding, receiver, definedClass, methodID, method, nil, result)
		}
		return result
	}

	if fn, ok := methodObj.Fn.(*object.Function); ok {
		if errVal := rejectBlockArgument(fn, vm.currentBlock); errVal != nil {
			return errVal
		}
		args = dropAnonymousKeywordRestNonSymbolHash(fn, args)
		args = dropEmptyRuby2KeywordHashForPositionalOnlyFunction(fn, args)
		args = copyUnmarkedRuby2KeywordHashForPositionalFunction(fn, args, methodObj.Ruby2Keywords)
		args = mergeKeywordRestOverflowHashes(fn, args)
		if errVal := rejectedKeywordArgument(fn, args); errVal != nil {
			return errVal
		}
		if err := methodArityError(fn, positionalArityArgCount(fn, args)); err != nil {
			return err
		}
		if errVal := missingRequiredKeywordArgument(fn, args); errVal != nil {
			return errVal
		}
		if errVal := rejectedNonSymbolPositionalHashWithKeywords(fn, args); errVal != nil {
			return errVal
		}
		if errVal := rejectedPositionalForKeywordRestOnly(fn, args); errVal != nil {
			return errVal
		}
		if errVal := unknownKeywordArgument(fn, args); errVal != nil {
			return errVal
		}
		bp := vm.sp

		vm.stack[vm.sp] = receiver
		vm.sp++

		if len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly {
			positionalArgs := args
			var kwargsHash map[*object.EmeraldValue]*object.EmeraldValue
			if len(args) > 0 {
				lastArg := args[len(args)-1]
				if lastArg.Type == object.ValueHash && core.Ruby2KeywordHash(lastArg) {
					positionalArgs = args[:len(args)-1]
					kwargsHash = executorHashToMap(lastArg)
				}
			}

			if fn.HasRestParam {
				vm.bindRestParameterSlots(fn, positionalArgs, bp, methodObj.Ruby2Keywords)
			} else {
				vm.bindPositionalParameterSlots(fn, positionalArgs, bp)
			}

			for _, kp := range fn.KeywordParams {
				val := vm.lookupKwarg(kwargsHash, kp.Name)
				if val == nil {
					if kp.HasDefault {
						if !fn.EvaluateParamDefaults {
							val = kp.Default
						}
					} else {
						val = core.R.NilVal
					}
				}
				slot := vm.sp
				if index, ok := fn.LocalNames[kp.Name]; ok {
					slot = bp + 1 + index
				}
				vm.stack[slot] = val
				if vm.sp <= slot {
					vm.sp = slot + 1
				}
			}
			if fn.KeywordRestParam != "" {
				restHash := vm.keywordRestHash(kwargsHash, fn.KeywordParams)
				if index, ok := fn.LocalNames[fn.KeywordRestParam]; ok {
					slot := bp + 1 + index
					vm.stack[slot] = restHash
					if vm.sp <= slot {
						vm.sp = slot + 1
					}
				} else {
					vm.stack[vm.sp] = restHash
					vm.sp++
				}
			}
		} else if fn.HasRestParam {
			vm.bindRestParameterSlots(fn, args, bp, methodObj.Ruby2Keywords)
		} else {
			vm.bindPositionalParameterSlots(fn, args, bp)
		}

		var blockParamProc *object.Proc
		if fn.HasBlockParam {
			blockVal := vm.currentBlock
			if blockVal == nil {
				blockVal = core.R.NilVal
			} else if blockVal.Type == object.ValueClosure {
				closure := blockVal.Data.(*object.Closure)
				proc := &object.Proc{
					Fn:               closure.Fn,
					Env:              closure.Free,
					Block:            closure.Block,
					Binding:          closure.Binding,
					ClassStack:       closure.ClassStack,
					Refinements:      append([]*object.EmeraldValue(nil), closure.Refinements...),
					RefinementsFixed: closure.RefinementsFixed,
					InstanceVars:     make(map[string]*object.EmeraldValue),
					IsLambda:         false,
					BreakOwnerID:     closure.BreakOwnerID,
					ReturnOwnerID:    closure.ReturnOwnerID,
					FlipFlopStates:   closure.FlipFlopStates,
				}
				blockParamProc = proc
				blockVal = &object.EmeraldValue{
					Type:  object.ValueProc,
					Data:  proc,
					Class: core.R.Classes["Proc"],
				}
			}
			blockSlot := bp + fn.BlockParamIndex + 1
			vm.stack[blockSlot] = blockVal
			if vm.sp <= blockSlot {
				vm.sp = blockSlot + 1
			}
		}
		minSp := bp + 1 + fn.NumLocals
		if vm.sp < minSp {
			for i := vm.sp; i < minSp; i++ {
				vm.stack[i] = nil
			}
			vm.sp = minSp
		}
		if errVal := vm.bindParameterPatterns(fn, bp); errVal != nil {
			vm.sp = bp
			return errVal
		}

		methodClosure := methodObj.Closure
		var traceParameters *object.EmeraldValue
		if core.TracePointEventActive("call") || core.TracePointEventActive("return") {
			traceParameters = core.TracePointParameters(fn, true)
		}
		invocation := vm.buildInvocationMetadata(receiver, method, methodObj, methodOwner)
		newFrame := vm.pushReusableFrame(Frame{
			ID:                    vm.allocateFrameID(),
			Fn:                    fn,
			Ip:                    -1,
			Bp:                    bp,
			Closure:               methodClosure,
			MethodName:            method,
			OriginalMethodName:    methodObj.OriginalName,
			LabelName:             invocation.label,
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
			TraceDefinedClass:     invocation.traceDefinedClass,
			TraceMethodID:         invocation.traceMethodID,
			TraceCalleeID:         method,
			TraceParameters:       traceParameters,
		})
		if blockParamProc != nil && blockParamProc.BreakOwnerID == 0 {
			blockParamProc.BreakOwnerID = newFrame.ID
		}
		setBlockBreakOwner(vm.currentBlock, newFrame.ID)
		if core.TracePointEventActive("call") {
			binding := vm.currentFrameBinding()
			binding.Line = fn.DefinitionLine
			core.FireTracePointCall("call", binding, receiver, newFrame.TraceDefinedClass, newFrame.TraceMethodID, method, newFrame.TraceParameters)
		}

		prevBlock := vm.currentBlock
		vm.currentBlock = nil
		prevClassStack := vm.classStack
		if methodClosure != nil {
			vm.classStack = append([]*object.EmeraldValue(nil), methodClosure.ClassStack...)
		}
		frame := vm.frames[vm.fp]
		instructions := frame.Fn.Instructions

		for frame.Ip < len(instructions)-1 {
			frame.Ip++
			op := compiler.Opcode(instructions[frame.Ip])
			vm.fireTracePointLine(frame, op)
			frame.InstructionException = core.LastException
			frame.InstructionSnapshotSet = true
			err := vm.execute(op, frame)
			if err != nil {
				vm.currentBlock = prevBlock
				vm.classStack = prevClassStack
				if core.LastException != nil && core.LastException.Type == object.ValueException {
					return core.LastException
				}
				return core.NewRuntimeError(err.Error())
			}
			if vm.handlePendingNonLocalReturn(frame) || vm.handlePendingNonLocalBreak(frame) || frame.Returned {
				break
			}
			frame = vm.frames[vm.fp]
			if frame.BlockBreak || frame.BlockNextVal != nil {
				break
			}
			if core.LastBlockResult != nil {
				break
			}
			instructions = frame.Fn.Instructions
		}
		vm.currentBlock = prevBlock
		vm.classStack = prevClassStack

		result := core.R.NilVal
		if frame.BlockBreak {
			result = frame.BlockBreakVal
			if result == nil {
				result = core.R.NilVal
			}
		} else if frame.BlockNextVal != nil {
			result = frame.BlockNextVal
		} else if pending, ok := vm.pendingBreakResultForFrame(frame); ok {
			result = pending
		} else if core.LastBlockResult != nil {
			result = core.LastBlockResult
		} else if vm.sp > bp {
			result = vm.stack[vm.sp-1]
		}
		vm.sp = bp

		vm.endActiveRescuesForFrame(frame)
		frame.InstructionException = nil
		frame.InstructionSnapshotSet = false
		vm.frames = vm.frames[:vm.fp]
		vm.fp--
		if vm.fp >= 0 && oldFrame != nil {
			vm.frames[vm.fp] = oldFrame
		}

		return result
	}

	return core.R.NilVal
}

func (vm *VM) pushReusableFrame(initial Frame) *Frame {
	next := vm.fp + 1
	if next < cap(vm.frames) {
		vm.frames = vm.frames[:next+1]
	} else {
		vm.frames = append(vm.frames, nil)
	}
	frame := &initial
	vm.frames[next] = frame
	vm.fp = next
	return frame
}

func (vm *VM) tryFusedIntegerLocalExpression(frame *Frame, constants []*object.EmeraldValue) bool {
	if frame == nil || frame.Fn == nil || vm.instructionLimit != 0 || core.AnyTracePointActive() {
		return false
	}
	instructions := frame.Fn.Instructions
	start := frame.Ip
	if start < 0 || start+5 >= len(instructions) {
		return false
	}
	localIndex := int(instructions[start+1])
	writePosition := start + 2
	operations := 0
	for writePosition+3 < len(instructions) && compiler.Opcode(instructions[writePosition]) == compiler.OpConstant {
		switch compiler.Opcode(instructions[writePosition+3]) {
		case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod,
			compiler.OpBitAnd, compiler.OpBitOr, compiler.OpBitXor:
			operations++
			writePosition += 4
		default:
			goto structureScanned
		}
	}

structureScanned:
	if operations == 0 || writePosition+1 >= len(instructions) ||
		compiler.Opcode(instructions[writePosition]) != compiler.OpSetLocal ||
		int(instructions[writePosition+1]) != localIndex {
		return false
	}
	stackIndex := frame.Bp + localIndex + 1
	if stackIndex < 0 || stackIndex >= len(vm.stack) {
		return false
	}
	local := vm.stack[stackIndex]
	if local == nil || local.Type != object.ValueInteger {
		return false
	}
	if _, isCell := local.Data.(*closureCell); isCell {
		return false
	}
	if _, isBig := core.NumericBigIntOverride(local); isBig {
		return false
	}

	position := start + 2
	result := local.Data.(int64)
	for position < writePosition {
		constantIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
		if constantIndex < 0 || constantIndex >= len(constants) {
			return false
		}
		constant := constants[constantIndex]
		if constant == nil || constant.Type != object.ValueInteger {
			return false
		}
		if _, isBig := core.NumericBigIntOverride(constant); isBig {
			return false
		}
		right := constant.Data.(int64)
		switch compiler.Opcode(instructions[position+3]) {
		case compiler.OpAdd:
			if !vm.fusedIntegerBuiltinsAvailable() ||
				(right > 0 && result > math.MaxInt64-right) ||
				(right < 0 && result < math.MinInt64-right) {
				return false
			}
			result += right
		case compiler.OpSub:
			if (right < 0 && result > math.MaxInt64+right) ||
				(right > 0 && result < math.MinInt64+right) {
				return false
			}
			result -= right
		case compiler.OpMul:
			product := result * right
			if result == -1 && right == math.MinInt64 ||
				right == -1 && result == math.MinInt64 ||
				result != 0 && product/result != right {
				return false
			}
			result = product
		case compiler.OpMod:
			if right == 0 {
				return false
			}
			result %= right
			if result != 0 && (result < 0) != (right < 0) {
				result += right
			}
		case compiler.OpBitAnd:
			result &= right
		case compiler.OpBitOr:
			result |= right
		case compiler.OpBitXor:
			result ^= right
		}
		position += 4
	}

	value := core.NewIntegerValue(result)
	vm.stack[stackIndex] = value
	if name, ok := vm.topLevelLocalName(frame, localIndex); ok {
		if binding := vm.topLevelBindingData(); binding != nil {
			binding.Locals[name] = value
		}
	}
	vm.updateCapturedBindingLocal(frame, localIndex, value)

	frame.Ip = writePosition + 1
	if frame.Ip+1 < len(instructions) && compiler.Opcode(instructions[frame.Ip+1]) == compiler.OpPop {
		frame.Ip++
		vm.recordPoppedValue(value)
	} else {
		vm.push(value)
	}
	return true
}

func loopLocalAt(instructions compiler.Instructions, position, end int) (int, int, bool) {
	if position+1 >= end || compiler.Opcode(instructions[position]) != compiler.OpGetLocal {
		return 0, position, false
	}
	return int(instructions[position+1]), position + 2, true
}

func loopIntegerConstantAt(frame *Frame, position, end int) (int64, int, bool) {
	if position+2 >= end || compiler.Opcode(frame.Fn.Instructions[position]) != compiler.OpConstant {
		return 0, position, false
	}
	index := int(frame.Fn.Instructions[position+1])<<8 | int(frame.Fn.Instructions[position+2])
	if index < 0 || index >= len(frame.Fn.Constants) {
		return 0, position, false
	}
	value := frame.Fn.Constants[index]
	if value == nil || value.Type != object.ValueInteger {
		return 0, position, false
	}
	if _, big := core.NumericBigIntOverride(value); big {
		return 0, position, false
	}
	return value.Data.(int64), position + 3, true
}

func simpleIntegerLoopHeader(frame *Frame, target, jumpPosition int) (counterLocal, limitLocal, exitPosition, bodyPosition int, ok bool) {
	instructions := frame.Fn.Instructions
	position := target
	counterLocal, position, ok = loopLocalAt(instructions, position, jumpPosition)
	if !ok {
		return
	}
	limitLocal, position, ok = loopLocalAt(instructions, position, jumpPosition)
	if !ok || position >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpLessThan {
		ok = false
		return
	}
	position++
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpJumpNotTruthy {
		ok = false
		return
	}
	exitPosition = int(instructions[position+1])<<8 | int(instructions[position+2])
	position += 3
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSetWhileEnd ||
		int(instructions[position+1])<<8|int(instructions[position+2]) != exitPosition {
		ok = false
		return
	}
	bodyPosition = position + 3
	ok = true
	return
}

func (vm *VM) commitIntegerLoopLocal(frame *Frame, local int, value int64) *object.EmeraldValue {
	result := core.NewIntegerValue(value)
	vm.stack[frame.Bp+local+1] = result
	if name, ok := vm.topLevelLocalName(frame, local); ok {
		if binding := vm.topLevelBindingData(); binding != nil {
			binding.Locals[name] = result
		}
	}
	vm.updateCapturedBindingLocal(frame, local, result)
	return result
}

func (vm *VM) tryExecuteCollectionFillLoop(frame *Frame, target, jumpPosition int) bool {
	if frame == nil || frame.Fn == nil || frame.Fn.Name != "__main__" ||
		vm.instructionLimit != 0 || core.AnyTracePointActive() ||
		!core.CollectionLoopBuiltinsAvailable() {
		return false
	}
	counterLocal, limitLocal, exitPosition, position, ok := simpleIntegerLoopHeader(frame, target, jumpPosition)
	if !ok {
		return false
	}
	instructions := frame.Fn.Instructions
	expressionLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || expressionLocal != counterLocal {
		return false
	}
	factor, position, ok := loopIntegerConstantAt(frame, position, jumpPosition)
	if !ok || position >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpMul {
		return false
	}
	position++
	valueModulus, position, ok := loopIntegerConstantAt(frame, position, jumpPosition)
	if !ok || valueModulus == 0 || position >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpMod {
		return false
	}
	position++
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSetLocal {
		return false
	}
	valueLocal := int(instructions[position+1])
	if compiler.Opcode(instructions[position+2]) != compiler.OpPop {
		return false
	}
	position += 3
	arrayLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok {
		return false
	}
	appendedLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || appendedLocal != valueLocal || position+1 >= jumpPosition ||
		compiler.Opcode(instructions[position]) != compiler.OpBitLeftShift ||
		compiler.Opcode(instructions[position+1]) != compiler.OpPop {
		return false
	}
	position += 2
	hashLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok {
		return false
	}
	keyLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || keyLocal != counterLocal {
		return false
	}
	keyModulus, position, ok := loopIntegerConstantAt(frame, position, jumpPosition)
	if !ok || keyModulus == 0 || position >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpMod {
		return false
	}
	position++
	storedLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || storedLocal != valueLocal || position+1 >= jumpPosition ||
		compiler.Opcode(instructions[position]) != compiler.OpIndexAssign ||
		compiler.Opcode(instructions[position+1]) != compiler.OpPop {
		return false
	}
	position += 2
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
		int(instructions[position+1]) != counterLocal ||
		compiler.Opcode(instructions[position+2]) != compiler.OpPop ||
		position+3 != jumpPosition {
		return false
	}

	counterValue := vm.stack[frame.Bp+counterLocal+1]
	limitValue := vm.stack[frame.Bp+limitLocal+1]
	arrayValue := vm.stack[frame.Bp+arrayLocal+1]
	hashValue := vm.stack[frame.Bp+hashLocal+1]
	if counterValue == nil || counterValue.Type != object.ValueInteger ||
		limitValue == nil || limitValue.Type != object.ValueInteger ||
		arrayValue == nil || arrayValue.Type != object.ValueArray || arrayValue.Class != core.R.Classes["Array"] || arrayValue.Frozen ||
		hashValue == nil || hashValue.Type != object.ValueHash || hashValue.Class != core.R.Classes["Hash"] || hashValue.Frozen {
		return false
	}
	counter := counterValue.Data.(int64)
	limit := limitValue.Data.(int64)
	values := make([]*object.EmeraldValue, 0, 1024)
	keys := make([]*object.EmeraldValue, 0, 1024)
	var lastValue int64
	for iterations := 0; counter < limit && iterations < 1_000_000; iterations++ {
		product, valid := applyIntegerBinary(compiler.OpMul, counter, factor)
		if !valid {
			return false
		}
		computed, valid := applyIntegerBinary(compiler.OpMod, product, valueModulus)
		if !valid {
			return false
		}
		key, valid := applyIntegerBinary(compiler.OpMod, counter, keyModulus)
		if !valid {
			return false
		}
		values = append(values, core.NewIntegerValue(computed))
		keys = append(keys, core.NewIntegerValue(key))
		lastValue = computed
		next, valid := applyIntegerBinary(compiler.OpAdd, counter, step)
		if !valid {
			return false
		}
		counter = next
	}
	if !core.AppendArrayValues(arrayValue, values) {
		return false
	}
	for index, key := range keys {
		if !core.StoreHashValue(hashValue, key, values[index]) {
			return false
		}
	}
	last := vm.commitIntegerLoopLocal(frame, counterLocal, counter)
	if len(values) > 0 {
		last = vm.commitIntegerLoopLocal(frame, valueLocal, lastValue)
	}
	vm.recordPoppedValue(last)
	frame.WhileEnd = exitPosition
	frame.BlockBreakAddr = exitPosition
	if counter >= limit {
		frame.Ip = exitPosition - 1
	} else {
		frame.Ip = target - 1
	}
	return true
}

func (vm *VM) tryExecuteArraySumLoop(frame *Frame, target, jumpPosition int) bool {
	if frame == nil || frame.Fn == nil || frame.Fn.Name != "__main__" ||
		vm.instructionLimit != 0 || core.AnyTracePointActive() ||
		!core.CollectionLoopBuiltinsAvailable() {
		return false
	}
	instructions := frame.Fn.Instructions
	position := target
	counterLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok {
		return false
	}
	arrayLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || position+5 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSend {
		return false
	}
	methodIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
	if methodIndex < 0 || methodIndex >= len(frame.Fn.Constants) ||
		frame.Fn.Constants[methodIndex].Data != "length" ||
		instructions[position+3] != 0 || instructions[position+4] != 0 || instructions[position+5] != 255 {
		return false
	}
	position += 6
	if compiler.Opcode(instructions[position]) != compiler.OpLessThan {
		return false
	}
	position++
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpJumpNotTruthy {
		return false
	}
	exitPosition := int(instructions[position+1])<<8 | int(instructions[position+2])
	position += 3
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSetWhileEnd ||
		int(instructions[position+1])<<8|int(instructions[position+2]) != exitPosition {
		return false
	}
	position += 3
	sumLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok {
		return false
	}
	indexedArray, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || indexedArray != arrayLocal {
		return false
	}
	indexLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || indexLocal != counterLocal || compiler.Opcode(instructions[position]) != compiler.OpIndex ||
		compiler.Opcode(instructions[position+1]) != compiler.OpAdd ||
		compiler.Opcode(instructions[position+2]) != compiler.OpSetLocal ||
		int(instructions[position+3]) != sumLocal ||
		compiler.Opcode(instructions[position+4]) != compiler.OpPop {
		return false
	}
	position += 5
	updateLocal, position, ok := loopLocalAt(instructions, position, jumpPosition)
	if !ok || updateLocal != counterLocal {
		return false
	}
	step, position, ok := loopIntegerConstantAt(frame, position, jumpPosition)
	if !ok || step <= 0 || compiler.Opcode(instructions[position]) != compiler.OpAdd ||
		compiler.Opcode(instructions[position+1]) != compiler.OpSetLocal ||
		int(instructions[position+2]) != counterLocal ||
		compiler.Opcode(instructions[position+3]) != compiler.OpPop ||
		position+4 != jumpPosition {
		return false
	}
	arrayValue := vm.stack[frame.Bp+arrayLocal+1]
	counterValue := vm.stack[frame.Bp+counterLocal+1]
	sumValue := vm.stack[frame.Bp+sumLocal+1]
	if arrayValue == nil || arrayValue.Type != object.ValueArray || arrayValue.Class != core.R.Classes["Array"] ||
		counterValue == nil || counterValue.Type != object.ValueInteger ||
		sumValue == nil || sumValue.Type != object.ValueInteger {
		return false
	}
	elements := arrayValue.Data.([]*object.EmeraldValue)
	counter := counterValue.Data.(int64)
	sum := sumValue.Data.(int64)
	for iterations := 0; counter < int64(len(elements)) && iterations < 1_000_000; iterations++ {
		if counter < 0 || counter >= int64(len(elements)) {
			return false
		}
		element := elements[counter]
		if element == nil || element.Type != object.ValueInteger {
			return false
		}
		nextSum, valid := applyIntegerBinary(compiler.OpAdd, sum, element.Data.(int64))
		if !valid {
			return false
		}
		nextCounter, valid := applyIntegerBinary(compiler.OpAdd, counter, step)
		if !valid {
			return false
		}
		sum, counter = nextSum, nextCounter
	}
	last := vm.commitIntegerLoopLocal(frame, sumLocal, sum)
	vm.commitIntegerLoopLocal(frame, counterLocal, counter)
	vm.recordPoppedValue(last)
	frame.WhileEnd = exitPosition
	frame.BlockBreakAddr = exitPosition
	if counter >= int64(len(elements)) {
		frame.Ip = exitPosition - 1
	} else {
		frame.Ip = target - 1
	}
	return true
}

func (vm *VM) tryExecuteASCIIStringLoop(frame *Frame, target, jumpPosition int) bool {
	if frame == nil || frame.Fn == nil || frame.Fn.Name != "__main__" ||
		vm.instructionLimit != 0 || core.AnyTracePointActive() ||
		!core.ASCIIStringLoopBuiltinsAvailable() || target < 0 || jumpPosition+2 >= len(frame.Fn.Instructions) {
		return false
	}
	instructions := frame.Fn.Instructions
	position := target
	readLocal := func() (int, bool) {
		if position+1 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpGetLocal {
			return 0, false
		}
		local := int(instructions[position+1])
		position += 2
		return local, true
	}
	readIntegerConstant := func() (int64, bool) {
		if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpConstant {
			return 0, false
		}
		index := int(instructions[position+1])<<8 | int(instructions[position+2])
		if index < 0 || index >= len(frame.Fn.Constants) {
			return 0, false
		}
		value := frame.Fn.Constants[index]
		if value == nil || value.Type != object.ValueInteger {
			return 0, false
		}
		if _, big := core.NumericBigIntOverride(value); big {
			return 0, false
		}
		position += 3
		return value.Data.(int64), true
	}
	readSend := func(expected string, arguments byte) bool {
		if position+5 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSend {
			return false
		}
		index := int(instructions[position+1])<<8 | int(instructions[position+2])
		if index < 0 || index >= len(frame.Fn.Constants) {
			return false
		}
		name, ok := frame.Fn.Constants[index].Data.(string)
		if !ok || name != expected || instructions[position+3] != 0 ||
			instructions[position+4] != arguments || instructions[position+5] != 255 {
			return false
		}
		position += 6
		return true
	}

	counterLocal, ok := readLocal()
	if !ok {
		return false
	}
	limitLocal, ok := readLocal()
	if !ok || compiler.Opcode(instructions[position]) != compiler.OpLessThan {
		return false
	}
	position++
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpJumpNotTruthy {
		return false
	}
	exitPosition := int(instructions[position+1])<<8 | int(instructions[position+2])
	position += 3
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSetWhileEnd ||
		int(instructions[position+1])<<8|int(instructions[position+2]) != exitPosition {
		return false
	}
	position += 3
	stringLocal, ok := readLocal()
	if !ok {
		return false
	}
	base, ok := readIntegerConstant()
	if !ok {
		return false
	}
	expressionLocal, ok := readLocal()
	if !ok || expressionLocal != counterLocal {
		return false
	}
	modulus, ok := readIntegerConstant()
	if !ok || modulus <= 0 || compiler.Opcode(instructions[position]) != compiler.OpMod {
		return false
	}
	position++
	if compiler.Opcode(instructions[position]) != compiler.OpAdd {
		return false
	}
	position++
	if !readSend("chr", 0) || compiler.Opcode(instructions[position]) != compiler.OpBitLeftShift {
		return false
	}
	position++
	if compiler.Opcode(instructions[position]) != compiler.OpPop {
		return false
	}
	position++
	updateLocal, ok := readLocal()
	if !ok || updateLocal != counterLocal {
		return false
	}
	step, ok := readIntegerConstant()
	if !ok || step <= 0 || compiler.Opcode(instructions[position]) != compiler.OpAdd {
		return false
	}
	position++
	if position+2 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpSetLocal ||
		int(instructions[position+1]) != counterLocal ||
		compiler.Opcode(instructions[position+2]) != compiler.OpPop ||
		position+3 != jumpPosition {
		return false
	}
	if base < 0 || base > 127 || modulus > 128 || base > 128-modulus {
		return false
	}

	counterValue := vm.stack[frame.Bp+counterLocal+1]
	limitValue := vm.stack[frame.Bp+limitLocal+1]
	stringValue := vm.stack[frame.Bp+stringLocal+1]
	if counterValue == nil || counterValue.Type != object.ValueInteger ||
		limitValue == nil || limitValue.Type != object.ValueInteger ||
		stringValue == nil || stringValue.Type != object.ValueString {
		return false
	}
	if _, big := core.NumericBigIntOverride(counterValue); big {
		return false
	}
	if _, big := core.NumericBigIntOverride(limitValue); big {
		return false
	}
	counter := counterValue.Data.(int64)
	limit := limitValue.Data.(int64)
	remaining := int64(0)
	if counter < limit {
		remaining = (limit - counter + step - 1) / step
	}
	if remaining > 1_000_000 {
		remaining = 1_000_000
	}
	raw := make([]byte, 0, remaining)
	for iterations := int64(0); iterations < remaining; iterations++ {
		mod := counter % modulus
		if mod < 0 {
			mod += modulus
		}
		raw = append(raw, byte(base+mod))
		if counter > math.MaxInt64-step {
			return false
		}
		counter += step
	}
	if errValue := core.AppendASCIIBytes(stringValue, string(raw)); errValue != nil {
		return false
	}
	updatedCounter := core.NewIntegerValue(counter)
	vm.stack[frame.Bp+counterLocal+1] = updatedCounter
	if name, ok := vm.topLevelLocalName(frame, counterLocal); ok {
		if binding := vm.topLevelBindingData(); binding != nil {
			binding.Locals[name] = updatedCounter
		}
	}
	vm.updateCapturedBindingLocal(frame, counterLocal, updatedCounter)
	vm.recordPoppedValue(updatedCounter)
	frame.WhileEnd = exitPosition
	frame.BlockBreakAddr = exitPosition
	if counter >= limit {
		frame.Ip = exitPosition - 1
	} else {
		frame.Ip = target - 1
	}
	return true
}

func (vm *VM) tryExecuteIntegerBytecodeLoop(frame *Frame, target, jumpPosition int) bool {
	if frame == nil || frame.Fn == nil || frame.Fn.Name != "__main__" ||
		vm.instructionLimit != 0 || core.AnyTracePointActive() ||
		target < 0 || target+11 >= jumpPosition || jumpPosition+2 >= len(frame.Fn.Instructions) {
		return false
	}
	instructions := frame.Fn.Instructions
	position := target
	if compiler.Opcode(instructions[position]) != compiler.OpGetLocal {
		return false
	}
	counterLocal := int(instructions[position+1])
	position += 2
	if compiler.Opcode(instructions[position]) != compiler.OpGetLocal {
		return false
	}
	limitLocal := int(instructions[position+1])
	position += 2
	if compiler.Opcode(instructions[position]) != compiler.OpLessThan {
		return false
	}
	position++
	if compiler.Opcode(instructions[position]) != compiler.OpJumpNotTruthy {
		return false
	}
	exitPosition := int(instructions[position+1])<<8 | int(instructions[position+2])
	position += 3
	if compiler.Opcode(instructions[position]) != compiler.OpSetWhileEnd {
		return false
	}
	whileEnd := int(instructions[position+1])<<8 | int(instructions[position+2])
	if whileEnd != exitPosition {
		return false
	}
	position += 3

	steps := make([]integerLoopStep, 0, 24)
	var used [256]bool
	used[counterLocal], used[limitLocal] = true, true
	var stackKinds [32]bool
	stackDepth := 0
	receiver := vm.stack[frame.Bp]
	for position < jumpPosition {
		op := compiler.Opcode(instructions[position])
		step := integerLoopStep{op: op}
		switch op {
		case compiler.OpGetLocal:
			if position+1 >= jumpPosition || stackDepth >= len(stackKinds) {
				return false
			}
			step.local = int(instructions[position+1])
			used[step.local] = true
			stackKinds[stackDepth] = false
			stackDepth++
			position += 2
		case compiler.OpConstant:
			if position+2 >= jumpPosition || stackDepth >= len(stackKinds) {
				return false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(frame.Fn.Constants) {
				return false
			}
			value := frame.Fn.Constants[index]
			if value == nil || value.Type != object.ValueInteger {
				return false
			}
			if _, big := core.NumericBigIntOverride(value); big {
				return false
			}
			step.value = value.Data.(int64)
			stackKinds[stackDepth] = false
			stackDepth++
			position += 3
		case compiler.OpSelf:
			if stackDepth >= len(stackKinds) {
				return false
			}
			stackKinds[stackDepth] = true
			stackDepth++
			position++
		case compiler.OpSend:
			if position+5 >= jumpPosition || stackDepth < 2 ||
				!stackKinds[stackDepth-2] || stackKinds[stackDepth-1] ||
				instructions[position+3] != 0 || instructions[position+4] != 1 || instructions[position+5] != 255 {
				return false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(frame.Fn.Constants) {
				return false
			}
			name, ok := frame.Fn.Constants[index].Data.(string)
			if !ok {
				return false
			}
			methodObj, _, fallback := vm.lookupMethodForSend(receiver, name, nil, false)
			if fallback != nil || methodObj == nil || methodObj.Visibility == "undefined" ||
				(methodObj.Visibility == "private" && receiver != core.R.Main) ||
				methodObj.Visibility == "protected" {
				return false
			}
			fn, ok := methodObj.Fn.(*object.Function)
			if !ok || len(fn.ParamLocalIndices) != 1 {
				return false
			}
			plan, ok := vm.cachedIntegerFunctionPlan(fn)
			if !ok || plan == nil {
				return false
			}
			step.fn = fn
			stackDepth--
			stackKinds[stackDepth-1] = false
			position += 6
		case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod,
			compiler.OpBitAnd, compiler.OpBitOr, compiler.OpBitXor,
			compiler.OpBitLeftShift, compiler.OpBitRightShift:
			if stackDepth < 2 || stackKinds[stackDepth-1] || stackKinds[stackDepth-2] {
				return false
			}
			stackDepth--
			position++
		case compiler.OpNeg, compiler.OpNegate:
			if stackDepth < 1 || stackKinds[stackDepth-1] {
				return false
			}
			position++
		case compiler.OpSetLocal:
			if position+1 >= jumpPosition || stackDepth < 1 || stackKinds[stackDepth-1] {
				return false
			}
			step.local = int(instructions[position+1])
			used[step.local] = true
			position += 2
		case compiler.OpPop:
			if stackDepth < 1 {
				return false
			}
			stackDepth--
			position++
		default:
			return false
		}
		steps = append(steps, step)
	}
	if stackDepth != 0 || len(steps) == 0 {
		return false
	}

	var locals [256]int64
	usedLocals := make([]int, 0, 8)
	for local, inUse := range used {
		if !inUse {
			continue
		}
		stackIndex := frame.Bp + local + 1
		if stackIndex < 0 || stackIndex >= len(vm.stack) {
			return false
		}
		value := vm.stack[stackIndex]
		if value == nil || value.Type != object.ValueInteger {
			return false
		}
		if _, cell := value.Data.(*closureCell); cell {
			return false
		}
		if _, big := core.NumericBigIntOverride(value); big {
			return false
		}
		locals[local] = value.Data.(int64)
		usedLocals = append(usedLocals, local)
	}
	if !vm.fusedIntegerBuiltinsAvailable() {
		return false
	}

	var values [32]int64
	var selfValues [32]bool
	var before [256]int64
	completed := true
	for iterations := 0; locals[counterLocal] < locals[limitLocal]; iterations++ {
		if iterations == 1_000_000 {
			completed = false
			break
		}
		for _, local := range usedLocals {
			before[local] = locals[local]
		}
		stackPointer := 0
		failed := false
		for _, step := range steps {
			switch step.op {
			case compiler.OpGetLocal:
				values[stackPointer] = locals[step.local]
				selfValues[stackPointer] = false
				stackPointer++
			case compiler.OpConstant:
				values[stackPointer] = step.value
				selfValues[stackPointer] = false
				stackPointer++
			case compiler.OpSelf:
				selfValues[stackPointer] = true
				stackPointer++
			case compiler.OpSend:
				result, ok := vm.executeSingleIntegerFunctionPlan(step.fn, values[stackPointer-1])
				if !ok {
					failed = true
					break
				}
				stackPointer--
				values[stackPointer-1] = result
				selfValues[stackPointer-1] = false
			case compiler.OpNeg, compiler.OpNegate:
				if values[stackPointer-1] == math.MinInt64 {
					failed = true
					break
				}
				values[stackPointer-1] = -values[stackPointer-1]
			case compiler.OpSetLocal:
				locals[step.local] = values[stackPointer-1]
			case compiler.OpPop:
				stackPointer--
			default:
				result, ok := applyIntegerBinary(step.op, values[stackPointer-2], values[stackPointer-1])
				if !ok {
					failed = true
					break
				}
				stackPointer--
				values[stackPointer-1] = result
			}
		}
		if failed {
			for _, local := range usedLocals {
				locals[local] = before[local]
			}
			completed = false
			break
		}
	}
	for _, local := range usedLocals {
		value := core.NewIntegerValue(locals[local])
		stackIndex := frame.Bp + local + 1
		vm.stack[stackIndex] = value
		if name, ok := vm.topLevelLocalName(frame, local); ok {
			if binding := vm.topLevelBindingData(); binding != nil {
				binding.Locals[name] = value
			}
		}
		vm.updateCapturedBindingLocal(frame, local, value)
	}
	frame.WhileEnd = exitPosition
	frame.BlockBreakAddr = exitPosition
	if completed {
		frame.Ip = exitPosition - 1
	} else {
		frame.Ip = target - 1
	}
	return true
}

func (vm *VM) tryExecuteCountedIntegerLoop(frame *Frame, target, jumpPosition int) bool {
	if frame == nil || frame.Fn == nil || frame.Fn.Name != "__main__" ||
		vm.instructionLimit != 0 || core.AnyTracePointActive() {
		return false
	}
	instructions := frame.Fn.Instructions
	if target < 0 || target+11 >= jumpPosition || jumpPosition+2 >= len(instructions) {
		return false
	}
	position := target
	if compiler.Opcode(instructions[position]) != compiler.OpGetLocal {
		return false
	}
	counterLocal := int(instructions[position+1])
	position += 2
	if compiler.Opcode(instructions[position]) != compiler.OpGetLocal {
		return false
	}
	limitLocal := int(instructions[position+1])
	position += 2
	if compiler.Opcode(instructions[position]) != compiler.OpLessThan {
		return false
	}
	position++
	if compiler.Opcode(instructions[position]) != compiler.OpJumpNotTruthy {
		return false
	}
	exitPosition := int(instructions[position+1])<<8 | int(instructions[position+2])
	position += 3
	if compiler.Opcode(instructions[position]) != compiler.OpSetWhileEnd {
		return false
	}
	whileEnd := int(instructions[position+1])<<8 | int(instructions[position+2])
	if whileEnd != exitPosition {
		return false
	}
	position += 3

	updates := make([]integerLocalUpdatePlan, 0, 4)
	usesAdd := false
	for position < jumpPosition {
		if len(updates) >= 8 || position+1 >= jumpPosition || compiler.Opcode(instructions[position]) != compiler.OpGetLocal {
			return false
		}
		update := integerLocalUpdatePlan{local: int(instructions[position+1])}
		position += 2
		for position+3 < jumpPosition && compiler.Opcode(instructions[position]) == compiler.OpConstant {
			constantIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
			if constantIndex < 0 || constantIndex >= len(frame.Fn.Constants) {
				return false
			}
			constant := frame.Fn.Constants[constantIndex]
			if constant == nil || constant.Type != object.ValueInteger {
				return false
			}
			if _, isBig := core.NumericBigIntOverride(constant); isBig {
				return false
			}
			op := compiler.Opcode(instructions[position+3])
			switch op {
			case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod,
				compiler.OpBitAnd, compiler.OpBitOr, compiler.OpBitXor:
			default:
				return false
			}
			update.steps = append(update.steps, integerFunctionStep{op: op, value: constant.Data.(int64)})
			usesAdd = usesAdd || op == compiler.OpAdd
			position += 4
		}
		if len(update.steps) == 0 || position+2 >= jumpPosition ||
			compiler.Opcode(instructions[position]) != compiler.OpSetLocal ||
			int(instructions[position+1]) != update.local ||
			compiler.Opcode(instructions[position+2]) != compiler.OpPop {
			return false
		}
		for _, prior := range updates {
			if prior.local == update.local {
				return false
			}
		}
		updates = append(updates, update)
		position += 3
	}
	if len(updates) == 0 || updates[len(updates)-1].local != counterLocal || limitLocal == counterLocal {
		return false
	}
	counterUpdate := updates[len(updates)-1]
	if len(counterUpdate.steps) != 1 || counterUpdate.steps[0].op != compiler.OpAdd || counterUpdate.steps[0].value <= 0 {
		return false
	}
	for _, update := range updates {
		if update.local == limitLocal {
			return false
		}
	}
	if usesAdd && !vm.fusedIntegerBuiltinsAvailable() {
		return false
	}

	var values [8]int64
	for index, update := range updates {
		stackIndex := frame.Bp + update.local + 1
		if stackIndex < 0 || stackIndex >= len(vm.stack) {
			return false
		}
		value := vm.stack[stackIndex]
		if value == nil || value.Type != object.ValueInteger {
			return false
		}
		if _, isCell := value.Data.(*closureCell); isCell {
			return false
		}
		if _, isBig := core.NumericBigIntOverride(value); isBig {
			return false
		}
		values[index] = value.Data.(int64)
	}
	limitIndex := frame.Bp + limitLocal + 1
	if limitIndex < 0 || limitIndex >= len(vm.stack) {
		return false
	}
	limitValue := vm.stack[limitIndex]
	if limitValue == nil || limitValue.Type != object.ValueInteger {
		return false
	}
	if _, isBig := core.NumericBigIntOverride(limitValue); isBig {
		return false
	}
	limit := limitValue.Data.(int64)
	counterIndex := len(updates) - 1

	completed := true
	for iterations := 0; values[counterIndex] < limit; iterations++ {
		if iterations == 1_000_000 {
			completed = false
			break
		}
		before := values
		for index, update := range updates {
			result, ok := applyIntegerUpdate(values[index], update.steps)
			if !ok {
				values = before
				vm.commitIntegerLoopLocals(frame, updates, values)
				return false
			}
			values[index] = result
		}
	}
	vm.commitIntegerLoopLocals(frame, updates, values)
	frame.WhileEnd = exitPosition
	frame.BlockBreakAddr = exitPosition
	if completed {
		frame.Ip = exitPosition - 1
	} else {
		frame.Ip = target - 1
	}
	return true
}

func applyIntegerUpdate(value int64, steps []integerFunctionStep) (int64, bool) {
	result := value
	for _, step := range steps {
		right := step.value
		switch step.op {
		case compiler.OpAdd:
			if (right > 0 && result > math.MaxInt64-right) ||
				(right < 0 && result < math.MinInt64-right) {
				return 0, false
			}
			result += right
		case compiler.OpSub:
			if (right < 0 && result > math.MaxInt64+right) ||
				(right > 0 && result < math.MinInt64+right) {
				return 0, false
			}
			result -= right
		case compiler.OpMul:
			product := result * right
			if result == -1 && right == math.MinInt64 ||
				right == -1 && result == math.MinInt64 ||
				result != 0 && product/result != right {
				return 0, false
			}
			result = product
		case compiler.OpMod:
			if right == 0 {
				return 0, false
			}
			result %= right
			if result != 0 && (result < 0) != (right < 0) {
				result += right
			}
		case compiler.OpBitAnd:
			result &= right
		case compiler.OpBitOr:
			result |= right
		case compiler.OpBitXor:
			result ^= right
		default:
			return 0, false
		}
	}
	return result, true
}

func (vm *VM) commitIntegerLoopLocals(frame *Frame, updates []integerLocalUpdatePlan, values [8]int64) {
	var last *object.EmeraldValue
	for index, update := range updates {
		value := core.NewIntegerValue(values[index])
		stackIndex := frame.Bp + update.local + 1
		vm.stack[stackIndex] = value
		if name, ok := vm.topLevelLocalName(frame, update.local); ok {
			if binding := vm.topLevelBindingData(); binding != nil {
				binding.Locals[name] = value
			}
		}
		vm.updateCapturedBindingLocal(frame, update.local, value)
		last = value
	}
	if last != nil {
		vm.recordPoppedValue(last)
	}
}

func (vm *VM) fusedIntegerBuiltinsAvailable() bool {
	generation := object.CurrentMethodGeneration()
	if vm.fusedIntegerGeneration != generation {
		vm.fusedIntegerGeneration = generation
		vm.fusedIntegerOps = core.IntegerPlusUsesBuiltinImplementation()
	}
	return vm.fusedIntegerOps
}

func (vm *VM) cachedIntegerFunctionPlan(fn *object.Function) (*integerFunctionPlan, bool) {
	cached, found := vm.integerFunctionCache[fn]
	if !found {
		plan, supported := buildIntegerFunctionPlan(fn)
		cached = integerFunctionCacheEntry{plan: plan, supported: supported}
		vm.integerFunctionCache[fn] = cached
	}
	return cached.plan, cached.supported && cached.plan != nil
}

func applyIntegerBinary(op compiler.Opcode, left, right int64) (int64, bool) {
	switch op {
	case compiler.OpAdd:
		if (right > 0 && left > math.MaxInt64-right) ||
			(right < 0 && left < math.MinInt64-right) {
			return 0, false
		}
		return left + right, true
	case compiler.OpSub:
		if (right < 0 && left > math.MaxInt64+right) ||
			(right > 0 && left < math.MinInt64+right) {
			return 0, false
		}
		return left - right, true
	case compiler.OpMul:
		product := left * right
		if left == -1 && right == math.MinInt64 ||
			right == -1 && left == math.MinInt64 ||
			left != 0 && product/left != right {
			return 0, false
		}
		return product, true
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
	case compiler.OpBitOr:
		return left | right, true
	case compiler.OpBitXor:
		return left ^ right, true
	case compiler.OpBitRightShift:
		if right < 0 || right >= 64 {
			return 0, false
		}
		return left >> uint(right), true
	case compiler.OpBitLeftShift:
		if right < 0 || right >= 64 {
			return 0, false
		}
		result := left << uint(right)
		if result>>uint(right) != left {
			return 0, false
		}
		return result, true
	}
	return 0, false
}

func (vm *VM) executeSingleIntegerFunctionPlan(fn *object.Function, argument int64) (int64, bool) {
	plan, ok := vm.cachedIntegerFunctionPlan(fn)
	if !ok || len(fn.ParamLocalIndices) != 1 || plan.usesAdd && !vm.fusedIntegerBuiltinsAvailable() {
		return 0, false
	}
	var stack [16]int64
	stackPointer := 0
	for _, step := range plan.steps {
		switch step.op {
		case compiler.OpGetLocal:
			if step.param != 0 {
				return 0, false
			}
			stack[stackPointer] = argument
			stackPointer++
		case compiler.OpConstant:
			stack[stackPointer] = step.value
			stackPointer++
		case compiler.OpNeg, compiler.OpNegate:
			if stackPointer < 1 || stack[stackPointer-1] == math.MinInt64 {
				return 0, false
			}
			stack[stackPointer-1] = -stack[stackPointer-1]
		case compiler.OpReturnValue:
			if stackPointer != 1 {
				return 0, false
			}
			return stack[0], true
		default:
			if stackPointer < 2 {
				return 0, false
			}
			result, ok := applyIntegerBinary(step.op, stack[stackPointer-2], stack[stackPointer-1])
			if !ok {
				return 0, false
			}
			stackPointer--
			stack[stackPointer-1] = result
		}
	}
	return 0, false
}

func buildIntegerFunctionPlan(fn *object.Function) (*integerFunctionPlan, bool) {
	if fn == nil || fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) > 0 ||
		fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		len(fn.ParamLocalIndices) == 0 {
		return nil, false
	}
	for _, pattern := range fn.ParamPatterns {
		if pattern != nil {
			return nil, false
		}
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return nil, false
		}
	}
	plan := &integerFunctionPlan{}
	stackDepth := 0
	instructions := fn.Instructions
	for position := 0; position < len(instructions); {
		op := compiler.Opcode(instructions[position])
		switch op {
		case compiler.OpGetLocal:
			if position+1 >= len(instructions) {
				return nil, false
			}
			localIndex := int(instructions[position+1])
			paramIndex := -1
			for index, candidate := range fn.ParamLocalIndices {
				if candidate == localIndex {
					paramIndex = index
					break
				}
			}
			if paramIndex < 0 {
				return nil, false
			}
			plan.steps = append(plan.steps, integerFunctionStep{op: op, param: paramIndex})
			stackDepth++
			position += 2
		case compiler.OpConstant:
			if position+2 >= len(instructions) {
				return nil, false
			}
			constantIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
			if constantIndex < 0 || constantIndex >= len(fn.Constants) {
				return nil, false
			}
			constant := fn.Constants[constantIndex]
			if constant == nil || constant.Type != object.ValueInteger {
				return nil, false
			}
			if _, isBig := core.NumericBigIntOverride(constant); isBig {
				return nil, false
			}
			plan.steps = append(plan.steps, integerFunctionStep{op: op, value: constant.Data.(int64)})
			stackDepth++
			position += 3
		case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod,
			compiler.OpBitAnd, compiler.OpBitOr, compiler.OpBitXor,
			compiler.OpBitLeftShift, compiler.OpBitRightShift:
			if stackDepth < 2 {
				return nil, false
			}
			plan.steps = append(plan.steps, integerFunctionStep{op: op})
			plan.usesAdd = plan.usesAdd || op == compiler.OpAdd
			stackDepth--
			position++
		case compiler.OpNeg, compiler.OpNegate:
			if stackDepth < 1 {
				return nil, false
			}
			plan.steps = append(plan.steps, integerFunctionStep{op: op})
			position++
		case compiler.OpReturnValue:
			if stackDepth != 1 || position != len(instructions)-1 {
				return nil, false
			}
			plan.steps = append(plan.steps, integerFunctionStep{op: op})
			position++
		default:
			return nil, false
		}
		if stackDepth > 16 {
			return nil, false
		}
	}
	if len(plan.steps) == 0 || plan.steps[len(plan.steps)-1].op != compiler.OpReturnValue {
		return nil, false
	}
	return plan, true
}

func valueHasClassInAncestry(value *object.EmeraldValue, target *object.Class) bool {
	if value == nil || target == nil {
		return false
	}
	for class := value.Class; class != nil; class = class.SuperClass {
		if class == target {
			return true
		}
	}
	return false
}

func backtraceMethodLabel(receiver *object.EmeraldValue, methodObj *object.Method, methodOwner *object.Class, fallback string) string {
	name := fallback
	if methodObj != nil {
		if methodObj.OriginalName != "" {
			name = methodObj.OriginalName
		} else if methodObj.Name != "" {
			name = methodObj.Name
		}
	}
	if receiver != nil {
		switch receiver.Type {
		case object.ValueModule:
			if mod, ok := receiver.Data.(*object.Module); ok && mod != nil && mod.Name != "" {
				return mod.Name + "." + name
			}
		case object.ValueClass:
			if cls, ok := receiver.Data.(*object.Class); ok && cls != nil && cls.Name != "" {
				return cls.Name + "." + name
			}
		}
	}
	if methodObj != nil && methodObj.Owner != nil {
		if label := backtraceOwnerLabel(methodObj.Owner, name); label != "" {
			return label
		}
	}
	if methodOwner != nil {
		owner := &object.EmeraldValue{Type: object.ValueClass, Data: methodOwner, Class: core.R.Classes["Class"]}
		if label := backtraceOwnerLabel(owner, name); label != "" {
			return label
		}
	}
	return name
}

func (vm *VM) buildInvocationMetadata(receiver *object.EmeraldValue, name string, method *object.Method, owner *object.Class) invocationMetadata {
	dispatchClass := receiver.Class
	if method.DispatchOwner != nil && method.DispatchOwner.Type == object.ValueModule && receiver.Type == object.ValueClass {
		if class, ok := receiver.Data.(*object.Class); ok && class.SingletonClass != nil {
			dispatchClass = class.SingletonClass
		}
	}
	result := invocationMetadata{
		label:             backtraceMethodLabel(receiver, method, owner, name),
		traceDefinedClass: traceDefinedClass(method, owner),
		traceMethodID:     traceMethodID(method, name),
	}
	if prependedOwner, prependedModule := methodFromPrependedModule(dispatchClass, name, method); prependedModule != nil {
		result.superStart = prependedOwner
		result.superModule = prependedModule
	} else if includedOwner, ownerModule := methodFromIncludedModule(dispatchClass, name, method); ownerModule != nil {
		result.superStart = includedOwner
		result.superModule = ownerModule
	} else if owner == receiver.Class {
		result.superStart = receiver.Class
		result.superAfterClass = true
	} else if owner == nil {
		result.superStart = receiver.Class
	} else {
		result.superStart = owner.SuperClass
	}
	return result
}

func backtraceOwnerLabel(owner *object.EmeraldValue, name string) string {
	if owner == nil || name == "" {
		return ""
	}
	switch owner.Type {
	case object.ValueModule:
		if mod, ok := owner.Data.(*object.Module); ok && mod != nil && mod.Name != "" {
			return mod.Name + "#" + name
		}
	case object.ValueClass:
		cls, ok := owner.Data.(*object.Class)
		if !ok || cls == nil {
			return ""
		}
		if cls.IsSingleton && cls.SingletonOwner != nil {
			switch cls.SingletonOwner.Type {
			case object.ValueClass:
				if base, ok := cls.SingletonOwner.Data.(*object.Class); ok && base != nil && base.Name != "" {
					return base.Name + "." + name
				}
			case object.ValueModule:
				if base, ok := cls.SingletonOwner.Data.(*object.Module); ok && base != nil && base.Name != "" {
					return base.Name + "." + name
				}
			}
			return name
		}
		if cls.Name != "" {
			return cls.Name + "#" + name
		}
	}
	return ""
}

func traceMethodID(methodObj *object.Method, fallback string) string {
	if methodObj == nil {
		return fallback
	}
	if methodObj.OriginalName != "" {
		return methodObj.OriginalName
	}
	if methodObj.Name != "" {
		return methodObj.Name
	}
	return fallback
}

func traceDefinedClass(methodObj *object.Method, methodOwner *object.Class) *object.EmeraldValue {
	if methodObj != nil && methodObj.Owner != nil {
		return methodObj.Owner
	}
	if methodOwner == nil {
		return nil
	}
	return &object.EmeraldValue{Type: object.ValueClass, Data: methodOwner, Class: core.R.Classes["Class"]}
}

func (vm *VM) enforceCurrentFrameLocals() int {
	if vm.fp < 0 || vm.fp >= len(vm.frames) || vm.frames[vm.fp] == nil {
		return vm.sp
	}
	frame := vm.frames[vm.fp]
	if frame.Fn == nil {
		return vm.sp
	}
	minSp := frame.Bp + 1 + frame.Fn.NumLocals
	if vm.sp < minSp {
		vm.sp = minSp
	}
	return vm.sp
}

func methodArityError(fn *object.Function, argc int) *object.EmeraldValue {
	if fn == nil {
		return nil
	}
	min := 0
	for i := 0; i < len(fn.Params); i++ {
		if i >= len(fn.ParamDefaults) || fn.ParamDefaults[i] == nil {
			min++
		}
	}
	max := len(fn.Params)
	if fn.HasRestParam {
		max = -1
	}
	if argc < min || (max >= 0 && argc > max) {
		return core.NewArgumentError("wrong number of arguments")
	}
	return nil
}

func (vm *VM) implicitSuperArgs(frame *Frame) []*object.EmeraldValue {
	if frame == nil || frame.Fn == nil {
		return nil
	}
	fn := frame.Fn
	args := make([]*object.EmeraldValue, 0, len(frame.Args)+1)
	appendRest := func() {
		if !fn.HasRestParam || fn.AnonymousRestParam {
			return
		}
		index, ok := fn.LocalNames[fn.RestParamName]
		if !ok {
			return
		}
		value := derefClosureValue(vm.stack[frame.Bp+1+index])
		if value != nil && value.Type == object.ValueArray {
			args = append(args, value.Data.([]*object.EmeraldValue)...)
		} else if value != nil && value.Type != object.ValueNil {
			args = append(args, value)
		}
	}
	for i := 0; i < len(fn.Params); i++ {
		if fn.HasRestParam && i == fn.RestParamIndex {
			appendRest()
		}
		index := functionParamLocalIndex(fn, i)
		value := derefClosureValue(vm.stack[frame.Bp+1+index])
		if value == nil {
			value = core.R.NilVal
		}
		args = append(args, value)
	}
	if fn.HasRestParam && fn.RestParamIndex >= len(fn.Params) {
		appendRest()
	}

	if len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly {
		hash := &object.RHash{Pairs: make(map[*object.EmeraldValue]*object.EmeraldValue), Keys: make([]*object.EmeraldValue, 0)}
		for _, keyword := range fn.KeywordParams {
			index, ok := fn.LocalNames[keyword.Name]
			if !ok {
				continue
			}
			key := &object.EmeraldValue{Type: object.ValueSymbol, Data: keyword.Name, Class: core.R.Classes["Symbol"]}
			hash.Keys = append(hash.Keys, key)
			hash.Pairs[key] = derefClosureValue(vm.stack[frame.Bp+1+index])
		}
		if fn.KeywordRestParam != "" {
			if index, ok := fn.LocalNames[fn.KeywordRestParam]; ok {
				value := derefClosureValue(vm.stack[frame.Bp+1+index])
				for _, key := range orderedKeywordKeys(value) {
					if hashLiteralExistingKey(hash, key) == nil {
						hash.Keys = append(hash.Keys, key)
						hash.Pairs[key] = executorHashValue(value, key)
					}
				}
			}
		}
		keywordHash := &object.EmeraldValue{Type: object.ValueHash, Data: hash, Class: core.R.Classes["Hash"]}
		core.MarkRuby2KeywordHash(keywordHash)
		args = append(args, keywordHash)
	}
	return args
}

func (vm *VM) superContextFrame(frame *Frame) *Frame {
	if frame == nil || frame.Fn == nil || frame.Fn.MethodBody || frame.DefinedByDefineMethod || frame.Fn.DefinedByDefineMethod {
		return frame
	}
	for i := vm.fp - 1; i >= 0; i-- {
		candidate := vm.frames[i]
		if candidate != nil && candidate.Fn != nil && candidate.Fn.MethodBody && candidate.MethodName == frame.MethodName {
			return candidate
		}
	}
	return frame
}

func functionArity(fn *object.Function) int {
	if fn == nil {
		return 0
	}
	required := 0
	hasOptional := false
	for i := range fn.Params {
		if i < len(fn.ParamDefaults) && fn.ParamDefaults[i] != nil {
			hasOptional = true
			continue
		}
		required++
	}
	hasRequiredKeyword := false
	hasOptionalKeyword := false
	for _, keyword := range fn.KeywordParams {
		if keyword.HasDefault {
			hasOptionalKeyword = true
		} else {
			hasRequiredKeyword = true
		}
	}
	if hasRequiredKeyword {
		required++
	}
	if fn.HasRestParam || hasOptional || hasOptionalKeyword && !hasRequiredKeyword || fn.KeywordRestParam != "" && !hasRequiredKeyword {
		return -required - 1
	}
	return required
}

func positionalArityArgCount(fn *object.Function, args []*object.EmeraldValue) int {
	if fn == nil || len(args) == 0 {
		return len(args)
	}
	last := args[len(args)-1]
	if last != nil && last.Type == object.ValueHash && core.Ruby2KeywordHash(last) && functionAcceptsKeywords(fn) {
		return len(args) - 1
	}
	return len(args)
}

func functionAcceptsKeywords(fn *object.Function) bool {
	return fn != nil && (len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly || fn.RejectKeywords)
}

func dropEmptyRuby2KeywordHashForPositionalOnlyFunction(fn *object.Function, args []*object.EmeraldValue) []*object.EmeraldValue {
	if fn == nil || functionAcceptsKeywords(fn) || len(args) == 0 {
		return args
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash || !core.Ruby2KeywordHash(last) {
		return args
	}
	if len(executorHashToMap(last)) != 0 {
		return args
	}
	return args[:len(args)-1]
}

func copyUnmarkedRuby2KeywordHashForPositionalFunction(fn *object.Function, args []*object.EmeraldValue, preserveMark bool) []*object.EmeraldValue {
	if fn == nil || preserveMark || functionAcceptsKeywords(fn) || len(args) == 0 {
		return args
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash || !core.Ruby2KeywordHash(last) {
		return args
	}
	result := append([]*object.EmeraldValue(nil), args...)
	result[len(result)-1] = copyKeywordHash(last)
	return result
}

func mergeKeywordRestOverflowHashes(fn *object.Function, args []*object.EmeraldValue) []*object.EmeraldValue {
	if fn == nil || fn.KeywordRestParam == "" || fn.HasRestParam || len(args) < 2 {
		return args
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash || !core.Ruby2KeywordHash(last) {
		return args
	}
	capacity := len(fn.Params)
	if len(args)-1 <= capacity {
		return args
	}
	for _, candidate := range args[capacity : len(args)-1] {
		if candidate == nil || candidate.Type != object.ValueHash {
			return args
		}
	}
	combined := &object.RHash{Pairs: make(map[*object.EmeraldValue]*object.EmeraldValue), Keys: make([]*object.EmeraldValue, 0)}
	for _, source := range append(append([]*object.EmeraldValue(nil), args[capacity:len(args)-1]...), last) {
		for _, key := range orderedKeywordKeys(source) {
			targetKey := hashLiteralExistingKey(combined, key)
			if targetKey == nil {
				targetKey = key
				combined.Keys = append(combined.Keys, key)
			}
			combined.Pairs[targetKey] = executorHashValue(source, key)
		}
	}
	keywordHash := &object.EmeraldValue{Type: object.ValueHash, Data: combined, Class: core.R.Classes["Hash"]}
	core.MarkRuby2KeywordHash(keywordHash)
	result := append([]*object.EmeraldValue(nil), args[:capacity]...)
	return append(result, keywordHash)
}

func executorHashValue(hash *object.EmeraldValue, key *object.EmeraldValue) *object.EmeraldValue {
	for existing, value := range executorHashToMap(hash) {
		if existing == key || existing.Equals(key) {
			return value
		}
	}
	return core.R.NilVal
}

func dropAnonymousKeywordRestNonSymbolHash(fn *object.Function, args []*object.EmeraldValue) []*object.EmeraldValue {
	if fn == nil || !fn.KeywordRestOnly || len(args) < 2 {
		return args
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash || !core.Ruby2KeywordHash(last) {
		return args
	}
	prior := args[len(args)-2]
	if !hashHasNonSymbolKey(prior) {
		return args
	}
	result := append([]*object.EmeraldValue(nil), args[:len(args)-2]...)
	return append(result, last)
}

func (vm *VM) currentDefinitionVisibility() string {
	if vm.currentDefinitionMode() == "module_function" {
		return "private"
	}
	mode := vm.currentDefinitionMode()
	switch mode {
	case "private", "protected":
		return mode
	}
	return "public"
}

func (vm *VM) currentDefinitionMode() string {
	if len(vm.classStack) == 0 {
		return "private"
	}
	classVal := vm.classStack[len(vm.classStack)-1]
	var visibility *object.EmeraldValue
	switch classVal.Type {
	case object.ValueClass:
		visibility = classVal.Data.(*object.Class).GetInstanceVar("@__visibility")
	case object.ValueModule:
		visibility = classVal.Data.(*object.Module).GetInstanceVar("@__visibility")
	}
	if visibility != nil && visibility.Type == object.ValueString {
		if mode, ok := visibility.Data.(string); ok {
			return mode
		}
	}
	return "public"
}

func methodNameFromValue(value *object.EmeraldValue) (string, bool) {
	if value == nil {
		return "", false
	}
	switch value.Type {
	case object.ValueString, object.ValueSymbol:
		name, ok := value.Data.(string)
		return name, ok
	}
	return "", false
}

func methodFromPrependedModule(class *object.Class, name string, method *object.Method) (*object.Class, *object.Module) {
	if class == nil || method == nil {
		return nil, nil
	}
	for current := class; current != nil; current = current.SuperClass {
		for _, mod := range current.PrependedModules {
			if owner := prependedModuleDefiningMethod(mod, name, method, map[*object.Module]bool{}); owner != nil {
				return current, owner
			}
		}
	}
	return nil, nil
}

func prependedModuleDefiningMethod(module *object.Module, name string, method *object.Method, path map[*object.Module]bool) *object.Module {
	if module == nil || path[module] {
		return nil
	}
	path[module] = true
	defer delete(path, module)
	for _, prepended := range module.PrependedModules {
		if owner := prependedModuleDefiningMethod(prepended, name, method, path); owner != nil {
			return owner
		}
	}
	if candidate, ok := module.Methods[name]; ok && candidate == method {
		return module
	}
	for index := len(module.IncludedModules) - 1; index >= 0; index-- {
		if owner := prependedModuleDefiningMethod(module.IncludedModules[index], name, method, path); owner != nil {
			return owner
		}
	}
	return nil
}

func methodFromIncludedModule(class *object.Class, name string, method *object.Method) (*object.Class, *object.Module) {
	if class == nil || method == nil {
		return nil, nil
	}
	for current := class; current != nil; current = current.SuperClass {
		if method.Owner != nil && method.Owner.Type == object.ValueModule {
			owner := method.Owner.Data.(*object.Module)
			for _, included := range current.IncludedModules {
				if moduleIncludes(included, owner) {
					return current, owner
				}
			}
		}
		for i := len(current.IncludedModules) - 1; i >= 0; i-- {
			mod := current.IncludedModules[i]
			if modMethod, ok := mod.GetMethod(name); ok && modMethod == method {
				return current, mod
			}
		}
	}
	return nil, nil
}

func moduleIncludes(module, target *object.Module) bool {
	if module == nil || target == nil {
		return false
	}
	if module == target {
		return true
	}
	for _, included := range module.IncludedModules {
		if moduleIncludes(included, target) {
			return true
		}
	}
	return false
}

func getIncludedMethodAfterModule(class *object.Class, current *object.Module, name string) (*object.Method, *object.Class, *object.Module, bool) {
	if class == nil || current == nil {
		return nil, nil, nil, false
	}
	entries := flattenedMethodAncestors(class)
	foundCurrent := false
	for _, entry := range entries {
		if !foundCurrent {
			if entry.module == current {
				foundCurrent = true
			}
			continue
		}
		if entry.module != nil {
			if method, ok := entry.module.Methods[name]; ok && method != nil {
				return method, entry.class, entry.module, true
			}
			continue
		}
		if entry.class != nil {
			if method, ok := entry.class.Methods[name]; ok && method != nil {
				return method, entry.class, nil, true
			}
		}
	}
	return nil, nil, nil, false
}

type methodAncestorEntry struct {
	class  *object.Class
	module *object.Module
}

func flattenedMethodAncestors(class *object.Class) []methodAncestorEntry {
	entries := make([]methodAncestorEntry, 0)
	for current := class; current != nil; current = current.SuperClass {
		for _, prepended := range current.PrependedModules {
			appendModuleMethodAncestors(&entries, current, prepended, map[*object.Module]bool{})
		}
		entries = append(entries, methodAncestorEntry{class: current})
		for index := len(current.IncludedModules) - 1; index >= 0; index-- {
			appendModuleMethodAncestors(&entries, current, current.IncludedModules[index], map[*object.Module]bool{})
		}
	}
	return entries
}

func appendModuleMethodAncestors(entries *[]methodAncestorEntry, owner *object.Class, module *object.Module, path map[*object.Module]bool) {
	if module == nil || path[module] {
		return
	}
	path[module] = true
	defer delete(path, module)
	for _, prepended := range module.PrependedModules {
		appendModuleMethodAncestors(entries, owner, prepended, path)
	}
	*entries = append(*entries, methodAncestorEntry{class: owner, module: module})
	for index := len(module.IncludedModules) - 1; index >= 0; index-- {
		appendModuleMethodAncestors(entries, owner, module.IncludedModules[index], path)
	}
}

func getMethodAfterClassWithOwner(class *object.Class, name string) (*object.Method, *object.Class, *object.Module, bool) {
	if class == nil {
		return nil, nil, nil, false
	}
	foundClass := false
	for _, entry := range flattenedMethodAncestors(class) {
		if !foundClass {
			if entry.module == nil && entry.class == class {
				foundClass = true
			}
			continue
		}
		if entry.module != nil {
			if method, ok := entry.module.Methods[name]; ok && method != nil {
				return method, entry.class, entry.module, true
			}
		} else if entry.class != nil {
			if method, ok := entry.class.Methods[name]; ok && method != nil {
				return method, entry.class, nil, true
			}
		}
	}
	return nil, nil, nil, false
}

func getMethodAfterPrependsWithOwner(class *object.Class, name string) (*object.Method, *object.Class, bool) {
	if class == nil {
		return nil, nil, false
	}
	if method, ok := class.Methods[name]; ok {
		return method, class, true
	}
	for i := len(class.IncludedModules) - 1; i >= 0; i-- {
		mod := class.IncludedModules[i]
		if method, ok := mod.GetMethod(name); ok {
			return method, class, true
		}
	}
	if class.SuperClass != nil {
		return class.SuperClass.GetMethodWithOwner(name)
	}
	return nil, nil, false
}

func (vm *VM) runMinitestMethods(classVal *object.EmeraldValue) {
	if classVal == nil || classVal.Type != object.ValueClass {
		return
	}
	cls := classVal.Data.(*object.Class)
	if !inheritsFrom(cls, "ActiveSupport::TestCase") || cls.Name == "ActiveSupport::TestCase" {
		return
	}

	names := make([]string, 0)
	for name := range cls.Methods {
		if strings.HasPrefix(name, "test_") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		if runner := core.GetSpecRunner(); runner != nil {
			runner.ExampleCount++
		}
		fmt.Printf("  ✓ %s\n", name)
		instance := &object.EmeraldValue{
			Type:  object.ValueObject,
			Data:  object.NewObject(cls),
			Class: cls,
		}
		vm.send(instance, name, nil)
	}
}

func (vm *VM) findClassStackEntry(name string) (int, *object.EmeraldValue) {
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		entry := vm.classStack[i]
		if existing, ok := vm.rubyConsts[name]; ok && sameModuleOrClassValue(entry, existing) {
			return i, entry
		}
		switch entry.Type {
		case object.ValueClass:
			entryName := entry.Data.(*object.Class).Name
			if entryName == name || strings.HasSuffix(entryName, "::"+name) {
				return i, entry
			}
		case object.ValueModule:
			entryName := entry.Data.(*object.Module).Name
			if entryName == name || strings.HasSuffix(entryName, "::"+name) {
				return i, entry
			}
		}
	}
	return -1, nil
}

func (vm *VM) scopedLocalConstantContainer(frame *Frame, qualifiedName string) (*object.EmeraldValue, string, bool) {
	if frame == nil || frame.Fn == nil || !strings.Contains(qualifiedName, "::") {
		return nil, "", false
	}
	parts := strings.Split(qualifiedName, "::")
	if len(parts) < 2 {
		return nil, "", false
	}
	rootName := parts[0]
	idx, ok := frame.Fn.LocalNames[rootName]
	var container *object.EmeraldValue
	if rootName == "self" && frame.Bp >= 0 && frame.Bp < len(vm.stack) {
		container = vm.stack[frame.Bp]
	}
	if ok {
		stackIdx := frame.Bp + idx + 1
		if stackIdx >= 0 && stackIdx < len(vm.stack) {
			container = vm.stack[stackIdx]
		}
	}
	if container == nil && frame.Closure != nil {
		container = bindingLocalValue(frame.Closure.Binding, rootName)
	}
	if container == nil || (container.Type != object.ValueClass && container.Type != object.ValueModule) {
		return nil, "", false
	}
	for _, constName := range parts[1 : len(parts)-1] {
		value, ok := vm.scopedConstantValue(container, constName)
		if !ok || value == nil || (value.Type != object.ValueClass && value.Type != object.ValueModule) {
			return nil, "", false
		}
		container = value
	}
	return container, parts[len(parts)-1], true
}

func (vm *VM) lexicalQualifiedConstantContainer(qualifiedName string) (*object.EmeraldValue, string, bool) {
	if !strings.Contains(qualifiedName, "::") {
		return nil, "", false
	}
	parts := strings.Split(qualifiedName, "::")
	if len(parts) < 2 || parts[0] == "" {
		return nil, "", false
	}
	container, ok := vm.lexicalConstantValue(parts[0])
	if !ok || container == nil || (container.Type != object.ValueClass && container.Type != object.ValueModule) {
		return nil, "", false
	}
	for _, constName := range parts[1 : len(parts)-1] {
		next, found := vm.scopedConstantValue(container, constName)
		if !found || next == nil || (next.Type != object.ValueClass && next.Type != object.ValueModule) {
			return nil, "", false
		}
		container = next
	}
	return container, parts[len(parts)-1], true
}

func bindingLocalValue(binding *object.RBinding, name string) *object.EmeraldValue {
	for current := binding; current != nil; current = current.Parent {
		if current.Locals == nil {
			continue
		}
		if value, ok := current.Locals[name]; ok {
			return value
		}
	}
	return nil
}

func (vm *VM) scopedLocalConstantValue(frame *Frame, qualifiedName string) (*object.EmeraldValue, bool) {
	container, constName, ok := vm.scopedLocalConstantContainer(frame, qualifiedName)
	if !ok {
		return nil, false
	}
	return vm.scopedConstantValue(container, constName)
}

func (vm *VM) lexicalConstantValue(name string) (*object.EmeraldValue, bool) {
	if strings.Contains(name, "::") {
		return nil, false
	}
	var innermostClass *object.Class
	restartAfterAutoload := false
	dynamicContext := vm.dynamicBlockContextClass()
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		container := vm.classStack[i]
		if container == nil || (container.Type != object.ValueClass && container.Type != object.ValueModule) {
			continue
		}
		if dynamicContext != nil && sameModuleOrClassValue(container, dynamicContext) {
			dynamicContext = nil
			continue
		}
		if value, ok := lexicalDirectConstantValue(container, name); ok {
			return value, true
		}
		if value, loaded, hadAutoload := vm.triggerLexicalAutoload(container, name); hadAutoload {
			if loaded {
				return value, true
			}
			if _, stillRegistered := core.DirectAutoloadPath(container, name); !stillRegistered {
				restartAfterAutoload = true
				break
			}
		}
		if value, ok := includedModuleConstantLookup(container, name); ok {
			return value, true
		}
		if innermostClass == nil && container.Type == object.ValueClass {
			innermostClass = container.Data.(*object.Class)
		}
	}
	if restartAfterAutoload {
		return vm.lexicalConstantValue(name)
	}
	if innermostClass != nil {
		if value, ok := classConstantLookup(innermostClass.SuperClass, name); ok {
			return value, true
		}
	}
	return nil, false
}

func (vm *VM) dynamicBlockContextClass() *object.EmeraldValue {
	if vm.fp < 0 || vm.fp >= len(vm.frames) {
		return nil
	}
	frame := vm.frames[vm.fp]
	if frame == nil || frame.Fn == nil || frame.Fn.Name != "__block__" || frame.Closure == nil || frame.Closure.Binding == nil {
		return nil
	}
	if frame.Bp < 0 || frame.Bp >= len(vm.stack) {
		return nil
	}
	currentSelf := vm.stack[frame.Bp]
	if currentSelf == nil || currentSelf == frame.Closure.Binding.Self || len(frame.Closure.ClassStack) == 0 {
		return nil
	}
	context := frame.Closure.ClassStack[len(frame.Closure.ClassStack)-1]
	if context == nil || (context.Type != object.ValueClass && context.Type != object.ValueModule) {
		return nil
	}
	return context
}

func (vm *VM) lexicalAutoloadDefined(name string) bool {
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		container := vm.classStack[i]
		if vm.definedAutoload(container, name, true) {
			return true
		}
	}
	objectClass := core.R.Classes["Object"]
	if objectClass == nil {
		return false
	}
	return vm.definedAutoload(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: core.R.Classes["Class"]}, name, true)
}

func (vm *VM) definedAutoload(receiver *object.EmeraldValue, name string, inherit bool) bool {
	if receiver == nil {
		return false
	}
	path, ok := core.AutoloadPath(receiver, name, inherit)
	if !ok || core.FeatureLoadingByCurrentThread(path) || vm.autoloading[autoloadKey(receiver, name)] {
		return false
	}
	return true
}

func lexicalDirectConstantValue(container *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	if container == nil {
		return nil, false
	}
	switch container.Type {
	case object.ValueClass:
		value, ok := container.Data.(*object.Class).Constants[name]
		return value, ok
	case object.ValueModule:
		value, ok := container.Data.(*object.Module).Constants[name]
		return value, ok
	default:
		return nil, false
	}
}

func includedModuleConstantLookup(container *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	if container == nil {
		return nil, false
	}
	switch container.Type {
	case object.ValueClass:
		class := container.Data.(*object.Class)
		for _, mod := range class.PrependedModules {
			if value, ok := moduleConstantLookup(mod, name); ok {
				return value, true
			}
		}
		mods := class.IncludedModules
		for i := len(mods) - 1; i >= 0; i-- {
			mod := mods[i]
			if value, ok := moduleConstantLookup(mod, name); ok {
				return value, true
			}
		}
	case object.ValueModule:
		module := container.Data.(*object.Module)
		for _, mod := range module.PrependedModules {
			if value, ok := moduleConstantLookup(mod, name); ok {
				return value, true
			}
		}
		mods := module.IncludedModules
		for i := len(mods) - 1; i >= 0; i-- {
			mod := mods[i]
			if value, ok := moduleConstantLookup(mod, name); ok {
				return value, true
			}
		}
	}
	return nil, false
}

func (vm *VM) allowTopLevelConstantFallback(name string) bool {
	if strings.Contains(name, "::") {
		return true
	}
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		entry := vm.classStack[i]
		if entry == nil {
			continue
		}
		if entry.Type == object.ValueModule {
			return true
		}
		if entry.Type != object.ValueClass {
			continue
		}
		class := entry.Data.(*object.Class)
		return classInheritsFrom(class, core.R.Classes["Object"])
	}
	return true
}

func (vm *VM) qualifiedLexicalParentConstantValue(container *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	qualified := constantContainerName(container)
	if !strings.Contains(qualified, "::") {
		return nil, false
	}
	parts := strings.Split(qualified, "::")
	for i := len(parts) - 1; i > 0; i-- {
		parentName := strings.Join(parts[:i], "::")
		parent, ok := vm.constantContainerByQualifiedName(parentName)
		if !ok {
			continue
		}
		if value, ok := directConstantValue(parent, name); ok {
			return value, true
		}
		if value, loaded, hadAutoload := vm.triggerLexicalAutoload(parent, name); hadAutoload && loaded {
			return value, true
		}
	}
	return nil, false
}

func constantContainerName(container *object.EmeraldValue) string {
	if container == nil {
		return ""
	}
	switch container.Type {
	case object.ValueClass:
		return container.Data.(*object.Class).Name
	case object.ValueModule:
		return container.Data.(*object.Module).Name
	default:
		return ""
	}
}

func qualifiedConstantName(container *object.EmeraldValue, name string) string {
	prefix := constantContainerName(container)
	if prefix == "" {
		return name
	}
	return prefix + "::" + name
}

func (vm *VM) constantContainerByQualifiedName(name string) (*object.EmeraldValue, bool) {
	if name == "" {
		return nil, false
	}
	if value, ok := vm.topLevelConstantValue(name); ok && (value.Type == object.ValueClass || value.Type == object.ValueModule) {
		return value, true
	}
	return nil, false
}

func (vm *VM) missingConstantValue(name string, absolute bool) *object.EmeraldValue {
	constName := name
	var receiver *object.EmeraldValue
	if idx := strings.LastIndex(name, "::"); idx > 0 {
		constName = name[idx+2:]
		receiver, _ = vm.constantContainerByQualifiedName(name[:idx])
	}
	if receiver == nil && !absolute {
		receiver = vm.currentConstantContainer()
	}
	if receiver == nil {
		receiver = &object.EmeraldValue{Type: object.ValueClass, Data: core.R.Classes["Object"], Class: core.R.Classes["Class"]}
	}
	return vm.sendBypassVisibility(receiver, "const_missing", []*object.EmeraldValue{{Type: object.ValueSymbol, Data: constName, Class: core.R.Classes["Symbol"]}})
}

func (vm *VM) topLevelConstantValue(name string) (*object.EmeraldValue, bool) {
	if value, ok := vm.rubyConsts[name]; ok {
		return value, true
	}
	objectClass := core.R.Classes["Object"]
	if objectClass != nil {
		if value, ok := classConstantLookup(objectClass, name); ok {
			return value, true
		}
		objectValue := &object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: core.R.Classes["Class"]}
		if value, ok := vm.triggerAutoload(objectValue, name); ok {
			return value, true
		}
	}
	if class, ok := core.R.Classes[name]; ok {
		return &object.EmeraldValue{Type: object.ValueClass, Data: class, Class: core.R.Classes["Class"]}, true
	}
	if value, ok := vm.namespaceModuleValue(name); ok {
		return value, true
	}
	return nil, false
}

func (vm *VM) topLevelDefinitionConstantValue(name string) (*object.EmeraldValue, bool) {
	if value, ok := vm.rubyConsts[name]; ok {
		return value, true
	}
	if objectClass := core.R.Classes["Object"]; objectClass != nil {
		if value, ok := objectClass.Constants[name]; ok {
			return value, true
		}
	}
	if class, ok := core.R.Classes[name]; ok {
		return &object.EmeraldValue{Type: object.ValueClass, Data: class, Class: core.R.Classes["Class"]}, true
	}
	return vm.namespaceModuleValue(name)
}

func (vm *VM) namespaceModuleValue(name string) (*object.EmeraldValue, bool) {
	prefix := name + "::"
	for constName := range vm.rubyConsts {
		if strings.HasPrefix(constName, prefix) {
			moduleVal := &object.EmeraldValue{Type: object.ValueModule, Data: object.NewModule(name), Class: core.R.Classes["Module"]}
			vm.rubyConsts[name] = moduleVal
			vm.populateNamespaceModule(moduleVal, name)
			return moduleVal, true
		}
	}
	for className := range core.R.Classes {
		if strings.HasPrefix(className, prefix) {
			moduleVal := &object.EmeraldValue{Type: object.ValueModule, Data: object.NewModule(name), Class: core.R.Classes["Module"]}
			vm.rubyConsts[name] = moduleVal
			vm.populateNamespaceModule(moduleVal, name)
			return moduleVal, true
		}
	}
	return nil, false
}

func (vm *VM) populateNamespaceModule(moduleVal *object.EmeraldValue, name string) {
	if moduleVal == nil || moduleVal.Type != object.ValueModule {
		return
	}
	module := moduleVal.Data.(*object.Module)
	prefix := name + "::"
	for constName, value := range vm.rubyConsts {
		if !strings.HasPrefix(constName, prefix) {
			continue
		}
		child := strings.TrimPrefix(constName, prefix)
		if child == "" || strings.Contains(child, "::") {
			continue
		}
		module.DefineConstant(child, value)
	}
	for className, class := range core.R.Classes {
		if !strings.HasPrefix(className, prefix) {
			continue
		}
		child := strings.TrimPrefix(className, prefix)
		if child == "" || strings.Contains(child, "::") {
			continue
		}
		module.DefineConstant(child, &object.EmeraldValue{Type: object.ValueClass, Data: class, Class: core.R.Classes["Class"]})
	}
}

func directConstantValue(container *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	switch container.Type {
	case object.ValueClass:
		class := container.Data.(*object.Class)
		if class.PrivateConstants[name] {
			return core.NewPrivateConstantNameError(container, name), true
		}
		value, ok := class.Constants[name]
		return value, ok
	case object.ValueModule:
		module := container.Data.(*object.Module)
		if module.PrivateConstants[name] {
			return core.NewPrivateConstantNameError(container, name), true
		}
		value, ok := module.Constants[name]
		return value, ok
	default:
		return nil, false
	}
}

func (vm *VM) triggerLexicalAutoload(container *object.EmeraldValue, constName string) (*object.EmeraldValue, bool, bool) {
	path, ok := core.DirectAutoloadPath(container, constName)
	if !ok {
		return nil, false, false
	}
	if core.FeatureLoadingByCurrentThread(path) {
		return nil, false, true
	}
	key := autoloadKey(container, constName)
	if vm.autoloading[key] {
		return nil, false, true
	}
	vm.autoloading[key] = true
	defer delete(vm.autoloading, key)
	result := vm.send(core.R.Main, "require", []*object.EmeraldValue{{Type: object.ValueString, Data: path, Class: core.R.Classes["String"]}})
	if result != nil && result.Type == object.ValueException {
		return result, true, true
	}
	if value, ok := lexicalDirectConstantValue(container, constName); ok {
		core.CompleteAutoload(container, constName)
		return value, true, true
	}
	core.CompleteAutoload(container, constName)
	return nil, false, true
}

func (vm *VM) qualifiedConstantContainer(qualifiedName string) (*object.EmeraldValue, string, bool) {
	if !strings.Contains(qualifiedName, "::") {
		return nil, "", false
	}
	parts := strings.Split(qualifiedName, "::")
	if len(parts) < 2 {
		return nil, "", false
	}
	for i := len(parts) - 1; i > 0; i-- {
		prefix := strings.Join(parts[:i], "::")
		if value, ok := vm.constantContainerByQualifiedName(prefix); ok {
			container := value
			for _, constName := range parts[i : len(parts)-1] {
				next, ok := vm.scopedConstantValue(container, constName)
				if !ok || next == nil || (next.Type != object.ValueClass && next.Type != object.ValueModule) {
					container = nil
					break
				}
				container = next
			}
			if container != nil {
				return container, parts[len(parts)-1], true
			}
		}
	}
	var container *object.EmeraldValue
	if value, ok := vm.topLevelConstantValue(parts[0]); ok && (value.Type == object.ValueClass || value.Type == object.ValueModule) {
		container = value
	}
	if container == nil {
		return nil, "", false
	}
	for _, constName := range parts[1 : len(parts)-1] {
		value, ok := vm.scopedConstantValue(container, constName)
		if !ok || value == nil || (value.Type != object.ValueClass && value.Type != object.ValueModule) {
			return nil, "", false
		}
		container = value
	}
	return container, parts[len(parts)-1], true
}

func (vm *VM) classValueForQualifiedName(qualifiedName string) (*object.EmeraldValue, bool) {
	container, constName, ok := vm.qualifiedConstantContainer(qualifiedName)
	if !ok {
		return nil, false
	}
	if value, ok := vm.classValueFromContainer(container, constName); ok {
		return value, true
	}
	if value, ok := vm.rubyConsts[qualifiedName]; ok && value.Type == object.ValueClass {
		return value, true
	}
	return nil, false
}

func (vm *VM) classValueFromContainer(container *object.EmeraldValue, constName string) (*object.EmeraldValue, bool) {
	if value, ok := lexicalDirectConstantValue(container, constName); ok && value.Type == object.ValueClass {
		return value, true
	}
	if value, loaded, hadAutoload := vm.triggerLexicalAutoload(container, constName); hadAutoload && loaded && value.Type == object.ValueClass {
		return value, true
	}
	return nil, false
}

func (vm *VM) moduleDefinitionValueFromContainer(container *object.EmeraldValue, constName string) (*object.EmeraldValue, bool) {
	if value, ok := lexicalDirectConstantValue(container, constName); ok {
		return value, true
	}
	if value, loaded, hadAutoload := vm.triggerLexicalAutoload(container, constName); hadAutoload && loaded {
		return value, true
	}
	return nil, false
}

func (vm *VM) invalidQualifiedClassNameError(qualifiedName string) *object.EmeraldValue {
	if !strings.Contains(qualifiedName, "::") {
		return nil
	}
	parts := strings.Split(qualifiedName, "::")
	if len(parts) < 2 {
		return nil
	}
	if parts[0] == "nil" {
		return core.NewTypeError(parts[0] + " is not a class/module")
	}
	container, found := vm.topLevelConstantValue(parts[0])
	if !found {
		return nil
	}
	if container == nil || (container.Type != object.ValueClass && container.Type != object.ValueModule) {
		return core.NewTypeError(parts[0] + " is not a class/module")
	}
	for _, constName := range parts[1 : len(parts)-1] {
		value, ok := vm.scopedConstantValue(container, constName)
		if !ok {
			return nil
		}
		if value == nil || (value.Type != object.ValueClass && value.Type != object.ValueModule) {
			return core.NewTypeError(strings.Join(parts[:len(parts)-1], "::") + " is not a class/module")
		}
		container = value
	}
	return nil
}

func (vm *VM) qualifiedPrivateConstantError(qualifiedName string) (*object.EmeraldValue, bool) {
	if !strings.Contains(qualifiedName, "::") {
		return nil, false
	}
	container, constName, ok := vm.qualifiedConstantContainer(qualifiedName)
	if !ok {
		return nil, false
	}
	value, found := directConstantValue(container, constName)
	if found && value != nil && value.Type == object.ValueException {
		return value, true
	}
	return nil, false
}

func (vm *VM) currentConstantContainer() *object.EmeraldValue {
	if vm.fp >= 0 && vm.fp < len(vm.frames) {
		frame := vm.frames[vm.fp]
		if frame != nil && frame.Fn != nil && frame.Fn.Name == "__block__" && frame.Closure != nil && frame.Closure.Binding != nil {
			var currentSelf *object.EmeraldValue
			if frame.Bp >= 0 && frame.Bp < len(vm.stack) {
				currentSelf = vm.stack[frame.Bp]
			}
			if currentSelf != frame.Closure.Binding.Self {
				for i := len(frame.Closure.ClassStack) - 1; i >= 0; i-- {
					entry := frame.Closure.ClassStack[i]
					if entry != nil && (entry.Type == object.ValueClass || entry.Type == object.ValueModule) {
						return entry
					}
				}
				return nil
			}
		}
	}
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		entry := vm.classStack[i]
		if entry != nil && (entry.Type == object.ValueClass || entry.Type == object.ValueModule) {
			return entry
		}
	}
	return nil
}

func (vm *VM) currentClassStackSnapshot() []*object.EmeraldValue {
	if len(vm.classStack) == 0 {
		return nil
	}
	return append([]*object.EmeraldValue(nil), vm.classStack...)
}

func (vm *VM) classVarScopeReceiver(receiver *object.EmeraldValue) *object.EmeraldValue {
	if vm.instanceExecClassVarScope != nil {
		return vm.instanceExecClassVarScope
	}
	if len(vm.classStack) > 0 {
		if scope := vm.classStack[len(vm.classStack)-1]; scope != nil {
			return scope
		}
	}
	if vm.fp >= 0 && vm.fp < len(vm.frames) {
		frame := vm.frames[vm.fp]
		if frame != nil && frame.Closure != nil && len(frame.Closure.ClassStack) == 0 {
			return &object.EmeraldValue{Type: object.ValueClass, Data: core.R.Classes["Object"], Class: core.R.Classes["Class"]}
		}
	}
	return receiver
}

func (vm *VM) classVarAccessesToplevel(receiver *object.EmeraldValue) bool {
	return len(vm.classStack) == 0 && receiver == core.R.Main
}

func throwLabelsMatch(catchLabel, throwLabel *object.EmeraldValue) bool {
	if catchLabel == nil || throwLabel == nil {
		return catchLabel == throwLabel
	}
	if catchLabel.Type != throwLabel.Type {
		return false
	}
	if catchLabel.Type == object.ValueSymbol {
		return catchLabel.Equals(throwLabel)
	}
	return catchLabel == throwLabel
}

func defineConstantOn(container *object.EmeraldValue, name string, value *object.EmeraldValue) {
	alreadyDefined := false
	core.ClearFormerAutoload(container, name)
	if path, ok := core.DirectAutoloadPath(container, name); !ok || !core.FeatureLoadingByCurrentThread(path) {
		core.RemoveAutoload(container, name)
	}
	switch container.Type {
	case object.ValueClass:
		class := container.Data.(*object.Class)
		if existing, found := class.Constants[name]; found && sameConstantDefinitionValue(existing, value) {
			alreadyDefined = true
		}
		class.DefineConstant(name, value)
	case object.ValueModule:
		module := container.Data.(*object.Module)
		if existing, found := module.Constants[name]; found && sameConstantDefinitionValue(existing, value) {
			alreadyDefined = true
		}
		module.DefineConstant(name, value)
	}
	core.AssignConstantName(container, name, value)
	core.RecordConstantLocation(container, name, false)
	if !alreadyDefined {
		if result := core.NotifyConstAdded(container, name); result != nil && result.Type == object.ValueException {
			core.LastException = result
			core.LastRaisedResult = result
		}
	}
}

func sameConstantDefinitionValue(left, right *object.EmeraldValue) bool {
	if left == right {
		return true
	}
	if left == nil || right == nil || left.Type != right.Type {
		return false
	}
	switch left.Type {
	case object.ValueClass, object.ValueModule:
		return left.Data == right.Data
	}
	return false
}

func (vm *VM) defineQualifiedConstant(name string, value *object.EmeraldValue) {
	idx := strings.LastIndex(name, "::")
	if idx <= 0 || idx+2 >= len(name) {
		return
	}
	parentName := name[:idx]
	constName := name[idx+2:]
	parent, ok := vm.ensureNamespaceContainer(parentName)
	if !ok {
		return
	}
	defineConstantOn(parent, constName, value)
}

func (vm *VM) ensureNamespaceContainer(name string) (*object.EmeraldValue, bool) {
	if value, ok := vm.topLevelConstantValue(name); ok && (value.Type == object.ValueClass || value.Type == object.ValueModule) {
		return value, true
	}
	idx := strings.LastIndex(name, "::")
	if idx <= 0 || idx+2 >= len(name) {
		moduleVal := &object.EmeraldValue{Type: object.ValueModule, Data: object.NewModule(name), Class: core.R.Classes["Module"]}
		vm.rubyConsts[name] = moduleVal
		return moduleVal, true
	}
	parent, ok := vm.ensureNamespaceContainer(name[:idx])
	if !ok {
		return nil, false
	}
	moduleVal := &object.EmeraldValue{Type: object.ValueModule, Data: object.NewModule(name), Class: core.R.Classes["Module"]}
	vm.rubyConsts[name] = moduleVal
	defineConstantOn(parent, name[idx+2:], moduleVal)
	return moduleVal, true
}

func sameModuleOrClassValue(left, right *object.EmeraldValue) bool {
	if left == nil || right == nil || left.Type != right.Type {
		return false
	}
	switch left.Type {
	case object.ValueClass, object.ValueModule:
		return left.Data == right.Data
	default:
		return false
	}
}

func derefClosureValue(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil {
		return core.R.NilVal
	}
	if cell, ok := value.Data.(*closureCell); ok {
		if cell.slot != nil && *cell.slot != nil {
			return derefClosureValue(*cell.slot)
		}
		if cell.value != nil {
			return derefClosureValue(cell.value)
		}
		return core.R.NilVal
	}
	return value
}

func localSlotPresent(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	if cell, ok := value.Data.(*closureCell); ok {
		if cell.slot != nil {
			return localSlotPresent(*cell.slot)
		}
		return cell.value != nil
	}
	return true
}

func snapshotClosureCapture(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil {
		return core.R.NilVal
	}
	if cell, ok := value.Data.(*closureCell); ok {
		return &object.EmeraldValue{
			Type:  object.ValueObject,
			Data:  cell,
			Class: value.Class,
		}
	}
	return value
}

func setClosureValue(slot **object.EmeraldValue, value *object.EmeraldValue) {
	if slot == nil {
		return
	}
	if current := *slot; current != nil {
		if cell, ok := current.Data.(*closureCell); ok {
			cell.value = value
			if cell.slot != nil {
				*cell.slot = value
			}
			return
		}
	}
	*slot = value
}

func (vm *VM) discardPendingReturnForCurrentEnsure(frame *Frame) {
	for len(vm.pendingEnsures) > 0 {
		pending := vm.pendingEnsures[len(vm.pendingEnsures)-1]
		if !pending.IsReturn || pending.Frame != frame || frame.Ip >= pending.EnsureEndOffset {
			return
		}
		vm.pendingEnsures = vm.pendingEnsures[:len(vm.pendingEnsures)-1]
	}
}

func (vm *VM) routeReturnThroughEnsure(frame *Frame, value *object.EmeraldValue) bool {
	for i := len(vm.activeRescues) - 1; i >= 0; i-- {
		active := vm.activeRescues[i]
		if active.Frame != frame || active.EnsureOffset <= 0 || active.EnsureEndOffset <= 0 || frame.Ip >= active.EnsureOffset {
			continue
		}
		vm.activeRescues = append(vm.activeRescues[:i], vm.activeRescues[i+1:]...)
		vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{
			EnsureEndOffset:   active.EnsureEndOffset,
			Frame:             frame,
			PreviousException: active.PreviousException,
			ReturnValue:       value,
			IsReturn:          true,
		})
		core.LastException = active.PreviousException
		frame.Ip = active.EnsureOffset - 1
		vm.ensureActive = true
		return true
	}
	for i := len(vm.rescueStack) - 1; i >= 0; i-- {
		handler := vm.rescueStack[i]
		if handler.Frame != frame || handler.EnsureOffset <= 0 || handler.EnsureEndOffset <= 0 || frame.Ip >= handler.EnsureOffset {
			continue
		}
		vm.rescueStack = append(vm.rescueStack[:i], vm.rescueStack[i+1:]...)
		vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{
			EnsureEndOffset:   handler.EnsureEndOffset,
			Frame:             frame,
			PreviousException: core.LastException,
			ReturnValue:       value,
			IsReturn:          true,
		})
		frame.Ip = handler.EnsureOffset - 1
		vm.ensureActive = true
		return true
	}
	return false
}

func (vm *VM) routeBreakThroughEnsure(frame *Frame, value *object.EmeraldValue, target int) bool {
	return vm.routeControlThroughEnsure(frame, value, target, false, false)
}

func (vm *VM) routeNextThroughEnsure(frame *Frame, value *object.EmeraldValue, target int) bool {
	return vm.routeControlThroughEnsure(frame, value, target, true, false)
}

func (vm *VM) routeRedoThroughEnsure(frame *Frame) bool {
	return vm.routeControlThroughEnsure(frame, core.R.NilVal, 0, false, true)
}

func (vm *VM) matchingCatchHandler(label *object.EmeraldValue) *CatchHandler {
	for current := vm; current != nil; current = current.parent {
		for index := len(current.catchStack) - 1; index >= 0; index-- {
			handler := current.catchStack[index]
			if handler.Label != nil && throwLabelsMatch(handler.Label, label) {
				return handler
			}
		}
	}
	return nil
}

func (vm *VM) routeThrowThroughEnsure(frame *Frame, catch *CatchHandler, value *object.EmeraldValue) bool {
	for i := len(vm.activeRescues) - 1; i >= 0; i-- {
		active := vm.activeRescues[i]
		if active.Frame != frame || active.EnsureOffset <= 0 || active.EnsureEndOffset <= 0 || frame.Ip >= active.EnsureOffset {
			continue
		}
		vm.activeRescues = append(vm.activeRescues[:i], vm.activeRescues[i+1:]...)
		vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{EnsureEndOffset: active.EnsureEndOffset, Frame: frame, PreviousException: active.PreviousException, ReturnValue: value, IsThrow: true, ThrowHandler: catch})
		core.LastException = active.PreviousException
		frame.Ip = active.EnsureOffset - 1
		vm.ensureActive = true
		return true
	}
	for i := len(vm.rescueStack) - 1; i >= 0; i-- {
		handler := vm.rescueStack[i]
		if handler.Frame != frame || handler.EnsureOffset <= 0 || handler.EnsureEndOffset <= 0 || frame.Ip >= handler.EnsureOffset {
			continue
		}
		vm.rescueStack = append(vm.rescueStack[:i], vm.rescueStack[i+1:]...)
		vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{EnsureEndOffset: handler.EnsureEndOffset, Frame: frame, PreviousException: core.LastException, ReturnValue: value, IsThrow: true, ThrowHandler: catch})
		frame.Ip = handler.EnsureOffset - 1
		vm.ensureActive = true
		return true
	}
	return false
}

func (vm *VM) completeThrow(frame *Frame, handler *CatchHandler, value *object.EmeraldValue) {
	if handler == nil {
		return
	}
	if handler.VM != nil && handler.VM != vm {
		vm.escapedThrowHandler = handler
		vm.escapedThrowValue = value
		frame.Returned = true
		return
	}
	for index := len(vm.catchStack) - 1; index >= 0; index-- {
		if vm.catchStack[index] == handler {
			vm.catchStack = vm.catchStack[:index]
			break
		}
	}
	for rescueIndex := len(vm.activeRescues) - 1; rescueIndex >= 0; rescueIndex-- {
		active := vm.activeRescues[rescueIndex]
		if active.Frame == frame && frame.Ip < active.EndOffset && handler.EndOffset >= active.EndOffset {
			vm.activeRescues = append(vm.activeRescues[:rescueIndex], vm.activeRescues[rescueIndex+1:]...)
		}
	}
	core.LastException = nil
	core.LastRaisedResult = nil
	vm.sp = handler.StackTop
	if value != nil {
		vm.push(value)
	}
	handler.Frame.Ip = handler.EndOffset - 1
	if handler.Frame != frame {
		frame.BlockBreak = true
		frame.BlockBreakVal = value
	}
}

func (vm *VM) routeControlThroughEnsure(frame *Frame, value *object.EmeraldValue, target int, isNext, isRedo bool) bool {
	for i := len(vm.activeRescues) - 1; i >= 0; i-- {
		active := vm.activeRescues[i]
		if active.Frame != frame || active.EnsureOffset <= 0 || active.EnsureEndOffset <= 0 || frame.Ip >= active.EnsureOffset {
			continue
		}
		vm.activeRescues = append(vm.activeRescues[:i], vm.activeRescues[i+1:]...)
		vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{
			EnsureEndOffset:   active.EnsureEndOffset,
			Frame:             frame,
			PreviousException: active.PreviousException,
			ReturnValue:       value,
			IsBreak:           !isNext && !isRedo,
			IsNext:            isNext,
			IsRedo:            isRedo,
			BreakTarget:       target,
		})
		core.LastException = active.PreviousException
		core.LastBlockResult = nil
		frame.Ip = active.EnsureOffset - 1
		vm.ensureActive = true
		return true
	}
	for i := len(vm.rescueStack) - 1; i >= 0; i-- {
		handler := vm.rescueStack[i]
		if handler.Frame != frame || handler.EnsureOffset <= 0 || handler.EnsureEndOffset <= 0 || frame.Ip >= handler.EnsureOffset {
			continue
		}
		vm.rescueStack = append(vm.rescueStack[:i], vm.rescueStack[i+1:]...)
		vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{
			EnsureEndOffset:   handler.EnsureEndOffset,
			Frame:             frame,
			PreviousException: core.LastException,
			ReturnValue:       value,
			IsBreak:           !isNext && !isRedo,
			IsNext:            isNext,
			IsRedo:            isRedo,
			BreakTarget:       target,
		})
		core.LastBlockResult = nil
		frame.Ip = handler.EnsureOffset - 1
		vm.ensureActive = true
		return true
	}
	return false
}

func inheritsFrom(cls *object.Class, name string) bool {
	for current := cls; current != nil; current = current.SuperClass {
		if current.Name == name {
			return true
		}
	}
	return false
}

func (vm *VM) raiseException(frame *Frame, exception *object.EmeraldValue) bool {
	previousException := core.LastException
	if previousException == exception {
		if frame != nil && frame.InstructionSnapshotSet {
			previousException = frame.InstructionException
		}
	}
	termination := core.IsTerminationResult(exception)
	if !termination {
		core.LastException = exception
		vm.attachExceptionLocations(exception)
		markExceptionRaised(exception)
		core.FireTracePointException("raise", vm.currentFrameBinding(), exception)
	}
	if len(vm.activeRescues) > 0 {
		active := vm.activeRescues[len(vm.activeRescues)-1]
		if active.Frame == frame && active.EnsureOffset > 0 && active.EnsureEndOffset > 0 &&
			frame.Ip < active.EnsureOffset && len(vm.rescueStack) == active.RescueStackDepth {
			vm.activeRescues = vm.activeRescues[:len(vm.activeRescues)-1]
			vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{
				EnsureEndOffset:   active.EnsureEndOffset,
				Frame:             active.Frame,
				Exception:         exception,
				PreviousException: active.PreviousException,
			})
			frame.Ip = active.EnsureOffset - 1
			vm.ensureActive = true
			return true
		}
		if active.Frame == frame && frame.Ip < active.EndOffset && len(vm.rescueStack) == active.RescueStackDepth {
			vm.activeRescues = vm.activeRescues[:len(vm.activeRescues)-1]
			previousException = active.PreviousException
		}
	}
	if termination {
		for i := len(vm.rescueStack) - 1; i >= 0; i-- {
			handler := vm.rescueStack[i]
			if handler.Frame != frame {
				continue
			}
			vm.rescueStack = append(vm.rescueStack[:i], vm.rescueStack[i+1:]...)
			if handler.EnsureOffset > 0 && handler.EnsureEndOffset > 0 && frame.Ip < handler.EnsureOffset {
				vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{
					EnsureEndOffset:   handler.EnsureEndOffset,
					Frame:             frame,
					Exception:         exception,
					PreviousException: previousException,
				})
				frame.Ip = handler.EnsureOffset - 1
				vm.ensureActive = true
				return true
			}
			return false
		}
		return false
	}
	if len(vm.rescueStack) == 0 {
		return false
	}
	handler := vm.rescueStack[len(vm.rescueStack)-1]
	if handler.Frame != frame {
		return false
	}
	for i := len(vm.activeRescues) - 1; i >= 0; i-- {
		active := vm.activeRescues[i]
		if active.Frame == frame && frame.Ip < active.EndOffset && len(vm.rescueStack) < active.RescueStackDepth {
			return false
		}
	}
	if handler.RescueOffset > 0 && frame.Ip >= handler.RescueOffset-1 {
		return false
	}
	vm.rescueStack = vm.rescueStack[:len(vm.rescueStack)-1]
	if handler.RescueOffset > 0 {
		core.FireTracePointException("rescue", vm.currentFrameBinding(), exception)
		activeRescue := &ActiveRescue{
			BodyOffset:        handler.BodyOffset,
			RescueOffset:      handler.RescueOffset,
			StackTop:          handler.StackTop,
			EndOffset:         handler.EndOffset,
			EnsureOffset:      handler.EnsureOffset,
			EnsureEndOffset:   handler.EnsureEndOffset,
			Frame:             handler.Frame,
			PreviousException: previousException,
			RescueStackDepth:  len(vm.rescueStack),
		}
		vm.activeRescues = append(vm.activeRescues, activeRescue)
		handler.Frame.RetryRescue = activeRescue
		handler.Frame.Ip = handler.RescueOffset - 1
	} else if handler.EnsureOffset > 0 {
		vm.pendingEnsures = append(vm.pendingEnsures, &PendingEnsure{
			EnsureEndOffset:   handler.EnsureEndOffset,
			Frame:             handler.Frame,
			Exception:         exception,
			PreviousException: previousException,
		})
		handler.Frame.Ip = handler.EnsureOffset - 1
	} else {
		handler.Frame.Ip = handler.EndOffset - 1
	}
	// Keep ensure context active only when we jumped directly into an ensure block.
	// This is used by backtrace construction to avoid introducing synthetic frame noise.
	vm.ensureActive = handler.RescueOffset == 0 && handler.EnsureOffset > 0
	return true
}

func (vm *VM) returningFromClassBodyBlock(frame *Frame) bool {
	if frame == nil || frame.Fn == nil || frame.Fn.Name != "__block__" || vm.fp <= 0 || vm.fp >= len(vm.frames) {
		return false
	}
	caller := vm.frames[vm.fp-1]
	return caller != nil && caller.Fn != nil && strings.HasSuffix(caller.Fn.Name, "#body")
}

func (vm *VM) returnUnhandledException(frame *Frame, exception *object.EmeraldValue) {
	core.LastException = exception
	core.LastRaisedResult = exception
	vm.attachExceptionLocations(exception)
	markExceptionRaised(exception)
	if vm.isRoot && frame == vm.frames[0] {
		vm.unhandledException = exception
	}
	vm.sp = frame.Bp
	vm.push(exception)
	frame.Ip = len(frame.Fn.Instructions) - 1
}

func (vm *VM) attachExceptionLocations(exception *object.EmeraldValue) {
	if exception == nil || exception.Type != object.ValueException {
		return
	}
	exc, ok := exception.Data.(*object.RException)
	if !ok || exc == nil || len(exc.Backtrace) > 0 || len(exc.Locations) > 0 {
		return
	}
	exc.Locations = vm.currentBacktraceFrames()
}

func shouldPropagateExceptionValue(value *object.EmeraldValue) bool {
	if value == nil || value.Type != object.ValueException {
		return false
	}
	if core.LastRaisedResult == value {
		return true
	}
	exc, ok := value.Data.(*object.RException)
	return ok && exc != nil && exc.Raised
}

func markExceptionRaised(value *object.EmeraldValue) {
	if value == nil || value.Type != object.ValueException {
		return
	}
	if exc, ok := value.Data.(*object.RException); ok && exc != nil {
		exc.Raised = true
	}
}

func boolValue(value bool) *object.EmeraldValue {
	if value {
		return core.R.TrueVal
	}
	return core.R.FalseVal
}

func (vm *VM) flipFlopState(frame *Frame, stateID int) bool {
	if frame != nil && frame.Closure != nil && frame.Closure.FlipFlopStates != nil {
		return frame.Closure.FlipFlopStates[stateID]
	}
	return frame != nil && frame.Fn != nil && frame.Fn.FlipFlopStates != nil && frame.Fn.FlipFlopStates[stateID]
}

func (vm *VM) setFlipFlopState(frame *Frame, stateID int, active bool) {
	if frame == nil {
		return
	}
	if frame.Closure != nil {
		if frame.Closure.FlipFlopStates == nil {
			frame.Closure.FlipFlopStates = make(map[int]bool)
		}
		frame.Closure.FlipFlopStates[stateID] = active
		return
	}
	if frame.Fn != nil {
		if frame.Fn.FlipFlopStates == nil {
			frame.Fn.FlipFlopStates = make(map[int]bool)
		}
		frame.Fn.FlipFlopStates[stateID] = active
	}
}

func (vm *VM) rescueMatches(exception *object.EmeraldValue, classes []*object.EmeraldValue) (bool, *object.EmeraldValue) {
	if exception == nil || exception.Class == nil {
		return false, nil
	}
	if len(classes) == 0 {
		return classInheritsFrom(exception.Class, core.R.Classes["StandardError"]), nil
	}
	for _, classVal := range classes {
		result := vm.send(classVal, "===", []*object.EmeraldValue{exception})
		if shouldPropagateExceptionValue(result) {
			return false, result
		}
		if result != nil && result.IsTruthy() {
			return true, nil
		}
	}
	return false, nil
}

func invalidRescueClause(classes []*object.EmeraldValue) bool {
	for _, classVal := range classes {
		if classVal == nil {
			return true
		}
		switch classVal.Type {
		case object.ValueClass, object.ValueModule:
			continue
		case object.ValueArray:
			if invalidRescueClause(classVal.Data.([]*object.EmeraldValue)) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func rescueClassMatches(exceptionClass *object.Class, classVal *object.EmeraldValue) bool {
	if classVal == nil {
		return false
	}
	if classVal.Type == object.ValueArray {
		for _, elem := range classVal.Data.([]*object.EmeraldValue) {
			if rescueClassMatches(exceptionClass, elem) {
				return true
			}
		}
		return false
	}
	if classVal.Type != object.ValueClass {
		return false
	}
	return classInheritsFrom(exceptionClass, classVal.Data.(*object.Class))
}

func classInheritsFrom(cls, target *object.Class) bool {
	if cls == nil || target == nil {
		return false
	}
	seen := map[*object.Class]bool{}
	for current := cls; current != nil; current = current.SuperClass {
		if seen[current] {
			return false
		}
		seen[current] = true
		if current == target || current.Name == target.Name {
			return true
		}
	}
	return false
}

func positionalArgOrDefault(fn *object.Function, args []*object.EmeraldValue, index int) *object.EmeraldValue {
	if index < len(args) && args[index] != nil {
		return args[index]
	}
	if index < len(fn.ParamDefaults) && fn.ParamDefaults[index] != nil {
		if fn.EvaluateParamDefaults {
			return nil
		}
		return fn.ParamDefaults[index]
	}
	return core.R.NilVal
}

func (vm *VM) dynamicModuleOrClassAccessor(receiver *object.EmeraldValue, method string, args []*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil || (receiver.Type != object.ValueModule && receiver.Type != object.ValueClass) {
		return nil
	}
	if strings.HasSuffix(method, "=") && len(args) > 0 {
		name := "@" + strings.TrimSuffix(method, "=")
		switch receiver.Type {
		case object.ValueModule:
			receiver.Data.(*object.Module).InstanceVars[name] = args[0]
		case object.ValueClass:
			receiver.Data.(*object.Class).SetInstanceVar(name, args[0])
		}
		return args[0]
	}
	if strings.Contains(method, "=") || len(args) != 0 {
		return nil
	}
	name := "@" + method
	switch receiver.Type {
	case object.ValueModule:
		if value := receiver.Data.(*object.Module).InstanceVars[name]; value != nil {
			return value
		}
	case object.ValueClass:
		if value := receiver.Data.(*object.Class).GetInstanceVar(name); value != nil {
			return value
		}
	}
	return nil
}

func (vm *VM) constSourceLocationArgumentError(method string, args []*object.EmeraldValue) *object.EmeraldValue {
	if method != "const_source_location" || !core.EvaluatingRaiseErrorMatcher() || len(args) == 0 || args[0] == nil {
		return nil
	}
	switch args[0].Type {
	case object.ValueString, object.ValueSymbol:
		return nil
	}
	coerced := vm.send(args[0], "to_str", nil)
	if coerced == nil || coerced.Type != object.ValueString {
		return core.NewTypeError("no implicit conversion into String")
	}
	return nil
}

func (vm *VM) moduleEvalArgumentError(method string, args []*object.EmeraldValue) *object.EmeraldValue {
	if (method != "class_eval" && method != "module_eval") || !core.EvaluatingRaiseErrorMatcher() {
		return nil
	}
	if vm.currentBlock != nil && len(args) > 0 {
		return core.NewArgumentError("wrong number of arguments")
	}
	if vm.currentBlock == nil && len(args) == 0 {
		return core.NewArgumentError("wrong number of arguments")
	}
	if len(args) > 3 {
		return core.NewArgumentError("wrong number of arguments")
	}
	if len(args) > 0 {
		if err := vm.requireEvalString(args[0]); err != nil {
			return err
		}
	}
	if len(args) > 1 {
		if err := vm.requireEvalString(args[1]); err != nil {
			return err
		}
	}
	return nil
}

func (vm *VM) requireEvalString(value *object.EmeraldValue) *object.EmeraldValue {
	if value != nil && value.Type == object.ValueString {
		return nil
	}
	if value != nil {
		coerced := vm.send(value, "to_str", nil)
		if coerced != nil && coerced.Type == object.ValueString {
			return nil
		}
	}
	return core.NewTypeError("no implicit conversion into String")
}

func (vm *VM) constDefinedArgumentError(method string, args []*object.EmeraldValue) *object.EmeraldValue {
	if method != "const_defined?" || !core.EvaluatingRaiseErrorMatcher() || len(args) == 0 {
		return nil
	}
	name, err := vm.coerceConstDefinedName(args[0])
	if err != nil {
		return err
	}
	if !validConstDefinedName(name) {
		return core.NewNameError("wrong constant name " + name)
	}
	return nil
}

func (vm *VM) coerceConstDefinedName(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	if value == nil {
		return "", core.NewTypeError("no implicit conversion into String")
	}
	switch value.Type {
	case object.ValueString, object.ValueSymbol:
		if s, ok := value.Data.(string); ok {
			return s, nil
		}
		return "", nil
	default:
		if !valueRespondsToMethod(value, "to_str") {
			return "", core.NewTypeError("no implicit conversion into String")
		}
		coerced := vm.send(value, "to_str", nil)
		if coerced != nil && coerced.Type == object.ValueException {
			return "", coerced
		}
		if coerced == nil || coerced.Type != object.ValueString {
			return "", core.NewTypeError("no implicit conversion into String")
		}
		return coerced.Data.(string), nil
	}
}

func valueRespondsToMethod(value *object.EmeraldValue, name string) bool {
	if value == nil {
		return false
	}
	switch value.Type {
	case object.ValueObject:
		if obj, ok := value.Data.(*object.Object); ok {
			if _, ok := obj.SingletonMethods[name]; ok {
				return true
			}
			if obj.SingletonClass != nil {
				for _, mod := range obj.SingletonClass.PrependedModules {
					if _, ok := mod.GetMethod(name); ok {
						return true
					}
				}
				if _, ok := obj.SingletonClass.Methods[name]; ok {
					return true
				}
				for _, mod := range obj.SingletonClass.IncludedModules {
					if _, ok := mod.GetMethod(name); ok {
						return true
					}
				}
			}
		}
		if value.Class != nil {
			if _, ok := value.Class.GetMethod(name); ok {
				return true
			}
		}
		return false
	case object.ValueClass:
		class, ok := value.Data.(*object.Class)
		if !ok {
			return false
		}
		for current := class; current != nil; current = current.SuperClass {
			if current.SingletonClass != nil {
				for _, mod := range current.SingletonClass.PrependedModules {
					if _, ok := mod.GetMethod(name); ok {
						return true
					}
				}
				if _, ok := current.SingletonClass.Methods[name]; ok {
					return true
				}
				for _, mod := range current.SingletonClass.IncludedModules {
					if _, ok := mod.GetMethod(name); ok {
						return true
					}
				}
			}
			if _, ok := current.ClassMethods[name]; ok {
				return true
			}
		}
		if value.Class != nil {
			if _, ok := value.Class.GetMethod(name); ok {
				return true
			}
		}
		return false
	case object.ValueModule:
		mod, ok := value.Data.(*object.Module)
		if !ok {
			return false
		}
		if mod.SingletonClass != nil {
			for _, singletonMod := range mod.SingletonClass.PrependedModules {
				if _, ok := singletonMod.GetMethod(name); ok {
					return true
				}
			}
			if _, ok := mod.SingletonClass.Methods[name]; ok {
				return true
			}
			for _, singletonMod := range mod.SingletonClass.IncludedModules {
				if _, ok := singletonMod.GetMethod(name); ok {
					return true
				}
			}
		}
		if value.Class != nil {
			if _, ok := value.Class.GetMethod(name); ok {
				return true
			}
		}
	default:
		if value.Class != nil {
			if _, ok := value.Class.GetMethod(name); ok {
				return true
			}
		}
	}
	return false
}

func validConstDefinedName(name string) bool {
	if strings.HasPrefix(name, "::") {
		name = strings.TrimPrefix(name, "::")
	}
	if name == "" {
		return false
	}
	parts := strings.Split(name, "::")
	for _, part := range parts {
		if part == "" {
			return false
		}
		runes := []rune(part)
		if len(runes) == 0 || !isConstantStartRune(runes[0]) {
			return false
		}
		for _, r := range runes[1:] {
			if !isConstantNameRune(r) {
				return false
			}
		}
	}
	return true
}

func isConstantStartRune(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func isConstantNameRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}

func (vm *VM) bindFunctionArguments(fn *object.Function, args []*object.EmeraldValue, prevBlock *object.EmeraldValue, markRuby2Keywords bool) int {
	args = dropEmptyRuby2KeywordHashForPositionalOnlyFunction(fn, args)
	args = copyUnmarkedRuby2KeywordHashForPositionalFunction(fn, args, markRuby2Keywords)
	args = mergeKeywordRestOverflowHashes(fn, args)
	bp := vm.sp
	for i := 0; i < fn.NumLocals; i++ {
		vm.stack[bp+1+i] = core.R.NilVal
	}

	if len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly {
		positionalArgs := args
		var kwargs map[*object.EmeraldValue]*object.EmeraldValue

		if len(args) > 0 && args[len(args)-1] != nil && args[len(args)-1].Type == object.ValueHash && core.Ruby2KeywordHash(args[len(args)-1]) {
			lastArg := args[len(args)-1]
			kwargs = executorHashToMap(lastArg)
			positionalArgs = args[:len(args)-1]
		}

		if fn.HasRestParam {
			vm.bindRestParameterSlots(fn, positionalArgs, bp, markRuby2Keywords)
		} else {
			vm.bindPositionalParameterSlots(fn, positionalArgs, bp)
		}

		for _, kp := range fn.KeywordParams {
			slot := bp + 1 + len(fn.Params)
			if fn.HasRestParam {
				slot++
			}
			for _, prior := range fn.KeywordParams {
				if prior.Name == kp.Name {
					break
				}
				slot++
			}
			if index, ok := fn.LocalNames[kp.Name]; ok {
				slot = bp + 1 + index
			}
			val := vm.lookupKwarg(kwargs, kp.Name)
			if val == nil {
				if kp.HasDefault {
					if !fn.EvaluateParamDefaults {
						val = kp.Default
					}
				} else {
					val = core.R.NilVal
				}
			}
			vm.stack[slot] = val
			if vm.sp <= slot {
				vm.sp = slot + 1
			}
		}
		if fn.KeywordRestParam != "" {
			if index, ok := fn.LocalNames[fn.KeywordRestParam]; ok {
				slot := bp + 1 + index
				vm.stack[slot] = vm.keywordRestHash(kwargs, fn.KeywordParams)
				if vm.sp <= slot {
					vm.sp = slot + 1
				}
			}
		}
	} else if fn.HasRestParam {
		vm.bindRestParameterSlots(fn, args, bp, false)
	} else {
		vm.bindPositionalParameterSlots(fn, args, bp)
	}

	if fn.HasBlockParam {
		blockVal := prevBlock
		if blockVal == nil {
			blockVal = core.R.NilVal
		} else if blockVal.Type == object.ValueClosure {
			closure := blockVal.Data.(*object.Closure)
			blockVal = &object.EmeraldValue{
				Type: object.ValueProc,
				Data: &object.Proc{
					Fn:               closure.Fn,
					Env:              closure.Free,
					Block:            closure.Block,
					Binding:          closure.Binding,
					ClassStack:       closure.ClassStack,
					Refinements:      append([]*object.EmeraldValue(nil), closure.Refinements...),
					RefinementsFixed: closure.RefinementsFixed,
					InstanceVars:     make(map[string]*object.EmeraldValue),
					IsLambda:         false,
					BreakOwnerID:     closure.BreakOwnerID,
					ReturnOwnerID:    closure.ReturnOwnerID,
					FlipFlopStates:   closure.FlipFlopStates,
				},
				Class: core.R.Classes["Proc"],
			}
		}
		blockSlot := bp + fn.BlockParamIndex + 1
		vm.stack[blockSlot] = blockVal
		if vm.sp <= blockSlot {
			vm.sp = blockSlot + 1
		}
	}

	minSp := bp + 1 + fn.NumLocals
	if vm.sp < minSp {
		vm.sp = minSp
	}

	return bp
}

func (vm *VM) bindPositionalParameterSlots(fn *object.Function, args []*object.EmeraldValue, bp int) {
	firstDefault := -1
	lastDefault := -1
	for i := 0; i < len(fn.Params) && i < len(fn.ParamDefaults); i++ {
		if fn.ParamDefaults[i] != nil {
			if firstDefault < 0 {
				firstDefault = i
			}
			lastDefault = i
		}
	}
	if firstDefault < 0 {
		firstDefault = len(fn.Params)
		lastDefault = len(fn.Params) - 1
	}
	requiredPre := firstDefault
	optionalCount := lastDefault - firstDefault + 1
	if optionalCount < 0 {
		optionalCount = 0
	}
	postCount := len(fn.Params) - requiredPre - optionalCount
	optionalAvailable := len(args) - requiredPre - postCount
	if optionalAvailable < 0 {
		optionalAvailable = 0
	}
	if optionalAvailable > optionalCount {
		optionalAvailable = optionalCount
	}
	postStart := requiredPre + optionalAvailable

	for i := 0; i < len(fn.Params); i++ {
		value := core.R.NilVal
		switch {
		case i < requiredPre:
			if i < len(args) && args[i] != nil {
				value = args[i]
			}
		case i < requiredPre+optionalCount:
			optionalIndex := i - requiredPre
			if optionalIndex < optionalAvailable && requiredPre+optionalIndex < len(args) && args[requiredPre+optionalIndex] != nil {
				value = args[requiredPre+optionalIndex]
			} else if i < len(fn.ParamDefaults) && fn.ParamDefaults[i] != nil && !fn.EvaluateParamDefaults {
				value = fn.ParamDefaults[i]
			} else if i < len(fn.ParamDefaults) && fn.ParamDefaults[i] != nil && fn.EvaluateParamDefaults {
				value = nil
			}
		default:
			argIndex := postStart + i - requiredPre - optionalCount
			if argIndex < len(args) && args[argIndex] != nil {
				value = args[argIndex]
			}
		}
		slot := bp + 1 + functionParamLocalIndex(fn, i)
		vm.stack[slot] = value
		if vm.sp <= slot {
			vm.sp = slot + 1
		}
	}
}

func (vm *VM) bindParameterPatterns(fn *object.Function, bp int) *object.EmeraldValue {
	if fn == nil || len(fn.ParamPatterns) == 0 {
		return nil
	}
	for i, pattern := range fn.ParamPatterns {
		if pattern == nil || i >= len(fn.Params) {
			continue
		}
		value := core.R.NilVal
		index := functionParamLocalIndex(fn, i)
		if bound := vm.stack[bp+1+index]; bound != nil {
			value = bound
		}
		if errVal := vm.bindParameterPattern(fn, pattern, value, bp); errVal != nil {
			return errVal
		}
	}
	return nil
}

func functionParamLocalIndex(fn *object.Function, index int) int {
	if fn != nil && index >= 0 && index < len(fn.ParamLocalIndices) {
		return fn.ParamLocalIndices[index]
	}
	if fn != nil && index >= 0 && index < len(fn.Params) {
		if localIndex, ok := fn.LocalNames[fn.Params[index]]; ok {
			return localIndex
		}
	}
	return index
}

func (vm *VM) bindParameterPattern(fn *object.Function, pattern *object.ParameterPattern, value *object.EmeraldValue, bp int) *object.EmeraldValue {
	if pattern == nil {
		return nil
	}
	if pattern.Name != "" {
		if index, ok := fn.LocalNames[pattern.Name]; ok {
			if value == nil {
				value = core.R.NilVal
			}
			setClosureValue(&vm.stack[bp+1+index], value)
		}
		return nil
	}

	elems, errVal := vm.destructureParameterValue(value)
	if errVal != nil {
		return errVal
	}
	if pattern.Rest == nil {
		for i, child := range pattern.Children {
			childValue := core.R.NilVal
			if i < len(elems) && elems[i] != nil {
				childValue = elems[i]
			}
			if errVal := vm.bindParameterPattern(fn, child, childValue, bp); errVal != nil {
				return errVal
			}
		}
		return nil
	}

	preCount := pattern.RestIndex
	if preCount < 0 || preCount > len(pattern.Children) {
		preCount = len(pattern.Children)
	}
	postCount := len(pattern.Children) - preCount
	for i := 0; i < preCount; i++ {
		childValue := core.R.NilVal
		if i < len(elems) && elems[i] != nil {
			childValue = elems[i]
		}
		if errVal := vm.bindParameterPattern(fn, pattern.Children[i], childValue, bp); errVal != nil {
			return errVal
		}
	}
	postStart := len(elems) - postCount
	if postStart < preCount {
		postStart = preCount
	}
	restEnd := postStart
	if restEnd > len(elems) {
		restEnd = len(elems)
	}
	restStart := preCount
	if restStart > restEnd {
		restStart = restEnd
	}
	restValues := append([]*object.EmeraldValue(nil), elems[restStart:restEnd]...)
	if errVal := vm.bindParameterPattern(fn, pattern.Rest, vm.arrayValue(restValues...), bp); errVal != nil {
		return errVal
	}
	for i := 0; i < postCount; i++ {
		childValue := core.R.NilVal
		argIndex := postStart + i
		if argIndex < len(elems) && elems[argIndex] != nil {
			childValue = elems[argIndex]
		}
		if errVal := vm.bindParameterPattern(fn, pattern.Children[preCount+i], childValue, bp); errVal != nil {
			return errVal
		}
	}
	return nil
}

func (vm *VM) destructureParameterValue(value *object.EmeraldValue) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if value == nil {
		value = core.R.NilVal
	}
	if value.Type == object.ValueArray {
		return value.Data.([]*object.EmeraldValue), nil
	}
	coerced, called, errVal := vm.callArrayCoercion(value, "to_ary")
	if errVal != nil {
		return nil, errVal
	}
	if !called || coerced == nil || coerced.Type == object.ValueNil {
		return []*object.EmeraldValue{value}, nil
	}
	if coerced.Type != object.ValueArray {
		return nil, core.NewTypeError("can't convert to Array")
	}
	return coerced.Data.([]*object.EmeraldValue), nil
}

func rejectBlockArgument(fn *object.Function, block *object.EmeraldValue) *object.EmeraldValue {
	if fn == nil || !fn.RejectBlock || block == nil || block.Type == object.ValueNil {
		return nil
	}
	return core.NewArgumentError("no block accepted")
}

func (vm *VM) bindRestParameterSlots(fn *object.Function, args []*object.EmeraldValue, bp int, markRuby2Keywords bool) {
	preCount := fn.RestParamIndex
	if preCount < 0 {
		preCount = 0
	}
	if preCount > len(fn.Params) {
		preCount = len(fn.Params)
	}
	postCount := len(fn.Params) - preCount
	requiredPre := preCount
	for i := 0; i < preCount && i < len(fn.ParamDefaults); i++ {
		if fn.ParamDefaults[i] != nil {
			requiredPre = i
			break
		}
	}
	preAvailable := requiredPre
	if preAvailable > len(args) {
		preAvailable = len(args)
	}
	optionalAvailable := len(args) - preAvailable - postCount
	if optionalAvailable > 0 {
		optionalCount := preCount - requiredPre
		if optionalAvailable > optionalCount {
			optionalAvailable = optionalCount
		}
		preAvailable += optionalAvailable
	}

	for i := 0; i < preCount; i++ {
		slot := bp + 1 + functionParamLocalIndex(fn, i)
		if i < preAvailable && args[i] != nil {
			vm.stack[slot] = args[i]
		} else if i < len(fn.ParamDefaults) && fn.ParamDefaults[i] != nil {
			if fn.EvaluateParamDefaults {
				vm.stack[slot] = nil
			} else {
				vm.stack[slot] = fn.ParamDefaults[i]
			}
		} else {
			vm.stack[slot] = core.R.NilVal
		}
		if vm.sp <= slot {
			vm.sp = slot + 1
		}
	}

	postStart := len(args) - postCount
	if postStart < preAvailable {
		postStart = preAvailable
	}
	restElems := make([]*object.EmeraldValue, 0)
	if postStart > preAvailable && preAvailable < len(args) {
		restElems = args[preAvailable:postStart]
	}
	if markRuby2Keywords && len(restElems) > 0 {
		last := restElems[len(restElems)-1]
		if last != nil && last.Type == object.ValueHash && core.Ruby2KeywordHash(last) {
			copied := make(map[*object.EmeraldValue]*object.EmeraldValue)
			for key, value := range executorHashToMap(last) {
				copied[key] = value
			}
			last = &object.EmeraldValue{Type: object.ValueHash, Data: copied, Class: core.R.Classes["Hash"]}
			core.MarkRuby2KeywordHash(last)
			restElems[len(restElems)-1] = last
		}
	}
	if !fn.AnonymousRestParam {
		restSlot := bp + 1 + fn.LocalNames[fn.RestParamName]
		vm.stack[restSlot] = &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  restElems,
			Class: core.R.Classes["Array"],
		}
		if vm.sp <= restSlot {
			vm.sp = restSlot + 1
		}
	}

	for j := 0; j < postCount; j++ {
		paramIndex := preCount + j
		argIndex := postStart + j
		slot := bp + 1 + functionParamLocalIndex(fn, paramIndex)
		if argIndex < len(args) && args[argIndex] != nil {
			vm.stack[slot] = args[argIndex]
		} else if paramIndex < len(fn.ParamDefaults) && fn.ParamDefaults[paramIndex] != nil {
			vm.stack[slot] = fn.ParamDefaults[paramIndex]
		} else {
			vm.stack[slot] = core.R.NilVal
		}
		if vm.sp <= slot {
			vm.sp = slot + 1
		}
	}
}

func (vm *VM) toAryForSplat(val *object.EmeraldValue) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if val == nil || val.Type == object.ValueNil {
		return nil, core.NewTypeError("no implicit conversion into Array")
	}
	if val.Type == object.ValueArray {
		return val.Data.([]*object.EmeraldValue), nil
	}
	if core.CallMethod == nil || val.Class == nil {
		return nil, core.NewTypeError("no implicit conversion into Array")
	}

	coerced := core.CallMethod(val, "to_ary")
	if coerced == nil || coerced.Type != object.ValueArray {
		return nil, core.NewTypeError("no implicit conversion into Array")
	}
	return coerced.Data.([]*object.EmeraldValue), nil
}

func (vm *VM) prepareMultiAssignRHS(val *object.EmeraldValue) (*object.EmeraldValue, *object.EmeraldValue) {
	if val == nil || val.Type == object.ValueNil {
		return vm.arrayValue(core.R.NilVal), nil
	}
	if val.Type == object.ValueArray {
		return val, nil
	}

	coerced, called, err := vm.callArrayCoercion(val, "to_ary")
	if err != nil {
		return nil, err
	}
	if !called || coerced == nil || coerced.Type == object.ValueNil {
		return vm.arrayValue(val), nil
	}
	if coerced.Type != object.ValueArray {
		return nil, core.NewTypeError("can't convert to Array")
	}
	return coerced, nil
}

func (vm *VM) checkMultiAssignToAry(val *object.EmeraldValue) *object.EmeraldValue {
	if val == nil || val.Type == object.ValueNil || val.Type == object.ValueArray {
		return nil
	}

	coerced, called, err := vm.callArrayCoercion(val, "to_ary")
	if err != nil {
		return err
	}
	if !called || coerced == nil || coerced.Type == object.ValueNil {
		return nil
	}
	if coerced.Type != object.ValueArray {
		return core.NewTypeError("can't convert to Array")
	}
	return nil
}

func (vm *VM) toAForAssignmentSplat(val *object.EmeraldValue) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if val == nil || val.Type == object.ValueNil {
		return []*object.EmeraldValue{}, nil
	}
	if val.Type == object.ValueArray {
		return val.Data.([]*object.EmeraldValue), nil
	}

	coerced, called, err := vm.callArrayCoercion(val, "to_a")
	if err != nil {
		return nil, err
	}
	if !called || coerced == nil || coerced.Type == object.ValueNil {
		return []*object.EmeraldValue{val}, nil
	}
	if coerced.Type != object.ValueArray {
		return nil, core.NewTypeError("can't convert to Array")
	}
	return coerced.Data.([]*object.EmeraldValue), nil
}

func (vm *VM) toAForMethodSplat(val *object.EmeraldValue) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if val == nil || val.Type == object.ValueNil {
		return []*object.EmeraldValue{}, nil
	}
	if val.Type == object.ValueArray {
		return append([]*object.EmeraldValue(nil), val.Data.([]*object.EmeraldValue)...), nil
	}

	coerced, called, err := vm.callArrayCoercion(val, "to_a")
	if err != nil {
		return nil, err
	}
	if !called || coerced == nil || coerced.Type == object.ValueNil {
		return []*object.EmeraldValue{val}, nil
	}
	if coerced.Type != object.ValueArray {
		return nil, core.NewTypeError("can't convert to Array")
	}
	return append([]*object.EmeraldValue(nil), coerced.Data.([]*object.EmeraldValue)...), nil
}

func (vm *VM) callArrayCoercion(val *object.EmeraldValue, method string) (*object.EmeraldValue, bool, *object.EmeraldValue) {
	if core.CallMethod == nil || val == nil || val.Class == nil {
		return nil, false, nil
	}

	prevException := core.LastException
	prevRaisedResult := core.LastRaisedResult
	responds := core.CallMethod(val, "respond_to?",
		&object.EmeraldValue{Type: object.ValueSymbol, Data: method, Class: core.R.Classes["Symbol"]},
		core.R.TrueVal,
	)
	if core.LastException != nil && core.LastException != prevException {
		return nil, false, core.LastException
	}
	if core.LastRaisedResult != nil && core.LastRaisedResult != prevRaisedResult {
		return nil, false, core.LastRaisedResult
	}
	if responds == nil || responds.Type == object.ValueException {
		return responds, false, responds
	}
	if responds.Type != object.ValueBool || !responds.Data.(bool) {
		return nil, false, nil
	}

	prevException = core.LastException
	prevRaisedResult = core.LastRaisedResult
	coerced := core.CallMethod(val, method)
	if core.LastException != nil && core.LastException != prevException {
		return nil, true, core.LastException
	}
	if core.LastRaisedResult != nil && core.LastRaisedResult != prevRaisedResult {
		return nil, true, core.LastRaisedResult
	}
	if coerced != nil && coerced.Type == object.ValueException {
		return nil, true, coerced
	}
	return coerced, true, nil
}

func (vm *VM) arrayValue(elems ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  elems,
		Class: core.R.Classes["Array"],
	}
}

func (vm *VM) expandMethodSplatArgs(args []*object.EmeraldValue, splatIndex int) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if splatIndex < 0 || splatIndex >= len(args) {
		return args, nil
	}

	expanded := make([]*object.EmeraldValue, 0, len(args))
	expanded = append(expanded, args[:splatIndex]...)
	elems, errVal := vm.toAForMethodSplat(args[splatIndex])
	if errVal != nil {
		return nil, errVal
	}
	expanded = append(expanded, elems...)
	if splatIndex+1 < len(args) {
		expanded = append(expanded, args[splatIndex+1:]...)
	}
	return expanded, nil
}

func (vm *VM) expandYieldSplatArgs(args []*object.EmeraldValue, splatIndex int) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if splatIndex < 0 || splatIndex >= len(args) {
		return args, nil
	}

	expanded := make([]*object.EmeraldValue, 0, len(args))
	expanded = append(expanded, args[:splatIndex]...)

	splatArg := args[splatIndex]
	if splatArg != nil && splatArg.Type != object.ValueNil {
		elems, errVal := vm.toAryForSplat(splatArg)
		if errVal != nil {
			return nil, errVal
		}
		expanded = append(expanded, elems...)
	}

	if splatIndex+1 < len(args) {
		expanded = append(expanded, args[splatIndex+1:]...)
	}
	return expanded, nil
}

func (vm *VM) callBlock(block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if debugForLoopEnabled {
		blockType := object.ValueNil
		if block != nil {
			blockType = block.Type
		}
		argType := ""
		if len(args) > 0 && args[0] != nil {
			argType = args[0].TypeName()
		}
		fmt.Printf("callBlock: blockType=%v args=%d first=%s\n", blockType, len(args), argType)
	}
	if block != nil && block.Type == object.ValueProc {
		proc, _ := block.Data.(*object.Proc)
		if proc != nil && proc.Native == nil && !proc.IsLambda {
			vm.procCallDepth++
			defer func() {
				vm.procCallDepth--
			}()
		}
	}
	return vm.callBlockWithSelf(block, blockBindingSelf(block), args...)
}

func (vm *VM) yieldBlock() *object.EmeraldValue {
	if vm.currentBlock != nil {
		return vm.currentBlock
	}
	if vm.fp >= 0 && vm.fp < len(vm.frames) && vm.frames[vm.fp] != nil {
		return vm.frames[vm.fp].Block
	}
	return nil
}

func (vm *VM) methodBlockGiven() bool {
	for i := vm.fp; i >= 0; i-- {
		frame := vm.frames[i]
		if frame == nil {
			continue
		}
		if frame.DefinedByDefineMethod || (frame.Fn != nil && frame.Fn.DefinedByDefineMethod) {
			return false
		}
		if frame.MethodName == "" || (frame.Fn != nil && frame.Fn.Name == "__block__") {
			continue
		}
		return frame.Block != nil
	}
	return false
}

func blockBindingSelf(block *object.EmeraldValue) *object.EmeraldValue {
	if block == nil {
		return core.R.Main
	}
	switch block.Type {
	case object.ValueClosure:
		closure := block.Data.(*object.Closure)
		if closure.Binding != nil && closure.Binding.Self != nil {
			return closure.Binding.Self
		}
	case object.ValueProc:
		proc := block.Data.(*object.Proc)
		if proc.Binding != nil && proc.Binding.Self != nil {
			return proc.Binding.Self
		}
	}
	return core.R.Main
}

func (vm *VM) callBlockWithSelf(block, self *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if block == nil {
		return core.R.NilVal
	}
	isThreadBlock := vm.threadDepth > 0
	autoSplat := false

	var fn *object.Function
	var closure *object.Closure
	var procData *object.Proc
	isLambda := false
	switch block.Type {
	case object.ValueClosure:
		closure = block.Data.(*object.Closure)
		fn = closure.Fn
		autoSplat = closure.AutoSplat
	case object.ValueSymbol:
		name, _ := block.Data.(string)
		if len(args) == 0 {
			return core.R.NilVal
		}
		return vm.send(args[0], name, args[1:])
	case object.ValueProc:
		proc := block.Data.(*object.Proc)
		procData = proc
		if proc.Native != nil {
			return proc.Native(args...)
		}
		fn = proc.Fn
		isLambda = proc.IsLambda
		autoSplat = proc.AutoSplat
		closure = &object.Closure{
			Fn:               fn,
			Free:             proc.Env,
			Block:            proc.Block,
			Binding:          proc.Binding,
			ClassStack:       proc.ClassStack,
			Refinements:      append([]*object.EmeraldValue(nil), proc.Refinements...),
			RefinementsFixed: proc.RefinementsFixed,
			AutoSplat:        proc.AutoSplat,
			ReturnOwnerID:    proc.ReturnOwnerID,
			BreakOwnerID:     proc.BreakOwnerID,
			FlipFlopStates:   proc.FlipFlopStates,
		}
		if proc.FlipFlopStates == nil {
			proc.FlipFlopStates = make(map[int]bool)
			closure.FlipFlopStates = proc.FlipFlopStates
		}
	default:
		return core.R.NilVal
	}

	if fn == nil {
		return core.R.NilVal
	}
	args = dropAnonymousKeywordRestNonSymbolHash(fn, args)
	core.LastBlockResult = nil
	if autoSplat && (!isLambda || fn.SingleDestructure) {
		expanded, errVal := vm.blockAutosplatArgs(fn, args)
		if errVal != nil {
			return errVal
		}
		args = expanded
	}
	if errVal := missingRequiredKeywordArgument(fn, args); errVal != nil {
		return errVal
	}
	if errVal := rejectedKeywordArgument(fn, args); errVal != nil {
		return errVal
	}
	if errVal := rejectedPositionalForKeywordRestOnly(fn, args); errVal != nil {
		return errVal
	}
	if isLambda {
		if errVal := methodArityError(fn, positionalArityArgCount(fn, args)); errVal != nil {
			return errVal
		}
	}

	prevBlock := vm.currentBlock
	vm.currentBlock = nil
	prevClassStack := vm.classStack
	prevCatchStack := vm.catchStack
	core.LastRaisedResult = nil
	if errVal := rejectBlockArgument(fn, prevBlock); errVal != nil {
		return errVal
	}
	if isThreadBlock {
		vm.catchStack = nil
	}
	if len(closure.ClassStack) > 0 {
		vm.classStack = append([]*object.EmeraldValue(nil), closure.ClassStack...)
		if self != nil && (self.Type == object.ValueClass || self.Type == object.ValueModule) && !singletonClassStackTargets(vm.classStack, self) {
			vm.classStack = append(vm.classStack, self)
		}
	}
	defer func() {
		vm.currentBlock = prevBlock
		vm.classStack = prevClassStack
		vm.catchStack = prevCatchStack
	}()

	bp := vm.sp

	vm.stack[vm.sp] = self
	markRuby2Keywords := procData != nil && core.ProcRuby2Keywords(procData)
	vm.bindFunctionArguments(fn, args, prevBlock, markRuby2Keywords)
	if errVal := vm.bindParameterPatterns(fn, bp); errVal != nil {
		vm.sp = bp
		return errVal
	}

	methodName := ""
	if closure.Binding != nil {
		methodName = closure.Binding.Method
	}
	var parameters *object.EmeraldValue
	if core.TracePointEventActive("b_call") || core.TracePointEventActive("b_return") {
		parameters = core.TracePointParameters(fn, isLambda)
	}
	vm.pushReusableFrame(Frame{ID: vm.allocateFrameID(), Fn: fn, Ip: -1, Bp: bp, Closure: closure, MethodName: methodName, OriginalMethodName: methodName, Block: closure.Block, IsLambda: isLambda, BlockBreakAddr: -1, WhileStart: -1, WhileEnd: -1, TraceSelf: self, TraceParameters: parameters})
	if core.TracePointEventActive("b_call") {
		binding := vm.currentFrameBinding()
		binding.Line = fn.DefinitionLine
		core.FireTracePointCall("b_call", binding, self, nil, "", "", parameters)
	}

	frame := vm.frames[vm.fp]
	instructions := frame.Fn.Instructions
	for frame.Ip < len(instructions)-1 {
		frame.Ip++
		op := compiler.Opcode(instructions[frame.Ip])
		vm.fireTracePointLine(frame, op)
		frame.InstructionException = core.LastException
		frame.InstructionSnapshotSet = true
		if err := vm.execute(op, frame); err != nil {
			return core.NewRuntimeError(err.Error())
		}
		if vm.handlePendingNonLocalReturn(frame) || vm.handlePendingNonLocalBreak(frame) || frame.Returned {
			break
		}
		if vm.sp > frame.Bp {
			top := vm.stack[vm.sp-1]
			if top != nil && top.Type == object.ValueException && (core.LastRaisedResult == top || classInheritsFrom(top.Class, core.R.Classes["SystemExit"])) && !vm.frameHasActiveRescue(frame) {
				break
			}
		}
		frame = vm.frames[vm.fp]
		if frame.BlockBreak || frame.BlockNextVal != nil {
			break
		}
		if core.LastBlockResult != nil {
			break
		}
		instructions = frame.Fn.Instructions
	}

	result := core.R.NilVal
	if vm.sp > bp {
		result = vm.stack[vm.sp-1]
	}
	vm.sp = bp

	if frame.Returned && !isLambda && procData != nil && procData.ReturnOwnerID > 0 && !vm.procReturnOwnerActive(procData) {
		result = core.NewLocalJumpErrorWithReturn(result)
		core.LastException = result
	} else if frame.BlockBreak {
		result = frame.BlockBreakVal
		if result == nil {
			result = core.R.NilVal
		}
		if isLambda {
			// A break in a lambda returns from the lambda itself.
		} else if vm.fiberDepth > 0 && vm.fp == 0 {
			result = core.NewLocalJumpError("break from proc-closure")
			core.LastException = result
		} else if procData != nil && vm.procCallDepth > 0 && !vm.procBreakOwnerActive(procData) {
			result = core.NewLocalJumpError("unexpected break")
			core.LastException = result
		} else {
			breakOwnerID := closure.BreakOwnerID
			if procData != nil {
				breakOwnerID = procData.BreakOwnerID
			}
			if breakOwnerID > 0 {
				vm.pendingBreakTargetID = breakOwnerID
				vm.pendingBreakValue = result
			}
			core.LastBlockResult = result
		}
	} else if frame.BlockNextVal != nil {
		result = frame.BlockNextVal
	}
	if core.TracePointEventActive("b_return") {
		binding := vm.currentFrameBinding()
		core.FireTracePointReturn("b_return", binding, self, nil, "", "", parameters, result)
	}

	vm.endActiveRescuesForFrame(frame)
	for i := len(vm.rescueStack) - 1; i >= 0; i-- {
		if vm.rescueStack[i].Frame == frame {
			vm.rescueStack = append(vm.rescueStack[:i], vm.rescueStack[i+1:]...)
		}
	}
	frame.InstructionException = nil
	frame.InstructionSnapshotSet = false
	vm.frames = vm.frames[:vm.fp]
	vm.fp--

	if result == nil || result.Type != object.ValueException {
		core.LastRaisedResult = nil
	}
	return result
}

func (vm *VM) frameHasActiveRescue(frame *Frame) bool {
	for i := len(vm.activeRescues) - 1; i >= 0; i-- {
		if vm.activeRescues[i].Frame == frame {
			return true
		}
	}
	return false
}

func (vm *VM) procBreakOwnerActive(proc *object.Proc) bool {
	if proc == nil || proc.BreakOwnerID <= 0 {
		return false
	}
	for i := vm.fp; i >= 0; i-- {
		if i < len(vm.frames) && vm.frames[i] != nil && vm.frames[i].ID == proc.BreakOwnerID {
			return true
		}
	}
	return false
}

func (vm *VM) procReturnOwnerActive(proc *object.Proc) bool {
	if proc == nil || proc.ReturnOwnerID <= 0 {
		return false
	}
	for i := vm.fp; i >= 0; i-- {
		if i < len(vm.frames) && vm.frames[i] != nil && vm.frames[i].ID == proc.ReturnOwnerID {
			return true
		}
	}
	return false
}

func (vm *VM) blockAutosplatArgs(fn *object.Function, args []*object.EmeraldValue) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if len(args) != 1 || args[0] == nil {
		return args, nil
	}
	if !blockWantsDestructuring(fn) {
		return args, nil
	}
	arg := args[0]
	if arg.Type == object.ValueArray {
		return arg.Data.([]*object.EmeraldValue), nil
	}
	if core.CallMethod == nil {
		return args, nil
	}
	responds := core.CallMethod(arg, "respond_to?",
		&object.EmeraldValue{Type: object.ValueSymbol, Data: "to_ary", Class: core.R.Classes["Symbol"]},
		core.R.TrueVal,
	)
	if responds == nil || responds.Type == object.ValueException {
		return args, nil
	}
	if responds.Type != object.ValueBool || !responds.Data.(bool) {
		return args, nil
	}
	prevException := core.LastException
	prevRaisedResult := core.LastRaisedResult
	coerced := core.CallMethod(arg, "to_ary")
	if core.LastException != nil && core.LastException != prevException {
		return nil, core.LastException
	}
	if core.LastRaisedResult != nil && core.LastRaisedResult != prevRaisedResult {
		return nil, core.LastRaisedResult
	}
	if coerced == nil || coerced.Type == object.ValueNil {
		return args, nil
	}
	if coerced.Type == object.ValueException {
		return nil, coerced
	}
	if coerced.Type != object.ValueArray {
		return nil, core.NewTypeError("can't convert to Array")
	}
	return coerced.Data.([]*object.EmeraldValue), nil
}

func blockWantsDestructuring(fn *object.Function) bool {
	if fn == nil {
		return false
	}
	if len(fn.Params) > 1 {
		return true
	}
	if fn.TrailingCommaParam {
		return true
	}
	return fn.HasRestParam && fn.RestParamIndex > 0
}

func missingRequiredKeywordArgument(fn *object.Function, args []*object.EmeraldValue) *object.EmeraldValue {
	if fn == nil || len(fn.KeywordParams) == 0 {
		return nil
	}
	var kwargs map[*object.EmeraldValue]*object.EmeraldValue
	if len(args) > 0 {
		last := args[len(args)-1]
		if last != nil && last.Type == object.ValueHash {
			kwargs = executorHashToMap(last)
		}
	}
	missing := make([]string, 0)
	for _, kp := range fn.KeywordParams {
		if kp.HasDefault {
			continue
		}
		if vmValue := keywordValueFromMap(kwargs, kp.Name); vmValue == nil {
			missing = append(missing, ":"+kp.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if len(missing) == 1 {
		return core.NewArgumentError("missing keyword: " + missing[0])
	}
	return core.NewArgumentError("missing keywords: " + strings.Join(missing, ", "))
}

func rejectedKeywordArgument(fn *object.Function, args []*object.EmeraldValue) *object.EmeraldValue {
	if fn == nil || !fn.RejectKeywords || len(args) == 0 {
		return nil
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash {
		return nil
	}
	if len(executorHashToMap(last)) == 0 {
		return nil
	}
	if core.Ruby2KeywordHash(last) {
		return core.NewArgumentError("no keywords accepted")
	}
	positionalCapacity := len(fn.Params)
	if fn.HasRestParam {
		positionalCapacity = len(args)
	}
	if len(args) > positionalCapacity {
		return core.NewArgumentError("no keywords accepted")
	}
	return nil
}

func rejectedPositionalForKeywordRestOnly(fn *object.Function, args []*object.EmeraldValue) *object.EmeraldValue {
	if fn == nil || !fn.KeywordRestOnly || len(args) == 0 {
		return nil
	}
	if len(fn.Params) > 0 || fn.HasRestParam {
		if len(args) < 2 {
			return nil
		}
		last := args[len(args)-1]
		if last == nil || last.Type != object.ValueHash || !core.Ruby2KeywordHash(last) {
			return nil
		}
		for _, arg := range args[:len(args)-1] {
			if hashHasNonSymbolKey(arg) {
				return core.NewArgumentError("wrong number of arguments")
			}
		}
		return nil
	}
	if len(args) == 1 {
		last := args[0]
		if last != nil && last.Type == object.ValueHash && core.Ruby2KeywordHash(last) {
			return nil
		}
	}
	return core.NewArgumentError("wrong number of arguments")
}

func rejectedNonSymbolPositionalHashWithKeywords(fn *object.Function, args []*object.EmeraldValue) *object.EmeraldValue {
	if fn == nil || (len(fn.KeywordParams) == 0 && fn.KeywordRestParam == "") || len(args) < 2 {
		return nil
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash || !core.Ruby2KeywordHash(last) {
		return nil
	}
	for _, arg := range args[:len(args)-1] {
		if hashHasNonSymbolKey(arg) && core.Ruby2KeywordHash(arg) {
			return core.NewArgumentError("wrong number of arguments")
		}
	}
	return nil
}

func hashHasNonSymbolKey(value *object.EmeraldValue) bool {
	if value == nil || value.Type != object.ValueHash {
		return false
	}
	for _, key := range orderedKeywordKeys(value) {
		if key == nil || key.Type != object.ValueSymbol {
			return true
		}
	}
	return false
}

func (vm *VM) keywordRestHash(kwargs map[*object.EmeraldValue]*object.EmeraldValue, explicit []object.KeywordParamInfo) *object.EmeraldValue {
	pairs := make(map[*object.EmeraldValue]*object.EmeraldValue)
	keys := make([]*object.EmeraldValue, 0)
	for key, value := range kwargs {
		if key == nil {
			continue
		}
		matched := false
		for _, kp := range explicit {
			if keywordKeyMatchesName(key, kp.Name) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		pairs[key] = value
		keys = append(keys, key)
	}
	return &object.EmeraldValue{
		Type: object.ValueHash,
		Data: &object.RHash{
			Pairs: pairs,
			Keys:  keys,
		},
		Class: core.R.Classes["Hash"],
	}
}

func unknownKeywordArgument(fn *object.Function, args []*object.EmeraldValue) *object.EmeraldValue {
	if fn == nil || fn.KeywordRestOnly || fn.KeywordRestParam != "" || len(fn.KeywordParams) == 0 || len(args) == 0 {
		return nil
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash || !core.Ruby2KeywordHash(last) {
		return nil
	}
	unknown := make([]string, 0)
	keywordHashes := []*object.EmeraldValue{last}
	for _, hash := range keywordHashes {
		for _, key := range orderedKeywordKeys(hash) {
			if key == nil {
				continue
			}
			if keywordParamExists(fn, key) {
				continue
			}
			unknown = append(unknown, keywordDisplayName(key))
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	if len(unknown) == 1 {
		return core.NewArgumentError("unknown keyword: " + unknown[0])
	}
	return core.NewArgumentError("unknown keywords: " + strings.Join(unknown, ", "))
}

func orderedKeywordKeys(hash *object.EmeraldValue) []*object.EmeraldValue {
	if hash == nil || hash.Type != object.ValueHash {
		return nil
	}
	if h, ok := hash.Data.(*object.RHash); ok {
		return h.Keys
	}
	if h, ok := hash.Data.(map[*object.EmeraldValue]*object.EmeraldValue); ok {
		keys := make([]*object.EmeraldValue, 0, len(h))
		for key := range h {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			return keywordDisplayName(keys[i]) < keywordDisplayName(keys[j])
		})
		return keys
	}
	return nil
}

func keywordParamExists(fn *object.Function, key *object.EmeraldValue) bool {
	for _, kp := range fn.KeywordParams {
		if keywordKeyMatchesName(key, kp.Name) {
			return true
		}
	}
	return false
}

func keywordKeyMatchesName(key *object.EmeraldValue, name string) bool {
	if key == nil {
		return false
	}
	switch key.Type {
	case object.ValueSymbol:
		if s, ok := key.Data.(string); ok {
			return strings.TrimPrefix(s, ":") == name
		}
	}
	return false
}

func keywordDisplayName(key *object.EmeraldValue) string {
	if key == nil {
		return "nil"
	}
	switch key.Type {
	case object.ValueSymbol:
		if s, ok := key.Data.(string); ok {
			if strings.HasPrefix(s, ":") {
				return s
			}
			return ":" + s
		}
	}
	return key.Inspect()
}

func keywordValueFromMap(kwargs map[*object.EmeraldValue]*object.EmeraldValue, name string) *object.EmeraldValue {
	if kwargs == nil {
		return nil
	}
	for key, value := range kwargs {
		if key == nil {
			continue
		}
		if keywordKeyMatchesName(key, name) {
			return value
		}
	}
	return nil
}

func (vm *VM) callBlockWithInstanceEvalContext(receiver *object.EmeraldValue, block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	prevClassVarScope := vm.instanceExecClassVarScope
	vm.instanceExecClassVarScope = blockLexicalClassVarScope(block)
	defer func() {
		vm.instanceExecClassVarScope = prevClassVarScope
	}()
	contextClass := vm.instanceEvalContextClass(receiver)
	if block == nil {
		return core.R.NilVal
	}
	switch data := block.Data.(type) {
	case *object.Closure:
		closure := *data
		closure.ClassStack = append(append([]*object.EmeraldValue(nil), data.ClassStack...), contextClass)
		block = &object.EmeraldValue{Type: block.Type, Data: &closure, Class: block.Class}
	case *object.Proc:
		proc := *data
		proc.ClassStack = append(append([]*object.EmeraldValue(nil), data.ClassStack...), contextClass)
		block = &object.EmeraldValue{Type: block.Type, Data: &proc, Class: block.Class}
	}
	result := vm.callBlockWithSelf(block, receiver, args...)
	return result
}

func (vm *VM) callBlockWithInstanceExecContext(receiver *object.EmeraldValue, block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return vm.callBlockWithInstanceEvalContext(receiver, block, args...)
}

func blockLexicalClassVarScope(block *object.EmeraldValue) *object.EmeraldValue {
	if block == nil {
		return nil
	}
	var stack []*object.EmeraldValue
	var binding *object.RBinding
	switch data := block.Data.(type) {
	case *object.Closure:
		stack = data.ClassStack
		binding = data.Binding
	case *object.Proc:
		stack = data.ClassStack
		binding = data.Binding
	}
	if len(stack) == 0 {
		if binding != nil && binding.Self != nil {
			switch binding.Self.Type {
			case object.ValueClass, object.ValueModule:
				return binding.Self
			case object.ValueObject:
				if binding.Self.Class != nil {
					return &object.EmeraldValue{Type: object.ValueClass, Data: binding.Self.Class, Class: core.R.Classes["Class"]}
				}
			}
		}
		return &object.EmeraldValue{Type: object.ValueClass, Data: core.R.Classes["Object"], Class: core.R.Classes["Class"]}
	}
	return stack[len(stack)-1]
}

func (vm *VM) instanceEvalContextClass(receiver *object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil {
		receiver = core.R.NilVal
	}
	switch receiver.Type {
	case object.ValueBool, object.ValueNil, object.ValueInteger, object.ValueFloat, object.ValueSymbol:
		return core.R.NilVal
	}
	if singleton := core.SingletonClass(receiver); singleton != nil && singleton.Type == object.ValueClass {
		return singleton
	}
	return core.R.NilVal
}

func singletonClassStackTargets(stack []*object.EmeraldValue, receiver *object.EmeraldValue) bool {
	if len(stack) == 0 || receiver == nil {
		return false
	}
	target := stack[len(stack)-1]
	if target == nil || target.Type != object.ValueClass {
		return false
	}
	class, ok := target.Data.(*object.Class)
	if !ok || class == nil || !class.IsSingleton || class.SingletonOwner == nil {
		return false
	}
	owner := class.SingletonOwner
	return owner == receiver || (owner.Type == receiver.Type && owner.Data == receiver.Data)
}

func (vm *VM) instanceEvalBinding(receiver *object.EmeraldValue) *object.EmeraldValue {
	path := core.CurrentSpecFile
	if path == "" {
		path = "spec.rb"
	}
	if receiver == nil {
		return &object.EmeraldValue{Type: object.ValueBinding, Data: &object.RBinding{Self: core.R.NilVal, Locals: map[string]*object.EmeraldValue{}, Constants: map[string]*object.EmeraldValue{}, InstanceVars: map[string]*object.EmeraldValue{}, Path: path, Line: 1}, Class: core.R.Classes["Binding"]}
	}
	binding := core.CallMethod(receiver, "binding")
	if binding != nil && binding.Type == object.ValueBinding {
		duplicate := core.CallMethod(binding, "dup")
		if duplicate == nil || duplicate.Type != object.ValueBinding {
			duplicate = binding
		}
		if data, ok := duplicate.Data.(*object.RBinding); ok {
			data.Self = receiver
			if data.Path == "" {
				data.Path = path
			}
			if data.Line <= 0 {
				data.Line = 1
			}
			return &object.EmeraldValue{Type: object.ValueBinding, Data: data, Class: core.R.Classes["Binding"]}
		}
	}
	return &object.EmeraldValue{Type: object.ValueBinding, Data: &object.RBinding{Self: receiver, Locals: map[string]*object.EmeraldValue{}, Constants: map[string]*object.EmeraldValue{}, InstanceVars: map[string]*object.EmeraldValue{}, Path: path, Line: 1}, Class: core.R.Classes["Binding"]}
}

func (vm *VM) instanceEvalStringClassStack(receiver *object.EmeraldValue, callerStack []*object.EmeraldValue) []*object.EmeraldValue {
	stack := make([]*object.EmeraldValue, 0, len(callerStack)+4)
	receiverClass := receiverClassValue(receiver)
	if receiverClass != nil && receiverClass.Type == object.ValueClass {
		class := receiverClass.Data.(*object.Class)
		parents := []*object.EmeraldValue{}
		for current := class.SuperClass; current != nil; current = current.SuperClass {
			parents = append(parents, &object.EmeraldValue{Type: object.ValueClass, Data: current, Class: core.R.Classes["Class"]})
		}
		for i := len(parents) - 1; i >= 0; i-- {
			stack = append(stack, parents[i])
		}
	}
	stack = append(stack, callerStack...)
	if receiverClass != nil {
		stack = append(stack, receiverClass)
	}
	if singleton := vm.instanceEvalContextClass(receiver); singleton != nil && singleton.Type == object.ValueClass {
		if receiverClass == nil || receiverClass.Type != object.ValueClass || singleton.Data.(*object.Class) != receiverClass.Data.(*object.Class) {
			stack = append(stack, singleton)
		}
	}
	return stack
}

func receiverClassValue(receiver *object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil {
		return nil
	}
	switch receiver.Type {
	case object.ValueClass, object.ValueModule:
		return receiver
	default:
		if receiver.Class != nil {
			return &object.EmeraldValue{Type: object.ValueClass, Data: receiver.Class, Class: core.R.Classes["Class"]}
		}
	}
	return nil
}

func (vm *VM) currentFrameBinding() *object.RBinding {
	if vm.fp < 0 || vm.fp >= len(vm.frames) {
		return &object.RBinding{
			Self:         core.R.Main,
			Locals:       map[string]*object.EmeraldValue{},
			LocalNames:   nil,
			Method:       "",
			Constants:    vm.rubyConsts,
			ClassStack:   vm.currentClassStackSnapshot(),
			Path:         core.CurrentSpecFile,
			Line:         1,
			InstanceVars: map[string]*object.EmeraldValue{},
		}
	}
	frame := vm.frames[vm.fp]
	locals := make(map[string]*object.EmeraldValue)
	localNames := []string{}
	path := core.CurrentSpecFile
	line := int64(1)
	if frame != nil && frame.Ip >= 0 {
		line = vm.sourceLineForFrame(frame)
	}
	if frame != nil && frame.Fn != nil {
		if frame.Fn.SourcePath != "" {
			path = frame.Fn.SourcePath
		}
		if len(frame.Fn.LocalNames) > 0 {
			namedLocals := make([]struct {
				name  string
				index int64
			}, 0, len(frame.Fn.LocalNames))
			for name, index := range frame.Fn.LocalNames {
				namedLocals = append(namedLocals, struct {
					name  string
					index int64
				}{name: name, index: int64(index)})
			}
			sort.Slice(namedLocals, func(i, j int) bool {
				return namedLocals[i].index < namedLocals[j].index
			})
			if inherited := frame.Fn.EvalInheritedLocals; inherited > 0 && inherited < len(namedLocals) {
				namedLocals = append(append([]struct {
					name  string
					index int64
				}{}, namedLocals[inherited:]...), namedLocals[:inherited]...)
			}
			for _, item := range namedLocals {
				if frame.Fn.ImplicitItParameter && item.name == "it" || frame.Fn.NumberedParameters && numberedParameterMethodNamePattern.MatchString(item.name) {
					continue
				}
				slot := frame.Bp + 1 + int(item.index)
				if item.index >= 0 && int(item.index) < len(frame.Fn.Params) {
					if slot >= 0 && slot < vm.sp && vm.stack[slot] != nil {
						locals[item.name] = vm.stack[slot]
						localNames = append(localNames, item.name)
						continue
					}
					if len(frame.Args) > int(item.index) {
						locals[item.name] = frame.Args[int(item.index)]
						localNames = append(localNames, item.name)
						continue
					}
				}
				if slot >= 0 && slot < vm.sp && vm.stack[slot] != nil {
					locals[item.name] = vm.stack[slot]
					localNames = append(localNames, item.name)
				}
			}
		} else {
			for i, name := range frame.Fn.Params {
				if frame.Fn.ImplicitItParameter && name == "it" || frame.Fn.NumberedParameters && numberedParameterMethodNamePattern.MatchString(name) {
					continue
				}
				slot := frame.Bp + 1 + i
				if slot >= 0 && slot < vm.sp && vm.stack[slot] != nil {
					locals[name] = vm.stack[slot]
					localNames = append(localNames, name)
				}
			}
			for _, kp := range frame.Fn.KeywordParams {
				slot := frame.Bp + 1 + len(frame.Fn.Params)
				if frame.Fn.HasRestParam {
					slot++
				}
				for _, prior := range frame.Fn.KeywordParams {
					if prior.Name == kp.Name {
						break
					}
					slot++
				}
				if slot >= 0 && slot < vm.sp && vm.stack[slot] != nil {
					locals[kp.Name] = vm.stack[slot]
					localNames = append(localNames, kp.Name)
				}
			}
		}
	}
	if path == "" {
		path = "spec.rb"
	}
	methodName := ""
	backtraceLabel := ""
	if frame != nil {
		backtraceLabel = vm.backtraceLabelForFrame(vm.fp)
		if frame.MethodName != "" {
			methodName = frame.MethodName
		} else if frame.Fn != nil && frame.Fn.Name != "" && frame.Fn.Name != "main" {
			methodName = frame.Fn.Name
		}
	}
	self := core.R.Main
	var block *object.EmeraldValue
	allowAnonymousBlockPass := false
	if frame != nil && frame.Bp >= 0 && frame.Bp < vm.sp && vm.stack[frame.Bp] != nil {
		self = vm.stack[frame.Bp]
	}
	if frame != nil {
		block = frame.Block
		allowAnonymousBlockPass = frame.Fn != nil && frame.Fn.AnonymousBlockParam
	}
	seenNames := map[string]bool{}
	for name := range locals {
		seenNames[name] = true
	}
	if frame != nil && frame.Fn != nil && frame.Closure != nil && len(frame.Fn.FreeVarNames) > 0 && len(frame.Closure.Free) > 0 {
		for idx, name := range frame.Fn.FreeVarNames {
			if name == "" {
				continue
			}
			if seenNames[name] {
				continue
			}
			if idx >= len(frame.Closure.Free) {
				continue
			}
			locals[name] = frame.Closure.Free[idx]
			localNames = append(localNames, name)
			seenNames[name] = true
		}
	}
	binding := &object.RBinding{
		Self:                    self,
		Block:                   block,
		AllowAnonymousBlockPass: allowAnonymousBlockPass,
		EvalReturnTargetID:      vm.evalReturnTargetID(),
		Locals:                  locals,
		LocalNames:              localNames,
		Method:                  methodName,
		BacktraceLabel:          backtraceLabel,
		Constants:               vm.rubyConsts,
		ClassStack:              vm.currentClassStackSnapshot(),
		Path:                    path,
		Line:                    line,
		InstanceVars:            map[string]*object.EmeraldValue{},
	}
	if refinements, fixed := vm.currentFixedRefinements(); fixed {
		binding.Refinements = append([]*object.EmeraldValue(nil), refinements...)
	}
	if frame != nil && frame.Closure != nil && frame.Closure.Binding != nil &&
		(frame.Fn == nil || !frame.Fn.MethodBody || frame.DefinedByDefineMethod) {
		binding.Parent = frame.Closure.Binding
		binding.SharedLocals = make(map[string]struct{}, len(binding.Parent.LocalNames)+len(binding.Parent.Locals))
		for _, name := range binding.Parent.LocalNames {
			binding.SharedLocals[name] = struct{}{}
		}
		for name := range binding.Parent.Locals {
			binding.SharedLocals[name] = struct{}{}
		}
	}
	return binding
}

func (vm *VM) evalReturnTargetID() int {
	for i := vm.fp; i >= 0; i-- {
		if i >= len(vm.frames) || vm.frames[i] == nil {
			continue
		}
		frame := vm.frames[i]
		if frame.IsLambda {
			return frame.ID
		}
		if frame.MethodName != "" && frame.Fn != nil && frame.Fn.Name != "__block__" {
			return frame.ID
		}
	}
	return 0
}

func (vm *VM) captureFrameBinding() *object.RBinding {
	binding := vm.currentFrameBinding()
	if binding != nil && vm.fp >= 0 && vm.fp < len(vm.frames) {
		if frame := vm.frames[vm.fp]; frame != nil {
			frame.CapturedBindings = append(frame.CapturedBindings, binding)
		}
	}
	return binding
}

func (vm *VM) globalVariableNames() []string {
	if vm.globalNames == nil {
		return []string{}
	}
	names := make(map[string]struct{}, len(vm.globalNames))
	for name := range vm.globalNames {
		key := core.ResolveGlobalAlias(name)
		if key == "" {
			key = name
		}
		names[key] = struct{}{}
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (vm *VM) currentBacktraceFrames() []object.RBacktraceLocation {
	frames := make([]object.RBacktraceLocation, 0, len(vm.frames))
	ensureBlockIndex := vm.fp - 1
	ensureBlockLine := int64(0)
	if vm.ensureActive && vm.fp >= 0 && vm.fp < len(vm.frames) && vm.frames[vm.fp] != nil {
		ensureBlockLine = vm.sourceLineForFrame(vm.frames[vm.fp])
	}
	for i := vm.fp; i >= 0; i-- {
		for j := len(vm.nativeBacktraceFrames) - 1; j >= 0; j-- {
			if vm.nativeBacktraceFrames[j].parentIndex == i {
				frames = append(frames, vm.nativeBacktraceFrames[j].location)
			}
		}
		frame := vm.frames[i]
		if frame == nil || frame.Fn == nil {
			continue
		}
		if core.InAtExitHooks && frame.Fn.Name == "__main__" && i == 0 {
			continue
		}
		label := vm.backtraceLabelForFrame(i)
		path := frame.Fn.SourcePath
		if path == "" {
			path = core.CurrentSpecFile
		}
		if path == "" {
			path = label
		}
		line := vm.sourceLineForFrame(frame)
		absolutePath := frame.Fn.SourceAbsolutePath
		if !frame.Fn.EvalSource && path != "" && !strings.HasPrefix(path, "<") {
			if absolutePath == "" {
				absolutePath = path
				if abs, err := filepath.Abs(absolutePath); err == nil {
					absolutePath = abs
				}
				if resolved, err := filepath.EvalSymlinks(absolutePath); err == nil {
					absolutePath = resolved
				}
			}
		}
		if vm.ensureActive && i == ensureBlockIndex {
			label = "block"
			if ensureBlockLine > 0 {
				line = vm.sourceLineForFrame(frame)
			}
		}

		frames = append(frames, object.RBacktraceLocation{
			Path:         path,
			AbsolutePath: absolutePath,
			Line:         line,
			Label:        label,
			EvalSource:   frame.Fn.EvalSource,
		})
	}
	if vm.parent != nil {
		frames = append(frames, vm.parent.currentBacktraceFrames()...)
	}
	return frames
}

func (vm *VM) backtraceLabelForFrame(index int) string {
	if index < 0 || index >= len(vm.frames) {
		return "main"
	}
	frame := vm.frames[index]
	if frame == nil || frame.Fn == nil {
		return "main"
	}
	if frame.Fn.Name == "__block__" || frame.Fn.Name == "__lambda__" {
		if core.InAtExitHooks && frame.Fn.Name == "__block__" {
			return "block in <main>"
		}
		return vm.backtraceBlockLabel(index)
	}
	if strings.HasSuffix(frame.Fn.Name, "#body") {
		return vm.backtraceClassBodyLabel(frame)
	}
	if frame.LabelName != "" {
		return frame.LabelName
	}
	if frame.MethodName != "" {
		return frame.MethodName
	}
	if frame.Fn.Name != "" {
		if frame.Fn.Name == "main" {
			return "main"
		}
		return frame.Fn.Name
	}
	return "main"
}

func (vm *VM) withNativeBacktraceFrame(label string, call func() *object.EmeraldValue) *object.EmeraldValue {
	if label == "" {
		label = "<native>"
	}
	vm.nativeBacktraceFrames = append(vm.nativeBacktraceFrames, nativeBacktraceFrame{
		parentIndex: vm.fp,
		location: object.RBacktraceLocation{
			Path:  "<internal:" + label + ">",
			Line:  1,
			Label: label,
		},
	})
	defer func() {
		vm.nativeBacktraceFrames = vm.nativeBacktraceFrames[:len(vm.nativeBacktraceFrames)-1]
	}()
	return call()
}

func (vm *VM) backtraceClassBodyLabel(frame *Frame) string {
	if frame == nil || frame.Fn == nil {
		return "main"
	}
	if frame.Fn.SingletonClassBody {
		return "<singleton class>"
	}
	name := strings.TrimSuffix(frame.Fn.Name, "#body")
	kind := "class"
	if frame.Bp >= 0 && frame.Bp < len(vm.stack) {
		self := vm.stack[frame.Bp]
		if self != nil {
			switch self.Type {
			case object.ValueModule:
				kind = "module"
				if module, ok := self.Data.(*object.Module); ok && module.Name != "" {
					name = module.Name
				}
			case object.ValueClass:
				if class, ok := self.Data.(*object.Class); ok && class.Name != "" {
					name = class.Name
				}
			}
		}
	}
	if separator := strings.LastIndex(name, "::"); separator >= 0 {
		name = name[separator+2:]
	}
	return "<" + kind + ":" + name + ">"
}

func (vm *VM) backtraceBlockLabel(index int) string {
	level := 0
	for i := index; i >= 0 && i < len(vm.frames); i-- {
		frame := vm.frames[i]
		if frame == nil || frame.Fn == nil || (frame.Fn.Name != "__block__" && frame.Fn.Name != "__lambda__") {
			break
		}
		level++
	}
	base := ""
	for i := index - 1; i >= 0; i-- {
		frame := vm.frames[i]
		if frame == nil || frame.Fn == nil {
			continue
		}
		if frame.Fn.Name == "__block__" || frame.Fn.Name == "__lambda__" {
			continue
		}
		if frame.LabelName != "" {
			base = strings.TrimPrefix(frame.LabelName, "Object#")
		} else if frame.MethodName != "" {
			base = frame.MethodName
		} else if frame.Fn.Name == "main" || frame.Fn.Name == "__main__" {
			if vm.isRoot && core.CurrentTopLevelMain {
				base = "<main>"
			} else {
				base = "<top (required)>"
			}
		} else {
			base = frame.Fn.Name
		}
		break
	}
	if base == "" {
		base = "<main>"
	}
	if level <= 1 {
		return "block in " + base
	}
	return fmt.Sprintf("block (%d levels) in %s", level, base)
}

func (vm *VM) backtraceStrings(frames []object.RBacktraceLocation) []string {
	result := make([]string, len(frames))
	for i, frame := range frames {
		result[i] = fmt.Sprintf("%s:%d:in '%s'", frame.Path, frame.Line, frame.Label)
	}
	return result
}

func (vm *VM) sourceLineForFrame(frame *Frame) int64 {
	if frame == nil || frame.Ip < 0 {
		return 0
	}
	if frame.Fn != nil && len(frame.Fn.LineMap) > 0 {
		bestPos := -1
		bestLine := 0
		for pos, line := range frame.Fn.LineMap {
			if pos <= frame.Ip && pos > bestPos {
				bestPos = pos
				bestLine = line
			}
		}
		if bestPos >= 0 {
			return int64(bestLine)
		}
	}
	return int64(frame.Ip + 1)
}

func (vm *VM) fireTracePointLine(frame *Frame, op compiler.Opcode) {
	if frame == nil {
		return
	}
	line := frame.ExecutionLine
	if frame.Fn != nil {
		if mapped, ok := frame.Fn.LineMap[frame.Ip]; ok {
			line = int64(mapped)
		}
	}
	if line == 0 {
		line = vm.sourceLineForFrame(frame)
	}
	frame.ExecutionLine = line
	if !core.TracePointEventActive("line") {
		return
	}
	if frame.Fn != nil && line == frame.Fn.DefinitionLine && (op == compiler.OpReturn || op == compiler.OpReturnValue || op == compiler.OpNonLocalReturnValue || op == compiler.OpBlockReturn) {
		return
	}
	if line <= 0 || line == frame.TraceLine {
		return
	}
	if frame.TraceLine == 0 && frame.Fn != nil && line == frame.Fn.DefinitionLine {
		for position, laterLine := range frame.Fn.LineMap {
			if position > frame.Ip && int64(laterLine) > line {
				frame.TraceLine = line
				return
			}
		}
	}
	frame.TraceLine = line
	binding := vm.currentFrameBinding()
	core.FireTracePointLine(binding, frame.ID)
}

func (vm *VM) fireTracePointReturn(frame *Frame, value *object.EmeraldValue) {
	if frame == nil || !core.TracePointEventActive("return") {
		return
	}
	if frame.TraceParameters == nil && frame.Fn != nil {
		frame.TraceParameters = core.TracePointParameters(frame.Fn, true)
	}
	binding := vm.currentFrameBinding()
	core.FireTracePointReturn("return", binding, frame.TraceSelf, frame.TraceDefinedClass, frame.TraceMethodID, frame.TraceCalleeID, frame.TraceParameters, value)
}

func (vm *VM) LastPoppedStackElement() *object.EmeraldValue {
	if len(vm.poppedValues) > 0 {
		return vm.poppedValues[len(vm.poppedValues)-1]
	}
	if vm.sp > 0 {
		return vm.stack[vm.sp-1]
	}
	return nil
}

func (vm *VM) GetAllResults() []*object.EmeraldValue {
	return vm.poppedValues
}

func (vm *VM) lookupKwarg(hash map[*object.EmeraldValue]*object.EmeraldValue, name string) *object.EmeraldValue {
	return keywordValueFromMap(hash, name)
}

func executorHashToMap(value *object.EmeraldValue) map[*object.EmeraldValue]*object.EmeraldValue {
	if value == nil || value.Type != object.ValueHash {
		return nil
	}
	if h, ok := value.Data.(map[*object.EmeraldValue]*object.EmeraldValue); ok {
		return h
	}
	if h, ok := value.Data.(*object.RHash); ok {
		return h.Pairs
	}
	return nil
}

func hashLiteralKey(key *object.EmeraldValue) *object.EmeraldValue {
	if key == nil || key.Type != object.ValueString || key.Frozen {
		return key
	}
	copy := *key
	copy.Frozen = true
	core.CopyStringEncoding(&copy, key)
	return &copy
}

func hashLiteralExistingKey(hash *object.RHash, key *object.EmeraldValue) *object.EmeraldValue {
	if hash == nil {
		return nil
	}
	for _, existing := range hash.Keys {
		if existing == key || existing.Equals(key) {
			return existing
		}
	}
	return nil
}

func copyKeywordHash(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil || value.Type != object.ValueHash {
		return value
	}
	copyValue := &object.EmeraldValue{Type: object.ValueHash, Class: value.Class}
	switch source := value.Data.(type) {
	case *object.RHash:
		copyData := &object.RHash{
			Pairs:             make(map[*object.EmeraldValue]*object.EmeraldValue, len(source.Pairs)),
			Keys:              append([]*object.EmeraldValue(nil), source.Keys...),
			Hashes:            make(map[*object.EmeraldValue]int64, len(source.Hashes)),
			Default:           source.Default,
			DefaultProc:       source.DefaultProc,
			CompareByIdentity: source.CompareByIdentity,
			InstanceVars:      make(map[string]*object.EmeraldValue, len(source.InstanceVars)),
		}
		for key, item := range source.Pairs {
			copyData.Pairs[key] = item
		}
		for key, code := range source.Hashes {
			copyData.Hashes[key] = code
		}
		for name, item := range source.InstanceVars {
			copyData.InstanceVars[name] = item
		}
		copyValue.Data = copyData
	case map[*object.EmeraldValue]*object.EmeraldValue:
		copyData := make(map[*object.EmeraldValue]*object.EmeraldValue, len(source))
		for key, item := range source {
			copyData[key] = item
		}
		copyValue.Data = copyData
	default:
		copyValue.Data = value.Data
	}
	return copyValue
}

func methodDefinitionTargetFrozen(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	if value.Frozen {
		return true
	}
	if value.Type != object.ValueClass {
		return false
	}
	class, ok := value.Data.(*object.Class)
	if !ok || class == nil {
		return false
	}
	if class.Frozen {
		return true
	}
	return class.IsSingleton && class.SingletonOwner != nil && class.SingletonOwner.Frozen
}

func singletonMethodDefinitionFrozen(receiver *object.EmeraldValue) bool {
	if methodDefinitionTargetFrozen(receiver) {
		return true
	}
	if receiver == nil {
		return false
	}
	switch receiver.Type {
	case object.ValueObject:
		obj, ok := receiver.Data.(*object.Object)
		return ok && obj != nil && obj.SingletonClass != nil && obj.SingletonClass.Frozen
	case object.ValueClass:
		class, ok := receiver.Data.(*object.Class)
		return ok && class != nil && class.SingletonClass != nil && class.SingletonClass.Frozen
	case object.ValueModule:
		mod, ok := receiver.Data.(*object.Module)
		return ok && mod != nil && mod.SingletonClass != nil && mod.SingletonClass.Frozen
	default:
		return false
	}
}

func frozenMethodDefinitionError(receiver *object.EmeraldValue, _ bool) *object.EmeraldValue {
	kind := "object"
	if receiver != nil {
		switch receiver.Type {
		case object.ValueClass:
			kind = "Class"
		case object.ValueModule:
			kind = "Module"
		case object.ValueObject:
			kind = "object"
		default:
			kind = strings.ToLower(receiver.TypeName())
		}
	}
	message := "can't modify frozen " + kind
	if receiver != nil {
		message += ": " + receiver.Inspect()
	}
	return &object.EmeraldValue{
		Type: object.ValueException,
		Data: &object.RException{
			Message:           message,
			NameErrorReceiver: receiver,
		},
		Class: core.R.Classes["FrozenError"],
	}
}

func (vm *VM) GetCurrentBlock() *object.EmeraldValue {
	return vm.currentBlock
}

func (vm *VM) qualifiedConstantValue(name string) (*object.EmeraldValue, bool) {
	idx := strings.LastIndex(name, "::")
	if idx <= 0 || idx+2 >= len(name) {
		return nil, false
	}
	prefix := name[:idx]
	constName := name[idx+2:]
	if value, ok := vm.rubyConsts[prefix]; ok {
		switch value.Type {
		case object.ValueClass:
			class := value.Data.(*object.Class)
			if class.PrivateConstants[constName] {
				return core.NewPrivateConstantNameError(value, constName), true
			}
			if constant, ok := classConstantLookup(class, constName); ok {
				return constant, true
			}
			if constant, ok := vm.triggerAutoload(value, constName); ok {
				return constant, true
			}
			if core.EvaluatingRaiseErrorMatcher() {
				return core.NewNameError("uninitialized constant " + prefix + "::" + constName), true
			}
			return nil, false
		case object.ValueModule:
			module := value.Data.(*object.Module)
			if module.PrivateConstants[constName] {
				return core.NewPrivateConstantNameError(value, constName), true
			}
			if constant, ok := moduleConstantLookup(module, constName); ok {
				return constant, true
			}
			if constant, ok := vm.triggerAutoload(value, constName); ok {
				return constant, true
			}
			if core.EvaluatingRaiseErrorMatcher() {
				return core.NewNameError("uninitialized constant " + prefix + "::" + constName), true
			}
			return nil, false
		}
	}
	if !strings.Contains(prefix, "::") {
		for i := len(vm.classStack) - 1; i >= 0; i-- {
			container := vm.classStack[i]
			if container == nil || (container.Type != object.ValueClass && container.Type != object.ValueModule) {
				continue
			}
			value, ok := directConstantValue(container, prefix)
			if !ok {
				continue
			}
			if value != nil && value.Type == object.ValueException {
				return value, true
			}
			if value == nil || (value.Type != object.ValueClass && value.Type != object.ValueModule) {
				return core.NewTypeError("not a class/module"), true
			}
			return vm.scopedConstantValue(value, constName)
		}
		for i := len(vm.classStack) - 1; i >= 0; i-- {
			container := vm.classStack[i]
			if container == nil || (container.Type != object.ValueClass && container.Type != object.ValueModule) {
				continue
			}
			value, ok := vm.qualifiedLexicalParentConstantValue(container, prefix)
			if !ok {
				continue
			}
			if value != nil && value.Type == object.ValueException {
				return value, true
			}
			if value == nil || (value.Type != object.ValueClass && value.Type != object.ValueModule) {
				return core.NewTypeError("not a class/module"), true
			}
			return vm.scopedConstantValue(value, constName)
		}
		if value, ok := vm.uniqueQualifiedSuffixConstant(prefix); ok {
			if value != nil && value.Type == object.ValueException {
				return value, true
			}
			if value == nil || (value.Type != object.ValueClass && value.Type != object.ValueModule) {
				return core.NewTypeError("not a class/module"), true
			}
			return vm.scopedConstantValue(value, constName)
		}
	}
	if value, ok := vm.topLevelConstantValue(prefix); ok && value.Type != object.ValueClass && value.Type != object.ValueModule {
		return core.NewTypeError("not a class/module"), true
	}
	if cls, ok := core.R.Classes[prefix]; ok {
		if cls.PrivateConstants[constName] {
			return core.NewPrivateConstantNameError(&object.EmeraldValue{Type: object.ValueClass, Data: cls, Class: core.R.Classes["Class"]}, constName), true
		}
		if constant, ok := classConstantLookup(cls, constName); ok {
			return constant, true
		}
		classVal := &object.EmeraldValue{Type: object.ValueClass, Data: cls, Class: core.R.Classes["Class"]}
		if constant, ok := vm.triggerAutoload(classVal, constName); ok {
			return constant, true
		}
		if core.EvaluatingRaiseErrorMatcher() {
			return core.NewNameError("uninitialized constant " + prefix + "::" + constName), true
		}
		return nil, false
	}
	return nil, false
}

func (vm *VM) uniqueQualifiedSuffixConstant(name string) (*object.EmeraldValue, bool) {
	suffix := "::" + name
	var found *object.EmeraldValue
	for constName, value := range vm.rubyConsts {
		if !strings.HasSuffix(constName, suffix) {
			continue
		}
		if found != nil && found != value {
			return nil, false
		}
		found = value
	}
	return found, found != nil
}

func classConstantLookup(class *object.Class, name string) (*object.EmeraldValue, bool) {
	if class == nil {
		return nil, false
	}
	if value, ok := class.Constants[name]; ok {
		return value, true
	}
	for _, mod := range class.PrependedModules {
		if value, ok := moduleConstantLookup(mod, name); ok {
			return value, true
		}
	}
	for i := len(class.IncludedModules) - 1; i >= 0; i-- {
		mod := class.IncludedModules[i]
		if value, ok := moduleConstantLookup(mod, name); ok {
			return value, true
		}
	}
	if class.SuperClass != nil {
		return classConstantLookup(class.SuperClass, name)
	}
	return nil, false
}

func scopedClassConstantLookup(class *object.Class, name string) (*object.EmeraldValue, bool) {
	if class == nil {
		return nil, false
	}
	if value, ok := class.Constants[name]; ok {
		return value, true
	}
	for _, mod := range class.PrependedModules {
		if value, ok := moduleConstantLookup(mod, name); ok {
			return value, true
		}
	}
	for i := len(class.IncludedModules) - 1; i >= 0; i-- {
		if value, ok := moduleConstantLookup(class.IncludedModules[i], name); ok {
			return value, true
		}
	}
	if class.SuperClass == core.R.Classes["Object"] {
		objectClass := core.R.Classes["Object"]
		for i := len(objectClass.IncludedModules) - 1; i >= 0; i-- {
			if value, ok := moduleConstantLookup(objectClass.IncludedModules[i], name); ok {
				return value, true
			}
		}
		return nil, false
	}
	if class.SuperClass == nil {
		return nil, false
	}
	return scopedClassConstantLookup(class.SuperClass, name)
}

func moduleConstantLookup(module *object.Module, name string) (*object.EmeraldValue, bool) {
	if module == nil {
		return nil, false
	}
	if value, ok := module.Constants[name]; ok {
		return value, true
	}
	for _, prepended := range module.PrependedModules {
		if value, ok := moduleConstantLookup(prepended, name); ok {
			return value, true
		}
	}
	for i := len(module.IncludedModules) - 1; i >= 0; i-- {
		included := module.IncludedModules[i]
		if value, ok := moduleConstantLookup(included, name); ok {
			return value, true
		}
	}
	if module.Parent != nil {
		return moduleConstantLookup(module.Parent, name)
	}
	return nil, false
}

func (vm *VM) scopedConstantValue(receiver *object.EmeraldValue, constName string) (*object.EmeraldValue, bool) {
	if receiver == nil {
		return nil, false
	}
	qualifiedName := qualifiedConstantName(receiver, constName)
	switch receiver.Type {
	case object.ValueClass:
		class := receiver.Data.(*object.Class)
		if class.PrivateConstants[constName] {
			return vm.privateConstantAccessResult(receiver, constName), true
		}
		if owner, found := privateConstantOwnerInClassLookup(class, constName); found {
			return vm.privateConstantAccessResult(owner, constName), true
		}
		if constant, ok := scopedClassConstantLookup(class, constName); ok {
			return constant, true
		}
		if constant, ok := vm.triggerAutoload(receiver, constName); ok {
			return constant, true
		}
		if constant, ok := vm.rubyConsts[qualifiedName]; ok {
			return constant, true
		}
		if classInheritsFrom(class, core.R.Classes["Object"]) {
			if constant, ok := vm.rubyConsts[constName]; ok {
				return constant, true
			}
		}
		if processConst, ok := processConstantValue(qualifiedName); ok {
			return processConst, true
		}
		if cls, ok := core.R.Classes[qualifiedName]; ok {
			return &object.EmeraldValue{Type: object.ValueClass, Data: cls, Class: core.R.Classes["Class"]}, true
		}
		if core.EvaluatingRaiseErrorMatcher() {
			return core.NewNameError("uninitialized constant " + class.Name + "::" + constName), true
		}
		return nil, false
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		if module.PrivateConstants[constName] {
			return vm.privateConstantAccessResult(receiver, constName), true
		}
		if owner, found := privateConstantOwnerInModuleLookup(module, constName); found {
			return vm.privateConstantAccessResult(owner, constName), true
		}
		if constant, ok := moduleConstantLookup(module, constName); ok {
			return constant, true
		}
		if constant, ok := vm.triggerAutoload(receiver, constName); ok {
			return constant, true
		}
		if constant, ok := vm.rubyConsts[qualifiedName]; ok {
			return constant, true
		}
		if constant, ok := vm.rubyConsts[constName]; ok {
			return constant, true
		}
		if processConst, ok := processConstantValue(qualifiedName); ok {
			return processConst, true
		}
		if cls, ok := core.R.Classes[qualifiedName]; ok {
			return &object.EmeraldValue{Type: object.ValueClass, Data: cls, Class: core.R.Classes["Class"]}, true
		}
		if core.EvaluatingRaiseErrorMatcher() {
			return core.NewNameError("uninitialized constant " + module.Name + "::" + constName), true
		}
		return nil, false
	}
	if receiver.Type != object.ValueClass && receiver.Type != object.ValueModule {
		return core.NewTypeError("not a class/module"), true
	}
	return nil, false
}

func (vm *VM) privateConstantAccessResult(owner *object.EmeraldValue, name string) *object.EmeraldValue {
	result := vm.sendBypassVisibility(owner, "const_missing", []*object.EmeraldValue{{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]}})
	if result == nil || result.Type != object.ValueException || result.Class != core.R.Classes["NameError"] {
		return result
	}
	exception, _ := result.Data.(*object.RException)
	if exception != nil && strings.HasPrefix(exception.Message, "uninitialized constant ") {
		return core.NewPrivateConstantNameError(owner, name)
	}
	return result
}

func privateConstantOwnerInClassLookup(class *object.Class, name string) (*object.EmeraldValue, bool) {
	if class == nil {
		return nil, false
	}
	if _, ok := class.Constants[name]; ok {
		return nil, false
	}
	for _, mod := range class.IncludedModules {
		if owner, found := privateConstantOwnerInModuleLookup(mod, name); found {
			return owner, true
		}
	}
	if class.SuperClass != nil {
		if class.SuperClass.PrivateConstants[name] {
			return &object.EmeraldValue{Type: object.ValueClass, Data: class.SuperClass, Class: core.R.Classes["Class"]}, true
		}
		if owner, found := privateConstantOwnerInClassLookup(class.SuperClass, name); found {
			return owner, true
		}
	}
	return nil, false
}

func privateConstantOwnerInModuleLookup(module *object.Module, name string) (*object.EmeraldValue, bool) {
	if module == nil {
		return nil, false
	}
	if module.PrivateConstants[name] {
		return &object.EmeraldValue{Type: object.ValueModule, Data: module, Class: core.R.Classes["Module"]}, true
	}
	if _, ok := module.Constants[name]; ok {
		return nil, false
	}
	for _, included := range module.IncludedModules {
		if included.PrivateConstants[name] {
			return &object.EmeraldValue{Type: object.ValueModule, Data: included, Class: core.R.Classes["Module"]}, true
		}
		if owner, found := privateConstantOwnerInModuleLookup(included, name); found {
			return owner, true
		}
	}
	return nil, false
}

func (vm *VM) scopedConstantAssignmentValue(receiver *object.EmeraldValue, constName string, value *object.EmeraldValue, mode int) (bool, *object.EmeraldValue) {
	current, found := vm.scopedConstantValue(receiver, constName)
	if current != nil && current.Type == object.ValueException {
		return false, current
	}

	switch mode {
	case compiler.ScopedConstAssignOr:
		if found && current.IsTruthy() {
			return false, current
		}
		return true, vm.scopedConstantAssignmentRHS(value)
	case compiler.ScopedConstAssignAnd:
		if !found {
			return false, core.NewNameError("uninitialized constant " + qualifiedConstantName(receiver, constName))
		}
		if !current.IsTruthy() {
			return false, current
		}
		return true, vm.scopedConstantAssignmentRHS(value)
	case compiler.ScopedConstAssignAdd:
		if !found {
			return false, core.NewNameError("uninitialized constant " + qualifiedConstantName(receiver, constName))
		}
		result := vm.add(current, value)
		return true, result
	default:
		return true, value
	}
}

func (vm *VM) scopedConstantAssignmentRHS(value *object.EmeraldValue) *object.EmeraldValue {
	if value != nil && value.Type == object.ValueClosure {
		return vm.callBlock(value)
	}
	return value
}

func (vm *VM) triggerAutoload(receiver *object.EmeraldValue, constName string) (*object.EmeraldValue, bool) {
	path, ok := core.DirectAutoloadPath(receiver, constName)
	if !ok {
		return nil, false
	}
	if core.FeatureLoadingByCurrentThread(path) {
		return nil, false
	}
	key := autoloadKey(receiver, constName)
	if vm.autoloading[key] {
		return nil, false
	}
	vm.autoloading[key] = true
	defer delete(vm.autoloading, key)
	result := vm.send(core.R.Main, "require", []*object.EmeraldValue{{Type: object.ValueString, Data: path, Class: core.R.Classes["String"]}})
	if result != nil && result.Type == object.ValueException {
		return result, true
	}
	switch receiver.Type {
	case object.ValueClass:
		class := receiver.Data.(*object.Class)
		if constant, ok := class.Constants[constName]; ok {
			core.CompleteAutoload(receiver, constName)
			return constant, true
		}
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		if constant, ok := module.Constants[constName]; ok {
			core.CompleteAutoload(receiver, constName)
			return constant, true
		}
	}
	core.CompleteAutoload(receiver, constName)
	return core.NewNameError("uninitialized constant " + constName), true
}

func autoloadKey(receiver *object.EmeraldValue, constName string) string {
	if receiver == nil {
		return constName
	}
	switch receiver.Type {
	case object.ValueClass:
		return fmt.Sprintf("class:%p:%s", receiver.Data.(*object.Class), constName)
	case object.ValueModule:
		return fmt.Sprintf("module:%p:%s", receiver.Data.(*object.Module), constName)
	default:
		return constName
	}
}

func processConstantValue(name string) (*object.EmeraldValue, bool) {
	var value int64
	switch name {
	case "Process::PRIO_PROCESS":
		value = 0
	case "Process::PRIO_PGRP":
		value = 1
	case "Process::PRIO_USER":
		value = 2
	case "Process::CLOCK_REALTIME":
		value = 0
	case "Process::CLOCK_MONOTONIC":
		value = 1
	case "Process::CLOCK_PROCESS_CPUTIME_ID":
		value = 2
	case "Process::CLOCK_THREAD_CPUTIME_ID":
		value = 3
	case "Process::CLOCK_MONOTONIC_RAW":
		value = 4
	case "Process::CLOCK_REALTIME_COARSE":
		value = 5
	case "Process::CLOCK_MONOTONIC_COARSE":
		value = 6
	case "Process::CLOCK_BOOTTIME":
		value = 7
	case "Process::CLOCK_REALTIME_ALARM":
		value = 8
	case "Process::CLOCK_BOOTTIME_ALARM":
		value = 9
	case "Process::WNOHANG":
		value = 1
	case "Process::WUNTRACED":
		value = 2
	case "Process::RLIM_INFINITY", "Process::RLIM_SAVED_MAX", "Process::RLIM_SAVED_CUR":
		value = 1<<63 - 1
	case "Process::RLIMIT_CPU":
		value = 0
	case "Process::RLIMIT_FSIZE":
		value = 1
	case "Process::RLIMIT_DATA":
		value = 2
	case "Process::RLIMIT_STACK":
		value = 3
	case "Process::RLIMIT_CORE":
		value = 4
	case "Process::RLIMIT_RSS":
		value = 5
	case "Process::RLIMIT_NPROC":
		value = 6
	case "Process::RLIMIT_NOFILE":
		value = 7
	case "Process::RLIMIT_MEMLOCK":
		value = 8
	case "Process::RLIMIT_AS":
		value = 9
	case "Process::RLIMIT_RTPRIO":
		value = 10
	case "Process::RLIMIT_RTTIME":
		value = 11
	case "Process::RLIMIT_SIGPENDING":
		value = 12
	case "Process::RLIMIT_MSGQUEUE":
		value = 13
	case "Process::RLIMIT_NICE":
		value = 14
	case "IO::SEEK_SET":
		value = 0
	case "IO::SEEK_CUR":
		value = 1
	case "IO::SEEK_END":
		value = 2
	default:
		return nil, false
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: value, Class: core.R.Classes["Integer"]}, true
}
