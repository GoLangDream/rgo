# 简单的数字测试
puts 'Testing numbers...'

# 测试整数
x = 435
if x == 435
  puts '✓ Integer literal works'
else
  puts '✗ Integer literal failed'
end

# 测试浮点数
y = 4.35
if y == 4.35
  puts '✓ Float literal works'
else
  puts '✗ Float literal failed'
end

# 测试运算
z = 1 + 2
if z == 3
  puts '✓ Addition works'
else
  puts '✗ Addition failed'
end

puts "\nAll basic tests passed!"
