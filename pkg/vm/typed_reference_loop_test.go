package vm

import "testing"

func TestTypedSSAReferenceGetterLoopPreservesResult(t *testing.T) {
	_, output := runRuby(t, `
class TypedReferenceLoopBox
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
box = TypedReferenceLoopBox.new
i = 0
sum = 0
while i < 1000
  sum += box.outer
  i += 1
end
puts sum
`)
	if output != "7000\n" {
		t.Fatalf("typed reference getter loop output=%q", output)
	}
}

func TestTypedSSAReferenceGetterLoopFallsBackForPrivateNestedCall(t *testing.T) {
	_, output := runRuby(t, `
class TypedPrivateReferenceLoopBox
  def initialize
    @value = 7
  end
  def inner
    @value
  end
  private :inner
  def outer
    inner
  end
end
box = TypedPrivateReferenceLoopBox.new
i = 0
sum = 0
while i < 100
  sum += box.outer
  i += 1
end
puts sum
`)
	if output != "700\n" {
		t.Fatalf("private nested call fallback output=%q", output)
	}
}
