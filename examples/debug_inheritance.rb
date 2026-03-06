class Animal
  def speak
    puts "Animal speaks"
    "animal sound"
  end
end

class Dog < Animal
end

puts "Creating Dog..."
dog = Dog.new
puts "Dog created"

puts "Calling speak..."
result = dog.speak
puts "Result:"
puts result
