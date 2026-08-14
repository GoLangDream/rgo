package core

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

func installOpenSSLModule(objectClass *object.Class) {
	if objectClass == nil {
		return
	}
	if existing := objectClass.Constants["OpenSSL"]; existing != nil && existing.Type == object.ValueModule {
		return
	}

	openssl := object.NewModule("OpenSSL")
	openssl.DefineMethod("fixed_length_secure_compare", &object.Method{Name: "fixed_length_secure_compare", Fn: opensslFixedLengthSecureCompare, Arity: 2})
	openssl.DefineMethod("secure_compare", &object.Method{Name: "secure_compare", Fn: opensslSecureCompare, Arity: 2})
	openssl.Constants["VERSION"] = rubyString("4.0.0")
	openssl.Constants["OPENSSL_VERSION"] = rubyString("OpenSSL 3.0.0")
	openssl.Constants["OPENSSL_VERSION_NUMBER"] = newInt(0x30000000)

	randomModule := object.NewModule("OpenSSL::Random")
	randomModule.DefineMethod("random_bytes", &object.Method{Name: "random_bytes", Fn: opensslRandomBytes, Arity: -1})
	randomModule.DefineMethod("pseudo_bytes", &object.Method{Name: "pseudo_bytes", Fn: opensslRandomBytes, Arity: -1})
	openssl.Constants["Random"] = &object.EmeraldValue{Type: object.ValueModule, Data: randomModule, Class: R.Classes["Module"]}
	cipherClass := object.NewClass("OpenSSL::Cipher")
	cipherClass.SuperClass = R.Classes["Object"]
	cipherClass.DefineClassMethod("ciphers", &object.Method{Name: "ciphers", Fn: opensslCipherCiphers, Arity: 0})
	cipherErrorClass := object.NewClass("OpenSSL::Cipher::CipherError")
	cipherErrorClass.SuperClass = R.Classes["StandardError"]
	cipherClass.DefineConstant("CipherError", classEmeraldValue(cipherErrorClass))
	openssl.Constants["Cipher"] = classEmeraldValue(cipherClass)
	R.Classes["OpenSSL::Cipher"] = cipherClass
	R.Classes["OpenSSL::Cipher::CipherError"] = cipherErrorClass
	installOpenSSLSSL(openssl)
	installOpenSSLDigest(openssl)
	installOpenSSLKDF(openssl)
	installOpenSSLX509(openssl)

	value := &object.EmeraldValue{Type: object.ValueModule, Data: openssl, Class: R.Classes["Module"]}
	objectClass.DefineConstant("OpenSSL", value)
	AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "OpenSSL", value)
}

func opensslCipherCiphers(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
}

func installOpenSSLSSL(openssl *object.Module) {
	sslModule := object.NewModule("OpenSSL::SSL")
	for name, mode := range map[string]int64{
		"VERIFY_NONE":                 0x00,
		"VERIFY_PEER":                 0x01,
		"VERIFY_FAIL_IF_NO_PEER_CERT": 0x02,
		"VERIFY_CLIENT_ONCE":          0x04,
		"VERIFY_POST_HANDSHAKE":       0x08,
	} {
		sslModule.Constants[name] = NewIntegerValue(mode)
	}
	sslErrorClass := object.NewClass("OpenSSL::SSL::SSLError")
	sslErrorClass.SuperClass = R.Classes["StandardError"]
	sslModule.Constants["SSLError"] = classEmeraldValue(sslErrorClass)
	R.Classes["OpenSSL::SSL::SSLError"] = sslErrorClass
	for _, name := range []string{"SSLErrorWaitReadable", "SSLErrorWaitWritable"} {
		klass := object.NewClass("OpenSSL::SSL::" + name)
		klass.SuperClass = sslErrorClass
		sslModule.Constants[name] = classEmeraldValue(klass)
		R.Classes[klass.Name] = klass
	}
	contextClass := object.NewClass("OpenSSL::SSL::SSLContext")
	contextClass.SuperClass = R.Classes["Object"]
	contextClass.DefineMethod("set_params", &object.Method{Name: "set_params", Fn: opensslSSLContextSetParams, Arity: -1})
	contextValue := classEmeraldValue(contextClass)
	sslModule.Constants["SSLContext"] = contextValue
	socketClass := object.NewClass("OpenSSL::SSL::SSLSocket")
	socketClass.SuperClass = R.Classes["Object"]
	sslModule.Constants["SSLSocket"] = classEmeraldValue(socketClass)
	R.Classes["OpenSSL::SSL::SSLSocket"] = socketClass
	openssl.Constants["SSL"] = &object.EmeraldValue{Type: object.ValueModule, Data: sslModule, Class: R.Classes["Module"]}
	R.Classes["OpenSSL::SSL::SSLContext"] = contextClass
}

func opensslSSLContextSetParams(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..1)", len(args)))
	}
	if len(args) == 1 {
		receiverInstanceVarMap(receiver)["@params"] = args[0]
	}
	return receiver
}

func installOpenSSLDigest(openssl *object.Module) {
	digestClass := object.NewClass("OpenSSL::Digest")
	digestClass.SuperClass = R.Classes["Object"]
	digestClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: opensslDigestClassNew, Arity: -1})
	digestClass.DefineClassMethod("digest", &object.Method{Name: "digest", Fn: opensslDigestClassDigest, Arity: -1})
	digestClass.DefineClassMethod("hexdigest", &object.Method{Name: "hexdigest", Fn: opensslDigestClassHexdigest, Arity: -1})
	digestClass.DefineClassMethod("base64digest", &object.Method{Name: "base64digest", Fn: opensslDigestClassBase64digest, Arity: -1})
	defineOpenSSLDigestInstanceMethods(digestClass)

	digestError := object.NewClass("OpenSSL::Digest::DigestError")
	digestError.SuperClass = R.Classes["StandardError"]
	digestClass.DefineConstant("DigestError", &object.EmeraldValue{Type: object.ValueClass, Data: digestError, Class: R.Classes["Class"]})

	for i := range digestAlgorithms {
		spec := &digestAlgorithms[i]
		klass := object.NewClass("OpenSSL::Digest::" + spec.name)
		klass.SuperClass = digestClass
		klass.SetInstanceVar("@__rgo_digest_spec", &object.EmeraldValue{Type: object.ValueObject, Data: spec, Class: R.Classes["Object"]})
		klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: opensslNamedDigestClassNew, Arity: -1})
		defineOpenSSLDigestInstanceMethods(klass)
		digestClass.DefineConstant(spec.name, &object.EmeraldValue{Type: object.ValueClass, Data: klass, Class: R.Classes["Class"]})
	}
	openssl.Constants["Digest"] = &object.EmeraldValue{Type: object.ValueClass, Data: digestClass, Class: R.Classes["Class"]}

	hmacModule := object.NewModule("OpenSSL::HMAC")
	hmacModule.DefineMethod("digest", &object.Method{Name: "digest", Fn: opensslHMACDigest, Arity: 3})
	hmacModule.DefineMethod("hexdigest", &object.Method{Name: "hexdigest", Fn: opensslHMACHexdigest, Arity: 3})
	openssl.Constants["HMAC"] = &object.EmeraldValue{Type: object.ValueModule, Data: hmacModule, Class: R.Classes["Module"]}
}

func defineOpenSSLDigestInstanceMethods(klass *object.Class) {
	klass.DefineMethod("update", &object.Method{Name: "update", Fn: digestAlgorithmUpdate, Arity: 1})
	klass.DefineMethod("<<", &object.Method{Name: "<<", Fn: digestAlgorithmUpdate, Arity: 1})
	klass.DefineMethod("reset", &object.Method{Name: "reset", Fn: digestAlgorithmReset, Arity: 0})
	klass.DefineMethod("digest", &object.Method{Name: "digest", Fn: digestAlgorithmDigest, Arity: -1})
	klass.DefineMethod("hexdigest", &object.Method{Name: "hexdigest", Fn: digestAlgorithmHexdigest, Arity: -1})
	klass.DefineMethod("digest_length", &object.Method{Name: "digest_length", Fn: digestAlgorithmLength, Arity: 0})
	klass.DefineMethod("block_length", &object.Method{Name: "block_length", Fn: digestAlgorithmBlockLength, Arity: 0})
	klass.DefineMethod("name", &object.Method{Name: "name", Fn: opensslDigestName, Arity: 0})
}

func opensslDigestSpecByName(raw string) *digestAlgorithmSpec {
	name := strings.ToUpper(strings.ReplaceAll(raw, "-", ""))
	for i := range digestAlgorithms {
		if digestAlgorithms[i].name == name {
			return &digestAlgorithms[i]
		}
	}
	return nil
}

func opensslDigestError(message string) *object.EmeraldValue {
	class := R.Classes["StandardError"]
	if opensslValue := R.Classes["Object"].Constants["OpenSSL"]; opensslValue != nil && opensslValue.Type == object.ValueModule {
		if openssl, ok := opensslValue.Data.(*object.Module); ok {
			if digestValue := openssl.Constants["Digest"]; digestValue != nil && digestValue.Type == object.ValueClass {
				if digestClass, ok := digestValue.Data.(*object.Class); ok {
					if errorValue := digestClass.Constants["DigestError"]; errorValue != nil && errorValue.Type == object.ValueClass {
						class, _ = errorValue.Data.(*object.Class)
					}
				}
			}
		}
	}
	return newRuntimeException(class, message)
}

func opensslDigestSpecArgument(value *object.EmeraldValue) (*digestAlgorithmSpec, *object.EmeraldValue) {
	if state := digestAlgorithmState(value); state != nil {
		return state.spec, nil
	}
	raw, errVal := opensslStringArgument(value)
	if errVal != nil {
		return nil, errVal
	}
	spec := opensslDigestSpecByName(raw)
	if spec == nil {
		return nil, opensslDigestError("Unsupported digest algorithm (" + raw + ")")
	}
	return spec, nil
}

func opensslDigestClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1..2)", len(args)))
	}
	spec, errVal := opensslDigestSpecArgument(args[0])
	if errVal != nil {
		return errVal
	}
	var initial []byte
	if len(args) == 2 {
		raw, stringErr := opensslStringArgument(args[1])
		if stringErr != nil {
			return stringErr
		}
		initial = []byte(raw)
	}
	klass, _ := receiver.Data.(*object.Class)
	return digestAlgorithmNewValueForClass(spec, klass, initial)
}

func opensslNamedDigestClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..1)", len(args)))
	}
	klass, _ := receiver.Data.(*object.Class)
	spec := digestAlgorithmSpecFromClass(klass)
	var initial []byte
	if len(args) == 1 {
		raw, errVal := opensslStringArgument(args[0])
		if errVal != nil {
			return errVal
		}
		initial = []byte(raw)
	}
	return digestAlgorithmNewValueForClass(spec, klass, initial)
}

func opensslDigestName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	state := digestAlgorithmState(receiver)
	if state == nil || state.spec == nil {
		return R.NilVal
	}
	return stringWithEncoding(state.spec.name, "US-ASCII")
}

func opensslDigestClassResult(receiver *object.EmeraldValue, args []*object.EmeraldValue, mode string) *object.EmeraldValue {
	var spec *digestAlgorithmSpec
	var dataArg *object.EmeraldValue
	if klass, ok := receiver.Data.(*object.Class); ok {
		spec = digestAlgorithmSpecFromClass(klass)
	}
	if spec != nil {
		if len(args) != 1 {
			return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1)", len(args)))
		}
		dataArg = args[0]
	} else {
		if len(args) != 2 {
			return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 2)", len(args)))
		}
		var errVal *object.EmeraldValue
		spec, errVal = opensslDigestSpecArgument(args[0])
		if errVal != nil {
			return errVal
		}
		dataArg = args[1]
	}
	raw, stringErr := opensslStringArgument(dataArg)
	if stringErr != nil {
		return stringErr
	}
	sum := digestAlgorithmSum(spec, []byte(raw))
	switch mode {
	case "hex":
		return stringWithEncoding(hex.EncodeToString(sum), "US-ASCII")
	case "base64":
		return stringWithEncoding(base64.StdEncoding.EncodeToString(sum), "US-ASCII")
	default:
		return stringWithEncoding(string(sum), "ASCII-8BIT")
	}
}

func opensslDigestClassDigest(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return opensslDigestClassResult(receiver, args, "raw")
}

func opensslDigestClassHexdigest(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return opensslDigestClassResult(receiver, args, "hex")
}

func opensslDigestClassBase64digest(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return opensslDigestClassResult(receiver, args, "base64")
}

func opensslHMACResult(args []*object.EmeraldValue, asHex bool) *object.EmeraldValue {
	if len(args) != 3 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 3)", len(args)))
	}
	spec, errVal := opensslDigestSpecArgument(args[0])
	if errVal != nil {
		return errVal
	}
	key, keyErr := opensslStringArgument(args[1])
	if keyErr != nil {
		return keyErr
	}
	data, dataErr := opensslStringArgument(args[2])
	if dataErr != nil {
		return dataErr
	}
	mac := hmac.New(spec.newHash, []byte(key))
	_, _ = mac.Write([]byte(data))
	sum := mac.Sum(nil)
	if asHex {
		return stringWithEncoding(hex.EncodeToString(sum), "US-ASCII")
	}
	return stringWithEncoding(string(sum), "ASCII-8BIT")
}

func opensslHMACDigest(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return opensslHMACResult(args, false)
}

func opensslHMACHexdigest(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return opensslHMACResult(args, true)
}

func opensslRandomBytes(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..1)", len(args)))
	}
	length := int64(16)
	if len(args) == 1 {
		integer, errVal := randomIntegerArgument(args[0])
		if errVal != nil {
			return errVal
		}
		if !integer.IsInt64() {
			return NewArgumentError("string size too big")
		}
		length = integer.Int64()
	}
	if length < 0 {
		return NewArgumentError("negative string size")
	}
	if uint64(length) > uint64(^uint(0)>>1) {
		return NewArgumentError("string size too big")
	}
	buf := make([]byte, int(length))
	if _, err := rand.Read(buf); err != nil {
		return NewRuntimeError(err.Error())
	}
	return stringWithEncoding(string(buf), "ASCII-8BIT")
}

func opensslStringArgument(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	raw, ok, viaToStr, errVal := evalCoerceToString(value)
	if errVal != nil {
		return "", errVal
	}
	if !ok {
		return "", conversionTypeErrorToStringForMode(value, viaToStr)
	}
	return raw, nil
}

func opensslFixedLengthSecureCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 2)", len(args)))
	}
	left, errVal := opensslStringArgument(args[0])
	if errVal != nil {
		return errVal
	}
	right, errVal := opensslStringArgument(args[1])
	if errVal != nil {
		return errVal
	}
	if len(left) != len(right) {
		return NewArgumentError("inputs must be of equal length")
	}
	return boolValue(subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1)
}

func opensslSecureCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 2)", len(args)))
	}
	left, errVal := opensslStringArgument(args[0])
	if errVal != nil {
		return errVal
	}
	right, errVal := opensslStringArgument(args[1])
	if errVal != nil {
		return errVal
	}
	leftHash := sha256.Sum256([]byte(left))
	rightHash := sha256.Sum256([]byte(right))
	if subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) != 1 {
		return R.FalseVal
	}
	if CallMethod == nil {
		return boolValue(args[0] == args[1] || left == right)
	}
	equal := CallMethod(args[0], "==", args[1])
	if equal != nil && equal.Type == object.ValueException {
		return equal
	}
	return boolValue(equal == R.TrueVal)
}
