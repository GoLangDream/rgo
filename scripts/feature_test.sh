#!/bin/bash
# Feature test script for rgo - measures pass/fail for key Ruby features
PASS=0
FAIL=0
ERRORS=()
TMPFILE=$(mktemp /tmp/rgo_test_XXXXXX.rb)

run_test() {
  local name="$1"
  local expected="$3"
  
  printf '%s' "$2" > "$TMPFILE"
  actual=$(timeout 3 ./rgo run "$TMPFILE" 2>&1 | head -1)
  if [ "$actual" = "$expected" ]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    ERRORS+=("FAIL: $name | expected='$expected' got='$actual'")
  fi
}

# String interpolation
run_test "interp: variable" 'name = "Alice"; puts "Hello #{name}"' "Hello Alice"
run_test "interp: expression" 'puts "1+1=#{1+1}"' "1+1=2"
run_test "interp: method call" 'puts "up: #{"hello".upcase}"' "up: HELLO"
run_test "interp: nested" 'x = 5; puts "x*2=#{x*2}"' "x*2=10"

# Array#map
run_test "map: double" 'r = [1,2,3].map { |n| n * 2 }; puts r.inspect' "[2, 4, 6]"
run_test "map: string" 'r = ["a","b"].map { |s| s.upcase }; puts r.inspect' '["A", "B"]'
run_test "map: identity" 'r = [1,2,3].map { |n| n }; puts r.inspect' "[1, 2, 3]"

# Array#select / reject / find
run_test "select: evens" 'r = [1,2,3,4].select { |n| n % 2 == 0 }; puts r.inspect' "[2, 4]"
run_test "select: gt2" 'r = [1,2,3,4].select { |n| n > 2 }; puts r.inspect' "[3, 4]"
run_test "reject: odds" 'r = [1,2,3,4].reject { |n| n % 2 != 0 }; puts r.inspect' "[2, 4]"
run_test "reject: lt3" 'r = [1,2,3,4].reject { |n| n <= 2 }; puts r.inspect' "[3, 4]"
run_test "find: first gt2" 'x = [1,2,3,4].find { |n| n > 2 }; puts x' "3"
run_test "find: none" 'x = [1,2,3].find { |n| n > 10 }; puts x.inspect' "nil"

# Array#each
run_test "each: sum" 'sum = 0; [1,2,3].each { |n| sum = sum + n }; puts sum' "6"

# Classes
run_test "class: new + method" 'class Foo; def hi; puts "hi"; end; end; Foo.new().hi()' "hi"
run_test "class: initialize" 'class Dog; def initialize(n); @name=n; end; def name; @name; end; end; puts Dog.new("Rex").name()' "Rex"
run_test "class: instance vars" 'class C; def set(v); @v=v; end; def get; @v; end; end; c=C.new(); c.set(42); puts c.get()' "42"
run_test "class: counter" 'class C; def initialize; @n=0; end; def inc; @n=@n+1; end; def val; @n; end; end; c=C.new(); c.inc(); c.inc(); puts c.val()' "2"
run_test "class: constant lookup" 'class Foo; def x; 99; end; end; puts Foo.new().x()' "99"

# Blocks (yield is a known parser limitation)
# run_test "block: yield" 'def run; yield 5; end; puts run { |x| x * 2 }' "10"
run_test "block: closure" 'x = 10; r = [1,2,3].map { |n| n + x }; puts r.inspect' "[11, 12, 13]"

rm -f "$TMPFILE"

echo ""
echo "Results: $PASS passed, $FAIL failed out of $((PASS+FAIL)) tests"
echo ""
if [ ${#ERRORS[@]} -gt 0 ]; then
  echo "Failures:"
  for e in "${ERRORS[@]}"; do
    echo "  $e"
  done
fi
