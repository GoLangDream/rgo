package vm

import (
	"strings"
	"testing"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

func typedSSAFunction(instructions []byte, constants []*object.EmeraldValue) *object.Function {
	return &object.Function{
		Name:              "typed_ssa_test",
		Instructions:      instructions,
		Constants:         constants,
		Params:            []string{"value"},
		ParamLocalIndices: []int{0},
		NumLocals:         1,
	}
}

func TestTypedSSACompilesAndExecutesBranchWithoutBoxedRegisters(t *testing.T) {
	// if value > 0; value + 1; else; value - 1; end
	instructions := compiler.Make(compiler.OpGetLocalFast, 0)
	instructions = append(instructions, compiler.Make(compiler.OpConstant, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpGreaterThan)...)
	jumpPosition := len(instructions)
	instructions = append(instructions, compiler.Make(compiler.OpJumpNotTruthy, 0, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpGetLocalFast, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpConstant, 1)...)
	instructions = append(instructions, compiler.Make(compiler.OpAdd)...)
	instructions = append(instructions, compiler.Make(compiler.OpReturnValue)...)
	elsePosition := len(instructions)
	instructions[jumpPosition+1] = byte(elsePosition >> 8)
	instructions[jumpPosition+2] = byte(elsePosition)
	instructions = append(instructions, compiler.Make(compiler.OpGetLocalFast, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpConstant, 2)...)
	instructions = append(instructions, compiler.Make(compiler.OpSub)...)
	instructions = append(instructions, compiler.Make(compiler.OpReturnValue)...)

	fn := typedSSAFunction(instructions, []*object.EmeraldValue{core.NewIntegerValue(0), core.NewIntegerValue(1), core.NewIntegerValue(1)})
	plan, ok := compileTypedSSAPlan(fn)
	if !ok || plan == nil || len(plan.ops) == 0 || len(plan.integerOps) != 3 {
		t.Fatalf("expected typed branch plan, got ok=%t plan=%#v", ok, plan)
	}
	vm := compileTestVM(t, "")
	positive, executed := vm.executeTypedSSAPlan(plan, fn, core.R.Main, []*object.EmeraldValue{core.NewIntegerValue(4)})
	if !executed || positive.Inspect() != "5" {
		t.Fatalf("positive typed branch result=%v executed=%t", positive, executed)
	}
	negative, executed := vm.executeTypedSSAPlan(plan, fn, core.R.Main, []*object.EmeraldValue{core.NewIntegerValue(-4)})
	if !executed || negative.Inspect() != "-5" {
		t.Fatalf("negative typed branch result=%v executed=%t", negative, executed)
	}
	raw, executed := vm.executeTypedSSAUnboxedArgsPlanTrusted(plan, fn, []int64{4})
	if !executed || raw.kind != typedSSAInteger || raw.int != 5 {
		t.Fatalf("unboxed branch kernel result=%#v executed=%t", raw, executed)
	}
}

func TestTypedSSABitwisePlanPreservesExactIntegerSemantics(t *testing.T) {
	instructions := compiler.Make(compiler.OpGetLocalFast, 0)
	instructions = append(instructions, compiler.Make(compiler.OpConstant, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpBitLeftShift)...)
	instructions = append(instructions, compiler.Make(compiler.OpReturnValue)...)
	fn := typedSSAFunction(instructions, []*object.EmeraldValue{core.NewIntegerValue(2)})
	plan, ok := compileTypedSSAPlan(fn)
	if !ok || plan == nil {
		t.Fatal("expected typed bitwise plan")
	}
	vm := compileTestVM(t, "")
	result, executed := vm.executeTypedSSAPlan(plan, fn, core.R.Main, []*object.EmeraldValue{core.NewIntegerValue(10)})
	if !executed || result == nil || result.Inspect() != "40" {
		t.Fatalf("typed bitwise result=%v executed=%t", result, executed)
	}
}

func TestTypedSSAFloatArithmeticUsesRawValue(t *testing.T) {
	instructions := append(compiler.Make(compiler.OpGetLocalFast, 0), compiler.Make(compiler.OpConstant, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpAdd)...)
	instructions = append(instructions, compiler.Make(compiler.OpReturnValue)...)
	fn := typedSSAFunction(instructions, []*object.EmeraldValue{core.NewFloatValue(1.5)})
	plan, ok := compileTypedSSAPlan(fn)
	if !ok || plan == nil || !plan.hasFloat {
		t.Fatalf("expected float typed plan, got ok=%t plan=%#v", ok, plan)
	}
	vm := compileTestVM(t, "")
	result, executed := vm.executeTypedSSAPlan(plan, fn, core.R.Main, []*object.EmeraldValue{core.NewFloatValue(2.25)})
	if !executed || result == nil || result.Type != object.ValueFloat || result.Data.(float64) != 3.75 {
		t.Fatalf("raw float arithmetic result=%v executed=%t", result, executed)
	}
}

func TestTypedSSAFloatBranchDivisionAndModuloPreserveRubySemantics(t *testing.T) {
	result, _ := runRuby(t, `
def float_step(value)
  if value < 10.0
    value * 1.5 + 0.25
  else
    value / 2.0
  end
end

[float_step(2.0), float_step(12.0), 2.5 % 1.0]
`)
	if result.Inspect() != "[3.25, 6.0, 0.5]" {
		t.Fatalf("typed float branch changed Ruby result: %s", result.Inspect())
	}
}

func TestTypedSSAFloatBuiltinGuardRejectsRedefinition(t *testing.T) {
	vm := compileTestVM(t, "")
	floatClass := core.R.Classes["Float"]
	previous, existed := floatClass.Methods["+"]
	defer func() {
		if existed {
			floatClass.Methods["+"] = previous
		} else {
			delete(floatClass.Methods, "+")
		}
	}()
	floatClass.DefineMethod("+", &object.Method{
		Name: "+",
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return core.NewIntegerValue(99)
		},
		Arity: 1,
	})
	if core.FloatPlusUsesBuiltinImplementation() {
		t.Fatal("Float#+ builtin guard stayed enabled after method replacement")
	}
	instructions := append(compiler.Make(compiler.OpGetLocalFast, 0), compiler.Make(compiler.OpConstant, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpAdd)...)
	instructions = append(instructions, compiler.Make(compiler.OpReturnValue)...)
	fn := typedSSAFunction(instructions, []*object.EmeraldValue{core.NewFloatValue(1.0)})
	plan, ok := compileTypedSSAPlan(fn)
	if !ok || plan == nil {
		t.Fatal("expected float plan")
	}
	if result, executed := vm.executeTypedSSAPlan(plan, fn, core.R.Main, []*object.EmeraldValue{core.NewFloatValue(1.0)}); executed || result != nil {
		t.Fatalf("redefined Float#+ must side-exit typed SSA: result=%v executed=%t", result, executed)
	}
}

func TestTypedSSAStringArithmeticUsesRawValue(t *testing.T) {
	instructions := append(compiler.Make(compiler.OpGetLocalFast, 0), compiler.Make(compiler.OpConstant, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpAdd)...)
	instructions = append(instructions, compiler.Make(compiler.OpReturnValue)...)
	fn := typedSSAFunction(instructions, []*object.EmeraldValue{core.NewStringValue("!")})
	plan, ok := compileTypedSSAPlan(fn)
	if !ok || plan == nil || !plan.hasString {
		t.Fatalf("expected raw string typed plan, got ok=%t plan=%#v", ok, plan)
	}
	vm := compileTestVM(t, "")
	result, executed := vm.executeTypedSSAPlan(plan, fn, core.R.Main, []*object.EmeraldValue{core.NewStringValue("hello")})
	if !executed || result == nil || result.Type != object.ValueString || result.Data.(string) != "hello!" {
		t.Fatalf("raw string arithmetic result=%v executed=%t", result, executed)
	}
}

func TestTypedSSAStringEqualityPreservesRubySemantics(t *testing.T) {
	result, _ := runRuby(t, `
def same(value)
  value == "ok"
end

[same("ok"), same("no")]
`)
	if result.Inspect() != "[true, false]" {
		t.Fatalf("typed string equality changed Ruby result: %s", result.Inspect())
	}
}

func TestTypedSSAStringBuiltinGuardRejectsRedefinition(t *testing.T) {
	vm := compileTestVM(t, "")
	stringClass := core.R.Classes["String"]
	previous, existed := stringClass.Methods["+"]
	defer func() {
		if existed {
			stringClass.Methods["+"] = previous
		} else {
			delete(stringClass.Methods, "+")
		}
	}()
	stringClass.DefineMethod("+", &object.Method{
		Name: "+",
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return core.NewStringValue("redefined")
		},
		Arity: 1,
	})
	if core.StringPlusUsesBuiltinImplementation() {
		t.Fatal("String#+ builtin guard stayed enabled after method replacement")
	}
	instructions := append(compiler.Make(compiler.OpGetLocalFast, 0), compiler.Make(compiler.OpConstant, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpAdd)...)
	instructions = append(instructions, compiler.Make(compiler.OpReturnValue)...)
	fn := typedSSAFunction(instructions, []*object.EmeraldValue{core.NewStringValue("!")})
	plan, ok := compileTypedSSAPlan(fn)
	if !ok || plan == nil {
		t.Fatal("expected string plan")
	}
	if result, executed := vm.executeTypedSSAPlan(plan, fn, core.R.Main, []*object.EmeraldValue{core.NewStringValue("hello")}); executed || result != nil {
		t.Fatalf("redefined String#+ must side-exit typed SSA: result=%v executed=%t", result, executed)
	}
}

func TestTypedSSANestedReferenceCallPreservesResult(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSANestedCallBox
  def initialize
    @value = 7
  end

  def inner
    @value
  end

  def outer
    inner
  end
end
TypedSSANestedCallBox.new.outer
`)
	if result == nil || result.Inspect() != "7" {
		t.Fatalf("nested typed reference call result=%v", result)
	}
}

func TestTypedSSAReferenceMethodPreservesObjectIdentity(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAReferenceIdentityBox
  def initialize
    @value = "hello"
  end

  def inner
    @value
  end

  def outer
    inner
  end
end

box = TypedSSAReferenceIdentityBox.new
value = box.outer
[value.equal?(box.instance_variable_get(:@value)), value]
`)
	if result == nil || result.Inspect() != `[true, "hello"]` {
		t.Fatalf("typed reference method changed object identity: %v", result)
	}
}

func TestTypedSSADeoptsBeforeUnsupportedReferenceArithmetic(t *testing.T) {
	instructions := append(compiler.Make(compiler.OpGetLocalFast, 0), compiler.Make(compiler.OpConstant, 0)...)
	instructions = append(instructions, compiler.Make(compiler.OpAdd)...)
	instructions = append(instructions, compiler.Make(compiler.OpReturnValue)...)
	fn := typedSSAFunction(instructions, []*object.EmeraldValue{core.NewIntegerValue(1)})
	plan, ok := compileTypedSSAPlan(fn)
	if !ok || plan == nil {
		t.Fatal("expected typed arithmetic plan")
	}
	vm := compileTestVM(t, "")
	stringValue := &object.EmeraldValue{Type: object.ValueString, Data: "x", Class: core.R.Classes["String"]}
	if result, executed := vm.executeTypedSSAPlan(plan, fn, core.R.Main, []*object.EmeraldValue{stringValue}); executed || result != nil {
		t.Fatalf("unsupported reference arithmetic must deopt without a result: result=%v executed=%t", result, executed)
	}
}

func TestTypedSSAUnboxedMultiArgumentRegionPreservesBranches(t *testing.T) {
	result, _ := runRuby(t, `
def clamp(value, low, high)
  if value < low
    low
  elsif value > high
    high
  else
    value
  end
end

[clamp(-3, 0, 10), clamp(4, 0, 10), clamp(99, 0, 10)]
`)
	if result.Inspect() != "[0, 4, 10]" {
		t.Fatalf("unboxed multi-argument region changed Ruby result: %s", result.Inspect())
	}
}

func TestTypedSSAUnboxedIntegerCallLoopPreservesClampSemantics(t *testing.T) {
	result, _ := runRuby(t, `
def clamp(value, low, high)
  if value < low
    low
  elsif value > high
    high
  else
    value
  end
end

total = 0
index = -5
while index < 6
  total += clamp(index, 0, 3)
  index += 1
end
[total, index]
`)
	if result.Inspect() != "[12, 6]" {
		t.Fatalf("unboxed integer call loop changed clamp result: %s", result.Inspect())
	}
}

func TestAggressiveHotMethodPreservesBranchAndDynamicSend(t *testing.T) {
	previous := registerIRAggressiveEnabled
	registerIRAggressiveEnabled = true
	defer func() { registerIRAggressiveEnabled = previous }()
	result, _ := runRuby(t, `
def render(value)
  if value > 0
    value.to_s
  else
    value.inspect
  end
end

[render(2), render(-1)]
`)
	if result.Inspect() != `["2", "-1"]` {
		t.Fatalf("aggressive method changed Ruby result: %s", result.Inspect())
	}
}

func TestTypedSSAGenerationGuardPreservesRedefinitionSemantics(t *testing.T) {
	result, _ := runRuby(t, `
def classify(value)
  if value
    value + 1
  else
    value - 1
  end
end

before = classify(1)
class Integer
  def +(other)
    99
  end
end
[before, classify(1)]
`)
	if result.Inspect() != "[2, 99]" {
		t.Fatalf("typed SSA ignored Integer redefinition: %s", result.Inspect())
	}
}

func TestTypedSSAIntegerLoopKernelPreservesBranchCallee(t *testing.T) {
	result, _ := runRuby(t, `
def classify(value)
  if value > 0
    value + 1
  else
    value - 1
  end
end

total = 0
index = 0
while index < 5
  total += classify(index)
  index += 1
end
total
`)
	if result.Inspect() != "13" {
		t.Fatalf("typed integer loop changed branch result: %s", result.Inspect())
	}
}

func TestTypedSSABlockCompilesBlockReturnAndPreservesMap(t *testing.T) {
	result, _ := runRuby(t, `
[1, 2, 3].map { |value| value > 1 ? value + 10 : value - 10 }
`)
	if result.Inspect() != "[-9, 12, 13]" {
		t.Fatalf("typed block changed map result: %s", result.Inspect())
	}
}

func TestTypedSSABlockCallGraphPreservesPureAndEffectfulSemantics(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSABlockCallHelper
  def add(value)
    value + 1
  end
end

helper = TypedSSABlockCallHelper.new
pure = [1, 2, 3].map { |value| helper.add(value) }
native = [1, 2, 3].map { |value| value.to_s }
lists = [[], []]
lists.map { |list| list << 7 }
[pure, native, lists]
`)
	if result == nil || result.Inspect() != `[[2, 3, 4], ["1", "2", "3"], [[7], [7]]]` {
		t.Fatalf("typed block call graph changed pure/native or effectful semantics: %v", result)
	}
}

func TestTypedSSABlockDeoptsForCapturedFreeValue(t *testing.T) {
	result, _ := runRuby(t, `
offset = 4
[1, 2].map { |value| value + offset }
`)
	if result.Inspect() != "[5, 6]" {
		t.Fatalf("captured block result changed after typed admission: %s", result.Inspect())
	}
}

func TestTypedSSATrustedNativeBranchPreservesSemanticsAndRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TrustedNativeBranchValue
end

values = [TrustedNativeBranchValue.new, 1, TrustedNativeBranchValue.new]
before = values.map { |item| item.is_a?(TrustedNativeBranchValue) ? item.class.to_s : item.to_s }

class TrustedNativeBranchValue
  def class
    :changed
  end
end

after = values.map { |item| item.is_a?(TrustedNativeBranchValue) ? item.class.to_s : item.to_s }
[before, after]
`)
	expected := `[["TrustedNativeBranchValue", "1", "TrustedNativeBranchValue"], ["changed", "1", "changed"]]`
	if result == nil || result.Inspect() != expected {
		t.Fatalf("trusted native branch changed semantics or ignored redefinition: %v", result)
	}
}

func TestTypedSSATrustedNativeBranchRefreshesConstants(t *testing.T) {
	result, _ := runRuby(t, `
class TrustedNativeConstLeft
end
class TrustedNativeConstRight
end

values = [TrustedNativeConstLeft.new, TrustedNativeConstRight.new]
before = values.map { |item| item.is_a?(TrustedNativeConstLeft) ? 1 : 0 }
TrustedNativeConstLeft = TrustedNativeConstRight
after = values.map { |item| item.is_a?(TrustedNativeConstLeft) ? 1 : 0 }
[before, after]
`)
	if result == nil || result.Inspect() != "[[1, 0], [0, 1]]" {
		t.Fatalf("trusted native branch reused a stale constant: %v", result)
	}
}

func TestTypedSSATrustedNativeBranchPreservesUnicodeLength(t *testing.T) {
	result, _ := runRuby(t, `
["abc", "hé", "你"].map { |item| item.is_a?(String) ? item.length : 0 }
`)
	if result == nil || result.Inspect() != "[3, 2, 1]" {
		t.Fatalf("trusted native branch changed Unicode length: %v", result)
	}
}

func TestTypedSSATrustedArrayArithmeticConsumerPreservesRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
values = Array.new(1024, "abc")
sum = 0
values.each { |item| sum += item.length }
before = sum

class String
  def length
    99
  end
end

sum = 0
values.each { |item| sum += item.length }
[before, sum]
`)
	if result == nil || result.Inspect() != "[3072, 101376]" {
		t.Fatalf("trusted Array arithmetic consumer ignored String#length redefinition: %v", result)
	}
}

func TestTypedSSAStringTypeBranchBatchPreservesRedefinitionAndMixedInput(t *testing.T) {
	result, _ := runRuby(t, `
values = Array.new(1024, "abc")
mixed = ["abc", 7]
before = [values.map { |item| item.is_a?(String) ? item.length : 0 }[0], mixed.map { |item| item.is_a?(String) ? item.length : 0 }]
class String
  def length
    99
  end
end
after_length = values.map { |item| item.is_a?(String) ? item.length : 0 }[0]
class String
  def is_a?(klass)
    false
  end
end
after_predicate = values.map { |item| item.is_a?(String) ? item.length : 0 }[0]
[before, after_length, after_predicate]
`)
	if result == nil || result.Inspect() != "[[3, [3, 0]], 99, 0]" {
		t.Fatalf("String type branch batch ignored mixed input or redefinition: %v", result)
	}
}

func TestRegisterIRStoreFreeTerminalWriteDeoptsAfterMethodRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
result = nil
16.times { |index| result = index.to_s }
before = result

class Integer
  def to_s
    "changed"
  end
end

16.times { |index| result = index.to_s }
[before, result]
`)
	if result == nil || result.Inspect() != `["15", "changed"]` {
		t.Fatalf("terminal captured write ignored a send redefinition: %v", result)
	}
}

func TestIntegerTimesTypedStringStorePreservesFinalMutableResult(t *testing.T) {
	result, _ := runRuby(t, `
result = nil
2048.times { |index| result = index.to_s }
before = result
result << "!"
first = [before, result, before.equal?(result)]
class Integer
  def to_s
    "changed"
  end
end
changed = nil
2048.times { |index| changed = index.to_s }
[first, changed]
`)
	if result == nil || result.Inspect() != `[["2047!", "2047!", true], "changed"]` {
		t.Fatalf("dead String result lowering changed final identity or mutability: %v", result)
	}
}

func TestIntegerTimesTypedStringBranchPreservesPredicateGuard(t *testing.T) {
	result, _ := runRuby(t, `
result = nil
2048.times { |index| result = index.is_a?(Integer) ? index.to_s : "" }
before = result
result << "!"
first = [before, result, before.equal?(result)]
class Object
  def is_a?(klass)
    false
  end
end
changed = nil
2048.times { |index| changed = index.is_a?(Integer) ? index.to_s : "" }
[first, changed]
`)
	if result == nil || result.Inspect() != `[["2047!", "2047!", true], ""]` {
		t.Fatalf("dead String branch lowering ignored predicate redefinition: %v", result)
	}
}

func TestCachedTypedHotMethodAdmitsTerminalInstanceWrite(t *testing.T) {
	vm := compileTestVM(t, `
class CachedTypedHotMutation
  def update(value)
    @value = value > 3 ? value.to_s : ""
  end
end
`)
	if err := vm.Run(); err != nil {
		t.Fatalf("class definition failed: %v", err)
	}
	classValue := vm.rubyConsts["CachedTypedHotMutation"]
	var cls *object.Class
	if classValue != nil {
		cls, _ = classValue.Data.(*object.Class)
	}
	if cls == nil {
		t.Fatal("class was not defined in VM constants")
	}
	method, _, found := cls.GetMethodWithOwner("update")
	if !found || method == nil {
		t.Fatal("update method was not defined")
	}
	fn, ok := method.Fn.(*object.Function)
	if !ok || fn == nil {
		t.Fatal("update is not a Ruby function")
	}
	plan, compiled := compileRegisterIR(fn)
	if !compiled || plan == nil {
		options := defaultRegisterIRCompileOptions()
		options.allowStringLiterals = true
		plan, compiled = compileRegisterIRWithOptions(fn, options)
	}
	if !compiled || plan == nil {
		t.Fatalf("Register IR rejected method: opcodes=%s sequence=%s bytecode=%v", registerIRUnsupportedOpcodeSummary(fn), registerIROpcodeSequence(fn), fn.Instructions)
	}
	if !registerIRPlanSafeForDirectNoFrameWithOptions(plan, false, true) {
		t.Fatalf("terminal instance write should be direct-safe: %s", registerIROpcodeSequence(fn))
	}
	receiver := object.NewObjectValue(cls)
	var executed bool
	var result *object.EmeraldValue
	for index := 0; index < registerIRDirectNoFrameWarmupCalls+2; index++ {
		result, executed = vm.tryExecuteCachedTypedHotMethod(method, receiver, []*object.EmeraldValue{core.NewIntegerValue(7)})
	}
	if !executed || result == nil || result.Type != object.ValueString || result.Data.(string) != "7" {
		t.Fatalf("cached typed hot method did not execute terminal write: result=%v executed=%t", result, executed)
	}
	stored := core.DynamicInstanceVar(receiver, "@value")
	if stored == nil || stored.Type != object.ValueString || stored.Data.(string) != "7" {
		t.Fatalf("terminal instance write lost value: %v", stored)
	}
}

func TestCachedTypedHotMethodPreservesCallerBlockContext(t *testing.T) {
	vm := compileTestVM(t, `
class CachedTypedHotBlockContext
  def convert(value)
    value + 1
  end
end
`)
	if err := vm.Run(); err != nil {
		t.Fatalf("class definition failed: %v", err)
	}
	classValue := vm.rubyConsts["CachedTypedHotBlockContext"]
	cls, _ := classValue.Data.(*object.Class)
	if cls == nil {
		t.Fatal("class was not defined in VM constants")
	}
	method, _, found := cls.GetMethodWithOwner("convert")
	if !found || method == nil {
		t.Fatal("convert method was not defined")
	}
	receiver := object.NewObjectValue(cls)
	marker := core.R.NilVal
	vm.currentBlock = marker
	for index := 0; index < registerIRDirectNoFrameWarmupCalls+2; index++ {
		result, executed := vm.tryExecuteCachedTypedHotMethod(method, receiver, []*object.EmeraldValue{core.NewIntegerValue(7)})
		if index >= registerIRDirectNoFrameWarmupCalls && (!executed || result == nil || result.Inspect() != "8") {
			t.Fatalf("block-context typed method did not execute: result=%v executed=%t", result, executed)
		}
	}
	if vm.currentBlock != marker {
		t.Fatal("typed hot method did not restore caller block")
	}
}

func TestCachedBytecodeMethodPreservesCallerBlockSemantics(t *testing.T) {
	result, _ := runRuby(t, `
class CachedBytecodeBlockObservation
  def block_state
    block_given?
  end

  def convert(value)
    begin
      raise "negative" if value < 0
      value.to_s
    rescue
      "fallback"
    end
  end
end
observer = CachedBytecodeBlockObservation.new
values = Array.new(1024, 7)
states = values.map { |item| observer.block_state }
converted = values.map { |item| observer.convert(item) }
[states[0], converted[0]]
`)
	if result == nil || result.Inspect() != `[false, "7"]` {
		t.Fatalf("cached bytecode caller block changed semantics: %v", result)
	}
}

func TestCachedTypedHotTerminalInstanceWriteDeoptsAfterRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class CachedTypedHotMutationRedefinition
  def update(value)
    @value = value > 3 ? value.to_s : ""
  end
end

holder = CachedTypedHotMutationRedefinition.new
values = Array.new(16, 7)
before = values.map { |item| holder.update(item) }

class Integer
  def to_s
    "changed"
  end
end

after = values.map { |item| holder.update(item) }
[before[0], after[0], holder.update(2)]
`)
	if result == nil || result.Inspect() != `["7", "changed", ""]` {
		t.Fatalf("cached typed hot terminal write ignored redefinition: %v", result)
	}
}

func TestTypedHotArrayCallPreservesMutationAndRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedHotArrayMutation
  def update(value)
    @value = value > 3 ? value.to_s : ""
  end
end

holder = TypedHotArrayMutation.new
values = Array.new(1024, 7)
before = values.map { |value| holder.update(value) }

class Integer
  def to_s
    "changed"
  end
end

after = values.map { |value| holder.update(value) }
seen = []
values.each { |value| seen << holder.update(value) }
[before.length, before[0], after[0], seen.length, seen[0], holder.update(2)]
`)
	if result == nil || result.Inspect() != `[1024, "7", "changed", 1024, "changed", ""]` {
		t.Fatalf("typed hot Array call changed mutation or redefinition semantics: %v", result)
	}
}

func TestTypedHotArrayEffectfulStringMapLazyPreservesSnapshotAndIdentity(t *testing.T) {
	result, _ := runRuby(t, `
class TypedHotArrayLazyMutation
  def initialize
    @value = ""
  end

  def update(value)
    @value = value > 3 ? value.to_s : ""
  end

  attr_reader :value
end

holder = TypedHotArrayLazyMutation.new
values = Array.new(1024, 7)
mapped = values.map { |value| holder.update(value) }
values[0] = 2
first = mapped[0]
last = mapped[-1]
same = last.object_id == mapped[-1].object_id
first << "x"
[mapped.length, first, mapped[1], last, holder.value, same, values[0]]`)
	if result == nil || result.Inspect() != `[1024, "7x", "7", "7", "7", true, 2]` {
		t.Fatalf("typed hot effectful lazy map changed snapshot or identity semantics: %v", result)
	}
}

func TestTypedLazyStringMapEachLengthAvoidsMaterializationAndDeopts(t *testing.T) {
	result, _ := runRuby(t, `
class TypedLazyStringMapEachHelper
  def initialize
    @value = ""
  end

  def update(value)
    @value = value > 3 ? value.to_s : ""
  end
end

helper = TypedLazyStringMapEachHelper.new
values = Array.new(1024, 7)
mapped = values.map { |value| helper.update(value) }
sum = 0
mapped.each { |item| sum += item.length }
before = sum

class String
  def length
    9
  end
end

sum = 0
mapped.each { |item| sum += item.length }
[before, sum, mapped[0], mapped[1]]
`)
	if result == nil || result.Inspect() != `[1024, 9216, "7", "7"]` {
		t.Fatalf("lazy String map each length changed materialization or deopt semantics: %v", result)
	}
}

func TestTypedLazyObjectStringMapEachLengthPreservesMutation(t *testing.T) {
	result, _ := runRuby(t, `
class TypedLazyObjectStringEachBox
  def initialize(value)
    @value = value
  end

  def value
    @value
  end
end

values = Array.new(1024) { TypedLazyObjectStringEachBox.new(7) }
mapped = values.map { |item| item.value.to_s }
sum = 0
mapped.each { |item| sum += item.length }
mapped[0] << "x"
[sum, mapped[0], mapped[1], mapped[0].equal?(mapped[1])]
`)
	if result == nil || result.Inspect() != `[1024, "7x", "7", false]` {
		t.Fatalf("lazy object String map each length changed String identity: %v", result)
	}
}

func TestTypedHotArrayCallAllowsOuterTerminalInstanceStore(t *testing.T) {
	result, _ := runRuby(t, `
class TypedHotArrayOuterStoreHelper
  def convert(value)
    value > 3 ? value.to_s : ""
  end
end
class TypedHotArrayOuterStore
  def initialize
    @last = ""
  end

  def values
    helper = TypedHotArrayOuterStoreHelper.new
    Array.new(1024, 7).map { |value| @last = helper.convert(value) }
  end

  def last
    @last
  end
end
holder = TypedHotArrayOuterStore.new
before = holder.values
before.each { |item| item.length }
before[0] << "x"
class Integer
  def to_s
    "changed"
  end
end
after = holder.values
	[before[0], before[1], after[-1], holder.last]`)
	if result == nil || result.Inspect() != `["7x", "7", "changed", "changed"]` {
		t.Fatalf("typed hot Array outer store changed mutation or redefinition semantics: %v", result)
	}
}

func TestTypedHotArrayCallDeoptsOnReceiverGuardMiss(t *testing.T) {
	result, _ := runRuby(t, `
class TypedHotArrayReceiverA
  def update(value)
    @value = "a"
  end
end

class TypedHotArrayReceiverB
  def update(value)
    @value = "b"
  end
end

left = TypedHotArrayReceiverA.new
right = TypedHotArrayReceiverB.new
values = Array.new(1024, left)
values[512] = right
mapped = values.map { |item| item.update(1) }
[mapped.length, mapped[0], mapped[512], mapped[513]]
`)
	if result == nil || result.Inspect() != `[1024, "a", "b", "a"]` {
		t.Fatalf("typed hot Array receiver guard miss changed results: %v", result)
	}
}

func TestTypedHotArrayCallFallsBackForHeterogeneousElements(t *testing.T) {
	result, _ := runRuby(t, `
class TypedHotArrayHeterogeneous
  def update(value)
    @value = value > 3 ? value.to_s : ""
  end

  attr_reader :value
end

holder = TypedHotArrayHeterogeneous.new
values = Array.new(1024, 7)
values[512] = 7.5
mapped = values.map { |value| holder.update(value) }
[mapped.length, mapped[0], mapped[512], mapped[513], holder.value]
`)
	if result == nil || result.Inspect() != `[1024, "7", "7.5", "7", "7"]` {
		t.Fatalf("typed hot Array heterogeneous element changed results: %v", result)
	}
}

func TestTypedHotArrayCallFallsBackForNext(t *testing.T) {
	result, _ := runRuby(t, `
values = Array.new(1024, 1)
mapped = values.map { |value| next "skip" if value == 1; value }
seen = 0
values.each { |value| next if value == 1; seen += 1 }
[mapped.length, mapped[0], seen]
`)
	if result == nil || result.Inspect() != `[1024, "skip", 0]` {
		t.Fatalf("typed hot Array next fallback changed semantics: %v", result)
	}
}

func TestTypedHotArrayCallFallsBackForException(t *testing.T) {
	result, _ := runRuby(t, `
class TypedHotArrayException
  def update(value)
    raise "boom" if value == 2
    @value = value.to_s
  end
end

holder = TypedHotArrayException.new
values = Array.new(1024, 1)
values[512] = 2
message = begin
  values.map { |value| holder.update(value) }
  nil
rescue => error
  error.message
end
message
`)
	if result == nil || result.Inspect() != `"boom"` {
		t.Fatalf("typed hot Array exception fallback changed semantics: %v", result)
	}
}

func TestTypedHotTimesCallPreservesMutationAndRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedHotTimesMutation
  def update(value)
    @value = value > 3 ? value.to_s : ""
  end

  attr_reader :value
end

holder = TypedHotTimesMutation.new
1024.times { |value| holder.update(value) }
before = holder.value
1024.times { |value| holder.update(value) }
repeat = holder.value
before << "!"

class Integer
  def to_s
    "changed"
  end
end

1024.times { |value| holder.update(value) }
after = holder.value
[before, repeat, before.equal?(repeat), after, holder.update(2)]
`)
	if result == nil || result.Inspect() != `["1023!", "1023", false, "changed", ""]` {
		t.Fatalf("typed hot times call changed mutation or redefinition semantics: %v", result)
	}
}

func TestTypedHotTimesCallPreservesFrozenReceiverError(t *testing.T) {
	err := runRubyExpectError(t, `
class TypedHotTimesFrozen
  def update(value)
    @value = value > 3 ? value.to_s : ""
  end
end

holder = TypedHotTimesFrozen.new
holder.freeze
1024.times { |value| holder.update(value) }
`)
	if err == nil || !strings.Contains(err.Error(), "FrozenError") {
		t.Fatalf("typed hot times swallowed frozen receiver error: %v", err)
	}
}

func TestArrayNewConstructorBatchDefaultModePreservesSemantics(t *testing.T) {
	previousAggressive := registerIRAggressiveEnabled
	registerIRAggressiveEnabled = false
	defer func() { registerIRAggressiveEnabled = previousAggressive }()
	result, _ := runRuby(t, `
class DefaultBatchConstructor
  def initialize(value)
    @value = value
  end
  attr_reader :value
end

items = Array.new(32) { |index| DefaultBatchConstructor.new(index) }
[items.length, items[0].value, items[31].value]
`)
	if result == nil || result.Inspect() != "[32, 0, 31]" {
		t.Fatalf("default constructor batch changed semantics: %v", result)
	}
}

func TestTypedSSABatchCallBlockPreservesMapAndEach(t *testing.T) {
	result, _ := runRuby(t, `
def classify(value)
  if value > 1
    value + 10
  else
    value - 10
  end
end

mapped = [1, 2, 3].map { |value| classify(value) }
seen = []
[1, 2, 3].each { |value| seen << classify(value) }
[mapped, seen]
`)
	if result.Inspect() != `[[-9, 12, 13], [-9, 12, 13]]` {
		t.Fatalf("typed batch call changed map/each result: %s", result.Inspect())
	}
}

func TestTypedSSABatchCallLongArrayAndRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
def add_one(value)
  value + 1
end

before = Array.new(1024, 1).map { |value| add_one(value) }
class Integer
  def +(other)
    99
  end
end
after = Array.new(1024, 1).map { |value| add_one(value) }
[before.length, before[0], after.length, after[0]]
`)
	if result.Inspect() != `[1024, 2, 1024, 99]` {
		t.Fatalf("typed batch call ignored method generation: %s", result.Inspect())
	}
}

func TestTypedSSAStringMapLazyResultPreservesSnapshotAndStringIdentity(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAStringMapLazyHelper
  def render(value)
    value.to_s + "!"
  end
end

helper = TypedSSAStringMapLazyHelper.new
values = Array.new(1024, 7)
mapped = values.map { |value| helper.render(value) }
values[0] = 99
first = mapped[0]
same = first.object_id == mapped[0].object_id
first << "x"
class Integer
  def to_s
    "changed"
  end
end
[first, mapped[0], mapped[1], same, mapped.length, mapped.class]
`)
	if result == nil || result.Inspect() != `["7!x", "7!x", "7!", true, 1024, Array]` {
		t.Fatalf("lazy String map result changed snapshot or identity semantics: %v", result)
	}
}

func TestTypedSSAStringMapLazyLengthPreservesSnapshot(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAStringMapLazyLengthHelper
  def render(value)
    value.to_s + "!"
  end
end

helper = TypedSSAStringMapLazyLengthHelper.new
values = Array.new(1024, 7)
mapped = values.map { |value| helper.render(value).length }
values[0] = 99
class Integer
  def to_s
    "changed"
  end
end
[mapped[0], mapped[1], mapped.length, mapped.class]
`)
	if result == nil || result.Inspect() != `[2, 2, 1024, Array]` {
		t.Fatalf("lazy String length map changed snapshot semantics: %v", result)
	}
}

func TestTypedSSAIntegerMapLazyResultPreservesSnapshotAndIntegerSemantics(t *testing.T) {
	result, _ := runRuby(t, `
def typed_ssa_lazy_classify(value)
  if value > 3
    value + 1
  else
    value - 1
  end
end

values = Array.new(1024, 7)
mapped = values.map { |value| typed_ssa_lazy_classify(value) }
values[0] = 99
first = mapped[0]
same = first.object_id == mapped[0].object_id
class Integer
  def +(other)
    99
  end
end
[first, mapped[0], mapped[1], same, mapped.length, mapped.class]
`)
	if result == nil || result.Inspect() != `[8, 8, 8, true, 1024, Array]` {
		t.Fatalf("lazy Integer map result changed snapshot or identity semantics: %v", result)
	}
}

func TestRegisterIRIntegerLinearLazyArrayMapPreservesSnapshot(t *testing.T) {
	result, _ := runRuby(t, `
values = Array.new(1024, 7)
mapped = values.map { |value| value + 1 }
values[0] = 99
first = mapped[0]
same = first.object_id == mapped[0].object_id
class Integer
  def +(other)
    99
  end
end
[first, mapped[0], mapped[1], same, mapped.length, mapped.class]
`)
	if result == nil || result.Inspect() != `[8, 8, 8, true, 1024, Array]` {
		t.Fatalf("lazy Register IR Integer map changed snapshot or identity semantics: %v", result)
	}
}

func TestTypedSSABatchObjectGetterAndRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSABatchObjectGetter
  def initialize(value)
    @value = value
  end

  def value
    @value
  end
end

boxes = Array.new(1024) { TypedSSABatchObjectGetter.new(7) }
before = boxes.map { |box| box.value }
class TypedSSABatchObjectGetter
  def value
    @value + 1
  end
end
after = boxes.map { |box| box.value }
[before.length, before[0], after.length, after[0], after[1023]]
`)
	if result == nil || result.Inspect() != "[1024, 7, 1024, 8, 8]" {
		t.Fatalf("typed object getter batch changed result or redefinition semantics: %v", result)
	}
}

func TestTypedSSABatchObjectGetterSingletonGuard(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSABatchObjectSingletonGuard
  def initialize(value)
    @value = value
  end

  def value
    @value
  end
end

normal = TypedSSABatchObjectSingletonGuard.new(7)
special = TypedSSABatchObjectSingletonGuard.new(9)
special.define_singleton_method(:value) { 42 }
direct = special.value
boxes = Array.new(1024, normal)
boxes[512] = special
mapped = boxes.map { |box| box.value }
[direct, mapped[0], mapped[512], mapped[513]]
`)
	if result == nil || result.Inspect() != "[42, 7, 42, 7]" {
		if result == nil {
			t.Fatalf("typed object getter batch returned nil for singleton receiver")
		}
		t.Fatalf("typed object getter batch ignored singleton receiver: %s", result.Inspect())
	}
}

func TestTypedSSABatchCallPrivateMethodPreservesLargeMap(t *testing.T) {
	result, _ := runRuby(t, `
def classify_private(value)
  if value > 3
    value + 1
  else
    value - 1
  end
end

values = Array.new(2048, 7)
mapped = values.map { |value| classify_private(value) }
[mapped.length, mapped[0], mapped[2047]]
`)
	if result.Inspect() != `[2048, 8, 8]` {
		t.Fatalf("typed batch private method changed large map result: %s", result.Inspect())
	}
}

func TestTypedSSABatchPrimitiveCalleePreservesBranchSemantics(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSABatchPrimitiveHelper
  def classify(value)
    if value > 0
      value + 10
    else
      value - 10
    end
  end
end

helper = TypedSSABatchPrimitiveHelper.new
positive = Array.new(1024, 1).map { |value| helper.classify(value) }
negative = Array.new(1024, -1).map { |value| helper.classify(value) }
class TypedSSABatchPrimitiveHelper
  def classify(value)
    value + 100
  end
end
redefined = Array.new(1024, 1).map { |value| helper.classify(value) }
[positive.length, positive[0], negative.length, negative[0], redefined[0]]
`)
	if result == nil || result.Inspect() != "[1024, 11, 1024, -11, 101]" {
		t.Fatalf("primitive batch callee changed branch result: %v", result)
	}
}

func TestTypedSSAArrayFieldReducePreservesResultAndRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAFieldReduceBox
  def initialize(value)
    @value = value
  end

  def value
    @value
  end
end

boxes = Array.new(1024) { TypedSSAFieldReduceBox.new(7) }
total = 0
boxes.each { |box| total += box.value }
before = total
class TypedSSAFieldReduceBox
  def value
    @value + 1
  end
end
total = 0
boxes.each { |box| total += box.value }
[before, total]
`)
	if result == nil || result.Inspect() != "[7168, 8192]" {
		t.Fatalf("field reduce changed result or redefinition semantics: %v", result)
	}
}

func TestTypedSSABatchCompactGetterKeepsReflectiveValue(t *testing.T) {
	previousCompact := object.CompactObjectLayouts
	object.CompactObjectLayouts = true
	defer func() { object.CompactObjectLayouts = previousCompact }()

	result, _ := runRuby(t, `
class TypedSSACompactGetterBox
  def initialize(value)
    @value = value
  end

  def value
    @value
  end
end

boxes = Array.new(1024) { TypedSSACompactGetterBox.new(7) }
before = boxes.map { |box| box.value }
boxes[0].instance_variable_set(:@value, 11)
boxes[1].instance_variables
boxes[1].instance_variable_set(:@value, 13)
after = boxes.map { |box| box.value }
[before[0], after[0], after[1], boxes[1].instance_variable_get(:@value)]
`)
	if result == nil || result.Inspect() != "[7, 11, 13, 13]" {
		t.Fatalf("compact getter batch changed reflective value: %v", result)
	}
}

func TestTypedSSAArrayFieldReduceReplaysOnIntegerOverflow(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAFieldReduceOverflowBox
	def initialize
		@value = 7
	end

	def value
		@value
	end
end

boxes = Array.new(1024) { TypedSSAFieldReduceOverflowBox.new }
total = 9223372036854768646
boxes.each { |box| total += box.value }
total
`)
	if result == nil || result.Inspect() != "9223372036854775814" {
		t.Fatalf("field reduce did not replay on integer overflow: %v", result)
	}
}

func TestTypedSSABatchNativeCallAndRedefinition(t *testing.T) {
	result, _ := runRuby(t, `values = Array.new(1024, :x)
before = values.map { |value| value.to_s }
class Symbol
  def to_s
    :redefined
  end
end
after = values.map { |value| value.to_s }
[before.length, before[0], after.length, after[0]]`)
	if result == nil || result.Inspect() != `[1024, "x", 1024, :redefined]` {
		t.Fatalf("typed native batch ignored Symbol#to_s redefinition: %s", result.Inspect())
	}
}

func TestTypedSSABatchUnboxedStringHelperPreservesRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAStringBatchHelper
  def render(value)
    value.to_s + "!"
  end
end
helper = TypedSSAStringBatchHelper.new
values = Array.new(1024, 7)
before = values.map { |value| helper.render(value) }
before[0] << "x"
class Integer
  def to_s
    "changed"
  end
end
after_integer = values.map { |value| helper.render(value) }
class String
  def +(other)
    "string-redefined"
  end
end
after_string = values.map { |value| helper.render(value) }
[before.length, before[0], before[1], after_integer[0], after_string[0]]
`)
	if result == nil || result.Inspect() != `[1024, "7!x", "7!", "changed!", "string-redefined"]` {
		t.Fatalf("typed String batch ignored operator redefinition: %v", result)
	}
}

func TestTypedSSABatchRescueIntegerStringPreservesFallbackAndRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSARescueStringHelper
  def render(value)
    begin
      if value > 3
        value.to_s
      else
        ""
      end
    rescue
      "fallback"
    end
  end
end
helper = TypedSSARescueStringHelper.new
values = Array.new(1024, 7)
before = values.map { |value| helper.render(value) }
before[0] << "x"
mixed = [7, "bad", 7].map { |value| helper.render(value) }
class Integer
  def to_s
    "changed"
  end
end
after = values.map { |value| helper.render(value) }
[before.length, before[0], before[1], mixed, after.length, after[0]]
`)
	if result == nil || result.Inspect() != `[1024, "7x", "7", ["7", "fallback", "7"], 1024, "changed"]` {
		t.Fatalf("typed rescue String batch changed fallback or redefinition semantics: %v", result)
	}
}

func TestTypedSSAStringLengthBatchPreservesBuiltinRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAStringLengthHelper
  def render(value)
    value.to_s + "!"
  end
end
helper = TypedSSAStringLengthHelper.new
values = Array.new(1024, 7)
before = values.map { |value| helper.render(value).length }
class Integer
  def to_s
    "changed"
  end
end
after_integer = values.map { |value| helper.render(value).length }
class String
  def length
    99
  end
end
after_string = values.map { |value| helper.render(value).length }
[before[0], after_integer[0], after_string[0]]
`)
	if result == nil || result.Inspect() != `[2, 8, 99]` {
		t.Fatalf("typed String length batch ignored builtin redefinition: %v", result)
	}
}

func TestTypedSSAObjectGetterIntegerToStringBatchPreservesRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAObjectIntegerToStringBox
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(1024) { TypedSSAObjectIntegerToStringBox.new(7) }
before = values.map { |item| item.value.to_s }
class TypedSSAObjectIntegerToStringBox
  def value
    8
  end
end
after_getter = values.map { |item| item.value.to_s }
class Integer
  def to_s
    "changed"
  end
end
after_integer = values.map { |item| item.value.to_s }
[before[0], after_getter[0], after_integer[0]]
`)
	if result == nil || result.Inspect() != `["7", "8", "changed"]` {
		if result == nil {
			t.Fatalf("object getter String batch returned nil")
		}
		t.Fatalf("object getter String batch ignored redefinition: %s", result.Inspect())
	}
}

func TestTypedSSAObjectGetterIntegerToStringBatchPreservesNonLocalReturn(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAObjectIntegerToStringReturnBox
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
def return_from_object_map(values)
  values.map { |item| return item.value.to_s }
  :after
end
values = Array.new(1024) { TypedSSAObjectIntegerToStringReturnBox.new(7) }
return_from_object_map(values)
`)
	if result == nil || result.Inspect() != `"7"` {
		t.Fatalf("object getter String batch bypassed non-local return: %v", result)
	}
}

func TestTypedSSAObjectGetterStringConcatBatchPreservesRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAObjectStringConcatBox
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(1024) { TypedSSAObjectStringConcatBox.new("item") }
before = values.map { |item| item.value + "!" }
before[0] << "x"
class String
  def +(other)
    "changed"
  end
end
after = values.map { |item| item.value + "!" }
[before[0], before[1], after[0]]
`)
	if result == nil || result.Inspect() != `["item!x", "item!", "changed"]` {
		t.Fatalf("object getter String#+ batch ignored redefinition: %v", result)
	}
}

func TestTypedSSAObjectGetterStringLengthBatchPreservesRedefinition(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAObjectStringLengthBox
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
integer_values = Array.new(1024) { TypedSSAObjectStringLengthBox.new(7) }
string_values = Array.new(1024) { TypedSSAObjectStringLengthBox.new("item") }
unicode_values = Array.new(1024) { TypedSSAObjectStringLengthBox.new("hé") }
before = [integer_values.map { |item| item.value.to_s.length }[0], string_values.map { |item| (item.value + "!").length }[0], unicode_values.map { |item| (item.value + "!").length }[0]]
class Integer
  def to_s
    "changed"
  end
end
after_integer = integer_values.map { |item| item.value.to_s.length }[0]
class String
  def +(other)
    "changed"
  end
end
after_plus = string_values.map { |item| (item.value + "!").length }[0]
class String
  def length
    99
  end
end
after_length = [integer_values.map { |item| item.value.to_s.length }[0], string_values.map { |item| (item.value + "!").length }[0]]
[before, after_integer, after_plus, after_length]
`)
	if result == nil || result.Inspect() != `[[1, 5, 3], 7, 7, [99, 99]]` {
		t.Fatalf("object getter String length batch ignored redefinition: %v", result)
	}
}

func TestTypedSSAObjectGetterIntegerToStringBatchPreservesSingleton(t *testing.T) {
	result, _ := runRuby(t, `
class TypedSSAObjectIntegerToStringSingletonBox
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(1024) { TypedSSAObjectIntegerToStringSingletonBox.new(7) }
class << values[500]
  def value
    9
  end
end
mapped = values.map { |item| item.value.to_s }
[mapped[0], mapped[500], mapped[501]]
`)
	if result == nil || result.Inspect() != `["7", "9", "7"]` {
		t.Fatalf("object getter String batch ignored singleton method: %v", result)
	}
}

func TestTypedSSAYieldBlockPreservesNestedYield(t *testing.T) {
	result, _ := runRuby(t, `
class Holder
  def initialize
    @ids = [1, 2, 3]
    @objects = {1 => 10, 2 => 20, 3 => 30}
  end

  def each_value
    @ids.each do |id|
      yield(@objects[id])
    end
  end
end

values = []
Holder.new.each_value { |value| values << value + 1 }
values
`)
	if result.Inspect() != "[11, 21, 31]" {
		t.Fatalf("typed yield block changed nested callback result: %s", result.Inspect())
	}
}
