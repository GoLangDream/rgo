class Test
  def foo
    puts "In foo"
    42
  end
end

puts "Creating Test instance"
t = Test.new
puts "Calling foo"
result = t.foo
puts "Result:"
p result
