def foo
  yield
end

foo do
  42
end
