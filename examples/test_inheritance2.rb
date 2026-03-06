# 测试类继承和方法查找

class Animal
  def speak
    puts "Animal speaks"
  end
end

class Dog < Animal
  def bark
    puts "Woof!"
  end
end

puts "Creating a Dog instance..."
dog = Dog.new

puts "Calling dog.bark..."
dog.bark

puts "Calling dog.speak (inherited from Animal)..."
dog.speak

puts "\nInheritance test completed!"
