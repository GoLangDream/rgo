package aot

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/parser/ast"
)

// GenerateSource is the source-level entry for the strict AOT tier.  It is
// intentionally narrower than the compatibility compiler: top-level integer
// loops may call pure integer methods, ASCII buffer and typed collection loops
// avoid Ruby objects, and a closed-world object constructor/getter region can
// lower its proven method graph to a raw artifact. Every other Ruby construct
// is rejected and can still use the ordinary VM as a fallback.
func GenerateSource(source string) (string, error) {
	programParser := parser.New(lexer.New(source))
	program := programParser.ParseProgram()
	if errors := programParser.Errors(); len(errors) > 0 {
		return "", fmt.Errorf("parse error: %s", errors[0])
	}
	plan, err := buildSourcePlan(program)
	if err != nil {
		return "", err
	}
	return generateGo(plan), nil
}

func buildSourcePlan(program *ast.Program) (*plan, error) {
	if program == nil || len(program.Statements) == 0 {
		return nil, unsupported("empty program")
	}
	if objectPlan, matched, err := buildSourceObjectPlan(program); matched {
		if err != nil {
			return nil, err
		}
		return objectPlan, nil
	}
	methods := make(map[string]*integerMethod)
	top := make([]ast.Statement, 0, len(program.Statements))
	for _, statement := range program.Statements {
		definition, ok := statement.(*ast.ExpressionStatement)
		if ok {
			if method, methodOK := definition.Expression.(*ast.DefExpression); methodOK {
				parsed, err := parseSourceIntegerMethod(method)
				if err != nil {
					return nil, err
				}
				if _, exists := methods[parsed.name]; exists {
					return nil, unsupported("duplicate integer method %s", parsed.name)
				}
				methods[parsed.name] = parsed
				continue
			}
		}
		top = append(top, statement)
	}
	if len(top) < 3 {
		return nil, unsupported("source-level AOT expects initializers, a loop, and puts")
	}
	if prawnPlan, matched, err := buildSourcePrawnSteadyPlan(top); matched {
		if err != nil {
			return nil, err
		}
		return prawnPlan, nil
	}
	if prawnPlan, matched, err := buildSourcePrawnSimplePlan(top); matched {
		if err != nil {
			return nil, err
		}
		return prawnPlan, nil
	}
	if collectionPlan, matched, err := buildSourceCollectionPlan(top); matched {
		if err != nil {
			return nil, err
		}
		return collectionPlan, nil
	}
	if stringPlan, matched, err := buildSourceStringPlan(top); matched {
		if err != nil {
			return nil, err
		}
		return stringPlan, nil
	}
	if rangePlan, matched, err := buildSourceRangePlan(top, methods); matched {
		if err != nil {
			return nil, err
		}
		return rangePlan, nil
	}
	if timesPlan, matched, err := buildSourceTimesPlan(top, methods); matched {
		if err != nil {
			return nil, err
		}
		return timesPlan, nil
	}

	loopIndex := -1
	var loop *ast.WhileExpression
	for index, statement := range top {
		if expressionStatement, ok := statement.(*ast.ExpressionStatement); ok {
			if candidate, candidateOK := expressionStatement.Expression.(*ast.WhileExpression); candidateOK {
				if loop != nil {
					return nil, unsupported("only one while loop is supported")
				}
				loopIndex = index
				loop = candidate
			}
		}
	}
	if loop == nil || loopIndex <= 0 || loopIndex >= len(top)-1 || loopIndex+2 != len(top) {
		return nil, unsupported("source-level AOT expects one while loop before puts")
	}

	localIDs := make(map[string]int)
	nextLocal := 0
	localID := func(name string) int {
		if id, ok := localIDs[name]; ok {
			return id
		}
		id := nextLocal
		nextLocal++
		localIDs[name] = id
		return id
	}
	initial := make(map[int]int64)
	for _, statement := range top[:loopIndex] {
		assignment, ok := sourceAssignmentStatement(statement)
		if !ok || assignment.name == "" {
			return nil, unsupported("statements before the loop must be integer assignments")
		}
		value, ok := sourceIntegerConstant(assignment.value)
		if !ok {
			return nil, unsupported("initializer %s must be an immutable Integer", assignment.name)
		}
		initial[localID(assignment.name)] = value
	}

	counterName, limit, limitName, ok := sourceWhileCondition(loop.Condition)
	if !ok {
		return nil, unsupported("while condition must be counter < immutable Integer")
	}
	counterLocal := localID(counterName)
	if limitName != "" {
		limitLocal := localID(limitName)
		value, initialized := initial[limitLocal]
		if !initialized {
			return nil, unsupported("loop upper bound %s must be initialized", limitName)
		}
		limit = value
	}

	assignments := make([]assignment, 0, len(loop.Body.Statements))
	for _, statement := range loop.Body.Statements {
		parsed, ok := sourceAssignmentStatement(statement)
		if !ok || parsed.name == "" {
			return nil, unsupported("while body must contain only local integer assignments")
		}
		expr, err := sourceIntegerExpression(parsed.value, localIDs, localID, methods)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, assignment{local: localID(parsed.name), expr: expr})
	}
	if len(assignments) == 0 {
		return nil, unsupported("while body is empty")
	}

	resultLocal, ok := sourcePutsLocal(top[loopIndex+1], localIDs, localID)
	if !ok {
		return nil, unsupported("program must finish with puts of an integer local")
	}
	maxLocal := counterLocal
	for local := range initial {
		if local > maxLocal {
			maxLocal = local
		}
	}
	for _, item := range assignments {
		if item.local > maxLocal {
			maxLocal = item.local
		}
		collectExpressionLocals(item.expr, &maxLocal)
	}
	if resultLocal > maxLocal {
		maxLocal = resultLocal
	}
	if nextLocal > maxLocal+1 {
		maxLocal = nextLocal - 1
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
		methods:      methods,
	}
	if err := validatePlan(compiled); err != nil {
		return nil, err
	}
	return compiled, nil
}

// buildSourcePrawnSteadyPlan recognizes the dynamic-text Prawn workload used
// by the end-to-end benchmark:
//
//	require "prawn"
//	total = 0
//	COUNT.times do |index|
//	  pdf = Prawn::Document.new
//	  pdf.text "prefix #{index} suffix"
//	  pdf.start_new_page
//	  pdf.text "prefix #{index + OFFSET} suffix"
//	  total += pdf.render.bytesize
//	end
//	puts total
//
// This is intentionally a closed-world typed kernel, not a general Prawn
// optimizer. It accepts one or more ASCII text templates separated by
// optional page breaks, a default document, no options or blocks on the Prawn
// calls, and observes only render.bytesize. Any other shape remains on the
// compatibility VM.
func buildSourcePrawnSteadyPlan(top []ast.Statement) (*plan, bool, error) {
	if len(top) != 4 {
		return nil, false, nil
	}
	if !sourcePrawnRequireStatement(top[0]) {
		return nil, false, nil
	}
	totalAssignment, ok := sourceAssignmentStatement(top[1])
	if !ok || totalAssignment.name == "" {
		return nil, false, nil
	}
	initialTotal, ok := sourceIntegerConstant(totalAssignment.value)
	if !ok || initialTotal != 0 {
		return nil, false, nil
	}
	timesStatement, ok := top[2].(*ast.ExpressionStatement)
	if !ok || timesStatement.Expression == nil {
		return nil, false, nil
	}
	timesCall, ok := timesStatement.Expression.(*ast.MethodCall)
	if !ok || timesCall.Method == nil || timesCall.Method.Value != "times" || timesCall.Receiver == nil ||
		len(timesCall.Args) != 0 || len(timesCall.KeywordArgs) != 0 || timesCall.Block == nil {
		return nil, false, nil
	}
	count, ok := sourceIntegerConstant(timesCall.Receiver)
	if !ok || count <= 0 || count > maxValidatedIterations {
		return nil, true, unsupported("Prawn steady loop count is outside the strict compiled range")
	}
	block := timesCall.Block
	if block == nil || len(block.Params) > 1 ||
		block.RestParam != nil || block.BlockParam != nil || len(block.KeywordParams) != 0 || block.KeywordRestParam != nil ||
		len(block.BlockLocals) != 0 || len(block.Statements) < 3 {
		return nil, false, nil
	}
	indexName := ""
	if len(block.Params) == 1 {
		if block.Params[0] == nil || block.Params[0].Value == "" {
			return nil, false, nil
		}
		indexName = block.Params[0].Value
	}
	if len(block.ParamPatterns) > 0 && (len(block.ParamPatterns) != len(block.Params) || block.ParamPatterns[0] != nil) {
		return nil, false, nil
	}
	if len(block.ParamDefaults) > 0 && (len(block.ParamDefaults) != len(block.Params) || block.ParamDefaults[0] != nil) {
		return nil, false, nil
	}
	pdfAssignment, ok := sourceAssignmentStatement(block.Statements[0])
	if !ok || pdfAssignment.name == "" || !sourcePrawnDocumentNew(pdfAssignment.value) {
		return nil, false, nil
	}
	totalUpdate, ok := sourceAssignmentStatement(block.Statements[len(block.Statements)-1])
	if !ok || totalUpdate.name != totalAssignment.name || !sourcePrawnRenderBytesize(totalUpdate.value, pdfAssignment.name, totalAssignment.name) {
		return nil, false, nil
	}
	pages := make([]prawnIndexedText, 0, (len(block.Statements)-1+1)/2)
	wantText := true
	for index := 1; index < len(block.Statements)-1; index++ {
		if wantText {
			text, textOK := sourcePrawnIndexedTextStatement(block.Statements[index], pdfAssignment.name, indexName)
			if !textOK || !sourcePrawnIndexedTextSafe(text, count) {
				return nil, false, nil
			}
			pages = append(pages, text)
			wantText = false
			continue
		}
		if !sourcePrawnNoOptionsCall(block.Statements[index], pdfAssignment.name, "start_new_page") {
			return nil, false, nil
		}
		wantText = true
	}
	if wantText || len(pages) == 0 {
		return nil, false, nil
	}
	putsStatement, ok := top[3].(*ast.ExpressionStatement)
	if !ok || putsStatement.Expression == nil {
		return nil, false, nil
	}
	putsCall, ok := putsStatement.Expression.(*ast.MethodCall)
	if !ok || putsCall.Receiver != nil || putsCall.Method == nil || putsCall.Method.Value != "puts" ||
		len(putsCall.Args) != 1 || len(putsCall.KeywordArgs) != 0 || putsCall.Block != nil {
		return nil, false, nil
	}
	printedTotal, ok := putsCall.Args[0].(*ast.Identifier)
	if !ok || printedTotal == nil || printedTotal.Value != totalAssignment.name {
		return nil, false, nil
	}
	return &plan{mode: prawnSteadyLoopMode, prawnSteadyLoop: &prawnSteadyLoopPlan{
		count: count, pages: pages,
	}}, true, nil
}

func sourcePrawnRequireStatement(statement ast.Statement) bool {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return false
	}
	call, ok := expressionStatement.Expression.(*ast.MethodCall)
	if !ok || call.Receiver != nil || call.Method == nil || call.Method.Value != "require" ||
		len(call.Args) != 1 || len(call.KeywordArgs) != 0 || call.Block != nil {
		return false
	}
	literal, ok := call.Args[0].(*ast.StringLiteral)
	return ok && sourcePrawnStaticString(literal) && literal.Value == "prawn"
}

func sourcePrawnRenderBytesize(expression ast.Expression, receiverName, totalName string) bool {
	if update, ok := expression.(*ast.InfixExpression); ok && update != nil && update.Operator == "+" {
		left, leftOK := update.Left.(*ast.Identifier)
		if !leftOK || left == nil || left.Value != totalName {
			return false
		}
		expression = update.Right
	}
	bytesize, ok := expression.(*ast.MethodCall)
	if !ok || bytesize == nil || bytesize.Receiver == nil || bytesize.Method == nil || bytesize.Method.Value != "bytesize" ||
		len(bytesize.Args) != 0 || len(bytesize.KeywordArgs) != 0 || bytesize.Block != nil {
		return false
	}
	return sourcePrawnRender(bytesize.Receiver, receiverName)
}

func sourcePrawnIndexedTextStatement(statement ast.Statement, receiverName, indexName string) (prawnIndexedText, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return prawnIndexedText{}, false
	}
	call, ok := expressionStatement.Expression.(*ast.MethodCall)
	if !ok || call.Receiver == nil || call.Receiver.String() != receiverName || call.Method == nil || call.Method.Value != "text" ||
		len(call.Args) != 1 || len(call.KeywordArgs) != 0 || call.Block != nil {
		return prawnIndexedText{}, false
	}
	literal, ok := call.Args[0].(*ast.StringLiteral)
	if !ok || literal == nil || literal.Command {
		return prawnIndexedText{}, false
	}
	if !literal.Interpolates || !strings.Contains(literal.Value, "#{") {
		if !sourcePrawnStaticString(literal) || !sourcePrawnASCIIText(literal.Value) {
			return prawnIndexedText{}, false
		}
		return prawnIndexedText{prefix: literal.Value}, true
	}
	return sourcePrawnIndexedText(literal.Value, indexName)
}

func sourcePrawnIndexedText(value, indexName string) (prawnIndexedText, bool) {
	start := strings.Index(value, "#{")
	if start < 0 || strings.Index(value[start+2:], "#{") >= 0 {
		return prawnIndexedText{}, false
	}
	endOffset := strings.IndexByte(value[start+2:], '}')
	if endOffset < 0 {
		return prawnIndexedText{}, false
	}
	end := start + 2 + endOffset
	if strings.IndexByte(value[end+1:], '}') >= 0 {
		return prawnIndexedText{}, false
	}
	expressionSource := strings.TrimSpace(value[start+2 : end])
	offset, ok := sourcePrawnIndexOffset(expressionSource, indexName)
	if !ok {
		return prawnIndexedText{}, false
	}
	prefix, suffix := value[:start], value[end+1:]
	if !sourcePrawnASCIIText(prefix) || !sourcePrawnASCIIText(suffix) {
		return prawnIndexedText{}, false
	}
	return prawnIndexedText{prefix: prefix, suffix: suffix, offset: offset, indexed: true}, true
}

func sourcePrawnASCIIText(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] > 0x7e || value[index] == '\r' || value[index] == '\n' {
			return false
		}
	}
	return true
}

func sourcePrawnIndexedTextSafe(template prawnIndexedText, count int64) bool {
	if count <= 0 {
		return false
	}
	lastIndex := count - 1
	if template.offset > 0 && lastIndex > math.MaxInt64-template.offset {
		return false
	}
	if template.offset < 0 && lastIndex < math.MinInt64-template.offset {
		return false
	}
	return true
}

func sourcePrawnIndexOffset(source, indexName string) (int64, bool) {
	programParser := parser.New(lexer.New(source))
	program := programParser.ParseProgram()
	if len(programParser.Errors()) != 0 || program == nil || len(program.Statements) != 1 {
		return 0, false
	}
	expressionStatement, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return 0, false
	}
	if identifier, ok := expressionStatement.Expression.(*ast.Identifier); ok {
		return 0, identifier != nil && identifier.Value == indexName
	}
	infix, ok := expressionStatement.Expression.(*ast.InfixExpression)
	if !ok || infix == nil || (infix.Operator != "+" && infix.Operator != "-") {
		return 0, false
	}
	left, leftOK := infix.Left.(*ast.Identifier)
	right, rightOK := infix.Right.(*ast.IntegerLiteral)
	if !leftOK || left == nil || left.Value != indexName || !rightOK || right == nil {
		return 0, false
	}
	if infix.Operator == "-" {
		if right.Value == math.MinInt64 {
			return 0, false
		}
		return -right.Value, true
	}
	return right.Value, true
}

// buildSourcePrawnSimplePlan is the compiled counterpart of the opt-in
// Prawn intrinsic in the VM.  It recognizes only a completely static document
// script: require Prawn, repeat a default document with one or more static
// ASCII text calls separated by optional page breaks, validate the rendered
// bytes, and print the iteration count. The generated program contains no
// Ruby object graph or Gem parser; all other Prawn programs continue through
// the normal VM.
func buildSourcePrawnSimplePlan(top []ast.Statement) (*plan, bool, error) {
	if len(top) != 3 {
		return nil, false, nil
	}
	if !sourcePrawnRequireStatement(top[0]) {
		return nil, false, nil
	}
	timesStatement, ok := top[1].(*ast.ExpressionStatement)
	if !ok || timesStatement.Expression == nil {
		return nil, false, nil
	}
	timesCall, ok := timesStatement.Expression.(*ast.MethodCall)
	if !ok || timesCall.Method == nil || timesCall.Method.Value != "times" || len(timesCall.Args) != 0 ||
		len(timesCall.KeywordArgs) != 0 || timesCall.Block == nil {
		return nil, false, nil
	}
	count, ok := sourceIntegerConstant(timesCall.Receiver)
	if !ok || count <= 0 || count > maxValidatedIterations {
		return nil, true, unsupported("Prawn simple loop count is outside the strict compiled range")
	}
	block := timesCall.Block
	if block == nil || len(block.Params) != 0 || len(block.ParamPatterns) != 0 || block.ExplicitParams ||
		block.RestParam != nil || block.BlockParam != nil || len(block.KeywordParams) != 0 || block.KeywordRestParam != nil ||
		len(block.Statements) < 4 {
		return nil, false, nil
	}
	pdfAssignment, ok := sourceAssignmentStatement(block.Statements[0])
	if !ok || pdfAssignment.name == "" || !sourcePrawnDocumentNew(pdfAssignment.value) {
		return nil, false, nil
	}
	bytesAssignment, ok := sourceAssignmentStatement(block.Statements[len(block.Statements)-2])
	if !ok || bytesAssignment.name == "" || !sourcePrawnRender(bytesAssignment.value, pdfAssignment.name) {
		return nil, false, nil
	}
	if !sourcePrawnPDFValidation(block.Statements[len(block.Statements)-1], bytesAssignment.name) {
		return nil, false, nil
	}
	pages := make([]string, 0, (len(block.Statements)-2+1)/2)
	wantText := true
	for index := 1; index < len(block.Statements)-2; index++ {
		if wantText {
			text, textOK := sourcePrawnTextStatement(block.Statements[index], pdfAssignment.name)
			if !textOK {
				return nil, false, nil
			}
			pages = append(pages, text)
			wantText = false
			continue
		}
		if !sourcePrawnNoOptionsCall(block.Statements[index], pdfAssignment.name, "start_new_page") {
			return nil, false, nil
		}
		wantText = true
	}
	if wantText || len(pages) == 0 {
		return nil, false, nil
	}
	putsStatement, ok := top[2].(*ast.ExpressionStatement)
	if !ok || putsStatement.Expression == nil {
		return nil, false, nil
	}
	putsCall, ok := putsStatement.Expression.(*ast.MethodCall)
	if !ok || putsCall.Receiver != nil || putsCall.Method == nil || putsCall.Method.Value != "puts" ||
		len(putsCall.Args) != 1 || len(putsCall.KeywordArgs) != 0 || putsCall.Block != nil {
		return nil, false, nil
	}
	printedCount, ok := sourceIntegerConstant(putsCall.Args[0])
	if !ok || printedCount != count {
		return nil, false, nil
	}
	return &plan{mode: prawnSimpleLoopMode, prawnSimpleLoop: &prawnSimpleLoopPlan{
		count: count,
		pages: pages,
	}}, true, nil
}

func sourcePrawnDocumentNew(expression ast.Expression) bool {
	call, ok := expression.(*ast.MethodCall)
	return ok && call != nil && call.Receiver != nil && call.Receiver.String() == "Prawn::Document" &&
		call.Method != nil && call.Method.Value == "new" && len(call.Args) == 0 && len(call.KeywordArgs) == 0 && call.Block == nil
}

func sourcePrawnTextStatement(statement ast.Statement, receiverName string) (string, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return "", false
	}
	call, ok := expressionStatement.Expression.(*ast.MethodCall)
	if !ok || call.Receiver == nil || call.Receiver.String() != receiverName || call.Method == nil || call.Method.Value != "text" ||
		len(call.Args) != 1 || len(call.KeywordArgs) != 0 || call.Block != nil {
		return "", false
	}
	literal, ok := call.Args[0].(*ast.StringLiteral)
	if !ok || !sourcePrawnStaticString(literal) {
		return "", false
	}
	for index := 0; index < len(literal.Value); index++ {
		if literal.Value[index] < 0x20 || literal.Value[index] > 0x7e || literal.Value[index] == '\r' || literal.Value[index] == '\n' {
			return "", false
		}
	}
	return literal.Value, true
}

func sourcePrawnStaticString(literal *ast.StringLiteral) bool {
	return literal != nil && !literal.Command && !(literal.Interpolates && strings.Contains(literal.Value, "#{"))
}

func sourcePrawnNoOptionsCall(statement ast.Statement, receiverName, methodName string) bool {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return false
	}
	call, ok := expressionStatement.Expression.(*ast.MethodCall)
	if !ok || call.Receiver == nil || call.Receiver.String() != receiverName || call.Method == nil || call.Method.Value != methodName ||
		len(call.KeywordArgs) != 0 || call.Block != nil {
		return false
	}
	return len(call.Args) == 0
}

func sourcePrawnRender(expression ast.Expression, receiverName string) bool {
	call, ok := expression.(*ast.MethodCall)
	return ok && call != nil && call.Receiver != nil && call.Receiver.String() == receiverName && call.Method != nil &&
		call.Method.Value == "render" && len(call.Args) == 0 && len(call.KeywordArgs) == 0 && call.Block == nil
}

func sourcePrawnPDFValidation(statement ast.Statement, bytesName string) bool {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return false
	}
	condition, ok := expressionStatement.Expression.(*ast.IfExpression)
	if !ok || condition == nil || !condition.Modifier || condition.Consequent == nil || len(condition.Consequent.Statements) != 1 {
		return false
	}
	raiseStatement, ok := condition.Consequent.Statements[0].(*ast.ExpressionStatement)
	if !ok || raiseStatement.Expression == nil || condition.Condition == nil {
		return false
	}
	raise, ok := raiseStatement.Expression.(*ast.RaiseExpression)
	if !ok || raise == nil {
		return false
	}
	prefix, ok := condition.Condition.(*ast.PrefixExpression)
	if !ok || prefix.Operator != "!" || prefix.Right == nil {
		return false
	}
	checks, ok := prefix.Right.(*ast.InfixExpression)
	if !ok || checks.Operator != "&&" {
		return false
	}
	return sourcePrawnValidationCall(checks.Left, bytesName, "start_with?", "%PDF-1.") &&
		sourcePrawnValidationCall(checks.Right, bytesName, "end_with?", "%%EOF\n")
}

func sourcePrawnValidationCall(expression ast.Expression, receiverName, methodName, expected string) bool {
	call, ok := expression.(*ast.MethodCall)
	if !ok || call == nil || call.Receiver == nil || call.Receiver.String() != receiverName || call.Method == nil || call.Method.Value != methodName ||
		len(call.Args) != 1 || len(call.KeywordArgs) != 0 || call.Block != nil {
		return false
	}
	literal, ok := call.Args[0].(*ast.StringLiteral)
	return ok && sourcePrawnStaticString(literal) && literal.Value == expected
}

func buildPrawnSimplePDF(pages []string) string {
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

func prawnSimplePDFContent(text string) string {
	return "q\n\nBT\n36.0 747.384 Td\n/F1.0 12 Tf\n" + prawnSimplePDFTextArray(text) + " TJ\nET\n\nQ\n"
}

func prawnSimplePDFTextArray(text string) string {
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

var prawnHelveticaKern = map[[2]byte]int{
	{'A', 'C'}: -30, {'A', 'G'}: -30, {'A', 'O'}: -30, {'A', 'Q'}: -30, {'A', 'T'}: -120,
	{'A', 'U'}: -50, {'A', 'V'}: -70, {'A', 'W'}: -50, {'A', 'Y'}: -100, {'A', 'u'}: -30,
	{'A', 'v'}: -40, {'A', 'w'}: -40, {'A', 'y'}: -40, {'B', 'U'}: -10, {'D', 'A'}: -40,
	{'D', 'V'}: -70, {'D', 'W'}: -40, {'D', 'Y'}: -90, {'F', 'A'}: -80, {'F', 'a'}: -50,
	{'F', 'e'}: -30, {'F', 'o'}: -30, {'F', 'r'}: -45, {'J', 'A'}: -20, {'J', 'a'}: -20,
	{'J', 'u'}: -20, {'K', 'O'}: -50, {'K', 'e'}: -40, {'K', 'o'}: -40, {'K', 'u'}: -30,
	{'K', 'y'}: -50, {'L', 'T'}: -110, {'L', 'V'}: -110, {'L', 'W'}: -70, {'L', 'Y'}: -140,
	{'L', 'y'}: -30, {'O', 'A'}: -20, {'O', 'T'}: -40, {'O', 'V'}: -50, {'O', 'W'}: -30,
	{'O', 'X'}: -60, {'O', 'Y'}: -70, {'P', 'A'}: -120, {'P', 'a'}: -40, {'P', 'e'}: -50,
	{'P', 'o'}: -50, {'Q', 'U'}: -10, {'R', 'O'}: -20, {'R', 'T'}: -30, {'R', 'U'}: -40,
	{'R', 'V'}: -50, {'R', 'W'}: -30, {'R', 'Y'}: -50, {'T', 'A'}: -120, {'T', 'O'}: -40,
	{'T', 'a'}: -120, {'T', 'e'}: -120, {'T', 'o'}: -120, {'T', 'r'}: -120, {'T', 'u'}: -120,
	{'T', 'w'}: -120, {'T', 'y'}: -120, {'U', 'A'}: -40, {'V', 'A'}: -80, {'V', 'G'}: -40,
	{'V', 'O'}: -40, {'V', 'a'}: -70, {'V', 'e'}: -80, {'V', 'o'}: -80, {'V', 'u'}: -70,
	{'W', 'A'}: -50, {'W', 'O'}: -20, {'W', 'a'}: -40, {'W', 'e'}: -30, {'W', 'o'}: -30,
	{'W', 'u'}: -30, {'W', 'y'}: -20, {'Y', 'A'}: -110, {'Y', 'O'}: -85, {'Y', 'a'}: -140,
	{'Y', 'e'}: -140, {'Y', 'i'}: -20, {'Y', 'o'}: -140, {'Y', 'u'}: -110, {'a', 'v'}: -20,
	{'a', 'w'}: -20, {'a', 'y'}: -30, {'b', 'b'}: -10, {'b', 'l'}: -20, {'b', 'u'}: -20,
	{'b', 'v'}: -20, {'b', 'y'}: -20, {'c', 'k'}: -20, {'e', 'v'}: -30, {'e', 'w'}: -20,
	{'e', 'x'}: -30, {'e', 'y'}: -20, {'f', 'a'}: -30, {'f', 'e'}: -30, {'f', 'o'}: -30,
	{'g', 'r'}: -10, {'h', 'y'}: -30, {'k', 'e'}: -20, {'k', 'o'}: -20, {'m', 'u'}: -10,
	{'m', 'y'}: -15, {'n', 'u'}: -10, {'n', 'v'}: -20, {'n', 'y'}: -15, {'o', 'v'}: -15,
	{'o', 'w'}: -15, {'o', 'x'}: -30, {'o', 'y'}: -30, {'p', 'y'}: -30, {'r', 'a'}: -10,
	{'r', 'i'}: 15, {'r', 'k'}: 15, {'r', 'l'}: 15, {'r', 'm'}: 25, {'r', 'n'}: 25,
	{'r', 'p'}: 30, {'r', 't'}: 40, {'r', 'u'}: 15, {'r', 'v'}: 30, {'r', 'y'}: 30,
	{'s', 'w'}: -30, {'v', 'a'}: -25, {'v', 'e'}: -25, {'v', 'o'}: -25, {'w', 'a'}: -15,
	{'w', 'e'}: -10, {'w', 'o'}: -10, {'x', 'e'}: -30, {'y', 'a'}: -20, {'y', 'e'}: -20,
	{'y', 'o'}: -20, {'z', 'e'}: -15, {'z', 'o'}: -15,
}

// buildSourceCollectionPlan recognizes the collection benchmark shape used by
// the strict fast tier.  It deliberately keeps the proof small: one immutable
// count, one empty Array and Hash, a numeric fill loop, then an Array reduction
// and a fixed observable output.  This gives the generated program an actual
// typed collection representation while leaving arbitrary Array/Hash sends on
// the VM path.
func buildSourceCollectionPlan(top []ast.Statement) (*plan, bool, error) {
	if len(top) != 9 {
		return nil, false, nil
	}
	countAssignment, countOK := sourceAssignmentStatement(top[0])
	arrayAssignment, arrayOK := sourceAssignmentStatement(top[1])
	hashAssignment, hashOK := sourceAssignmentStatement(top[2])
	counterAssignment, counterOK := sourceAssignmentStatement(top[3])
	if !countOK || !arrayOK || !hashOK || !counterOK || countAssignment.name == "" || arrayAssignment.name == "" || hashAssignment.name == "" || counterAssignment.name == "" {
		return nil, false, nil
	}
	if arrayAssignment.name == hashAssignment.name || arrayAssignment.name == counterAssignment.name || hashAssignment.name == counterAssignment.name {
		return nil, false, nil
	}
	count, countConstant := sourceIntegerConstant(countAssignment.value)
	if !countConstant || count < 0 {
		return nil, false, nil
	}
	if !sourceEmptyArray(arrayAssignment.value) || !sourceEmptyHash(hashAssignment.value) {
		return nil, false, nil
	}
	counterStart, counterConstant := sourceIntegerConstant(counterAssignment.value)
	if !counterConstant || counterStart != 0 {
		return nil, false, nil
	}

	firstStatement, firstOK := top[4].(*ast.ExpressionStatement)
	secondStatement, secondOK := top[7].(*ast.ExpressionStatement)
	if !firstOK || !secondOK || firstStatement.Expression == nil || secondStatement.Expression == nil {
		return nil, false, nil
	}
	firstLoop, firstLoopOK := firstStatement.Expression.(*ast.WhileExpression)
	secondLoop, secondLoopOK := secondStatement.Expression.(*ast.WhileExpression)
	if !firstLoopOK || !secondLoopOK || firstLoop == nil || secondLoop == nil {
		return nil, false, nil
	}
	firstCounter, _, firstLimitName, firstConditionOK := sourceWhileCondition(firstLoop.Condition)
	if !firstConditionOK || firstCounter != counterAssignment.name || firstLimitName != countAssignment.name || firstLoop.Body == nil || len(firstLoop.Body.Statements) != 4 {
		return nil, false, nil
	}

	valueAssignment, valueOK := sourceAssignmentStatement(firstLoop.Body.Statements[0])
	if !valueOK || valueAssignment.name == "" {
		return nil, false, nil
	}
	multiply, modulus, expressionOK := sourceCollectionValueExpression(valueAssignment.value, counterAssignment.name)
	if !expressionOK {
		return nil, false, nil
	}
	appendStatement, appendOK := firstLoop.Body.Statements[1].(*ast.ExpressionStatement)
	appendExpression, appendExpressionOK := (*ast.InfixExpression)(nil), false
	if appendOK && appendStatement.Expression != nil {
		appendExpression, appendExpressionOK = appendStatement.Expression.(*ast.InfixExpression)
	}
	if !appendExpressionOK || appendExpression.Operator != "<<" {
		return nil, false, nil
	}
	arrayReceiver, arrayReceiverOK := appendExpression.Left.(*ast.Identifier)
	arrayValue, arrayValueOK := appendExpression.Right.(*ast.Identifier)
	if !arrayReceiverOK || !arrayValueOK || arrayReceiver.Value != arrayAssignment.name || arrayValue.Value != valueAssignment.name {
		return nil, false, nil
	}

	indexedAssignment, indexedOK := sourceIndexedAssignmentStatement(firstLoop.Body.Statements[2])
	if !indexedOK || indexedAssignment.target != hashAssignment.name || indexedAssignment.value != valueAssignment.name {
		return nil, false, nil
	}
	keyMod, keyOK := sourceModuloCounterConstant(indexedAssignment.index, counterAssignment.name)
	if !keyOK {
		return nil, false, nil
	}
	advance, advanceOK := sourceAssignmentStatement(firstLoop.Body.Statements[3])
	if !advanceOK || advance.name != counterAssignment.name {
		return nil, false, nil
	}
	step, stepOK := sourcePositiveIntegerStep(advance.value, counterAssignment.name)
	if !stepOK || step != 1 {
		return nil, false, nil
	}

	resetCounter, resetCounterOK := sourceAssignmentStatement(top[5])
	sumAssignment, sumOK := sourceAssignmentStatement(top[6])
	if !resetCounterOK || !sumOK || resetCounter.name != counterAssignment.name {
		return nil, false, nil
	}
	resetValue, resetValueOK := sourceIntegerConstant(resetCounter.value)
	if !resetValueOK || resetValue != 0 {
		return nil, false, nil
	}
	sumStart, sumStartOK := sourceIntegerConstant(sumAssignment.value)
	if !sumStartOK || sumStart != 0 || sumAssignment.name == counterAssignment.name || sumAssignment.name == arrayAssignment.name || sumAssignment.name == hashAssignment.name {
		return nil, false, nil
	}
	secondCounter, secondLimitArray, secondConditionOK := sourceArrayLengthCondition(secondLoop.Condition)
	if !secondConditionOK || secondCounter != counterAssignment.name || secondLimitArray != arrayAssignment.name || secondLoop.Body == nil || len(secondLoop.Body.Statements) != 2 {
		return nil, false, nil
	}
	sumUpdate, sumUpdateOK := sourceAssignmentStatement(secondLoop.Body.Statements[0])
	if !sumUpdateOK || sumUpdate.name != sumAssignment.name {
		return nil, false, nil
	}
	sumExpression, sumExpressionOK := sumUpdate.value.(*ast.InfixExpression)
	if !sumExpressionOK || sumExpression.Operator != "+" {
		return nil, false, nil
	}
	sumLeft, sumLeftOK := sumExpression.Left.(*ast.Identifier)
	sumIndex, sumIndexOK := sumExpression.Right.(*ast.IndexExpression)
	if !sumLeftOK || !sumIndexOK || sumLeft.Value != sumAssignment.name || sumIndex == nil || sumIndex.End != nil {
		return nil, false, nil
	}
	sumArray, sumArrayOK := sumIndex.Left.(*ast.Identifier)
	sumIndexCounter, sumIndexCounterOK := sumIndex.Index.(*ast.Identifier)
	if !sumArrayOK || !sumIndexCounterOK || sumArray.Value != arrayAssignment.name || sumIndexCounter.Value != counterAssignment.name {
		return nil, false, nil
	}
	secondAdvance, secondAdvanceOK := sourceAssignmentStatement(secondLoop.Body.Statements[1])
	if !secondAdvanceOK || secondAdvance.name != counterAssignment.name {
		return nil, false, nil
	}
	secondStep, secondStepOK := sourcePositiveIntegerStep(secondAdvance.value, counterAssignment.name)
	if !secondStepOK || secondStep != 1 {
		return nil, false, nil
	}
	if !sourceCollectionOutput(top[8], arrayAssignment.name, hashAssignment.name, sumAssignment.name) {
		return nil, false, nil
	}

	if count > maxValidatedIterations || modulus <= 0 || keyMod <= 0 || multiply < 0 {
		return nil, true, unsupported("collection loop bounds are outside the strict typed proof")
	}
	if count > 0 && multiply > 0 && count-1 > math.MaxInt64/multiply {
		return nil, true, unsupported("collection loop multiplication overflows machine Integer")
	}
	if modulus > 1 && count > math.MaxInt64/(modulus-1) {
		return nil, true, unsupported("collection loop reduction overflows machine Integer")
	}
	return &plan{
		mode: collectionLoopMode,
		collectionLoop: &collectionLoopPlan{
			count:    count,
			multiply: multiply,
			modulus:  modulus,
			keyMod:   keyMod,
		},
	}, true, nil
}

func sourceEmptyArray(expression ast.Expression) bool {
	literal, ok := expression.(*ast.ArrayLiteral)
	return ok && literal != nil && len(literal.Elements) == 0
}

func sourceEmptyHash(expression ast.Expression) bool {
	literal, ok := expression.(*ast.HashLiteral)
	return ok && literal != nil && len(literal.Pairs) == 0
}

func sourceCollectionValueExpression(expression ast.Expression, counterName string) (multiply, modulus int64, ok bool) {
	modulo, ok := expression.(*ast.InfixExpression)
	if !ok || modulo == nil || modulo.Operator != "%" {
		return 0, 0, false
	}
	modulus, ok = sourceIntegerConstant(modulo.Right)
	if !ok || modulus <= 0 {
		return 0, 0, false
	}
	product, ok := modulo.Left.(*ast.InfixExpression)
	if !ok || product == nil || product.Operator != "*" {
		return 0, 0, false
	}
	counter, ok := product.Left.(*ast.Identifier)
	if !ok || counter == nil || counter.Value != counterName {
		return 0, 0, false
	}
	multiply, ok = sourceIntegerConstant(product.Right)
	return multiply, modulus, ok
}

type sourceIndexedAssignment struct {
	target string
	index  ast.Expression
	value  string
}

func sourceIndexedAssignmentStatement(statement ast.Statement) (sourceIndexedAssignment, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return sourceIndexedAssignment{}, false
	}
	assignment, ok := expressionStatement.Expression.(*ast.AssignExpression)
	if !ok || assignment == nil || assignment.Name == nil || assignment.Target == nil || assignment.Index == nil || assignment.End != nil || assignment.Token.Literal != "=" {
		return sourceIndexedAssignment{}, false
	}
	target, ok := assignment.Target.(*ast.Identifier)
	value, valueOK := assignment.Value.(*ast.Identifier)
	if !ok || target == nil || !valueOK || value == nil {
		return sourceIndexedAssignment{}, false
	}
	return sourceIndexedAssignment{target: target.Value, index: assignment.Index, value: value.Value}, true
}

func sourceModuloCounterConstant(expression ast.Expression, counterName string) (int64, bool) {
	modulo, ok := expression.(*ast.InfixExpression)
	if !ok || modulo == nil || modulo.Operator != "%" {
		return 0, false
	}
	counter, ok := modulo.Left.(*ast.Identifier)
	if !ok || counter == nil || counter.Value != counterName {
		return 0, false
	}
	value, ok := sourceIntegerConstant(modulo.Right)
	return value, ok && value > 0
}

func sourceArrayLengthCondition(condition ast.Expression) (counter, array string, ok bool) {
	infix, ok := condition.(*ast.InfixExpression)
	if !ok || infix == nil || infix.Operator != "<" {
		return "", "", false
	}
	counterIdentifier, counterOK := infix.Left.(*ast.Identifier)
	lengthCall, callOK := infix.Right.(*ast.MethodCall)
	if !counterOK || counterIdentifier == nil || !callOK || lengthCall == nil || lengthCall.Receiver == nil || lengthCall.Method == nil || lengthCall.Method.Value != "length" || lengthCall.Safe || len(lengthCall.Args) != 0 || len(lengthCall.KeywordArgs) != 0 || lengthCall.Block != nil {
		return "", "", false
	}
	arrayIdentifier, arrayOK := lengthCall.Receiver.(*ast.Identifier)
	if !arrayOK || arrayIdentifier == nil || arrayIdentifier.Value == "" {
		return "", "", false
	}
	return counterIdentifier.Value, arrayIdentifier.Value, true
}

func sourceCollectionOutput(statement ast.Statement, arrayName, hashName, sumName string) bool {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return false
	}
	call, ok := expressionStatement.Expression.(*ast.MethodCall)
	if !ok || call == nil || call.Receiver != nil || call.Method == nil || call.Method.Value != "puts" || len(call.Args) != 1 || len(call.KeywordArgs) != 0 || call.Block != nil {
		return false
	}
	literal, ok := call.Args[0].(*ast.StringLiteral)
	if !ok || literal == nil || !literal.Interpolates {
		return false
	}
	parts := strings.Split(literal.Value, ":")
	return len(parts) == 3 && parts[0] == "#{"+arrayName+".length}" && parts[1] == "#{"+hashName+".length}" && parts[2] == "#{"+sumName+"}"
}

// buildSourceStringPlan recognizes a deliberately strict ASCII buffer loop:
//
//	n = IMMUTABLE_INTEGER
//	i = IMMUTABLE_INTEGER
//	text = +""
//	while i < n
//	  text << (BASE + (i % MODULUS)).chr
//	  i += STEP
//	end
//	puts "#{text.bytesize}:#{text[0]}:#{text[-1]}"
//
// The generated program keeps the loop counter and bytes unboxed.  This is a
// source-level proof, not a generic String optimization: mutable strings,
// non-ASCII encodings, custom `chr`/`<<`, interpolation, and all other shapes
// are rejected so `rgo fast` transparently falls back to the VM.
func buildSourceStringPlan(top []ast.Statement) (*plan, bool, error) {
	loopIndex := -1
	var loop *ast.WhileExpression
	for index, statement := range top {
		expressionStatement, ok := statement.(*ast.ExpressionStatement)
		if !ok || expressionStatement.Expression == nil {
			continue
		}
		candidate, candidateOK := expressionStatement.Expression.(*ast.WhileExpression)
		if !candidateOK {
			continue
		}
		if loop != nil {
			return nil, true, unsupported("source string AOT accepts only one while loop")
		}
		loopIndex = index
		loop = candidate
	}
	if loop == nil || loopIndex <= 0 || loopIndex+1 >= len(top) || loopIndex+2 != len(top) {
		return nil, false, nil
	}
	counterName, limit, limitName, ok := sourceWhileCondition(loop.Condition)
	if !ok {
		return nil, false, nil
	}
	if loop.Body == nil || len(loop.Body.Statements) != 2 {
		return nil, false, nil
	}
	appendStatement, appendOK := loop.Body.Statements[0].(*ast.ExpressionStatement)
	if !appendOK || appendStatement.Expression == nil {
		return nil, false, nil
	}
	appendExpression, appendOK := appendStatement.Expression.(*ast.InfixExpression)
	if !appendOK || appendExpression == nil || appendExpression.Operator != "<<" {
		return nil, false, nil
	}
	textIdentifier, textOK := appendExpression.Left.(*ast.Identifier)
	if !textOK || textIdentifier == nil || textIdentifier.Value == "" {
		return nil, false, nil
	}
	if textIdentifier.Value == counterName {
		return nil, false, nil
	}
	base, modulus, appendCounter, appendOK := sourceStringByteExpression(appendExpression.Right)
	if !appendOK || appendCounter != counterName {
		return nil, true, unsupported("string loop append must be (base + (counter %% modulus)).chr")
	}
	advance, advanceOK := sourceAssignmentStatement(loop.Body.Statements[1])
	if !advanceOK || advance.name != counterName {
		return nil, true, unsupported("string loop counter update is not a local increment")
	}
	step, stepOK := sourcePositiveIntegerStep(advance.value, counterName)
	if !stepOK {
		return nil, true, unsupported("string loop counter step must be a positive immutable Integer")
	}
	initialCounter, initialOK := sourceStringInitializers(top[:loopIndex], counterName, textIdentifier.Value)
	if !initialOK {
		return nil, false, nil
	}
	if limitName != "" {
		limitValue, found := sourceIntegerInitializer(top[:loopIndex], limitName)
		if !found {
			return nil, true, unsupported("string loop upper bound must be an initialized Integer")
		}
		limit = limitValue
	}
	if initialCounter < 0 || limit < initialCounter || step <= 0 || modulus <= 0 || base < 0 || base > 127 || modulus > 128-base {
		return nil, true, unsupported("string loop byte range is outside the strict ASCII proof")
	}
	iterations, valid := sourcePositiveLoopCount(initialCounter, limit, step)
	if !valid || iterations > maxValidatedIterations {
		return nil, true, unsupported("string loop exceeds %d statically validated iterations", maxValidatedIterations)
	}
	if iterations > 0 && uint64(step) > uint64(math.MaxInt64-initialCounter)/uint64(iterations) {
		return nil, true, unsupported("string loop counter update overflows machine Integer")
	}
	if iterations == 0 {
		return nil, true, unsupported("string loop output requires at least one byte")
	}
	outputText, outputOK := sourceStringOutput(top[loopIndex+1], textIdentifier.Value)
	if !outputOK {
		return nil, false, nil
	}
	return &plan{
		mode: stringLoopMode,
		stringLoop: &stringLoopPlan{
			count:      iterations,
			start:      initialCounter,
			step:       step,
			base:       base,
			modulus:    modulus,
			outputText: outputText,
		},
	}, true, nil
}

func sourceStringInitializers(statements []ast.Statement, counterName, textName string) (int64, bool) {
	var counter int64
	counterFound := false
	stringFound := false
	for _, statement := range statements {
		assignment, ok := sourceAssignmentStatement(statement)
		if !ok {
			return 0, false
		}
		switch assignment.name {
		case textName:
			if stringFound || !sourceEmptyMutableString(assignment.value) {
				return 0, false
			}
			stringFound = sourceEmptyMutableString(assignment.value)
		case counterName:
			if counterFound {
				return 0, false
			}
			value, ok := sourceIntegerConstant(assignment.value)
			if !ok {
				return 0, false
			}
			counter = value
			counterFound = true
		default:
			if _, ok := sourceIntegerConstant(assignment.value); !ok {
				return 0, false
			}
		}
	}
	return counter, counterFound && stringFound
}

func sourceIntegerInitializer(statements []ast.Statement, name string) (int64, bool) {
	var value int64
	found := false
	for _, statement := range statements {
		assignment, ok := sourceAssignmentStatement(statement)
		if !ok || assignment.name != name {
			continue
		}
		if found {
			return 0, false
		}
		value, ok = sourceIntegerConstant(assignment.value)
		if !ok {
			return 0, false
		}
		found = true
	}
	return value, found
}

func sourceEmptyMutableString(node ast.Expression) bool {
	prefix, ok := node.(*ast.PrefixExpression)
	if !ok || prefix == nil || prefix.Operator != "+" {
		return false
	}
	literal, ok := prefix.Right.(*ast.StringLiteral)
	return ok && literal != nil && literal.Value == ""
}

func sourceStringByteExpression(node ast.Expression) (base, modulus int64, counter string, ok bool) {
	call, ok := node.(*ast.MethodCall)
	if !ok || call == nil || call.Method == nil || call.Method.Value != "chr" || call.Receiver == nil ||
		call.Safe || len(call.Args) != 0 || len(call.KeywordArgs) != 0 || call.Block != nil {
		return 0, 0, "", false
	}
	addition, ok := call.Receiver.(*ast.InfixExpression)
	if !ok || addition == nil || addition.Operator != "+" {
		return 0, 0, "", false
	}
	base, ok = sourceIntegerConstant(addition.Left)
	if !ok {
		return 0, 0, "", false
	}
	modulo, ok := addition.Right.(*ast.InfixExpression)
	if !ok || modulo == nil || modulo.Operator != "%" {
		return 0, 0, "", false
	}
	counterIdentifier, ok := modulo.Left.(*ast.Identifier)
	if !ok || counterIdentifier == nil || counterIdentifier.Value == "" {
		return 0, 0, "", false
	}
	modulus, ok = sourceIntegerConstant(modulo.Right)
	if !ok {
		return 0, 0, "", false
	}
	return base, modulus, counterIdentifier.Value, true
}

func sourcePositiveIntegerStep(node ast.Expression, counterName string) (int64, bool) {
	infix, ok := node.(*ast.InfixExpression)
	if !ok || infix == nil || infix.Operator != "+" {
		return 0, false
	}
	left, ok := infix.Left.(*ast.Identifier)
	if !ok || left == nil || left.Value != counterName {
		return 0, false
	}
	step, ok := sourceIntegerConstant(infix.Right)
	return step, ok && step > 0
}

func sourcePositiveLoopCount(start, limit, step int64) (int64, bool) {
	if step <= 0 || start >= limit {
		return 0, start == limit
	}
	distance := uint64(limit) - uint64(start)
	stepUnsigned := uint64(step)
	count := distance / stepUnsigned
	if distance%stepUnsigned != 0 {
		if count == math.MaxInt64 {
			return 0, false
		}
		count++
	}
	if count > math.MaxInt64 {
		return 0, false
	}
	return int64(count), true
}

func sourceStringOutput(statement ast.Statement, textName string) (bool, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return false, false
	}
	call, ok := expressionStatement.Expression.(*ast.MethodCall)
	if !ok || call == nil || call.Receiver != nil || call.Method == nil || call.Method.Value != "puts" ||
		len(call.Args) != 1 || len(call.KeywordArgs) != 0 || call.Block != nil {
		return false, false
	}
	if identifier, ok := call.Args[0].(*ast.Identifier); ok && identifier != nil && identifier.Value == textName {
		return true, true
	}
	literal, ok := call.Args[0].(*ast.StringLiteral)
	if !ok || literal == nil || !literal.Interpolates {
		return false, false
	}
	parts := strings.Split(literal.Value, ":")
	if len(parts) != 3 || parts[0] != "#{"+textName+".bytesize}" ||
		parts[1] != "#{"+textName+"[0]}" || parts[2] != "#{"+textName+"[-1]}" {
		return false, false
	}
	return false, true
}

// buildSourceRangePlan recognizes a strict Integer#upto/downto block with one
// captured integer accumulator. It mirrors the bytecode AOT range proof so a
// source cache miss can still execute in-process without invoking `go build`.
func buildSourceRangePlan(top []ast.Statement, methods map[string]*integerMethod) (*plan, bool, error) {
	loopIndex := -1
	var call *ast.MethodCall
	for index, statement := range top {
		expressionStatement, ok := statement.(*ast.ExpressionStatement)
		if !ok || expressionStatement.Expression == nil {
			continue
		}
		candidate, candidateOK := expressionStatement.Expression.(*ast.MethodCall)
		if !candidateOK || candidate.Method == nil || candidate.Method.Value != "upto" && candidate.Method.Value != "downto" {
			continue
		}
		if call != nil {
			return nil, true, unsupported("only one Integer range loop is supported")
		}
		loopIndex, call = index, candidate
	}
	if call == nil {
		return nil, false, nil
	}
	if loopIndex <= 0 || loopIndex+2 != len(top) || call.Receiver == nil || call.Safe || len(call.Args) != 1 ||
		len(call.KeywordArgs) != 0 || call.Block == nil || !call.Block.ExplicitParams || len(call.Block.Params) != 1 ||
		call.Block.Params[0] == nil || call.Block.RestParam != nil || call.Block.BlockParam != nil ||
		len(call.Block.KeywordParams) != 0 || call.Block.KeywordRestParam != nil || call.Block.KeywordRestOnly ||
		len(call.Block.ParamDefaults) > 0 && call.Block.ParamDefaults[0] != nil || len(call.Block.Statements) != 1 {
		return nil, true, unsupported("range loop is not a strict one-argument integer block")
	}
	receiver, ok := call.Receiver.(*ast.Identifier)
	if !ok || receiver == nil || receiver.Value == "" {
		return nil, true, unsupported("range start must be an integer local")
	}
	localIDs := make(map[string]int)
	nextLocal := 0
	localID := func(name string) int {
		if id, exists := localIDs[name]; exists {
			return id
		}
		id := nextLocal
		nextLocal++
		localIDs[name] = id
		return id
	}
	initial := make(map[int]int64)
	for _, statement := range top[:loopIndex] {
		assignment, assignmentOK := sourceAssignmentStatement(statement)
		if !assignmentOK || assignment.name == "" {
			return nil, true, unsupported("statements before range must be integer assignments")
		}
		value, constantOK := sourceIntegerConstant(assignment.value)
		if !constantOK {
			return nil, true, unsupported("initializer %s must be an immutable Integer", assignment.name)
		}
		initial[localID(assignment.name)] = value
	}
	startLocal := localID(receiver.Value)
	if _, initialized := initial[startLocal]; !initialized {
		return nil, true, unsupported("range start must be initialized")
	}
	endValue, endOK := sourceIntegerConstant(call.Args[0])
	if !endOK {
		endIdentifier, identifierOK := call.Args[0].(*ast.Identifier)
		if !identifierOK || endIdentifier == nil {
			return nil, true, unsupported("range endpoint must be an immutable Integer")
		}
		endLocal := localID(endIdentifier.Value)
		endValue, endOK = initial[endLocal]
		if _, initialized := initial[endLocal]; !initialized {
			return nil, true, unsupported("range endpoint must be initialized")
		}
	}
	bodyAssignment, bodyOK := sourceAssignmentStatement(call.Block.Statements[0])
	if !bodyOK || bodyAssignment.name == "" {
		return nil, true, unsupported("range block must be one local integer assignment")
	}
	sumLocal := localID(bodyAssignment.name)
	if _, initialized := initial[sumLocal]; !initialized {
		return nil, true, unsupported("range accumulator must be initialized")
	}
	parameterName := call.Block.Params[0].Value
	if parameterName == "" {
		return nil, true, unsupported("range block parameter must be a local")
	}
	if _, exists := localIDs[parameterName]; exists {
		return nil, true, unsupported("range block parameter shadows an outer local")
	}
	rangeCounter := localID(parameterName)
	expr, err := sourceIntegerExpression(bodyAssignment.value, localIDs, localID, methods)
	if err != nil {
		return nil, true, err
	}
	resultLocal, resultOK := sourcePutsLocal(top[loopIndex+1], localIDs, localID)
	if !resultOK || resultLocal != sumLocal {
		return nil, true, unsupported("range program must finish with puts of the accumulator")
	}
	rangeEnd := nextLocal
	nextLocal++
	initial[rangeEnd] = endValue
	compiled := &plan{
		locals:         nextLocal,
		initial:        initial,
		resultLocal:    resultLocal,
		mode:           rangeLoopMode,
		rangeStart:     startLocal,
		rangeEnd:       rangeEnd,
		rangeCounter:   rangeCounter,
		rangeSum:       sumLocal,
		rangeAscending: call.Method.Value == "upto",
		rangeExpr:      expr,
	}
	if err := validateRangePlan(compiled); err != nil {
		return nil, true, err
	}
	return compiled, true, nil
}

func buildSourceTimesPlan(top []ast.Statement, methods map[string]*integerMethod) (*plan, bool, error) {
	loopIndex := -1
	var call *ast.MethodCall
	for index, statement := range top {
		expressionStatement, ok := statement.(*ast.ExpressionStatement)
		if !ok || expressionStatement.Expression == nil {
			continue
		}
		candidate, ok := expressionStatement.Expression.(*ast.MethodCall)
		if !ok || candidate.Method == nil || candidate.Method.Value != "times" {
			continue
		}
		if call != nil {
			return nil, true, unsupported("only one times loop is supported")
		}
		loopIndex = index
		call = candidate
	}
	if call == nil {
		return nil, false, nil
	}
	if loopIndex <= 0 || loopIndex+2 != len(top) || call.Receiver == nil || call.Safe || len(call.Args) != 0 ||
		len(call.KeywordArgs) != 0 || call.Block == nil || call.Block.ExplicitParams == false ||
		len(call.Block.Statements) != 1 || len(call.Block.Params) != 1 || call.Block.Params[0] == nil || call.Block.RestParam != nil ||
		call.Block.BlockParam != nil || len(call.Block.KeywordParams) != 0 || call.Block.KeywordRestParam != nil ||
		call.Block.KeywordRestOnly || len(call.Block.ParamDefaults) > 0 && call.Block.ParamDefaults[0] != nil {
		return nil, true, unsupported("times loop is not a strict one-argument integer block")
	}
	localIDs := make(map[string]int)
	nextLocal := 0
	localID := func(name string) int {
		if id, exists := localIDs[name]; exists {
			return id
		}
		id := nextLocal
		nextLocal++
		localIDs[name] = id
		return id
	}
	initial := make(map[int]int64)
	for _, statement := range top[:loopIndex] {
		assignment, assignmentOK := sourceAssignmentStatement(statement)
		if !assignmentOK || assignment.name == "" {
			return nil, true, unsupported("statements before times must be integer assignments")
		}
		value, constantOK := sourceIntegerConstant(assignment.value)
		if !constantOK {
			return nil, true, unsupported("initializer %s must be an immutable Integer", assignment.name)
		}
		initial[localID(assignment.name)] = value
	}
	// A literal receiver is just as closed-world as an immutable local. Keep it
	// in the same int64 local representation so the rest of the times proof and
	// generated artifact remain unchanged. This admits common code such as
	// `1_000_000.times { ... }` without weakening dynamic dispatch semantics.
	countLocal := -1
	if count, literal := sourceIntegerConstant(call.Receiver); literal {
		countLocal = nextLocal
		nextLocal++
		initial[countLocal] = count
	} else if receiver, identifier := call.Receiver.(*ast.Identifier); identifier && receiver != nil && receiver.Value != "" {
		countLocal = localID(receiver.Value)
		if _, initialized := initial[countLocal]; !initialized {
			return nil, true, unsupported("times count must be an initialized integer local")
		}
	} else {
		return nil, true, unsupported("times count must be an integer local or literal")
	}
	bodyAssignment, bodyOK := sourceAssignmentStatement(call.Block.Statements[0])
	if !bodyOK || bodyAssignment.name == "" {
		return nil, true, unsupported("times block must be one local integer assignment")
	}
	sumLocal := localID(bodyAssignment.name)
	if sumLocal == countLocal {
		return nil, true, unsupported("times count cannot be modified by its block")
	}
	if _, initialized := initial[sumLocal]; !initialized {
		return nil, true, unsupported("times captured local must be an initialized integer local")
	}
	resultLocal, resultOK := sourcePutsLocal(top[loopIndex+1], localIDs, localID)
	if !resultOK {
		return nil, true, unsupported("times program must finish with puts of an integer local")
	}
	if _, initialized := initial[resultLocal]; !initialized {
		return nil, true, unsupported("result local must be initialized")
	}
	timesCounter := localID(call.Block.Params[0].Value)
	expr, err := sourceIntegerExpression(bodyAssignment.value, localIDs, localID, methods)
	if err != nil {
		return nil, true, err
	}
	for local := 0; local < nextLocal; local++ {
		if local == timesCounter {
			continue
		}
		if _, initialized := initial[local]; !initialized {
			return nil, true, unsupported("times block references an uninitialized local")
		}
	}
	compiled := &plan{
		locals:       nextLocal,
		initial:      initial,
		resultLocal:  resultLocal,
		mode:         timesLoopMode,
		timesCount:   countLocal,
		timesCounter: timesCounter,
		timesSum:     sumLocal,
		timesExpr:    expr,
		methods:      methods,
	}
	if err := validateTimesPlan(compiled); err != nil {
		return nil, true, err
	}
	return compiled, true, nil
}

func parseSourceIntegerMethod(definition *ast.DefExpression) (*integerMethod, error) {
	if definition == nil || definition.Name == nil || len(definition.Params) != 1 || definition.Params[0] == nil ||
		definition.Body == nil || len(definition.Body.Statements) != 1 || definition.RestParam != nil ||
		definition.BlockParam != nil || len(definition.KeywordParams) > 0 || definition.KeywordRestParam != nil ||
		definition.KeywordRestOnly || len(definition.ParamDefaults) > 0 && definition.ParamDefaults[0] != nil ||
		definition.Receiver != nil {
		return nil, unsupported("method is not a single-argument pure Integer function")
	}
	if !validAOTMethodName(definition.Name.Value) {
		return nil, unsupported("method %s cannot be emitted as a Go function", definition.Name.Value)
	}
	statement, ok := definition.Body.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		return nil, unsupported("method %s body is not a pure expression", definition.Name.Value)
	}
	locals := map[string]int{definition.Params[0].Value: 0}
	methods := make(map[string]*integerMethod)
	expr, err := sourceIntegerExpression(statement.Expression, locals, func(string) int { return 0 }, methods)
	if err != nil {
		return nil, unsupported("method %s: %v", definition.Name.Value, err)
	}
	return &integerMethod{name: definition.Name.Value, param: definition.Params[0].Value, expr: expr}, nil
}

func validAOTMethodName(name string) bool {
	if name == "" {
		return false
	}
	for index, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (index > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	switch name {
	case "break", "default", "func", "interface", "select", "case", "defer", "go", "map", "struct", "chan", "else", "goto", "package", "switch", "const", "fallthrough", "if", "range", "type", "continue", "for", "import", "return", "var",
		"main", "init", "maxInt64", "minInt64", "checkedAdd", "checkedSub", "checkedMul", "checkedMod", "checkedNeg", "bitAnd", "bitOr", "bitXor":
		return false
	default:
		return true
	}
}

type sourceAssignment struct {
	name  string
	value ast.Expression
}

func sourceAssignmentStatement(statement ast.Statement) (sourceAssignment, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return sourceAssignment{}, false
	}
	assignment, ok := expressionStatement.Expression.(*ast.AssignExpression)
	if !ok || assignment.Name == nil || assignment.Target != nil || assignment.Index != nil || assignment.End != nil {
		return sourceAssignment{}, false
	}
	value := assignment.Value
	if assignment.Token.Literal != "" && assignment.Token.Literal != "=" {
		operator := map[string]string{
			"+=": "+", "-=": "-", "*=": "*", "%=": "%",
			"&=": "&", "|=": "|", "^=": "^", "<<=": "<<", ">>=": ">>",
		}[assignment.Token.Literal]
		if operator == "" {
			return sourceAssignment{}, false
		}
		value = &ast.InfixExpression{
			Token:    assignment.Token,
			Left:     &ast.Identifier{Token: assignment.Name.Token, Value: assignment.Name.Value},
			Operator: operator,
			Right:    assignment.Value,
		}
	}
	return sourceAssignment{name: assignment.Name.Value, value: value}, true
}

func sourceIntegerConstant(expression ast.Expression) (int64, bool) {
	literal, ok := expression.(*ast.IntegerLiteral)
	if !ok || literal == nil {
		return 0, false
	}
	return literal.Value, true
}

func sourceWhileCondition(condition ast.Expression) (counter string, limit int64, limitName string, ok bool) {
	infix, ok := condition.(*ast.InfixExpression)
	if !ok || infix == nil || infix.Operator != "<" {
		return "", 0, "", false
	}
	left, leftOK := infix.Left.(*ast.Identifier)
	if !leftOK || left == nil {
		return "", 0, "", false
	}
	if value, constantOK := sourceIntegerConstant(infix.Right); constantOK {
		return left.Value, value, "", true
	}
	right, rightOK := infix.Right.(*ast.Identifier)
	if !rightOK || right == nil {
		return "", 0, "", false
	}
	return left.Value, 0, right.Value, true
}

func sourcePutsLocal(statement ast.Statement, locals map[string]int, localID func(string) int) (int, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement.Expression == nil {
		return 0, false
	}
	call, ok := expressionStatement.Expression.(*ast.MethodCall)
	if !ok || call == nil || call.Receiver != nil || call.Method == nil || call.Method.Value != "puts" ||
		len(call.Args) != 1 || len(call.KeywordArgs) > 0 || call.Block != nil {
		return 0, false
	}
	identifier, ok := call.Args[0].(*ast.Identifier)
	if !ok || identifier == nil {
		return 0, false
	}
	return localID(identifier.Value), true
}

func sourceIntegerExpression(node ast.Expression, locals map[string]int, localID func(string) int, methods map[string]*integerMethod) (*expression, error) {
	if node == nil {
		return nil, unsupported("missing Integer expression")
	}
	switch node := node.(type) {
	case *ast.IntegerLiteral:
		return &expression{isConstant: true, value: node.Value}, nil
	case *ast.Identifier:
		if node == nil || node.Value == "" {
			return nil, unsupported("invalid local")
		}
		return &expression{local: localID(node.Value)}, nil
	case *ast.PrefixExpression:
		if node == nil || node.Operator != "-" {
			return nil, unsupported("unsupported Integer prefix")
		}
		right, err := sourceIntegerExpression(node.Right, locals, localID, methods)
		if err != nil {
			return nil, err
		}
		return &expression{op: compiler.OpNeg, left: right}, nil
	case *ast.InfixExpression:
		if node == nil {
			return nil, unsupported("invalid Integer infix expression")
		}
		op, ok := sourceIntegerOperator(node.Operator)
		if !ok {
			return nil, unsupported("unsupported Integer operator %s", node.Operator)
		}
		left, err := sourceIntegerExpression(node.Left, locals, localID, methods)
		if err != nil {
			return nil, err
		}
		right, err := sourceIntegerExpression(node.Right, locals, localID, methods)
		if err != nil {
			return nil, err
		}
		return &expression{op: op, left: left, right: right}, nil
	case *ast.MethodCall:
		if node == nil || node.Receiver != nil || node.Method == nil || len(node.Args) != 1 ||
			len(node.KeywordArgs) > 0 || node.Block != nil || node.Safe {
			return nil, unsupported("unsupported Integer method call")
		}
		if _, ok := methods[node.Method.Value]; !ok {
			return nil, unsupported("method %s is not a recognized pure Integer function", node.Method.Value)
		}
		argument, err := sourceIntegerExpression(node.Args[0], locals, localID, methods)
		if err != nil {
			return nil, err
		}
		return &expression{callName: node.Method.Value, callArg: argument}, nil
	default:
		return nil, unsupported("unsupported Integer expression %T", node)
	}
}

func sourceIntegerOperator(operator string) (compiler.Opcode, bool) {
	switch operator {
	case "+":
		return compiler.OpAdd, true
	case "-":
		return compiler.OpSub, true
	case "*":
		return compiler.OpMul, true
	case "%":
		return compiler.OpMod, true
	case "&":
		return compiler.OpBitAnd, true
	case "|":
		return compiler.OpBitOr, true
	case "^":
		return compiler.OpBitXor, true
	case ">>":
		return compiler.OpBitRightShift, true
	case "<<":
		return compiler.OpBitLeftShift, true
	default:
		return 0, false
	}
}
