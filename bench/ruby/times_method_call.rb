def transform(value)
  value * 3 + 1
end

sum = 0
1_000_000.times { |i| sum += transform(i) }
puts sum
