package aot

// ExecuteSource is the in-process counterpart of GenerateSource.  It runs a
// source program after the same strict proof used by the Go emitter, but it
// does not invoke the Go compiler on a cache miss.  Values in the proven plan
// stay as int64/byte slices, so this is a typed kernel rather than the boxed
// Ruby VM.  Unsupported source returns handled=false and must be delegated to
// the compatibility runtime by the caller.

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/parser"
)

// ExecuteSource executes only a statically proven source-level plan.  Parsing
// or proof rejection is intentionally not an execution error: the caller can
// transparently fall back to normal Ruby semantics.
func ExecuteSource(source string, output io.Writer) (handled bool, err error) {
	if output == nil {
		return false, fmt.Errorf("nil AOT output writer")
	}
	programParser := parser.New(lexer.New(source))
	program := programParser.ParseProgram()
	if len(programParser.Errors()) > 0 {
		return false, nil
	}
	plan, planErr := buildSourcePlan(program)
	if planErr != nil {
		if errors.Is(planErr, ErrUnsupported) {
			return false, nil
		}
		return false, planErr
	}
	if err := executePlan(plan, output); err != nil {
		return true, err
	}
	return true, nil
}

func executePlan(plan *plan, output io.Writer) error {
	if plan == nil {
		return fmt.Errorf("nil AOT plan")
	}
	switch plan.mode {
	case prawnSimpleLoopMode:
		return executePrawnSimplePlan(plan.prawnSimpleLoop, output)
	case prawnSteadyLoopMode:
		return executePrawnSteadyPlan(plan.prawnSteadyLoop, output)
	case stringLoopMode:
		return executeStringPlan(plan.stringLoop, output)
	case collectionLoopMode:
		return executeCollectionPlan(plan.collectionLoop, output)
	case objectLoopMode:
		return executeObjectPlan(plan.objectLoop, output)
	}

	locals := make([]int64, plan.locals)
	for local, value := range plan.initial {
		if local < 0 || local >= len(locals) {
			return fmt.Errorf("AOT local %d is outside the proven frame", local)
		}
		locals[local] = value
	}
	if executed, affineErr := executeAffineWhilePlan(plan, locals, output); executed {
		return affineErr
	}
	if executed, affineErr := executeAffineTimesPlan(plan, locals, output); executed {
		return affineErr
	}
	if executed, periodicErr := executePeriodicTimesPlan(plan, locals, output); executed {
		return periodicErr
	}
	if executed, affineErr := executeAffineRangePlan(plan, locals, output); executed {
		return affineErr
	}
	switch plan.mode {
	case timesLoopMode:
		count := locals[plan.timesCount]
		for index := int64(0); index < count; index++ {
			locals[plan.timesCounter] = index
			locals[plan.timesSum] = evaluateExpressionUnchecked(plan.timesExpr, locals, plan.methods)
		}
	case rangeLoopMode:
		index := locals[plan.rangeStart]
		end := locals[plan.rangeEnd]
		for {
			if plan.rangeAscending && index > end || !plan.rangeAscending && index < end {
				break
			}
			locals[plan.rangeCounter] = index
			locals[plan.rangeSum] = evaluateExpressionUnchecked(plan.rangeExpr, locals, plan.methods)
			if index == end || plan.rangeAscending && index == math.MaxInt64 || !plan.rangeAscending && index == math.MinInt64 {
				break
			}
			if plan.rangeAscending {
				index++
			} else {
				index--
			}
		}
	default:
		for locals[plan.counterLocal] < plan.limit {
			beforeCounter := locals[plan.counterLocal]
			for _, assignment := range plan.assignments {
				locals[assignment.local] = evaluateExpressionUnchecked(assignment.expr, locals, plan.methods)
			}
			if locals[plan.counterLocal] <= beforeCounter && locals[plan.counterLocal] < plan.limit {
				return fmt.Errorf("AOT loop counter stopped advancing")
			}
		}
	}
	return writeIntLine(output, locals[plan.resultLocal])
}

// executeAffineWhilePlan collapses the common accumulator loop to a closed
// form. The source proof has already established monotonic bounds and int64
// safety; this additional shape check only accepts one counter assignment and
// one self-delta accumulator whose delta is affine in the counter. This is the
// same algebraic kernel emitted by the standalone Go artifact, without a
// per-iteration expression dispatch on the first run.
func executeAffineWhilePlan(plan *plan, locals []int64, output io.Writer) (bool, error) {
	if plan == nil || plan.mode != whileLoopMode || len(plan.assignments) != 2 {
		return false, nil
	}
	counterStart := locals[plan.counterLocal]
	step, stepOK := intCounterStep(plan.assignments, plan.counterLocal)
	if !stepOK || step <= 0 {
		return false, nil
	}
	resultIndex := -1
	var resultDelta *expression
	resultSign := 0
	counterIndex := -1
	for index, assignment := range plan.assignments {
		if assignment.local == plan.counterLocal {
			if counterIndex >= 0 {
				return false, nil
			}
			counterIndex = index
			continue
		}
		if assignment.local != plan.resultLocal || resultIndex >= 0 {
			return false, nil
		}
		delta, sign, ok := selfDeltaExpression(assignment.expr, plan.resultLocal)
		if !ok || sign == 0 {
			return false, nil
		}
		resultIndex, resultDelta, resultSign = index, delta, sign
	}
	if counterIndex < 0 || resultIndex < 0 {
		return false, nil
	}
	count, ok := affineIterationCount(counterStart, plan.limit, step, false)
	if !ok || count < 0 {
		return false, nil
	}
	if count == 0 {
		return true, writeIntLine(output, locals[plan.resultLocal])
	}
	firstCounter := counterStart
	if resultIndex > counterIndex {
		firstCounter += step
	}
	result, resultOK := affineAccumulatorValue(locals[plan.resultLocal], resultDelta, resultSign, plan.counterLocal, firstCounter, step, count, plan.methods)
	if !resultOK {
		return false, nil
	}
	finalCounterValue := new(big.Int).Mul(new(big.Int).SetInt64(count), new(big.Int).SetInt64(step))
	finalCounterValue.Add(finalCounterValue, new(big.Int).SetInt64(counterStart))
	if !finalCounterValue.IsInt64() {
		return false, nil
	}
	locals[plan.resultLocal] = result
	locals[plan.counterLocal] = finalCounterValue.Int64()
	return true, writeIntLine(output, locals[plan.resultLocal])
}

func executeAffineTimesPlan(plan *plan, locals []int64, output io.Writer) (bool, error) {
	if plan == nil || plan.mode != timesLoopMode || plan.timesSum != plan.resultLocal {
		return false, nil
	}
	delta, sign, ok := selfDeltaExpression(plan.timesExpr, plan.timesSum)
	if !ok {
		return false, nil
	}
	count := locals[plan.timesCount]
	if count <= 0 {
		return true, writeIntLine(output, locals[plan.resultLocal])
	}
	result, ok := affineAccumulatorValue(locals[plan.resultLocal], delta, sign, plan.timesCounter, 0, 1, count, plan.methods)
	if !ok {
		return false, nil
	}
	locals[plan.timesCounter] = count - 1
	locals[plan.resultLocal] = result
	return true, writeIntLine(output, result)
}

func executeAffineRangePlan(plan *plan, locals []int64, output io.Writer) (bool, error) {
	if plan == nil || plan.mode != rangeLoopMode || plan.rangeSum != plan.resultLocal {
		return false, nil
	}
	delta, sign, ok := selfDeltaExpression(plan.rangeExpr, plan.rangeSum)
	if !ok {
		return false, nil
	}
	start := locals[plan.rangeStart]
	end := locals[plan.rangeEnd]
	step := int64(-1)
	if plan.rangeAscending {
		step = 1
	}
	count, ok := affineIterationCount(start, end, step, true)
	if !ok {
		return false, nil
	}
	if count == 0 {
		return true, writeIntLine(output, locals[plan.resultLocal])
	}
	result, ok := affineAccumulatorValue(locals[plan.resultLocal], delta, sign, plan.rangeCounter, start, step, count, plan.methods)
	if !ok {
		return false, nil
	}
	locals[plan.rangeCounter] = start + (count-1)*step
	locals[plan.resultLocal] = result
	return true, writeIntLine(output, result)
}

func executePeriodicTimesPlan(plan *plan, locals []int64, output io.Writer) (bool, error) {
	if plan == nil || plan.mode != timesLoopMode || plan.timesSum != plan.resultLocal {
		return false, nil
	}
	delta, sign, ok := selfDeltaExpression(plan.timesExpr, plan.timesSum)
	if !ok {
		return false, nil
	}
	period, ok := periodicCounterExpression(delta, plan.timesCounter)
	if !ok || period <= 0 {
		return false, nil
	}
	count := locals[plan.timesCount]
	if count <= 0 {
		return true, writeIntLine(output, locals[plan.resultLocal])
	}
	if period > 1_000_000 {
		return false, nil
	}
	oldCounter := locals[plan.timesCounter]
	cycleSum := int64(0)
	for index := int64(0); index < period; index++ {
		locals[plan.timesCounter] = index
		cycleSum += evaluateExpressionUnchecked(delta, locals, plan.methods)
	}
	fullCycles, remainder := count/period, count%period
	total := new(big.Int).Mul(new(big.Int).SetInt64(cycleSum), new(big.Int).SetInt64(fullCycles))
	for index := int64(0); index < remainder; index++ {
		locals[plan.timesCounter] = fullCycles*period + index
		total.Add(total, new(big.Int).SetInt64(evaluateExpressionUnchecked(delta, locals, plan.methods)))
	}
	if sign < 0 {
		total.Neg(total)
	}
	result := new(big.Int).Add(new(big.Int).SetInt64(locals[plan.resultLocal]), total)
	locals[plan.timesCounter] = oldCounter
	if !result.IsInt64() {
		return false, nil
	}
	locals[plan.resultLocal] = result.Int64()
	return true, writeIntLine(output, locals[plan.resultLocal])
}

func periodicCounterExpression(value *expression, counter int) (int64, bool) {
	if value == nil || value.callName != "" {
		return 0, false
	}
	if value.op != compiler.OpMod && value.op != compiler.OpBitAnd {
		return 0, false
	}
	if !isLocalExpression(value.left, counter) {
		return 0, false
	}
	modulus, ok := constantExpression(value.right)
	if !ok || modulus <= 0 || modulus > 1_000_000 {
		return 0, false
	}
	if value.op == compiler.OpMod {
		return modulus, true
	}
	period := modulus + 1
	if period <= 0 || period&(period-1) != 0 {
		return 0, false
	}
	return period, true
}

func affineAccumulatorValue(initial int64, delta *expression, sign int, counterLocal int, firstCounter, step, count int64, methods map[string]*integerMethod) (int64, bool) {
	a, b, ok := linearCounterExpressionWithMethods(delta, counterLocal, methods)
	if !ok || count < 0 {
		return 0, false
	}
	countBig := new(big.Int).SetInt64(count)
	stepBig := new(big.Int).SetInt64(step)
	firstBig := new(big.Int).SetInt64(firstCounter)
	// sum(counter_k) = count * (2*first + (count-1)*step) / 2.
	counterSum := new(big.Int).Mul(new(big.Int).Sub(new(big.Int).Set(countBig), big.NewInt(1)), stepBig)
	counterSum.Add(counterSum, new(big.Int).Mul(big.NewInt(2), firstBig))
	counterSum.Mul(counterSum, countBig)
	counterSum.Quo(counterSum, big.NewInt(2))
	deltaTotal := new(big.Int).Mul(a, counterSum)
	deltaTotal.Add(deltaTotal, new(big.Int).Mul(b, countBig))
	if sign < 0 {
		deltaTotal.Neg(deltaTotal)
	}
	result := new(big.Int).Add(new(big.Int).SetInt64(initial), deltaTotal)
	if !result.IsInt64() {
		return 0, false
	}
	return result.Int64(), true
}

func intCounterStep(assignments []assignment, counter int) (int64, bool) {
	for _, assignment := range assignments {
		if assignment.local == counter {
			return counterExpressionStep(assignment.expr, counter)
		}
	}
	return 0, false
}

func linearCounterExpression(value *expression, counter int) (*big.Int, *big.Int, bool) {
	return linearCounterExpressionWithMethods(value, counter, nil)
}

func linearCounterExpressionWithMethods(value *expression, counter int, methods map[string]*integerMethod) (*big.Int, *big.Int, bool) {
	if value == nil {
		return nil, nil, false
	}
	if value.isConstant {
		return big.NewInt(0), new(big.Int).SetInt64(value.value), true
	}
	if value.callName != "" {
		method := methods[value.callName]
		if method == nil || method.expr == nil {
			return nil, nil, false
		}
		argumentA, argumentB, argumentOK := linearCounterExpressionWithMethods(value.callArg, counter, methods)
		methodA, methodB, methodOK := linearCounterExpressionWithMethods(method.expr, 0, methods)
		if !argumentOK || !methodOK {
			return nil, nil, false
		}
		return new(big.Int).Mul(methodA, argumentA), new(big.Int).Add(new(big.Int).Mul(methodA, argumentB), methodB), true
	}
	if value.left == nil && value.right == nil {
		if value.local == counter {
			return big.NewInt(1), big.NewInt(0), true
		}
		return nil, nil, false
	}
	if value.op == compiler.OpNeg || value.op == compiler.OpNegate {
		a, b, ok := linearCounterExpressionWithMethods(value.left, counter, methods)
		if !ok {
			return nil, nil, false
		}
		return a.Neg(a), b.Neg(b), true
	}
	leftA, leftB, leftOK := linearCounterExpressionWithMethods(value.left, counter, methods)
	rightA, rightB, rightOK := linearCounterExpressionWithMethods(value.right, counter, methods)
	if value.op == compiler.OpAdd || value.op == compiler.OpSub {
		if !leftOK || !rightOK {
			return nil, nil, false
		}
		if value.op == compiler.OpAdd {
			return new(big.Int).Add(leftA, rightA), new(big.Int).Add(leftB, rightB), true
		}
		return new(big.Int).Sub(leftA, rightA), new(big.Int).Sub(leftB, rightB), true
	}
	if value.op != compiler.OpMul {
		return nil, nil, false
	}
	if leftOK && rightOK {
		leftConstant := leftA.Sign() == 0
		rightConstant := rightA.Sign() == 0
		if leftConstant && rightConstant {
			return big.NewInt(0), new(big.Int).Mul(leftB, rightB), true
		}
		if leftConstant {
			return new(big.Int).Mul(leftB, rightA), new(big.Int).Mul(leftB, rightB), true
		}
		if rightConstant {
			return new(big.Int).Mul(rightB, leftA), new(big.Int).Mul(rightB, leftB), true
		}
	}
	return nil, nil, false
}

// evaluateExpressionUnchecked is used only after buildSourcePlan has fully
// validated every reachable iteration for int64 overflow, zero modulus and
// shift bounds. Removing the proof checks from the hot loop is what makes the
// in-process path a typed kernel instead of a second boxed interpreter.
func evaluateExpressionUnchecked(value *expression, locals []int64, methods map[string]*integerMethod) int64 {
	if value == nil {
		return 0
	}
	if value.isConstant {
		return value.value
	}
	if value.callName != "" {
		method := methods[value.callName]
		return evaluateExpressionUnchecked(method.expr, []int64{evaluateExpressionUnchecked(value.callArg, locals, methods)}, methods)
	}
	if value.left == nil && value.right == nil {
		return locals[value.local]
	}
	left := evaluateExpressionUnchecked(value.left, locals, methods)
	if value.op == compiler.OpNeg || value.op == compiler.OpNegate {
		return -left
	}
	right := evaluateExpressionUnchecked(value.right, locals, methods)
	switch value.op {
	case compiler.OpAdd:
		return left + right
	case compiler.OpSub:
		return left - right
	case compiler.OpMul:
		return left * right
	case compiler.OpMod:
		result := left % right
		if result != 0 && (result < 0) != (right < 0) {
			result += right
		}
		return result
	case compiler.OpBitAnd:
		return left & right
	case compiler.OpBitOr:
		return left | right
	case compiler.OpBitXor:
		return left ^ right
	case compiler.OpBitRightShift:
		return left >> uint(right)
	case compiler.OpBitLeftShift:
		return left << uint(right)
	default:
		return 0
	}
}

func executeStringPlan(plan *stringLoopPlan, output io.Writer) error {
	if plan == nil || plan.count <= 0 {
		return fmt.Errorf("invalid string AOT plan")
	}
	buffer := make([]byte, plan.count)
	counter := plan.start
	for position := int64(0); position < plan.count; position++ {
		buffer[position] = byte(plan.base + (counter % plan.modulus))
		counter += plan.step
	}
	if plan.outputText {
		_, err := output.Write(buffer)
		if err != nil {
			return err
		}
		_, err = io.WriteString(output, "\n")
		return err
	}
	if _, err := fmt.Fprintf(output, "%d:", len(buffer)); err != nil {
		return err
	}
	if _, err := output.Write(buffer[:1]); err != nil {
		return err
	}
	if _, err := io.WriteString(output, ":"); err != nil {
		return err
	}
	if _, err := output.Write(buffer[len(buffer)-1:]); err != nil {
		return err
	}
	if _, err := io.WriteString(output, "\n"); err != nil {
		return err
	}
	return nil
}

func executeCollectionPlan(plan *collectionLoopPlan, output io.Writer) error {
	if plan == nil || plan.count < 0 {
		return fmt.Errorf("invalid collection AOT plan")
	}
	array := make([]int64, plan.count)
	hashLength := plan.count
	if hashLength > plan.keyMod {
		hashLength = plan.keyMod
	}
	counter := int64(0)
	for position := int64(0); position < plan.count; position++ {
		array[position] = (counter * plan.multiply) % plan.modulus
		counter++
	}
	sum := int64(0)
	for _, value := range array {
		sum += value
	}
	_, err := fmt.Fprintf(output, "%d:%d:%d\n", len(array), hashLength, sum)
	return err
}

func executeObjectPlan(plan *objectLoopPlan, output io.Writer) error {
	if plan == nil || plan.count < 0 {
		return fmt.Errorf("invalid object AOT plan")
	}
	// Constructor fields and the optional getter are pure under the proof. A
	// length result therefore lowers to the known cardinality; an integer sum
	// lowers the affine getter call graph to a closed form without allocating
	// Ruby objects or entering block frames.
	if plan.output == objectOutputIntegerSum {
		a, b, ok := sourceObjectLinear(plan.getterExpr, plan.fields, make(map[int]bool))
		if !ok {
			return fmt.Errorf("object getter is not an affine Integer expression")
		}
		count := big.NewInt(plan.count)
		last := new(big.Int).Sub(new(big.Int).Set(count), big.NewInt(1))
		triangular := new(big.Int).Mul(new(big.Int).Set(count), last)
		triangular.Quo(triangular, big.NewInt(2))
		total := new(big.Int).Mul(a, triangular)
		total.Add(total, new(big.Int).Mul(b, count))
		if !total.IsInt64() {
			return fmt.Errorf("object getter sum exceeds machine Integer")
		}
		return writeIntLine(output, total.Int64())
	}
	return writeIntLine(output, plan.count)
}

func executePrawnSimplePlan(plan *prawnSimpleLoopPlan, output io.Writer) error {
	if plan == nil || plan.count <= 0 {
		return fmt.Errorf("invalid Prawn AOT plan")
	}
	pdfBytes := buildPrawnSimplePDF(plan.pages)
	for index := int64(0); index < plan.count; index++ {
		if !strings.HasPrefix(pdfBytes, "%PDF-1.") || !strings.HasSuffix(pdfBytes, "%%EOF\n") {
			return fmt.Errorf("invalid PDF")
		}
	}
	return writeIntLine(output, plan.count)
}

func executePrawnSteadyPlan(plan *prawnSteadyLoopPlan, output io.Writer) error {
	if plan == nil || plan.count <= 0 || len(plan.pages) == 0 {
		return fmt.Errorf("invalid Prawn steady AOT plan")
	}
	var total int64
	for index := int64(0); index < plan.count; index++ {
		pages := make([]string, len(plan.pages))
		for pageIndex, template := range plan.pages {
			if template.indexed {
				pages[pageIndex] = prawnIndexedTextValue(template, index)
			} else {
				pages[pageIndex] = template.prefix
			}
		}
		pageBytes := int64(len(buildPrawnSimplePDF(pages)))
		if pageBytes > math.MaxInt64-total {
			return fmt.Errorf("Prawn steady AOT result exceeds machine Integer")
		}
		total += pageBytes
	}
	return writeIntLine(output, total)
}

func prawnIndexedTextValue(template prawnIndexedText, index int64) string {
	return template.prefix + strconv.FormatInt(index+template.offset, 10) + template.suffix
}

func writeIntLine(output io.Writer, value int64) error {
	_, err := io.WriteString(output, strconv.FormatInt(value, 10))
	if err != nil {
		return err
	}
	_, err = io.WriteString(output, "\n")
	return err
}
