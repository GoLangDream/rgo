class Test
  def foo
    42
  end
end

t = Test.new
if t.respond_to?("foo")
  puts "Method foo exists"
else
  puts "Method foo does NOT exist"
end
