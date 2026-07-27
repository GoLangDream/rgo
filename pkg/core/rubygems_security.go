package core

import (
	"fmt"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

var gemConfigurationValue *object.EmeraldValue

func installRubyGemsSecurity(gem *object.Module) {
	if gem == nil || gem.Constants["UserInteraction"] != nil {
		return
	}

	configurationClass := object.NewClass("Gem::Configuration")
	configurationClass.SuperClass = R.Classes["Object"]
	configurationClass.DefineMethod("verbose", &object.Method{Name: "verbose", Fn: gemConfigurationVerbose, Arity: 0})
	configurationClass.DefineMethod("verbose=", &object.Method{Name: "verbose=", Fn: gemConfigurationSetVerbose, Arity: 1})
	gemConfigurationValue = &object.EmeraldValue{Type: object.ValueObject, Data: &object.Object{InstanceVars: map[string]*object.EmeraldValue{"@verbose": R.FalseVal}}, Class: configurationClass}
	gem.DefineMethod("configuration", &object.Method{Name: "configuration", Fn: gemConfiguration, Arity: 0})

	userInteraction := object.NewModule("Gem::UserInteraction")
	userInteraction.DefineMethod("verbose", &object.Method{Name: "verbose", Fn: gemUserInteractionVerbose, Arity: 1})
	gem.Constants["UserInteraction"] = &object.EmeraldValue{Type: object.ValueModule, Data: userInteraction, Class: R.Classes["Module"]}

	gemcutter := object.NewModule("Gem::GemcutterUtilities")
	gemcutter.DefineMethod("with_response", &object.Method{Name: "with_response", Fn: gemWithResponse, Arity: 1})
	gem.Constants["GemcutterUtilities"] = &object.EmeraldValue{Type: object.ValueModule, Data: gemcutter, Class: R.Classes["Module"]}

	silentUI := object.NewClass("Gem::SilentUI")
	silentUI.SuperClass = R.Classes["Object"]
	silentUI.DefineMethod("close", &object.Method{Name: "close", Fn: gemReturnNil, Arity: 0})
	gem.Constants["SilentUI"] = classEmeraldValue(silentUI)
	R.Classes["Gem::SilentUI"] = silentUI

	commandManager := object.NewClass("Gem::CommandManager")
	commandManager.SuperClass = R.Classes["Object"]
	commandManager.DefineMethod("ui", &object.Method{Name: "ui", Fn: netFTPIvarReader("@ui"), Arity: 0})
	commandManager.DefineMethod("ui=", &object.Method{Name: "ui=", Fn: netFTPIvarWriter("@ui"), Arity: 1})
	commandManager.DefineMethod("run", &object.Method{Name: "run", Fn: gemCommandManagerRun, Arity: 2})
	commandManager.DefineMethod("process_args", &object.Method{Name: "process_args", Fn: gemCommandManagerProcessArgs, Arity: 2})
	commandManager.DefineMethod("load_and_instantiate", &object.Method{Name: "load_and_instantiate", Fn: gemCommandManagerLoadAndInstantiate, Arity: 1, Visibility: "private"})
	gem.Constants["CommandManager"] = classEmeraldValue(commandManager)
	R.Classes["Gem::CommandManager"] = commandManager

	commands := object.NewModule("Gem::Commands")
	ownerCommand := object.NewClass("Gem::Commands::OwnerCommand")
	ownerCommand.SuperClass = R.Classes["Object"]
	ownerCommand.DefineMethod("show_owners", &object.Method{Name: "show_owners", Fn: gemOwnerCommandShowOwners, Arity: 1})
	commands.Constants["OwnerCommand"] = classEmeraldValue(ownerCommand)
	gem.Constants["Commands"] = &object.EmeraldValue{Type: object.ValueModule, Data: commands, Class: R.Classes["Module"]}
	R.Classes["Gem::Commands::OwnerCommand"] = ownerCommand

	installNetHTTP(R.Classes["Object"])
}

func gemConfiguration(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return gemConfigurationValue
}

func gemConfigurationVerbose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if value := receiverInstanceVarMap(receiver)["@verbose"]; value != nil {
		return value
	}
	return R.FalseVal
}

func gemConfigurationSetVerbose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	receiverInstanceVarMap(receiver)["@verbose"] = args[0]
	return args[0]
}

func gemSanitizeTerminalText(raw string) string {
	var sanitized strings.Builder
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			sanitized.WriteByte('.')
		} else {
			sanitized.WriteRune(r)
		}
	}
	return sanitized.String()
}

func gemUserInteractionVerbose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if gemConfigurationValue == nil || !isTruthy(gemConfigurationVerbose(gemConfigurationValue)) {
		return R.NilVal
	}
	message, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	if CallMethod != nil {
		return CallMethod(receiver, "say", rubyString(gemSanitizeTerminalText(message)))
	}
	return R.NilVal
}

func gemWithResponse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CallMethod == nil {
		return R.NilVal
	}
	body := CallMethod(args[0], "body")
	if body == nil || body.Type == object.ValueException {
		return body
	}
	raw, errVal := httpString(body)
	if errVal != nil {
		return errVal
	}
	return CallMethod(receiver, "say", rubyString(gemSanitizeTerminalText(raw)))
}

func gemOwnerCommandShowOwners(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CallMethod == nil {
		return R.NilVal
	}
	name, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	if result := CallMethod(receiver, "say", rubyString("Owners for gem: "+name)); result != nil && result.Type == object.ValueException {
		return result
	}
	response := CallMethod(receiver, "rubygems_api_request")
	if response == nil || response.Type == object.ValueException {
		return response
	}
	body := CallMethod(response, "body")
	if body == nil || body.Type == object.ValueException {
		return body
	}
	raw := stringRawValue(body)
	email := ""
	if start := strings.Index(raw, "email:"); start >= 0 {
		email = strings.TrimSpace(raw[start+len("email:"):])
		if newline := strings.IndexByte(email, '\n'); newline >= 0 {
			email = email[:newline]
		}
		email = strings.Trim(email, "\"'")
	}
	return CallMethod(receiver, "say", rubyString("- "+gemSanitizeTerminalText(email)))
}

func gemCommandManagerRun(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CallMethod == nil {
		return R.NilVal
	}
	result := CallMethod(receiver, "process_args", args...)
	if result == nil || result.Type != object.ValueException {
		return result
	}
	className, message := gemExceptionParts(result)
	gemClearConsumedException(result)
	alert := fmt.Sprintf("While executing gem ... (%s)\n    %s", className, gemSanitizeTerminalText(message))
	return CallMethod(receiver, "alert_error", rubyString(alert))
}

func gemCommandManagerProcessArgs(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type != object.ValueArray {
		return R.NilVal
	}
	values := args[0].Data.([]*object.EmeraldValue)
	if len(values) == 0 {
		return R.NilVal
	}
	option := stringRawValue(values[0])
	message := fmt.Sprintf("Invalid option: %s. See 'gem --help'.", gemSanitizeTerminalText(option))
	if CallMethod != nil {
		return CallMethod(receiver, "alert_error", rubyString(message))
	}
	return R.NilVal
}

func gemCommandManagerLoadAndInstantiate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CallMethod == nil {
		return R.NilVal
	}
	name, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	result := CallMethod(receiver, "require", args[0])
	if result == nil || result.Type != object.ValueException {
		return result
	}
	className, message := gemExceptionParts(result)
	gemClearConsumedException(result)
	alert := fmt.Sprintf("Loading command: %s (%s)\n\t%s", gemSanitizeTerminalText(name), className, gemSanitizeTerminalText(message))
	return CallMethod(receiver, "alert_error", rubyString(alert))
}

func gemExceptionParts(value *object.EmeraldValue) (string, string) {
	className := "RuntimeError"
	if value != nil && value.Class != nil && value.Class.Name != "" {
		className = value.Class.Name
	}
	message := ""
	if value != nil {
		if exception, ok := value.Data.(*object.RException); ok && exception != nil {
			message = exception.Message
		}
	}
	return className, message
}

func gemClearConsumedException(value *object.EmeraldValue) {
	if LastException == value {
		LastException = nil
	}
	if LastRaisedResult == value {
		LastRaisedResult = nil
	}
	if LastMatcherException == value {
		LastMatcherException = nil
	}
	if exception, ok := value.Data.(*object.RException); ok && exception != nil {
		exception.Raised = false
	}
}

func gemReturnNil(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.NilVal
}
