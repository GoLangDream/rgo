//go:build linux && cgo

package core

import "testing"

func TestOnigRegexpSearchReusesCompiledPattern(t *testing.T) {
	const pattern = `(?<word>a+)`
	first, _, errText := compileOnigRegexp(pattern, "")
	if first == nil || errText != "" {
		t.Fatalf("failed to compile first regexp: %q", errText)
	}
	second, _, errText := compileOnigRegexp(pattern, "")
	if second == nil || errText != "" {
		t.Fatalf("failed to compile second regexp: %q", errText)
	}
	if first != second {
		t.Fatal("expected identical regexp pattern to reuse compiled entry")
	}

	indices, handled, errText := onigRegexpSearch(pattern, "aaa", "")
	if !handled || errText != "" {
		t.Fatalf("cached regexp search failed: handled=%v error=%q", handled, errText)
	}
	if len(indices) != 4 || indices[0] != 0 || indices[1] != 3 || indices[2] != 0 || indices[3] != 3 {
		t.Fatalf("unexpected cached regexp indices: %v", indices)
	}
}
