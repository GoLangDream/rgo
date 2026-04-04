package vm

import (
	"fmt"
	"os"
	"runtime"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

const StackSize = 2048
const MaxFrames = 1024

var DevMode = os.Getenv("RGO_DEV") == "1"

var CurrentVM *VM

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

	// Block control flow
	BlockBreakAddr int                  // jump target for break inside while loop
	WhileStart     int                  // start IP of the while loop body
	WhileEnd       int                  // end IP of the while loop body
	BlockBreak     bool                 // true if break was executed in this block
	BlockBreakVal  *object.EmeraldValue // value returned by break
	BlockNextVal   *object.EmeraldValue // value returned by next
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
	constants  []*object.EmeraldValue
	globals    []*object.EmeraldValue
	rubyConsts map[string]*object.EmeraldValue

	stack []*object.EmeraldValue
	sp    int

	frames []*Frame
	fp     int

	instructions compiler.Instructions

	poppedValues []*object.EmeraldValue

	currentBlock *object.EmeraldValue
	classStack   []*object.EmeraldValue

	rescueStack  []*RescueHandler
	ensureActive bool

	catchStack []*CatchHandler
}

func New(bytecode *compiler.Bytecode) *VM {
	core.Init()

	mainFn := &object.Function{
		Name:         "__main__",
		Instructions: bytecode.Instructions,
		NumLocals:    0,
	}

	mainFrame := &Frame{
		Fn: mainFn,
		Ip: -1,
		Bp: 0,
	}

	vm := &VM{
		constants:    bytecode.Constants,
		globals:      make([]*object.EmeraldValue, 100),
		rubyConsts:   make(map[string]*object.EmeraldValue),
		stack:        make([]*object.EmeraldValue, StackSize),
		sp:           0,
		frames:       []*Frame{mainFrame},
		fp:           0,
		instructions: bytecode.Instructions,
	}

	vm.stack[0] = core.R.Main
	CurrentVM = vm
	core.CallBlock = CallBlock
	core.CallMethod = func(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
		return vm.send(receiver, method, args)
	}
	core.CallBlockWithArgs = func(block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		return vm.callBlock(block, args...)
	}

	return vm
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

func (vm *VM) execute(op compiler.Opcode, frame *Frame) error {
	switch op {
	case compiler.OpConstant:
		idx := vm.readUint16()
		vm.push(vm.constants[idx])

	case compiler.OpTrue:
		vm.push(core.R.TrueVal)

	case compiler.OpFalse:
		vm.push(core.R.FalseVal)

	case compiler.OpNil:
		vm.push(core.R.NilVal)

	case compiler.OpPop:
		vm.pop()

	case compiler.OpAdd:
		right := vm.pop()
		left := vm.pop()
		result := vm.add(left, right)
		vm.push(result)

	case compiler.OpSub:
		right := vm.pop()
		left := vm.pop()
		result := vm.sub(left, right)
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
		vm.push(result)

	case compiler.OpGreaterThanOrEqual:
		right := vm.pop()
		left := vm.pop()
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
		vm.push(result)

	case compiler.OpLessThanOrEqual:
		right := vm.pop()
		left := vm.pop()
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
			vm.push(core.R.NilVal)
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
			vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l << r, Class: core.R.Classes["Integer"]})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpBitRightShift:
		right := vm.pop()
		left := vm.pop()
		l, lok := left.Data.(int64)
		r, rok := right.Data.(int64)
		if lok && rok {
			vm.push(&object.EmeraldValue{Type: object.ValueInteger, Data: l >> r, Class: core.R.Classes["Integer"]})
		} else {
			vm.push(core.R.NilVal)
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
		vm.push(result)

	case compiler.OpIndexAssign:
		value := vm.pop()
		index := vm.pop()
		left := vm.pop()
		result := vm.indexAssign(left, index, value)
		vm.push(result)

	case compiler.OpGetGlobal:
		idx := vm.readUint16()
		vm.push(vm.globals[idx])

	case compiler.OpSetGlobal:
		idx := vm.readUint16()
		vm.globals[idx] = vm.peek(0)

	case compiler.OpGetConstant:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)
		if val, ok := vm.rubyConsts[name]; ok {
			vm.push(val)
		} else if cls, ok := core.R.Classes[name]; ok {
			vm.push(&object.EmeraldValue{
				Type:  object.ValueClass,
				Data:  cls,
				Class: core.R.Classes["Class"],
			})
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetConstant:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)
		vm.rubyConsts[name] = vm.peek(0)
		if len(vm.classStack) > 0 {
			top := vm.classStack[len(vm.classStack)-1]
			if top.Type == object.ValueClass && top.Data.(*object.Class).Name == name {
				vm.classStack = vm.classStack[:len(vm.classStack)-1]
			}
		}

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

	case compiler.OpGetInstanceVar:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)
		receiver := vm.stack[frame.Bp]
		if obj, ok := receiver.Data.(*object.Object); ok {
			if val, ok := obj.InstanceVars[name]; ok {
				vm.push(val)
			} else {
				vm.push(core.R.NilVal)
			}
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetInstanceVar:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)
		val := vm.peek(0)
		receiver := vm.stack[frame.Bp]
		if obj, ok := receiver.Data.(*object.Object); ok {
			obj.InstanceVars[name] = val
		}

	case compiler.OpGetClassVar:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)
		receiver := vm.stack[frame.Bp]
		if obj, ok := receiver.Data.(*object.Object); ok {
			if val, ok := obj.ClassVars[name]; ok {
				vm.push(val)
			} else {
				vm.push(core.R.NilVal)
			}
		} else {
			vm.push(core.R.NilVal)
		}

	case compiler.OpSetClassVar:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)
		val := vm.peek(0)
		receiver := vm.stack[frame.Bp]
		if obj, ok := receiver.Data.(*object.Object); ok {
			obj.ClassVars[name] = val
		}

	case compiler.OpGetFree:
		idx := vm.readUint8()
		vm.push(frame.Closure.Free[idx])

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
		methodName := vm.constants[methodNameIdx].Data.(string)

		args := make([]*object.EmeraldValue, int(numArgs))
		for i := 0; i < int(numArgs); i++ {
			args[numArgs-1-i] = vm.pop()
		}

		var block *object.EmeraldValue
		if blockArg == 1 {
			block = vm.pop()
		}
		receiver := vm.pop()

		prevBlock := vm.currentBlock
		vm.currentBlock = block
		result := vm.send(receiver, methodName, args)
		vm.currentBlock = prevBlock
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

	case compiler.OpYield:
		result := vm.callBlock(vm.currentBlock)
		vm.push(result)

	case compiler.OpYieldWithValue:
		numArgs := int(vm.readUint8())
		args := make([]*object.EmeraldValue, numArgs)
		for i := numArgs - 1; i >= 0; i-- {
			args[i] = vm.pop()
		}
		result := vm.callBlock(vm.currentBlock, args...)
		vm.push(result)

	case compiler.OpDefineMethod:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)

		closureVal := vm.pop()
		closure, ok := closureVal.Data.(*object.Closure)
		if !ok {
			return fmt.Errorf("expected closure, got %T", closureVal.Data)
		}

		method := &object.Method{
			Name: name,
			Fn:   closure.Fn,
		}

		if len(vm.classStack) > 0 {
			classVal := vm.classStack[len(vm.classStack)-1]
			cls := classVal.Data.(*object.Class)
			cls.DefineMethod(name, method)
		} else {
			mainObj := core.R.Main.Data.(*object.Object)
			mainObj.Class.DefineMethod(name, method)
		}

		vm.push(closureVal)

	case compiler.OpDefineClassMethod:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)

		fn := &object.Function{
			Name:         name,
			Instructions: vm.pop().Data.([]byte),
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
		name := vm.constants[nameIdx].Data.(string)

		var class *object.Class
		if existing, ok := vm.rubyConsts[name]; ok && existing.Type == object.ValueClass {
			class = existing.Data.(*object.Class)
		} else {
			class = object.NewClass(name)
			class.SuperClass = core.R.Classes["Object"]
		}

		classVal := &object.EmeraldValue{
			Type:  object.ValueClass,
			Data:  class,
			Class: core.R.Classes["Class"],
		}
		vm.rubyConsts[name] = classVal
		vm.classStack = append(vm.classStack, classVal)
		vm.push(classVal)

	case compiler.OpModule:
		nameIdx := vm.readUint16()
		name := vm.constants[nameIdx].Data.(string)

		module := object.NewModule(name)

		vm.push(&object.EmeraldValue{
			Type:  object.ValueModule,
			Data:  module,
			Class: core.R.Classes["Module"],
		})

	case compiler.OpDup:
		vm.push(vm.peek(0))

	case compiler.OpClosure:
		fnIdx := vm.readUint16()
		numFree := vm.readUint8()

		constant := vm.constants[fnIdx]
		fn, ok := constant.Data.(*object.Function)
		if !ok {
			return fmt.Errorf("not a function: %v", constant)
		}

		free := make([]*object.EmeraldValue, numFree)
		for i := numFree - 1; i >= 0; i-- {
			free[i] = vm.pop()
		}

		closure := &object.Closure{
			Fn:   fn,
			Free: free,
		}

		vm.push(&object.EmeraldValue{
			Type:  object.ValueClosure,
			Data:  closure,
			Class: core.R.Classes["Proc"],
		})

	case compiler.OpLambda:
		fnIdx := vm.readUint16()
		numFree := vm.readUint8()

		fn, ok := vm.constants[fnIdx].Data.(*object.Function)
		if !ok {
			return fmt.Errorf("not a function: %v", vm.constants[fnIdx])
		}

		free := make([]*object.EmeraldValue, numFree)
		for i := numFree - 1; i >= 0; i-- {
			free[i] = vm.pop()
		}

		proc := &object.Proc{
			Fn:       fn,
			Env:      free,
			IsLambda: true,
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
			exception = &object.EmeraldValue{
				Type:  object.ValueException,
				Data:  &object.RException{Message: "RuntimeError"},
				Class: core.R.Classes["RuntimeError"],
			}
		}
		core.LastException = exception

		if len(vm.rescueStack) > 0 {
			handler := vm.rescueStack[len(vm.rescueStack)-1]
			if handler.Frame == frame {
				vm.rescueStack = vm.rescueStack[:len(vm.rescueStack)-1]
				if handler.EnsureOffset > 0 {
					handler.Frame.Ip = handler.EnsureOffset - 1
				} else {
					handler.Frame.Ip = handler.EndOffset - 1
				}
				vm.ensureActive = false
				return nil
			}
		}

	case compiler.OpRescue:
		if core.LastException == nil {
			vm.push(core.R.FalseVal)
			return nil
		}
		if len(vm.rescueStack) == 0 {
			vm.push(core.R.FalseVal)
			return nil
		}

	case compiler.OpCatch:
		labelIdx := vm.readUint16()

		label := vm.constants[labelIdx]
		endOffset := vm.readUint16()

		handler := &CatchHandler{
			Label:     label,
			EndOffset: endOffset,
			Frame:     frame,
		}
		vm.catchStack = append(vm.catchStack, handler)

	case compiler.OpThrow:
		var label *object.EmeraldValue
		if vm.sp > 0 {
			label = vm.pop()
		}
		var value *object.EmeraldValue
		if vm.sp > 0 {
			value = vm.pop()
		}

		for i := len(vm.catchStack) - 1; i >= 0; i-- {
			handler := vm.catchStack[i]
			if handler.Label != nil && handler.Label.Equals(label) {
				vm.catchStack = vm.catchStack[:i]
				if value != nil {
					vm.push(value)
				}
				handler.Frame.Ip = handler.EndOffset - 1
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
	vm.poppedValues = append(vm.poppedValues, val)
	return val
}

func (vm *VM) peek(n int) *object.EmeraldValue {
	return vm.stack[vm.sp-1-n]
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
	}
	return core.R.NilVal
}

func (vm *VM) div(left, right *object.EmeraldValue) *object.EmeraldValue {
	switch l := left.Data.(type) {
	case int64:
		switch r := right.Data.(type) {
		case int64:
			if r == 0 {
				return core.R.NilVal
			}
			return &object.EmeraldValue{Type: object.ValueInteger, Data: l / r, Class: core.R.Classes["Integer"]}
		case float64:
			if r == 0 {
				return core.R.NilVal
			}
			return &object.EmeraldValue{Type: object.ValueFloat, Data: float64(l) / r, Class: core.R.Classes["Float"]}
		}
	case float64:
		switch r := right.Data.(type) {
		case int64:
			if r == 0 {
				return core.R.NilVal
			}
			return &object.EmeraldValue{Type: object.ValueFloat, Data: l / float64(r), Class: core.R.Classes["Float"]}
		case float64:
			if r == 0 {
				return core.R.NilVal
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
			if r < 0 {
				return &object.EmeraldValue{Type: object.ValueFloat, Data: 1.0 / vm.powInt(l, -int(r)), Class: core.R.Classes["Float"]}
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

func (vm *VM) powInt(base int64, exp int) int64 {
	result := int64(1)
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
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
		l[index] = value
	}
	return value
}

func (vm *VM) send(receiver *object.EmeraldValue, method string, args []*object.EmeraldValue) *object.EmeraldValue {
	if method == "__exec_class_body__" && receiver.Type == object.ValueClass && vm.currentBlock != nil {
		block := vm.currentBlock
		vm.currentBlock = nil
		return vm.callBlock(block, receiver)
	}

	var methodObj *object.Method
	var ok bool

	if receiver.Type == object.ValueClass {
		cls := receiver.Data.(*object.Class)
		if m, found := cls.ClassMethods[method]; found {
			methodObj = m
			ok = true
		}
	}

	if !ok {
		methodObj, ok = receiver.Class.GetMethod(method)
	}

	if !ok {
		return core.R.NilVal
	}

	if fn, ok := methodObj.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue); ok {
		return fn(receiver, args...)
	}

	if fn, ok := methodObj.Fn.(*object.Function); ok {
		oldFrame := vm.frames[vm.fp]

		bp := vm.sp

		vm.stack[vm.sp] = receiver
		vm.sp++

		if len(fn.KeywordParams) > 0 && len(args) > 0 {
			lastArg := args[len(args)-1]
			positionalArgs := args[:len(args)-1]

			if fn.HasRestParam {
				normalCount := fn.RestParamIndex
				if normalCount > len(positionalArgs) {
					normalCount = len(positionalArgs)
				}
				for i := 0; i < normalCount; i++ {
					vm.stack[vm.sp] = positionalArgs[i]
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
				for _, arg := range positionalArgs {
					vm.stack[vm.sp] = arg
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
			if normalCount > len(args) {
				normalCount = len(args)
			}
			for i := 0; i < normalCount; i++ {
				vm.stack[vm.sp] = args[i]
				vm.sp++
			}
			restElems := make([]*object.EmeraldValue, 0)
			if len(args) > fn.RestParamIndex {
				restElems = args[fn.RestParamIndex:]
			}
			vm.stack[vm.sp] = &object.EmeraldValue{
				Type:  object.ValueArray,
				Data:  restElems,
				Class: core.R.Classes["Array"],
			}
			vm.sp++
		} else {
			for _, arg := range args {
				vm.stack[vm.sp] = arg
				vm.sp++
			}
		}

		newFrame := &Frame{
			Fn: fn,
			Ip: -1,
			Bp: bp,
		}
		vm.frames = append(vm.frames, newFrame)
		vm.fp++

		frame := vm.frames[vm.fp]
		instructions := frame.Fn.Instructions

		for frame.Ip < len(instructions)-1 {
			frame.Ip++
			op := compiler.Opcode(instructions[frame.Ip])
			err := vm.execute(op, frame)
			if err != nil {
				return core.R.NilVal
			}
			frame = vm.frames[vm.fp]
			instructions = frame.Fn.Instructions
		}

		result := core.R.NilVal
		if vm.sp > bp {
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

func (vm *VM) callBlock(block *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if block == nil {
		return core.R.NilVal
	}

	var fn *object.Function
	var closure *object.Closure
	switch block.Type {
	case object.ValueClosure:
		closure = block.Data.(*object.Closure)
		fn = closure.Fn
	case object.ValueProc:
		proc := block.Data.(*object.Proc)
		fn = proc.Fn
	default:
		return core.R.NilVal
	}

	if fn == nil {
		return core.R.NilVal
	}

	bp := vm.sp

	vm.stack[vm.sp] = core.R.Main
	vm.sp++

	for _, arg := range args {
		vm.stack[vm.sp] = arg
		vm.sp++
	}

	newFrame := &Frame{Fn: fn, Ip: -1, Bp: bp, Closure: closure, BlockBreak: false, BlockBreakVal: nil, BlockNextVal: nil, BlockBreakAddr: -1, WhileStart: -1, WhileEnd: -1}
	vm.frames = append(vm.frames, newFrame)
	vm.fp++

	frame := vm.frames[vm.fp]
	instructions := frame.Fn.Instructions
	for frame.Ip < len(instructions)-1 {
		frame.Ip++
		op := compiler.Opcode(instructions[frame.Ip])
		if err := vm.execute(op, frame); err != nil {
			break
		}
		frame = vm.frames[vm.fp]
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

func (vm *VM) LastPoppedStackElement() *object.EmeraldValue {
	if vm.sp > 0 {
		return vm.stack[vm.sp-1]
	}
	if len(vm.poppedValues) > 0 {
		return vm.poppedValues[len(vm.poppedValues)-1]
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
