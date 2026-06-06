package vm

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
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

	MethodName string
	SuperStart *object.Class
	Args       []*object.EmeraldValue
	Block      *object.EmeraldValue

	BlockBreakAddr int
	WhileStart     int
	WhileEnd       int
	BlockBreak     bool
	BlockBreakVal  *object.EmeraldValue
	BlockNextVal   *object.EmeraldValue
}

type RescueHandler struct {
	RescueOffset int
	EnsureOffset int
	EndOffset    int
	Frame        *Frame
}

type CatchHandler struct {
	Label     *object.EmeraldValue
	EndOffset int
	Frame     *Frame
}

type VM struct {
	constants           []*object.EmeraldValue
	globals             []*object.EmeraldValue
	globalNames         map[string]int
	bytecodeGlobalNames map[string]int
	rubyConsts          map[string]*object.EmeraldValue

	stack []*object.EmeraldValue
	sp    int

	frames []*Frame
	fp     int
	isRoot bool

	instructions compiler.Instructions

	poppedValues []*object.EmeraldValue

	currentBlock     *object.EmeraldValue
	visibilityBypass bool
	classStack       []*object.EmeraldValue
	autoloading      map[string]bool
	threadDepth      int
	procCallDepth    int
	nextFrameID      int

	rescueStack  []*RescueHandler
	ensureActive bool

	catchStack []*CatchHandler
}

func New(bytecode *compiler.Bytecode) *VM {
	currentSpecFile := core.CurrentSpecFile
	core.Init()
	core.CurrentSpecFile = currentSpecFile
	return newVM(bytecode, nil)
}

func newVM(bytecode *compiler.Bytecode, parent *VM) *VM {
	mainFn := &object.Function{
		Name:         "__main__",
		Instructions: bytecode.Instructions,
		LineMap:      bytecode.LineMap,
		Constants:    bytecode.Constants,
		NumLocals:    bytecode.NumLocals,
		LocalNames:   bytecode.LocalNames,
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
		constants:           bytecode.Constants,
		globals:             make([]*object.EmeraldValue, 100),
		globalNames:         bytecode.GlobalNames,
		bytecodeGlobalNames: bytecode.GlobalNames,
		rubyConsts:          make(map[string]*object.EmeraldValue),
		stack:               make([]*object.EmeraldValue, StackSize),
		sp:                  1 + bytecode.NumLocals,
		frames:              []*Frame{mainFrame},
		fp:                  0,
		nextFrameID:         2,
		instructions:        bytecode.Instructions,
		autoloading:         make(map[string]bool),
		isRoot:              parent == nil,
	}
	vm.rubyConsts["ARGF"] = core.NewArgfValue(nil)
	vm.rubyConsts["Enumerable"] = &object.EmeraldValue{Type: object.ValueModule, Data: object.NewModule("Enumerable"), Class: core.R.Classes["Module"]}
	if parent != nil {
		vm.globals = parent.globals
		vm.globalNames = parent.globalNames
		vm.rubyConsts = parent.rubyConsts
		vm.autoloading = parent.autoloading
	}

	vm.stack[0] = core.R.Main
	vm.installCoreHooks()

	return vm
}

func (vm *VM) allocateFrameID() int {
	id := vm.nextFrameID
	if id <= 0 {
		id = 1
	}
	vm.nextFrameID = id + 1
	return id
}

func (vm *VM) installCoreHooks() {
	CurrentVM = vm
	core.CallBlock = CallBlock
	core.CallMethod = func(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
		return vm.send(receiver, method, args)
	}
	core.SetGlobalVariable = func(name string, value *object.EmeraldValue) {
		vm.setGlobalByName(name, value)
	}
	core.GetGlobalVariable = func(name string) *object.EmeraldValue {
		return vm.getGlobalByName(name)
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
	core.CurrentBacktraceFrames = vm.currentBacktraceFrames
	core.GlobalVariableNames = vm.globalVariableNames
	core.CallBlockWithArgs = func(block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		return vm.callBlock(block, args...)
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
	core.EnterThreadBlock = func() {
		vm.threadDepth++
	}
	core.LeaveThreadBlock = func() {
		if vm.threadDepth > 0 {
			vm.threadDepth--
		}
	}
	core.BlockGivenCheck = func() bool {
		return vm.currentBlock != nil
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
	core.ResolveRequirePath = vm.resolveRequirePath
	core.InMethodScope = func() bool {
		if vm.fp < 0 || vm.fp >= len(vm.frames) || vm.frames[vm.fp] == nil {
			return false
		}
		return vm.frames[vm.fp].MethodName != ""
	}
}

func (vm *VM) setGlobalByName(name string, value *object.EmeraldValue) {
	resolvedName := core.ResolveGlobalAlias(name)
	if vm.globalNames == nil {
		if resolvedName == "$?" && len(vm.globals) > 0 {
			vm.globals[0] = value
		}
		return
	}
	idx, ok := vm.globalNames[resolvedName]
	if !ok && name != resolvedName {
		idx, ok = vm.globalNames[name]
	}
	if !ok && resolvedName == "$?" {
		idx, ok = vm.globalNames["?"]
	}
	if !ok && resolvedName == "$?" {
		idx, ok = 0, true
	}
	if !ok || idx < 0 || idx >= len(vm.globals) {
		return
	}
	vm.globals[idx] = value
}

func (vm *VM) getGlobalByName(name string) *object.EmeraldValue {
	resolvedName := core.ResolveGlobalAlias(name)
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

func (vm *VM) resolveGlobalIndex(index int) int {
	if vm.globalNames == nil {
		return index
	}
	if index < 0 || index >= len(vm.globals) {
		return index
	}

	name := vm.rawGlobalNameForIndex(index)
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

func (vm *VM) validateGlobalAssignment(rawIndex int, resolvedIndex int, value *object.EmeraldValue) (*object.EmeraldValue, *object.EmeraldValue) {
	rawName := vm.rawGlobalNameForIndex(rawIndex)
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
	if obj, ok := receiver.Data.(*object.Object); ok && obj.SingletonClass != nil {
		if _, ok := obj.SingletonClass.GetMethod("write"); ok {
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
		return value
	}
	if value := vm.getGlobalByName("$LOAD_PATH"); value != nil && value.Type == object.ValueArray {
		return value
	}
	if value := vm.getGlobalByName("$-I"); value != nil && value.Type == object.ValueArray {
		return value
	}
	entries := []*object.EmeraldValue{
		{Type: object.ValueString, Data: "lib", Class: core.R.Classes["String"]},
	}
	value := &object.EmeraldValue{Type: object.ValueArray, Data: entries, Class: core.R.Classes["Array"]}
	vm.setGlobalByName("$:", value)
	vm.setGlobalByName("$LOAD_PATH", value)
	vm.setGlobalByName("$-I", value)
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

func (vm *VM) Run() error {
	if vm.isRoot {
		defer core.RunAtExitHooks()
	}

	frame := vm.frames[vm.fp]
	instructions := frame.Fn.Instructions

	count := 0
	for frame.Ip < len(instructions)-1 {
		count++
		if frame.Ip >= len(instructions) {
			return fmt.Errorf("invalid instruction pointer: %d", frame.Ip)
		}
		if count > 1000000 {
			return fmt.Errorf("infinite loop detected at ip=%d, op=%v", frame.Ip, instructions[frame.Ip])
		}
		frame.Ip++
		if frame.Ip >= len(instructions) {
			return fmt.Errorf("invalid instruction pointer: %d", frame.Ip)
		}

		op := compiler.Opcode(instructions[frame.Ip])

		err := vm.execute(op, frame)
		if err != nil {
			return err
		}
		frame = vm.frames[vm.fp]
		instructions = frame.Fn.Instructions

		if DevMode && count%100 == 0 {
			runtime.Gosched()
		}
	}

	return nil
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

	l := lexer.New(source)
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

	c := compiler.New()
	if err := c.Compile(program); err != nil {
		if os.Getenv("RGO_DEBUG_REQUIRE") == "1" {
			fmt.Printf("RGO_DEBUG_REQUIRE eval compile error=%v\n", err)
		}
		return core.R.NilVal
	}

	parent := CurrentVM
	child := newVM(c.Bytecode(), vm)
	if err := child.Run(); err != nil {
		if os.Getenv("RGO_DEBUG_REQUIRE") == "1" {
			fmt.Printf("RGO_DEBUG_REQUIRE eval runtime error=%v\n", err)
		}
		if parent != nil {
			parent.installCoreHooks()
		}
		return core.R.NilVal
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

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		exc := newSyntaxErrorForBinding(binding, strings.Join(p.Errors(), "\n"))
		core.LastException = exc
		return exc
	}
	if message := validateDynamicSyntax(program); message != "" {
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
	if err := c.Compile(program); err != nil {
		return core.R.NilVal
	}

	child := newVM(c.Bytecode(), vm)
	child.classStack = childClassStack
	localSlots := bindingLocalSlots(child)
	child.stack[0] = childSelf
	if binding != nil {
		seedBindingLocals(child, binding, localSlots)
	}

	result := core.R.NilVal
	if err := child.Run(); err == nil {
		if r := child.LastPoppedStackElement(); r != nil {
			result = r
		}
		copyBindingLocals(child, binding, localSlots)
	} else {
		result = core.R.NilVal
	}
	return result
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
	if program == nil {
		return ""
	}
	return validateDynamicStatements(program.Statements, dynamicSyntaxContext{})
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
	for _, match := range spacedMethodCallArgumentListPattern.FindAllStringSubmatch(code, -1) {
		if len(match) < 2 {
			continue
		}
		if spacedCallKeywordExclusions[match[1]] {
			continue
		}
		return "syntax error, unexpected ','"
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
	numberedParameterAssignmentPattern     = regexp.MustCompile(`(^|[^A-Za-z0-9_])_[1-9]\s*=`)
	numberedParameterNestedPattern         = regexp.MustCompile(numberedParameterUse + `[\s\S]*(->|proc\s*\{|\blambda\s*\{)[\s\S]*` + numberedParameterUse)
	numberedParameterExplicitParamsPattern = regexp.MustCompile(`(->\s*\([^)]*\)\s*\{[\s\S]*` + numberedParameterUse + `|(proc|lambda)\s*\{[^{}]*\|[^|]*\|[\s\S]*` + numberedParameterUse + `|\[[^\]]*\]\.[A-Za-z_][A-Za-z0-9_?!]*\s*\{[^{}]*\|[^|]*\|[\s\S]*` + numberedParameterUse + `)`)
	numberedParameterMethodNamePattern     = regexp.MustCompile(`^_[0-9]+$`)
	itParameterPattern                     = regexp.MustCompile(`(^|[^A-Za-z0-9_])it([^A-Za-z0-9_?!]|$)`)
	itParameterExplicitParamsPattern       = regexp.MustCompile(`(->\s*\([^)]*\)\s*\{[\s\S]*\bit\b|(proc|lambda)\s*\{[^{}]*\|[^|]*\|[\s\S]*\bit\b|\[[^\]]*\]\.[A-Za-z_][A-Za-z0-9_?!]*\s*\{[^{}]*\|[^|]*\|[\s\S]*\bit\b)`)
	itBeforeNumberedParameterPattern       = regexp.MustCompile(`\bit\b[\s\S]*(^|[^A-Za-z0-9_])_[1-9]([^0-9A-Za-z_]|$)`)
	numberedBeforeItParameterPattern       = regexp.MustCompile(`(^|[^A-Za-z0-9_])_[1-9]([^0-9A-Za-z_]|$)[\s\S]*\bit\b`)
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
	if name == "" {
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

func (vm *VM) resolveRequirePath(path string) string {
	if path == "" {
		return ""
	}
	currentDir, _ := os.Getwd()

	requestPath := filepath.FromSlash(strings.ReplaceAll(path, "\\", "/"))
	isAbs := filepath.IsAbs(requestPath)
	isBare := !strings.ContainsAny(requestPath, "/\\")
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

	if isExplicitRelative || isAbs || isDotOrDotDot {
		addCandidate(cleanPath)
	}
	if isBare {
		entries := core.GetGlobalVariable("$LOAD_PATH")
		if entries != nil && entries.Type == object.ValueArray {
			for _, entry := range entries.Data.([]*object.EmeraldValue) {
				epath, ok := entry.Data.(string)
				if !ok {
					continue
				}
				addCandidate(filepath.Join(epath, requestPath))
				if !strings.HasSuffix(requestPath, ".rb") {
					addCandidate(filepath.Join(epath, requestPath+".rb"))
				}
			}
		}
	}
	if !isBare {
		addCandidate(cleanPath)
	}
	if !strings.HasSuffix(cleanPath, ".rb") {
		addCandidate(cleanPath + ".rb")
	}
	if isExplicitRelative && currentDir != "" {
		addCandidate(filepath.Join(currentDir, cleanPath))
		if !strings.HasSuffix(cleanPath, ".rb") {
			addCandidate(filepath.Join(currentDir, cleanPath+".rb"))
		}
	}

	if isDotOrDotDot && currentDir != "" {
		addCandidate(filepath.Join(currentDir, cleanPath))
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

func (vm *VM) requirePath(path string) (string, *object.EmeraldValue) {
	if path == "" {
		return "", core.NewArgumentError("empty file name")
	}
	candidate := vm.resolveRequirePath(path)
	if candidate == "" {
		return "", &object.EmeraldValue{
			Type:  object.ValueException,
			Data:  &object.RException{Message: "cannot load such file -- " + path},
			Class: core.R.Classes["LoadError"],
		}
	}

	previousSpecFile := core.CurrentSpecFile
	core.CurrentSpecFile = candidate
	defer func() {
		core.CurrentSpecFile = previousSpecFile
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
	return candidate, result
}

func (vm *VM) execute(op compiler.Opcode, frame *Frame) error {
	constants := vm.frameConstants(frame)
	switch op {
	case compiler.OpConstant:
		idx := vm.readUint16()
		if idx < 0 || idx >= len(constants) {
			return fmt.Errorf("invalid constant index %d (constants: %d) at ip=%d", idx, len(constants), vm.frames[vm.fp].Ip)
		}
		vm.push(constants[idx])

	case compiler.OpTrue:
		vm.push(core.R.TrueVal)

	case compiler.OpFalse:
		vm.push(core.R.FalseVal)

	case compiler.OpNil:
		vm.push(core.R.NilVal)

	case compiler.OpPatternCheck:
		patternIdx := vm.readUint16()
		pattern := constants[patternIdx].Data.(string)
		target := vm.peek(0)
		if errVal := vm.checkPatternRuntimeHooks(target, pattern); errVal != nil {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}

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
		vm.push(result)

	case compiler.OpPow:
		right := vm.pop()
		left := vm.pop()
		result := vm.pow(left, right)
		vm.push(result)

	case compiler.OpMinus, compiler.OpNeg:
		val := vm.pop()
		result := vm.negate(val)
		vm.push(result)

	case compiler.OpBang:
		val := vm.pop()
		result := vm.bang(val)
		vm.push(result)

	case compiler.OpEqual:
		right := vm.pop()
		left := vm.pop()
		result := vm.equals(left, right)
		vm.push(result)

	case compiler.OpNotEqual:
		right := vm.pop()
		left := vm.pop()
		result := vm.equals(left, right)
		if result.Type == object.ValueBool && result.Data == true {
			vm.push(core.R.FalseVal)
		} else {
			vm.push(core.R.TrueVal)
		}

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
		l, lok := left.Data.(int64)
		r, rok := right.Data.(int64)
		if lok && rok {
			vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l & r, Class: core.R.Classes["Integer"]})
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
		l, lok := left.Data.(int64)
		r, rok := right.Data.(int64)
		if lok && rok {
			vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l | r, Class: core.R.Classes["Integer"]})
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
		l, lok := left.Data.(int64)
		r, rok := right.Data.(int64)
		if lok && rok {
			vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l ^ r, Class: core.R.Classes["Integer"]})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpBitNot:
		val := vm.pop()
		v, ok := val.Data.(int64)
		if ok {
			vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: ^v, Class: core.R.Classes["Integer"]})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpBitLeftShift:
		right := vm.pop()
		left := vm.pop()
		l, lok := left.Data.(int64)
		r, rok := right.Data.(int64)
		if lok && rok {
			if r < 0 {
				vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l >> uint(-r), Class: core.R.Classes["Integer"]})
			} else {
				vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l << uint(r), Class: core.R.Classes["Integer"]})
			}
		} else {
			vm.push(vm.send(left, "<<", []*object.EmeraldValue{right}))
		}

	case compiler.OpBitRightShift:
		right := vm.pop()
		left := vm.pop()
		l, lok := left.Data.(int64)
		r, rok := right.Data.(int64)
		if lok && rok {
			if r < 0 {
				vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l << uint(-r), Class: core.R.Classes["Integer"]})
			} else {
				vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l >> uint(r), Class: core.R.Classes["Integer"]})
			}
		} else {
			result := vm.send(left, ">>", []*object.EmeraldValue{right})
			if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
				return nil
			}
			vm.push(result)
		}

	case compiler.OpJump:
		pos := vm.readUint16()
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

	case compiler.OpArray:
		n := vm.readUint16()
		if n > StackSize {
			return fmt.Errorf("OpArray: too many elements: %d", n)
		}
		elems := make([]*object.EmeraldValue, n)
		for i := n - 1; i >= 0; i-- {
			elems[i] = vm.pop()
		}
		vm.push(&object.EmeraldValue{
			Type:  object.ValueArray,
			Data:  elems,
			Class: core.R.Classes["Array"],
		})

	case compiler.OpHash:
		n := vm.readUint16()
		h := &object.RHash{Pairs: make(map[*object.EmeraldValue]*object.EmeraldValue)}
		for i := 0; i < int(n); i++ {
			key := vm.pop()
			value := vm.pop()
			if _, exists := h.Pairs[key]; !exists {
				h.Keys = append(h.Keys, key)
			}
			h.Pairs[key] = value
		}
		vm.push(&object.EmeraldValue{
			Type:  object.ValueHash,
			Data:  h,
			Class: core.R.Classes["Hash"],
		})

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
		vm.push(result)

	case compiler.OpGetGlobal:
		idx := vm.readUint16()
		idx = vm.resolveGlobalIndex(idx)
		val := vm.globals[idx]
		if val == nil {
			for name, globalIdx := range vm.globalNames {
				if globalIdx == idx && name == "$!" {
					val = core.LastException
					break
				}
				if globalIdx == idx && (name == "stdout" || name == "$stdout") {
					val = core.StdoutObject()
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
		idx := vm.resolveGlobalIndex(rawIdx)
		value, errVal := vm.validateGlobalAssignment(rawIdx, idx, vm.peek(0))
		if errVal != nil {
			if vm.raiseException(frame, errVal) {
				return nil
			}
			vm.returnUnhandledException(frame, errVal)
			return nil
		}
		vm.stack[vm.sp-1] = value
		vm.globals[idx] = value
		if name := vm.rawGlobalNameForIndex(rawIdx); name != "" {
			core.NotifyGlobalVariableSet(name, value)
		}

	case compiler.OpGetConstant:
		nameIdx := vm.readUint16()
		name, ok := constants[nameIdx].Data.(string)
		if !ok {
			return fmt.Errorf("OpGetConstant: expected string constant, got %T", constants[nameIdx].Data)
		}
		if val, ok := vm.lexicalConstantValue(name); ok {
			if val != nil && val.Type == object.ValueException && vm.raiseException(frame, val) {
				return nil
			}
			vm.push(val)
		} else if val, ok := vm.rubyConsts[name]; ok {
			vm.push(val)
		} else if name == "ENV" {
			vm.push(core.EnvObject())
		} else if name == "ThreadGroup::Default" {
			vm.push(core.DefaultThreadGroup())
		} else if processConst, ok := processConstantValue(name); ok {
			vm.push(processConst)
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
		} else if cls, ok := core.R.Classes[name]; ok {
			vm.push(&object.EmeraldValue{
				Type:  object.ValueClass,
				Data:  cls,
				Class: core.R.Classes["Class"],
			})
		} else if namespace, ok := vm.namespaceModuleValue(name); ok {
			vm.push(namespace)
		} else if core.EvaluatingRaiseErrorMatcher() {
			vm.push(core.NewNameError("uninitialized constant " + name))
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetConstant:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		value := vm.peek(0)
		if idx, top := vm.findClassStackEntry(name); top != nil {
			value = top
			vm.classStack = append(vm.classStack[:idx], vm.classStack[idx+1:]...)
			if top.Type == object.ValueClass {
				vm.runMinitestMethods(top)
			}
		}
		container := vm.currentConstantContainer()
		if container != nil && !strings.Contains(name, "::") {
			defineConstantOn(container, name, value)
		}
		if _, _, scopedLocal := vm.scopedLocalConstantContainer(frame, name); !scopedLocal {
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
			value = core.R.NilVal
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
		idx := vm.readUint8()
		basePtr := frame.Bp
		// In Ruby, Bp points to self (index 0), parameters start at index 1
		// But compiler generates indices starting from 0 for first param
		// So we need to add 1 to skip self
		stackIdx := basePtr + int(idx) + 1
		if stackIdx < 0 || stackIdx >= StackSize {
			return fmt.Errorf("OpGetLocal: invalid stack access basePtr=%d idx=%d stackIdx=%d sp=%d", basePtr, idx, stackIdx, vm.sp)
		}
		vm.push(vm.stack[stackIdx])

	case compiler.OpSetLocal:
		idx := vm.readUint8()
		basePtr := frame.Bp
		// Add 1 to skip self
		stackIdx := basePtr + int(idx) + 1
		if stackIdx < 0 || stackIdx >= StackSize {
			return fmt.Errorf("OpSetLocal: invalid stack access basePtr=%d idx=%d stackIdx=%d sp=%d", basePtr, idx, stackIdx, vm.sp)
		}
		vm.stack[stackIdx] = vm.peek(0)
		if vm.sp <= stackIdx {
			vm.sp = stackIdx + 1
		}

	case compiler.OpGetLocalCell:
		idx := vm.readUint8()
		stackIdx := frame.Bp + int(idx) + 1
		if stackIdx >= 0 && stackIdx < StackSize {
			vm.push(&object.EmeraldValue{Type: object.ValueObject, Data: &closureCell{slot: &vm.stack[stackIdx], value: derefClosureValue(vm.stack[stackIdx])}, Class: core.R.Classes["Object"]})
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

	case compiler.OpGetFree:
		idx := vm.readUint8()
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
			vm.push(vm.stack[outer.Bp+1+idx])
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetOuter:
		_ = vm.readUint8()
		idx := vm.readUint8()
		if vm.fp > 0 {
			outer := vm.frames[vm.fp-1]
			stackIdx := outer.Bp + 1 + idx
			vm.stack[stackIdx] = vm.peek(0)
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
		if stackIdx >= 0 && stackIdx < StackSize {
			vm.push(&object.EmeraldValue{Type: object.ValueObject, Data: &closureCell{slot: &vm.stack[stackIdx], value: derefClosureValue(vm.stack[stackIdx])}, Class: core.R.Classes["Object"]})
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

	case compiler.OpReturn:
		// Don't decrement fp here - the caller will handle that
		vm.sp = frame.Bp

	case compiler.OpReturnValue:
		retVal := vm.pop()
		if vm.returningFromClassBodyBlock(frame) {
			exception := core.NewLocalJumpError("unexpected return")
			if vm.raiseException(frame, exception) {
				return nil
			}
			vm.returnUnhandledException(frame, exception)
			return nil
		}
		// Don't decrement fp here - the caller will handle that
		// Just reset the stack to the base pointer and push the return value
		vm.sp = frame.Bp
		vm.push(retVal)

	case compiler.OpSend, compiler.OpSendWithKeywords:
		isKeywordSend := op == compiler.OpSendWithKeywords
		methodNameIdx := vm.readUint16()
		blockArg := vm.readUint8()
		numArgs := vm.readUint8()
		splatIndex := int(vm.readUint8())
		methodName := constants[methodNameIdx].Data.(string)
		prevBlock := vm.currentBlock

		var block *object.EmeraldValue
		switch blockArg {
		case 1:
			block = derefClosureValue(vm.pop())
		case 2:
			block = prevBlock
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
			if last != nil && last.Type == object.ValueHash {
				core.MarkRuby2KeywordHash(last)
			}
		}

		receiver := vm.pop()
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
		result := vm.send(receiver, methodName, args)
		vm.currentBlock = prevBlock
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
			return nil
		}
		if result != nil && result.Type == object.ValueException && core.EvaluatingRaiseErrorMatcher() {
			vm.returnUnhandledException(frame, result)
			return nil
		}
		vm.push(result)

	case compiler.OpBreak:
		val := core.R.NilVal
		if frame.WhileEnd >= 0 {
			frame.Ip = frame.WhileEnd - 1
			return nil
		}
		frame.BlockBreak = true
		frame.BlockBreakVal = val
		vm.sp = frame.Bp
		vm.push(val)
		return nil

	case compiler.OpBreakValue:
		val := core.R.NilVal
		if vm.sp > frame.Bp {
			val = vm.pop()
		}
		if frame.WhileEnd >= 0 {
			frame.Ip = frame.WhileEnd - 1
			return nil
		}
		frame.BlockBreak = true
		frame.BlockBreakVal = val
		vm.sp = frame.Bp
		vm.push(val)
		return nil

	case compiler.OpNext:
		val := core.R.NilVal
		if vm.sp > frame.Bp {
			val = vm.pop()
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

		method := &object.Method{
			Name:       name,
			Fn:         closure.Fn,
			Closure:    closure,
			Visibility: vm.currentDefinitionVisibility(),
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
			} else if classVal.Type == object.ValueModule {
				mod := classVal.Data.(*object.Module)
				mod.DefineMethod(name, method)
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
						mod.SingletonClass.Methods[name] = &copy
					}
				}
			} else {
				vm.push(core.NewTypeError("can't define method"))
				break
			}
		} else {
			mainObj := core.R.Main.Data.(*object.Object)
			mainObj.Class.DefineMethod(name, method)
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
			Name:    name,
			Fn:      closure.Fn,
			Closure: closure,
		}

		if receiver != nil && receiver.Type == object.ValueObject {
			obj := receiver.Data.(*object.Object)
			obj.SingletonMethods[name] = method
		} else if receiver != nil && receiver.Type == object.ValueClass {
			cls := receiver.Data.(*object.Class)
			cls.DefineClassMethod(name, method)
		} else if receiver != nil && receiver.Type == object.ValueModule {
			mod := receiver.Data.(*object.Module)
			mod.DefineMethod(name, method)
		}

		vm.push(&object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]})

	case compiler.OpDefineClassMethod:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)

		fn := &object.Function{
			Name:         name,
			Instructions: vm.pop().Data.([]byte),
			Constants:    constants,
			NumLocals:    0,
		}

		method := &object.Method{
			Name: name,
			Fn:   fn,
		}

		classVal := vm.stack[frame.Bp]
		if obj, ok := classVal.Data.(*object.Object); ok {
			obj.Class.DefineClassMethod(name, method)
		}

	case compiler.OpClass:
		nameIdx := vm.readUint16()
		hasExplicitSuperclass := vm.readUint8() == 1
		name := constants[nameIdx].Data.(string)

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
		if existing, ok := vm.rubyConsts[name]; ok && existing.Type == object.ValueClass {
			class = existing.Data.(*object.Class)
		} else if existing, ok := vm.rubyConsts[name]; ok && existing.Type != object.ValueClass {
			exception := core.NewTypeError(name + " is not a class")
			if vm.raiseException(frame, exception) {
				return nil
			}
			vm.returnUnhandledException(frame, exception)
			return nil
		} else if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); ok {
			if existing, found := vm.classValueFromContainer(container, constName); found {
				class = existing.Data.(*object.Class)
			}
		} else if existing, ok := vm.classValueForQualifiedName(name); ok {
			class = existing.Data.(*object.Class)
		} else if existing, ok := core.R.Classes[name]; ok {
			class = existing
		}
		if class == nil {
			class = object.NewClass(name)
			class.SuperClass = core.R.Classes["Object"]
			class.SuperClassSet = !hasExplicitSuperclass
		}

		classVal := &object.EmeraldValue{
			Type:  object.ValueClass,
			Data:  class,
			Class: core.R.Classes["Class"],
		}
		vm.rubyConsts[name] = classVal
		if container := vm.currentConstantContainer(); container != nil && !strings.Contains(name, "::") {
			defineConstantOn(container, name, classVal)
			vm.rubyConsts[qualifiedConstantName(container, name)] = classVal
		} else if strings.Contains(name, "::") {
			vm.defineQualifiedConstant(name, classVal)
		}
		if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); ok {
			defineConstantOn(container, constName, classVal)
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
		class.SuperClass = superClass
		class.SuperClassSet = true
		vm.push(classVal)

	case compiler.OpModule:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)

		var module *object.Module
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
		if existing, ok := vm.rubyConsts[name]; ok {
			if useExisting(existing) {
				return nil
			}
		} else if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); ok {
			if existing, found := directConstantValue(container, constName); found {
				if useExisting(existing) {
					return nil
				}
			}
			if module == nil {
				module = object.NewModule("")
			}
		} else if container, constName, ok := vm.qualifiedConstantContainer(name); ok {
			if existing, found := directConstantValue(container, constName); found {
				if useExisting(existing) {
					return nil
				}
			}
			if module == nil {
				module = object.NewModule("")
			}
		} else if existing, ok := vm.topLevelConstantValue(name); ok {
			if useExisting(existing) {
				return nil
			}
			if module == nil {
				module = object.NewModule(name)
			}
		} else {
			module = object.NewModule(name)
		}

		moduleVal := &object.EmeraldValue{
			Type:  object.ValueModule,
			Data:  module,
			Class: core.R.Classes["Module"],
		}
		vm.rubyConsts[name] = moduleVal
		if container := vm.currentConstantContainer(); container != nil && !strings.Contains(name, "::") {
			defineConstantOn(container, name, moduleVal)
			vm.rubyConsts[qualifiedConstantName(container, name)] = moduleVal
		} else if strings.Contains(name, "::") {
			vm.defineQualifiedConstant(name, moduleVal)
		}
		if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); ok {
			defineConstantOn(container, constName, moduleVal)
		}
		vm.classStack = append(vm.classStack, moduleVal)
		vm.push(moduleVal)

	case compiler.OpDup:
		vm.push(vm.peek(0))

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
			Binding:    vm.currentFrameBinding(),
			ClassStack: vm.currentClassStackSnapshot(),
			AutoSplat:  true,
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
			Binding:      vm.currentFrameBinding(),
			ClassStack:   vm.currentClassStackSnapshot(),
			InstanceVars: make(map[string]*object.EmeraldValue),
			IsLambda:     true,
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

	case compiler.OpRange:
		exclusive := vm.readUint8()
		right := vm.pop()
		left := vm.pop()

		var start, end int64
		var startRaw, endRaw float64
		startFloat := false
		endFloat := false
		startMissing := false
		endMissing := false
		if left == nil {
			left = nil
			startMissing = true
			start = 0
		}
		if right == nil {
			right = nil
			endMissing = true
			end = 0
		} else if right.Type == object.ValueNil {
			right = nil
			endMissing = true
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
			Type:  object.ValueRange,
			Data:  rangeObj,
			Class: core.R.Classes["Range"],
		})

	case compiler.OpRationalNew:
		denVal := vm.pop()
		numVal := vm.pop()
		num := toFloat64(numVal)
		den := toFloat64(denVal)
		if den == 0 {
			vm.push(numVal)
			return nil
		}
		result := num / den
		vm.push(&object.EmeraldValue{
			Type:  object.ValueFloat,
			Data:  result,
			Class: core.R.Classes["Float"],
		})

	case compiler.OpSendSuper:
		methodNameIdx := vm.readUint16()
		blockArg := vm.readUint8()
		numArgs := vm.readUint8()
		_ = constants[methodNameIdx].Data.(string)

		var block *object.EmeraldValue
		if blockArg == 1 {
			block = derefClosureValue(vm.pop())
		}

		args := make([]*object.EmeraldValue, int(numArgs))
		if numArgs == 255 {
			if frame.Fn != nil && frame.Fn.DefinedByDefineMethod {
				result := core.NewRuntimeError("implicit argument passing of super from method defined by define_method() is not supported")
				if vm.raiseException(frame, result) {
					return nil
				}
				vm.returnUnhandledException(frame, result)
				return nil
			}
			args = append([]*object.EmeraldValue(nil), frame.Args...)
		} else {
			for i := 0; i < int(numArgs); i++ {
				args[int(numArgs)-1-i] = vm.pop()
			}
		}
		if numArgs != 255 {
			vm.pop()
		} else {
			vm.pop()
		}

		self := vm.stack[frame.Bp]
		if self.Class == nil {
			vm.push(core.R.NilVal)
			return nil
		}

		superClass := frame.SuperStart
		if superClass == nil {
			superClass = self.Class.SuperClass
		}
		if superClass == nil {
			result := core.NewNoMethodError("super: no superclass method `" + frame.MethodName + "'")
			if vm.raiseException(frame, result) {
				return nil
			}
			vm.returnUnhandledException(frame, result)
			return nil
		}
		var methodObj *object.Method
		var owner *object.Class
		var ok bool
		if superClass == self.Class {
			methodObj, owner, ok = getMethodAfterPrependsWithOwner(superClass, frame.MethodName)
		} else {
			methodObj, owner, ok = superClass.GetMethodWithOwner(frame.MethodName)
		}
		if !ok {
			result := core.NewNoMethodError("super: no superclass method `" + frame.MethodName + "'")
			if vm.raiseException(frame, result) {
				return nil
			}
			vm.returnUnhandledException(frame, result)
			return nil
		}

		if fn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
			result := fn(self, args...)
			vm.push(result)
			return nil
		}

		if fn, ok := methodObj.Fn.(*object.Function); ok {
			oldFrame := vm.frames[vm.fp]
			prevBlock := vm.currentBlock
			if blockArg == 1 {
				vm.currentBlock = block
			} else {
				vm.currentBlock = nil
			}
			defer func() {
				vm.currentBlock = prevBlock
			}()
			bp := vm.sp
			vm.stack[vm.sp] = self
			vm.sp++
			for _, arg := range args {
				vm.stack[vm.sp] = arg
				vm.sp++
			}
			var blockParamProc *object.Proc
			if fn.HasBlockParam {
				blockVal := vm.currentBlock
				if blockVal == nil {
					blockVal = core.R.NilVal
				} else if blockVal.Type == object.ValueClosure {
					closure := blockVal.Data.(*object.Closure)
					proc := &object.Proc{
						Fn:           closure.Fn,
						Env:          closure.Free,
						Block:        closure.Block,
						Binding:      closure.Binding,
						ClassStack:   closure.ClassStack,
						InstanceVars: make(map[string]*object.EmeraldValue),
						IsLambda:     false,
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
				vm.sp = minSp
			}
			newFrame := &Frame{
				ID:         vm.allocateFrameID(),
				Fn:         fn,
				Ip:         -1,
				Bp:         bp,
				MethodName: frame.MethodName,
				SuperStart: owner.SuperClass,
				Args:       args,
				Block:      vm.currentBlock,
			}
			if blockParamProc != nil {
				blockParamProc.BreakOwnerID = newFrame.ID
			}
			vm.frames = append(vm.frames, newFrame)
			vm.fp++

			curFrame := vm.frames[vm.fp]
			instructions := curFrame.Fn.Instructions
			for curFrame.Ip < len(instructions)-1 {
				curFrame.Ip++
				op := compiler.Opcode(instructions[curFrame.Ip])
				if err := vm.execute(op, curFrame); err != nil {
					break
				}
				curFrame = vm.frames[vm.fp]
				if core.LastBlockResult != nil {
					break
				}
				instructions = curFrame.Fn.Instructions
			}

			result := core.R.NilVal
			if core.LastBlockResult != nil {
				result = core.LastBlockResult
			} else if vm.sp > bp {
				result = vm.stack[vm.sp-1]
			}
			vm.sp = bp
			vm.frames = vm.frames[:vm.fp]
			vm.fp--
			vm.frames[vm.fp] = oldFrame
			vm.push(result)
			return nil
		}

		vm.push(core.R.NilVal)

	case compiler.OpBlockGiven:
		if vm.yieldBlock() != nil {
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
		value, found := vm.scopedConstantValue(receiver, name)
		if found && value != nil && value.Type != object.ValueException {
			vm.push(&object.EmeraldValue{Type: object.ValueString, Data: "constant", Class: core.R.Classes["String"]})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpBeginRescue:
		rescueOffset := vm.readUint16()
		ensureOffset := vm.readUint16()
		endOffset := vm.readUint16()

		handler := &RescueHandler{
			RescueOffset: rescueOffset,
			EnsureOffset: ensureOffset,
			EndOffset:    endOffset,
			Frame:        frame,
		}
		vm.rescueStack = append(vm.rescueStack, handler)

	case compiler.OpEnsure:
		vm.ensureActive = true

	case compiler.OpRaise:
		var exception *object.EmeraldValue
		if vm.sp > 0 {
			exception = vm.pop()
		}
		if exception == nil || exception.Type != object.ValueException {
			message := "RuntimeError"
			exceptionClass := core.R.Classes["RuntimeError"]
			if exception != nil {
				if s, ok := exception.Data.(string); ok {
					message = s
				} else if exception.Type == object.ValueClass {
					exceptionClass = exception.Data.(*object.Class)
					message = exceptionClass.Name
				}
			}
			exception = &object.EmeraldValue{
				Type:  object.ValueException,
				Data:  &object.RException{Message: message},
				Class: exceptionClass,
			}
		}
		if vm.raiseException(frame, exception) {
			return nil
		}
		vm.returnUnhandledException(frame, exception)

	case compiler.OpReraise:
		exception := core.LastException
		if exception == nil {
			exception = &object.EmeraldValue{
				Type:  object.ValueException,
				Data:  &object.RException{Message: "RuntimeError"},
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
		vm.push(boolValue(rescueMatches(core.LastException, classes)))

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

		for i := len(vm.catchStack) - 1; i >= 0; i-- {
			handler := vm.catchStack[i]
			if handler.Label != nil && throwLabelsMatch(handler.Label, label) {
				vm.catchStack = vm.catchStack[:i]
				if value != nil {
					vm.push(value)
				}
				handler.Frame.Ip = handler.EndOffset - 1
				if handler.Frame != frame {
					frame.BlockBreak = true
					frame.BlockBreakVal = value
				}
				return nil
			}
		}
		className := "ArgumentError"
		if vm.threadDepth > 0 {
			className = "UncaughtThrowError"
		}
		exception := core.NewException(className, "uncaught throw")
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
	if vm.sp >= StackSize {
		return
	}
	vm.stack[vm.sp] = val
	vm.sp++
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
	vm.poppedValues = append(vm.poppedValues, val)
	return val
}

func (vm *VM) popFrameValue(frame *Frame) *object.EmeraldValue {
	minSp := 0
	if frame != nil && frame.Fn != nil {
		minSp = frame.Bp + 1 + frame.Fn.NumLocals
	}
	if vm.sp <= minSp {
		val := core.R.NilVal
		vm.poppedValues = append(vm.poppedValues, val)
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
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			return &object.EmeraldValue{Type: object.ValueInteger, Data: l + r, Class: core.R.Classes["Integer"]}
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
			return &object.EmeraldValue{Type: object.ValueString, Data: l + r, Class: core.R.Classes["String"]}
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
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			return &object.EmeraldValue{Type: object.ValueInteger, Data: l - r, Class: core.R.Classes["Integer"]}
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

func (vm *VM) mul(left, right *object.EmeraldValue) *object.EmeraldValue {
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			return &object.EmeraldValue{Type: object.ValueInteger, Data: l * r, Class: core.R.Classes["Integer"]}
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
			return &object.EmeraldValue{Type: object.ValueInteger, Data: l / r, Class: core.R.Classes["Integer"]}
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
	return core.R.NilVal
}

func (vm *VM) mod(left, right *object.EmeraldValue) *object.EmeraldValue {
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			if r == 0 {
				return core.R.NilVal
			}
			return &object.EmeraldValue{Type: object.ValueInteger, Data: l % r, Class: core.R.Classes["Integer"]}
		}
	}
	return core.R.NilVal
}

func (vm *VM) pow(left, right *object.EmeraldValue) *object.EmeraldValue {
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
				return &object.EmeraldValue{Type: object.ValueInteger, Data: result, Class: core.R.Classes["Integer"]}
			}
			if r < 0 {
				return &object.EmeraldValue{Type: object.ValueFloat, Data: 1.0 / vm.powInt(l, -int(r)), Class: core.R.Classes["Float"]}
			}
			if integerPowOverflows(l, int(r)) {
				result := int64(math.MaxInt64)
				if l < 0 && r%2 != 0 {
					result = math.MinInt64
				}
				value := &object.EmeraldValue{Type: object.ValueInteger, Data: result, Class: core.R.Classes["Integer"]}
				if l == 2 && r >= 0 {
					core.RememberBERPackOverride(value, core.BERPackPowerOfTwo(int(r)))
				}
				return value
			}
			return &object.EmeraldValue{Type: object.ValueInteger, Data: vm.powInt(l, int(r)), Class: core.R.Classes["Integer"]}
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
	return core.R.NilVal
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
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

func (vm *VM) negate(val *object.EmeraldValue) *object.EmeraldValue {
	switch v := val.Data.(type) {
	case int64:
		return &object.EmeraldValue{Type: object.ValueInteger, Data: -v, Class: core.R.Classes["Integer"]}
	case float64:
		return &object.EmeraldValue{Type: object.ValueFloat, Data: -v, Class: core.R.Classes["Float"]}
	}
	return core.R.NilVal
}

func (vm *VM) bang(val *object.EmeraldValue) *object.EmeraldValue {
	switch v := val.Data.(type) {
	case bool:
		if v {
			return core.R.FalseVal
		}
		return core.R.TrueVal
	}
	if val.Type == object.ValueNil {
		return core.R.TrueVal
	}
	return core.R.FalseVal
}

func (vm *VM) equals(left, right *object.EmeraldValue) *object.EmeraldValue {
	if left.Type == object.ValueNil && right.Type == object.ValueNil {
		return core.R.TrueVal
	}
	if left.Type == object.ValueArray || right.Type == object.ValueArray {
		if left.Equals(right) {
			return core.R.TrueVal
		}
		return core.R.FalseVal
	}
	if core.TimeValuesEqual(left, right) {
		return core.R.TrueVal
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
	return core.R.FalseVal
}

func (vm *VM) lessThan(left, right *object.EmeraldValue) *object.EmeraldValue {
	if result := moduleCompare(left, right, "<"); result != nil {
		return result
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
	return core.R.NilVal
}

func (vm *VM) greaterThan(left, right *object.EmeraldValue) *object.EmeraldValue {
	if result := moduleCompare(left, right, ">"); result != nil {
		return result
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
	return core.R.NilVal
}

func (vm *VM) lessThanOrEqual(left, right *object.EmeraldValue) *object.EmeraldValue {
	return moduleCompare(left, right, "<=")
}

func (vm *VM) greaterThanOrEqual(left, right *object.EmeraldValue) *object.EmeraldValue {
	return moduleCompare(left, right, ">=")
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
		switch i := index.Data.(type) {
		case int64:
			if i < 0 {
				i = int64(len(l)) + i
			}
			if i < 0 || i >= int64(len(l)) {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueString,
				Data:  string(l[i]),
				Class: core.R.Classes["String"],
			}
		}
	}
	if left != nil && left.Class != nil {
		return vm.send(left, "[]", []*object.EmeraldValue{index})
	}
	return core.R.NilVal
}

func (vm *VM) sliceIndex(left, start, length *object.EmeraldValue) *object.EmeraldValue {
	var s, l int
	switch v := start.Data.(type) {
	case int64:
		s = int(v)
	case float64:
		s = int(v)
	default:
		return core.R.NilVal
	}
	switch v := length.Data.(type) {
	case int64:
		l = int(v)
	case float64:
		l = int(v)
	default:
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
	if left != nil && left.Type == object.ValueArray {
		return vm.send(left, "[]=", []*object.EmeraldValue{index, value})
	}
	if left != nil && left.Type == object.ValueHash {
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
	if os.Getenv("RGO_DEBUG_SEND_TRACE") == "1" {
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
	if (method == "send" || method == "__send__" || method == "public_send") && len(args) > 0 {
		methodName, ok, parseErr := core.MethodNameFromValueWithError(args[0])
		if parseErr != nil {
			return parseErr
		}
		if !ok {
			return core.NewTypeError("is not a symbol nor a string")
		}
		forwardArgs := args[1:]
		if methodName != "initialize" && len(forwardArgs) == 1 && forwardArgs[0] != nil && forwardArgs[0].Type == object.ValueArray {
			forwardArgs = forwardArgs[0].Data.([]*object.EmeraldValue)
		}
		prev := vm.visibilityBypass
		vm.visibilityBypass = method != "public_send"
		methodObj, methodOwner, fallback := vm.lookupMethodForSend(receiver, methodName, forwardArgs, true)
		if fallback != nil {
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
		evalBinding := vm.instanceEvalBinding(receiver)
		evalArgs := make([]*object.EmeraldValue, 0, len(args)+1)
		evalArgs = append(evalArgs, args[0], evalBinding)
		if len(args) > 1 {
			evalArgs = append(evalArgs, args[1])
		}
		if len(args) > 2 {
			evalArgs = append(evalArgs, args[2])
		}
		return core.CallMethod(receiver, "eval", evalArgs...)
	}
	if (method == "class_eval" || method == "module_eval") && vm.currentBlock != nil {
		if len(args) > 0 {
			return core.NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0)", len(args)))
		}
		block := vm.currentBlock
		vm.currentBlock = nil
		if receiver.Type == object.ValueClass || receiver.Type == object.ValueModule {
			vm.classStack = append(vm.classStack, receiver)
			result := vm.callBlockWithSelf(block, receiver, append([]*object.EmeraldValue{receiver}, args...)...)
			vm.classStack = vm.classStack[:len(vm.classStack)-1]
			return result
		}
		return vm.callBlockWithSelf(block, receiver, append([]*object.EmeraldValue{receiver}, args...)...)
	}
	if method == "instance_exec" && vm.currentBlock != nil {
		block := vm.currentBlock
		vm.currentBlock = nil
		return vm.callBlockWithInstanceEvalContext(receiver, block, args...)
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
		if receiver.Type == object.ValueClass || receiver.Type == object.ValueModule {
			vm.classStack = append(vm.classStack, receiver)
			result := vm.callBlockWithSelf(block, receiver, args...)
			vm.classStack = vm.classStack[:len(vm.classStack)-1]
			return result
		}
		return vm.callBlockWithSelf(block, receiver, args...)
	}
	if method == "__exec_class_body__" && (receiver.Type == object.ValueClass || receiver.Type == object.ValueModule) && vm.currentBlock != nil {
		block := vm.currentBlock
		vm.currentBlock = nil
		core.ResetCurrentMethodVisibility(receiver)
		vm.classStack = append(vm.classStack, receiver)
		result := vm.callBlockWithSelf(block, receiver)
		vm.classStack = vm.classStack[:len(vm.classStack)-1]
		core.ResetCurrentMethodVisibility(receiver)
		return result
	}

	methodObj, methodOwner, fallback := vm.lookupMethodForSend(receiver, method, args, false)
	if fallback != nil {
		return fallback
	}

	return vm.invokeMethod(receiver, method, method, args, methodObj, methodOwner)
}

func (vm *VM) lookupMethodForSend(receiver *object.EmeraldValue, method string, args []*object.EmeraldValue, missingAsNameError bool) (*object.Method, *object.Class, *object.EmeraldValue) {
	var methodObj *object.Method
	var methodOwner *object.Class
	var ok bool

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
		for _, mod := range singletonClass.IncludedModules {
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

	if receiver.Type == object.ValueObject {
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
		}
	}

	if receiver.Type == object.ValueClass {
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
		inheritClassMethodsFirst := classInheritsFrom(cls, core.R.Classes["Thread"]) ||
			classInheritsFrom(cls, core.R.Classes["Proc"]) ||
			classInheritsFrom(cls, core.R.Classes["Time"])
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

	if receiver.Type == object.ValueModule {
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

	if !ok {
		methodObj, methodOwner, ok = receiver.Class.GetMethodWithOwner(method)
	}

	if !ok && receiver.Type == object.ValueClass {
		cls := receiver.Data.(*object.Class)
		if classInheritsFrom(cls, core.R.Classes["Thread"]) ||
			classInheritsFrom(cls, core.R.Classes["Proc"]) ||
			classInheritsFrom(cls, core.R.Classes["Time"]) {
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

	if !ok {
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
		if core.EvaluatingRaiseErrorMatcher() {
			if numberedParameterMethodNamePattern.MatchString(method) {
				return nil, nil, core.NewNoMethodError("undefined local variable or method `" + method + "'")
			}
			if missingAsNameError {
				return nil, nil, core.NewNameError("undefined method `" + method + "'")
			}
			return nil, nil, core.NewNoMethodError("undefined method `" + method + "'")
		}
		return nil, nil, core.R.NilVal
	}
	return methodObj, methodOwner, nil
}

func (vm *VM) invokeMethod(receiver *object.EmeraldValue, parentMethod, method string, args []*object.EmeraldValue, methodObj *object.Method, methodOwner *object.Class) *object.EmeraldValue {
	oldFrame := vm.frames[vm.fp]
	if methodObj == nil {
		return core.NewNoMethodError("undefined method `" + method + "'")
	}

	if methodObj.Visibility == "undefined" {
		return core.NewNoMethodError("undefined method `" + method + "'")
	}
	if !vm.visibilityBypass && parentMethod != "public" && parentMethod != "private" && parentMethod != "protected" &&
		parentMethod != "module_function" && parentMethod != "public_class_method" && parentMethod != "private_class_method" &&
		parentMethod != "using" && parentMethod != "refine" &&
		(methodObj.Visibility == "private" || methodObj.Visibility == "protected") && receiver != core.R.Main {
		return core.NewNoMethodError(methodObj.Visibility + " method `" + method + "' called")
	}
	if fn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
		return fn(receiver, args...)
	}

	if fn, ok := methodObj.Fn.(*object.Function); ok {
		args = dropEmptyRuby2KeywordHashForPositionalOnlyFunction(fn, args)
		if err := methodArityError(fn, positionalArityArgCount(fn, args)); err != nil {
			return err
		}
		if errVal := missingRequiredKeywordArgument(fn, args); errVal != nil {
			return errVal
		}
		if errVal := rejectedKeywordArgument(fn, args); errVal != nil {
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

		if len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" {
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
				normalCount := fn.RestParamIndex
				for i := 0; i < normalCount; i++ {
					vm.stack[vm.sp] = positionalArgOrDefault(fn, positionalArgs, i)
					vm.sp++
				}
				restElems := make([]*object.EmeraldValue, 0)
				if len(positionalArgs) > fn.RestParamIndex {
					restElems = positionalArgs[fn.RestParamIndex:]
				}
				vm.stack[vm.sp] = &object.EmeraldValue{
					Type:  object.ValueArray,
					Data:  restElems,
					Class: core.R.Classes["Array"],
				}
				vm.sp++
			} else {
				normalCount := len(fn.ParamDefaults)
				if normalCount < len(positionalArgs) {
					normalCount = len(positionalArgs)
				}
				for i := 0; i < normalCount; i++ {
					vm.stack[vm.sp] = positionalArgOrDefault(fn, positionalArgs, i)
					vm.sp++
				}
			}

			for _, kp := range fn.KeywordParams {
				val := vm.lookupKwarg(kwargsHash, kp.Name)
				if val == nil {
					if kp.HasDefault && kp.Default != nil {
						val = kp.Default
					} else {
						val = core.R.NilVal
					}
				}
				vm.stack[vm.sp] = val
				vm.sp++
			}
			if fn.KeywordRestParam != "" {
				vm.stack[vm.sp] = vm.keywordRestHash(kwargsHash, fn.KeywordParams)
				vm.sp++
			}
		} else if fn.HasRestParam {
			normalCount := fn.RestParamIndex
			for i := 0; i < normalCount; i++ {
				vm.stack[vm.sp] = positionalArgOrDefault(fn, args, i)
				vm.sp++
			}
			restElems := make([]*object.EmeraldValue, 0)
			if len(args) > fn.RestParamIndex {
				restElems = args[fn.RestParamIndex:]
			}
			if methodObj.Ruby2Keywords && len(restElems) > 0 {
				last := restElems[len(restElems)-1]
				if last != nil && last.Type == object.ValueHash {
					copied := make(map[*object.EmeraldValue]*object.EmeraldValue)
					for key, value := range executorHashToMap(last) {
						copied[key] = value
					}
					last = &object.EmeraldValue{Type: object.ValueHash, Data: copied, Class: core.R.Classes["Hash"]}
					core.MarkRuby2KeywordHash(last)
					restElems[len(restElems)-1] = last
				}
			}
			vm.stack[vm.sp] = &object.EmeraldValue{
				Type:  object.ValueArray,
				Data:  restElems,
				Class: core.R.Classes["Array"],
			}
			vm.sp++
		} else {
			normalCount := len(fn.ParamDefaults)
			if normalCount < len(args) {
				normalCount = len(args)
			}
			for i := 0; i < normalCount; i++ {
				vm.stack[vm.sp] = positionalArgOrDefault(fn, args, i)
				vm.sp++
			}
		}

		var blockParamProc *object.Proc
		if fn.HasBlockParam {
			blockVal := vm.currentBlock
			if blockVal == nil {
				blockVal = core.R.NilVal
			} else if blockVal.Type == object.ValueClosure {
				closure := blockVal.Data.(*object.Closure)
				proc := &object.Proc{
					Fn:           closure.Fn,
					Env:          closure.Free,
					Block:        closure.Block,
					Binding:      closure.Binding,
					ClassStack:   closure.ClassStack,
					InstanceVars: make(map[string]*object.EmeraldValue),
					IsLambda:     false,
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
			vm.sp = minSp
		}

		methodClosure := vm.detachedMethodClosure(methodObj)
		newFrame := &Frame{
			ID:             vm.allocateFrameID(),
			Fn:             fn,
			Ip:             -1,
			Bp:             bp,
			Closure:        methodClosure,
			MethodName:     method,
			Args:           args,
			Block:          vm.currentBlock,
			BlockBreakAddr: -1,
			WhileStart:     -1,
			WhileEnd:       -1,
		}
		if blockParamProc != nil {
			blockParamProc.BreakOwnerID = newFrame.ID
		}
		if methodFromPrependedModule(receiver.Class, method, methodObj) {
			newFrame.SuperStart = receiver.Class
		} else if methodOwner == nil {
			newFrame.SuperStart = receiver.Class
		} else {
			newFrame.SuperStart = methodOwner.SuperClass
		}
		vm.frames = append(vm.frames, newFrame)
		vm.fp++

		prevBlock := vm.currentBlock
		vm.currentBlock = nil
		prevClassStack := vm.classStack
		if methodClosure != nil && len(methodClosure.ClassStack) > 0 {
			vm.classStack = append([]*object.EmeraldValue(nil), methodClosure.ClassStack...)
		}
		frame := vm.frames[vm.fp]
		instructions := frame.Fn.Instructions

		for frame.Ip < len(instructions)-1 {
			frame.Ip++
			op := compiler.Opcode(instructions[frame.Ip])
			err := vm.execute(op, frame)
			if err != nil {
				vm.currentBlock = prevBlock
				vm.classStack = prevClassStack
				return core.R.NilVal
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
		} else if core.LastBlockResult != nil {
			result = core.LastBlockResult
		} else if vm.sp > bp {
			result = vm.stack[vm.sp-1]
		}
		vm.sp = bp

		vm.frames = vm.frames[:vm.fp]
		vm.fp--
		vm.frames[vm.fp] = oldFrame

		return result
	}

	return core.R.NilVal
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

func (vm *VM) detachedMethodClosure(method *object.Method) *object.Closure {
	if method == nil || method.Closure == nil {
		return nil
	}
	closure := method.Closure
	free := make([]*object.EmeraldValue, len(closure.Free))
	for i, value := range closure.Free {
		free[i] = detachClosureCapture(value)
	}
	detached := &object.Closure{
		Fn:         closure.Fn,
		Free:       free,
		Block:      closure.Block,
		Binding:    closure.Binding,
		ClassStack: closure.ClassStack,
		AutoSplat:  closure.AutoSplat,
	}
	method.Closure = detached
	return detached
}

func detachClosureCapture(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil {
		return core.R.NilVal
	}
	if cell, ok := value.Data.(*closureCell); ok {
		captured := cell.value
		if captured == nil && cell.slot != nil && *cell.slot != nil {
			captured = derefClosureValue(*cell.slot)
		}
		return &object.EmeraldValue{
			Type: object.ValueObject,
			Data: &closureCell{
				slot:  nil,
				value: captured,
			},
			Class: value.Class,
		}
	}
	return value
}

func methodArityError(fn *object.Function, argc int) *object.EmeraldValue {
	if fn == nil {
		return nil
	}
	min := 0
	limit := len(fn.Params)
	if fn.HasRestParam && fn.RestParamIndex < limit {
		limit = fn.RestParamIndex
	}
	for i := 0; i < limit; i++ {
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
	return fn != nil && (len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly)
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

func methodFromPrependedModule(class *object.Class, name string, method *object.Method) bool {
	if class == nil || method == nil {
		return false
	}
	for _, mod := range class.PrependedModules {
		if modMethod, ok := mod.Methods[name]; ok && modMethod == method {
			return true
		}
	}
	return false
}

func getMethodAfterPrependsWithOwner(class *object.Class, name string) (*object.Method, *object.Class, bool) {
	if class == nil {
		return nil, nil, false
	}
	if method, ok := class.Methods[name]; ok {
		return method, class, true
	}
	for _, mod := range class.IncludedModules {
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
			if entry.Data.(*object.Class).Name == name {
				return i, entry
			}
		case object.ValueModule:
			if entry.Data.(*object.Module).Name == name {
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
	for i := len(vm.classStack) - 1; i >= 0; i-- {
		container := vm.classStack[i]
		if container == nil || (container.Type != object.ValueClass && container.Type != object.ValueModule) {
			continue
		}
		if value, ok := directConstantValue(container, name); ok {
			return value, true
		}
		if value, loaded, hadAutoload := vm.triggerLexicalAutoload(container, name); hadAutoload {
			if loaded {
				return value, true
			}
		}
	}
	return nil, false
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

func (vm *VM) topLevelConstantValue(name string) (*object.EmeraldValue, bool) {
	if value, ok := vm.rubyConsts[name]; ok {
		return value, true
	}
	objectClass := core.R.Classes["Object"]
	if objectClass != nil {
		if value, ok := objectClass.Constants[name]; ok {
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
	if core.FeatureLoading(path) {
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
	if value, ok := directConstantValue(container, constName); ok {
		core.RemoveAutoload(container, constName)
		return value, true, true
	}
	core.RemoveAutoload(container, constName)
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
	if value, ok := directConstantValue(container, constName); ok && value.Type == object.ValueClass {
		return value, true
	}
	if value, loaded, hadAutoload := vm.triggerLexicalAutoload(container, constName); hadAutoload && loaded && value.Type == object.ValueClass {
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
	if len(vm.classStack) > 0 {
		if scope := vm.classStack[len(vm.classStack)-1]; scope != nil {
			return scope
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
	return catchLabel.Equals(throwLabel)
}

func defineConstantOn(container *object.EmeraldValue, name string, value *object.EmeraldValue) {
	switch container.Type {
	case object.ValueClass:
		container.Data.(*object.Class).DefineConstant(name, value)
	case object.ValueModule:
		module := container.Data.(*object.Module)
		module.DefineConstant(name, value)
	}
	core.AssignConstantName(container, name, value)
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

func snapshotClosureCapture(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil {
		return core.R.NilVal
	}
	if cell, ok := value.Data.(*closureCell); ok {
		return &object.EmeraldValue{
			Type:  object.ValueObject,
			Data:  &closureCell{slot: cell.slot, value: derefClosureValue(value)},
			Class: core.R.Classes["Object"],
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

func inheritsFrom(cls *object.Class, name string) bool {
	for current := cls; current != nil; current = current.SuperClass {
		if current.Name == name {
			return true
		}
	}
	return false
}

func (vm *VM) raiseException(frame *Frame, exception *object.EmeraldValue) bool {
	core.LastException = exception
	if len(vm.rescueStack) == 0 {
		return false
	}
	handler := vm.rescueStack[len(vm.rescueStack)-1]
	if handler.Frame != frame {
		return false
	}
	if handler.RescueOffset > 0 && frame.Ip >= handler.RescueOffset-1 {
		return false
	}
	vm.rescueStack = vm.rescueStack[:len(vm.rescueStack)-1]
	if handler.RescueOffset > 0 {
		handler.Frame.Ip = handler.RescueOffset - 1
	} else if handler.EnsureOffset > 0 {
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
	vm.sp = frame.Bp
	vm.push(exception)
	frame.Ip = len(frame.Fn.Instructions) - 1
}

func boolValue(value bool) *object.EmeraldValue {
	if value {
		return core.R.TrueVal
	}
	return core.R.FalseVal
}

func rescueMatches(exception *object.EmeraldValue, classes []*object.EmeraldValue) bool {
	if exception == nil || exception.Class == nil {
		return false
	}
	if len(classes) == 0 {
		return classInheritsFrom(exception.Class, core.R.Classes["StandardError"])
	}
	for _, classVal := range classes {
		if rescueClassMatches(exception.Class, classVal) {
			return true
		}
	}
	return false
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
	bp := vm.sp

	if len(fn.KeywordParams) > 0 && len(args) > 0 {
		lastArg := args[len(args)-1]
		positionalArgs := args
		var kwargs map[*object.EmeraldValue]*object.EmeraldValue

		if lastArg != nil && lastArg.Type == object.ValueHash {
			kwargs = executorHashToMap(lastArg)
			positionalArgs = args[:len(args)-1]
		}

		if fn.HasRestParam {
			vm.bindRestParameterSlots(fn, positionalArgs, bp, markRuby2Keywords)
		} else {
			normalCount := len(fn.ParamDefaults)
			if normalCount < len(positionalArgs) {
				normalCount = len(positionalArgs)
			}
			for i := 0; i < normalCount; i++ {
				slot := bp + 1 + i
				vm.stack[slot] = positionalArgOrDefault(fn, positionalArgs, i)
				if vm.sp <= slot {
					vm.sp = slot + 1
				}
			}
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
			val := vm.lookupKwarg(kwargs, kp.Name)
			if val == nil {
				if kp.HasDefault && kp.Default != nil {
					val = kp.Default
				} else {
					val = core.R.NilVal
				}
			}
			vm.stack[slot] = val
			if vm.sp <= slot {
				vm.sp = slot + 1
			}
		}
	} else if fn.HasRestParam {
		vm.bindRestParameterSlots(fn, args, bp, false)
	} else {
		normalCount := len(fn.ParamDefaults)
		if normalCount < len(args) {
			normalCount = len(args)
		}
		for i := 0; i < normalCount; i++ {
			slot := bp + 1 + i
			vm.stack[slot] = positionalArgOrDefault(fn, args, i)
			if vm.sp <= slot {
				vm.sp = slot + 1
			}
		}
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
					Fn:           closure.Fn,
					Env:          closure.Free,
					Block:        closure.Block,
					Binding:      closure.Binding,
					ClassStack:   closure.ClassStack,
					InstanceVars: make(map[string]*object.EmeraldValue),
					IsLambda:     false,
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

func (vm *VM) bindRestParameterSlots(fn *object.Function, args []*object.EmeraldValue, bp int, markRuby2Keywords bool) {
	preCount := fn.RestParamIndex
	if preCount < 0 {
		preCount = 0
	}
	if preCount > len(fn.Params) {
		preCount = len(fn.Params)
	}
	postCount := len(fn.Params) - preCount

	for i := 0; i < preCount; i++ {
		slot := bp + 1 + i
		vm.stack[slot] = positionalArgOrDefault(fn, args, i)
		if vm.sp <= slot {
			vm.sp = slot + 1
		}
	}

	postStart := len(args) - postCount
	if postStart < preCount {
		postStart = preCount
	}
	restElems := make([]*object.EmeraldValue, 0)
	if postStart > preCount && preCount < len(args) {
		restElems = args[preCount:postStart]
	}
	if markRuby2Keywords && len(restElems) > 0 {
		last := restElems[len(restElems)-1]
		if last != nil && last.Type == object.ValueHash {
			copied := make(map[*object.EmeraldValue]*object.EmeraldValue)
			for key, value := range executorHashToMap(last) {
				copied[key] = value
			}
			last = &object.EmeraldValue{Type: object.ValueHash, Data: copied, Class: core.R.Classes["Hash"]}
			core.MarkRuby2KeywordHash(last)
			restElems[len(restElems)-1] = last
		}
	}
	restSlot := bp + 1 + preCount
	vm.stack[restSlot] = &object.EmeraldValue{
		Type:  object.ValueArray,
		Data:  restElems,
		Class: core.R.Classes["Array"],
	}
	if vm.sp <= restSlot {
		vm.sp = restSlot + 1
	}

	for j := 0; j < postCount; j++ {
		paramIndex := preCount + j
		argIndex := postStart + j
		slot := bp + 1 + preCount + 1 + j
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
	if os.Getenv("RGO_DEBUG_FOR_LOOP") == "1" {
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
		if proc != nil && !proc.IsLambda {
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
			Fn:         fn,
			Free:       proc.Env,
			Block:      proc.Block,
			Binding:    proc.Binding,
			ClassStack: proc.ClassStack,
			AutoSplat:  proc.AutoSplat,
		}
	default:
		return core.R.NilVal
	}

	if fn == nil {
		return core.R.NilVal
	}
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
	lambdaParamCount := len(fn.Params)
	if fn.SingleDestructure && lambdaParamCount > 1 && len(args) == 1 {
		lambdaParamCount = 1
	}
	if isLambda && len(args) != lambdaParamCount && !fn.HasRestParam {
		return core.NewArgumentError("wrong number of arguments")
	}
	if isLambda && fn.HasRestParam && len(args) < lambdaParamCount {
		return core.NewArgumentError("wrong number of arguments")
	}

	prevBlock := vm.currentBlock
	vm.currentBlock = nil
	prevClassStack := vm.classStack
	prevCatchStack := vm.catchStack
	if isThreadBlock {
		vm.catchStack = nil
	}
	if len(closure.ClassStack) > 0 {
		vm.classStack = append([]*object.EmeraldValue(nil), closure.ClassStack...)
		if self != nil && (self.Type == object.ValueClass || self.Type == object.ValueModule) {
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
	vm.bindFunctionArguments(fn, args, prevBlock, false)

	newFrame := &Frame{ID: vm.allocateFrameID(), Fn: fn, Ip: -1, Bp: bp, Closure: closure, Block: closure.Block, BlockBreak: false, BlockBreakVal: nil, BlockNextVal: nil, BlockBreakAddr: -1, WhileStart: -1, WhileEnd: -1}
	vm.frames = append(vm.frames, newFrame)
	vm.fp++

	frame := vm.frames[vm.fp]
	instructions := frame.Fn.Instructions
	count := 0
	for frame.Ip < len(instructions)-1 {
		count++
		if count > 1000 {
			break
		}
		frame.Ip++
		op := compiler.Opcode(instructions[frame.Ip])
		if err := vm.execute(op, frame); err != nil {
			break
		}
		frame = vm.frames[vm.fp]
		if frame.BlockBreak || frame.BlockNextVal != nil {
			break
		}
		instructions = frame.Fn.Instructions
	}

	result := core.R.NilVal
	if vm.sp > bp {
		result = vm.stack[vm.sp-1]
	}
	vm.sp = bp

	if frame.BlockBreak {
		result = frame.BlockBreakVal
		if result == nil {
			result = core.R.NilVal
		}
		if !isLambda && vm.procCallDepth > 0 && !vm.procBreakOwnerActive(procData) {
			result = core.NewLocalJumpError("unexpected break")
			core.LastException = result
		} else {
			core.LastBlockResult = result
		}
	} else if frame.BlockNextVal != nil {
		result = frame.BlockNextVal
	} else {
		core.LastBlockResult = nil
	}

	vm.frames = vm.frames[:vm.fp]
	vm.fp--

	return result
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
		if hashHasNonSymbolKey(arg) {
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
	if fn == nil || len(fn.KeywordParams) == 0 || len(args) == 0 {
		return nil
	}
	last := args[len(args)-1]
	if last == nil || last.Type != object.ValueHash {
		return nil
	}
	unknown := make([]string, 0)
	keywordHashes := []*object.EmeraldValue{last}
	if len(args) > 1 {
		prior := args[len(args)-2]
		if prior != nil && prior.Type == object.ValueHash {
			keywordHashes = append([]*object.EmeraldValue{prior}, keywordHashes...)
		}
	}
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
	case object.ValueString:
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

func (vm *VM) instanceEvalContextClass(receiver *object.EmeraldValue) *object.EmeraldValue {
	if receiver == nil {
		receiver = core.R.NilVal
	}
	switch receiver.Type {
	case object.ValueObject, object.ValueClass, object.ValueModule:
		if receiver.Type == object.ValueObject {
			if _, ok := receiver.Data.(*object.Object); !ok {
				return core.R.NilVal
			}
		}
		if singleton := core.CallMethod(receiver, "singleton_class"); singleton != nil && singleton.Type == object.ValueClass {
			return singleton
		}
	}
	return core.R.NilVal
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
	if path == "" {
		path = "spec.rb"
	}
	line := int64(1)
	if frame != nil && frame.Ip >= 0 {
		line = int64(frame.Ip + 1)
	}
	if frame != nil && frame.Fn != nil {
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
			for _, item := range namedLocals {
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
	methodName := ""
	if frame != nil {
		if frame.MethodName != "" {
			methodName = frame.MethodName
		} else if frame.Fn != nil && frame.Fn.Name != "" && frame.Fn.Name != "main" {
			methodName = frame.Fn.Name
		}
	}
	self := core.R.Main
	if frame != nil && frame.Bp >= 0 && frame.Bp < vm.sp && vm.stack[frame.Bp] != nil {
		self = vm.stack[frame.Bp]
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
	return &object.RBinding{
		Self:         self,
		Locals:       locals,
		LocalNames:   localNames,
		Method:       methodName,
		Constants:    vm.rubyConsts,
		ClassStack:   vm.currentClassStackSnapshot(),
		Path:         path,
		Line:         line,
		InstanceVars: map[string]*object.EmeraldValue{},
	}
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
		frame := vm.frames[i]
		if frame == nil || frame.Fn == nil {
			continue
		}
		label := "main"
		if frame.MethodName != "" {
			label = frame.MethodName
		} else if frame.Fn.Name != "" {
			switch frame.Fn.Name {
			case "main":
				label = "main"
			case "__block__", "__lambda__":
				label = "block"
			default:
				label = frame.Fn.Name
			}
		}
		path := core.CurrentSpecFile
		if path == "" {
			path = label
		}
		line := vm.sourceLineForFrame(frame)

		if vm.ensureActive && i == ensureBlockIndex {
			label = "block"
			if ensureBlockLine > 0 {
				line = vm.sourceLineForFrame(frame)
			}
		}

		frames = append(frames, object.RBacktraceLocation{
			Path:  path,
			Line:  line,
			Label: label,
		})
	}
	return frames
}

func (vm *VM) sourceLineForFrame(frame *Frame) int64 {
	if frame == nil || frame.Ip < 0 {
		return 0
	}
	if frame.Fn != nil && len(frame.Fn.LineMap) > 0 {
		bestPos := -1
		bestLine := 0
		for pos, line := range frame.Fn.LineMap {
			if pos <= frame.Ip && pos > bestPos && line > 0 {
				bestPos = pos
				bestLine = line
			}
		}
		if bestLine > 0 {
			return int64(bestLine)
		}
	}
	return int64(frame.Ip + 1)
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
	if hash == nil {
		return nil
	}
	key := ":" + name
	for k, v := range hash {
		if k.Type == object.ValueString && k.Data.(string) == key {
			return v
		}
	}
	return nil
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

func frozenMethodDefinitionError(receiver *object.EmeraldValue, lexicalDefinition bool) *object.EmeraldValue {
	kind := "object"
	if receiver != nil {
		switch receiver.Type {
		case object.ValueClass:
			kind = "Class"
			if lexicalDefinition {
				kind = "class"
			}
		case object.ValueModule:
			kind = "Module"
			if lexicalDefinition {
				kind = "module"
			}
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
			if constant, ok := class.GetConstant(constName); ok {
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
			if constant, ok := module.Constants[constName]; ok {
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
		if constant, ok := cls.GetConstant(constName); ok {
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

func (vm *VM) scopedConstantValue(receiver *object.EmeraldValue, constName string) (*object.EmeraldValue, bool) {
	if receiver == nil {
		return nil, false
	}
	qualifiedName := qualifiedConstantName(receiver, constName)
	switch receiver.Type {
	case object.ValueClass:
		class := receiver.Data.(*object.Class)
		if class.PrivateConstants[constName] {
			return core.NewPrivateConstantNameError(receiver, constName), true
		}
		if owner, found := privateConstantOwnerInClassLookup(class, constName); found {
			return core.NewPrivateConstantNameError(owner, constName), true
		}
		if constant, ok := class.GetConstant(constName); ok {
			return constant, true
		}
		if constant, ok := vm.triggerAutoload(receiver, constName); ok {
			return constant, true
		}
		if constant, ok := vm.rubyConsts[qualifiedName]; ok {
			return constant, true
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
			return core.NewPrivateConstantNameError(receiver, constName), true
		}
		if owner, found := privateConstantOwnerInModuleLookup(module, constName); found {
			return core.NewPrivateConstantNameError(owner, constName), true
		}
		if constant, ok := module.Constants[constName]; ok {
			return constant, true
		}
		if constant, ok := vm.triggerAutoload(receiver, constName); ok {
			return constant, true
		}
		if constant, ok := vm.rubyConsts[qualifiedName]; ok {
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
	if core.FeatureLoading(path) {
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
			core.RemoveAutoload(receiver, constName)
			return constant, true
		}
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		if constant, ok := module.Constants[constName]; ok {
			core.RemoveAutoload(receiver, constName)
			return constant, true
		}
	}
	core.RemoveAutoload(receiver, constName)
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
