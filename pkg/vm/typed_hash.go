package vm

import (
	"os"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

// hashIntegerReduceLinearEnabled controls only the lazy-linear Hash read
// tier.  Keeping a narrow switch makes benchmark bisects compare the same
// producer and reducer setup while isolating this optimization.
var hashIntegerReduceLinearEnabled = os.Getenv("RGO_DISABLE_HASH_INTEGER_LINEAR_REDUCE") == ""

// hashIntegerMapLazyResultEnabled defers boxing the result of a proven
// affine Hash#map until the returned Array is observed.  The raw result
// backing is a snapshot, so later Hash mutation or method redefinition cannot
// change the already-completed map call.
var hashIntegerMapLazyResultEnabled = os.Getenv("RGO_DISABLE_HASH_INTEGER_MAP_LAZY_RESULT") == ""

// A proven affine Hash#map already performs the complete raw-int64 preflight
// before returning.  Keeping its Array result lazy on the first call avoids
// boxing values that the caller may never observe (the common hot benchmark
// only indexes one element).
const hashIntegerMapLazyResultHotThreshold uint8 = 1

const (
	hashIntegerExprInvalid uint8 = iota
	hashIntegerExprParam
	hashIntegerExprFree
	hashIntegerExprBinary
	hashIntegerExprLiteral
)

const hashIntegerExprInvalidNode = ^uint8(0)

type hashIntegerExprNode struct {
	kind        uint8
	param       uint8
	free        uint8
	opcode      compiler.Opcode
	left, right uint8
	literal     int64
}

type hashIntegerReducePlan struct {
	nodes      [64]hashIntegerExprNode
	nodeCount  uint8
	root       uint8
	freeIndex  uint8
	direct     bool
	outerOp    compiler.Opcode
	innerOp    compiler.Opcode
	innerLeft  uint8
	innerRight uint8
}

// compileHashIntegerReducePlan recognizes the side-effect-free accumulator
// region used by Hash#each, for example:
//
//	total += key + value
//
// Register IR may reuse a destination register for a binary operation. The
// matcher therefore copies each expression into an immutable node instead of
// retaining register aliases; this keeps the proof valid across compiler
// register allocation changes.
func compileHashIntegerReducePlan(fn *object.Function, plan *registerIRPlan) (hashIntegerReducePlan, bool) {
	invalid := hashIntegerReducePlan{root: hashIntegerExprInvalidNode, freeIndex: hashIntegerExprInvalidNode}
	if fn == nil || plan == nil || !plan.blockReturn || plan.hasBranches || plan.hasImplicitSends ||
		plan.hasExplicitReturn || plan.sendCount != 0 || len(fn.Params) != 2 ||
		len(fn.ParamLocalIndices) != 2 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		!simpleBlockParameterPatterns(fn) || fn.NumLocals > 64 {
		return invalid, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return invalid, false
		}
	}

	result := invalid
	result.root = hashIntegerExprInvalidNode
	result.freeIndex = hashIntegerExprInvalidNode
	registerNodes := [16]uint8{}
	for index := range registerNodes {
		registerNodes[index] = hashIntegerExprInvalidNode
	}
	localNodes := [64]uint8{}
	for index := range localNodes {
		localNodes[index] = hashIntegerExprInvalidNode
	}
	for parameter, local := range fn.ParamLocalIndices {
		if parameter >= 2 || local < 0 || local >= len(localNodes) {
			return invalid, false
		}
		// A parameter may be loaded directly or through its local slot. The
		// local mapping is filled when the corresponding LoadParam is seen.
	}
	addNode := func(node hashIntegerExprNode) (uint8, bool) {
		if result.nodeCount >= uint8(len(result.nodes)) {
			return hashIntegerExprInvalidNode, false
		}
		index := result.nodeCount
		result.nodes[index] = node
		result.nodeCount++
		return index, true
	}
	var storedFreeIndex uint8 = hashIntegerExprInvalidNode
	var storedNode uint8 = hashIntegerExprInvalidNode
	var returnNode uint8 = hashIntegerExprInvalidNode
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param >= 2 || instruction.dst >= uint8(len(registerNodes)) {
				return invalid, false
			}
			node, ok := addNode(hashIntegerExprNode{kind: hashIntegerExprParam, param: instruction.param})
			if !ok {
				return invalid, false
			}
			registerNodes[instruction.dst] = node
			local := fn.ParamLocalIndices[instruction.param]
			if local < 0 || local >= len(localNodes) {
				return invalid, false
			}
			localNodes[local] = node
		case registerIRLoadLocal:
			if instruction.dst >= uint8(len(registerNodes)) || instruction.param >= uint8(len(localNodes)) ||
				localNodes[instruction.param] == hashIntegerExprInvalidNode {
				return invalid, false
			}
			registerNodes[instruction.dst] = localNodes[instruction.param]
		case registerIRStoreLocal:
			if instruction.param >= uint8(len(localNodes)) || instruction.left >= uint8(len(registerNodes)) ||
				registerNodes[instruction.left] == hashIntegerExprInvalidNode {
				return invalid, false
			}
			localNodes[instruction.param] = registerNodes[instruction.left]
		case registerIRLoadFree:
			if instruction.dst >= uint8(len(registerNodes)) {
				return invalid, false
			}
			if result.freeIndex == hashIntegerExprInvalidNode {
				result.freeIndex = instruction.param
			} else if result.freeIndex != instruction.param {
				return invalid, false
			}
			node, ok := addNode(hashIntegerExprNode{kind: hashIntegerExprFree, free: instruction.param})
			if !ok {
				return invalid, false
			}
			registerNodes[instruction.dst] = node
		case registerIRBinary:
			if instruction.dst >= uint8(len(registerNodes)) || instruction.left >= uint8(len(registerNodes)) ||
				instruction.right >= uint8(len(registerNodes)) || registerNodes[instruction.left] == hashIntegerExprInvalidNode ||
				registerNodes[instruction.right] == hashIntegerExprInvalidNode ||
				instruction.opcode != compiler.OpAdd && instruction.opcode != compiler.OpSub {
				return invalid, false
			}
			node, ok := addNode(hashIntegerExprNode{
				kind: hashIntegerExprBinary, opcode: instruction.opcode,
				left: registerNodes[instruction.left], right: registerNodes[instruction.right],
			})
			if !ok {
				return invalid, false
			}
			registerNodes[instruction.dst] = node
		case registerIRStoreFree:
			if instruction.left >= uint8(len(registerNodes)) || registerNodes[instruction.left] == hashIntegerExprInvalidNode ||
				storedFreeIndex != hashIntegerExprInvalidNode {
				return invalid, false
			}
			storedFreeIndex = instruction.param
			storedNode = registerNodes[instruction.left]
		case registerIRReturn:
			if instruction.left >= uint8(len(registerNodes)) || registerNodes[instruction.left] == hashIntegerExprInvalidNode ||
				returnNode != hashIntegerExprInvalidNode {
				return invalid, false
			}
			returnNode = registerNodes[instruction.left]
		default:
			return invalid, false
		}
	}
	if result.freeIndex == hashIntegerExprInvalidNode || storedFreeIndex != result.freeIndex ||
		storedNode == hashIntegerExprInvalidNode || storedNode != returnNode || returnNode == hashIntegerExprInvalidNode ||
		!hashIntegerExprContainsFree(&result.nodes, returnNode, result.freeIndex) {
		return invalid, false
	}
	result.root = returnNode
	rootNode := result.nodes[returnNode]
	if rootNode.kind == hashIntegerExprBinary && rootNode.left < result.nodeCount && rootNode.right < result.nodeCount {
		leftNode := result.nodes[rootNode.left]
		rightNode := result.nodes[rootNode.right]
		if leftNode.kind == hashIntegerExprFree && rightNode.kind == hashIntegerExprBinary &&
			rightNode.left < result.nodeCount && rightNode.right < result.nodeCount {
			innerLeft := result.nodes[rightNode.left]
			innerRight := result.nodes[rightNode.right]
			if innerLeft.kind == hashIntegerExprParam && innerRight.kind == hashIntegerExprParam &&
				(innerLeft.param == 0 || innerLeft.param == 1) &&
				(innerRight.param == 0 || innerRight.param == 1) && innerLeft.param != innerRight.param {
				result.direct = true
				result.outerOp = rootNode.opcode
				result.innerOp = rightNode.opcode
				result.innerLeft = innerLeft.param
				result.innerRight = innerRight.param
			}
		}
	}
	return result, true
}

func hashIntegerExprContainsFree(nodes *[64]hashIntegerExprNode, index, freeIndex uint8) bool {
	if nodes == nil || index == hashIntegerExprInvalidNode || index >= uint8(len(nodes)) {
		return false
	}
	node := nodes[index]
	switch node.kind {
	case hashIntegerExprFree:
		return node.free == freeIndex
	case hashIntegerExprBinary:
		return hashIntegerExprContainsFree(nodes, node.left, freeIndex) ||
			hashIntegerExprContainsFree(nodes, node.right, freeIndex)
	default:
		return false
	}
}

func hashIntegerExprOpsAvailable(vm *VM, nodes *[64]hashIntegerExprNode, index uint8) bool {
	if vm == nil || nodes == nil || index == hashIntegerExprInvalidNode || index >= uint8(len(nodes)) {
		return false
	}
	node := nodes[index]
	if node.kind != hashIntegerExprBinary {
		return true
	}
	return vm.fusedIntegerOperationAvailable(node.opcode) &&
		hashIntegerExprOpsAvailable(vm, nodes, node.left) &&
		hashIntegerExprOpsAvailable(vm, nodes, node.right)
}

func evalHashIntegerExpr(nodes *[64]hashIntegerExprNode, index uint8, total, key, value int64) (int64, bool) {
	if nodes == nil || index == hashIntegerExprInvalidNode || index >= uint8(len(nodes)) {
		return 0, false
	}
	node := nodes[index]
	switch node.kind {
	case hashIntegerExprParam:
		switch node.param {
		case 0:
			return key, true
		case 1:
			return value, true
		default:
			return 0, false
		}
	case hashIntegerExprFree:
		return total, true
	case hashIntegerExprLiteral:
		return node.literal, true
	case hashIntegerExprBinary:
		left, ok := evalHashIntegerExpr(nodes, node.left, total, key, value)
		if !ok {
			return 0, false
		}
		right, ok := evalHashIntegerExpr(nodes, node.right, total, key, value)
		if !ok {
			return 0, false
		}
		return applyRegisterIRIntegerLinearOpRaw(node.opcode, left, right)
	default:
		return 0, false
	}
}

func evalHashIntegerReducePlan(plan *hashIntegerReducePlan, total, key, value int64) (int64, bool) {
	if plan == nil || !plan.direct {
		return 0, false
	}
	left, right := key, value
	if plan.innerLeft == 1 {
		left, right = value, key
	}
	inner, ok := applyRegisterIRIntegerLinearOpRaw(plan.innerOp, left, right)
	if !ok {
		return 0, false
	}
	return applyRegisterIRIntegerLinearOpRaw(plan.outerOp, total, inner)
}

// evalHashIntegerMapExpr evaluates the already topologically copied map
// graph with a fixed stack array.  Hash#map has no captured accumulator, so
// this avoids the recursive evaluator used by Hash#each and keeps the common
// `key + value` case to one small node loop plus checked arithmetic.
func evalHashIntegerMapExpr(nodes *[64]hashIntegerExprNode, nodeCount, root uint8, key, value int64) (int64, bool) {
	if nodes == nil || nodeCount == 0 || nodeCount > uint8(len(nodes)) || root >= nodeCount {
		return 0, false
	}
	var values [64]int64
	for index := uint8(0); index < nodeCount; index++ {
		node := nodes[index]
		switch node.kind {
		case hashIntegerExprParam:
			switch node.param {
			case 0:
				values[index] = key
			case 1:
				values[index] = value
			default:
				return 0, false
			}
		case hashIntegerExprLiteral:
			values[index] = node.literal
		case hashIntegerExprBinary:
			if node.left >= index || node.right >= index {
				return 0, false
			}
			result, ok := applyRegisterIRIntegerLinearOpRaw(node.opcode, values[node.left], values[node.right])
			if !ok {
				return 0, false
			}
			values[index] = result
		default:
			return 0, false
		}
	}
	return values[root], true
}

type hashIntegerMapPlan struct {
	nodes     [64]hashIntegerExprNode
	nodeCount uint8
	root      uint8
	direct    bool
	directOp  compiler.Opcode
}

type hashIntegerMapLazyPayload struct {
	region object.RHashLinearRegion
	plan   hashIntegerMapPlan
	length int
}

func (vm *VM) hashIntegerMapLazyResultHot(fn *object.Function) bool {
	if vm == nil || fn == nil {
		return false
	}
	if vm.hashIntegerMapLazyCalls == nil {
		vm.hashIntegerMapLazyCalls = make(map[*object.Function]uint8)
	}
	count := vm.hashIntegerMapLazyCalls[fn]
	if count < hashIntegerMapLazyResultHotThreshold {
		count++
		vm.hashIntegerMapLazyCalls[fn] = count
	}
	return count >= hashIntegerMapLazyResultHotThreshold
}

func (payload *hashIntegerMapLazyPayload) materialize() []*object.EmeraldValue {
	if payload == nil {
		return nil
	}
	values := make([]*object.EmeraldValue, payload.length)
	for index := range values {
		if value, ok := payload.elementAt(index); ok {
			values[index] = value
		} else {
			values[index] = core.R.NilVal
		}
	}
	return values
}

func (payload *hashIntegerMapLazyPayload) elementAt(index int) (*object.EmeraldValue, bool) {
	if payload == nil || index < 0 || index >= payload.length {
		return nil, false
	}
	key, value, ok := core.DirectHashLinearPairAt(&payload.region, index)
	if !ok {
		return nil, false
	}
	result, ok := evalHashIntegerMapPlan(&payload.plan, key, value)
	if !ok {
		return nil, false
	}
	return core.NewSmallIntegerValue(result), true
}

// compileHashIntegerMapPlan recognizes a side-effect-free two-argument Hash
// map expression such as `|key, value| key + value`.  It deliberately admits
// only Register IR loads, local aliases, integer literals, Add/Sub and the
// final block return.  Any send, capture, branch, assignment with observable
// effects, or non-integer literal stays on the ordinary Ruby path.
func compileHashIntegerMapPlan(fn *object.Function, plan *registerIRPlan) (hashIntegerMapPlan, bool) {
	invalid := hashIntegerMapPlan{root: hashIntegerExprInvalidNode}
	if fn == nil || plan == nil || !plan.blockReturn || plan.hasBranches || plan.hasImplicitSends ||
		plan.hasExplicitReturn || plan.sendCount != 0 || len(fn.Params) != 2 ||
		len(fn.ParamLocalIndices) != 2 || fn.HasRestParam || fn.HasBlockParam ||
		len(fn.KeywordParams) != 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		!simpleBlockParameterPatterns(fn) || fn.NumLocals > 64 {
		return invalid, false
	}
	for _, defaultValue := range fn.ParamDefaults {
		if defaultValue != nil {
			return invalid, false
		}
	}

	result := invalid
	registerNodes := [16]uint8{}
	for index := range registerNodes {
		registerNodes[index] = hashIntegerExprInvalidNode
	}
	localNodes := [64]uint8{}
	for index := range localNodes {
		localNodes[index] = hashIntegerExprInvalidNode
	}
	addNode := func(node hashIntegerExprNode) (uint8, bool) {
		if result.nodeCount >= uint8(len(result.nodes)) {
			return hashIntegerExprInvalidNode, false
		}
		index := result.nodeCount
		result.nodes[index] = node
		result.nodeCount++
		return index, true
	}
	returnNode := hashIntegerExprInvalidNode
	for _, instruction := range plan.instructions {
		switch instruction.op {
		case registerIRLoadParam:
			if instruction.param >= 2 || instruction.dst >= uint8(len(registerNodes)) {
				return invalid, false
			}
			node, ok := addNode(hashIntegerExprNode{kind: hashIntegerExprParam, param: instruction.param})
			if !ok {
				return invalid, false
			}
			registerNodes[instruction.dst] = node
			local := fn.ParamLocalIndices[instruction.param]
			if local < 0 || local >= len(localNodes) {
				return invalid, false
			}
			localNodes[local] = node
		case registerIRLoadLocal:
			if instruction.dst >= uint8(len(registerNodes)) || instruction.param >= uint8(len(localNodes)) ||
				localNodes[instruction.param] == hashIntegerExprInvalidNode {
				return invalid, false
			}
			registerNodes[instruction.dst] = localNodes[instruction.param]
		case registerIRStoreLocal:
			if instruction.param >= uint8(len(localNodes)) || instruction.left >= uint8(len(registerNodes)) ||
				registerNodes[instruction.left] == hashIntegerExprInvalidNode {
				return invalid, false
			}
			localNodes[instruction.param] = registerNodes[instruction.left]
		case registerIRLoadLiteral:
			if instruction.dst >= uint8(len(registerNodes)) || !typedIntegerArgument(instruction.value) ||
				core.AttachedSingletonClass(instruction.value) != nil {
				return invalid, false
			}
			node, ok := addNode(hashIntegerExprNode{kind: hashIntegerExprLiteral, literal: instruction.value.Data.(int64)})
			if !ok {
				return invalid, false
			}
			registerNodes[instruction.dst] = node
		case registerIRMove:
			if instruction.dst >= uint8(len(registerNodes)) || instruction.left >= uint8(len(registerNodes)) ||
				registerNodes[instruction.left] == hashIntegerExprInvalidNode {
				return invalid, false
			}
			registerNodes[instruction.dst] = registerNodes[instruction.left]
		case registerIRBinary:
			if instruction.dst >= uint8(len(registerNodes)) || instruction.left >= uint8(len(registerNodes)) ||
				instruction.right >= uint8(len(registerNodes)) || registerNodes[instruction.left] == hashIntegerExprInvalidNode ||
				registerNodes[instruction.right] == hashIntegerExprInvalidNode ||
				instruction.opcode != compiler.OpAdd && instruction.opcode != compiler.OpSub {
				return invalid, false
			}
			node, ok := addNode(hashIntegerExprNode{
				kind: hashIntegerExprBinary, opcode: instruction.opcode,
				left: registerNodes[instruction.left], right: registerNodes[instruction.right],
			})
			if !ok {
				return invalid, false
			}
			registerNodes[instruction.dst] = node
		case registerIRReturn:
			if instruction.left >= uint8(len(registerNodes)) || registerNodes[instruction.left] == hashIntegerExprInvalidNode ||
				returnNode != hashIntegerExprInvalidNode {
				return invalid, false
			}
			returnNode = registerNodes[instruction.left]
		default:
			return invalid, false
		}
	}
	if returnNode == hashIntegerExprInvalidNode {
		return invalid, false
	}
	result.root = returnNode
	rootNode := result.nodes[returnNode]
	if rootNode.kind == hashIntegerExprBinary && rootNode.left < result.nodeCount && rootNode.right < result.nodeCount {
		leftNode := result.nodes[rootNode.left]
		rightNode := result.nodes[rootNode.right]
		if leftNode.kind == hashIntegerExprParam && rightNode.kind == hashIntegerExprParam &&
			leftNode.param < 2 && rightNode.param < 2 {
			result.direct = true
			result.directOp = rootNode.opcode
		}
	}
	return result, true
}

func evalHashIntegerMapPlan(plan *hashIntegerMapPlan, key, value int64) (int64, bool) {
	if plan == nil {
		return 0, false
	}
	if plan.direct {
		leftFromValue, rightFromValue, ok := hashIntegerMapDirectParams(plan)
		if !ok {
			return 0, false
		}
		left, right := key, value
		if leftFromValue {
			left = value
		}
		if !rightFromValue {
			right = key
		}
		return applyRegisterIRIntegerLinearOpRaw(plan.directOp, left, right)
	}
	return evalHashIntegerMapExpr(&plan.nodes, plan.nodeCount, plan.root, key, value)
}

// hashIntegerMapDirectParams extracts the two parameter orientations once for
// the hot affine loop. compileHashIntegerMapPlan proves this shape, but keep a
// narrow structural check here because the helper is also used by lazy
// element access and must fail closed if a malformed plan is ever cached.
func hashIntegerMapDirectParams(plan *hashIntegerMapPlan) (leftFromValue, rightFromValue, ok bool) {
	if plan == nil || !plan.direct || plan.root >= plan.nodeCount || plan.nodeCount == 0 {
		return false, false, false
	}
	root := plan.nodes[plan.root]
	if root.kind != hashIntegerExprBinary || root.left >= plan.nodeCount || root.right >= plan.nodeCount {
		return false, false, false
	}
	left := plan.nodes[root.left]
	right := plan.nodes[root.right]
	if left.kind != hashIntegerExprParam || right.kind != hashIntegerExprParam || left.param > 1 || right.param > 1 {
		return false, false, false
	}
	return left.param == 1, right.param == 1, true
}

// tryExecuteHashIntegerReduceBlock fuses a pure two-argument Hash#each
// accumulator into one raw-int64 region. It preflights every live pair and
// every checked arithmetic operation before changing the captured value, so
// BigInt/overflow, heterogeneous keys/values, operator redefinition, and
// malformed hash storage all return false before any visible prefix exists.
func (vm *VM) tryExecuteHashIntegerReduceBlock(receiver, block *object.EmeraldValue, keys []*object.EmeraldValue, hash map[*object.EmeraldValue]*object.EmeraldValue) (*object.EmeraldValue, bool) {
	linearRegion, linearRegionOK := core.DirectHashLinearRegion(receiver)
	if !hashIntegerReduceLinearEnabled {
		linearRegionOK = false
	}
	if vm == nil || receiver == nil || receiver.Type != object.ValueHash || receiver.Class != core.R.HashClass ||
		core.AttachedSingletonClass(receiver) != nil ||
		block == nil || block.Type != object.ValueClosure || !typedSSABatchCallEnabled || !registerIRBatchBlockEnabled ||
		vm.instructionLimit != 0 || DevMode || core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() ||
		len(keys) < typedSSAFieldReduceMinElements || hash == nil && !linearRegionOK {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 || closureUsesRefinements(closure) ||
		closure.Fn.ForLoopCollectAsPair || len(closure.Free) == 0 {
		return nil, false
	}
	leaf, found := vm.cachedBlockLeafPlan(closure.Fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil ||
		closure.ReturnOwnerID > 0 && leaf.registerIR.hasExplicitReturn {
		return nil, false
	}
	plan, ok := compileHashIntegerReducePlan(closure.Fn, leaf.registerIR)
	if !ok || int(plan.freeIndex) >= len(closure.Free) || !hashIntegerExprOpsAvailable(vm, &plan.nodes, plan.root) {
		return nil, false
	}
	integerClass := core.R.IntegerClass
	current := derefClosureValue(closure.Free[plan.freeIndex])
	if !typedIntegerArgumentClass(current, integerClass) || core.AttachedSingletonClass(current) != nil {
		return nil, false
	}
	total := current.Data.(int64)
	generation := object.CurrentMethodGeneration()
	for index, key := range keys {
		var keyInteger int64
		var keyOK bool
		var valueInteger int64
		var valueOK bool
		if linearRegionOK && hash == nil {
			keyInteger, valueInteger, valueOK = core.DirectHashLinearPairAt(linearRegion, index)
			keyOK = valueOK
		} else {
			keyInteger, keyOK = typedSSAExactIntegerValueForClass(key, integerClass)
			pairValue, exists := hash[key]
			if !exists {
				continue
			}
			valueInteger, valueOK = typedSSAExactIntegerValueForClass(pairValue, integerClass)
		}
		if !keyOK || !valueOK {
			return nil, false
		}
		var updated bool
		if plan.direct {
			total, updated = evalHashIntegerReducePlan(&plan, total, keyInteger, valueInteger)
		} else {
			total, updated = evalHashIntegerExpr(&plan.nodes, plan.root, total, keyInteger, valueInteger)
		}
		if !updated {
			return nil, false
		}
	}
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	setClosureValue(&closure.Free[plan.freeIndex], core.NewIntegerValue(total))
	core.LastBlockResult = nil
	return receiver, true
}

// tryExecuteHashIntegerMapBlock fuses a pure two-argument Hash#map block into
// a checked raw-int64 pass.  The complete hash is validated before any result
// object becomes visible, so a heterogeneous pair, overflow, stale storage,
// or operator redefinition cleanly falls back to Ruby's normal map protocol.
func (vm *VM) tryExecuteHashIntegerMapBlock(receiver, block *object.EmeraldValue) (*object.EmeraldValue, bool) {
	if vm == nil || receiver == nil || receiver.Type != object.ValueHash || receiver.Class != core.R.HashClass ||
		core.AttachedSingletonClass(receiver) != nil || block == nil || block.Type != object.ValueClosure ||
		!hashIntegerMapRegionEnabled || !typedSSABatchCallEnabled || !registerIRBatchBlockEnabled || vm.instructionLimit != 0 || DevMode ||
		core.AnyTracePointActive() || core.ObjectSpaceAllocationTracing() || !core.HashEachUsesBuiltinImplementation() ||
		!core.HashMapUsesBuiltinImplementation() {
		return nil, false
	}
	closure, ok := block.Data.(*object.Closure)
	if !ok || closure == nil || closure.Fn == nil || closure.BreakOwnerID > 0 || closureUsesRefinements(closure) ||
		closure.Fn.ForLoopCollectAsPair {
		return nil, false
	}
	leaf, found := vm.cachedBlockLeafPlan(closure.Fn)
	if !found || leaf.kind != leafMethodRegisterIR || leaf.registerIR == nil ||
		closure.ReturnOwnerID > 0 && leaf.registerIR.hasExplicitReturn {
		return nil, false
	}
	plan, ok := compileHashIntegerMapPlan(closure.Fn, leaf.registerIR)
	if !ok || !hashIntegerExprOpsAvailable(vm, &plan.nodes, plan.root) {
		return nil, false
	}
	keys, hash := core.DirectHashIteration(receiver)
	linearRegion, linearRegionOK := core.DirectHashLinearRegion(receiver)
	if len(keys) < typedSSABatchCallMinElements || hash == nil && !linearRegionOK {
		return nil, false
	}
	if linearRegionOK && (linearRegion == nil || linearRegion.Count != len(keys)) {
		return nil, false
	}
	lazyResult := hashIntegerMapLazyResultEnabled && linearRegionOK && hash == nil && vm.hashIntegerMapLazyResultHot(closure.Fn)
	var mapped []*object.EmeraldValue
	if !lazyResult {
		mapped = make([]*object.EmeraldValue, len(keys))
	}
	mappedCount := 0
	integerClass := core.R.IntegerClass
	generation := object.CurrentMethodGeneration()
	directLeftFromValue, directRightFromValue, directPlanOK := hashIntegerMapDirectParams(&plan)
	if plan.direct && !directPlanOK {
		return nil, false
	}
	for index, key := range keys {
		var keyInteger int64
		var keyOK bool
		var valueInteger int64
		var valueOK bool
		if linearRegionOK && hash == nil {
			keyInteger, valueInteger, valueOK = core.DirectHashLinearPairAt(linearRegion, index)
			keyOK = valueOK
		} else {
			keyInteger, keyOK = typedSSAExactIntegerValueForClass(key, integerClass)
			pairValue, exists := hash[key]
			if !exists {
				continue
			}
			valueInteger, valueOK = typedSSAExactIntegerValueForClass(pairValue, integerClass)
		}
		if !keyOK || !valueOK {
			return nil, false
		}
		var result int64
		var valid bool
		if plan.direct {
			left, right := keyInteger, valueInteger
			if directLeftFromValue {
				left = valueInteger
			}
			if !directRightFromValue {
				right = keyInteger
			}
			result, valid = applyRegisterIRIntegerLinearOpRaw(plan.directOp, left, right)
		} else {
			result, valid = evalHashIntegerMapPlan(&plan, keyInteger, valueInteger)
		}
		if !valid || object.CurrentMethodGeneration() != generation {
			return nil, false
		}
		if !lazyResult {
			mapped[mappedCount] = core.NewSmallIntegerValue(result)
		}
		mappedCount++
	}
	if object.CurrentMethodGeneration() != generation {
		return nil, false
	}
	core.LastBlockResult = nil
	if lazyResult {
		payload := &hashIntegerMapLazyPayload{region: *linearRegion, plan: plan, length: mappedCount}
		result := &object.EmeraldValue{Type: object.ValueArray, Class: core.R.ArrayClass}
		result.SetLazyArrayRegion(&object.LazyArrayRegion{
			Length:           mappedCount,
			Payload:          payload,
			Materialize:      payload.materialize,
			ElementAt:        payload.elementAt,
			MethodGeneration: generation,
		})
		core.RegisterLazyArrayRegion(result)
		return result, true
	}
	mapped = mapped[:mappedCount]
	return &object.EmeraldValue{Type: object.ValueArray, Data: mapped, Class: core.R.ArrayClass}, true
}
