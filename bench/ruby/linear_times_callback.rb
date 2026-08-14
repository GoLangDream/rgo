n = ARGV.empty? ? 50_000_000 : ARGV[0].to_i
n.times do |i|
  i * 3 + 1
end
puts n
