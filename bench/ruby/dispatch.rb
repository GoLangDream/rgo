def mix_value(x)
  ((x * 33) ^ (x >> 3)) & 2147483647
end

n = 20000
i = 0
sum = 0
while i < n
  sum = (sum + mix_value(i)) & 2147483647
  i += 1
end
puts sum
