class Test
  def foo
    puts "In foo"
    42
  end
end

puts "Before new"
t = Test.new
puts "After new"
puts "Before call"
result = t.foo
puts "After call"
puts result
