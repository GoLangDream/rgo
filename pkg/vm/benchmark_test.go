package vm

import (
	"testing"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/parser"
)

func benchmarkRubySource(b *testing.B, source string) {
	core.Init()
	p := parser.New(lexer.New(source))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		b.Fatalf("parse errors: %v", p.Errors())
	}
	c := compiler.New()
	if err := c.Compile(program); err != nil {
		b.Fatalf("compile error: %v", err)
	}
	bytecode := c.Bytecode()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		machine := New(bytecode)
		if err := machine.Run(); err != nil {
			b.Fatalf("runtime error: %v", err)
		}
	}
}

func BenchmarkRubyArithmetic(b *testing.B) {
	benchmarkRubySource(b, `
n = 25000
i = 0
x = 1
while i < n
  x = (x * 1664525 + 1013904223) % 2147483647
  i += 1
end
x
`)
}

func BenchmarkRubyDispatch(b *testing.B) {
	benchmarkRubySource(b, `
def mix_value(x)
  ((x * 33) ^ (x >> 3)) & 2147483647
end
n = 20000
i = 0
sum = 0
while i < n
  sum = (sum + mix_value(i)) & 2147483647
  i += 1
end
sum
`)
}

func BenchmarkRubyBlocks(b *testing.B) {
	benchmarkRubySource(b, `
n = 100000
sum = 0
n.times do |i|
  sum += i & 7
end
sum
`)
}

func BenchmarkRubyIntegerTimesNativeSend(b *testing.B) {
	benchmarkRubySource(b, `
n = 20000
result = nil
n.times { |index| result = index.to_s }
result
`)
}

func BenchmarkRubyIntegerTimesNativeBranch(b *testing.B) {
	benchmarkRubySource(b, `
n = 20000
result = nil
n.times { |index| result = index.is_a?(Integer) ? index.to_s : "" }
result
`)
}

func BenchmarkRubyIntegerTimesDynamicMutation(b *testing.B) {
	benchmarkRubySource(b, `
class DynamicTimesMutationValue
  def update(value)
    @value = value > 3 ? value.to_s : ""
  end
end
holder = DynamicTimesMutationValue.new
20000.times { |index| holder.update(index) }
`)
}

func BenchmarkRubyArrayLiteralIndex(b *testing.B) {
	benchmarkRubySource(b, `
values = Array.new(20000, 1)
mapped = values.map { |x| [x, x + 1][0] }
mapped[0]
`)
}

func BenchmarkRubyArrayIntegerMap(b *testing.B) {
	benchmarkRubySource(b, `
values = Array.new(20000, 7)
mapped = values.map { |x| x + 1 }
mapped[0]
`)
}

func BenchmarkRubyArrayBranchMap(b *testing.B) {
	benchmarkRubySource(b, `
def classify_value(value)
  if value > 3
    value + 1
  else
    value - 1
  end
end
values = Array.new(20000, 7)
mapped = values.map { |item| classify_value(item) }
mapped[0]
`)
}

func BenchmarkRubyArrayBranchObjectMap(b *testing.B) {
	benchmarkRubySource(b, `
class BranchMapValue
  def classify(value)
    if value > 3
      value + 1
    else
      value - 1
    end
  end
end
classifier = BranchMapValue.new
values = Array.new(20000, 7)
mapped = values.map { |item| classifier.classify(item) }
mapped[0]
`)
}

func BenchmarkRubyArrayDynamicMutationMap(b *testing.B) {
	benchmarkRubySource(b, `
class DynamicMutationValue
  def initialize
    @value = ""
  end
  def update(value)
    @value = value > 3 ? value.to_s : ""
  end
end
value = DynamicMutationValue.new
values = Array.new(20000, 7)
values.map { |item| value.update(item) }
`)
}

func BenchmarkRubyArrayRubyStringHelperMap(b *testing.B) {
	benchmarkRubySource(b, `
class StringMapHelper
  def render(value)
    value.to_s + "!"
  end
end
helper = StringMapHelper.new
values = Array.new(20000, 7)
values.map { |value| helper.render(value) }[0]
`)
}

func BenchmarkRubyArrayRubyStringLengthChain(b *testing.B) {
	benchmarkRubySource(b, `
class StringLengthChainHelper
  def render(value)
    value.to_s + "!"
  end
end
helper = StringLengthChainHelper.new
values = Array.new(20000, 7)
mapped = values.map { |value| helper.render(value).length }
mapped[0]
`)
}

func BenchmarkRubyArrayFramedBlockInstanceStore(b *testing.B) {
	benchmarkRubySource(b, `
class FramedBlockInstanceStoreValue
  def initialize
    @last = ""
  end
  def run
    values = Array.new(20000, 7)
    values.map { |value| @last = value > 3 ? value.to_s : "" }
  end
end
FramedBlockInstanceStoreValue.new.run
`)
}

func BenchmarkRubyArrayFramedBlockDynamicStore(b *testing.B) {
	benchmarkRubySource(b, `
class FramedBlockDynamicStoreHelper
  def convert(value)
    value > 3 ? value.to_s : ""
  end
end
class FramedBlockDynamicStoreValue
  def initialize
    @last = ""
  end
  def run
    helper = FramedBlockDynamicStoreHelper.new
    values = Array.new(20000, 7)
    values.map { |value| @last = helper.convert(value) }
  end
end
FramedBlockDynamicStoreValue.new.run
`)
}

func BenchmarkRubyArrayFramedBlockDynamicStoreFullResult(b *testing.B) {
	benchmarkRubySource(b, `
class FramedBlockDynamicStoreFullHelper
  def convert(value)
    value > 3 ? value.to_s : ""
  end
end
class FramedBlockDynamicStoreFullValue
  def initialize
    @last = ""
  end
  def run
    helper = FramedBlockDynamicStoreFullHelper.new
    values = Array.new(20000, 7)
    values.map { |value| @last = helper.convert(value) }
  end
end
result = FramedBlockDynamicStoreFullValue.new.run
sum = 0
result.each { |item| sum += item.length }
sum
`)
}

func BenchmarkRubyArrayCachedBytecodeBlockMethod(b *testing.B) {
	benchmarkRubySource(b, `
class CachedBytecodeBlockMethodValue
  def convert(value)
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
helper = CachedBytecodeBlockMethodValue.new
values = Array.new(20000, 7)
mapped = values.map { |item| helper.convert(item) }
mapped[0]
`)
}

func BenchmarkRubyArrayCachedBytecodeBlockMethodFullResult(b *testing.B) {
	benchmarkRubySource(b, `
class CachedBytecodeBlockMethodFullValue
  def convert(value)
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
helper = CachedBytecodeBlockMethodFullValue.new
values = Array.new(20000, 7)
mapped = values.map { |item| helper.convert(item) }
sum = 0
mapped.each { |item| sum += item.length }
sum
`)
}

func BenchmarkRubyArrayNativeBranchMap(b *testing.B) {
	benchmarkRubySource(b, `
values = Array.new(20000, "abc")
mapped = values.map { |item| item.is_a?(String) ? item.length : 0 }
mapped[0]
`)
}

func BenchmarkRubyArrayConstantMethod(b *testing.B) {
	benchmarkRubySource(b, `
class ConstantMapValue
  def value
    7
  end
end
value = ConstantMapValue.new
values = Array.new(20000, value)
values.map { |item| item.value }
`)
}

func BenchmarkRubyArrayObjectGetter(b *testing.B) {
	benchmarkRubySource(b, `
class ObjectMapValue
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(20000) { ObjectMapValue.new(7) }
values.map { |item| item.value }
`)
}

func BenchmarkRubyArrayObjectGetterHot(b *testing.B) {
	benchmarkRubySource(b, `
class HotObjectMapValue
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(20000) { HotObjectMapValue.new(7) }
round = 0
result = nil
while round < 8
  result = values.map { |item| item.value }
  round += 1
end
result[0]
`)
}

func BenchmarkRubyArrayObjectGetterIntegerToS(b *testing.B) {
	benchmarkRubySource(b, `
class ObjectGetterIntegerToSValue
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(20000) { ObjectGetterIntegerToSValue.new(7) }
mapped = values.map { |item| item.value.to_s }
mapped[0]
`)
}

func BenchmarkRubyArrayObjectGetterIntegerToSLength(b *testing.B) {
	benchmarkRubySource(b, `
class ObjectGetterIntegerToSLengthValue
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(20000) { ObjectGetterIntegerToSLengthValue.new(7) }
mapped = values.map { |item| item.value.to_s.length }
mapped[0]
`)
}

func BenchmarkRubyArrayObjectGetterStringConcat(b *testing.B) {
	benchmarkRubySource(b, `
class ObjectGetterStringConcatValue
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(20000) { ObjectGetterStringConcatValue.new("item") }
mapped = values.map { |item| item.value + "!" }
mapped[0]
`)
}

func BenchmarkRubyArrayObjectGetterStringConcatLength(b *testing.B) {
	benchmarkRubySource(b, `
class ObjectGetterStringConcatLengthValue
  def initialize(value)
    @value = value
  end
  def value
    @value
  end
end
values = Array.new(20000) { ObjectGetterStringConcatLengthValue.new("item") }
mapped = values.map { |item| (item.value + "!").length }
mapped[0]
`)
}

func BenchmarkRubyHashEachTwoArg(b *testing.B) {
	benchmarkRubySource(b, `
values = {}
index = 0
while index < 20000
  values[index] = index + 1
  index += 1
end
sum = 0
values.each { |key, value| sum += key + value }
sum
`)
}

func BenchmarkRubyHashEachTwoArgHot(b *testing.B) {
	benchmarkRubySource(b, `
values = {}
index = 0
while index < 20000
  values[index] = index + 1
  index += 1
end
sum = 0
round = 0
while round < 5
  values.each { |key, value| sum += key + value }
  round += 1
end
sum
`)
}

func BenchmarkRubyHashEachRubyHelper(b *testing.B) {
	benchmarkRubySource(b, `
class HashEachTwoArgHelper
  def render(key, value)
    (key * 3 + value).to_s
  end
end
values = {}
index = 0
while index < 20000
  values[index] = index + 1
  index += 1
end
helper = HashEachTwoArgHelper.new
mapped = []
values.each { |key, value| mapped << helper.render(key, value) }
mapped[0]
`)
}

func BenchmarkRubyHashEachRubyHelperFullResult(b *testing.B) {
	benchmarkRubySource(b, `
class HashEachTwoArgFullHelper
  def render(key, value)
    (key * 3 + value).to_s
  end
end
values = {}
index = 0
while index < 20000
  values[index] = index + 1
  index += 1
end
helper = HashEachTwoArgFullHelper.new
mapped = []
values.each { |key, value| mapped << helper.render(key, value) }
sum = 0
mapped.each { |item| sum += item.length }
sum
`)
}

func BenchmarkRubyHashFramedBlock(b *testing.B) {
	benchmarkRubySource(b, `
class HashFramedBlockBenchmarkHelper
  def initialize
    @prefix = "item:"
  end

  def render(key, value)
    if value > 3
      @prefix + (key * 3 + value).to_s
    else
      "fallback"
    end
  end
end
values = {}
index = 0
while index < 20000
  values[index] = index + 1
  index += 1
end
helper = HashFramedBlockBenchmarkHelper.new
mapped = []
values.each { |key, value| mapped << helper.render(key, value) }
mapped[0]
`)
}

func BenchmarkRubyHashFramedBlockFullResult(b *testing.B) {
	benchmarkRubySource(b, `
class HashFramedBlockFullHelper
  def initialize
    @prefix = "item:"
  end

  def render(key, value)
    if value > 3
      @prefix + (key * 3 + value).to_s
    else
      "fallback"
    end
  end
end
values = {}
index = 0
while index < 20000
  values[index] = index + 1
  index += 1
end
helper = HashFramedBlockFullHelper.new
mapped = []
values.each { |key, value| mapped << helper.render(key, value) }
sum = 0
mapped.each { |item| sum += item.length }
sum
`)
}

func BenchmarkRubyHashMapTwoArg(b *testing.B) {
	benchmarkRubySource(b, `
values = {}
index = 0
while index < 20000
  values[index] = index + 1
  index += 1
end
mapped = values.map { |key, value| key + value }
mapped[0]
`)
}

func BenchmarkRubyHashMapTwoArgHot(b *testing.B) {
	benchmarkRubySource(b, `
values = {}
index = 0
while index < 20000
  values[index] = index + 1
  index += 1
end
mapped = nil
round = 0
while round < 5
  mapped = values.map { |key, value| key + value }
  round += 1
end
mapped[0]
`)
}

func BenchmarkRubyIntegerArraySum(b *testing.B) {
	benchmarkRubySource(b, `
values = Array.new(20000, 7)
values.sum
`)
}

func BenchmarkRubyCollections(b *testing.B) {
	benchmarkRubySource(b, `
n = 10000
array = []
hash = {}
i = 0
while i < n
  value = (i * 17) % 1009
  array << value
  hash[i % 997] = value
  i += 1
end
i = 0
sum = 0
while i < array.length
  sum += array[i]
  i += 1
end
sum
`)
}

func BenchmarkRubyStrings(b *testing.B) {
	benchmarkRubySource(b, `
n = 12000
i = 0
text = +""
while i < n
  text << (97 + (i % 26)).chr
  i += 1
end
text.bytesize
`)
}
