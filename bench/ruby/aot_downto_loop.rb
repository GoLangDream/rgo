start = 50_000_000
finish = 0
sum = 0
start.downto(finish) do |i|
  sum = sum + i * 3 + 1
end
puts sum
