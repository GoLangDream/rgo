n = 5_000_000
values = Array.new(n, 1)
values.each { |i| i * 3 + 1 }
puts values.length
