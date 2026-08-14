// Package aot contains deliberately small, strict ahead-of-time tiers.
//
// The ordinary RGo runtime remains the compatibility path. This package only
// accepts fully recognizable primitive loops (Integer loops, ASCII byte
// buffers, typed collections, a closed-world object region, and an explicitly
// opted-in Prawn profile). A proven plan can either emit standalone Go source
// or run as an in-process typed kernel. Rejecting an unknown construct is important:
// a compiled artifact must never silently change Ruby's dynamic dispatch or
// bignum rules.
package aot

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/object"
)

var ErrUnsupported = errors.New("source is outside the strict AOT subset")

type expression struct {
	op          compiler.Opcode
	isConstant  bool
	value       int64
	local       int
	left, right *expression
	callName    string
	callArg     *expression
}

type assignment struct {
	local int
	expr  *expression
}

type loopMode uint8

const (
	whileLoopMode loopMode = iota
	timesLoopMode
	rangeLoopMode
	stringLoopMode
	collectionLoopMode
	objectLoopMode
	prawnSimpleLoopMode
	prawnSteadyLoopMode
)

type plan struct {
	locals          int
	initial         map[int]int64
	counterLocal    int
	limit           int64
	assignments     []assignment
	resultLocal     int
	mode            loopMode
	timesCount      int
	timesCounter    int
	timesSum        int
	timesExpr       *expression
	rangeStart      int
	rangeEnd        int
	rangeCounter    int
	rangeSum        int
	rangeAscending  bool
	rangeExpr       *expression
	methods         map[string]*integerMethod
	stringLoop      *stringLoopPlan
	collectionLoop  *collectionLoopPlan
	objectLoop      *objectLoopPlan
	prawnSimpleLoop *prawnSimpleLoopPlan
	prawnSteadyLoop *prawnSteadyLoopPlan
}

// stringLoopPlan is the first non-integer source AOT shape.  It represents a
// checked ASCII byte buffer loop, not a Ruby String object: the generated
// program keeps bytes in a strings.Builder and materializes only the final
// observable output.  Any source shape outside this proof is rejected and
// therefore remains on the compatibility VM.
type stringLoopPlan struct {
	count      int64
	start      int64
	step       int64
	base       int64
	modulus    int64
	outputText bool
}

// collectionLoopPlan is a proven Array/Hash construction and reduction.  The
// generated program keeps the hot values as int64 and materializes no Ruby
// Array, Hash, Integer, or block.  Because the strict observable output only
// reads Hash#length, the compiler can also eliminate the Hash value stores and
// derive its unique-key count from the modulo range.  It is intentionally a
// shape proof rather than a generic collection optimizer; an unknown method
// override or object shape must remain on the compatibility VM.
type collectionLoopPlan struct {
	count    int64
	multiply int64
	modulus  int64
	keyMod   int64
}

// objectLoopPlan is a strict closed-world object region. It covers a class
// with a straight-line initializer, Array.new with a literal/index argument,
// and an optional pure getter map whose only observable result is
// Array#length or an affine Integer sum. The proof removes Ruby frames, boxed
// values and dynamic sends from that region; any additional observation
// rejects the plan.
type objectLoopPlan struct {
	count       int64
	fields      []objectFieldPlan
	argument    objectFieldValue
	getterField int
	getterExpr  *objectExpr
	mapResult   bool
	output      objectOutputKind
}

type objectOutputKind uint8

const (
	objectOutputLength objectOutputKind = iota
	objectOutputIntegerSum
)

type objectFieldKind uint8

const (
	objectFieldInteger objectFieldKind = iota
	objectFieldString
)

type objectFieldValue struct {
	kind      objectFieldKind
	integer   int64
	text      string
	fromIndex bool
	expr      *objectExpr
}

type objectFieldPlan struct {
	kind  objectFieldKind
	value objectFieldValue
}

// objectExpr is the deliberately small typed IR used by the closed-world
// object region. It describes only immutable Integer arithmetic, field reads,
// and the Array.new block index. Strings remain valid constructor fields for
// length-only programs, but they are never admitted to an integer sum.
type objectExprKind uint8

const (
	objectExprInteger objectExprKind = iota
	objectExprIndex
	objectExprField
	objectExprAdd
	objectExprSub
	objectExprMul
	objectExprMod
	objectExprNeg
)

type objectExpr struct {
	kind        objectExprKind
	integer     int64
	field       int
	left, right *objectExpr
}

type prawnSimpleLoopPlan struct {
	count int64
	pages []string
}

// prawnIndexedText is a deliberately small template for an ASCII Prawn text
// call whose only dynamic part is the Integer#times block index.  Keeping the
// template typed means the steady Prawn profile never enters Ruby String,
// Integer, or method dispatch for the proven region.
type prawnIndexedText struct {
	prefix  string
	suffix  string
	offset  int64
	indexed bool
}

type prawnSteadyLoopPlan struct {
	count int64
	pages []prawnIndexedText
}

type integerMethod struct {
	name  string
	param string
	expr  *expression
}

const maxValidatedIterations = 100_000_000

// Generate accepts a compiled top-level bytecode program and returns a
// standalone Go program.  The accepted Ruby shape is intentionally explicit:
// integer local initializers, `while counter < INTEGER`, an integer-only loop
// body, and a final `puts local`.  Methods, blocks, sends in the loop, mutable
// objects, and unknown statements are rejected.
func Generate(bytecode *compiler.Bytecode) (string, error) {
	p, err := buildPlan(bytecode)
	if err != nil {
		return "", err
	}
	return generateGo(p), nil
}

func unsupported(format string, args ...interface{}) error {
	if format == "" {
		return ErrUnsupported
	}
	return fmt.Errorf("%w: %s", ErrUnsupported, fmt.Sprintf(format, args...))
}

func buildPlan(bytecode *compiler.Bytecode) (*plan, error) {
	if bytecode == nil || len(bytecode.Instructions) == 0 {
		return nil, unsupported("empty program")
	}
	if rangePlan, matched, err := buildRangePlan(bytecode); matched {
		if err != nil {
			return nil, err
		}
		return rangePlan, nil
	}
	if timesPlan, matched, err := buildTimesPlan(bytecode); matched {
		if err != nil {
			return nil, err
		}
		return timesPlan, nil
	}
	loopStart, jumpPosition, exitPosition, counterLocal, limit, bodyStart, ok := findLoop(bytecode)
	if !ok {
		return nil, unsupported("expected an integer while loop")
	}

	initial, ok := parseInitializers(bytecode, loopStart)
	if !ok {
		return nil, unsupported("top-level statements before the loop must be integer assignments")
	}
	assignments, ok := parseBody(bytecode, bodyStart, jumpPosition)
	if !ok || len(assignments) == 0 {
		return nil, unsupported("loop body is not an integer-only assignment block")
	}
	limitLocal := loopLimitLocal(bytecode, loopStart)
	if limitLocal >= 0 {
		value, initialized := initial[limitLocal]
		if !initialized || localWritten(assignments, limitLocal) {
			return nil, unsupported("loop upper bound must be an immutable integer local")
		}
		limit = value
	}
	resultLocal, ok := parsePutsLocal(bytecode, exitPosition)
	if !ok {
		return nil, unsupported("program must finish with puts of a local integer")
	}

	maxLocal := counterLocal
	for local := range initial {
		if local > maxLocal {
			maxLocal = local
		}
	}
	for _, assignment := range assignments {
		if assignment.local > maxLocal {
			maxLocal = assignment.local
		}
		collectExpressionLocals(assignment.expr, &maxLocal)
	}
	if resultLocal > maxLocal {
		maxLocal = resultLocal
	}
	if bytecode.NumLocals > maxLocal+1 {
		maxLocal = bytecode.NumLocals - 1
	}
	if maxLocal < 0 || maxLocal >= 256 {
		return nil, unsupported("too many local slots")
	}
	for local := 0; local <= maxLocal; local++ {
		if local == counterLocal {
			continue
		}
		if _, initialized := initial[local]; !initialized && !localWritten(assignments, local) {
			return nil, unsupported("local %d is not initialized", local)
		}
	}
	if _, initialized := initial[counterLocal]; !initialized {
		return nil, unsupported("loop counter is not initialized")
	}
	if !localWritten(assignments, counterLocal) {
		return nil, unsupported("loop counter is never updated")
	}
	if _, initialized := initial[resultLocal]; !initialized && !localWritten(assignments, resultLocal) {
		return nil, unsupported("result local is not initialized")
	}

	compiled := &plan{
		locals:       maxLocal + 1,
		initial:      initial,
		counterLocal: counterLocal,
		limit:        limit,
		assignments:  assignments,
		resultLocal:  resultLocal,
	}
	if err := validatePlan(compiled); err != nil {
		return nil, err
	}
	return compiled, nil
}

func parseCapturedIntegerBlockExpression(fn *object.Function, sumLocal, counterLocal int) (*expression, bool) {
	if fn == nil || len(fn.Instructions) < 6 || len(fn.Params) != 1 || len(fn.ParamLocalIndices) != 1 ||
		fn.HasRestParam || fn.HasBlockParam || len(fn.KeywordParams) > 0 || fn.KeywordRestParam != "" || fn.KeywordRestOnly ||
		len(fn.FreeVarNames) != 1 || len(fn.ParamPatterns) == 1 && fn.ParamPatterns[0] != nil {
		return nil, false
	}
	instructions := fn.Instructions
	if compiler.Opcode(instructions[0]) != compiler.OpGetFree || instructions[1] != 0 ||
		(compiler.Opcode(instructions[2]) != compiler.OpGetLocal && compiler.Opcode(instructions[2]) != compiler.OpGetLocalFast) ||
		int(instructions[3]) != fn.ParamLocalIndices[0] {
		return nil, false
	}
	stack := []*expression{{local: sumLocal}, {local: counterLocal}}
	position := 4
	for position < len(instructions) {
		op := compiler.Opcode(instructions[position])
		switch op {
		case compiler.OpConstant:
			if position+2 >= len(instructions) {
				return nil, false
			}
			index := int(instructions[position+1])<<8 | int(instructions[position+2])
			if index < 0 || index >= len(fn.Constants) {
				return nil, false
			}
			value := fn.Constants[index]
			if value == nil || value.Type != object.ValueInteger || value.BigIntValue() != nil {
				return nil, false
			}
			integer, ok := value.Data.(int64)
			if !ok {
				return nil, false
			}
			stack = append(stack, &expression{isConstant: true, value: integer})
			position += 3
		case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod,
			compiler.OpBitAnd, compiler.OpBitOr, compiler.OpBitXor:
			if len(stack) < 2 {
				return nil, false
			}
			right := stack[len(stack)-1]
			left := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, &expression{op: op, left: left, right: right})
			position++
		case compiler.OpNeg, compiler.OpNegate:
			if len(stack) == 0 {
				return nil, false
			}
			value := stack[len(stack)-1]
			stack[len(stack)-1] = &expression{op: op, left: value}
			position++
		case compiler.OpSetFree:
			if position+1 >= len(instructions) || instructions[position+1] != 0 || len(stack) != 1 ||
				position+3 != len(instructions) || compiler.Opcode(instructions[position+2]) != compiler.OpBlockReturn {
				return nil, false
			}
			return stack[0], true
		default:
			return nil, false
		}
	}
	return nil, false
}

func parseIntegerInitialPrefix(bytecode *compiler.Bytecode) (map[int]int64, int, bool) {
	if bytecode == nil {
		return nil, 0, false
	}
	instructions := bytecode.Instructions
	initial := make(map[int]int64)
	position := 0
	for {
		value, next, ok := readIntegerConstant(bytecode, position, len(instructions))
		if !ok || next+2 >= len(instructions) || compiler.Opcode(instructions[next]) != compiler.OpSetLocal ||
			compiler.Opcode(instructions[next+2]) != compiler.OpPop {
			break
		}
		initial[int(instructions[next+1])] = value
		position = next + 3
	}
	return initial, position, position > 0
}

func buildTimesPlan(bytecode *compiler.Bytecode) (*plan, bool, error) {
	if bytecode == nil || len(bytecode.Instructions) == 0 {
		return nil, false, nil
	}
	instructions := bytecode.Instructions
	initial, position, prefixOK := parseIntegerInitialPrefix(bytecode)
	if !prefixOK {
		return nil, false, nil
	}
	countLocal, next, ok := readLocal(instructions, position)
	if !ok {
		return nil, false, nil
	}
	position = next
	if position+1 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpGetLocalCell {
		return nil, false, nil
	}
	sumLocal := int(instructions[position+1])
	position += 2
	if position+3 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpClosure || instructions[position+3] != 1 {
		return nil, false, nil
	}
	functionIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
	if functionIndex < 0 || functionIndex >= len(bytecode.Constants) {
		return nil, false, nil
	}
	functionValue := bytecode.Constants[functionIndex]
	if functionValue == nil || functionValue.Type != object.ValueFunction {
		return nil, false, nil
	}
	fn, ok := functionValue.Data.(*object.Function)
	if !ok || fn == nil {
		return nil, false, nil
	}
	position += 4
	if position+5 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpSend ||
		instructions[position+3] != 1 || instructions[position+4] != 0 || instructions[position+5] != 255 {
		return nil, false, nil
	}
	methodIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
	if methodIndex < 0 || methodIndex >= len(bytecode.Constants) {
		return nil, false, nil
	}
	methodName, ok := bytecode.Constants[methodIndex].Data.(string)
	if !ok || methodName != "times" {
		return nil, false, nil
	}
	position += 6
	if position >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpPop {
		return nil, true, unsupported("times call must be the only loop statement")
	}
	position++
	resultLocal, ok := parsePutsLocal(bytecode, position)
	if !ok {
		return nil, true, unsupported("times AOT program must finish with puts of a local integer")
	}
	if _, initialized := initial[countLocal]; !initialized {
		return nil, true, unsupported("times count must be an initialized integer local")
	}
	if _, initialized := initial[sumLocal]; !initialized {
		return nil, true, unsupported("times captured local must be an initialized integer local")
	}
	if _, initialized := initial[resultLocal]; !initialized {
		return nil, true, unsupported("result local is not initialized")
	}
	maxLocal := resultLocal
	for local := range initial {
		if local > maxLocal {
			maxLocal = local
		}
	}
	timesCounter := maxLocal + 1
	expr, ok := parseCapturedIntegerBlockExpression(fn, sumLocal, timesCounter)
	if !ok {
		return nil, true, unsupported("times block is not a strict integer assignment")
	}
	compiled := &plan{
		locals:       timesCounter + 1,
		initial:      initial,
		resultLocal:  resultLocal,
		mode:         timesLoopMode,
		timesCount:   countLocal,
		timesCounter: timesCounter,
		timesSum:     sumLocal,
		timesExpr:    expr,
	}
	if err := validateTimesPlan(compiled); err != nil {
		return nil, true, err
	}
	return compiled, true, nil
}

func buildRangePlan(bytecode *compiler.Bytecode) (*plan, bool, error) {
	initial, position, prefixOK := parseIntegerInitialPrefix(bytecode)
	if !prefixOK {
		return nil, false, nil
	}
	instructions := bytecode.Instructions
	startLocal, next, ok := readLocal(instructions, position)
	if !ok {
		return nil, false, nil
	}
	position = next
	endValue := int64(0)
	endLocal := -1
	if value, next, constantOK := readIntegerConstant(bytecode, position, len(instructions)); constantOK {
		endValue = value
		position = next
	} else if local, next, localOK := readLocal(instructions, position); localOK {
		endLocal = local
		value, initialized := initial[endLocal]
		if !initialized {
			return nil, true, unsupported("range endpoint must be an initialized integer local")
		}
		endValue = value
		position = next
	} else {
		return nil, false, nil
	}
	if position+1 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpGetLocalCell {
		return nil, false, nil
	}
	sumLocal := int(instructions[position+1])
	position += 2
	if position+3 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpClosure || instructions[position+3] != 1 {
		return nil, false, nil
	}
	functionIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
	if functionIndex < 0 || functionIndex >= len(bytecode.Constants) {
		return nil, false, nil
	}
	functionValue := bytecode.Constants[functionIndex]
	if functionValue == nil || functionValue.Type != object.ValueFunction {
		return nil, false, nil
	}
	fn, ok := functionValue.Data.(*object.Function)
	if !ok || fn == nil {
		return nil, false, nil
	}
	position += 4
	if position+5 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpSend ||
		instructions[position+3] != 1 || instructions[position+4] != 1 || instructions[position+5] != 255 {
		return nil, false, nil
	}
	methodIndex := int(instructions[position+1])<<8 | int(instructions[position+2])
	if methodIndex < 0 || methodIndex >= len(bytecode.Constants) {
		return nil, false, nil
	}
	methodName, ok := bytecode.Constants[methodIndex].Data.(string)
	if !ok || (methodName != "upto" && methodName != "downto") {
		return nil, false, nil
	}
	position += 6
	if position >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpPop {
		return nil, true, unsupported("range call must be the only loop statement")
	}
	position++
	resultLocal, ok := parsePutsLocal(bytecode, position)
	if !ok {
		return nil, true, unsupported("range AOT program must finish with puts of a local integer")
	}
	if _, initialized := initial[startLocal]; !initialized {
		return nil, true, unsupported("range start must be an initialized integer local")
	}
	if _, initialized := initial[sumLocal]; !initialized {
		return nil, true, unsupported("range captured local must be an initialized integer local")
	}
	if _, initialized := initial[resultLocal]; !initialized {
		return nil, true, unsupported("result local is not initialized")
	}
	maxLocal := resultLocal
	for local := range initial {
		if local > maxLocal {
			maxLocal = local
		}
	}
	rangeEnd := maxLocal + 1
	rangeCounter := maxLocal + 2
	initial[rangeEnd] = endValue
	expr, ok := parseCapturedIntegerBlockExpression(fn, sumLocal, rangeCounter)
	if !ok {
		return nil, true, unsupported("range block is not a strict integer assignment")
	}
	compiled := &plan{
		locals:         rangeCounter + 1,
		initial:        initial,
		resultLocal:    resultLocal,
		mode:           rangeLoopMode,
		rangeStart:     startLocal,
		rangeEnd:       rangeEnd,
		rangeCounter:   rangeCounter,
		rangeSum:       sumLocal,
		rangeAscending: methodName == "upto",
		rangeExpr:      expr,
	}
	if err := validateRangePlan(compiled); err != nil {
		return nil, true, err
	}
	return compiled, true, nil
}

func validateTimesPlan(p *plan) error {
	if p == nil || p.mode != timesLoopMode || p.timesExpr == nil {
		return unsupported("invalid times plan")
	}
	initial := cloneIntegerInitials(p.initial)
	count := initial[p.timesCount]
	initial[p.timesCounter] = 0
	if err, proven := validateAffineLoop(initial, p.timesCounter, 0, count, 1, false,
		[]assignment{{local: p.timesSum, expr: p.timesExpr}}, p.methods, false); proven {
		return err
	}
	if err, proven := validatePeriodicTimesPlan(p, count); proven {
		return err
	}
	values := make([]int64, p.locals)
	for local, value := range p.initial {
		values[local] = value
	}
	count = values[p.timesCount]
	if count <= 0 {
		return nil
	}
	for iteration := int64(0); iteration < count; iteration++ {
		if iteration >= maxValidatedIterations {
			return unsupported("times loop exceeds %d statically validated iterations", maxValidatedIterations)
		}
		values[p.timesCounter] = iteration
		value, ok := evaluateExpressionWithMethods(p.timesExpr, values, p.methods)
		if !ok {
			return unsupported("times block arithmetic can overflow a machine Integer")
		}
		values[p.timesSum] = value
	}
	return nil
}

// validatePeriodicTimesPlan proves a repeating counter expression (for
// example `i & 7` or `i % 26`) without walking every iteration. The proof
// tracks one period and bounds every cycle's accumulator prefix with big.Int,
// preserving Ruby Integer overflow semantics while keeping validation O(period).
func validatePeriodicTimesPlan(p *plan, count int64) (error, bool) {
	if p == nil || count <= 0 {
		return nil, count == 0
	}
	delta, sign, ok := selfDeltaExpression(p.timesExpr, p.timesSum)
	if !ok {
		return nil, false
	}
	period, ok := periodicCounterExpression(delta, p.timesCounter)
	if !ok || period <= 0 || period > 1_000_000 {
		return nil, false
	}
	values := make([]int64, p.locals)
	for local, value := range p.initial {
		values[local] = value
	}
	prefix := big.NewInt(0)
	minimum := big.NewInt(0)
	maximum := big.NewInt(0)
	for index := int64(0); index < period; index++ {
		values[p.timesCounter] = index
		value, valueOK := evaluateExpressionWithMethods(delta, values, p.methods)
		if !valueOK {
			return unsupported("times periodic expression can overflow a machine Integer"), true
		}
		if sign < 0 {
			value = -value
		}
		prefix.Add(prefix, big.NewInt(value))
		if prefix.Cmp(minimum) < 0 {
			minimum.Set(prefix)
		}
		if prefix.Cmp(maximum) > 0 {
			maximum.Set(prefix)
		}
	}
	cycleCount := new(big.Int).SetInt64(count / period)
	cycleBase := new(big.Int).Mul(new(big.Int).Set(prefix), cycleCount)
	boundMinimum := new(big.Int).Set(minimum)
	boundMaximum := new(big.Int).Set(maximum)
	if prefix.Sign() >= 0 {
		boundMaximum.Add(boundMaximum, cycleBase)
	} else {
		boundMinimum.Add(boundMinimum, cycleBase)
	}
	remainder := count % period
	prefixRemainder := new(big.Int).Set(cycleBase)
	for index := int64(0); index < remainder; index++ {
		values[p.timesCounter] = (count/period)*period + index
		value, valueOK := evaluateExpressionWithMethods(delta, values, p.methods)
		if !valueOK {
			return unsupported("times periodic expression can overflow a machine Integer"), true
		}
		if sign < 0 {
			value = -value
		}
		prefixRemainder.Add(prefixRemainder, big.NewInt(value))
		if prefixRemainder.Cmp(boundMinimum) < 0 {
			boundMinimum.Set(prefixRemainder)
		}
		if prefixRemainder.Cmp(boundMaximum) > 0 {
			boundMaximum.Set(prefixRemainder)
		}
	}
	initialValue := big.NewInt(p.initial[p.timesSum])
	boundMinimum.Add(boundMinimum, initialValue)
	boundMaximum.Add(boundMaximum, initialValue)
	if !boundMinimum.IsInt64() || !boundMaximum.IsInt64() {
		return unsupported("times periodic accumulator can overflow a machine Integer"), true
	}
	return nil, true
}

func validateRangePlan(p *plan) error {
	if p == nil || p.mode != rangeLoopMode || p.rangeExpr == nil {
		return unsupported("invalid range plan")
	}
	initial := cloneIntegerInitials(p.initial)
	start := initial[p.rangeStart]
	end := initial[p.rangeEnd]
	step := int64(-1)
	if p.rangeAscending {
		step = 1
	}
	initial[p.rangeCounter] = start
	if err, proven := validateAffineLoop(initial, p.rangeCounter, start, end, step, true,
		[]assignment{{local: p.rangeSum, expr: p.rangeExpr}}, p.methods, false); proven {
		return err
	}
	values := make([]int64, p.locals)
	for local, value := range p.initial {
		values[local] = value
	}
	start = values[p.rangeStart]
	end = values[p.rangeEnd]
	if p.rangeAscending && start > end || !p.rangeAscending && start < end {
		return nil
	}
	iterations := int64(0)
	for index := start; ; {
		if iterations >= maxValidatedIterations {
			return unsupported("range loop exceeds %d statically validated iterations", maxValidatedIterations)
		}
		values[p.rangeCounter] = index
		value, ok := evaluateExpressionWithMethods(p.rangeExpr, values, p.methods)
		if !ok {
			return unsupported("range block arithmetic can overflow a machine Integer")
		}
		values[p.rangeSum] = value
		iterations++
		if index == end || (p.rangeAscending && index == math.MaxInt64) || (!p.rangeAscending && index == math.MinInt64) {
			break
		}
		if p.rangeAscending {
			index++
		} else {
			index--
		}
	}
	return nil
}

func instructionWidth(instructions compiler.Instructions, position int) (int, bool) {
	if position < 0 || position >= len(instructions) {
		return 0, false
	}
	definition, ok := compiler.Lookup(byte(instructions[position]))
	if !ok {
		return 0, false
	}
	width := 1
	for _, operandWidth := range definition.OperandWidths {
		width += operandWidth
	}
	return width, position+width <= len(instructions)
}

func readUint16(instructions compiler.Instructions, position int) (int, bool) {
	if position < 0 || position+2 >= len(instructions) {
		return 0, false
	}
	return int(instructions[position+1])<<8 | int(instructions[position+2]), true
}

func readLocal(instructions compiler.Instructions, position int) (int, int, bool) {
	if position < 0 || position+1 >= len(instructions) ||
		(compiler.Opcode(instructions[position]) != compiler.OpGetLocal && compiler.Opcode(instructions[position]) != compiler.OpGetLocalFast) {
		return 0, position, false
	}
	return int(instructions[position+1]), position + 2, true
}

func readIntegerConstant(bytecode *compiler.Bytecode, position, end int) (int64, int, bool) {
	instructions := bytecode.Instructions
	if position < 0 || position+2 >= end || compiler.Opcode(instructions[position]) != compiler.OpConstant {
		return 0, position, false
	}
	index, ok := readUint16(instructions, position)
	if !ok || index < 0 || index >= len(bytecode.Constants) {
		return 0, position, false
	}
	value := bytecode.Constants[index]
	if value == nil || value.Type != object.ValueInteger || value.BigIntValue() != nil {
		return 0, position, false
	}
	integer, ok := value.Data.(int64)
	if !ok {
		return 0, position, false
	}
	return integer, position + 3, true
}

func findLoop(bytecode *compiler.Bytecode) (loopStart, jumpPosition, exitPosition, counterLocal int, limit int64, bodyStart int, ok bool) {
	instructions := bytecode.Instructions
	for position := 0; position < len(instructions); {
		candidate := position
		counter, next, localOK := readLocal(instructions, position)
		if !localOK {
			width, valid := instructionWidth(instructions, position)
			if !valid {
				return 0, 0, 0, 0, 0, 0, false
			}
			position += width
			continue
		}
		position = next
		_, next, isLocalLimit := readLocal(instructions, position)
		if isLocalLimit {
			position = next
			return findLoopWithHeader(bytecode, candidate, position, counter, 0)
		}
		limitValue, next, constantLimit := readIntegerConstant(bytecode, position, len(instructions))
		if !constantLimit {
			continue
		}
		return findLoopWithHeader(bytecode, candidate, next, counter, limitValue)
	}
	return 0, 0, 0, 0, 0, 0, false
}

func findLoopWithHeader(bytecode *compiler.Bytecode, start, position, counter int, limit int64) (int, int, int, int, int64, int, bool) {
	instructions := bytecode.Instructions
	if position >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpLessThan {
		return 0, 0, 0, 0, 0, 0, false
	}
	position++
	if position+2 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpJumpNotTruthy {
		return 0, 0, 0, 0, 0, 0, false
	}
	exit, ok := readUint16(instructions, position)
	if !ok {
		return 0, 0, 0, 0, 0, 0, false
	}
	position += 3
	if position+2 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpSetWhileEnd {
		return 0, 0, 0, 0, 0, 0, false
	}
	whileEnd, ok := readUint16(instructions, position)
	if !ok || whileEnd != exit {
		return 0, 0, 0, 0, 0, 0, false
	}
	body := position + 3
	for cursor := body; cursor < exit && cursor < len(instructions); {
		width, valid := instructionWidth(instructions, cursor)
		if !valid {
			return 0, 0, 0, 0, 0, 0, false
		}
		if compiler.Opcode(instructions[cursor]) == compiler.OpJump {
			target, targetOK := readUint16(instructions, cursor)
			if targetOK && target == start {
				return start, cursor, exit, counter, limit, body, true
			}
		}
		cursor += width
	}
	return 0, 0, 0, 0, 0, 0, false
}

func loopLimitLocal(bytecode *compiler.Bytecode, loopStart int) int {
	instructions := bytecode.Instructions
	_, position, ok := readLocal(instructions, loopStart)
	if !ok {
		return -1
	}
	local, _, ok := readLocal(instructions, position)
	if !ok {
		return -1
	}
	return local
}

func parseInitializers(bytecode *compiler.Bytecode, end int) (map[int]int64, bool) {
	initial := make(map[int]int64)
	position := 0
	for position < end {
		value, next, ok := readIntegerConstant(bytecode, position, end)
		if !ok || next+2 > end || compiler.Opcode(bytecode.Instructions[next]) != compiler.OpSetLocal {
			return nil, false
		}
		local := int(bytecode.Instructions[next+1])
		next += 2
		if next >= end || compiler.Opcode(bytecode.Instructions[next]) != compiler.OpPop {
			return nil, false
		}
		initial[local] = value
		position = next + 1
	}
	return initial, position == end
}

func parseBody(bytecode *compiler.Bytecode, start, end int) ([]assignment, bool) {
	stack := make([]*expression, 0, 8)
	assignments := make([]assignment, 0, 4)
	for position := start; position < end; {
		op := compiler.Opcode(bytecode.Instructions[position])
		switch op {
		case compiler.OpGetLocal, compiler.OpGetLocalFast:
			if position+1 >= end {
				return nil, false
			}
			stack = append(stack, &expression{local: int(bytecode.Instructions[position+1])})
			position += 2
		case compiler.OpConstant:
			value, next, ok := readIntegerConstant(bytecode, position, end)
			if !ok {
				return nil, false
			}
			stack = append(stack, &expression{value: value, op: compiler.OpConstant, isConstant: true})
			position = next
		case compiler.OpAdd, compiler.OpSub, compiler.OpMul, compiler.OpMod,
			compiler.OpBitAnd, compiler.OpBitOr, compiler.OpBitXor:
			if len(stack) < 2 {
				return nil, false
			}
			right := stack[len(stack)-1]
			left := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, &expression{op: op, left: left, right: right})
			position++
		case compiler.OpNeg, compiler.OpNegate:
			if len(stack) == 0 {
				return nil, false
			}
			value := stack[len(stack)-1]
			stack[len(stack)-1] = &expression{op: op, left: value}
			position++
		case compiler.OpSetLocal:
			if len(stack) == 0 || position+1 >= end {
				return nil, false
			}
			assignments = append(assignments, assignment{local: int(bytecode.Instructions[position+1]), expr: stack[len(stack)-1]})
			position += 2
		case compiler.OpPop:
			if len(stack) == 0 {
				return nil, false
			}
			stack = stack[:len(stack)-1]
			position++
		default:
			return nil, false
		}
	}
	return assignments, len(stack) == 0
}

func parsePutsLocal(bytecode *compiler.Bytecode, start int) (int, bool) {
	instructions := bytecode.Instructions
	position := start
	if position+1 < len(instructions) && compiler.Opcode(instructions[position]) == compiler.OpNil && compiler.Opcode(instructions[position+1]) == compiler.OpPop {
		position += 2
	}
	if position >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpSelf {
		return 0, false
	}
	position++
	resultLocal, position, ok := readLocal(instructions, position)
	if !ok || position+5 >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpSend {
		return 0, false
	}
	methodIndex, ok := readUint16(instructions, position)
	if !ok || methodIndex < 0 || methodIndex >= len(bytecode.Constants) {
		return 0, false
	}
	method, methodOK := bytecode.Constants[methodIndex].Data.(string)
	if !methodOK || method != "puts" || instructions[position+3] != 0 || instructions[position+4] != 1 || instructions[position+5] != 255 {
		return 0, false
	}
	position += 6
	if position >= len(instructions) || compiler.Opcode(instructions[position]) != compiler.OpPop {
		return 0, false
	}
	return resultLocal, position+1 == len(instructions)
}

func localWritten(assignments []assignment, local int) bool {
	for _, assignment := range assignments {
		if assignment.local == local {
			return true
		}
	}
	return false
}

type integerInterval struct {
	min int64
	max int64
}

func cloneIntegerInitials(initial map[int]int64) map[int]int64 {
	result := make(map[int]int64, len(initial))
	for local, value := range initial {
		result[local] = value
	}
	return result
}

// validateAffineLoop proves the common arithmetic-loop shape without walking
// every iteration.  The older evaluator below remains the fallback for
// constructs whose bounds cannot be represented by this interval analysis.
// This keeps compilation of a 50-million-iteration AOT loop effectively
// constant-time while still rejecting any expression whose conservative
// machine-integer range overflows.
func validateAffineLoop(initial map[int]int64, counter int, start, limit, step int64, inclusive bool, assignments []assignment, methods map[string]*integerMethod, requireCounterAssignment bool) (error, bool) {
	if step == 0 {
		return nil, false
	}
	count, ok := affineIterationCount(start, limit, step, inclusive)
	if !ok {
		return nil, false
	}
	if count == 0 {
		return nil, true
	}
	if count > maxValidatedIterations {
		kind := "loop"
		if !requireCounterAssignment {
			kind = "times/range loop"
		}
		return unsupported("%s exceeds %d statically validated iterations", kind, maxValidatedIterations), true
	}
	lastCounter, ok := affineLastValue(start, step, count)
	if !ok {
		return nil, false
	}

	intervals := make(map[int]integerInterval, len(initial)+1)
	known := make(map[int]bool, len(initial)+1)
	for local, value := range initial {
		intervals[local] = integerInterval{min: value, max: value}
		known[local] = true
	}
	intervals[counter] = integerInterval{min: minInt64(start, lastCounter), max: maxInt64(start, lastCounter)}
	known[counter] = true
	bodyWrites := make(map[int]bool, len(assignments))
	for _, assignment := range assignments {
		bodyWrites[assignment.local] = true
	}
	processed := map[int]bool{counter: true}
	seenWrites := make(map[int]bool, len(assignments))
	counterAssignment := false
	for _, assignment := range assignments {
		if seenWrites[assignment.local] {
			return nil, false
		}
		seenWrites[assignment.local] = true
		if assignment.local == counter {
			if !requireCounterAssignment {
				return nil, false
			}
			actualStep, stepOK := counterExpressionStep(assignment.expr, counter)
			if !stepOK || actualStep != step {
				return nil, false
			}
			counterAssignment = true
			continue
		}

		if delta, sign, self := selfDeltaExpression(assignment.expr, assignment.local); self {
			// The interval recurrence below is only exact for a delta that is
			// independent of the value being updated.  Reject exponential or
			// otherwise self-referential updates so the old per-iteration
			// evaluator can decide them conservatively.
			if containsLocalExpression(delta, assignment.local) {
				return nil, false
			}
			if referencesUnprocessedWrite(delta, bodyWrites, processed, counter) {
				return nil, false
			}
			if !known[assignment.local] {
				return nil, false
			}
			deltaRange, rangeOK := evaluateInterval(delta, intervals, known, methods)
			if !rangeOK {
				return nil, false
			}
			if sign < 0 {
				deltaRange, rangeOK = negateInterval(deltaRange)
				if !rangeOK {
					return nil, false
				}
			}
			deltaRange, rangeOK = scaleInterval(deltaRange, count)
			if !rangeOK {
				return nil, false
			}
			valueRange, rangeOK := addIntervals(intervals[assignment.local], deltaRange)
			if !rangeOK {
				return nil, false
			}
			intervals[assignment.local] = valueRange
			known[assignment.local] = true
			processed[assignment.local] = true
			continue
		}

		if referencesUnprocessedWrite(assignment.expr, bodyWrites, processed, counter) {
			return nil, false
		}
		valueRange, rangeOK := evaluateInterval(assignment.expr, intervals, known, methods)
		if !rangeOK {
			return nil, false
		}
		intervals[assignment.local] = valueRange
		known[assignment.local] = true
		processed[assignment.local] = true
	}
	if requireCounterAssignment && !counterAssignment {
		return nil, false
	}
	return nil, true
}

func affineIterationCount(start, limit, step int64, inclusive bool) (int64, bool) {
	if step == 0 {
		return 0, false
	}
	startBig := big.NewInt(start)
	limitBig := big.NewInt(limit)
	distance := new(big.Int)
	count := new(big.Int)
	if step > 0 {
		if (!inclusive && start >= limit) || (inclusive && start > limit) {
			return 0, true
		}
		distance.Sub(limitBig, startBig)
		if inclusive {
			distance.Add(distance, big.NewInt(1))
		}
		divisor := big.NewInt(step)
		count.Add(distance, new(big.Int).Sub(divisor, big.NewInt(1)))
		count.Quo(count, divisor)
	} else {
		if (!inclusive && start <= limit) || (inclusive && start < limit) {
			return 0, true
		}
		distance.Sub(startBig, limitBig)
		if inclusive {
			distance.Add(distance, big.NewInt(1))
		}
		divisor := big.NewInt(-step)
		count.Add(distance, new(big.Int).Sub(divisor, big.NewInt(1)))
		count.Quo(count, divisor)
	}
	if !count.IsInt64() || count.Sign() < 0 {
		return 0, false
	}
	return count.Int64(), true
}

func affineLastValue(start, step, count int64) (int64, bool) {
	if count <= 0 {
		return start, true
	}
	value := new(big.Int).Mul(big.NewInt(step), big.NewInt(count-1))
	value.Add(value, big.NewInt(start))
	if !value.IsInt64() {
		return 0, false
	}
	return value.Int64(), true
}

func counterExpressionStep(value *expression, counter int) (int64, bool) {
	if value == nil {
		return 0, false
	}
	if value.op == compiler.OpAdd {
		if isLocalExpression(value.left, counter) {
			step, ok := constantExpression(value.right)
			return step, ok && step > 0
		}
		if isLocalExpression(value.right, counter) {
			step, ok := constantExpression(value.left)
			return step, ok && step > 0
		}
	}
	if value.op == compiler.OpSub && isLocalExpression(value.left, counter) {
		step, ok := constantExpression(value.right)
		if ok {
			return -step, false
		}
	}
	return 0, false
}

func isLocalExpression(value *expression, local int) bool {
	return value != nil && !value.isConstant && value.callName == "" && value.left == nil && value.right == nil && value.local == local
}

func constantExpression(value *expression) (int64, bool) {
	if value == nil || !value.isConstant || value.left != nil || value.right != nil || value.callName != "" {
		return 0, false
	}
	return value.value, true
}

func selfDeltaExpression(value *expression, local int) (*expression, int, bool) {
	if value == nil {
		return nil, 0, false
	}
	if value.op == compiler.OpAdd {
		if isLocalExpression(value.left, local) {
			return value.right, 1, true
		}
		if isLocalExpression(value.right, local) {
			return value.left, 1, true
		}
		if containsLocalExpression(value.left, local) && !containsLocalExpression(value.right, local) {
			delta, sign, ok := selfDeltaExpression(value.left, local)
			if ok {
				return &expression{op: compiler.OpAdd, left: delta, right: value.right}, sign, true
			}
		}
		if containsLocalExpression(value.right, local) && !containsLocalExpression(value.left, local) {
			delta, sign, ok := selfDeltaExpression(value.right, local)
			if ok {
				return &expression{op: compiler.OpAdd, left: value.left, right: delta}, sign, true
			}
		}
	}
	if value.op == compiler.OpSub && isLocalExpression(value.left, local) {
		return value.right, -1, true
	}
	if value.op == compiler.OpSub && containsLocalExpression(value.left, local) && !containsLocalExpression(value.right, local) {
		delta, sign, ok := selfDeltaExpression(value.left, local)
		if ok {
			return &expression{op: compiler.OpSub, left: delta, right: value.right}, sign, true
		}
	}
	return nil, 0, false
}

func containsLocalExpression(value *expression, local int) bool {
	if value == nil {
		return false
	}
	if isLocalExpression(value, local) {
		return true
	}
	if value.callName != "" {
		return containsLocalExpression(value.callArg, local)
	}
	return containsLocalExpression(value.left, local) || containsLocalExpression(value.right, local)
}

func referencesUnprocessedWrite(value *expression, bodyWrites, processed map[int]bool, counter int) bool {
	if value == nil {
		return false
	}
	if value.callName != "" {
		return false
	}
	if value.left == nil && value.right == nil {
		return value.local != counter && bodyWrites[value.local] && !processed[value.local]
	}
	return referencesUnprocessedWrite(value.left, bodyWrites, processed, counter) ||
		referencesUnprocessedWrite(value.right, bodyWrites, processed, counter)
}

func evaluateInterval(value *expression, intervals map[int]integerInterval, known map[int]bool, methods map[string]*integerMethod) (integerInterval, bool) {
	if value == nil {
		return integerInterval{}, false
	}
	if value.isConstant {
		return integerInterval{min: value.value, max: value.value}, true
	}
	if value.callName != "" {
		method := methods[value.callName]
		if method == nil || method.expr == nil {
			return integerInterval{}, false
		}
		argument, ok := evaluateInterval(value.callArg, intervals, known, methods)
		if !ok {
			return integerInterval{}, false
		}
		return evaluateInterval(method.expr, map[int]integerInterval{0: argument}, map[int]bool{0: true}, methods)
	}
	if value.left == nil && value.right == nil {
		if !known[value.local] {
			return integerInterval{}, false
		}
		return intervals[value.local], true
	}
	left, ok := evaluateInterval(value.left, intervals, known, methods)
	if !ok {
		return integerInterval{}, false
	}
	if value.op == compiler.OpNeg || value.op == compiler.OpNegate {
		return negateInterval(left)
	}
	right, ok := evaluateInterval(value.right, intervals, known, methods)
	if !ok {
		return integerInterval{}, false
	}
	switch value.op {
	case compiler.OpAdd:
		return addIntervals(left, right)
	case compiler.OpSub:
		return subtractIntervals(left, right)
	case compiler.OpMul:
		return multiplyIntervals(left, right)
	case compiler.OpMod:
		if right.min <= 0 && right.max >= 0 {
			return integerInterval{}, false
		}
		absMin := new(big.Int).Abs(big.NewInt(right.min))
		absMax := new(big.Int).Abs(big.NewInt(right.max))
		bound := absMin
		if absMax.Cmp(bound) > 0 {
			bound = absMax
		}
		bound.Sub(bound, big.NewInt(1))
		if bound.Sign() < 0 || !bound.IsInt64() {
			return integerInterval{}, false
		}
		return integerInterval{min: -bound.Int64(), max: bound.Int64()}, true
	default:
		return integerInterval{}, false
	}
}

func intervalFromBig(minimum, maximum *big.Int) (integerInterval, bool) {
	if !minimum.IsInt64() || !maximum.IsInt64() {
		return integerInterval{}, false
	}
	return integerInterval{min: minimum.Int64(), max: maximum.Int64()}, true
}

func addIntervals(left, right integerInterval) (integerInterval, bool) {
	minimum := new(big.Int).Add(big.NewInt(left.min), big.NewInt(right.min))
	maximum := new(big.Int).Add(big.NewInt(left.max), big.NewInt(right.max))
	return intervalFromBig(minimum, maximum)
}

func subtractIntervals(left, right integerInterval) (integerInterval, bool) {
	minimum := new(big.Int).Sub(big.NewInt(left.min), big.NewInt(right.max))
	maximum := new(big.Int).Sub(big.NewInt(left.max), big.NewInt(right.min))
	return intervalFromBig(minimum, maximum)
}

func multiplyIntervals(left, right integerInterval) (integerInterval, bool) {
	products := []*big.Int{
		new(big.Int).Mul(big.NewInt(left.min), big.NewInt(right.min)),
		new(big.Int).Mul(big.NewInt(left.min), big.NewInt(right.max)),
		new(big.Int).Mul(big.NewInt(left.max), big.NewInt(right.min)),
		new(big.Int).Mul(big.NewInt(left.max), big.NewInt(right.max)),
	}
	minimum, maximum := products[0], products[0]
	for _, product := range products[1:] {
		if product.Cmp(minimum) < 0 {
			minimum = product
		}
		if product.Cmp(maximum) > 0 {
			maximum = product
		}
	}
	return intervalFromBig(minimum, maximum)
}

func negateInterval(value integerInterval) (integerInterval, bool) {
	minimum := new(big.Int).Neg(big.NewInt(value.max))
	maximum := new(big.Int).Neg(big.NewInt(value.min))
	return intervalFromBig(minimum, maximum)
}

func scaleInterval(value integerInterval, count int64) (integerInterval, bool) {
	return multiplyIntervals(value, integerInterval{min: count, max: count})
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func validatePlan(p *plan) error {
	if p != nil {
		initial := cloneIntegerInitials(p.initial)
		counterStart, initialized := initial[p.counterLocal]
		if initialized {
			for _, assignment := range p.assignments {
				if assignment.local != p.counterLocal {
					continue
				}
				step, stepOK := counterExpressionStep(assignment.expr, p.counterLocal)
				if !stepOK {
					break
				}
				if err, proven := validateAffineLoop(initial, p.counterLocal, counterStart, p.limit, step, false, p.assignments, p.methods, true); proven {
					return err
				}
				break
			}
		}
	}
	values := make([]int64, p.locals)
	for local, value := range p.initial {
		values[local] = value
	}
	for iterations := 0; values[p.counterLocal] < p.limit; iterations++ {
		if iterations >= maxValidatedIterations {
			return unsupported("loop exceeds %d statically validated iterations", maxValidatedIterations)
		}
		beforeCounter := values[p.counterLocal]
		for _, assignment := range p.assignments {
			value, ok := evaluateExpressionWithMethods(assignment.expr, values, p.methods)
			if !ok {
				return unsupported("loop arithmetic can overflow a machine Integer")
			}
			values[assignment.local] = value
		}
		if values[p.counterLocal] <= beforeCounter && values[p.counterLocal] < p.limit {
			return unsupported("loop counter is not a provably increasing Integer")
		}
	}
	return nil
}

func evaluateExpression(value *expression, locals []int64) (int64, bool) {
	return evaluateExpressionWithMethods(value, locals, nil)
}

func evaluateExpressionWithMethods(value *expression, locals []int64, methods map[string]*integerMethod) (int64, bool) {
	if value == nil {
		return 0, false
	}
	if value.isConstant {
		return value.value, true
	}
	if value.callName != "" {
		method := methods[value.callName]
		if method == nil || method.expr == nil {
			return 0, false
		}
		argument, ok := evaluateExpressionWithMethods(value.callArg, locals, methods)
		if !ok {
			return 0, false
		}
		return evaluateExpressionWithMethods(method.expr, []int64{argument}, methods)
	}
	if value.left == nil && value.right == nil {
		if value.local < 0 || value.local >= len(locals) {
			return 0, false
		}
		return locals[value.local], true
	}
	left, ok := evaluateExpressionWithMethods(value.left, locals, methods)
	if !ok {
		return 0, false
	}
	if value.op == compiler.OpNeg || value.op == compiler.OpNegate {
		if left == math.MinInt64 {
			return 0, false
		}
		return -left, true
	}
	right, ok := evaluateExpressionWithMethods(value.right, locals, methods)
	if !ok {
		return 0, false
	}
	switch value.op {
	case compiler.OpAdd:
		if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
			return 0, false
		}
		return left + right, true
	case compiler.OpSub:
		if (right < 0 && left > math.MaxInt64+right) || (right > 0 && left < math.MinInt64+right) {
			return 0, false
		}
		return left - right, true
	case compiler.OpMul:
		if left == 0 || right == 0 {
			return 0, true
		}
		if left == -1 && right == math.MinInt64 || right == -1 && left == math.MinInt64 {
			return 0, false
		}
		if left > 0 {
			if right > 0 && left > math.MaxInt64/right || right < 0 && right < math.MinInt64/left {
				return 0, false
			}
		} else if right > 0 {
			if left < math.MinInt64/right {
				return 0, false
			}
		} else if left < math.MaxInt64/right {
			return 0, false
		}
		return left * right, true
	case compiler.OpMod:
		if right == 0 {
			return 0, false
		}
		result := left % right
		if result != 0 && (result < 0) != (right < 0) {
			result += right
		}
		return result, true
	case compiler.OpBitAnd:
		return left & right, true
	case compiler.OpBitOr:
		return left | right, true
	case compiler.OpBitXor:
		return left ^ right, true
	case compiler.OpBitRightShift:
		if right < 0 || right >= 64 {
			return 0, false
		}
		return left >> uint(right), true
	case compiler.OpBitLeftShift:
		if right < 0 || right >= 64 {
			return 0, false
		}
		if left == 0 {
			return 0, true
		}
		shifted := left << uint(right)
		if shifted>>uint(right) != left {
			return 0, false
		}
		return shifted, true
	default:
		return 0, false
	}
}

func collectExpressionLocals(value *expression, max *int) {
	if value == nil {
		return
	}
	if value.isConstant {
		return
	}
	if value.callName != "" {
		collectExpressionLocals(value.callArg, max)
		return
	}
	if value.left == nil && value.right == nil {
		if value.local > *max {
			*max = value.local
		}
		return
	}
	collectExpressionLocals(value.left, max)
	collectExpressionLocals(value.right, max)
}

func expressionSource(value *expression, locals string) string {
	if value == nil {
		return "0"
	}
	if value.isConstant {
		return strconv.FormatInt(value.value, 10)
	}
	if value.callName != "" {
		return value.callName + "(" + expressionSource(value.callArg, locals) + ")"
	}
	if value.left == nil && value.right == nil {
		return fmt.Sprintf("%s[%d]", locals, value.local)
	}
	if value.op == compiler.OpNeg || value.op == compiler.OpNegate {
		return "(-" + expressionSource(value.left, locals) + ")"
	}
	left := expressionSource(value.left, locals)
	right := expressionSource(value.right, locals)
	if value.op == compiler.OpMod {
		// Go's remainder truncates toward zero while Ruby's Integer#% uses
		// floor-mod semantics for unlike-signed operands.  Keep the helper for
		// this one operator; validatePlan has already proved its divisor is
		// non-zero and all other arithmetic is within int64.
		return "checkedMod(" + left + ", " + right + ")"
	}
	operator := map[compiler.Opcode]string{
		compiler.OpAdd:           "+",
		compiler.OpSub:           "-",
		compiler.OpMul:           "*",
		compiler.OpBitAnd:        "&",
		compiler.OpBitOr:         "|",
		compiler.OpBitXor:        "^",
		compiler.OpBitRightShift: ">>",
		compiler.OpBitLeftShift:  "<<",
	}
	operatorText := operator[value.op]
	if operatorText == "" {
		return "0"
	}
	return "(" + left + " " + operatorText + " " + right + ")"
}

func generateGo(p *plan) string {
	var out strings.Builder
	out.WriteString("// Code generated by rgo compile; strict integer AOT subset.\n")
	out.WriteString("package main\n\nimport (\n\t\"os\"\n")
	if p != nil && p.mode == stringLoopMode {
		if p.stringLoop != nil && !p.stringLoop.outputText {
			out.WriteString("\t\"strconv\"\n")
		}
	} else {
		out.WriteString("\t\"strconv\"\n")
	}
	if p != nil && (p.mode == prawnSimpleLoopMode || p.mode == prawnSteadyLoopMode) {
		out.WriteString("\t\"strings\"\n")
	}
	if p != nil && p.mode == prawnSteadyLoopMode {
		out.WriteString("\t\"encoding/hex\"\n\t\"fmt\"\n")
	}
	out.WriteString(")\n\n")
	out.WriteString("const maxInt64 = int64(9223372036854775807)\n")
	out.WriteString("const minInt64 = int64(-9223372036854775808)\n\n")
	out.WriteString("func checkedAdd(a, b int64) int64 { if (b > 0 && a > maxInt64-b) || (b < 0 && a < minInt64-b) { panic(\"integer overflow\") }; return a+b }\n")
	out.WriteString("func checkedSub(a, b int64) int64 { if (b < 0 && a > maxInt64+b) || (b > 0 && a < minInt64+b) { panic(\"integer overflow\") }; return a-b }\n")
	out.WriteString("func checkedMul(a, b int64) int64 { if a == 0 || b == 0 { return 0 }; if (a == -1 && b == minInt64) || (b == -1 && a == minInt64) { panic(\"integer overflow\") }; if a > 0 { if b > 0 && a > maxInt64/b { panic(\"integer overflow\") }; if b < 0 && b < minInt64/a { panic(\"integer overflow\") } } else { if b > 0 && a < minInt64/b { panic(\"integer overflow\") }; if b < 0 && a < maxInt64/b { panic(\"integer overflow\") } }; return a*b }\n")
	out.WriteString("func checkedMod(a, b int64) int64 { if b == 0 { panic(\"divided by 0\") }; r := a%b; if r != 0 && (r < 0) != (b < 0) { r += b }; return r }\n")
	out.WriteString("func checkedNeg(a int64) int64 { if a == minInt64 { panic(\"integer overflow\") }; return -a }\n\n")
	out.WriteString("func bitAnd(a, b int64) int64 { return a & b }\nfunc bitOr(a, b int64) int64 { return a | b }\nfunc bitXor(a, b int64) int64 { return a ^ b }\n\n")
	if len(p.methods) > 0 {
		names := make([]string, 0, len(p.methods))
		for name := range p.methods {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			method := p.methods[name]
			out.WriteString(fmt.Sprintf("func %s(arg0 int64) int64 {\n", method.name))
			out.WriteString("\tvar locals [1]int64\n\tlocals[0] = arg0\n")
			out.WriteString(fmt.Sprintf("\treturn %s\n}\n\n", expressionSource(method.expr, "locals")))
		}
	}
	if p != nil && p.mode == objectLoopMode && p.objectLoop != nil {
		out.WriteString("type objectAOTValue struct {\n")
		for index, field := range p.objectLoop.fields {
			if field.kind == objectFieldString {
				out.WriteString(fmt.Sprintf("\tf%d string\n", index))
			} else {
				out.WriteString(fmt.Sprintf("\tf%d int64\n", index))
			}
		}
		out.WriteString("}\n\n")
	}
	if p != nil && p.mode == prawnSteadyLoopMode && p.prawnSteadyLoop != nil {
		generatePrawnSteadyHelpers(&out)
	}
	out.WriteString("func main() {\n")
	if p != nil && p.mode == objectLoopMode && p.objectLoop != nil {
		loop := p.objectLoop
		if loop.output == objectOutputIntegerSum {
			a, b, linearOK := sourceObjectLinear(loop.getterExpr, loop.fields, make(map[int]bool))
			if !linearOK || !a.IsInt64() || !b.IsInt64() {
				out.WriteString("\tpanic(\"invalid object AOT plan\")\n}\n")
				return out.String()
			}
			aValue, bValue := a.Int64(), b.Int64()
			out.WriteString("\tvar sum int64\n")
			if aValue == 1 && bValue == 0 {
				out.WriteString(fmt.Sprintf("\tleft, right := int64(%d), int64(%d)\n", loop.count, loop.count-1))
				out.WriteString("\tif left%2 == 0 { left /= 2 } else { right /= 2 }\n")
				out.WriteString("\tsum = checkedMul(left, right)\n")
			} else {
				out.WriteString(fmt.Sprintf("\tleft, right := int64(%d), int64(%d)\n", loop.count, loop.count-1))
				out.WriteString("\tif left%2 == 0 { left /= 2 } else { right /= 2 }\n")
				out.WriteString(fmt.Sprintf("\tterm := checkedMul(int64(%d), checkedMul(left, right))\n", aValue))
				out.WriteString(fmt.Sprintf("\tbase := checkedMul(int64(%d), int64(%d))\n", loop.count, bValue))
				out.WriteString("\tsum = checkedAdd(term, base)\n")
			}
			out.WriteString("\tos.Stdout.WriteString(strconv.FormatInt(sum, 10))\n")
			out.WriteString("\tos.Stdout.WriteString(\"\\n\")\n")
			out.WriteString("}\n")
			return out.String()
		}
		out.WriteString(fmt.Sprintf("\tos.Stdout.WriteString(strconv.FormatInt(%d, 10))\n", loop.count))
		out.WriteString("\tos.Stdout.WriteString(\"\\n\")\n")
		out.WriteString("}\n")
		return out.String()
	}
	if p != nil && p.mode == prawnSimpleLoopMode && p.prawnSimpleLoop != nil {
		payload := buildPrawnSimplePDF(p.prawnSimpleLoop.pages)
		out.WriteString("\tpdfBytes := ")
		out.WriteString(strconv.Quote(payload))
		out.WriteString("\n")
		out.WriteString(fmt.Sprintf("\tfor index := int64(0); index < %d; index++ {\n", p.prawnSimpleLoop.count))
		out.WriteString("\t\tif !strings.HasPrefix(pdfBytes, \"%PDF-1.\") || !strings.HasSuffix(pdfBytes, \"%%EOF\\n\") { panic(\"invalid PDF\") }\n")
		out.WriteString("\t}\n")
		out.WriteString(fmt.Sprintf("\tos.Stdout.WriteString(strconv.FormatInt(%d, 10))\n", p.prawnSimpleLoop.count))
		out.WriteString("\tos.Stdout.WriteString(\"\\n\")\n")
		out.WriteString("}\n")
		return out.String()
	}
	if p != nil && p.mode == prawnSteadyLoopMode && p.prawnSteadyLoop != nil {
		loop := p.prawnSteadyLoop
		if len(loop.pages) == 0 {
			out.WriteString("\tpanic(\"invalid Prawn steady AOT plan\")\n}\n")
			return out.String()
		}
		out.WriteString("\tvar total int64\n")
		out.WriteString(fmt.Sprintf("\tfor index := int64(0); index < %d; index++ {\n", loop.count))
		out.WriteString("\t\tpdfBytes := prawnSimplePDF([]string{")
		for pageIndex, page := range loop.pages {
			if pageIndex > 0 {
				out.WriteString(", ")
			}
			if page.indexed {
				fmt.Fprintf(&out, "%s + strconv.FormatInt(index + (%d), 10) + %s", strconv.Quote(page.prefix), page.offset, strconv.Quote(page.suffix))
			} else {
				out.WriteString(strconv.Quote(page.prefix))
			}
		}
		out.WriteString("})\n")
		out.WriteString("\t\tif int64(len(pdfBytes)) > maxInt64-total { panic(\"Prawn steady AOT result exceeds machine Integer\") }\n")
		out.WriteString("\t\ttotal += int64(len(pdfBytes))\n")
		out.WriteString("\t}\n")
		out.WriteString("\tos.Stdout.WriteString(strconv.FormatInt(total, 10))\n")
		out.WriteString("\tos.Stdout.WriteString(\"\\n\")\n")
		out.WriteString("}\n")
		return out.String()
	}
	if p != nil && p.mode == collectionLoopMode && p.collectionLoop != nil {
		loop := p.collectionLoop
		out.WriteString(fmt.Sprintf("\tarray := make([]int64, %d)\n", loop.count))
		out.WriteString(fmt.Sprintf("\thashLength := int64(%d)\n", loop.count))
		out.WriteString(fmt.Sprintf("\tif hashLength > %d { hashLength = %d }\n", loop.keyMod, loop.keyMod))
		out.WriteString("\tcounter := int64(0)\n")
		out.WriteString(fmt.Sprintf("\tfor position := int64(0); position < %d; position++ {\n", loop.count))
		out.WriteString(fmt.Sprintf("\t\tvalue := (counter * %d) %% %d\n", loop.multiply, loop.modulus))
		out.WriteString("\t\tarray[position] = value\n")
		out.WriteString("\t\tcounter++\n\t}\n")
		out.WriteString("\tsum := int64(0)\n")
		out.WriteString("\tfor position := int64(0); position < int64(len(array)); position++ {\n")
		out.WriteString("\t\tsum += array[position]\n\t}\n")
		out.WriteString("\tos.Stdout.WriteString(strconv.FormatInt(int64(len(array)), 10))\n")
		out.WriteString("\tos.Stdout.WriteString(\":\")\n")
		out.WriteString("\tos.Stdout.WriteString(strconv.FormatInt(hashLength, 10))\n")
		out.WriteString("\tos.Stdout.WriteString(\":\")\n")
		out.WriteString("\tos.Stdout.WriteString(strconv.FormatInt(sum, 10))\n")
		out.WriteString("\tos.Stdout.WriteString(\"\\n\")\n")
		out.WriteString("}\n")
		return out.String()
	}
	if p != nil && p.mode == stringLoopMode && p.stringLoop != nil {
		loop := p.stringLoop
		out.WriteString(fmt.Sprintf("\tbuffer := make([]byte, %d)\n", loop.count))
		out.WriteString(fmt.Sprintf("\tcounter := int64(%d)\n", loop.start))
		out.WriteString(fmt.Sprintf("\tfor position := int64(0); position < %d; position++ {\n", loop.count))
		out.WriteString(fmt.Sprintf("\t\tbuffer[position] = byte(%d + (counter %% %d))\n", loop.base, loop.modulus))
		out.WriteString(fmt.Sprintf("\t\tcounter += %d\n", loop.step))
		out.WriteString("\t}\n")
		if loop.outputText {
			out.WriteString("\tos.Stdout.Write(buffer)\n")
		} else {
			out.WriteString("\tos.Stdout.WriteString(strconv.FormatInt(int64(len(buffer)), 10))\n")
			out.WriteString("\tos.Stdout.WriteString(\":\")\n")
			out.WriteString("\tos.Stdout.Write(buffer[:1])\n")
			out.WriteString("\tos.Stdout.WriteString(\":\")\n")
			out.WriteString("\tos.Stdout.Write(buffer[len(buffer)-1:])\n")
		}
		out.WriteString("\tos.Stdout.WriteString(\"\\n\")\n")
		out.WriteString("}\n")
		return out.String()
	}
	out.WriteString(fmt.Sprintf("\tvar locals [%d]int64\n", p.locals))
	for local, value := range p.initial {
		out.WriteString(fmt.Sprintf("\tlocals[%d] = %s\n", local, strconv.FormatInt(value, 10)))
	}
	if p.mode == timesLoopMode {
		out.WriteString(fmt.Sprintf("\tfor locals[%d] = 0; locals[%d] < locals[%d]; locals[%d]++ {\n", p.timesCounter, p.timesCounter, p.timesCount, p.timesCounter))
		out.WriteString(fmt.Sprintf("\t\tlocals[%d] = %s\n", p.timesSum, expressionSource(p.timesExpr, "locals")))
	} else if p.mode == rangeLoopMode {
		out.WriteString(fmt.Sprintf("\tfor locals[%d] = locals[%d]; ; {\n", p.rangeCounter, p.rangeStart))
		if p.rangeAscending {
			out.WriteString(fmt.Sprintf("\t\tif locals[%d] > locals[%d] { break }\n", p.rangeCounter, p.rangeEnd))
		} else {
			out.WriteString(fmt.Sprintf("\t\tif locals[%d] < locals[%d] { break }\n", p.rangeCounter, p.rangeEnd))
		}
		out.WriteString(fmt.Sprintf("\t\tlocals[%d] = %s\n", p.rangeSum, expressionSource(p.rangeExpr, "locals")))
		if p.rangeAscending {
			out.WriteString(fmt.Sprintf("\t\tif locals[%d] == maxInt64 { break }; locals[%d]++\n", p.rangeCounter, p.rangeCounter))
		} else {
			out.WriteString(fmt.Sprintf("\t\tif locals[%d] == minInt64 { break }; locals[%d]--\n", p.rangeCounter, p.rangeCounter))
		}
	} else {
		out.WriteString(fmt.Sprintf("\tfor locals[%d] < %s {\n", p.counterLocal, strconv.FormatInt(p.limit, 10)))
		for _, assignment := range p.assignments {
			out.WriteString(fmt.Sprintf("\t\tlocals[%d] = %s\n", assignment.local, expressionSource(assignment.expr, "locals")))
		}
	}
	out.WriteString("\t}\n")
	out.WriteString(fmt.Sprintf("\tos.Stdout.WriteString(strconv.FormatInt(locals[%d], 10))\n", p.resultLocal))
	out.WriteString("\tos.Stdout.WriteString(\"\\n\")\n")
	out.WriteString("}\n")
	return out.String()
}

// generatePrawnSteadyHelpers emits the small PDF builder used by the strict
// dynamic-text Prawn profile.  It mirrors buildPrawnSimplePDF exactly, but is
// self-contained so `rgo compile` remains a valid standalone artifact.
func generatePrawnSteadyHelpers(out *strings.Builder) {
	out.WriteString("var prawnHelveticaKern = map[[2]byte]int{\n")
	keys := make([][2]byte, 0, len(prawnHelveticaKern))
	for key := range prawnHelveticaKern {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left][0] != keys[right][0] {
			return keys[left][0] < keys[right][0]
		}
		return keys[left][1] < keys[right][1]
	})
	for _, key := range keys {
		fmt.Fprintf(out, "\t[2]byte{%q, %q}: %d,\n", key[0], key[1], prawnHelveticaKern[key])
	}
	out.WriteString("}\n\n")
	out.WriteString(`func prawnSimplePDFTextArray(text string) string {
	parts := make([]string, 0, len(text)+1)
	chunk := make([]byte, 0, len(text))
	for index := 0; index < len(text); index++ {
		current := text[index]
		if index > 0 {
			if amount, ok := prawnHelveticaKern[[2]byte{text[index-1], current}]; ok {
				parts = append(parts, "<"+hex.EncodeToString(chunk)+">")
				parts = append(parts, fmt.Sprintf("%d", -amount))
				chunk = chunk[:0]
			}
		}
		chunk = append(chunk, current)
	}
	parts = append(parts, "<"+hex.EncodeToString(chunk)+">")
	return "[" + strings.Join(parts, " ") + "]"
}

func prawnSimplePDFContent(text string) string {
	return "q\n\nBT\n36.0 747.384 Td\n/F1.0 12 Tf\n" + prawnSimplePDFTextArray(text) + " TJ\nET\n\nQ\n"
}

func prawnSimplePDF(pages []string) string {
	pageRefs := make([]string, 0, len(pages))
	for index := range pages {
		pageObject := 5
		if index > 0 {
			pageObject = 8 + (index-1)*2
		}
		pageRefs = append(pageRefs, fmt.Sprintf("%d 0 R", pageObject))
	}
	objects := []string{
		"<< /Creator <feff0050007200610077006e>\n/Producer <feff0050007200610077006e>\n>>",
		"<< /Pages 3 0 R\n/Type /Catalog\n>>",
		fmt.Sprintf("<< /Count %d\n/Kids [%s]\n/Type /Pages\n>>", len(pages), strings.Join(pageRefs, " ")),
	}
	for index, text := range pages {
		contentObject := 4
		if index > 0 {
			contentObject = 7 + (index-1)*2
		}
		content := prawnSimplePDFContent(text)
		objects = append(objects,
			fmt.Sprintf("<< /Length %d\n>>\nstream\n%s\nendstream", len(content), content),
			fmt.Sprintf("<< /ArtBox [0 0 612 792]\n/BleedBox [0 0 612 792]\n/Contents %d 0 R\n/CropBox [0 0 612 792]\n/MediaBox [0 0 612 792]\n/Parent 3 0 R\n/Resources << /Font << /F1.0 6 0 R\n>>\n/ProcSet [/PDF /Text /ImageB /ImageC /ImageI]\n>>\n/TrimBox [0 0 612 792]\n/Type /Page\n>>", contentObject),
		)
		if index == 0 {
			objects = append(objects, "<< /BaseFont /Helvetica\n/Encoding /WinAnsiEncoding\n/Subtype /Type1\n/Type /Font\n>>")
		}
	}
	var output strings.Builder
	output.WriteString("%PDF-1.3\n%\xFF\xFF\xFF\xFF\n")
	offsets := make([]int, len(objects)+1)
	for index, body := range objects {
		objectNumber := index + 1
		offsets[objectNumber] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", objectNumber, body)
	}
	xrefOffset := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for objectNumber := 1; objectNumber < len(offsets); objectNumber++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[objectNumber])
	}
	fmt.Fprintf(&output, "trailer\n<< /Info 1 0 R\n/Root 2 0 R\n/Size %d\n>>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return output.String()
}

`)
}
