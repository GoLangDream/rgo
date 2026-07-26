package vm

import (
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/object"
)

func (vm *VM) matchPattern(target *object.EmeraldValue, source string, frame *Frame) (bool, *object.EmeraldValue) {
	bindings := make(map[string]*object.EmeraldValue)
	matched, errVal := vm.matchPatternValue(target, strings.TrimSpace(source), frame, bindings)
	if errVal != nil || !matched {
		return matched, errVal
	}
	for name, value := range bindings {
		vm.bindPatternLocal(frame, name, value)
	}
	return true, nil
}

func (vm *VM) matchPatternValue(target *object.EmeraldValue, pattern string, frame *Frame, bindings map[string]*object.EmeraldValue) (bool, *object.EmeraldValue) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false, nil
	}
	if parts := splitPatternTopLevel(pattern, '|'); len(parts) > 1 {
		for _, part := range parts {
			trial := clonePatternBindings(bindings)
			matched, errVal := vm.matchPatternValue(target, part, frame, trial)
			if errVal != nil {
				return false, errVal
			}
			if matched {
				for key, value := range trial {
					bindings[key] = value
				}
				return true, nil
			}
		}
		return false, nil
	}
	if left, right, ok := splitPatternOperator(pattern, "=>"); ok {
		matched, errVal := vm.matchPatternValue(target, left, frame, bindings)
		if matched && errVal == nil {
			bindPatternName(bindings, strings.TrimSpace(right), target)
		}
		return matched, errVal
	}
	if strings.HasPrefix(pattern, "^") {
		source := strings.TrimSpace(strings.TrimPrefix(pattern, "^"))
		if isSimplePatternLocalName(source) {
			value := bindings[source]
			if value == nil {
				value = vm.patternLocal(frame, source)
			}
			return vm.patternPinMatches(value, target)
		}
		if strings.HasPrefix(source, "(") && strings.HasSuffix(source, ")") {
			source = strings.TrimSpace(source[1 : len(source)-1])
		} else if strings.HasPrefix(source, "@") || strings.HasPrefix(source, "$") {
			source = strings.ReplaceAll(source, " ", "")
		}
		value := vm.evalSourceWithBinding(source, vm.currentFrameBinding())
		if value != nil && value.Type == object.ValueException {
			return false, value
		}
		return vm.patternPinMatches(value, target)
	}
	if open := firstPatternContainer(pattern); open > 0 && isPatternConstantName(strings.TrimSpace(pattern[:open])) {
		name := strings.TrimSpace(pattern[:open])
		constant, ok := vm.lexicalConstantValue(name)
		if !ok {
			constant, ok = vm.topLevelConstantValue(name)
		}
		if !ok || constant == nil {
			return false, nil
		}
		caseResult := vm.send(constant, "===", []*object.EmeraldValue{target})
		if caseResult == nil || caseResult.Type == object.ValueException {
			return false, caseResult
		}
		if !caseResult.IsTruthy() {
			return false, nil
		}
		inner := pattern[open:]
		return vm.matchPatternValue(target, normalizePatternContainer(inner), frame, bindings)
	}
	if strings.HasPrefix(pattern, "[") && strings.HasSuffix(pattern, "]") {
		return vm.matchArrayPattern(target, pattern[1:len(pattern)-1], frame, bindings)
	}
	if strings.HasPrefix(pattern, "{") && strings.HasSuffix(pattern, "}") {
		return vm.matchHashPattern(target, pattern[1:len(pattern)-1], frame, bindings)
	}
	if _, _, ok := splitPatternOperator(pattern, ":"); ok {
		return vm.matchHashPattern(target, pattern, frame, bindings)
	}
	if hasTopLevelPatternComma(pattern) {
		return vm.matchArrayPattern(target, pattern, frame, bindings)
	}
	return vm.matchPatternAtom(target, pattern, frame, bindings)
}

func (vm *VM) matchArrayPattern(target *object.EmeraldValue, body string, frame *Frame, bindings map[string]*object.EmeraldValue) (bool, *object.EmeraldValue) {
	values, errVal := vm.patternArrayValues(target)
	if errVal != nil || values == nil {
		return false, errVal
	}
	parts := splitPatternTopLevel(body, ',')
	if len(parts) == 1 && strings.TrimSpace(parts[0]) == "" {
		parts = nil
	}
	partial := strings.HasSuffix(strings.TrimSpace(body), ",")
	if partial && len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	splat := -1
	lastSplat := -1
	for i, part := range parts {
		if strings.HasPrefix(strings.TrimSpace(part), "*") {
			if splat < 0 {
				splat = i
			}
			lastSplat = i
		}
	}
	if splat < 0 {
		if (!partial && len(values) != len(parts)) || (partial && len(values) < len(parts)) {
			return false, nil
		}
		for i, part := range parts {
			matched, errVal := vm.matchPatternValue(values[i], part, frame, bindings)
			if errVal != nil || !matched {
				return matched, errVal
			}
		}
		return true, nil
	}
	if lastSplat > splat {
		prefixCount := splat
		middleCount := lastSplat - splat - 1
		suffixCount := len(parts) - lastSplat - 1
		if len(values) < prefixCount+middleCount+suffixCount {
			return false, nil
		}
		for i := 0; i < prefixCount; i++ {
			matched, errVal := vm.matchPatternValue(values[i], parts[i], frame, bindings)
			if errVal != nil || !matched {
				return matched, errVal
			}
		}
		for i := 0; i < suffixCount; i++ {
			matched, errVal := vm.matchPatternValue(values[len(values)-suffixCount+i], parts[lastSplat+1+i], frame, bindings)
			if errVal != nil || !matched {
				return matched, errVal
			}
		}
		for middleStart := prefixCount; middleStart+middleCount <= len(values)-suffixCount; middleStart++ {
			trial := clonePatternBindings(bindings)
			matched := true
			for i := 0; i < middleCount; i++ {
				itemMatched, errVal := vm.matchPatternValue(values[middleStart+i], parts[splat+1+i], frame, trial)
				if errVal != nil {
					return false, errVal
				}
				if !itemMatched {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			preName := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[splat]), "*"))
			postName := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[lastSplat]), "*"))
			if preName != "" {
				bindPatternName(trial, preName, vm.arrayValue(values[prefixCount:middleStart]...))
			}
			if postName != "" {
				bindPatternName(trial, postName, vm.arrayValue(values[middleStart+middleCount:len(values)-suffixCount]...))
			}
			for name, value := range trial {
				bindings[name] = value
			}
			return true, nil
		}
		return false, nil
	}
	prefixCount := splat
	suffixCount := len(parts) - splat - 1
	if len(values) < prefixCount+suffixCount {
		return false, nil
	}
	for i := 0; i < prefixCount; i++ {
		matched, errVal := vm.matchPatternValue(values[i], parts[i], frame, bindings)
		if errVal != nil || !matched {
			return matched, errVal
		}
	}
	for i := 0; i < suffixCount; i++ {
		matched, errVal := vm.matchPatternValue(values[len(values)-suffixCount+i], parts[splat+1+i], frame, bindings)
		if errVal != nil || !matched {
			return matched, errVal
		}
	}
	restName := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[splat]), "*"))
	if restName != "" {
		rest := append([]*object.EmeraldValue(nil), values[prefixCount:len(values)-suffixCount]...)
		bindPatternName(bindings, restName, &object.EmeraldValue{Type: object.ValueArray, Data: rest, Class: core.R.Classes["Array"]})
	}
	return true, nil
}

func (vm *VM) matchHashPattern(target *object.EmeraldValue, body string, frame *Frame, bindings map[string]*object.EmeraldValue) (bool, *object.EmeraldValue) {
	hash, keys, errVal := vm.patternHashValue(target, body)
	if errVal != nil || hash == nil {
		return false, errVal
	}
	parts := splitPatternTopLevel(body, ',')
	required := 0
	exact := false
	restName := ""
	requiredNames := map[string]bool{}
	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" || part == "**" {
			continue
		}
		compactPart := strings.ReplaceAll(part, " ", "")
		if compactPart == "**nil" {
			exact = true
			continue
		}
		if strings.HasPrefix(compactPart, "**") {
			restName = strings.TrimPrefix(compactPart, "**")
			continue
		}
		keyText, valuePattern, ok := splitPatternOperator(part, ":")
		if !ok {
			return false, nil
		}
		keyName := strings.Trim(strings.TrimSpace(keyText), "'\"")
		value, found := patternHashLookup(hash, keyName)
		if !found {
			return false, nil
		}
		required++
		requiredNames[keyName] = true
		valuePattern = strings.TrimSpace(valuePattern)
		if valuePattern == "" {
			valuePattern = keyName
		}
		matched, errVal := vm.matchPatternValue(value, valuePattern, frame, bindings)
		if errVal != nil || !matched {
			return matched, errVal
		}
	}
	if (exact || strings.TrimSpace(body) == "") && len(hash) != required {
		return false, nil
	}
	if restName != "" && restName != "nil" {
		rest := &object.RHash{
			Pairs:  make(map[*object.EmeraldValue]*object.EmeraldValue),
			Keys:   make([]*object.EmeraldValue, 0, len(hash)),
			Hashes: make(map[*object.EmeraldValue]int64),
		}
		for _, key := range keys {
			value, found := hash[key]
			if !found {
				continue
			}
			if key != nil && key.Type == object.ValueSymbol {
				keyName, _ := key.Data.(string)
				if requiredNames[strings.TrimPrefix(keyName, ":")] {
					continue
				}
			}
			rest.Keys = append(rest.Keys, key)
			rest.Pairs[key] = value
		}
		bindPatternName(bindings, restName, &object.EmeraldValue{Type: object.ValueHash, Data: rest, Class: core.R.Classes["Hash"]})
	}
	return true, nil
}

func (vm *VM) matchPatternAtom(target *object.EmeraldValue, pattern string, frame *Frame, bindings map[string]*object.EmeraldValue) (bool, *object.EmeraldValue) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "_" || strings.HasPrefix(pattern, "_") {
		if pattern != "_" {
			bindPatternName(bindings, pattern, target)
		}
		return true, nil
	}
	switch pattern {
	case "nil":
		return target != nil && target.Type == object.ValueNil, nil
	case "true":
		return target == core.R.TrueVal || (target != nil && target.Type == object.ValueBool && target.Data == true), nil
	case "false":
		return target == core.R.FalseVal || (target != nil && target.Type == object.ValueBool && target.Data == false), nil
	}
	if integer, err := strconv.ParseInt(strings.ReplaceAll(pattern, "_", ""), 0, 64); err == nil {
		return target != nil && target.Type == object.ValueInteger && target.Data.(int64) == integer, nil
	}
	if len(pattern) >= 2 && ((pattern[0] == '"' && pattern[len(pattern)-1] == '"') || (pattern[0] == '\'' && pattern[len(pattern)-1] == '\'')) {
		value := vm.evalSourceWithBinding(pattern, vm.currentFrameBinding())
		if value != nil && value.Type == object.ValueException {
			return false, value
		}
		return vm.patternValuesEqual(value, target), nil
	}
	if strings.HasPrefix(pattern, ":") {
		return target != nil && target.Type == object.ValueSymbol && strings.TrimPrefix(target.Data.(string), ":") == strings.TrimPrefix(pattern, ":"), nil
	}
	if isPatternConstantName(pattern) {
		constant, ok := vm.lexicalConstantValue(pattern)
		if !ok {
			constant, ok = vm.topLevelConstantValue(pattern)
		}
		if !ok || constant == nil {
			return false, nil
		}
		result := vm.send(constant, "===", []*object.EmeraldValue{target})
		if result != nil && result.Type == object.ValueException {
			return false, result
		}
		return result != nil && result.IsTruthy(), nil
	}
	bindPatternName(bindings, pattern, target)
	return true, nil
}

func (vm *VM) patternArrayValues(target *object.EmeraldValue) ([]*object.EmeraldValue, *object.EmeraldValue) {
	if cached := vm.patternArrayCache[target]; cached != nil {
		if cached.Type == object.ValueArray {
			return cached.Data.([]*object.EmeraldValue), nil
		}
	}
	if !vm.patternRespondsTo(target, "deconstruct") {
		return nil, nil
	}
	result := vm.send(target, "deconstruct", nil)
	if result != nil && result.Type == object.ValueException {
		return nil, result
	}
	if !patternValueIsArray(result) {
		return nil, core.NewTypeError("deconstruct must return Array")
	}
	if result.Type == object.ValueArray {
		if vm.patternArrayCache != nil {
			vm.patternArrayCache[target] = result
		}
		return result.Data.([]*object.EmeraldValue), nil
	}
	return nil, nil
}

func (vm *VM) patternHashValue(target *object.EmeraldValue, body string) (map[*object.EmeraldValue]*object.EmeraldValue, []*object.EmeraldValue, *object.EmeraldValue) {
	_, refined := vm.lookupActiveRefinedMethod(target, "deconstruct_keys")
	if target != nil && target.Type == object.ValueHash && !refined {
		return executorHashToMap(target), executorHashKeys(target), nil
	}
	if !vm.patternRespondsTo(target, "deconstruct_keys") {
		return nil, nil, nil
	}
	keysArg := vm.patternDeconstructKeysArgument(body)
	result := vm.send(target, "deconstruct_keys", []*object.EmeraldValue{keysArg})
	if result != nil && result.Type == object.ValueException {
		return nil, nil, result
	}
	if !patternValueIsHash(result) {
		return nil, nil, core.NewTypeError("deconstruct_keys must return Hash")
	}
	return executorHashToMap(result), executorHashKeys(result), nil
}

func executorHashKeys(value *object.EmeraldValue) []*object.EmeraldValue {
	if value == nil || value.Type != object.ValueHash {
		return nil
	}
	if hash, ok := value.Data.(*object.RHash); ok {
		return append([]*object.EmeraldValue(nil), hash.Keys...)
	}
	pairs := executorHashToMap(value)
	keys := make([]*object.EmeraldValue, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	return keys
}

func (vm *VM) patternDeconstructKeysArgument(body string) *object.EmeraldValue {
	parts := splitPatternTopLevel(body, ',')
	keys := make([]*object.EmeraldValue, 0, len(parts))
	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		compact := strings.ReplaceAll(part, " ", "")
		if strings.HasPrefix(compact, "**") {
			if len(compact) > 2 && compact != "**nil" {
				return core.R.NilVal
			}
			continue
		}
		keyText, _, ok := splitPatternOperator(part, ":")
		if !ok {
			continue
		}
		name := strings.Trim(strings.TrimSpace(keyText), "'\"")
		keys = append(keys, &object.EmeraldValue{Type: object.ValueSymbol, Data: name, Class: core.R.Classes["Symbol"]})
	}
	return vm.arrayValue(keys...)
}

func (vm *VM) bindPatternLocal(frame *Frame, name string, value *object.EmeraldValue) {
	if frame == nil || frame.Fn == nil || name == "" || name == "_" {
		return
	}
	index, ok := frame.Fn.LocalNames[name]
	if !ok {
		if frame.Closure != nil {
			for index, freeName := range frame.Fn.FreeVarNames {
				if freeName == name && index < len(frame.Closure.Free) {
					setClosureValue(&frame.Closure.Free[index], value)
					return
				}
			}
		}
		return
	}
	slot := frame.Bp + index + 1
	if slot < 0 || slot >= len(vm.stack) {
		return
	}
	vm.stack[slot] = value
	if vm.sp <= slot {
		vm.sp = slot + 1
	}
}

func (vm *VM) patternLocal(frame *Frame, name string) *object.EmeraldValue {
	if frame == nil || frame.Fn == nil {
		return nil
	}
	index, ok := frame.Fn.LocalNames[name]
	if !ok {
		return nil
	}
	slot := frame.Bp + index + 1
	if slot < 0 || slot >= len(vm.stack) {
		return nil
	}
	return derefClosureValue(vm.stack[slot])
}

func (vm *VM) patternValuesEqual(left, right *object.EmeraldValue) bool {
	if left == nil || right == nil {
		return left == right
	}
	result := vm.equals(left, right)
	return result != nil && result.Type == object.ValueBool && result.Data.(bool)
}

func (vm *VM) patternPinMatches(pattern, target *object.EmeraldValue) (bool, *object.EmeraldValue) {
	if pattern == nil {
		return false, nil
	}
	result := vm.send(pattern, "===", []*object.EmeraldValue{target})
	if result != nil && result.Type == object.ValueException {
		return false, result
	}
	return result != nil && result.IsTruthy(), nil
}

func isSimplePatternLocalName(name string) bool {
	if name == "" || !((name[0] >= 'a' && name[0] <= 'z') || name[0] == '_') {
		return false
	}
	for i := 1; i < len(name); i++ {
		ch := name[i]
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

func bindPatternName(bindings map[string]*object.EmeraldValue, name string, value *object.EmeraldValue) {
	name = strings.TrimSpace(name)
	if name == "" || name == "_" || strings.HasPrefix(name, "^") {
		return
	}
	bindings[name] = value
}

func patternHashLookup(hash map[*object.EmeraldValue]*object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	for key, value := range hash {
		if key == nil || key.Type != object.ValueSymbol {
			continue
		}
		keyName, _ := key.Data.(string)
		if strings.TrimPrefix(keyName, ":") == name {
			return value, true
		}
	}
	return nil, false
}

func splitPatternTopLevel(source string, separator byte) []string {
	parts := []string{}
	start, depth := 0, 0
	quote := byte(0)
	for i := 0; i < len(source); i++ {
		ch := source[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		switch ch {
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
		default:
			if ch == separator && depth == 0 {
				parts = append(parts, strings.TrimSpace(source[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(source[start:]))
	return parts
}

func splitPatternOperator(source, operator string) (string, string, bool) {
	depth := 0
	quote := byte(0)
	for i := 0; i+len(operator) <= len(source); i++ {
		ch := source[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		switch ch {
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
		}
		if depth == 0 && strings.HasPrefix(source[i:], operator) {
			return source[:i], source[i+len(operator):], true
		}
	}
	return "", "", false
}

func firstPatternContainer(pattern string) int {
	leftBracket := strings.IndexByte(pattern, '[')
	leftParen := strings.IndexByte(pattern, '(')
	if leftBracket < 0 {
		return leftParen
	}
	if leftParen < 0 || leftBracket < leftParen {
		return leftBracket
	}
	return leftParen
}

func normalizePatternContainer(pattern string) string {
	if strings.HasPrefix(pattern, "(") && strings.HasSuffix(pattern, ")") {
		body := pattern[1 : len(pattern)-1]
		if _, _, ok := splitPatternOperator(body, ":"); ok {
			return "{" + body + "}"
		}
		return "[" + body + "]"
	}
	if strings.HasPrefix(pattern, "[") && strings.HasSuffix(pattern, "]") {
		body := pattern[1 : len(pattern)-1]
		if _, _, ok := splitPatternOperator(body, ":"); ok {
			return "{" + body + "}"
		}
	}
	return pattern
}

func hasTopLevelPatternComma(pattern string) bool {
	return len(splitPatternTopLevel(pattern, ',')) > 1
}

func isPatternConstantName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	last := name
	if index := strings.LastIndex(name, "::"); index >= 0 {
		last = name[index+2:]
	}
	return last != "" && last[0] >= 'A' && last[0] <= 'Z'
}

func clonePatternBindings(source map[string]*object.EmeraldValue) map[string]*object.EmeraldValue {
	clone := make(map[string]*object.EmeraldValue, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
