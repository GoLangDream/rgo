# RGo Critical Issues Fix Plan

## TL;DR
> **Summary**: Fix Array/Hash enumerable methods (map, select, etc.) to properly call blocks
> **Deliverables**: Working map, select, reject, find, each with block support
> **Effort**: Medium
> **Critical Path**: Fix block passing → Implement block calling in methods

## Analysis Summary

### Current State
- ✅ Parser supports `do...end` and `{ }` blocks
- ✅ Basic Ruby features work (variables, control flow, methods, classes)
- ✅ Block syntax parses correctly
- ❌ **CRITICAL**: Enumerable methods don't call blocks (stub implementations)
- ❌ String interpolation `#{}` not working
- ❌ Hash#each block parameter destructuring not working

### Test Results
```ruby
# Works
x = 10; puts x + y                    # ✅ Variables, operators
if x < y; puts "less"; end            # ✅ Control flow
def add(a,b); a+b; end; add(5,3)     # ✅ Methods
arr = [1,2,3]; arr.length             # ✅ Array basics
h = {a:1}; h[:a]                      # ✅ Hash basics
"hello".upcase                        # ✅ String methods

# Broken
[1,2,3].map { |n| n*2 }               # ❌ Returns [1,2,3] not [2,4,6]
[1,2,3].select { |n| n>1 }            # ❌ Returns [1,2,3] not [2,3]
"Hello #{name}"                       # ❌ Returns literal string
{a:1}.each { |k,v| puts k }           # ❌ Block params not destructured
```

### Root Causes

**1. Enumerable Methods Are Stubs**
- Location: `pkg/core/init.go` lines 1666-1690
- `arrayMap`: Just copies array, doesn't call block
- `arraySelect`: Just copies array, doesn't call block
- `arrayFind`: Hardcoded to find `1`, doesn't call block

**2. Block Not Passed to Methods**
- Methods receive `args` but block is not in args
- Need to check how blocks are passed in VM

**3. Missing Features**
- String interpolation parser support
- Hash iteration with key-value destructuring

## Priority Fixes

### P0 - Fix Enumerable Methods (CRITICAL)
**Impact**: Unblocks 40%+ of Array/Hash specs

Methods to fix:
- Array#map
- Array#select  
- Array#reject
- Array#find
- Array#each (already works but verify)
- Hash#select
- Hash#reject
- Hash#map

### P1 - String Interpolation
**Impact**: Unblocks 20%+ of String specs

Add parser support for `#{}` in strings.

### P2 - Hash Block Destructuring
**Impact**: Unblocks Hash iteration specs

Fix `{a:1}.each { |k,v| ... }` to properly destructure.

## Implementation Plan

### Task 1: Fix Array#map to call block
- Check how block is passed in `args`
- Call block for each element
- Collect results
- Return new array

### Task 2: Fix Array#select, reject, find
- Similar pattern to map
- select: keep if block returns truthy
- reject: keep if block returns falsy
- find: return first where block returns truthy

### Task 3: Fix Hash methods
- Hash#map, select, reject
- Handle key-value pairs

### Task 4: Add string interpolation
- Parser: recognize `#{}` in strings
- Compiler: emit code to evaluate and concatenate
- VM: execute interpolation

## Quick Test After Fixes

```ruby
# Should output [2, 4, 6]
puts [1,2,3].map { |n| n * 2 }.inspect

# Should output [2, 3]
puts [1,2,3].select { |n| n > 1 }.inspect

# Should output 2
puts [1,2,3].find { |n| n > 1 }

# Should output "Hello Alice"
name = "Alice"
puts "Hello #{name}"
```

## Expected Impact

After fixes:
- Array specs: 40% → 70%+
- Hash specs: 15% → 50%+
- String specs: 55% → 75%+
