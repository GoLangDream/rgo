# Implement Block Passing in RGo VM

## Summary
Fix `[1,2,3].map { |n| n * 2 }` to return `[2, 4, 6]` instead of `[1, 2, 3]`.

## Root Causes
1. VM's OpSend ignores blockArg (`_ = blockArg` at line 443)
2. Array#map, select, reject, find are stubs that don't call blocks

## Implementation

### Step 1: Add currentBlock field to VM
File: `pkg/vm/executor.go`
Add to VM struct:
```go
currentBlock *object.EmeraldValue
```

### Step 2: Fix OpSend to store block
In the OpSend case, replace `_ = blockArg` with code to store the block:
```go
var block *object.EmeraldValue
if blockArg == 1 {
    block = vm.pop()
}
vm.currentBlock = block
```

### Step 3: Add CallBlock helper
Add package-level function in executor.go:
```go
var CallBlock func(...*object.EmeraldValue) *object.EmeraldValue

func init() {
    CallBlock = callBlock
}

func callBlock(args ...*object.EmeraldValue) *object.EmeraldValue {
    return R.NilVal  // placeholder - will be replaced by VM
}
```

Actually simpler: add a method to VM that core can call via a package variable.

### Step 4: Fix arrayMap
In `pkg/core/init.go`, rewrite arrayMap to call the block:
```go
func arrayMap(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
    arr := receiver.Data.([]*object.EmeraldValue)
    result := make([]*object.EmeraldValue, len(arr))
    for i, elem := range arr {
        // Call the block with elem as argument
        // Need to use VM's callBlock mechanism
        result[i] = elem  // TEMP: still stub
    }
    return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: R.Classes["Array"]}
}
```

### Step 5: Fix arraySelect, arrayReject, arrayFind
Similar pattern - call block for each element.

## Test Cases
```ruby
[1,2,3].map { |n| n * 2 }  # Should be [2,4,6]
[1,2,3].select { |n| n > 1 }  # Should be [2,3]
[1,2,3].find { |n| n > 1 }     # Should be 2
```

## Files to Modify
- pkg/vm/executor.go (add currentBlock, fix OpSend, add CallBlock)
- pkg/core/init.go (fix arrayMap, arraySelect, arrayReject, arrayFind)

## Acceptance Criteria
- [ ] `[1,2,3].map { |n| n * 2 }` returns [2,4,6]
- [ ] `[1,2,3].select { |n| n > 1 }` returns [2,3]
- [ ] `[1,2,3].find { |n| n > 1 }` returns 2
- [ ] All existing tests pass
