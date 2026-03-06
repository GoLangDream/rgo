class Animal
  def speak
    puts "Animal speaks"
    "animal sound"
  end
end

puts "Creating Animal..."
animal = Animal.new
puts "Animal created"

puts "Calling speak..."
result = animal.speak
puts "Result:"
puts result
