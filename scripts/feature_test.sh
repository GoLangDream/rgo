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

# === String interpolation ===
run_test "interp: variable" 'name = "Alice"; puts "Hello #{name}"' "Hello Alice"
run_test "interp: expression" 'puts "1+1=#{1+1}"' "1+1=2"
run_test "interp: method call" 'puts "up: #{"hello".upcase}"' "up: HELLO"
run_test "interp: nested" 'x = 5; puts "x*2=#{x*2}"' "x*2=10"

# === Array#map ===
run_test "map: double" 'r = [1,2,3].map { |n| n * 2 }; puts r.inspect' "[2, 4, 6]"
run_test "map: string" 'r = ["a","b"].map { |s| s.upcase }; puts r.inspect' '["A", "B"]'
run_test "map: identity" 'r = [1,2,3].map { |n| n }; puts r.inspect' "[1, 2, 3]"

# === Array#select / reject / find ===
run_test "select: evens" 'r = [1,2,3,4].select { |n| n % 2 == 0 }; puts r.inspect' "[2, 4]"
run_test "select: gt2" 'r = [1,2,3,4].select { |n| n > 2 }; puts r.inspect' "[3, 4]"
run_test "reject: odds" 'r = [1,2,3,4].reject { |n| n % 2 != 0 }; puts r.inspect' "[2, 4]"
run_test "reject: lt3" 'r = [1,2,3,4].reject { |n| n <= 2 }; puts r.inspect' "[3, 4]"
run_test "find: first gt2" 'x = [1,2,3,4].find { |n| n > 2 }; puts x' "3"
run_test "find: none" 'x = [1,2,3].find { |n| n > 10 }; puts x.inspect' "nil"

# === Array#each ===
run_test "each: sum" 'sum = 0; [1,2,3].each { |n| sum = sum + n }; puts sum' "6"

# === Array#reduce ===
run_test "reduce: sum" 'puts [1,2,3,4].reduce(0) { |acc, n| acc + n }' "10"
run_test "reduce: no init" 'puts [1,2,3,4].reduce { |acc, n| acc + n }' "10"
run_test "inject: sum" 'puts [1,2,3].inject(0) { |acc, n| acc + n }' "6"

# === Array#flat_map ===
run_test "flat_map: double" 'r = [1,2,3].flat_map { |n| [n, n*2] }; puts r.inspect' "[1, 2, 2, 4, 3, 6]"

# === Array#each_with_object ===
run_test "each_with_obj" 'r = [1,2,3].each_with_object([]) { |n, arr| arr.push(n*2) }; puts r.inspect' "[2, 4, 6]"

# === Array#partition ===
run_test "partition" 'a, b = [1,2,3,4].partition { |n| n.even? }; puts a.inspect; puts b.inspect' "[2, 4]"

# === Array#take_while / drop_while ===
run_test "take_while" 'puts [1,2,3,4,5].take_while { |n| n < 4 }.inspect' "[1, 2, 3]"
run_test "drop_while" 'puts [1,2,3,4,5].drop_while { |n| n < 4 }.inspect' "[4, 5]"

# === Array#any? / all? / none? ===
run_test "any? true" 'puts [1,2,3].any? { |n| n > 2 }' "true"
run_test "any? false" 'puts [1,2,3].any? { |n| n > 5 }' "false"
run_test "all? true" 'puts [1,2,3].all? { |n| n > 0 }' "true"
run_test "all? false" 'puts [1,2,3].all? { |n| n > 1 }' "false"
run_test "none? true" 'puts [1,2,3].none? { |n| n > 5 }' "true"
run_test "none? false" 'puts [1,2,3].none? { |n| n > 2 }' "false"

# === Array#sort_by ===
run_test "sort_by" 'puts ["banana","apple","cherry"].sort_by { |s| s.length }.inspect' '["apple", "banana", "cherry"]'

# === Array#min_by / max_by ===
run_test "min_by" 'puts ["banana","apple","cherry"].min_by { |s| s.length }' "apple"
run_test "max_by" 'puts ["banana","apple","cherry"].max_by { |s| s.length }' "cherry"

# === Array#each_with_index ===
run_test "each_with_index" 'result = []; ["a","b","c"].each_with_index { |v, i| result.push("#{i}:#{v}") }; puts result.first' "0:a"

# === Array#sort ===
run_test "sort: numbers" 'puts [3,1,2].sort.inspect' "[1, 2, 3]"
run_test "sort: strings" 'puts ["c","a","b"].sort.inspect' '["a", "b", "c"]'

# === Array misc ===
run_test "flatten" 'puts [1,[2,[3,4]],5].flatten.inspect' "[1, 2, 3, 4, 5]"
run_test "uniq" 'puts [1,2,2,3,3].uniq.inspect' "[1, 2, 3]"
run_test "compact" 'puts [1,nil,2,nil,3].compact.inspect' "[1, 2, 3]"
run_test "zip" 'puts [1,2,3].zip([4,5,6]).inspect' "[[1, 4], [2, 5], [3, 6]]"
run_test "take" 'puts [1,2,3,4,5].take(3).inspect' "[1, 2, 3]"
run_test "drop" 'puts [1,2,3,4,5].drop(2).inspect' "[3, 4, 5]"
run_test "first" 'puts [1,2,3].first' "1"
run_test "last" 'puts [1,2,3].last' "3"
run_test "push + pop" 'a = [1,2]; a.push(3); puts a.pop' "3"
run_test "shift + unshift" 'a = [2,3]; a.unshift(1); puts a.shift' "1"
run_test "length" 'puts [1,2,3].length' "3"
run_test "include?" 'puts [1,2,3].include?(2)' "true"
run_test "count" 'puts [1,2,3,2].count' "4"
run_test "sum" 'puts [1,2,3].sum' "6"
run_test "min" 'puts [3,1,2].min' "1"
run_test "max" 'puts [3,1,2].max' "3"
run_test "reverse" 'puts [1,2,3].reverse.inspect' "[3, 2, 1]"
run_test "join" 'puts [1,2,3].join(",")' "1,2,3"
run_test "empty? true" 'puts [].empty?' "true"
run_test "empty? false" 'puts [1].empty?' "false"
run_test "index" 'puts [1,2,3].index(2)' "1"

# === String methods ===
run_test "str upcase" 'puts "hello".upcase' "HELLO"
run_test "str downcase" 'puts "HELLO".downcase' "hello"
run_test "str strip" 'puts "  hi  ".strip' "hi"
run_test "str length" 'puts "hello".length' "5"
run_test "str reverse" 'puts "hello".reverse' "olleh"
run_test "str include?" 'puts "hello".include?("ell")' "true"
run_test "str start_with?" 'puts "hello".start_with?("hel")' "true"
run_test "str end_with?" 'puts "hello".end_with?("llo")' "true"
run_test "str to_i" 'puts "42".to_i' "42"
run_test "str to_f" 'puts "3.14".to_f' "3.14"
run_test "str capitalize" 'puts "hello world".capitalize' "Hello world"
run_test "str split" 'puts "a,b,c".split(",").inspect' '["a", "b", "c"]'
run_test "str gsub" 'puts "hello world".gsub("l", "r")' "herro worrd"
run_test "str sub" 'puts "hello world".sub("l", "r")' "herlo world"
run_test "str chomp" 'puts "hello\n".chomp' "hello"
run_test "str chop" 'puts "hello".chop' "hell"
run_test "str chars" 'puts "abc".chars.inspect' '["a", "b", "c"]'
run_test "str bytes first" 'puts "abc".bytes.first' "97"
run_test "str * repetition" 'puts "ab" * 3' "ababab"
run_test "str [] index" 'puts "hello"[1]' "e"
run_test "str [] slice" 'puts "hello"[1,3]' "ell"
run_test "str concat" 'puts "hello" + " world"' "hello world"
run_test "str empty? true" 'puts "".empty?' "true"
run_test "str empty? false" 'puts "a".empty?' "false"
run_test "str lstrip" 'puts "  hi".lstrip' "hi"
run_test "str rstrip" 'puts "hi  ".rstrip' "hi"
run_test "str swapcase" 'puts "Hello".swapcase' "hELLO"
run_test "str count chars" 'puts "hello".count("l")' "2"
run_test "str delete chars" 'puts "hello".delete("l")' "heo"

# === Hash methods ===
run_test "hash index" 'h = {a: 1, b: 2}; puts h[:a]' "1"
run_test "hash set" 'h = {}; h[:x] = 42; puts h[:x]' "42"
run_test "hash keys" 'h = {a: 1, b: 2}; puts h.keys.length' "2"
run_test "hash values" 'h = {a: 1, b: 2}; puts h.values.sort.inspect' "[1, 2]"
run_test "hash each" 'sum = 0; {a: 1, b: 2}.each { |k, v| sum = sum + v }; puts sum' "3"
run_test "hash merge" 'h = {a: 1}.merge({b: 2}); puts h.length' "2"
run_test "hash delete" 'h = {a: 1, b: 2}; h.delete(:a); puts h.length' "1"
run_test "hash has_key?" 'puts({a: 1}.key?(:a))' "true"
run_test "hash empty? true" 'puts({}.empty?)' "true"
run_test "hash select" 'h = {a: 1, b: 2, c: 3}; r = h.select { |k, v| v > 1 }; puts r.length' "2"
run_test "hash reject" 'h = {a: 1, b: 2, c: 3}; r = h.reject { |k, v| v > 1 }; puts r.length' "1"
run_test "hash length" 'puts({a: 1, b: 2}.length)' "2"
run_test "hash to_a" 'h = {a: 1}; puts h.to_a.length' "1"
run_test "hash fetch" 'puts({a: 42}.fetch(:a))' "42"

# === Integer methods ===
run_test "int times" 'n = 0; 3.times { n = n + 1 }; puts n' "3"
run_test "int upto" 'n = 0; 1.upto(5) { |i| n = n + i }; puts n' "15"
run_test "int downto" 'n = 0; 5.downto(1) { |i| n = n + i }; puts n' "15"
run_test "int odd?" 'puts 3.odd?' "true"
run_test "int even?" 'puts 4.even?' "true"
run_test "int abs" 'puts(-5.abs)' "5"
run_test "int to_s" 'puts 42.to_s' "42"
run_test "int to_f" 'puts 3.to_f' "3.0"
run_test "int zero?" 'puts 0.zero?' "true"
run_test "int positive?" 'puts 5.positive?' "true"
run_test "int negative?" 'puts(-3.negative?)' "true"
run_test "int succ" 'puts 4.succ' "5"
run_test "int pred" 'puts 4.pred' "3"
run_test "int gcd" 'puts 12.gcd(8)' "4"
run_test "int lcm" 'puts 4.lcm(6)' "12"
run_test "int divmod" 'puts 10.divmod(3).inspect' "[3, 1]"
run_test "int pow" 'puts 2 ** 8' "256"
run_test "int bit_and" 'puts(12 & 10)' "8"
run_test "int bit_or" 'puts(12 | 10)' "14"
run_test "int bit_xor" 'puts(12 ^ 10)' "6"
run_test "int left_shift" 'puts(1 << 3)' "8"
run_test "int right_shift" 'puts(8 >> 2)' "2"
run_test "int chr" 'puts 65.chr' "A"
run_test "int floor" 'puts 5.floor' "5"
run_test "int ceil" 'puts 5.ceil' "5"
run_test "int round" 'puts 5.round' "5"
run_test "int digits" 'puts 123.digits.inspect' "[3, 2, 1]"

# === Float methods ===
run_test "float floor" 'puts 3.7.floor' "3"
run_test "float ceil" 'puts 3.2.ceil' "4"
run_test "float round" 'puts 3.5.round' "4"
run_test "float abs" 'puts(-1.5.abs)' "1.5"
run_test "float to_i" 'puts 3.9.to_i' "3"
run_test "float to_f" 'puts 3.0.to_f' "3.0"
run_test "float zero?" 'puts 0.0.zero?' "true"
run_test "float positive?" 'puts 1.5.positive?' "true"
run_test "float negative?" 'puts(-1.5.negative?)' "true"
run_test "float nan?" 'puts((0.0 / 0.0).nan?)' "true"
run_test "float infinite? nil" 'puts 1.0.infinite?.inspect' "nil"
run_test "float infinite? pos" 'puts((1.0 / 0.0).infinite?)' "1"
run_test "float finite? true" 'puts 1.5.finite?' "true"

# === Control flow ===
run_test "if/else" 'x = 5; if x > 3; puts "big"; else; puts "small"; end' "big"
run_test "while" 'i = 0; while i < 3; i = i + 1; end; puts i' "3"
run_test "until" 'i = 0; until i >= 3; i = i + 1; end; puts i' "3"
run_test "case/when match" 'x = 2; case x; when 1; puts "one"; when 2; puts "two"; else; puts "other"; end' "two"
run_test "case/when else" 'x = 5; case x; when 1; puts "one"; else; puts "other"; end' "other"
run_test "ternary" 'puts(3 > 2 ? "yes" : "no")' "yes"

# === Classes ===
run_test "class: new + method" 'class Foo; def hi; puts "hi"; end; end; Foo.new().hi()' "hi"
run_test "class: initialize" 'class Dog; def initialize(n); @name=n; end; def name; @name; end; end; d = Dog.new("Rex"); puts d.name()' "Rex"
run_test "class: instance vars" 'class C; def set(v); @v=v; end; def get; @v; end; end; c=C.new(); c.set(42); puts c.get()' "42"
run_test "class: counter" 'class C; def initialize; @n=0; end; def inc; @n=@n+1; end; def val; @n; end; end; c=C.new(); c.inc(); c.inc(); puts c.val()' "2"
run_test "class: constant" 'class Foo; def x; 99; end; end; puts Foo.new().x()' "99"

# === Blocks / yield ===
run_test "block: closure" 'x = 10; r = [1,2,3].map { |n| n + x }; puts r.inspect' "[11, 12, 13]"
run_test "yield basic" 'def run; yield 5; end; puts run { |x| x * 2 }' "10"
run_test "block_given?" 'def foo; if block_given?; yield; else; "no block"; end; end; puts foo { "block!" }' "block!"

# === Exception handling ===
run_test "raise rescue" 'begin; raise "err"; rescue => e; puts e.message; end' "err"
run_test "ensure" 'x = 0; begin; raise "e"; rescue; x = 1; ensure; x = x + 10; end; puts x' "11"

# === Kernel methods ===
run_test "puts nil" 'puts nil.inspect' "nil"
run_test "p integer" 'p 42' "42"
run_test "loop break" 'i = 0; loop { i = i + 1; break if i == 3 }; puts i' "3"

# === Type checks ===
run_test "is_a? Integer" 'puts 42.is_a?(Integer)' "true"
run_test "is_a? String" 'puts "hi".is_a?(String)' "true"
run_test "is_a? Array" 'puts [].is_a?(Array)' "true"
run_test "nil?" 'puts nil.nil?' "true"
run_test "respond_to?" 'puts "hi".respond_to?(:upcase)' "true"

# === Comparable / Range ===
run_test "spaceship" 'puts(1 <=> 2)' "-1"
run_test "range each" 'sum = 0; (1..5).each { |i| sum = sum + i }; puts sum' "15"
run_test "range to_a" 'puts (1..5).to_a.inspect' "[1, 2, 3, 4, 5]"
run_test "range include?" 'puts (1..5).include?(3)' "true"

# === Multiple assignment ===
run_test "multi assign" 'a, b = 1, 2; puts a; puts b' "1"
run_test "swap" 'a = 1; b = 2; a, b = b, a; puts a; puts b' "2"

# === Symbol ===
run_test "symbol to_s" 'puts :hello.to_s' "hello"
run_test "symbol ==" 'puts(:foo == :foo)' "true"

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
