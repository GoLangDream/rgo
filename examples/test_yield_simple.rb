def foo
  yield
end

foo { 42 }
