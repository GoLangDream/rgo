package vm

import (
	"testing"

	"github.com/GoLangDream/rgo/pkg/object"
)

func TestTypedStringCallLoopPreservesCounterResult(t *testing.T) {
	result, _ := runRuby(t, `
def step(value)
  value + "x"
end
i = 0
while i < 1000
  step("a")
  i += 1
end
i
`)
	if result == nil || result.Inspect() != "1000" {
		t.Fatalf("typed String call loop result=%v", result)
	}
}

func TestTypedStringCallLoopPreservesAssignedString(t *testing.T) {
	result, _ := runRuby(t, `
def step(value)
  value + "x"
end
i = 0
text = ""
while i < 1000
  text = step("a")
  i += 1
end
text
`)
	if result == nil || result.Type != object.ValueString || result.Data.(string) != "ax" {
		t.Fatalf("typed String assignment loop result=%v", result)
	}
}

func TestTypedStringPlanRejectsReferenceGraphForDeadResult(t *testing.T) {
	result, _ := runRuby(t, `
$suffix = "x"
def step(value)
  value + $suffix
end
i = 0
while i < 3
  step("a")
  i += 1
end
i
`)
	if result == nil || result.Inspect() != "3" {
		t.Fatalf("reference String call loop result=%v", result)
	}
}

func TestTypedPrimitiveCallLoopAcceptsFloatCallee(t *testing.T) {
	result, _ := runRuby(t, `
def step(value)
  value * 1.5 + 0.25
end
i = 0
while i < 1000
  step(1.0)
  i += 1
end
i
`)
	if result == nil || result.Inspect() != "1000" {
		t.Fatalf("typed Float call loop result=%v", result)
	}
}
