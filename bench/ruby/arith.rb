n = 25000
i = 0
x = 1
while i < n
  x = (x * 1664525 + 1013904223) % 2147483647
  i += 1
end
puts x
