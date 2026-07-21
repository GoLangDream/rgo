package core

import (
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

type uriData struct {
	scheme       *string
	userinfo     *string
	host         *string
	port         *int64
	path         *string
	query        *string
	fragment     *string
	registry     *string
	opaque       *string
	typecode     *string
	dn           *string
	attributes   *string
	scope        *string
	filter       *string
	extensions   *string
	to           *string
	headers      [][2]string
	explicitPort bool
}

var uriSchemePattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*):(.*)$`)
var uriExtractPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*:[^\s<>"]*`)

func uriStringPointer(value string) *string { return &value }

func installURIModule(objectClass *object.Class) {
	if objectClass == nil {
		return
	}
	if existing, ok := objectClass.Constants["URI"]; ok && existing != nil && existing.Type == object.ValueModule {
		return
	}
	module := object.NewModule("URI")
	module.DefineMethod("parse", &object.Method{Name: "parse", Fn: uriParse, Arity: 1})
	module.DefineMethod("split", &object.Method{Name: "split", Fn: uriSplit, Arity: 1})
	module.DefineMethod("join", &object.Method{Name: "join", Fn: uriJoin, Arity: -1})
	module.DefineMethod("extract", &object.Method{Name: "extract", Fn: uriExtract, Arity: -1})
	module.DefineMethod("regexp", &object.Method{Name: "regexp", Fn: uriRegexp, Arity: -1})

	for _, name := range []string{"Error", "InvalidURIError", "InvalidComponentError", "BadURIError"} {
		klass := object.NewClass("URI::" + name)
		if name == "Error" {
			klass.SuperClass = R.Classes["StandardError"]
		} else {
			klass.SuperClass = R.Classes["URI::Error"]
		}
		R.Classes["URI::"+name] = klass
		module.Constants[name] = &object.EmeraldValue{Type: object.ValueClass, Data: klass, Class: R.Classes["Class"]}
	}

	generic := object.NewClass("URI::Generic")
	generic.SuperClass = objectClass
	installURIGenericMethods(generic)
	http := uriSubclass("URI::HTTP", generic)
	https := uriSubclass("URI::HTTPS", http)
	ftp := uriSubclass("URI::FTP", generic)
	ldap := uriSubclass("URI::LDAP", generic)
	mailto := uriSubclass("URI::MailTo", generic)
	for name, klass := range map[string]*object.Class{"Generic": generic, "HTTP": http, "HTTPS": https, "FTP": ftp, "LDAP": ldap, "MailTo": mailto} {
		R.Classes["URI::"+name] = klass
		module.Constants[name] = &object.EmeraldValue{Type: object.ValueClass, Data: klass, Class: R.Classes["Class"]}
	}
	http.DefineMethod("request_uri", &object.Method{Name: "request_uri", Fn: uriRequestURI, Arity: 0})
	ftp.DefineMethod("typecode", &object.Method{Name: "typecode", Fn: uriTypecode, Arity: 0})
	ftp.DefineMethod("typecode=", &object.Method{Name: "typecode=", Fn: uriSetTypecode, Arity: 1})
	ldap.DefineMethod("dn", &object.Method{Name: "dn", Fn: uriDN, Arity: 0})
	ldap.DefineMethod("attributes", &object.Method{Name: "attributes", Fn: uriAttributes, Arity: 0})
	ldap.DefineMethod("scope", &object.Method{Name: "scope", Fn: uriScope, Arity: 0})
	ldap.DefineMethod("filter", &object.Method{Name: "filter", Fn: uriFilter, Arity: 0})
	ldap.DefineMethod("extensions", &object.Method{Name: "extensions", Fn: uriExtensions, Arity: 0})
	mailto.DefineMethod("to", &object.Method{Name: "to", Fn: uriTo, Arity: 0})
	mailto.DefineMethod("headers", &object.Method{Name: "headers", Fn: uriHeaders, Arity: 0})
	mailto.DefineMethod("to=", &object.Method{Name: "to=", Fn: uriSetTo, Arity: 1})
	mailto.DefineMethod("headers=", &object.Method{Name: "headers=", Fn: uriSetHeaders, Arity: 1})

	parser := object.NewClass("URI::RFC2396_Parser")
	parser.SuperClass = objectClass
	parser.DefineClassMethod("new", &object.Method{Name: "new", Fn: uriParserNew, Arity: -1})
	parser.DefineMethod("parse", &object.Method{Name: "parse", Fn: uriParse, Arity: 1})
	parser.DefineMethod("split", &object.Method{Name: "split", Fn: uriSplit, Arity: 1})
	parser.DefineMethod("join", &object.Method{Name: "join", Fn: uriJoin, Arity: -1})
	parser.DefineMethod("extract", &object.Method{Name: "extract", Fn: uriExtract, Arity: -1})
	parser.DefineMethod("make_regexp", &object.Method{Name: "make_regexp", Fn: uriRegexp, Arity: -1})
	parser.DefineMethod("escape", &object.Method{Name: "escape", Fn: uriEscape, Arity: -1})
	parser.DefineMethod("unescape", &object.Method{Name: "unescape", Fn: uriUnescape, Arity: 1})
	parser.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: uriParserInspect, Arity: 0})
	R.Classes["URI::Parser"] = parser
	R.Classes["URI::RFC2396_Parser"] = parser
	module.Constants["Parser"] = &object.EmeraldValue{Type: object.ValueClass, Data: parser, Class: R.Classes["Class"]}
	module.Constants["RFC2396_Parser"] = module.Constants["Parser"]
	module.Constants["RFC3986_Parser"] = module.Constants["Parser"]
	module.Constants["DEFAULT_PARSER"] = uriParserNew(module.Constants["Parser"])
	module.Constants["VERSION"] = rubyString("1.0.4")

	moduleValue := &object.EmeraldValue{Type: object.ValueModule, Data: module, Class: R.Classes["Module"]}
	objectClass.DefineConstant("URI", moduleValue)
	AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "URI", moduleValue)
	method := &object.Method{Name: "URI", Fn: uriKernel, Arity: 1, Visibility: "private"}
	objectClass.DefineMethod("URI", method)
	if kernel := R.Classes["Kernel"]; kernel != nil {
		kernel.DefineMethod("URI", method)
		kernel.DefineClassMethod("URI", &object.Method{Name: "URI", Fn: uriKernel, Arity: 1})
	}
}

func uriSubclass(name string, superclass *object.Class) *object.Class {
	klass := object.NewClass(name)
	klass.SuperClass = superclass
	klass.DefineClassMethod("build", &object.Method{Name: "build", Fn: uriClassBuild, Arity: 1})
	klass.DefineClassMethod("build2", &object.Method{Name: "build2", Fn: uriClassBuild, Arity: 1})
	klass.DefineClassMethod("component", &object.Method{Name: "component", Fn: uriClassComponent, Arity: 0})
	return klass
}

func installURIGenericMethods(klass *object.Class) {
	klass.DefineClassMethod("build", &object.Method{Name: "build", Fn: uriClassBuild, Arity: 1})
	klass.DefineClassMethod("build2", &object.Method{Name: "build2", Fn: uriClassBuild, Arity: 1})
	klass.DefineClassMethod("component", &object.Method{Name: "component", Fn: uriClassComponent, Arity: 0})
	klass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: uriToS, Arity: 0})
	klass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: uriInspect, Arity: 0})
	klass.DefineMethod("==", &object.Method{Name: "==", Fn: uriEqual, Arity: 1})
	klass.DefineMethod("eql?", &object.Method{Name: "eql?", Fn: uriEqual, Arity: 1})
	klass.DefineMethod("hash", &object.Method{Name: "hash", Fn: uriHash, Arity: 0})
	klass.DefineMethod("normalize", &object.Method{Name: "normalize", Fn: uriNormalize, Arity: 0})
	klass.DefineMethod("normalize!", &object.Method{Name: "normalize!", Fn: uriNormalizeBang, Arity: 0})
	klass.DefineMethod("merge", &object.Method{Name: "merge", Fn: uriMerge, Arity: 1})
	klass.DefineMethod("+", &object.Method{Name: "+", Fn: uriMerge, Arity: 1})
	klass.DefineMethod("merge!", &object.Method{Name: "merge!", Fn: uriMergeBang, Arity: 1})
	klass.DefineMethod("route_to", &object.Method{Name: "route_to", Fn: uriRouteTo, Arity: 1})
	klass.DefineMethod("route_from", &object.Method{Name: "route_from", Fn: uriRouteFrom, Arity: 1})
	klass.DefineMethod("-", &object.Method{Name: "-", Fn: uriRouteFrom, Arity: 1})
	klass.DefineMethod("select", &object.Method{Name: "select", Fn: uriSelect, Arity: -1})
	klass.DefineMethod("component", &object.Method{Name: "component", Fn: uriComponent, Arity: 0})
	klass.DefineMethod("component_ary", &object.Method{Name: "component_ary", Fn: uriComponentAry, Arity: 0})
	klass.DefineMethod("absolute?", &object.Method{Name: "absolute?", Fn: uriAbsolute, Arity: 0})
	klass.DefineMethod("absolute", &object.Method{Name: "absolute", Fn: uriAbsolute, Arity: 0})
	klass.DefineMethod("relative?", &object.Method{Name: "relative?", Fn: uriRelative, Arity: 0})
	klass.DefineMethod("hierarchical?", &object.Method{Name: "hierarchical?", Fn: uriHierarchical, Arity: 0})
	klass.DefineMethod("request_uri", &object.Method{Name: "request_uri", Fn: uriRequestURI, Arity: 0})
	klass.DefineMethod("scheme", &object.Method{Name: "scheme", Fn: uriScheme, Arity: 0})
	klass.DefineMethod("userinfo", &object.Method{Name: "userinfo", Fn: uriUserinfo, Arity: 0})
	klass.DefineMethod("user", &object.Method{Name: "user", Fn: uriUser, Arity: 0})
	klass.DefineMethod("password", &object.Method{Name: "password", Fn: uriPassword, Arity: 0})
	klass.DefineMethod("host", &object.Method{Name: "host", Fn: uriHost, Arity: 0})
	klass.DefineMethod("port", &object.Method{Name: "port", Fn: uriPort, Arity: 0})
	klass.DefineMethod("default_port", &object.Method{Name: "default_port", Fn: uriDefaultPort, Arity: 0})
	klass.DefineMethod("path", &object.Method{Name: "path", Fn: uriPath, Arity: 0})
	klass.DefineMethod("query", &object.Method{Name: "query", Fn: uriQuery, Arity: 0})
	klass.DefineMethod("fragment", &object.Method{Name: "fragment", Fn: uriFragment, Arity: 0})
	klass.DefineMethod("registry", &object.Method{Name: "registry", Fn: uriRegistry, Arity: 0})
	klass.DefineMethod("opaque", &object.Method{Name: "opaque", Fn: uriOpaque, Arity: 0})
	klass.DefineMethod("scheme=", &object.Method{Name: "scheme=", Fn: uriSetScheme, Arity: 1})
	klass.DefineMethod("userinfo=", &object.Method{Name: "userinfo=", Fn: uriSetUserinfo, Arity: 1})
	klass.DefineMethod("user=", &object.Method{Name: "user=", Fn: uriSetUser, Arity: 1})
	klass.DefineMethod("password=", &object.Method{Name: "password=", Fn: uriSetPassword, Arity: 1})
	klass.DefineMethod("host=", &object.Method{Name: "host=", Fn: uriSetHost, Arity: 1})
	klass.DefineMethod("port=", &object.Method{Name: "port=", Fn: uriSetPort, Arity: 1})
	klass.DefineMethod("path=", &object.Method{Name: "path=", Fn: uriSetPath, Arity: 1})
	klass.DefineMethod("query=", &object.Method{Name: "query=", Fn: uriSetQuery, Arity: 1})
	klass.DefineMethod("fragment=", &object.Method{Name: "fragment=", Fn: uriSetFragment, Arity: 1})
	klass.DefineMethod("registry=", &object.Method{Name: "registry=", Fn: uriSetRegistry, Arity: 1})
	klass.DefineMethod("opaque=", &object.Method{Name: "opaque=", Fn: uriSetOpaque, Arity: 1})
}

func uriKernel(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 1 {
		if _, ok := args[0].Data.(*uriData); ok {
			return args[0]
		}
	}
	return uriParse(receiver, args...)
}

func uriParse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	if _, ok := args[0].Data.(*uriData); ok {
		return args[0]
	}
	_, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	return parseURIString(raw)
}

func parseURIString(raw string) *object.EmeraldValue {
	data := &uriData{}
	rest := raw
	if match := uriSchemePattern.FindStringSubmatch(raw); match != nil {
		scheme := match[1]
		data.scheme = &scheme
		rest = match[2]
	}
	if index := strings.Index(rest, "#"); index >= 0 {
		fragment := rest[index+1:]
		data.fragment = &fragment
		rest = rest[:index]
	}
	scheme := strings.ToLower(uriPointerString(data.scheme))
	if scheme == "mailto" {
		data.to = uriStringPointer(rest)
		if index := strings.Index(rest, "?"); index >= 0 {
			data.to = uriStringPointer(rest[:index])
			for _, field := range strings.Split(rest[index+1:], "&") {
				parts := strings.SplitN(field, "=", 2)
				if len(parts) == 2 {
					data.headers = append(data.headers, [2]string{parts[0], parts[1]})
				}
			}
		}
		return uriValue(data, "MailTo")
	}
	if data.scheme != nil && !strings.HasPrefix(rest, "//") && !strings.HasPrefix(rest, "/") {
		data.opaque = uriStringPointer(rest)
		return uriValue(data, "Generic")
	}
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		end := strings.IndexByte(rest, '/')
		authority := rest
		if end >= 0 {
			authority, rest = rest[:end], rest[end:]
		} else {
			rest = ""
		}
		parseURIAuthority(data, authority)
	}
	if index := strings.Index(rest, "?"); index >= 0 {
		data.query = uriStringPointer(rest[index+1:])
		rest = rest[:index]
	}
	data.path = uriStringPointer(rest)
	className := "Generic"
	switch scheme {
	case "http":
		className = "HTTP"
	case "https":
		className = "HTTPS"
	case "ftp":
		className = "FTP"
		ftpPath := strings.TrimPrefix(rest, "/")
		if strings.HasPrefix(strings.ToUpper(ftpPath), "%2F") {
			ftpPath = "/" + ftpPath[3:]
		}
		if index := strings.LastIndex(strings.ToLower(ftpPath), ";type="); index >= 0 {
			data.typecode = uriStringPointer(ftpPath[index+6:])
			ftpPath = ftpPath[:index]
		}
		data.path = uriStringPointer(ftpPath)
	case "ldap", "ldaps":
		className = "LDAP"
		dn := strings.TrimPrefix(rest, "/")
		parts := strings.Split(dn, "?")
		data.dn = uriStringPointer(parts[0])
		if len(parts) > 1 && parts[1] != "" {
			data.attributes = uriStringPointer(parts[1])
		}
		if len(parts) > 2 && parts[2] != "" {
			data.scope = uriStringPointer(parts[2])
		}
		if len(parts) > 3 && parts[3] != "" {
			data.filter = uriStringPointer(parts[3])
		}
		if len(parts) > 4 && parts[4] != "" {
			data.extensions = uriStringPointer(parts[4])
		}
		data.path, data.query = nil, nil
	}
	return uriValue(data, className)
}

func parseURIAuthority(data *uriData, authority string) {
	if index := strings.LastIndex(authority, "@"); index >= 0 {
		data.userinfo = uriStringPointer(authority[:index])
		authority = authority[index+1:]
	}
	host := authority
	if strings.HasPrefix(authority, "[") {
		if end := strings.Index(authority, "]"); end >= 0 {
			host = authority[:end+1]
			if end+1 < len(authority) && authority[end+1] == ':' {
				if port, err := strconv.ParseInt(authority[end+2:], 10, 64); err == nil {
					data.port, data.explicitPort = &port, true
				}
			}
		}
	} else if index := strings.LastIndex(authority, ":"); index >= 0 {
		if authority[index+1:] == "" {
			host = authority[:index]
		} else if port, err := strconv.ParseInt(authority[index+1:], 10, 64); err == nil {
			host, data.port, data.explicitPort = authority[:index], &port, true
		}
	}
	data.host = uriStringPointer(host)
}

func uriValue(data *uriData, className string) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueObject, Data: data, Class: R.Classes["URI::"+className]}
}

func uriValueData(receiver *object.EmeraldValue) (*uriData, *object.EmeraldValue) {
	data, ok := receiver.Data.(*uriData)
	if !ok || data == nil {
		return nil, newRuntimeException(R.Classes["URI::InvalidURIError"], "bad URI")
	}
	return data, nil
}

func uriPointerValue(value *string) *object.EmeraldValue {
	if value == nil {
		return R.NilVal
	}
	return rubyString(*value)
}
func uriPointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func uriDefaultPortNumber(data *uriData) (int64, bool) {
	switch strings.ToLower(uriPointerString(data.scheme)) {
	case "http":
		return 80, true
	case "https":
		return 443, true
	case "ftp":
		return 21, true
	case "ldap":
		return 389, true
	case "ldaps":
		return 636, true
	}
	return 0, false
}

func uriToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := uriValueData(receiver)
	if errVal != nil {
		return errVal
	}
	return rubyString(uriDataString(data))
}

func uriDataString(data *uriData) string {
	var out strings.Builder
	if data.scheme != nil {
		out.WriteString(*data.scheme)
		out.WriteByte(':')
	}
	if strings.EqualFold(uriPointerString(data.scheme), "mailto") {
		out.WriteString(uriPointerString(data.to))
		if len(data.headers) > 0 {
			out.WriteByte('?')
			for index, header := range data.headers {
				if index > 0 {
					out.WriteByte('&')
				}
				out.WriteString(header[0])
				out.WriteByte('=')
				out.WriteString(header[1])
			}
		}
	} else if data.opaque != nil {
		out.WriteString(*data.opaque)
	} else {
		if data.host != nil || data.userinfo != nil || data.registry != nil {
			out.WriteString("//")
			if data.userinfo != nil {
				out.WriteString(*data.userinfo)
				out.WriteByte('@')
			}
			if data.registry != nil {
				out.WriteString(*data.registry)
			} else if data.host != nil {
				out.WriteString(*data.host)
			}
			if data.explicitPort && data.port != nil {
				if defaultPort, ok := uriDefaultPortNumber(data); !ok || *data.port != defaultPort {
					out.WriteByte(':')
					out.WriteString(strconv.FormatInt(*data.port, 10))
				}
			}
		}
		if strings.EqualFold(uriPointerString(data.scheme), "ftp") {
			out.WriteByte('/')
			ftpPath := uriPointerString(data.path)
			if strings.HasPrefix(ftpPath, "/") {
				out.WriteString("%2F" + ftpPath[1:])
			} else {
				out.WriteString(ftpPath)
			}
			if data.typecode != nil {
				out.WriteString(";type=" + *data.typecode)
			}
		} else if strings.EqualFold(uriPointerString(data.scheme), "ldap") || strings.EqualFold(uriPointerString(data.scheme), "ldaps") {
			out.WriteByte('/')
			out.WriteString(uriPointerString(data.dn))
			values := []*string{data.attributes, data.scope, data.filter, data.extensions}
			last := -1
			for i, value := range values {
				if value != nil {
					last = i
				}
			}
			for i := 0; i <= last; i++ {
				out.WriteByte('?')
				out.WriteString(uriPointerString(values[i]))
			}
		} else if data.path != nil {
			out.WriteString(*data.path)
		}
		if data.query != nil {
			out.WriteByte('?')
			out.WriteString(*data.query)
		}
	}
	if data.fragment != nil {
		out.WriteByte('#')
		out.WriteString(*data.fragment)
	}
	return out.String()
}

func uriComponentNames(value *object.EmeraldValue) []string {
	if value != nil && value.Class != nil {
		switch value.Class.Name {
		case "URI::HTTP", "URI::HTTPS":
			return []string{"scheme", "userinfo", "host", "port", "path", "query", "fragment"}
		case "URI::FTP":
			return []string{"scheme", "userinfo", "host", "port", "path", "typecode"}
		case "URI::LDAP":
			return []string{"scheme", "host", "port", "dn", "attributes", "scope", "filter", "extensions"}
		case "URI::MailTo":
			return []string{"scheme", "to", "headers"}
		}
	}
	return []string{"scheme", "userinfo", "host", "port", "registry", "path", "opaque", "query", "fragment"}
}

func uriComponent(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return uriNamesArray(uriComponentNames(receiver))
}
func uriClassComponent(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	return uriNamesArray(uriComponentNames(&object.EmeraldValue{Class: klass}))
}
func uriNamesArray(names []string) *object.EmeraldValue {
	values := make([]*object.EmeraldValue, len(names))
	for i, n := range names {
		values[i] = rubySymbol(n)
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func uriComponentValue(receiver *object.EmeraldValue, name string) *object.EmeraldValue {
	switch name {
	case "scheme":
		return uriScheme(receiver)
	case "userinfo":
		return uriUserinfo(receiver)
	case "host":
		return uriHost(receiver)
	case "port":
		return uriPort(receiver)
	case "registry":
		return uriRegistry(receiver)
	case "path":
		return uriPath(receiver)
	case "opaque":
		return uriOpaque(receiver)
	case "query":
		return uriQuery(receiver)
	case "fragment":
		return uriFragment(receiver)
	case "typecode":
		return uriTypecode(receiver)
	case "dn":
		return uriDN(receiver)
	case "attributes":
		return uriAttributes(receiver)
	case "scope":
		return uriScope(receiver)
	case "filter":
		return uriFilter(receiver)
	case "extensions":
		return uriExtensions(receiver)
	case "to":
		return uriTo(receiver)
	case "headers":
		return uriHeaders(receiver)
	}
	return R.NilVal
}

func uriComponentAry(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	names := uriComponentNames(receiver)
	values := make([]*object.EmeraldValue, len(names))
	for i, n := range names {
		values[i] = uriComponentValue(receiver, n)
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}
func uriSelect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	valid := map[string]bool{}
	for _, n := range uriComponentNames(receiver) {
		valid[n] = true
	}
	values := make([]*object.EmeraldValue, 0, len(args))
	for _, arg := range args {
		if arg == nil || arg.Type != object.ValueSymbol || !valid[arg.Data.(string)] {
			return NewArgumentError("expected one of components")
		}
		values = append(values, uriComponentValue(receiver, arg.Data.(string)))
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func uriScheme(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.scheme)
}
func uriUserinfo(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.userinfo)
}
func uriHost(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.host)
}
func uriPath(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.path)
}
func uriQuery(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.query)
}
func uriFragment(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.fragment)
}
func uriRegistry(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.registry)
}
func uriOpaque(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.opaque)
}
func uriTypecode(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.typecode)
}
func uriDN(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.dn)
}
func uriAttributes(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.attributes)
}
func uriScope(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.scope)
}
func uriFilter(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.filter)
}
func uriExtensions(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.extensions)
}
func uriTo(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriPointerValue(d.to)
}

func uriPort(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if d.port != nil {
		return newInt(*d.port)
	}
	if p, ok := uriDefaultPortNumber(d); ok {
		return newInt(p)
	}
	return R.NilVal
}
func uriDefaultPort(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if p, ok := uriDefaultPortNumber(d); ok {
		return newInt(p)
	}
	return R.NilVal
}
func uriUser(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if d.userinfo == nil {
		return R.NilVal
	}
	v := strings.SplitN(*d.userinfo, ":", 2)[0]
	return rubyString(v)
}
func uriPassword(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if d.userinfo == nil || !strings.Contains(*d.userinfo, ":") {
		return R.NilVal
	}
	return rubyString(strings.SplitN(*d.userinfo, ":", 2)[1])
}
func uriHeaders(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	values := make([]*object.EmeraldValue, 0, len(d.headers))
	for _, h := range d.headers {
		values = append(values, &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{rubyString(h[0]), rubyString(h[1])}, Class: R.Classes["Array"]})
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func uriSetString(receiver, value *object.EmeraldValue, target **string) *object.EmeraldValue {
	if value == nil || value.Type == object.ValueNil {
		*target = nil
		return R.NilVal
	}
	_, raw, e := cgiStringArg(value)
	if e != nil {
		return e
	}
	*target = uriStringPointer(raw)
	return value
}
func uriSetScheme(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriSetString(r, a[0], &d.scheme)
}
func uriSetHost(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if r.Class == R.Classes["URI::MailTo"] {
		return uriInvalidSet("host")
	}
	result := uriSetString(r, a[0], &d.host)
	if result == nil || result.Type != object.ValueException {
		d.userinfo = nil
	}
	return result
}
func uriSetPath(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if r.Class == R.Classes["URI::MailTo"] {
		return uriInvalidSet("path")
	}
	return uriSetString(r, a[0], &d.path)
}
func uriSetQuery(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if r.Class == R.Classes["URI::MailTo"] {
		return uriInvalidSet("query")
	}
	return uriSetString(r, a[0], &d.query)
}
func uriSetFragment(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriSetString(r, a[0], &d.fragment)
}
func uriSetRegistry(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if d.host != nil {
		return uriInvalidSet("registry")
	}
	return uriSetString(r, a[0], &d.registry)
}
func uriSetOpaque(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if d.host != nil || d.path != nil {
		return uriInvalidSet("opaque")
	}
	return uriSetString(r, a[0], &d.opaque)
}
func uriSetTypecode(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriSetString(r, a[0], &d.typecode)
}
func uriSetTo(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	return uriSetString(r, a[0], &d.to)
}

func uriSetUserinfo(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if r.Class == R.Classes["URI::MailTo"] {
		return uriInvalidSet("userinfo")
	}
	if a[0] != nil && a[0].Type == object.ValueArray {
		v := a[0].Data.([]*object.EmeraldValue)
		parts := []string{}
		for _, x := range v {
			_, s, e := cgiStringArg(x)
			if e != nil {
				return e
			}
			parts = append(parts, s)
		}
		d.userinfo = uriStringPointer(strings.Join(parts, ":"))
		return a[0]
	}
	return uriSetString(r, a[0], &d.userinfo)
}
func uriSetUser(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	if r.Class == R.Classes["URI::MailTo"] {
		return uriInvalidSet("user")
	}
	d, _ := r.Data.(*uriData)
	_, user, e := cgiStringArg(a[0])
	if e != nil {
		return e
	}
	d.userinfo = uriStringPointer(user)
	return a[0]
}
func uriSetPassword(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	if r.Class == R.Classes["URI::MailTo"] {
		return uriInvalidSet("password")
	}
	d, _ := r.Data.(*uriData)
	if d.userinfo == nil {
		return uriInvalidSet("password")
	}
	_, password, e := cgiStringArg(a[0])
	if e != nil {
		return e
	}
	user := strings.SplitN(*d.userinfo, ":", 2)[0]
	d.userinfo = uriStringPointer(user + ":" + password)
	return a[0]
}
func uriSetPort(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	if r.Class == R.Classes["URI::MailTo"] {
		return uriInvalidSet("port")
	}
	d, _ := r.Data.(*uriData)
	v, ok := valueToInteger(a[0])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	d.port = &v
	d.explicitPort = true
	return a[0]
}
func uriSetHeaders(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := r.Data.(*uriData)
	if a[0] == nil || a[0].Type != object.ValueArray {
		return typeError("expected Array")
	}
	d.headers = nil
	for _, pair := range a[0].Data.([]*object.EmeraldValue) {
		if pair.Type != object.ValueArray {
			continue
		}
		v := pair.Data.([]*object.EmeraldValue)
		if len(v) < 2 {
			continue
		}
		_, k, e := cgiStringArg(v[0])
		if e != nil {
			return e
		}
		_, val, e := cgiStringArg(v[1])
		if e != nil {
			return e
		}
		d.headers = append(d.headers, [2]string{k, val})
	}
	return a[0]
}
func uriInvalidSet(name string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["URI::InvalidURIError"], "cannot set "+name)
}

func uriClone(value *object.EmeraldValue) *object.EmeraldValue {
	d, _ := value.Data.(*uriData)
	copyData := *d
	copyData.headers = append([][2]string(nil), d.headers...)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &copyData, Class: value.Class}
}
func uriNormalize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	clone := uriClone(receiver)
	uriNormalizeData(clone.Data.(*uriData))
	return clone
}
func uriNormalizeBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*uriData)
	before := uriDataString(d)
	uriNormalizeData(d)
	if uriDataString(d) == before {
		return R.NilVal
	}
	return receiver
}
func uriNormalizeData(d *uriData) {
	if d.scheme != nil {
		s := strings.ToLower(uriNormalizePercent(*d.scheme))
		d.scheme = &s
	}
	if d.host != nil {
		h := strings.ToLower(uriNormalizePercent(*d.host))
		d.host = &h
		if d.path != nil && *d.path == "" {
			d.path = uriStringPointer("/")
		}
	}
	if d.userinfo != nil {
		userinfo := uriNormalizePercent(*d.userinfo)
		if userinfo == "" {
			d.userinfo = nil
		} else {
			d.userinfo = &userinfo
		}
	}
	for _, target := range []**string{&d.path, &d.query, &d.fragment, &d.opaque} {
		if *target != nil {
			value := uriNormalizePercent(**target)
			*target = &value
		}
	}
}

func uriNormalizePercent(value string) string {
	const hex = "0123456789ABCDEF"
	var out strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] == '%' && index+2 < len(value) {
			hi, lo := fromHex(value[index+1]), fromHex(value[index+2])
			if hi >= 0 && lo >= 0 {
				decoded := byte(hi<<4 | lo)
				if decoded >= 'A' && decoded <= 'Z' || decoded >= 'a' && decoded <= 'z' || decoded >= '0' && decoded <= '9' || strings.ContainsRune("-._~", rune(decoded)) {
					out.WriteByte(decoded)
				} else {
					out.WriteByte('%')
					out.WriteByte(hex[decoded>>4])
					out.WriteByte(hex[decoded&15])
				}
				index += 2
				continue
			}
		}
		out.WriteByte(value[index])
	}
	return out.String()
}
func uriEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 || args[0] == nil || receiver.Class != args[0].Class {
		return R.FalseVal
	}
	left := uriClone(receiver)
	right := uriClone(args[0])
	uriNormalizeData(left.Data.(*uriData))
	uriNormalizeData(right.Data.(*uriData))
	return boolValue(uriDataString(left.Data.(*uriData)) == uriDataString(right.Data.(*uriData)))
}
func uriHash(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	normalized := uriNormalize(receiver)
	return CallMethod(rubyString(uriDataString(normalized.Data.(*uriData))), "hash")
}
func uriInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString("#<" + receiver.Class.Name + " " + uriDataString(receiver.Data.(*uriData)) + ">")
}
func uriAbsolute(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*uriData)
	return boolValue(d.scheme != nil)
}
func uriRelative(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*uriData)
	return boolValue(d.scheme == nil)
}
func uriHierarchical(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*uriData)
	return boolValue(d.opaque == nil)
}

func uriMerge(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	other := uriParse(receiver, args...)
	if other.Type == object.ValueException {
		return other
	}
	base := uriClone(receiver)
	b := base.Data.(*uriData)
	r := other.Data.(*uriData)
	if r.scheme != nil {
		return other
	}
	if b.scheme == nil {
		return newRuntimeException(R.Classes["URI::BadURIError"], "both URI are relative")
	}
	if r.host != nil {
		r.scheme = b.scheme
		other.Class = receiver.Class
		return other
	}
	if uriPointerString(r.path) == "" {
		if r.query != nil {
			b.query = r.query
		}
		if r.fragment != nil {
			b.fragment = r.fragment
		}
		return base
	}
	b.query = nil
	b.fragment = nil
	if strings.HasPrefix(uriPointerString(r.path), "/") {
		b.path = uriStringPointer(uriCleanPath(uriPointerString(r.path)))
	} else {
		basePath := uriPointerString(b.path)
		if basePath == "" && b.host != nil {
			basePath = "/"
		}
		directory := basePath
		if !strings.HasSuffix(directory, "/") {
			directory = path.Dir(directory) + "/"
		}
		b.path = uriStringPointer(uriCleanPath(directory + uriPointerString(r.path)))
	}
	b.query = r.query
	b.fragment = r.fragment
	return base
}
func uriMergeBang(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	merged := uriMerge(receiver, args...)
	if merged.Type == object.ValueException {
		return merged
	}
	*receiver.Data.(*uriData) = *merged.Data.(*uriData)
	receiver.Class = merged.Class
	return receiver
}
func uriCleanPath(value string) string {
	trailing := strings.HasSuffix(value, "/") || strings.HasSuffix(value, "/.") || strings.HasSuffix(value, "/..") || value == "." || value == ".."
	clean := path.Clean(value)
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	if trailing && clean != "/" {
		clean += "/"
	}
	if clean == "." {
		return ""
	}
	return clean
}
func uriJoin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	result := uriParse(receiver, args[0])
	if result.Type == object.ValueException {
		return result
	}
	for _, arg := range args[1:] {
		result = uriMerge(result, arg)
		if result.Type == object.ValueException {
			return result
		}
	}
	return result
}

func uriRouteTo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return uriRoute(receiver, args[0], false)
}
func uriRouteFrom(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return uriRoute(receiver, args[0], true)
}
func uriRoute(receiver, otherValue *object.EmeraldValue, reverse bool) *object.EmeraldValue {
	other := uriParse(receiver, otherValue)
	if other.Type == object.ValueException {
		return other
	}
	source, target := receiver, other
	if reverse {
		source, target = other, receiver
	}
	s := source.Data.(*uriData)
	t := target.Data.(*uriData)
	if !strings.EqualFold(uriPointerString(s.scheme), uriPointerString(t.scheme)) {
		return target
	}
	if strings.EqualFold(uriPointerString(s.scheme), "mailto") {
		relative := &uriData{}
		if uriPointerString(s.to) == uriPointerString(t.to) && t.fragment != nil {
			relative.fragment = t.fragment
		}
		return uriValue(relative, "Generic")
	}
	if !strings.EqualFold(uriPointerString(s.host), uriPointerString(t.host)) || uriPortNumber(s) != uriPortNumber(t) {
		copyData := *t
		copyData.scheme = nil
		return uriValue(&copyData, "Generic")
	}
	if uriDataString(s) == uriDataString(t) {
		return uriValue(&uriData{path: uriStringPointer("")}, "Generic")
	}
	if uriPointerString(s.path) == uriPointerString(t.path) && uriPointerString(s.query) == uriPointerString(t.query) {
		return uriValue(&uriData{path: uriStringPointer(""), fragment: t.fragment}, "Generic")
	}
	if strings.Contains(uriPointerString(t.path), "/./") || strings.Contains(uriPointerString(t.path), "/../") {
		return uriValue(&uriData{path: t.path, query: t.query, fragment: t.fragment}, "Generic")
	}
	sourcePath := uriPointerString(s.path)
	if !strings.HasSuffix(sourcePath, "/") {
		sourcePath = path.Dir(sourcePath)
	}
	if t.query != nil && strings.TrimSuffix(uriPointerString(t.path), "/") == strings.TrimSuffix(sourcePath, "/") {
		return uriValue(&uriData{path: uriStringPointer(""), query: t.query, fragment: t.fragment}, "Generic")
	}
	rel, err := filepath.Rel(sourcePath, uriPointerString(t.path))
	if err != nil {
		return target
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		if strings.HasSuffix(uriPointerString(t.path), "/") {
			rel = "./"
		} else {
			rel = ""
		}
	}
	if strings.HasSuffix(uriPointerString(t.path), "/") && rel != "" && !strings.HasSuffix(rel, "/") {
		rel += "/"
	}
	d := &uriData{path: uriStringPointer(rel), query: t.query, fragment: t.fragment}
	return uriValue(d, "Generic")
}
func uriPortNumber(d *uriData) int64 {
	if d.port != nil {
		return *d.port
	}
	p, _ := uriDefaultPortNumber(d)
	return p
}

func uriRequestURI(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*uriData)
	value := uriPointerString(d.path)
	if value == "" {
		value = "/"
	}
	if d.query != nil {
		value += "?" + *d.query
	}
	return rubyString(value)
}

func uriSplit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := uriParse(receiver, args...)
	if value.Type == object.ValueException {
		return value
	}
	return uriComponentAry(value)
}
func uriExtract(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	_, raw, e := cgiStringArg(args[0])
	if e != nil {
		return e
	}
	allowed := map[string]bool{}
	if len(args) == 2 && args[1] != nil && args[1].Type == object.ValueArray {
		for _, v := range args[1].Data.([]*object.EmeraldValue) {
			_, s, err := cgiStringArg(v)
			if err != nil {
				return err
			}
			allowed[strings.ToLower(s)] = true
		}
	}
	matches := uriExtractPattern.FindAllString(raw, -1)
	values := []*object.EmeraldValue{}
	for _, match := range matches {
		scheme := strings.ToLower(strings.SplitN(match, ":", 2)[0])
		if len(allowed) > 0 && !allowed[scheme] {
			continue
		}
		value := rubyString(strings.TrimRight(match, ".,"))
		if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
			if result := CallBlockWithArgs(CurrentBlockValue(), value); result != nil && result.Type == object.ValueException {
				return result
			}
		} else {
			values = append(values, value)
		}
	}
	if BlockGivenCheck != nil && BlockGivenCheck() {
		return R.NilVal
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}
func uriRegexp(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	scheme := `[A-Za-z][A-Za-z0-9+.-]*`
	if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueArray {
		values := args[0].Data.([]*object.EmeraldValue)
		if len(values) == 0 {
			return regexpClassNew(&object.EmeraldValue{Type: object.ValueClass, Data: R.Classes["Regexp"], Class: R.Classes["Class"]}, rubyString(`\A\z.`))
		}
		names := make([]string, 0, len(values))
		for _, value := range values {
			_, name, errVal := cgiStringArg(value)
			if errVal != nil {
				return errVal
			}
			names = append(names, regexp.QuoteMeta(name))
		}
		scheme = `(?:` + strings.Join(names, "|") + `)`
	}
	return regexpClassNew(&object.EmeraldValue{Type: object.ValueClass, Data: R.Classes["Regexp"], Class: R.Classes["Class"]}, rubyString(scheme+`:[^\s<>"]*`))
}
func uriEscape(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return cgiEscapeURIComponent(receiver, args[0])
}
func uriUnescape(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return cgiUnescapeURIComponent(receiver, args[0])
}
func uriParserNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueObject, Data: &object.Object{InstanceVars: map[string]*object.EmeraldValue{}}, Class: R.Classes["URI::RFC2396_Parser"]}
}
func uriParserInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString("#<URI::RFC2396_Parser>")
}

func uriClassBuild(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	className := strings.TrimPrefix(klass.Name, "URI::")
	if className == "MailTo" {
		return uriMailToBuild(args[0])
	}
	data := &uriData{}
	if className != "Generic" {
		s := strings.ToLower(className)
		if s == "mailto" {
			s = "mailto"
		}
		data.scheme = &s
	}
	return uriValue(data, className)
}

func uriMailToBuild(argument *object.EmeraldValue) *object.EmeraldValue {
	data := &uriData{scheme: uriStringPointer("mailto"), to: uriStringPointer("")}
	var toValue, headersValue *object.EmeraldValue
	switch {
	case argument != nil && argument.Type == object.ValueArray:
		values := argument.Data.([]*object.EmeraldValue)
		if len(values) > 0 {
			toValue = values[0]
		}
		if len(values) > 1 {
			headersValue = values[1]
		}
	case argument != nil && argument.Type == object.ValueHash:
		hash := valueToHashMap(argument)
		toValue, _ = hashLookup(hash, rubySymbol("to"))
		headersValue, _ = hashLookup(hash, rubySymbol("headers"))
	default:
		return typeError("expected Array or Hash")
	}
	if toValue != nil && toValue.Type != object.ValueNil {
		_, to, errVal := cgiStringArg(toValue)
		if errVal != nil {
			return errVal
		}
		if strings.ContainsAny(to, "?:") {
			return newRuntimeException(R.Classes["URI::InvalidComponentError"], "bad component")
		}
		data.to = uriStringPointer(to)
	}
	if headersValue != nil && headersValue.Type != object.ValueNil {
		if headersValue.Type != object.ValueArray {
			return newRuntimeException(R.Classes["URI::InvalidComponentError"], "bad component")
		}
		for _, value := range headersValue.Data.([]*object.EmeraldValue) {
			_, header, errVal := cgiStringArg(value)
			if errVal != nil {
				return errVal
			}
			parts := strings.Split(header, "=")
			if len(parts) != 2 || parts[0] == "" || strings.Contains(parts[1], "?") {
				return newRuntimeException(R.Classes["URI::InvalidComponentError"], "bad component")
			}
			data.headers = append(data.headers, [2]string{parts[0], parts[1]})
		}
	}
	return uriValue(data, "MailTo")
}
