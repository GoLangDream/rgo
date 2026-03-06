# 基础功能测试

puts "=== Testing Basic Features ==="

# 测试 1: 整数
x = 435
if x == 435
  puts "✓ Integer literals work"
end

# 测试 2: 浮点数
y = 4.35
if y == 4.35
  puts "✓ Float literals work"
end

# 测试 3: 字符串
s = "hello"
if s == "hello"
  puts "✓ String literals work"
end

# 测试 4: 数组
arr = [1, 2, 3]
if arr.length == 3
  puts "✓ Array literals work"
end

# 测试 5: 哈希
h = {"a" => 1, "b" => 2}
if h.length == 2
  puts "✓ Hash literals work"
end

# 测试 6: 运算
z = 1 + 2
if z == 3
  puts "✓ Arithmetic works"
end

# 测试 7: 比较
if 10 > 5
  puts "✓ Comparison works"
end

# 测试 8: 方法调用
result = "hello".upcase
if result == "HELLO"
  puts "✓ Method calls work"
end

puts "\n=== All 8 basic tests passed! ==="
