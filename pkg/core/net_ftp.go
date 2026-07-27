package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

var netFTPDefaultPassive = true

func installNetFTP(objectClass *object.Class) {
	if objectClass == nil {
		return
	}
	installSocketModule(objectClass)
	installOpenSSLModule(objectClass)

	netValue := objectClass.Constants["Net"]
	var netModule *object.Module
	if netValue != nil && netValue.Type == object.ValueModule {
		netModule, _ = netValue.Data.(*object.Module)
	}
	if netModule == nil {
		netModule = object.NewModule("Net")
		netValue = &object.EmeraldValue{Type: object.ValueModule, Data: netModule, Class: R.Classes["Module"]}
		objectClass.DefineConstant("Net", netValue)
		AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "Net", netValue)
	}
	if netModule.Constants["FTP"] != nil {
		return
	}

	ftpError := newNetFTPErrorClass("Net::FTPError", R.Classes["StandardError"])
	ftpPermError := newNetFTPErrorClass("Net::FTPPermError", ftpError)
	ftpTempError := newNetFTPErrorClass("Net::FTPTempError", ftpError)
	ftpProtoError := newNetFTPErrorClass("Net::FTPProtoError", ftpError)
	ftpReplyError := newNetFTPErrorClass("Net::FTPReplyError", ftpError)
	for name, class := range map[string]*object.Class{
		"FTPError":      ftpError,
		"FTPPermError":  ftpPermError,
		"FTPTempError":  ftpTempError,
		"FTPProtoError": ftpProtoError,
		"FTPReplyError": ftpReplyError,
	} {
		R.Classes["Net::"+name] = class
		netModule.DefineConstant(name, classEmeraldValue(class))
	}

	ftp := object.NewClass("Net::FTP")
	ftp.SuperClass = objectClass
	ftp.DefineConstant("FTP_PORT", newInt(21))
	ftp.DefineConstant("VERSION", rubyString("0.3.5"))
	ftp.DefineClassMethod("default_passive", &object.Method{Name: "default_passive", Fn: netFTPClassDefaultPassive, Arity: 0})
	ftp.DefineClassMethod("default_passive=", &object.Method{Name: "default_passive=", Fn: netFTPClassSetDefaultPassive, Arity: 1})
	ftp.DefineClassMethod("open", &object.Method{Name: "open", Fn: netFTPClassOpen, Arity: -1})
	ftp.DefineMethod("initialize", &object.Method{Name: "initialize", Fn: netFTPInitialize, Arity: -1, Visibility: "private"})

	for _, name := range []string{
		"binary", "passive", "debug_mode", "resume", "open_timeout", "read_timeout",
		"ssl_handshake_timeout", "last_response", "lastresp", "last_response_code",
	} {
		ivar := "@" + name
		ftp.DefineMethod(name, &object.Method{Name: name, Fn: netFTPIvarReader(ivar), Arity: 0})
		ftp.DefineMethod(name+"=", &object.Method{Name: name + "=", Fn: netFTPIvarWriter(ivar), Arity: 1})
	}
	ftp.DefineMethod("connect", &object.Method{Name: "connect", Fn: netFTPConnect, Arity: -1})
	ftp.DefineMethod("login", &object.Method{Name: "login", Fn: netFTPLogin, Arity: -1})
	ftp.DefineMethod("list", &object.Method{Name: "list", Fn: netFTPList, Arity: -1})
	ftp.DefineMethod("dir", &object.Method{Name: "dir", Fn: netFTPList, Arity: -1})
	ftp.DefineMethod("ls", &object.Method{Name: "ls", Fn: netFTPList, Arity: -1})
	ftp.DefineMethod("nlst", &object.Method{Name: "nlst", Fn: netFTPNlst, Arity: -1})
	ftp.DefineMethod("sendcmd", &object.Method{Name: "sendcmd", Fn: netFTPSendCommand, Arity: 1})
	ftp.DefineMethod("voidcmd", &object.Method{Name: "voidcmd", Fn: netFTPVoidCommand, Arity: 1})
	ftp.DefineMethod("abort", &object.Method{Name: "abort", Fn: netFTPAbort, Arity: 0})
	ftp.DefineMethod("acct", &object.Method{Name: "acct", Fn: netFTPArgumentVoidCommand("ACCT"), Arity: 1})
	ftp.DefineMethod("chdir", &object.Method{Name: "chdir", Fn: netFTPChdir, Arity: 1})
	ftp.DefineMethod("delete", &object.Method{Name: "delete", Fn: netFTPArgumentVoidCommand("DELE"), Arity: 1})
	ftp.DefineMethod("help", &object.Method{Name: "help", Fn: netFTPOptionalResponseCommand("HELP"), Arity: -1})
	ftp.DefineMethod("get", &object.Method{Name: "get", Fn: netFTPGet, Arity: -1})
	ftp.DefineMethod("getbinaryfile", &object.Method{Name: "getbinaryfile", Fn: netFTPGetBinaryFile, Arity: -1})
	ftp.DefineMethod("gettextfile", &object.Method{Name: "gettextfile", Fn: netFTPGetTextFile, Arity: -1})
	ftp.DefineMethod("noop", &object.Method{Name: "noop", Fn: netFTPNoArgumentVoidCommand("NOOP"), Arity: 0})
	ftp.DefineMethod("quit", &object.Method{Name: "quit", Fn: netFTPNoArgumentVoidCommand("QUIT"), Arity: 0})
	ftp.DefineMethod("rmdir", &object.Method{Name: "rmdir", Fn: netFTPArgumentVoidCommand("RMD"), Arity: 1})
	ftp.DefineMethod("site", &object.Method{Name: "site", Fn: netFTPArgumentVoidCommand("SITE"), Arity: 1})
	ftp.DefineMethod("status", &object.Method{Name: "status", Fn: netFTPOptionalResponseCommand("STAT"), Arity: -1})
	ftp.DefineMethod("rename", &object.Method{Name: "rename", Fn: netFTPRename, Arity: 2})
	ftp.DefineMethod("retrbinary", &object.Method{Name: "retrbinary", Fn: netFTPRetrieveBinary, Arity: -1})
	ftp.DefineMethod("retrlines", &object.Method{Name: "retrlines", Fn: netFTPRetrieveLines, Arity: 1})
	ftp.DefineMethod("storbinary", &object.Method{Name: "storbinary", Fn: netFTPStoreBinary, Arity: -1})
	ftp.DefineMethod("storlines", &object.Method{Name: "storlines", Fn: netFTPStoreLines, Arity: 2})
	ftp.DefineMethod("pwd", &object.Method{Name: "pwd", Fn: netFTPPwd, Arity: 0})
	ftp.DefineMethod("put", &object.Method{Name: "put", Fn: netFTPPut, Arity: -1})
	ftp.DefineMethod("putbinaryfile", &object.Method{Name: "putbinaryfile", Fn: netFTPPutBinaryFile, Arity: -1})
	ftp.DefineMethod("puttextfile", &object.Method{Name: "puttextfile", Fn: netFTPPutTextFile, Arity: -1})
	ftp.DefineMethod("mkdir", &object.Method{Name: "mkdir", Fn: netFTPMkdir, Arity: 1})
	ftp.DefineMethod("mdtm", &object.Method{Name: "mdtm", Fn: netFTPMdtm, Arity: 1})
	ftp.DefineMethod("mtime", &object.Method{Name: "mtime", Fn: netFTPMtime, Arity: -1})
	ftp.DefineMethod("size", &object.Method{Name: "size", Fn: netFTPSize, Arity: 1})
	ftp.DefineMethod("system", &object.Method{Name: "system", Fn: netFTPSystem, Arity: 0})
	ftp.DefineMethod("welcome", &object.Method{Name: "welcome", Fn: netFTPIvarReader("@welcome"), Arity: 0})
	ftp.DefineMethod("set_socket", &object.Method{Name: "set_socket", Fn: netFTPSetSocket, Arity: 1, Visibility: "private"})
	ftp.DefineMethod("close", &object.Method{Name: "close", Fn: netFTPClose, Arity: 0})
	ftp.DefineMethod("closed?", &object.Method{Name: "closed?", Fn: netFTPClosed, Arity: 0})
	ftp.DefineMethod("return_code", &object.Method{Name: "return_code", Fn: netFTPReturnCode, Arity: 0})
	ftp.DefineMethod("return_code=", &object.Method{Name: "return_code=", Fn: netFTPSetReturnCode, Arity: 1})

	R.Classes["Net::FTP"] = ftp
	netModule.DefineConstant("FTP", classEmeraldValue(ftp))
	netFTPDefaultPassive = true
}

func newNetFTPErrorClass(name string, superclass *object.Class) *object.Class {
	class := object.NewClass(name)
	class.SuperClass = superclass
	return class
}

func netFTPClassDefaultPassive(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(netFTPDefaultPassive)
}

func netFTPClassSetDefaultPassive(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	netFTPDefaultPassive = isTruthy(args[0])
	return args[0]
}

func netFTPClassOpen(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CallMethod == nil {
		return R.NilVal
	}
	ftp := CallMethod(receiver, "new", args...)
	if ftp == nil || ftp.Type == object.ValueException {
		return ftp
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), ftp)
		closeResult := CallMethod(ftp, "close")
		if result != nil && result.Type == object.ValueException {
			return result
		}
		if closeResult != nil && closeResult.Type == object.ValueException {
			return closeResult
		}
		return result
	}
	return ftp
}

func netFTPInitialize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	vars := receiverInstanceVarMap(receiver)
	vars["@binary"] = R.TrueVal
	vars["@passive"] = boolValue(netFTPDefaultPassive)
	vars["@debug_mode"] = R.FalseVal
	vars["@resume"] = R.FalseVal
	vars["@open_timeout"] = R.NilVal
	vars["@read_timeout"] = newInt(60)
	vars["@ssl_handshake_timeout"] = R.NilVal
	vars["@ssl_context"] = R.NilVal
	vars["@private_data_connection"] = R.FalseVal
	vars["@sock"] = R.NilVal
	vars["@last_response"] = R.NilVal
	vars["@lastresp"] = R.NilVal
	vars["@last_response_code"] = R.NilVal
	vars["@welcome"] = R.NilVal

	var host, user, password, account *object.EmeraldValue
	if len(args) > 0 {
		host = args[0]
	}
	if len(args) > 1 {
		user = args[1]
	}
	if len(args) > 2 {
		password = args[2]
	}
	if len(args) > 3 {
		account = args[3]
	}
	if len(args) > 4 {
		return NewArgumentError("wrong number of arguments")
	}

	options := (*object.EmeraldValue)(nil)
	if user != nil && user.Type != object.ValueString && user.Type != object.ValueNil {
		options = netFTPOptionsHash(user)
		if options == nil {
			return typeError("no implicit conversion into Hash")
		}
		user, password, account = nil, nil, nil
	}
	if options != nil {
		if value, ok := netFTPOption(options, "passive"); ok {
			vars["@passive"] = value
		}
		if value, ok := netFTPOption(options, "debug_mode"); ok {
			vars["@debug_mode"] = value
		}
		if value, ok := netFTPOption(options, "open_timeout"); ok {
			vars["@open_timeout"] = value
		}
		if value, ok := netFTPOption(options, "read_timeout"); ok {
			vars["@read_timeout"] = value
		}
		if value, ok := netFTPOption(options, "ssl_handshake_timeout"); ok {
			vars["@ssl_handshake_timeout"] = value
		}
		if value, ok := netFTPOption(options, "username"); ok {
			user = value
		}
		if value, ok := netFTPOption(options, "password"); ok {
			password = value
		}
		if value, ok := netFTPOption(options, "account"); ok {
			account = value
		}
		if ssl, ok := netFTPOption(options, "ssl"); ok && isTruthy(ssl) {
			contextClass := netFTPSSLContextValue()
			if contextClass != nil && CallMethod != nil {
				context := CallMethod(contextClass, "new")
				if context != nil && context.Type == object.ValueException {
					return context
				}
				params := emptyHashValue()
				if ssl.Type == object.ValueHash {
					params = ssl
				}
				if context != nil {
					if result := CallMethod(context, "set_params", params); result != nil && result.Type == object.ValueException {
						return result
					}
					vars["@ssl_context"] = context
				}
			}
			vars["@private_data_connection"] = R.TrueVal
			if value, exists := netFTPOption(options, "private_data_connection"); exists {
				vars["@private_data_connection"] = value
			}
		} else if value, exists := netFTPOption(options, "private_data_connection"); exists && isTruthy(value) {
			return NewArgumentError("private_data_connection can be set to true only when ssl is enabled")
		}
	}

	if host != nil && host.Type != object.ValueNil && CallMethod != nil {
		port := newInt(21)
		if options != nil {
			if value, ok := netFTPOption(options, "port"); ok {
				port = value
			}
		}
		if result := CallMethod(receiver, "connect", host, port); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	if user != nil && user.Type != object.ValueNil && CallMethod != nil {
		if password == nil {
			password = R.NilVal
		}
		if account == nil {
			account = R.NilVal
		}
		if result := CallMethod(receiver, "login", user, password, account); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return R.NilVal
}

func netFTPSSLContextValue() *object.EmeraldValue {
	opensslValue := R.Classes["Object"].Constants["OpenSSL"]
	if opensslValue == nil || opensslValue.Type != object.ValueModule {
		return nil
	}
	openssl, _ := opensslValue.Data.(*object.Module)
	if openssl == nil {
		return nil
	}
	sslValue := openssl.Constants["SSL"]
	if sslValue == nil || sslValue.Type != object.ValueModule {
		return nil
	}
	ssl, _ := sslValue.Data.(*object.Module)
	if ssl == nil {
		return nil
	}
	return ssl.Constants["SSLContext"]
}

func netFTPOptionsHash(value *object.EmeraldValue) *object.EmeraldValue {
	if value == nil {
		return nil
	}
	if value.Type == object.ValueHash {
		return value
	}
	if CallMethod == nil || !receiverHasCallableMethod(value, "to_hash") {
		return nil
	}
	converted := CallMethod(value, "to_hash")
	if converted != nil && converted.Type == object.ValueHash {
		return converted
	}
	return nil
}

func netFTPOption(options *object.EmeraldValue, name string) (*object.EmeraldValue, bool) {
	if options == nil || options.Type != object.ValueHash {
		return nil, false
	}
	for key, value := range valueToHashMap(options) {
		if strings.TrimPrefix(specName(key), ":") == name {
			return value, true
		}
	}
	return nil, false
}

func netFTPIvarReader(name string) func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue {
	return func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		if value := receiverInstanceVarMap(receiver)[name]; value != nil {
			return value
		}
		return R.NilVal
	}
}

func netFTPIvarWriter(name string) func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue {
	return func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		if len(args) != 1 {
			return NewArgumentError("wrong number of arguments")
		}
		receiverInstanceVarMap(receiver)[name] = args[0]
		return args[0]
	}
}

func netFTPSetSocket(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	receiverInstanceVarMap(receiver)["@sock"] = args[0]
	return args[0]
}

func netFTPConnect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	port := newInt(21)
	if len(args) == 2 {
		port = args[1]
	}
	socketClass := R.Classes["TCPSocket"]
	if socketClass == nil {
		return newRuntimeException(R.Classes["LoadError"], "socket is unavailable")
	}
	socket := tcpSocketOpen(classEmeraldValue(socketClass), args[0], port)
	if socket == nil || socket.Type == object.ValueException {
		return socket
	}
	receiverInstanceVarMap(receiver)["@sock"] = socket
	response := netFTPReadResponse(receiver, true)
	if response == nil || response.Type == object.ValueException {
		return response
	}
	if code := netFTPResponseCode(response); len(code) != 3 || code[0] != '2' {
		return netFTPResponseError(response, true)
	}
	return R.NilVal
}

func netFTPLogin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 3 {
		return NewArgumentError("wrong number of arguments")
	}
	user := "anonymous"
	password := "anonymous@"
	account := ""
	passwordProvided := false
	accountProvided := false
	if len(args) > 0 {
		var errVal *object.EmeraldValue
		user, errVal = httpString(args[0])
		if errVal != nil {
			return errVal
		}
	}
	if len(args) > 1 && args[1].Type != object.ValueNil {
		var errVal *object.EmeraldValue
		password, errVal = httpString(args[1])
		if errVal != nil {
			return errVal
		}
		passwordProvided = true
	} else if len(args) > 0 {
		password = ""
	}
	if len(args) > 2 && args[2].Type != object.ValueNil {
		var errVal *object.EmeraldValue
		account, errVal = httpString(args[2])
		if errVal != nil {
			return errVal
		}
		accountProvided = true
	}
	response := netFTPCommand(receiver, "USER "+user, true)
	if response.Type == object.ValueException {
		return response
	}
	code := netFTPResponseCode(response)
	if code == "331" {
		if !passwordProvided && len(args) > 0 {
			return netFTPReplyException(response)
		}
		response = netFTPCommand(receiver, "PASS "+password, true)
		if response.Type == object.ValueException {
			return response
		}
		code = netFTPResponseCode(response)
	}
	if code == "332" {
		if !accountProvided {
			return netFTPReplyException(response)
		}
		response = netFTPCommand(receiver, "ACCT "+account, true)
		if response.Type == object.ValueException {
			return response
		}
		code = netFTPResponseCode(response)
	}
	if len(code) != 3 || code[0] != '2' {
		return netFTPResponseError(response, true)
	}
	receiverInstanceVarMap(receiver)["@welcome"] = response
	return R.NilVal
}

func netFTPReadResponse(receiver *object.EmeraldValue, update bool) *object.EmeraldValue {
	socket := receiverInstanceVarMap(receiver)["@sock"]
	if socket == nil || socket.Type == object.ValueNil {
		return newRuntimeException(R.Classes["IOError"], "not connected")
	}
	line := socketGets(socket, rubyString("\n"))
	if line == nil || line.Type == object.ValueException || line.Type == object.ValueNil {
		return line
	}
	raw := stringRawValue(line)
	full := raw
	if len(raw) >= 4 && raw[3] == '-' {
		code := raw[:3] + " "
		for {
			next := socketGets(socket, rubyString("\n"))
			if next == nil || next.Type == object.ValueException || next.Type == object.ValueNil {
				return next
			}
			nextRaw := stringRawValue(next)
			full += nextRaw
			if strings.HasPrefix(nextRaw, code) {
				break
			}
		}
		line = rubyString(full)
	}
	if update {
		netFTPStoreResponse(receiver, line)
	}
	return line
}

func netFTPStoreResponse(receiver, response *object.EmeraldValue) {
	code := netFTPResponseCode(response)
	vars := receiverInstanceVarMap(receiver)
	vars["@last_response"] = response
	vars["@lastresp"] = rubyString(code)
	vars["@last_response_code"] = rubyString(code)
}

func netFTPResponseCode(response *object.EmeraldValue) string {
	if response == nil || response.Type != object.ValueString {
		return ""
	}
	raw := stringRawValue(response)
	if len(raw) < 3 {
		return raw
	}
	return raw[:3]
}

func netFTPCommand(receiver *object.EmeraldValue, command string, acceptReply bool) *object.EmeraldValue {
	response := netFTPRawCommand(receiver, command)
	if response == nil || response.Type == object.ValueException {
		return response
	}
	code := netFTPResponseCode(response)
	if len(code) != 3 || code[0] < '1' || code[0] > '5' {
		return netFTPResponseError(response, false)
	}
	if code[0] == '4' || code[0] == '5' || (!acceptReply && code[0] != '2') {
		return netFTPResponseError(response, !acceptReply)
	}
	return response
}

func netFTPRawCommand(receiver *object.EmeraldValue, command string) *object.EmeraldValue {
	socket := receiverInstanceVarMap(receiver)["@sock"]
	if socket == nil || socket.Type == object.ValueNil {
		return newRuntimeException(R.Classes["IOError"], "not connected")
	}
	if result := socketPuts(socket, rubyString(command)); result.Type == object.ValueException {
		return result
	}
	response := netFTPReadResponse(receiver, true)
	if response == nil || response.Type == object.ValueException {
		return response
	}
	return response
}

func netFTPResponseError(response *object.EmeraldValue, reply bool) *object.EmeraldValue {
	code := netFTPResponseCode(response)
	class := R.Classes["Net::FTPProtoError"]
	if len(code) == 3 {
		switch code[0] {
		case '4':
			class = R.Classes["Net::FTPTempError"]
		case '5':
			class = R.Classes["Net::FTPPermError"]
		case '1', '3':
			if reply {
				class = R.Classes["Net::FTPReplyError"]
			}
		}
	}
	return newRuntimeException(class, stringRawValue(response))
}

func netFTPReplyException(response *object.EmeraldValue) *object.EmeraldValue {
	return newRuntimeException(R.Classes["Net::FTPReplyError"], stringRawValue(response))
}

func netFTPSendCommand(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	command, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	return netFTPCommand(receiver, command, true)
}

func netFTPVoidCommand(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	command, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	result := netFTPCommand(receiver, command, false)
	if result.Type == object.ValueException {
		return result
	}
	return R.NilVal
}

func netFTPAbort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	previous := receiverInstanceVarMap(receiver)["@last_response"]
	response := netFTPCommand(receiver, "ABOR", true)
	if response.Type == object.ValueException {
		return newRuntimeException(R.Classes["Net::FTPProtoError"], stringRawValue(receiverInstanceVarMap(receiver)["@last_response"]))
	}
	code := netFTPResponseCode(response)
	if code != "225" && code != "226" {
		return newRuntimeException(R.Classes["Net::FTPProtoError"], stringRawValue(response))
	}
	receiverInstanceVarMap(receiver)["@last_response"] = previous
	return response
}

func netFTPArgumentVoidCommand(command string) func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue {
	return func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		value, errVal := httpString(args[0])
		if errVal != nil {
			return errVal
		}
		result := netFTPCommand(receiver, command+" "+value, false)
		if result.Type == object.ValueException {
			return result
		}
		return R.NilVal
	}
}

func netFTPChdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	command := "CWD " + path
	if path == ".." {
		response := netFTPRawCommand(receiver, "CDUP")
		if response == nil || response.Type == object.ValueException {
			return response
		}
		if netFTPResponseCode(response) == "500" {
			command = "CWD .."
		} else {
			if netFTPResponseCode(response) != "200" {
				return netFTPResponseError(response, false)
			}
			return R.NilVal
		}
	}
	response := netFTPCommand(receiver, command, false)
	if response.Type == object.ValueException {
		return response
	}
	return R.NilVal
}

func netFTPNoArgumentVoidCommand(command string) func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue {
	return func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		result := netFTPCommand(receiver, command, false)
		if result.Type == object.ValueException {
			return result
		}
		return R.NilVal
	}
}

func netFTPOptionalResponseCommand(command string) func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue {
	return func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
		if len(args) > 1 {
			return NewArgumentError("wrong number of arguments")
		}
		fullCommand := command
		if len(args) == 1 {
			value, errVal := httpString(args[0])
			if errVal != nil {
				return errVal
			}
			fullCommand += " " + value
		}
		return netFTPCommand(receiver, fullCommand, false)
	}
}

func netFTPRename(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	from, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	to, errVal := httpString(args[1])
	if errVal != nil {
		return errVal
	}
	first := netFTPCommand(receiver, "RNFR "+from, true)
	if first.Type == object.ValueException {
		return first
	}
	if netFTPResponseCode(first) != "350" {
		return netFTPResponseError(first, true)
	}
	second := netFTPCommand(receiver, "RNTO "+to, false)
	if second.Type == object.ValueException {
		return second
	}
	return R.NilVal
}

func netFTPPwd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	response := netFTPCommand(receiver, "PWD", false)
	if response.Type == object.ValueException {
		return response
	}
	raw := stringRawValue(response)
	start := strings.Index(raw, "\"")
	if start < 0 {
		return rubyString("")
	}
	rest := raw[start+1:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return rubyString("")
	}
	return rubyString(strings.ReplaceAll(rest[:end], "\"\"", "\""))
}

func netFTPMkdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	response := netFTPCommand(receiver, "MKD "+path, false)
	if response.Type == object.ValueException {
		return response
	}
	raw := stringRawValue(response)
	start := strings.Index(raw, "\"")
	if start < 0 {
		return rubyString(path)
	}
	quoted := raw[start+1:]
	var result strings.Builder
	for i := 0; i < len(quoted); i++ {
		if quoted[i] == '"' {
			if i+1 < len(quoted) && quoted[i+1] == '"' {
				result.WriteByte('"')
				i++
				continue
			}
			break
		}
		result.WriteByte(quoted[i])
	}
	return rubyString(result.String())
}

func netFTPSize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	response := netFTPCommand(receiver, "SIZE "+path, false)
	if response.Type == object.ValueException {
		return response
	}
	fields := strings.Fields(stringRawValue(response))
	if len(fields) < 2 {
		return R.NilVal
	}
	size, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return R.NilVal
	}
	return newInt(size)
}

func netFTPMdtm(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	response := netFTPCommand(receiver, "MDTM "+path, false)
	if response.Type == object.ValueException {
		return response
	}
	fields := strings.Fields(stringRawValue(response))
	if len(fields) < 2 {
		return rubyString("")
	}
	return rubyString(fields[1])
}

func netFTPMtime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	timestamp := netFTPMdtm(receiver, args[0])
	if timestamp.Type == object.ValueException {
		return timestamp
	}
	raw := stringRawValue(timestamp)
	if len(raw) < 14 || CallMethod == nil {
		return timestamp
	}
	method := "gm"
	if len(args) == 2 && isTruthy(args[1]) {
		method = "local"
	}
	timeClass := R.Classes["Time"]
	if timeClass == nil {
		return timestamp
	}
	parts := []*object.EmeraldValue{
		rubyString(raw[0:4]), rubyString(raw[4:6]), rubyString(raw[6:8]),
		rubyString(raw[8:10]), rubyString(raw[10:12]), rubyString(raw[12:14]),
	}
	return CallMethod(classEmeraldValue(timeClass), method, parts...)
}

func netFTPSystem(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	response := netFTPCommand(receiver, "SYST", false)
	if response.Type == object.ValueException {
		return response
	}
	raw := strings.TrimSuffix(stringRawValue(response), "\n")
	receiverInstanceVarMap(receiver)["@last_response"] = rubyString(raw)
	if len(raw) > 4 {
		return rubyString(raw[4:])
	}
	return rubyString("")
}

func netFTPActiveDataSocket(receiver *object.EmeraldValue, command string) (*object.EmeraldValue, *object.EmeraldValue) {
	serverClass := R.Classes["TCPServer"]
	if serverClass == nil {
		return nil, newRuntimeException(R.Classes["LoadError"], "socket is unavailable")
	}
	server := tcpServerNew(classEmeraldValue(serverClass), rubyString("127.0.0.1"), newInt(0))
	if server == nil || server.Type == object.ValueException {
		return nil, server
	}
	port := socketDataOf(server).localPort
	portCommand := "PORT 127,0,0,1," + strconv.FormatInt(port/256, 10) + "," + strconv.FormatInt(port%256, 10)
	if response := netFTPCommand(receiver, portCommand, false); response == nil || response.Type == object.ValueException {
		_ = socketClose(server)
		return nil, response
	}
	preliminary := netFTPCommand(receiver, command, true)
	if preliminary == nil || preliminary.Type == object.ValueException {
		_ = socketClose(server)
		return nil, preliminary
	}
	code := netFTPResponseCode(preliminary)
	if len(code) != 3 || code[0] != '1' {
		_ = socketClose(server)
		return nil, netFTPResponseError(preliminary, true)
	}
	dataSocket := tcpServerAccept(server)
	_ = socketClose(server)
	if dataSocket == nil || dataSocket.Type == object.ValueException {
		return nil, dataSocket
	}
	return dataSocket, nil
}

func netFTPFinishDataTransfer(receiver *object.EmeraldValue) *object.EmeraldValue {
	response := netFTPReadResponse(receiver, true)
	if response == nil || response.Type == object.ValueException {
		return response
	}
	code := netFTPResponseCode(response)
	if len(code) != 3 || code[0] != '2' {
		return netFTPResponseError(response, false)
	}
	return R.NilVal
}

func netFTPRetrieveBinary(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 3 {
		return NewArgumentError("wrong number of arguments")
	}
	command, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	blockSize, ok := valueToInteger(args[1])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, command)
	if transferErr != nil {
		return transferErr
	}
	block := (*object.EmeraldValue)(nil)
	if CurrentBlockValue != nil {
		block = CurrentBlockValue()
	}
	for {
		chunk := socketRecv(dataSocket, newInt(blockSize))
		if chunk == nil || chunk.Type == object.ValueNil {
			break
		}
		if chunk.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return chunk
		}
		if block != nil && CallBlockWithArgs != nil {
			if result := CallBlockWithArgs(block, chunk); result != nil && result.Type == object.ValueException {
				_ = socketClose(dataSocket)
				return result
			}
		}
	}
	_ = socketClose(dataSocket)
	return netFTPFinishDataTransfer(receiver)
}

func netFTPRetrieveLines(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	command, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, command)
	if transferErr != nil {
		return transferErr
	}
	block := (*object.EmeraldValue)(nil)
	if CurrentBlockValue != nil {
		block = CurrentBlockValue()
	}
	for {
		line := socketGets(dataSocket, rubyString("\n"))
		if line == nil || line.Type == object.ValueNil {
			break
		}
		if line.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return line
		}
		raw := strings.TrimSuffix(stringRawValue(line), "\n")
		raw = strings.TrimSuffix(raw, "\r")
		if block != nil && CallBlockWithArgs != nil {
			if result := CallBlockWithArgs(block, rubyString(raw)); result != nil && result.Type == object.ValueException {
				_ = socketClose(dataSocket)
				return result
			}
		}
	}
	_ = socketClose(dataSocket)
	return netFTPFinishDataTransfer(receiver)
}

func netFTPList(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return netFTPCollectListing(receiver, "LIST", args...)
}

func netFTPNlst(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return netFTPCollectListing(receiver, "NLST", args...)
}

func netFTPCollectListing(receiver *object.EmeraldValue, baseCommand string, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError("wrong number of arguments")
	}
	command := baseCommand
	if len(args) == 1 {
		path, errVal := httpString(args[0])
		if errVal != nil {
			return errVal
		}
		command += " " + path
	}
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, command)
	if transferErr != nil {
		return transferErr
	}
	block := (*object.EmeraldValue)(nil)
	if CurrentBlockValue != nil {
		block = CurrentBlockValue()
	}
	lines := make([]*object.EmeraldValue, 0)
	for {
		line := socketGets(dataSocket, rubyString("\n"))
		if line == nil || line.Type == object.ValueNil {
			break
		}
		if line.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return line
		}
		raw := strings.TrimSuffix(stringRawValue(line), "\n")
		raw = strings.TrimSuffix(raw, "\r")
		value := rubyString(raw)
		lines = append(lines, value)
		if block != nil && CallBlockWithArgs != nil {
			if result := CallBlockWithArgs(block, value); result != nil && result.Type == object.ValueException {
				_ = socketClose(dataSocket)
				return result
			}
		}
	}
	_ = socketClose(dataSocket)
	if result := netFTPFinishDataTransfer(receiver); result.Type == object.ValueException {
		return result
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: lines, Class: R.Classes["Array"]}
}

func netFTPGet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if isTruthy(receiverInstanceVarMap(receiver)["@binary"]) {
		return netFTPGetBinaryFile(receiver, args...)
	}
	return netFTPGetTextFile(receiver, args...)
}

func netFTPFileArguments(args []*object.EmeraldValue, defaultBlockSize int64) (remote, local string, blockSize int64, errVal *object.EmeraldValue) {
	if len(args) < 1 || len(args) > 3 {
		return "", "", 0, NewArgumentError("wrong number of arguments")
	}
	remote, errVal = httpString(args[0])
	if errVal != nil {
		return
	}
	local = filepath.Base(remote)
	if len(args) > 1 && args[1].Type != object.ValueNil {
		local, errVal = httpString(args[1])
		if errVal != nil {
			return
		}
	}
	blockSize = defaultBlockSize
	if len(args) > 2 {
		var ok bool
		blockSize, ok = valueToInteger(args[2])
		if !ok {
			errVal = typeError("no implicit conversion into Integer")
		}
	}
	return
}

func netFTPPrepareResume(receiver *object.EmeraldValue, remote, local string) (int64, *object.EmeraldValue) {
	if !isTruthy(receiverInstanceVarMap(receiver)["@resume"]) {
		return 0, nil
	}
	info, err := os.Stat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, newRuntimeException(R.Classes["IOError"], err.Error())
	}
	offset := info.Size()
	response := netFTPCommand(receiver, "REST "+strconv.FormatInt(offset, 10), true)
	if response == nil || response.Type == object.ValueException {
		return 0, response
	}
	if netFTPResponseCode(response) != "350" {
		return 0, netFTPResponseError(response, true)
	}
	return offset, nil
}

func netFTPGetBinaryFile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	remote, local, blockSize, errVal := netFTPFileArguments(args, 4096)
	if errVal != nil {
		return errVal
	}
	offset, resumeErr := netFTPPrepareResume(receiver, remote, local)
	if resumeErr != nil {
		return resumeErr
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if offset > 0 {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(local, flags, 0o666)
	if err != nil {
		return newRuntimeException(R.Classes["IOError"], err.Error())
	}
	defer file.Close()
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, "RETR "+remote)
	if transferErr != nil {
		return transferErr
	}
	block := currentNetFTPBlock()
	for {
		chunk := socketRecv(dataSocket, newInt(blockSize))
		if chunk == nil || chunk.Type == object.ValueNil {
			break
		}
		if chunk.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return chunk
		}
		if _, err := file.WriteString(stringRawValue(chunk)); err != nil {
			_ = socketClose(dataSocket)
			return newRuntimeException(R.Classes["IOError"], err.Error())
		}
		if result := netFTPYieldBlock(block, chunk); result != nil {
			_ = socketClose(dataSocket)
			return result
		}
	}
	_ = socketClose(dataSocket)
	return netFTPFinishDataTransfer(receiver)
}

func netFTPGetTextFile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	remote, local, _, errVal := netFTPFileArguments(args, 4096)
	if errVal != nil {
		return errVal
	}
	file, err := os.OpenFile(local, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		return newRuntimeException(R.Classes["IOError"], err.Error())
	}
	defer file.Close()
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, "RETR "+remote)
	if transferErr != nil {
		return transferErr
	}
	block := currentNetFTPBlock()
	for {
		line := socketGets(dataSocket, rubyString("\n"))
		if line == nil || line.Type == object.ValueNil {
			break
		}
		if line.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return line
		}
		raw := strings.TrimSuffix(stringRawValue(line), "\n")
		raw = strings.TrimSuffix(raw, "\r")
		if _, err := file.WriteString(raw + "\n"); err != nil {
			_ = socketClose(dataSocket)
			return newRuntimeException(R.Classes["IOError"], err.Error())
		}
		if result := netFTPYieldBlock(block, rubyString(raw)); result != nil {
			_ = socketClose(dataSocket)
			return result
		}
	}
	_ = socketClose(dataSocket)
	return netFTPFinishDataTransfer(receiver)
}

func currentNetFTPBlock() *object.EmeraldValue {
	if CurrentBlockValue != nil {
		return CurrentBlockValue()
	}
	return nil
}

func netFTPYieldBlock(block, value *object.EmeraldValue) *object.EmeraldValue {
	if block == nil || CallBlockWithArgs == nil {
		return nil
	}
	result := CallBlockWithArgs(block, value)
	if result != nil && result.Type == object.ValueException {
		return result
	}
	return nil
}

func netFTPPut(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if isTruthy(receiverInstanceVarMap(receiver)["@binary"]) {
		return netFTPPutBinaryFile(receiver, args...)
	}
	return netFTPPutTextFile(receiver, args...)
}

func netFTPPutArguments(args []*object.EmeraldValue, defaultBlockSize int64) (local, remote string, blockSize int64, errVal *object.EmeraldValue) {
	if len(args) < 1 || len(args) > 3 {
		return "", "", 0, NewArgumentError("wrong number of arguments")
	}
	local, errVal = httpString(args[0])
	if errVal != nil {
		return
	}
	remote = filepath.Base(local)
	if len(args) > 1 && args[1].Type != object.ValueNil {
		remote, errVal = httpString(args[1])
		if errVal != nil {
			return
		}
	}
	blockSize = defaultBlockSize
	if len(args) > 2 {
		var ok bool
		blockSize, ok = valueToInteger(args[2])
		if !ok {
			errVal = typeError("no implicit conversion into Integer")
		}
	}
	return
}

func netFTPPutBinaryFile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	local, remote, blockSize, errVal := netFTPPutArguments(args, 4096)
	if errVal != nil {
		return errVal
	}
	file, err := os.Open(local)
	if err != nil {
		return newRuntimeException(R.Classes["IOError"], err.Error())
	}
	defer file.Close()
	command := "STOR " + remote
	if isTruthy(receiverInstanceVarMap(receiver)["@resume"]) {
		size := netFTPSize(receiver, rubyString(remote))
		if size.Type == object.ValueException {
			return size
		}
		offset, ok := valueToInteger(size)
		if ok && offset > 0 {
			_, _ = file.Seek(offset, 0)
		}
		command = "APPE " + remote
	}
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, command)
	if transferErr != nil {
		return transferErr
	}
	block := currentNetFTPBlock()
	buffer := make([]byte, blockSize)
	for {
		count, readErr := file.Read(buffer)
		if count > 0 {
			chunk := rubyString(string(buffer[:count]))
			if result := socketWrite(dataSocket, chunk); result.Type == object.ValueException {
				_ = socketClose(dataSocket)
				return result
			}
			if result := netFTPYieldBlock(block, chunk); result != nil {
				_ = socketClose(dataSocket)
				return result
			}
		}
		if readErr != nil || count == 0 {
			break
		}
	}
	_ = socketClose(dataSocket)
	return netFTPFinishDataTransfer(receiver)
}

func netFTPPutTextFile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	local, remote, _, errVal := netFTPPutArguments(args, 4096)
	if errVal != nil {
		return errVal
	}
	content, err := os.ReadFile(local)
	if err != nil {
		return newRuntimeException(R.Classes["IOError"], err.Error())
	}
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, "STOR "+remote)
	if transferErr != nil {
		return transferErr
	}
	block := currentNetFTPBlock()
	for _, line := range strings.SplitAfter(string(content), "\n") {
		if line == "" {
			continue
		}
		raw := strings.TrimSuffix(line, "\n")
		raw = strings.TrimSuffix(raw, "\r") + "\r\n"
		value := rubyString(raw)
		if result := socketWrite(dataSocket, value); result.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return result
		}
		if result := netFTPYieldBlock(block, value); result != nil {
			_ = socketClose(dataSocket)
			return result
		}
	}
	_ = socketClose(dataSocket)
	return netFTPFinishDataTransfer(receiver)
}

func netFTPStoreBinary(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 3 || len(args) > 4 {
		return NewArgumentError("wrong number of arguments")
	}
	command, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	blockSize, ok := valueToInteger(args[2])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, command)
	if transferErr != nil {
		return transferErr
	}
	block := (*object.EmeraldValue)(nil)
	if CurrentBlockValue != nil {
		block = CurrentBlockValue()
	}
	for {
		chunk := CallMethod(args[1], "read", newInt(blockSize))
		if chunk == nil || chunk.Type == object.ValueNil || (chunk.Type == object.ValueString && stringRawValue(chunk) == "") {
			break
		}
		if chunk.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return chunk
		}
		if result := socketWrite(dataSocket, chunk); result.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return result
		}
		if block != nil && CallBlockWithArgs != nil {
			if result := CallBlockWithArgs(block, chunk); result != nil && result.Type == object.ValueException {
				_ = socketClose(dataSocket)
				return result
			}
		}
	}
	_ = socketClose(dataSocket)
	return netFTPFinishDataTransfer(receiver)
}

func netFTPStoreLines(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	command, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	dataSocket, transferErr := netFTPActiveDataSocket(receiver, command)
	if transferErr != nil {
		return transferErr
	}
	block := (*object.EmeraldValue)(nil)
	if CurrentBlockValue != nil {
		block = CurrentBlockValue()
	}
	for {
		line := CallMethod(args[1], "gets")
		if line == nil || line.Type == object.ValueNil {
			break
		}
		if line.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return line
		}
		raw := strings.TrimSuffix(stringRawValue(line), "\n")
		raw = strings.TrimSuffix(raw, "\r") + "\r\n"
		converted := rubyString(raw)
		if result := socketWrite(dataSocket, converted); result.Type == object.ValueException {
			_ = socketClose(dataSocket)
			return result
		}
		if block != nil && CallBlockWithArgs != nil {
			if result := CallBlockWithArgs(block, converted); result != nil && result.Type == object.ValueException {
				_ = socketClose(dataSocket)
				return result
			}
		}
	}
	_ = socketClose(dataSocket)
	return netFTPFinishDataTransfer(receiver)
}

func netFTPClose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	socket := receiverInstanceVarMap(receiver)["@sock"]
	if socket == nil || socket.Type == object.ValueNil || CallMethod == nil {
		return R.NilVal
	}
	closed := CallMethod(socket, "closed?")
	if closed != nil && closed.Type == object.ValueException {
		return closed
	}
	if !isTruthy(closed) {
		if result := CallMethod(socket, "close"); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return R.NilVal
}

func netFTPClosed(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	socket := receiverInstanceVarMap(receiver)["@sock"]
	if socket == nil || socket.Type == object.ValueNil || CallMethod == nil {
		return R.TrueVal
	}
	result := CallMethod(socket, "closed?")
	if result == nil {
		return R.FalseVal
	}
	return result
}

func netFTPReturnCode(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	_ = builtinWarn(receiver, rubyString("warning: Net::FTP#return_code is obsolete and do nothing"))
	return rubyString("\n")
}

func netFTPSetReturnCode(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	_ = builtinWarn(receiver, rubyString("warning: Net::FTP#return_code= is obsolete and do nothing"))
	if len(args) == 1 {
		return args[0]
	}
	return R.NilVal
}
