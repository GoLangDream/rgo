## Task 13: Proc and Lambda Classes Implementation

### What was implemented:
1. Added Proc class to `pkg/core/init.go`:
   - Created procClass in createClasses()
   - Added to R.Classes["Proc"]
   
2. Implemented Proc methods in defineMethods():
   - call: Placeholder for future VM integration
   - []: Alias for call
   - arity: Returns number of parameters (uses len(Fn.Params))
   - lambda?: Returns true if IsLambda flag is set

3. Updated VM executor (`pkg/vm/executor.go`):
   - OpClosure: Now creates closures with Proc class
   - OpLambda: Creates Proc struct with IsLambda=true flag

4. Updated test to include Proc in expected classes

### Key patterns discovered:
- Proc struct already existed in pkg/object/value.go with IsLambda field
- Closure struct is similar but doesn't have IsLambda flag
- Function struct uses Params array, not Arity field
- Methods use BuiltinMethod signature: func(receiver, args...) *EmeraldValue

### Limitations:
- procCall() is a placeholder - actual block calling requires deeper VM integration
- Parser doesn't support Proc.new syntax yet (Task 14 will handle this)
- Block/proc conversion needs more work in compiler/parser

### Files modified:
- pkg/core/init.go: Added Proc class and methods
- pkg/vm/executor.go: Updated OpClosure and OpLambda
- pkg/core/core_test.go: Added Proc to expected classes
