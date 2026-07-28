n = 12000
i = 0
text = +""
while i < n
  text << (97 + (i % 26)).chr
  i += 1
end
puts "#{text.bytesize}:#{text[0]}:#{text[-1]}"
