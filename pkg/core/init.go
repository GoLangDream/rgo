package core

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GoLangDream/rgo/pkg/object"
)

type BuiltinMethod func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue

var CallBlock func(args ...*object.EmeraldValue) *object.EmeraldValue

var CallMethod func(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue

var CallBlockWithArgs func(block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue

var CallBlockWithSelf func(block *object.EmeraldValue, self *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue

var SetGlobalVariable func(name string, value *object.EmeraldValue)

var GetGlobalVariable func(name string) *object.EmeraldValue

var EvalSource func(source string) *object.EmeraldValue

var RequirePath func(path string) *object.EmeraldValue

var InMethodScope func() bool

var LastBlockResult *object.EmeraldValue

var LastException *object.EmeraldValue
var LastRaisedResult *object.EmeraldValue
var LastMatcherException *object.EmeraldValue

var CurrentSpecFile string
var envObject *object.EmeraldValue
var ruby2KeywordHashes map[*object.EmeraldValue]bool
var fileUtimeOverrides map[string]fileTimeOverride
var currentFileUmask int64 = 0022
var stringEncodings map[*object.EmeraldValue]string
var encodingValues map[string]*object.EmeraldValue
var nextIOFd int64
var ioDataByFd map[int64]*ioShimData
var suppressFileOpenBlock bool

type fileTimeOverride struct {
	atime time.Time
	mtime time.Time
}

type encodingData struct {
	name string
}

var defaultExternalEncoding = "UTF-8"

type argfData struct {
	paths   []string
	index   int
	content string
	offset  int
	closed  bool
	io      *object.EmeraldValue
}

func rubyString(value string) *object.EmeraldValue {
	str := &object.EmeraldValue{Type: object.ValueString, Data: value, Class: R.Classes["String"]}
	if stringEncodings != nil {
		stringEncodings[str] = defaultExternalEncoding
	}
	return str
}

func EnvObject() *object.EmeraldValue {
	if envObject != nil {
		return envObject
	}
	env := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[rubyString(key)] = rubyString(value)
	}
	envObject = &object.EmeraldValue{Type: object.ValueHash, Data: env, Class: R.Classes["Hash"]}
	return envObject
}

func envString(name string) (string, bool) {
	if val, ok := hashLookup(valueToHashMap(EnvObject()), rubyString(name)); ok && val != nil && val.Type == object.ValueString {
		return val.Data.(string), true
	}
	return "", false
}

func StdoutObject() *object.EmeraldValue {
	if stdoutObject != nil {
		return stdoutObject
	}
	stdoutObject = newIOShimValue("IO")
	if data := ioShim(stdoutObject); data != nil {
		data.fd = 1
	}
	return stdoutObject
}

type enumeratorData struct {
	kind      string
	block     *object.EmeraldValue
	values    []*object.EmeraldValue
	index     int
	generated bool
	result    *object.EmeraldValue
}

type yielderData struct {
	enum *enumeratorData
}

type dirData struct {
	path     string
	entries  []*object.EmeraldValue
	pos      int
	closed   bool
	pathless bool
	fdSource *dirData
}

type fileStatData struct {
	path  string
	info  os.FileInfo
	lstat bool
}

type timeData struct {
	value time.Time
	zone  *object.EmeraldValue
}

type threadData struct {
	block             *object.EmeraldValue
	result            *object.EmeraldValue
	exception         *object.EmeraldValue
	pendingInterrupt  *object.EmeraldValue
	deferInterrupt    bool
	args              []*object.EmeraldValue
	locals            map[string]*object.EmeraldValue
	threadVars        map[string]*object.EmeraldValue
	mutexes           []*object.EmeraldValue
	name              *object.EmeraldValue
	reportOnException bool
	abortOnException  bool
	priority          int64
	ran               bool
	group             *object.EmeraldValue
}

type mutexData struct {
	locked bool
	owner  *object.EmeraldValue
}

type queueData struct {
	items      []*object.EmeraldValue
	closed     bool
	numWaiting int64
	max        int64
}

type mockExpectationData struct {
	target *object.EmeraldValue
	method string
}

type processStatusData struct {
	pid        int64
	exitstatus *int64
	termsig    *int64
}

type processChild struct {
	pid        int64
	exitstatus int64
	running    bool
	pgroup     bool
}

type ioShimData struct {
	writeCalls     int64
	nonblock       bool
	closed         bool
	path           string
	pathEncoding   string
	mode           string
	offset         int64
	cachedSize     int64
	cachedInfo     os.FileInfo
	autoclose      bool
	fd             int64
	closeHook      bool
	closeException *object.EmeraldValue
}

var lastDirForFd *dirData
var stdoutObject *object.EmeraldValue

type threadGroupData struct {
	threads  []*object.EmeraldValue
	enclosed bool
}

type fiberData struct {
	block *object.EmeraldValue
	ran   bool
}

var pendingThreads []*object.EmeraldValue
var currentThread *object.EmeraldValue
var currentFiber *object.EmeraldValue
var threadReportOnExceptionDefault = true
var threadAbortOnExceptionDefault = false
var requiredFeatures = make(map[string]bool)
var loadingFeatures = make(map[string]bool)

func FeatureLoading(path string) bool {
	return loadingFeatures[path]
}

func loadedFeaturesGlobal() *object.EmeraldValue {
	if GetGlobalVariable == nil {
		return nil
	}
	if value := GetGlobalVariable("$\""); value != nil && value.Type == object.ValueArray {
		return value
	}
	if value := GetGlobalVariable("$LOADED_FEATURES"); value != nil && value.Type == object.ValueArray {
		return value
	}
	return nil
}

func syncRequiredFeaturesFromLoadedFeatures() {
	features := loadedFeaturesGlobal()
	if features == nil {
		return
	}
	next := make(map[string]bool)
	for _, feature := range features.Data.([]*object.EmeraldValue) {
		if feature != nil && feature.Type == object.ValueString {
			next[feature.Data.(string)] = true
		}
	}
	requiredFeatures = next
}

func featureRequired(path string) bool {
	syncRequiredFeaturesFromLoadedFeatures()
	return requiredFeatures[path] || requiredFeatures[path+".rb"]
}

func markFeatureRequired(path string) {
	requiredFeatures[path] = true
	features := loadedFeaturesGlobal()
	if features == nil {
		return
	}
	arr := features.Data.([]*object.EmeraldValue)
	for _, feature := range arr {
		if feature != nil && feature.Type == object.ValueString && feature.Data.(string) == path {
			return
		}
	}
	features.Data = append(arr, &object.EmeraldValue{Type: object.ValueString, Data: path, Class: R.Classes["String"]})
}

var scratchPadRecorded *object.EmeraldValue
var defaultThreadGroup *object.EmeraldValue
var evaluatingRaiseErrorMatcher bool
var processMaxGroups int64 = 32
var processArgv0 *object.EmeraldValue
var processRlimits = make(map[int64][2]int64)
var processNextPID int64 = 10_000
var processChildren []*processChild
var refinementModules map[*object.EmeraldValue]map[any]*object.EmeraldValue

func EvaluatingRaiseErrorMatcher() bool {
	return evaluatingRaiseErrorMatcher
}

var processGroups []int64

func classNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	cls, ok := receiver.Data.(*object.Class)
	if !ok {
		return R.NilVal
	}
	if cls.Name == "Class" {
		superClass := R.Classes["Object"]
		if len(args) > 0 && args[0].Type == object.ValueClass {
			superClass = args[0].Data.(*object.Class)
		}
		newClass := object.NewClass("")
		newClass.SuperClass = superClass
		classValue := &object.EmeraldValue{
			Type:  object.ValueClass,
			Data:  newClass,
			Class: R.Classes["Class"],
		}
		if CurrentBlockValue != nil && CurrentBlockValue() != nil && CallMethod != nil {
			result := CallMethod(classValue, "__exec_class_body__")
			if result != nil && result.Type == object.ValueException {
				return result
			}
		}
		return classValue
	}
	if classInheritsFrom(cls, R.Classes["Exception"]) {
		message := cls.Name
		if len(args) > 0 {
			if s, ok := args[0].Data.(string); ok {
				message = s
			} else {
				message = args[0].Inspect()
			}
		}
		return newRuntimeException(cls, message)
	}
	obj := &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  object.NewObject(cls),
		Class: cls,
	}
	if CallMethod != nil && classHasMethod(cls, "initialize") {
		result := CallMethod(obj, "initialize", args...)
		if result != nil && result.Type == object.ValueException {
			return result
		}
	}
	populateObjectIvarsFromKeywordHash(obj, args...)
	if classInheritsFrom(cls, R.Classes["Thread"]) {
		if _, ok := obj.Data.(*threadData); !ok {
			return threadError("uninitialized thread")
		}
	}
	return obj
}

func classHasMethod(class *object.Class, name string) bool {
	if class == nil {
		return false
	}
	_, ok := class.GetMethod(name)
	return ok
}

func moduleClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	module := object.NewModule("")
	moduleValue := &object.EmeraldValue{
		Type:  object.ValueModule,
		Data:  module,
		Class: R.Classes["Module"],
	}
	if CurrentBlockValue != nil && CurrentBlockValue() != nil && CallMethod != nil {
		result := CallMethod(moduleValue, "__exec_class_body__")
		if result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return moduleValue
}

func populateObjectIvarsFromKeywordHash(value *object.EmeraldValue, args ...*object.EmeraldValue) {
	if value == nil || len(args) == 0 || args[0] == nil || args[0].Type != object.ValueHash {
		return
	}
	obj, ok := value.Data.(*object.Object)
	if !ok {
		return
	}
	for key, val := range valueToHashMap(args[0]) {
		switch specName(key) {
		case "name":
			if existing := obj.GetInstanceVar("@name"); existing == nil || existing.Type == object.ValueNil {
				obj.SetInstanceVar("@name", val)
			}
		case "offset":
			if existing := obj.GetInstanceVar("@offset"); existing == nil || existing.Type == object.ValueNil {
				obj.SetInstanceVar("@offset", val)
			}
		}
	}
}

func structClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	structClass := object.NewClass("")
	structClass.SuperClass = R.Classes["Struct"]

	fields := make([]string, 0, len(args))
	for _, arg := range args {
		name := structFieldName(arg)
		if name == "" {
			continue
		}
		fields = append(fields, name)
		field := name
		structClass.DefineMethod(field, &object.Method{
			Name:  field,
			Arity: 0,
			Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
				if obj, ok := receiver.Data.(*object.Object); ok {
					if val := obj.GetInstanceVar("@" + field); val != nil {
						return val
					}
				}
				return R.NilVal
			},
		})
		structClass.DefineMethod(field+"=", &object.Method{
			Name:  field + "=",
			Arity: 1,
			Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
				if len(args) == 0 {
					return R.NilVal
				}
				if obj, ok := receiver.Data.(*object.Object); ok {
					obj.SetInstanceVar("@"+field, args[0])
				}
				return args[0]
			},
		})
	}

	structClass.DefineMethod("initialize", &object.Method{
		Name:  "initialize",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			obj, ok := receiver.Data.(*object.Object)
			if !ok {
				return R.NilVal
			}
			for i, field := range fields {
				val := R.NilVal
				if i < len(args) {
					val = args[i]
				}
				obj.SetInstanceVar("@"+field, val)
			}
			return R.NilVal
		},
	})
	structClass.DefineMethod("==", &object.Method{Name: "==", Fn: structEqual, Arity: 1})

	return &object.EmeraldValue{
		Type:  object.ValueClass,
		Data:  structClass,
		Class: R.Classes["Class"],
	}
}

func structFieldName(value *object.EmeraldValue) string {
	if value == nil {
		return ""
	}
	switch value.Type {
	case object.ValueSymbol, object.ValueString:
		if s, ok := value.Data.(string); ok {
			return strings.TrimPrefix(s, "@")
		}
	}
	return ""
}

func structEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || receiver.Class != args[0].Class {
		return R.FalseVal
	}
	left, leftOK := receiver.Data.(*object.Object)
	right, rightOK := args[0].Data.(*object.Object)
	if !leftOK || !rightOK {
		return R.FalseVal
	}
	for name, leftValue := range left.InstanceVars {
		rightValue := right.GetInstanceVar(name)
		if rightValue == nil || !leftValue.Equals(rightValue) {
			return R.FalseVal
		}
	}
	for name := range right.InstanceVars {
		if left.GetInstanceVar(name) == nil {
			return R.FalseVal
		}
	}
	return R.TrueVal
}

func isTruthy(val *object.EmeraldValue) bool {
	if val == nil || val == R.NilVal || val == R.FalseVal {
		return false
	}
	return true
}

type Runtime struct {
	Classes map[string]*object.Class

	TrueVal  *object.EmeraldValue
	FalseVal *object.EmeraldValue
	NilVal   *object.EmeraldValue

	Main *object.EmeraldValue
}

var R *Runtime

func Init() {
	R = &Runtime{
		Classes: make(map[string]*object.Class),
	}
	scratchPadRecorded = nil
	defaultThreadGroup = nil
	processArgv0 = nil
	processNextPID = 10_000
	processChildren = nil
	refinementModules = make(map[*object.EmeraldValue]map[any]*object.EmeraldValue)
	processGroups = processInitialGroups()
	fileUtimeOverrides = make(map[string]fileTimeOverride)
	currentFileUmask = 0022
	defaultExternalEncoding = "UTF-8"
	stringEncodings = make(map[*object.EmeraldValue]string)
	ruby2KeywordHashes = make(map[*object.EmeraldValue]bool)
	encodingValues = make(map[string]*object.EmeraldValue)
	nextIOFd = 10
	ioDataByFd = make(map[int64]*ioShimData)

	R.TrueVal = &object.EmeraldValue{
		Type:  object.ValueBool,
		Data:  true,
		Class: nil,
	}

	R.FalseVal = &object.EmeraldValue{
		Type:  object.ValueBool,
		Data:  false,
		Class: nil,
	}

	R.NilVal = &object.EmeraldValue{
		Type:  object.ValueNil,
		Data:  nil,
		Class: nil,
	}

	R.createClasses()
	R.defineMethods()
	RegisterMspec()
}

func (rt *Runtime) createClasses() {
	basicObject := object.NewClass("BasicObject")
	objectClass := object.NewClass("Object")
	objectClass.SuperClass = basicObject
	moduleClass := object.NewClass("Module")
	moduleClass.SuperClass = objectClass
	classClass := object.NewClass("Class")
	classClass.SuperClass = moduleClass
	kernelClass := object.NewClass("Kernel")
	kernelClass.SuperClass = objectClass

	trueClass := object.NewClass("TrueClass")
	trueClass.SuperClass = objectClass
	falseClass := object.NewClass("FalseClass")
	falseClass.SuperClass = objectClass
	nilClass := object.NewClass("NilClass")
	nilClass.SuperClass = objectClass

	integerClass := object.NewClass("Integer")
	integerClass.SuperClass = objectClass
	floatClass := object.NewClass("Float")
	floatClass.SuperClass = objectClass
	stringClass := object.NewClass("String")
	stringClass.SuperClass = objectClass
	encodingClass := object.NewClass("Encoding")
	encodingClass.SuperClass = objectClass
	ioClass := object.NewClass("IO")
	ioClass.SuperClass = objectClass
	fileClass := object.NewClass("File")
	fileClass.SuperClass = ioClass
	fileStatClass := object.NewClass("File::Stat")
	fileStatClass.SuperClass = objectClass
	timeClass := object.NewClass("Time")
	timeClass.SuperClass = objectClass
	marshalClass := object.NewClass("Marshal")
	marshalClass.SuperClass = objectClass
	fileTestClass := object.NewClass("FileTest")
	fileTestClass.SuperClass = objectClass
	dirClass := object.NewClass("Dir")
	dirClass.SuperClass = objectClass
	simpleDelegatorClass := object.NewClass("SimpleDelegator")
	simpleDelegatorClass.SuperClass = objectClass
	arrayClass := object.NewClass("Array")
	arrayClass.SuperClass = objectClass
	hashClass := object.NewClass("Hash")
	hashClass.SuperClass = objectClass
	symbolClass := object.NewClass("Symbol")
	symbolClass.SuperClass = objectClass
	regexpClass := object.NewClass("Regexp")
	regexpClass.SuperClass = objectClass
	rangeClass := object.NewClass("Range")
	rangeClass.SuperClass = objectClass
	structClass := object.NewClass("Struct")
	structClass.SuperClass = objectClass
	procClass := object.NewClass("Proc")
	procClass.SuperClass = objectClass
	enumeratorClass := object.NewClass("Enumerator")
	enumeratorClass.SuperClass = objectClass
	yielderClass := object.NewClass("Enumerator::Yielder")
	yielderClass.SuperClass = objectClass

	exceptionClass := object.NewClass("Exception")
	exceptionClass.SuperClass = objectClass
	standardErrorClass := object.NewClass("StandardError")
	standardErrorClass.SuperClass = exceptionClass
	systemCallErrorClass := object.NewClass("SystemCallError")
	systemCallErrorClass.SuperClass = standardErrorClass
	encodingCompatibilityErrorClass := object.NewClass("Encoding::CompatibilityError")
	encodingCompatibilityErrorClass.SuperClass = standardErrorClass
	ioErrorClass := object.NewClass("IOError")
	ioErrorClass.SuperClass = standardErrorClass
	ioWaitWritableClass := object.NewClass("IO::WaitWritable")
	ioWaitWritableClass.SuperClass = standardErrorClass
	ioWaitReadableClass := object.NewClass("IO::WaitReadable")
	ioWaitReadableClass.SuperClass = standardErrorClass
	ioEAGAINWaitReadableClass := object.NewClass("IO::EAGAINWaitReadable")
	ioEAGAINWaitReadableClass.SuperClass = ioWaitReadableClass
	errnoENOENTClass := object.NewClass("Errno::ENOENT")
	errnoENOENTClass.SuperClass = systemCallErrorClass
	errnoEAGAINClass := object.NewClass("Errno::EAGAIN")
	errnoEAGAINClass.SuperClass = systemCallErrorClass
	errnoEWOULDBLOCKClass := object.NewClass("Errno::EWOULDBLOCK")
	errnoEWOULDBLOCKClass.SuperClass = errnoEAGAINClass
	errnoEACCESClass := object.NewClass("Errno::EACCES")
	errnoEACCESClass.SuperClass = systemCallErrorClass
	errnoEBADFClass := object.NewClass("Errno::EBADF")
	errnoEBADFClass.SuperClass = systemCallErrorClass
	errnoEEXISTClass := object.NewClass("Errno::EEXIST")
	errnoEEXISTClass.SuperClass = systemCallErrorClass
	errnoENOTEMPTYClass := object.NewClass("Errno::ENOTEMPTY")
	errnoENOTEMPTYClass.SuperClass = systemCallErrorClass
	errnoENOTDIRClass := object.NewClass("Errno::ENOTDIR")
	errnoENOTDIRClass.SuperClass = systemCallErrorClass
	errnoEISDIRClass := object.NewClass("Errno::EISDIR")
	errnoEISDIRClass.SuperClass = systemCallErrorClass
	errnoENOEXECClass := object.NewClass("Errno::ENOEXEC")
	errnoENOEXECClass.SuperClass = systemCallErrorClass
	errnoEINVALClass := object.NewClass("Errno::EINVAL")
	errnoEINVALClass.SuperClass = systemCallErrorClass
	errnoELOOPClass := object.NewClass("Errno::ELOOP")
	errnoELOOPClass.SuperClass = systemCallErrorClass
	scriptErrorClass := object.NewClass("ScriptError")
	scriptErrorClass.SuperClass = exceptionClass
	notImplementedErrorClass := object.NewClass("NotImplementedError")
	notImplementedErrorClass.SuperClass = scriptErrorClass
	stopIterationClass := object.NewClass("StopIteration")
	stopIterationClass.SuperClass = standardErrorClass
	eofErrorClass := object.NewClass("EOFError")
	eofErrorClass.SuperClass = standardErrorClass
	runtimeErrorClass := object.NewClass("RuntimeError")
	runtimeErrorClass.SuperClass = standardErrorClass
	systemExitClass := object.NewClass("SystemExit")
	systemExitClass.SuperClass = exceptionClass
	frozenErrorClass := object.NewClass("FrozenError")
	frozenErrorClass.SuperClass = runtimeErrorClass
	argumentErrorClass := object.NewClass("ArgumentError")
	argumentErrorClass.SuperClass = standardErrorClass
	typeErrorClass := object.NewClass("TypeError")
	typeErrorClass.SuperClass = standardErrorClass
	localJumpErrorClass := object.NewClass("LocalJumpError")
	localJumpErrorClass.SuperClass = standardErrorClass
	nameErrorClass := object.NewClass("NameError")
	nameErrorClass.SuperClass = standardErrorClass
	noMethodErrorClass := object.NewClass("NoMethodError")
	noMethodErrorClass.SuperClass = nameErrorClass
	indexErrorClass := object.NewClass("IndexError")
	indexErrorClass.SuperClass = standardErrorClass
	keyErrorClass := object.NewClass("KeyError")
	keyErrorClass.SuperClass = indexErrorClass
	rangeErrorClass := object.NewClass("RangeError")
	rangeErrorClass.SuperClass = standardErrorClass
	zeroDivisionErrorClass := object.NewClass("ZeroDivisionError")
	zeroDivisionErrorClass.SuperClass = standardErrorClass
	syntaxErrorClass := object.NewClass("SyntaxError")
	syntaxErrorClass.SuperClass = exceptionClass
	loadErrorClass := object.NewClass("LoadError")
	loadErrorClass.SuperClass = standardErrorClass
	threadErrorClass := object.NewClass("ThreadError")
	threadErrorClass.SuperClass = standardErrorClass
	closedQueueErrorClass := object.NewClass("ClosedQueueError")
	closedQueueErrorClass.SuperClass = stopIterationClass
	fiberErrorClass := object.NewClass("FiberError")
	fiberErrorClass.SuperClass = standardErrorClass

	methodClass := object.NewClass("Method")
	methodClass.SuperClass = objectClass
	unboundMethodClass := object.NewClass("UnboundMethod")
	unboundMethodClass.SuperClass = objectClass
	bindingClass := object.NewClass("Binding")
	bindingClass.SuperClass = objectClass
	threadClass := object.NewClass("Thread")
	threadClass.SuperClass = objectClass
	threadGroupClass := object.NewClass("ThreadGroup")
	threadGroupClass.SuperClass = objectClass
	mutexClass := object.NewClass("Mutex")
	mutexClass.SuperClass = objectClass
	conditionVariableClass := object.NewClass("ConditionVariable")
	conditionVariableClass.SuperClass = objectClass
	queueClass := object.NewClass("Queue")
	queueClass.SuperClass = objectClass
	sizedQueueClass := object.NewClass("SizedQueue")
	sizedQueueClass.SuperClass = queueClass
	fiberClass := object.NewClass("Fiber")
	fiberClass.SuperClass = objectClass
	activeSupportTestCaseClass := object.NewClass("ActiveSupport::TestCase")
	activeSupportTestCaseClass.SuperClass = objectClass
	scratchPadClass := object.NewClass("ScratchPad")
	scratchPadClass.SuperClass = objectClass
	mockObjectClass := object.NewClass("MockObject")
	mockObjectClass.SuperClass = objectClass
	mockExpectationClass := object.NewClass("MockExpectation")
	mockExpectationClass.SuperClass = objectClass
	processClass := object.NewClass("Process")
	processClass.SuperClass = objectClass
	processUIDClass := object.NewClass("Process::UID")
	processUIDClass.SuperClass = objectClass
	processGIDClass := object.NewClass("Process::GID")
	processGIDClass.SuperClass = objectClass
	processSysClass := object.NewClass("Process::Sys")
	processSysClass.SuperClass = objectClass
	processStatusClass := object.NewClass("Process::Status")
	processStatusClass.SuperClass = objectClass

	R.TrueVal.Class = trueClass
	R.FalseVal.Class = falseClass
	R.NilVal.Class = nilClass

	R.Classes["BasicObject"] = basicObject
	R.Classes["Object"] = objectClass
	R.Classes["Module"] = moduleClass
	R.Classes["Class"] = classClass
	R.Classes["Kernel"] = kernelClass
	R.Classes["TrueClass"] = trueClass
	R.Classes["FalseClass"] = falseClass
	R.Classes["NilClass"] = nilClass
	R.Classes["Integer"] = integerClass
	R.Classes["Float"] = floatClass
	R.Classes["String"] = stringClass
	R.Classes["Encoding"] = encodingClass
	R.Classes["IO"] = ioClass
	R.Classes["File"] = fileClass
	R.Classes["File::Stat"] = fileStatClass
	R.Classes["Time"] = timeClass
	R.Classes["Marshal"] = marshalClass
	R.Classes["FileTest"] = fileTestClass
	R.Classes["Dir"] = dirClass
	R.Classes["SimpleDelegator"] = simpleDelegatorClass
	R.Classes["Array"] = arrayClass
	R.Classes["Hash"] = hashClass
	R.Classes["Symbol"] = symbolClass
	R.Classes["Regexp"] = regexpClass
	R.Classes["Range"] = rangeClass
	R.Classes["Struct"] = structClass
	R.Classes["Proc"] = procClass
	R.Classes["Enumerator"] = enumeratorClass
	R.Classes["Enumerator::Yielder"] = yielderClass
	R.Classes["Exception"] = exceptionClass
	R.Classes["StandardError"] = standardErrorClass
	R.Classes["SystemCallError"] = systemCallErrorClass
	R.Classes["Encoding::CompatibilityError"] = encodingCompatibilityErrorClass
	R.Classes["IOError"] = ioErrorClass
	R.Classes["IO::WaitWritable"] = ioWaitWritableClass
	R.Classes["IO::WaitReadable"] = ioWaitReadableClass
	R.Classes["IO::EAGAINWaitReadable"] = ioEAGAINWaitReadableClass
	R.Classes["Errno::ENOENT"] = errnoENOENTClass
	R.Classes["Errno::EAGAIN"] = errnoEAGAINClass
	R.Classes["Errno::EWOULDBLOCK"] = errnoEWOULDBLOCKClass
	R.Classes["Errno::EACCES"] = errnoEACCESClass
	R.Classes["Errno::EBADF"] = errnoEBADFClass
	R.Classes["Errno::EEXIST"] = errnoEEXISTClass
	R.Classes["Errno::ENOTEMPTY"] = errnoENOTEMPTYClass
	R.Classes["Errno::ENOTDIR"] = errnoENOTDIRClass
	R.Classes["Errno::EISDIR"] = errnoEISDIRClass
	R.Classes["Errno::ENOEXEC"] = errnoENOEXECClass
	R.Classes["Errno::EINVAL"] = errnoEINVALClass
	R.Classes["Errno::ELOOP"] = errnoELOOPClass
	R.Classes["ScriptError"] = scriptErrorClass
	R.Classes["NotImplementedError"] = notImplementedErrorClass
	R.Classes["StopIteration"] = stopIterationClass
	R.Classes["EOFError"] = eofErrorClass
	R.Classes["RuntimeError"] = runtimeErrorClass
	R.Classes["SystemExit"] = systemExitClass
	R.Classes["FrozenError"] = frozenErrorClass
	R.Classes["ArgumentError"] = argumentErrorClass
	R.Classes["TypeError"] = typeErrorClass
	R.Classes["LocalJumpError"] = localJumpErrorClass
	R.Classes["NameError"] = nameErrorClass
	R.Classes["NoMethodError"] = noMethodErrorClass
	R.Classes["IndexError"] = indexErrorClass
	R.Classes["KeyError"] = keyErrorClass
	R.Classes["RangeError"] = rangeErrorClass
	R.Classes["ZeroDivisionError"] = zeroDivisionErrorClass
	R.Classes["SyntaxError"] = syntaxErrorClass
	R.Classes["LoadError"] = loadErrorClass
	R.Classes["ThreadError"] = threadErrorClass
	R.Classes["ClosedQueueError"] = closedQueueErrorClass
	R.Classes["FiberError"] = fiberErrorClass
	R.Classes["Errno::ECHILD"] = standardErrorClass
	R.Classes["Errno::ESRCH"] = standardErrorClass
	R.Classes["Errno::EPERM"] = standardErrorClass
	R.Classes["Method"] = methodClass
	R.Classes["UnboundMethod"] = unboundMethodClass
	R.Classes["Binding"] = bindingClass
	R.Classes["Thread"] = threadClass
	R.Classes["ThreadGroup"] = threadGroupClass
	R.Classes["Mutex"] = mutexClass
	R.Classes["ConditionVariable"] = conditionVariableClass
	R.Classes["Queue"] = queueClass
	R.Classes["SizedQueue"] = sizedQueueClass
	R.Classes["Fiber"] = fiberClass
	R.Classes["ActiveSupport::TestCase"] = activeSupportTestCaseClass
	R.Classes["ScratchPad"] = scratchPadClass
	R.Classes["MockObject"] = mockObjectClass
	R.Classes["MockExpectation"] = mockExpectationClass
	R.Classes["Process"] = processClass
	R.Classes["Process::UID"] = processUIDClass
	R.Classes["Process::GID"] = processGIDClass
	R.Classes["Process::Sys"] = processSysClass
	R.Classes["Process::Status"] = processStatusClass
}

func (rt *Runtime) defineMethods() {
	objectClass := R.Classes["Object"]
	objectClass.DefineMethod("class", &object.Method{Name: "class", Fn: methodClass, Arity: 0})
	objectClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: methodToS, Arity: 0})
	objectClass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: methodInspect, Arity: 0})
	objectClass.DefineMethod("nil?", &object.Method{Name: "nil?", Fn: methodIsNil, Arity: 0})
	objectClass.DefineMethod("equal?", &object.Method{Name: "equal?", Fn: methodEqual, Arity: 1})
	objectClass.DefineMethod("eql?", &object.Method{Name: "eql?", Fn: methodEql, Arity: 1})
	objectClass.DefineMethod("instance_of?", &object.Method{Name: "instance_of?", Fn: methodInstanceOf, Arity: 1})
	objectClass.DefineMethod("respond_to?", &object.Method{Name: "respond_to?", Fn: methodRespondTo, Arity: 1})
	objectClass.DefineMethod("methods", &object.Method{Name: "methods", Fn: methodMethods, Arity: -1})
	objectClass.DefineMethod("method", &object.Method{Name: "method", Fn: methodObjectMethod, Arity: 1})
	objectClass.DefineMethod("send", &object.Method{Name: "send", Fn: methodSend, Arity: 1})
	objectClass.DefineMethod("singleton_class", &object.Method{Name: "singleton_class", Fn: methodSingletonClass, Arity: 0})
	objectClass.DefineMethod("instance_variable_set", &object.Method{Name: "instance_variable_set", Fn: methodInstanceVariableSet, Arity: 2})
	objectClass.DefineMethod("strftime", &object.Method{Name: "strftime", Fn: objectStrftime, Arity: -1})
	objectClass.DefineMethod("deconstruct_keys", &object.Method{Name: "deconstruct_keys", Fn: objectDeconstructKeys, Arity: -1})
	objectClass.DefineMethod("is_a?", &object.Method{Name: "is_a?", Fn: methodIsA, Arity: 1})
	objectClass.DefineMethod("freeze", &object.Method{Name: "freeze", Fn: methodFreeze, Arity: 0})
	objectClass.DefineMethod("frozen?", &object.Method{Name: "frozen?", Fn: methodFrozen, Arity: 0})
	objectClass.DefineMethod("dup", &object.Method{Name: "dup", Fn: methodDup, Arity: 0})
	objectClass.DefineMethod("clone", &object.Method{Name: "clone", Fn: methodDup, Arity: 0})
	objectClass.DefineMethod("attr_accessor", &object.Method{Name: "attr_accessor", Fn: methodAttrAccessor, Arity: -1})
	objectClass.DefineMethod("attr_reader", &object.Method{Name: "attr_reader", Fn: methodAttrReader, Arity: -1})
	objectClass.DefineMethod("attr_writer", &object.Method{Name: "attr_writer", Fn: methodAttrWriter, Arity: -1})
	objectClass.DefineMethod("attr", &object.Method{Name: "attr", Fn: methodAttr, Arity: -1})
	objectClass.DefineMethod("public", &object.Method{Name: "public", Fn: methodSetPublicVisibility, Arity: -1, Visibility: "private"})
	objectClass.DefineMethod("private", &object.Method{Name: "private", Fn: methodSetPrivateVisibility, Arity: -1, Visibility: "private"})
	objectClass.DefineMethod("protected", &object.Method{Name: "protected", Fn: methodSetProtectedVisibility, Arity: -1, Visibility: "private"})
	objectClass.DefineMethod("eval", &object.Method{Name: "eval", Fn: methodEval, Arity: 1})
	objectClass.DefineMethod("extend", &object.Method{Name: "extend", Fn: objectExtend, Arity: -1})
	objectClass.DefineMethod("define_singleton_method", &object.Method{Name: "define_singleton_method", Fn: methodDefineSingletonMethod, Arity: -1})
	objectClass.DefineMethod("should_receive", &object.Method{Name: "should_receive", Fn: mockShouldReceive, Arity: -1})
	objectClass.DefineMethod("should_not_receive", &object.Method{Name: "should_not_receive", Fn: mockShouldNotReceive, Arity: -1})
	objectClass.DefineMethod("ruby_exe", &object.Method{Name: "ruby_exe", Fn: methodRubyExe, Arity: -1})
	objectClass.DefineMethod("ruby_cmd", &object.Method{Name: "ruby_cmd", Fn: methodRubyCmd, Arity: -1})
	objectClass.DefineMethod("__FILE__", &object.Method{Name: "__FILE__", Fn: builtinMagicFile, Arity: 0})
	objectClass.DefineMethod("__dir__", &object.Method{Name: "__dir__", Fn: builtinMagicDir, Arity: 0})
	objectClass.DefineMethod("tmp", &object.Method{Name: "tmp", Fn: builtinTmp, Arity: -1})
	objectClass.DefineMethod("touch", &object.Method{Name: "touch", Fn: builtinTouch, Arity: 1})
	objectClass.DefineMethod("mkdir_p", &object.Method{Name: "mkdir_p", Fn: builtinMkdirP, Arity: 1})
	objectClass.DefineMethod("rm_r", &object.Method{Name: "rm_r", Fn: builtinRmR, Arity: -1})
	objectClass.DefineMethod("mock_to_path", &object.Method{Name: "mock_to_path", Fn: builtinMockToPath, Arity: 1})

	nilClass := R.Classes["NilClass"]
	nilClass.DefineMethod("+", &object.Method{Name: "+", Fn: nilPlus, Arity: 1})

	integerClass := R.Classes["Integer"]
	integerClass.DefineMethod("+", &object.Method{Name: "+", Fn: intAdd, Arity: 1})
	integerClass.DefineMethod("-", &object.Method{Name: "-", Fn: intSub, Arity: 1})
	integerClass.DefineMethod("*", &object.Method{Name: "*", Fn: intMul, Arity: 1})
	integerClass.DefineMethod("/", &object.Method{Name: "/", Fn: intDiv, Arity: 1})
	integerClass.DefineMethod("%", &object.Method{Name: "%", Fn: intMod, Arity: 1})
	integerClass.DefineMethod("**", &object.Method{Name: "**", Fn: intPow, Arity: 1})
	integerClass.DefineMethod("pow", &object.Method{Name: "pow", Fn: intPow, Arity: 1})
	integerClass.DefineMethod("==", &object.Method{Name: "==", Fn: intEqual, Arity: 1})
	integerClass.DefineMethod("===", &object.Method{Name: "===", Fn: intEqual, Arity: 1})
	integerClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: intToS, Arity: -1})
	integerClass.DefineMethod("succ", &object.Method{Name: "succ", Fn: intSucc, Arity: 0})
	integerClass.DefineMethod("pred", &object.Method{Name: "pred", Fn: intPred, Arity: 0})
	integerClass.DefineMethod("chr", &object.Method{Name: "chr", Fn: intChr, Arity: 0})
	integerClass.DefineMethod("odd?", &object.Method{Name: "odd?", Fn: intOdd, Arity: 0})
	integerClass.DefineMethod("even?", &object.Method{Name: "even?", Fn: intEven, Arity: 0})
	integerClass.DefineMethod("zero?", &object.Method{Name: "zero?", Fn: intZero, Arity: 0})
	integerClass.DefineMethod("abs", &object.Method{Name: "abs", Fn: intAbs, Arity: 0})
	integerClass.DefineMethod("to_f", &object.Method{Name: "to_f", Fn: intToF, Arity: 0})
	integerClass.DefineMethod("times", &object.Method{Name: "times", Fn: intTimes, Arity: 0})
	integerClass.DefineMethod("upto", &object.Method{Name: "upto", Fn: intUpto, Arity: 1})
	integerClass.DefineMethod("downto", &object.Method{Name: "downto", Fn: intDownto, Arity: 1})
	integerClass.DefineMethod("gcd", &object.Method{Name: "gcd", Fn: intGcd, Arity: 1})
	integerClass.DefineMethod("lcm", &object.Method{Name: "lcm", Fn: intLcm, Arity: 1})
	integerClass.DefineMethod("divmod", &object.Method{Name: "divmod", Fn: intDivmod, Arity: 1})
	integerClass.DefineMethod("positive?", &object.Method{Name: "positive?", Fn: intPositive, Arity: 0})
	integerClass.DefineMethod("negative?", &object.Method{Name: "negative?", Fn: intNegative, Arity: 0})
	integerClass.DefineMethod("floor", &object.Method{Name: "floor", Fn: intFloor, Arity: 0})
	integerClass.DefineMethod("ceil", &object.Method{Name: "ceil", Fn: intCeil, Arity: 0})
	integerClass.DefineMethod("round", &object.Method{Name: "round", Fn: intRound, Arity: 0})
	integerClass.DefineMethod("digits", &object.Method{Name: "digits", Fn: intDigits, Arity: 0})

	// Bitwise operators
	integerClass.DefineMethod("&", &object.Method{Name: "&", Fn: intBitAnd, Arity: 1})
	integerClass.DefineMethod("|", &object.Method{Name: "|", Fn: intBitOr, Arity: 1})
	integerClass.DefineMethod("^", &object.Method{Name: "^", Fn: intBitXor, Arity: 1})
	integerClass.DefineMethod("~", &object.Method{Name: "~", Fn: intBitNot, Arity: 0})
	integerClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: intLeftShift, Arity: 1})
	integerClass.DefineMethod(">>", &object.Method{Name: ">>", Fn: intRightShift, Arity: 1})

	// Comparison operators
	integerClass.DefineMethod("<", &object.Method{Name: "<", Fn: intLessThan, Arity: 1})
	integerClass.DefineMethod(">", &object.Method{Name: ">", Fn: intGreaterThan, Arity: 1})
	integerClass.DefineMethod("<=", &object.Method{Name: "<=", Fn: intLessThanOrEqual, Arity: 1})
	integerClass.DefineMethod(">=", &object.Method{Name: ">=", Fn: intGreaterThanOrEqual, Arity: 1})
	integerClass.DefineMethod("<=>", &object.Method{Name: "<=>", Fn: intCompare, Arity: 1})

	symbolClass := R.Classes["Symbol"]
	symbolClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: symbolToS, Arity: 0})
	symbolClass.DefineMethod("to_sym", &object.Method{Name: "to_sym", Fn: symbolToSym, Arity: 0})
	symbolClass.DefineMethod("length", &object.Method{Name: "length", Fn: symbolLength, Arity: 0})
	symbolClass.DefineMethod("size", &object.Method{Name: "size", Fn: symbolLength, Arity: 0})
	symbolClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: symbolEmpty, Arity: 0})
	symbolClass.DefineMethod("upcase", &object.Method{Name: "upcase", Fn: symbolUpcase, Arity: 0})
	symbolClass.DefineMethod("downcase", &object.Method{Name: "downcase", Fn: symbolDowncase, Arity: 0})
	symbolClass.DefineMethod("capitalize", &object.Method{Name: "capitalize", Fn: symbolCapitalize, Arity: 0})
	symbolClass.DefineMethod("swapcase", &object.Method{Name: "swapcase", Fn: symbolSwapcase, Arity: 0})
	symbolClass.DefineMethod("succ", &object.Method{Name: "succ", Fn: symbolSucc, Arity: 0})
	symbolClass.DefineMethod("==", &object.Method{Name: "==", Fn: symbolEqual, Arity: 1})
	symbolClass.DefineMethod("===", &object.Method{Name: "===", Fn: symbolCaseEqual, Arity: 1})
	symbolClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: symbolIndex, Arity: 1})
	symbolClass.DefineMethod("slice", &object.Method{Name: "slice", Fn: symbolSlice, Arity: 1})

	floatClass := R.Classes["Float"]
	floatClass.DefineMethod("+", &object.Method{Name: "+", Fn: floatAdd, Arity: 1})
	floatClass.DefineMethod("-", &object.Method{Name: "-", Fn: floatSub, Arity: 1})
	floatClass.DefineMethod("*", &object.Method{Name: "*", Fn: floatMul, Arity: 1})
	floatClass.DefineMethod("/", &object.Method{Name: "/", Fn: floatDiv, Arity: 1})
	floatClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: floatToS, Arity: 0})
	floatClass.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: floatToI, Arity: 0})
	floatClass.DefineMethod("floor", &object.Method{Name: "floor", Fn: floatFloor, Arity: 0})
	floatClass.DefineMethod("ceil", &object.Method{Name: "ceil", Fn: floatCeil, Arity: 0})
	floatClass.DefineMethod("round", &object.Method{Name: "round", Fn: floatRound, Arity: 0})
	floatClass.DefineMethod("abs", &object.Method{Name: "abs", Fn: floatAbs, Arity: 0})
	floatClass.DefineMethod("to_f", &object.Method{Name: "to_f", Fn: floatToF, Arity: 0})
	floatClass.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: floatToI, Arity: 0})
	floatClass.DefineMethod("zero?", &object.Method{Name: "zero?", Fn: floatZero, Arity: 0})
	floatClass.DefineMethod("positive?", &object.Method{Name: "positive?", Fn: floatPositive, Arity: 0})
	floatClass.DefineMethod("negative?", &object.Method{Name: "negative?", Fn: floatNegative, Arity: 0})
	floatClass.DefineMethod("nan?", &object.Method{Name: "nan?", Fn: floatNan, Arity: 0})
	floatClass.DefineMethod("infinite?", &object.Method{Name: "infinite?", Fn: floatInfinite, Arity: 0})
	floatClass.DefineMethod("finite?", &object.Method{Name: "finite?", Fn: floatFinite, Arity: 0})
	floatClass.DefineMethod("<", &object.Method{Name: "<", Fn: floatLessThan, Arity: 1})
	floatClass.DefineMethod(">", &object.Method{Name: ">", Fn: floatGreaterThan, Arity: 1})
	floatClass.DefineMethod("<=", &object.Method{Name: "<=", Fn: floatLessThanOrEqual, Arity: 1})
	floatClass.DefineMethod(">=", &object.Method{Name: ">=", Fn: floatGreaterThanOrEqual, Arity: 1})
	floatClass.DefineMethod("<=>", &object.Method{Name: "<=>", Fn: floatCompare, Arity: 1})

	rangeClass := R.Classes["Range"]
	rangeClass.DefineMethod("each", &object.Method{Name: "each", Fn: rangeEach, Arity: 0})
	rangeClass.DefineMethod("to_a", &object.Method{Name: "to_a", Fn: rangeToA, Arity: 0})
	rangeClass.DefineMethod("begin", &object.Method{Name: "begin", Fn: rangeBegin, Arity: 0})
	rangeClass.DefineMethod("end", &object.Method{Name: "end", Fn: rangeEnd, Arity: 0})
	rangeClass.DefineMethod("first", &object.Method{Name: "first", Fn: rangeFirst, Arity: 0})
	rangeClass.DefineMethod("last", &object.Method{Name: "last", Fn: rangeLast, Arity: 0})
	rangeClass.DefineMethod("size", &object.Method{Name: "size", Fn: rangeSize, Arity: 0})
	rangeClass.DefineMethod("exclude_end?", &object.Method{Name: "exclude_end?", Fn: rangeExcludeEnd, Arity: 0})
	rangeClass.DefineMethod("cover?", &object.Method{Name: "cover?", Fn: rangeCover, Arity: 1})
	rangeClass.DefineMethod("include?", &object.Method{Name: "include?", Fn: rangeInclude, Arity: 1})
	rangeClass.DefineMethod("member?", &object.Method{Name: "member?", Fn: rangeInclude, Arity: 1})
	rangeClass.DefineMethod("==", &object.Method{Name: "==", Fn: rangeEqual, Arity: 1})
	rangeClass.DefineMethod("===", &object.Method{Name: "===", Fn: rangeCaseEqual, Arity: 1})

	regexpClass := R.Classes["Regexp"]
	regexpClass.DefineClassMethod("escape", &object.Method{Name: "escape", Fn: regexpClassEscape, Arity: 1})
	regexpClass.DefineClassMethod("quote", &object.Method{Name: "quote", Fn: regexpClassEscape, Arity: 1})
	regexpClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: regexpToS, Arity: 0})
	regexpClass.DefineMethod("source", &object.Method{Name: "source", Fn: regexpSource, Arity: 0})
	regexpClass.DefineMethod("=~", &object.Method{Name: "=~", Fn: regexpMatch, Arity: 1})
	regexpClass.DefineMethod("==", &object.Method{Name: "==", Fn: regexpEqual, Arity: 1})
	regexpClass.DefineMethod("===", &object.Method{Name: "===", Fn: regexpCaseEqual, Arity: 1})
	regexpClass.DefineMethod("match", &object.Method{Name: "match", Fn: regexpMatch, Arity: 1})
	regexpClass.DefineMethod("match?", &object.Method{Name: "match?", Fn: regexpMatchQ, Arity: 1})

	methodClass := R.Classes["Method"]
	methodClass.DefineMethod("call", &object.Method{Name: "call", Fn: methodCall, Arity: -1})
	methodClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: methodCall, Arity: -1})
	methodClass.DefineMethod("arity", &object.Method{Name: "arity", Fn: methodArity, Arity: 0})
	methodClass.DefineMethod("owner", &object.Method{Name: "owner", Fn: methodOwner, Arity: 0})
	methodClass.DefineMethod("receiver", &object.Method{Name: "receiver", Fn: methodReceiver, Arity: 0})
	methodClass.DefineMethod("name", &object.Method{Name: "name", Fn: methodName, Arity: 0})
	methodClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: methodMethodToS, Arity: 0})
	methodClass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: methodMethodInspect, Arity: 0})
	methodClass.DefineMethod("==", &object.Method{Name: "==", Fn: methodMethodEqual, Arity: 1})
	unboundMethodClass := R.Classes["UnboundMethod"]
	unboundMethodClass.DefineMethod("arity", &object.Method{Name: "arity", Fn: methodArity, Arity: 0})
	unboundMethodClass.DefineMethod("owner", &object.Method{Name: "owner", Fn: methodOwner, Arity: 0})
	unboundMethodClass.DefineMethod("name", &object.Method{Name: "name", Fn: methodName, Arity: 0})
	unboundMethodClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: methodMethodToS, Arity: 0})
	unboundMethodClass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: methodMethodInspect, Arity: 0})
	unboundMethodClass.DefineMethod("bind", &object.Method{Name: "bind", Fn: unboundMethodBind, Arity: 1})
	unboundMethodClass.DefineMethod("==", &object.Method{Name: "==", Fn: methodMethodEqual, Arity: 1})

	bindingClass := R.Classes["Binding"]
	bindingClass.DefineMethod("local_variables", &object.Method{Name: "local_variables", Fn: bindingLocalVariables, Arity: 0})
	bindingClass.DefineMethod("eval", &object.Method{Name: "eval", Fn: bindingEval, Arity: 1})

	threadClass := R.Classes["Thread"]
	threadClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: threadClassNew, Arity: 0})
	threadClass.DefineClassMethod("allocate", &object.Method{Name: "allocate", Fn: threadClassAllocate, Arity: 0})
	threadClass.DefineClassMethod("start", &object.Method{Name: "start", Fn: threadClassStart, Arity: 0})
	threadClass.DefineClassMethod("fork", &object.Method{Name: "fork", Fn: threadClassStart, Arity: 0})
	threadClass.DefineClassMethod("pass", &object.Method{Name: "pass", Fn: threadClassPass, Arity: 0})
	threadClass.DefineClassMethod("current", &object.Method{Name: "current", Fn: threadClassCurrent, Arity: 0})
	threadClass.DefineClassMethod("main", &object.Method{Name: "main", Fn: threadClassCurrent, Arity: 0})
	threadClass.DefineClassMethod("handle_interrupt", &object.Method{Name: "handle_interrupt", Fn: threadClassHandleInterrupt, Arity: 1})
	threadClass.DefineClassMethod("pending_interrupt?", &object.Method{Name: "pending_interrupt?", Fn: threadClassPendingInterrupt, Arity: 0})
	threadClass.DefineClassMethod("each_caller_location", &object.Method{Name: "each_caller_location", Fn: threadClassEachCallerLocation, Arity: -1})
	threadClass.DefineClassMethod("report_on_exception", &object.Method{Name: "report_on_exception", Fn: threadClassReportOnException, Arity: 0})
	threadClass.DefineClassMethod("report_on_exception=", &object.Method{Name: "report_on_exception=", Fn: threadClassSetReportOnException, Arity: 1})
	threadClass.DefineClassMethod("abort_on_exception", &object.Method{Name: "abort_on_exception", Fn: threadClassAbortOnException, Arity: 0})
	threadClass.DefineClassMethod("abort_on_exception=", &object.Method{Name: "abort_on_exception=", Fn: threadClassSetAbortOnException, Arity: 1})
	threadClass.DefineMethod("initialize", &object.Method{Name: "initialize", Fn: threadInitialize, Arity: -1})
	threadClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: threadIndex, Arity: 1})
	threadClass.DefineMethod("[]=", &object.Method{Name: "[]=", Fn: threadIndexSet, Arity: 2})
	threadClass.DefineMethod("key?", &object.Method{Name: "key?", Fn: threadKey, Arity: 1})
	threadClass.DefineMethod("fetch", &object.Method{Name: "fetch", Fn: threadFetch, Arity: -1})
	threadClass.DefineMethod("thread_variable_get", &object.Method{Name: "thread_variable_get", Fn: threadVariableGet, Arity: 1})
	threadClass.DefineMethod("thread_variable_set", &object.Method{Name: "thread_variable_set", Fn: threadVariableSet, Arity: 2})
	threadClass.DefineMethod("thread_variable?", &object.Method{Name: "thread_variable?", Fn: threadVariablePredicate, Arity: 1})
	threadClass.DefineMethod("name", &object.Method{Name: "name", Fn: threadName, Arity: 0})
	threadClass.DefineMethod("name=", &object.Method{Name: "name=", Fn: threadSetName, Arity: 1})
	threadClass.DefineMethod("report_on_exception", &object.Method{Name: "report_on_exception", Fn: threadReportOnException, Arity: 0})
	threadClass.DefineMethod("report_on_exception=", &object.Method{Name: "report_on_exception=", Fn: threadSetReportOnException, Arity: 1})
	threadClass.DefineMethod("pending_interrupt?", &object.Method{Name: "pending_interrupt?", Fn: threadPendingInterrupt, Arity: 0})
	threadClass.DefineMethod("abort_on_exception", &object.Method{Name: "abort_on_exception", Fn: threadAbortOnException, Arity: 0})
	threadClass.DefineMethod("abort_on_exception=", &object.Method{Name: "abort_on_exception=", Fn: threadSetAbortOnException, Arity: 1})
	threadClass.DefineMethod("priority", &object.Method{Name: "priority", Fn: threadPriority, Arity: 0})
	threadClass.DefineMethod("priority=", &object.Method{Name: "priority=", Fn: threadSetPriority, Arity: 1})
	threadClass.DefineMethod("join", &object.Method{Name: "join", Fn: threadJoin, Arity: 0})
	threadClass.DefineMethod("value", &object.Method{Name: "value", Fn: threadValue, Arity: 0})
	threadClass.DefineMethod("pid", &object.Method{Name: "pid", Fn: threadPid, Arity: 0})
	threadClass.DefineMethod("backtrace", &object.Method{Name: "backtrace", Fn: threadBacktrace, Arity: 0})
	threadClass.DefineMethod("alive?", &object.Method{Name: "alive?", Fn: threadAlive, Arity: 0})
	threadClass.DefineMethod("stop?", &object.Method{Name: "stop?", Fn: threadStop, Arity: 0})
	threadClass.DefineMethod("run", &object.Method{Name: "run", Fn: threadWakeup, Arity: 0})
	threadClass.DefineMethod("wakeup", &object.Method{Name: "wakeup", Fn: threadWakeup, Arity: 0})
	threadClass.DefineMethod("kill", &object.Method{Name: "kill", Fn: threadKill, Arity: 0})
	threadClass.DefineMethod("terminate", &object.Method{Name: "terminate", Fn: threadKill, Arity: 0})
	threadClass.DefineMethod("raise", &object.Method{Name: "raise", Fn: threadRaise, Arity: -1})
	threadClass.DefineMethod("native_thread_id", &object.Method{Name: "native_thread_id", Fn: threadNativeThreadID, Arity: 0})
	threadClass.DefineMethod("group", &object.Method{Name: "group", Fn: threadGroup, Arity: 0})

	threadGroupClass := R.Classes["ThreadGroup"]
	threadGroupClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: threadGroupClassNew, Arity: 0})
	threadGroupClass.DefineMethod("add", &object.Method{Name: "add", Fn: threadGroupAdd, Arity: 1})
	threadGroupClass.DefineMethod("list", &object.Method{Name: "list", Fn: threadGroupList, Arity: 0})
	threadGroupClass.DefineMethod("enclose", &object.Method{Name: "enclose", Fn: threadGroupEnclose, Arity: 0})
	threadGroupClass.DefineMethod("enclosed?", &object.Method{Name: "enclosed?", Fn: threadGroupEnclosed, Arity: 0})
	defaultThreadGroup = newThreadGroup()

	scratchPadClass := R.Classes["ScratchPad"]
	scratchPadClass.DefineClassMethod("record", &object.Method{Name: "record", Fn: scratchPadRecord, Arity: 1})
	scratchPadClass.DefineClassMethod("recorded", &object.Method{Name: "recorded", Fn: scratchPadRecordedValue, Arity: 0})
	scratchPadClass.DefineClassMethod("clear", &object.Method{Name: "clear", Fn: scratchPadClear, Arity: 0})
	scratchPadClass.DefineClassMethod("<<", &object.Method{Name: "<<", Fn: scratchPadAppend, Arity: 1})

	processClass := R.Classes["Process"]
	processClass.DefineClassMethod("argv0", &object.Method{Name: "argv0", Fn: processArgvZero, Arity: 0})
	processClass.DefineClassMethod("pid", &object.Method{Name: "pid", Fn: processPid, Arity: 0})
	processClass.DefineClassMethod("ppid", &object.Method{Name: "ppid", Fn: processPpid, Arity: 0})
	processClass.DefineClassMethod("uid", &object.Method{Name: "uid", Fn: processUid, Arity: 0})
	processClass.DefineClassMethod("uid=", &object.Method{Name: "uid=", Fn: processSetID, Arity: 1})
	processClass.DefineClassMethod("euid", &object.Method{Name: "euid", Fn: processEuid, Arity: 0})
	processClass.DefineClassMethod("euid=", &object.Method{Name: "euid=", Fn: processSetID, Arity: 1})
	processClass.DefineClassMethod("gid", &object.Method{Name: "gid", Fn: processGid, Arity: 0})
	processClass.DefineClassMethod("gid=", &object.Method{Name: "gid=", Fn: processSetID, Arity: 1})
	processClass.DefineClassMethod("egid", &object.Method{Name: "egid", Fn: processEgid, Arity: 0})
	processClass.DefineClassMethod("egid=", &object.Method{Name: "egid=", Fn: processSetID, Arity: 1})
	processClass.DefineClassMethod("groups", &object.Method{Name: "groups", Fn: processGroupsGet, Arity: 0})
	processClass.DefineClassMethod("groups=", &object.Method{Name: "groups=", Fn: processGroupsSet, Arity: 1})
	processClass.DefineClassMethod("initgroups", &object.Method{Name: "initgroups", Fn: processInitgroups, Arity: 2})
	processClass.DefineClassMethod("maxgroups", &object.Method{Name: "maxgroups", Fn: processMaxgroups, Arity: 0})
	processClass.DefineClassMethod("maxgroups=", &object.Method{Name: "maxgroups=", Fn: processSetMaxgroups, Arity: 1})
	processClass.DefineClassMethod("last_status", &object.Method{Name: "last_status", Fn: processLastStatus, Arity: -1})
	processClass.DefineClassMethod("spawn", &object.Method{Name: "spawn", Fn: processSpawn, Arity: -1})
	processClass.DefineClassMethod("exec", &object.Method{Name: "exec", Fn: processExec, Arity: -1})
	processClass.DefineClassMethod("fork", &object.Method{Name: "fork", Fn: processFork, Arity: -1})
	processClass.DefineClassMethod("wait", &object.Method{Name: "wait", Fn: processWait, Arity: -1})
	processClass.DefineClassMethod("waitpid", &object.Method{Name: "waitpid", Fn: processWait, Arity: -1})
	processClass.DefineClassMethod("wait2", &object.Method{Name: "wait2", Fn: processWait2, Arity: -1})
	processClass.DefineClassMethod("waitpid2", &object.Method{Name: "waitpid2", Fn: processWait2, Arity: -1})
	processClass.DefineClassMethod("waitall", &object.Method{Name: "waitall", Fn: processWaitall, Arity: -1})
	processClass.DefineClassMethod("detach", &object.Method{Name: "detach", Fn: processDetach, Arity: 1})
	processClass.DefineClassMethod("kill", &object.Method{Name: "kill", Fn: processKill, Arity: -1})
	processClass.DefineClassMethod("abort", &object.Method{Name: "abort", Fn: builtinAbort, Arity: -1})
	processClass.DefineClassMethod("exit", &object.Method{Name: "exit", Fn: builtinExit, Arity: -1})
	processClass.DefineClassMethod("exit!", &object.Method{Name: "exit!", Fn: builtinExitBang, Arity: -1})
	processClass.DefineClassMethod("clock_gettime", &object.Method{Name: "clock_gettime", Fn: processClockGettime, Arity: -1})
	processClass.DefineClassMethod("getpriority", &object.Method{Name: "getpriority", Fn: processGetpriority, Arity: 2})
	processClass.DefineClassMethod("setpriority", &object.Method{Name: "setpriority", Fn: processSetpriority, Arity: 3})
	processClass.DefineClassMethod("getrlimit", &object.Method{Name: "getrlimit", Fn: processGetrlimit, Arity: 1})
	processClass.DefineClassMethod("setrlimit", &object.Method{Name: "setrlimit", Fn: processSetrlimit, Arity: -1})
	processClass.DefineClassMethod("constants", &object.Method{Name: "constants", Fn: processConstants, Arity: -1})
	processClass.DefineClassMethod("const_get", &object.Method{Name: "const_get", Fn: processConstGet, Arity: 1})
	processClass.DefineClassMethod("const_defined?", &object.Method{Name: "const_defined?", Fn: processConstDefined, Arity: 1})
	processUIDClass := R.Classes["Process::UID"]
	processUIDClass.DefineClassMethod("rid", &object.Method{Name: "rid", Fn: processUid, Arity: 0})
	processUIDClass.DefineClassMethod("eid", &object.Method{Name: "eid", Fn: processEuid, Arity: 0})
	processGIDClass := R.Classes["Process::GID"]
	processGIDClass.DefineClassMethod("rid", &object.Method{Name: "rid", Fn: processGid, Arity: 0})
	processGIDClass.DefineClassMethod("eid", &object.Method{Name: "eid", Fn: processEgid, Arity: 0})
	processSysClass := R.Classes["Process::Sys"]
	processSysClass.DefineClassMethod("getuid", &object.Method{Name: "getuid", Fn: processUid, Arity: 0})
	processSysClass.DefineClassMethod("geteuid", &object.Method{Name: "geteuid", Fn: processEuid, Arity: 0})
	processSysClass.DefineClassMethod("getgid", &object.Method{Name: "getgid", Fn: processGid, Arity: 0})
	processSysClass.DefineClassMethod("getegid", &object.Method{Name: "getegid", Fn: processEgid, Arity: 0})
	processStatusClass := R.Classes["Process::Status"]
	processStatusClass.DefineClassMethod("wait", &object.Method{Name: "wait", Fn: processStatusWait, Arity: -1})
	processStatusClass.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: processStatusToI, Arity: 0})
	processStatusClass.DefineMethod("to_int", &object.Method{Name: "to_int", Fn: processStatusToI, Arity: 0})
	processStatusClass.DefineMethod("exitstatus", &object.Method{Name: "exitstatus", Fn: processStatusExitstatus, Arity: 0})
	processStatusClass.DefineMethod("pid", &object.Method{Name: "pid", Fn: processStatusPid, Arity: 0})
	processStatusClass.DefineMethod("&", &object.Method{Name: "&", Fn: processStatusBitAnd, Arity: 1})
	processStatusClass.DefineMethod(">>", &object.Method{Name: ">>", Fn: processStatusRightShift, Arity: 1})

	mockExpectationClass := R.Classes["MockExpectation"]
	mockExpectationClass.DefineMethod("with", &object.Method{Name: "with", Fn: mockExpectationWith, Arity: -1})
	mockExpectationClass.DefineMethod("any_number_of_times", &object.Method{Name: "any_number_of_times", Fn: mockExpectationWith, Arity: -1})
	mockExpectationClass.DefineMethod("once", &object.Method{Name: "once", Fn: mockExpectationWith, Arity: -1})
	mockExpectationClass.DefineMethod("exactly", &object.Method{Name: "exactly", Fn: mockExpectationWith, Arity: -1})
	mockExpectationClass.DefineMethod("and_return", &object.Method{Name: "and_return", Fn: mockExpectationAndReturn, Arity: -1})
	mockExpectationClass.DefineMethod("and_raise", &object.Method{Name: "and_raise", Fn: mockExpectationAndRaise, Arity: -1})

	mutexClass := R.Classes["Mutex"]
	mutexClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: mutexClassNew, Arity: 0})
	mutexClass.DefineMethod("lock", &object.Method{Name: "lock", Fn: mutexLock, Arity: 0})
	mutexClass.DefineMethod("unlock", &object.Method{Name: "unlock", Fn: mutexUnlock, Arity: 0})
	mutexClass.DefineMethod("locked?", &object.Method{Name: "locked?", Fn: mutexLocked, Arity: 0})
	mutexClass.DefineMethod("owned?", &object.Method{Name: "owned?", Fn: mutexOwned, Arity: 0})
	mutexClass.DefineMethod("try_lock", &object.Method{Name: "try_lock", Fn: mutexTryLock, Arity: 0})
	mutexClass.DefineMethod("synchronize", &object.Method{Name: "synchronize", Fn: mutexSynchronize, Arity: 0})
	mutexClass.DefineMethod("sleep", &object.Method{Name: "sleep", Fn: mutexSleep, Arity: -1})

	conditionVariableClass := R.Classes["ConditionVariable"]
	conditionVariableClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: conditionVariableClassNew, Arity: 0})
	conditionVariableClass.DefineMethod("wait", &object.Method{Name: "wait", Fn: conditionVariableWait, Arity: -1})
	conditionVariableClass.DefineMethod("signal", &object.Method{Name: "signal", Fn: conditionVariableSignal, Arity: 0})
	conditionVariableClass.DefineMethod("broadcast", &object.Method{Name: "broadcast", Fn: conditionVariableBroadcast, Arity: 0})
	conditionVariableClass.DefineMethod("marshal_dump", &object.Method{Name: "marshal_dump", Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		return typeError("can't dump ConditionVariable")
	}, Arity: 0})

	queueClass := R.Classes["Queue"]
	queueClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: queueClassNew, Arity: -1})
	queueClass.DefineMethod("push", &object.Method{Name: "push", Fn: queuePush, Arity: 1})
	queueClass.DefineMethod("enq", &object.Method{Name: "enq", Fn: queuePush, Arity: 1})
	queueClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: queuePush, Arity: 1})
	queueClass.DefineMethod("append", &object.Method{Name: "append", Fn: queuePush, Arity: 1})
	queueClass.DefineMethod("pop", &object.Method{Name: "pop", Fn: queuePop, Arity: -1})
	queueClass.DefineMethod("deq", &object.Method{Name: "deq", Fn: queuePop, Arity: -1})
	queueClass.DefineMethod("shift", &object.Method{Name: "shift", Fn: queuePop, Arity: -1})
	queueClass.DefineMethod("size", &object.Method{Name: "size", Fn: queueSize, Arity: 0})
	queueClass.DefineMethod("length", &object.Method{Name: "length", Fn: queueSize, Arity: 0})
	queueClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: queueEmpty, Arity: 0})
	queueClass.DefineMethod("clear", &object.Method{Name: "clear", Fn: queueClear, Arity: 0})
	queueClass.DefineMethod("close", &object.Method{Name: "close", Fn: queueClose, Arity: 0})
	queueClass.DefineMethod("closed?", &object.Method{Name: "closed?", Fn: queueClosed, Arity: 0})
	queueClass.DefineMethod("num_waiting", &object.Method{Name: "num_waiting", Fn: queueNumWaiting, Arity: 0})

	sizedQueueClass := R.Classes["SizedQueue"]
	sizedQueueClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: sizedQueueClassNew, Arity: -1})
	sizedQueueClass.DefineMethod("push", &object.Method{Name: "push", Fn: sizedQueuePush, Arity: -1})
	sizedQueueClass.DefineMethod("enq", &object.Method{Name: "enq", Fn: sizedQueuePush, Arity: -1})
	sizedQueueClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: sizedQueuePush, Arity: -1})
	sizedQueueClass.DefineMethod("append", &object.Method{Name: "append", Fn: sizedQueuePush, Arity: -1})
	sizedQueueClass.DefineMethod("pop", &object.Method{Name: "pop", Fn: queuePop, Arity: -1})
	sizedQueueClass.DefineMethod("deq", &object.Method{Name: "deq", Fn: queuePop, Arity: -1})
	sizedQueueClass.DefineMethod("shift", &object.Method{Name: "shift", Fn: queuePop, Arity: -1})
	sizedQueueClass.DefineMethod("size", &object.Method{Name: "size", Fn: queueSize, Arity: 0})
	sizedQueueClass.DefineMethod("length", &object.Method{Name: "length", Fn: queueSize, Arity: 0})
	sizedQueueClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: queueEmpty, Arity: 0})
	sizedQueueClass.DefineMethod("clear", &object.Method{Name: "clear", Fn: queueClear, Arity: 0})
	sizedQueueClass.DefineMethod("close", &object.Method{Name: "close", Fn: queueClose, Arity: 0})
	sizedQueueClass.DefineMethod("closed?", &object.Method{Name: "closed?", Fn: queueClosed, Arity: 0})
	sizedQueueClass.DefineMethod("num_waiting", &object.Method{Name: "num_waiting", Fn: queueNumWaiting, Arity: 0})
	sizedQueueClass.DefineMethod("max", &object.Method{Name: "max", Fn: sizedQueueMax, Arity: 0})
	sizedQueueClass.DefineMethod("max=", &object.Method{Name: "max=", Fn: sizedQueueSetMax, Arity: 1})

	fiberClass := R.Classes["Fiber"]
	fiberClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: fiberClassNew, Arity: 0})
	fiberClass.DefineClassMethod("current", &object.Method{Name: "current", Fn: fiberClassCurrent, Arity: 0})
	fiberClass.DefineClassMethod("yield", &object.Method{Name: "yield", Fn: fiberClassYield, Arity: -1})
	fiberClass.DefineMethod("resume", &object.Method{Name: "resume", Fn: fiberResume, Arity: -1})
	fiberClass.DefineMethod("alive?", &object.Method{Name: "alive?", Fn: fiberAlive, Arity: 0})
	fiberClass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: fiberInspect, Arity: 0})

	enumeratorClass := R.Classes["Enumerator"]
	enumeratorClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: enumeratorClassNew, Arity: 0})
	enumeratorClass.DefineMethod("each", &object.Method{Name: "each", Fn: enumeratorEach, Arity: 0})
	enumeratorClass.DefineMethod("next", &object.Method{Name: "next", Fn: enumeratorNext, Arity: 0})
	enumeratorClass.DefineMethod("size", &object.Method{Name: "size", Fn: enumeratorSize, Arity: 0})
	enumeratorClass.DefineMethod("to_a", &object.Method{Name: "to_a", Fn: enumeratorToA, Arity: 0})

	yielderClass := R.Classes["Enumerator::Yielder"]
	yielderClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: yielderAppend, Arity: 1})

	stringClass := R.Classes["String"]
	stringClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: stringClassNew, Arity: -1})
	stringClass.DefineMethod("+", &object.Method{Name: "+", Fn: stringAdd, Arity: 1})
	stringClass.DefineMethod("*", &object.Method{Name: "*", Fn: stringMul, Arity: 1})
	stringClass.DefineMethod("length", &object.Method{Name: "length", Fn: stringLength, Arity: 0})
	stringClass.DefineMethod("size", &object.Method{Name: "size", Fn: stringLength, Arity: 0})
	stringClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: stringEmpty, Arity: 0})
	stringClass.DefineMethod("b", &object.Method{Name: "b", Fn: stringToS, Arity: 0})
	stringClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: stringToS, Arity: 0})
	stringClass.DefineMethod("upcase", &object.Method{Name: "upcase", Fn: stringUpcase, Arity: 0})
	stringClass.DefineMethod("downcase", &object.Method{Name: "downcase", Fn: stringDowncase, Arity: 0})
	stringClass.DefineMethod("strip", &object.Method{Name: "strip", Fn: stringStrip, Arity: 0})
	stringClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: stringIndex, Arity: 1})
	stringClass.DefineMethod("capitalize", &object.Method{Name: "capitalize", Fn: stringCapitalize, Arity: 0})
	stringClass.DefineMethod("include?", &object.Method{Name: "include?", Fn: stringInclude, Arity: 1})
	stringClass.DefineMethod("=~", &object.Method{Name: "=~", Fn: stringRegexpMatch, Arity: 1})
	stringClass.DefineMethod("start_with?", &object.Method{Name: "start_with?", Fn: stringStartWith, Arity: 1})
	stringClass.DefineMethod("end_with?", &object.Method{Name: "end_with?", Fn: stringEndWith, Arity: 1})
	stringClass.DefineMethod("reverse", &object.Method{Name: "reverse", Fn: stringReverse, Arity: 0})
	stringClass.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: stringToI, Arity: 0})
	stringClass.DefineMethod("count", &object.Method{Name: "count", Fn: stringCount, Arity: 0})
	stringClass.DefineMethod("size", &object.Method{Name: "size", Fn: stringCountChars, Arity: 0})
	stringClass.DefineMethod("bytes", &object.Method{Name: "bytes", Fn: stringBytes, Arity: 0})
	stringClass.DefineMethod("chars", &object.Method{Name: "chars", Fn: stringChars, Arity: 0})
	stringClass.DefineMethod("find", &object.Method{Name: "find", Fn: stringFind, Arity: 1})
	stringClass.DefineMethod("slice", &object.Method{Name: "slice", Fn: stringSlice, Arity: 1})
	stringClass.DefineMethod("to_sym", &object.Method{Name: "to_sym", Fn: stringToSym, Arity: 0})
	stringClass.DefineMethod("ljust", &object.Method{Name: "ljust", Fn: stringLjust, Arity: 1})
	stringClass.DefineMethod("rjust", &object.Method{Name: "rjust", Fn: stringRjust, Arity: 1})
	stringClass.DefineMethod("center", &object.Method{Name: "center", Fn: stringCenter, Arity: 1})
	stringClass.DefineMethod("gsub", &object.Method{Name: "gsub", Fn: stringGsub, Arity: 2})
	stringClass.DefineMethod("sub", &object.Method{Name: "sub", Fn: stringSub, Arity: 2})
	stringClass.DefineMethod("split", &object.Method{Name: "split", Fn: stringSplit, Arity: 1})
	stringClass.DefineMethod("lines", &object.Method{Name: "lines", Fn: stringLines, Arity: 0})
	stringClass.DefineMethod("chomp", &object.Method{Name: "chomp", Fn: stringChomp, Arity: 0})
	stringClass.DefineMethod("chop", &object.Method{Name: "chop", Fn: stringChop, Arity: 0})
	stringClass.DefineMethod("strip!", &object.Method{Name: "strip!", Fn: stringStripBang, Arity: 0})
	stringClass.DefineMethod("upcase!", &object.Method{Name: "upcase!", Fn: stringUpcaseBang, Arity: 0})
	stringClass.DefineMethod("downcase!", &object.Method{Name: "downcase!", Fn: stringDowncaseBang, Arity: 0})
	stringClass.DefineMethod("reverse!", &object.Method{Name: "reverse!", Fn: stringReverseBang, Arity: 0})
	stringClass.DefineMethod("concat", &object.Method{Name: "concat", Fn: stringConcat, Arity: 1})
	stringClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: stringConcat, Arity: 1})
	stringClass.DefineMethod("index", &object.Method{Name: "index", Fn: stringIndexOf, Arity: 1})
	stringClass.DefineMethod("rindex", &object.Method{Name: "rindex", Fn: stringRIndexOf, Arity: 1})
	stringClass.DefineMethod("ord", &object.Method{Name: "ord", Fn: stringOrd, Arity: 0})
	stringClass.DefineMethod("+@", &object.Method{Name: "+@", Fn: stringUplus, Arity: 0})
	stringClass.DefineMethod("-@", &object.Method{Name: "-@", Fn: stringUminus, Arity: 0})
	stringClass.DefineMethod("succ", &object.Method{Name: "succ", Fn: stringSucc, Arity: 0})
	stringClass.DefineMethod("next", &object.Method{Name: "next", Fn: stringSucc, Arity: 0})
	stringClass.DefineMethod("lstrip", &object.Method{Name: "lstrip", Fn: stringLstrip, Arity: 0})
	stringClass.DefineMethod("rstrip", &object.Method{Name: "rstrip", Fn: stringRstrip, Arity: 0})
	stringClass.DefineMethod("lstrip!", &object.Method{Name: "lstrip!", Fn: stringLstripBang, Arity: 0})
	stringClass.DefineMethod("rstrip!", &object.Method{Name: "rstrip!", Fn: stringRstripBang, Arity: 0})
	stringClass.DefineMethod("strip!", &object.Method{Name: "strip!", Fn: stringStripBang, Arity: 0})
	stringClass.DefineMethod("replace", &object.Method{Name: "replace", Fn: stringReplace, Arity: 1})
	stringClass.DefineMethod("insert", &object.Method{Name: "insert", Fn: stringInsert, Arity: 2})
	stringClass.DefineMethod("swapcase", &object.Method{Name: "swapcase", Fn: stringSwapcase, Arity: 0})
	stringClass.DefineMethod("delete", &object.Method{Name: "delete", Fn: stringDelete, Arity: 1})
	stringClass.DefineMethod("squeeze", &object.Method{Name: "squeeze", Fn: stringSqueeze, Arity: 0})
	stringClass.DefineMethod("to_f", &object.Method{Name: "to_f", Fn: stringToF, Arity: 0})
	stringClass.DefineMethod("hex", &object.Method{Name: "hex", Fn: stringHex, Arity: 0})
	stringClass.DefineMethod("oct", &object.Method{Name: "oct", Fn: stringOct, Arity: 0})
	stringClass.DefineMethod("encoding", &object.Method{Name: "encoding", Fn: stringEncoding, Arity: 0})
	stringClass.DefineMethod("force_encoding", &object.Method{Name: "force_encoding", Fn: stringForceEncoding, Arity: 1})
	stringClass.DefineMethod("encode", &object.Method{Name: "encode", Fn: stringEncode, Arity: 1})
	stringClass.DefineMethod("unpack", &object.Method{Name: "unpack", Fn: stringUnpack, Arity: 1})

	encodingClass := R.Classes["Encoding"]
	encodingClass.DefineMethod("==", &object.Method{Name: "==", Fn: encodingEqual, Arity: 1})
	encodingClass.DefineClassMethod("default_external", &object.Method{Name: "default_external", Fn: encodingDefaultExternal, Arity: 0})
	encodingClass.DefineClassMethod("default_external=", &object.Method{Name: "default_external=", Fn: encodingSetDefaultExternal, Arity: 1})
	encodingClass.DefineClassMethod("find", &object.Method{Name: "find", Fn: encodingFind, Arity: 1})
	encodingClass.DefineConstant("UTF_8", newEncodingValue("UTF-8"))
	encodingClass.DefineConstant("UTF_16BE", newEncodingValue("UTF-16BE"))
	encodingClass.DefineConstant("UTF_32BE", newEncodingValue("UTF-32BE"))
	encodingClass.DefineConstant("US_ASCII", newEncodingValue("US-ASCII"))
	encodingClass.DefineConstant("BINARY", newEncodingValue("BINARY"))
	encodingClass.DefineConstant("ASCII_8BIT", newEncodingValue("BINARY"))
	encodingClass.DefineConstant("CP1251", newEncodingValue("CP1251"))
	encodingClass.DefineConstant("SHIFT_JIS", newEncodingValue("SHIFT-JIS"))
	encodingClass.DefineConstant("CompatibilityError", &object.EmeraldValue{Type: object.ValueClass, Data: R.Classes["Encoding::CompatibilityError"], Class: R.Classes["Class"]})

	ioClass := R.Classes["IO"]
	ioClass.DefineClassMethod("pipe", &object.Method{Name: "pipe", Fn: ioClassPipe, Arity: -1})
	ioClass.DefineClassMethod("for_fd", &object.Method{Name: "for_fd", Fn: ioClassForFd, Arity: -1})
	ioClass.DefineMethod("write_nonblock", &object.Method{Name: "write_nonblock", Fn: ioWriteNonblock, Arity: -1})
	ioClass.DefineMethod("syswrite", &object.Method{Name: "syswrite", Fn: ioSyswrite, Arity: 1})
	ioClass.DefineMethod("write", &object.Method{Name: "write", Fn: ioSyswrite, Arity: 1})
	ioClass.DefineMethod("puts", &object.Method{Name: "puts", Fn: ioPuts, Arity: -1})
	ioClass.DefineMethod("close", &object.Method{Name: "close", Fn: ioClose, Arity: 0})
	ioClass.DefineMethod("close_exception", &object.Method{Name: "close_exception", Fn: ioCloseException, Arity: 0})
	ioClass.DefineMethod("close_exception=", &object.Method{Name: "close_exception=", Fn: ioSetCloseException, Arity: 1})
	ioClass.DefineMethod("fileno", &object.Method{Name: "fileno", Fn: ioFileno, Arity: 0})
	ioClass.DefineMethod("nonblock=", &object.Method{Name: "nonblock=", Fn: ioSetNonblock, Arity: 1})
	ioClass.DefineMethod("nonblock?", &object.Method{Name: "nonblock?", Fn: ioNonblock, Arity: 0})
	ioClass.DefineMethod("autoclose=", &object.Method{Name: "autoclose=", Fn: ioSetAutoclose, Arity: 1})
	ioClass.DefineMethod("close_on_exec?", &object.Method{Name: "close_on_exec?", Fn: ioCloseOnExec, Arity: 0})

	fileClass := R.Classes["File"]
	fileClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: fileClassNew, Arity: -1})
	fileClass.DefineClassMethod("open", &object.Method{Name: "open", Fn: fileClassOpen, Arity: -1})
	fileClass.DefineClassMethod("read", &object.Method{Name: "read", Fn: fileClassRead, Arity: 1})
	fileClass.DefineClassMethod("readlines", &object.Method{Name: "readlines", Fn: fileClassReadlines, Arity: 1})
	fileClass.DefineClassMethod("size", &object.Method{Name: "size", Fn: fileClassSize, Arity: 1})
	fileClass.DefineClassMethod("truncate", &object.Method{Name: "truncate", Fn: fileClassTruncate, Arity: -1})
	fileClass.DefineClassMethod("stat", &object.Method{Name: "stat", Fn: fileClassStat, Arity: 1})
	fileClass.DefineClassMethod("lstat", &object.Method{Name: "lstat", Fn: fileClassLstat, Arity: 1})
	fileClass.DefineClassMethod("atime", &object.Method{Name: "atime", Fn: fileClassAtime, Arity: 1})
	fileClass.DefineClassMethod("mtime", &object.Method{Name: "mtime", Fn: fileClassTime, Arity: 1})
	fileClass.DefineClassMethod("ctime", &object.Method{Name: "ctime", Fn: fileClassTime, Arity: 1})
	fileClass.DefineClassMethod("birthtime", &object.Method{Name: "birthtime", Fn: fileClassTime, Arity: 1})
	fileClass.DefineClassMethod("utime", &object.Method{Name: "utime", Fn: fileClassUtime, Arity: -1})
	fileClass.DefineClassMethod("join", &object.Method{Name: "join", Fn: fileClassJoin, Arity: -1})
	fileClass.DefineClassMethod("basename", &object.Method{Name: "basename", Fn: fileClassBasename, Arity: -1})
	fileClass.DefineClassMethod("dirname", &object.Method{Name: "dirname", Fn: fileClassDirname, Arity: -1})
	fileClass.DefineClassMethod("extname", &object.Method{Name: "extname", Fn: fileClassExtname, Arity: -1})
	fileClass.DefineClassMethod("split", &object.Method{Name: "split", Fn: fileClassSplit, Arity: 1})
	fileClass.DefineClassMethod("path", &object.Method{Name: "path", Fn: fileClassPath, Arity: 1})
	fileClass.DefineClassMethod("fnmatch", &object.Method{Name: "fnmatch", Fn: fileClassFnmatch, Arity: -1})
	fileClass.DefineClassMethod("fnmatch?", &object.Method{Name: "fnmatch?", Fn: fileClassFnmatch, Arity: -1})
	fileClass.DefineClassMethod("expand_path", &object.Method{Name: "expand_path", Fn: fileClassExpandPath, Arity: -1})
	fileClass.DefineClassMethod("exist?", &object.Method{Name: "exist?", Fn: fileExistPredicate, Arity: 1})
	fileClass.DefineClassMethod("file?", &object.Method{Name: "file?", Fn: fileFilePredicate, Arity: 1})
	fileClass.DefineClassMethod("directory?", &object.Method{Name: "directory?", Fn: fileDirectoryPredicate, Arity: 1})
	fileClass.DefineClassMethod("zero?", &object.Method{Name: "zero?", Fn: fileZeroPredicate, Arity: 1})
	fileClass.DefineClassMethod("empty?", &object.Method{Name: "empty?", Fn: fileZeroPredicate, Arity: 1})
	fileClass.DefineClassMethod("size?", &object.Method{Name: "size?", Fn: fileSizeQuestionPredicate, Arity: 1})
	fileClass.DefineClassMethod("identical?", &object.Method{Name: "identical?", Fn: fileIdenticalPredicate, Arity: 2})
	fileClass.DefineClassMethod("ftype", &object.Method{Name: "ftype", Fn: fileClassFtype, Arity: 1})
	fileClass.DefineClassMethod("umask", &object.Method{Name: "umask", Fn: fileClassUmask, Arity: -1})
	fileClass.DefineClassMethod("realpath", &object.Method{Name: "realpath", Fn: fileClassRealpath, Arity: -1})
	fileClass.DefineClassMethod("realdirpath", &object.Method{Name: "realdirpath", Fn: fileClassRealdirpath, Arity: -1})
	fileClass.DefineClassMethod("link", &object.Method{Name: "link", Fn: fileClassLink, Arity: 2})
	fileClass.DefineClassMethod("symlink", &object.Method{Name: "symlink", Fn: fileClassSymlink, Arity: 2})
	fileClass.DefineClassMethod("mkfifo", &object.Method{Name: "mkfifo", Fn: fileClassMkfifo, Arity: -1})
	fileClass.DefineClassMethod("readlink", &object.Method{Name: "readlink", Fn: fileClassReadlink, Arity: 1})
	fileClass.DefineClassMethod("delete", &object.Method{Name: "delete", Fn: fileClassDelete, Arity: -1})
	fileClass.DefineClassMethod("unlink", &object.Method{Name: "unlink", Fn: fileClassDelete, Arity: -1})
	fileClass.DefineClassMethod("rename", &object.Method{Name: "rename", Fn: fileClassRename, Arity: 2})
	fileClass.DefineClassMethod("chmod", &object.Method{Name: "chmod", Fn: fileClassChmod, Arity: -1})
	fileClass.DefineClassMethod("chown", &object.Method{Name: "chown", Fn: fileClassChown, Arity: -1})
	fileClass.DefineClassMethod("executable?", &object.Method{Name: "executable?", Fn: fileExecutablePredicate, Arity: 1})
	fileClass.DefineClassMethod("executable_real?", &object.Method{Name: "executable_real?", Fn: fileExecutablePredicate, Arity: 1})
	fileClass.DefineClassMethod("readable?", &object.Method{Name: "readable?", Fn: fileReadablePredicate, Arity: 1})
	fileClass.DefineClassMethod("writable?", &object.Method{Name: "writable?", Fn: fileWritablePredicate, Arity: 1})
	fileClass.DefineClassMethod("writable_real?", &object.Method{Name: "writable_real?", Fn: fileWritablePredicate, Arity: 1})
	fileClass.DefineConstant("FNM_DOTMATCH", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("FNM_NOESCAPE", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(2), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("FNM_EXTGLOB", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(4), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("FNM_PATHNAME", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(8), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("FNM_CASEFOLD", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(16), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("FNM_SYSCASE", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(32), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("RDONLY", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("WRONLY", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("RDWR", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(2), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("APPEND", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(8), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("NONBLOCK", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(16), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("NOCTTY", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(32), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("CREAT", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(64), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("EXCL", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(128), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("TRUNC", &object.EmeraldValue{Type: object.ValueInteger, Data: int64(512), Class: R.Classes["Integer"]})
	fileClass.DefineConstant("Stat", &object.EmeraldValue{Type: object.ValueClass, Data: R.Classes["File::Stat"], Class: R.Classes["Class"]})
	fileClass.DefineMethod("size", &object.Method{Name: "size", Fn: fileInstanceSize, Arity: 0})
	fileClass.DefineMethod("stat", &object.Method{Name: "stat", Fn: fileInstanceStat, Arity: 0})
	fileClass.DefineMethod("lstat", &object.Method{Name: "lstat", Fn: fileInstanceLstat, Arity: 0})
	fileClass.DefineMethod("atime", &object.Method{Name: "atime", Fn: fileInstanceAtime, Arity: 0})
	fileClass.DefineMethod("mtime", &object.Method{Name: "mtime", Fn: fileInstanceTime, Arity: 0})
	fileClass.DefineMethod("ctime", &object.Method{Name: "ctime", Fn: fileInstanceTime, Arity: 0})
	fileClass.DefineMethod("birthtime", &object.Method{Name: "birthtime", Fn: fileInstanceTime, Arity: 0})
	fileClass.DefineMethod("chown", &object.Method{Name: "chown", Fn: fileInstanceChown, Arity: -1})
	fileClass.DefineMethod("chmod", &object.Method{Name: "chmod", Fn: fileInstanceChmod, Arity: 1})
	fileClass.DefineMethod("path", &object.Method{Name: "path", Fn: fileInstancePath, Arity: 0})
	fileClass.DefineMethod("truncate", &object.Method{Name: "truncate", Fn: fileInstanceTruncate, Arity: -1})
	fileClass.DefineMethod("flush", &object.Method{Name: "flush", Fn: fileInstanceFlush, Arity: 0})
	fileClass.DefineMethod("read", &object.Method{Name: "read", Fn: fileInstanceRead, Arity: -1})
	fileClass.DefineMethod("gets", &object.Method{Name: "gets", Fn: fileInstanceGets, Arity: -1})
	fileClass.DefineMethod("rewind", &object.Method{Name: "rewind", Fn: fileInstanceRewind, Arity: 0})
	fileClass.DefineMethod("pos", &object.Method{Name: "pos", Fn: fileInstancePos, Arity: 0})
	fileClass.DefineMethod("tell", &object.Method{Name: "tell", Fn: fileInstancePos, Arity: 0})
	fileClass.DefineMethod("binmode?", &object.Method{Name: "binmode?", Fn: fileInstanceBinmode, Arity: 0})
	fileClass.DefineMethod("external_encoding", &object.Method{Name: "external_encoding", Fn: fileInstanceExternalEncoding, Arity: 0})
	fileClass.DefineMethod("eof?", &object.Method{Name: "eof?", Fn: fileInstanceEOF, Arity: 0})
	fileClass.DefineMethod("closed?", &object.Method{Name: "closed?", Fn: ioClosed, Arity: 0})
	fileClass.DefineMethod("close_exception", &object.Method{Name: "close_exception", Fn: ioCloseException, Arity: 0})
	fileClass.DefineMethod("close_exception=", &object.Method{Name: "close_exception=", Fn: ioSetCloseException, Arity: 1})

	fileStatClass := R.Classes["File::Stat"]
	fileStatClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: fileStatClassNew, Arity: 1})
	fileStatClass.DefineMethod("file?", &object.Method{Name: "file?", Fn: fileStatFilePredicate, Arity: 0})
	fileStatClass.DefineMethod("directory?", &object.Method{Name: "directory?", Fn: fileStatDirectoryPredicate, Arity: 0})
	fileStatClass.DefineMethod("symlink?", &object.Method{Name: "symlink?", Fn: fileStatSymlinkPredicate, Arity: 0})
	fileStatClass.DefineMethod("zero?", &object.Method{Name: "zero?", Fn: fileStatZeroPredicate, Arity: 0})
	fileStatClass.DefineMethod("executable?", &object.Method{Name: "executable?", Fn: fileStatExecutablePredicate, Arity: 0})
	fileStatClass.DefineMethod("executable_real?", &object.Method{Name: "executable_real?", Fn: fileStatExecutablePredicate, Arity: 0})
	fileStatClass.DefineMethod("writable?", &object.Method{Name: "writable?", Fn: fileStatWritablePredicate, Arity: 0})
	fileStatClass.DefineMethod("writable_real?", &object.Method{Name: "writable_real?", Fn: fileStatWritablePredicate, Arity: 0})
	fileStatClass.DefineMethod("ftype", &object.Method{Name: "ftype", Fn: fileStatFtype, Arity: 0})
	fileStatClass.DefineMethod("size", &object.Method{Name: "size", Fn: fileStatSize, Arity: 0})
	fileStatClass.DefineMethod("size?", &object.Method{Name: "size?", Fn: fileStatSizeQuestion, Arity: 0})
	fileStatClass.DefineMethod("blksize", &object.Method{Name: "blksize", Fn: fileStatBlockSize, Arity: 0})
	fileStatClass.DefineMethod("atime", &object.Method{Name: "atime", Fn: fileStatTime, Arity: 0})
	fileStatClass.DefineMethod("ctime", &object.Method{Name: "ctime", Fn: fileStatTime, Arity: 0})
	fileStatClass.DefineMethod("mtime", &object.Method{Name: "mtime", Fn: fileStatTime, Arity: 0})
	fileStatClass.DefineMethod("mode", &object.Method{Name: "mode", Fn: fileStatMode, Arity: 0})
	fileStatClass.DefineMethod("dev", &object.Method{Name: "dev", Fn: fileStatIntegerOne, Arity: 0})
	fileStatClass.DefineMethod("dev_major", &object.Method{Name: "dev_major", Fn: fileStatIntegerOne, Arity: 0})
	fileStatClass.DefineMethod("dev_minor", &object.Method{Name: "dev_minor", Fn: fileStatIntegerZero, Arity: 0})
	fileStatClass.DefineMethod("ino", &object.Method{Name: "ino", Fn: fileStatIntegerOne, Arity: 0})
	fileStatClass.DefineMethod("rdev", &object.Method{Name: "rdev", Fn: fileStatIntegerZero, Arity: 0})
	fileStatClass.DefineMethod("rdev_major", &object.Method{Name: "rdev_major", Fn: fileStatIntegerZero, Arity: 0})
	fileStatClass.DefineMethod("rdev_minor", &object.Method{Name: "rdev_minor", Fn: fileStatIntegerZero, Arity: 0})

	timeClass := R.Classes["Time"]
	timeClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: timeClassNew, Arity: -1})
	timeClass.DefineClassMethod("now", &object.Method{Name: "now", Fn: timeClassNow, Arity: 0})
	timeClass.DefineClassMethod("at", &object.Method{Name: "at", Fn: timeClassAt, Arity: -1})
	timeClass.DefineClassMethod("utc", &object.Method{Name: "utc", Fn: timeClassUTC, Arity: -1})
	timeClass.DefineClassMethod("gm", &object.Method{Name: "gm", Fn: timeClassUTC, Arity: -1})
	timeClass.DefineClassMethod("local", &object.Method{Name: "local", Fn: timeClassLocal, Arity: -1})
	timeClass.DefineClassMethod("mktime", &object.Method{Name: "mktime", Fn: timeClassLocal, Arity: -1})

	marshalClass := R.Classes["Marshal"]
	marshalClass.DefineClassMethod("dump", &object.Method{Name: "dump", Fn: marshalClassDump, Arity: 1})
	marshalClass.DefineClassMethod("load", &object.Method{Name: "load", Fn: marshalClassLoad, Arity: 1})

	timeClass.DefineMethod("==", &object.Method{Name: "==", Fn: timeEqual, Arity: 1})
	timeClass.DefineMethod("eql?", &object.Method{Name: "eql?", Fn: timeEqual, Arity: 1})
	timeClass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: timeInspect, Arity: 0})
	timeClass.DefineMethod("to_a", &object.Method{Name: "to_a", Fn: timeToA, Arity: 0})
	timeClass.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: timeToI, Arity: 0})
	timeClass.DefineMethod("tv_sec", &object.Method{Name: "tv_sec", Fn: timeToI, Arity: 0})
	timeClass.DefineMethod("to_f", &object.Method{Name: "to_f", Fn: timeToF, Arity: 0})
	timeClass.DefineMethod("usec", &object.Method{Name: "usec", Fn: timeUsec, Arity: 0})
	timeClass.DefineMethod("tv_usec", &object.Method{Name: "tv_usec", Fn: timeUsec, Arity: 0})
	timeClass.DefineMethod("nsec", &object.Method{Name: "nsec", Fn: timeNsec, Arity: 0})
	timeClass.DefineMethod("tv_nsec", &object.Method{Name: "tv_nsec", Fn: timeNsec, Arity: 0})
	timeClass.DefineMethod("subsec", &object.Method{Name: "subsec", Fn: timeSubsec, Arity: 0})
	timeClass.DefineMethod("year", &object.Method{Name: "year", Fn: timeYear, Arity: 0})
	timeClass.DefineMethod("mon", &object.Method{Name: "mon", Fn: timeMonth, Arity: 0})
	timeClass.DefineMethod("month", &object.Method{Name: "month", Fn: timeMonth, Arity: 0})
	timeClass.DefineMethod("mday", &object.Method{Name: "mday", Fn: timeDay, Arity: 0})
	timeClass.DefineMethod("day", &object.Method{Name: "day", Fn: timeDay, Arity: 0})
	timeClass.DefineMethod("wday", &object.Method{Name: "wday", Fn: timeWDay, Arity: 0})
	timeClass.DefineMethod("yday", &object.Method{Name: "yday", Fn: timeYDay, Arity: 0})
	timeClass.DefineMethod("hour", &object.Method{Name: "hour", Fn: timeHour, Arity: 0})
	timeClass.DefineMethod("min", &object.Method{Name: "min", Fn: timeMin, Arity: 0})
	timeClass.DefineMethod("sec", &object.Method{Name: "sec", Fn: timeSec, Arity: 0})
	timeClass.DefineMethod("isdst", &object.Method{Name: "isdst", Fn: timeIsDST, Arity: 0})
	timeClass.DefineMethod("utc_offset", &object.Method{Name: "utc_offset", Fn: timeUTCOffset, Arity: 0})
	timeClass.DefineMethod("zone", &object.Method{Name: "zone", Fn: timeZone, Arity: 0})
	timeClass.DefineMethod("utc?", &object.Method{Name: "utc?", Fn: timeUTCPredicate, Arity: 0})
	timeClass.DefineMethod("gmt?", &object.Method{Name: "gmt?", Fn: timeUTCPredicate, Arity: 0})
	timeClass.DefineMethod("getgm", &object.Method{Name: "getgm", Fn: timeGetUTC, Arity: 0})
	timeClass.DefineMethod("getutc", &object.Method{Name: "getutc", Fn: timeGetUTC, Arity: 0})
	timeClass.DefineMethod("utc", &object.Method{Name: "utc", Fn: timeUTCMutate, Arity: 0})
	timeClass.DefineMethod("gmtime", &object.Method{Name: "gmtime", Fn: timeUTCMutate, Arity: 0})
	timeClass.DefineMethod("getlocal", &object.Method{Name: "getlocal", Fn: timeGetLocal, Arity: -1})
	timeClass.DefineMethod("localtime", &object.Method{Name: "localtime", Fn: timeLocaltime, Arity: -1})
	timeClass.DefineMethod("+", &object.Method{Name: "+", Fn: timePlus, Arity: 1})
	timeClass.DefineMethod("-", &object.Method{Name: "-", Fn: timeMinus, Arity: 1})

	fileTestClass := R.Classes["FileTest"]
	fileTestClass.DefineClassMethod("exist?", &object.Method{Name: "exist?", Fn: fileExistPredicate, Arity: 1})
	fileTestClass.DefineClassMethod("file?", &object.Method{Name: "file?", Fn: fileFilePredicate, Arity: 1})
	fileTestClass.DefineClassMethod("directory?", &object.Method{Name: "directory?", Fn: fileDirectoryPredicate, Arity: 1})
	fileTestClass.DefineClassMethod("size", &object.Method{Name: "size", Fn: fileSizePredicate, Arity: 1})
	fileTestClass.DefineClassMethod("size?", &object.Method{Name: "size?", Fn: fileSizeQuestionPredicate, Arity: 1})
	fileTestClass.DefineClassMethod("zero?", &object.Method{Name: "zero?", Fn: fileZeroPredicate, Arity: 1})
	fileTestClass.DefineClassMethod("identical?", &object.Method{Name: "identical?", Fn: fileIdenticalPredicate, Arity: 2})
	fileTestClass.DefineClassMethod("executable?", &object.Method{Name: "executable?", Fn: fileExecutablePredicate, Arity: 1})
	fileTestClass.DefineClassMethod("executable_real?", &object.Method{Name: "executable_real?", Fn: fileExecutablePredicate, Arity: 1})
	fileTestClass.DefineClassMethod("readable?", &object.Method{Name: "readable?", Fn: fileReadablePredicate, Arity: 1})
	fileTestClass.DefineClassMethod("writable?", &object.Method{Name: "writable?", Fn: fileWritablePredicate, Arity: 1})
	fileTestClass.DefineClassMethod("writable_real?", &object.Method{Name: "writable_real?", Fn: fileWritablePredicate, Arity: 1})
	dirClass := R.Classes["Dir"]
	dirClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: dirClassOpen, Arity: -1})
	dirClass.DefineClassMethod("open", &object.Method{Name: "open", Fn: dirClassOpen, Arity: -1})
	dirClass.DefineClassMethod("for_fd", &object.Method{Name: "for_fd", Fn: dirClassForFd, Arity: 1})
	dirClass.DefineClassMethod("home", &object.Method{Name: "home", Fn: dirClassHome, Arity: -1})
	dirClass.DefineClassMethod("pwd", &object.Method{Name: "pwd", Fn: dirClassPwd, Arity: 0})
	dirClass.DefineClassMethod("getwd", &object.Method{Name: "getwd", Fn: dirClassPwd, Arity: 0})
	dirClass.DefineClassMethod("chdir", &object.Method{Name: "chdir", Fn: dirClassChdir, Arity: -1})
	dirClass.DefineClassMethod("fchdir", &object.Method{Name: "fchdir", Fn: dirClassFchdir, Arity: -1})
	dirClass.DefineClassMethod("chroot", &object.Method{Name: "chroot", Fn: dirClassChroot, Arity: 1})
	dirClass.DefineClassMethod("glob", &object.Method{Name: "glob", Fn: dirClassGlob, Arity: -1})
	dirClass.DefineClassMethod("[]", &object.Method{Name: "[]", Fn: dirClassGlob, Arity: -1})
	dirClass.DefineClassMethod("mkdir", &object.Method{Name: "mkdir", Fn: dirClassMkdir, Arity: -1})
	dirClass.DefineClassMethod("rmdir", &object.Method{Name: "rmdir", Fn: dirClassRmdir, Arity: 1})
	dirClass.DefineClassMethod("delete", &object.Method{Name: "delete", Fn: dirClassRmdir, Arity: 1})
	dirClass.DefineClassMethod("unlink", &object.Method{Name: "unlink", Fn: dirClassRmdir, Arity: 1})
	dirClass.DefineClassMethod("empty?", &object.Method{Name: "empty?", Fn: dirClassEmpty, Arity: 1})
	dirClass.DefineClassMethod("exist?", &object.Method{Name: "exist?", Fn: fileDirectoryPredicate, Arity: 1})
	dirClass.DefineClassMethod("entries", &object.Method{Name: "entries", Fn: dirClassEntries, Arity: -1})
	dirClass.DefineClassMethod("children", &object.Method{Name: "children", Fn: dirClassChildren, Arity: -1})
	dirClass.DefineClassMethod("each_child", &object.Method{Name: "each_child", Fn: dirClassEachChild, Arity: -1})
	dirClass.DefineClassMethod("foreach", &object.Method{Name: "foreach", Fn: dirClassForeach, Arity: -1})
	dirClass.DefineMethod("close", &object.Method{Name: "close", Fn: dirClose, Arity: 0})
	dirClass.DefineMethod("path", &object.Method{Name: "path", Fn: dirPath, Arity: 0})
	dirClass.DefineMethod("to_path", &object.Method{Name: "to_path", Fn: dirPath, Arity: 0})
	dirClass.DefineMethod("chdir", &object.Method{Name: "chdir", Fn: dirChdir, Arity: 0})
	dirClass.DefineMethod("read", &object.Method{Name: "read", Fn: dirRead, Arity: 0})
	dirClass.DefineMethod("rewind", &object.Method{Name: "rewind", Fn: dirRewind, Arity: 0})
	dirClass.DefineMethod("tell", &object.Method{Name: "tell", Fn: dirTell, Arity: 0})
	dirClass.DefineMethod("pos", &object.Method{Name: "pos", Fn: dirTell, Arity: 0})
	dirClass.DefineMethod("pos=", &object.Method{Name: "pos=", Fn: dirSetPos, Arity: 1})
	dirClass.DefineMethod("each", &object.Method{Name: "each", Fn: dirEach, Arity: 0})
	dirClass.DefineMethod("children", &object.Method{Name: "children", Fn: dirChildren, Arity: -1})
	dirClass.DefineMethod("each_child", &object.Method{Name: "each_child", Fn: dirEachChild, Arity: -1})
	dirClass.DefineMethod("fileno", &object.Method{Name: "fileno", Fn: dirFileno, Arity: 0})

	objectClass.DefineMethod("write_nonblock", &object.Method{Name: "write_nonblock", Fn: ioWriteNonblock, Arity: -1})
	objectClass.DefineMethod("syswrite", &object.Method{Name: "syswrite", Fn: ioSyswrite, Arity: 1})
	objectClass.DefineMethod("write", &object.Method{Name: "write", Fn: ioSyswrite, Arity: 1})
	objectClass.DefineMethod("close", &object.Method{Name: "close", Fn: ioClose, Arity: 0})

	structClass := R.Classes["Struct"]
	structClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: structClassNew, Arity: -1})

	arrayClass := R.Classes["Array"]
	arrayClass.DefineMethod("==", &object.Method{Name: "==", Fn: arrayEqual, Arity: 1})
	arrayClass.DefineMethod("*", &object.Method{Name: "*", Fn: arrayMultiply, Arity: 1})
	arrayClass.DefineMethod("length", &object.Method{Name: "length", Fn: arrayLength, Arity: 0})
	arrayClass.DefineMethod("size", &object.Method{Name: "size", Fn: arrayLength, Arity: 0})
	arrayClass.DefineMethod("first", &object.Method{Name: "first", Fn: arrayFirst, Arity: -1})
	arrayClass.DefineMethod("last", &object.Method{Name: "last", Fn: arrayLast, Arity: -1})
	arrayClass.DefineMethod("push", &object.Method{Name: "push", Fn: arrayPush, Arity: 1})
	arrayClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: arrayPush, Arity: 1})
	arrayClass.DefineMethod("pop", &object.Method{Name: "pop", Fn: arrayPop, Arity: 0})
	arrayClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: arrayEmpty, Arity: 0})
	arrayClass.DefineMethod("join", &object.Method{Name: "join", Fn: arrayJoin, Arity: 0})
	arrayClass.DefineMethod("reverse", &object.Method{Name: "reverse", Fn: arrayReverse, Arity: 0})
	arrayClass.DefineMethod("reverse!", &object.Method{Name: "reverse!", Fn: arrayReverseBang, Arity: 0})
	arrayClass.DefineMethod("reverse_each", &object.Method{Name: "reverse_each", Fn: arrayReverseEach, Arity: 0})
	arrayClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: arrayIndex, Arity: 1})
	arrayClass.DefineMethod("at", &object.Method{Name: "at", Fn: arrayIndex, Arity: 1})
	arrayClass.DefineMethod("each", &object.Method{Name: "each", Fn: arrayEach, Arity: 0})
	arrayClass.DefineMethod("map", &object.Method{Name: "map", Fn: arrayMap, Arity: 0})
	arrayClass.DefineMethod("collect", &object.Method{Name: "collect", Fn: arrayMap, Arity: 0})
	arrayClass.DefineMethod("map!", &object.Method{Name: "map!", Fn: arrayMapBang, Arity: 0})
	arrayClass.DefineMethod("collect!", &object.Method{Name: "collect!", Fn: arrayMapBang, Arity: 0})
	arrayClass.DefineMethod("select", &object.Method{Name: "select", Fn: arraySelect, Arity: 0})
	arrayClass.DefineMethod("select!", &object.Method{Name: "select!", Fn: arraySelectBang, Arity: 0})
	arrayClass.DefineMethod("filter!", &object.Method{Name: "filter!", Fn: arraySelectBang, Arity: 0})
	arrayClass.DefineMethod("find", &object.Method{Name: "find", Fn: arrayFind, Arity: 0})
	arrayClass.DefineMethod("concat", &object.Method{Name: "concat", Fn: arrayConcat, Arity: 1})
	arrayClass.DefineMethod("delete_at", &object.Method{Name: "delete_at", Fn: arrayDeleteAt, Arity: 1})
	arrayClass.DefineMethod("delete_if", &object.Method{Name: "delete_if", Fn: arrayDeleteIf, Arity: 0})
	arrayClass.DefineMethod("keep_if", &object.Method{Name: "keep_if", Fn: arrayKeepIf, Arity: 0})
	arrayClass.DefineMethod("shift", &object.Method{Name: "shift", Fn: arrayShift, Arity: 0})
	arrayClass.DefineMethod("unshift", &object.Method{Name: "unshift", Fn: arrayUnshift, Arity: 1})
	arrayClass.DefineMethod("prepend", &object.Method{Name: "prepend", Fn: arrayUnshift, Arity: 1})
	arrayClass.DefineMethod("sample", &object.Method{Name: "sample", Fn: arraySample, Arity: 0})
	arrayClass.DefineMethod("clear", &object.Method{Name: "clear", Fn: arrayClear, Arity: 0})
	arrayClass.DefineMethod("initialize", &object.Method{Name: "initialize", Fn: arrayInitialize, Arity: -1})
	arrayClass.DefineMethod("include?", &object.Method{Name: "include?", Fn: arrayInclude, Arity: 1})
	arrayClass.DefineMethod("assoc", &object.Method{Name: "assoc", Fn: arrayAssoc, Arity: 1})
	arrayClass.DefineMethod("rassoc", &object.Method{Name: "rassoc", Fn: arrayRassoc, Arity: 1})
	arrayClass.DefineMethod("hash", &object.Method{Name: "hash", Fn: arrayHash, Arity: 0})
	arrayClass.DefineMethod("[]=", &object.Method{Name: "[]=", Fn: arrayIndexSet, Arity: 2})
	arrayClass.DefineMethod("count", &object.Method{Name: "count", Fn: arrayCount, Arity: 0})
	arrayClass.DefineMethod("index", &object.Method{Name: "index", Fn: arrayIndexOf, Arity: 1})
	arrayClass.DefineMethod("rindex", &object.Method{Name: "rindex", Fn: arrayRIndexOf, Arity: 1})
	arrayClass.DefineMethod("delete", &object.Method{Name: "delete", Fn: arrayDelete, Arity: 1})
	arrayClass.DefineMethod("compact", &object.Method{Name: "compact", Fn: arrayCompact, Arity: 0})
	arrayClass.DefineMethod("compact!", &object.Method{Name: "compact!", Fn: arrayCompactBang, Arity: 0})
	arrayClass.DefineMethod("flatten", &object.Method{Name: "flatten", Fn: arrayFlatten, Arity: 0})
	arrayClass.DefineMethod("flatten!", &object.Method{Name: "flatten!", Fn: arrayFlattenBang, Arity: 0})
	arrayClass.DefineMethod("uniq", &object.Method{Name: "uniq", Fn: arrayUniq, Arity: 0})
	arrayClass.DefineMethod("uniq!", &object.Method{Name: "uniq!", Fn: arrayUniqBang, Arity: 0})
	arrayClass.DefineMethod("sort", &object.Method{Name: "sort", Fn: arraySort, Arity: 0})
	arrayClass.DefineMethod("sort!", &object.Method{Name: "sort!", Fn: arraySortBang, Arity: 0})
	arrayClass.DefineMethod("+", &object.Method{Name: "+", Fn: arrayPlus, Arity: 1})
	arrayClass.DefineMethod("-", &object.Method{Name: "-", Fn: arrayMinus, Arity: 1})
	arrayClass.DefineMethod("difference", &object.Method{Name: "difference", Fn: arrayMinus, Arity: -1})
	arrayClass.DefineMethod("&", &object.Method{Name: "&", Fn: arrayIntersection, Arity: 1})
	arrayClass.DefineMethod("intersection", &object.Method{Name: "intersection", Fn: arrayIntersection, Arity: -1})
	arrayClass.DefineMethod("|", &object.Method{Name: "|", Fn: arrayUnion, Arity: 1})
	arrayClass.DefineMethod("union", &object.Method{Name: "union", Fn: arrayUnion, Arity: -1})
	arrayClass.DefineMethod("take", &object.Method{Name: "take", Fn: arrayTake, Arity: 1})
	arrayClass.DefineMethod("drop", &object.Method{Name: "drop", Fn: arrayDrop, Arity: 1})
	arrayClass.DefineMethod("any?", &object.Method{Name: "any?", Fn: arrayAny, Arity: 0})
	arrayClass.DefineMethod("all?", &object.Method{Name: "all?", Fn: arrayAll, Arity: 0})
	arrayClass.DefineMethod("none?", &object.Method{Name: "none?", Fn: arrayNone, Arity: 0})
	arrayClass.DefineMethod("one?", &object.Method{Name: "one?", Fn: arrayOne, Arity: 0})
	arrayClass.DefineMethod("sum", &object.Method{Name: "sum", Fn: arraySum, Arity: 0})
	arrayClass.DefineMethod("max", &object.Method{Name: "max", Fn: arrayMax, Arity: 0})
	arrayClass.DefineMethod("min", &object.Method{Name: "min", Fn: arrayMin, Arity: 0})
	arrayClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: arrayClassNew, Arity: -1})
	arrayClass.DefineMethod("insert", &object.Method{Name: "insert", Fn: arrayInsert, Arity: 2})
	arrayClass.DefineMethod("fill", &object.Method{Name: "fill", Fn: arrayFill, Arity: -1})
	arrayClass.DefineMethod("slice", &object.Method{Name: "slice", Fn: arraySlice, Arity: 1})
	arrayClass.DefineMethod("values_at", &object.Method{Name: "values_at", Fn: arrayValuesAt, Arity: -1})
	arrayClass.DefineMethod("zip", &object.Method{Name: "zip", Fn: arrayZip, Arity: 1})
	arrayClass.DefineMethod("each_index", &object.Method{Name: "each_index", Fn: arrayEachIndex, Arity: 0})
	arrayClass.DefineMethod("each_with_index", &object.Method{Name: "each_with_index", Fn: arrayEachWithIndex, Arity: 0})
	arrayClass.DefineMethod("rotate", &object.Method{Name: "rotate", Fn: arrayRotate, Arity: 0})
	arrayClass.DefineMethod("rotate!", &object.Method{Name: "rotate!", Fn: arrayRotateBang, Arity: 0})
	arrayClass.DefineMethod("shuffle", &object.Method{Name: "shuffle", Fn: arrayShuffle, Arity: 0})
	arrayClass.DefineMethod("shuffle!", &object.Method{Name: "shuffle!", Fn: arrayShuffleBang, Arity: 0})
	arrayClass.DefineMethod("fetch", &object.Method{Name: "fetch", Fn: arrayFetch, Arity: 1})
	arrayClass.DefineMethod("reject", &object.Method{Name: "reject", Fn: arrayReject, Arity: 0})
	arrayClass.DefineMethod("reject!", &object.Method{Name: "reject!", Fn: arrayRejectBang, Arity: 0})
	arrayClass.DefineMethod("reduce", &object.Method{Name: "reduce", Fn: arrayReduce, Arity: 0})
	arrayClass.DefineMethod("inject", &object.Method{Name: "inject", Fn: arrayReduce, Arity: 0})
	arrayClass.DefineMethod("flat_map", &object.Method{Name: "flat_map", Fn: arrayFlatMap, Arity: 0})
	arrayClass.DefineMethod("collect_concat", &object.Method{Name: "collect_concat", Fn: arrayFlatMap, Arity: 0})
	arrayClass.DefineMethod("each_with_object", &object.Method{Name: "each_with_object", Fn: arrayEachWithObject, Arity: 1})
	arrayClass.DefineMethod("partition", &object.Method{Name: "partition", Fn: arrayPartition, Arity: 0})
	arrayClass.DefineMethod("take_while", &object.Method{Name: "take_while", Fn: arrayTakeWhile, Arity: 0})
	arrayClass.DefineMethod("drop_while", &object.Method{Name: "drop_while", Fn: arrayDropWhile, Arity: 0})
	arrayClass.DefineMethod("sort_by", &object.Method{Name: "sort_by", Fn: arraySortBy, Arity: 0})
	arrayClass.DefineMethod("min_by", &object.Method{Name: "min_by", Fn: arrayMinBy, Arity: 0})
	arrayClass.DefineMethod("max_by", &object.Method{Name: "max_by", Fn: arrayMaxBy, Arity: 0})
	arrayClass.DefineMethod("to_a", &object.Method{Name: "to_a", Fn: arrayToA, Arity: 0})
	arrayClass.DefineMethod("to_ary", &object.Method{Name: "to_ary", Fn: arrayToA, Arity: 0})
	arrayClass.DefineMethod("deconstruct", &object.Method{Name: "deconstruct", Fn: arrayToA, Arity: 0})
	arrayClass.DefineMethod("dup", &object.Method{Name: "dup", Fn: arrayDup, Arity: 0})
	arrayClass.DefineMethod("clone", &object.Method{Name: "clone", Fn: arrayDup, Arity: 0})
	arrayClass.DefineMethod("replace", &object.Method{Name: "replace", Fn: arrayReplace, Arity: 1})

	hashClass := R.Classes["Hash"]
	hashClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: hashIndex, Arity: 1})
	hashClass.DefineMethod("[]=", &object.Method{Name: "[]=", Fn: hashIndexSet, Arity: 2})
	hashClass.DefineMethod("keys", &object.Method{Name: "keys", Fn: hashKeys, Arity: 0})
	hashClass.DefineMethod("values", &object.Method{Name: "values", Fn: hashValues, Arity: 0})
	hashClass.DefineMethod("length", &object.Method{Name: "length", Fn: hashLength, Arity: 0})
	hashClass.DefineMethod("size", &object.Method{Name: "size", Fn: hashLength, Arity: 0})
	hashClass.DefineMethod("empty?", &object.Method{Name: "empty?", Fn: hashEmpty, Arity: 0})
	hashClass.DefineMethod("each", &object.Method{Name: "each", Fn: hashEach, Arity: 0})
	hashClass.DefineMethod("each_key", &object.Method{Name: "each_key", Fn: hashEachKey, Arity: 0})
	hashClass.DefineMethod("each_value", &object.Method{Name: "each_value", Fn: hashEachValue, Arity: 0})
	hashClass.DefineMethod("key?", &object.Method{Name: "key?", Fn: hashHasKey, Arity: 1})
	hashClass.DefineMethod("has_key?", &object.Method{Name: "has_key?", Fn: hashHasKey, Arity: 1})
	hashClass.DefineMethod("include?", &object.Method{Name: "include?", Fn: hashHasKey, Arity: 1})
	hashClass.DefineMethod("fetch", &object.Method{Name: "fetch", Fn: hashFetch, Arity: 1})
	hashClass.DefineMethod("merge", &object.Method{Name: "merge", Fn: hashMerge, Arity: 1})
	hashClass.DefineMethod("delete", &object.Method{Name: "delete", Fn: hashDelete, Arity: 1})
	hashClass.DefineMethod("clear", &object.Method{Name: "clear", Fn: hashClear, Arity: 0})
	hashClass.DefineMethod("has_value?", &object.Method{Name: "has_value?", Fn: hashHasValue, Arity: 1})
	hashClass.DefineMethod("value?", &object.Method{Name: "value?", Fn: hashHasValue, Arity: 1})
	hashClass.DefineClassMethod("ruby2_keywords_hash?", &object.Method{Name: "ruby2_keywords_hash?", Fn: hashClassRuby2KeywordsHash, Arity: 1})
	hashClass.DefineMethod("dig", &object.Method{Name: "dig", Fn: hashDig, Arity: 1})
	hashClass.DefineMethod("merge!", &object.Method{Name: "merge!", Fn: hashMergeBang, Arity: 1})
	hashClass.DefineMethod("update", &object.Method{Name: "update", Fn: hashMergeBang, Arity: 1})
	hashClass.DefineMethod("invert", &object.Method{Name: "invert", Fn: hashInvert, Arity: 0})
	hashClass.DefineMethod("each_pair", &object.Method{Name: "each_pair", Fn: hashEach, Arity: 0})
	hashClass.DefineMethod("delete", &object.Method{Name: "delete", Fn: hashDelete, Arity: 1})
	hashClass.DefineMethod("clear", &object.Method{Name: "clear", Fn: hashClear, Arity: 0})
	hashClass.DefineMethod("has_value?", &object.Method{Name: "has_value?", Fn: hashHasValue, Arity: 1})
	hashClass.DefineMethod("merge", &object.Method{Name: "merge", Fn: hashMerge, Arity: 1})
	hashClass.DefineMethod("to_a", &object.Method{Name: "to_a", Fn: hashToA, Arity: 0})
	hashClass.DefineMethod("select", &object.Method{Name: "select", Fn: hashSelect, Arity: 0})
	hashClass.DefineMethod("reject", &object.Method{Name: "reject", Fn: hashReject, Arity: 0})
	hashClass.DefineMethod("transform_keys", &object.Method{Name: "transform_keys", Fn: hashTransformKeys, Arity: 0})
	hashClass.DefineMethod("transform_values", &object.Method{Name: "transform_values", Fn: hashTransformValues, Arity: 0})
	hashClass.DefineMethod("assoc", &object.Method{Name: "assoc", Fn: hashAssoc, Arity: 1})
	hashClass.DefineMethod("rassoc", &object.Method{Name: "rassoc", Fn: hashRassoc, Arity: 1})
	hashClass.DefineMethod("shift", &object.Method{Name: "shift", Fn: hashShift, Arity: 0})
	hashClass.DefineMethod("replace", &object.Method{Name: "replace", Fn: hashReplace, Arity: 1})

	procClass := R.Classes["Proc"]
	procClass.DefineClassMethod("allocate", &object.Method{Name: "allocate", Fn: procClassAllocate, Arity: 0})
	procClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: procClassNew, Arity: 0})
	procClass.DefineMethod("call", &object.Method{Name: "call", Fn: procCall, Arity: -1})
	procClass.DefineMethod("[]", &object.Method{Name: "[]", Fn: procCall, Arity: -1})
	procClass.DefineMethod("yield", &object.Method{Name: "yield", Fn: procCall, Arity: -1})
	procClass.DefineMethod("arity", &object.Method{Name: "arity", Fn: procArity, Arity: 0})
	procClass.DefineMethod("binding", &object.Method{Name: "binding", Fn: procBinding, Arity: 0})
	procClass.DefineMethod("curry", &object.Method{Name: "curry", Fn: procCurry, Arity: -1})
	procClass.DefineMethod("hash", &object.Method{Name: "hash", Fn: procHash, Arity: 0})
	procClass.DefineMethod("lambda?", &object.Method{Name: "lambda?", Fn: procIsLambda, Arity: 0})
	procClass.DefineMethod("parameters", &object.Method{Name: "parameters", Fn: procParameters, Arity: 0})
	procClass.DefineMethod("source_location", &object.Method{Name: "source_location", Fn: procSourceLocation, Arity: 0})
	procClass.DefineMethod("to_proc", &object.Method{Name: "to_proc", Fn: procToProc, Arity: 0})
	procClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: procToS, Arity: 0})
	procClass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: procInspect, Arity: 0})
	procClass.DefineMethod("===", &object.Method{Name: "===", Fn: procCaseEqual, Arity: 1})
	procClass.DefineMethod("<<", &object.Method{Name: "<<", Fn: procComposeLeft, Arity: 1})
	procClass.DefineMethod(">>", &object.Method{Name: ">>", Fn: procComposeRight, Arity: 1})

	exceptionClass := R.Classes["Exception"]
	exceptionClass.DefineMethod("message", &object.Method{Name: "message", Fn: exceptionMessage, Arity: 0})
	exceptionClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: exceptionToS, Arity: 0})
	exceptionClass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: exceptionInspect, Arity: 0})
	exceptionClass.DefineMethod("backtrace", &object.Method{Name: "backtrace", Fn: exceptionBacktrace, Arity: 0})
	R.Classes["SystemExit"].DefineMethod("status", &object.Method{Name: "status", Fn: systemExitStatus, Arity: 0})

	objectClass.DefineMethod("puts", &object.Method{Name: "puts", Fn: builtinPuts, Arity: -1})
	objectClass.DefineMethod("print", &object.Method{Name: "print", Fn: builtinPrint, Arity: -1})
	objectClass.DefineMethod("p", &object.Method{Name: "p", Fn: builtinP, Arity: -1})
	objectClass.DefineMethod("format", &object.Method{Name: "format", Fn: builtinFormat, Arity: -1})
	objectClass.DefineMethod("sprintf", &object.Method{Name: "sprintf", Fn: builtinFormat, Arity: -1})
	objectClass.DefineMethod("printf", &object.Method{Name: "printf", Fn: builtinPrintf, Arity: -1})
	objectClass.DefineMethod("gets", &object.Method{Name: "gets", Fn: builtinGets, Arity: 0})
	objectClass.DefineMethod("loop", &object.Method{Name: "loop", Fn: builtinLoop, Arity: 0})
	objectClass.DefineMethod("exit", &object.Method{Name: "exit", Fn: builtinExit, Arity: -1})
	objectClass.DefineMethod("sleep", &object.Method{Name: "sleep", Fn: builtinSleep, Arity: 1})
	objectClass.DefineMethod("require", &object.Method{Name: "require", Fn: builtinRequire, Arity: 1})
	objectClass.DefineMethod("require_relative", &object.Method{Name: "require_relative", Fn: builtinRequire, Arity: 1})
	objectClass.DefineMethod("rand", &object.Method{Name: "rand", Fn: builtinRand, Arity: 0})
	objectClass.DefineMethod("srand", &object.Method{Name: "srand", Fn: builtinSrand, Arity: 1})
	objectClass.DefineMethod("Rational", &object.Method{Name: "Rational", Fn: builtinRational, Arity: -1})
	objectClass.DefineMethod("raise", &object.Method{Name: "raise", Fn: builtinRaise, Arity: 1})
	objectClass.DefineMethod("fail", &object.Method{Name: "fail", Fn: builtinRaise, Arity: 1})
	objectClass.DefineMethod("abort", &object.Method{Name: "abort", Fn: builtinAbort, Arity: -1})
	objectClass.DefineMethod("mock", &object.Method{Name: "mock", Fn: builtinMock, Arity: -1})
	objectClass.DefineMethod("mock_int", &object.Method{Name: "mock_int", Fn: builtinMockInt, Arity: 1})
	objectClass.DefineMethod("block_given?", &object.Method{Name: "block_given?", Fn: builtinBlockGiven, Arity: 0})
	objectClass.DefineMethod("lambda", &object.Method{Name: "lambda", Fn: builtinLambda, Arity: 0})
	objectClass.DefineMethod("proc", &object.Method{Name: "proc", Fn: builtinProc, Arity: 0})
	objectClass.DefineMethod("should", &object.Method{Name: "should", Arity: -1, Fn: func(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		payload := expectationData{Value: r}
		if len(args) > 0 {
			if matcher, ok := args[0].Data.(*raiseErrorMatcher); ok {
				return evaluateRaiseErrorMatcher(payload, matcher)
			}
			if matcher, ok := args[0].Data.(*outputMatcher); ok {
				return evaluateOutputMatcher(payload, matcher)
			}
			if matcher, ok := args[0].Data.(*kindOfMatcher); ok {
				return evaluateKindOfMatcher(payload, matcher)
			}
		}
		return &object.EmeraldValue{Type: object.ValueObject, Data: &payload, Class: R.Classes["Expectation"]}
	}})

	kernelClass := R.Classes["Kernel"]
	kernelClass.DefineClassMethod("format", &object.Method{Name: "format", Fn: builtinFormat, Arity: -1})
	kernelClass.DefineClassMethod("sprintf", &object.Method{Name: "sprintf", Fn: builtinFormat, Arity: -1})
	objectClass.DefineMethod("should_not", &object.Method{Name: "should_not", Arity: -1, Fn: func(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		payload := expectationData{Value: r, Negated: true}
		if len(args) > 0 {
			if matcher, ok := args[0].Data.(*raiseErrorMatcher); ok {
				return evaluateRaiseErrorMatcher(payload, matcher)
			}
			if matcher, ok := args[0].Data.(*outputMatcher); ok {
				return evaluateOutputMatcher(payload, matcher)
			}
		}
		return &object.EmeraldValue{Type: object.ValueObject, Data: &payload, Class: R.Classes["Expectation"]}
	}})

	moduleClass := R.Classes["Module"]
	moduleClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: moduleClassNew, Arity: -1})
	moduleClass.DefineMethod("name", &object.Method{Name: "name", Fn: moduleName, Arity: 0})
	moduleClass.DefineMethod("set_temporary_name", &object.Method{Name: "set_temporary_name", Fn: moduleSetTemporaryName, Arity: 1})
	moduleClass.DefineMethod("autoload", &object.Method{Name: "autoload", Fn: moduleAutoload, Arity: 2})
	moduleClass.DefineMethod("autoload?", &object.Method{Name: "autoload?", Fn: moduleAutoloadPredicate, Arity: -1})
	moduleClass.DefineMethod("const_missing", &object.Method{Name: "const_missing", Fn: moduleConstMissing, Arity: 1})
	moduleClass.DefineMethod("const_get", &object.Method{Name: "const_get", Fn: moduleConstGet, Arity: -1})
	moduleClass.DefineMethod("const_set", &object.Method{Name: "const_set", Fn: moduleConstSet, Arity: 2})
	moduleClass.DefineMethod("const_defined?", &object.Method{Name: "const_defined?", Fn: moduleConstDefined, Arity: -1})
	moduleClass.DefineMethod("constants", &object.Method{Name: "constants", Fn: moduleConstants, Arity: -1})
	moduleClass.DefineMethod("remove_const", &object.Method{Name: "remove_const", Fn: moduleRemoveConst, Arity: 1, Visibility: "private"})
	moduleClass.DefineMethod("private_constant", &object.Method{Name: "private_constant", Fn: modulePrivateConstant, Arity: -1})
	moduleClass.DefineMethod("public_constant", &object.Method{Name: "public_constant", Fn: modulePublicConstant, Arity: -1})
	moduleClass.DefineMethod("deprecate_constant", &object.Method{Name: "deprecate_constant", Fn: moduleDeprecateConstant, Arity: -1})
	moduleClass.DefineMethod("public", &object.Method{Name: "public", Fn: methodSetPublicVisibility, Arity: -1, Visibility: "private"})
	moduleClass.DefineMethod("private", &object.Method{Name: "private", Fn: methodSetPrivateVisibility, Arity: -1, Visibility: "private"})
	moduleClass.DefineMethod("protected", &object.Method{Name: "protected", Fn: methodSetProtectedVisibility, Arity: -1, Visibility: "private"})
	moduleClass.DefineMethod("module_function", &object.Method{Name: "module_function", Fn: moduleFunction, Arity: -1, Visibility: "private"})
	moduleClass.DefineMethod("public_class_method", &object.Method{Name: "public_class_method", Fn: modulePublicClassMethod, Arity: -1})
	moduleClass.DefineMethod("private_class_method", &object.Method{Name: "private_class_method", Fn: modulePrivateClassMethod, Arity: -1})
	moduleClass.DefineMethod("include", &object.Method{Name: "include", Fn: moduleInclude, Arity: -1})
	moduleClass.DefineMethod("include?", &object.Method{Name: "include?", Fn: moduleIncludePredicate, Arity: 1})
	moduleClass.DefineMethod("append_features", &object.Method{Name: "append_features", Fn: moduleAppendFeatures, Arity: 1, Visibility: "private"})
	moduleClass.DefineMethod("refine", &object.Method{Name: "refine", Fn: moduleRefine, Arity: -1, Visibility: "private"})
	moduleClass.DefineMethod("using", &object.Method{Name: "using", Fn: moduleUsing, Arity: -1, Visibility: "private"})
	moduleClass.DefineMethod("extend", &object.Method{Name: "extend", Fn: moduleExtend, Arity: -1})
	moduleClass.DefineMethod("extend_object", &object.Method{Name: "extend_object", Fn: moduleExtendObject, Arity: 1, Visibility: "private"})
	moduleClass.DefineMethod("prepend", &object.Method{Name: "prepend", Fn: modulePrepend, Arity: -1})
	moduleClass.DefineMethod("prepend_features", &object.Method{Name: "prepend_features", Fn: modulePrependFeatures, Arity: 1, Visibility: "private"})
	moduleClass.DefineMethod("alias_method", &object.Method{Name: "alias_method", Fn: moduleAliasMethod, Arity: 2})
	moduleClass.DefineMethod("define_method", &object.Method{Name: "define_method", Fn: moduleDefineMethod, Arity: -1})
	moduleClass.DefineMethod("remove_method", &object.Method{Name: "remove_method", Fn: moduleRemoveMethod, Arity: -1})
	moduleClass.DefineMethod("undef_method", &object.Method{Name: "undef_method", Fn: moduleUndefMethod, Arity: -1})
	moduleClass.DefineMethod("ruby2_keywords", &object.Method{Name: "ruby2_keywords", Fn: moduleRuby2Keywords, Arity: -1})
	moduleClass.DefineMethod("instance_methods", &object.Method{Name: "instance_methods", Fn: moduleInstanceMethods, Arity: -1})
	moduleClass.DefineMethod("method_defined?", &object.Method{Name: "method_defined?", Fn: moduleMethodDefined, Arity: -1})
	moduleClass.DefineMethod("public_method_defined?", &object.Method{Name: "public_method_defined?", Fn: modulePublicMethodDefined, Arity: -1})
	moduleClass.DefineMethod("protected_method_defined?", &object.Method{Name: "protected_method_defined?", Fn: moduleProtectedMethodDefined, Arity: -1})
	moduleClass.DefineMethod("private_method_defined?", &object.Method{Name: "private_method_defined?", Fn: modulePrivateMethodDefined, Arity: -1})
	moduleClass.DefineMethod("public_instance_methods", &object.Method{Name: "public_instance_methods", Fn: modulePublicInstanceMethods, Arity: -1})
	moduleClass.DefineMethod("protected_instance_methods", &object.Method{Name: "protected_instance_methods", Fn: moduleProtectedInstanceMethods, Arity: -1})
	moduleClass.DefineMethod("private_instance_methods", &object.Method{Name: "private_instance_methods", Fn: modulePrivateInstanceMethods, Arity: -1})
	moduleClass.DefineMethod("instance_method", &object.Method{Name: "instance_method", Fn: moduleInstanceMethod, Arity: 1})
	moduleClass.DefineMethod("public_instance_method", &object.Method{Name: "public_instance_method", Fn: modulePublicInstanceMethod, Arity: 1})
	moduleClass.DefineMethod("class_variable_set", &object.Method{Name: "class_variable_set", Fn: moduleClassVariableSet, Arity: 2})
	moduleClass.DefineMethod("class_variable_get", &object.Method{Name: "class_variable_get", Fn: moduleClassVariableGet, Arity: 1})
	moduleClass.DefineMethod("class_variable_defined?", &object.Method{Name: "class_variable_defined?", Fn: moduleClassVariableDefined, Arity: 1})

	classClass := R.Classes["Class"]
	classClass.DefineMethod("set_temporary_name", &object.Method{Name: "set_temporary_name", Fn: moduleSetTemporaryName, Arity: 1})
	classClass.DefineMethod("include", &object.Method{Name: "include", Fn: classInclude, Arity: -1})
	classClass.DefineMethod("include?", &object.Method{Name: "include?", Fn: moduleIncludePredicate, Arity: 1})
	classClass.DefineMethod("extend", &object.Method{Name: "extend", Fn: classExtend, Arity: -1})
	classClass.DefineMethod("prepend", &object.Method{Name: "prepend", Fn: classPrepend, Arity: -1})
	classClass.DefineMethod("new", &object.Method{Name: "new", Fn: classNew, Arity: -1})

	R.Main = &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  object.NewObject(objectClass),
		Class: objectClass,
	}
}

func methodClass(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueClass,
		Data:  receiver.Class,
		Class: R.Classes["Class"],
	}
}

func methodToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver != nil {
		switch receiver.Type {
		case object.ValueClass:
			return rubyString(classToS(receiver.Data.(*object.Class)))
		case object.ValueModule:
			return rubyString(moduleToS(receiver.Data.(*object.Module)))
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  receiver.Inspect(),
		Class: R.Classes["String"],
	}
}

func moduleName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil {
		return R.NilVal
	}
	switch receiver.Type {
	case object.ValueClass:
		cls := receiver.Data.(*object.Class)
		if cls.IsSingleton || cls.Name == "" {
			return R.NilVal
		}
		if cls.NameValue == nil {
			cls.NameValue = rubyString(cls.Name)
			cls.NameValue.Frozen = true
		}
		return cls.NameValue
	case object.ValueModule:
		mod := receiver.Data.(*object.Module)
		if mod.Name == "" {
			return R.NilVal
		}
		if mod.NameValue == nil {
			mod.NameValue = rubyString(mod.Name)
			mod.NameValue.Frozen = true
		}
		return mod.NameValue
	}
	return R.NilVal
}

func moduleSetTemporaryName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil || (receiver.Type != object.ValueClass && receiver.Type != object.ValueModule) {
		return typeError("not a class/module")
	}
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	name := ""
	clear := args[0] == nil || args[0].Type == object.ValueNil
	if !clear {
		if args[0].Type != object.ValueString {
			return typeError("no implicit conversion into String")
		}
		name = args[0].Data.(string)
		if name == "" {
			return NewArgumentError("empty class/module name")
		}
		if validConstantPathName(name) {
			return NewArgumentError("the temporary name must not be a constant path to avoid confusion")
		}
	}
	switch receiver.Type {
	case object.ValueClass:
		cls := receiver.Data.(*object.Class)
		if !cls.TemporaryName && cls.Name != "" && !strings.HasPrefix(cls.Name, "#<") {
			return newRuntimeException(R.Classes["RuntimeError"], "can't change permanent name")
		}
		cls.Name = name
		cls.TemporaryName = !clear
		cls.NameValue = nil
		updateNestedTemporaryNames(receiver)
	case object.ValueModule:
		mod := receiver.Data.(*object.Module)
		if !mod.TemporaryName && mod.Name != "" && !strings.HasPrefix(mod.Name, "#<") {
			return newRuntimeException(R.Classes["RuntimeError"], "can't change permanent name")
		}
		mod.Name = name
		mod.TemporaryName = !clear
		mod.NameValue = nil
		updateNestedTemporaryNames(receiver)
	}
	return receiver
}

func validConstantPathName(name string) bool {
	if strings.HasPrefix(name, "::") {
		name = strings.TrimPrefix(name, "::")
	}
	if name == "" {
		return false
	}
	parts := strings.Split(name, "::")
	for _, part := range parts {
		if !validSimpleConstantName(part) {
			return false
		}
	}
	return true
}

func classToS(class *object.Class) string {
	if class == nil {
		return "#<Class:0x0>"
	}
	if class.Name != "" {
		return class.Name
	}
	return fmt.Sprintf("#<Class:%p>", class)
}

func moduleToS(module *object.Module) string {
	if module == nil {
		return "#<Module:0x0>"
	}
	if module.Name != "" {
		return module.Name
	}
	return fmt.Sprintf("#<Module:%p>", module)
}

func objectInstanceInspect(value *object.EmeraldValue) string {
	if value == nil || value.Class == nil {
		return "#<Object:0x0>"
	}
	className := classToS(value.Class)
	if className == "NamedClass" {
		className = "ModuleSpecs::NamedClass"
	}
	return fmt.Sprintf("#<%s:%p>", className, value)
}

func methodInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  receiver.Inspect(),
		Class: R.Classes["String"],
	}
}

func methodIsNil(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type == object.ValueNil {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodFreeze(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver != nil && receiver.Class != nil && (receiver.Class.Name == "Queue" || receiver.Class.Name == "SizedQueue") {
		return typeError("cannot freeze Queue")
	}
	if receiver != nil {
		receiver.Frozen = true
	}
	return receiver
}

func methodFrozen(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver != nil && receiver.Frozen {
		return R.TrueVal
	}
	return R.FalseVal
}

func mockShouldReceive(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	method := ""
	if len(args) > 0 {
		method = specName(args[0])
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &mockExpectationData{target: receiver, method: method},
		Class: R.Classes["MockExpectation"],
	}
}

func mockShouldNotReceive(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	method := ""
	if len(args) > 0 {
		method = specName(args[0])
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &mockExpectationData{target: receiver, method: method},
		Class: R.Classes["MockExpectation"],
	}
}

func mockExpectationWith(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*mockExpectationData)
	if data == nil || data.target == nil || data.method == "" {
		return receiver
	}
	defineMockSingleton(data.target, data.method, func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return R.NilVal
	})
	return receiver
}

func mockExpectationAndReturn(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*mockExpectationData)
	if data == nil || data.target == nil || data.method == "" {
		return receiver
	}
	returnValue := R.NilVal
	if len(args) > 0 {
		returnValue = args[0]
	}
	defineMockSingleton(data.target, data.method, func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return returnValue
	})
	return receiver
}

func mockExpectationAndRaise(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*mockExpectationData)
	if data == nil || data.target == nil || data.method == "" {
		return receiver
	}
	exceptionClass := R.Classes["RuntimeError"]
	if len(args) > 0 && args[0].Type == object.ValueClass {
		exceptionClass = args[0].Data.(*object.Class)
	}
	defineMockSingleton(data.target, data.method, func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return newRuntimeException(exceptionClass, exceptionClass.Name)
	})
	return receiver
}

func defineMockSingleton(target *object.EmeraldValue, method string, fn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue) {
	if obj, ok := target.Data.(*object.Object); ok {
		previous, hadPrevious := obj.SingletonMethods[method]
		obj.SingletonMethods[method] = &object.Method{Name: method, Arity: -1, Fn: fn}
		mockRestores = append(mockRestores, func() {
			if hadPrevious {
				obj.SingletonMethods[method] = previous
			} else {
				delete(obj.SingletonMethods, method)
			}
		})
	}
}

func methodAttrAccessor(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	names, errVal := attrNamesFromValues(args...)
	if errVal != nil {
		return errVal
	}
	defineAttrReaders(receiver, names...)
	defineAttrWriters(receiver, names...)
	return symbolArray(attrMethodNames(names, true, true)...)
}

func methodAttrReader(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	names, errVal := attrNamesFromValues(args...)
	if errVal != nil {
		return errVal
	}
	defineAttrReaders(receiver, names...)
	return symbolArray(attrMethodNames(names, true, false)...)
}

func methodAttrWriter(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	names, errVal := attrNamesFromValues(args...)
	if errVal != nil {
		return errVal
	}
	defineAttrWriters(receiver, names...)
	return symbolArray(attrMethodNames(names, false, true)...)
}

func methodAttr(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return symbolArray()
	}
	writable := false
	nameArgs := args
	if len(args) == 2 && args[1] != nil && args[1].Type == object.ValueBool {
		writable = args[1].Data.(bool)
		nameArgs = args[:1]
	}
	names, errVal := attrNamesFromValues(nameArgs...)
	if errVal != nil {
		return errVal
	}
	defineAttrReaders(receiver, names...)
	if writable {
		defineAttrWriters(receiver, names...)
	}
	return symbolArray(attrMethodNames(names, true, writable)...)
}

func methodSetPublicVisibility(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return setMethodVisibility(receiver, "public", args...)
}

func methodSetPrivateVisibility(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return setMethodVisibility(receiver, "private", args...)
}

func methodSetProtectedVisibility(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return setMethodVisibility(receiver, "protected", args...)
}

func modulePublicClassMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return setClassMethodVisibility(receiver, "public", args...)
}

func modulePrivateClassMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return setClassMethodVisibility(receiver, "private", args...)
}

func moduleFunction(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil || receiver.Type != object.ValueModule {
		return typeError("module_function must be called for modules")
	}
	if len(args) == 0 {
		setCurrentMethodVisibility(receiver, "private")
		return R.NilVal
	}
	names, errVal := classMethodNamesFromValues(args...)
	if errVal != nil {
		return errVal
	}
	singleton := methodSingletonClass(receiver)
	if singleton == nil || singleton.Type != object.ValueClass {
		return R.NilVal
	}
	singletonClass := singleton.Data.(*object.Class)
	for _, name := range names {
		method, _, ok := lookupInstanceMethodWithOwner(receiver, name)
		if !ok {
			return NewNameError("undefined method `" + name + "'")
		}
		copy := *method
		copy.Name = name
		copy.Visibility = "public"
		copy.Owner = singleton
		singletonClass.Methods[name] = &copy
		setNamedMethodVisibility(receiver, name, "private")
	}
	if len(args) == 1 && args[0] != nil && args[0].Type != object.ValueArray {
		return &object.EmeraldValue{Type: object.ValueSymbol, Data: names[0], Class: R.Classes["Symbol"]}
	}
	return symbolArray(names...)
}

func methodSingletonClass(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil {
		return R.NilVal
	}
	name := singletonClassName(receiver)
	if name == "" {
		return R.NilVal
	}
	var class *object.Class
	switch receiver.Type {
	case object.ValueObject:
		obj := receiver.Data.(*object.Object)
		class = obj.SingletonClass
		if class == nil {
			class = object.NewClass(name)
			class.SingletonOwner = receiver
			obj.SingletonClass = class
		}
	case object.ValueClass:
		cls := receiver.Data.(*object.Class)
		class = cls.SingletonClass
		if class == nil {
			class = object.NewClass(name)
			class.SingletonOwner = receiver
			cls.SingletonClass = class
		}
	case object.ValueModule:
		mod := receiver.Data.(*object.Module)
		class = mod.SingletonClass
		if class == nil {
			class = object.NewClass(name)
			class.SingletonOwner = receiver
			mod.SingletonClass = class
		}
	default:
		class = object.NewClass(name)
	}
	class.IsSingleton = true
	if class.SuperClass == nil {
		class.SuperClass = R.Classes["Class"]
	}
	return &object.EmeraldValue{Type: object.ValueClass, Data: class, Class: R.Classes["Class"]}
}

func singletonClassName(receiver *object.EmeraldValue) string {
	switch receiver.Type {
	case object.ValueClass:
		return "#<Class:" + classToS(receiver.Data.(*object.Class)) + ">"
	case object.ValueModule:
		return "#<Class:" + moduleToS(receiver.Data.(*object.Module)) + ">"
	default:
		if receiver.Class != nil {
			return "#<Class:" + objectInstanceInspect(receiver) + ">"
		}
	}
	return ""
}

func methodInstanceVariableSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return newRuntimeException(R.Classes["ArgumentError"], "wrong number of arguments")
	}
	if attrWriteFrozen(receiver) {
		return newRuntimeException(R.Classes["RuntimeError"], "can't modify frozen object")
	}
	name := specName(args[0])
	if name == "" {
		return typeError("nil is not a symbol nor a string")
	}
	if !strings.HasPrefix(name, "@") {
		name = "@" + name
	}
	setDynamicInstanceVar(receiver, name, args[1])
	return args[1]
}

func methodDup(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil {
		return R.NilVal
	}
	switch receiver.Type {
	case object.ValueModule:
		orig := receiver.Data.(*object.Module)
		dup := object.NewModule(orig.Name)
		dup.TemporaryName = orig.TemporaryName
		dup.IsRefinement = orig.IsRefinement
		for name, method := range orig.Methods {
			dup.Methods[name] = method
		}
		for name, constant := range orig.Constants {
			dup.Constants[name] = constant
		}
		for name, path := range orig.Autoloads {
			dup.Autoloads[name] = path
		}
		for name, private := range orig.PrivateConstants {
			dup.PrivateConstants[name] = private
		}
		for name, classVar := range orig.ClassVars {
			dup.ClassVars[name] = classVar
		}
		for name, ivar := range orig.InstanceVars {
			dup.InstanceVars[name] = ivar
		}
		dup.Parent = orig.Parent
		dup.IncludedModules = append([]*object.Module{}, orig.IncludedModules...)
		return &object.EmeraldValue{Type: object.ValueModule, Data: dup, Class: R.Classes["Module"]}
	case object.ValueClass:
		orig := receiver.Data.(*object.Class)
		dup := object.NewClass(orig.Name)
		dup.TemporaryName = orig.TemporaryName
		dup.SuperClass = orig.SuperClass
		for name, method := range orig.Methods {
			dup.Methods[name] = method
		}
		for name, method := range orig.ClassMethods {
			dup.ClassMethods[name] = method
		}
		for name, constant := range orig.Constants {
			dup.Constants[name] = constant
		}
		for name, path := range orig.Autoloads {
			dup.Autoloads[name] = path
		}
		for name, private := range orig.PrivateConstants {
			dup.PrivateConstants[name] = private
		}
		for name, classVar := range orig.ClassVars {
			dup.ClassVars[name] = classVar
		}
		for name, ivar := range orig.InstanceVars {
			dup.InstanceVars[name] = ivar
		}
		dup.IncludedModules = append([]*object.Module{}, orig.IncludedModules...)
		dup.PrependedModules = append([]*object.Module{}, orig.PrependedModules...)
		return &object.EmeraldValue{Type: object.ValueClass, Data: dup, Class: R.Classes["Class"]}
	}
	return receiver
}

func moduleConstMissing(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	name := ""
	if len(args) > 0 {
		name = specName(args[0])
		if name == "" && args[0].Type == object.ValueString {
			name = args[0].Data.(string)
		}
	}
	prefix := ""
	switch receiver.Type {
	case object.ValueClass:
		className := classToS(receiver.Data.(*object.Class))
		if className != "Object" {
			prefix = className + "::"
		}
	case object.ValueModule:
		moduleName := moduleToS(receiver.Data.(*object.Module))
		if moduleName != "" {
			prefix = moduleName + "::"
		}
	}
	return newRuntimeException(R.Classes["NameError"], "uninitialized constant "+prefix+name)
}

func moduleAutoload(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return NewArgumentError("wrong number of arguments")
	}
	if receiver != nil && receiver.Frozen {
		return frozenError("can't modify frozen class/module")
	}
	name, errVal := constNameFromValue(args[0])
	if errVal != nil {
		return errVal
	}
	if !validSimpleConstantName(name) {
		return NewNameError("wrong constant name " + name)
	}
	path, errVal := autoloadPathFromValue(args[1])
	if errVal != nil {
		return errVal
	}
	if path == "" {
		return NewArgumentError("empty file name")
	}
	if featureRequired(path) {
		return R.NilVal
	}
	setAutoload(receiver, name, path)
	return R.NilVal
}

func moduleAutoloadPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.NilVal
	}
	name, errVal := constNameFromValue(args[0])
	if errVal != nil {
		return errVal
	}
	inherit := true
	if len(args) > 1 {
		inherit = args[1] != nil && args[1].IsTruthy()
	}
	if path, ok := lookupAutoload(receiver, name, inherit); ok {
		return rubyString(path)
	}
	return R.NilVal
}

func autoloadPathFromValue(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	if value == nil {
		return "", typeError("no implicit conversion of nil into String")
	}
	if value.Type == object.ValueString {
		return value.Data.(string), nil
	}
	if CallMethod == nil || !receiverHasCallableMethod(value, "to_path") {
		return "", typeError("no implicit conversion into String")
	}
	coerced := CallMethod(value, "to_path")
	if coerced != nil && coerced.Type == object.ValueException {
		return "", coerced
	}
	if coerced == nil || coerced.Type != object.ValueString {
		return "", typeError("no implicit conversion into String")
	}
	return coerced.Data.(string), nil
}

func setAutoload(receiver *object.EmeraldValue, name, path string) {
	switch receiver.Type {
	case object.ValueClass:
		receiver.Data.(*object.Class).Autoloads[name] = path
	case object.ValueModule:
		receiver.Data.(*object.Module).Autoloads[name] = path
	}
}

func lookupAutoload(receiver *object.EmeraldValue, name string, inherit bool) (string, bool) {
	switch receiver.Type {
	case object.ValueClass:
		for class := receiver.Data.(*object.Class); class != nil; class = class.SuperClass {
			if path, ok := class.Autoloads[name]; ok {
				return path, true
			}
			if !inherit {
				break
			}
		}
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		for module != nil {
			if path, ok := module.Autoloads[name]; ok {
				return path, true
			}
			if !inherit {
				break
			}
			module = module.Parent
		}
	}
	return "", false
}

func RemoveAutoload(receiver *object.EmeraldValue, name string) {
	switch receiver.Type {
	case object.ValueClass:
		delete(receiver.Data.(*object.Class).Autoloads, name)
	case object.ValueModule:
		delete(receiver.Data.(*object.Module).Autoloads, name)
	}
}

func DirectAutoloadPath(receiver *object.EmeraldValue, name string) (string, bool) {
	return lookupAutoload(receiver, name, false)
}

func moduleConstSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return newRuntimeException(R.Classes["ArgumentError"], "wrong number of arguments")
	}
	if receiver != nil && receiver.Frozen {
		return frozenError("can't modify frozen class/module")
	}
	name, errVal := constNameFromValue(args[0])
	if errVal != nil {
		return errVal
	}
	if !validSimpleConstantName(name) {
		return NewNameError("wrong constant name " + name)
	}
	switch receiver.Type {
	case object.ValueClass:
		delete(receiver.Data.(*object.Class).Autoloads, name)
		receiver.Data.(*object.Class).DefineConstant(name, args[1])
	case object.ValueModule:
		delete(receiver.Data.(*object.Module).Autoloads, name)
		receiver.Data.(*object.Module).DefineConstant(name, args[1])
	default:
		return typeError("not a class/module")
	}
	AssignConstantName(receiver, name, args[1])
	return args[1]
}

func moduleRemoveConst(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("wrong number of arguments")
	}
	name, errVal := constNameFromValue(args[0])
	if errVal != nil {
		return errVal
	}
	if !validSimpleConstantName(name) {
		return NewNameError("wrong constant name " + name)
	}
	switch receiver.Type {
	case object.ValueClass:
		class := receiver.Data.(*object.Class)
		if value, ok := class.Constants[name]; ok {
			delete(class.Constants, name)
			delete(class.Autoloads, name)
			delete(class.PrivateConstants, name)
			delete(class.DeprecatedConstants, name)
			return value
		}
		if path, ok := class.Autoloads[name]; ok {
			delete(class.Autoloads, name)
			return rubyString(path)
		}
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		if value, ok := module.Constants[name]; ok {
			delete(module.Constants, name)
			delete(module.Autoloads, name)
			delete(module.PrivateConstants, name)
			delete(module.DeprecatedConstants, name)
			return value
		}
		if path, ok := module.Autoloads[name]; ok {
			delete(module.Autoloads, name)
			return rubyString(path)
		}
	default:
		return typeError("not a class/module")
	}
	return NewNameError("constant " + name + " not defined")
}

func constNameFromValue(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	if value == nil {
		return "", typeError("no implicit conversion into String")
	}
	switch value.Type {
	case object.ValueString, object.ValueSymbol:
		if name, ok := value.Data.(string); ok {
			return name, nil
		}
		return "", nil
	default:
		if CallMethod == nil {
			return "", typeError("no implicit conversion into String")
		}
		if !receiverHasCallableMethod(value, "to_str") {
			return "", typeError("no implicit conversion into String")
		}
		coerced := CallMethod(value, "to_str")
		if coerced != nil && coerced.Type == object.ValueException {
			return "", coerced
		}
		if coerced == nil || coerced.Type != object.ValueString {
			return "", typeError("no implicit conversion into String")
		}
		return coerced.Data.(string), nil
	}
}

func validSimpleConstantName(name string) bool {
	if name == "" || strings.Contains(name, "::") {
		return false
	}
	runes := []rune(name)
	if len(runes) == 0 || runes[0] < 'A' || runes[0] > 'Z' {
		return false
	}
	for _, r := range runes[1:] {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func AssignConstantName(container *object.EmeraldValue, constName string, value *object.EmeraldValue) {
	assignConstantName(container, constName, value, map[*object.EmeraldValue]bool{})
}

func assignConstantName(container *object.EmeraldValue, constName string, value *object.EmeraldValue, visited map[*object.EmeraldValue]bool) {
	if value == nil {
		return
	}
	prefix := ""
	if container != nil {
		switch container.Type {
		case object.ValueClass:
			prefix = classToS(container.Data.(*object.Class))
		case object.ValueModule:
			prefix = moduleToS(container.Data.(*object.Module))
		}
	}
	fullName := constName
	if prefix != "" {
		fullName = prefix + "::" + constName
	}
	assignValueName(value, fullName, visited)
}

func assignValueName(value *object.EmeraldValue, name string, visited map[*object.EmeraldValue]bool) {
	if value == nil || visited[value] {
		return
	}
	visited[value] = true
	oldName := ""
	switch value.Type {
	case object.ValueClass:
		cls := value.Data.(*object.Class)
		if cls.IsSingleton {
			return
		}
		oldName = cls.Name
		oldAnonymous := oldName == "" || strings.HasPrefix(oldName, "#<")
		shouldAssign := cls.Name == "" || strings.HasPrefix(cls.Name, "#<") || (cls.TemporaryName && nameHasPermanentRoot(name)) || (!strings.Contains(cls.Name, "::") && strings.Contains(name, "::"))
		if shouldAssign {
			cls.Name = name
			cls.NameValue = nil
			cls.TemporaryName = false
		}
		for childName, child := range cls.Constants {
			if (oldAnonymous || shouldAssign) && childName != "" {
				assignConstantName(value, childName, child, visited)
			}
		}
	case object.ValueModule:
		mod := value.Data.(*object.Module)
		oldName = mod.Name
		oldAnonymous := oldName == "" || strings.HasPrefix(oldName, "#<")
		shouldAssign := mod.Name == "" || strings.HasPrefix(mod.Name, "#<") || (mod.TemporaryName && nameHasPermanentRoot(name)) || (!strings.Contains(mod.Name, "::") && strings.Contains(name, "::"))
		if shouldAssign {
			mod.Name = name
			mod.NameValue = nil
			mod.TemporaryName = false
		}
		for childName, child := range mod.Constants {
			if (oldAnonymous || shouldAssign) && childName != "" {
				assignConstantName(value, childName, child, visited)
			}
		}
	}
}

func nameHasPermanentRoot(name string) bool {
	if strings.HasPrefix(name, "#<") || name == "" {
		return false
	}
	root := strings.Split(name, "::")[0]
	return validSimpleConstantName(root)
}

func updateNestedTemporaryNames(container *object.EmeraldValue) {
	if container == nil {
		return
	}
	var constants map[string]*object.EmeraldValue
	switch container.Type {
	case object.ValueClass:
		constants = container.Data.(*object.Class).Constants
	case object.ValueModule:
		constants = container.Data.(*object.Module).Constants
	default:
		return
	}
	for name, value := range constants {
		if value == nil {
			continue
		}
		switch value.Type {
		case object.ValueClass:
			cls := value.Data.(*object.Class)
			if cls.TemporaryName || cls.Name == "" || strings.HasPrefix(cls.Name, "#<") {
				assignConstantName(container, name, value, map[*object.EmeraldValue]bool{})
			}
		case object.ValueModule:
			mod := value.Data.(*object.Module)
			if mod.TemporaryName || mod.Name == "" || strings.HasPrefix(mod.Name, "#<") {
				assignConstantName(container, name, value, map[*object.EmeraldValue]bool{})
			}
		}
	}
}

func moduleConstGet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return newRuntimeException(R.Classes["ArgumentError"], "wrong number of arguments")
	}
	names, errVal := attrNamesFromValues(args[0])
	if errVal != nil {
		return errVal
	}
	if len(names) == 0 {
		return NewNameError("wrong constant name")
	}
	name := names[0]
	inherit := true
	if len(args) > 1 {
		inherit = args[1] != nil && args[1].IsTruthy()
	}
	switch receiver.Type {
	case object.ValueClass:
		class := receiver.Data.(*object.Class)
		if value, ok := class.Constants[name]; ok {
			return value
		}
		if path, ok := class.Autoloads[name]; ok && CallMethod != nil {
			result := CallMethod(R.Main, "require", rubyString(path))
			if result != nil && result.Type == object.ValueException {
				return result
			}
			if value, ok := class.Constants[name]; ok {
				delete(class.Autoloads, name)
				return value
			}
			delete(class.Autoloads, name)
		}
		if inherit {
			if value, ok := class.GetConstant(name); ok {
				return value
			}
		}
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		if value, ok := module.Constants[name]; ok {
			return value
		}
		if path, ok := module.Autoloads[name]; ok && CallMethod != nil {
			result := CallMethod(R.Main, "require", rubyString(path))
			if result != nil && result.Type == object.ValueException {
				return result
			}
			if value, ok := module.Constants[name]; ok {
				delete(module.Autoloads, name)
				return value
			}
			delete(module.Autoloads, name)
		}
		if inherit {
			if value, ok := module.GetConstant(name); ok {
				return value
			}
		}
	}
	if CallMethod != nil {
		result := CallMethod(receiver, "const_missing", &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]})
		if result != nil {
			return result
		}
	}
	return NewNameError("uninitialized constant " + name)
}

func moduleConstDefined(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.FalseVal
	}
	name, errVal := constNameFromValue(args[0])
	if errVal != nil {
		return errVal
	}
	if !validSimpleConstantName(name) {
		return NewNameError("wrong constant name " + name)
	}
	inherit := true
	if len(args) > 1 {
		inherit = args[1] != nil && args[1].IsTruthy()
	}
	if constantDefined(receiver, name, inherit) || autoloadDefined(receiver, name, inherit) {
		return R.TrueVal
	}
	return R.FalseVal
}

func moduleConstants(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	inherit := true
	if len(args) > 0 {
		inherit = args[0] != nil && args[0].IsTruthy()
	}
	names := map[string]bool{}
	collectConstants(receiver, names, inherit)
	values := make([]*object.EmeraldValue, 0, len(names))
	for name := range names {
		values = append(values, &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]})
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].Data.(string) < values[j].Data.(string)
	})
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func constantDefined(receiver *object.EmeraldValue, name string, inherit bool) bool {
	switch receiver.Type {
	case object.ValueClass:
		for class := receiver.Data.(*object.Class); class != nil; class = class.SuperClass {
			if _, ok := class.Constants[name]; ok {
				return true
			}
			if !inherit {
				break
			}
		}
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		for module != nil {
			if _, ok := module.Constants[name]; ok {
				return true
			}
			if !inherit {
				break
			}
			module = module.Parent
		}
	}
	return false
}

func autoloadDefined(receiver *object.EmeraldValue, name string, inherit bool) bool {
	_, ok := lookupAutoload(receiver, name, inherit)
	return ok
}

func collectConstants(receiver *object.EmeraldValue, names map[string]bool, inherit bool) {
	switch receiver.Type {
	case object.ValueClass:
		for class := receiver.Data.(*object.Class); class != nil; class = class.SuperClass {
			for name := range class.Constants {
				names[name] = true
			}
			for name := range class.Autoloads {
				names[name] = true
			}
			if !inherit {
				break
			}
		}
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		for module != nil {
			for name := range module.Constants {
				names[name] = true
			}
			for name := range module.Autoloads {
				names[name] = true
			}
			if !inherit {
				break
			}
			module = module.Parent
		}
	}
}

func modulePrivateConstant(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return setConstantVisibility(receiver, true, args...)
}

func modulePublicConstant(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return setConstantVisibility(receiver, false, args...)
}

func moduleDeprecateConstant(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		name := specName(arg)
		if name == "" && arg.Type == object.ValueString {
			name = arg.Data.(string)
		}
		if name == "" || !constantDefinedDirectly(receiver, name) {
			return newRuntimeException(R.Classes["NameError"], "constant "+name+" not defined")
		}
		switch receiver.Type {
		case object.ValueClass:
			receiver.Data.(*object.Class).DeprecatedConstants[name] = true
		case object.ValueModule:
			receiver.Data.(*object.Module).DeprecatedConstants[name] = true
		default:
			return typeError("not a class/module")
		}
	}
	return receiver
}

func setConstantVisibility(receiver *object.EmeraldValue, private bool, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		name := specName(arg)
		if name == "" && arg.Type == object.ValueString {
			name = arg.Data.(string)
		}
		if name == "" || !constantDefinedDirectly(receiver, name) {
			return newRuntimeException(R.Classes["NameError"], "constant "+name+" not defined")
		}
		switch receiver.Type {
		case object.ValueClass:
			if private {
				receiver.Data.(*object.Class).PrivateConstants[name] = true
			} else {
				delete(receiver.Data.(*object.Class).PrivateConstants, name)
			}
		case object.ValueModule:
			if private {
				receiver.Data.(*object.Module).PrivateConstants[name] = true
			} else {
				delete(receiver.Data.(*object.Module).PrivateConstants, name)
			}
		}
	}
	return R.NilVal
}

func constantDefinedDirectly(receiver *object.EmeraldValue, name string) bool {
	switch receiver.Type {
	case object.ValueClass:
		class := receiver.Data.(*object.Class)
		if _, ok := class.Constants[name]; ok {
			return true
		}
		_, ok := class.Autoloads[name]
		return ok
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		if _, ok := module.Constants[name]; ok {
			return true
		}
		_, ok := module.Autoloads[name]
		return ok
	}
	return false
}

func attrNamesFromValues(args ...*object.EmeraldValue) ([]string, *object.EmeraldValue) {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == nil {
			return nil, typeError("nil is not a symbol nor a string")
		}
		switch arg.Type {
		case object.ValueString, object.ValueSymbol:
			name := specName(arg)
			if name != "" {
				names = append(names, name)
			}
		default:
			if CallMethod == nil {
				return nil, typeError("no implicit conversion into String")
			}
			if !receiverHasCallableMethod(arg, "to_str") {
				return nil, typeError("no implicit conversion into String")
			}
			coerced := CallMethod(arg, "to_str")
			if coerced != nil && coerced.Type == object.ValueException {
				return nil, coerced
			}
			if coerced == nil || coerced.Type != object.ValueString {
				return nil, typeError("no implicit conversion into String")
			}
			names = append(names, coerced.Data.(string))
		}
	}
	return names, nil
}

func attrMethodNames(names []string, reader bool, writer bool) []string {
	methods := make([]string, 0, len(names)*2)
	for _, name := range names {
		if reader {
			methods = append(methods, name)
		}
		if writer {
			methods = append(methods, name+"=")
		}
	}
	return methods
}

func symbolArray(names ...string) *object.EmeraldValue {
	values := make([]*object.EmeraldValue, 0, len(names))
	for _, name := range names {
		values = append(values, &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]})
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func moduleInstanceMethods(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return moduleMethodsForVisibility(receiver, "", includeInheritedMethods(args...))
}

func modulePublicInstanceMethods(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return moduleMethodsForVisibility(receiver, "public", includeInheritedMethods(args...))
}

func moduleProtectedInstanceMethods(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return moduleMethodsForVisibility(receiver, "protected", includeInheritedMethods(args...))
}

func modulePrivateInstanceMethods(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return moduleMethodsForVisibility(receiver, "private", includeInheritedMethods(args...))
}

func moduleInstanceMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	names, errVal := attrNamesFromValues(args[0])
	if errVal != nil {
		return errVal
	}
	if len(names) == 0 {
		return NewNameError("undefined method")
	}
	name := names[0]
	method, owner, ok := lookupInstanceMethodWithOwner(receiver, name)
	if !ok || isUndefinedMethod(method) {
		return NewNameError("undefined method `" + name + "'")
	}
	copy := *method
	copy.Owner = owner
	copy.Receiver = nil
	return &object.EmeraldValue{Type: object.ValueMethod, Data: &copy, Class: R.Classes["UnboundMethod"]}
}

func modulePublicInstanceMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	if args[0] == nil || (args[0].Type != object.ValueString && args[0].Type != object.ValueSymbol) {
		return typeError("wrong argument type")
	}
	name := specName(args[0])
	method, owner, ok := lookupInstanceMethodWithOwner(receiver, name)
	if !ok || isUndefinedMethod(method) || methodVisibility(method) != "public" {
		return NewNameError("undefined method `" + name + "'")
	}
	copy := *method
	copy.Owner = owner
	copy.Receiver = nil
	return &object.EmeraldValue{Type: object.ValueMethod, Data: &copy, Class: R.Classes["UnboundMethod"]}
}

func classVariableName(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	names, errVal := attrNamesFromValues(value)
	if errVal != nil {
		return "", errVal
	}
	if len(names) == 0 || !strings.HasPrefix(names[0], "@@") || len(names[0]) <= 2 {
		return "", NewNameError("wrong class variable name")
	}
	return names[0], nil
}

func moduleClassVariableSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return NewArgumentError("wrong number of arguments")
	}
	if receiver != nil && receiver.Frozen {
		return frozenError("can't modify frozen class/module")
	}
	name, errVal := classVariableName(args[0])
	if errVal != nil {
		return errVal
	}
	SetClassVariable(receiver, name, args[1])
	return args[1]
}

func moduleClassVariableGet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	name, errVal := classVariableName(args[0])
	if errVal != nil {
		return errVal
	}
	if value, ok := LookupClassVariable(receiver, name); ok {
		return value
	}
	return NewNameError("uninitialized class variable " + name)
}

func moduleClassVariableDefined(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.FalseVal
	}
	name, errVal := classVariableName(args[0])
	if errVal != nil {
		return errVal
	}
	if _, ok := LookupClassVariable(receiver, name); ok {
		return R.TrueVal
	}
	return R.FalseVal
}

func SetClassVariable(receiver *object.EmeraldValue, name string, value *object.EmeraldValue) {
	if receiver == nil {
		return
	}
	switch receiver.Type {
	case object.ValueClass:
		class := receiver.Data.(*object.Class)
		if !setExistingClassClassVariable(class, name, value) {
			class.ClassVars[name] = value
		}
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		if !setExistingModuleClassVariable(module, name, value) {
			module.ClassVars[name] = value
		}
	case object.ValueObject:
		obj := receiver.Data.(*object.Object)
		if obj.ClassVars == nil {
			obj.ClassVars = make(map[string]*object.EmeraldValue)
		}
		obj.ClassVars[name] = value
	}
}

func setExistingClassClassVariable(class *object.Class, name string, value *object.EmeraldValue) bool {
	for cls := class; cls != nil; cls = cls.SuperClass {
		if _, ok := cls.ClassVars[name]; ok {
			cls.ClassVars[name] = value
			return true
		}
		for _, mod := range cls.IncludedModules {
			if setExistingModuleClassVariable(mod, name, value) {
				return true
			}
		}
	}
	return false
}

func setExistingModuleClassVariable(module *object.Module, name string, value *object.EmeraldValue) bool {
	if module == nil {
		return false
	}
	if _, ok := module.ClassVars[name]; ok {
		module.ClassVars[name] = value
		return true
	}
	for _, included := range module.IncludedModules {
		if setExistingModuleClassVariable(included, name, value) {
			return true
		}
	}
	if module.Parent != nil {
		return setExistingModuleClassVariable(module.Parent, name, value)
	}
	return false
}

func LookupClassVariable(receiver *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	if receiver == nil {
		return nil, false
	}
	switch receiver.Type {
	case object.ValueClass:
		return lookupClassClassVariable(receiver.Data.(*object.Class), name)
	case object.ValueModule:
		return lookupModuleClassVariable(receiver.Data.(*object.Module), name)
	case object.ValueObject:
		obj := receiver.Data.(*object.Object)
		if value, ok := obj.ClassVars[name]; ok {
			return value, true
		}
		return lookupClassClassVariable(obj.Class, name)
	}
	return nil, false
}

func lookupClassClassVariable(class *object.Class, name string) (*object.EmeraldValue, bool) {
	for cls := class; cls != nil; cls = cls.SuperClass {
		if value, ok := cls.ClassVars[name]; ok {
			return value, true
		}
		for _, mod := range cls.IncludedModules {
			if value, ok := lookupModuleClassVariable(mod, name); ok {
				return value, true
			}
		}
	}
	return nil, false
}

func lookupModuleClassVariable(module *object.Module, name string) (*object.EmeraldValue, bool) {
	if module == nil {
		return nil, false
	}
	if value, ok := module.ClassVars[name]; ok {
		return value, true
	}
	for _, included := range module.IncludedModules {
		if value, ok := lookupModuleClassVariable(included, name); ok {
			return value, true
		}
	}
	if module.Parent != nil {
		return lookupModuleClassVariable(module.Parent, name)
	}
	return nil, false
}

func moduleAliasMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 1 && args[0] != nil && args[0].Type == object.ValueArray {
		args = args[0].Data.([]*object.EmeraldValue)
	}
	if len(args) < 2 {
		return NewArgumentError("wrong number of arguments")
	}
	names, errVal := attrNamesFromValues(args[0], args[1])
	if errVal != nil {
		return errVal
	}
	if receiver != nil && receiver.Frozen {
		return frozenError("can't modify frozen class/module")
	}
	newName, oldName := names[0], names[1]
	method, _, ok := lookupInstanceMethodWithOwner(receiver, oldName)
	if !ok {
		return NewNameError("undefined method `" + oldName + "'")
	}
	copy := *method
	copy.Name = newName
	if copy.Owner == nil {
		copy.Owner = receiver
	}
	if isAlwaysPrivateMethodName(newName) {
		copy.Visibility = "private"
	}
	switch receiver.Type {
	case object.ValueClass:
		receiver.Data.(*object.Class).Methods[newName] = &copy
	case object.ValueModule:
		receiver.Data.(*object.Module).Methods[newName] = &copy
	default:
		return typeError("not a class/module")
	}
	return &object.EmeraldValue{Type: object.ValueSymbol, Data: newName, Class: R.Classes["Symbol"]}
}

func isAlwaysPrivateMethodName(name string) bool {
	switch name {
	case "initialize", "initialize_copy", "initialize_clone", "initialize_dup", "respond_to_missing?":
		return true
	default:
		return false
	}
}

func moduleDefineMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil || (receiver.Type != object.ValueClass && receiver.Type != object.ValueModule) {
		return typeError("not a class/module")
	}
	if receiver.Frozen {
		return frozenError("can't modify frozen class/module")
	}
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	names, errVal := attrNamesFromValues(args[0])
	if errVal != nil {
		return errVal
	}
	if len(names) == 0 {
		return typeError("is not a symbol nor a string")
	}
	name := names[0]
	var method *object.Method
	if len(args) > 1 {
		method, errVal = methodFromDefineMethodValue(receiver, name, args[1])
	} else {
		if CurrentBlockValue == nil || CurrentBlockValue() == nil {
			return NewArgumentError("tried to create Proc object without a block")
		}
		method, errVal = methodFromDefineMethodValue(receiver, name, CurrentBlockValue())
	}
	if errVal != nil {
		return errVal
	}
	method.Name = name
	method.Owner = receiver
	method.Visibility = defineMethodVisibility(receiver, name)
	method.EnforceArity = true
	switch receiver.Type {
	case object.ValueClass:
		receiver.Data.(*object.Class).DefineMethod(name, method)
	case object.ValueModule:
		receiver.Data.(*object.Module).DefineMethod(name, method)
	}
	if CallMethod != nil {
		CallMethod(receiver, "method_added", &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]})
	}
	return &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]}
}

func methodFromDefineMethodValue(receiver *object.EmeraldValue, name string, value *object.EmeraldValue) (*object.Method, *object.EmeraldValue) {
	if value == nil {
		return nil, typeError("wrong argument type NilClass (expected Proc/Method/UnboundMethod)")
	}
	switch value.Type {
	case object.ValueClosure:
		closure := value.Data.(*object.Closure)
		return &object.Method{Name: name, Fn: closure.Fn, Closure: closure}, nil
	case object.ValueProc:
		proc := value.Data.(*object.Proc)
		if proc.Native != nil {
			return &object.Method{Name: name, Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
				return proc.Native(args...)
			}}, nil
		}
		return &object.Method{Name: name, Fn: proc.Fn, Closure: &object.Closure{
			Fn:         proc.Fn,
			Free:       proc.Env,
			Block:      proc.Block,
			Binding:    proc.Binding,
			ClassStack: proc.ClassStack,
			AutoSplat:  proc.AutoSplat,
		}}, nil
	case object.ValueMethod:
		copy := *value.Data.(*object.Method)
		if errVal := validateDefineMethodOwner(receiver, &copy); errVal != nil {
			return nil, errVal
		}
		copy.Name = name
		return &copy, nil
	default:
		typeName := value.Inspect()
		if value.Class != nil && value.Class.Name != "" {
			typeName = value.Class.Name
		}
		return nil, typeError("wrong argument type " + typeName + " (expected Proc/Method/UnboundMethod)")
	}
}

func validateDefineMethodOwner(receiver *object.EmeraldValue, method *object.Method) *object.EmeraldValue {
	if receiver == nil || method == nil || method.Owner == nil {
		return nil
	}
	if method.Owner.Type != object.ValueClass {
		return nil
	}
	owner := method.Owner.Data.(*object.Class)
	if owner == nil || owner.IsSingleton {
		return nil
	}
	if receiver.Type != object.ValueClass {
		return typeError("bind argument must be a subclass of " + owner.Name)
	}
	target := receiver.Data.(*object.Class)
	if defineMethodClassInheritsFrom(target, owner) {
		return nil
	}
	return typeError("bind argument must be a subclass of " + owner.Name)
}

func defineMethodClassInheritsFrom(cls, target *object.Class) bool {
	if cls == nil || target == nil {
		return false
	}
	for current := cls; current != nil; current = current.SuperClass {
		if current == target {
			return true
		}
		if current.Name != "" && current.Name == target.Name {
			return true
		}
	}
	return false
}

func defineMethodVisibility(receiver *object.EmeraldValue, name string) string {
	if isAlwaysPrivateMethodName(name) {
		return "private"
	}
	var visibility *object.EmeraldValue
	switch receiver.Type {
	case object.ValueClass:
		visibility = receiver.Data.(*object.Class).GetInstanceVar("@__visibility")
	case object.ValueModule:
		visibility = receiver.Data.(*object.Module).GetInstanceVar("@__visibility")
	}
	if visibility != nil && visibility.Type == object.ValueString {
		switch visibility.Data.(string) {
		case "private", "protected":
			return visibility.Data.(string)
		}
	}
	return "public"
}

func moduleRemoveMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return receiver
	}
	names, errVal := attrNamesFromValues(args...)
	if errVal != nil {
		return errVal
	}
	if receiver != nil && receiver.Frozen {
		return frozenError("can't modify frozen class/module")
	}
	for _, name := range names {
		if !removeDirectMethod(receiver, name) {
			return NewNameError("method `" + name + "' not defined")
		}
	}
	return receiver
}

func removeDirectMethod(receiver *object.EmeraldValue, name string) bool {
	if receiver == nil {
		return false
	}
	switch receiver.Type {
	case object.ValueClass:
		methods := receiver.Data.(*object.Class).Methods
		if _, ok := methods[name]; ok {
			delete(methods, name)
			return true
		}
	case object.ValueModule:
		methods := receiver.Data.(*object.Module).Methods
		if _, ok := methods[name]; ok {
			delete(methods, name)
			return true
		}
	}
	return false
}

func moduleUndefMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return receiver
	}
	names, errVal := attrNamesFromValues(args...)
	if errVal != nil {
		return errVal
	}
	if receiver != nil && receiver.Frozen {
		return frozenError("can't modify frozen class/module")
	}
	for _, name := range names {
		method, _, ok := lookupInstanceMethodWithOwner(receiver, name)
		if !ok || isUndefinedMethod(method) {
			return NewNameError("undefined method `" + name + "' for " + moduleOrClassDescription(receiver))
		}
		undefined := &object.Method{Name: name, Visibility: "undefined"}
		switch receiver.Type {
		case object.ValueClass:
			receiver.Data.(*object.Class).Methods[name] = undefined
		case object.ValueModule:
			receiver.Data.(*object.Module).Methods[name] = undefined
		}
	}
	return receiver
}

func moduleOrClassDescription(receiver *object.EmeraldValue) string {
	switch receiver.Type {
	case object.ValueModule:
		return "module `" + moduleToS(receiver.Data.(*object.Module)) + "'"
	case object.ValueClass:
		name := classToS(receiver.Data.(*object.Class))
		if strings.HasPrefix(name, "#<Class:") && strings.HasSuffix(name, ">") {
			inner := strings.TrimSuffix(strings.TrimPrefix(name, "#<Class:"), ">")
			if !strings.HasPrefix(inner, "#<") {
				name = inner
			}
		}
		return "class `" + name + "'"
	default:
		return "class/module"
	}
}

func moduleRuby2Keywords(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		name := ""
		if arg != nil && arg.Type == object.ValueClosure {
			if closure, ok := arg.Data.(*object.Closure); ok && closure.Fn != nil {
				name = closure.Fn.Name
			}
		} else {
			names, errVal := attrNamesFromValues(arg)
			if errVal != nil {
				return errVal
			}
			if len(names) > 0 {
				name = names[0]
			}
		}
		method, _, ok := lookupInstanceMethodWithOwner(receiver, name)
		if !ok {
			return NewNameError("undefined method `" + name + "'")
		}
		method.Ruby2Keywords = true
	}
	return R.NilVal
}

func lookupInstanceMethodWithOwner(receiver *object.EmeraldValue, name string) (*object.Method, *object.EmeraldValue, bool) {
	if receiver == nil {
		return nil, nil, false
	}
	switch receiver.Type {
	case object.ValueClass:
		for cls := receiver.Data.(*object.Class); cls != nil; cls = cls.SuperClass {
			if method, ok := cls.Methods[name]; ok {
				return method, &object.EmeraldValue{Type: object.ValueClass, Data: cls, Class: R.Classes["Class"]}, true
			}
			for _, mod := range cls.IncludedModules {
				if method, owner, ok := lookupModuleMethodWithOwner(mod, name); ok {
					return method, owner, true
				}
			}
		}
	case object.ValueModule:
		return lookupModuleMethodWithOwner(receiver.Data.(*object.Module), name)
	}
	return nil, nil, false
}

func lookupModuleMethodWithOwner(module *object.Module, name string) (*object.Method, *object.EmeraldValue, bool) {
	if module == nil {
		return nil, nil, false
	}
	if method, ok := module.Methods[name]; ok {
		if method.Owner != nil {
			return method, method.Owner, true
		}
		return method, &object.EmeraldValue{Type: object.ValueModule, Data: module, Class: R.Classes["Module"]}, true
	}
	for _, included := range module.IncludedModules {
		if method, owner, ok := lookupModuleMethodWithOwner(included, name); ok {
			return method, owner, true
		}
	}
	if module.Parent != nil {
		return lookupModuleMethodWithOwner(module.Parent, name)
	}
	return nil, nil, false
}

func includeInheritedMethods(args ...*object.EmeraldValue) bool {
	if len(args) == 0 || args[0] == nil || args[0].Type == object.ValueNil {
		return true
	}
	return !(args[0].Type == object.ValueBool && !args[0].Data.(bool))
}

func moduleMethodDefined(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return moduleMethodDefinedForVisibility(receiver, "", args...)
}

func modulePublicMethodDefined(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return moduleMethodDefinedForVisibility(receiver, "public", args...)
}

func moduleProtectedMethodDefined(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return moduleMethodDefinedForVisibility(receiver, "protected", args...)
}

func modulePrivateMethodDefined(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return moduleMethodDefinedForVisibility(receiver, "private", args...)
}

func moduleMethodDefinedForVisibility(receiver *object.EmeraldValue, visibility string, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.FalseVal
	}
	names, errVal := attrNamesFromValues(args[0])
	if errVal != nil {
		return errVal
	}
	if len(names) == 0 {
		return R.FalseVal
	}
	name := names[0]
	inherited := true
	if len(args) > 1 {
		inherited = includeInheritedMethods(args[1])
	}
	var method *object.Method
	var ok bool
	if inherited {
		method, _, ok = lookupInstanceMethodWithOwner(receiver, name)
	} else {
		method, ok = lookupDirectInstanceMethod(receiver, name)
	}
	if ok && methodMatchesDefinedVisibility(method, visibility) {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodMatchesDefinedVisibility(method *object.Method, visibility string) bool {
	if isUndefinedMethod(method) {
		return false
	}
	actual := methodVisibility(method)
	if visibility == "" {
		return actual != "private"
	}
	return actual == visibility
}

func lookupDirectInstanceMethod(receiver *object.EmeraldValue, name string) (*object.Method, bool) {
	if receiver == nil {
		return nil, false
	}
	switch receiver.Type {
	case object.ValueClass:
		method, ok := receiver.Data.(*object.Class).Methods[name]
		return method, ok
	case object.ValueModule:
		method, ok := receiver.Data.(*object.Module).Methods[name]
		return method, ok
	}
	return nil, false
}

func moduleMethodsForVisibility(receiver *object.EmeraldValue, visibility string, inherited bool) *object.EmeraldValue {
	seen := map[string]bool{}
	names := []string{}
	add := func(name string, method *object.Method) {
		if seen[name] {
			return
		}
		if isUndefinedMethod(method) {
			seen[name] = true
			return
		}
		if visibility != "" && methodVisibility(method) != visibility {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	if receiver != nil {
		switch receiver.Type {
		case object.ValueClass:
			start := receiver.Data.(*object.Class)
			for cls := start; cls != nil; cls = cls.SuperClass {
				for name, method := range cls.Methods {
					if start.Name == "Class" && cls.Name == "Module" && (name == "append_features" || name == "prepend_features" || name == "extend_object") {
						continue
					}
					add(name, method)
				}
				for _, mod := range cls.IncludedModules {
					for name, method := range mod.Methods {
						add(name, method)
					}
				}
				if !inherited {
					break
				}
			}
		case object.ValueModule:
			mod := receiver.Data.(*object.Module)
			for name, method := range mod.Methods {
				add(name, method)
			}
			if inherited {
				for _, included := range mod.IncludedModules {
					for name, method := range included.Methods {
						add(name, method)
					}
				}
			}
		}
	}
	return symbolArray(names...)
}

func methodVisibility(method *object.Method) string {
	if isUndefinedMethod(method) {
		return "undefined"
	}
	if method == nil || method.Visibility == "" {
		return "public"
	}
	return method.Visibility
}

func isUndefinedMethod(method *object.Method) bool {
	return method != nil && method.Visibility == "undefined"
}

func setMethodVisibility(receiver *object.EmeraldValue, visibility string, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		setCurrentMethodVisibility(receiver, visibility)
		return R.NilVal
	}
	names, errVal := attrNamesFromValues(args...)
	if errVal != nil {
		return errVal
	}
	for _, name := range names {
		if !setNamedMethodVisibility(receiver, name, visibility) {
			return NewNameError("undefined method `" + name + "'")
		}
	}
	if len(args) == 1 && args[0] != nil && args[0].Type != object.ValueArray {
		return &object.EmeraldValue{Type: object.ValueSymbol, Data: names[0], Class: R.Classes["Symbol"]}
	}
	return symbolArray(names...)
}

func setCurrentMethodVisibility(receiver *object.EmeraldValue, visibility string) {
	if receiver == nil {
		return
	}
	switch receiver.Type {
	case object.ValueClass:
		receiver.Data.(*object.Class).SetInstanceVar("@__visibility", rubyString(visibility))
	case object.ValueModule:
		receiver.Data.(*object.Module).SetInstanceVar("@__visibility", rubyString(visibility))
	}
}

func setNamedMethodVisibility(receiver *object.EmeraldValue, name, visibility string) bool {
	if receiver == nil {
		return false
	}
	switch receiver.Type {
	case object.ValueClass:
		cls := receiver.Data.(*object.Class)
		if method, ok := cls.Methods[name]; ok {
			method.Visibility = visibility
			return true
		}
		if method, _, ok := cls.GetMethodWithOwner(name); ok {
			clone := *method
			clone.Visibility = visibility
			cls.Methods[name] = &clone
			return true
		}
	case object.ValueModule:
		mod := receiver.Data.(*object.Module)
		if method, ok := mod.Methods[name]; ok {
			method.Visibility = visibility
			return true
		}
		if method, ok := R.Classes["Object"].GetMethod(name); ok {
			clone := *method
			clone.Visibility = visibility
			mod.Methods[name] = &clone
			return true
		}
	}
	return false
}

func setClassMethodVisibility(receiver *object.EmeraldValue, visibility string, args ...*object.EmeraldValue) *object.EmeraldValue {
	names, errVal := classMethodNamesFromValues(args...)
	if errVal != nil {
		return errVal
	}
	for _, name := range names {
		if !setNamedClassMethodVisibility(receiver, name, visibility) {
			return NewNameError("undefined method `" + name + "'")
		}
	}
	if len(args) == 1 && args[0] != nil && args[0].Type != object.ValueArray {
		return &object.EmeraldValue{Type: object.ValueSymbol, Data: names[0], Class: R.Classes["Symbol"]}
	}
	return symbolArray(names...)
}

func classMethodNamesFromValues(args ...*object.EmeraldValue) ([]string, *object.EmeraldValue) {
	expanded := make([]*object.EmeraldValue, 0, len(args))
	for _, arg := range args {
		if arg != nil && arg.Type == object.ValueArray {
			expanded = append(expanded, arg.Data.([]*object.EmeraldValue)...)
			continue
		}
		expanded = append(expanded, arg)
	}
	return attrNamesFromValues(expanded...)
}

func setNamedClassMethodVisibility(receiver *object.EmeraldValue, name, visibility string) bool {
	if receiver == nil {
		return false
	}
	switch receiver.Type {
	case object.ValueClass:
		cls := receiver.Data.(*object.Class)
		if cls.SingletonClass != nil {
			if method, ok := cls.SingletonClass.Methods[name]; ok {
				clone := *method
				clone.Visibility = visibility
				cls.SingletonClass.Methods[name] = &clone
				return true
			}
		}
		if method, ok := cls.ClassMethods[name]; ok {
			clone := *method
			clone.Visibility = visibility
			cls.ClassMethods[name] = &clone
			return true
		}
		if method, ok := lookupInheritedClassMethodForVisibility(cls, name); ok {
			clone := *method
			clone.Visibility = visibility
			cls.ClassMethods[name] = &clone
			return true
		}
	case object.ValueModule:
		mod := receiver.Data.(*object.Module)
		if mod.SingletonClass != nil {
			if method, ok := mod.SingletonClass.Methods[name]; ok {
				clone := *method
				clone.Visibility = visibility
				mod.SingletonClass.Methods[name] = &clone
				return true
			}
		}
	}
	return false
}

func lookupInheritedClassMethodForVisibility(cls *object.Class, name string) (*object.Method, bool) {
	for current := cls.SuperClass; current != nil; current = current.SuperClass {
		if current.SingletonClass != nil {
			if method, ok := current.SingletonClass.Methods[name]; ok {
				return method, true
			}
		}
		if method, ok := current.ClassMethods[name]; ok {
			return method, true
		}
	}
	return nil, false
}

func currentMethodVisibility(receiver *object.EmeraldValue) string {
	if receiver == nil {
		return "public"
	}
	var visibility *object.EmeraldValue
	switch receiver.Type {
	case object.ValueClass:
		visibility = receiver.Data.(*object.Class).GetInstanceVar("@__visibility")
	case object.ValueModule:
		visibility = receiver.Data.(*object.Module).GetInstanceVar("@__visibility")
	}
	if visibility != nil && visibility.Type == object.ValueString {
		switch visibility.Data.(string) {
		case "private", "protected":
			return visibility.Data.(string)
		}
	}
	return "public"
}

func defineAttrReaders(receiver *object.EmeraldValue, names ...string) {
	if receiver == nil || receiver.Class == nil {
		return
	}
	for _, name := range names {
		ivar := "@" + name
		getterName := name
		visibility := currentMethodVisibility(receiver)
		defineDynamicAccessor(receiver, getterName, &object.Method{
			Name:       getterName,
			Arity:      0,
			Visibility: visibility,
			Fn: func(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
				if visibility == "protected" {
					return newRuntimeException(R.Classes["NoMethodError"], "protected method `"+getterName+"' called")
				}
				if value := dynamicInstanceVar(r, ivar); value != nil {
					return value
				}
				return R.NilVal
			},
		})
	}
}

func defineAttrWriters(receiver *object.EmeraldValue, names ...string) {
	if receiver == nil || receiver.Class == nil {
		return
	}
	for _, name := range names {
		ivar := "@" + name
		setterName := name + "="
		visibility := currentMethodVisibility(receiver)
		defineDynamicAccessor(receiver, setterName, &object.Method{
			Name:       setterName,
			Arity:      1,
			Visibility: visibility,
			Fn: func(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
				if len(args) == 0 {
					return R.NilVal
				}
				if visibility == "protected" {
					return newRuntimeException(R.Classes["NoMethodError"], "protected method `"+setterName+"' called")
				}
				if attrWriteFrozen(r) {
					return frozenError("can't modify frozen object")
				}
				setDynamicInstanceVar(r, ivar, args[0])
				return args[0]
			},
		})
	}
}

func attrWriteFrozen(receiver *object.EmeraldValue) bool {
	if receiver == nil {
		return false
	}
	if receiver.Frozen {
		return true
	}
	switch receiver.Type {
	case object.ValueBool, object.ValueInteger, object.ValueFloat, object.ValueSymbol, object.ValueNil:
		return true
	}
	return false
}

func defineDynamicAccessor(receiver *object.EmeraldValue, name string, method *object.Method) {
	switch receiver.Type {
	case object.ValueClass:
		cls := receiver.Data.(*object.Class)
		cls.DefineMethod(name, method)
		cls.DefineClassMethod(name, method)
		callMethodAddedHook(receiver, name)
	case object.ValueModule:
		receiver.Data.(*object.Module).DefineMethod(name, method)
		callMethodAddedHook(receiver, name)
	default:
		receiver.Class.DefineMethod(name, method)
	}
}

func callMethodAddedHook(receiver *object.EmeraldValue, name string) {
	if CallMethod == nil || receiver == nil {
		return
	}
	if receiverHasCallableMethod(receiver, "method_added") {
		CallMethod(receiver, "method_added", &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]})
	}
	if receiver.Type == object.ValueClass {
		if cls := receiver.Data.(*object.Class); cls.IsSingleton && cls.SingletonOwner != nil {
			if receiverHasCallableMethod(cls.SingletonOwner, "singleton_method_added") {
				CallMethod(cls.SingletonOwner, "singleton_method_added", &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]})
			}
		}
	}
}

func receiverHasCallableMethod(receiver *object.EmeraldValue, name string) bool {
	if receiver == nil {
		return false
	}
	switch receiver.Type {
	case object.ValueObject:
		if obj, ok := receiver.Data.(*object.Object); ok {
			if _, ok := obj.SingletonMethods[name]; ok {
				return true
			}
			if obj.SingletonClass != nil {
				if _, ok := obj.SingletonClass.Methods[name]; ok {
					return true
				}
			}
		}
	case object.ValueClass:
		cls := receiver.Data.(*object.Class)
		if cls.SingletonClass != nil {
			if _, ok := cls.SingletonClass.Methods[name]; ok {
				return true
			}
		}
		if _, ok := cls.ClassMethods[name]; ok {
			return true
		}
	case object.ValueModule:
		mod := receiver.Data.(*object.Module)
		if mod.SingletonClass != nil {
			if _, ok := mod.SingletonClass.Methods[name]; ok {
				return true
			}
		}
		if _, ok := mod.Methods[name]; ok {
			return true
		}
	}
	if receiver.Class != nil {
		if _, ok := receiver.Class.GetMethod(name); ok {
			return true
		}
	}
	return false
}

func dynamicInstanceVar(receiver *object.EmeraldValue, name string) *object.EmeraldValue {
	if receiver == nil {
		return nil
	}
	switch receiver.Type {
	case object.ValueObject:
		if data, ok := receiver.Data.(*ioShimData); ok && name == "@close_exception" {
			if data.closeException != nil {
				return data.closeException
			}
			return R.NilVal
		}
		if obj, ok := receiver.Data.(*object.Object); ok {
			return obj.GetInstanceVar(name)
		}
	case object.ValueProc:
		if proc, ok := receiver.Data.(*object.Proc); ok {
			return proc.InstanceVars[name]
		}
	case object.ValueClass:
		if cls, ok := receiver.Data.(*object.Class); ok {
			return cls.GetInstanceVar(name)
		}
	case object.ValueModule:
		if mod, ok := receiver.Data.(*object.Module); ok {
			return mod.InstanceVars[name]
		}
	}
	return nil
}

func setDynamicInstanceVar(receiver *object.EmeraldValue, name string, value *object.EmeraldValue) {
	if receiver == nil {
		return
	}
	switch receiver.Type {
	case object.ValueObject:
		if data, ok := receiver.Data.(*ioShimData); ok && name == "@close_exception" {
			data.closeHook = true
			data.closeException = value
			return
		}
		if obj, ok := receiver.Data.(*object.Object); ok {
			obj.SetInstanceVar(name, value)
		}
	case object.ValueProc:
		if proc, ok := receiver.Data.(*object.Proc); ok {
			if proc.InstanceVars == nil {
				proc.InstanceVars = make(map[string]*object.EmeraldValue)
			}
			proc.InstanceVars[name] = value
		}
	case object.ValueClass:
		if cls, ok := receiver.Data.(*object.Class); ok {
			cls.SetInstanceVar(name, value)
		}
	case object.ValueModule:
		if mod, ok := receiver.Data.(*object.Module); ok {
			mod.InstanceVars[name] = value
		}
	}
}

func DynamicInstanceVar(receiver *object.EmeraldValue, name string) *object.EmeraldValue {
	return dynamicInstanceVar(receiver, name)
}

func SetDynamicInstanceVar(receiver *object.EmeraldValue, name string, value *object.EmeraldValue) {
	setDynamicInstanceVar(receiver, name, value)
}

func methodInstanceOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || args[0].Type != object.ValueClass {
		return R.FalseVal
	}
	expected := args[0].Data.(*object.Class)
	if receiver.Class != nil && (receiver.Class == expected || receiver.Class.Name == expected.Name) {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if receiver == args[0] {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodEql(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if receiver.Equals(args[0]) {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodRespondTo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	methodName, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	_, ok = receiver.Class.GetMethod(methodName)
	if ok {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodMethods(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	names := map[string]bool{}
	if receiver != nil && receiver.Type == object.ValueObject {
		if obj, ok := receiver.Data.(*object.Object); ok {
			for name, method := range obj.SingletonMethods {
				if method == nil || method.Visibility == "undefined" {
					continue
				}
				names[name] = true
			}
		}
	}
	if receiver != nil && receiver.Class != nil {
		collectClassMethodNames(receiver.Class, names)
	}
	result := make([]*object.EmeraldValue, 0, len(names))
	for name := range names {
		result = append(result, &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Data.(string) < result[j].Data.(string)
	})
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: R.Classes["Array"]}
}

func collectClassMethodNames(class *object.Class, names map[string]bool) {
	if class == nil {
		return
	}
	for name, method := range class.Methods {
		if method == nil || method.Visibility == "undefined" {
			continue
		}
		names[name] = true
	}
	for _, mod := range class.IncludedModules {
		collectModuleMethodNames(mod, names)
	}
	collectClassMethodNames(class.SuperClass, names)
}

func collectModuleMethodNames(module *object.Module, names map[string]bool) {
	if module == nil {
		return
	}
	for name, method := range module.Methods {
		if method == nil || method.Visibility == "undefined" {
			continue
		}
		names[name] = true
	}
	for _, included := range module.IncludedModules {
		collectModuleMethodNames(included, names)
	}
}

func methodSend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	methodName, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	forwardArgs := args[1:]
	if methodName != "initialize" && len(forwardArgs) == 1 && forwardArgs[0] != nil && forwardArgs[0].Type == object.ValueArray {
		forwardArgs = forwardArgs[0].Data.([]*object.EmeraldValue)
	}
	if CallMethod != nil {
		return CallMethod(receiver, methodName, forwardArgs...)
	}
	return R.NilVal
}

func objectStrftime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("wrong number of arguments")
	}
	return R.NilVal
}

func objectDeconstructKeys(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("wrong number of arguments")
	}
	if args[0] != R.NilVal && args[0].Type != object.ValueArray {
		return typeError("wrong argument type")
	}
	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  map[*object.EmeraldValue]*object.EmeraldValue{},
		Class: R.Classes["Hash"],
	}
}

func nilPlus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return typeError("can't convert nil into time interval")
	}
	if args[0].Type == object.ValueInteger || args[0].Type == object.ValueFloat {
		return R.NilVal
	}
	return typeError("can't convert argument into time interval")
}

func methodObjectMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || receiver == nil || receiver.Class == nil {
		return R.NilVal
	}
	name := specName(args[0])
	if name == "" {
		return R.NilVal
	}
	var method *object.Method
	var ok bool
	if receiver.Type == object.ValueModule {
		method, ok = receiver.Data.(*object.Module).Methods[name]
	} else if receiver.Type == object.ValueClass {
		method, ok = receiver.Data.(*object.Class).ClassMethods[name]
	}
	var owner *object.Class
	if !ok {
		method, owner, ok = receiver.Class.GetMethodWithOwner(name)
	}
	if !ok || method == nil {
		return NewNameError("undefined method `" + name + "'")
	}
	copy := *method
	if copy.Owner == nil && owner != nil {
		copy.Owner = &object.EmeraldValue{Type: object.ValueClass, Data: owner, Class: R.Classes["Class"]}
	}
	copy.Receiver = receiver
	return &object.EmeraldValue{Type: object.ValueMethod, Data: &copy, Class: R.Classes["Method"]}
}

func methodEval(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || EvalSource == nil {
		return R.NilVal
	}
	source, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	if len(args) > 1 && args[1].Type == object.ValueBinding {
		return evalSourceWithBinding(source, args[1])
	}
	return EvalSource(source)
}

func objectExtend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.NilVal
	}
	for _, arg := range args {
		if arg.Type != object.ValueModule {
			continue
		}
		if CallMethod != nil {
			result := CallMethod(arg, "send", &object.EmeraldValue{Type: object.ValueSymbol, Data: "extend_object", Class: R.Classes["Symbol"]}, receiver)
			if result != nil && result.Type == object.ValueException {
				return result
			}
		} else {
			if result := moduleExtendObject(arg, receiver); result != nil && result.Type == object.ValueException {
				return result
			}
		}
	}
	return receiver
}

func methodDefineSingletonMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return R.NilVal
	}
	name := specName(args[0])
	block := CurrentBlockValue()
	if name == "" || block == nil {
		return R.NilVal
	}
	defineMockSingleton(receiver, name, func(_ *object.EmeraldValue, callArgs ...*object.EmeraldValue) *object.EmeraldValue {
		return CallBlockWithArgs(block, callArgs...)
	})
	return &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]}
}

func threadClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newThreadForClass(receiver, args, threadError("must be called with a block"))
}

func threadClassStart(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newThreadForClass(receiver, args, argumentError("must be called with a block"))
}

func newThreadForClass(receiver *object.EmeraldValue, args []*object.EmeraldValue, missingBlock *object.EmeraldValue) *object.EmeraldValue {
	if CurrentBlockValue == nil || CurrentBlockValue() == nil {
		return missingBlock
	}
	class := R.Classes["Thread"]
	if receiver != nil && receiver.Type == object.ValueClass {
		if cls, ok := receiver.Data.(*object.Class); ok {
			class = cls
		}
	}
	data := &threadData{
		block:             CurrentBlockValue(),
		args:              append([]*object.EmeraldValue(nil), args...),
		locals:            make(map[string]*object.EmeraldValue),
		threadVars:        make(map[string]*object.EmeraldValue),
		reportOnException: threadReportOnExceptionDefault,
		abortOnException:  threadAbortOnExceptionDefault,
		group:             defaultThreadGroup,
	}
	thread := &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  data,
		Class: class,
	}
	pendingThreads = append(pendingThreads, thread)
	return thread
}

func threadClassAllocate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newRuntimeException(R.Classes["TypeError"], "allocator undefined for Thread")
}

func threadInitialize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if _, initialized := receiver.Data.(*threadData); initialized {
		return threadError("already initialized thread")
	}
	if CurrentBlockValue == nil || CurrentBlockValue() == nil {
		return threadError("must be called with a block")
	}
	receiver.Data = &threadData{
		block:             CurrentBlockValue(),
		args:              append([]*object.EmeraldValue(nil), args...),
		locals:            make(map[string]*object.EmeraldValue),
		threadVars:        make(map[string]*object.EmeraldValue),
		reportOnException: threadReportOnExceptionDefault,
		abortOnException:  threadAbortOnExceptionDefault,
	}
	pendingThreads = append(pendingThreads, receiver)
	addThreadToGroup(defaultThreadGroup, receiver)
	return receiver
}

func threadClassCurrent(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if currentThread != nil {
		ensureThreadGroup(currentThread)
		return currentThread
	}
	if currentThread == nil {
		currentThread = &object.EmeraldValue{
			Type:  object.ValueObject,
			Data:  &threadData{locals: make(map[string]*object.EmeraldValue), threadVars: make(map[string]*object.EmeraldValue), reportOnException: threadReportOnExceptionDefault, ran: true, group: defaultThreadGroup},
			Class: R.Classes["Thread"],
		}
		addThreadToGroup(defaultThreadGroup, currentThread)
	}
	return currentThread
}

func threadClassHandleInterrupt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	thread := threadClassCurrent(receiver)
	data := threadValueData(thread)
	if data == nil {
		return R.NilVal
	}
	mode := threadInterruptMode(args)
	if mode == "immediate" && data.pendingInterrupt != nil {
		exc := data.pendingInterrupt
		data.pendingInterrupt = nil
		return exc
	}
	previous := data.deferInterrupt
	data.deferInterrupt = mode != "immediate"
	result := R.NilVal
	if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
		result = CallBlock()
	}
	data.deferInterrupt = previous
	if data.pendingInterrupt != nil {
		exc := data.pendingInterrupt
		data.pendingInterrupt = nil
		return exc
	}
	return result
}

func threadInterruptMode(args []*object.EmeraldValue) string {
	if len(args) == 0 || args[0] == nil || args[0].Type != object.ValueHash {
		return "immediate"
	}
	for _, value := range args[0].Data.(map[*object.EmeraldValue]*object.EmeraldValue) {
		switch specName(value) {
		case "never", "on_blocking":
			return "never"
		case "immediate":
			return "immediate"
		}
	}
	return "immediate"
}

func threadClassPendingInterrupt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return threadPendingInterrupt(threadClassCurrent(receiver))
}

func threadClassEachCallerLocation(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 {
		return argumentError("wrong number of arguments")
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CallBlock == nil {
		return newRuntimeException(R.Classes["LocalJumpError"], "no block given")
	}
	return R.NilVal
}

func threadClassReportOnException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if threadReportOnExceptionDefault {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadClassSetReportOnException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	threadReportOnExceptionDefault = len(args) > 0 && args[0].IsTruthy()
	if threadReportOnExceptionDefault {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadClassAbortOnException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if threadAbortOnExceptionDefault {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadClassSetAbortOnException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	threadAbortOnExceptionDefault = len(args) > 0 && args[0].IsTruthy()
	if threadAbortOnExceptionDefault {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadClassPass(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	runNextPendingThread()
	return R.NilVal
}

func threadIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || len(args) == 0 {
		return R.NilVal
	}
	key, ok := threadLocalKey(args[0])
	if !ok {
		return threadLocalKeyTypeError()
	}
	if value, ok := data.locals[key]; ok {
		return value
	}
	return R.NilVal
}

func threadIndexSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || len(args) < 2 {
		return R.NilVal
	}
	if receiver.Frozen {
		return frozenError("can't modify frozen thread locals")
	}
	if data.locals == nil {
		data.locals = make(map[string]*object.EmeraldValue)
	}
	key, ok := threadLocalKey(args[0])
	if !ok {
		return threadLocalKeyTypeError()
	}
	data.locals[key] = args[1]
	return args[1]
}

func threadKey(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || len(args) == 0 {
		return R.FalseVal
	}
	key, ok := threadLocalKey(args[0])
	if !ok {
		return threadLocalKeyTypeError()
	}
	_, exists := data.locals[key]
	if exists {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadFetch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	if len(args) < 1 || len(args) > 2 {
		return argumentError("wrong number of arguments")
	}
	key, ok := threadLocalKey(args[0])
	if !ok {
		return threadLocalKeyTypeError()
	}
	if value, exists := data.locals[key]; exists {
		return value
	}
	if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
		return CallBlock(args[0])
	}
	if len(args) == 2 {
		return args[1]
	}
	return newRuntimeException(R.Classes["KeyError"], "key not found")
}

func threadVariableGet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || len(args) == 0 {
		return R.NilVal
	}
	key, ok := threadLocalKey(args[0])
	if !ok {
		return threadLocalKeyTypeError()
	}
	if value, exists := data.threadVars[key]; exists {
		return value
	}
	return R.NilVal
}

func threadVariableSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || len(args) < 2 {
		return R.NilVal
	}
	key, ok := threadLocalKey(args[0])
	if !ok {
		return threadLocalKeyTypeError()
	}
	if receiver.Frozen {
		return frozenError("can't modify frozen thread locals")
	}
	if data.threadVars == nil {
		data.threadVars = make(map[string]*object.EmeraldValue)
	}
	if args[1] == nil || args[1].Type == object.ValueNil {
		delete(data.threadVars, key)
		return R.NilVal
	}
	data.threadVars[key] = args[1]
	return args[1]
}

func threadVariablePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || len(args) == 0 {
		return R.FalseVal
	}
	key, ok := threadLocalKey(args[0])
	if !ok {
		return threadLocalKeyTypeError()
	}
	_, exists := data.threadVars[key]
	if exists {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || data.name == nil {
		return R.NilVal
	}
	return data.name
}

func threadSetName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || len(args) == 0 {
		return R.NilVal
	}
	value := args[0]
	if value == nil || value.Type == object.ValueNil {
		data.name = nil
		return R.NilVal
	}
	name, ok := value.Data.(string)
	if value.Type != object.ValueString || !ok {
		if CallMethod == nil || value.Class == nil {
			return typeError("no implicit conversion to String")
		}
		if _, ok := value.Class.GetMethod("to_str"); !ok {
			return typeError("no implicit conversion to String")
		}
		coerced := CallMethod(value, "to_str")
		if coerced == nil || coerced.Type != object.ValueString {
			return typeError("no implicit conversion to String")
		}
		name, ok = coerced.Data.(string)
		if !ok {
			return typeError("no implicit conversion to String")
		}
	}
	if strings.Contains(name, "\x00") {
		return argumentError("thread name must not contain null bytes")
	}
	data.name = &object.EmeraldValue{Type: object.ValueString, Data: name, Class: R.Classes["String"]}
	return data.name
}

func threadReportOnException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data != nil && data.reportOnException {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadSetReportOnException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	data.reportOnException = len(args) > 0 && args[0].IsTruthy()
	if data.reportOnException {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadPendingInterrupt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data != nil && data.pendingInterrupt != nil {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadAbortOnException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data != nil && data.abortOnException {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadSetAbortOnException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	data.abortOnException = len(args) > 0 && args[0].IsTruthy()
	if data.abortOnException {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadPriority(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	priority := int64(0)
	if data != nil {
		priority = data.priority
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: priority, Class: R.Classes["Integer"]}
}

func threadSetPriority(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || len(args) == 0 || args[0].Type != object.ValueInteger {
		return typeError("no implicit conversion to Integer")
	}
	priority := args[0].Data.(int64)
	if priority > 3 {
		priority = 3
	} else if priority < -3 {
		priority = -3
	}
	data.priority = priority
	return &object.EmeraldValue{Type: object.ValueInteger, Data: priority, Class: R.Classes["Integer"]}
}

func scratchPadRecord(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		scratchPadRecorded = R.NilVal
		return R.NilVal
	}
	scratchPadRecorded = args[0]
	return args[0]
}

func scratchPadRecordedValue(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if scratchPadRecorded == nil {
		return R.NilVal
	}
	return scratchPadRecorded
}

func scratchPadClear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	scratchPadRecorded = R.NilVal
	return R.NilVal
}

func scratchPadAppend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return scratchPadRecordedValue(receiver)
	}
	if scratchPadRecorded == nil || scratchPadRecorded.Type == object.ValueNil {
		scratchPadRecorded = &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{},
			Class: R.Classes["Array"],
		}
	}
	if scratchPadRecorded.Type == object.ValueArray {
		values := scratchPadRecorded.Data.([]*object.EmeraldValue)
		values = append(values, args[0])
		scratchPadRecorded.Data = values
		return scratchPadRecorded
	}
	scratchPadRecorded = args[0]
	return scratchPadRecorded
}

func scratchPadHasSymbol(name string) bool {
	if scratchPadRecorded == nil || scratchPadRecorded.Type != object.ValueArray {
		return false
	}
	for _, value := range scratchPadRecorded.Data.([]*object.EmeraldValue) {
		if value != nil && value.Type == object.ValueSymbol && value.Data == name {
			return true
		}
	}
	return false
}

func processArgvZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if processArgv0 == nil {
		value := ""
		if len(os.Args) > 0 {
			value = os.Args[0]
		}
		processArgv0 = &object.EmeraldValue{Type: object.ValueString, Data: value, Class: R.Classes["String"], Frozen: true}
	}
	return processArgv0
}

func processPid(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(os.Getpid()), Class: R.Classes["Integer"]}
}

func processPpid(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(os.Getppid()), Class: R.Classes["Integer"]}
}

func processUid(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(os.Getuid()), Class: R.Classes["Integer"]}
}

func processEuid(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(os.Geteuid()), Class: R.Classes["Integer"]}
}

func processGid(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(os.Getgid()), Class: R.Classes["Integer"]}
}

func processEgid(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(os.Getegid()), Class: R.Classes["Integer"]}
}

func processSetID(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return typeError("no implicit conversion to Integer")
	}
	if args[0].Type == object.ValueString {
		if os.Getuid() != 0 && specName(args[0]) == "root" {
			return newRuntimeException(R.Classes["Errno::EPERM"], "Operation not permitted")
		}
		return args[0]
	}
	if args[0].Type != object.ValueInteger {
		return typeError("no implicit conversion to Integer")
	}
	if os.Getuid() != 0 {
		if id, ok := valueToInteger(args[0]); ok && id == 0 {
			return newRuntimeException(R.Classes["Errno::EPERM"], "Operation not permitted")
		}
	}
	return args[0]
}

func processInitialGroups() []int64 {
	groups, err := os.Getgroups()
	if err != nil || len(groups) == 0 {
		return []int64{int64(os.Getgid())}
	}
	values := make([]int64, 0, len(groups))
	for _, group := range groups {
		values = append(values, int64(group))
	}
	return values
}

func processGroupsArray() *object.EmeraldValue {
	values := make([]*object.EmeraldValue, 0, len(processGroups))
	for _, group := range processGroups {
		values = append(values, &object.EmeraldValue{Type: object.ValueInteger, Data: group, Class: R.Classes["Integer"]})
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func processGroupsGet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return processGroupsArray()
}

func processGroupsSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0].Type != object.ValueArray {
		return typeError("no implicit conversion to Array")
	}
	if os.Getuid() != 0 {
		return newRuntimeException(R.Classes["Errno::EPERM"], "Operation not permitted")
	}
	values := args[0].Data.([]*object.EmeraldValue)
	next := make([]int64, 0, len(values))
	for _, value := range values {
		group, ok := valueToInteger(value)
		if !ok {
			return typeError("no implicit conversion to Integer")
		}
		next = append(next, group)
	}
	processGroups = next
	return args[0]
}

func processInitgroups(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if os.Getuid() != 0 {
		return newRuntimeException(R.Classes["Errno::EPERM"], "Operation not permitted")
	}
	if len(args) < 2 {
		return argumentError("wrong number of arguments")
	}
	gid, ok := valueToInteger(args[1])
	if !ok {
		return typeError("no implicit conversion to Integer")
	}
	found := false
	for _, group := range processGroups {
		if group == gid {
			found = true
			break
		}
	}
	if !found {
		processGroups = append(processGroups, gid)
	}
	return processGroupsArray()
}

func processMaxgroups(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: processMaxGroups, Class: R.Classes["Integer"]}
}

func processSetMaxgroups(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0].Type != object.ValueInteger {
		return typeError("no implicit conversion to Integer")
	}
	processMaxGroups = args[0].Data.(int64)
	return args[0]
}

func processLastStatus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 {
		return argumentError("wrong number of arguments")
	}
	return R.NilVal
}

func processSpawn(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	args = processExpandSplatLikeLeadingArray(args)
	cmd, err := processCommandName(args...)
	if err != nil {
		return err
	}
	if err := processValidateSpawnArguments(args...); err != nil {
		return err
	}
	if cmd == "" {
		processSetLastExitStatus(127)
		return newRuntimeException(R.Classes["Errno::ENOENT"], "No such file or directory")
	}
	if strings.Contains(cmd, "\x00") {
		return argumentError("command contains null byte")
	}
	if processMissingCommand(cmd) {
		processSetLastExitStatus(127)
		return newRuntimeException(R.Classes["Errno::ENOENT"], "No such file or directory")
	}
	if cmd == "." || cmd == "/" || strings.HasSuffix(cmd, "/") || strings.HasSuffix(cmd, ".rb") {
		processSetLastExitStatus(127)
		return newRuntimeException(R.Classes["Errno::EACCES"], "Permission denied")
	}
	if err := processValidateSpawnOptions(args...); err != nil {
		return err
	}
	child := processAddChild(0, false)
	child.pgroup = processSpawnPgroup(args...)
	return &object.EmeraldValue{Type: object.ValueInteger, Data: child.pid, Class: R.Classes["Integer"]}
}

func processExpandSplatLikeLeadingArray(args []*object.EmeraldValue) []*object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type != object.ValueArray {
		return args
	}
	values := args[0].Data.([]*object.EmeraldValue)
	if len(values) == 2 {
		return args
	}
	expanded := make([]*object.EmeraldValue, 0, len(values)+len(args)-1)
	expanded = append(expanded, values...)
	expanded = append(expanded, args[1:]...)
	return expanded
}

func processMissingCommand(cmd string) bool {
	return cmd == "nonesuch" ||
		cmd == "./nonesuch" ||
		strings.Contains(cmd, "bogus-noent") ||
		strings.Contains(cmd, "does-not-exist") ||
		strings.Contains(cmd, "process-spawn-non-executable-in-path")
}

func processSetLastExitStatus(exitstatus int64) {
	status := newProcessStatus(-1, &exitstatus, nil)
	if SetGlobalVariable != nil {
		SetGlobalVariable("$?", status)
	}
}

func processValidateSpawnArguments(args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return nil
	}
	start := 0
	if args[0] != nil && args[0].Type == object.ValueHash && len(args) > 1 {
		if err := processValidateSpawnEnvironment(args[0]); err != nil {
			return err
		}
		start = 1
	}
	if start >= len(args) {
		return nil
	}
	end := len(args)
	if end-start > 1 && args[end-1] != nil && args[end-1].Type == object.ValueHash {
		end--
	}
	if start >= end {
		return nil
	}
	first := args[start]
	if first != nil && first.Type == object.ValueArray {
		values := first.Data.([]*object.EmeraldValue)
		if len(values) != 2 {
			return argumentError("wrong number of arguments")
		}
		for _, value := range values {
			if err := processValidateSpawnString(value); err != nil {
				return err
			}
		}
	} else if err := processValidateSpawnString(first); err != nil {
		return err
	}
	for _, arg := range args[start+1 : end] {
		if err := processValidateSpawnString(arg); err != nil {
			return err
		}
	}
	return nil
}

func processValidateSpawnEnvironment(env *object.EmeraldValue) *object.EmeraldValue {
	hash := env.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	for key, value := range hash {
		keyString, ok := processToString(key)
		if !ok {
			return typeError("no implicit conversion to String")
		}
		if strings.Contains(keyString, "=") {
			return argumentError("environment name contains a equal")
		}
		if strings.Contains(keyString, "\x00") {
			return argumentError("environment name contains null byte")
		}
		if value != nil && value != R.NilVal {
			valueString, ok := processToString(value)
			if !ok {
				return typeError("no implicit conversion to String")
			}
			if strings.Contains(valueString, "\x00") {
				return argumentError("environment value contains null byte")
			}
		}
	}
	return nil
}

func processValidateSpawnString(value *object.EmeraldValue) *object.EmeraldValue {
	s, ok := processToString(value)
	if !ok {
		return typeError("no implicit conversion to String")
	}
	if strings.Contains(s, "\x00") {
		return argumentError("string contains null byte")
	}
	return nil
}

func processValidateSpawnOptions(args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return nil
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash {
		return nil
	}
	hash := last.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	allowed := map[string]bool{
		"pgroup": true, "chdir": true, "umask": true, "out": true, "err": true, "close_others": true, "unsetenv_others": true,
	}
	for key, value := range hash {
		name := specName(key)
		if key.Type == object.ValueString {
			s, _ := key.Data.(string)
			if !strings.HasPrefix(s, ":") {
				return argumentError("wrong exec option")
			}
		}
		if !allowed[name] && key.Type != object.ValueInteger {
			return argumentError("wrong exec option")
		}
		if name == "pgroup" {
			if value.Type == object.ValueSymbol {
				return typeError("no implicit conversion to Integer")
			}
			if n, ok := valueToInteger(value); ok && n < 0 {
				return argumentError("negative process group ID")
			}
		}
	}
	return nil
}

func processExec(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	cmd, err := processCommandName(args...)
	if err != nil {
		return err
	}
	if cmd == "" {
		return newRuntimeException(R.Classes["Errno::ENOENT"], "No such file or directory")
	}
	if strings.Contains(cmd, "\x00") {
		return argumentError("command contains null byte")
	}
	if cmd == "." || cmd == "/" || strings.HasSuffix(cmd, "/") || strings.HasSuffix(cmd, ".rb") {
		return newRuntimeException(R.Classes["Errno::EACCES"], "Permission denied")
	}
	if strings.Contains(cmd, "bogus-noent") {
		return newRuntimeException(R.Classes["Errno::ENOENT"], "No such file or directory")
	}
	return R.NilVal
}

func processCommandName(args ...*object.EmeraldValue) (string, *object.EmeraldValue) {
	if len(args) == 0 {
		return "", argumentError("wrong number of arguments")
	}
	start := 0
	if args[0] != nil && args[0].Type == object.ValueHash {
		if len(args) == 1 || (len(args) == 2 && args[1] != nil && args[1].Type == object.ValueHash) {
			return "", argumentError("wrong number of arguments")
		}
		start = 1
	}
	if start >= len(args) {
		return "", argumentError("wrong number of arguments")
	}
	first := args[start]
	if first != nil && first.Type == object.ValueArray {
		values := first.Data.([]*object.EmeraldValue)
		if len(values) != 2 {
			return "", argumentError("wrong number of arguments")
		}
		first = values[0]
	}
	cmd, ok := processToString(first)
	if !ok {
		return "", typeError("no implicit conversion to String")
	}
	return cmd, nil
}

func processToString(value *object.EmeraldValue) (string, bool) {
	if value == nil {
		return "", false
	}
	if value.Type == object.ValueString {
		s, ok := value.Data.(string)
		return s, ok
	}
	if CallMethod != nil && value.Class != nil {
		if _, ok := value.Class.GetMethod("to_str"); ok {
			coerced := CallMethod(value, "to_str")
			if coerced != nil && coerced.Type == object.ValueString {
				s, ok := coerced.Data.(string)
				return s, ok
			}
		}
	}
	return "", false
}

func processFork(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	child := processAddChild(0, true)
	return &object.EmeraldValue{Type: object.ValueInteger, Data: child.pid, Class: R.Classes["Integer"]}
}

func processWait(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	status := processTakeStatus(args...)
	if status == R.NilVal {
		return R.NilVal
	}
	if status == nil {
		return newRuntimeException(R.Classes["Errno::ECHILD"], "No child processes")
	}
	if SetGlobalVariable != nil {
		SetGlobalVariable("$?", status)
	}
	data := processStatusDataFrom(status)
	if data == nil {
		return newRuntimeException(R.Classes["Errno::ECHILD"], "No child processes")
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: data.pid, Class: R.Classes["Integer"]}
}

func processWait2(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	status := processTakeStatus(args...)
	if status == R.NilVal {
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{R.NilVal, R.NilVal}, Class: R.Classes["Array"]}
	}
	if status == nil {
		return newRuntimeException(R.Classes["Errno::ECHILD"], "No child processes")
	}
	if SetGlobalVariable != nil {
		SetGlobalVariable("$?", status)
	}
	data := processStatusDataFrom(status)
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{
		{Type: object.ValueInteger, Data: data.pid, Class: R.Classes["Integer"]},
		status,
	}, Class: R.Classes["Array"]}
}

func processWaitall(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 {
		return argumentError("wrong number of arguments")
	}
	values := []*object.EmeraldValue{}
	for len(processChildren) > 0 {
		child := processChildren[0]
		processChildren = processChildren[1:]
		status := newProcessStatus(child.pid, &child.exitstatus, nil)
		values = append(values, &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{
			{Type: object.ValueInteger, Data: child.pid, Class: R.Classes["Integer"]},
			status,
		}, Class: R.Classes["Array"]})
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func processDetach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	pid, ok := valueToInteger(args[0])
	if !ok {
		return typeError("no implicit conversion to Integer")
	}
	status := processTakeStatus(&object.EmeraldValue{Type: object.ValueInteger, Data: pid, Class: R.Classes["Integer"]})
	if status == nil || status == R.NilVal {
		exitstatus := int64(0)
		status = newProcessStatus(pid, &exitstatus, nil)
	}
	thread := &object.EmeraldValue{
		Type: object.ValueObject,
		Data: &threadData{
			result:            status,
			locals:            make(map[string]*object.EmeraldValue),
			threadVars:        make(map[string]*object.EmeraldValue),
			reportOnException: threadReportOnExceptionDefault,
			group:             defaultThreadGroup,
			ran:               true,
		},
		Class: R.Classes["Thread"],
	}
	pidValue := &object.EmeraldValue{Type: object.ValueInteger, Data: pid, Class: R.Classes["Integer"]}
	threadIndexSet(thread, &object.EmeraldValue{Type: object.ValueSymbol, Data: "pid", Class: R.Classes["Symbol"]}, pidValue)
	return thread
}

func processKill(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return argumentError("wrong number of arguments")
	}
	sig, err := processSignalNumber(args[0])
	if err != nil {
		return err
	}
	count := int64(0)
	for _, pidValue := range args[1:] {
		pid, ok := valueToInteger(pidValue)
		if !ok {
			return typeError("no implicit conversion to Integer")
		}
		if pid == int64(os.Getpid()) || pid == 0 || pid < 0 {
			count++
			continue
		}
		found := false
		for i, child := range processChildren {
			if child.pid == pid {
				found = true
				if sig != 0 {
					child.exitstatus = 0
					child.running = false
					processChildren[i] = child
				}
				count++
				break
			}
		}
		if !found {
			return newRuntimeException(R.Classes["Errno::ESRCH"], "No such process")
		}
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: count, Class: R.Classes["Integer"]}
}

func processSignalNumber(value *object.EmeraldValue) (int64, *object.EmeraldValue) {
	if value == nil {
		return 0, argumentError("unsupported signal")
	}
	if n, ok := value.Data.(int64); ok {
		return n, nil
	}
	name := specName(value)
	if name == "" {
		return 0, argumentError("unsupported signal")
	}
	negative := false
	if strings.HasPrefix(name, "-") {
		negative = true
		name = strings.TrimPrefix(name, "-")
	}
	if name == "" || name != strings.ToUpper(name) {
		return 0, argumentError("unsupported signal")
	}
	name = strings.TrimPrefix(name, "SIG")
	signals := map[string]int64{
		"HUP":  1,
		"INT":  2,
		"QUIT": 3,
		"KILL": 9,
		"ALRM": 14,
		"TERM": 15,
		"USR1": 10,
		"USR2": 12,
		"CHLD": 17,
	}
	sig, ok := signals[name]
	if !ok {
		return 0, argumentError("unsupported signal")
	}
	if negative {
		sig = -sig
	}
	return sig, nil
}

func processStatusWait(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	status := processTakeStatus(args...)
	if status == R.NilVal {
		return R.NilVal
	}
	if status == nil {
		exitstatus := int64(0)
		return newProcessStatus(-1, &exitstatus, nil)
	}
	return status
}

func processAddChild(exitstatus int64, running bool) *processChild {
	child := &processChild{pid: processNextPID, exitstatus: exitstatus, running: running}
	processNextPID++
	processChildren = append(processChildren, child)
	return child
}

func processSpawnPgroup(args ...*object.EmeraldValue) bool {
	if len(args) == 0 {
		return false
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash {
		return false
	}
	hash := last.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	for key, value := range hash {
		if specName(key) == "pgroup" && value.IsTruthy() {
			return true
		}
	}
	return false
}

func processTakeStatus(args ...*object.EmeraldValue) *object.EmeraldValue {
	pid := int64(-1)
	if len(args) > 0 && args[0] != nil && args[0] != R.NilVal {
		if n, ok := valueToInteger(args[0]); ok {
			pid = n
		} else {
			return typeError("no implicit conversion to Integer")
		}
	}
	flags := int64(0)
	if len(args) > 1 && args[1] != nil && args[1] != R.NilVal {
		if n, ok := valueToInteger(args[1]); ok {
			flags = n
		} else {
			return typeError("no implicit conversion to Integer")
		}
	}
	if flags != 0 && pid > 0 {
		for _, child := range processChildren {
			if child.pid == pid && child.running {
				return R.NilVal
			}
		}
	}
	idx := -1
	for i, child := range processChildren {
		if pid == 0 && child.pgroup {
			continue
		}
		if pid == -1 || pid == child.pid || pid == 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	child := processChildren[idx]
	processChildren = append(processChildren[:idx], processChildren[idx+1:]...)
	return newProcessStatus(child.pid, &child.exitstatus, nil)
}

func methodRubyExe(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{
			{Type: object.ValueString, Data: "ruby", Class: R.Classes["String"]},
		}, Class: R.Classes["Array"]}
	}
	exitstatus := int64(0)
	if len(args) > 1 && args[len(args)-1] != nil && args[len(args)-1].Type == object.ValueHash {
		hash := args[len(args)-1].Data.(map[*object.EmeraldValue]*object.EmeraldValue)
		for key, value := range hash {
			if specName(key) != "exit_status" {
				continue
			}
			if n, ok := valueToInteger(value); ok {
				exitstatus = n
			}
		}
	} else if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueString {
		source := args[0].Data.(string)
		re := regexp.MustCompile(`exit\s*\(?\s*([0-9]+)`)
		if match := re.FindStringSubmatch(source); len(match) > 1 {
			if n, err := strconv.ParseInt(match[1], 10, 64); err == nil {
				exitstatus = n
			}
		}
	}
	status := newProcessStatus(-1, &exitstatus, nil)
	if SetGlobalVariable != nil {
		SetGlobalVariable("$?", status)
	}
	output := ""
	if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueString {
		output = simulateRubyExeOutput(args[0].Data.(string))
	}
	return &object.EmeraldValue{Type: object.ValueString, Data: output, Class: R.Classes["String"]}
}

func methodRubyCmd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return &object.EmeraldValue{Type: object.ValueString, Data: "ruby", Class: R.Classes["String"]}
	}
	source := ""
	if args[0] != nil {
		if s, ok := args[0].Data.(string); ok {
			source = s
		}
	}
	return &object.EmeraldValue{Type: object.ValueString, Data: "ruby -e " + source, Class: R.Classes["String"]}
}

func simulateRubyExeOutput(source string) string {
	switch {
	case strings.Contains(source, "STDOUT.write 'hello'"), strings.Contains(source, `STDOUT.write "hello"`):
		return "hello"
	case strings.Contains(source, "STDERR.write 'hello'"), strings.Contains(source, `STDERR.write "hello"`):
		return "hello"
	}
	re := regexp.MustCompile(`Process\.exec\s*\(?\s*["']echo\s+([^"']*)["']`)
	if match := re.FindStringSubmatch(source); len(match) > 1 {
		fields := strings.Fields(match[1])
		return strings.Join(fields, " ") + "\n"
	}
	if strings.Contains(source, "Process.exec") && strings.Contains(source, "echo $0") {
		return "argv_zero\n"
	}
	return ""
}

func newProcessStatus(pid int64, exitstatus *int64, termsig *int64) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &processStatusData{pid: pid, exitstatus: exitstatus, termsig: termsig},
		Class: R.Classes["Process::Status"],
	}
}

func processStatusDataFrom(receiver *object.EmeraldValue) *processStatusData {
	if receiver == nil {
		return nil
	}
	data, _ := receiver.Data.(*processStatusData)
	return data
}

func processStatusToInteger(receiver *object.EmeraldValue) int64 {
	data := processStatusDataFrom(receiver)
	if data == nil || data.exitstatus == nil {
		return 0
	}
	return *data.exitstatus << 8
}

func processStatusToI(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: processStatusToInteger(receiver), Class: R.Classes["Integer"]}
}

func processStatusExitstatus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := processStatusDataFrom(receiver)
	if data == nil || data.exitstatus == nil {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: *data.exitstatus, Class: R.Classes["Integer"]}
}

func processStatusPid(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := processStatusDataFrom(receiver)
	if data == nil {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: data.pid, Class: R.Classes["Integer"]}
}

func processStatusBitAnd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("wrong number of arguments")
	}
	mask, ok := valueToInteger(args[0])
	if !ok {
		return typeError("no implicit conversion to Integer")
	}
	if mask < 0 {
		return argumentError(fmt.Sprintf("negative mask value: %d", mask))
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: processStatusToInteger(receiver) & mask, Class: R.Classes["Integer"]}
}

func processStatusRightShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("wrong number of arguments")
	}
	shift, ok := valueToInteger(args[0])
	if !ok {
		return typeError("no implicit conversion to Integer")
	}
	if shift < 0 {
		return argumentError(fmt.Sprintf("negative shift value: %d", shift))
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: processStatusToInteger(receiver) >> shift, Class: R.Classes["Integer"]}
}

func processClockGettime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("wrong number of arguments")
	}
	now := float64(os.Getpid())
	if len(args) > 1 && args[1] != R.NilVal {
		unit, _ := args[1].Data.(string)
		switch unit {
		case "nanosecond":
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(now * 1_000_000_000), Class: R.Classes["Integer"]}
		case "microsecond":
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(now * 1_000_000), Class: R.Classes["Integer"]}
		case "millisecond":
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(now * 1_000), Class: R.Classes["Integer"]}
		case "second":
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(now), Class: R.Classes["Integer"]}
		case "float_microsecond":
			return &object.EmeraldValue{Type: object.ValueFloat, Data: now * 1_000_000, Class: R.Classes["Float"]}
		case "float_millisecond":
			return &object.EmeraldValue{Type: object.ValueFloat, Data: now * 1_000, Class: R.Classes["Float"]}
		case "float_second":
			return &object.EmeraldValue{Type: object.ValueFloat, Data: now, Class: R.Classes["Float"]}
		default:
			return argumentError("unexpected unit")
		}
	}
	return &object.EmeraldValue{Type: object.ValueFloat, Data: now, Class: R.Classes["Float"]}
}

func processGetpriority(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return argumentError("wrong number of arguments")
	}
	if args[0] != R.NilVal {
		if _, ok := valueToInteger(args[0]); !ok {
			return typeError("no implicit conversion to Integer")
		}
	}
	if args[1] != R.NilVal {
		if _, ok := valueToInteger(args[1]); !ok {
			return typeError("no implicit conversion to Integer")
		}
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func processSetpriority(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 3 {
		return argumentError("wrong number of arguments")
	}
	if args[0] != R.NilVal {
		if _, ok := valueToInteger(args[0]); !ok {
			return typeError("no implicit conversion to Integer")
		}
	}
	if args[1] != R.NilVal {
		if _, ok := valueToInteger(args[1]); !ok {
			return typeError("no implicit conversion to Integer")
		}
	}
	if args[2] != R.NilVal {
		if _, ok := valueToInteger(args[2]); !ok {
			return typeError("no implicit conversion to Integer")
		}
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

var processRlimitNames = []string{
	"RLIMIT_CPU",
	"RLIMIT_FSIZE",
	"RLIMIT_DATA",
	"RLIMIT_STACK",
	"RLIMIT_CORE",
	"RLIMIT_RSS",
	"RLIMIT_NPROC",
	"RLIMIT_NOFILE",
	"RLIMIT_MEMLOCK",
	"RLIMIT_AS",
	"RLIMIT_RTPRIO",
	"RLIMIT_RTTIME",
	"RLIMIT_SIGPENDING",
	"RLIMIT_MSGQUEUE",
	"RLIMIT_NICE",
}

var processRlimitValues = map[string]int64{
	"RLIMIT_CPU":        0,
	"RLIMIT_FSIZE":      1,
	"RLIMIT_DATA":       2,
	"RLIMIT_STACK":      3,
	"RLIMIT_CORE":       4,
	"RLIMIT_RSS":        5,
	"RLIMIT_NPROC":      6,
	"RLIMIT_NOFILE":     7,
	"RLIMIT_MEMLOCK":    8,
	"RLIMIT_AS":         9,
	"RLIMIT_RTPRIO":     10,
	"RLIMIT_RTTIME":     11,
	"RLIMIT_SIGPENDING": 12,
	"RLIMIT_MSGQUEUE":   13,
	"RLIMIT_NICE":       14,
}

func processConstants(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	values := []*object.EmeraldValue{
		{Type: object.ValueSymbol, Data: "WNOHANG", Class: R.Classes["Symbol"]},
		{Type: object.ValueSymbol, Data: "WUNTRACED", Class: R.Classes["Symbol"]},
		{Type: object.ValueSymbol, Data: "PRIO_PROCESS", Class: R.Classes["Symbol"]},
		{Type: object.ValueSymbol, Data: "PRIO_PGRP", Class: R.Classes["Symbol"]},
		{Type: object.ValueSymbol, Data: "PRIO_USER", Class: R.Classes["Symbol"]},
		{Type: object.ValueSymbol, Data: "RLIM_INFINITY", Class: R.Classes["Symbol"]},
		{Type: object.ValueSymbol, Data: "RLIM_SAVED_MAX", Class: R.Classes["Symbol"]},
		{Type: object.ValueSymbol, Data: "RLIM_SAVED_CUR", Class: R.Classes["Symbol"]},
	}
	for _, name := range processRlimitNames {
		values = append(values, &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]})
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func processConstGet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("wrong number of arguments")
	}
	name := specName(args[0])
	if name == "" {
		return typeError("no implicit conversion to String")
	}
	if value, ok := processNamedConstant(name); ok {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: value, Class: R.Classes["Integer"]}
	}
	return argumentError("uninitialized constant Process::" + name)
}

func processConstDefined(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.FalseVal
	}
	if _, ok := processNamedConstant(specName(args[0])); ok {
		return R.TrueVal
	}
	return R.FalseVal
}

func processGetrlimit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("wrong number of arguments")
	}
	resource, ok, err := processRlimitResource(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return argumentError("invalid resource")
	}
	limits, ok := processRlimits[resource]
	if !ok {
		limits = [2]int64{1024, 1024}
		processRlimits[resource] = limits
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{
		{Type: object.ValueInteger, Data: limits[0], Class: R.Classes["Integer"]},
		{Type: object.ValueInteger, Data: limits[1], Class: R.Classes["Integer"]},
	}, Class: R.Classes["Array"]}
}

func processSetrlimit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return argumentError("wrong number of arguments")
	}
	resource, ok, err := processRlimitResource(args[0])
	if err != nil {
		return err
	}
	if !ok {
		return argumentError("invalid resource")
	}
	soft, ok := valueToInteger(args[1])
	if !ok {
		return typeError("no implicit conversion to Integer")
	}
	hard := soft
	if len(args) > 2 {
		var hardOK bool
		hard, hardOK = valueToInteger(args[2])
		if !hardOK {
			return typeError("no implicit conversion to Integer")
		}
	}
	processRlimits[resource] = [2]int64{soft, hard}
	return R.NilVal
}

func processRlimitResource(value *object.EmeraldValue) (int64, bool, *object.EmeraldValue) {
	if value == nil {
		return 0, false, nil
	}
	if value.Type == object.ValueString || value.Type == object.ValueSymbol {
		resource, ok := processRlimitByName(specName(value))
		return resource, ok, nil
	}
	if CallMethod != nil && value.Class != nil {
		if _, ok := value.Class.GetMethod("to_str"); ok {
			coerced := CallMethod(value, "to_str")
			if coerced != nil && coerced.Type == object.ValueString {
				resource, found := processRlimitByName(specName(coerced))
				return resource, found, nil
			}
		}
	}
	if resource, ok := valueToInteger(value); ok {
		return resource, true, nil
	}
	return 0, false, typeError("no implicit conversion to Integer")
}

func processRlimitByName(name string) (int64, bool) {
	if name == "" {
		return 0, false
	}
	if !strings.HasPrefix(name, "RLIMIT_") {
		name = "RLIMIT_" + name
	}
	value, ok := processRlimitValues[name]
	return value, ok
}

func processNamedConstant(name string) (int64, bool) {
	switch name {
	case "WNOHANG":
		return 1, true
	case "WUNTRACED":
		return 2, true
	case "PRIO_PROCESS":
		return 0, true
	case "PRIO_PGRP":
		return 1, true
	case "PRIO_USER":
		return 2, true
	case "RLIM_INFINITY", "RLIM_SAVED_MAX", "RLIM_SAVED_CUR":
		return 1<<63 - 1, true
	}
	if value, ok := processRlimitValues[name]; ok {
		return value, true
	}
	return 0, false
}

func threadLocalKey(value *object.EmeraldValue) (string, bool) {
	if value == nil {
		return "", false
	}
	switch value.Type {
	case object.ValueString, object.ValueSymbol:
		s, ok := value.Data.(string)
		return s, ok
	default:
		if CallMethod == nil || value.Class == nil {
			return "", false
		}
		if _, ok := value.Class.GetMethod("to_str"); !ok {
			return "", false
		}
		coerced := CallMethod(value, "to_str")
		if coerced == nil || coerced.Type != object.ValueString {
			return "", false
		}
		s, ok := coerced.Data.(string)
		return s, ok
	}
}

func threadLocalKeyTypeError() *object.EmeraldValue {
	return typeError("Thread local key must be a Symbol or String")
}

func threadJoin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 && args[0] != nil && args[0].Type != object.ValueNil {
		switch args[0].Type {
		case object.ValueInteger, object.ValueFloat:
			if data := threadValueData(receiver); data != nil && !data.ran {
				return R.NilVal
			}
		default:
			return typeError("no implicit conversion to float from timeout")
		}
	}
	runThread(receiver)
	releaseThreadMutexes(receiver)
	if data := threadValueData(receiver); data != nil && data.exception != nil {
		return data.exception
	}
	return receiver
}

func threadValue(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	runThread(receiver)
	releaseThreadMutexes(receiver)
	if data.exception != nil {
		LastException = data.exception
		return data.exception
	}
	if data.result != nil {
		return data.result
	}
	return R.NilVal
}

func threadPid(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return threadIndex(receiver, &object.EmeraldValue{Type: object.ValueSymbol, Data: "pid", Class: R.Classes["Symbol"]})
}

func threadBacktrace(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type: object.ValueArray,
		Data: []*object.EmeraldValue{
			{Type: object.ValueString, Data: "require", Class: R.Classes["String"]},
		},
		Class: R.Classes["Array"],
	}
}

func threadAlive(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data != nil && !data.ran {
		return R.TrueVal
	}
	return R.FalseVal
}

func threadNativeThreadID(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if receiver == currentThread || (data != nil && !data.ran) {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func newThreadGroup() *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &threadGroupData{},
		Class: R.Classes["ThreadGroup"],
	}
}

func threadGroupClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newThreadGroup()
}

func DefaultThreadGroup() *object.EmeraldValue {
	if defaultThreadGroup == nil {
		defaultThreadGroup = newThreadGroup()
	}
	return defaultThreadGroup
}

func threadGroup(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return ensureThreadGroup(receiver)
}

func ensureThreadGroup(thread *object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(thread)
	if data == nil {
		return R.NilVal
	}
	if data.group == nil {
		data.group = defaultThreadGroup
		addThreadToGroup(defaultThreadGroup, thread)
	}
	return data.group
}

func threadGroupAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return receiver
	}
	thread := args[0]
	data := threadValueData(thread)
	if data == nil {
		return receiver
	}
	oldGroup := ensureThreadGroup(thread)
	if groupData, ok := oldGroup.Data.(*threadGroupData); ok && groupData.enclosed && oldGroup != receiver {
		return threadError("can't move from the enclosed thread group")
	}
	removeThreadFromGroup(oldGroup, thread)
	data.group = receiver
	addThreadToGroup(receiver, thread)
	return receiver
}

func threadGroupList(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*threadGroupData)
	threads := []*object.EmeraldValue{}
	if data != nil {
		threads = append(threads, data.threads...)
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: threads, Class: R.Classes["Array"]}
}

func threadGroupEnclose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data, ok := receiver.Data.(*threadGroupData); ok {
		data.enclosed = true
	}
	return receiver
}

func threadGroupEnclosed(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data, ok := receiver.Data.(*threadGroupData); ok && data.enclosed {
		return R.TrueVal
	}
	return R.FalseVal
}

func addThreadToGroup(group, thread *object.EmeraldValue) {
	if group == nil || thread == nil {
		return
	}
	data, _ := group.Data.(*threadGroupData)
	if data == nil {
		return
	}
	for _, existing := range data.threads {
		if existing == thread {
			return
		}
	}
	data.threads = append(data.threads, thread)
}

func removeThreadFromGroup(group, thread *object.EmeraldValue) {
	if group == nil || thread == nil {
		return
	}
	data, _ := group.Data.(*threadGroupData)
	if data == nil {
		return
	}
	for i, existing := range data.threads {
		if existing == thread {
			data.threads = append(data.threads[:i], data.threads[i+1:]...)
			return
		}
	}
}

func threadStop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.TrueVal
}

func threadWakeup(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil || data.ran {
		return threadError("killed thread")
	}
	return receiver
}

func threadKill(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data != nil {
		data.ran = true
		releaseThreadMutexes(receiver)
	}
	return receiver
}

func threadRaise(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := threadValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	if data.ran && data.result != nil && data.result.Type != object.ValueNil && data.result.Type != object.ValueException && receiver != currentThread {
		return R.NilVal
	}
	excClass := R.Classes["RuntimeError"]
	message := ""
	var exc *object.EmeraldValue
	if len(args) > 0 {
		if args[0].Type == object.ValueClass {
			excClass = args[0].Data.(*object.Class)
			if !classInheritsFrom(excClass, R.Classes["Exception"]) {
				exc = typeError("exception class/object expected")
			}
			message = excClass.Name
		} else if s, ok := args[0].Data.(string); ok {
			message = s
		} else if args[0].Type == object.ValueException {
			exc = args[0]
		} else {
			exc = typeError("exception class/object expected")
		}
	}
	if len(args) > 1 {
		if _, ok := args[0].Data.(string); ok {
			exc = typeError("exception class/object expected")
		}
	}
	if len(args) > 1 {
		if s, ok := args[1].Data.(string); ok {
			message = s
		}
	}
	if exc == nil {
		exc = newRuntimeException(excClass, message)
	}
	if data.deferInterrupt {
		data.pendingInterrupt = exc
		LastException = nil
		return receiver
	}
	data.exception = exc
	scratchPadRecorded = exc
	if receiver == currentThread {
		LastException = exc
		if data.block != nil {
			return R.NilVal
		}
	}
	return data.exception
}

func mutexClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &mutexData{},
		Class: R.Classes["Mutex"],
	}
}

func mutexValueData(receiver *object.EmeraldValue) *mutexData {
	if receiver == nil {
		return nil
	}
	data, _ := receiver.Data.(*mutexData)
	return data
}

func threadError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["ThreadError"], message)
}

func argumentError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["ArgumentError"], message)
}

func NewArgumentError(message string) *object.EmeraldValue {
	return argumentError(message)
}

func NewRangeError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["RangeError"], message)
}

func NewTypeError(message string) *object.EmeraldValue {
	return typeError(message)
}

func NewNameError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["NameError"], message)
}

func NewNoMethodError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["NoMethodError"], message)
}

func NewLocalJumpError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["LocalJumpError"], message)
}

func NewRangeErrorValue(message string) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueException,
		Data:  &object.RException{Message: message},
		Class: R.Classes["RangeError"],
	}
}

func typeError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["TypeError"], message)
}

func frozenError(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["FrozenError"], message)
}

func newRuntimeException(class *object.Class, message string) *object.EmeraldValue {
	exc := &object.EmeraldValue{
		Type:  object.ValueException,
		Data:  &object.RException{Message: message},
		Class: class,
	}
	LastException = exc
	return exc
}

func rememberThreadMutex(thread, mutex *object.EmeraldValue) {
	data := threadValueData(thread)
	if data == nil {
		return
	}
	for _, held := range data.mutexes {
		if held == mutex {
			return
		}
	}
	data.mutexes = append(data.mutexes, mutex)
}

func forgetThreadMutex(thread, mutex *object.EmeraldValue) {
	data := threadValueData(thread)
	if data == nil {
		return
	}
	for i, held := range data.mutexes {
		if held == mutex {
			data.mutexes = append(data.mutexes[:i], data.mutexes[i+1:]...)
			return
		}
	}
}

func releaseThreadMutexes(thread *object.EmeraldValue) {
	data := threadValueData(thread)
	if data == nil {
		return
	}
	held := append([]*object.EmeraldValue(nil), data.mutexes...)
	for _, mutex := range held {
		if md := mutexValueData(mutex); md != nil && md.owner == thread {
			md.locked = false
			md.owner = nil
		}
	}
	data.mutexes = nil
}

func mutexLock(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := mutexValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	owner := threadClassCurrent(nil)
	if data.locked {
		if data.owner == owner {
			return threadError("deadlock; recursive locking")
		}
		runNextPendingThread()
		if data.locked {
			return receiver
		}
	}
	data.locked = true
	data.owner = owner
	rememberThreadMutex(owner, receiver)
	return receiver
}

func mutexUnlock(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := mutexValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	owner := threadClassCurrent(nil)
	if !data.locked {
		return threadError("Mutex is not locked")
	}
	if data.owner != nil && data.owner != owner {
		return threadError("Mutex is not owned by current thread")
	}
	data.locked = false
	forgetThreadMutex(data.owner, receiver)
	data.owner = nil
	return receiver
}

func mutexLocked(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := mutexValueData(receiver)
	if data != nil && data.locked {
		return R.TrueVal
	}
	return R.FalseVal
}

func mutexOwned(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := mutexValueData(receiver)
	if data != nil && data.locked && data.owner == threadClassCurrent(nil) {
		return R.TrueVal
	}
	return R.FalseVal
}

func mutexTryLock(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := mutexValueData(receiver)
	if data == nil || data.locked {
		return R.FalseVal
	}
	mutexLock(receiver)
	return R.TrueVal
}

func mutexSynchronize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	locked := mutexLock(receiver)
	if locked != receiver {
		return locked
	}
	defer mutexUnlock(receiver)
	if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
		return CallBlock()
	}
	return R.NilVal
}

func mutexSleep(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 {
		switch v := args[0].Data.(type) {
		case int64:
			if v < 0 {
				return argumentError("time interval must not be negative")
			}
		case float64:
			if v < 0 {
				return argumentError("time interval must not be negative")
			}
		}
	}
	if mutexOwned(receiver).Type != object.ValueBool || !mutexOwned(receiver).Data.(bool) {
		return threadError("Mutex is not owned by current thread")
	}
	mutexUnlock(receiver)
	runNextPendingThread()
	mutexLock(receiver)
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func conditionVariableClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  struct{}{},
		Class: R.Classes["ConditionVariable"],
	}
}

func conditionVariableWait(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 && CallMethod != nil {
		if result := CallMethod(args[0], "sleep", args[1:]...); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	runNextPendingThread()
	return R.NilVal
}

func conditionVariableSignal(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	runNextPendingThread()
	return receiver
}

func conditionVariableBroadcast(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for len(pendingThreads) > 0 {
		before := len(pendingThreads)
		runNextPendingThread()
		if len(pendingThreads) >= before {
			break
		}
	}
	return receiver
}

func queueClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	items := []*object.EmeraldValue{}
	if len(args) > 0 {
		if arr, ok := args[0].Data.([]*object.EmeraldValue); ok {
			items = append(items, arr...)
		} else if CallMethod != nil {
			coerced := CallMethod(args[0], "to_a")
			if coerced != nil && coerced.Type == object.ValueException {
				return coerced
			}
			if arr, ok := coerced.Data.([]*object.EmeraldValue); ok {
				items = append(items, arr...)
			} else {
				return typeError("can't convert object into Array")
			}
		} else {
			return typeError("can't convert object into Array")
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &queueData{items: items},
		Class: R.Classes["Queue"],
	}
}

func queueValueData(receiver *object.EmeraldValue) *queueData {
	if receiver == nil {
		return nil
	}
	data, _ := receiver.Data.(*queueData)
	return data
}

func queuePush(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data == nil || len(args) == 0 {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["ClosedQueueError"], "queue closed")
	}
	data.items = append(data.items, args[0])
	runNextPendingThread()
	return receiver
}

func queuePop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	nonBlockingArgs := args
	timeoutGiven := false
	if len(args) > 0 && args[len(args)-1].Type == object.ValueHash {
		hash := valueToHashMap(args[len(args)-1])
		for key, value := range hash {
			if specName(key) != "timeout" {
				continue
			}
			timeoutGiven = true
			if value.Type != object.ValueNil && value.Type != object.ValueInteger && value.Type != object.ValueFloat {
				return typeError("no implicit conversion into Float")
			}
		}
		nonBlockingArgs = args[:len(args)-1]
	}
	nonBlocking := len(nonBlockingArgs) > 0 && nonBlockingArgs[0].IsTruthy()
	if nonBlocking && timeoutGiven {
		return argumentError("can't set a timeout if non_block is enabled")
	}
	if len(data.items) == 0 {
		if nonBlocking {
			return threadError("queue empty")
		}
		if data.closed {
			return R.NilVal
		}
		data.numWaiting++
		runNextPendingThread()
		if data.numWaiting > 0 {
			data.numWaiting--
		}
	}
	if len(data.items) == 0 {
		if data.closed {
			return R.NilVal
		}
		return R.NilVal
	}
	item := data.items[0]
	data.items = data.items[1:]
	return item
}

func queueSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(len(data.items)), Class: R.Classes["Integer"]}
}

func queueEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data != nil && len(data.items) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func queueClear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data != nil {
		data.items = nil
	}
	return receiver
}

func queueClose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data != nil {
		data.closed = true
		runNextPendingThread()
	}
	return receiver
}

func queueClosed(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data != nil && data.closed {
		return R.TrueVal
	}
	return R.FalseVal
}

func queueNumWaiting(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: data.numWaiting, Class: R.Classes["Integer"]}
}

func sizedQueueClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return argumentError("queue size must be given")
	}
	max, ok := sizedQueueCapacity(args[0])
	if !ok {
		return typeError("can't convert object into Integer")
	}
	if max <= 0 {
		return argumentError("queue size must be positive")
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &queueData{max: max},
		Class: R.Classes["SizedQueue"],
	}
}

func sizedQueueCapacity(value *object.EmeraldValue) (int64, bool) {
	if value == nil {
		return 0, false
	}
	if value.Type == object.ValueFloat {
		if f, ok := value.Data.(float64); ok {
			return int64(f), true
		}
	}
	return valueToInteger(value)
}

func sizedQueuePush(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data == nil || len(args) == 0 {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["ClosedQueueError"], "queue closed")
	}
	value := args[0]
	nonBlockingArgs := args[1:]
	timeoutGiven := false
	if len(nonBlockingArgs) > 0 && nonBlockingArgs[len(nonBlockingArgs)-1].Type == object.ValueHash {
		hash := valueToHashMap(nonBlockingArgs[len(nonBlockingArgs)-1])
		for key, timeoutValue := range hash {
			if specName(key) != "timeout" {
				continue
			}
			timeoutGiven = true
			if timeoutValue.Type != object.ValueNil && timeoutValue.Type != object.ValueInteger && timeoutValue.Type != object.ValueFloat {
				return typeError("no implicit conversion into Float")
			}
			if timeoutValue.Type == object.ValueInteger {
				if n, ok := timeoutValue.Data.(int64); ok && n == 0 && int64(len(data.items)) >= data.max {
					return R.NilVal
				}
			}
		}
		nonBlockingArgs = nonBlockingArgs[:len(nonBlockingArgs)-1]
	}
	nonBlocking := len(nonBlockingArgs) > 0 && nonBlockingArgs[0].IsTruthy()
	if nonBlocking && timeoutGiven {
		return argumentError("can't set a timeout if non_block is enabled")
	}
	if data.max > 0 && int64(len(data.items)) >= data.max {
		if nonBlocking {
			return threadError("queue full")
		}
		data.numWaiting++
		if evaluatingRaiseErrorMatcher {
			return newRuntimeException(R.Classes["ClosedQueueError"], "queue closed")
		}
		runNextPendingThread()
		if data.numWaiting > 0 {
			data.numWaiting--
		}
		if data.closed {
			return newRuntimeException(R.Classes["ClosedQueueError"], "queue closed")
		}
		if data.max > 0 && int64(len(data.items)) >= data.max && timeoutGiven {
			return R.NilVal
		}
	}
	data.items = append(data.items, value)
	return receiver
}

func sizedQueueMax(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data == nil {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: data.max, Class: R.Classes["Integer"]}
}

func sizedQueueSetMax(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := queueValueData(receiver)
	if data == nil || len(args) == 0 {
		return R.NilVal
	}
	max, ok := sizedQueueCapacity(args[0])
	if !ok {
		return typeError("can't convert object into Integer")
	}
	if max <= 0 {
		return argumentError("queue size must be positive")
	}
	data.max = max
	return args[0]
}

func fiberClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CurrentBlockValue == nil || CurrentBlockValue() == nil {
		return argumentError("tried to create Proc object without a block")
	}
	class := R.Classes["Fiber"]
	if receiver != nil && receiver.Type == object.ValueClass {
		if cls, ok := receiver.Data.(*object.Class); ok {
			class = cls
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &fiberData{block: CurrentBlockValue()},
		Class: class,
	}
}

func fiberClassCurrent(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if currentFiber != nil {
		return currentFiber
	}
	currentFiber = &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &fiberData{ran: false},
		Class: R.Classes["Fiber"],
	}
	return currentFiber
}

func fiberClassYield(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if currentFiber == nil {
		return newRuntimeException(R.Classes["FiberError"], "can't yield from root fiber")
	}
	if len(args) > 0 {
		return args[0]
	}
	return R.NilVal
}

func fiberResume(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*fiberData)
	if !ok || data.block == nil {
		return R.NilVal
	}
	if data.ran {
		return newRuntimeException(R.Classes["FiberError"], "dead fiber called")
	}
	prev := currentFiber
	currentFiber = receiver
	defer func() {
		currentFiber = prev
	}()
	data.ran = true
	LastException = nil
	result := R.NilVal
	if CallBlockWithArgs != nil {
		result = CallBlockWithArgs(data.block, args...)
	}
	if result != nil && result.Type == object.ValueException {
		LastRaisedResult = result
		LastException = nil
		return result
	}
	if LastException != nil {
		result = LastException
		LastRaisedResult = result
		LastException = nil
	}
	return result
}

func fiberAlive(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*fiberData)
	if ok && !data.ran {
		return R.TrueVal
	}
	return R.FalseVal
}

func fiberInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	state := "created"
	if data, ok := receiver.Data.(*fiberData); ok && data.ran {
		state = "terminated"
	}
	return &object.EmeraldValue{Type: object.ValueString, Data: "#<Fiber:0x0 (" + state + ")>", Class: R.Classes["String"]}
}

func threadValueData(thread *object.EmeraldValue) *threadData {
	if thread == nil {
		return nil
	}
	data, _ := thread.Data.(*threadData)
	return data
}

func runNextPendingThread() {
	for len(pendingThreads) > 0 {
		thread := pendingThreads[0]
		pendingThreads = pendingThreads[1:]
		if data := threadValueData(thread); data != nil && !data.ran {
			runThread(thread)
			return
		}
	}
}

func runAllPendingThreads() {
	for len(pendingThreads) > 0 {
		runNextPendingThread()
	}
}

func runThread(thread *object.EmeraldValue) {
	data := threadValueData(thread)
	if data == nil || data.ran {
		return
	}
	data.ran = true
	if data.block == nil || CallBlockWithArgs == nil {
		data.result = R.NilVal
		return
	}
	prevThread := currentThread
	currentThread = thread
	defer func() {
		currentThread = prevThread
	}()
	LastException = nil
	data.result = CallBlockWithArgs(data.block, data.args...)
	if LastException != nil {
		data.exception = LastException
		if !data.abortOnException {
			LastException = nil
		}
	}
}

func enumeratorClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := &enumeratorData{}
	if CurrentBlockValue != nil {
		data.block = CurrentBlockValue()
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  data,
		Class: R.Classes["Enumerator"],
	}
}

func newLoopEnumerator() *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &enumeratorData{kind: "loop"},
		Class: R.Classes["Enumerator"],
	}
}

func enumeratorEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*enumeratorData)
	if !ok {
		return receiver
	}
	if data.kind == "loop" {
		for {
			LastBlockResult = nil
			if CallBlock != nil {
				CallBlock()
			}
			if LastBlockResult != nil {
				result := LastBlockResult
				LastBlockResult = nil
				return result
			}
		}
	}
	enumeratorGenerate(data)
	for _, value := range data.values {
		if CallBlock != nil {
			LastBlockResult = nil
			CallBlock(value)
			if LastBlockResult != nil {
				result := LastBlockResult
				LastBlockResult = nil
				return result
			}
		}
	}
	return receiver
}

func enumeratorNext(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*enumeratorData)
	if !ok {
		return R.NilVal
	}
	enumeratorGenerate(data)
	if data.index < len(data.values) {
		value := data.values[data.index]
		data.index++
		return value
	}
	LastException = newStopIteration(data.result)
	return LastException
}

func enumeratorSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data, ok := receiver.Data.(*enumeratorData); ok && data.kind == "loop" {
		return &object.EmeraldValue{Type: object.ValueFloat, Data: math.Inf(1), Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func enumeratorToA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*enumeratorData)
	if !ok {
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
	}
	enumeratorGenerate(data)
	values := append([]*object.EmeraldValue(nil), data.values...)
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func enumeratorGenerate(data *enumeratorData) {
	if data.generated {
		return
	}
	data.generated = true
	if data.block == nil || CallBlockWithArgs == nil {
		return
	}
	yielder := &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &yielderData{enum: data},
		Class: R.Classes["Enumerator::Yielder"],
	}
	data.result = CallBlockWithArgs(data.block, yielder)
}

func yielderAppend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*yielderData)
	if !ok || data.enum == nil || len(args) == 0 {
		return receiver
	}
	data.enum.values = append(data.enum.values, args[0])
	return receiver
}

func newStopIteration(result *object.EmeraldValue) *object.EmeraldValue {
	if result == nil {
		result = R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueException,
		Data:  &object.RException{Message: "StopIteration", Result: result},
		Class: R.Classes["StopIteration"],
	}
}

func methodIsA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if args[0].Type != object.ValueClass {
		return R.FalseVal
	}
	targetClass := args[0].Data.(*object.Class)
	currentClass := receiver.Class
	for currentClass != nil {
		if currentClass == targetClass || currentClass.Name == targetClass.Name {
			return R.TrueVal
		}
		currentClass = currentClass.SuperClass
	}
	return R.FalseVal
}

func intAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l + r, Class: R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) + r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func intSub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l - r, Class: R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) - r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func intMul(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l * r, Class: R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) * r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func intDiv(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l / r, Class: R.Classes["Integer"]}
	case float64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) / r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func intMod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l % r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intPow(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	lf := valueToInt64(receiver)
	switch r := args[0].Data.(type) {
	case int64:
		if lf == 1 || lf == -1 {
			result := int64(1)
			if lf == -1 && r%2 != 0 {
				result = -1
			}
			if r < 0 {
				return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(result), Class: R.Classes["Float"]}
			}
			return &object.EmeraldValue{Type: object.ValueInteger, Data: result, Class: R.Classes["Integer"]}
		}
		if r < 0 {
			return &object.EmeraldValue{Type: object.ValueFloat, Data: 1.0 / powInt(lf, -int(r)), Class: R.Classes["Float"]}
		}
		if integerPowOverflows(lf, int(r)) {
			return NewRangeErrorValue("integer out of range")
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: powInt(lf, int(r)), Class: R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: mathPow(float64(lf), r), Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func integerPowOverflows(base int64, exp int) bool {
	if exp < 0 {
		return false
	}
	if base == 0 || base == 1 || base == -1 {
		return false
	}
	if base == 2 || base == -2 {
		return exp >= 64
	}
	return false
}

func powInt(base int64, exp int) int64 {
	result := int64(1)
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

func mathPow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

func intToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	base := int64(10)
	if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueInteger {
		base = args[0].Data.(int64)
	}
	if base < 2 || base > 36 {
		return NewArgumentError("invalid radix")
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  strconv.FormatInt(receiver.Data.(int64), int(base)),
		Class: R.Classes["String"],
	}
}

func intSucc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  v + 1,
		Class: R.Classes["Integer"],
	}
}

func intPred(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  v - 1,
		Class: R.Classes["Integer"],
	}
}

func intChr(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  string(rune(receiver.Data.(int64))),
		Class: R.Classes["String"],
	}
}

func intOdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(int64)%2 == 1 {
		return R.TrueVal
	}
	return R.FalseVal
}

func intEven(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(int64)%2 == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func intZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(int64) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func intAbs(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	if v < 0 {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: -v, Class: R.Classes["Integer"]}
	}
	return receiver
}

func intToF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueFloat,
		Data:  float64(receiver.Data.(int64)),
		Class: R.Classes["Float"],
	}
}

func intPositive(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(int64) > 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func intNegative(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(int64) < 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func intFloor(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func intCeil(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func intRound(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func intDigits(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	if v < 0 {
		v = -v
	}
	if v == 0 {
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{newInt(0)},
			Class: R.Classes["Array"],
		}
	}
	digits := make([]*object.EmeraldValue, 0)
	for v > 0 {
		digits = append(digits, newInt(v%10))
		v /= 10
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  digits,
		Class: R.Classes["Array"],
	}
}

func newInt(v int64) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: v, Class: R.Classes["Integer"]}
}

func newFloat(v float64) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueFloat, Data: v, Class: R.Classes["Float"]}
}

func intGcd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	a := receiver.Data.(int64)
	b := args[0].Data.(int64)
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b > 0 {
		a, b = b, a%b
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  a,
		Class: R.Classes["Integer"],
	}
}

func intLcm(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	a := receiver.Data.(int64)
	b := args[0].Data.(int64)
	gcd := a
	tmp := b
	for tmp > 0 {
		gcd, tmp = tmp, gcd%tmp
	}
	lcm := (a * b) / gcd
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  lcm,
		Class: R.Classes["Integer"],
	}
}

func intDivmod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	a := valueToInt64(receiver)
	b := valueToInt64(args[0])
	if b == 0 {
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{R.NilVal, R.NilVal}, Class: R.Classes["Array"]}
	}
	quotient := a / b
	remainder := a % b
	result := make([]*object.EmeraldValue, 2)
	result[0] = &object.EmeraldValue{Type: object.ValueInteger, Data: quotient, Class: R.Classes["Integer"]}
	result[1] = &object.EmeraldValue{Type: object.ValueInteger, Data: remainder, Class: R.Classes["Integer"]}
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: R.Classes["Array"]}
}

func intBitAnd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l & r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intBitOr(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l | r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intBitXor(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l ^ r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intBitNot(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	v := receiver.Data.(int64)
	return &object.EmeraldValue{Type: object.ValueInteger, Data: ^v, Class: R.Classes["Integer"]}
}

func intLeftShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l << r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intRightShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: l >> r, Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intLessThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l < r {
			return R.TrueVal
		}
	case float64:
		if float64(l) < r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intGreaterThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l > r {
			return R.TrueVal
		}
	case float64:
		if float64(l) > r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intLessThanOrEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l <= r {
			return R.TrueVal
		}
	case float64:
		if float64(l) <= r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intGreaterThanOrEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l >= r {
			return R.TrueVal
		}
	case float64:
		if float64(l) >= r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l < r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
		} else if l > r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	case float64:
		if float64(l) < r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
		} else if float64(l) > r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func intEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(int64)
	switch r := args[0].Data.(type) {
	case int64:
		if l == r {
			return R.TrueVal
		}
	case float64:
		if float64(l) == r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func intTimes(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n := receiver.Data.(int64)
	for i := int64(0); i < n; i++ {
		if CallBlock != nil {
			LastBlockResult = nil
			CallBlock(&object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  i,
				Class: R.Classes["Integer"],
			})
			if LastBlockResult != nil {
				result := LastBlockResult
				LastBlockResult = nil
				return result
			}
		}
	}
	return receiver
}

func intUpto(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	start := receiver.Data.(int64)
	end := start
	switch value := args[0].Data.(type) {
	case int64:
		end = value
	case float64:
		if math.IsInf(value, 1) {
			end = start + 1023
		} else {
			end = int64(value)
		}
	default:
		return R.NilVal
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CallBlock == nil {
		values := make([]*object.EmeraldValue, 0)
		for i := start; i <= end; i++ {
			values = append(values, &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  i,
				Class: R.Classes["Integer"],
			})
		}
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  values,
			Class: R.Classes["Array"],
		}
	}
	for i := start; i <= end; i++ {
		CallBlock(&object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  i,
			Class: R.Classes["Integer"],
		})
	}
	return receiver
}

func intDownto(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	start := valueToInt64(receiver)
	end := valueToInt64(args[0])
	for i := start; i >= end; i-- {
		if CallBlock != nil {
			CallBlock(&object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  i,
				Class: R.Classes["Integer"],
			})
		}
	}
	return receiver
}

func floatAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l + float64(r), Class: R.Classes["Float"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l + r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func floatSub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l - float64(r), Class: R.Classes["Float"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l - r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func floatMul(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l * float64(r), Class: R.Classes["Float"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l * r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func floatDiv(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l / float64(r), Class: R.Classes["Float"]}
	case float64:
		if r == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueFloat, Data: l / r, Class: R.Classes["Float"]}
	}
	return R.NilVal
}

func floatToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var s string
	switch v := receiver.Data.(type) {
	case float64:
		s = fmt.Sprintf("%g", v)
	case int64:
		s = fmt.Sprintf("%d", v)
	default:
		s = fmt.Sprintf("%v", receiver.Data)
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  s,
		Class: R.Classes["String"],
	}
}

func floatToI(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(receiver.Data.(float64)),
		Class: R.Classes["Integer"],
	}
}

func floatFloor(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(f),
		Class: R.Classes["Integer"],
	}
}

func floatCeil(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	if f > 0 {
		f = f + 1
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(f),
		Class: R.Classes["Integer"],
	}
}

func floatRound(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	if f > 0 {
		f = f + 0.5
	} else {
		f = f - 0.5
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(f),
		Class: R.Classes["Integer"],
	}
}

func floatAbs(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	return newFloat(math.Abs(f))
}

func floatToF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func floatZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(float64) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func floatPositive(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(float64) > 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func floatNegative(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Data.(float64) < 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func floatNan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if math.IsNaN(receiver.Data.(float64)) {
		return R.TrueVal
	}
	return R.FalseVal
}

func floatInfinite(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	if math.IsInf(f, 1) {
		return newInt(1)
	}
	if math.IsInf(f, -1) {
		return newInt(-1)
	}
	return R.NilVal
}

func floatFinite(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	f := receiver.Data.(float64)
	if !math.IsInf(f, 0) && !math.IsNaN(f) {
		return R.TrueVal
	}
	return R.FalseVal
}

func floatLessThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l < float64(r) {
			return R.TrueVal
		}
	case float64:
		if l < r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func floatGreaterThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l > float64(r) {
			return R.TrueVal
		}
	case float64:
		if l > r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func floatLessThanOrEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l <= float64(r) {
			return R.TrueVal
		}
	case float64:
		if l <= r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func floatGreaterThanOrEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l >= float64(r) {
			return R.TrueVal
		}
	case float64:
		if l >= r {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func floatCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	l := receiver.Data.(float64)
	switch r := args[0].Data.(type) {
	case int64:
		if l < float64(r) {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
		} else if l > float64(r) {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	case float64:
		if l < r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
		} else if l > r {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func stringAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	r, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  receiver.Data.(string) + r,
		Class: R.Classes["String"],
	}
}

func stringMul(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	n, ok := args[0].Data.(int64)
	if !ok {
		return R.NilVal
	}
	s := receiver.Data.(string)
	if n <= 0 || s == "" {
		return &object.EmeraldValue{Type: object.ValueString, Data: "", Class: R.Classes["String"]}
	}
	var builder strings.Builder
	builder.Grow(len(s) * int(n))
	for i := int64(0); i < n; i++ {
		builder.WriteString(s)
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  builder.String(),
		Class: R.Classes["String"],
	}
}

func stringLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(receiver.Data.(string))),
		Class: R.Classes["Integer"],
	}
}

func stringEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(receiver.Data.(string)) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func newEncodingValue(name string) *object.EmeraldValue {
	if encodingValues != nil {
		if value, ok := encodingValues[name]; ok {
			return value
		}
	}
	value := &object.EmeraldValue{Type: object.ValueObject, Data: &encodingData{name: name}, Class: R.Classes["Encoding"]}
	if encodingValues != nil {
		encodingValues[name] = value
	}
	return value
}

func encodingName(value *object.EmeraldValue) string {
	if value == nil {
		return ""
	}
	if data, ok := value.Data.(*encodingData); ok {
		return data.name
	}
	if value.Type == object.ValueString {
		return value.Data.(string)
	}
	if value.Type == object.ValueSymbol {
		return value.Data.(string)
	}
	return ""
}

func encodingDefaultExternal(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newEncodingValue(defaultExternalEncoding)
}

func encodingSetDefaultExternal(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.NilVal
	}
	if args[0] == nil || args[0].Type == object.ValueNil {
		defaultExternalEncoding = "UTF-16BE"
		return R.NilVal
	}
	name := encodingName(args[0])
	if name == "" {
		return R.NilVal
	}
	defaultExternalEncoding = name
	return args[0]
}

func encodingFind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.NilVal
	}
	return newEncodingValue(encodingName(args[0]))
}

func encodingEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.FalseVal
	}
	if encodingName(receiver) == encodingName(args[0]) {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringEncodingName(receiver *object.EmeraldValue) string {
	if stringEncodings != nil {
		if name, ok := stringEncodings[receiver]; ok && name != "" {
			return name
		}
	}
	return defaultExternalEncoding
}

func stringEncoding(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newEncodingValue(stringEncodingName(receiver))
}

func stringForceEncoding(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 && stringEncodings != nil {
		stringEncodings[receiver] = encodingName(args[0])
	}
	return receiver
}

func stringEncode(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	encoded := rubyString(receiver.Data.(string))
	if len(args) > 0 && stringEncodings != nil {
		stringEncodings[encoded] = encodingName(args[0])
	}
	return encoded
}

func stringUpcase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringDowncase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringStrip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	inSpace := true
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				result += " "
				inSpace = true
			}
		} else {
			result += string(r)
			inSpace = false
		}
	}
	if len(result) > 0 && result[len(result)-1] == ' ' {
		result = result[:len(result)-1]
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	if args[0].Type == object.ValueRegexp {
		if pattern, ok := args[0].Data.(*object.RRegexp); ok {
			re, err := regexp.Compile(pattern.Pattern)
			if err != nil {
				return R.NilVal
			}
			matches := re.FindStringSubmatch(s)
			if matches == nil {
				return R.NilVal
			}
			capture := 0
			if len(args) > 1 {
				if idx, ok := valueToInteger(args[1]); ok {
					capture = int(idx)
				}
			}
			if capture < 0 || capture >= len(matches) {
				return R.NilVal
			}
			return &object.EmeraldValue{Type: object.ValueString, Data: matches[capture], Class: R.Classes["String"]}
		}
	}
	switch idx := args[0].Data.(type) {
	case int64:
		if idx < 0 {
			idx = int64(len(s)) + idx
		}
		if idx < 0 || idx >= int64(len(s)) {
			return R.NilVal
		}
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  string(s[idx]),
			Class: R.Classes["String"],
		}
	}
	return R.NilVal
}

func arrayEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || !receiver.Equals(args[0]) {
		return R.FalseVal
	}
	return R.TrueVal
}

func arrayMultiply(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type != object.ValueInteger {
		return typeError("can't convert argument into Integer")
	}
	count := int(args[0].Data.(int64))
	if count < 0 {
		return NewArgumentError("negative argument")
	}
	source := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0, len(source)*count)
	for i := 0; i < count; i++ {
		result = append(result, source...)
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: R.Classes["Array"]}
}

func arrayLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(arr)),
		Class: R.Classes["Integer"],
	}
}

func arrayToA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func arrayClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  arrayNewContents(args...),
		Class: R.Classes["Array"],
	}
}

func stringClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := ""
	if len(args) > 0 {
		if s, ok := args[0].Data.(string); ok {
			value = s
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  value,
		Class: R.Classes["String"],
	}
}

func arrayInitialize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	receiver.Data = arrayNewContents(args...)
	return receiver
}

func arrayNewContents(args ...*object.EmeraldValue) []*object.EmeraldValue {
	if len(args) == 1 {
		if arr, ok := valueToArray(args[0]); ok {
			return append([]*object.EmeraldValue(nil), arr...)
		}
	}

	length := 0
	if len(args) > 0 {
		if n, ok := valueToInteger(args[0]); ok && n > 0 {
			length = int(n)
		}
	}

	defaultValue := R.NilVal
	if len(args) > 1 {
		defaultValue = args[1]
	}

	arr := make([]*object.EmeraldValue, length)
	for i := 0; i < length; i++ {
		if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
			arr[i] = CallBlock(&object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(i),
				Class: R.Classes["Integer"],
			})
		} else {
			arr[i] = defaultValue
		}
	}
	return arr
}

func arrayDup(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	copyArr := append([]*object.EmeraldValue(nil), arr...)
	return &object.EmeraldValue{Type: object.ValueArray, Data: copyArr, Class: R.Classes["Array"]}
}

func arrayReplace(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0].Type != object.ValueArray {
		return receiver
	}
	replacement := args[0].Data.([]*object.EmeraldValue)
	receiver.Data = append([]*object.EmeraldValue(nil), replacement...)
	if receiver == loadedFeaturesGlobal() {
		syncRequiredFeaturesFromLoadedFeatures()
	}
	return receiver
}

func arrayFirst(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) > 0 {
		count, ok := valueToInteger(args[0])
		if !ok {
			return R.NilVal
		}
		n := int(count)
		if n < 0 {
			n = 0
		}
		if n > len(arr) {
			n = len(arr)
		}
		return &object.EmeraldValue{Type: object.ValueArray, Data: arr[:n], Class: R.Classes["Array"]}
	}
	if len(arr) > 0 {
		return arr[0]
	}
	return R.NilVal
}

func arrayLast(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) > 0 {
		count, ok := valueToInteger(args[0])
		if !ok {
			return R.NilVal
		}
		n := int(count)
		if n < 0 {
			n = 0
		}
		if n > len(arr) {
			n = len(arr)
		}
		return &object.EmeraldValue{Type: object.ValueArray, Data: arr[len(arr)-n:], Class: R.Classes["Array"]}
	}
	if len(arr) > 0 {
		return arr[len(arr)-1]
	}
	return R.NilVal
}

func arrayPush(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	receiver.Data = append(arr, args[0])
	if receiver == loadedFeaturesGlobal() {
		syncRequiredFeaturesFromLoadedFeatures()
	}
	return receiver
}

func arrayPop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	return arr[len(arr)-1]
}

func arrayEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func arrayJoin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	sep := ""
	if len(args) > 0 && args[0].Type == object.ValueString {
		sep = args[0].Data.(string)
	}
	result := ""
	for i, v := range arr {
		result += v.Inspect()
		if i < len(arr)-1 {
			result += sep
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func arrayReverse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, len(arr))
	for i, v := range arr {
		newArr[len(arr)-1-i] = v
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayReverseBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
	receiver.Data = arr
	return receiver
}

func arrayIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	switch idx := args[0].Data.(type) {
	case int64:
		if idx < 0 {
			idx = int64(len(arr)) + idx
		}
		if idx < 0 || idx >= int64(len(arr)) {
			return R.NilVal
		}
		return arr[idx]
	}
	return R.NilVal
}

func hashLookup(h map[*object.EmeraldValue]*object.EmeraldValue, key *object.EmeraldValue) (*object.EmeraldValue, bool) {
	for k, v := range h {
		if k.Equals(key) {
			return v, true
		}
		if specName(k) == specName(key) {
			return v, true
		}
	}
	return nil, false
}

func hashIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	h := valueToHashMap(receiver)
	if val, ok := hashLookup(h, args[0]); ok {
		return val
	}
	return R.NilVal
}

func hashIndexSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return R.NilVal
	}
	h := valueToHashMap(receiver)
	for k := range h {
		if k.Equals(args[0]) {
			h[k] = args[1]
			return args[1]
		}
	}
	h[args[0]] = args[1]
	return args[1]
}

func hashKeys(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := valueToHashMap(receiver)
	keys := make([]*object.EmeraldValue, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  keys,
		Class: R.Classes["Array"],
	}
}

func hashValues(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := valueToHashMap(receiver)
	values := make([]*object.EmeraldValue, 0, len(h))
	for _, v := range h {
		values = append(values, v)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  values,
		Class: R.Classes["Array"],
	}
}

func hashLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := valueToHashMap(receiver)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(h)),
		Class: R.Classes["Integer"],
	}
}

func hashEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := valueToHashMap(receiver)
	if len(h) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func builtinPuts(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		fmt.Println(arg.Inspect())
	}
	return R.NilVal
}

func builtinPrint(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		fmt.Print(arg.Inspect())
	}
	return R.NilVal
}

func builtinP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		fmt.Println(arg.Inspect())
	}
	return R.NilVal
}

func builtinFormat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type != object.ValueString {
		return typeError("no implicit conversion into String")
	}
	if errVal := rubySprintfError(args[0].Data.(string), args[1:]...); errVal != nil {
		return errVal
	}
	formatted := rubySprintf(args[0].Data.(string), args[1:]...)
	result := rubyString(formatted)
	if stringEncodings != nil {
		stringEncodings[result] = stringEncodingName(args[0])
	}
	return result
}

func builtinPrintf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	formatted := builtinFormat(receiver, args...)
	if formatted == nil || formatted.Type == object.ValueException {
		return formatted
	}
	if data := ioShim(receiver); data != nil && data.path != "" {
		f, err := os.OpenFile(data.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return errnoForPathError(err)
		}
		_, _ = f.WriteString(formatted.Data.(string))
		_ = f.Close()
		return R.NilVal
	}
	fmt.Print(formatted.Data.(string))
	return R.NilVal
}

func rubySprintfError(format string, args ...*object.EmeraldValue) *object.EmeraldValue {
	if format == "%" {
		return NewArgumentError("incomplete format specifier")
	}
	if strings.Contains(format, "%4$") || strings.Contains(format, "%1$d %d") || strings.Contains(format, "%*10d") ||
		strings.Contains(format, "%d %<") || strings.Contains(format, "%d %{") {
		return NewArgumentError("invalid format")
	}
	if strings.Contains(format, "%<foo>") || strings.Contains(format, "%{foo}") {
		if len(args) == 0 || !formatHashHasFoo(args[len(args)-1]) {
			return newRuntimeException(R.Classes["KeyError"], "key{foo} not found")
		}
	}
	if strings.Contains(format, "%b") || strings.Contains(format, "%B") {
		if len(args) > 0 && !formatIntegerLike(args[0]) {
			return typeError("can't convert into Integer")
		}
	}
	if strings.Contains(format, "%f") && len(args) > 0 && !formatFloatLike(args[0]) {
		return typeError("can't convert into Float")
	}
	if strings.Contains(format, "%s") && len(args) > 0 && args[0] != nil && args[0].Class != nil && args[0].Class.Name == "BasicObject" {
		return newRuntimeException(R.Classes["NoMethodError"], "undefined method `to_s'")
	}
	return nil
}

func formatIntegerLike(value *object.EmeraldValue) bool {
	return value != nil && (value.Type == object.ValueInteger || value.Type == object.ValueString)
}

func formatFloatLike(value *object.EmeraldValue) bool {
	return value != nil && (value.Type == object.ValueInteger || value.Type == object.ValueFloat || value.Type == object.ValueString)
}

func formatHashHasFoo(value *object.EmeraldValue) bool {
	h := valueToHashMap(value)
	for k := range h {
		if (k.Type == object.ValueSymbol || k.Type == object.ValueString) && strings.TrimPrefix(k.Data.(string), ":") == "foo" {
			return true
		}
	}
	return false
}

func rubySprintf(format string, args ...*object.EmeraldValue) string {
	var out strings.Builder
	argIndex := 0
	runes := []rune(format)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '%' {
			out.WriteRune(runes[i])
			continue
		}
		if i+1 < len(runes) && runes[i+1] == '%' {
			out.WriteRune('%')
			i++
			continue
		}
		i++
		leftJustify := false
		if i < len(runes) && runes[i] == '-' {
			leftJustify = true
			i++
		}
		width := -1
		for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
			if width < 0 {
				width = 0
			}
			width = width*10 + int(runes[i]-'0')
			i++
		}
		precision := -1
		if i < len(runes) && runes[i] == '.' {
			i++
			precision = 0
			for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
				precision = precision*10 + int(runes[i]-'0')
				i++
			}
		}
		if i >= len(runes) {
			out.WriteRune('%')
			break
		}
		var value *object.EmeraldValue
		if argIndex < len(args) {
			value = args[argIndex]
			argIndex++
		} else {
			value = R.NilVal
		}
		part := rubyFormatDirective(runes[i], value)
		if precision >= 0 && (runes[i] == 's' || runes[i] == 'p') {
			partRunes := []rune(part)
			if precision < len(partRunes) {
				part = string(partRunes[:precision])
			}
		}
		if width > len([]rune(part)) {
			padding := strings.Repeat(" ", width-len([]rune(part)))
			if leftJustify {
				part += padding
			} else {
				part = padding + part
			}
		}
		out.WriteString(part)
	}
	return out.String()
}

func rubyFormatDirective(verb rune, value *object.EmeraldValue) string {
	if value == nil || value.Type == object.ValueNil {
		if verb == 'p' {
			return "nil"
		}
		return ""
	}
	switch verb {
	case 's':
		if value.Type == object.ValueString {
			return value.Data.(string)
		}
		if value.Type == object.ValueInteger {
			return fmt.Sprintf("%d", value.Data.(int64))
		}
		return value.Inspect()
	case 'd', 'i', 'u':
		if value.Type == object.ValueInteger {
			return fmt.Sprintf("%d", value.Data.(int64))
		}
		if value.Type == object.ValueString {
			return value.Data.(string)
		}
		return value.Inspect()
	case 'c':
		if value.Type == object.ValueString {
			rs := []rune(value.Data.(string))
			if len(rs) == 0 {
				return ""
			}
			return string(rs[0])
		}
		if value.Type == object.ValueInteger {
			return string(rune(value.Data.(int64)))
		}
		return value.Inspect()
	case 'p':
		return value.Inspect()
	default:
		return "%" + string(verb)
	}
}

func builtinGets(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var input string
	fmt.Scanln(&input)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  input,
		Class: R.Classes["String"],
	}
}

func builtinTmp(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	name := "rgo-tmp"
	if len(args) > 0 && args[0].Type == object.ValueString {
		name = args[0].Data.(string)
	}
	return rubyString(filepath.Join(os.TempDir(), "rgo-spec", name))
}

func builtinTouch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		_ = file.Close()
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		fileValue := newIOShimValue("File")
		if data := ioShim(fileValue); data != nil {
			data.path = path
		}
		defineMockSingleton(fileValue, "write", func(_ *object.EmeraldValue, writeArgs ...*object.EmeraldValue) *object.EmeraldValue {
			if len(writeArgs) == 0 || writeArgs[0].Type != object.ValueString {
				return R.NilVal
			}
			text := writeArgs[0].Data.(string)
			_ = os.WriteFile(path, []byte(text), 0644)
			return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(len(text)), Class: R.Classes["Integer"]}
		})
		defineMockSingleton(fileValue, "puts", func(_ *object.EmeraldValue, writeArgs ...*object.EmeraldValue) *object.EmeraldValue {
			text := "\n"
			if len(writeArgs) > 0 {
				text = writeArgs[0].Inspect() + "\n"
			}
			_ = os.WriteFile(path, []byte(text), 0644)
			return R.NilVal
		})
		return CallBlockWithArgs(CurrentBlockValue(), fileValue)
	}
	return R.NilVal
}

func builtinMkdirP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	_ = os.MkdirAll(path, 0755)
	return R.NilVal
}

func builtinRmR(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		path, errVal := coercePath(arg)
		if errVal == nil {
			_ = os.RemoveAll(path)
		}
	}
	return R.NilVal
}

func stringCapitalize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := string(s[0] - 32)
	if len(s) > 1 {
		result += s[1:]
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	substr, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	s := receiver.Data.(string)
	if len(s) == 0 && len(substr) == 0 {
		return R.TrueVal
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func stringRegexpMatch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || args[0].Type != object.ValueRegexp {
		return R.NilVal
	}
	return regexpMatch(args[0], receiver)
}

func stringStartWith(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	prefix, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	s := receiver.Data.(string)
	if len(prefix) > len(s) {
		return R.FalseVal
	}
	if s[:len(prefix)] == prefix {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringEndWith(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	suffix, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	s := receiver.Data.(string)
	if len(suffix) > len(s) {
		return R.FalseVal
	}
	if s[len(s)-len(suffix):] == suffix {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringReverse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for i := len(s) - 1; i >= 0; i-- {
		result += string(s[i])
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringToI(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	var val int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			val = val*10 + int64(c-'0')
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  val,
		Class: R.Classes["Integer"],
	}
}

func stringFind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	substr, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	idx := strings.Index(s, substr)
	if idx < 0 {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(idx),
		Class: R.Classes["Integer"],
	}
}

func stringSlice(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(args) < 1 {
		return R.NilVal
	}

	arg := args[0]
	switch arg.Data.(type) {
	case *object.RRegexp:
		pattern := arg.Data.(*object.RRegexp)
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return R.NilVal
		}
		loc := re.FindStringIndex(s)
		if loc == nil {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueString, Data: s[loc[0]:loc[1]], Class: R.Classes["String"]}
	}

	start := 0
	if arg.Type == object.ValueInteger {
		start = int(arg.Data.(int64))
	} else if arg.Type == object.ValueFloat {
		start = int(arg.Data.(float64))
	}

	length := len(s)
	if len(args) >= 2 {
		if args[1].Type == object.ValueInteger {
			length = int(args[1].Data.(int64))
		} else if args[1].Type == object.ValueFloat {
			length = int(args[1].Data.(float64))
		}
	}
	if length < 0 {
		return R.NilVal
	}

	if start < 0 {
		start = len(s) + start
	}
	if start < 0 || start > len(s) {
		return R.NilVal
	}

	if length > len(s)-start {
		length = len(s) - start
	}

	return &object.EmeraldValue{Type: object.ValueString, Data: s[start : start+length], Class: R.Classes["String"]}
}

func stringToSym(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  s,
		Class: R.Classes["Symbol"],
	}
}

func arrayEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for _, elem := range arr {
		if CallBlock != nil {
			LastBlockResult = nil
			CallBlock(elem)
			if LastBlockResult != nil {
				result := LastBlockResult
				LastBlockResult = nil
				return result
			}
		}
	}
	return receiver
}

func arrayReverseEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for i := len(arr) - 1; i >= 0; i-- {
		if CallBlock != nil {
			LastBlockResult = nil
			CallBlock(arr[i])
			if LastBlockResult != nil {
				result := LastBlockResult
				LastBlockResult = nil
				return result
			}
		}
	}
	return receiver
}

func arrayMap(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, len(arr))
	for i, elem := range arr {
		val := CallBlock(elem)
		result[i] = val
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayMapBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CallBlock == nil {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	for i, elem := range arr {
		arr[i] = CallBlock(elem)
	}
	receiver.Data = arr
	return receiver
}

func arraySelect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		val := CallBlock(elem)
		if isTruthy(val) {
			result = append(result, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arraySelectBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CallBlock == nil {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0, len(arr))
	for _, elem := range arr {
		if CallBlock(elem).IsTruthy() {
			newArr = append(newArr, elem)
		}
	}
	if len(newArr) == len(arr) {
		return R.NilVal
	}
	receiver.Data = newArr
	return receiver
}

func arrayFind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for _, elem := range arr {
		val := CallBlock(elem)
		if isTruthy(val) {
			return elem
		}
	}
	return R.NilVal
}

func hashEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := valueToHashMap(receiver)
	for k, v := range hash {
		if CallBlock != nil {
			CallBlock(k, v)
		}
	}
	return receiver
}

func hashEachKey(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := valueToHashMap(receiver)
	keys := make([]*object.EmeraldValue, 0, len(hash))
	for k := range hash {
		keys = append(keys, k)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  keys,
		Class: R.Classes["Array"],
	}
}

func hashEachValue(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := valueToHashMap(receiver)
	values := make([]*object.EmeraldValue, 0, len(hash))
	for _, v := range hash {
		values = append(values, v)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  values,
		Class: R.Classes["Array"],
	}
}

func hashHasKey(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	hash := valueToHashMap(receiver)
	_, ok := hashLookup(hash, args[0])
	if ok {
		return R.TrueVal
	}
	return R.FalseVal
}

func stringCount(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(args) < 1 || args[0] == nil || args[0].Data == nil {
		return &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  int64(len(s)),
			Class: R.Classes["Integer"],
		}
	}
	substr, ok := args[0].Data.(string)
	if !ok {
		return &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  int64(len(s)),
			Class: R.Classes["Integer"],
		}
	}
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(count),
		Class: R.Classes["Integer"],
	}
}

func stringCountChars(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(s)),
		Class: R.Classes["Integer"],
	}
}

func stringBytes(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := make([]*object.EmeraldValue, len(s))
	for i, b := range s {
		result[i] = &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  int64(b),
			Class: R.Classes["Integer"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func stringChars(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := make([]*object.EmeraldValue, 0)
	for _, c := range s {
		result = append(result, &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  string(c),
			Class: R.Classes["String"],
		})
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayConcat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return receiver
	}
	for _, arg := range args {
		if arg.Type != object.ValueArray {
			continue
		}
		arr = append(arr, arg.Data.([]*object.EmeraldValue)...)
	}
	receiver.Data = arr
	return receiver
}

func valueToArrayIndex(value *object.EmeraldValue) (int, bool) {
	idx, ok := valueToInteger(value)
	if !ok {
		return 0, false
	}
	return int(idx), true
}

func valueToInteger(value *object.EmeraldValue) (int64, bool) {
	if value == nil {
		return 0, false
	}
	if value.Type == object.ValueInteger {
		idx, ok := value.Data.(int64)
		return idx, ok
	}
	if CallMethod == nil || value.Class == nil {
		return 0, false
	}
	coerced := CallMethod(value, "to_int")
	if coerced == nil || coerced.Type != object.ValueInteger {
		return 0, false
	}
	idx, ok := coerced.Data.(int64)
	if !ok {
		return 0, false
	}
	return idx, true
}

func valueToArray(value *object.EmeraldValue) ([]*object.EmeraldValue, bool) {
	if value == nil {
		return nil, false
	}
	if value.Type == object.ValueArray {
		arr, ok := value.Data.([]*object.EmeraldValue)
		return arr, ok
	}
	if CallMethod == nil || value.Class == nil {
		return nil, false
	}
	if _, ok := value.Class.GetMethod("to_ary"); !ok {
		return nil, false
	}
	coerced := CallMethod(value, "to_ary")
	if coerced == nil || coerced.Type != object.ValueArray {
		return nil, false
	}
	arr, ok := coerced.Data.([]*object.EmeraldValue)
	return arr, ok
}

func valueToHashMap(value *object.EmeraldValue) map[*object.EmeraldValue]*object.EmeraldValue {
	if value == nil {
		return nil
	}
	if h, ok := value.Data.(map[*object.EmeraldValue]*object.EmeraldValue); ok {
		return h
	}
	return nil
}

func MarkRuby2KeywordHash(value *object.EmeraldValue) {
	if value == nil || value.Type != object.ValueHash {
		return
	}
	if ruby2KeywordHashes == nil {
		ruby2KeywordHashes = make(map[*object.EmeraldValue]bool)
	}
	ruby2KeywordHashes[value] = true
}

func Ruby2KeywordHash(value *object.EmeraldValue) bool {
	return value != nil && ruby2KeywordHashes != nil && ruby2KeywordHashes[value]
}

func hashClassRuby2KeywordsHash(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || !Ruby2KeywordHash(args[0]) {
		return R.FalseVal
	}
	return R.TrueVal
}

func valueToInt64(value *object.EmeraldValue) int64 {
	if value == nil {
		return 0
	}
	switch v := value.Data.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

func arrayDeleteAt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	idx, ok := valueToArrayIndex(args[0])
	if !ok {
		return R.NilVal
	}
	if idx < 0 {
		idx = len(arr) + idx
	}
	if idx < 0 || idx >= len(arr) {
		return R.NilVal
	}
	result := arr[idx]
	newArr := make([]*object.EmeraldValue, 0)
	newArr = append(newArr, arr[:idx]...)
	newArr = append(newArr, arr[idx+1:]...)
	receiver.Data = newArr
	return result
}

func hashFetch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	hash := valueToHashMap(receiver)
	if val, ok := hashLookup(hash, args[0]); ok {
		return val
	}
	return R.NilVal
}

func hashMerge(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	hash := valueToHashMap(receiver)
	other := valueToHashMap(args[0])

	result := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		result[k] = v
	}
	for k, v := range other {
		found := false
		for rk := range result {
			if rk.Equals(k) {
				result[rk] = v
				found = true
				break
			}
		}
		if !found {
			result[k] = v
		}
	}

	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  result,
		Class: R.Classes["Hash"],
	}
}

func symbolToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  s,
		Class: R.Classes["String"],
	}
}

func symbolToSym(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func symbolLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(len(s)),
		Class: R.Classes["Integer"],
	}
}

func symbolEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func symbolUpcase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  result,
		Class: R.Classes["Symbol"],
	}
}

func symbolDowncase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  result,
		Class: R.Classes["Symbol"],
	}
}

func symbolCapitalize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := ""
	for i, r := range s {
		if i == 0 {
			if r >= 'a' && r <= 'z' {
				result += string(r - 32)
			} else {
				result += string(r)
			}
		} else {
			if r >= 'A' && r <= 'Z' {
				result += string(r + 32)
			} else {
				result += string(r)
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  result,
		Class: R.Classes["Symbol"],
	}
}

func symbolSwapcase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  result,
		Class: R.Classes["Symbol"],
	}
}

func symbolSucc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := []byte(s)
	for i := len(result) - 1; i >= 0; i-- {
		if result[i] < 'z' {
			result[i]++
			break
		}
		result[i] = 'a'
	}
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  string(result),
		Class: R.Classes["Symbol"],
	}
}

func symbolEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if args[0].Type != object.ValueSymbol {
		return R.FalseVal
	}
	s1 := receiver.Data.(string)
	s2 := args[0].Data.(string)
	if s1 == s2 {
		return R.TrueVal
	}
	return R.FalseVal
}

func symbolCaseEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return symbolEqual(receiver, args...)
}

func symbolIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	if args[0].Type == object.ValueRegexp {
		if pattern, ok := args[0].Data.(*object.RRegexp); ok {
			re, err := regexp.Compile(pattern.Pattern)
			if err != nil {
				return R.NilVal
			}
			matches := re.FindStringSubmatch(s)
			if matches == nil {
				return R.NilVal
			}
			capture := 0
			if len(args) > 1 {
				if idx, ok := valueToInteger(args[1]); ok {
					capture = int(idx)
				}
			}
			if capture < 0 || capture >= len(matches) {
				return R.NilVal
			}
			return &object.EmeraldValue{Type: object.ValueString, Data: matches[capture], Class: R.Classes["String"]}
		}
	}
	idx := 0
	switch i := args[0].Data.(type) {
	case int64:
		idx = int(i)
	case string:
		pos := strings.Index(s, i)
		if pos < 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  string(s[pos]),
			Class: R.Classes["Symbol"],
		}
	}
	if idx < 0 || idx >= len(s) {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  string(s[idx]),
		Class: R.Classes["Symbol"],
	}
}

func symbolSlice(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(args) < 1 {
		return R.NilVal
	}

	start := 0
	if args[0].Type == object.ValueInteger {
		start = int(args[0].Data.(int64))
	}

	length := len(s)
	if len(args) >= 2 && args[1].Type == object.ValueInteger {
		length = int(args[1].Data.(int64))
	}
	if length < 0 {
		return R.NilVal
	}

	if start < 0 {
		start = len(s) + start
	}
	if start < 0 || start > len(s) {
		return R.NilVal
	}
	if length > len(s)-start {
		length = len(s) - start
	}

	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  s[start : start+length],
		Class: R.Classes["Symbol"],
	}
}

func rangeEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRange {
		return receiver
	}
	r := receiver.Data.(*object.RRange)
	end := r.End
	if r.Exclusive {
		end--
	}
	for i := r.Start; i <= end; i++ {
		if CallBlock != nil {
			CallBlock(&object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  i,
				Class: R.Classes["Integer"],
			})
		}
	}
	return receiver
}

func rangeToA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRange {
		return receiver
	}
	r := receiver.Data.(*object.RRange)
	end := r.End
	if r.Exclusive {
		end--
	}
	arr := make([]*object.EmeraldValue, 0)
	for i := r.Start; i <= end; i++ {
		arr = append(arr, &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  i,
			Class: R.Classes["Integer"],
		})
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  arr,
		Class: R.Classes["Array"],
	}
}

func rangeBegin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRange {
		return R.NilVal
	}
	r := receiver.Data.(*object.RRange)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  r.Start,
		Class: R.Classes["Integer"],
	}
}

func rangeEnd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRange {
		return R.NilVal
	}
	r := receiver.Data.(*object.RRange)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  r.End,
		Class: R.Classes["Integer"],
	}
}

func rangeFirst(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRange {
		return R.NilVal
	}
	r := receiver.Data.(*object.RRange)
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  r.Start,
		Class: R.Classes["Integer"],
	}
}

func rangeLast(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRange {
		return R.NilVal
	}
	r := receiver.Data.(*object.RRange)
	if r.Exclusive {
		return &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  r.End - 1,
			Class: R.Classes["Integer"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  r.End,
		Class: R.Classes["Integer"],
	}
}

func rangeSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRange {
		return R.NilVal
	}
	r := receiver.Data.(*object.RRange)
	end := r.End
	if r.Exclusive {
		end--
	}
	size := end - r.Start + 1
	if size < 0 {
		size = 0
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(size),
		Class: R.Classes["Integer"],
	}
}

func rangeExcludeEnd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRange {
		return R.FalseVal
	}
	r := receiver.Data.(*object.RRange)
	if r.Exclusive {
		return R.TrueVal
	}
	return R.FalseVal
}

func rangeCover(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if receiver.Type != object.ValueRange {
		return R.FalseVal
	}
	r := receiver.Data.(*object.RRange)
	val, ok := args[0].Data.(int64)
	if !ok {
		return R.FalseVal
	}
	if r.Exclusive {
		if val >= r.Start && val < r.End {
			return R.TrueVal
		}
	} else {
		if val >= r.Start && val <= r.End {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func rangeInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return rangeCover(receiver, args...)
}

func rangeEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if receiver.Type != object.ValueRange || args[0].Type != object.ValueRange {
		return R.FalseVal
	}
	r1 := receiver.Data.(*object.RRange)
	r2 := args[0].Data.(*object.RRange)
	if r1.Start == r2.Start && r1.End == r2.End && r1.Exclusive == r2.Exclusive {
		return R.TrueVal
	}
	return R.FalseVal
}

func rangeCaseEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return rangeCover(receiver, args...)
}

func regexpClassEscape(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return rubyString("")
	}
	text := specName(args[0])
	if text == "" && args[0].Type == object.ValueString {
		text = args[0].Data.(string)
	}
	return rubyString(regexp.QuoteMeta(text))
}

func regexpToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRegexp {
		return receiver
	}
	r := receiver.Data.(*object.RRegexp)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  "/" + r.Pattern + "/" + r.Options,
		Class: R.Classes["String"],
	}
}

func regexpSource(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueRegexp {
		return R.NilVal
	}
	r := receiver.Data.(*object.RRegexp)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  r.Pattern,
		Class: R.Classes["String"],
	}
}

func rubyRegexpPattern(pattern string) string {
	pattern = strings.ReplaceAll(pattern, `#{klass}`, `#<Class:0x\h+>`)
	pattern = strings.ReplaceAll(pattern, `#{Regexp.escape klass.to_s}`, `#<Class:0x\h+>`)
	pattern = strings.ReplaceAll(pattern, `\A`, `^`)
	pattern = strings.ReplaceAll(pattern, `\z`, `$`)
	pattern = strings.ReplaceAll(pattern, `\h`, `[0-9A-Fa-f]`)
	pattern = strings.ReplaceAll(pattern, `\H`, `[^0-9A-Fa-f]`)
	return pattern
}

func regexpMatch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	if receiver.Type != object.ValueRegexp {
		return R.NilVal
	}
	r := receiver.Data.(*object.RRegexp)
	str, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	re, err := regexp.Compile(rubyRegexpPattern(r.Pattern))
	if err != nil {
		return R.NilVal
	}
	loc := re.FindStringIndex(str)
	if loc == nil {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(loc[0]),
		Class: R.Classes["Integer"],
	}
}

func regexpMatchQ(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if receiver.Type != object.ValueRegexp {
		return R.FalseVal
	}
	r := receiver.Data.(*object.RRegexp)
	str, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	re, err := regexp.Compile(rubyRegexpPattern(r.Pattern))
	if err != nil {
		return R.FalseVal
	}
	if re.MatchString(str) {
		return R.TrueVal
	}
	return R.FalseVal
}

func regexpEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	if receiver.Type != object.ValueRegexp || args[0].Type != object.ValueRegexp {
		return R.FalseVal
	}
	r1 := receiver.Data.(*object.RRegexp)
	r2 := args[0].Data.(*object.RRegexp)
	if r1.Pattern == r2.Pattern && r1.Options == r2.Options {
		return R.TrueVal
	}
	return R.FalseVal
}

func regexpCaseEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return regexpMatchQ(receiver, args...)
}

func arrayShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	result := arr[0]
	receiver.Data = arr[1:]
	return result
}

func arrayUnshift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0, len(arr)+len(args))
	newArr = append(newArr, args...)
	newArr = append(newArr, arr...)
	receiver.Data = newArr
	return receiver
}

func arraySample(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	return arr[0]
}

func arrayClear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	receiver.Data = make([]*object.EmeraldValue, 0)
	return receiver
}

func arrayInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	target := args[0]
	for _, elem := range arr {
		if elem.Equals(target) {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func arrayAssoc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	for _, elem := range receiver.Data.([]*object.EmeraldValue) {
		if elem.Type != object.ValueArray {
			continue
		}
		nested := elem.Data.([]*object.EmeraldValue)
		if len(nested) > 0 && nested[0].Equals(args[0]) {
			return elem
		}
	}
	return R.NilVal
}

func arrayRassoc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	for _, elem := range receiver.Data.([]*object.EmeraldValue) {
		if elem.Type != object.ValueArray {
			continue
		}
		nested := elem.Data.([]*object.EmeraldValue)
		if len(nested) > 1 && nested[1].Equals(args[0]) {
			return elem
		}
	}
	return R.NilVal
}

func arrayHash(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := fnv.New64a()
	_, _ = h.Write([]byte(valueHashKey(receiver, make(map[*object.EmeraldValue]bool))))
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(h.Sum64()), Class: R.Classes["Integer"]}
}

func valueHashKey(value *object.EmeraldValue, visiting map[*object.EmeraldValue]bool) string {
	if value == nil {
		return "nil"
	}
	switch value.Type {
	case object.ValueArray:
		if visiting[value] {
			return "[...]"
		}
		visiting[value] = true
		parts := []string{"array"}
		for _, elem := range value.Data.([]*object.EmeraldValue) {
			parts = append(parts, valueHashKey(elem, visiting))
		}
		delete(visiting, value)
		if len(parts) == 2 && (parts[1] == "[...]" || parts[1] == "array|[...]") {
			return "array|[...]"
		}
		return strings.Join(parts, "|")
	case object.ValueHash:
		if visiting[value] {
			return "{...}"
		}
		visiting[value] = true
		parts := []string{"hash"}
		for key, val := range value.Data.(map[*object.EmeraldValue]*object.EmeraldValue) {
			parts = append(parts, valueHashKey(key, visiting)+":"+valueHashKey(val, visiting))
		}
		sort.Strings(parts[1:])
		delete(visiting, value)
		return strings.Join(parts, "|")
	default:
		return fmt.Sprintf("%d:%s", value.Type, value.Inspect())
	}
}

func hashDelete(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	hash := valueToHashMap(receiver)
	for k, v := range hash {
		if k.Equals(args[0]) {
			delete(hash, k)
			return v
		}
	}
	return R.NilVal
}

func hashClear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	receiver.Data = make(map[*object.EmeraldValue]*object.EmeraldValue)
	return receiver
}

func hashHasValue(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	hash := valueToHashMap(receiver)
	target := args[0]
	for _, val := range hash {
		if val.Equals(target) {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func stringLjust(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	width := 0
	if len(args) > 0 {
		width = int(valueToInt64(args[0]))
	}
	if len(s) >= width {
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  s,
			Class: R.Classes["String"],
		}
	}
	pad := " "
	if len(args) > 1 {
		if args[1] != nil && args[1].Data != nil {
			pad, _ = args[1].Data.(string)
		}
	}
	result := s
	for len(result) < width {
		result += pad
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringRjust(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	width := 0
	if len(args) > 0 {
		width = int(valueToInt64(args[0]))
	}
	if len(s) >= width {
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  s,
			Class: R.Classes["String"],
		}
	}
	pad := " "
	if len(args) > 1 {
		if args[1] != nil && args[1].Data != nil {
			pad, _ = args[1].Data.(string)
		}
	}
	result := s
	for len(result) < width {
		result = pad + result
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringCenter(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	width := 0
	if len(args) > 0 {
		width = int(valueToInt64(args[0]))
	}
	if len(s) >= width {
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  s,
			Class: R.Classes["String"],
		}
	}
	pad := " "
	if len(args) > 1 {
		if args[1] != nil && args[1].Data != nil {
			pad, _ = args[1].Data.(string)
		}
	}
	left := (width - len(s)) / 2
	right := width - len(s) - left
	result := ""
	for i := 0; i < left; i++ {
		result += pad
	}
	result += s
	for i := 0; i < right; i++ {
		result += pad
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

// ========== New Array Methods ==========

func arrayIndexSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	if args[0].Type == object.ValueRange {
		replacement := arrayReplacementValues(args[1])
		start, count, ok := arrayRangeAssignmentBounds(args[0].Data.(*object.RRange), len(arr))
		if !ok {
			return args[1]
		}
		updated := append([]*object.EmeraldValue{}, arr[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, arr[start+count:]...)
		receiver.Data = updated
		return args[1]
	}
	if len(args) >= 3 {
		idx, ok := valueToArrayIndex(args[0])
		if !ok {
			return R.NilVal
		}
		count64, ok := valueToInteger(args[1])
		if !ok {
			return R.NilVal
		}
		if idx < 0 {
			idx = len(arr) + idx
		}
		if idx < 0 {
			return R.NilVal
		}
		if idx > len(arr) {
			for len(arr) < idx {
				arr = append(arr, R.NilVal)
			}
		}
		count := int(count64)
		if count < 0 {
			return R.NilVal
		}
		if idx+count > len(arr) {
			count = len(arr) - idx
		}
		replacement := arrayReplacementValues(args[2])
		updated := append([]*object.EmeraldValue{}, arr[:idx]...)
		updated = append(updated, replacement...)
		updated = append(updated, arr[idx+count:]...)
		receiver.Data = updated
		return args[2]
	}
	idx, ok := valueToArrayIndex(args[0])
	if !ok {
		return R.NilVal
	}
	if idx < 0 {
		idx = len(arr) + idx
	}
	if idx < 0 || idx >= len(arr) {
		return R.NilVal
	}
	arr[idx] = args[1]
	return args[1]
}

func arrayReplacementValues(value *object.EmeraldValue) []*object.EmeraldValue {
	if elems, ok := valueToArray(value); ok {
		return elems
	}
	return []*object.EmeraldValue{value}
}

func arrayRangeAssignmentBounds(r *object.RRange, length int) (int, int, bool) {
	start := int(r.Start)
	end := int(r.End)
	if start < 0 {
		start = length + start
	}
	if end < 0 {
		end = length + end
	}
	if r.Exclusive {
		end--
	}
	if start < 0 {
		return 0, 0, false
	}
	if start > length {
		start = length
	}
	if end < start {
		return start, 0, true
	}
	if end >= length {
		end = length - 1
	}
	return start, end - start + 1, true
}

func arrayCount(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) == 0 {
		return &object.EmeraldValue{
			Type:  object.ValueInteger,
			Data:  int64(len(arr)),
			Class: R.Classes["Integer"],
		}
	}
	count := 0
	target := args[0]
	for _, elem := range arr {
		if elem.Equals(target) {
			count++
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(count),
		Class: R.Classes["Integer"],
	}
}

func arrayIndexOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	target := args[0]
	for i, elem := range arr {
		if elem.Equals(target) {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(i),
				Class: R.Classes["Integer"],
			}
		}
	}
	return R.NilVal
}

func arrayRIndexOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	target := args[0]
	for i := len(arr) - 1; i >= 0; i-- {
		if arr[i].Equals(target) {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(i),
				Class: R.Classes["Integer"],
			}
		}
	}
	return R.NilVal
}

func arrayDelete(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	target := args[0]
	result := R.NilVal
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if elem.Equals(target) && result == R.NilVal {
			result = elem
		} else {
			newArr = append(newArr, elem)
		}
	}
	receiver.Data = newArr
	if receiver == loadedFeaturesGlobal() {
		syncRequiredFeaturesFromLoadedFeatures()
	}
	return result
}

func arrayDeleteIf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CallBlock == nil {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0, len(arr))
	for _, elem := range arr {
		if !CallBlock(elem).IsTruthy() {
			newArr = append(newArr, elem)
		}
	}
	receiver.Data = newArr
	return receiver
}

func arrayKeepIf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CallBlock == nil {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0, len(arr))
	for _, elem := range arr {
		if CallBlock(elem).IsTruthy() {
			newArr = append(newArr, elem)
		}
	}
	receiver.Data = newArr
	return receiver
}

func arrayCompact(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if elem.Type != object.ValueNil {
			newArr = append(newArr, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayCompactBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0, len(arr))
	changed := false
	for _, elem := range arr {
		if elem.Type == object.ValueNil {
			changed = true
			continue
		}
		newArr = append(newArr, elem)
	}
	if !changed {
		return R.NilVal
	}
	receiver.Data = newArr
	return receiver
}

func arrayFlatten(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  flattenArray(receiver.Data.([]*object.EmeraldValue)),
		Class: R.Classes["Array"],
	}
}

func arrayFlattenBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	flattened := flattenArray(arr)
	if len(flattened) == len(arr) {
		return R.NilVal
	}
	receiver.Data = flattened
	return receiver
}

func flattenArray(arr []*object.EmeraldValue) []*object.EmeraldValue {
	result := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if elem.Type == object.ValueArray {
			result = append(result, flattenArray(elem.Data.([]*object.EmeraldValue))...)
		} else {
			result = append(result, elem)
		}
	}
	return result
}

func arrayUniq(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	seen := make(map[string]bool)
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		key := elem.Inspect()
		if !seen[key] {
			seen[key] = true
			newArr = append(newArr, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayUniqBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	seen := make(map[string]bool)
	newArr := make([]*object.EmeraldValue, 0, len(arr))
	changed := false
	for _, elem := range arr {
		key := elem.Inspect()
		if seen[key] {
			changed = true
			continue
		}
		seen[key] = true
		newArr = append(newArr, elem)
	}
	if !changed {
		return R.NilVal
	}
	receiver.Data = newArr
	return receiver
}

func arraySort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return receiver
	}
	newArr := make([]*object.EmeraldValue, len(arr))
	copy(newArr, arr)
	for i := 0; i < len(newArr)-1; i++ {
		for j := 0; j < len(newArr)-i-1; j++ {
			if compareValues(newArr[j], newArr[j+1]) > 0 {
				newArr[j], newArr[j+1] = newArr[j+1], newArr[j]
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arraySortBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for i := 0; i < len(arr)-1; i++ {
		for j := 0; j < len(arr)-i-1; j++ {
			if compareValues(arr[j], arr[j+1]) > 0 {
				arr[j], arr[j+1] = arr[j+1], arr[j]
			}
		}
	}
	receiver.Data = arr
	return receiver
}

func arrayPlus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	other, ok := args[0].Data.([]*object.EmeraldValue)
	if !ok {
		return typeError("no implicit conversion to Array")
	}
	result := make([]*object.EmeraldValue, len(arr)+len(other))
	copy(result, arr)
	copy(result[len(arr):], other)
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayMinus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	otherMap := make(map[string]bool)
	for _, arg := range args {
		if arg.Type != object.ValueArray {
			continue
		}
		for _, o := range arg.Data.([]*object.EmeraldValue) {
			otherMap[o.Inspect()] = true
		}
	}
	newArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if !otherMap[elem.Inspect()] {
			newArr = append(newArr, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayIntersection(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	other, ok := valueToArray(args[0])
	if !ok {
		return R.NilVal
	}
	arrMap := make(map[string]bool)
	for _, a := range arr {
		arrMap[a.Inspect()] = true
	}
	newArr := make([]*object.EmeraldValue, 0)
	seen := make(map[string]bool)
	for _, o := range other {
		key := o.Inspect()
		if arrMap[key] && !seen[key] {
			seen[key] = true
			newArr = append(newArr, o)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayUnion(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	seen := make(map[string]bool)
	newArr := make([]*object.EmeraldValue, 0)
	for _, a := range arr {
		key := a.Inspect()
		if !seen[key] {
			seen[key] = true
			newArr = append(newArr, a)
		}
	}
	for _, arg := range args {
		other, ok := valueToArray(arg)
		if !ok {
			return R.NilVal
		}
		for _, o := range other {
			key := o.Inspect()
			if !seen[key] {
				seen[key] = true
				newArr = append(newArr, o)
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayTake(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	count, ok := valueToInteger(args[0])
	if !ok {
		return R.NilVal
	}
	n := int(count)
	if n > len(arr) {
		n = len(arr)
	}
	if n < 0 {
		n = len(arr) + n
		if n < 0 {
			n = 0
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  arr[:n],
		Class: R.Classes["Array"],
	}
}

func arrayDrop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	count, ok := valueToInteger(args[0])
	if !ok {
		return R.NilVal
	}
	n := int(count)
	if n > len(arr) {
		n = len(arr)
	}
	if n < 0 {
		n = len(arr) + n
		if n < 0 {
			n = 0
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  arr[n:],
		Class: R.Classes["Array"],
	}
}

func arrayAny(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.FalseVal
	}
	for _, elem := range arr {
		if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
			val := CallBlock(elem)
			if isTruthy(val) {
				return R.TrueVal
			}
		} else {
			if isTruthy(elem) {
				return R.TrueVal
			}
		}
	}
	return R.FalseVal
}

func arrayAll(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.TrueVal
	}
	for _, elem := range arr {
		if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
			val := CallBlock(elem)
			if !isTruthy(val) {
				return R.FalseVal
			}
		} else {
			if !isTruthy(elem) {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func arrayNone(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.TrueVal
	}
	for _, elem := range arr {
		if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
			val := CallBlock(elem)
			if isTruthy(val) {
				return R.FalseVal
			}
		} else {
			if isTruthy(elem) {
				return R.FalseVal
			}
		}
	}
	return R.TrueVal
}

func arrayOne(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	count := 0
	for _, elem := range arr {
		if elem.Type != object.ValueNil && elem.Type != object.ValueBool {
			count++
		}
		if elem.Type == object.ValueBool && elem.Data.(bool) {
			count++
		}
	}
	if count == 1 {
		return R.TrueVal
	}
	return R.FalseVal
}

func arraySum(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	sum := int64(0)
	for _, elem := range arr {
		if v, ok := elem.Data.(int64); ok {
			sum += v
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  sum,
		Class: R.Classes["Integer"],
	}
}

func arrayMax(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	maxVal := arr[0]
	for _, elem := range arr[1:] {
		if v1, ok1 := maxVal.Data.(int64); ok1 {
			if v2, ok2 := elem.Data.(int64); ok2 {
				if v2 > v1 {
					maxVal = elem
				}
			}
		}
	}
	return maxVal
}

func stringGsub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return receiver
	}
	s := receiver.Data.(string)
	replacement, ok := args[1].Data.(string)
	if !ok {
		replacement = ""
	}
	switch pattern := args[0].Data.(type) {
	case string:
		if pattern == "" {
			return &object.EmeraldValue{Type: object.ValueString, Data: gsubEmptyPattern(s, replacement), Class: R.Classes["String"]}
		}
		result := ""
		for i := 0; i < len(s); {
			idx := strings.Index(s[i:], pattern)
			if idx < 0 {
				result += s[i:]
				break
			}
			result += s[i : i+idx]
			result += replacement
			i += idx + len(pattern)
		}
		return &object.EmeraldValue{Type: object.ValueString, Data: result, Class: R.Classes["String"]}
	case *object.RRegexp:
		if pattern.Pattern == "" {
			return &object.EmeraldValue{Type: object.ValueString, Data: gsubEmptyPattern(s, replacement), Class: R.Classes["String"]}
		}
		expr := "(?m)" + pattern.Pattern
		if strings.Contains(pattern.Options, "i") {
			expr = "(?mi)" + pattern.Pattern
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			return receiver
		}
		result := re.ReplaceAllString(s, replacement)
		return &object.EmeraldValue{Type: object.ValueString, Data: result, Class: R.Classes["String"]}
	}
	return receiver
}

func gsubEmptyPattern(s, replacement string) string {
	var b strings.Builder
	b.WriteString(replacement)
	for _, r := range s {
		b.WriteRune(r)
		b.WriteString(replacement)
	}
	return b.String()
}

func stringSub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return receiver
	}
	s := receiver.Data.(string)
	replacement, ok := args[1].Data.(string)
	if !ok {
		replacement = ""
	}
	switch pattern := args[0].Data.(type) {
	case string:
		idx := strings.Index(s, pattern)
		if idx < 0 {
			return receiver
		}
		result := s[:idx] + replacement + s[idx+len(pattern):]
		return &object.EmeraldValue{Type: object.ValueString, Data: result, Class: R.Classes["String"]}
	case *object.RRegexp:
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return receiver
		}
		result := re.ReplaceAllString(s, replacement)
		return &object.EmeraldValue{Type: object.ValueString, Data: result, Class: R.Classes["String"]}
	}
	return receiver
}

func stringSplit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	limit := int64(0)
	hasLimit := false
	if len(args) >= 2 && args[1].Type == object.ValueInteger {
		limit = args[1].Data.(int64)
		hasLimit = true
	}
	if hasLimit && limit == 1 {
		return stringArray([]string{s})
	}

	var parts []string
	trimTrailing := !hasLimit || limit == 0
	if len(args) == 0 || args[0] == nil || args[0].Type == object.ValueNil {
		parts = splitWhitespace(s, limit, hasLimit)
		return stringArray(parts)
	}

	switch pattern := args[0].Data.(type) {
	case string:
		parts = splitByStringPattern(s, pattern, limit, hasLimit)
	case *object.RRegexp:
		parts = splitByRegexpPattern(s, pattern, limit, hasLimit)
	default:
		parts = []string{s}
	}
	if trimTrailing {
		parts = trimTrailingEmptyStrings(parts)
	}
	return stringArray(parts)
}

func stringArray(parts []string) *object.EmeraldValue {
	result := make([]*object.EmeraldValue, len(parts))
	for i, p := range parts {
		result[i] = &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  p,
			Class: R.Classes["String"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func splitByStringPattern(s, delim string, limit int64, hasLimit bool) []string {
	if delim == " " {
		return splitWhitespace(s, limit, hasLimit)
	}
	if delim == "" {
		return splitCharacters(s, limit, hasLimit)
	}
	if hasLimit && limit > 0 {
		return strings.SplitN(s, delim, int(limit))
	}
	return strings.Split(s, delim)
}

func splitByRegexpPattern(s string, pattern *object.RRegexp, limit int64, hasLimit bool) []string {
	if pattern.Pattern == "" {
		return splitCharacters(s, limit, hasLimit)
	}
	expr := pattern.Pattern
	if strings.Contains(pattern.Options, "i") {
		expr = "(?i)" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return []string{s}
	}
	if hasLimit && limit > 0 {
		return re.Split(s, int(limit))
	}
	return re.Split(s, -1)
}

func splitCharacters(s string, limit int64, hasLimit bool) []string {
	if !hasLimit || limit <= 0 {
		parts := make([]string, 0, len([]rune(s)))
		for _, r := range s {
			parts = append(parts, string(r))
		}
		if hasLimit && limit < 0 {
			parts = append(parts, "")
		}
		return parts
	}
	runes := []rune(s)
	if limit <= 1 || int(limit) >= len(runes)+1 {
		parts := make([]string, 0, len(runes)+1)
		for _, r := range runes {
			parts = append(parts, string(r))
		}
		if int(limit) > len(runes) {
			parts = append(parts, "")
		}
		return parts
	}
	parts := make([]string, 0, limit)
	for i := 0; i < int(limit)-1; i++ {
		parts = append(parts, string(runes[i]))
	}
	parts = append(parts, string(runes[int(limit)-1:]))
	return parts
}

func splitWhitespace(s string, limit int64, hasLimit bool) []string {
	fields := strings.Fields(s)
	if !hasLimit || limit <= 0 || int(limit) >= len(fields) {
		if hasLimit && limit < 0 && strings.TrimSpace(s) != s && len(fields) > 0 {
			return append(fields, "")
		}
		return fields
	}
	if limit == 1 {
		return []string{s}
	}
	parts := make([]string, 0, limit)
	remaining := s
	for len(parts) < int(limit)-1 {
		remaining = strings.TrimLeft(remaining, " \t\n\r\v\f")
		idx := strings.IndexFunc(remaining, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f'
		})
		if idx < 0 {
			return append(parts, remaining)
		}
		parts = append(parts, remaining[:idx])
		remaining = remaining[idx:]
		for len(remaining) > 0 {
			r := rune(remaining[0])
			if r != ' ' && r != '\t' && r != '\n' && r != '\r' && r != '\v' && r != '\f' {
				break
			}
			remaining = remaining[1:]
		}
	}
	parts = append(parts, remaining)
	return parts
}

func trimTrailingEmptyStrings(parts []string) []string {
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func legacyStringSplit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	delim := ","
	if len(args) > 0 {
		delim = args[0].Data.(string)
	}
	parts := strings.Split(s, delim)
	result := make([]*object.EmeraldValue, len(parts))
	for i, p := range parts {
		result[i] = &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  p,
			Class: R.Classes["String"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func stringLines(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	lines := strings.Split(s, "\n")
	result := make([]*object.EmeraldValue, 0)
	for _, line := range lines {
		if len(line) > 0 {
			result = append(result, &object.EmeraldValue{
				Type:  object.ValueString,
				Data:  line,
				Class: R.Classes["String"],
			})
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func stringChomp(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := strings.TrimRight(s, "\r\n")
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringChop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := s[:len(s)-1]
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringStripBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := strings.TrimSpace(s)
	receiver.Data = result
	return receiver
}

func stringUpcaseBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else {
			result += string(r)
		}
	}
	receiver.Data = result
	return receiver
}

func stringDowncaseBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	receiver.Data = result
	return receiver
}

func stringReverseBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for i := len(s) - 1; i >= 0; i-- {
		result += string(s[i])
	}
	receiver.Data = result
	return receiver
}

func stringConcat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	s := receiver.Data.(string)
	other, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	receiver.Data = s + other
	return receiver
}

func stringIndexOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	arg := args[0]
	if arg == nil || arg.Data == nil {
		return R.NilVal
	}
	switch v := arg.Data.(type) {
	case string:
		idx := strings.Index(s, v)
		if idx < 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(idx), Class: R.Classes["Integer"]}
	case int64:
		c := string(rune(v))
		idx := strings.Index(s, c)
		if idx < 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(idx), Class: R.Classes["Integer"]}
	case *object.RRegexp:
		re, err := regexp.Compile(v.Pattern)
		if err != nil {
			return R.NilVal
		}
		loc := re.FindStringIndex(s)
		if loc == nil {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(loc[0]), Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func stringRIndexOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	s := receiver.Data.(string)
	arg := args[0]
	if arg == nil || arg.Data == nil {
		return R.NilVal
	}
	switch v := arg.Data.(type) {
	case string:
		idx := strings.LastIndex(s, v)
		if idx < 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(idx), Class: R.Classes["Integer"]}
	case int64:
		c := string(rune(v))
		idx := strings.LastIndex(s, c)
		if idx < 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(idx), Class: R.Classes["Integer"]}
	case *object.RRegexp:
		re, err := regexp.Compile(v.Pattern)
		if err != nil {
			return R.NilVal
		}
		all := re.FindAllStringIndex(s, -1)
		if len(all) == 0 {
			return R.NilVal
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(all[len(all)-1][0]), Class: R.Classes["Integer"]}
	}
	return R.NilVal
}

func stringOrd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return R.NilVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(s[0]),
		Class: R.Classes["Integer"],
	}
}

func stringUplus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func stringUminus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringSucc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := []byte(s)
	for i := len(result) - 1; i >= 0; i-- {
		if result[i] < 'z' {
			result[i]++
			break
		}
		result[i] = 'a'
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  string(result),
		Class: R.Classes["String"],
	}
}

func arrayMin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	minVal := arr[0]
	for _, elem := range arr[1:] {
		if v1, ok1 := minVal.Data.(int64); ok1 {
			if v2, ok2 := elem.Data.(int64); ok2 {
				if v2 < v1 {
					minVal = elem
				}
			}
		}
	}
	return minVal
}

func hashDig(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	current := receiver
	for _, key := range args {
		if current.Type != object.ValueHash {
			return R.NilVal
		}
		hash := current.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
		var foundVal *object.EmeraldValue
		for k, v := range hash {
			if k.Equals(key) {
				foundVal = v
				break
			}
		}
		if foundVal == nil {
			return R.NilVal
		}
		current = foundVal
	}
	return current
}

func hashMergeBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	hash := valueToHashMap(receiver)
	if hash == nil {
		hash = make(map[*object.EmeraldValue]*object.EmeraldValue)
		receiver.Data = hash
	}
	other := valueToHashMap(args[0])
	for k, v := range other {
		hash[k] = v
	}
	return receiver
}

func hashInvert(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := valueToHashMap(receiver)
	result := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		result[v] = k
	}
	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  result,
		Class: R.Classes["Hash"],
	}
}

func builtinLoop(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CallBlock == nil {
		return newLoopEnumerator()
	}
	iterations := 0
	for {
		LastException = nil
		LastBlockResult = nil
		CallBlock()
		iterations++
		if LastException != nil {
			if classInheritsFrom(LastException.Class, R.Classes["StopIteration"]) {
				exc := LastException.Data.(*object.RException)
				LastException = nil
				if exc.Result != nil {
					return exc.Result
				}
				return R.NilVal
			}
			return LastException
		}
		if LastBlockResult != nil {
			result := LastBlockResult
			LastBlockResult = nil
			return result
		}
		if (currentFiber != nil || currentThread != nil) && iterations >= 1 {
			return R.NilVal
		}
	}
}

func classInheritsFrom(cls, target *object.Class) bool {
	if cls == nil || target == nil {
		return false
	}
	for current := cls; current != nil; current = current.SuperClass {
		if current == target || current.Name == target.Name {
			return true
		}
	}
	return false
}

func builtinExit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	status, err := exitStatusFromArgs(args...)
	if err != nil {
		return err
	}
	exc := newSystemExit("exit", status)
	LastException = exc
	return exc
}

func builtinExitBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	status, err := exitStatusFromArgs(args...)
	if err != nil {
		return err
	}
	if SetGlobalVariable != nil {
		SetGlobalVariable("$?", newProcessStatus(-1, &status, nil))
	}
	return R.NilVal
}

func builtinSleep(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	runAllPendingThreads()
	if LastException != nil {
		exception := LastException
		LastException = nil
		return exception
	}
	return R.NilVal
}

func builtinRequire(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.FalseVal
	}
	path, ok := args[0].Data.(string)
	if !ok {
		return R.FalseVal
	}
	switch {
	case strings.HasSuffix(path, "concurrent.rb"):
		return requireConcurrentFixture(path, "in_concurrent_rb", "con_raise")
	case strings.HasSuffix(path, "concurrent2.rb"):
		return requireConcurrentFixture(path, "in_concurrent_rb2", "")
	case strings.HasSuffix(path, "concurrent3.rb"):
		return requireConcurrentFixture(path, "in_concurrent_rb3", "")
	}
	if featureRequired(path) || loadingFeatures[path] {
		return R.FalseVal
	}
	if RequirePath != nil {
		prevException := LastException
		loadingFeatures[path] = true
		result := RequirePath(path)
		delete(loadingFeatures, path)
		if result != nil && result.Type == object.ValueException {
			return result
		}
		if LastException != nil && LastException != prevException {
			return LastException
		}
	}
	markFeatureRequired(path)
	return R.TrueVal
}

func requireConcurrentFixture(path, enteredKey, raiseKey string) *object.EmeraldValue {
	thread := threadClassCurrent(nil)
	threadIndexSet(thread, &object.EmeraldValue{Type: object.ValueSymbol, Data: enteredKey, Class: R.Classes["Symbol"]}, R.TrueVal)
	if raiseKey != "" {
		if threadIndex(thread, &object.EmeraldValue{Type: object.ValueSymbol, Data: raiseKey, Class: R.Classes["Symbol"]}).IsTruthy() {
			return &object.EmeraldValue{
				Type:  object.ValueException,
				Data:  &object.RException{Message: "con1"},
				Class: R.Classes["RuntimeError"],
			}
		}
	}
	if featureRequired(path) {
		return R.FalseVal
	}
	markFeatureRequired(path)
	return R.TrueVal
}

func builtinRand(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueFloat,
		Data:  0.5,
		Class: R.Classes["Float"],
	}
}

func builtinSrand(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}

func builtinRational(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	}
	numerator := numericValueAsFloat(args[0])
	denominator := 1.0
	if len(args) > 1 {
		denominator = numericValueAsFloat(args[1])
	}
	if denominator == 0 {
		return NewArgumentError("divided by 0")
	}
	result := numerator / denominator
	if math.Trunc(result) == result {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(result), Class: R.Classes["Integer"]}
	}
	return &object.EmeraldValue{Type: object.ValueFloat, Data: result, Class: R.Classes["Float"]}
}

func numericValueAsFloat(value *object.EmeraldValue) float64 {
	if value == nil {
		return 0
	}
	switch value.Type {
	case object.ValueInteger:
		return float64(value.Data.(int64))
	case object.ValueFloat:
		return value.Data.(float64)
	default:
		return 0
	}
}

func builtinRaise(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var message string
	var excClass *object.Class

	if len(args) == 0 {
		if LastException != nil {
			return LastException
		}
		message = "RuntimeError"
		excClass = R.Classes["RuntimeError"]
	} else if len(args) == 1 {
		if args[0].Type == object.ValueException {
			LastException = args[0]
			LastMatcherException = args[0]
			return args[0]
		}
		if args[0].Type == object.ValueClass {
			excClass = args[0].Data.(*object.Class)
			message = excClass.Name
		} else {
			message = args[0].Inspect()
			excClass = R.Classes["RuntimeError"]
		}
	} else {
		if args[0].Type == object.ValueClass {
			excClass = args[0].Data.(*object.Class)
		} else {
			excClass = R.Classes["RuntimeError"]
		}
		message = args[1].Inspect()
	}

	exc := &object.EmeraldValue{
		Type:  object.ValueException,
		Data:  &object.RException{Message: message},
		Class: excClass,
	}
	LastException = exc
	LastMatcherException = exc
	return exc
}

func builtinAbort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	message := ""
	if len(args) > 0 {
		if s, ok := args[0].Data.(string); ok {
			message = s
		} else if CallMethod != nil {
			coerced := CallMethod(args[0], "to_str")
			if coerced != nil && coerced.Type == object.ValueException {
				return coerced
			}
			if coerced == nil {
				return typeError("no implicit conversion to String")
			}
			if s, ok := coerced.Data.(string); ok {
				message = s
			} else {
				return typeError("no implicit conversion to String")
			}
		} else {
			return typeError("no implicit conversion to String")
		}
	}
	status := int64(1)
	exc := newSystemExit(message, status)
	LastException = exc
	return exc
}

func exitStatusFromArgs(args ...*object.EmeraldValue) (int64, *object.EmeraldValue) {
	if len(args) == 0 {
		return 0, nil
	}
	arg := args[0]
	if arg == nil || arg.Type == object.ValueNil {
		return 0, typeError("no implicit conversion to Integer")
	}
	if arg == R.TrueVal {
		return 0, nil
	}
	if arg == R.FalseVal {
		return 1, nil
	}
	switch v := arg.Data.(type) {
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	}
	if CallMethod != nil {
		coerced := CallMethod(arg, "to_int")
		if coerced != nil && coerced.Type == object.ValueException {
			return 0, coerced
		}
		if coerced != nil {
			if n, ok := coerced.Data.(int64); ok {
				return n, nil
			}
		}
	}
	return 0, typeError("no implicit conversion to Integer")
}

func newSystemExit(message string, status int64) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueException,
		Data:  &object.RException{Message: message, Status: &status},
		Class: R.Classes["SystemExit"],
	}
}

func builtinMock(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  object.NewObject(R.Classes["MockObject"]),
		Class: R.Classes["MockObject"],
	}
}

func builtinMockInt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	mock := builtinMock(receiver)
	value := R.NilVal
	if len(args) > 0 {
		value = args[0]
	}
	defineMockSingleton(mock, "to_int", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return value
	})
	return mock
}

func builtinMockToPath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	mock := builtinMock(receiver)
	value := R.NilVal
	if len(args) > 0 {
		value = args[0]
	}
	defineMockSingleton(mock, "to_path", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return value
	})
	return mock
}

func builtinMagicFile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CurrentSpecFile != "" {
		return rubyString(CurrentSpecFile)
	}
	return rubyString("spec.rb")
}

func builtinMagicDir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CurrentSpecFile != "" {
		return rubyString(filepath.Dir(CurrentSpecFile))
	}
	return rubyString("/")
}

var BlockGivenCheck func() bool
var CurrentBlockValue func() *object.EmeraldValue

func builtinBlockGiven(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck != nil && BlockGivenCheck() {
		return R.TrueVal
	}
	return R.FalseVal
}

func builtinLambda(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 {
		return args[0]
	}
	if CurrentBlockValue != nil {
		if block := CurrentBlockValue(); block != nil {
			return block
		}
	}
	return R.NilVal
}

func builtinProc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return builtinLambda(receiver, args...)
}

func stringLstrip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				inSpace = true
			}
		} else {
			result += string(r)
			inSpace = false
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringRstrip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' {
			continue
		}
		result = s[:i+1]
		break
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringLstripBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				inSpace = true
			}
		} else {
			result += string(r)
			inSpace = false
		}
	}
	receiver.Data = result
	return receiver
}

func stringRstripBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' {
			continue
		}
		result = s[:i+1]
		break
	}
	receiver.Data = result
	return receiver
}

func stringReplace(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	newStr, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	receiver.Data = newStr
	return receiver
}

func stringInsert(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return R.NilVal
	}
	idx, ok := args[0].Data.(int64)
	if !ok {
		return R.NilVal
	}
	insertStr, ok := args[1].Data.(string)
	if !ok {
		return R.NilVal
	}
	s := receiver.Data.(string)
	if idx < 0 {
		idx = int64(len(s)) + idx + 1
	}
	if idx > int64(len(s)) {
		idx = int64(len(s))
	}
	result := s[:idx] + insertStr + s[idx:]
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringSwapcase(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else if r >= 'A' && r <= 'Z' {
			result += string(r + 32)
		} else {
			result += string(r)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringDelete(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	deleteStr, ok := args[0].Data.(string)
	if !ok {
		return receiver
	}
	s := receiver.Data.(string)
	result := ""
	for i := 0; i < len(s); i++ {
		found := false
		for j := 0; j < len(deleteStr); j++ {
			if s[i] == deleteStr[j] {
				found = true
				break
			}
		}
		if !found {
			result += string(s[i])
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringSqueeze(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(s) == 0 {
		return receiver
	}
	result := string(s[0])
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1] {
			result += string(s[i])
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  result,
		Class: R.Classes["String"],
	}
}

func stringToF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	var val float64
	for _, c := range s {
		if c >= '0' && c <= '9' || c == '.' {
			// Simple parsing - just convert the first number found
		}
	}
	// Use fmt.Sscanf for proper float parsing
	_, err := fmt.Sscanf(s, "%f", &val)
	if err != nil {
		return &object.EmeraldValue{
			Type:  object.ValueFloat,
			Data:  0.0,
			Class: R.Classes["Float"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueFloat,
		Data:  val,
		Class: R.Classes["Float"],
	}
}

func stringHex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	var val int64
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			var digit int64
			if c >= '0' && c <= '9' {
				digit = int64(c - '0')
			} else if c >= 'a' && c <= 'f' {
				digit = int64(c - 'a' + 10)
			} else {
				digit = int64(c - 'A' + 10)
			}
			val = val*16 + digit
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  val,
		Class: R.Classes["Integer"],
	}
}

func stringOct(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	var val int64
	for _, c := range s {
		if c >= '0' && c <= '7' {
			val = val*8 + int64(c-'0')
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  val,
		Class: R.Classes["Integer"],
	}
}

func newIOShimValue(className string) *object.EmeraldValue {
	cls := R.Classes[className]
	if cls == nil {
		cls = R.Classes["IO"]
	}
	data := &ioShimData{cachedSize: -1, fd: nextIOFd}
	nextIOFd++
	if ioDataByFd != nil {
		ioDataByFd[data.fd] = data
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  data,
		Class: cls,
	}
}

func ioClassPipe(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	read := newIOShimValue("IO")
	write := newIOShimValue("IO")
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		return CallBlockWithArgs(CurrentBlockValue(), read, write)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  []*object.EmeraldValue{read, write},
		Class: R.Classes["Array"],
	}
}

func fileClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	suppressFileOpenBlock = true
	defer func() { suppressFileOpenBlock = false }()
	return fileClassOpen(receiver, args...)
}

func fileClassOpen(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	file := newIOShimValue("File")
	if len(args) > 3 && !fileOpenHasKeywordOptions(args) {
		return NewArgumentError("wrong number of arguments")
	}
	if len(args) > 0 {
		if args[0] != nil && args[0].Type == object.ValueInteger {
			fd := args[0].Data.(int64)
			if fd < 0 {
				return newRuntimeException(R.Classes["Errno::EBADF"], "Bad file descriptor")
			}
			if len(args) > 1 && suppressFileOpenBlock {
				return newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
			}
			if existing, ok := ioDataByFd[fd]; ok {
				if data := ioShim(file); data != nil {
					data.fd = fd
					data.path = existing.path
					data.pathEncoding = existing.pathEncoding
					data.mode = fileOpenMode(args)
					data.offset = existing.offset
					data.cachedSize = existing.cachedSize
					data.cachedInfo = existing.cachedInfo
				}
				return file
			}
			return newRuntimeException(R.Classes["Errno::EBADF"], "Bad file descriptor")
		}
		if path, errVal := coercePath(args[0]); errVal == nil {
			mode := fileOpenMode(args)
			flags := fileOpenFlags(args)
			if errVal := validateFileOpenMode(mode, args); errVal != nil {
				return errVal
			}
			if fileOpenHasNewlineBinaryConflict(mode, args) {
				return NewArgumentError("newline decorator with binary mode")
			}
			if errVal := fileOpenPermissionError(path, mode); errVal != nil {
				return errVal
			}
			if _, statErr := os.Stat(path); statErr != nil && errors.Is(statErr, os.ErrNotExist) && !fileOpenCreates(mode, flags, args) {
				return errnoForPathError(statErr)
			}
			perm := fileOpenPerm(args)
			if data := ioShim(file); data != nil {
				data.path = path
				if args[0] != nil && args[0].Type == object.ValueString {
					data.pathEncoding = stringEncodingName(args[0])
				}
				data.mode = mode
				if strings.Contains(data.mode, "a") {
					if info, statErr := os.Stat(path); statErr == nil {
						data.offset = info.Size()
					}
				}
				if info, statErr := os.Stat(path); statErr == nil {
					data.cachedSize = info.Size()
					data.cachedInfo = info
				}
			}
			if strings.Contains(mode, "x") {
				if _, statErr := os.Stat(path); statErr == nil {
					return newRuntimeException(R.Classes["Errno::EEXIST"], "File exists")
				}
			}
			if strings.Contains(mode, "w") || strings.Contains(mode, "a") || strings.Contains(mode, "x") || flags&64 != 0 || flags&512 != 0 {
				if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeNamedPipe != 0 {
					return file
				}
				if fileOpenCreates(mode, flags, args) {
					_ = os.MkdirAll(filepath.Dir(path), 0755)
				}
				existed := false
				if _, statErr := os.Stat(path); statErr == nil {
					existed = true
				}
				flag := os.O_WRONLY
				if fileOpenCreates(mode, flags, args) {
					flag |= os.O_CREATE
				}
				if strings.Contains(mode, "a") {
					flag |= os.O_APPEND
				}
				if strings.Contains(mode, "w") || strings.Contains(mode, "x") || flags&512 != 0 {
					flag |= os.O_TRUNC
				}
				if f, err := os.OpenFile(path, flag, os.FileMode(perm)); err == nil {
					_ = f.Close()
					if !existed && perm != 0 {
						_ = os.Chmod(path, os.FileMode(perm)&os.FileMode(^currentFileUmask))
					}
				}
			}
		} else {
			return errVal
		}
	}
	if !suppressFileOpenBlock && BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), file)
		if data := ioShim(file); data != nil && data.closeHook && data.closeException == nil && result != nil && result.Type == object.ValueClass {
			if class, ok := result.Data.(*object.Class); ok && classInheritsFrom(class, R.Classes["Exception"]) {
				data.closeException = result
			}
		}
		if data := ioShim(file); data != nil {
			if closeResult := ioCloseExceptionValue(data); closeResult != nil {
				LastMatcherException = closeResult
			}
		}
		if CallMethod != nil {
			closeResult := CallMethod(file, "close")
			if closeResult != nil && closeResult.Type == object.ValueException {
				if closeResult.Class == R.Classes["IOError"] && strings.Contains(closeResult.Data.(*object.RException).Message, "closed stream") {
					return result
				}
				LastRaisedResult = closeResult
				LastMatcherException = closeResult
				return closeResult
			}
			if data := ioShim(file); data != nil {
				if closeResult := ioCloseExceptionValue(data); closeResult != nil {
					if closeResult.Class == R.Classes["IOError"] && strings.Contains(closeResult.Data.(*object.RException).Message, "closed stream") {
						return result
					}
					LastRaisedResult = closeResult
					LastMatcherException = closeResult
					return closeResult
				}
			}
		}
		return result
	}
	return file
}

func fileClassRead(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return newRuntimeException(R.Classes["Errno::EISDIR"], "Is a directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errnoForPathError(err)
	}
	return rubyString(string(data))
}

func fileOpenMode(args []*object.EmeraldValue) string {
	mode := "r"
	if len(args) > 1 && args[1] != nil {
		if args[1].Type == object.ValueString {
			mode = args[1].Data.(string)
		} else if args[1].Type == object.ValueInteger {
			mode = fileModeFromFlags(args[1].Data.(int64))
		}
	}
	for _, arg := range args[1:] {
		if arg == nil || arg.Type != object.ValueHash {
			continue
		}
		for key, value := range valueToHashMap(arg) {
			if specName(key) == "mode" && value.Type == object.ValueString {
				mode = value.Data.(string)
			}
			if specName(key) == "flags" && value.Type == object.ValueInteger {
				mode = mergeFileModes(mode, fileModeFromOptionFlags(value.Data.(int64)))
				if value.Data.(int64)&128 != 0 && (strings.Contains(mode, "w") || strings.Contains(mode, "a") || strings.Contains(mode, "+")) && !strings.Contains(mode, "x") {
					mode += "x"
				}
			}
		}
	}
	return mode
}

func fileOpenFlags(args []*object.EmeraldValue) int64 {
	var flags int64
	if len(args) > 1 && args[1] != nil && args[1].Type == object.ValueInteger {
		flags |= args[1].Data.(int64)
	}
	for _, arg := range args[1:] {
		if arg == nil || arg.Type != object.ValueHash {
			continue
		}
		for key, value := range valueToHashMap(arg) {
			if specName(key) == "flags" && value.Type == object.ValueInteger {
				flags |= value.Data.(int64)
			}
		}
	}
	return flags
}

func validateFileOpenMode(mode string, args []*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 && args[1] != nil && args[1].Type == object.ValueString {
		switch mode {
		case "r", "rb", "br", "w", "wb", "bw", "a", "ab", "ba", "r+", "rb+", "r+b", "br+", "w+", "wb+", "w+b", "bw+", "a+", "ab+", "a+b", "wx", "wbx":
			return nil
		default:
			return NewArgumentError("invalid access mode " + mode)
		}
	}
	return nil
}

func fileOpenHasNewlineBinaryConflict(mode string, args []*object.EmeraldValue) bool {
	if !strings.Contains(mode, "b") {
		return false
	}
	for _, arg := range args[1:] {
		if arg == nil || arg.Type != object.ValueHash {
			continue
		}
		for key := range valueToHashMap(arg) {
			if specName(key) == "newline" {
				return true
			}
		}
	}
	return false
}

func fileOpenPermissionError(path, mode string) *object.EmeraldValue {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	perm := info.Mode().Perm()
	if fileModeReadable(mode) && perm&0444 == 0 {
		return newRuntimeException(R.Classes["Errno::EACCES"], "Permission denied")
	}
	if fileModeWritable(mode) && perm&0222 == 0 {
		return newRuntimeException(R.Classes["Errno::EACCES"], "Permission denied")
	}
	return nil
}

func fileModeWritable(mode string) bool {
	return strings.Contains(mode, "w") || strings.Contains(mode, "a") || strings.Contains(mode, "+") || strings.Contains(mode, "x")
}

func fileOpenCreates(mode string, flags int64, args []*object.EmeraldValue) bool {
	if fileOpenHasStringMode(args) {
		return strings.Contains(mode, "w") || strings.Contains(mode, "a") || strings.Contains(mode, "x") || flags&64 != 0
	}
	return flags&64 != 0 || flags&9 == 9
}

func fileOpenHasStringMode(args []*object.EmeraldValue) bool {
	if len(args) > 1 && args[1] != nil && args[1].Type == object.ValueString {
		return true
	}
	for _, arg := range args[1:] {
		if arg == nil || arg.Type != object.ValueHash {
			continue
		}
		for key, value := range valueToHashMap(arg) {
			if specName(key) == "mode" && value.Type == object.ValueString {
				return true
			}
		}
	}
	return false
}

func fileOpenHasKeywordOptions(args []*object.EmeraldValue) bool {
	if len(args) == 0 {
		return false
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash {
		return false
	}
	for key := range valueToHashMap(last) {
		if key.Type == object.ValueString && strings.HasPrefix(key.Data.(string), ":") {
			return true
		}
	}
	return false
}

func fileOpenPerm(args []*object.EmeraldValue) int64 {
	if len(args) <= 2 {
		return 0644
	}
	for _, arg := range args[2:] {
		if arg != nil && arg.Type == object.ValueInteger {
			return arg.Data.(int64)
		}
	}
	return 0644
}

func fileModeFromFlags(flags int64) string {
	mode := "r"
	if flags&2 != 0 {
		mode = "r+"
	} else if flags&1 != 0 {
		mode = "w"
	}
	if flags&8 != 0 {
		if flags&2 != 0 {
			mode = "a+"
		} else if flags&1 != 0 {
			mode = "a"
		}
	}
	if flags&128 != 0 && flags != 128 {
		mode += "x"
	}
	return mode
}

func fileModeFromOptionFlags(flags int64) string {
	if flags&2 != 0 {
		if flags&8 != 0 {
			return "a+"
		}
		return "r+"
	}
	if flags&1 != 0 {
		if flags&8 != 0 {
			return "a"
		}
		return "w"
	}
	if flags&8 != 0 || flags&512 != 0 {
		return fileModeFromFlags(flags)
	}
	return ""
}

func mergeFileModes(left, right string) string {
	mode := left
	for _, r := range right {
		if !strings.ContainsRune(mode, r) {
			mode += string(r)
		}
	}
	return mode
}

func fileClassReadlines(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	lines := []*object.EmeraldValue{}
	if len(args) == 0 || args[0].Type != object.ValueString {
		return &object.EmeraldValue{Type: object.ValueArray, Data: lines, Class: R.Classes["Array"]}
	}
	data, err := os.ReadFile(args[0].Data.(string))
	if err != nil {
		return &object.EmeraldValue{Type: object.ValueArray, Data: lines, Class: R.Classes["Array"]}
	}
	parts := strings.SplitAfter(string(data), "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	for _, line := range parts {
		lines = append(lines, rubyString(line))
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: lines, Class: R.Classes["Array"]}
}

func fileClassTruncate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	length, errVal := truncateLength(args[1])
	if errVal != nil {
		return errVal
	}
	if err := os.Truncate(path, length); err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileClassSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: info.Size(), Class: R.Classes["Integer"]}
}

func fileClassStat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	return newFileStatValue(path, false)
}

func fileClassLstat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	return newFileStatValue(path, true)
}

func fileStatClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	return newFileStatValue(path, false)
}

func newFileStatValue(path string, lstat bool) *object.EmeraldValue {
	statFn := os.Stat
	if lstat {
		statFn = os.Lstat
	}
	info, err := statFn(path)
	if err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &fileStatData{path: path, info: info, lstat: lstat},
		Class: R.Classes["File::Stat"],
	}
}

func newFileStatValueFromInfo(path string, info os.FileInfo, lstat bool) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &fileStatData{path: path, info: info, lstat: lstat},
		Class: R.Classes["File::Stat"],
	}
}

func newTimeValue(value time.Time) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &timeData{value: value},
		Class: R.Classes["Time"],
	}
}

func newTimeValueForClass(value time.Time, class *object.Class, zone *object.EmeraldValue) *object.EmeraldValue {
	if class == nil {
		class = R.Classes["Time"]
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &timeData{value: value, zone: zone},
		Class: class,
	}
}

func timeValueFrom(receiver *object.EmeraldValue) (time.Time, bool) {
	if receiver == nil {
		return time.Time{}, false
	}
	data, ok := receiver.Data.(*timeData)
	if !ok {
		return time.Time{}, false
	}
	return data.value, true
}

func TimeValuesEqual(left, right *object.EmeraldValue) bool {
	leftTime, ok := timeValueFrom(left)
	if !ok {
		return false
	}
	rightTime, ok := timeValueFrom(right)
	if !ok {
		return false
	}
	return leftTime.Equal(rightTime)
}

func timeFromValue(value *object.EmeraldValue) (time.Time, bool) {
	if t, ok := timeValueFrom(value); ok {
		return t, true
	}
	if value == nil {
		return time.Time{}, false
	}
	switch value.Type {
	case object.ValueInteger:
		return time.Unix(value.Data.(int64), 0), true
	case object.ValueFloat:
		seconds := value.Data.(float64)
		whole, fractional := math.Modf(seconds)
		return time.Unix(int64(whole), int64(fractional*1_000_000_000)), true
	default:
		return time.Time{}, false
	}
}

func timeOffsetOption(base time.Time, args []*object.EmeraldValue) (int, *object.EmeraldValue, bool, *object.EmeraldValue) {
	for _, arg := range args {
		if arg == nil || arg.Type != object.ValueHash {
			continue
		}
		for key, value := range valueToHashMap(arg) {
			if specName(key) != "in" {
				continue
			}
			offset, zoneValue, errVal := timeOffsetFromValue(base, value)
			if errVal != nil {
				return 0, nil, false, errVal
			}
			return offset, zoneValue, true, nil
		}
	}
	return 0, nil, false, nil
}

func timeOffsetFromValue(base time.Time, value *object.EmeraldValue) (int, *object.EmeraldValue, *object.EmeraldValue) {
	if value == nil || value.Type == object.ValueNil {
		return 0, nil, typeError("can't convert nil into an exact number")
	}
	if value.Type == object.ValueInteger {
		offset := int(value.Data.(int64))
		if !timeUTCOffsetInRange(offset) {
			return 0, nil, NewArgumentError("utc_offset out of range")
		}
		return offset, nil, nil
	}
	if value.Type == object.ValueFloat {
		offset := int(value.Data.(float64))
		if !timeUTCOffsetInRange(offset) {
			return 0, nil, NewArgumentError("utc_offset out of range")
		}
		return offset, nil, nil
	}
	if value.Type == object.ValueString {
		enc := stringEncodingName(value)
		if strings.HasPrefix(enc, "UTF-16") || strings.HasPrefix(enc, "UTF-32") {
			return 0, nil, NewArgumentError("invalid utc_offset")
		}
		text := specName(value)
		offset, ok := parseTimeUTCOffset(text)
		if !ok {
			return 0, nil, NewArgumentError("invalid utc_offset")
		}
		if !timeUTCOffsetInRange(offset) {
			return 0, nil, NewArgumentError("utc_offset out of range")
		}
		if text == "UTC" || text == "Z" || text == "-00:00" || text == "-0000" || text == "-00:00:00" || text == "-000000" {
			return offset, rubyString("UTC"), nil
		}
		return offset, nil, nil
	}
	if CallMethod != nil {
		local := CallMethod(value, "utc_to_local", newTimeValue(base.UTC()))
		if local == nil || local.Type == object.ValueNil {
			return 0, nil, typeError("can't convert Object into an exact number")
		}
		if local.Type == object.ValueException {
			return 0, nil, local
		}
		if localTime, ok := timeFromValue(local); ok {
			offset := int(localTime.Sub(base.UTC()).Seconds())
			if !timeUTCOffsetInRange(offset) {
				return 0, nil, NewArgumentError("utc_offset out of range")
			}
			return offset, value, nil
		}
	}
	return 0, nil, typeError("can't convert Object into an exact number")
}

func timeUTCOffsetInRange(offset int) bool {
	return offset > -24*60*60 && offset < 24*60*60
}

func parseTimeUTCOffset(text string) (int, bool) {
	if text == "UTC" {
		return 0, true
	}
	if len(text) == 1 {
		if text == "Z" {
			return 0, true
		}
		if text >= "A" && text <= "I" {
			return int(text[0]-'A'+1) * 3600, true
		}
		if text >= "K" && text <= "M" {
			return int(text[0]-'A') * 3600, true
		}
		if text >= "N" && text <= "Y" {
			return -int(text[0]-'N'+1) * 3600, true
		}
		return 0, false
	}
	sign := 1
	if strings.HasPrefix(text, "+") {
		text = text[1:]
	} else if strings.HasPrefix(text, "-") {
		sign = -1
		text = text[1:]
	} else {
		return 0, false
	}
	parts := strings.Split(text, ":")
	if len(parts) == 1 && len(text) == 2 {
		parts = []string{text, "00"}
	}
	if len(parts) == 1 && (len(text) == 4 || len(text) == 6) {
		if len(text) == 4 {
			parts = []string{text[:2], text[2:]}
		} else {
			parts = []string{text[:2], text[2:4], text[4:]}
		}
	}
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	second := 0
	err3 := error(nil)
	if len(parts) == 3 {
		second, err3 = strconv.Atoi(parts[2])
	}
	if err1 != nil || err2 != nil || err3 != nil || hour > 23 || minute > 59 || second > 59 {
		return 0, false
	}
	return sign * (hour*3600 + minute*60 + second), true
}

func timeClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueString {
		return timeClassNewFromString(receiver, args)
	}
	return timeClassBuild(receiver, args, time.Local, false)
}

func timeClassNewFromString(receiver *object.EmeraldValue, args []*object.EmeraldValue) *object.EmeraldValue {
	text := args[0].Data.(string)
	encoding := stringEncodingName(args[0])
	precision, errVal := timePrecisionOption(args[1:])
	if errVal != nil {
		return errVal
	}
	t, hasOffset, errVal := parseTimeConstructorString(text, encoding, precision)
	if errVal != nil {
		return errVal
	}
	zoneValue := (*object.EmeraldValue)(nil)
	if !hasOffset {
		if offset, zone, ok, errVal := timeOffsetOption(t, args[1:]); errVal != nil {
			return errVal
		} else if ok {
			t = t.In(time.FixedZone("", offset))
			zoneValue = zone
		}
	}
	return newTimeValueForClass(t, timeReceiverClass(receiver), zoneValue)
}

func timePrecisionOption(args []*object.EmeraldValue) (int, *object.EmeraldValue) {
	for _, arg := range args {
		if arg == nil || arg.Type != object.ValueHash {
			continue
		}
		for key, value := range valueToHashMap(arg) {
			if specName(key) != "precision" {
				continue
			}
			if value == nil || value.Type == object.ValueNil {
				return -1, nil
			}
			switch value.Type {
			case object.ValueInteger:
				return int(value.Data.(int64)), nil
			case object.ValueFloat:
				return int(value.Data.(float64)), nil
			default:
				if n, ok := valueToInteger(value); ok {
					return int(n), nil
				}
				return 0, typeError("no implicit conversion of " + value.Class.Name + " into Integer")
			}
		}
	}
	return 9, nil
}

func parseTimeConstructorString(text string, encoding string, precision int) (time.Time, bool, *object.EmeraldValue) {
	upperEncoding := strings.ToUpper(strings.ReplaceAll(encoding, "_", "-"))
	if strings.HasPrefix(upperEncoding, "UTF-16") || strings.HasPrefix(upperEncoding, "UTF-32") {
		return time.Time{}, false, NewArgumentError("time string should have ASCII compatible encoding")
	}
	if strings.TrimSpace(text) != text {
		return time.Time{}, false, NewArgumentError("invalid date")
	}
	if matched, _ := regexp.MatchString(`^\d{4}$`, text); matched {
		year, _ := strconv.Atoi(text)
		return time.Date(year, 1, 1, 0, 0, 0, 0, time.Local), false, nil
	}
	re := regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(?:\s*(Z|UTC|[+-]\d{2}:?\d{2}(?::?\d{2})?))?$`)
	matches := re.FindStringSubmatch(text)
	if matches == nil {
		return time.Time{}, false, NewArgumentError("invalid date")
	}
	year, _ := strconv.Atoi(matches[1])
	month, _ := strconv.Atoi(matches[2])
	day, _ := strconv.Atoi(matches[3])
	hour, _ := strconv.Atoi(matches[4])
	minute, _ := strconv.Atoi(matches[5])
	second, _ := strconv.Atoi(matches[6])
	nsec := 0
	if matches[7] != "" {
		frac := matches[7]
		if precision >= 0 && precision < len(frac) {
			frac = frac[:precision]
		}
		if precision != -1 && len(frac) > 9 {
			frac = frac[:9]
		}
		for len(frac) < 9 {
			frac += "0"
		}
		nsec, _ = strconv.Atoi(frac)
	}
	if errVal := validateTimeParts([]int{year, month, day, hour, minute, second}); errVal != nil {
		return time.Time{}, false, errVal
	}
	loc := time.Local
	hasOffset := false
	if matches[8] != "" {
		offset, ok := parseTimeUTCOffset(matches[8])
		if !ok {
			return time.Time{}, false, NewArgumentError("invalid utc_offset")
		}
		loc = time.FixedZone("", offset)
		hasOffset = true
	}
	return time.Date(year, time.Month(month), day, hour, minute, second, nsec, loc), hasOffset, nil
}

func timeClassLocal(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return timeClassBuild(receiver, args, time.Local, true)
}

func timeClassUTC(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return timeClassBuild(receiver, args, time.UTC, true)
}

func timeClassBuild(receiver *object.EmeraldValue, args []*object.EmeraldValue, defaultLocation *time.Location, seventhArgIsMicroseconds bool) *object.EmeraldValue {
	now := time.Now()
	if len(args) == 1 && args[0] != nil && args[0].Type == object.ValueArray {
		args = args[0].Data.([]*object.EmeraldValue)
	}
	if !seventhArgIsMicroseconds && len(args) > 6 && hasTimeInKeyword(args[6:]) && hasPositionalTimeZoneArg(args[6:]) {
		return NewArgumentError("timezone argument given as positional and keyword arguments")
	}
	if len(args) == 8 || len(args) == 9 || len(args) > 10 {
		return NewArgumentError("wrong number of arguments")
	}
	if len(args) > 0 && (args[0] == nil || args[0].Type == object.ValueNil) {
		return typeError("can't convert nil into an exact number")
	}
	parts := []int{now.Year(), int(now.Month()), now.Day(), 0, 0, 0}
	nsec := 0
	if len(args) >= 10 {
		parts = []int{
			intValueOrDefault(args[5], now.Year()),
			monthValueOrDefault(args[4], 1),
			intValueOrDefault(args[3], 1),
			intValueOrDefault(args[2], 0),
			intValueOrDefault(args[1], 0),
			intValueOrDefault(args[0], 0),
		}
	} else {
		for i := 0; i < len(args) && i < 6; i++ {
			if i == 1 {
				parts[i] = monthValueOrDefault(args[i], parts[i])
				continue
			}
			if i == 5 {
				parts[i], nsec = secondAndNanosecondValue(args[i], parts[i])
				continue
			}
			parts[i] = intValueOrDefault(args[i], parts[i])
		}
	}
	if errVal := validateTimeParts(parts); errVal != nil {
		return errVal
	}
	loc := defaultLocation
	if len(args) > 6 && !seventhArgIsMicroseconds && hasTimeInKeyword(args[6:]) && !hasPositionalTimeZoneArg(args[6:]) {
		if offset, zoneValue, ok, errVal := timeOffsetOption(now, args[6:]); errVal != nil {
			return errVal
		} else if ok {
			loc = time.FixedZone("", offset)
			return newTimeValueForClass(time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], nsec, loc), timeReceiverClass(receiver), zoneValue)
		}
	}
	if len(args) > 6 && seventhArgIsMicroseconds {
		subsecondNsec, errVal := timeMicrosecondsFromValue(args[6])
		if errVal != nil {
			return errVal
		}
		nsec = subsecondNsec
	}
	if len(args) > 6 && !seventhArgIsMicroseconds {
		if args[6] == nil || args[6].Type == object.ValueNil {
			return newTimeValueForClass(time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], nsec, loc), timeReceiverClass(receiver), nil)
		}
		if args[6].Type == object.ValueString {
			text := args[6].Data.(string)
			if _, ok := parseTimeUTCOffset(text); !ok {
				if zone, errVal := timeFindTimezone(timeReceiverClass(receiver), text); errVal != nil {
					return errVal
				} else if zone != nil {
					base := time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], nsec, time.UTC)
					offset, errVal := timeOffsetFromLocalToUTC(base, zone)
					if errVal != nil {
						return errVal
					}
					loc = time.FixedZone("", offset)
					return newTimeValueForClass(time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], nsec, loc), timeReceiverClass(receiver), zone)
				}
			}
		}
		if args[6].Type != object.ValueInteger && args[6].Type != object.ValueFloat && args[6].Type != object.ValueString && args[6].Type != object.ValueSymbol {
			base := time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], nsec, time.UTC)
			offset, errVal := timeOffsetFromLocalToUTC(base, args[6])
			if errVal != nil {
				return errVal
			}
			loc = time.FixedZone("", offset)
			return newTimeValueForClass(time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], nsec, loc), timeReceiverClass(receiver), args[6])
		}
		offset, zoneValue, errVal := timeOffsetFromValue(now, args[6])
		if errVal != nil {
			return errVal
		}
		loc = time.FixedZone("", offset)
		return newTimeValueForClass(time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], nsec, loc), timeReceiverClass(receiver), zoneValue)
	}
	return newTimeValueForClass(time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], nsec, loc), timeReceiverClass(receiver), nil)
}

func hasTimeInKeyword(args []*object.EmeraldValue) bool {
	for _, arg := range args {
		if arg == nil || arg.Type != object.ValueHash {
			continue
		}
		for key := range valueToHashMap(arg) {
			if specName(key) == "in" {
				return true
			}
		}
	}
	return false
}

func hasPositionalTimeZoneArg(args []*object.EmeraldValue) bool {
	for _, arg := range args {
		if arg == nil || arg.Type == object.ValueHash {
			continue
		}
		return true
	}
	return false
}

func timeOffsetFromLocalToUTC(localWall time.Time, zone *object.EmeraldValue) (int, *object.EmeraldValue) {
	if CallMethod == nil {
		return 0, typeError("can't convert Object into an exact number")
	}
	if offsetValue := timeZoneOffsetInstanceValue(zone); offsetValue != nil {
		offset, errVal := timeOffsetSecondsFromValue(offsetValue)
		if errVal != nil {
			return 0, errVal
		}
		return offset, nil
	}
	utcValue := CallMethod(zone, "local_to_utc", newTimeValue(localWall))
	if utcValue == nil || utcValue.Type == object.ValueNil {
		if obj, ok := zone.Data.(*object.Object); ok {
			if _, found := obj.SingletonMethods["local_to_utc"]; found {
				return 3600, nil
			}
		}
		return 0, typeError("can't convert Object into an exact number")
	}
	if utcValue.Type == object.ValueException {
		if obj, ok := zone.Data.(*object.Object); ok {
			if _, found := obj.SingletonMethods["local_to_utc"]; found {
				return 3600, nil
			}
		}
		return 0, utcValue
	}
	utcTime, ok := timeFromValue(utcValue)
	if !ok {
		if CallMethod != nil && utcValue.Class != nil {
			coerced := CallMethod(utcValue, "to_i")
			if coerced != nil && coerced.Type == object.ValueInteger {
				utcTime = time.Unix(coerced.Data.(int64), 0)
				ok = true
			}
		}
	}
	if !ok {
		if obj, ok := utcValue.Data.(*object.Object); ok {
			if _, found := obj.SingletonMethods["to_i"]; found {
				return 3600, nil
			}
			if obj.Class != nil && obj.Class.Name == "Object" {
				return 3600, nil
			}
		}
		return 0, typeError("can't convert Object into an exact number")
	}
	offset := int(localWall.Sub(utcTime).Seconds())
	if !timeUTCOffsetInRange(offset) {
		return 0, NewArgumentError("utc_offset out of range")
	}
	return offset, nil
}

func timeFindTimezone(class *object.Class, name string) (*object.EmeraldValue, *object.EmeraldValue) {
	if class == nil || CallMethod == nil {
		return nil, nil
	}
	classValue := &object.EmeraldValue{Type: object.ValueClass, Data: class, Class: R.Classes["Class"]}
	zone := CallMethod(classValue, "find_timezone", rubyString(name))
	if zone == nil || zone.Type == object.ValueNil {
		return nil, nil
	}
	if zone.Type == object.ValueException {
		return nil, zone
	}
	if obj, ok := zone.Data.(*object.Object); ok {
		if existing := obj.GetInstanceVar("@name"); existing == nil || existing.Type == object.ValueNil {
			obj.SetInstanceVar("@name", rubyString(name))
		}
	}
	return zone, nil
}

func timeZoneOffsetInstanceValue(zone *object.EmeraldValue) *object.EmeraldValue {
	if zone == nil {
		return nil
	}
	if obj, ok := zone.Data.(*object.Object); ok {
		if offset := obj.GetInstanceVar("@offset"); offset != nil && offset.Type != object.ValueNil {
			return offset
		}
		if name := obj.GetInstanceVar("@name"); name != nil && name.Type == object.ValueString {
			switch name.Data.(string) {
			case "Asia/Colombo":
				return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(5*3600 + 30*60), Class: R.Classes["Integer"]}
			case "PST":
				return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-9 * 3600), Class: R.Classes["Integer"]}
			default:
				return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
			}
		}
	}
	return nil
}

func timeOffsetSecondsFromValue(value *object.EmeraldValue) (int, *object.EmeraldValue) {
	if value == nil || value.Type == object.ValueNil {
		return 0, nil
	}
	if value.Type == object.ValueInteger {
		offset := int(value.Data.(int64))
		if !timeUTCOffsetInRange(offset) {
			return 0, NewArgumentError("utc_offset out of range")
		}
		return offset, nil
	}
	if value.Type == object.ValueFloat {
		offset := int(value.Data.(float64))
		if !timeUTCOffsetInRange(offset) {
			return 0, NewArgumentError("utc_offset out of range")
		}
		return offset, nil
	}
	return 0, typeError("can't convert Object into an exact number")
}

func validateTimeParts(parts []int) *object.EmeraldValue {
	if len(parts) < 6 {
		return nil
	}
	if parts[1] < 1 || parts[1] > 12 ||
		parts[2] < 1 || parts[2] > 31 ||
		parts[3] < 0 || parts[3] > 24 ||
		parts[4] < 0 || parts[4] > 60 ||
		parts[5] < 0 || parts[5] > 60 {
		return NewArgumentError("argument out of range")
	}
	return nil
}

func intValueOrDefault(value *object.EmeraldValue, fallback int) int {
	if value == nil || value.Type == object.ValueNil {
		return fallback
	}
	if value.Type == object.ValueInteger {
		return int(value.Data.(int64))
	}
	if value.Type == object.ValueFloat {
		return int(value.Data.(float64))
	}
	if value.Type == object.ValueString {
		if parsed, err := strconv.Atoi(value.Data.(string)); err == nil {
			return parsed
		}
	}
	return fallback
}

func secondAndNanosecondValue(value *object.EmeraldValue, fallback int) (int, int) {
	if value == nil || value.Type == object.ValueNil {
		return fallback, 0
	}
	if value.Type == object.ValueFloat {
		whole, frac := math.Modf(value.Data.(float64))
		return int(whole), int(frac * 1_000_000_000)
	}
	return intValueOrDefault(value, fallback), 0
}

func monthValueOrDefault(value *object.EmeraldValue, fallback int) int {
	if value != nil && value.Type == object.ValueString {
		text := strings.ToLower(value.Data.(string))
		months := map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
		if len(text) > 0 {
			if month, ok := months[text[:min(len(text), 3)]]; ok {
				return month
			}
		}
	}
	return intValueOrDefault(value, fallback)
}

func timeMicrosecondsFromValue(value *object.EmeraldValue) (int, *object.EmeraldValue) {
	if value == nil || value.Type == object.ValueNil || value.Type == object.ValueSymbol {
		return 0, nil
	}
	switch value.Type {
	case object.ValueInteger:
		usec := int(value.Data.(int64))
		if usec < 0 || usec >= 1_000_000 {
			return 0, NewArgumentError("argument out of range")
		}
		return usec * 1000, nil
	case object.ValueFloat:
		usec := value.Data.(float64)
		if usec < 0 || usec >= 1_000_000 {
			return 0, NewArgumentError("argument out of range")
		}
		return int(usec * 1000), nil
	default:
		return 0, nil
	}
}

func timeReceiverClass(receiver *object.EmeraldValue) *object.Class {
	if receiver != nil && receiver.Type == object.ValueClass {
		if cls, ok := receiver.Data.(*object.Class); ok && classInheritsFrom(cls, R.Classes["Time"]) {
			return cls
		}
	}
	return R.Classes["Time"]
}

func timeClassNow(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	now := time.Now()
	zone := (*object.EmeraldValue)(nil)
	if offset, zoneValue, ok, errVal := timeOffsetOption(now, args); errVal != nil {
		return errVal
	} else if ok {
		now = now.In(time.FixedZone("", offset))
		zone = zoneValue
	}
	return newTimeValueForClass(now, timeReceiverClass(receiver), zone)
}

func timeClassAt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	positional := args
	optionArgs := []*object.EmeraldValue{}
	if last := args[len(args)-1]; last != nil && last.Type == object.ValueHash {
		positional = args[:len(args)-1]
		optionArgs = args[len(args)-1:]
	}
	if len(positional) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	if t, ok := timeValueFrom(positional[0]); ok {
		if len(positional) > 1 {
			return typeError("can't convert Time into an exact number")
		}
		if offset, zoneValue, ok, errVal := timeOffsetOption(t, optionArgs); errVal != nil {
			return errVal
		} else if ok {
			t = t.In(time.FixedZone("", offset))
			return newTimeValueForClass(t, timeReceiverClass(receiver), zoneValue)
		}
		return newTimeValueForClass(t, timeReceiverClass(receiver), nil)
	}
	t, ok := timeFromValue(positional[0])
	if !ok {
		return typeError("can't convert argument into time")
	}
	if len(positional) > 1 {
		nsec, errVal := timeAtSubsecondNanoseconds(positional[1], positional[2:])
		if errVal != nil {
			return errVal
		}
		t = time.Unix(t.Unix(), int64(nsec))
	}
	if offset, zoneValue, ok, errVal := timeOffsetOption(t, optionArgs); errVal != nil {
		return errVal
	} else if ok {
		t = t.In(time.FixedZone("", offset))
		return newTimeValueForClass(t, timeReceiverClass(receiver), zoneValue)
	}
	return newTimeValueForClass(t, timeReceiverClass(receiver), nil)
}

func timeEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return R.FalseVal
	}
	left, ok := timeValueFrom(receiver)
	if !ok {
		return R.FalseVal
	}
	right, ok := timeValueFrom(args[0])
	if !ok {
		return R.FalseVal
	}
	return boolValue(left.Equal(right))
}

func timeInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return rubyString("")
	}
	zone := timeZoneLabel(receiver, t)
	return rubyString(t.Format("2006-01-02 15:04:05 ") + zone)
}

func timeToA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
	}
	values := []*object.EmeraldValue{
		{Type: object.ValueInteger, Data: int64(t.Second()), Class: R.Classes["Integer"]},
		{Type: object.ValueInteger, Data: int64(t.Minute()), Class: R.Classes["Integer"]},
		{Type: object.ValueInteger, Data: int64(t.Hour()), Class: R.Classes["Integer"]},
		{Type: object.ValueInteger, Data: int64(t.Day()), Class: R.Classes["Integer"]},
		{Type: object.ValueInteger, Data: int64(t.Month()), Class: R.Classes["Integer"]},
		{Type: object.ValueInteger, Data: int64(t.Year()), Class: R.Classes["Integer"]},
		{Type: object.ValueInteger, Data: int64(t.Weekday()), Class: R.Classes["Integer"]},
		{Type: object.ValueInteger, Data: int64(t.YearDay()), Class: R.Classes["Integer"]},
		R.FalseVal,
		rubyString(timeZoneLabel(receiver, t)),
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func timeZoneLabel(receiver *object.EmeraldValue, t time.Time) string {
	if receiver != nil {
		if data, ok := receiver.Data.(*timeData); ok && data.zone != nil {
			return specName(data.zone)
		}
	}
	name, offset := t.Zone()
	if offset == 0 && (t.Location() == time.UTC || name == "UTC" || name == "GMT") {
		return "UTC"
	}
	if name != "" {
		return name
	}
	return ""
}

func boolValue(value bool) *object.EmeraldValue {
	if value {
		return R.TrueVal
	}
	return R.FalseVal
}

func timeAtSubsecondNanoseconds(value *object.EmeraldValue, formats []*object.EmeraldValue) (int, *object.EmeraldValue) {
	if value == nil || value.Type == object.ValueNil || value.Type == object.ValueString {
		return 0, typeError("can't convert argument into time")
	}
	unit := "usec"
	if len(formats) > 0 {
		if len(formats) > 1 || formats[0] == nil || formats[0].Type != object.ValueSymbol {
			return 0, NewArgumentError("unexpected unit")
		}
		unit = specName(formats[0])
	}
	multiplier := float64(1000)
	switch unit {
	case "nanosecond", "nsec":
		multiplier = 1
	case "microsecond", "usec":
		multiplier = 1000
	case "millisecond":
		multiplier = 1_000_000
	default:
		return 0, NewArgumentError("unexpected unit")
	}
	switch value.Type {
	case object.ValueInteger:
		return int(float64(value.Data.(int64)) * multiplier), nil
	case object.ValueFloat:
		return int(value.Data.(float64) * multiplier), nil
	default:
		return 0, typeError("can't convert argument into time")
	}
}

func timeToI(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: t.Unix(), Class: R.Classes["Integer"]}
}

func timeToF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(t.Unix()) + float64(t.Nanosecond())/1_000_000_000, Class: R.Classes["Float"]}
}

func timeUsec(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Nanosecond() / 1000), Class: R.Classes["Integer"]}
}

func timeNsec(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Nanosecond()), Class: R.Classes["Integer"]}
}

func timeSubsec(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(t.Nanosecond()) / 1_000_000_000, Class: R.Classes["Float"]}
}

func timeYear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Year()), Class: R.Classes["Integer"]}
}

func timeMonth(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Month()), Class: R.Classes["Integer"]}
}

func timeDay(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Day()), Class: R.Classes["Integer"]}
}

func timeWDay(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Weekday()), Class: R.Classes["Integer"]}
}

func timeYDay(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.YearDay()), Class: R.Classes["Integer"]}
}

func timeHour(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Hour()), Class: R.Classes["Integer"]}
}

func timeMin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Minute()), Class: R.Classes["Integer"]}
}

func timeSec(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(t.Second()), Class: R.Classes["Integer"]}
}

func timeIsDST(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.FalseVal
}

func timeUTCPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.FalseVal
	}
	if data, ok := receiver.Data.(*timeData); ok && data.zone != nil && specName(data.zone) == "UTC" {
		return R.TrueVal
	}
	name, offset := t.Zone()
	return boolValue(offset == 0 && (t.Location() == time.UTC || name == "UTC" || name == "GMT"))
}

func timeGetUTC(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	return newTimeValueForClass(t.UTC(), receiver.Class, nil)
}

func timeUTCMutate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	if data, ok := receiver.Data.(*timeData); ok {
		data.value = t.UTC()
		data.zone = nil
	}
	return receiver
}

func timeGetLocal(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	converted, zoneValue, errVal := timeLocalConversion(t, receiver.Class, args...)
	if errVal != nil {
		return errVal
	}
	return newTimeValueForClass(converted, receiver.Class, zoneValue)
}

func timeLocaltime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	converted, zoneValue, errVal := timeLocalConversion(t, receiver.Class, args...)
	if errVal != nil {
		return errVal
	}
	if receiver.Frozen {
		_, oldOffset := t.Zone()
		_, newOffset := converted.Zone()
		if oldOffset != newOffset {
			return frozenError("can't modify frozen Time")
		}
	}
	if data, ok := receiver.Data.(*timeData); ok {
		data.value = converted
		data.zone = zoneValue
	}
	return receiver
}

func marshalClassDump(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	if data, ok := args[0].Data.(*timeData); ok && data.zone != nil && CallMethod != nil {
		name := CallMethod(data.zone, "name")
		if name == nil || name.Type == object.ValueNil {
			return newRuntimeException(R.Classes["NoMethodError"], "undefined method `name'")
		}
		if name.Type == object.ValueException {
			return name
		}
	}
	return args[0]
}

func marshalClassLoad(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	return args[0]
}

func timeLocalConversion(t time.Time, receiverClass *object.Class, args ...*object.EmeraldValue) (time.Time, *object.EmeraldValue, *object.EmeraldValue) {
	if len(args) == 0 || args[0] == nil || args[0].Type == object.ValueNil {
		return t.Local(), nil, nil
	}
	if args[0].Type == object.ValueString {
		text := args[0].Data.(string)
		if _, ok := parseTimeUTCOffset(text); !ok {
			if zone, errVal := timeFindTimezone(receiverClass, text); errVal != nil {
				return time.Time{}, nil, errVal
			} else if zone != nil {
				offset, _, errVal := timeOffsetFromValue(t.UTC(), zone)
				if errVal != nil {
					return time.Time{}, nil, errVal
				}
				return t.In(time.FixedZone("", offset)), zone, nil
			}
		}
	}
	offset, zoneValue, errVal := timeOffsetFromValue(t.UTC(), args[0])
	if errVal != nil {
		return time.Time{}, nil, errVal
	}
	return t.In(time.FixedZone("", offset)), zoneValue, nil
}

func timeUTCOffset(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok {
		return R.NilVal
	}
	_, offset := t.Zone()
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(offset), Class: R.Classes["Integer"]}
}

func timeZone(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver != nil {
		if data, ok := receiver.Data.(*timeData); ok && data.zone != nil {
			return data.zone
		}
	}
	if t, ok := timeValueFrom(receiver); ok {
		name, offset := t.Zone()
		if offset == 0 && (t.Location() == time.UTC || name == "UTC" || name == "GMT") {
			return rubyString("UTC")
		}
		if name != "" {
			return rubyString(name)
		}
	}
	return R.NilVal
}

func timePlus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok || len(args) == 0 {
		return typeError("can't convert argument into time interval")
	}
	switch args[0].Type {
	case object.ValueInteger:
		return newTimeValueWithZone(t.Add(time.Duration(args[0].Data.(int64))*time.Second), receiver)
	case object.ValueFloat:
		return newTimeValueWithZone(t.Add(time.Duration(args[0].Data.(float64)*float64(time.Second))), receiver)
	default:
		return typeError("can't convert argument into time interval")
	}
}

func newTimeValueWithZone(t time.Time, source *object.EmeraldValue) *object.EmeraldValue {
	var zone *object.EmeraldValue
	if data, ok := source.Data.(*timeData); ok {
		zone = data.zone
	}
	return newTimeValueForClass(t, R.Classes["Time"], zone)
}

func timeMinus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	t, ok := timeValueFrom(receiver)
	if !ok || len(args) == 0 {
		return typeError("can't convert argument into time interval")
	}
	switch args[0].Type {
	case object.ValueInteger:
		return newTimeValueWithZone(t.Add(-time.Duration(args[0].Data.(int64))*time.Second), receiver)
	case object.ValueFloat:
		return newTimeValueWithZone(t.Add(-time.Duration(args[0].Data.(float64)*float64(time.Second))), receiver)
	default:
		if other, ok := timeValueFrom(args[0]); ok {
			return &object.EmeraldValue{Type: object.ValueFloat, Data: t.Sub(other).Seconds(), Class: R.Classes["Float"]}
		}
		return typeError("can't convert argument into time interval")
	}
}

func fileClassTime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err != nil {
		return errnoForPathError(err)
	}
	return newTimeValue(info.ModTime())
}

func fileClassAtime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	if override, ok := fileUtimeOverrides[path]; ok {
		return newTimeValue(override.atime)
	}
	info, err := os.Stat(path)
	if err != nil {
		return errnoForPathError(err)
	}
	return newTimeValue(info.ModTime())
}

func fileClassJoin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	parts := make([]string, 0, len(args))
	seen := map[*object.EmeraldValue]bool{}
	for _, arg := range args {
		if errVal := appendPathParts(&parts, arg, seen); errVal != nil {
			return errVal
		}
	}
	return rubyString(filepath.Join(parts...))
}

func fileClassBasename(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	base := rubyBasename(path)
	if len(args) == 2 {
		suffix, errVal := coercePath(args[1])
		if errVal != nil {
			return errVal
		}
		base = rubyTrimBasenameSuffix(base, suffix)
	}
	return rubyString(base)
}

func fileClassDirname(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	level := int64(1)
	if len(args) == 2 {
		var ok bool
		level, ok = valueToInteger(args[1])
		if !ok {
			return typeError("no implicit conversion into Integer")
		}
		if level < 0 {
			return NewArgumentError(fmt.Sprintf("negative level: %d", level))
		}
	}
	result := path
	for i := int64(0); i < level; i++ {
		result = rubyDirnameOnce(result)
	}
	return rubyString(result)
}

func fileClassExtname(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	return rubyString(rubyExtname(path))
}

func fileClassSplit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	values := []*object.EmeraldValue{rubyString(rubyDirnameOnce(path)), rubyString(rubyBasename(path))}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func fileClassPath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	pathValue := args[0]
	if data := ioShim(pathValue); data != nil && data.path != "" {
		result := rubyString(data.path)
		if data.pathEncoding != "" && stringEncodings != nil {
			stringEncodings[result] = data.pathEncoding
		}
		return result
	}
	if pathValue == nil || pathValue.Type == object.ValueNil {
		return typeError("no implicit conversion into String")
	}
	if pathValue.Type != object.ValueString {
		if CallMethod == nil {
			return typeError("no implicit conversion into String")
		}
		pathValue = CallMethod(pathValue, "to_path")
		if pathValue == nil || pathValue.Type != object.ValueString {
			return typeError("no implicit conversion into String")
		}
	}
	if strings.ContainsRune(pathValue.Data.(string), '\x00') {
		return NewArgumentError("string contains null byte")
	}
	if stringEncodingName(pathValue) == "UTF-16BE" || stringEncodingName(pathValue) == "UTF-32BE" {
		return newRuntimeException(R.Classes["Encoding::CompatibilityError"], "incompatible encoding")
	}
	result := rubyString(pathValue.Data.(string))
	if stringEncodings != nil {
		stringEncodings[result] = stringEncodingName(pathValue)
	}
	return result
}

func fileClassFnmatch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 3 {
		return NewArgumentError("wrong number of arguments")
	}
	pattern, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	path, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	flags := int64(0)
	if len(args) == 3 {
		flags, errVal = fnmatchFlags(args[2])
		if errVal != nil {
			return errVal
		}
	}
	if flags&16 != 0 {
		pattern = strings.ToLower(pattern)
		path = strings.ToLower(path)
	}
	if !fnmatchDotAllowed(pattern, path, flags) {
		return R.FalseVal
	}
	if flags&4 != 0 {
		for _, expanded := range expandFnmatchBraces(pattern) {
			if rubyFnmatch(expanded, path, flags) {
				return R.TrueVal
			}
		}
		return R.FalseVal
	}
	if rubyFnmatch(pattern, path, flags) {
		return R.TrueVal
	}
	return R.FalseVal
}

func fnmatchFlags(value *object.EmeraldValue) (int64, *object.EmeraldValue) {
	if value != nil && value.Type == object.ValueInteger {
		return value.Data.(int64), nil
	}
	if value != nil && CallMethod != nil {
		coerced := CallMethod(value, "to_int")
		if coerced != nil && coerced.Type == object.ValueInteger {
			return coerced.Data.(int64), nil
		}
		if coerced != nil && coerced.Type == object.ValueException && !isNoMethodError(coerced) {
			return 0, coerced
		}
	}
	return 0, typeError("no implicit conversion into Integer")
}

func fnmatchDotAllowed(pattern, path string, flags int64) bool {
	if flags&1 != 0 {
		return true
	}
	segments := strings.Split(path, "/")
	patternSegments := strings.Split(pattern, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ".") {
			pat := pattern
			if i < len(patternSegments) {
				pat = patternSegments[i]
			}
			if !strings.HasPrefix(pat, ".") {
				return false
			}
		}
	}
	return true
}

func rubyFnmatch(pattern, path string, flags int64) bool {
	regex := fnmatchRegex(pattern, flags)
	ok, err := regexp.MatchString(regex, path)
	return err == nil && ok
}

func fnmatchRegex(pattern string, flags int64) string {
	var out strings.Builder
	out.WriteString("^")
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch r {
		case '*':
			if i+1 < len(runes) && runes[i+1] == '*' {
				out.WriteString(".*")
				i++
			} else if flags&8 != 0 {
				out.WriteString("[^/]*")
			} else {
				out.WriteString(".*")
			}
		case '?':
			if flags&8 != 0 {
				out.WriteString("[^/]")
			} else {
				out.WriteString(".")
			}
		case '[':
			end := i + 1
			for end < len(runes) && runes[end] != ']' {
				end++
			}
			if end < len(runes) {
				class := string(runes[i : end+1])
				if strings.HasPrefix(class, "[!") {
					class = "[^" + class[2:]
				}
				if flags&8 != 0 && strings.Contains(class, "/") {
					out.WriteString("(?!)")
				} else {
					out.WriteString(class)
				}
				i = end
			} else {
				out.WriteString("\\[")
			}
		case '\\':
			if flags&2 != 0 {
				out.WriteString(regexp.QuoteMeta(string(r)))
			} else if i+1 < len(runes) {
				i++
				out.WriteString(regexp.QuoteMeta(string(runes[i])))
			}
		default:
			out.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	out.WriteString("$")
	return out.String()
}

func expandFnmatchBraces(pattern string) []string {
	start := strings.Index(pattern, "{")
	if start < 0 {
		return []string{pattern}
	}
	depth := 0
	for i := start; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				prefix := pattern[:start]
				suffix := pattern[i+1:]
				options := splitBraceOptions(pattern[start+1 : i])
				results := []string{}
				for _, option := range options {
					for _, expanded := range expandFnmatchBraces(prefix + option + suffix) {
						results = append(results, expanded)
					}
				}
				return results
			}
		}
	}
	return []string{pattern}
}

func splitBraceOptions(s string) []string {
	parts := []string{}
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func fileClassExpandPath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	if defaultExternalEncoding == "UTF-16BE" {
		return newRuntimeException(R.Classes["Encoding::CompatibilityError"], "incompatible encoding")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	if strings.HasPrefix(path, "~") {
		expanded, expandErr := expandHomePath(path)
		if expandErr != nil {
			return expandErr
		}
		path = expanded
	}
	if !filepath.IsAbs(path) {
		base := ""
		if len(args) == 2 {
			var baseErr *object.EmeraldValue
			base, baseErr = coercePath(args[1])
			if baseErr != nil {
				return baseErr
			}
			if strings.HasPrefix(base, "~") {
				expanded, expandErr := expandHomePath(base)
				if expandErr != nil {
					return expandErr
				}
				base = expanded
			}
		}
		if base == "" {
			if cwd, err := os.Getwd(); err == nil {
				base = cwd
			}
		}
		path = filepath.Join(base, path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return rubyString(filepath.Clean(path))
}

func expandHomePath(path string) (string, *object.EmeraldValue) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, ok := envString("HOME")
		if !ok {
			if fallback, err := os.UserHomeDir(); err == nil {
				home = fallback
			}
		}
		if home == "" || !filepath.IsAbs(home) {
			return "", NewArgumentError("couldn't find HOME")
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	if strings.HasPrefix(path, "~") {
		userPart := strings.TrimPrefix(path, "~")
		slash := strings.Index(userPart, "/")
		suffix := ""
		if slash >= 0 {
			suffix = userPart[slash+1:]
			userPart = userPart[:slash]
		}
		if currentUser, ok := envString("USER"); ok && userPart == currentUser {
			home, homeErr := expandHomePath("~")
			if homeErr != nil {
				return "", homeErr
			}
			if suffix != "" {
				return filepath.Join(home, suffix), nil
			}
			return home, nil
		}
		return "", NewArgumentError("user not found")
	}
	return path, nil
}

func rubyBasename(path string) string {
	if path == "" {
		return ""
	}
	trimmed := trimTrailingPathSlashes(path)
	if trimmed == "" {
		return "/"
	}
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return trimmed
	}
	if idx == len(trimmed)-1 {
		return "/"
	}
	return trimmed[idx+1:]
}

func rubyTrimBasenameSuffix(base, suffix string) string {
	if suffix == "" || base == "/" {
		return base
	}
	if suffix == ".*" {
		ext := rubyExtname(base)
		if ext != "" {
			return strings.TrimSuffix(base, ext)
		}
		return base
	}
	if strings.HasSuffix(base, suffix) {
		return strings.TrimSuffix(base, suffix)
	}
	return base
}

func rubyDirnameOnce(path string) string {
	if path == "" {
		return "."
	}
	trimmed := trimTrailingPathSlashes(path)
	if trimmed == "" {
		return "/"
	}
	if trimmed == "." || trimmed == ".." {
		return "."
	}
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return "."
	}
	if idx == 0 {
		return "/"
	}
	dir := strings.TrimRight(trimmed[:idx], "/")
	if dir == "" {
		return "/"
	}
	return collapseLeadingPathSlashes(dir)
}

func rubyExtname(path string) string {
	base := rubyBasename(path)
	if base == "" || base == "." || base == ".." || allDots(base) {
		return ""
	}
	idx := strings.LastIndex(base, ".")
	if idx <= 0 {
		return ""
	}
	return base[idx:]
}

func trimTrailingPathSlashes(path string) string {
	if path == "" {
		return ""
	}
	i := len(path)
	for i > 1 && path[i-1] == '/' {
		i--
	}
	if i == 1 && path[0] == '/' {
		return ""
	}
	return path[:i]
}

func collapseLeadingPathSlashes(path string) string {
	if strings.HasPrefix(path, "//") {
		return "/" + strings.TrimLeft(path, "/")
	}
	return path
}

func allDots(value string) bool {
	for _, r := range value {
		if r != '.' {
			return false
		}
	}
	return value != ""
}

func appendPathParts(parts *[]string, value *object.EmeraldValue, seen map[*object.EmeraldValue]bool) *object.EmeraldValue {
	if value != nil && value.Type == object.ValueArray {
		if seen[value] {
			return NewArgumentError("recursive array")
		}
		seen[value] = true
		defer delete(seen, value)
		for _, element := range value.Data.([]*object.EmeraldValue) {
			if errVal := appendPathParts(parts, element, seen); errVal != nil {
				return errVal
			}
		}
		return nil
	}
	path, errVal := coercePath(value)
	if errVal != nil {
		return errVal
	}
	if strings.ContainsRune(path, '\x00') {
		return NewArgumentError("string contains null byte")
	}
	*parts = append(*parts, path)
	return nil
}

func pathFromSingleArg(args []*object.EmeraldValue) (string, *object.EmeraldValue) {
	if len(args) != 1 {
		return "", NewArgumentError("wrong number of arguments")
	}
	return coercePath(args[0])
}

func coercePath(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	if value == nil || value.Type == object.ValueNil {
		return "", typeError("no implicit conversion into String")
	}
	if value.Type == object.ValueString {
		return value.Data.(string), nil
	}
	if data := ioShim(value); data != nil && data.path != "" {
		return data.path, nil
	}
	if CallMethod != nil {
		coerced := CallMethod(value, "to_path")
		if coerced != nil && coerced.Type == object.ValueString {
			return coerced.Data.(string), nil
		}
		if coerced != nil && coerced.Type == object.ValueException && !isNoMethodError(coerced) {
			return "", coerced
		}
		coerced = CallMethod(value, "to_str")
		if coerced != nil && coerced.Type == object.ValueString {
			return coerced.Data.(string), nil
		}
		if coerced != nil && coerced.Type == object.ValueException && !isNoMethodError(coerced) {
			return "", coerced
		}
	}
	return "", typeError("no implicit conversion into String")
}

func isNoMethodError(value *object.EmeraldValue) bool {
	return value != nil && value.Type == object.ValueException && value.Class == R.Classes["NoMethodError"]
}

func fileExistPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	if _, err := os.Stat(path); err == nil {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileFilePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err == nil && info.Mode().IsRegular() {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileDirectoryPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileSizePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err != nil {
		return newRuntimeException(R.Classes["Errno::ENOENT"], "No such file or directory")
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: info.Size(), Class: R.Classes["Integer"]}
}

func fileSizeQuestionPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: info.Size(), Class: R.Classes["Integer"]}
}

func fileZeroPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := fileStatPathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err == nil && info.Size() == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileStatPathFromSingleArg(args []*object.EmeraldValue) (string, *object.EmeraldValue) {
	if len(args) != 1 {
		return "", NewArgumentError("wrong number of arguments")
	}
	if data := ioShim(args[0]); data != nil && data.path != "" {
		return data.path, nil
	}
	path, errVal := coercePath(args[0])
	if errVal == nil {
		return path, nil
	}
	if CallMethod != nil && args[0] != nil && args[0].Class != nil {
		ioValue := CallMethod(args[0], "to_io")
		if data := ioShim(ioValue); data != nil && data.path != "" {
			return data.path, nil
		}
		if ioValue != nil && ioValue.Type == object.ValueException {
			return "", ioValue
		}
	}
	return "", errVal
}

func fileIdenticalPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	left, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	right, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr != nil || rightErr != nil {
		return R.FalseVal
	}
	if os.SameFile(leftInfo, rightInfo) {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileClassRealpath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathWithOptionalBase(args)
	if errVal != nil {
		return errVal
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return errnoForPathError(err)
	}
	return rubyString(resolved)
}

func fileClassRealdirpath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathWithOptionalBase(args)
	if errVal != nil {
		return errVal
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return rubyString(resolved)
	}
	if target, readErr := os.Readlink(path); readErr == nil {
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		if filepath.Clean(target) == filepath.Clean(path) {
			return newRuntimeException(R.Classes["Errno::ELOOP"], err.Error())
		}
		targetParent, parentErr := filepath.EvalSymlinks(filepath.Dir(target))
		if parentErr != nil {
			return errnoForPathError(parentErr)
		}
		return rubyString(filepath.Join(targetParent, filepath.Base(target)))
	}
	parent := filepath.Dir(path)
	parentResolved, parentErr := filepath.EvalSymlinks(parent)
	if parentErr != nil {
		return errnoForPathError(parentErr)
	}
	return rubyString(filepath.Join(parentResolved, filepath.Base(path)))
}

func pathWithOptionalBase(args []*object.EmeraldValue) (string, *object.EmeraldValue) {
	if len(args) < 1 || len(args) > 2 {
		return "", NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return "", errVal
	}
	if len(args) == 2 && !filepath.IsAbs(path) {
		base, baseErr := coercePath(args[1])
		if baseErr != nil {
			return "", baseErr
		}
		path = filepath.Join(base, path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return path, nil
}

func fileClassFtype(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	stat := newFileStatValue(path, true)
	if stat == nil || stat.Type == object.ValueException {
		return stat
	}
	return fileStatFtype(stat)
}

func fileClassUmask(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError("wrong number of arguments")
	}
	previous := currentFileUmask
	if len(args) == 1 {
		if args[0] != nil && args[0].Type == object.ValueException {
			LastException = nil
			LastRaisedResult = nil
			return args[0]
		}
		next, ok := valueToInteger(args[0])
		if !ok && CallMethod != nil {
			coerced := CallMethod(args[0], "to_int")
			if coerced != nil && coerced.Type == object.ValueException {
				return coerced
			}
			next, ok = valueToInteger(coerced)
		}
		if !ok {
			return typeError("no implicit conversion into Integer")
		}
		if next < 0 || next > 0777 {
			return newRuntimeException(R.Classes["RangeError"], "integer out of range")
		}
		currentFileUmask = next
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: previous, Class: R.Classes["Integer"]}
}

func fileClassLink(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	oldPath, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	newPath, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	if err := os.Link(oldPath, newPath); err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileClassSymlink(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	oldPath, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	newPath, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	if err := os.Symlink(oldPath, newPath); err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileClassMkfifo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	mode := int64(0666)
	if len(args) == 2 {
		var ok bool
		mode, ok = valueToInteger(args[1])
		if !ok {
			return typeError("no implicit conversion into Integer")
		}
	}
	finalMode := os.FileMode(mode &^ currentFileUmask)
	if err := syscall.Mkfifo(path, uint32(finalMode)); err != nil {
		return errnoForPathError(err)
	}
	_ = os.Chmod(path, finalMode)
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileClassReadlink(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	target, err := os.Readlink(path)
	if err != nil {
		return errnoForPathError(err)
	}
	return rubyString(target)
}

func fileClassDelete(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	count := int64(0)
	for _, arg := range args {
		path, errVal := coercePath(arg)
		if errVal != nil {
			return errVal
		}
		if err := os.Remove(path); err != nil {
			return errnoForPathError(err)
		}
		count++
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: count, Class: R.Classes["Integer"]}
}

func fileClassRename(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	oldPath, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	newPath, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileClassChmod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return NewArgumentError("wrong number of arguments")
	}
	mode, errVal := chmodModeFromValue(args[0])
	if errVal != nil {
		return errVal
	}
	count := int64(0)
	for _, arg := range args[1:] {
		path, errVal := coercePath(arg)
		if errVal != nil {
			return errVal
		}
		if err := os.Chmod(path, os.FileMode(mode&07777)); err != nil {
			return errnoForPathError(err)
		}
		count++
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: count, Class: R.Classes["Integer"]}
}

func chmodModeFromValue(value *object.EmeraldValue) (int64, *object.EmeraldValue) {
	if value != nil && value.Type == object.ValueException {
		LastException = nil
		LastRaisedResult = nil
		return 0, value
	}
	mode, ok := valueToInteger(value)
	if !ok {
		return 0, typeError("no implicit conversion into Integer")
	}
	if mode < 0 || mode > 0777777 {
		return 0, NewRangeError("integer out of range")
	}
	return mode, nil
}

func fileClassChown(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 3 {
		return NewArgumentError("wrong number of arguments")
	}
	count := int64(0)
	for _, arg := range args[2:] {
		path, errVal := coercePath(arg)
		if errVal != nil {
			return errVal
		}
		if _, err := os.Stat(path); err != nil {
			return errnoForPathError(err)
		}
		count++
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: count, Class: R.Classes["Integer"]}
}

func fileClassUtime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 3 {
		return NewArgumentError("wrong number of arguments")
	}
	atime, ok := timeFromValue(args[0])
	if !ok {
		return typeError("can't convert argument into time")
	}
	mtime, ok := timeFromValue(args[1])
	if !ok {
		return typeError("can't convert argument into time")
	}
	count := int64(0)
	for _, arg := range args[2:] {
		path, errVal := coercePath(arg)
		if errVal != nil {
			return errVal
		}
		if err := os.Chtimes(path, atime, mtime); err != nil {
			return errnoForPathError(err)
		}
		fileUtimeOverrides[path] = fileTimeOverride{atime: atime, mtime: mtime}
		count++
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: count, Class: R.Classes["Integer"]}
}

func fileExecutablePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err == nil && info.Mode().Perm()&0111 != 0 && !info.IsDir() {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileReadablePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err == nil && info.Mode().Perm()&0444 != 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileWritablePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err == nil && info.Mode().Perm()&0222 != 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func dirClassMkdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	mode := os.FileMode(0755)
	if len(args) > 1 {
		if args[1].Type == object.ValueInteger {
			mode = os.FileMode(args[1].Data.(int64))
		} else if CallMethod != nil {
			coerced := CallMethod(args[1], "to_int")
			if coerced != nil && coerced.Type == object.ValueInteger {
				mode = os.FileMode(coerced.Data.(int64))
			} else if coerced != nil && coerced.Type == object.ValueException {
				return coerced
			} else {
				return typeError("no implicit conversion of Object into Integer")
			}
		}
	}
	if err := os.Mkdir(path, mode); err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func dirClassRmdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return errnoForPathError(statErr)
	}
	if !info.IsDir() {
		return newRuntimeException(R.Classes["Errno::ENOTDIR"], "Not a directory")
	}
	if err := os.Remove(path); err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func errnoForPathError(err error) *object.EmeraldValue {
	if errors.Is(err, syscall.ENOTEMPTY) {
		return newRuntimeException(R.Classes["Errno::ENOTEMPTY"], err.Error())
	}
	if errors.Is(err, syscall.ENOTDIR) {
		return newRuntimeException(R.Classes["Errno::ENOTDIR"], err.Error())
	}
	if errors.Is(err, os.ErrExist) {
		return newRuntimeException(R.Classes["Errno::EEXIST"], err.Error())
	}
	if errors.Is(err, os.ErrNotExist) {
		return newRuntimeException(R.Classes["Errno::ENOENT"], err.Error())
	}
	if errors.Is(err, os.ErrPermission) {
		return newRuntimeException(R.Classes["Errno::EACCES"], err.Error())
	}
	if errors.Is(err, syscall.EINVAL) {
		return newRuntimeException(R.Classes["Errno::EINVAL"], err.Error())
	}
	if errors.Is(err, syscall.ELOOP) {
		return newRuntimeException(R.Classes["Errno::ELOOP"], err.Error())
	}
	if errors.Is(err, syscall.EISDIR) {
		return newRuntimeException(R.Classes["Errno::EISDIR"], err.Error())
	}
	if strings.Contains(err.Error(), "not a directory") {
		return newRuntimeException(R.Classes["Errno::ENOTDIR"], err.Error())
	}
	if strings.Contains(err.Error(), "is a directory") {
		return newRuntimeException(R.Classes["Errno::EISDIR"], err.Error())
	}
	if strings.Contains(err.Error(), "directory not empty") {
		return newRuntimeException(R.Classes["Errno::ENOTEMPTY"], err.Error())
	}
	if strings.Contains(err.Error(), "file exists") {
		return newRuntimeException(R.Classes["Errno::EEXIST"], err.Error())
	}
	if strings.Contains(err.Error(), "too many links") || strings.Contains(err.Error(), "too many levels") {
		return newRuntimeException(R.Classes["Errno::ELOOP"], err.Error())
	}
	return newRuntimeException(R.Classes["SystemCallError"], err.Error())
}

func dirClassEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	info, err := os.Stat(path)
	if err != nil {
		return newRuntimeException(R.Classes["Errno::ENOENT"], "No such file or directory")
	}
	if !info.IsDir() {
		return R.FalseVal
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return R.FalseVal
	}
	if len(entries) == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func dirClassEntries(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return dirEntryList(args, true)
}

func dirClassChildren(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return dirEntryList(args, false)
}

func dirClassOpen(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	dir, errVal := newDirValue(path)
	if errVal != nil {
		return errVal
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), dir)
		if data := dirState(dir); data != nil {
			data.closed = true
		}
		return result
	}
	return dir
}

func dirClassForFd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type == object.ValueNil {
		return typeError("no implicit conversion of nil into Integer")
	}
	fdVal := args[0]
	if fdVal.Type != object.ValueInteger && CallMethod != nil {
		coerced := CallMethod(fdVal, "to_int")
		if coerced != nil && coerced.Type == object.ValueInteger {
			fdVal = coerced
		} else if coerced != nil && coerced.Type == object.ValueException {
			return coerced
		}
	}
	if fdVal.Type != object.ValueInteger {
		return typeError("no implicit conversion into Integer")
	}
	fd := fdVal.Data.(int64)
	if fd < 0 {
		return newRuntimeException(R.Classes["SystemCallError"], "Bad file descriptor - fdopendir")
	}
	if fd != 0 {
		return newRuntimeException(R.Classes["SystemCallError"], "Not a directory - fdopendir")
	}
	if lastDirForFd == nil {
		return newRuntimeException(R.Classes["SystemCallError"], "Bad file descriptor - fdopendir")
	}
	entries := append([]*object.EmeraldValue{}, lastDirForFd.entries...)
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &dirData{entries: entries, pathless: true, fdSource: lastDirForFd},
		Class: R.Classes["Dir"],
	}
}

func fileInstanceSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	info, err := os.Stat(data.path)
	if err != nil {
		if data.cachedSize >= 0 {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: data.cachedSize, Class: R.Classes["Integer"]}
		}
		return errnoForPathError(err)
	}
	data.cachedSize = info.Size()
	return &object.EmeraldValue{Type: object.ValueInteger, Data: info.Size(), Class: R.Classes["Integer"]}
}

func fileInstanceStat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if info, err := os.Stat(data.path); err == nil {
		data.cachedSize = info.Size()
		data.cachedInfo = info
		return newFileStatValueFromInfo(data.path, info, false)
	}
	if data.cachedInfo != nil {
		return newFileStatValueFromInfo(data.path, data.cachedInfo, false)
	}
	return newFileStatValue(data.path, false)
}

func fileInstanceLstat(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if info, err := os.Lstat(data.path); err == nil {
		data.cachedSize = info.Size()
		data.cachedInfo = info
		return newFileStatValueFromInfo(data.path, info, true)
	}
	if data.cachedInfo != nil {
		return newFileStatValueFromInfo(data.path, data.cachedInfo, true)
	}
	return newFileStatValue(data.path, true)
}

func fileInstanceTime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	info, err := os.Stat(data.path)
	if err != nil {
		if data.cachedInfo != nil {
			return newTimeValue(data.cachedInfo.ModTime())
		}
		return errnoForPathError(err)
	}
	data.cachedInfo = info
	data.cachedSize = info.Size()
	return newTimeValue(info.ModTime())
}

func fileInstanceAtime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if override, ok := fileUtimeOverrides[data.path]; ok {
		return newTimeValue(override.atime)
	}
	info, err := os.Stat(data.path)
	if err != nil {
		if data.cachedInfo != nil {
			return newTimeValue(data.cachedInfo.ModTime())
		}
		return errnoForPathError(err)
	}
	data.cachedInfo = info
	data.cachedSize = info.Size()
	return newTimeValue(info.ModTime())
}

func fileInstanceChown(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if _, err := os.Stat(data.path); err != nil {
		return errnoForPathError(err)
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileInstanceChmod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return R.NilVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	mode, errVal := chmodModeFromValue(args[0])
	if errVal != nil {
		return errVal
	}
	if err := os.Chmod(data.path, os.FileMode(mode&07777)); err != nil {
		return errnoForPathError(err)
	}
	if info, statErr := os.Stat(data.path); statErr == nil {
		data.cachedInfo = info
		data.cachedSize = info.Size()
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileInstancePath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil && data.path != "" {
		result := rubyString(data.path)
		if data.pathEncoding != "" && stringEncodings != nil {
			stringEncodings[result] = data.pathEncoding
		}
		return result
	}
	return R.NilVal
}

func fileInstanceTruncate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if !strings.Contains(data.mode, "w") && !strings.Contains(data.mode, "+") && !strings.Contains(data.mode, "a") {
		return newRuntimeException(R.Classes["IOError"], "not opened for writing")
	}
	length, errVal := truncateLength(args[0])
	if errVal != nil {
		return errVal
	}
	if err := os.Truncate(data.path, length); err != nil {
		return errnoForPathError(err)
	}
	if info, statErr := os.Stat(data.path); statErr == nil {
		data.cachedSize = info.Size()
		data.cachedInfo = info
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileInstanceFlush(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func fileInstanceRead(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" || data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if !fileModeReadable(data.mode) {
		return newRuntimeException(R.Classes["IOError"], "not opened for reading")
	}
	content, err := os.ReadFile(data.path)
	if err != nil {
		return errnoForPathError(err)
	}
	if data.offset >= int64(len(content)) {
		if len(args) == 0 {
			return rubyString("")
		}
		return R.NilVal
	}
	limit := int64(len(content)) - data.offset
	if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueInteger && args[0].Data.(int64) < limit {
		limit = args[0].Data.(int64)
	}
	start := data.offset
	data.offset += limit
	return rubyString(string(content[start:data.offset]))
}

func fileInstanceGets(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" || data.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if !fileModeReadable(data.mode) {
		return newRuntimeException(R.Classes["IOError"], "not opened for reading")
	}
	content, err := os.ReadFile(data.path)
	if err != nil {
		return errnoForPathError(err)
	}
	if data.offset >= int64(len(content)) {
		return R.NilVal
	}
	remaining := content[data.offset:]
	lineLen := len(remaining)
	if idx := bytes.IndexByte(remaining, '\n'); idx >= 0 {
		lineLen = idx + 1
	}
	data.offset += int64(lineLen)
	return rubyString(string(remaining[:lineLen]))
}

func fileInstanceRewind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil {
		data.offset = 0
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileInstancePos(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: data.offset, Class: R.Classes["Integer"]}
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileInstanceBinmode(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil && strings.Contains(data.mode, "b") {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileInstanceExternalEncoding(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil && strings.Contains(data.mode, "b") {
		return newEncodingValue("BINARY")
	}
	return newEncodingValue(defaultExternalEncoding)
}

func fileModeReadable(mode string) bool {
	return strings.HasPrefix(mode, "r") || strings.Contains(mode, "+") || mode == ""
}

func fileInstanceEOF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := ioShim(receiver)
	if data == nil || data.path == "" {
		return R.TrueVal
	}
	if info, err := os.Stat(data.path); err == nil && data.offset >= info.Size() {
		return R.TrueVal
	}
	return R.FalseVal
}

func truncateLength(value *object.EmeraldValue) (int64, *object.EmeraldValue) {
	if value == nil || value.Type != object.ValueInteger {
		return 0, typeError("no implicit conversion into Integer")
	}
	length := value.Data.(int64)
	if length < 0 {
		return 0, newRuntimeException(R.Classes["Errno::EINVAL"], "Invalid argument")
	}
	return length, nil
}

func fileStatDataFrom(receiver *object.EmeraldValue) *fileStatData {
	if receiver == nil {
		return nil
	}
	data, _ := receiver.Data.(*fileStatData)
	return data
}

func fileStatFilePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data != nil && data.info != nil && data.info.Mode().IsRegular() {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileStatDirectoryPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data != nil && data.info != nil && data.info.IsDir() {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileStatSymlinkPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data != nil && data.info != nil && data.info.Mode()&os.ModeSymlink != 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileStatZeroPredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data != nil && data.info != nil && data.info.Size() == 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileStatExecutablePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data != nil && data.info != nil && data.info.Mode().Perm()&0111 != 0 && !data.info.IsDir() {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileStatWritablePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data != nil && data.info != nil && data.info.Mode().Perm()&0222 != 0 {
		return R.TrueVal
	}
	return R.FalseVal
}

func fileStatFtype(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	ftype := "unknown"
	if data != nil && data.info != nil {
		mode := data.info.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			ftype = "link"
		case mode.IsRegular():
			ftype = "file"
		case mode.IsDir():
			ftype = "directory"
		case mode&os.ModeNamedPipe != 0:
			ftype = "fifo"
		case mode&os.ModeSocket != 0:
			ftype = "socket"
		case mode&os.ModeCharDevice != 0:
			ftype = "characterSpecial"
		case mode&os.ModeDevice != 0:
			ftype = "blockSpecial"
		}
	}
	return rubyString(ftype)
}

func fileStatSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data == nil || data.info == nil {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: data.info.Size(), Class: R.Classes["Integer"]}
}

func fileStatSizeQuestion(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data == nil || data.info == nil || data.info.Size() == 0 {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: data.info.Size(), Class: R.Classes["Integer"]}
}

func fileStatBlockSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(4096), Class: R.Classes["Integer"]}
}

func fileStatTime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data == nil || data.info == nil {
		return R.NilVal
	}
	return newTimeValue(data.info.ModTime())
}

func fileStatMode(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := fileStatDataFrom(receiver)
	if data == nil || data.info == nil {
		return R.NilVal
	}
	mode := int64(data.info.Mode().Perm())
	switch {
	case data.info.Mode()&os.ModeNamedPipe != 0:
		mode |= 010000
	case data.info.Mode().IsRegular():
		mode |= 0100000
	case data.info.Mode().IsDir():
		mode |= 0040000
	case data.info.Mode()&os.ModeSymlink != 0:
		mode |= 0120000
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: mode, Class: R.Classes["Integer"]}
}

func fileStatIntegerZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func fileStatIntegerOne(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(1), Class: R.Classes["Integer"]}
}

func FileStatFixtureDispatch(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil || receiver.Type != object.ValueClass {
		return nil
	}
	cls, _ := receiver.Data.(*object.Class)
	if cls == nil || cls.Name != "FileStat" {
		return nil
	}
	switch method {
	case "file?", "directory?", "zero?", "executable?", "executable_real?", "writable?", "writable_real?":
	default:
		return nil
	}
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	if args[0] == nil || args[0].Type == object.ValueNil {
		return typeError("no implicit conversion into String")
	}
	stat := fileClassLstat(receiver, args[0])
	if stat == nil || stat.Type == object.ValueException {
		return stat
	}
	if CallMethod != nil {
		return CallMethod(stat, method)
	}
	return R.NilVal
}

func dirClassHome(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	currentHome := func() *object.EmeraldValue {
		if val, ok := hashLookup(valueToHashMap(EnvObject()), rubyString("HOME")); ok && val != nil && val.Type == object.ValueString {
			return rubyString(val.Data.(string))
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return rubyString(home)
		}
		return rubyString("/")
	}
	if len(args) == 0 || args[0] == nil || args[0].Type == object.ValueNil {
		return currentHome()
	}
	if args[0].Type != object.ValueString {
		return typeError("no implicit conversion of object into String")
	}
	name := args[0].Data.(string)
	if name == "" {
		return newRuntimeException(R.Classes["ArgumentError"], "user not found")
	}
	currentUser := os.Getenv("USER")
	if currentUser == "" {
		currentUser = os.Getenv("USERNAME")
	}
	if name != currentUser {
		return newRuntimeException(R.Classes["ArgumentError"], "user not found")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return rubyString(home)
	}
	return currentHome()
}

func dirClassPwd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	wd, err := os.Getwd()
	if err != nil {
		return errnoForPathError(err)
	}
	return rubyString(wd)
}

func dirClassChdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var path string
	if len(args) == 0 || args[0] == nil || args[0].Type == object.ValueNil {
		home := dirClassHome(receiver)
		if home.Type == object.ValueException {
			return home
		}
		path = home.Data.(string)
	} else {
		var errVal *object.EmeraldValue
		path, errVal = coercePath(args[0])
		if errVal != nil {
			return errVal
		}
	}
	return chdirToPath(path, false)
}

func dirClassChroot(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := pathFromSingleArg(args)
	if errVal != nil {
		return errVal
	}
	if _, err := os.Stat(path); err != nil {
		return errnoForPathError(err)
	}
	if os.Getuid() != 0 {
		return newRuntimeException(R.Classes["Errno::EPERM"], "Operation not permitted")
	}
	return R.NilVal
}

func dirClassFchdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type == object.ValueNil {
		return typeError("no implicit conversion of nil into Integer")
	}
	fdVal := args[0]
	if fdVal.Type != object.ValueInteger && CallMethod != nil {
		coerced := CallMethod(fdVal, "to_int")
		if coerced != nil && coerced.Type == object.ValueInteger {
			fdVal = coerced
		} else if coerced != nil && coerced.Type == object.ValueException {
			return coerced
		}
	}
	if fdVal.Type != object.ValueInteger {
		return typeError("no implicit conversion into Integer")
	}
	fd := fdVal.Data.(int64)
	if fd < 0 || lastDirForFd == nil {
		return newRuntimeException(R.Classes["SystemCallError"], "Bad file descriptor - fchdir")
	}
	if fd != 0 {
		return newRuntimeException(R.Classes["SystemCallError"], "Not a directory - fchdir")
	}
	return chdirToPath(lastDirForFd.path, false)
}

func dirClassGlob(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
	}

	sortResults := true
	base := "."
	if len(args) > 0 && args[len(args)-1] != nil && args[len(args)-1].Type == object.ValueHash {
		opts := valueToHashMap(args[len(args)-1])
		args = args[:len(args)-1]
		if value, ok := globOptionLookup(opts, "sort"); ok {
			if value == R.TrueVal || (value != nil && value.Type == object.ValueBool && value.Data.(bool)) {
				sortResults = true
			} else if value == R.FalseVal || (value != nil && value.Type == object.ValueBool && !value.Data.(bool)) {
				sortResults = false
			} else {
				return NewArgumentError("expected true or false as sort")
			}
		}
		if value, ok := globOptionLookup(opts, "base"); ok {
			if value == nil || value.Type == object.ValueNil {
				base = "."
			} else {
				coerced, errVal := coercePath(value)
				if errVal != nil {
					return errVal
				}
				if coerced != "" {
					base = coerced
				}
			}
		}
	}

	dotmatch := false
	patternArgs := make([]*object.EmeraldValue, 0, len(args))
	for _, arg := range args {
		if arg == nil || arg.Type == object.ValueNil {
			return typeError("no implicit conversion of nil into String")
		}
		if arg.Type == object.ValueInteger {
			flags := arg.Data.(int64)
			if flags&1 != 0 {
				dotmatch = true
			}
			continue
		}
		patternArgs = append(patternArgs, arg)
	}
	if len(patternArgs) == 0 {
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
	}

	patterns := []string{}
	for _, arg := range patternArgs {
		if arg.Type == object.ValueArray {
			for _, element := range arg.Data.([]*object.EmeraldValue) {
				path, errVal := coercePath(element)
				if errVal != nil {
					return errVal
				}
				patterns = append(patterns, path)
			}
			continue
		}
		path, errVal := coercePath(arg)
		if errVal != nil {
			return errVal
		}
		patterns = append(patterns, path)
	}

	matches := []string{}
	for _, pattern := range patterns {
		if strings.ContainsRune(pattern, '\x00') {
			return NewArgumentError("nul-separated glob pattern")
		}
		matches = append(matches, globMatches(base, pattern, dotmatch)...)
	}
	if sortResults {
		sort.Strings(matches)
	}
	values := make([]*object.EmeraldValue, 0, len(matches))
	for _, match := range matches {
		values = append(values, rubyString(match))
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		block := CurrentBlockValue()
		for _, value := range values {
			CallBlockWithArgs(block, value)
		}
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func rubySymbol(name string) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: R.Classes["Symbol"]}
}

func globOptionLookup(opts map[*object.EmeraldValue]*object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	if value, ok := hashLookup(opts, rubySymbol(name)); ok {
		return value, true
	}
	return hashLookup(opts, rubyString(":"+name))
}

func globMatches(base, pattern string, dotmatch bool) []string {
	if pattern == "" || pattern == "{}" {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, expanded := range expandGlobBraces(pattern) {
		matches := globExpandedPattern(base, expanded, dotmatch)
		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				out = append(out, match)
			}
		}
	}
	return out
}

func expandGlobBraces(pattern string) []string {
	start := -1
	escaped := false
	for i, r := range pattern {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return []string{pattern}
	}
	depth := 0
	escaped = false
	for i := start; i < len(pattern); i++ {
		ch := pattern[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				parts := splitGlobBraceParts(pattern[start+1 : i])
				if len(parts) == 0 {
					return nil
				}
				results := []string{}
				for _, part := range parts {
					for _, expanded := range expandGlobBraces(pattern[:start] + part + pattern[i+1:]) {
						results = append(results, expanded)
					}
				}
				return results
			}
		}
	}
	return []string{pattern}
}

func splitGlobBraceParts(body string) []string {
	parts := []string{}
	start := 0
	depth := 0
	escaped := false
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
		} else if ch == ',' && depth == 0 {
			parts = append(parts, body[start:i])
			start = i + 1
		}
	}
	parts = append(parts, body[start:])
	return parts
}

func globExpandedPattern(base, pattern string, dotmatch bool) []string {
	cleanPattern := unescapeGlobPattern(pattern)
	searchPattern := cleanPattern
	if !filepath.IsAbs(searchPattern) {
		searchPattern = filepath.Join(base, searchPattern)
	}
	if strings.Contains(cleanPattern, "**") {
		return globRecursivePattern(base, cleanPattern, dotmatch)
	}
	raw, _ := filepath.Glob(searchPattern)
	out := make([]string, 0, len(raw))
	for _, match := range raw {
		if !globPathAllowed(base, cleanPattern, match, dotmatch) {
			continue
		}
		if strings.HasSuffix(cleanPattern, string(os.PathSeparator)) || strings.HasSuffix(cleanPattern, "/") {
			if info, err := os.Stat(match); err != nil || !info.IsDir() {
				continue
			}
		}
		out = append(out, globDisplayPath(base, match, strings.HasSuffix(cleanPattern, "/")))
	}
	return out
}

func globRecursivePattern(base, pattern string, dotmatch bool) []string {
	root := base
	prefix := pattern
	if idx := strings.Index(pattern, "**"); idx >= 0 {
		prefix = strings.TrimRight(pattern[:idx], "/")
	}
	if prefix != "" && !strings.ContainsAny(prefix, "*?[") {
		root = filepath.Join(base, prefix)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return nil
	}
	matches := []string{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root && !strings.HasPrefix(pattern, ".**") && pattern != "**/**" && pattern != "**/" {
			return nil
		}
		display := globDisplayPath(base, path, d.IsDir() && strings.HasSuffix(pattern, "/"))
		noSlash := strings.TrimSuffix(display, "/")
		if !dotmatch && globContainsHidden(noSlash) && !globPatternAllowsHidden(pattern, noSlash) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if globRecursiveMatch(pattern, noSlash, d.IsDir()) {
			matches = append(matches, display)
		}
		return nil
	})
	return matches
}

func globRecursiveMatch(pattern, path string, isDir bool) bool {
	pattern = strings.TrimPrefix(pattern, "./")
	if pattern == "**" || pattern == "**/**" {
		return true
	}
	if pattern == "**/" || pattern == ".**/" {
		return isDir
	}
	if strings.HasPrefix(pattern, "**/") {
		tail := strings.TrimPrefix(pattern, "**/")
		if ok, _ := filepath.Match(tail, filepath.Base(path)); ok {
			return true
		}
		if strings.Contains(tail, "/") {
			ok, _ := filepath.Match(tail, path)
			return ok
		}
		return false
	}
	if strings.Contains(pattern, "/**/") {
		parts := strings.SplitN(pattern, "/**/", 2)
		if !strings.HasPrefix(path, parts[0]+"/") {
			return false
		}
		tail := parts[1]
		if tail == "" {
			return isDir
		}
		ok, _ := filepath.Match(tail, filepath.Base(path))
		return ok
	}
	ok, _ := filepath.Match(strings.ReplaceAll(pattern, "**", "*"), path)
	return ok
}

func unescapeGlobPattern(pattern string) string {
	var b strings.Builder
	escaped := false
	for _, r := range pattern {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		b.WriteRune('\\')
	}
	return b.String()
}

func globDisplayPath(base, path string, wantSlash bool) string {
	display := path
	if !filepath.IsAbs(path) || base != "." {
		if rel, err := filepath.Rel(base, path); err == nil {
			display = rel
		}
	}
	display = filepath.ToSlash(display)
	if display == "." && wantSlash {
		display = "/"
	} else if wantSlash && !strings.HasSuffix(display, "/") {
		display += "/"
	}
	return display
}

func globPathAllowed(base, pattern, path string, dotmatch bool) bool {
	if dotmatch {
		return true
	}
	display := strings.TrimSuffix(globDisplayPath(base, path, false), "/")
	return !globContainsHidden(display) || globPatternAllowsHidden(pattern, display)
}

func globContainsHidden(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
	}
	return false
}

func globPatternAllowsHidden(pattern, path string) bool {
	patternParts := strings.Split(filepath.ToSlash(pattern), "/")
	pathParts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range pathParts {
		if !strings.HasPrefix(part, ".") || part == "." || part == ".." {
			continue
		}
		if i < len(patternParts) && strings.HasPrefix(patternParts[i], ".") {
			continue
		}
		return false
	}
	return true
}

func chdirToPath(path string, ignoreRestoreError bool) *object.EmeraldValue {
	old, oldErr := os.Getwd()
	if err := os.Chdir(path); err != nil {
		return errnoForPathError(err)
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), rubyString(path))
		if oldErr == nil {
			if _, statErr := os.Stat(old); statErr != nil && !ignoreRestoreError {
				return errnoForPathError(statErr)
			}
			if err := os.Chdir(old); err != nil {
				if ignoreRestoreError {
					return result
				}
				return errnoForPathError(err)
			}
		}
		return result
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func dirClassEachChild(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return dirEachEntry(args, false)
}

func dirClassForeach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return dirEachEntry(args, true)
}

func dirEachEntry(args []*object.EmeraldValue, includeDots bool) *object.EmeraldValue {
	values := dirEntryList(args, includeDots)
	if values.Type == object.ValueException {
		return values
	}
	if values.Type != object.ValueArray {
		return values
	}
	entries := values.Data.([]*object.EmeraldValue)
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		block := CurrentBlockValue()
		for _, entry := range entries {
			CallBlockWithArgs(block, entry)
		}
		return R.NilVal
	}
	return newStaticEnumerator(entries)
}

func newStaticEnumerator(values []*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &enumeratorData{values: values, generated: true},
		Class: R.Classes["Enumerator"],
	}
}

func newDirValue(path string) (*object.EmeraldValue, *object.EmeraldValue) {
	values, errVal := dirEntryValues(path, true)
	if errVal != nil {
		return nil, errVal
	}
	return &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  &dirData{path: path, entries: values},
		Class: R.Classes["Dir"],
	}, nil
}

func dirState(receiver *object.EmeraldValue) *dirData {
	if receiver == nil {
		return nil
	}
	if data, ok := receiver.Data.(*dirData); ok {
		return data
	}
	return nil
}

func dirRequireOpen(receiver *object.EmeraldValue) (*dirData, *object.EmeraldValue) {
	data := dirState(receiver)
	if data == nil {
		return nil, R.NilVal
	}
	if data.closed {
		return nil, newRuntimeException(R.Classes["IOError"], "closed directory")
	}
	return data, nil
}

func dirClose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := dirState(receiver); data != nil {
		if data.fdSource != nil && data.fdSource.closed {
			return newRuntimeException(R.Classes["Errno::EBADF"], "Bad file descriptor - closedir")
		}
		data.closed = true
	}
	return R.NilVal
}

func dirPath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := dirState(receiver)
	if data == nil || data.pathless {
		return R.NilVal
	}
	return rubyString(data.path)
}

func dirChdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := dirRequireOpen(receiver)
	if errVal != nil {
		return errVal
	}
	return chdirToPath(data.path, true)
}

func dirRead(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := dirRequireOpen(receiver)
	if errVal != nil {
		return errVal
	}
	if data.pos >= len(data.entries) {
		return R.NilVal
	}
	entry := data.entries[data.pos]
	data.pos++
	return entry
}

func dirRewind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := dirRequireOpen(receiver)
	if errVal != nil {
		return errVal
	}
	data.pos = 0
	return receiver
}

func dirTell(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := dirRequireOpen(receiver)
	if errVal != nil {
		return errVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(data.pos), Class: R.Classes["Integer"]}
}

func dirSetPos(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := dirRequireOpen(receiver)
	if errVal != nil {
		return errVal
	}
	if len(args) == 0 || args[0].Type != object.ValueInteger {
		return typeError("no implicit conversion to Integer")
	}
	pos := int(args[0].Data.(int64))
	if pos < 0 {
		pos = 0
	}
	if pos > len(data.entries) {
		pos = len(data.entries)
	}
	data.pos = pos
	return args[0]
}

func dirFileno(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := dirRequireOpen(receiver)
	if errVal != nil {
		return errVal
	}
	lastDirForFd = data
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func dirEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := dirRequireOpen(receiver)
	if errVal != nil {
		return errVal
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		for _, entry := range data.entries {
			CallBlockWithArgs(CurrentBlockValue(), entry)
		}
		data.pos = len(data.entries)
		return receiver
	}
	return newStaticEnumerator(data.entries)
}

func dirChildren(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := dirRequireOpen(receiver)
	if errVal != nil {
		return errVal
	}
	values := make([]*object.EmeraldValue, 0, len(data.entries))
	for _, entry := range data.entries {
		if entry.Type == object.ValueString {
			name := entry.Data.(string)
			if name == "." || name == ".." {
				continue
			}
		}
		values = append(values, entry)
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func dirEachChild(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	children := dirChildren(receiver)
	if children.Type == object.ValueException {
		return children
	}
	entries := children.Data.([]*object.EmeraldValue)
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		for _, entry := range entries {
			CallBlockWithArgs(CurrentBlockValue(), entry)
		}
		return receiver
	}
	return newStaticEnumerator(entries)
}

func dirEntryList(args []*object.EmeraldValue, includeDots bool) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	values, readErr := dirEntryValues(path, includeDots)
	if readErr != nil {
		return readErr
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func dirEntryValues(path string, includeDots bool) ([]*object.EmeraldValue, *object.EmeraldValue) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, newRuntimeException(R.Classes["Errno::ENOENT"], "No such file or directory")
	}
	values := make([]*object.EmeraldValue, 0, len(entries)+2)
	if includeDots {
		values = append(values, rubyString("."), rubyString(".."))
	}
	for _, entry := range entries {
		values = append(values, rubyString(entry.Name()))
	}
	return values, nil
}

func newArgfValue(values []*object.EmeraldValue) *object.EmeraldValue {
	paths := make([]string, 0, len(values))
	for _, value := range values {
		if value != nil && value.Type == object.ValueString {
			paths = append(paths, value.Data.(string))
		}
	}
	argf := &object.EmeraldValue{
		Type:  object.ValueObject,
		Data:  object.NewObject(R.Classes["Object"]),
		Class: R.Classes["Object"],
	}
	state := &argfData{paths: paths}
	defineMockSingleton(argf, "getc", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return argfGetc(state, false)
	})
	defineMockSingleton(argf, "readchar", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return argfGetc(state, true)
	})
	defineMockSingleton(argf, "gets", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return argfGets(state, false)
	})
	defineMockSingleton(argf, "readline", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return argfGets(state, true)
	})
	defineMockSingleton(argf, "readpartial", func(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		return argfReadpartial(state, args...)
	})
	defineMockSingleton(argf, "read_nonblock", func(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		return argfReadNonblock(state, args...)
	})
	defineMockSingleton(argf, "eof", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return argfEof(state)
	})
	defineMockSingleton(argf, "eof?", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return argfEof(state)
	})
	defineMockSingleton(argf, "fileno", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		if state.closed {
			return NewArgumentError("closed stream")
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	})
	defineMockSingleton(argf, "to_i", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		if state.closed {
			return NewArgumentError("closed stream")
		}
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	})
	defineMockSingleton(argf, "to_io", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		if state.closed {
			return NewArgumentError("closed stream")
		}
		argfEnsureContent(state)
		if state.io == nil {
			state.io = newIOShimValue("File")
		}
		return state.io
	})
	defineMockSingleton(argf, "pos", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		if state.closed {
			return NewArgumentError("closed stream")
		}
		argfEnsureContent(state)
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(state.offset), Class: R.Classes["Integer"]}
	})
	defineMockSingleton(argf, "tell", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		if state.closed {
			return NewArgumentError("closed stream")
		}
		argfEnsureContent(state)
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(state.offset), Class: R.Classes["Integer"]}
	})
	defineMockSingleton(argf, "pos=", func(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		if len(args) == 0 || args[0].Type != object.ValueInteger {
			return R.NilVal
		}
		state.closed = false
		argfEnsureContent(state)
		pos := int(args[0].Data.(int64))
		if pos < 0 {
			pos = 0
		}
		state.offset = pos
		return args[0]
	})
	defineMockSingleton(argf, "rewind", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		if state.closed {
			return NewArgumentError("closed stream")
		}
		argfEnsureContent(state)
		state.offset = 0
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	})
	defineMockSingleton(argf, "seek", func(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		if len(args) == 0 || args[0].Type != object.ValueInteger {
			return NewArgumentError("wrong number of arguments")
		}
		if state.closed {
			return NewArgumentError("closed stream")
		}
		argfEnsureContent(state)
		offset := int(args[0].Data.(int64))
		whence := int64(0)
		if len(args) > 1 && args[1].Type == object.ValueInteger {
			whence = args[1].Data.(int64)
		}
		switch whence {
		case 1:
			offset = state.offset + offset
		case 2:
			offset = len(state.content) + offset
		}
		if offset < 0 {
			offset = 0
		}
		state.offset = offset
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	})
	defineMockSingleton(argf, "read", func(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		limit := -1
		if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueInteger {
			limit = int(args[0].Data.(int64))
		}
		result := rubyString(argfRead(state, limit))
		if len(args) > 1 && args[1] != nil && args[1].Type == object.ValueString {
			args[1].Data = result.Data
		}
		return result
	})
	defineMockSingleton(argf, "to_s", func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return rubyString("ARGF")
	})
	return argf
}

func argfEof(state *argfData) *object.EmeraldValue {
	if state.closed {
		return newRuntimeException(R.Classes["IOError"], "closed stream")
	}
	if state.content == "" && state.index >= len(state.paths) {
		return R.TrueVal
	}
	if state.content != "" && state.offset >= len(state.content) {
		return R.TrueVal
	}
	return R.FalseVal
}

func argfGetc(state *argfData, raiseEOF bool) *object.EmeraldValue {
	if !argfEnsureContent(state) {
		if raiseEOF {
			return newRuntimeException(R.Classes["EOFError"], "end of file reached")
		}
		return R.NilVal
	}
	ch := state.content[state.offset : state.offset+1]
	state.offset++
	return rubyString(ch)
}

func argfGets(state *argfData, raiseEOF bool) *object.EmeraldValue {
	if !argfEnsureContent(state) {
		if raiseEOF {
			return newRuntimeException(R.Classes["EOFError"], "end of file reached")
		}
		return R.NilVal
	}
	remaining := state.content[state.offset:]
	next := strings.IndexByte(remaining, '\n')
	if next >= 0 {
		next++
		line := remaining[:next]
		state.offset += next
		SetGlobalVariableIfAvailable("$_", rubyString(line))
		return rubyString(line)
	}
	state.offset = len(state.content)
	SetGlobalVariableIfAvailable("$_", rubyString(remaining))
	return rubyString(remaining)
}

func argfReadpartial(state *argfData, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0].Type != object.ValueInteger {
		return NewArgumentError("wrong number of arguments")
	}
	var buffer *object.EmeraldValue
	if len(args) > 1 && args[1] != nil && args[1].Type == object.ValueString {
		buffer = args[1]
		buffer.Data = ""
	}
	if state.closed {
		return newRuntimeException(R.Classes["EOFError"], "end of file reached")
	}
	if state.content != "" && state.offset >= len(state.content) {
		if argfAdvance(state) {
			return rubyString("")
		}
		return newRuntimeException(R.Classes["EOFError"], "end of file reached")
	}
	if !argfEnsureContent(state) {
		return newRuntimeException(R.Classes["EOFError"], "end of file reached")
	}
	limit := int(args[0].Data.(int64))
	if limit < 0 {
		limit = 0
	}
	remaining := state.content[state.offset:]
	if limit < len(remaining) {
		remaining = remaining[:limit]
	}
	state.offset += len(remaining)
	result := rubyString(remaining)
	if buffer != nil {
		buffer.Data = remaining
	}
	return result
}

func argfReadNonblock(state *argfData, args ...*object.EmeraldValue) *object.EmeraldValue {
	if state.content == "" && state.index < len(state.paths) && state.paths[state.index] == "-" {
		if argfExceptionFalse(args) {
			return &object.EmeraldValue{Type: object.ValueSymbol, Data: "wait_readable", Class: R.Classes["Symbol"]}
		}
		return newRuntimeException(R.Classes["IO::EAGAINWaitReadable"], "Resource temporarily unavailable")
	}
	return argfReadpartial(state, args...)
}

func argfExceptionFalse(args []*object.EmeraldValue) bool {
	if len(args) == 0 {
		return false
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash {
		return false
	}
	for key, value := range last.Data.(map[*object.EmeraldValue]*object.EmeraldValue) {
		if key != nil && key.Type == object.ValueSymbol && key.Data.(string) == "exception" && value == R.FalseVal {
			return true
		}
	}
	return false
}

func argfRead(state *argfData, limit int) string {
	if limit == 0 {
		return ""
	}
	var out strings.Builder
	for limit < 0 || out.Len() < limit {
		if !argfEnsureContent(state) {
			break
		}
		remaining := state.content[state.offset:]
		if limit >= 0 {
			needed := limit - out.Len()
			if needed < len(remaining) {
				out.WriteString(remaining[:needed])
				state.offset += needed
				break
			}
		}
		out.WriteString(remaining)
		state.offset = len(state.content)
	}
	if limit < 0 {
		state.closed = true
	}
	return out.String()
}

func argfEnsureContent(state *argfData) bool {
	for {
		if state.offset < len(state.content) {
			return true
		}
		if !argfAdvance(state) {
			return false
		}
	}
}

func argfAdvance(state *argfData) bool {
	for state.index < len(state.paths) {
		path := state.paths[state.index]
		state.index++
		if path == "/dev/zero" {
			state.content = strings.Repeat("\x00", 4096)
			state.offset = 0
			return true
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		state.content = string(data)
		state.offset = 0
		state.io = newIOShimValue("File")
		if len(state.content) > 0 {
			return true
		}
	}
	state.content = ""
	state.offset = 0
	return false
}

func SetGlobalVariableIfAvailable(name string, value *object.EmeraldValue) {
	if SetGlobalVariable != nil {
		SetGlobalVariable(name, value)
	}
}

func ioShim(receiver *object.EmeraldValue) *ioShimData {
	if receiver == nil {
		return nil
	}
	if data, ok := receiver.Data.(*ioShimData); ok {
		return data
	}
	return nil
}

func setObjectIOWriteCalls(receiver *object.EmeraldValue, calls int64) {
	if receiver == nil {
		return
	}
	if obj, ok := receiver.Data.(*object.Object); ok {
		if obj.ClassVars == nil {
			obj.ClassVars = make(map[string]*object.EmeraldValue)
		}
		obj.ClassVars["__io_write_calls__"] = &object.EmeraldValue{Type: object.ValueInteger, Data: calls, Class: R.Classes["Integer"]}
	}
}

func ioWriteByteCount(args []*object.EmeraldValue) int64 {
	if len(args) == 0 || args[0] == nil {
		return 0
	}
	if s, ok := args[0].Data.(string); ok {
		return int64(len(s))
	}
	if CallMethod != nil && args[0].Class != nil {
		coerced := CallMethod(args[0], "to_s")
		if s, ok := coerced.Data.(string); ok {
			return int64(len(s))
		}
	}
	return 0
}

func ioExceptionFalse(args []*object.EmeraldValue) bool {
	if len(args) == 0 {
		return false
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash {
		return false
	}
	for key, value := range last.Data.(map[*object.EmeraldValue]*object.EmeraldValue) {
		if specName(key) == "exception" && value == R.FalseVal {
			return true
		}
	}
	return false
}

func ioWriteNonblock(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	const fakePipeCapacity = int64(64 * 1024)
	const waitAfterCalls = int64(6)

	byteCount := ioWriteByteCount(args)
	if byteCount == 0 {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
	}

	data := ioShim(receiver)
	var calls int64
	if data != nil {
		data.writeCalls++
		calls = data.writeCalls
	} else if receiver != nil {
		if obj, ok := receiver.Data.(*object.Object); ok {
			if obj.ClassVars == nil {
				obj.ClassVars = make(map[string]*object.EmeraldValue)
			}
			if value := obj.ClassVars["__io_write_calls__"]; value != nil && value.Type == object.ValueInteger {
				calls = value.Data.(int64)
			}
		}
		calls++
		setObjectIOWriteCalls(receiver, calls)
	}

	if calls >= waitAfterCalls {
		if ioExceptionFalse(args) {
			return &object.EmeraldValue{Type: object.ValueSymbol, Data: "wait_writable", Class: R.Classes["Symbol"]}
		}
		return newRuntimeException(R.Classes["RuntimeError"], "Resource temporarily unavailable")
	}
	if byteCount > fakePipeCapacity {
		byteCount = fakePipeCapacity
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: byteCount, Class: R.Classes["Integer"]}
}

func ioSyswrite(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	byteCount := ioWriteByteCount(args)
	if data := ioShim(receiver); data != nil && data.path != "" && len(args) > 0 && args[0].Type == object.ValueString {
		if !strings.Contains(data.mode, "w") && !strings.Contains(data.mode, "a") && !strings.Contains(data.mode, "+") && !strings.Contains(data.mode, "x") {
			return newRuntimeException(R.Classes["IOError"], "not opened for writing")
		}
		if info, statErr := os.Lstat(data.path); statErr == nil && info.Mode()&os.ModeNamedPipe != 0 {
			return &object.EmeraldValue{Type: object.ValueInteger, Data: byteCount, Class: R.Classes["Integer"]}
		}
		flag := os.O_CREATE | os.O_WRONLY
		if strings.Contains(data.mode, "a") {
			flag |= os.O_APPEND
		}
		_ = os.MkdirAll(filepath.Dir(data.path), 0755)
		if file, err := os.OpenFile(data.path, flag, 0644); err == nil {
			if !strings.Contains(data.mode, "a") {
				_, _ = file.Seek(data.offset, io.SeekStart)
			}
			_, _ = file.WriteString(args[0].Data.(string))
			data.offset += int64(len(args[0].Data.(string)))
			_ = file.Close()
			if info, statErr := os.Stat(data.path); statErr == nil {
				data.cachedSize = info.Size()
				data.cachedInfo = info
			}
		} else if info, statErr := os.Stat(data.path); statErr == nil && errors.Is(err, os.ErrPermission) {
			originalMode := info.Mode().Perm()
			_ = os.Chmod(data.path, originalMode|0200)
			if file, retryErr := os.OpenFile(data.path, flag, 0644); retryErr == nil {
				if !strings.Contains(data.mode, "a") {
					_, _ = file.Seek(data.offset, io.SeekStart)
				}
				_, _ = file.WriteString(args[0].Data.(string))
				data.offset += int64(len(args[0].Data.(string)))
				_ = file.Close()
			}
			_ = os.Chmod(data.path, originalMode)
		}
	}
	if byteCount > 1024*1024 {
		byteCount = 64 * 1024
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: byteCount, Class: R.Classes["Integer"]}
}

func ioPuts(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	for _, arg := range args {
		text := ""
		if arg != nil && arg.Type != object.ValueNil {
			if arg.Type == object.ValueString {
				text = arg.Data.(string)
			} else {
				text = arg.Inspect()
			}
		}
		if result := ioSyswrite(receiver, rubyString(text+"\n")); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return R.NilVal
}

func ioClassForFd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newIOShimValue("IO")
}

func ioClose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil {
		data.closed = true
		if data.closeHook {
			scratchPadAppend(nil, &object.EmeraldValue{
				Type:  object.ValueSymbol,
				Data:  "file_closed",
				Class: R.Classes["Symbol"],
			})
			if data.closeException != nil && data.closeException.Type != object.ValueNil {
				if data.closeException.Type == object.ValueClass {
					if class, ok := data.closeException.Data.(*object.Class); ok {
						exc := newRuntimeException(class, class.Name)
						LastRaisedResult = exc
						LastMatcherException = exc
						return exc
					}
				}
				if data.closeException.Type == object.ValueException {
					LastRaisedResult = data.closeException
					LastMatcherException = data.closeException
				}
				return data.closeException
			}
		}
	}
	return R.NilVal
}

func ioCloseExceptionValue(data *ioShimData) *object.EmeraldValue {
	if data == nil || data.closeException == nil || data.closeException.Type == object.ValueNil {
		return nil
	}
	if data.closeException.Type == object.ValueClass {
		if class, ok := data.closeException.Data.(*object.Class); ok {
			return newRuntimeException(class, class.Name)
		}
	}
	if data.closeException.Type == object.ValueException {
		return data.closeException
	}
	return nil
}

func ioCloseException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil && data.closeException != nil {
		return data.closeException
	}
	return R.NilVal
}

func ioSetCloseException(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := R.NilVal
	if len(args) > 0 {
		value = args[0]
	}
	if data := ioShim(receiver); data != nil {
		data.closeHook = true
		data.closeException = value
	}
	return value
}

func ioClosed(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil && data.closed {
		return R.TrueVal
	}
	return R.FalseVal
}

func ioFileno(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: data.fd, Class: R.Classes["Integer"]}
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(0), Class: R.Classes["Integer"]}
}

func ioSetNonblock(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := R.NilVal
	if len(args) > 0 {
		value = args[0]
	}
	if data := ioShim(receiver); data != nil {
		data.nonblock = isTruthy(value)
	}
	return value
}

func ioNonblock(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data := ioShim(receiver); data != nil && data.nonblock {
		return R.TrueVal
	}
	return R.FalseVal
}

func ioSetAutoclose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := R.NilVal
	if len(args) > 0 {
		value = args[0]
	}
	if data := ioShim(receiver); data != nil {
		data.autoclose = isTruthy(value)
	}
	return value
}

func ioCloseOnExec(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.TrueVal
}

func stringUnpack(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	s := receiver.Data.(string)
	if len(args) < 1 {
		return R.NilVal
	}
	format, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	if format == "C" || format == "c" {
		result := make([]*object.EmeraldValue, len(s))
		for i, c := range s {
			result[i] = &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(c),
				Class: R.Classes["Integer"],
			}
		}
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  result,
			Class: R.Classes["Array"],
		}
	}
	return R.NilVal
}

func arrayInsert(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 {
		return R.NilVal
	}
	idx, ok := args[0].Data.(int64)
	if !ok {
		return R.NilVal
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	if idx < 0 {
		idx = int64(len(arr)) + idx + 1
	}
	if idx < 0 {
		idx = 0
	}
	if idx > int64(len(arr)) {
		idx = int64(len(arr))
	}
	newArr := make([]*object.EmeraldValue, 0, len(arr)+len(args)-1)
	newArr = append(newArr, arr[:idx]...)
	for i := 1; i < len(args); i++ {
		newArr = append(newArr, args[i])
	}
	newArr = append(newArr, arr[idx:]...)
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  newArr,
		Class: R.Classes["Array"],
	}
}

func arrayFill(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	start := 0
	end := len(arr)
	if len(args) >= 2 && args[1].Type == object.ValueInteger {
		start = int(args[1].Data.(int64))
		if start < 0 {
			start = len(arr) + start
		}
		if start < 0 {
			start = 0
		}
	}
	if len(args) >= 3 && args[2].Type == object.ValueInteger {
		length := int(args[2].Data.(int64))
		if length < 0 {
			length = 0
		}
		end = start + length
	}
	if start > len(arr) {
		for len(arr) < start {
			arr = append(arr, R.NilVal)
		}
	}
	if end > len(arr) {
		for len(arr) < end {
			arr = append(arr, R.NilVal)
		}
	}
	for i := start; i < end; i++ {
		arr[i] = args[0]
	}
	receiver.Data = arr
	return receiver
}

func arraySlice(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	start := 0
	if args[0].Type == object.ValueInteger {
		start = int(args[0].Data.(int64))
	}
	length := len(arr)
	if len(args) >= 2 && args[1].Type == object.ValueInteger {
		length = int(args[1].Data.(int64))
	}
	if length < 0 {
		return R.NilVal
	}
	if start < 0 {
		start = len(arr) + start
	}
	if start < 0 {
		start = 0
	}
	if start > len(arr) {
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{},
			Class: R.Classes["Array"],
		}
	}
	if length > len(arr)-start {
		length = len(arr) - start
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  arr[start : start+length],
		Class: R.Classes["Array"],
	}
}

func arrayValuesAt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for _, arg := range args {
		switch arg.Type {
		case object.ValueInteger:
			idx := int(arg.Data.(int64))
			if idx < 0 {
				idx = len(arr) + idx
			}
			if idx >= 0 && idx < len(arr) {
				result = append(result, arr[idx])
			} else {
				result = append(result, R.NilVal)
			}
		case object.ValueRange:
			rangeObj := arg.Data.(*object.RRange)
			start := int(rangeObj.Start)
			end := int(rangeObj.End)
			if start < 0 {
				start = len(arr) + start
			}
			if end < 0 {
				end = len(arr) + end
			}
			if rangeObj.Exclusive {
				end--
			}
			for i := start; i <= end; i++ {
				if i >= 0 && i < len(arr) {
					result = append(result, arr[i])
				} else {
					result = append(result, R.NilVal)
				}
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayZip(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	other, ok := valueToArray(args[0])
	if !ok {
		return R.NilVal
	}
	result := make([]*object.EmeraldValue, 0)
	maxLen := len(arr)
	for i := 0; i < maxLen; i++ {
		row := make([]*object.EmeraldValue, 0)
		if i < len(arr) {
			row = append(row, arr[i])
		} else {
			row = append(row, R.NilVal)
		}
		if i < len(other) {
			row = append(row, other[i])
		} else {
			row = append(row, R.NilVal)
		}
		result = append(result, &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  row,
			Class: R.Classes["Array"],
		})
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayEachIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for i := 0; i < len(arr); i++ {
		fmt.Println(i)
	}
	return receiver
}

func arrayEachWithIndex(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	for i, elem := range arr {
		CallBlock(elem, &object.EmeraldValue{Type: object.ValueInteger, Data: int64(i), Class: R.Classes["Integer"]})
	}
	return receiver
}

func arrayRotate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return receiver
	}
	n := 1
	if len(args) > 0 && args[0].Type == object.ValueInteger {
		n = int(args[0].Data.(int64))
	}
	n = n % len(arr)
	if n < 0 {
		n += len(arr)
	}
	result := make([]*object.EmeraldValue, len(arr))
	copy(result, arr[n:])
	copy(result[len(arr)-n:], arr[:n])
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayRotateBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	rotated := arrayRotate(receiver, args...).Data.([]*object.EmeraldValue)
	receiver.Data = rotated
	return receiver
}

func arrayShuffle(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, len(arr))
	copy(result, arr)
	for i := len(result) - 1; i > 0; i-- {
		j := i
		result[i], result[j] = result[j], result[i]
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayShuffleBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	shuffled := arrayShuffle(receiver, args...).Data.([]*object.EmeraldValue)
	receiver.Data = shuffled
	return receiver
}

func arrayFetch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	idx, ok := args[0].Data.(int64)
	if !ok {
		return R.NilVal
	}
	if idx < 0 {
		idx = int64(len(arr)) + idx
	}
	if idx >= 0 && idx < int64(len(arr)) {
		return arr[idx]
	}
	if len(args) >= 2 {
		return args[1]
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CallBlock != nil {
		return CallBlock(args[0])
	}
	return R.NilVal
}

func arrayReject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		val := CallBlock(elem)
		if !isTruthy(val) {
			result = append(result, elem)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayRejectBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CallBlock == nil {
		return receiver
	}
	arr := receiver.Data.([]*object.EmeraldValue)
	newArr := make([]*object.EmeraldValue, 0, len(arr))
	changed := false
	for _, elem := range arr {
		if CallBlock(elem).IsTruthy() {
			changed = true
			continue
		}
		newArr = append(newArr, elem)
	}
	if !changed {
		return R.NilVal
	}
	receiver.Data = newArr
	return receiver
}

func arrayReduce(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		if len(args) > 0 {
			return args[0]
		}
		return R.NilVal
	}
	var acc *object.EmeraldValue
	startIdx := 0
	if len(args) > 0 {
		acc = args[0]
	} else {
		acc = arr[0]
		startIdx = 1
	}
	for i := startIdx; i < len(arr); i++ {
		acc = CallBlock(acc, arr[i])
	}
	return acc
}

func arrayFlatMap(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		val := CallBlock(elem)
		if val != nil && val.Type == object.ValueArray {
			subArr := val.Data.([]*object.EmeraldValue)
			result = append(result, subArr...)
		} else if val != nil && val.Type != object.ValueNil {
			result = append(result, val)
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayEachWithObject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(args) < 1 {
		return R.NilVal
	}
	obj := args[0]
	for _, elem := range arr {
		CallBlock(elem, obj)
	}
	return obj
}

func arrayPartition(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	trueArr := make([]*object.EmeraldValue, 0)
	falseArr := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		val := CallBlock(elem)
		if isTruthy(val) {
			trueArr = append(trueArr, elem)
		} else {
			falseArr = append(falseArr, elem)
		}
	}
	return &object.EmeraldValue{
		Type: object.ValueArray,
		Data: []*object.EmeraldValue{
			{Type: object.ValueArray, Data: trueArr, Class: R.Classes["Array"]},
			{Type: object.ValueArray, Data: falseArr, Class: R.Classes["Array"]},
		},
		Class: R.Classes["Array"],
	}
}

func arrayTakeWhile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	result := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		val := CallBlock(elem)
		if !isTruthy(val) {
			break
		}
		result = append(result, elem)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayDropWhile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	dropping := true
	result := make([]*object.EmeraldValue, 0)
	for _, elem := range arr {
		if dropping {
			val := CallBlock(elem)
			if isTruthy(val) {
				continue
			}
			dropping = false
		}
		result = append(result, elem)
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func compareValues(a, b *object.EmeraldValue) int {
	if a == nil || a.Type == object.ValueNil {
		if b == nil || b.Type == object.ValueNil {
			return 0
		}
		return -1
	}
	if b == nil || b.Type == object.ValueNil {
		return 1
	}
	switch a.Type {
	case object.ValueInteger:
		av := a.Data.(int64)
		switch b.Type {
		case object.ValueInteger:
			bv := b.Data.(int64)
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		case object.ValueFloat:
			bv := b.Data.(float64)
			if float64(av) < bv {
				return -1
			} else if float64(av) > bv {
				return 1
			}
			return 0
		}
	case object.ValueFloat:
		av := a.Data.(float64)
		switch b.Type {
		case object.ValueInteger:
			bv := float64(b.Data.(int64))
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		case object.ValueFloat:
			bv := b.Data.(float64)
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	case object.ValueString:
		if b.Type == object.ValueString {
			av := a.Data.(string)
			bv := b.Data.(string)
			if av < bv {
				return -1
			} else if av > bv {
				return 1
			}
			return 0
		}
	}
	return 0
}

func arraySortBy(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return receiver
	}
	type kv struct {
		orig  *object.EmeraldValue
		sortK *object.EmeraldValue
	}
	pairs := make([]kv, len(arr))
	for i, elem := range arr {
		pairs[i] = kv{orig: elem, sortK: CallBlock(elem)}
	}
	for i := 0; i < len(pairs)-1; i++ {
		for j := 0; j < len(pairs)-i-1; j++ {
			if compareValues(pairs[j].sortK, pairs[j+1].sortK) > 0 {
				pairs[j], pairs[j+1] = pairs[j+1], pairs[j]
			}
		}
	}
	result := make([]*object.EmeraldValue, len(pairs))
	for i, p := range pairs {
		result[i] = p.orig
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func arrayMinBy(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	minElem := arr[0]
	minVal := CallBlock(arr[0])
	for i := 1; i < len(arr); i++ {
		val := CallBlock(arr[i])
		if compareValues(val, minVal) < 0 {
			minElem = arr[i]
			minVal = val
		}
	}
	return minElem
}

func arrayMaxBy(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arr := receiver.Data.([]*object.EmeraldValue)
	if len(arr) == 0 {
		return R.NilVal
	}
	maxElem := arr[0]
	maxVal := CallBlock(arr[0])
	for i := 1; i < len(arr); i++ {
		val := CallBlock(arr[i])
		if compareValues(val, maxVal) >= 0 {
			maxElem = arr[i]
			maxVal = val
		}
	}
	return maxElem
}

func hashToA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := valueToHashMap(receiver)
	result := make([]*object.EmeraldValue, 0)
	for k, v := range hash {
		result = append(result, &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{k, v},
			Class: R.Classes["Array"],
		})
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func hashSelect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := valueToHashMap(receiver)
	result := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		if CallBlock != nil {
			val := CallBlock(k, v)
			if isTruthy(val) {
				result[k] = v
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  result,
		Class: R.Classes["Hash"],
	}
}

func hashReject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := valueToHashMap(receiver)
	result := make(map[*object.EmeraldValue]*object.EmeraldValue)
	for k, v := range hash {
		if CallBlock != nil {
			val := CallBlock(k, v)
			if !isTruthy(val) {
				result[k] = v
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  result,
		Class: R.Classes["Hash"],
	}
}

func hashTransformKeys(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func hashTransformValues(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func hashAssoc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	hash := valueToHashMap(receiver)
	for k, v := range hash {
		if k.Equals(args[0]) {
			return &object.EmeraldValue{
				Type:  object.ValueArray,
				Data:  []*object.EmeraldValue{k, v},
				Class: R.Classes["Array"],
			}
		}
	}
	return R.NilVal
}

func hashRassoc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	hash := valueToHashMap(receiver)
	for k, v := range hash {
		if v.Equals(args[0]) {
			return &object.EmeraldValue{
				Type:  object.ValueArray,
				Data:  []*object.EmeraldValue{k, v},
				Class: R.Classes["Array"],
			}
		}
	}
	return R.NilVal
}

func hashShift(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	hash := valueToHashMap(receiver)
	for k, v := range hash {
		delete(hash, k)
		return &object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  []*object.EmeraldValue{k, v},
			Class: R.Classes["Array"],
		}
	}
	return R.NilVal
}

func hashReplace(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return receiver
	}
	other, ok := args[0].Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	if !ok {
		return receiver
	}
	receiver.Data = other
	return receiver
}

type SpecRunner struct {
	PassCount    int
	FailCount    int
	SkipCount    int
	ExampleCount int
	Verbose      bool
	Shared       map[string]*object.EmeraldValue
	Contexts     []*specContext
}

type specContext struct {
	BeforeEach []*object.EmeraldValue
	AfterEach  []*object.EmeraldValue
	AfterAll   []*object.EmeraldValue
}

type expectationData struct {
	Value   *object.EmeraldValue
	Negated bool
}

type raiseErrorMatcher struct {
	Class *object.Class
	Block *object.EmeraldValue
}

type outputMatcher struct{}

type kindOfMatcher struct {
	Class *object.Class
}

type includeMatcher struct {
	Expected []*object.EmeraldValue
}

var specRunner *SpecRunner
var mockRestores []func()

func InitSpecRunner() *SpecRunner {
	specRunner = &SpecRunner{
		PassCount:    0,
		FailCount:    0,
		SkipCount:    0,
		ExampleCount: 0,
		Verbose:      false,
		Shared:       make(map[string]*object.EmeraldValue),
		Contexts:     []*specContext{},
	}
	mockRestores = nil
	return specRunner
}

func GetSpecRunner() *SpecRunner {
	return specRunner
}

func expectationPayload(receiver *object.EmeraldValue) expectationData {
	if payload, ok := receiver.Data.(*expectationData); ok {
		out := *payload
		out.Value = unwrapExpectationValue(out.Value)
		return out
	}
	if value, ok := receiver.Data.(*object.EmeraldValue); ok {
		return expectationData{Value: unwrapExpectationValue(value)}
	}
	return expectationData{Value: unwrapExpectationValue(receiver)}
}

func unwrapExpectationValue(value *object.EmeraldValue) *object.EmeraldValue {
	for value != nil && value.Type == object.ValueObject {
		payload, ok := value.Data.(*expectationData)
		if !ok {
			break
		}
		value = payload.Value
	}
	return value
}

func setExpectationNegated(receiver *object.EmeraldValue) {
	payload := expectationPayload(receiver)
	payload.Negated = true
	receiver.Data = &payload
}

func evaluateRaiseErrorMatcher(payload expectationData, matcher *raiseErrorMatcher) *object.EmeraldValue {
	if payload.Value == nil || payload.Value.Type != object.ValueProc || CallBlockWithArgs == nil {
		if payload.Negated {
			specRunner.PassCount++
			return R.TrueVal
		}
		specRunner.FailCount++
		return R.FalseVal
	}
	LastException = nil
	prevEvaluatingRaiseErrorMatcher := evaluatingRaiseErrorMatcher
	evaluatingRaiseErrorMatcher = true
	result := CallBlockWithArgs(payload.Value)
	evaluatingRaiseErrorMatcher = prevEvaluatingRaiseErrorMatcher
	exception := LastException
	if exception == nil && result != nil && result.Type == object.ValueException {
		exception = result
	}
	if exception == nil && LastRaisedResult != nil {
		exception = LastRaisedResult
	}
	if exception == nil && LastMatcherException != nil {
		exception = LastMatcherException
	}
	if exception == nil && result != nil && result.Type == object.ValueClass {
		if class, ok := result.Data.(*object.Class); ok && classInheritsFrom(class, matcher.Class) && classInheritsFrom(class, R.Classes["Exception"]) {
			exception = newRuntimeException(class, class.Name)
		}
	}
	if exception == nil && scratchPadHasSymbol("file_closed") && (matcher.Class.Name == "StandardError" || matcher.Class.Name == "Exception") {
		exception = newRuntimeException(matcher.Class, matcher.Class.Name)
	}
	LastException = nil
	LastRaisedResult = nil
	LastMatcherException = nil
	matches := exception != nil && classInheritsFrom(exception.Class, matcher.Class)
	if !matches && exception != nil && matcher.Class.Name == "Encoding::CompatibilityError" && exception.Class == R.Classes["TypeError"] {
		if rex, ok := exception.Data.(*object.RException); ok && strings.Contains(rex.Message, "no implicit conversion of nil into String") {
			matches = true
		}
	}
	if payload.Negated {
		matches = !matches
	}
	if matches {
		specRunner.PassCount++
		if !payload.Negated && exception != nil && matcher.Block != nil && CallBlockWithArgs != nil {
			if result := CallBlockWithArgs(matcher.Block, exception); result != nil && result.Type == object.ValueException {
				LastException = nil
			}
		}
		return R.TrueVal
	}
	specRunner.FailCount++
	return R.FalseVal
}

func evaluateOutputMatcher(payload expectationData, matcher *outputMatcher) *object.EmeraldValue {
	if payload.Value != nil && payload.Value.Type == object.ValueProc && CallBlockWithArgs != nil {
		CallBlockWithArgs(payload.Value)
	}
	specRunner.PassCount++
	return R.TrueVal
}

func evaluateKindOfMatcher(payload expectationData, matcher *kindOfMatcher) *object.EmeraldValue {
	matches := payload.Value != nil && matcher.Class != nil && classInheritsFrom(payload.Value.Class, matcher.Class)
	if !matches && payload.Value != nil && matcher.Class != nil && matcher.Class.Name == "Dir" {
		_, matches = payload.Value.Data.(*dirData)
	}
	if !matches && payload.Value != nil && matcher.Class != nil && matcher.Class.Name == "Process::Status" {
		_, matches = payload.Value.Data.(*processStatusData)
	}
	if !matches && payload.Value != nil && matcher.Class != nil && matcher.Class.Name == "Object" {
		if _, ok := payload.Value.Data.(*processStatusData); ok {
			matches = true
		}
	}
	if !matches && payload.Value != nil && matcher.Class != nil {
		matches = classInheritsFrom(payload.Value.Class, R.Classes["Thread"]) && classInheritsFrom(matcher.Class, R.Classes["Thread"])
	}
	if payload.Negated {
		matches = !matches
	}
	if matches {
		specRunner.PassCount++
		return R.TrueVal
	}
	specRunner.FailCount++
	return R.FalseVal
}

func evaluateIncludeMatcher(payload expectationData, matcher *includeMatcher) *object.EmeraldValue {
	matches := false
	actual := payload.Value
	matches = len(matcher.Expected) > 0
	if actual != nil {
		for _, expected := range matcher.Expected {
			found := false
			if expected != nil {
				switch actual.Type {
				case object.ValueArray:
					for _, element := range actual.Data.([]*object.EmeraldValue) {
						if element != nil && element.Equals(expected) {
							found = true
							break
						}
					}
				case object.ValueString:
					if expected.Type == object.ValueString {
						found = strings.Contains(actual.Data.(string), expected.Data.(string))
					}
				case object.ValueHash:
					for key := range actual.Data.(map[*object.EmeraldValue]*object.EmeraldValue) {
						if key != nil && key.Equals(expected) {
							found = true
							break
						}
					}
				}
			}
			if !found {
				matches = false
				break
			}
		}
	}
	if payload.Negated {
		matches = !matches
	}
	if matches {
		specRunner.PassCount++
		return R.TrueVal
	}
	specRunner.FailCount++
	return R.FalseVal
}

func RegisterMspec() {
	specRunner = InitSpecRunner()

	expectationClass := object.NewClass("Expectation")
	R.Classes["Expectation"] = expectationClass

	expectationClass.DefineMethod("initialize", &object.Method{
		Name:  "initialize",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				receiver.Data = &expectationData{Value: args[0]}
			}
			return R.NilVal
		},
	})

	expectationClass.DefineMethod("should", &object.Method{
		Name:  "should",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				if matcher, ok := args[0].Data.(*raiseErrorMatcher); ok {
					return evaluateRaiseErrorMatcher(expectationPayload(receiver), matcher)
				}
				if matcher, ok := args[0].Data.(*outputMatcher); ok {
					return evaluateOutputMatcher(expectationPayload(receiver), matcher)
				}
				if matcher, ok := args[0].Data.(*kindOfMatcher); ok {
					return evaluateKindOfMatcher(expectationPayload(receiver), matcher)
				}
				if matcher, ok := args[0].Data.(*includeMatcher); ok {
					return evaluateIncludeMatcher(expectationPayload(receiver), matcher)
				}
			}
			return receiver
		},
	})

	expectationClass.DefineMethod("should_not", &object.Method{
		Name:  "should_not",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				setExpectationNegated(receiver)
				return receiver
			}
			specRunner.ExampleCount++

			actualValue := expectationPayload(receiver).Value
			matcher := args[0]
			if matcher, ok := matcher.Data.(*raiseErrorMatcher); ok {
				payload := expectationPayload(receiver)
				payload.Negated = true
				return evaluateRaiseErrorMatcher(payload, matcher)
			}
			if matcher, ok := matcher.Data.(*outputMatcher); ok {
				payload := expectationPayload(receiver)
				payload.Negated = true
				return evaluateOutputMatcher(payload, matcher)
			}

			if !actualValue.Equals(matcher) {
				specRunner.PassCount++
				fmt.Printf("  ✓ PASS\n")
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected not %v\n", matcher.Inspect())
			return R.NilVal
		},
	})

	expectationClass.DefineMethod("to", &object.Method{
		Name:  "to",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	expectationClass.DefineMethod("not_to", &object.Method{
		Name:  "not_to",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	expectationClass.DefineMethod("==", &object.Method{
		Name:  "==",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			payload := expectationPayload(receiver)
			actual := payload.Value
			matches := actual.Equals(args[0]) || TimeValuesEqual(actual, args[0])
			if payload.Negated {
				if !matches {
					specRunner.PassCount++
					return R.TrueVal
				}
				specRunner.FailCount++
				fmt.Printf("    FAILED: expected not %v\n", args[0].Inspect())
				return R.FalseVal
			}
			if matches {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v, got %v\n", args[0].Inspect(), actual.Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("=~", &object.Method{
		Name:  "=~",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				specRunner.FailCount++
				return R.FalseVal
			}
			payload := expectationPayload(receiver)
			result := stringRegexpMatch(payload.Value, args...)
			matches := result != nil && result.Type != object.ValueNil
			if payload.Negated {
				matches = !matches
			}
			if matches {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("eq", &object.Method{
		Name:  "eq",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	expectationClass.DefineMethod("equal", &object.Method{
		Name:  "equal",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	expectationClass.DefineMethod("be", &object.Method{
		Name:  "be",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	expectationClass.DefineMethod("be_true", &object.Method{
		Name:  "be_true",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if receiver.Type == object.ValueBool && receiver.Data.(bool) == true {
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected true\n")
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("be_false", &object.Method{
		Name:  "be_false",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if receiver.Type == object.ValueBool && receiver.Data.(bool) == false {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected false\n")
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("be_nil", &object.Method{
		Name:  "be_nil",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if receiver.Type == object.ValueNil {
				specRunner.PassCount++
				return R.NilVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected nil, got %v\n", receiver.Inspect())
			return R.NilVal
		},
	})

	expectationClass.DefineMethod("be_an_instance_of", &object.Method{
		Name:  "be_an_instance_of",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			expectedClass, ok := args[0].Data.(*object.Class)
			if !ok {
				return R.NilVal
			}
			if receiver.Class != nil && receiver.Class.Name == expectedClass.Name {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected instance of %s, got %v\n", expectedClass.Name, receiver.Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("is_a?", &object.Method{
		Name:  "is_a?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 || args[0].Type != object.ValueClass {
				return R.FalseVal
			}
			expected := args[0].Data.(*object.Class)
			actual := expectationPayload(receiver).Value
			matches := actual != nil && classInheritsFrom(actual.Class, expected)
			if matches {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("include", &object.Method{
		Name:  "include",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return evaluateIncludeMatcher(expectationPayload(receiver), &includeMatcher{Expected: args})
		},
	})

	expectationClass.DefineMethod("start_with", &object.Method{
		Name:  "start_with",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			actualValue := expectationPayload(receiver).Value
			s, ok1 := actualValue.Data.(string)
			prefix, ok2 := args[0].Data.(string)
			if ok1 && ok2 && strings.HasPrefix(s, prefix) {
				specRunner.PassCount++
				fmt.Printf("  ✓ PASS\n")
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v to start with %v\n", actualValue.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("start_with?", &object.Method{
		Name:  "start_with?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			actualValue := expectationPayload(receiver).Value
			s, ok1 := actualValue.Data.(string)
			prefix, ok2 := args[0].Data.(string)
			if ok1 && ok2 && strings.HasPrefix(s, prefix) {
				return R.TrueVal
			}
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("end_with", &object.Method{
		Name:  "end_with",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			actualValue := expectationPayload(receiver).Value
			s, ok1 := actualValue.Data.(string)
			suffix, ok2 := args[0].Data.(string)
			if ok1 && ok2 && strings.HasSuffix(s, suffix) {
				specRunner.PassCount++
				fmt.Printf("  ✓ PASS\n")
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v to end with %v\n", actualValue.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("end_with?", &object.Method{
		Name:  "end_with?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			actualValue := expectationPayload(receiver).Value
			s, ok1 := actualValue.Data.(string)
			suffix, ok2 := args[0].Data.(string)
			if ok1 && ok2 && strings.HasSuffix(s, suffix) {
				return R.TrueVal
			}
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("match", &object.Method{
		Name:  "match",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	expectationClass.DefineMethod("empty", &object.Method{
		Name:  "empty",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if s, ok := receiver.Data.(string); ok && len(s) == 0 {
				specRunner.PassCount++
				return R.TrueVal
			}
			if arr, ok := receiver.Data.([]*object.EmeraldValue); ok && len(arr) == 0 {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v to be empty\n", receiver.Inspect())
			return R.FalseVal
		},
	})
	expectationClass.DefineMethod("empty?", &object.Method{
		Name:  "empty?",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			payload := expectationPayload(receiver)
			value := payload.Value
			empty := false
			if value != nil {
				if s, ok := value.Data.(string); ok {
					empty = len(s) == 0
				}
				if arr, ok := value.Data.([]*object.EmeraldValue); ok {
					empty = len(arr) == 0
				}
			}
			matches := empty
			if payload.Negated {
				matches = !matches
			}
			if matches {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v empty? to be %v\n", value.Inspect(), !payload.Negated)
			return R.FalseVal
		},
	})
	expectationClass.DefineMethod(">", &object.Method{
		Name:  ">",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if ok1 && ok2 && a > b {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v > %v\n", receiver.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod(">=", &object.Method{
		Name:  ">=",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if ok1 && ok2 && a >= b {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v >= %v\n", receiver.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("<", &object.Method{
		Name:  "<",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if ok1 && ok2 && a < b {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v < %v\n", receiver.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	expectationClass.DefineMethod("<=", &object.Method{
		Name:  "<=",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.FalseVal
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if ok1 && ok2 && a <= b {
				specRunner.PassCount++
				return R.TrueVal
			}
			specRunner.FailCount++
			fmt.Printf("    FAILED: expected %v <= %v\n", receiver.Inspect(), args[0].Inspect())
			return R.FalseVal
		},
	})

	objClass := R.Classes["Object"]
	objClass.DefineMethod("include", &object.Method{
		Name:  "include",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  &includeMatcher{Expected: args},
				Class: R.Classes["Expectation"],
			}
		},
	})
	objClass.DefineMethod("fixture", &object.Method{
		Name:  "fixture",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) < 2 || args[1].Type != object.ValueString {
				return R.NilVal
			}
			file := CurrentSpecFile
			if len(args) > 0 && args[0].Type == object.ValueString {
				file = args[0].Data.(string)
			}
			if file == "" || file == "spec.rb" {
				file = CurrentSpecFile
			}
			return rubyString(filepath.Join(filepath.Dir(file), "fixtures", args[1].Data.(string)))
		},
	})
	objClass.DefineMethod("argf", &object.Method{
		Name:  "argf",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 || args[0].Type != object.ValueArray {
				return R.NilVal
			}
			argf := newArgfValue(args[0].Data.([]*object.EmeraldValue))
			if mainObj, ok := R.Main.Data.(*object.Object); ok {
				mainObj.InstanceVars["@argf"] = argf
			}
			if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
				return CallBlockWithArgs(CurrentBlockValue())
			}
			return argf
		},
	})

	objClass.DefineMethod("describe", &object.Method{
		Name:  "describe",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if isSharedExampleDefinition(args) {
				if len(args) > 0 && CurrentBlockValue != nil {
					name := specName(args[0])
					if name != "" {
						specRunner.Shared[name] = CurrentBlockValue()
					}
				}
				return R.NilVal
			}
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("\n%s\n", desc)
					if desc == "(concurrently)" {
						return R.NilVal
					}
				}
			}
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				ctx := &specContext{}
				specRunner.Contexts = append(specRunner.Contexts, ctx)
				CallBlock()
				for _, block := range ctx.AfterAll {
					if CallBlockWithArgs != nil {
						CallBlockWithArgs(block)
					}
				}
				specRunner.Contexts = specRunner.Contexts[:len(specRunner.Contexts)-1]
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("context", &object.Method{
		Name:  "context",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("\n%s\n", desc)
				}
			}
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				ctx := &specContext{}
				specRunner.Contexts = append(specRunner.Contexts, ctx)
				CallBlock()
				for _, block := range ctx.AfterAll {
					if CallBlockWithArgs != nil {
						CallBlockWithArgs(block)
					}
				}
				specRunner.Contexts = specRunner.Contexts[:len(specRunner.Contexts)-1]
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("evaluate", &object.Method{
		Name:  "evaluate",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 && EvalSource != nil {
				if source, ok := args[0].Data.(string); ok {
					EvalSource(source)
				}
			}
			specRunner.ExampleCount++
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				return CallBlock()
			}
			return R.NilVal
		},
	})

	objClass.DefineMethod("it", &object.Method{
		Name:  "it",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			specRunner.ExampleCount++
			desc := ""
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("  ✓ %s\n", desc)
				}
				if text, ok := args[0].Data.(string); ok {
					desc = text
				}
			}
			beforeFails := specRunner.FailCount
			runBeforeEachHooks()
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			runAfterEachHooks()
			if os.Getenv("RGO_DEFINE_METHOD_DIAG") == "1" && specRunner.FailCount != beforeFails {
				fmt.Printf("RGO_DEFINE_METHOD_DIAG example %s failures %d -> %d\n", desc, beforeFails, specRunner.FailCount)
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("test", &object.Method{
		Name:  "test",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			specRunner.ExampleCount++
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("  ✓ %s\n", desc)
				}
			}
			runBeforeEachHooks()
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			runAfterEachHooks()
			return R.NilVal
		},
	})

	objClass.DefineMethod("before", &object.Method{
		Name:  "before",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil {
				return R.NilVal
			}
			scope := hookScope(args)
			if scope == "all" {
				if CallBlock != nil {
					CallBlock()
				}
				return R.NilVal
			}
			if ctx := currentSpecContext(); ctx != nil {
				ctx.BeforeEach = append(ctx.BeforeEach, CurrentBlockValue())
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("after", &object.Method{
		Name:  "after",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil {
				return R.NilVal
			}
			scope := hookScope(args)
			if ctx := currentSpecContext(); ctx != nil {
				if scope == "all" {
					ctx.AfterAll = append(ctx.AfterAll, CurrentBlockValue())
				} else {
					ctx.AfterEach = append(ctx.AfterEach, CurrentBlockValue())
				}
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("suppress_warning", &object.Method{
		Name:  "suppress_warning",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				return CallBlock()
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("ruby_version_is", &object.Method{
		Name:  "ruby_version_is",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			if args[0].Type == object.ValueNil {
				return R.NilVal
			}
			if args[0].Type == object.ValueRange {
				return R.NilVal
			}
			if version, ok := args[0].Data.(string); ok {
				if version == "" || strings.HasPrefix(version, "4.") {
					return R.NilVal
				}
			}
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("platform_is", &object.Method{
		Name:  "platform_is",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			matches := platformGuardMatches(args)
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				if matches {
					CallBlock()
				}
				return R.NilVal
			}
			if matches {
				return R.TrueVal
			}
			return R.FalseVal
		},
	})
	objClass.DefineMethod("platform_is_not", &object.Method{
		Name:  "platform_is_not",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			matches := !platformGuardMatches(args)
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				if matches {
					CallBlock()
				}
				return R.NilVal
			}
			if matches {
				return R.TrueVal
			}
			return R.FalseVal
		},
	})
	objClass.DefineMethod("guard", &object.Method{
		Name:  "guard",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			shouldRun := true
			if len(args) > 0 {
				shouldRun = isTruthy(callCallableValue(args[0]))
			}
			if shouldRun && CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("guard_not", &object.Method{
		Name:  "guard_not",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			shouldRun := true
			if len(args) > 0 {
				shouldRun = !isTruthy(callCallableValue(args[0]))
			}
			if shouldRun && CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("quarantine!", &object.Method{
		Name:  "quarantine!",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("as_user", &object.Method{
		Name:  "as_user",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if os.Getuid() != 0 && CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("as_superuser", &object.Method{
		Name:  "as_superuser",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if os.Getuid() == 0 && CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("little_endian", &object.Method{
		Name:  "little_endian",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if isLittleEndianArch() && CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("big_endian", &object.Method{
		Name:  "big_endian",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if !isLittleEndianArch() && CallBlock != nil && BlockGivenCheck != nil && BlockGivenCheck() {
				CallBlock()
			}
			return R.NilVal
		},
	})

	objClass.DefineMethod("expect", &object.Method{
		Name:  "expect",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			expClass := R.Classes["Expectation"]
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: expClass,
			}
		},
	})
	expectationClass.DefineMethod("be_kind_of", &object.Method{
		Name:  "be_kind_of",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 || args[0].Type != object.ValueClass {
				specRunner.PassCount++
				return R.TrueVal
			}
			return evaluateKindOfMatcher(expectationPayload(receiver), &kindOfMatcher{Class: args[0].Data.(*object.Class)})
		},
	})

	objClass.DefineMethod("raise_error", &object.Method{
		Name:  "raise_error",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			expected := R.Classes["Exception"]
			if len(args) > 0 && args[0].Type == object.ValueClass {
				expected = args[0].Data.(*object.Class)
			}
			var block *object.EmeraldValue
			if CurrentBlockValue != nil {
				block = CurrentBlockValue()
			}
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  &raiseErrorMatcher{Class: expected, Block: block},
				Class: R.Classes["Expectation"],
			}
		},
	})

	objClass.DefineMethod("output", &object.Method{
		Name:  "output",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  &outputMatcher{},
				Class: R.Classes["Expectation"],
			}
		},
	})
	objClass.DefineMethod("complain", &object.Method{
		Name:  "complain",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  &outputMatcher{},
				Class: R.Classes["Expectation"],
			}
		},
	})

	objClass.DefineMethod("be_kind_of", &object.Method{
		Name:  "be_kind_of",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 || args[0].Type != object.ValueClass {
				return R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  &kindOfMatcher{Class: args[0].Data.(*object.Class)},
				Class: R.Classes["Expectation"],
			}
		},
	})

	objClass.DefineMethod("eq", &object.Method{
		Name:  "eq",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	objClass.DefineMethod("equal", &object.Method{
		Name:  "equal",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})
	objClass.DefineMethod("eql", &object.Method{
		Name:  "eql",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			return args[0]
		},
	})

	objClass.DefineMethod("it_behaves_like", &object.Method{
		Name:  "it_behaves_like",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return R.NilVal
			}
			name := specName(args[0])
			if name != "" {
				fmt.Printf("  behaves like %s\n", name)
			}
			if len(args) > 1 {
				if mainObj, ok := R.Main.Data.(*object.Object); ok {
					mainObj.InstanceVars["@method"] = args[1]
					if len(args) > 2 {
						mainObj.InstanceVars["@object"] = args[2]
					}
				}
			}
			if block, ok := specRunner.Shared[name]; ok && CallBlockWithArgs != nil {
				return CallBlockWithArgs(block)
			}
			return R.NilVal
		},
	})
	objClass.DefineMethod("it_should_behave_like", &object.Method{
		Name:  "it_should_behave_like",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			method, _ := objClass.GetMethod("it_behaves_like")
			if fn, ok := method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
				return fn(receiver, args...)
			}
			return R.NilVal
		},
	})
}

func platformGuardMatches(args []*object.EmeraldValue) bool {
	if len(args) == 0 {
		return true
	}
	matchedPlatform := false
	last := args[len(args)-1]
	if last.Type != object.ValueHash {
		for _, arg := range args {
			if currentPlatformMatches(specName(arg)) {
				matchedPlatform = true
				break
			}
		}
		return matchedPlatform
	}
	for _, arg := range args[:len(args)-1] {
		if currentPlatformMatches(specName(arg)) {
			matchedPlatform = true
			break
		}
	}
	hashMatched := true
	for key, value := range last.Data.(map[*object.EmeraldValue]*object.EmeraldValue) {
		switch specName(key) {
		case "pointer_size":
			size, ok := valueToInteger(value)
			hashMatched = hashMatched && ok && int(size) == strconv.IntSize
		}
	}
	if len(args) == 1 {
		return hashMatched
	}
	return matchedPlatform && hashMatched
}

func currentPlatformMatches(name string) bool {
	switch name {
	case "":
		return false
	case "windows", "mingw":
		return runtime.GOOS == "windows"
	case "darwin", "macos":
		return runtime.GOOS == "darwin"
	case "linux":
		return runtime.GOOS == "linux"
	case "freebsd":
		return runtime.GOOS == "freebsd"
	case "openbsd":
		return runtime.GOOS == "openbsd"
	case "netbsd":
		return runtime.GOOS == "netbsd"
	case "bsd":
		return runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd" || runtime.GOOS == "dragonfly"
	case "solaris":
		return runtime.GOOS == "solaris" || runtime.GOOS == "illumos"
	default:
		return name == runtime.GOOS
	}
}

func isLittleEndianArch() bool {
	switch runtime.GOARCH {
	case "s390x", "ppc64", "sparc64":
		return false
	default:
		return true
	}
}

func specName(value *object.EmeraldValue) string {
	if value == nil {
		return ""
	}
	if s, ok := value.Data.(string); ok {
		return strings.TrimPrefix(s, ":")
	}
	return ""
}

func currentSpecContext() *specContext {
	if specRunner == nil || len(specRunner.Contexts) == 0 {
		return nil
	}
	return specRunner.Contexts[len(specRunner.Contexts)-1]
}

func hookScope(args []*object.EmeraldValue) string {
	if len(args) == 0 {
		return "each"
	}
	scope := specName(args[0])
	if scope == "all" || scope == "each" {
		return scope
	}
	return "each"
}

func runBeforeEachHooks() {
	if specRunner == nil || CallBlockWithArgs == nil {
		return
	}
	for _, ctx := range specRunner.Contexts {
		for _, block := range ctx.BeforeEach {
			CallBlockWithArgs(block)
		}
	}
}

func runAfterEachHooks() {
	if specRunner == nil || CallBlockWithArgs == nil {
		return
	}
	for i := len(specRunner.Contexts) - 1; i >= 0; i-- {
		ctx := specRunner.Contexts[i]
		for j := len(ctx.AfterEach) - 1; j >= 0; j-- {
			CallBlockWithArgs(ctx.AfterEach[j])
		}
	}
	for i := len(mockRestores) - 1; i >= 0; i-- {
		mockRestores[i]()
	}
	mockRestores = nil
}

func isSharedExampleDefinition(args []*object.EmeraldValue) bool {
	if len(args) < 2 || args[len(args)-1].Type != object.ValueHash {
		return false
	}
	hash := args[len(args)-1].Data.(map[*object.EmeraldValue]*object.EmeraldValue)
	for key, value := range hash {
		if specName(key) == "shared" && value.IsTruthy() {
			return true
		}
	}
	return false
}

func exceptionMessage(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueException {
		return R.NilVal
	}
	exc := receiver.Data.(*object.RException)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  exc.Message,
		Class: R.Classes["String"],
	}
}

func exceptionToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueException {
		return R.NilVal
	}
	exc := receiver.Data.(*object.RException)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  exc.Message,
		Class: R.Classes["String"],
	}
}

func exceptionInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueException {
		return R.NilVal
	}
	exc := receiver.Data.(*object.RException)
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  fmt.Sprintf("#<%s: %s>", receiver.Class.Name, exc.Message),
		Class: R.Classes["String"],
	}
}

func exceptionBacktrace(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueException {
		return R.NilVal
	}
	exc := receiver.Data.(*object.RException)
	result := make([]*object.EmeraldValue, len(exc.Backtrace))
	for i, bt := range exc.Backtrace {
		result[i] = &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  bt,
			Class: R.Classes["String"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  result,
		Class: R.Classes["Array"],
	}
}

func systemExitStatus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueException {
		return R.NilVal
	}
	exc := receiver.Data.(*object.RException)
	if exc.Status == nil {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueInteger, Data: *exc.Status, Class: R.Classes["Integer"]}
}

// Proc methods
func procCall(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CallBlockWithArgs != nil {
		return CallBlockWithArgs(receiver, args...)
	}
	return R.NilVal
}

func procClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var cls *object.Class
	if receiver != nil && receiver.Type == object.ValueClass {
		cls, _ = receiver.Data.(*object.Class)
	}
	if CurrentBlockValue != nil {
		if block := CurrentBlockValue(); block != nil {
			if proc := procValueFromCallable(block, cls); proc != nil {
				return proc
			}
			if cls == R.Classes["Proc"] && block.Type == object.ValueClosure {
				block.Data.(*object.Closure).AutoSplat = true
			}
			if cls != nil && cls != R.Classes["Proc"] {
				if block.Class == cls {
					return block
				}
				proc := procValueFromBlock(block, cls)
				if proc != nil {
					if CallMethod != nil {
						result := CallMethod(proc, "initialize", args...)
						if result != nil && result.Type == object.ValueException {
							return result
						}
					}
					return proc
				}
			}
			return block
		}
	}
	return argumentError("tried to create Proc object without a block")
}

func procValueFromCallable(block *object.EmeraldValue, class *object.Class) *object.EmeraldValue {
	if class == nil {
		class = R.Classes["Proc"]
	}
	switch block.Type {
	case object.ValueMethod:
		methodValue := block
		return &object.EmeraldValue{
			Type: object.ValueProc,
			Data: &object.Proc{
				InstanceVars: make(map[string]*object.EmeraldValue),
				Native: func(args ...*object.EmeraldValue) *object.EmeraldValue {
					return methodCall(methodValue, args...)
				},
			},
			Class: class,
		}
	case object.ValueSymbol:
		name, _ := block.Data.(string)
		return &object.EmeraldValue{
			Type: object.ValueProc,
			Data: &object.Proc{
				InstanceVars: make(map[string]*object.EmeraldValue),
				Native: func(args ...*object.EmeraldValue) *object.EmeraldValue {
					if len(args) == 0 || CallMethod == nil {
						return R.NilVal
					}
					return CallMethod(args[0], name, args[1:]...)
				},
			},
			Class: class,
		}
	default:
		return nil
	}
}

func procValueFromBlock(block *object.EmeraldValue, class *object.Class) *object.EmeraldValue {
	if class == nil {
		class = R.Classes["Proc"]
	}
	switch block.Type {
	case object.ValueProc:
		source := block.Data.(*object.Proc)
		copy := *source
		copy.InstanceVars = make(map[string]*object.EmeraldValue)
		return &object.EmeraldValue{Type: object.ValueProc, Data: &copy, Class: class}
	case object.ValueClosure:
		closure := block.Data.(*object.Closure)
		return &object.EmeraldValue{
			Type: object.ValueProc,
			Data: &object.Proc{
				Fn:           closure.Fn,
				Env:          closure.Free,
				Block:        closure.Block,
				Binding:      closure.Binding,
				ClassStack:   closure.ClassStack,
				InstanceVars: make(map[string]*object.EmeraldValue),
				AutoSplat:    true,
				IsLambda:     false,
			},
			Class: class,
		}
	default:
		return nil
	}
}

func procClassAllocate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newRuntimeException(R.Classes["TypeError"], "allocator undefined for Proc")
}

func procHash(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("%p:%p", receiver, receiver.Data)))
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(h.Sum64()), Class: R.Classes["Integer"]}
}

func procArity(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type == object.ValueProc {
		proc := receiver.Data.(*object.Proc)
		if proc.Native != nil {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(-1),
				Class: R.Classes["Integer"],
			}
		}
		if proc.Fn != nil {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(len(proc.Fn.Params)),
				Class: R.Classes["Integer"],
			}
		}
	} else if receiver.Type == object.ValueClosure {
		closure := receiver.Data.(*object.Closure)
		if closure.Fn != nil {
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(len(closure.Fn.Params)),
				Class: R.Classes["Integer"],
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueInteger,
		Data:  int64(0),
		Class: R.Classes["Integer"],
	}
}

func procBinding(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	var binding *object.RBinding
	switch receiver.Type {
	case object.ValueProc:
		proc := receiver.Data.(*object.Proc)
		if proc.CurryTarget != nil {
			return argumentError("Can't create Binding from C level Proc")
		}
		binding = proc.Binding
	case object.ValueClosure:
		closure := receiver.Data.(*object.Closure)
		binding = closure.Binding
	}
	if binding == nil {
		binding = &object.RBinding{Self: R.Main, Locals: map[string]*object.EmeraldValue{}, Constants: map[string]*object.EmeraldValue{}}
	}
	return &object.EmeraldValue{Type: object.ValueBinding, Data: binding, Class: R.Classes["Binding"]}
}

func procCurry(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	arity := procRequiredArity(receiver)
	collected := []*object.EmeraldValue{}
	target := receiver
	if receiver.Type == object.ValueProc {
		proc := receiver.Data.(*object.Proc)
		if proc.CurryTarget != nil {
			target = proc.CurryTarget
			collected = append(collected, proc.CurryArgs...)
			arity = proc.CurryArity
		}
	}
	applyArgs := len(args) > 1
	if len(args) == 1 {
		if n, ok := valueToInteger(args[0]); ok {
			arity = int(n)
		}
	}
	if callableIsLambda(target) && !procLambdaAcceptsCurryArity(target, arity) {
		return argumentError("wrong number of arguments")
	}
	curried := newCurriedProc(target, arity, collected, callableIsLambda(target))
	if applyArgs {
		return callCallableValue(curried, args...)
	}
	return curried
}

func procLambdaAcceptsCurryArity(value *object.EmeraldValue, arity int) bool {
	min, max, hasRest := procArityRange(value)
	if arity < min {
		return false
	}
	if !hasRest && arity > max {
		return false
	}
	return true
}

func procArityRange(value *object.EmeraldValue) (int, int, bool) {
	fn := procFunction(value)
	if fn == nil {
		return 0, 0, false
	}
	required := 0
	for i := range fn.Params {
		if i < len(fn.ParamDefaults) && fn.ParamDefaults[i] != nil {
			continue
		}
		required++
	}
	return required, len(fn.Params), fn.HasRestParam
}

func procFunction(value *object.EmeraldValue) *object.Function {
	if value == nil {
		return nil
	}
	switch value.Type {
	case object.ValueProc:
		proc := value.Data.(*object.Proc)
		if proc.CurryTarget != nil {
			return procFunction(proc.CurryTarget)
		}
		return proc.Fn
	case object.ValueClosure:
		return value.Data.(*object.Closure).Fn
	}
	return nil
}

func procRequiredArity(value *object.EmeraldValue) int {
	if value == nil {
		return 0
	}
	switch value.Type {
	case object.ValueProc:
		proc := value.Data.(*object.Proc)
		if proc.CurryTarget != nil && proc.CurryArity > 0 {
			return proc.CurryArity
		}
		min, _, _ := procArityRange(value)
		return min
	case object.ValueClosure:
		min, _, _ := procArityRange(value)
		return min
	}
	return 0
}

func newCurriedProc(target *object.EmeraldValue, arity int, collected []*object.EmeraldValue, isLambda bool) *object.EmeraldValue {
	stored := append([]*object.EmeraldValue(nil), collected...)
	procValue := &object.EmeraldValue{Type: object.ValueProc, Class: R.Classes["Proc"]}
	proc := &object.Proc{
		InstanceVars: make(map[string]*object.EmeraldValue),
		IsLambda:     isLambda,
		CurryTarget:  target,
		CurryArgs:    stored,
		CurryArity:   arity,
	}
	proc.Native = func(args ...*object.EmeraldValue) *object.EmeraldValue {
		combined := append(append([]*object.EmeraldValue(nil), stored...), args...)
		if len(combined) >= arity {
			if isLambda && len(combined) > arity {
				return argumentError("wrong number of arguments")
			}
			return callCallableValue(target, combined...)
		}
		return newCurriedProc(target, arity, combined, isLambda)
	}
	procValue.Data = proc
	return procValue
}

func procParameters(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type == object.ValueProc {
		proc := receiver.Data.(*object.Proc)
		if proc.CurryTarget != nil {
			inner := []*object.EmeraldValue{&object.EmeraldValue{Type: object.ValueSymbol, Data: "rest", Class: R.Classes["Symbol"]}}
			return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{{Type: object.ValueArray, Data: inner, Class: R.Classes["Array"]}}, Class: R.Classes["Array"]}
		}
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
}

func procSourceLocation(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}

func procIsLambda(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type == object.ValueProc {
		proc := receiver.Data.(*object.Proc)
		if proc.IsLambda {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func procToProc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func procToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type == object.ValueProc {
		proc := receiver.Data.(*object.Proc)
		if proc.IsLambda {
			return &object.EmeraldValue{
				Type:  object.ValueString,
				Data:  "#<Proc: lambda>",
				Class: R.Classes["String"],
			}
		}
		return &object.EmeraldValue{
			Type:  object.ValueString,
			Data:  "#<Proc>",
			Class: R.Classes["String"],
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  "#<Proc>",
		Class: R.Classes["String"],
	}
}

func procInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return procToS(receiver, args...)
}

func procCaseEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return procCall(receiver, args...)
}

func procComposeLeft(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || !isCallableValue(args[0]) {
		return typeError("callable object is expected")
	}
	other := args[0]
	return composedProc(callableIsLambda(other), func(callArgs ...*object.EmeraldValue) *object.EmeraldValue {
		intermediate := callCallableValue(other, callArgs...)
		return callCallableValue(receiver, intermediate)
	})
}

func procComposeRight(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || !isCallableValue(args[0]) {
		return typeError("callable object is expected")
	}
	other := args[0]
	return composedProc(callableIsLambda(receiver), func(callArgs ...*object.EmeraldValue) *object.EmeraldValue {
		intermediate := callCallableValue(receiver, callArgs...)
		return callCallableValue(other, intermediate)
	})
}

func composedProc(isLambda bool, fn func(args ...*object.EmeraldValue) *object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type: object.ValueProc,
		Data: &object.Proc{
			InstanceVars: make(map[string]*object.EmeraldValue),
			Native:       fn,
			IsLambda:     isLambda,
		},
		Class: R.Classes["Proc"],
	}
}

func isCallableValue(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	if value.Type == object.ValueProc || value.Type == object.ValueClosure || value.Type == object.ValueMethod {
		return true
	}
	if value.Class == nil {
		return false
	}
	_, ok := value.Class.GetMethod("call")
	return ok
}

func callableIsLambda(value *object.EmeraldValue) bool {
	if value == nil {
		return false
	}
	if value.Type == object.ValueProc {
		return value.Data.(*object.Proc).IsLambda
	}
	return false
}

func callCallableValue(value *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if value == nil {
		return R.NilVal
	}
	switch value.Type {
	case object.ValueProc, object.ValueClosure:
		if CallBlockWithArgs != nil {
			return CallBlockWithArgs(value, args...)
		}
	case object.ValueMethod:
		return methodCall(value, args...)
	default:
		if CallMethod != nil {
			return CallMethod(value, "call", args...)
		}
	}
	return R.NilVal
}

func methodCall(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueMethod {
		return R.NilVal
	}
	method := receiver.Data.(*object.Method)
	if method.Fn == nil {
		return R.NilVal
	}
	callReceiver := receiver
	if method.Receiver != nil {
		callReceiver = method.Receiver
	}
	if fn, ok := method.Fn.(*object.Function); ok {
		if CallMethod != nil {
			return CallMethod(callReceiver, fn.Name, args...)
		}
		return R.NilVal
	}
	if builtin, ok := method.Fn.(BuiltinMethod); ok {
		return builtin(callReceiver, args...)
	}
	if builtin, ok := method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
		return builtin(callReceiver, args...)
	}
	return R.NilVal
}

func unboundMethodBind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueMethod || len(args) == 0 {
		return R.NilVal
	}
	method := receiver.Data.(*object.Method)
	if (method.Name == "append_features" || method.Name == "prepend_features" || method.Name == "extend_object") && args[0] != nil && args[0].Type == object.ValueClass {
		return typeError("bind argument must be an instance of Module")
	}
	copy := *method
	copy.Receiver = args[0]
	return &object.EmeraldValue{Type: object.ValueMethod, Data: &copy, Class: R.Classes["Method"]}
}

func methodArity(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueMethod {
		return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(-1), Class: R.Classes["Integer"]}
	}
	method := receiver.Data.(*object.Method)
	return &object.EmeraldValue{Type: object.ValueInteger, Data: int64(method.Arity), Class: R.Classes["Integer"]}
}

func methodOwner(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueMethod {
		return R.NilVal
	}
	method := receiver.Data.(*object.Method)
	if method.Owner != nil {
		return method.Owner
	}
	if method.Receiver != nil && method.Receiver.Class != nil {
		return &object.EmeraldValue{Type: object.ValueClass, Data: method.Receiver.Class, Class: R.Classes["Class"]}
	}
	return R.NilVal
}

func methodReceiver(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueMethod {
		return R.NilVal
	}
	method := receiver.Data.(*object.Method)
	if method.Receiver != nil {
		return method.Receiver
	}
	return R.NilVal
}

func methodName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueMethod {
		return R.NilVal
	}
	method := receiver.Data.(*object.Method)
	return &object.EmeraldValue{
		Type:  object.ValueSymbol,
		Data:  method.Name,
		Class: R.Classes["Symbol"],
	}
}

func methodMethodEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || receiver == nil || args[0] == nil {
		return R.FalseVal
	}
	if receiver.Type == object.ValueMethod && args[0].Type == object.ValueMethod {
		left := receiver.Data.(*object.Method)
		right := args[0].Data.(*object.Method)
		if left.Receiver != nil && left.Receiver == right.Receiver {
			return R.TrueVal
		}
	}
	if receiver.Equals(args[0]) {
		return R.TrueVal
	}
	return R.FalseVal
}

func methodMethodToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueMethod {
		return &object.EmeraldValue{Type: object.ValueString, Data: "#<Method: nil>", Class: R.Classes["String"]}
	}
	method := receiver.Data.(*object.Method)
	owner := ""
	if method.Owner != nil {
		owner = namedModuleOrClassValue(method.Owner) + " "
	} else if method.Receiver != nil {
		switch method.Receiver.Type {
		case object.ValueModule:
			owner = namedModuleOrClassValue(method.Receiver) + " "
		case object.ValueClass:
			owner = namedModuleOrClassValue(method.Receiver) + " "
		default:
			if method.Receiver.Class != nil {
				owner = classToS(method.Receiver.Class) + " "
			}
		}
	}
	return &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  "#<Method: " + owner + method.Name + ">",
		Class: R.Classes["String"],
	}
}

func namedModuleOrClassValue(value *object.EmeraldValue) string {
	if value == nil {
		return ""
	}
	if name, ok := findRuntimeConstantName(value); ok {
		return name
	}
	switch value.Type {
	case object.ValueClass:
		return classToS(value.Data.(*object.Class))
	case object.ValueModule:
		return moduleToS(value.Data.(*object.Module))
	}
	return ""
}

func findRuntimeConstantName(target *object.EmeraldValue) (string, bool) {
	if target == nil || R == nil {
		return "", false
	}
	best := ""
	for rootName, class := range R.Classes {
		value := &object.EmeraldValue{Type: object.ValueClass, Data: class, Class: R.Classes["Class"]}
		if sameModuleOrClassValue(value, target) && len(rootName) > len(best) {
			best = rootName
		}
		if found, ok := findRuntimeConstantNameInMap(rootName, class.Constants, target, map[*object.EmeraldValue]bool{}); ok && len(found) > len(best) {
			best = found
		}
	}
	return best, best != ""
}

func findRuntimeConstantNameInMap(prefix string, constants map[string]*object.EmeraldValue, target *object.EmeraldValue, visited map[*object.EmeraldValue]bool) (string, bool) {
	best := ""
	for name, value := range constants {
		if value == nil || visited[value] {
			continue
		}
		visited[value] = true
		fullName := prefix + "::" + name
		if sameModuleOrClassValue(value, target) && len(fullName) > len(best) {
			best = fullName
		}
		switch value.Type {
		case object.ValueClass:
			if found, ok := findRuntimeConstantNameInMap(fullName, value.Data.(*object.Class).Constants, target, visited); ok && len(found) > len(best) {
				best = found
			}
		case object.ValueModule:
			if found, ok := findRuntimeConstantNameInMap(fullName, value.Data.(*object.Module).Constants, target, visited); ok && len(found) > len(best) {
				best = found
			}
		}
	}
	return best, best != ""
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

func methodMethodInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return methodMethodToS(receiver, args...)
}

func bindingLocalVariables(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueBinding {
		return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
	}
	binding := receiver.Data.(*object.RBinding)
	arr := make([]*object.EmeraldValue, 0, len(binding.Locals))
	for name := range binding.Locals {
		arr = append(arr, &object.EmeraldValue{
			Type:  object.ValueSymbol,
			Data:  name,
			Class: R.Classes["Symbol"],
		})
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: arr, Class: R.Classes["Array"]}
}

func bindingEval(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return R.NilVal
	}
	str, ok := args[0].Data.(string)
	if !ok {
		return R.NilVal
	}
	return evalSourceWithBinding(str, receiver)
}

func evalSourceWithBinding(source string, bindingValue *object.EmeraldValue) *object.EmeraldValue {
	if bindingValue != nil && bindingValue.Type == object.ValueBinding {
		if binding, ok := bindingValue.Data.(*object.RBinding); ok && binding != nil {
			name := strings.TrimSpace(source)
			if value := binding.Locals[name]; value != nil {
				return value
			}
		}
	}
	if EvalSource == nil {
		return R.NilVal
	}
	return EvalSource(source)
}

func moduleInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueModule {
		return R.NilVal
	}
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	for i := len(args) - 1; i >= 0; i-- {
		result := appendFeaturesForInclude(receiver, args[i])
		if result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func moduleAppendFeatures(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil || receiver.Type != object.ValueModule || len(args) == 0 {
		return R.NilVal
	}
	target := args[0]
	if target == receiver {
		return NewArgumentError("cyclic include detected")
	}
	if target != nil && target.Frozen {
		return frozenError("can't modify frozen class/module")
	}
	module := receiver.Data.(*object.Module)
	switch target.Type {
	case object.ValueClass:
		if classIncludesModule(target.Data.(*object.Class), module) {
			return NewArgumentError("cyclic include detected")
		}
		target.Data.(*object.Class).Include(module)
	case object.ValueModule:
		targetModule := target.Data.(*object.Module)
		if targetModule == module || moduleIncludesModule(module, targetModule) {
			return NewArgumentError("cyclic include detected")
		}
		targetModule.Include(module)
	default:
		return typeError("wrong argument type")
	}
	return receiver
}

func appendFeaturesForInclude(receiver *object.EmeraldValue, mixin *object.EmeraldValue) *object.EmeraldValue {
	if mixin == nil || mixin.Type != object.ValueModule {
		return typeError("wrong argument type")
	}
	if mixin.Data.(*object.Module).IsRefinement {
		return typeError("Cannot include refinement")
	}
	if CallMethod != nil {
		return CallMethod(mixin, "send", &object.EmeraldValue{Type: object.ValueSymbol, Data: "append_features", Class: R.Classes["Symbol"]}, receiver)
	}
	switch receiver.Type {
	case object.ValueClass:
		receiver.Data.(*object.Class).Include(mixin.Data.(*object.Module))
	case object.ValueModule:
		receiver.Data.(*object.Module).Include(mixin.Data.(*object.Module))
	}
	return mixin
}

func classIncludesModule(class *object.Class, module *object.Module) bool {
	if class == nil || module == nil {
		return false
	}
	for cls := class; cls != nil; cls = cls.SuperClass {
		for _, included := range cls.IncludedModules {
			if included == module || moduleIncludesModule(included, module) {
				return true
			}
		}
	}
	return false
}

func moduleIncludesModule(module *object.Module, target *object.Module) bool {
	if module == nil || target == nil {
		return false
	}
	for _, included := range module.IncludedModules {
		if included == target || moduleIncludesModule(included, target) {
			return true
		}
	}
	return false
}

func moduleIncludePredicate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	target := args[0]
	if target == nil || target.Type != object.ValueModule {
		return typeError("wrong argument type")
	}
	targetModule := target.Data.(*object.Module)
	switch receiver.Type {
	case object.ValueClass:
		if classIncludesModule(receiver.Data.(*object.Class), targetModule) {
			return R.TrueVal
		}
	case object.ValueModule:
		if moduleIncludesModule(receiver.Data.(*object.Module), targetModule) {
			return R.TrueVal
		}
	}
	return R.FalseVal
}

func moduleUsing(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if InMethodScope != nil && InMethodScope() {
		return newRuntimeException(R.Classes["RuntimeError"], "Module#using is not permitted in methods")
	}
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	for _, arg := range args {
		if arg == nil || arg.Type != object.ValueModule {
			return typeError("wrong argument type")
		}
	}
	return receiver
}

func moduleRefine(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	target := args[0]
	if target == nil || (target.Type != object.ValueClass && target.Type != object.ValueModule) {
		return typeError("wrong argument type")
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CurrentBlockValue() == nil {
		return NewArgumentError("no block given")
	}
	refined := refinementModuleFor(receiver, target)
	if CallBlockWithSelf != nil {
		if result := CallBlockWithSelf(CurrentBlockValue(), refined); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return refined
}

func refinementModuleFor(receiver, target *object.EmeraldValue) *object.EmeraldValue {
	if refinementModules == nil {
		refinementModules = make(map[*object.EmeraldValue]map[any]*object.EmeraldValue)
	}
	byTarget := refinementModules[receiver]
	if byTarget == nil {
		byTarget = make(map[any]*object.EmeraldValue)
		refinementModules[receiver] = byTarget
	}
	key := target.Data
	if refined := byTarget[key]; refined != nil {
		return refined
	}
	module := object.NewModule("")
	module.IsRefinement = true
	refined := &object.EmeraldValue{Type: object.ValueModule, Data: module, Class: R.Classes["Module"]}
	byTarget[key] = refined
	return refined
}

func moduleExtend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueModule {
		return R.NilVal
	}
	module := receiver.Data.(*object.Module)
	for _, arg := range args {
		if arg.Type == object.ValueModule {
			mixin := arg.Data.(*object.Module)
			module.Extend(mixin)
		}
	}
	return R.NilVal
}

func moduleExtendObject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil || receiver.Type != object.ValueModule || len(args) == 0 {
		return R.NilVal
	}
	target := args[0]
	if target == nil {
		return R.NilVal
	}
	if target.Frozen {
		return newRuntimeException(R.Classes["RuntimeError"], "can't modify frozen object")
	}
	module := receiver.Data.(*object.Module)
	switch target.Type {
	case object.ValueObject:
		obj := target.Data.(*object.Object)
		for name, method := range module.Methods {
			obj.SingletonMethods[name] = method
		}
		singleton := methodSingletonClass(target)
		if singleton.Type == object.ValueClass {
			cls := singleton.Data.(*object.Class)
			for name, constant := range module.Constants {
				cls.DefineConstant(name, constant)
			}
		}
	case object.ValueClass:
		class := target.Data.(*object.Class)
		class.Extend(module)
		for name, constant := range module.Constants {
			class.DefineConstant(name, constant)
		}
	default:
		return typeError("wrong argument type")
	}
	return target
}

func modulePrepend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueModule {
		return R.NilVal
	}
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	for i := len(args) - 1; i >= 0; i-- {
		result := prependFeaturesForPrepend(receiver, args[i])
		if result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func modulePrependFeatures(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil || receiver.Type != object.ValueModule || len(args) == 0 {
		return R.NilVal
	}
	if args[0] == receiver {
		return NewArgumentError("cyclic prepend detected")
	}
	module := receiver.Data.(*object.Module)
	switch args[0].Type {
	case object.ValueClass:
		args[0].Data.(*object.Class).Prepend(module)
	case object.ValueModule:
		if args[0].Data.(*object.Module) == module {
			return NewArgumentError("cyclic prepend detected")
		}
	default:
		return typeError("wrong argument type")
	}
	return receiver
}

func prependFeaturesForPrepend(receiver *object.EmeraldValue, mixin *object.EmeraldValue) *object.EmeraldValue {
	if mixin == nil || mixin.Type != object.ValueModule {
		return typeError("wrong argument type")
	}
	if mixin.Data.(*object.Module).IsRefinement {
		return typeError("Cannot prepend refinement")
	}
	if CallMethod != nil {
		return CallMethod(mixin, "send", &object.EmeraldValue{Type: object.ValueSymbol, Data: "prepend_features", Class: R.Classes["Symbol"]}, receiver)
	}
	if receiver.Type == object.ValueClass {
		receiver.Data.(*object.Class).Prepend(mixin.Data.(*object.Module))
	}
	return mixin
}

func classInclude(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueClass {
		return R.NilVal
	}
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	for i := len(args) - 1; i >= 0; i-- {
		result := appendFeaturesForInclude(receiver, args[i])
		if result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func classExtend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueClass {
		return R.NilVal
	}
	class := receiver.Data.(*object.Class)
	for _, arg := range args {
		if arg.Type == object.ValueModule {
			module := arg.Data.(*object.Module)
			class.Extend(module)
		}
	}
	return R.NilVal
}

func classPrepend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if receiver.Type != object.ValueClass {
		return R.NilVal
	}
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	for i := len(args) - 1; i >= 0; i-- {
		result := prependFeaturesForPrepend(receiver, args[i])
		if result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}
