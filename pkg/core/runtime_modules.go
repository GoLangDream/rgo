package core

import "github.com/GoLangDream/rgo/pkg/object"

type runtimeModuleMethod struct {
	name  string
	fn    interface{}
	arity int
}

func defineRuntimeModule(name string, methods []runtimeModuleMethod) *object.EmeraldValue {
	module := object.NewModule(name)
	for _, method := range methods {
		module.DefineMethod(method.name, &object.Method{
			Name:  method.name,
			Fn:    method.fn,
			Arity: method.arity,
		})
	}
	return &object.EmeraldValue{
		Type:  object.ValueModule,
		Data:  module,
		Class: R.Classes["Module"],
	}
}

func (rt *Runtime) initializeRuntimeModules() {
	rt.Enumerable = defineRuntimeModule("Enumerable", []runtimeModuleMethod{
		{"select", EnumerableSelect, 0},
		{"find_all", EnumerableSelect, 0},
		{"filter", EnumerableSelect, 0},
		{"map", EnumerableMap, -1},
		{"collect", EnumerableMap, -1},
		{"to_a", EnumerableToA, -1},
		{"entries", EnumerableToA, -1},
		{"first", EnumerableFirst, -1},
		{"take", EnumerableTake, -1},
		{"drop", EnumerableDrop, -1},
		{"each_entry", EnumerableEachEntry, -1},
		{"each_cons", EnumerableEachCons, -1},
		{"each_slice", EnumerableEachSlice, -1},
		{"cycle", EnumerableCycle, -1},
		{"all?", EnumerableAll, -1},
		{"any?", EnumerableAny, -1},
		{"none?", EnumerableNone, -1},
		{"one?", EnumerableOne, -1},
		{"min_by", EnumerableMinBy, -1},
		{"max_by", EnumerableMaxBy, -1},
		{"min", EnumerableMin, -1},
		{"max", EnumerableMax, -1},
		{"minmax", EnumerableMinmax, -1},
		{"sort", EnumerableSort, -1},
		{"inject", EnumerableInject, -1},
		{"reduce", EnumerableInject, -1},
		{"grep", EnumerableGrep, -1},
		{"grep_v", EnumerableGrepV, -1},
		{"zip", EnumerableZip, -1},
		{"tally", EnumerableTally, -1},
		{"to_h", EnumerableToH, -1},
		{"chunk_while", EnumerableChunkWhile, -1},
		{"slice_when", EnumerableSliceWhen, -1},
		{"slice_before", EnumerableSliceBefore, -1},
		{"slice_after", EnumerableSliceAfter, -1},
		{"chunk", EnumerableChunk, -1},
		{"count", EnumerableCount, -1},
		{"find", EnumerableFind, -1},
		{"detect", EnumerableFind, -1},
		{"find_index", EnumerableFindIndex, -1},
		{"include?", EnumerableInclude, 1},
		{"member?", EnumerableInclude, 1},
		{"group_by", EnumerableGroupBy, 0},
		{"partition", EnumerablePartition, 0},
		{"reject", EnumerableReject, 0},
		{"each_with_index", EnumerableEachWithIndex, -1},
		{"each_with_object", EnumerableEachWithObject, 1},
		{"flat_map", EnumerableFlatMap, 0},
		{"collect_concat", EnumerableFlatMap, 0},
		{"filter_map", EnumerableFilterMap, 0},
		{"compact", EnumerableCompact, 0},
		{"chain", EnumerableChain, -1},
		{"uniq", EnumerableUniq, 0},
		{"take_while", EnumerableTakeWhile, 0},
		{"drop_while", EnumerableDropWhile, 0},
		{"reverse_each", EnumerableReverseEach, 0},
		{"sum", EnumerableSum, -1},
		{"sort_by", EnumerableSortBy, 0},
		{"minmax_by", EnumerableMinmaxBy, -1},
	})
	rt.Comparable = defineRuntimeModule("Comparable", []runtimeModuleMethod{
		{"==", ComparableEqual, 1},
		{"<", ComparableLess, 1},
		{"<=", ComparableLessEqual, 1},
		{">", ComparableGreater, 1},
		{">=", ComparableGreaterEqual, 1},
		{"between?", ComparableBetween, 2},
		{"clamp", ComparableClamp, -1},
	})

	objectClass := rt.Classes["Object"]
	objectClass.Constants["Enumerable"] = rt.Enumerable
	objectClass.Constants["Comparable"] = rt.Comparable

	comparable := rt.Comparable.Data.(*object.Module)
	for _, name := range []string{"Time", "Numeric", "Integer", "Float", "Rational", "String", "Symbol", "File::Stat"} {
		if class := rt.Classes[name]; class != nil {
			class.Include(comparable)
		}
	}

	enumerable := rt.Enumerable.Data.(*object.Module)
	for _, name := range []string{"Array", "Hash", "Range", "StringIO", "Enumerator", "Struct", "Dir", "File", "IO", "Set", "ObjectSpace::WeakMap"} {
		if class := rt.Classes[name]; class != nil {
			class.Include(enumerable)
		}
	}
}

func InitializeRuntimeModules() {
	if R != nil && R.Enumerable == nil {
		R.initializeRuntimeModules()
	}
}
