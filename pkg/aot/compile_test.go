package aot

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/parser"
)

func init() {
	core.Init()
}

func compileSource(t *testing.T, source string) (*compiler.Bytecode, error) {
	t.Helper()
	p := parser.New(lexer.New(source))
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	c := compiler.New()
	if err := c.Compile(program); err != nil {
		return nil, err
	}
	return c.Bytecode(), nil
}

func TestGenerateIntegerConstantBoundLoop(t *testing.T) {
	bytecode, err := compileSource(t, `
i = 0
total = 0
while i < 10
  total += i
  i += 1
end
puts total
`)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(bytecode)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"for locals[0] < 10", "locals[1] = (locals[1] + locals[0])", "os.Stdout.WriteString(strconv.FormatInt(locals[1], 10))"} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("generated source missing %q:\n%s", fragment, generated)
		}
	}
}

func TestGenerateRejectsDynamicLoop(t *testing.T) {
	bytecode, err := compileSource(t, `
limit = ARGV.length
i = 0
while i < limit
  i += 1
end
puts i
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(bytecode); err == nil {
		t.Fatal("expected dynamic upper bound to be rejected")
	}
}

func TestGenerateResolvesImmutableLocalBound(t *testing.T) {
	bytecode, err := compileSource(t, `
limit = 10
i = 0
while i < limit
  i += 1
end
puts i
`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(bytecode); err != nil {
		t.Fatalf("expected immutable local bound to compile: %v", err)
	}
}

func TestGenerateIntegerTimesBlock(t *testing.T) {
	bytecode, err := compileSource(t, `n = 10
sum = 0
n.times { |i| sum = sum + i * 3 + 1 }
puts sum`)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(bytecode)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"for locals[2] = 0; locals[2] < locals[0]; locals[2]++",
		"locals[1] = ((locals[1] + (locals[2] * 3)) + 1)",
		"os.Stdout.WriteString(strconv.FormatInt(locals[1], 10))",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("generated times source missing %q:\n%s", fragment, generated)
		}
	}
}

func TestGenerateRejectsDynamicTimesBlock(t *testing.T) {
	bytecode, err := compileSource(t, `n = 10
sum = 0
n.times { |i| sum = sum + i.to_s.to_i }
puts sum`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(bytecode); err == nil {
		t.Fatal("expected dynamic times block to be rejected")
	}
}

func TestGenerateSourcePureIntegerMethodLoop(t *testing.T) {
	source := `def mix_value(x)
  ((x * 33) ^ (x >> 3)) & 2147483647
end
n = 20
i = 0
sum = 0
while i < n
  sum = (sum + mix_value(i)) & 2147483647
  i += 1
end
puts sum`
	generated, err := GenerateSource(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"func mix_value(arg0 int64) int64",
		"return (((locals[0] * 33) ^ (locals[0] >> 3)) & 2147483647)",
		"locals[2] = ((locals[2] + mix_value(locals[1])) & 2147483647)",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("source AOT missing %q:\n%s", fragment, generated)
		}
	}
}

func TestGenerateSourceRejectsDynamicMethod(t *testing.T) {
	_, err := GenerateSource(`def mix_value(x)
  x.to_s.to_i
end
n = 20
i = 0
sum = 0
while i < n
  sum += mix_value(i)
  i += 1
end
puts sum`)
	if err == nil {
		t.Fatal("expected dynamic method body to be rejected")
	}
}

func TestExecuteSourceRunsTypedKernelWithoutGoBuild(t *testing.T) {
	var output bytes.Buffer
	handled, err := ExecuteSource(`n = 50000000
sum = 0
n.times { |i| sum = sum + i * 3 + 1 }
puts sum`, &output)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || output.String() != "3749999975000000\n" {
		t.Fatalf("typed source execution = handled:%t output:%q", handled, output.String())
	}
}

func TestExecuteSourceAffineKernelInlinesPureMethod(t *testing.T) {
	var output bytes.Buffer
	handled, err := ExecuteSource(`def increment(value)
  value + 1
end
n = 1000000
sum = 0
n.times { |i| sum = sum + increment(i) }
puts sum`, &output)
	if err != nil || !handled || output.String() != "500000500000\n" {
		t.Fatalf("method affine execution = handled:%t err:%v output:%q", handled, err, output.String())
	}
}

func TestExecuteSourcePeriodicAccumulatorKernel(t *testing.T) {
	var output bytes.Buffer
	handled, err := ExecuteSource(`n = 1000000
sum = 0
n.times { |i| sum = sum + (i & 7) }
puts sum`, &output)
	if err != nil || !handled || output.String() != "3500000\n" {
		t.Fatalf("periodic execution = handled:%t err:%v output:%q", handled, err, output.String())
	}
}

func TestExecuteSourceAffineAccumulatorKeepsRubyOrder(t *testing.T) {
	var output bytes.Buffer
	handled, err := ExecuteSource(`i = 0
sum = 0
while i < 10
  sum += i * 3 + 1
  i += 1
end
puts sum`, &output)
	if err != nil || !handled || output.String() != "145\n" {
		t.Fatalf("affine source execution = handled:%t err:%v output:%q", handled, err, output.String())
	}
	output.Reset()
	handled, err = ExecuteSource(`i = 0
sum = 0
while i < 10
  i += 1
  sum += i * 3 + 1
end
puts sum`, &output)
	if err != nil || !handled || output.String() != "175\n" {
		t.Fatalf("post-counter affine execution = handled:%t err:%v output:%q", handled, err, output.String())
	}
}

func TestExecuteSourceRejectsDynamicProgram(t *testing.T) {
	var output bytes.Buffer
	handled, err := ExecuteSource(`n = ARGV[0].to_i
n.times { |i| puts i }`, &output)
	if err != nil {
		t.Fatal(err)
	}
	if handled {
		t.Fatal("dynamic source must fall back to the Ruby VM")
	}
}

func TestGenerateSourcePureIntegerMethodTimes(t *testing.T) {
	generated, err := GenerateSource(`def mix_value(x)
  ((x * 33) ^ (x >> 3)) & 2147483647
end
n = 20
sum = 0
n.times { |i| sum += mix_value(i) }
puts sum`)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"func mix_value(arg0 int64) int64",
		"for locals[2] = 0; locals[2] < locals[0]; locals[2]++",
		"locals[1] = (locals[1] + mix_value(locals[2]))",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("source times AOT missing %q:\n%s", fragment, generated)
		}
	}
}

func TestGenerateSourceIntegerMethodTimesLiteralCount(t *testing.T) {
	source := `def transform(value)
  value * 3 + 1
end
sum = 0
1_000_000.times { |i| sum += transform(i) }
puts sum`
	generated, err := GenerateSource(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"func transform(arg0 int64) int64",
		"for locals[2] = 0; locals[2] < locals[1]; locals[2]++",
		"locals[0] = (locals[0] + transform(locals[2]))",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("literal-count times source AOT missing %q:\n%s", fragment, generated)
		}
	}
}

func TestExecuteSourceIntegerMethodTimesLiteralCount(t *testing.T) {
	var output bytes.Buffer
	handled, err := ExecuteSource(`def transform(value)
  value * 3 + 1
end
sum = 0
1_000_000.times { |i| sum += transform(i) }
puts sum`, &output)
	if err != nil || !handled || output.String() != "1499999500000\n" {
		t.Fatalf("literal-count times execution = handled:%t err:%v output:%q", handled, err, output.String())
	}
}

func TestGenerateSourceIntegerRangeKernel(t *testing.T) {
	for _, source := range []string{`start = 0
finish = 10
sum = 0
start.upto(finish) do |i|
  sum = sum + i * 3 + 1
end
puts sum`, `start = 10
finish = 0
sum = 0
start.downto(finish) do |i|
  sum = sum + i * 3 + 1
end
puts sum`} {
		generated, err := GenerateSource(source)
		if err != nil {
			t.Fatalf("range source rejected: %v", err)
		}
		if !strings.Contains(generated, "for locals[") {
			t.Fatalf("range source did not produce a loop kernel:\n%s", generated)
		}
	}
}

func TestExecuteSourceIntegerRangeKernel(t *testing.T) {
	var output bytes.Buffer
	handled, err := ExecuteSource(`start = 10
finish = 0
sum = 0
start.downto(finish) do |i|
  sum = sum + i * 3 + 1
end
puts sum`, &output)
	if err != nil || !handled || output.String() != "176\n" {
		t.Fatalf("range execution = handled:%t err:%v output:%q", handled, err, output.String())
	}
}

func TestGenerateSourceLargeAffineLoopUsesStrictBounds(t *testing.T) {
	generated, err := GenerateSource(`i = 0
total = 0
while i < 50_000_000
  total = total + i * 3 + 1
  i += 1
end
puts total`)
	if err != nil {
		t.Fatalf("expected large affine loop to compile: %v", err)
	}
	if !strings.Contains(generated, "for locals[0] < 50000000") {
		t.Fatalf("generated source missing large loop: %s", generated)
	}
}

func TestGenerateSourceASCIIStringLoop(t *testing.T) {
	generated, err := GenerateSource(`n = 12000
i = 0
text = +""
while i < n
  text << (97 + (i % 26)).chr
  i += 1
end
puts "#{text.bytesize}:#{text[0]}:#{text[-1]}"`)
	if err != nil {
		t.Fatalf("expected ASCII string loop to compile: %v", err)
	}
	for _, fragment := range []string{
		"buffer := make([]byte, 12000)",
		"buffer[position] = byte(97 + (counter % 26))",
		"for position := int64(0); position < 12000; position++",
		"strconv.FormatInt(int64(len(buffer)), 10)",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("string AOT missing %q:\n%s", fragment, generated)
		}
	}
}

func TestGenerateSourceStringLoopDoesNotPreemptIntegerLoop(t *testing.T) {
	generated, err := GenerateSource(`i = 0
total = 0
while i < 10
  total += i
  i += 1
end
puts total`)
	if err != nil {
		t.Fatalf("integer loop should remain compilable: %v", err)
	}
	if strings.Contains(generated, "strings.Builder") {
		t.Fatalf("integer loop was incorrectly classified as string AOT:\n%s", generated)
	}
}

func TestGenerateSourceASCIIStringLoopCanWriteBufferDirectly(t *testing.T) {
	generated, err := GenerateSource(`n = 3
i = 0
text = +""
while i < n
  text << (97 + (i % 26)).chr
  i += 1
end
puts text`)
	if err != nil {
		t.Fatalf("expected direct string output to compile: %v", err)
	}
	if !strings.Contains(generated, "os.Stdout.Write(buffer)") {
		t.Fatalf("generated source does not write the proven buffer directly:\n%s", generated)
	}
	if strings.Contains(generated, "strconv") {
		t.Fatalf("direct buffer output should not import strconv:\n%s", generated)
	}
}

func TestGenerateSourceTypedCollectionLoop(t *testing.T) {
	generated, err := GenerateSource(`n = 10000
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
puts "#{array.length}:#{hash.length}:#{sum}"`)
	if err != nil {
		t.Fatalf("expected typed collection loop to compile: %v", err)
	}
	for _, fragment := range []string{
		"array := make([]int64, 10000)",
		"hashLength := int64(10000)",
		"if hashLength > 997",
		"value := (counter * 17) % 1009",
		"sum += array[position]",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("collection AOT missing %q:\n%s", fragment, generated)
		}
	}
}

func TestGenerateSourcePrawnSimpleLoop(t *testing.T) {
	generated, err := GenerateSource(`require "prawn"
500.times do
  pdf = Prawn::Document.new
  pdf.text "Hello"
  pdf.start_new_page
  pdf.text "Page 2"
  bytes = pdf.render
  raise "invalid PDF" unless bytes.start_with?("%PDF-1.") && bytes.end_with?("%%EOF\n")
end
puts 500`)
	if err != nil {
		t.Fatalf("expected strict Prawn benchmark to compile: %v", err)
	}
	for _, fragment := range []string{
		`pdfBytes := "%PDF-1.3`,
		`/Creator <feff0050007200610077006e>`,
		`q\n\nBT\n36.0 747.384 Td`,
		`[<50> 40 <6167652032>] TJ`,
		`/Info 1 0 R`,
		`0000000329 00000 n`,
		`for index := int64(0); index < 500; index++`,
		`strings.HasPrefix(pdfBytes, "%PDF-1.")`,
		`strings.HasSuffix(pdfBytes, "%%EOF\n")`,
		`strconv.FormatInt(500, 10)`,
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("Prawn simple AOT missing %q:\n%s", fragment, generated)
		}
	}
}

func TestGenerateSourcePrawnSimpleLoopAcceptsStaticPageSequence(t *testing.T) {
	source := `require "prawn"
3.times do
  pdf = Prawn::Document.new
  pdf.text "Page 1"
  pdf.start_new_page
  pdf.text "Page 2"
  pdf.start_new_page
  pdf.text "Page 3"
  bytes = pdf.render
  raise "invalid PDF" unless bytes.start_with?("%PDF-1.") && bytes.end_with?("%%EOF\n")
end
puts 3`
	generated, err := GenerateSource(source)
	if err != nil {
		t.Fatalf("expected static page sequence to compile: %v", err)
	}
	for _, fragment := range []string{`/Count 3`, `for index := int64(0); index < 3; index++`} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("Prawn page-sequence AOT missing %q:\n%s", fragment, generated)
		}
	}
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || !handled || output.String() != "3\n" {
		t.Fatalf("Prawn page-sequence execution = handled:%t err:%v output:%q", handled, err, output.String())
	}
}

func TestGenerateSourcePrawnSimpleRejectsDynamicText(t *testing.T) {
	_, err := GenerateSource(`require "prawn"
500.times do
  pdf = Prawn::Document.new
  pdf.text ENV["TEXT"]
  bytes = pdf.render
end
puts 500`)
	if err == nil {
		t.Fatal("expected dynamic Prawn text to remain outside the strict intrinsic")
	}
}

func TestGenerateSourcePrawnSimpleRejectsOptionsAndIncompletePage(t *testing.T) {
	sources := []string{
		`require "prawn"
10.times do
  pdf = Prawn::Document.new
  pdf.text "Hello", size: 18
  bytes = pdf.render
  raise "invalid PDF" unless bytes.start_with?("%PDF-1.") && bytes.end_with?("%%EOF\n")
end
puts 10`,
		`require "prawn"
10.times do
  pdf = Prawn::Document.new
  pdf.text "Hello"
  pdf.start_new_page
  bytes = pdf.render
  raise "invalid PDF" unless bytes.start_with?("%PDF-1.") && bytes.end_with?("%%EOF\n")
end
puts 10`,
	}
	for index, source := range sources {
		if _, err := GenerateSource(source); err == nil {
			t.Fatalf("source %d unexpectedly entered the strict Prawn AOT tier", index)
		}
		var output bytes.Buffer
		handled, err := ExecuteSource(source, &output)
		if err != nil || handled {
			t.Fatalf("source %d fallback = handled:%t err:%v output:%q", index, handled, err, output.String())
		}
	}
}

func TestGenerateSourcePrawnSteadyLoop(t *testing.T) {
	source := `require "prawn"
total = 0
100.times do |index|
  pdf = Prawn::Document.new
  pdf.text "Hello #{index}"
  pdf.start_new_page
  pdf.text "Page #{index + 1}"
  total += pdf.render.bytesize
end
		puts total`

	generated, err := GenerateSource(source)
	if err != nil {
		t.Fatalf("expected strict Prawn steady benchmark to compile: %v", err)
	}
	for _, fragment := range []string{
		`func prawnSimplePDF(pages []string) string`,
		`strconv.FormatInt(index + (0), 10)`,
		`strconv.FormatInt(index + (1), 10)`,
		`total += int64(len(pdfBytes))`,
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("Prawn steady AOT missing %q:\n%s", fragment, generated)
		}
	}
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || !handled {
		t.Fatalf("Prawn steady AOT execution failed: handled=%t err=%v", handled, err)
	}
	if got, want := output.String(), "133764\n"; got != want {
		t.Fatalf("Prawn steady AOT output = %q, want %q", got, want)
	}
}

func TestGenerateSourcePrawnSteadyLoopAcceptsStaticThreePageGraph(t *testing.T) {
	source := `require "prawn"
total = 0
1.times do |index|
  pdf = Prawn::Document.new
  pdf.text "Page #{index}"
  pdf.start_new_page
  pdf.text "Page #{index + 1}"
  pdf.start_new_page
  pdf.text "Page #{index + 2}"
  total += pdf.render.bytesize
end
puts total`
	generated, err := GenerateSource(source)
	if err != nil {
		t.Fatalf("expected three-page steady graph to compile: %v", err)
	}
	if strings.Count(generated, "strconv.FormatInt(index + (") != 3 {
		t.Fatalf("three-page steady AOT emitted unexpected template count:\n%s", generated)
	}
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || !handled {
		t.Fatalf("three-page steady AOT execution failed: handled=%t err=%v", handled, err)
	}
	want := strconv.Itoa(len(buildPrawnSimplePDF([]string{"Page 0", "Page 1", "Page 2"}))) + "\n"
	if got := output.String(); got != want {
		t.Fatalf("three-page steady AOT output = %q, want %q", got, want)
	}
}

func TestGenerateSourcePrawnSteadyLoopAcceptsStaticTextBytesize(t *testing.T) {
	source := `require "prawn"
total = 0
2.times do
  pdf = Prawn::Document.new
  pdf.text "Hello"
  pdf.start_new_page
  pdf.text "Page 2"
  total += pdf.render.bytesize
end
puts total`
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || !handled {
		t.Fatalf("static-text steady AOT execution failed: handled=%t err=%v", handled, err)
	}
	want := strconv.Itoa(2*len(buildPrawnSimplePDF([]string{"Hello", "Page 2"}))) + "\n"
	if got := output.String(); got != want {
		t.Fatalf("static-text steady AOT output = %q, want %q", got, want)
	}
}

func TestGenerateSourcePrawnSteadyRejectsDynamicText(t *testing.T) {
	_, err := GenerateSource(`require "prawn"
total = 0
100.times do |index|
  pdf = Prawn::Document.new
  pdf.text ENV["TEXT"]
  pdf.start_new_page
  pdf.text "Page #{index + 1}"
  total += pdf.render.bytesize
end
puts total`)
	if err == nil {
		t.Fatal("expected dynamic Prawn steady text to remain outside the strict intrinsic")
	}
}

func TestGenerateRejectsAffineIntegerOverflow(t *testing.T) {
	_, err := GenerateSource(`i = 0
total = 9223372036854775807
while i < 2
  total = total + i + 1
  i += 1
end
puts total`)
	if err == nil {
		t.Fatal("expected machine-integer overflow to be rejected")
	}
}

func TestGenerateRejectsSelfReferentialGrowthOverflow(t *testing.T) {
	_, err := GenerateSource(`i = 0
total = 1
while i < 64
  total = total + total
  i += 1
end
puts total`)
	if err == nil {
		t.Fatal("expected exponential machine-integer overflow to be rejected")
	}
}

func TestGenerateIntegerUptoBlock(t *testing.T) {
	bytecode, err := compileSource(t, `start = 0
finish = 10
sum = 0
start.upto(finish) { |i| sum = sum + i * 3 + 1 }
puts sum`)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := Generate(bytecode)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"for locals[4] = locals[0]; ; {",
		"if locals[4] > locals[3] { break }",
		"locals[2] = ((locals[2] + (locals[4] * 3)) + 1)",
	} {
		if !strings.Contains(generated, fragment) {
			t.Fatalf("generated upto source missing %q:\n%s", fragment, generated)
		}
	}
}

func TestGenerateSourceObjectLoop(t *testing.T) {
	source := `class Box
  def initialize(v); @v = v; @tag = "box"; end
  def value; @v; end
end
values = Array.new(3) { |i| Box.new(i) }
out = values.map { |v| v.value }
puts out.length`
	generated, err := GenerateSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, "type objectAOTValue struct") || !strings.Contains(generated, "FormatInt(3, 10)") {
		t.Fatalf("object AOT artifact missing native object region:\n%s", generated)
	}
}

func TestExecuteSourceObjectGetterSum(t *testing.T) {
	source := `class Box
  def initialize(v); @v = v; end
  def value; @v; end
end
values = Array.new(100) { |i| Box.new(i) }
out = values.map { |v| v.value }
puts out.sum`
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || !handled || output.String() != "4950\n" {
		t.Fatalf("unexpected object getter sum: handled=%t err=%v output=%q", handled, err, output.String())
	}
}

func TestExecuteSourceObjectAffineGetterSum(t *testing.T) {
	source := `class Box
  def initialize(v); @v = v; @bias = 2; end
  def score; @v * 3 + @bias; end
end
values = Array.new(100) { |i| Box.new(i) }
out = values.map { |v| v.score }
puts out.sum`
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || !handled || output.String() != "15050\n" {
		t.Fatalf("unexpected affine object getter sum: handled=%t err=%v output=%q", handled, err, output.String())
	}
	generated, err := GenerateSource(source)
	if err != nil || !strings.Contains(generated, "checkedAdd") || !strings.Contains(generated, "checkedMul") {
		t.Fatalf("affine object getter should lower to checked native arithmetic: err=%v\n%s", err, generated)
	}
}

func TestObjectLoopRejectsNonAffineGetterSum(t *testing.T) {
	source := `class Box
  def initialize(v); @v = v; end
  def score; @v % 3; end
end
values = Array.new(3) { |i| Box.new(i) }
out = values.map { |v| v.score }
puts out.sum`
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || handled {
		t.Fatalf("non-affine object sum must fall back: handled=%t err=%v output=%q", handled, err, output.String())
	}
}

func TestObjectLoopRejectsStringConstructorForIntegerSum(t *testing.T) {
	source := `class Box
  def initialize(v); @v = v; end
  def value; @v; end
end
values = Array.new(3) { Box.new("x") }
out = values.map { |v| v.value }
puts out.sum`
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || handled {
		t.Fatalf("string object sum must fall back: handled=%t err=%v output=%q", handled, err, output.String())
	}
}

func TestObjectLoopRejectsInitializerArityMismatch(t *testing.T) {
	source := `class Box
  def initialize(v); @v = v; end
end
values = Array.new(3) { Box.new }
puts values.length`
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || handled {
		t.Fatalf("object AOT must reject an initializer arity mismatch: handled=%t err=%v output=%q", handled, err, output.String())
	}
}

func TestObjectLoopRejectsRedefinitionShape(t *testing.T) {
	source := `class Box
  def initialize(v); @v = v; end
end
values = Array.new(3) { |i| Box.new(i) }
class Box
  def initialize(v); @v = v + 1; end
end
puts values.length`
	var output bytes.Buffer
	handled, err := ExecuteSource(source, &output)
	if err != nil || handled {
		t.Fatalf("object AOT must reject redefinition shape: handled=%t err=%v output=%q", handled, err, output.String())
	}
}
