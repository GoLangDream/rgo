start = 0
finish = 50_000_000
sum = 0
start.upto(finish) do |i|
  sum = sum + i * 3 + 1
end
puts sum
