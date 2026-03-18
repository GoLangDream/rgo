# Block Passing Implementation Learnings

## Block Passing Implementation (2026-03-19)

### Key Changes

1. **Compiler (pkg/compiler/compiler.go)**
   - Modified `BlockExpression` case to compile blocks with params as closures
   - Blocks with params now create a new scope, define params as locals, and emit OpClosure
   - Used `compileBlockAsValue` to keep last expression value on stack (removes OpPop)
   - Blocks without params still compile inline (for if/while bodies)

2. **VM Executor (pkg/vm/executor.go)**
   - OpSend handler now pops block from stack when blockArg=1
   - Stores block in vm.currentBlock before calling method
   - Restores previous block after method call (for nested blocks)
   - CallBlock and callBlock methods already implemented

3. **Core Methods (pkg/core/init.go)**
   - arrayMap, arraySelect, arrayReject, arrayFind already call CallBlock
   - No changes needed - they were waiting for block passing to work

### How It Works

1. Parser creates BlockExpression with Params
2. Compiler compiles block as closure (OpClosure instruction)
3. OpSend pops closure from stack and stores in vm.currentBlock
4. Array methods call CallBlock(elem) which invokes vm.callBlock
5. callBlock executes closure with args, returns result

### Verified Working

- `[1,2,3].map { |n| n * 2 }` => `[2,4,6]`
- `[1,2,3,4,5].select { |n| n > 2 }` => `[3,4,5]`
- `[1,2,3,4,5].reject { |n| n > 2 }` => `[1,2]`
- `[1,2,3,4,5].find { |n| n > 3 }` => `4`
- Chaining: `.map { }.each { }` works correctly

### Critical Insight

The key was distinguishing between:
- **Blocks with params** (method arguments) → compile as closures
- **Blocks without params** (if/while bodies) → compile inline

Using `compileBlockAsValue` ensures the last expression value stays on stack for return.
