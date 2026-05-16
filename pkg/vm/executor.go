package vm

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
)

const StackSize = 2048
const MaxFrames = 1024

var DevMode = os.Getenv("RGO_DEV") == "1"

var CurrentVM *VM

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
	constants   []*object.EmeraldValue
	globals     []*object.EmeraldValue
	globalNames map[string]int
	rubyConsts  map[string]*object.EmeraldValue

	stack []*object.EmeraldValue
	sp    int

	frames []*Frame
	fp     int

	instructions compiler.Instructions

	poppedValues []*object.EmeraldValue

	currentBlock     *object.EmeraldValue
	visibilityBypass bool
	classStack       []*object.EmeraldValue
	autoloading      map[string]bool

	rescueStack  []*RescueHandler
	ensureActive bool

	catchStack []*CatchHandler
}

func New(bytecode *compiler.Bytecode) *VM {
	core.Init()
	return newVM(bytecode, nil)
}

func newVM(bytecode *compiler.Bytecode, parent *VM) *VM {
	mainFn := &object.Function{
		Name:         "__main__",
		Instructions: bytecode.Instructions,
		Constants:    bytecode.Constants,
		NumLocals:    bytecode.NumLocals,
		LocalNames:   bytecode.LocalNames,
	}

	mainFrame := &Frame{
		Fn: mainFn,
		Ip: -1,
		Bp: 0,
	}

	vm := &VM{
		constants:    bytecode.Constants,
		globals:      make([]*object.EmeraldValue, 100),
		globalNames:  bytecode.GlobalNames,
		rubyConsts:   make(map[string]*object.EmeraldValue),
		stack:        make([]*object.EmeraldValue, StackSize),
		sp:           1 + bytecode.NumLocals,
		frames:       []*Frame{mainFrame},
		fp:           0,
		instructions: bytecode.Instructions,
		autoloading:  make(map[string]bool),
	}
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
	core.BlockGivenCheck = func() bool {
		return vm.currentBlock != nil
	}
	core.CurrentBlockValue = func() *object.EmeraldValue {
		return vm.currentBlock
	}
	core.EvalSource = func(source string) *object.EmeraldValue {
		return vm.evalSource(source)
	}
	core.RequirePath = func(path string) *object.EmeraldValue {
		return vm.requirePath(path)
	}
	core.InMethodScope = func() bool {
		if vm.fp < 0 || vm.fp >= len(vm.frames) || vm.frames[vm.fp] == nil {
			return false
		}
		return vm.frames[vm.fp].MethodName != ""
	}
}

func (vm *VM) setGlobalByName(name string, value *object.EmeraldValue) {
	if vm.globalNames == nil {
		if name == "$?" && len(vm.globals) > 0 {
			vm.globals[0] = value
		}
		return
	}
	idx, ok := vm.globalNames[name]
	if !ok && name == "$?" {
		idx, ok = vm.globalNames["?"]
	}
	if !ok && name == "$?" {
		idx, ok = 0, true
	}
	if !ok || idx < 0 || idx >= len(vm.globals) {
		return
	}
	vm.globals[idx] = value
}

func (vm *VM) getGlobalByName(name string) *object.EmeraldValue {
	if vm.globalNames == nil {
		return nil
	}
	idx, ok := vm.globalNames[name]
	if !ok && name == "$?" {
		idx, ok = vm.globalNames["?"]
	}
	if !ok || idx < 0 || idx >= len(vm.globals) {
		return nil
	}
	return vm.globals[idx]
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

func (vm *VM) Run() error {
	frame := vm.frames[vm.fp]
	instructions := frame.Fn.Instructions

	count := 0
	for frame.Ip < len(instructions)-1 {
		count++
		if count > 1000 {
			return fmt.Errorf("infinite loop detected at ip=%d, op=%v", frame.Ip, instructions[frame.Ip])
		}
		frame.Ip++

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
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return core.R.NilVal
	}

	c := compiler.New()
	if err := c.Compile(program); err != nil {
		return core.R.NilVal
	}

	parent := CurrentVM
	child := newVM(c.Bytecode(), vm)
	if err := child.Run(); err != nil {
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

func (vm *VM) requirePath(path string) *object.EmeraldValue {
	if path == "" {
		return core.NewArgumentError("empty file name")
	}
	candidates := []string{path}
	if !strings.HasSuffix(path, ".rb") {
		candidates = append(candidates, path+".rb")
	}
	for _, candidate := range candidates {
		content, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		return vm.evalSource(string(content))
	}
	return &object.EmeraldValue{
		Type:  object.ValueException,
		Data:  &object.RException{Message: "cannot load such file -- " + path},
		Class: core.R.Classes["LoadError"],
	}
}

func (vm *VM) execute(op compiler.Opcode, frame *Frame) error {
	constants := vm.frameConstants(frame)
	switch op {
	case compiler.OpConstant:
		idx := vm.readUint16()
		vm.push(constants[idx])

	case compiler.OpTrue:
		vm.push(core.R.TrueVal)

	case compiler.OpFalse:
		vm.push(core.R.FalseVal)

	case compiler.OpNil:
		vm.push(core.R.NilVal)

	case compiler.OpPop:
		vm.popFrameValue(frame)

	case compiler.OpAdd:
		right := vm.pop()
		left := vm.pop()
		result := vm.add(left, right)
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
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
			vm.push(core.R.NilVal)
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
		if n > 100 {
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
		h := make(map[*object.EmeraldValue]*object.EmeraldValue)
		for i := 0; i < int(n); i++ {
			key := vm.pop()
			value := vm.pop()
			h[key] = value
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
		val := vm.globals[idx]
		if val == nil {
			for name, globalIdx := range vm.globalNames {
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
			}
		}
		if val == nil {
			val = core.R.NilVal
		}
		vm.push(val)

	case compiler.OpSetGlobal:
		idx := vm.readUint16()
		vm.globals[idx] = vm.peek(0)

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
		if result != nil && result.Type == object.ValueException && vm.raiseException(frame, result) {
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
		if obj, ok := receiver.Data.(*object.Object); ok {
			obj.InstanceVars[name] = val
		} else if proc, ok := receiver.Data.(*object.Proc); ok {
			if proc.InstanceVars == nil {
				proc.InstanceVars = make(map[string]*object.EmeraldValue)
			}
			proc.InstanceVars[name] = val
		} else if module, ok := receiver.Data.(*object.Module); ok {
			module.InstanceVars[name] = val
		} else if class, ok := receiver.Data.(*object.Class); ok {
			class.SetInstanceVar(name, val)
		} else {
			core.SetDynamicInstanceVar(receiver, name, val)
		}

	case compiler.OpGetClassVar:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		receiver := vm.stack[frame.Bp]
		if val, ok := core.LookupClassVariable(receiver, name); ok {
			vm.push(val)
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetClassVar:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)
		val := vm.peek(0)
		receiver := vm.stack[frame.Bp]
		core.SetClassVariable(receiver, name, val)

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

	case compiler.OpReturn:
		// Don't decrement fp here - the caller will handle that
		vm.sp = frame.Bp

	case compiler.OpReturnValue:
		retVal := vm.pop()
		// Don't decrement fp here - the caller will handle that
		// Just reset the stack to the base pointer and push the return value
		vm.sp = frame.Bp
		vm.push(retVal)

	case compiler.OpSend:
		methodNameIdx := vm.readUint16()
		blockArg := vm.readUint8()
		numArgs := vm.readUint8()
		methodName := constants[methodNameIdx].Data.(string)

		var block *object.EmeraldValue
		if blockArg == 1 {
			block = derefClosureValue(vm.pop())
		}

		args := make([]*object.EmeraldValue, int(numArgs))
		for i := 0; i < int(numArgs); i++ {
			args[numArgs-1-i] = vm.pop()
		}

		receiver := vm.pop()

		prevBlock := vm.currentBlock
		if blockArg == 1 {
			vm.currentBlock = block
		} else {
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
		if vm.sp > frame.Bp {
			val = vm.stack[vm.sp-1]
			vm.sp--
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

	case compiler.OpBreakValue:
		val := core.R.NilVal
		if vm.sp > frame.Bp {
			val = vm.stack[vm.sp-1]
			vm.sp--
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

	case compiler.OpSetWhileEnd:
		target := vm.readUint16()
		frame.WhileEnd = int(target)
		frame.BlockBreakAddr = int(target)

	case compiler.OpRedo:
		frame.Ip = -1
		vm.sp = frame.Bp + 1 + frame.Fn.NumLocals

	case compiler.OpYield:
		result := vm.callBlock(vm.yieldBlock())
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
		result := vm.callBlock(vm.yieldBlock(), args...)
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
			method.Owner = classVal
			if classVal.Type == object.ValueClass {
				cls := classVal.Data.(*object.Class)
				cls.DefineMethod(name, method)
			} else if classVal.Type == object.ValueModule {
				mod := classVal.Data.(*object.Module)
				mod.DefineMethod(name, method)
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
		name := constants[nameIdx].Data.(string)

		var class *object.Class
		if existing, ok := vm.rubyConsts[name]; ok && existing.Type == object.ValueClass {
			class = existing.Data.(*object.Class)
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
		}

		classVal := &object.EmeraldValue{
			Type:  object.ValueClass,
			Data:  class,
			Class: core.R.Classes["Class"],
		}
		vm.rubyConsts[name] = classVal
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
			return fmt.Errorf("OpInherited: expected superclass, got %v", superVal)
		}
		class := classVal.Data.(*object.Class)
		superClass := superVal.Data.(*object.Class)
		if class.SuperClass != nil && class.SuperClass != core.R.Classes["Object"] && class.SuperClass != superClass {
			exception := core.NewTypeError("superclass mismatch for class " + class.Name)
			if vm.raiseException(frame, exception) {
				return nil
			}
			vm.returnUnhandledException(frame, exception)
			return nil
		}
		class.SuperClass = superClass
		vm.push(classVal)

	case compiler.OpModule:
		nameIdx := vm.readUint16()
		name := constants[nameIdx].Data.(string)

		var module *object.Module
		if existing, ok := vm.rubyConsts[name]; ok && existing.Type == object.ValueModule {
			module = existing.Data.(*object.Module)
		} else if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); ok {
			if existing, found := vm.scopedConstantValue(container, constName); found && existing.Type == object.ValueModule {
				module = existing.Data.(*object.Module)
			} else {
				module = object.NewModule("")
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
		if container, constName, ok := vm.scopedLocalConstantContainer(frame, name); ok {
			defineConstantOn(container, constName, moduleVal)
		}
		vm.classStack = append(vm.classStack, moduleVal)
		vm.push(moduleVal)

	case compiler.OpDup:
		vm.push(vm.peek(0))

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
		if val.Type == object.ValueArray {
			elems := val.Data.([]*object.EmeraldValue)
			for _, elem := range elems {
				vm.push(elem)
			}
		} else {
			vm.push(val)
		}

	case compiler.OpRange:
		exclusive := vm.readUint8()
		right := vm.pop()
		left := vm.pop()

		var start, end int64
		if l, ok := left.Data.(int64); ok {
			start = l
		} else if l, ok := left.Data.(float64); ok {
			start = int64(l)
		} else {
			vm.push(core.R.NilVal)
			return nil
		}
		if r, ok := right.Data.(int64); ok {
			end = r
		} else if r, ok := right.Data.(float64); ok {
			end = int64(r)
		} else {
			vm.push(core.R.NilVal)
			return nil
		}

		rangeObj := &object.RRange{
			Start:     start,
			End:       end,
			Exclusive: exclusive == 1,
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
			vm.push(core.R.NilVal)
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
			vm.push(core.R.NilVal)
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
			if fn.HasBlockParam {
				blockVal := vm.currentBlock
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
			newFrame := &Frame{
				Fn:         fn,
				Ip:         -1,
				Bp:         bp,
				MethodName: frame.MethodName,
				SuperStart: owner.SuperClass,
				Args:       args,
				Block:      vm.currentBlock,
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
				instructions = curFrame.Fn.Instructions
			}

			result := core.R.NilVal
			if vm.sp > bp {
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
		classes := make([]*object.EmeraldValue, count)
		for i := count - 1; i >= 0; i-- {
			classes[i] = vm.pop()
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
			if handler.Label != nil && handler.Label.Equals(label) {
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
				return core.NewRangeErrorValue("integer out of range")
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
	switch l := left.Data.(type) {
	case []*object.EmeraldValue:
		switch i := index.Data.(type) {
		case int64:
			if i >= 0 && i < int64(len(l)) {
				l[i] = value
			}
		}
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
	if receiver == nil {
		receiver = core.R.NilVal
	}
	receiver = derefClosureValue(receiver)
	if method == "send" && len(args) > 0 {
		methodName, ok := methodNameFromValue(args[0])
		if !ok {
			return core.R.NilVal
		}
		forwardArgs := args[1:]
		if methodName != "initialize" && len(forwardArgs) == 1 && forwardArgs[0] != nil && forwardArgs[0].Type == object.ValueArray {
			forwardArgs = forwardArgs[0].Data.([]*object.EmeraldValue)
		}
		prev := vm.visibilityBypass
		vm.visibilityBypass = true
		result := vm.send(receiver, methodName, forwardArgs)
		vm.visibilityBypass = prev
		return result
	}
	if method == "instance_eval" && vm.currentBlock != nil {
		block := vm.currentBlock
		vm.currentBlock = nil
		return vm.callBlockWithSelf(block, receiver, args...)
	}
	if (method == "class_eval" || method == "module_eval") && vm.currentBlock != nil {
		if len(args) > 0 {
			return core.NewArgumentError("wrong number of arguments")
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
		return vm.callBlockWithSelf(block, receiver, args...)
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
		vm.classStack = append(vm.classStack, receiver)
		result := vm.callBlockWithSelf(block, receiver)
		vm.classStack = vm.classStack[:len(vm.classStack)-1]
		return result
	}

	var methodObj *object.Method
	var methodOwner *object.Class
	var ok bool

	methodName := method

	if receiver.Type == object.ValueObject {
		if obj, isObject := receiver.Data.(*object.Object); isObject {
			if m, found := obj.SingletonMethods[method]; found {
				methodObj = m
				methodOwner = nil
				ok = true
			} else if obj.SingletonClass != nil {
				if m, found := obj.SingletonClass.Methods[method]; found {
					methodObj = m
					methodOwner = obj.SingletonClass
					ok = true
				}
			}
		}
	}

	if receiver.Type == object.ValueClass {
		cls := receiver.Data.(*object.Class)
		if cls.SingletonClass != nil {
			if m, found := cls.SingletonClass.Methods[method]; found {
				methodObj = m
				methodOwner = cls.SingletonClass
				ok = true
			}
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
				if current.SingletonClass != nil {
					if m, found := current.SingletonClass.Methods[method]; found {
						methodObj = m
						methodOwner = current.SingletonClass
						ok = true
						break
					}
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
	}

	if receiver.Type == object.ValueModule {
		mod := receiver.Data.(*object.Module)
		if mod.SingletonClass != nil {
			if m, found := mod.SingletonClass.Methods[method]; found {
				methodObj = m
				methodOwner = mod.SingletonClass
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
			if current.SingletonClass != nil {
				if m, found := current.SingletonClass.Methods[method]; found {
					methodObj = m
					methodOwner = current.SingletonClass
					ok = true
					break
				}
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
			return fallback
		}
		if fallback := core.FileStatFixtureDispatch(receiver, method, args...); fallback != nil {
			return fallback
		}
		if fallback := vm.constSourceLocationArgumentError(method, args); fallback != nil {
			return fallback
		}
		if fallback := vm.moduleEvalArgumentError(method, args); fallback != nil {
			return fallback
		}
		if fallback := vm.constDefinedArgumentError(method, args); fallback != nil {
			return fallback
		}
		if core.EvaluatingRaiseErrorMatcher() {
			return core.NewNoMethodError("undefined method `" + method + "'")
		}
		return core.R.NilVal
	}
	if methodObj.Visibility == "undefined" {
		return core.NewNoMethodError("undefined method `" + method + "'")
	}
	if !vm.visibilityBypass && method != "public" && method != "private" && method != "protected" &&
		method != "module_function" && method != "public_class_method" && method != "private_class_method" &&
		method != "using" && method != "refine" &&
		(methodObj.Visibility == "private" || methodObj.Visibility == "protected") && receiver != core.R.Main {
		return core.NewNoMethodError(methodObj.Visibility + " method `" + method + "' called")
	}
	if fn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
		return fn(receiver, args...)
	}

	if fn, ok := methodObj.Fn.(*object.Function); ok {
		if methodObj.EnforceArity {
			if err := methodArityError(fn, len(args)); err != nil {
				return err
			}
		}
		oldFrame := vm.frames[vm.fp]

		bp := vm.sp

		vm.stack[vm.sp] = receiver
		vm.sp++

		if len(fn.KeywordParams) > 0 && len(args) > 0 {
			lastArg := args[len(args)-1]
			positionalArgs := args[:len(args)-1]

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

			var kwargsHash map[*object.EmeraldValue]*object.EmeraldValue
			if lastArg.Type == object.ValueHash {
				kwargsHash = lastArg.Data.(map[*object.EmeraldValue]*object.EmeraldValue)
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
					for key, value := range last.Data.(map[*object.EmeraldValue]*object.EmeraldValue) {
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

		if fn.HasBlockParam {
			blockVal := vm.currentBlock
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

		methodClosure := vm.detachedMethodClosure(methodObj)
		newFrame := &Frame{
			Fn:             fn,
			Ip:             -1,
			Bp:             bp,
			Closure:        methodClosure,
			MethodName:     methodName,
			Args:           args,
			Block:          vm.currentBlock,
			BlockBreakAddr: -1,
			WhileStart:     -1,
			WhileEnd:       -1,
		}
		if methodFromPrependedModule(receiver.Class, methodName, methodObj) {
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
			if frame.BlockBreak {
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
		return &object.EmeraldValue{
			Type: object.ValueObject,
			Data: &closureCell{
				value: derefClosureValue(&object.EmeraldValue{Type: value.Type, Data: cell, Class: value.Class}),
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

func (vm *VM) currentDefinitionVisibility() string {
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
		switch visibility.Data.(string) {
		case "private", "protected":
			return visibility.Data.(string)
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
	if !ok {
		return nil, "", false
	}
	stackIdx := frame.Bp + idx + 1
	if stackIdx < 0 || stackIdx >= len(vm.stack) {
		return nil, "", false
	}
	container := vm.stack[stackIdx]
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
		if value, ok := vm.qualifiedLexicalParentConstantValue(container, name); ok {
			return value, true
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
	return nil, false
}

func directConstantValue(container *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	switch container.Type {
	case object.ValueClass:
		class := container.Data.(*object.Class)
		if class.PrivateConstants[name] {
			return core.NewNameError("private constant " + class.Name + "::" + name + " referenced"), true
		}
		value, ok := class.Constants[name]
		return value, ok
	case object.ValueModule:
		module := container.Data.(*object.Module)
		if module.PrivateConstants[name] {
			return core.NewNameError("private constant " + module.Name + "::" + name + " referenced"), true
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
	vm.rescueStack = vm.rescueStack[:len(vm.rescueStack)-1]
	if handler.RescueOffset > 0 {
		handler.Frame.Ip = handler.RescueOffset - 1
	} else if handler.EnsureOffset > 0 {
		handler.Frame.Ip = handler.EnsureOffset - 1
	} else {
		handler.Frame.Ip = handler.EndOffset - 1
	}
	vm.ensureActive = false
	return true
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
	if value.Type == object.ValueObject {
		if obj, ok := value.Data.(*object.Object); ok {
			if _, ok := obj.SingletonMethods[name]; ok {
				return true
			}
			if obj.SingletonClass != nil {
				if _, ok := obj.SingletonClass.Methods[name]; ok {
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

func (vm *VM) callBlock(block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
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

	var fn *object.Function
	var closure *object.Closure
	isLambda := false
	autoSplat := false
	switch block.Type {
	case object.ValueClosure:
		closure = block.Data.(*object.Closure)
		fn = closure.Fn
		autoSplat = closure.AutoSplat
	case object.ValueProc:
		proc := block.Data.(*object.Proc)
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
	if isLambda && len(args) != len(fn.Params) && !fn.HasRestParam {
		return core.NewArgumentError("wrong number of arguments")
	}
	if autoSplat && !isLambda && len(args) == 1 && len(fn.Params) > 1 && args[0] != nil && args[0].Type == object.ValueArray {
		args = args[0].Data.([]*object.EmeraldValue)
	}

	prevBlock := vm.currentBlock
	vm.currentBlock = nil
	prevClassStack := vm.classStack
	if len(closure.ClassStack) > 0 {
		vm.classStack = append([]*object.EmeraldValue(nil), closure.ClassStack...)
		if self != nil && (self.Type == object.ValueClass || self.Type == object.ValueModule) {
			vm.classStack = append(vm.classStack, self)
		}
	}
	defer func() {
		vm.currentBlock = prevBlock
		vm.classStack = prevClassStack
	}()

	bp := vm.sp

	vm.stack[vm.sp] = self
	vm.sp++

	for i := 0; i < fn.NumLocals; i++ {
		vm.stack[bp+1+i] = core.R.NilVal
	}
	for _, arg := range args {
		vm.stack[vm.sp] = arg
		vm.sp++
	}
	minSp := bp + 1 + fn.NumLocals
	if vm.sp < minSp {
		vm.sp = minSp
	}

	newFrame := &Frame{Fn: fn, Ip: -1, Bp: bp, Closure: closure, Block: closure.Block, BlockBreak: false, BlockBreakVal: nil, BlockNextVal: nil, BlockBreakAddr: -1, WhileStart: -1, WhileEnd: -1}
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
		if frame.BlockBreak {
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
		core.LastBlockResult = result
	} else if frame.BlockNextVal != nil {
		result = frame.BlockNextVal
	} else {
		core.LastBlockResult = nil
	}

	vm.frames = vm.frames[:vm.fp]
	vm.fp--

	return result
}

func (vm *VM) currentFrameBinding() *object.RBinding {
	if vm.fp < 0 || vm.fp >= len(vm.frames) {
		return &object.RBinding{Self: core.R.Main, Locals: map[string]*object.EmeraldValue{}, Constants: vm.rubyConsts}
	}
	frame := vm.frames[vm.fp]
	locals := make(map[string]*object.EmeraldValue)
	if frame != nil && frame.Fn != nil {
		for i, name := range frame.Fn.Params {
			slot := frame.Bp + 1 + i
			if slot >= 0 && slot < vm.sp && vm.stack[slot] != nil {
				locals[name] = vm.stack[slot]
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
			}
		}
	}
	self := core.R.Main
	if frame != nil && frame.Bp >= 0 && frame.Bp < vm.sp && vm.stack[frame.Bp] != nil {
		self = vm.stack[frame.Bp]
	}
	return &object.RBinding{Self: self, Locals: locals, Constants: vm.rubyConsts}
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
				return core.NewNameError("private constant " + prefix + "::" + constName + " referenced"), true
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
				return core.NewNameError("private constant " + prefix + "::" + constName + " referenced"), true
			}
			if constant, ok := module.GetConstant(constName); ok {
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
	if cls, ok := core.R.Classes[prefix]; ok {
		if cls.PrivateConstants[constName] {
			return core.NewNameError("private constant " + prefix + "::" + constName + " referenced"), true
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

func (vm *VM) scopedConstantValue(receiver *object.EmeraldValue, constName string) (*object.EmeraldValue, bool) {
	if receiver == nil {
		return nil, false
	}
	switch receiver.Type {
	case object.ValueClass:
		class := receiver.Data.(*object.Class)
		if class.PrivateConstants[constName] {
			return core.NewNameError("private constant " + class.Name + "::" + constName + " referenced"), true
		}
		if constant, ok := class.GetConstant(constName); ok {
			return constant, true
		}
		if constant, ok := vm.triggerAutoload(receiver, constName); ok {
			return constant, true
		}
		if core.EvaluatingRaiseErrorMatcher() {
			return core.NewNameError("uninitialized constant " + class.Name + "::" + constName), true
		}
		return nil, false
	case object.ValueModule:
		module := receiver.Data.(*object.Module)
		if module.PrivateConstants[constName] {
			return core.NewNameError("private constant " + module.Name + "::" + constName + " referenced"), true
		}
		if constant, ok := module.GetConstant(constName); ok {
			return constant, true
		}
		if constant, ok := vm.triggerAutoload(receiver, constName); ok {
			return constant, true
		}
		if core.EvaluatingRaiseErrorMatcher() {
			return core.NewNameError("uninitialized constant " + module.Name + "::" + constName), true
		}
		return nil, false
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
