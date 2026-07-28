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
