n = 10000
array = []
hash = {}
i = 0
while i < n
  value = (i * 17) % 1009
  array << value
  hash[i % 997] = value
  i += 1
end

i = 0
sum = 0
while i < array.length
  sum += array[i]
  i += 1
end
puts "#{array.length}:#{hash.length}:#{sum}"
