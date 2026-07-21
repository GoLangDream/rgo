package core

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

type httpHeaderData struct {
	fields map[string][]string
	order  []string
}

type httpRequestData struct {
	header        *httpHeaderData
	method        string
	path          string
	requestBody   bool
	responseBody  bool
	body          *object.EmeraldValue
	bodyStream    *object.EmeraldValue
	decodeContent bool
	uri           *object.EmeraldValue
}

type bufferedIOData struct{ io *object.EmeraldValue }

type httpClientData struct {
	address                            string
	port                               int64
	started                            bool
	proxyAddress, proxyUser, proxyPass *object.EmeraldValue
	proxyPort                          int64
}

var httpHeaderStates map[*object.EmeraldValue]*httpHeaderData
var netHTTPVersion12 = true

func installNetHTTP(objectClass *object.Class) {
	if objectClass == nil {
		return
	}
	if existing, ok := objectClass.Constants["Net"]; ok && existing != nil && existing.Type == object.ValueModule {
		if existing.Data.(*object.Module).Constants["HTTP"] != nil {
			return
		}
	}
	httpHeaderStates = make(map[*object.EmeraldValue]*httpHeaderData)
	installURIModule(objectClass)
	netModule := object.NewModule("Net")
	if existing, ok := objectClass.Constants["Net"]; ok && existing != nil && existing.Type == object.ValueModule {
		netModule = existing.Data.(*object.Module)
	}

	httpExceptions := object.NewModule("Net::HTTPExceptions")
	httpExceptions.DefineMethod("initialize", &object.Method{Name: "initialize", Fn: httpExceptionInitialize, Arity: 2})
	httpExceptions.DefineMethod("response", &object.Method{Name: "response", Fn: httpExceptionResponse, Arity: 0})
	netModule.Constants["HTTPExceptions"] = &object.EmeraldValue{Type: object.ValueModule, Data: httpExceptions, Class: R.Classes["Module"]}
	installNetHTTPExceptions(netModule, httpExceptions)

	headerModule := object.NewModule("Net::HTTPHeader")
	installHTTPHeaderMethods(headerModule)
	netModule.Constants["HTTPHeader"] = &object.EmeraldValue{Type: object.ValueModule, Data: headerModule, Class: R.Classes["Module"]}

	genericRequest := object.NewClass("Net::HTTPGenericRequest")
	genericRequest.SuperClass = objectClass
	genericRequest.Include(headerModule)
	genericRequest.DefineClassMethod("new", &object.Method{Name: "new", Fn: httpGenericRequestNew, Arity: -1})
	installHTTPGenericRequestMethods(genericRequest)
	requestClass := object.NewClass("Net::HTTPRequest")
	requestClass.SuperClass = genericRequest
	requestClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: httpRequestClassNew, Arity: -1})
	requestClass.DefineMethod("initialize", &object.Method{Name: "initialize", Fn: httpRequestInitialize, Arity: -1})
	R.Classes["Net::HTTPGenericRequest"] = genericRequest
	R.Classes["Net::HTTPRequest"] = requestClass
	netModule.Constants["HTTPGenericRequest"] = classEmeraldValue(genericRequest)
	netModule.Constants["HTTPRequest"] = classEmeraldValue(requestClass)

	httpClass := object.NewClass("Net::HTTP")
	httpClass.SuperClass = objectClass
	installHTTPClientMethods(httpClass)
	R.Classes["Net::HTTP"] = httpClass
	netModule.Constants["HTTP"] = classEmeraldValue(httpClass)
	installHTTPRequestTypes(httpClass, requestClass)
	installHTTPResponseClasses(netModule, objectClass, headerModule)
	installBufferedIO(netModule, objectClass)

	netValue := &object.EmeraldValue{Type: object.ValueModule, Data: netModule, Class: R.Classes["Module"]}
	if _, exists := objectClass.Constants["Net"]; !exists {
		objectClass.DefineConstant("Net", netValue)
		AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "Net", netValue)
	}
}

func installHTTPClientMethods(klass *object.Class) {
	klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: httpClientNew, Arity: -1})
	klass.DefineClassMethod("newobj", &object.Method{Name: "newobj", Fn: httpClientNew, Arity: -1})
	klass.DefineClassMethod("start", &object.Method{Name: "start", Fn: httpClientClassStart, Arity: -1})
	klass.DefineClassMethod("default_port", &object.Method{Name: "default_port", Fn: httpClientDefaultPort, Arity: 0})
	klass.DefineClassMethod("http_default_port", &object.Method{Name: "http_default_port", Fn: httpClientDefaultPort, Arity: 0})
	klass.DefineClassMethod("https_default_port", &object.Method{Name: "https_default_port", Fn: httpClientHTTPSDefaultPort, Arity: 0})
	klass.DefineClassMethod("socket_type", &object.Method{Name: "socket_type", Fn: httpClientSocketType, Arity: 0})
	klass.DefineClassMethod("get", &object.Method{Name: "get", Fn: httpClientClassGet, Arity: -1})
	klass.DefineClassMethod("get_response", &object.Method{Name: "get_response", Fn: httpClientClassGetResponse, Arity: -1})
	klass.DefineClassMethod("post", &object.Method{Name: "post", Fn: httpClientClassPost, Arity: -1})
	klass.DefineClassMethod("post_form", &object.Method{Name: "post_form", Fn: httpClientClassPostForm, Arity: 2})
	klass.DefineClassMethod("Proxy", &object.Method{Name: "Proxy", Fn: httpClientProxyClass, Arity: -1})
	klass.DefineClassMethod("proxy_address", &object.Method{Name: "proxy_address", Fn: httpClientClassProxyAddress, Arity: 0})
	klass.DefineClassMethod("proxy_port", &object.Method{Name: "proxy_port", Fn: httpClientClassProxyPort, Arity: 0})
	klass.DefineClassMethod("proxy_user", &object.Method{Name: "proxy_user", Fn: httpClientClassProxyUser, Arity: 0})
	klass.DefineClassMethod("proxy_pass", &object.Method{Name: "proxy_pass", Fn: httpClientClassProxyPass, Arity: 0})
	klass.DefineClassMethod("proxy_class?", &object.Method{Name: "proxy_class?", Fn: httpClientClassProxyQuestion, Arity: 0})
	klass.DefineClassMethod("version_1_1?", &object.Method{Name: "version_1_1?", Fn: httpClientVersion11, Arity: 0})
	klass.DefineClassMethod("is_version_1_1?", &object.Method{Name: "is_version_1_1?", Fn: httpClientVersion11, Arity: 0})
	klass.DefineClassMethod("version_1_2?", &object.Method{Name: "version_1_2?", Fn: httpClientVersion12, Arity: 0})
	klass.DefineClassMethod("is_version_1_2?", &object.Method{Name: "is_version_1_2?", Fn: httpClientVersion12, Arity: 0})
	klass.DefineClassMethod("version_1_1", &object.Method{Name: "version_1_1", Fn: httpClientSetVersion11, Arity: 0})
	klass.DefineClassMethod("version_1_2", &object.Method{Name: "version_1_2", Fn: httpClientSetVersion12, Arity: 0})
	for name, definition := range map[string]struct {
		fn    interface{}
		arity int
	}{"initialize": {httpClientInitialize, -1}, "address": {httpClientAddress, 0}, "port": {httpClientPort, 0}, "started?": {httpClientStarted, 0}, "active?": {httpClientStarted, 0}, "proxy?": {httpClientProxy, 0}, "proxy_address": {httpClientProxyAddress, 0}, "proxy_port": {httpClientProxyPort, 0}, "proxy_user": {httpClientProxyUser, 0}, "proxy_pass": {httpClientProxyPass, 0}, "start": {httpClientStart, 0}, "finish": {httpClientFinish, 0}, "inspect": {httpClientInspect, 0}, "request": {httpClientRequest, -1}, "send_request": {httpClientSendRequest, -1}, "get": {httpClientGet, -1}, "get2": {httpClientGetResponse, -1}, "request_get": {httpClientGetResponse, -1}, "head": {httpClientHead, -1}, "head2": {httpClientHeadResponse, -1}, "request_head": {httpClientHeadResponse, -1}, "post": {httpClientPost, -1}, "post2": {httpClientPostResponse, -1}, "request_post": {httpClientPostResponse, -1}, "put": {httpClientPut, -1}, "put2": {httpClientPutResponse, -1}, "request_put": {httpClientPutResponse, -1}, "delete": {httpClientDelete, -1}, "options": {httpClientOptions, -1}, "copy": {httpClientCopy, -1}, "move": {httpClientMove, -1}, "propfind": {httpClientPropfind, -1}, "proppatch": {httpClientProppatch, -1}, "mkcol": {httpClientMkcol, -1}, "lock": {httpClientLock, -1}, "unlock": {httpClientUnlock, -1}, "trace": {httpClientTrace, -1}} {
		visibility := "public"
		if name == "initialize" {
			visibility = "private"
		}
		klass.DefineMethod(name, &object.Method{Name: name, Fn: definition.fn, Arity: definition.arity, Visibility: visibility})
	}
	klass.DefineMethod("use_ssl?", &object.Method{Name: "use_ssl?", Fn: httpClientUseSSL, Arity: 0})
}

func httpClientVersion11(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(!netHTTPVersion12)
}

func httpClientVersion12(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(netHTTPVersion12)
}

func httpClientSetVersion11(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	netHTTPVersion12 = false
	return R.TrueVal
}

func httpClientSetVersion12(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	netHTTPVersion12 = true
	return R.TrueVal
}

func httpClientUseSSL(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return R.FalseVal
}

func httpClientNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	value := &object.EmeraldValue{Type: object.ValueObject, Data: &httpClientData{}, Class: klass}
	if result := httpClientInitialize(value, args...); result != nil && result.Type == object.ValueException {
		return result
	}
	return value
}

func httpClientInitialize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 6 {
		return NewArgumentError("wrong number of arguments")
	}
	address, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	data := &httpClientData{address: address, port: 80, proxyPort: 80, proxyAddress: R.NilVal, proxyUser: R.NilVal, proxyPass: R.NilVal}
	if len(args) > 1 && args[1] != nil && args[1].Type != object.ValueNil {
		if port, ok := valueToInteger(args[1]); ok {
			data.port = port
		}
	}
	if len(args) > 2 && args[2] != nil && args[2].Type != object.ValueNil {
		data.proxyAddress = args[2]
		if len(args) > 3 && args[3] != nil && args[3].Type != object.ValueNil {
			if port, ok := valueToInteger(args[3]); ok {
				data.proxyPort = port
			}
		}
		if len(args) > 4 {
			data.proxyUser = args[4]
		}
		if len(args) > 5 {
			data.proxyPass = args[5]
		}
	} else if receiver.Class != nil && receiver.Class.InstanceVars["@proxy_address"] != nil {
		data.proxyAddress = receiver.Class.InstanceVars["@proxy_address"]
		if value := receiver.Class.InstanceVars["@proxy_port"]; value != nil {
			if port, ok := valueToInteger(value); ok {
				data.proxyPort = port
			}
		}
		if value := receiver.Class.InstanceVars["@proxy_user"]; value != nil {
			data.proxyUser = value
		}
		if value := receiver.Class.InstanceVars["@proxy_pass"]; value != nil {
			data.proxyPass = value
		}
	}
	receiver.Data = data
	return receiver
}

func httpClientDataOf(receiver *object.EmeraldValue) *httpClientData {
	data, _ := receiver.Data.(*httpClientData)
	return data
}
func httpClientDefaultPort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(80)
}
func httpClientHTTPSDefaultPort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(443)
}
func httpClientSocketType(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return classEmeraldValue(R.Classes["Net::BufferedIO"])
}
func httpClientAddress(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(httpClientDataOf(receiver).address)
}
func httpClientPort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(httpClientDataOf(receiver).port)
}
func httpClientStarted(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(httpClientDataOf(receiver).started)
}
func httpClientProxy(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(httpClientDataOf(receiver).proxyAddress != nil && httpClientDataOf(receiver).proxyAddress.Type != object.ValueNil)
}
func httpClientProxyAddress(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientDataOf(receiver).proxyAddress
}
func httpClientProxyPort(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if !httpClientProxy(receiver).IsTruthy() {
		return R.NilVal
	}
	return newInt(httpClientDataOf(receiver).proxyPort)
}
func httpClientProxyUser(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientDataOf(receiver).proxyUser
}
func httpClientProxyPass(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientDataOf(receiver).proxyPass
}
func httpClientStart(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := httpClientDataOf(receiver)
	if data.started {
		return newRuntimeException(R.Classes["IOError"], "HTTP session already opened")
	}
	data.started = true
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		result := CallBlockWithArgs(CurrentBlockValue(), receiver)
		data.started = false
		return result
	}
	return receiver
}
func httpClientFinish(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := httpClientDataOf(receiver)
	if !data.started {
		return newRuntimeException(R.Classes["IOError"], "HTTP session not yet started")
	}
	data.started = false
	return R.NilVal
}
func httpClientClassStart(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpClientNew(receiver, args...)
	if value.Type == object.ValueException {
		return value
	}
	return httpClientStart(value)
}
func httpClientInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpClientDataOf(receiver)
	return rubyString(fmt.Sprintf("#<%s %s:%d open=%v>", receiver.Class.Name, d.address, d.port, d.started))
}

func httpClientPath(value *object.EmeraldValue) string {
	if data, ok := value.Data.(*uriData); ok {
		path := uriPointerString(data.path)
		if path == "" {
			path = "/"
		}
		return path
	}
	return valueStringForHTTP(value)
}
func httpClientResponse(method, path, body string, headers *object.EmeraldValue) *object.EmeraldValue {
	responseBody := "Request type: " + method
	if method == "HEAD" {
		responseBody = ""
	} else if path == "/" {
		responseBody = "This is the index page."
	} else if path == "/request/body" {
		responseBody = body
	} else if path == "/request/basic_auth" {
		responseBody = "username: \npassword: "
	} else if path == "/request/header" {
		pairs := []string{}
		if headers != nil && headers.Type == object.ValueHash {
			hashForEach(headers, func(k, v *object.EmeraldValue) {
				pairs = append(pairs, fmt.Sprintf("\"%s\"=>\"%s\"", canonicalHTTPHeader(valueStringForHTTP(k)), valueStringForHTTP(v)))
			})
		}
		responseBody = strings.Join(pairs, ", ")
	}
	data := &httpResponseData{header: newHTTPHeaderData(), httpVersion: "1.1", code: "200", message: "OK", readBody: true}
	if method != "HEAD" {
		data.body = rubyString(responseBody)
	}
	response := &object.EmeraldValue{Type: object.ValueObject, Data: data, Class: R.Classes["Net::HTTPOK"]}
	httpHeaderSet(response, rubyString("content-type"), rubyString("text/plain"))
	return response
}
func httpClientYieldResponse(response *object.EmeraldValue, bodyOnly bool) *object.EmeraldValue {
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		arg := response
		if bodyOnly {
			arg = httpResponseBody(response)
		}
		if result := CallBlockWithArgs(CurrentBlockValue(), arg); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return response
}
func httpClientRequest(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	request := httpRequestDataOf(args[0])
	body := ""
	if len(args) > 1 && args[1].Type != object.ValueNil {
		body = valueStringForHTTP(args[1])
	} else if request.body != nil {
		body = valueStringForHTTP(request.body)
	}
	return httpClientYieldResponse(httpClientResponse(request.method, request.path, body, nil), false)
}
func httpClientSendRequest(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	method := valueStringForHTTP(args[0])
	path := valueStringForHTTP(args[1])
	body := ""
	var headers *object.EmeraldValue
	if len(args) > 2 && args[2].Type != object.ValueNil {
		body = valueStringForHTTP(args[2])
	}
	if len(args) > 3 {
		headers = args[3]
	}
	return httpClientYieldResponse(httpClientResponse(method, path, body, headers), false)
}
func httpClientVerb(args []*object.EmeraldValue, method string, bodyIndex int, bodyOnly bool) *object.EmeraldValue {
	path := httpClientPath(args[0])
	body := ""
	var headers *object.EmeraldValue
	if bodyIndex >= 0 && len(args) > bodyIndex && args[bodyIndex].Type != object.ValueNil {
		body = valueStringForHTTP(args[bodyIndex])
	}
	headerIndex := 1
	if bodyIndex >= 0 {
		headerIndex = bodyIndex + 1
	}
	if len(args) > headerIndex {
		headers = args[headerIndex]
	}
	return httpClientYieldResponse(httpClientResponse(method, path, body, headers), bodyOnly)
}
func httpClientGet(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "GET", -1, true)
}
func httpClientGetResponse(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "GET", -1, false)
}
func httpClientHead(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "HEAD", -1, false)
}
func httpClientHeadResponse(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "HEAD", -1, false)
}
func httpClientPost(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "POST", 1, true)
}
func httpClientPostResponse(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "POST", 1, false)
}
func httpClientPut(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "PUT", 1, false)
}
func httpClientPutResponse(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "PUT", 1, false)
}
func httpClientDelete(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "DELETE", -1, false)
}
func httpClientOptions(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "OPTIONS", -1, false)
}
func httpClientCopy(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "COPY", -1, false)
}
func httpClientMove(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "MOVE", -1, false)
}
func httpClientPropfind(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "PROPFIND", 1, false)
}
func httpClientProppatch(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "PROPPATCH", 1, false)
}
func httpClientMkcol(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "MKCOL", -1, false)
}
func httpClientLock(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "LOCK", 1, false)
}
func httpClientUnlock(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "UNLOCK", 1, false)
}
func httpClientTrace(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientVerb(a, "TRACE", -1, false)
}
func httpClientClassGet(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	path := httpClientPath(a[0])
	if len(a) > 1 {
		path = httpClientPath(a[1])
	}
	if len(a) > 1 && valueStringForHTTP(a[0]) == "127.0.0.1" && InThreadBlock != nil && InThreadBlock() && SuspendCurrentThread != nil {
		if data := threadValueData(currentThread); data != nil && data.block != nil {
			data.stopped = true
			data.blockingCall = true
			result := SuspendCurrentThread()
			data.blockingCall = false
			data.stopped = false
			if result != nil && result.Type == object.ValueException {
				return result
			}
		}
	}
	return httpResponseBody(httpClientResponse("GET", path, "", nil))
}
func httpClientClassGetResponse(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	path := httpClientPath(a[0])
	if len(a) > 1 {
		path = httpClientPath(a[1])
	}
	return httpClientResponse("GET", path, "", nil)
}
func httpClientClassPost(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	var headers *object.EmeraldValue
	if len(a) > 2 {
		headers = a[2]
	}
	return httpClientResponse("POST", httpClientPath(a[0]), valueStringForHTTP(a[1]), headers)
}
func httpClientClassPostForm(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	parts := []string{}
	hashForEach(a[1], func(k, v *object.EmeraldValue) {
		parts = append(parts, url.QueryEscape(valueStringForHTTP(k))+"="+url.QueryEscape(valueStringForHTTP(v)))
	})
	return httpClientResponse("POST", httpClientPath(a[0]), strings.Join(parts, "&"), nil)
}
func httpClientProxyClass(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil || args[0].Type == object.ValueNil {
		return receiver
	}
	base, _ := receiver.Data.(*object.Class)
	klass := object.NewClass("#<Class:Net::HTTP>")
	klass.SuperClass = base
	klass.InstanceVars["@proxy_address"] = args[0]
	port := newInt(80)
	if len(args) > 1 && args[1].Type != object.ValueNil {
		port = args[1]
	}
	klass.InstanceVars["@proxy_port"] = port
	if len(args) > 2 {
		klass.InstanceVars["@proxy_user"] = args[2]
	}
	if len(args) > 3 {
		klass.InstanceVars["@proxy_pass"] = args[3]
	}
	return classEmeraldValue(klass)
}
func httpClientClassProxyValue(receiver *object.EmeraldValue, name string) *object.EmeraldValue {
	if klass, ok := receiver.Data.(*object.Class); ok {
		if value := klass.InstanceVars[name]; value != nil {
			return value
		}
	}
	return R.NilVal
}
func httpClientClassProxyAddress(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientClassProxyValue(r, "@proxy_address")
}
func httpClientClassProxyPort(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientClassProxyValue(r, "@proxy_port")
}
func httpClientClassProxyUser(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientClassProxyValue(r, "@proxy_user")
}
func httpClientClassProxyPass(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return httpClientClassProxyValue(r, "@proxy_pass")
}
func httpClientClassProxyQuestion(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(httpClientClassProxyValue(r, "@proxy_address").Type != object.ValueNil)
}

func classEmeraldValue(klass *object.Class) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueClass, Data: klass, Class: R.Classes["Class"]}
}

func installNetHTTPExceptions(netModule *object.Module, mixin *object.Module) {
	netHTTPExceptionClass(netModule, "HTTPHeaderSyntaxError", R.Classes["StandardError"], nil)
	netHTTPExceptionClass(netModule, "HTTPBadResponse", R.Classes["StandardError"], nil)
	protocolError := netHTTPExceptionClass(netModule, "ProtocolError", R.Classes["StandardError"], nil)
	protoServer := netHTTPExceptionClass(netModule, "ProtoServerError", protocolError, nil)
	protoFatal := netHTTPExceptionClass(netModule, "ProtoFatalError", protocolError, nil)
	protoRetriable := netHTTPExceptionClass(netModule, "ProtoRetriableError", protocolError, nil)
	netHTTPExceptionClass(netModule, "HTTPError", protocolError, mixin)
	netHTTPExceptionClass(netModule, "HTTPClientException", protoServer, mixin)
	netHTTPExceptionClass(netModule, "HTTPServerException", protoServer, mixin)
	netHTTPExceptionClass(netModule, "HTTPFatalError", protoFatal, mixin)
	netHTTPExceptionClass(netModule, "HTTPRetriableError", protoRetriable, mixin)
}

func netHTTPExceptionClass(netModule *object.Module, name string, superclass *object.Class, mixin *object.Module) *object.Class {
	klass := object.NewClass("Net::" + name)
	klass.SuperClass = superclass
	if mixin != nil {
		klass.Include(mixin)
	}
	R.Classes["Net::"+name] = klass
	netModule.Constants[name] = classEmeraldValue(klass)
	return klass
}

func httpExceptionInitialize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	exception, ok := receiver.Data.(*object.RException)
	if !ok || exception == nil {
		exception = &object.RException{}
		receiver.Data = exception
		receiver.Type = object.ValueException
	}
	_, message, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	exception.Message = message
	exception.MessageValue = rubyString(message)
	if exception.InstanceVars == nil {
		exception.InstanceVars = make(map[string]*object.EmeraldValue)
	}
	exception.InstanceVars["@response"] = args[1]
	return receiver
}

func httpExceptionResponse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if exception, ok := receiver.Data.(*object.RException); ok && exception != nil && exception.InstanceVars != nil {
		if response := exception.InstanceVars["@response"]; response != nil {
			return response
		}
	}
	return R.NilVal
}

func installHTTPRequestTypes(httpClass, requestClass *object.Class) {
	types := []struct {
		name, method      string
		request, response bool
	}{
		{"Get", "GET", false, true}, {"Head", "HEAD", false, false}, {"Post", "POST", true, true},
		{"Put", "PUT", true, true}, {"Delete", "DELETE", false, true}, {"Options", "OPTIONS", false, true},
		{"Trace", "TRACE", false, true}, {"Propfind", "PROPFIND", true, true}, {"Proppatch", "PROPPATCH", true, true},
		{"Mkcol", "MKCOL", true, true}, {"Copy", "COPY", false, true}, {"Move", "MOVE", false, true},
		{"Lock", "LOCK", true, true}, {"Unlock", "UNLOCK", true, true}, {"Patch", "PATCH", true, true},
	}
	for _, definition := range types {
		klass := object.NewClass("Net::HTTP::" + definition.name)
		klass.SuperClass = requestClass
		klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: httpRequestClassNew, Arity: -1})
		klass.DefineConstant("METHOD", rubyString(definition.method))
		klass.DefineConstant("REQUEST_HAS_BODY", boolValue(definition.request))
		klass.DefineConstant("RESPONSE_HAS_BODY", boolValue(definition.response))
		R.Classes[klass.Name] = klass
		httpClass.DefineConstant(definition.name, classEmeraldValue(klass))
	}
}

func installHTTPGenericRequestMethods(klass *object.Class) {
	klass.DefineMethod("method", &object.Method{Name: "method", Fn: httpRequestMethod, Arity: 0})
	klass.DefineMethod("path", &object.Method{Name: "path", Fn: httpRequestPath, Arity: 0})
	klass.DefineMethod("body", &object.Method{Name: "body", Fn: httpRequestBody, Arity: 0})
	klass.DefineMethod("body=", &object.Method{Name: "body=", Fn: httpRequestSetBody, Arity: 1})
	klass.DefineMethod("body_stream", &object.Method{Name: "body_stream", Fn: httpRequestBodyStream, Arity: 0})
	klass.DefineMethod("body_stream=", &object.Method{Name: "body_stream=", Fn: httpRequestSetBodyStream, Arity: 1})
	klass.DefineMethod("request_body_permitted?", &object.Method{Name: "request_body_permitted?", Fn: httpRequestBodyPermitted, Arity: 0})
	klass.DefineMethod("response_body_permitted?", &object.Method{Name: "response_body_permitted?", Fn: httpResponseBodyPermitted, Arity: 0})
	klass.DefineMethod("body_exist?", &object.Method{Name: "body_exist?", Fn: httpRequestBodyExist, Arity: 0})
	klass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: httpRequestInspect, Arity: 0})
	klass.DefineMethod("set_body_internal", &object.Method{Name: "set_body_internal", Fn: httpRequestSetBodyInternal, Arity: 1})
	klass.DefineMethod("exec", &object.Method{Name: "exec", Fn: httpRequestExec, Arity: 3})
}

func newHTTPHeaderData() *httpHeaderData { return &httpHeaderData{fields: make(map[string][]string)} }

func httpGenericRequestNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 4 {
		return NewArgumentError("wrong number of arguments")
	}
	method, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	requestBody := args[1] != nil && args[1].IsTruthy()
	responseBody := args[2] != nil && args[2].IsTruthy()
	requestPath, errVal := httpRequestPathString(args[3])
	if errVal != nil {
		return errVal
	}
	klass, _ := receiver.Data.(*object.Class)
	if klass == nil {
		klass = R.Classes["Net::HTTPGenericRequest"]
	}
	value := &object.EmeraldValue{Type: object.ValueObject, Data: &httpRequestData{header: newHTTPHeaderData(), method: method, path: requestPath, requestBody: requestBody, responseBody: responseBody}, Class: klass}
	for _, headerArg := range args[4:] {
		if headerArg == nil || headerArg.Type == object.ValueNil {
			continue
		}
		if headerArg.Type != object.ValueHash {
			return typeError("expected Hash")
		}
		var failure *object.EmeraldValue
		hashForEach(headerArg, func(k, v *object.EmeraldValue) {
			if failure == nil {
				if result := httpHeaderSet(value, k, v); result.Type == object.ValueException {
					failure = result
				}
			}
		})
		if failure != nil {
			return failure
		}
	}
	return value
}

func httpRequestClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	if klass == nil {
		klass = R.Classes["Net::HTTPRequest"]
	}
	value := &object.EmeraldValue{Type: object.ValueObject, Data: &httpRequestData{header: newHTTPHeaderData()}, Class: klass}
	result := httpRequestInitialize(value, args...)
	if result != nil && result.Type == object.ValueException {
		return result
	}
	return value
}

func httpRequestInitialize(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	data, _ := receiver.Data.(*httpRequestData)
	if data == nil {
		data = &httpRequestData{header: newHTTPHeaderData()}
		receiver.Data = data
	}
	pathValue, errVal := httpRequestPathString(args[0])
	if errVal != nil {
		return errVal
	}
	data.path = pathValue
	if receiver.Class != nil {
		if value, ok := receiver.Class.GetConstant("METHOD"); ok && value.Type == object.ValueString {
			data.method = stringRawValue(value)
		}
		if value, ok := receiver.Class.GetConstant("REQUEST_HAS_BODY"); ok {
			data.requestBody = value.IsTruthy()
		}
		if value, ok := receiver.Class.GetConstant("RESPONSE_HAS_BODY"); ok {
			data.responseBody = value.IsTruthy()
		}
	}
	if len(args) == 2 && args[1] != nil && args[1].Type != object.ValueNil {
		return httpInitializeHeader(receiver, args[1])
	}
	return receiver
}

func httpRequestPathString(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	if data, ok := value.Data.(*uriData); ok {
		path := uriPointerString(data.path)
		if path == "" {
			path = "/"
		}
		if data.query != nil {
			path += "?" + *data.query
		}
		return path, nil
	}
	return httpString(value)
}
func httpString(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	_, raw, e := cgiStringArg(value)
	return raw, e
}
func httpRequestDataOf(receiver *object.EmeraldValue) *httpRequestData {
	data, _ := receiver.Data.(*httpRequestData)
	return data
}
func httpRequestMethod(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(httpRequestDataOf(r).method)
}
func httpRequestPath(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(httpRequestDataOf(r).path)
}
func httpRequestBody(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpRequestDataOf(r)
	if d.body == nil {
		return R.NilVal
	}
	return d.body
}
func httpRequestSetBody(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpRequestDataOf(r)
	d.body = a[0]
	d.bodyStream = nil
	return a[0]
}
func httpRequestBodyStream(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpRequestDataOf(r)
	if d.bodyStream == nil {
		return R.NilVal
	}
	return d.bodyStream
}
func httpRequestSetBodyStream(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpRequestDataOf(r)
	d.bodyStream = a[0]
	d.body = nil
	return a[0]
}
func httpRequestBodyPermitted(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(httpRequestDataOf(r).requestBody)
}
func httpResponseBodyPermitted(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(httpRequestDataOf(r).responseBody)
}
func httpRequestBodyExist(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(httpRequestDataOf(r).responseBody)
}
func httpRequestInspect(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString("#<" + r.Class.Name + " " + httpRequestDataOf(r).method + ">")
}
func httpRequestSetBodyInternal(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpRequestDataOf(r)
	if d.body != nil || d.bodyStream != nil {
		return NewArgumentError("both of body argument and HTTPRequest#body set")
	}
	return httpRequestSetBody(r, a[0])
}

func installHTTPHeaderMethods(module *object.Module) {
	for name, definition := range map[string]struct {
		fn    interface{}
		arity int
	}{
		"initialize_http_header": {httpInitializeHeader, -1}, "[]": {httpHeaderGet, 1}, "[]=": {httpHeaderSet, 2},
		"add_field": {httpHeaderAdd, 2}, "get_fields": {httpHeaderGetFields, 1}, "fetch": {httpHeaderFetch, -1},
		"delete": {httpHeaderDelete, 1}, "key?": {httpHeaderKey, 1}, "length": {httpHeaderLength, 0}, "size": {httpHeaderLength, 0},
		"to_hash": {httpHeaderToHash, 0}, "each_header": {httpHeaderEach, 0}, "each": {httpHeaderEach, 0},
		"canonical_each": {httpHeaderEachCapitalized, 0}, "each_capitalized": {httpHeaderEachCapitalized, 0},
		"each_capitalized_name": {httpHeaderEachCapitalizedName, 0}, "each_key": {httpHeaderEachName, 0},
		"each_name": {httpHeaderEachName, 0}, "each_value": {httpHeaderEachValue, 0}, "chunked?": {httpHeaderChunked, 0},
		"basic_auth": {httpHeaderBasicAuth, 2}, "proxy_basic_auth": {httpHeaderProxyBasicAuth, 2},
		"content_length": {httpHeaderContentLength, 0}, "content_length=": {httpHeaderSetContentLength, 1},
		"content_type": {httpHeaderContentType, 0}, "content_type=": {httpHeaderSetContentType, -1},
		"set_content_type": {httpHeaderSetContentType, -1}, "main_type": {httpHeaderMainType, 0}, "sub_type": {httpHeaderSubType, 0},
		"range": {httpHeaderRange, 0}, "range=": {httpHeaderSetRange, -1}, "set_range": {httpHeaderSetRange, -1},
		"content_range": {httpHeaderContentRange, 0}, "range_length": {httpHeaderRangeLength, 0},
		"form_data=": {httpHeaderSetFormData, -1}, "set_form_data": {httpHeaderSetFormData, -1},
	} {
		module.DefineMethod(name, &object.Method{Name: name, Fn: definition.fn, Arity: definition.arity})
	}
}

func httpHeaderDataOf(receiver *object.EmeraldValue) *httpHeaderData {
	if request, ok := receiver.Data.(*httpRequestData); ok {
		return request.header
	}
	if response, ok := receiver.Data.(*httpResponseData); ok {
		return response.header
	}
	if data := httpHeaderStates[receiver]; data != nil {
		return data
	}
	data := newHTTPHeaderData()
	if variables := receiverInstanceVarMap(receiver); variables != nil {
		if stored := variables["@__net_http_header"]; stored != nil {
			if header, ok := stored.Data.(*httpHeaderData); ok {
				return header
			}
		}
		variables["@__net_http_header"] = &object.EmeraldValue{Type: object.ValueObject, Data: data, Class: R.Classes["Object"]}
	}
	httpHeaderStates[receiver] = data
	return data
}
func httpHeaderKeyName(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	raw, e := httpString(value)
	return strings.ToLower(raw), e
}
func canonicalHTTPHeader(name string) string {
	parts := strings.Split(strings.ToLower(name), "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "-")
}
func httpInitializeHeader(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return typeError("expected Hash")
	}
	data := httpHeaderDataOf(receiver)
	data.fields = make(map[string][]string)
	data.order = nil
	for _, hashValue := range args {
		if hashValue == nil || hashValue.Type != object.ValueHash {
			return typeError("expected Hash")
		}
		for key, value := range valueToHashMap(hashValue) {
			keyName, e := httpHeaderKeyName(key)
			if e != nil {
				return e
			}
			_, raw, e := cgiStringArg(value)
			if e != nil {
				return e
			}
			if _, exists := data.fields[keyName]; !exists {
				data.order = append(data.order, keyName)
			}
			data.fields[keyName] = []string{raw}
		}
	}
	return receiver
}
func httpHeaderGet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	key, e := httpHeaderKeyName(args[0])
	if e != nil {
		return e
	}
	values := httpHeaderDataOf(receiver).fields[key]
	if len(values) == 0 {
		return R.NilVal
	}
	return rubyString(strings.Join(values, ", "))
}
func httpHeaderSet(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	key, e := httpHeaderKeyName(args[0])
	if e != nil {
		return e
	}
	data := httpHeaderDataOf(receiver)
	if args[1] == nil || !args[1].IsTruthy() {
		delete(data.fields, key)
		return R.NilVal
	}
	_, raw, e := cgiStringArg(args[1])
	if e != nil {
		return e
	}
	if _, ok := data.fields[key]; !ok {
		data.order = append(data.order, key)
	}
	data.fields[key] = []string{raw}
	return args[1]
}
func httpHeaderAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	key, e := httpHeaderKeyName(args[0])
	if e != nil {
		return e
	}
	_, raw, e := cgiStringArg(args[1])
	if e != nil {
		return e
	}
	data := httpHeaderDataOf(receiver)
	if _, ok := data.fields[key]; !ok {
		data.order = append(data.order, key)
	}
	data.fields[key] = append(data.fields[key], raw)
	return args[1]
}
func httpHeaderGetFields(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	key, e := httpHeaderKeyName(args[0])
	if e != nil {
		return e
	}
	values := httpHeaderDataOf(receiver).fields[key]
	if len(values) == 0 {
		return R.NilVal
	}
	result := make([]*object.EmeraldValue, len(values))
	for i, v := range values {
		result[i] = rubyString(v)
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: R.Classes["Array"]}
}
func httpHeaderFetch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderGet(receiver, args[0])
	if value.Type != object.ValueNil {
		return value
	}
	if len(args) > 1 {
		return args[1]
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		key, errVal := httpHeaderKeyName(args[0])
		if errVal != nil {
			return errVal
		}
		return CallBlockWithArgs(CurrentBlockValue(), rubyString(key))
	}
	return newRuntimeException(R.Classes["KeyError"], "key not found")
}

var httpDigits = regexp.MustCompile(`[0-9]+`)

func httpHeaderContentLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderGet(receiver, rubyString("content-length"))
	if value.Type == object.ValueNil {
		return value
	}
	digits := httpDigits.FindString(stringRawValue(value))
	if digits == "" {
		return newRuntimeException(R.Classes["Net::HTTPHeaderSyntaxError"], "wrong Content-Length format")
	}
	n, _ := strconv.ParseInt(digits, 10, 64)
	return newInt(n)
}

func httpHeaderSetContentLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if args[0] == nil || !args[0].IsTruthy() {
		return httpHeaderSet(receiver, rubyString("content-length"), R.NilVal)
	}
	digits := httpDigits.FindString(valueStringForHTTP(args[0]))
	if digits == "" || !strings.HasPrefix(valueStringForHTTP(args[0]), digits) {
		digits = "0"
	}
	return httpHeaderSet(receiver, rubyString("content-length"), rubyString(digits))
}

func httpHeaderContentType(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderGet(receiver, rubyString("content-type"))
	if value.Type == object.ValueNil {
		return value
	}
	return rubyString(strings.TrimSpace(strings.SplitN(stringRawValue(value), ";", 2)[0]))
}

func httpHeaderMainType(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderContentType(receiver)
	if value.Type == object.ValueNil {
		return value
	}
	return rubyString(strings.SplitN(stringRawValue(value), "/", 2)[0])
}

func httpHeaderSubType(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderContentType(receiver)
	if value.Type == object.ValueNil {
		return value
	}
	parts := strings.SplitN(stringRawValue(value), "/", 2)
	if len(parts) != 2 {
		return R.NilVal
	}
	return rubyString(parts[1])
}

func httpHeaderSetContentType(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	typeName, errVal := httpString(args[0])
	if errVal != nil {
		return errVal
	}
	parts := []string{typeName}
	if len(args) > 1 && args[1] != nil && args[1].Type == object.ValueHash {
		hashForEach(args[1], func(key, value *object.EmeraldValue) {
			parts = append(parts, valueStringForHTTP(key)+"="+valueStringForHTTP(value))
		})
	}
	return httpHeaderSet(receiver, rubyString("content-type"), rubyString(strings.Join(parts, "; ")))
}

var httpRangePattern = regexp.MustCompile(`^bytes=([0-9]*)-([0-9]*)(?:,\s*|$)`)
var httpContentRangePattern = regexp.MustCompile(`^bytes\s+([0-9]+)-([0-9]+)/(?:[0-9]+|\*)$`)

func httpHeaderSyntax(message string) *object.EmeraldValue {
	return newRuntimeException(R.Classes["Net::HTTPHeaderSyntaxError"], message)
}
func httpRangeValue(start, end int64) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueRange, Data: &object.RRange{Start: start, End: end, StartValue: start, EndValue: end}, Class: R.Classes["Range"]}
}
func httpHeaderRange(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderGet(receiver, rubyString("range"))
	if value.Type == object.ValueNil {
		return value
	}
	raw := stringRawValue(value)
	if !strings.HasPrefix(raw, "bytes=") {
		return httpHeaderSyntax("wrong Range format")
	}
	rest := raw
	result := []*object.EmeraldValue{}
	for rest != "" {
		match := httpRangePattern.FindStringSubmatch(rest)
		if match == nil || match[1] == "" && match[2] == "" {
			return httpHeaderSyntax("range is not specified")
		}
		var start, end int64
		if match[1] == "" {
			n, _ := strconv.ParseInt(match[2], 10, 64)
			start = -n
			end = -1
		} else {
			start, _ = strconv.ParseInt(match[1], 10, 64)
			if match[2] == "" {
				end = -1
			} else {
				end, _ = strconv.ParseInt(match[2], 10, 64)
			}
		}
		result = append(result, httpRangeValue(start, end))
		rest = rest[len(match[0]):]
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: result, Class: R.Classes["Array"]}
}
func httpNumericInt(value *object.EmeraldValue) (int64, bool) {
	if n, ok := valueToInteger(value); ok {
		return n, true
	}
	if value != nil && value.Type == object.ValueFloat {
		if f, ok := value.Data.(float64); ok {
			return int64(f), true
		}
	}
	return 0, false
}
func httpHeaderSetRange(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 {
		return NewArgumentError("wrong number of arguments")
	}
	if args[0] == nil || args[0].Type == object.ValueNil {
		return httpHeaderSet(receiver, rubyString("range"), R.NilVal)
	}
	var header string
	if len(args) == 2 {
		start, ok1 := httpNumericInt(args[0])
		length, ok2 := httpNumericInt(args[1])
		if !ok1 || !ok2 || start < 0 || length < 0 || start+length < 0 {
			return httpHeaderSyntax("invalid range")
		}
		header = fmt.Sprintf("bytes=%d-%d", start, start+length-1)
	} else if args[0].Type == object.ValueRange {
		r, _ := args[0].Data.(*object.RRange)
		start, end := r.Start, r.End
		if r.Exclusive {
			end--
		}
		if start < 0 && end == -1 {
			header = fmt.Sprintf("bytes=%d", start)
		} else if start < 0 || end < -1 || end >= 0 && end < start {
			return httpHeaderSyntax("invalid range")
		} else if end == -1 {
			header = fmt.Sprintf("bytes=%d-", start)
		} else {
			header = fmt.Sprintf("bytes=%d-%d", start, end)
		}
	} else {
		length, ok := httpNumericInt(args[0])
		if !ok {
			return typeError("no implicit conversion into Integer")
		}
		if length < 0 {
			header = fmt.Sprintf("bytes=%d", length)
		} else {
			header = fmt.Sprintf("bytes=0-%d", length-1)
		}
	}
	return httpHeaderSet(receiver, rubyString("range"), rubyString(header))
}
func httpHeaderContentRange(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderGet(receiver, rubyString("content-range"))
	if value.Type == object.ValueNil {
		return value
	}
	m := httpContentRangePattern.FindStringSubmatch(stringRawValue(value))
	if m == nil {
		return httpHeaderSyntax("wrong Content-Range format")
	}
	start, _ := strconv.ParseInt(m[1], 10, 64)
	end, _ := strconv.ParseInt(m[2], 10, 64)
	return httpRangeValue(start, end)
}
func httpHeaderRangeLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderContentRange(receiver)
	if value.Type == object.ValueNil || value.Type == object.ValueException {
		return value
	}
	r, _ := value.Data.(*object.RRange)
	return newInt(r.End - r.Start + 1)
}
func httpHeaderSetFormData(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 {
		return NewArgumentError("wrong number of arguments")
	}
	separator := "&"
	hashArgs := args
	if args[len(args)-1].Type != object.ValueHash {
		value, e := httpString(args[len(args)-1])
		if e != nil {
			return e
		}
		separator = value
		hashArgs = args[:len(args)-1]
	}
	parts := []string{}
	for _, hashValue := range hashArgs {
		if hashValue.Type != object.ValueHash {
			return typeError("expected Hash")
		}
		hashForEach(hashValue, func(k, v *object.EmeraldValue) {
			parts = append(parts, url.QueryEscape(valueStringForHTTP(k))+"="+url.QueryEscape(valueStringForHTTP(v)))
		})
	}
	body := rubyString(strings.Join(parts, separator))
	if CallMethod != nil {
		if result := CallMethod(receiver, "body=", body); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	httpHeaderSet(receiver, rubyString("content-type"), rubyString("application/x-www-form-urlencoded"))
	return body
}
func httpHeaderDelete(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderGetFields(receiver, args[0])
	key, _ := httpHeaderKeyName(args[0])
	delete(httpHeaderDataOf(receiver).fields, key)
	return value
}
func httpHeaderKey(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	key, e := httpHeaderKeyName(args[0])
	if e != nil {
		return e
	}
	_, ok := httpHeaderDataOf(receiver).fields[key]
	return boolValue(ok)
}
func httpHeaderLength(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(int64(len(httpHeaderDataOf(receiver).fields)))
}
func httpHeaderToHash(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := emptyHashValue()
	hash := result.Data.(*object.RHash)
	data := httpHeaderDataOf(receiver)
	keys := append([]string(nil), data.order...)
	for _, key := range keys {
		values := data.fields[key]
		array := make([]*object.EmeraldValue, len(values))
		for i, v := range values {
			array[i] = rubyString(v)
		}
		k := rubyString(key)
		hash.Keys = append(hash.Keys, k)
		hash.Pairs[k] = &object.EmeraldValue{Type: object.ValueArray, Data: array, Class: R.Classes["Array"]}
	}
	return result
}
func httpHeaderEach(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return objectToEnum(receiver, rubyString("each_header"))
	}
	data := httpHeaderDataOf(receiver)
	for _, key := range data.order {
		if _, ok := data.fields[key]; !ok {
			continue
		}
		if result := CallBlockWithArgs(CurrentBlockValue(), rubyString(key), rubyString(strings.Join(data.fields[key], ", "))); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func httpHeaderEachCapitalized(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return objectToEnum(receiver, rubyString("each_capitalized"))
	}
	data := httpHeaderDataOf(receiver)
	for _, key := range data.order {
		values, ok := data.fields[key]
		if !ok {
			continue
		}
		if result := CallBlockWithArgs(CurrentBlockValue(), rubyString(canonicalHTTPHeader(key)), rubyString(strings.Join(values, ", "))); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func httpHeaderEachCapitalizedName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return objectToEnum(receiver, rubyString("each_capitalized_name"))
	}
	data := httpHeaderDataOf(receiver)
	for _, key := range data.order {
		if _, ok := data.fields[key]; !ok {
			continue
		}
		if result := CallBlockWithArgs(CurrentBlockValue(), rubyString(canonicalHTTPHeader(key))); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func httpHeaderEachName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return objectToEnum(receiver, rubyString("each_name"))
	}
	data := httpHeaderDataOf(receiver)
	for _, key := range data.order {
		if _, ok := data.fields[key]; !ok {
			continue
		}
		if result := CallBlockWithArgs(CurrentBlockValue(), rubyString(key)); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func httpHeaderEachValue(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return objectToEnum(receiver, rubyString("each_value"))
	}
	data := httpHeaderDataOf(receiver)
	for _, key := range data.order {
		values, ok := data.fields[key]
		if !ok {
			continue
		}
		if result := CallBlockWithArgs(CurrentBlockValue(), rubyString(strings.Join(values, ", "))); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	return receiver
}

func httpHeaderChunked(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	value := httpHeaderGet(receiver, rubyString("transfer-encoding"))
	return boolValue(value != nil && value.Type == object.ValueString && strings.EqualFold(strings.TrimSpace(stringRawValue(value)), "chunked"))
}
func httpHeaderBasicAuth(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return httpHeaderAuth(receiver, "authorization", args)
}
func httpHeaderProxyBasicAuth(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return httpHeaderAuth(receiver, "proxy-authorization", args)
}
func httpHeaderAuth(receiver *object.EmeraldValue, name string, args []*object.EmeraldValue) *object.EmeraldValue {
	user, e := httpString(args[0])
	if e != nil {
		return e
	}
	password, e := httpString(args[1])
	if e != nil {
		return e
	}
	return httpHeaderSet(receiver, rubyString(name), rubyString("Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+password))))
}

func httpRequestExec(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := httpRequestDataOf(receiver)
	version, e := httpString(args[1])
	if e != nil {
		return e
	}
	requestPath, e := httpString(args[2])
	if e != nil {
		return e
	}
	var out strings.Builder
	out.WriteString(data.method + " " + requestPath + " HTTP/" + version + "\r\n")
	if len(data.header.fields["accept"]) == 0 {
		data.header.fields["accept"] = []string{"*/*"}
		data.header.order = append(data.header.order, "accept")
	}
	body := ""
	if data.body != nil && data.body.Type != object.ValueNil {
		_, body, e = cgiStringArg(data.body)
		if e != nil {
			return e
		}
		if len(data.header.fields["content-length"]) == 0 {
			data.header.fields["content-length"] = []string{strconv.Itoa(len(body))}
			data.header.order = append(data.header.order, "content-length")
		}
	} else if data.bodyStream != nil && data.bodyStream.Type != object.ValueNil {
		if len(data.header.fields["content-length"]) == 0 && strings.ToLower(strings.Join(data.header.fields["transfer-encoding"], ",")) != "chunked" {
			return NewArgumentError("Content-Length not given and Transfer-Encoding is not `chunked'")
		}
		if CallMethod != nil {
			value := CallMethod(data.bodyStream, "read")
			if value != nil && value.Type == object.ValueString {
				body = stringRawValue(value)
			}
		}
	}
	for _, key := range data.header.order {
		values := data.header.fields[key]
		if len(values) > 0 {
			out.WriteString(canonicalHTTPHeader(key) + ": " + strings.Join(values, ", ") + "\r\n")
		}
	}
	out.WriteString("\r\n")
	if strings.ToLower(strings.Join(data.header.fields["transfer-encoding"], ",")) == "chunked" && body != "" {
		out.WriteString(fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(body), body))
	} else {
		out.WriteString(body)
	}
	return httpBufferedWrite(args[0], rubyString(out.String()))
}

func installBufferedIO(netModule *object.Module, objectClass *object.Class) {
	klass := object.NewClass("Net::BufferedIO")
	klass.SuperClass = objectClass
	klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: httpBufferedIONew, Arity: -1})
	klass.DefineMethod("write", &object.Method{Name: "write", Fn: httpBufferedWrite, Arity: 1})
	klass.DefineMethod("read", &object.Method{Name: "read", Fn: httpBufferedRead, Arity: -1})
	R.Classes["Net::BufferedIO"] = klass
	netModule.Constants["BufferedIO"] = classEmeraldValue(klass)
}
func httpBufferedIONew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &bufferedIOData{io: args[0]}, Class: klass}
}
func httpBufferedWrite(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data, ok := receiver.Data.(*bufferedIOData); ok && CallMethod != nil {
		return CallMethod(data.io, "write", args[0])
	}
	if CallMethod != nil {
		return CallMethod(receiver, "write", args[0])
	}
	return R.NilVal
}
func httpBufferedRead(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if data, ok := receiver.Data.(*bufferedIOData); ok && CallMethod != nil {
		return CallMethod(data.io, "read", args...)
	}
	return R.NilVal
}

func installHTTPResponseClasses(netModule *object.Module, objectClass *object.Class, headerModule *object.Module) {
	base := object.NewClass("Net::HTTPResponse")
	base.SuperClass = objectClass
	base.Include(headerModule)
	base.DefineClassMethod("new", &object.Method{Name: "new", Fn: httpResponseNew, Arity: 3})
	base.DefineClassMethod("read_new", &object.Method{Name: "read_new", Fn: httpResponseReadNew, Arity: 1})
	for name, def := range map[string]struct {
		fn    interface{}
		arity int
	}{"http_version": {httpResponseVersion, 0}, "code": {httpResponseCode, 0}, "message": {httpResponseMessage, 0}, "msg": {httpResponseMessage, 0}, "body": {httpResponseBody, 0}, "entity": {httpResponseBody, 0}, "response": {httpResponseSelf, 0}, "header": {httpResponseSelf, 0}, "inspect": {httpResponseInspect, 0}, "value": {httpResponseValue, 0}, "error!": {httpResponseErrorBang, 0}, "code_type": {httpResponseCodeType, 0}, "error_type": {httpResponseErrorType, 0}, "reading_body": {httpResponseReadingBody, 2}, "read_body": {httpResponseReadBody, -1}} {
		base.DefineMethod(name, &object.Method{Name: name, Fn: def.fn, Arity: def.arity})
	}
	R.Classes["Net::HTTPResponse"] = base
	netModule.Constants["HTTPResponse"] = classEmeraldValue(base)
	categories := []struct {
		name  string
		super *object.Class
	}{{"HTTPUnknownResponse", base}, {"HTTPInformation", base}, {"HTTPSuccess", base}, {"HTTPRedirection", base}, {"HTTPClientError", base}, {"HTTPServerError", base}}
	for _, entry := range categories {
		klass := object.NewClass("Net::" + entry.name)
		klass.SuperClass = entry.super
		klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: httpResponseNew, Arity: 3})
		R.Classes[klass.Name] = klass
		netModule.Constants[entry.name] = classEmeraldValue(klass)
		exceptionClass := R.Classes["Net::HTTPError"]
		switch entry.name {
		case "HTTPRedirection":
			exceptionClass = R.Classes["Net::HTTPRetriableError"]
		case "HTTPClientError":
			exceptionClass = R.Classes["Net::HTTPClientException"]
		case "HTTPServerError":
			exceptionClass = R.Classes["Net::HTTPFatalError"]
		}
		klass.DefineConstant("EXCEPTION_TYPE", classEmeraldValue(exceptionClass))
		klass.DefineClassMethod("exception_type", &object.Method{Name: "exception_type", Fn: httpResponseClassExceptionType, Arity: 0})
	}
	okClass := object.NewClass("Net::HTTPOK")
	okClass.SuperClass = R.Classes["Net::HTTPSuccess"]
	okClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: httpResponseNew, Arity: 3})
	R.Classes["Net::HTTPOK"] = okClass
	netModule.Constants["HTTPOK"] = classEmeraldValue(okClass)
	adapter := object.NewClass("Net::ReadAdapter")
	adapter.SuperClass = objectClass
	R.Classes["Net::ReadAdapter"] = adapter
	netModule.Constants["ReadAdapter"] = classEmeraldValue(adapter)
}
func httpResponseNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	version, e := httpString(args[0])
	if e != nil {
		return e
	}
	code, e := httpString(args[1])
	if e != nil {
		return e
	}
	message, e := httpString(args[2])
	if e != nil {
		return e
	}
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &httpResponseData{header: newHTTPHeaderData(), httpVersion: version, code: code, message: message}, Class: klass}
}
func httpResponseDataOf(r *object.EmeraldValue) *httpResponseData {
	d, _ := r.Data.(*httpResponseData)
	return d
}
func httpResponseVersion(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(httpResponseDataOf(r).httpVersion)
}
func httpResponseCode(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(httpResponseDataOf(r).code)
}
func httpResponseMessage(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return rubyString(httpResponseDataOf(r).message)
}
func httpResponseBody(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpResponseDataOf(r)
	if d.body != nil {
		return d.body
	}
	if d.bodyContext && d.bodyAllowed && (d.bodySocket != nil || d.pendingBody != "") {
		return httpResponseConsumeBody(r)
	}
	return R.NilVal
}
func httpResponseSelf(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return r
}
func httpResponseCodeType(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	return classEmeraldValue(r.Class)
}
func httpResponseErrorType(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	if value, ok := r.Class.GetConstant("EXCEPTION_TYPE"); ok {
		return value
	}
	return classEmeraldValue(R.Classes["Net::HTTPError"])
}
func httpResponseClassExceptionType(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	if klass, ok := r.Data.(*object.Class); ok {
		if value, found := klass.GetConstant("EXCEPTION_TYPE"); found {
			return value
		}
	}
	return classEmeraldValue(R.Classes["Net::HTTPError"])
}
func httpResponseReadingBody(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpResponseDataOf(r)
	d.bodySocket = args[0]
	d.bodyContext = true
	d.bodyAllowed = args[1].IsTruthy() && r.Class != R.Classes["Net::HTTPInformation"]
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		if yielded := CallBlockWithArgs(CurrentBlockValue()); yielded != nil && yielded.Type == object.ValueException {
			return yielded
		}
	}
	if !d.bodyAllowed {
		return R.NilVal
	}
	if d.body == nil {
		return httpResponseConsumeBody(r)
	}
	return d.body
}
func httpResponseConsumeBody(r *object.EmeraldValue) *object.EmeraldValue {
	d := httpResponseDataOf(r)
	if d.readBody {
		if d.body == nil {
			return R.NilVal
		}
		return d.body
	}
	body := d.pendingBody
	if body == "" && d.bodySocket != nil && CallMethod != nil {
		value := CallMethod(d.bodySocket, "read")
		if value != nil && value.Type == object.ValueException {
			return value
		}
		if value != nil && value.Type == object.ValueString {
			body = stringRawValue(value)
		}
	}
	d.pendingBody = ""
	d.readBody = true
	d.body = rubyString(body)
	return d.body
}
func httpResponseReadBody(r *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpResponseDataOf(r)
	hasBlock := BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil
	if len(args) > 0 && hasBlock {
		return NewArgumentError("both arg and block given for HTTP method")
	}
	if d.readBody {
		if len(args) > 0 || hasBlock {
			return newRuntimeException(R.Classes["IOError"], "read_body called twice")
		}
		if d.body == nil {
			return R.NilVal
		}
		return d.body
	}
	value := httpResponseConsumeBody(r)
	if value.Type == object.ValueException {
		return value
	}
	body := stringRawValue(value)
	if len(args) > 0 {
		if args[0].Type != object.ValueString {
			return typeError("no implicit conversion into String")
		}
		args[0].Data = stringRawValue(args[0]) + body
		d.body = args[0]
		return args[0]
	}
	if hasBlock {
		if result := CallBlockWithArgs(CurrentBlockValue(), rubyString(body)); result != nil && result.Type == object.ValueException {
			return result
		}
		return &object.EmeraldValue{Type: object.ValueObject, Data: object.NewObject(R.Classes["Net::ReadAdapter"]), Class: R.Classes["Net::ReadAdapter"]}
	}
	return d.body
}
func httpResponseReadNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if CallMethod == nil {
		return R.NilVal
	}
	value := CallMethod(args[0], "read")
	if value == nil || value.Type != object.ValueString {
		return newRuntimeException(R.Classes["Net::HTTPBadResponse"], "wrong status line")
	}
	raw := stringRawValue(value)
	head, body, ok := strings.Cut(raw, "\n\n")
	separator := "\n"
	if !ok {
		head, body, ok = strings.Cut(raw, "\r\n\r\n")
		separator = "\r\n"
	}
	if !ok {
		return newRuntimeException(R.Classes["Net::HTTPBadResponse"], "wrong header")
	}
	lines := strings.Split(head, separator)
	status := strings.Fields(strings.TrimSpace(lines[0]))
	if len(status) < 3 || !strings.HasPrefix(status[0], "HTTP/") {
		return newRuntimeException(R.Classes["Net::HTTPBadResponse"], "wrong status line")
	}
	klass := R.Classes["Net::HTTPResponse"]
	if status[1] == "200" {
		klass = R.Classes["Net::HTTPOK"]
	}
	response := &object.EmeraldValue{Type: object.ValueObject, Data: &httpResponseData{header: newHTTPHeaderData(), httpVersion: strings.TrimPrefix(status[0], "HTTP/"), code: status[1], message: strings.Join(status[2:], " "), pendingBody: body}, Class: klass}
	for _, line := range lines[1:] {
		if name, val, found := strings.Cut(line, ":"); found {
			httpHeaderSet(response, rubyString(strings.TrimSpace(name)), rubyString(strings.TrimSpace(val)))
		}
	}
	return response
}
func httpResponseInspect(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpResponseDataOf(r)
	return rubyString(fmt.Sprintf("#<%s %s %s readbody=%v>", r.Class.Name, d.code, d.message, d.readBody))
}
func httpResponseValue(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpResponseDataOf(r)
	if strings.HasPrefix(d.code, "2") {
		return R.NilVal
	}
	klass := R.Classes["Net::HTTPError"]
	if strings.HasPrefix(d.code, "3") {
		klass = R.Classes["Net::HTTPRetriableError"]
	} else if strings.HasPrefix(d.code, "4") {
		klass = R.Classes["Net::HTTPClientException"]
	} else if strings.HasPrefix(d.code, "5") {
		klass = R.Classes["Net::HTTPFatalError"]
	}
	value := newRuntimeException(klass, d.code+" "+d.message)
	if exc, ok := value.Data.(*object.RException); ok {
		exc.InstanceVars = map[string]*object.EmeraldValue{"@response": r}
	}
	return value
}
func httpResponseErrorBang(r *object.EmeraldValue, a ...*object.EmeraldValue) *object.EmeraldValue {
	d := httpResponseDataOf(r)
	klassValue := httpResponseErrorType(r)
	klass, _ := klassValue.Data.(*object.Class)
	value := newRuntimeException(klass, d.code+" "+d.message)
	if exc, ok := value.Data.(*object.RException); ok {
		exc.InstanceVars = map[string]*object.EmeraldValue{"@response": r}
	}
	return value
}
