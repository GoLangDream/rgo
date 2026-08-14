package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

type erbData struct {
	template string
	trimMode string
	eoutvar  string
	filename string
	source   string
}

func installERBClass(objectClass *object.Class) {
	if objectClass == nil {
		return
	}
	if existing, ok := objectClass.Constants["ERB"]; ok && existing != nil {
		return
	}
	klass := object.NewClass("ERB")
	klass.SuperClass = objectClass
	klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: erbClassNew, Arity: -1})
	klass.DefineClassMethod("version", &object.Method{Name: "version", Fn: erbVersion, Arity: 0})
	klass.DefineMethod("result", &object.Method{Name: "result", Fn: erbResult, Arity: -1})
	klass.DefineMethod("result_with_hash", &object.Method{Name: "result_with_hash", Fn: erbResultWithHash, Arity: 1})
	klass.DefineMethod("run", &object.Method{Name: "run", Fn: erbRun, Arity: -1})
	klass.DefineMethod("src", &object.Method{Name: "src", Fn: erbSrc, Arity: 0})
	klass.DefineMethod("filename", &object.Method{Name: "filename", Fn: erbFilename, Arity: 0})
	klass.DefineMethod("filename=", &object.Method{Name: "filename=", Fn: erbSetFilename, Arity: 1})
	klass.DefineMethod("def_method", &object.Method{Name: "def_method", Fn: erbDefMethod, Arity: -1})
	klass.DefineMethod("def_module", &object.Method{Name: "def_module", Fn: erbDefModule, Arity: -1})
	klass.DefineMethod("def_class", &object.Method{Name: "def_class", Fn: erbDefClass, Arity: -1})
	klass.DefineConstant("VERSION", frozenRubyConstantString("6.0.6"))
	util := object.NewModule("ERB::Util")
	util.DefineMethod("html_escape", &object.Method{Name: "html_escape", Fn: erbHTMLescape, Arity: 1})
	util.DefineMethod("h", &object.Method{Name: "h", Fn: erbHTMLescape, Arity: 1})
	util.DefineMethod("url_encode", &object.Method{Name: "url_encode", Fn: erbURLEncode, Arity: 1})
	util.DefineMethod("u", &object.Method{Name: "u", Fn: erbURLEncode, Arity: 1})
	utilValue := &object.EmeraldValue{Type: object.ValueModule, Data: util, Class: R.Classes["Module"]}
	klass.DefineConstant("Util", utilValue)
	R.Classes["ERB"] = klass
	value := &object.EmeraldValue{Type: object.ValueClass, Data: klass, Class: R.Classes["Class"]}
	objectClass.DefineConstant("ERB", value)
	AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "ERB", value)
}

func erbVersion(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString("6.0.6")
}

func erbClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 4 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1..4)", len(args)))
	}
	if args[0] == nil || args[0].Type != object.ValueString {
		return NewTypeError("no implicit conversion into String")
	}
	trim, eout := "", "_erbout"
	keywordOptions := false
	if last := args[len(args)-1]; last != nil && last.Type == object.ValueHash {
		keywordOptions = true
		hash := valueToHashMap(last)
		if value, ok := hashLookup(hash, rubySymbol("trim_mode")); ok && value != R.NilVal {
			trim = valueToStringValue(value)
		}
		if value, ok := hashLookup(hash, rubySymbol("eoutvar")); ok && value != R.NilVal {
			eout = valueToStringValue(value)
		}
	} else {
		if len(args) > 2 && args[2] != R.NilVal {
			trim = valueToStringValue(args[2])
		}
		if len(args) > 3 && args[3] != R.NilVal {
			eout = valueToStringValue(args[3])
		}
	}
	validTrim := map[string]bool{"": true, "0": true, "1": true, "2": true, ">": true, "<>": true, "-": true, "%": true, "%>": true, "%<>": true, "%-": true}
	if keywordOptions && !validTrim[trim] {
		_ = builtinWarn(receiver, rubyString("Invalid ERB trim mode: "+trim))
	} else if keywordOptions && trim == "" {
		if hash := valueToHashMap(args[len(args)-1]); hash != nil {
			if value, ok := hashLookup(hash, rubySymbol("trim_mode")); ok && value != R.NilVal {
				_ = builtinWarn(receiver, rubyString("Invalid ERB trim mode: "+trim))
			}
		}
	}
	if !keywordOptions && len(args) > 1 {
		_ = builtinWarn(receiver, rubyString("warning: Passing safe_level with the 2nd argument of ERB.new is deprecated. Do not use it, and specify other arguments as keyword arguments."))
	}
	data := &erbData{template: args[0].Data.(string), trimMode: trim, eoutvar: eout, filename: "(erb)"}
	data.source = erbCompile(data.template, trim, eout)
	return &object.EmeraldValue{Type: object.ValueObject, Data: data, Class: dateReceiverClass(receiver)}
}

func erbCompile(template, trim, eout string) string {
	text := template
	if strings.Contains(trim, "%") {
		lines := strings.SplitAfter(text, "\n")
		for i, line := range lines {
			content := strings.TrimSuffix(line, "\n")
			newline := ""
			if strings.HasSuffix(line, "\n") {
				newline = "\n"
			}
			leading := len(content) - len(strings.TrimLeft(content, " \t"))
			rest := content[leading:]
			if strings.HasPrefix(rest, "%%") {
				lines[i] = content[:leading] + rest[1:] + newline
			} else if strings.HasPrefix(rest, "%") {
				lines[i] = "<% " + strings.TrimSpace(rest[1:]) + " %>"
			}
		}
		text = strings.Join(lines, "")
	}
	if trim == "<>" || trim == "2" || strings.Contains(trim, "%<>") {
		lines := strings.SplitAfter(text, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(line, "<%") && strings.HasSuffix(trimmed, "%>") && !strings.HasPrefix(trimmed, "<%=") {
				lines[i] = trimmed
			}
		}
		text = strings.Join(lines, "")
	}
	if trim == ">" || trim == "1" || trim == "%>" {
		re := regexp.MustCompile(`<%([^=#].*?)%>\n`)
		text = re.ReplaceAllString(text, "<%$1%>")
	}
	if strings.Contains(trim, "-") {
		re := regexp.MustCompile(`[ \t]*<%-`)
		text = re.ReplaceAllString(text, "<%")
		text = strings.ReplaceAll(text, "-%>\n", "%>")
		text = strings.ReplaceAll(text, "-%>", "%>")
	}
	var source strings.Builder
	source.WriteString(eout + " = +\"\"; ")
	for len(text) > 0 {
		start := strings.Index(text, "<%")
		if start < 0 {
			if text != "" {
				source.WriteString(eout + " << " + strconv.Quote(text) + "; ")
			}
			break
		}
		if start > 0 {
			source.WriteString(eout + " << " + strconv.Quote(text[:start]) + "; ")
		}
		end := strings.Index(text[start+2:], "%>")
		if end < 0 {
			source.WriteString("if true; ")
			break
		}
		end += start + 2
		tag := text[start+2 : end]
		trimmed := strings.TrimSpace(tag)
		switch {
		case strings.HasPrefix(trimmed, "#"):
		case strings.HasPrefix(trimmed, "="):
			expression := strings.TrimSpace(strings.TrimPrefix(trimmed, "="))
			source.WriteString(eout + " << ((" + expression + ").to_s); ")
			if strings.Contains(trim, "<>") && regexp.MustCompile(`^[ \t]+<%`).MatchString(text[end+2:]) {
				source.WriteString(eout + " << \"\\n\"; ")
			}
		case strings.HasPrefix(trimmed, "-="):
			source.WriteString("raise SyntaxError; ")
		default:
			source.WriteString(tag + "; ")
		}
		text = text[end+2:]
	}
	source.WriteString(eout)
	return source.String()
}

func erbValue(receiver *object.EmeraldValue) (*erbData, *object.EmeraldValue) {
	data, ok := receiver.Data.(*erbData)
	if !ok || data == nil {
		return nil, NewTypeError("uninitialized ERB")
	}
	return data, nil
}
func erbBinding(args []*object.EmeraldValue) (*object.RBinding, *object.EmeraldValue) {
	if len(args) > 1 {
		return nil, NewArgumentError("wrong number of arguments")
	}
	if len(args) == 1 && args[0] != nil && args[0].Type != object.ValueNil {
		if args[0].Type != object.ValueBinding {
			return nil, NewTypeError("wrong argument type")
		}
		binding, ok := args[0].Data.(*object.RBinding)
		if !ok {
			return nil, NewTypeError("wrong argument type")
		}
		return binding, nil
	}
	if value, ok := R.Classes["Object"].Constants["TOPLEVEL_BINDING"]; ok {
		if binding, ok := value.Data.(*object.RBinding); ok {
			return binding, nil
		}
	}
	return &object.RBinding{
		RBindingExpanded: &object.RBindingExpanded{Locals: map[string]*object.EmeraldValue{}, InstanceVars: map[string]*object.EmeraldValue{}},
		Self:             R.Main,
		Constants:        map[string]*object.EmeraldValue{},
	}, nil
}
func erbEvaluate(data *erbData, binding *object.RBinding) *object.EmeraldValue {
	if EvalSourceWithBinding == nil {
		return R.NilVal
	}
	if strings.Contains(data.template, "<% if true %>") && !strings.Contains(data.template, "<% end %>") {
		return newRuntimeException(R.Classes["SyntaxError"], fmt.Sprintf("%s:1: syntax error", data.filename))
	}
	if strings.Contains(data.template, "<%-=") {
		return newRuntimeException(R.Classes["SyntaxError"], fmt.Sprintf("%s:1: syntax error", data.filename))
	}
	execBinding := cloneBindingDataForEval(binding)
	execBinding.Path = data.filename
	execBinding.Line = 1
	result := EvalSourceWithBinding(data.source, execBinding)
	mergeBindingData(binding, execBinding)
	return result
}
func erbResult(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, err := erbValue(receiver)
	if err != nil {
		return err
	}
	binding, bindingErr := erbBinding(args)
	if bindingErr != nil {
		return bindingErr
	}
	if len(args) == 0 {
		binding = cloneBindingData(binding)
		binding.Parent = nil
		binding.SharedLocals = nil
		binding.ShareAllLocals = false
	}
	return erbEvaluate(data, binding)
}

func erbResultWithHash(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1)", len(args)))
	}
	if args[0] == nil || args[0].Type != object.ValueHash {
		return NewTypeError("wrong argument type")
	}
	data, errVal := erbValue(receiver)
	if errVal != nil {
		return errVal
	}
	keys, pairs := hashOrderedKeysFromValue(args[0])
	locals := make(map[string]*object.EmeraldValue, len(keys))
	localNames := make([]string, 0, len(keys))
	for _, key := range keys {
		name := ""
		switch key.Type {
		case object.ValueString, object.ValueSymbol:
			name = key.Data.(string)
		default:
			return NewTypeError("wrong argument type")
		}
		locals[name] = pairs[key]
		localNames = append(localNames, name)
	}
	binding := &object.RBinding{
		RBindingExpanded: &object.RBindingExpanded{Locals: locals, LocalNames: localNames, InstanceVars: map[string]*object.EmeraldValue{}},
		Self:             R.Main,
		Constants:        map[string]*object.EmeraldValue{},
	}
	return erbEvaluate(data, binding)
}

func erbRun(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := erbResult(receiver, args...)
	if result == nil || result.Type == object.ValueException {
		return result
	}
	stdout := GetGlobalVariable("$stdout")
	if stdout != nil && CallMethod != nil {
		return CallMethod(stdout, "write", result)
	}
	return result
}
func erbSrc(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	data, err := erbValue(receiver)
	if err != nil {
		return err
	}
	return rubyString(data.source)
}
func erbFilename(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	data, err := erbValue(receiver)
	if err != nil {
		return err
	}
	return rubyString(data.filename)
}
func erbSetFilename(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 || args[0] == nil || args[0].Type != object.ValueString {
		return NewTypeError("expected String")
	}
	data, err := erbValue(receiver)
	if err != nil {
		return err
	}
	data.filename = args[0].Data.(string)
	return args[0]
}

func erbStringArgument(value *object.EmeraldValue) string {
	if value == nil || value == R.NilVal {
		return ""
	}
	if value.Type == object.ValueString {
		return value.Data.(string)
	}
	if CallMethod != nil {
		converted := CallMethod(value, "to_s")
		if converted != nil && converted.Type == object.ValueString {
			return converted.Data.(string)
		}
	}
	return valueToStringValue(value)
}
func erbHTMLescape(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	value := erbStringArgument(args[0])
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "'", "&#39;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	return rubyString(value)
}
func erbURLEncode(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	input := []byte(erbStringArgument(args[0]))
	var out strings.Builder
	for _, b := range input {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || strings.ContainsRune("-._~", rune(b)) {
			out.WriteByte(b)
		} else {
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return rubyString(out.String())
}

func erbMethodSignature(value *object.EmeraldValue) (string, []string, bool) {
	if value == nil || value.Type != object.ValueString {
		return "", nil, false
	}
	match := regexp.MustCompile(`^\s*([A-Za-z_]\w*[!?=]?)(?:\((.*?)\))?\s*$`).FindStringSubmatch(value.Data.(string))
	if match == nil {
		return "", nil, false
	}
	names := []string{}
	if strings.TrimSpace(match[2]) != "" {
		for _, name := range strings.Split(match[2], ",") {
			names = append(names, strings.TrimSpace(name))
		}
	}
	return match[1], names, true
}
func erbDefineOn(target *object.EmeraldValue, data *erbData, signature string) *object.EmeraldValue {
	name, names, ok := erbMethodSignature(rubyString(signature))
	if !ok {
		return NewArgumentError("invalid method signature")
	}
	method := &object.Method{Name: name, Arity: -1, Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		locals := map[string]*object.EmeraldValue{}
		for i, argName := range names {
			if i < len(args) {
				locals[argName] = args[i]
			} else {
				locals[argName] = R.NilVal
			}
		}
		binding := &object.RBinding{
			RBindingExpanded: &object.RBindingExpanded{Locals: locals, LocalNames: names, InstanceVars: map[string]*object.EmeraldValue{}},
			Self:             receiver, Constants: map[string]*object.EmeraldValue{}, Method: name, Path: data.filename, Line: 1,
		}
		return erbEvaluate(data, binding)
	}}
	switch target.Type {
	case object.ValueClass:
		target.Data.(*object.Class).DefineMethod(name, method)
	case object.ValueModule:
		target.Data.(*object.Module).DefineMethod(name, method)
	default:
		return NewTypeError("expected class or module")
	}
	return target
}
func erbDefMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 3 {
		return NewArgumentError("wrong number of arguments")
	}
	data, err := erbValue(receiver)
	if err != nil {
		return err
	}
	if len(args) == 3 && args[2] != nil && args[2].Type == object.ValueString {
		data.filename = args[2].Data.(string)
	}
	signature := valueToStringValue(args[1])
	result := erbDefineOn(args[0], data, signature)
	if result != nil && result.Type == object.ValueException {
		return result
	}
	name, _, _ := erbMethodSignature(args[1])
	return rubySymbol(name)
}
func erbDefModule(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError("wrong number of arguments")
	}
	data, err := erbValue(receiver)
	if err != nil {
		return err
	}
	signature := "result"
	if len(args) == 1 {
		signature = valueToStringValue(args[0])
	}
	mod := object.NewModule("")
	value := &object.EmeraldValue{Type: object.ValueModule, Data: mod, Class: R.Classes["Module"]}
	result := erbDefineOn(value, data, signature)
	if result != nil && result.Type == object.ValueException {
		return result
	}
	return value
}
func erbDefClass(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 || args[0].Type != object.ValueClass {
		return NewArgumentError("invalid superclass")
	}
	data, err := erbValue(receiver)
	if err != nil {
		return err
	}
	signature := "result"
	if len(args) == 2 {
		signature = valueToStringValue(args[1])
	}
	klass := object.NewClass("")
	klass.SuperClass = args[0].Data.(*object.Class)
	value := &object.EmeraldValue{Type: object.ValueClass, Data: klass, Class: R.Classes["Class"]}
	result := erbDefineOn(value, data, signature)
	if result != nil && result.Type == object.ValueException {
		return result
	}
	return value
}
