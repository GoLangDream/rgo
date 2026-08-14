n = 100000
sum = 0
n.times do |i|
  sum += i & 7
end
puts sum
