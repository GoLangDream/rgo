# RGo Demo - 展示当前支持的功能

# 基础运算
puts 2 + 3 * 4 # 14
puts 2**10 # 1024
puts 17 % 5 # 2

# 变量
x = 10
y = 20
puts x + y # 30

# 比较和逻辑
puts 5 > 3            # true
puts 5 >= 5           # true
puts 1 == 1           # true
puts 1 != 2           # true
puts false # false
puts true || false # true

# if/elsif/else
score = 85
if score >= 90
  puts 'A'
elsif score >= 80
  puts 'B'
elsif score >= 70
  puts 'C'
else
  puts 'F'
end

# while 循环
sum = 0
i = 1
while i <= 10
  sum += i
  i += 1
end
puts sum # 55

# until 循环
count = 0
count += 1 until count >= 5
puts count # 5

# 数组
arr = [1, 2, 3]
puts arr

# 字符串方法
puts 'hello'.upcase   # HELLO
puts 'WORLD'.downcase # world

# 数组方法
numbers = [1, 2, 3, 4, 5]
puts numbers.first    # 1
puts numbers.last     # 5
puts numbers.length   # 5
