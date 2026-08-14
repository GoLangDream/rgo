package core

import (
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoLangDream/rgo/pkg/object"
)

func init() {
	Init()
}

func TestMspecRegistrationIsExplicit(t *testing.T) {
	Init()
	if _, ok := R.Classes["Object"].GetMethod("describe"); ok {
		t.Fatal("ordinary runtime unexpectedly exposes MSpec describe")
	}
	InitWithMspec()
	if _, ok := R.Classes["Object"].GetMethod("describe"); !ok {
		t.Fatal("MSpec runtime did not expose describe")
	}
	Init()
}

func TestSmallIntegerValuesAreCanonicalWithinRuntime(t *testing.T) {
	Init()
	first := NewIntegerValue(1024)
	second := NewIntegerValue(1024)
	if first != second {
		t.Fatal("expected cached Integer values to be canonical")
	}
	if first.Data != int64(1024) || first.Class != R.Classes["Integer"] {
		t.Fatalf("unexpected cached Integer value: %#v", first)
	}
	if NewIntegerValue(4097) != NewIntegerValue(4097) {
		t.Fatal("expected lazily paged Integer values to be canonical")
	}
	if NewIntegerValue(-4096) != NewIntegerValue(-4096) {
		t.Fatal("expected common negative Integer values to be canonical")
	}
	if NewIntegerValue(-4097) == NewIntegerValue(-4097) {
		t.Fatal("expected values below the immediate cache to remain independently allocated")
	}
	if NewIntegerValue(65536) == NewIntegerValue(65536) {
		t.Fatal("expected values outside the immediate cache to remain independently allocated")
	}
}

func TestIntegerToSReusesOnlyImmutablePayload(t *testing.T) {
	Init()
	first := intToS(NewIntegerValue(7))
	second := intToS(NewIntegerValue(7))
	if first == nil || second == nil || first == second || first.Data != "7" || second.Data != "7" {
		t.Fatalf("unexpected Integer#to_s values: first=%#v second=%#v", first, second)
	}
	if got := stringConcatOne(first, NewStringValue("x")); got != first || first.Data != "7x" || second.Data != "7" {
		t.Fatalf("Integer#to_s payload sharing leaked through String mutation: first=%s second=%s", first.Inspect(), second.Inspect())
	}
	if got := IntegerToSRawBuiltin(70000); got != "70000" {
		t.Fatalf("uncached Integer#to_s payload = %q", got)
	}
}

func TestIntegerToSLengthRawBuiltinMatchesDecimalFormatting(t *testing.T) {
	for _, value := range []int64{
		math.MinInt64, -1_000_000_000_000_000_000, -100, -10, -1, 0, 1, 9, 10, 99,
		100, 999, 1_000, 1_000_000_000_000_000_000, math.MaxInt64,
	} {
		want := len(strconv.FormatInt(value, 10))
		if got := IntegerToSLengthRawBuiltin(value); got != want {
			t.Fatalf("Integer#to_s length for %d = %d, want %d", value, got, want)
		}
	}
}

func TestAppendASCIIBytePatternWritesAndDeclinesOverflow(t *testing.T) {
	Init()
	value := &object.EmeraldValue{Type: object.ValueString, Data: "", Class: R.Classes["String"]}
	result, handled := AppendASCIIBytePattern(value, 97, 26, 0, 1, 52)
	if !handled || result != value {
		t.Fatalf("ASCII byte pattern was not handled: handled=%v result=%p want=%p", handled, result, value)
	}
	if got := value.Data.(string); got != "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("unexpected ASCII byte pattern result: %q", got)
	}
	long := &object.EmeraldValue{Type: object.ValueString, Data: "", Class: R.Classes["String"]}
	if _, handled = AppendASCIIBytePattern(long, 97, 26, 0, 1, 12000); !handled {
		t.Fatal("long ASCII byte pattern was not handled")
	}
	wantLong := strings.Repeat("abcdefghijklmnopqrstuvwxyz", 461) + "abcdefghijklmn"
	if got := long.Data.(string); got != wantLong {
		t.Fatalf("long ASCII byte pattern crossed chunk boundary incorrectly: got tail %q want tail %q", got[len(got)-16:], wantLong[len(wantLong)-16:])
	}
	stepped := &object.EmeraldValue{Type: object.ValueString, Data: "seed", Class: R.Classes["String"]}
	if _, handled = AppendASCIIBytePattern(stepped, 65, 5, -1, 2, 7); !handled {
		t.Fatal("stepped ASCII byte pattern was not handled")
	}
	if got := stepped.Data.(string); got != "seedEBDACEB" {
		t.Fatalf("unexpected stepped ASCII byte pattern result: %q", got)
	}
	overflow := &object.EmeraldValue{Type: object.ValueString, Data: "seed", Class: R.Classes["String"]}
	if _, handled = AppendASCIIBytePattern(overflow, 97, 26, math.MaxInt64, 1, 1); handled {
		t.Fatal("overflowing ASCII byte pattern must decline")
	}
	if overflow.Data.(string) != "seed" {
		t.Fatalf("overflow decline mutated String: %q", overflow.Data.(string))
	}
}

func TestDuplicateStringDoesNotShareColdSidecar(t *testing.T) {
	Init()
	receiver := NewStringValue("abc")
	builder := &strings.Builder{}
	builder.WriteString("abc")
	receiver.SetStringBuilder(builder)

	duplicate := duplicateStringValue(receiver, false, false)
	if receiver.StringBuilderValue() != builder {
		t.Fatal("source StringBuilder changed while duplicating String")
	}
	if duplicate.StringBuilderValue() != nil {
		t.Fatal("String#dup retained the source StringBuilder")
	}
	duplicate.SetStringBuilder(&strings.Builder{})
	if receiver.StringBuilderValue() != builder {
		t.Fatal("mutating duplicate StringBuilder changed source sidecar")
	}
}

func TestIntegerArithmeticUsesCheckedInt64FastPath(t *testing.T) {
	Init()
	left := NewIntegerValue(120)
	right := NewIntegerValue(7)
	if got := intAdd(left, right); got != NewIntegerValue(127) || got.BigIntValue() != nil {
		t.Fatalf("integer addition did not use canonical int64 result: %s", got.Inspect())
	}
	if got := intSub(left, right); got != NewIntegerValue(113) || got.BigIntValue() != nil {
		t.Fatalf("integer subtraction did not use canonical int64 result: %s", got.Inspect())
	}
	if got := intMul(left, right); got != NewIntegerValue(840) || got.BigIntValue() != nil {
		t.Fatalf("integer multiplication did not use canonical int64 result: %s", got.Inspect())
	}
	if got := intMod(NewIntegerValue(-5), NewIntegerValue(3)); got != NewIntegerValue(1) {
		t.Fatalf("floor modulo = %s, want 1", got.Inspect())
	}
	if got := intMod(NewIntegerValue(5), NewIntegerValue(-3)); got != NewIntegerValue(-1) {
		t.Fatalf("negative-divisor modulo = %s, want -1", got.Inspect())
	}

	overflow := intAdd(NewIntegerValue(math.MaxInt64), NewIntegerValue(1))
	if overflow.BigIntValue() == nil || overflow.Inspect() != "9223372036854775808" {
		t.Fatalf("addition overflow lost arbitrary precision: %s", overflow.Inspect())
	}
	underflow := intSub(NewIntegerValue(math.MinInt64), NewIntegerValue(1))
	if underflow.BigIntValue() == nil || underflow.Inspect() != "-9223372036854775809" {
		t.Fatalf("subtraction underflow lost arbitrary precision: %s", underflow.Inspect())
	}
	productOverflow := intMul(NewIntegerValue(math.MinInt64), NewIntegerValue(-1))
	if productOverflow.BigIntValue() == nil || productOverflow.Inspect() != "9223372036854775808" {
		t.Fatalf("multiplication overflow lost arbitrary precision: %s", productOverflow.Inspect())
	}
}

func TestJSONQuotedStringFastPathKeepsEscapingSlowPath(t *testing.T) {
	for _, value := range []string{"ruby", "<ruby>", "你好"} {
		if !jsonStringNeedsNoEscape(value) {
			t.Fatalf("ordinary UTF-8 JSON string %q should use the no-escape path", value)
		}
	}
	for _, value := range []string{"line\nfeed", `quote"`, `slash\\`, "\u2028", string([]byte{0xff})} {
		if jsonStringNeedsNoEscape(value) {
			t.Fatalf("JSON string %q must use the escaping slow path", value)
		}
	}
	if got := jsonQuotedString("ruby"); got != `"ruby"` {
		t.Fatalf("ordinary JSON quoting = %q, want %q", got, `"ruby"`)
	}
}

func TestNormalizeEncodingNameForIOAvoidsCopyForCanonicalName(t *testing.T) {
	if got := normalizeEncodingNameForIO("UTF-8"); got != "UTF-8" {
		t.Fatalf("unexpected canonical encoding: %q", got)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_ = normalizeEncodingNameForIO("UTF-8")
	}); allocations != 0 {
		t.Fatalf("canonical encoding normalization allocated %v times", allocations)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_ = normalizeEncodingNameForIO("windows-1252")
		_ = normalizeEncodingNameForIO("utf-8")
	}); allocations != 0 {
		t.Fatalf("common encoding alias normalization allocated %v times", allocations)
	}
	for input, expected := range map[string]string{
		"ascii_8bit":   "ASCII-8BIT",
		"bom|utf-8":    "UTF-8",
		"Shift_JIS":    "SHIFT-JIS",
		"Windows-1252": "WINDOWS-1252",
	} {
		if got := normalizeEncodingNameForIO(input); got != expected {
			t.Fatalf("normalizeEncodingNameForIO(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestBCryptHashWithSettingMatchesReferenceVector(t *testing.T) {
	const setting = "$2a$10$XajjQvNhvvRt5GSeFk1xFe"
	const expected = "$2a$10$XajjQvNhvvRt5GSeFk1xFeyqRrsxkhBkUiQeg0dt.wU1qD4aFDcga"

	actual, err := bcryptHashWithSetting([]byte("allmine"), setting)
	if err != nil {
		t.Fatalf("bcrypt hash failed: %v", err)
	}
	if actual != expected {
		t.Fatalf("bcrypt hash mismatch:\nactual:   %s\nexpected: %s", actual, expected)
	}
}

func TestBCryptHashWithSettingChangesForWrongPassword(t *testing.T) {
	const setting = "$2a$04$XajjQvNhvvRt5GSeFk1xFe"
	correct, err := bcryptHashWithSetting([]byte("allmine"), setting)
	if err != nil {
		t.Fatalf("bcrypt correct-password hash failed: %v", err)
	}
	wrong, err := bcryptHashWithSetting([]byte("notmine"), setting)
	if err != nil {
		t.Fatalf("bcrypt wrong-password hash failed: %v", err)
	}
	if correct == wrong {
		t.Fatal("bcrypt produced the same hash for different passwords")
	}
}

func mkInt(v int64) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueInteger, Data: v, Class: R.Classes["Integer"]}
}

func mkFloat(v float64) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueFloat, Data: v, Class: R.Classes["Float"]}
}

func mkStr(v string) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueString, Data: v, Class: R.Classes["String"]}
}

func mkArr(elems ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueArray, Data: elems, Class: R.Classes["Array"]}
}

func mkMapHash(pairs map[*object.EmeraldValue]*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueHash, Data: pairs, Class: R.Classes["Hash"]}
}

func mkRHash(pairs map[*object.EmeraldValue]*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  &object.RHash{Pairs: pairs},
		Class: R.Classes["Hash"],
	}
}

func TestHashBucketsTrackInsertLookupAndDelete(t *testing.T) {
	hash := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	for i := int64(0); i < 1000; i++ {
		hashIndexSet(hash, mkInt(i), mkInt(i*3))
	}
	data := hashData(hash)
	if data.BucketSize != 1000 || len(data.Buckets) == 0 {
		t.Fatalf("expected 1000 indexed keys, got size=%d buckets=%d", data.BucketSize, len(data.Buckets))
	}
	for i := int64(0); i < 1000; i++ {
		assertInt(t, hashIndex(hash, mkInt(i)), i*3)
	}
	for i := int64(0); i < 1000; i += 2 {
		assertInt(t, hashDelete(hash, mkInt(i)), i*3)
	}
	if got := hashLength(hash).Data.(int64); got != 500 {
		t.Fatalf("expected 500 keys after delete, got %d", got)
	}
	for i := int64(1); i < 1000; i += 2 {
		assertInt(t, hashIndex(hash, mkInt(i)), i*3)
	}
	if data.BucketSize != 500 {
		t.Fatalf("expected rebuilt bucket size 500, got %d", data.BucketSize)
	}
}

func TestIntegerHashBatchPreservesOrderAndDuplicateAssignments(t *testing.T) {
	hash := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	hashIndexSet(hash, mkInt(0), mkInt(99))
	keys := []*object.EmeraldValue{mkInt(0), mkInt(1), mkInt(0)}
	values := []*object.EmeraldValue{mkInt(7), mkInt(11), mkInt(13)}
	if !IntegerHashBatchAvailable(hash) {
		t.Fatal("expected plain integer-key Hash to admit batch storage")
	}
	if !StoreIntegerHashBatch(hash, keys, values) {
		t.Fatal("integer hash batch storage failed")
	}
	data := hashData(hash)
	if len(data.Pairs) != 2 || len(data.Keys) != 2 {
		t.Fatalf("expected two keys after duplicate assignment, pairs=%d keys=%d", len(data.Pairs), len(data.Keys))
	}
	assertInt(t, hashIndex(hash, mkInt(0)), 13)
	assertInt(t, hashIndex(hash, mkInt(1)), 11)
	if data.Keys[0].Data.(int64) != 0 || data.Keys[1].Data.(int64) != 1 {
		t.Fatalf("batch changed insertion order: %#v", data.Keys)
	}

	nonInteger := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	hashIndexSet(nonInteger, mkStr("key"), mkInt(1))
	if IntegerHashBatchAvailable(nonInteger) {
		t.Fatal("non-integer-key Hash must remain on the generic path")
	}
}

func TestIntegerHashBatchCanonicalKeysUpdatesInPlace(t *testing.T) {
	Init()
	hash := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	keys := []*object.EmeraldValue{NewIntegerValue(0), NewIntegerValue(1), NewIntegerValue(0)}
	values := []*object.EmeraldValue{NewIntegerValue(7), NewIntegerValue(11), NewIntegerValue(13)}
	if !StoreIntegerHashBatch(hash, keys, values) {
		t.Fatal("canonical integer hash batch storage failed")
	}
	data := hashData(hash)
	if len(data.Pairs) != 2 || len(data.Keys) != 2 || data.Keys[0] != keys[0] || data.Keys[1] != keys[1] {
		t.Fatalf("canonical batch changed order or key identity: pairs=%d keys=%#v", len(data.Pairs), data.Keys)
	}
	assertInt(t, hashIndex(hash, NewIntegerValue(0)), 13)
	assertInt(t, hashIndex(hash, NewIntegerValue(1)), 11)
}

func TestIntegerHashBatchGroupedPreservesExistingOrderAndDuplicates(t *testing.T) {
	Init()
	hash := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	seed := NewIntegerValue(0)
	hashIndexSet(hash, seed, NewIntegerValue(99))
	keys := make([]*object.EmeraldValue, 4097)
	values := make([]*object.EmeraldValue, 4097)
	for index := range keys[:4096] {
		integer := int64(index % 997)
		keys[index] = NewIntegerValue(integer)
		values[index] = NewIntegerValue(int64(index))
	}
	keys[4096] = NewIntegerValue(0)
	values[4096] = NewIntegerValue(777)
	if !StoreIntegerHashBatch(hash, keys, values) {
		t.Fatal("grouped canonical integer hash batch storage failed")
	}
	data := hashData(hash)
	if len(data.Pairs) != 997 || len(data.Keys) != 997 || data.Keys[0] != seed {
		t.Fatalf("grouped batch changed key order or count: pairs=%d keys=%d first=%p want=%p", len(data.Pairs), len(data.Keys), data.Keys[0], seed)
	}
	assertInt(t, hashIndex(hash, NewIntegerValue(0)), 777)
	assertInt(t, hashIndex(hash, NewIntegerValue(1)), 3989)
}

func TestIntegerHashRawBatchPreservesExistingOrderAndDuplicates(t *testing.T) {
	Init()
	hash := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	seed := NewIntegerValue(0)
	hashIndexSet(hash, seed, NewIntegerValue(99))
	keys := make([]int64, 4097)
	values := make([]*object.EmeraldValue, 4097)
	for index := range keys[:4096] {
		keys[index] = int64(index % 997)
		values[index] = NewIntegerValue(int64(index))
	}
	keys[4096] = 0
	values[4096] = NewIntegerValue(777)
	if !StoreIntegerHashRawBatchTrustedCanonical(hash, keys, values) {
		t.Fatal("raw grouped canonical integer hash batch storage failed")
	}
	data := hashData(hash)
	if len(data.Pairs) != 997 || len(data.Keys) != 997 || data.Keys[0] != seed {
		t.Fatalf("raw grouped batch changed key order or count: pairs=%d keys=%d first=%p want=%p", len(data.Pairs), len(data.Keys), data.Keys[0], seed)
	}
	assertInt(t, hashIndex(hash, NewIntegerValue(0)), 777)
	assertInt(t, hashIndex(hash, NewIntegerValue(1)), 3989)
}

func TestIntegerHashLinearBatchPreservesOrderAndOverflowFallback(t *testing.T) {
	Init()
	hash := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	next, ok := StoreIntegerHashLinearBatch(hash, 2, 2, 3, 4)
	if !ok || next != 10 {
		t.Fatalf("linear batch failed: ok=%v next=%d", ok, next)
	}
	region, ok := DirectHashLinearRegion(hash)
	if !ok || region.Start != 2 || region.Step != 2 || region.ValueOffset != 3 || region.Count != 4 {
		t.Fatalf("linear region metadata missing or incorrect: ok=%v region=%#v", ok, region)
	}
	if raw, ok := hash.Data.(*object.RHash); !ok || raw.Pairs != nil {
		t.Fatalf("linear batch should defer pointer-map materialization: %#v", hash.Data)
	}
	if value, ok := DirectHashLinearValue(region, NewIntegerValue(2)); !ok || value != 5 {
		t.Fatalf("linear direct value mismatch: ok=%v value=%d", ok, value)
	}
	if _, ok := DirectHashLinearValue(region, NewIntegerValue(math.MaxInt64)); ok {
		t.Fatal("linear direct value must reject checked-add overflow")
	}
	if key, value, ok := DirectHashLinearPairAt(region, 2); !ok || key != 6 || value != 9 {
		t.Fatalf("linear direct pair mismatch: ok=%v key=%d value=%d", ok, key, value)
	}
	if _, _, ok := DirectHashLinearPairAt(region, region.Count); ok {
		t.Fatal("linear direct pair must reject an out-of-range index")
	}
	data := hashData(hash)
	if len(data.Keys) != 4 || data.Keys[0].Data.(int64) != 2 || data.Keys[3].Data.(int64) != 8 {
		t.Fatalf("linear batch changed insertion order: %#v", data.Keys)
	}
	assertInt(t, hashIndex(hash, NewIntegerValue(2)), 5)
	assertInt(t, hashIndex(hash, NewIntegerValue(8)), 11)
	if _, ok := DirectHashLinearRegion(hash); ok {
		t.Fatal("general Hash lookup must consume lazy linear region")
	}
	mutated := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	if _, ok := StoreIntegerHashLinearBatch(mutated, 0, 1, 1, 4); !ok {
		t.Fatal("linear mutation fixture failed")
	}
	if !StoreHashValue(mutated, NewIntegerValue(1), NewIntegerValue(99)) {
		t.Fatal("Hash mutation fixture failed")
	}
	if _, ok := DirectHashLinearRegion(mutated); ok {
		t.Fatal("general Hash mutation must consume lazy linear region")
	}
	assertInt(t, hashIndex(mutated, NewIntegerValue(1)), 99)
	overflow := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	if _, ok := StoreIntegerHashLinearBatch(overflow, math.MaxInt64, 1, 0, 2); ok {
		t.Fatal("overflowing linear batch must decline before mutating Hash")
	}
	if data := hashData(overflow); len(data.Keys) != 0 || len(data.Pairs) != 0 {
		t.Fatalf("overflow fallback mutated Hash: %#v", data)
	}

	prefixHash := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	seed := NewIntegerValue(0)
	hashIndexSet(prefixHash, seed, NewIntegerValue(99))
	next, ok = StoreIntegerHashLinearBatch(prefixHash, 1, 1, 10, 3)
	if !ok || next != 4 {
		t.Fatalf("linear batch with prefix failed: ok=%v next=%d", ok, next)
	}
	prefixData := hashData(prefixHash)
	if len(prefixData.Keys) != 4 || prefixData.Keys[0] != seed {
		t.Fatalf("linear batch with prefix changed insertion order: %#v", prefixData.Keys)
	}
	assertInt(t, hashIndex(prefixHash, NewIntegerValue(0)), 99)
	assertInt(t, hashIndex(prefixHash, NewIntegerValue(1)), 11)
	affinePrefix := mkRHash(make(map[*object.EmeraldValue]*object.EmeraldValue))
	hashIndexSet(affinePrefix, NewIntegerValue(0), NewIntegerValue(1))
	if _, ok := StoreIntegerHashLinearBatch(affinePrefix, 1, 1, 1, 3); !ok {
		t.Fatal("affine prefix linear batch failed")
	}
	if region, ok := DirectHashLinearRegion(affinePrefix); !ok || region.Count != 4 {
		t.Fatalf("affine prefix should remain lazy: ok=%v region=%#v", ok, region)
	}
	if raw, ok := affinePrefix.Data.(*object.RHash); !ok || raw.Pairs != nil {
		t.Fatal("affine prefix should defer pointer-map materialization")
	}
}

func callMethod(t *testing.T, receiver *object.EmeraldValue, name string, args ...*object.EmeraldValue) *object.EmeraldValue {
	t.Helper()
	method, ok := receiver.Class.GetMethod(name)
	if !ok {
		t.Fatalf("method %s not found on %s", name, receiver.Class.Name)
	}
	fn := method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue)
	return fn(receiver, args...)
}

func assertInt(t *testing.T, val *object.EmeraldValue, expected int64) {
	t.Helper()
	if val.Type != object.ValueInteger {
		t.Fatalf("expected Integer, got %v", val.Type)
	}
	if val.Data.(int64) != expected {
		t.Errorf("expected %d, got %d", expected, val.Data.(int64))
	}
}

func assertFloat(t *testing.T, val *object.EmeraldValue, expected float64) {
	t.Helper()
	if val.Type != object.ValueFloat {
		t.Fatalf("expected Float, got %v", val.Type)
	}
	if val.Data.(float64) != expected {
		t.Errorf("expected %f, got %f", expected, val.Data.(float64))
	}
}

func assertStr(t *testing.T, val *object.EmeraldValue, expected string) {
	t.Helper()
	if val.Type != object.ValueString {
		t.Fatalf("expected String, got %v", val.Type)
	}
	if val.Data.(string) != expected {
		t.Errorf("expected %q, got %q", expected, val.Data.(string))
	}
}

func assertBool(t *testing.T, val *object.EmeraldValue, expected bool) {
	t.Helper()
	if expected {
		if val != R.TrueVal {
			t.Errorf("expected true, got %v", val)
		}
	} else {
		if val != R.FalseVal {
			t.Errorf("expected false, got %v", val)
		}
	}
}

func assertNil(t *testing.T, val *object.EmeraldValue) {
	t.Helper()
	if val != R.NilVal {
		t.Errorf("expected nil, got %v", val)
	}
}

func assertExceptionType(t *testing.T, val *object.EmeraldValue, expected *object.Class) {
	t.Helper()
	if val == nil {
		t.Fatalf("expected exception of type %s, got nil", expected.Name)
	}
	if val.Type != object.ValueException {
		t.Fatalf("expected exception, got %v", val.Type)
	}
	if val.Class != expected {
		t.Fatalf("expected exception class %s, got %s", expected.Name, val.Class.Name)
	}
}

// === Init ===

func TestInitCreatesClasses(t *testing.T) {
	expected := []string{
		"BasicObject", "Object", "Module", "Class",
		"TrueClass", "FalseClass", "NilClass",
		"Integer", "Float", "String", "Array", "Hash",
		"Symbol", "Regexp", "Range", "Proc",
	}
	for _, name := range expected {
		if _, ok := R.Classes[name]; !ok {
			t.Errorf("class %s not found", name)
		}
	}
}

func TestInitClassHierarchy(t *testing.T) {
	if R.Classes["Object"].SuperClass != R.Classes["BasicObject"] {
		t.Error("Object should inherit from BasicObject")
	}
	if R.Classes["Integer"].SuperClass != R.Classes["Numeric"] {
		t.Error("Integer should inherit from Numeric")
	}
	if R.Classes["Numeric"].SuperClass != R.Classes["Object"] {
		t.Error("Numeric should inherit from Object")
	}
	if R.Classes["String"].SuperClass != R.Classes["Object"] {
		t.Error("String should inherit from Object")
	}
}

func TestInitSingletons(t *testing.T) {
	if R.TrueVal == nil || R.TrueVal.Data != true {
		t.Error("TrueVal not initialized")
	}
	if R.FalseVal == nil || R.FalseVal.Data != false {
		t.Error("FalseVal not initialized")
	}
	if R.NilVal == nil || R.NilVal.Type != object.ValueNil {
		t.Error("NilVal not initialized")
	}
}

// === Integer Methods ===

func TestIntAdd(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(3), "+", mkInt(4)), 7)
}

func TestIntAddFloat(t *testing.T) {
	assertFloat(t, callMethod(t, mkInt(3), "+", mkFloat(1.5)), 4.5)
}

func TestIntSub(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(10), "-", mkInt(3)), 7)
}

func TestIntMul(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(3), "*", mkInt(4)), 12)
}

func TestIntDiv(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(10), "/", mkInt(3)), 3)
}

func TestIntDivByZero(t *testing.T) {
	assertExceptionType(t, callMethod(t, mkInt(10), "/", mkInt(0)), R.Classes["ZeroDivisionError"])
}

func TestIntMod(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(17), "%", mkInt(5)), 2)
}

func TestIntModByZero(t *testing.T) {
	assertExceptionType(t, callMethod(t, mkInt(17), "%", mkInt(0)), R.Classes["ZeroDivisionError"])
}

func TestIntPow(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(2), "**", mkInt(10)), 1024)
}

func TestIntToS(t *testing.T) {
	assertStr(t, callMethod(t, mkInt(42), "to_s"), "42")
}

func TestIntSucc(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(5), "succ"), 6)
}

func TestIntPred(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(5), "pred"), 4)
}

func TestIntChr(t *testing.T) {
	assertStr(t, callMethod(t, mkInt(65), "chr"), "A")
}

func TestIntOdd(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(3), "odd?"), true)
	assertBool(t, callMethod(t, mkInt(4), "odd?"), false)
}

func TestIntEven(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(4), "even?"), true)
	assertBool(t, callMethod(t, mkInt(3), "even?"), false)
}

func TestIntZero(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(0), "zero?"), true)
	assertBool(t, callMethod(t, mkInt(1), "zero?"), false)
}

func TestIntAbs(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(-5), "abs"), 5)
	assertInt(t, callMethod(t, mkInt(5), "abs"), 5)
}

func TestIntToF(t *testing.T) {
	assertFloat(t, callMethod(t, mkInt(5), "to_f"), 5.0)
}

func TestIntGcdTypeErrorForNonIntegerArg(t *testing.T) {
	got := callMethod(t, mkInt(10), "gcd", mkFloat(2.5))
	assertExceptionType(t, got, R.Classes["TypeError"])
}

func TestIntGcdWithLargerNumbers(t *testing.T) {
	got := callMethod(t, mkInt(1152921504606846976), "gcd", mkInt(2))
	assertInt(t, got, 2)
}

func TestIntGcdWithNegativeValues(t *testing.T) {
	assertInt(t, callMethod(t, mkInt(-12), "gcd", mkInt(6)), 6)
	assertInt(t, callMethod(t, mkInt(12), "gcd", mkInt(-6)), 6)
	assertInt(t, callMethod(t, mkInt(-12), "gcd", mkInt(-6)), 6)
}

func TestIntLcmTypeErrorForNonIntegerArg(t *testing.T) {
	got := callMethod(t, mkInt(10), "lcm", mkFloat(2.5))
	assertExceptionType(t, got, R.Classes["TypeError"])
}

func TestIntLcmWithLargerNumbers(t *testing.T) {
	got := callMethod(t, mkInt(1152921504606846976), "lcm", mkInt(2))
	assertInt(t, got, 1152921504606846976)
}

// === Float Methods ===

func TestFloatAdd(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(1.5), "+", mkFloat(2.5)), 4.0)
}

func TestFloatAddInt(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(1.5), "+", mkInt(2)), 3.5)
}

func TestFloatSub(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(5.5), "-", mkFloat(2.0)), 3.5)
}

func TestFloatMul(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(2.5), "*", mkFloat(4.0)), 10.0)
}

func TestFloatDiv(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(10.0), "/", mkFloat(4.0)), 2.5)
}

func TestFloatDivByZero(t *testing.T) {
	assertFloat(t, callMethod(t, mkFloat(10.0), "/", mkFloat(0)), math.Inf(1))
}

func TestFloatToS(t *testing.T) {
	assertStr(t, callMethod(t, mkFloat(3.14), "to_s"), "3.14")
}

func TestFloatToI(t *testing.T) {
	assertInt(t, callMethod(t, mkFloat(3.14), "to_i"), 3)
}

// === String Methods ===

func TestStringAdd(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "+", mkStr(" world")), "hello world")
}

func TestStringValueBatchKeepsStringInterfaceAlive(t *testing.T) {
	batch := NewStringValueBatchWithByteCapacity(2, 16)
	first := batch.New("first")
	second := batch.NewInteger(42)
	runtime.GC()

	if first == nil || second == nil || first == second {
		t.Fatal("StringValueBatch did not return distinct values")
	}
	if got, ok := first.Data.(string); !ok || got != "first" {
		t.Fatalf("first batch value = %#v, want string %q", first.Data, "first")
	}
	if got, ok := second.Data.(string); !ok || got != "42" {
		t.Fatalf("second batch value = %#v, want string %q", second.Data, "42")
	}
	smallIntegerBatch := NewStringValueBatchWithByteCapacity(1, 8)
	smallInteger := smallIntegerBatch.NewInteger(7)
	if got, ok := smallInteger.Data.(string); !ok || got != "7" {
		t.Fatalf("single-digit NewInteger = %#v, want string %q", smallInteger.Data, "7")
	}

	formatBatch := NewStringValueBatchWithByteCapacity(5, 64)
	for _, tc := range []struct {
		value  int64
		suffix string
		want   string
	}{
		{value: -9, suffix: "!", want: "-9!"},
		{value: -1, suffix: "", want: "-1"},
		{value: 0, suffix: "!", want: "0!"},
		{value: 9, suffix: "!", want: "9!"},
		{value: 10, suffix: "!", want: "10!"},
	} {
		got := formatBatch.NewIntegerSuffix(tc.value, tc.suffix)
		if actual, ok := got.Data.(string); !ok || actual != tc.want {
			t.Fatalf("NewIntegerSuffix(%d, %q) = %#v, want %q", tc.value, tc.suffix, got.Data, tc.want)
		}
	}
}

func TestStringConcatWithNonStringRaisesTypeErrorWithoutPanicking(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("String#concat should not panic for non-string argument: %v", recovered)
		}
	}()
	assertExceptionType(t, callMethod(t, mkStr("hello"), "concat", R.NilVal), R.Classes["TypeError"])
}

func TestStringMul(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("ab"), "*", mkInt(3)), "ababab")
}

func TestStringLength(t *testing.T) {
	assertInt(t, callMethod(t, mkStr("hello"), "length"), 5)
}

func TestStringSize(t *testing.T) {
	assertInt(t, callMethod(t, mkStr("hello"), "size"), 5)
}

func TestStringEmpty(t *testing.T) {
	assertBool(t, callMethod(t, mkStr(""), "empty?"), true)
	assertBool(t, callMethod(t, mkStr("x"), "empty?"), false)
}

func TestStringUpcase(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "upcase"), "HELLO")
}

func TestStringDowncase(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("HELLO"), "downcase"), "hello")
}

func TestStringCapitalize(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "capitalize"), "Hello")
}

func TestStringReverse(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "reverse"), "olleh")
}

func TestStringChopRemovesOneByteFromBinaryString(t *testing.T) {
	value := mkStr("\xD0\xBF")
	SetStringEncoding(value, "BINARY")
	if encoding := stringEncodingName(value); encoding != "BINARY" {
		t.Fatalf("expected BINARY encoding, got %q", encoding)
	}
	chunks := stringCharacterChunks(value)
	if len(chunks) != 2 || len(chunks[1]) != 1 {
		t.Fatalf("expected two byte chunks, got %#v", chunks)
	}
	assertStr(t, stringChop(value), "\xD0")
}

func TestStringInclude(t *testing.T) {
	assertBool(t, callMethod(t, mkStr("hello world"), "include?", mkStr("world")), true)
	assertBool(t, callMethod(t, mkStr("hello"), "include?", mkStr("xyz")), false)
}

func TestStringStartWith(t *testing.T) {
	assertBool(t, callMethod(t, mkStr("hello"), "start_with?", mkStr("hel")), true)
	assertBool(t, callMethod(t, mkStr("hello"), "start_with?", mkStr("xyz")), false)
}

func TestStringEndWith(t *testing.T) {
	assertBool(t, callMethod(t, mkStr("hello"), "end_with?", mkStr("llo")), true)
	assertBool(t, callMethod(t, mkStr("hello"), "end_with?", mkStr("xyz")), false)
}

func TestStringIndex(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "[]", mkInt(0)), "h")
	assertStr(t, callMethod(t, mkStr("hello"), "[]", mkInt(4)), "o")
}

func TestStringIndexNegative(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "[]", mkInt(-1)), "o")
}

func TestStringIndexOutOfBounds(t *testing.T) {
	assertNil(t, callMethod(t, mkStr("hello"), "[]", mkInt(10)))
}

func TestStringToI(t *testing.T) {
	assertInt(t, callMethod(t, mkStr("42"), "to_i"), 42)
}

func TestStringToS(t *testing.T) {
	s := mkStr("hello")
	result := callMethod(t, s, "to_s")
	if result != s {
		t.Error("to_s should return self for strings")
	}
}

func TestBuiltinFormatFixedFiveFloatMatchesGeneralFormatter(t *testing.T) {
	format := mkStr("%.5f")
	values := []*object.EmeraldValue{mkInt(1), mkFloat(1.25), mkFloat(-0.0), mkFloat(math.NaN()), mkFloat(math.Inf(1))}
	for _, value := range values {
		got := builtinFormat(nil, format, value)
		want := rubyString(rubySprintfEncoded("%.5f", stringEncodingName(format), value))
		if got == nil || got.Type != object.ValueString || got.Data != want.Data {
			t.Fatalf("fixed float format mismatch for %#v: got %#v want %#v", value, got, want)
		}
	}
}

// === Array Methods ===

func TestArrayLength(t *testing.T) {
	arr := mkArr(mkInt(1), mkInt(2), mkInt(3))
	assertInt(t, callMethod(t, arr, "length"), 3)
}

func TestArrayFirst(t *testing.T) {
	arr := mkArr(mkInt(10), mkInt(20))
	assertInt(t, callMethod(t, arr, "first"), 10)
}

func TestArrayFirstEmpty(t *testing.T) {
	arr := mkArr()
	assertNil(t, callMethod(t, arr, "first"))
}

func TestArrayLast(t *testing.T) {
	arr := mkArr(mkInt(10), mkInt(20))
	assertInt(t, callMethod(t, arr, "last"), 20)
}

func TestArrayLastEmpty(t *testing.T) {
	arr := mkArr()
	assertNil(t, callMethod(t, arr, "last"))
}

func TestArrayPush(t *testing.T) {
	arr := mkArr(mkInt(1))
	result := callMethod(t, arr, "push", mkInt(2))
	elems := result.Data.([]*object.EmeraldValue)
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(elems))
	}
	assertInt(t, elems[1], 2)
}

func TestArrayPop(t *testing.T) {
	arr := mkArr(mkInt(1), mkInt(2))
	assertInt(t, callMethod(t, arr, "pop"), 2)
}

func TestArrayPopEmpty(t *testing.T) {
	arr := mkArr()
	assertNil(t, callMethod(t, arr, "pop"))
}

func TestArrayEmpty(t *testing.T) {
	assertBool(t, callMethod(t, mkArr(), "empty?"), true)
	assertBool(t, callMethod(t, mkArr(mkInt(1)), "empty?"), false)
}

func TestArrayReverse(t *testing.T) {
	arr := mkArr(mkInt(1), mkInt(2), mkInt(3))
	result := callMethod(t, arr, "reverse")
	elems := result.Data.([]*object.EmeraldValue)
	assertInt(t, elems[0], 3)
	assertInt(t, elems[1], 2)
	assertInt(t, elems[2], 1)
}

func TestArrayIndex(t *testing.T) {
	arr := mkArr(mkInt(10), mkInt(20), mkInt(30))
	assertInt(t, callMethod(t, arr, "[]", mkInt(1)), 20)
}

func TestArrayIndexNegative(t *testing.T) {
	arr := mkArr(mkInt(10), mkInt(20), mkInt(30))
	assertInt(t, callMethod(t, arr, "[]", mkInt(-1)), 30)
}

func TestArrayIndexOutOfBounds(t *testing.T) {
	arr := mkArr(mkInt(1))
	assertNil(t, callMethod(t, arr, "[]", mkInt(5)))
}

func TestArrayDeleteAtNonIntegerDoesNotPanic(t *testing.T) {
	arr := mkArr(mkInt(1), mkInt(2))
	arg := &object.EmeraldValue{Type: object.ValueObject, Data: "not-int", Class: R.Classes["Object"]}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("delete_at panicked for non-integer argument: %v", recovered)
		}
	}()

	result := callMethod(t, arr, "delete_at", arg)
	assertNil(t, result)
}

func TestHashIndexSetNilMap(t *testing.T) {
	hash := mkMapHash(nil)
	key := mkStr("k")
	assertInt(t, callMethod(t, hash, "[]=", key, mkInt(42)), 42)
	h, ok := hash.Data.(*object.RHash)
	if !ok {
		t.Fatalf("expected RHash data, got %T", hash.Data)
	}
	if len(h.Pairs) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(h.Pairs))
	}
	assertInt(t, callMethod(t, hash, "[]", key), 42)
}

func TestHashIndexSetReplacesExistingKeyInRHash(t *testing.T) {
	originalKey := mkStr("k")
	otherKey := mkStr("k")
	rHash := &object.RHash{
		Pairs: map[*object.EmeraldValue]*object.EmeraldValue{
			originalKey: mkInt(1),
		},
		Keys: []*object.EmeraldValue{originalKey},
	}
	hash := &object.EmeraldValue{
		Type:  object.ValueHash,
		Data:  rHash,
		Class: R.Classes["Hash"],
	}

	assertInt(t, callMethod(t, hash, "[]=", otherKey, mkInt(2)), 2)
	if len(rHash.Pairs) != 1 {
		t.Fatalf("expected no growth when replacing equivalent key, got %d", len(rHash.Pairs))
	}
	if len(rHash.Keys) != 1 {
		t.Fatalf("expected one tracked key after replacement, got %d", len(rHash.Keys))
	}
	assertInt(t, callMethod(t, hash, "[]", originalKey), 2)
	assertInt(t, callMethod(t, hash, "[]", otherKey), 2)
}

// === Object Methods ===

func TestObjectNilQuestion(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(1), "nil?"), false)
	assertBool(t, callMethod(t, R.NilVal, "nil?"), true)
}

func TestObjectRespondTo(t *testing.T) {
	assertBool(t, callMethod(t, mkInt(1), "respond_to?", mkStr("+")), true)
	assertBool(t, callMethod(t, mkInt(1), "respond_to?", mkStr("nonexistent")), false)
}

func TestObjectToS(t *testing.T) {
	result := callMethod(t, mkInt(42), "to_s")
	if result.Type != object.ValueString {
		t.Errorf("expected String, got %v", result.Type)
	}
}

func TestStringLjustTruncatesRepeatedPad(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "ljust", mkInt(20), mkStr("1234")), "hello123412341234123")
}

func TestStringRjustTruncatesRepeatedPad(t *testing.T) {
	assertStr(t, callMethod(t, mkStr("hello"), "rjust", mkInt(20), mkStr("1234")), "123412341234123hello")
}

func TestStringLjustEmptyPadRaisesArgumentErrorWithoutHanging(t *testing.T) {
	done := make(chan *object.EmeraldValue, 1)
	go func() {
		done <- callMethod(t, mkStr("hello"), "ljust", mkInt(10), mkStr(""))
	}()

	select {
	case result := <-done:
		assertExceptionType(t, result, R.Classes["ArgumentError"])
	case <-time.After(100 * time.Millisecond):
		t.Fatal("String#ljust with empty pad did not return")
	}
}

func TestStringJustifyHandlesObjectBackedStringValues(t *testing.T) {
	wrapped := &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  object.NewObject(R.Classes["String"]),
		Class: R.Classes["String"],
	}

	result := callMethod(t, wrapped, "ljust", mkInt(2))
	assertStr(t, result, "  ")
}

func TestStringMethodsHandleObjectBackedStringValues(t *testing.T) {
	wrapped := &object.EmeraldValue{
		Type:  object.ValueString,
		Data:  object.NewObject(R.Classes["String"]),
		Class: R.Classes["String"],
	}

	assertStr(t, callMethod(t, wrapped, "+", mkStr("suffix")), "suffix")
	assertStr(t, callMethod(t, wrapped, "chomp"), "")
	assertStr(t, callMethod(t, wrapped, "upcase"), "")
	assertStr(t, callMethod(t, wrapped, "<<", mkStr("suffix")), "suffix")
	assertInt(t, callMethod(t, wrapped, "<=>", wrapped), 0)
	assertExceptionType(t, callMethod(t, mkStr("abc"), "insert", mkInt(-5), mkStr("x")), R.Classes["IndexError"])
}

func TestShellQuoteJoinPreservesRubyGlobalReferences(t *testing.T) {
	got := shellQuoteJoin([]string{"rgo", "-e", `$magic_comment_result = "x"`})
	want := `'rgo' '-e' '$magic_comment_result = "x"'`
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestComparableEqualStopsObjectCompareRecursion(t *testing.T) {
	previousCallMethod := CallMethod
	defer func() { CallMethod = previousCallMethod }()

	left := &object.EmeraldValue{Type: object.ValueObject, Data: object.NewObject(R.Classes["Object"]), Class: R.Classes["Object"]}
	right := &object.EmeraldValue{Type: object.ValueObject, Data: object.NewObject(R.Classes["Object"]), Class: R.Classes["Object"]}
	calls := 0
	CallMethod = func(receiver *object.EmeraldValue, method string, args ...*object.EmeraldValue) *object.EmeraldValue {
		calls++
		if calls > 4 {
			return R.NilVal
		}
		if ComparableEqual(receiver, args...) == R.TrueVal {
			return mkInt(0)
		}
		return R.NilVal
	}

	result := ComparableEqual(left, right)
	if result != R.FalseVal || calls != 1 {
		t.Fatalf("expected one comparison and false, got calls=%d result=%s", calls, result.Inspect())
	}
}

func TestExceptionMessageRetainsObjectAndCallsToS(t *testing.T) {
	previousCallMethod := CallMethod
	defer func() { CallMethod = previousCallMethod }()
	CallMethod = func(receiver *object.EmeraldValue, name string, args ...*object.EmeraldValue) *object.EmeraldValue {
		method, ok := receiver.Class.GetMethod(name)
		if !ok {
			return R.NilVal
		}
		return method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue)(receiver, args...)
	}

	messageClass := object.NewClass("MessageForExceptionTest")
	messageClass.SuperClass = R.Classes["Object"]
	messageClass.DefineMethod("to_s", &object.Method{Name: "to_s", Arity: 0, Fn: func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return mkStr("converted message")
	}})
	message := &object.EmeraldValue{Type: object.ValueObject, Data: object.NewObject(messageClass), Class: messageClass}

	exceptionClassValue := &object.EmeraldValue{Type: object.ValueClass, Data: R.Classes["Exception"], Class: R.Classes["Class"]}
	exception := classNew(exceptionClassValue, message)
	assertStr(t, callMethod(t, exception, "to_s"), "converted message")
}

func TestExceptionEqualComparesClassMessageAndBacktrace(t *testing.T) {
	left := newExceptionObject(R.Classes["RuntimeError"], "message")
	right := newExceptionObject(R.Classes["RuntimeError"], "message")
	exceptionSetBacktrace(left, mkArr(mkStr("same:1")))
	exceptionSetBacktrace(right, mkArr(mkStr("same:1")))

	assertBool(t, callMethod(t, left, "==", right), true)
}

func TestExceptionDupCallsInitializeDup(t *testing.T) {
	previousCallMethod := CallMethod
	defer func() { CallMethod = previousCallMethod }()

	called := false
	klass := object.NewClass("CopyableExceptionTest")
	klass.SuperClass = R.Classes["Exception"]
	klass.DefineMethod("initialize_dup", &object.Method{Name: "initialize_dup", Arity: 1, Fn: func(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		called = true
		return receiver
	}})
	CallMethod = func(receiver *object.EmeraldValue, name string, args ...*object.EmeraldValue) *object.EmeraldValue {
		method, ok := receiver.Class.GetMethod(name)
		if !ok {
			return R.NilVal
		}
		return method.Fn.(func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue)(receiver, args...)
	}

	duplicate := methodDup(newExceptionObject(klass, "message"))
	if duplicate.Type == object.ValueException && !called {
		t.Fatal("Exception#dup did not call initialize_dup")
	}
}

func TestRegexpEmptyLookaheadQuantifierReturnsEmptyMatch(t *testing.T) {
	pattern := &object.EmeraldValue{
		Type:  object.ValueRegexp,
		Data:  &object.RRegexp{Pattern: `(?:(?=a)|a)*`},
		Class: R.Classes["Regexp"],
	}
	result := regexpMatchValue(pattern, true, mkStr("aaa"))
	data, ok := matchDataPayload(result)
	if !ok || len(data.Matches) != 1 || data.Matches[0] != "" {
		indices, handled, errText := onigRegexpSearch(`(?:(?=a)|a)*`, "aaa", "")
		t.Fatalf("expected one empty match, got %s (indices=%v handled=%v error=%q fallback=%v)", result.Inspect(), indices, handled, errText, regexpEmptyLookaheadFallbackIndices(`(?:(?=a)|a)*`, "aaa"))
	}
}

func TestRegexpBinaryAndWindowsDotMatchOneByte(t *testing.T) {
	for _, option := range []string{"n", "s"} {
		pattern := &object.EmeraldValue{
			Type:  object.ValueRegexp,
			Data:  &object.RRegexp{Pattern: ".", Options: option},
			Class: R.Classes["Regexp"],
		}
		result := regexpMatchValue(pattern, true, mkStr("\xc3\xa9"))
		data, ok := matchDataPayload(result)
		if !ok || len(data.Matches) != 1 || data.Matches[0] != "\xc3" {
			t.Fatalf("/%s dot expected first byte, got %s", option, result.Inspect())
		}
	}
}

func TestCompileRubyRegexpSharesCompiledPatternAcrossObjects(t *testing.T) {
	first, err := compileRubyRegexp(&object.RRegexp{Pattern: `\A[a-z]+\z`, Options: "i"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := compileRubyRegexp(&object.RRegexp{Pattern: `\A[a-z]+\z`, Options: "i"})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equivalent Ruby regexps did not share the compiled regexp")
	}
}

func TestStringScannerRegexpSharesCompiledPattern(t *testing.T) {
	firstValue := &object.EmeraldValue{Type: object.ValueRegexp, Data: &object.RRegexp{Pattern: `(?<word>[a-z]+)`}}
	secondValue := &object.EmeraldValue{Type: object.ValueRegexp, Data: &object.RRegexp{Pattern: `(?<word>[a-z]+)`}}
	first, _, errVal := stringScannerRegexp(firstValue)
	if errVal != nil || first == nil {
		t.Fatalf("first StringScanner regexp compile failed: %v", errVal)
	}
	second, _, errVal := stringScannerRegexp(secondValue)
	if errVal != nil || second == nil {
		t.Fatalf("second StringScanner regexp compile failed: %v", errVal)
	}
	if first != second {
		t.Fatal("equivalent StringScanner regexps did not share the compiled regexp")
	}
}
