package vm

import (
	"testing"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

func TestClosureRefinementCheckInvalidatesAtMethodGeneration(t *testing.T) {
	core.Init()
	class := object.NewClass("RefinementCacheProbe")
	scope := &object.EmeraldValue{Type: object.ValueClass, Data: class}
	closure := &object.Closure{ClassStack: []*object.EmeraldValue{scope}}
	if closureUsesRefinements(closure) {
		t.Fatal("fresh class scope unexpectedly reports refinements")
	}
	class.UsedRefinements = []*object.EmeraldValue{core.R.NilVal}
	object.BumpMethodGeneration()
	if !closureUsesRefinements(closure) {
		t.Fatal("method-generation change did not invalidate refinement cache")
	}
}
