i = 0
total = 0
while i < 50_000_000
  total = total + i * 3 + 1
  i = i + 1
end
puts total
