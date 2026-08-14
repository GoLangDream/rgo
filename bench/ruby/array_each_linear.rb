n = 1_000_000
values = Array.new(n) { |i| i }
values.each { |i| i * 3 + 1 }
puts values.length
