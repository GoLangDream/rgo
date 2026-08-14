package aot

import (
	"math/big"
	"strings"

	"github.com/GoLangDream/rgo/pkg/parser/ast"
)

// buildSourceObjectPlan recognizes a small but useful object hot region:
//
//	class Box
//	  def initialize(value); @value = value; @tag = "box"; end
//	  def value; @value; end
//	end
//	values = Array.new(N) { |i| Box.new(i) }
//	out = values.map { |value| value.value }
//	puts out.length
//
// The compiler accepts only straight-line ivar stores, an exact builtin
// Array/Class construction shape, and an optional pure getter map whose only
// observable result is length or an affine integer sum. This is deliberately
// a closed-world proof: unknown methods, redefinitions, side effects, and
// value observations keep the compatibility VM path.
func buildSourceObjectPlan(program *ast.Program) (*plan, bool, error) {
	if program == nil || len(program.Statements) < 3 {
		return nil, false, nil
	}
	classExpr, ok := sourceObjectClassStatement(program.Statements[0])
	if !ok || classExpr.Name == nil || classExpr.Name.Value == "" || classExpr.SuperClass != nil {
		return nil, false, nil
	}
	fields, getterExpressions, initializerArity, ok := sourceObjectClassMethods(classExpr)
	if !ok {
		return nil, false, nil
	}

	valuesAssignment, ok := sourceAssignmentStatement(program.Statements[1])
	if !ok || valuesAssignment.name == "" {
		return nil, false, nil
	}
	arrayCall, ok := valuesAssignment.value.(*ast.MethodCall)
	if !ok || arrayCall == nil || !sourceObjectName(arrayCall.Receiver, "Array") || arrayCall.Method == nil ||
		arrayCall.Method.Value != "new" || len(arrayCall.Args) != 1 || len(arrayCall.KeywordArgs) != 0 || arrayCall.Block == nil {
		return nil, false, nil
	}
	count, ok := sourceIntegerConstant(arrayCall.Args[0])
	if !ok || count < 0 || count > maxValidatedIterations {
		if ok {
			return nil, true, unsupported("object loop count is outside the strict typed proof")
		}
		return nil, false, nil
	}
	block := arrayCall.Block
	if block == nil || block.RestParam != nil || block.BlockParam != nil || len(block.KeywordParams) != 0 ||
		block.KeywordRestParam != nil || len(block.Statements) != 1 || len(block.Params) > 1 ||
		!sourceObjectSimplePatterns(block) {
		return nil, false, nil
	}
	constructorStatement, ok := block.Statements[0].(*ast.ExpressionStatement)
	if !ok || constructorStatement.Expression == nil {
		return nil, false, nil
	}
	constructorCall, ok := constructorStatement.Expression.(*ast.MethodCall)
	if !ok || constructorCall == nil || !sourceObjectName(constructorCall.Receiver, classExpr.Name.Value) ||
		constructorCall.Method == nil || constructorCall.Method.Value != "new" || len(constructorCall.KeywordArgs) != 0 ||
		constructorCall.Block != nil || len(constructorCall.Args) > 1 {
		return nil, false, nil
	}
	if len(constructorCall.Args) != initializerArity {
		return nil, false, nil
	}
	if len(constructorCall.Args) == 1 {
		if _, literal := sourceIntegerConstant(constructorCall.Args[0]); !literal {
			if len(block.Params) != 1 {
				return nil, false, nil
			}
			parameter, parameterOK := constructorCall.Args[0].(*ast.Identifier)
			if !parameterOK || parameter == nil || parameter.Value != block.Params[0].Value {
				return nil, false, nil
			}
		}
	} else if len(block.Params) != 0 && len(block.Params) != 1 {
		return nil, false, nil
	}

	argument := objectFieldValue{}
	if len(constructorCall.Args) == 1 {
		argument, ok = sourceObjectFieldValue(constructorCall.Args[0], block)
		if !ok {
			return nil, false, nil
		}
	}
	for index := range fields {
		if fields[index].value.fromIndex {
			fields[index].kind = argument.kind
			fields[index].value = argument
			fields[index].value.expr = objectExprFromFieldValue(argument)
		}
	}

	objectPlan := &objectLoopPlan{count: count, fields: fields}
	statementIndex := 2
	if statementIndex < len(program.Statements)-1 {
		mapAssignment, mapOK := sourceAssignmentStatement(program.Statements[statementIndex])
		if !mapOK || mapAssignment.name == "" {
			return nil, false, nil
		}
		mapCall, mapOK := mapAssignment.value.(*ast.MethodCall)
		if !mapOK || mapCall == nil || !sourceObjectName(mapCall.Receiver, valuesAssignment.name) || mapCall.Method == nil ||
			mapCall.Method.Value != "map" || len(mapCall.Args) != 0 || len(mapCall.KeywordArgs) != 0 || mapCall.Block == nil {
			return nil, false, nil
		}
		mapBlock := mapCall.Block
		if len(mapBlock.Params) != 1 || mapBlock.RestParam != nil || mapBlock.BlockParam != nil || len(mapBlock.KeywordParams) != 0 ||
			mapBlock.KeywordRestParam != nil || len(mapBlock.Statements) != 1 || !sourceObjectSimplePatterns(mapBlock) {
			return nil, false, nil
		}
		getterStatement, getterOK := mapBlock.Statements[0].(*ast.ExpressionStatement)
		getterCall, getterCallOK := expressionMethodCall(getterStatement)
		if !getterOK || !getterCallOK || getterCall == nil || getterCall.Receiver == nil || getterCall.Method == nil ||
			len(getterCall.Args) != 0 || len(getterCall.KeywordArgs) != 0 || getterCall.Block != nil {
			return nil, false, nil
		}
		getterReceiver, receiverOK := getterCall.Receiver.(*ast.Identifier)
		if !receiverOK || getterReceiver == nil || getterReceiver.Value != mapBlock.Params[0].Value {
			return nil, false, nil
		}
		getterExpr, getterOK := getterExpressions[getterCall.Method.Value]
		if !getterOK || !sourceObjectIntegerExpression(getterExpr, fields) {
			return nil, false, nil
		}
		objectPlan.mapResult = true
		objectPlan.getterExpr = getterExpr
		objectPlan.getterField = objectExprDirectField(getterExpr)
		statementIndex++
		_ = mapAssignment
	}
	if statementIndex >= len(program.Statements) {
		return nil, false, nil
	}
	outputKind, outputOK := sourceObjectOutput(program.Statements[statementIndex], valuesAssignment.name, objectPlan.mapResult)
	if statementIndex != len(program.Statements)-1 || !outputOK {
		return nil, false, nil
	}
	if outputKind == objectOutputIntegerSum && (!objectPlan.mapResult || objectPlan.getterExpr == nil || !sourceObjectIntegerExpression(objectPlan.getterExpr, objectPlan.fields) || !sourceObjectSumSafe(objectPlan)) {
		return nil, false, nil
	}
	objectPlan.output = outputKind
	return &plan{mode: objectLoopMode, objectLoop: objectPlan}, true, nil
}

func sourceObjectClassStatement(statement ast.Statement) (*ast.ClassExpression, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement == nil {
		return nil, false
	}
	classExpr, ok := expressionStatement.Expression.(*ast.ClassExpression)
	return classExpr, ok && classExpr != nil
}

func sourceObjectClassMethods(classExpr *ast.ClassExpression) ([]objectFieldPlan, map[string]*objectExpr, int, bool) {
	if classExpr == nil || classExpr.Body == nil {
		return nil, nil, 0, false
	}
	var fields []objectFieldPlan
	fieldIndex := make(map[string]int)
	getters := make(map[string]*objectExpr)
	initializerFound := false
	for _, statement := range classExpr.Body.Statements {
		expressionStatement, ok := statement.(*ast.ExpressionStatement)
		if !ok || expressionStatement == nil {
			return nil, nil, 0, false
		}
		definition, ok := expressionStatement.Expression.(*ast.DefExpression)
		if !ok || definition == nil || definition.Name == nil || definition.Receiver != nil ||
			definition.RestParam != nil || definition.BlockParam != nil || len(definition.KeywordParams) != 0 ||
			definition.KeywordRestParam != nil || !sourceObjectSimpleFunctionPatterns(definition) || definition.Body == nil {
			return nil, nil, 0, false
		}
		if definition.Name.Value == "initialize" {
			if initializerFound || len(definition.Params) > 1 {
				return nil, nil, 0, false
			}
			initializerFound = true
			for _, bodyStatement := range definition.Body.Statements {
				assignment, assignmentOK := sourceObjectInstanceAssignment(bodyStatement)
				if !assignmentOK || assignment.Name == nil || assignment.Name.Value == "" {
					return nil, nil, 0, false
				}
				if _, exists := fieldIndex[assignment.Name.Value]; exists {
					return nil, nil, 0, false
				}
				value, valueOK := sourceObjectFieldValue(assignment.Value, nil)
				if !valueOK {
					if identifier, identifierOK := assignment.Value.(*ast.Identifier); identifierOK && identifier != nil && len(definition.Params) == 1 && identifier.Value == definition.Params[0].Value {
						value = objectFieldValue{kind: objectFieldInteger, fromIndex: true, expr: &objectExpr{kind: objectExprIndex}}
						valueOK = true
					}
				}
				if !valueOK {
					return nil, nil, 0, false
				}
				fieldIndex[assignment.Name.Value] = len(fields)
				fields = append(fields, objectFieldPlan{kind: value.kind, value: value})
			}
			continue
		}
		if len(definition.Params) != 0 || len(definition.Body.Statements) != 1 {
			return nil, nil, 0, false
		}
		getterStatement, getterOK := definition.Body.Statements[0].(*ast.ExpressionStatement)
		if !getterOK || getterStatement == nil {
			return nil, nil, 0, false
		}
		getterExpr, getterOK := sourceObjectExpression(getterStatement.Expression, fieldIndex)
		if !getterOK {
			return nil, nil, 0, false
		}
		if _, exists := getters[definition.Name.Value]; exists {
			return nil, nil, 0, false
		}
		getters[definition.Name.Value] = getterExpr
	}
	initializerArity := 0
	for _, statement := range classExpr.Body.Statements {
		if expressionStatement, ok := statement.(*ast.ExpressionStatement); ok {
			if definition, ok := expressionStatement.Expression.(*ast.DefExpression); ok && definition != nil && definition.Name != nil && definition.Name.Value == "initialize" {
				initializerArity = len(definition.Params)
				break
			}
		}
	}
	return fields, getters, initializerArity, initializerFound
}

func sourceObjectSimplePatterns(block *ast.BlockExpression) bool {
	if block == nil {
		return false
	}
	if block.RejectKeywords || block.RejectBlock || len(block.BlockLocals) != 0 || block.SingleDestructure || block.KeywordRestOnly {
		return false
	}
	for _, defaultValue := range block.ParamDefaults {
		if defaultValue != nil {
			return false
		}
	}
	for _, pattern := range block.ParamPatterns {
		if pattern != nil && (pattern.Name == nil || len(pattern.Children) != 0 || pattern.Rest != nil || pattern.RestIndex != 0) {
			return false
		}
	}
	return true
}

func sourceObjectSimpleFunctionPatterns(definition *ast.DefExpression) bool {
	if definition == nil {
		return false
	}
	if definition.RejectKeywords || definition.RejectBlock {
		return false
	}
	for _, defaultValue := range definition.ParamDefaults {
		if defaultValue != nil {
			return false
		}
	}
	for _, pattern := range definition.ParamPatterns {
		if pattern != nil && (pattern.Name == nil || len(pattern.Children) != 0 || pattern.Rest != nil || pattern.RestIndex != 0) {
			return false
		}
	}
	return true
}

func sourceObjectExpression(expression ast.Expression, fields map[string]int) (*objectExpr, bool) {
	switch value := expression.(type) {
	case *ast.IntegerLiteral:
		if value == nil {
			return nil, false
		}
		return &objectExpr{kind: objectExprInteger, integer: value.Value}, true
	case *ast.InstanceVariable:
		if value == nil || value.Name == "" {
			return nil, false
		}
		index, ok := fields[value.Name]
		if !ok {
			return nil, false
		}
		return &objectExpr{kind: objectExprField, field: index}, true
	case *ast.PrefixExpression:
		if value == nil || value.Operator != "-" {
			return nil, false
		}
		right, ok := sourceObjectExpression(value.Right, fields)
		if !ok {
			return nil, false
		}
		return &objectExpr{kind: objectExprNeg, left: right}, true
	case *ast.InfixExpression:
		if value == nil {
			return nil, false
		}
		left, leftOK := sourceObjectExpression(value.Left, fields)
		right, rightOK := sourceObjectExpression(value.Right, fields)
		if !leftOK || !rightOK {
			return nil, false
		}
		kind := objectExprKind(255)
		switch value.Operator {
		case "+":
			kind = objectExprAdd
		case "-":
			kind = objectExprSub
		case "*":
			kind = objectExprMul
		case "%":
			kind = objectExprMod
		default:
			return nil, false
		}
		return &objectExpr{kind: kind, left: left, right: right}, true
	default:
		return nil, false
	}
}

func objectExprFromFieldValue(value objectFieldValue) *objectExpr {
	if value.expr != nil {
		return value.expr
	}
	if value.kind != objectFieldInteger {
		return nil
	}
	return &objectExpr{kind: objectExprInteger, integer: value.integer}
}

func objectExprDirectField(expression *objectExpr) int {
	if expression == nil || expression.kind != objectExprField {
		return -1
	}
	return expression.field
}

func sourceObjectIntegerExpression(expression *objectExpr, fields []objectFieldPlan) bool {
	if expression == nil {
		return false
	}
	switch expression.kind {
	case objectExprInteger, objectExprIndex:
		return true
	case objectExprField:
		return expression.field >= 0 && expression.field < len(fields) && fields[expression.field].kind == objectFieldInteger
	case objectExprAdd, objectExprSub, objectExprMul:
		return sourceObjectIntegerExpression(expression.left, fields) && sourceObjectIntegerExpression(expression.right, fields)
	case objectExprNeg:
		return sourceObjectIntegerExpression(expression.left, fields)
	default:
		return false
	}
}

func sourceObjectInstanceAssignment(statement ast.Statement) (*ast.AssignExpression, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement == nil {
		return nil, false
	}
	assignment, ok := expressionStatement.Expression.(*ast.AssignExpression)
	if !ok || assignment == nil || assignment.Name == nil || assignment.Target != nil || assignment.Index != nil || assignment.End != nil || assignment.Token.Literal != "=" {
		return nil, false
	}
	return assignment, true
}

func sourceObjectFieldValue(expression ast.Expression, block *ast.BlockExpression) (objectFieldValue, bool) {
	switch value := expression.(type) {
	case *ast.IntegerLiteral:
		if value == nil {
			return objectFieldValue{}, false
		}
		return objectFieldValue{kind: objectFieldInteger, integer: value.Value, expr: &objectExpr{kind: objectExprInteger, integer: value.Value}}, true
	case *ast.StringLiteral:
		if value == nil || value.Command || value.Interpolates && strings.Contains(value.Value, "#{") {
			return objectFieldValue{}, false
		}
		return objectFieldValue{kind: objectFieldString, text: value.Value}, true
	case *ast.Identifier:
		if value == nil || block == nil || len(block.Params) != 1 || value.Value != block.Params[0].Value {
			return objectFieldValue{}, false
		}
		return objectFieldValue{kind: objectFieldInteger, fromIndex: true, expr: &objectExpr{kind: objectExprIndex}}, true
	default:
		return objectFieldValue{}, false
	}
}

func sourceObjectName(expression ast.Expression, expected string) bool {
	if expected == "" || expression == nil {
		return false
	}
	switch value := expression.(type) {
	case *ast.Constant:
		return value != nil && value.Name == expected
	case *ast.Identifier:
		return value != nil && value.Value == expected
	default:
		return false
	}
}

func expressionMethodCall(statement *ast.ExpressionStatement) (*ast.MethodCall, bool) {
	if statement == nil || statement.Expression == nil {
		return nil, false
	}
	call, ok := statement.Expression.(*ast.MethodCall)
	return call, ok && call != nil
}

func sourceObjectOutput(statement ast.Statement, valuesName string, mapResult bool) (objectOutputKind, bool) {
	expressionStatement, ok := statement.(*ast.ExpressionStatement)
	if !ok || expressionStatement == nil || expressionStatement.Expression == nil {
		return objectOutputLength, false
	}
	puts, ok := expressionStatement.Expression.(*ast.MethodCall)
	if !ok || puts == nil || puts.Receiver != nil || puts.Method == nil || puts.Method.Value != "puts" || len(puts.Args) != 1 || len(puts.KeywordArgs) != 0 || puts.Block != nil {
		return objectOutputLength, false
	}
	lengthCall, ok := puts.Args[0].(*ast.MethodCall)
	if !ok || lengthCall == nil || lengthCall.Method == nil || len(lengthCall.Args) != 0 || len(lengthCall.KeywordArgs) != 0 || lengthCall.Block != nil {
		return objectOutputLength, false
	}
	identifier, ok := lengthCall.Receiver.(*ast.Identifier)
	if !ok || identifier == nil {
		return objectOutputLength, false
	}
	if mapResult {
		if identifier.Value == valuesName {
			return objectOutputLength, false
		}
		switch lengthCall.Method.Value {
		case "length":
			return objectOutputLength, true
		case "sum":
			return objectOutputIntegerSum, true
		default:
			return objectOutputLength, false
		}
	}
	return objectOutputLength, identifier.Value == valuesName && lengthCall.Method.Value == "length"
}

func sourceObjectSumSafe(plan *objectLoopPlan) bool {
	if plan == nil || plan.getterExpr == nil {
		return false
	}
	if plan.count < 0 {
		return false
	}
	if plan.count == 0 {
		return true
	}
	a, b, ok := sourceObjectLinear(plan.getterExpr, plan.fields, make(map[int]bool))
	if !ok {
		return false
	}
	if !a.IsInt64() || !b.IsInt64() {
		return false
	}
	count := big.NewInt(plan.count)
	last := new(big.Int).Sub(new(big.Int).Set(count), big.NewInt(1))
	startValue := new(big.Int).Set(b)
	endValue := new(big.Int).Add(new(big.Int).Mul(a, last), b)
	if !startValue.IsInt64() || !endValue.IsInt64() {
		return false
	}
	triangular := new(big.Int).Mul(new(big.Int).Set(count), last)
	triangular.Quo(triangular, big.NewInt(2))
	termA := new(big.Int).Mul(a, triangular)
	termB := new(big.Int).Mul(b, count)
	if !termA.IsInt64() || !termB.IsInt64() {
		return false
	}
	total := new(big.Int).Add(termA, termB)
	return total.IsInt64()
}

// sourceObjectLinear resolves an integer getter to a*index+b. Multiplication
// is accepted only when one operand is constant; modulo is intentionally kept
// out of the sum tier until its Ruby floor-mod and periodic overflow proof is
// represented in the same IR. The conservative rejection is a side-exit, not
// a semantic change.
func sourceObjectLinear(expression *objectExpr, fields []objectFieldPlan, visiting map[int]bool) (*big.Int, *big.Int, bool) {
	if expression == nil {
		return nil, nil, false
	}
	zero := func() *big.Int { return big.NewInt(0) }
	constant := func(value int64) (*big.Int, *big.Int) { return zero(), big.NewInt(value) }
	switch expression.kind {
	case objectExprInteger:
		a, b := constant(expression.integer)
		return a, b, true
	case objectExprIndex:
		return big.NewInt(1), zero(), true
	case objectExprField:
		if expression.field < 0 || expression.field >= len(fields) || visiting[expression.field] {
			return nil, nil, false
		}
		field := fields[expression.field]
		if field.kind != objectFieldInteger {
			return nil, nil, false
		}
		value := field.value
		if value.expr == nil {
			a, b := constant(value.integer)
			return a, b, true
		}
		visiting[expression.field] = true
		a, b, ok := sourceObjectLinear(value.expr, fields, visiting)
		delete(visiting, expression.field)
		return a, b, ok
	case objectExprNeg:
		a, b, ok := sourceObjectLinear(expression.left, fields, visiting)
		if !ok {
			return nil, nil, false
		}
		return new(big.Int).Neg(a), new(big.Int).Neg(b), true
	case objectExprAdd, objectExprSub:
		leftA, leftB, leftOK := sourceObjectLinear(expression.left, fields, visiting)
		rightA, rightB, rightOK := sourceObjectLinear(expression.right, fields, visiting)
		if !leftOK || !rightOK {
			return nil, nil, false
		}
		if expression.kind == objectExprAdd {
			return new(big.Int).Add(leftA, rightA), new(big.Int).Add(leftB, rightB), true
		}
		return new(big.Int).Sub(leftA, rightA), new(big.Int).Sub(leftB, rightB), true
	case objectExprMul:
		leftA, leftB, leftOK := sourceObjectLinear(expression.left, fields, visiting)
		rightA, rightB, rightOK := sourceObjectLinear(expression.right, fields, visiting)
		if !leftOK || !rightOK {
			return nil, nil, false
		}
		if leftA.Sign() == 0 {
			return new(big.Int).Mul(leftB, rightA), new(big.Int).Mul(leftB, rightB), true
		}
		if rightA.Sign() == 0 {
			return new(big.Int).Mul(rightB, leftA), new(big.Int).Mul(rightB, leftB), true
		}
		return nil, nil, false
	default:
		return nil, nil, false
	}
}
