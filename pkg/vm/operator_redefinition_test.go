package vm

import "testing"

func TestRubyStringPlusRedefinitionAffectsOperatorBytecode(t *testing.T) {
	result, _ := runRuby(t, `
class String
  def +(other)
    "override"
  end
end
"a" + "b"
`)
	if result == nil || result.TypeName() != "String" || result.Data.(string) != "override" {
		t.Fatalf("String#+ redefinition was bypassed: %v", result)
	}
}

func TestRubyFloatPlusRedefinitionAffectsOperatorBytecode(t *testing.T) {
	result, _ := runRuby(t, `
class Float
  def +(other)
    99.0
  end
end
1.0 + 1.0
`)
	if result == nil || result.TypeName() != "Float" || result.Data.(float64) != 99.0 {
		t.Fatalf("Float#+ redefinition was bypassed: %v", result)
	}
}

func TestRubyIntegerBitwiseRedefinitionAffectsOperatorBytecode(t *testing.T) {
	result, _ := runRuby(t, `
class Integer
  def &(other)
    101
  end
  def |(other)
    102
  end
  def ^(other)
    103
  end
end
[5 & 3, 5 | 3, 5 ^ 3]
`)
	if result == nil || result.Inspect() != "[101, 102, 103]" {
		t.Fatalf("Integer bitwise redefinition was bypassed: %v", result)
	}
}
