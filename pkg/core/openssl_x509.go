package core

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GoLangDream/rgo/pkg/object"
)

const (
	opensslASN1UTF8String = int64(12)
	opensslASN1IA5String  = int64(22)
)

type opensslX509NameEntry struct {
	key      string
	value    string
	asn1Type int64
}

type opensslX509NameData struct {
	entries []opensslX509NameEntry
}

type opensslRSAData struct {
	id     uint64
	bits   int64
	public bool
}

type opensslX509CertificateData struct {
	version    *object.EmeraldValue
	serial     *object.EmeraldValue
	subject    *object.EmeraldValue
	issuer     *object.EmeraldValue
	publicKey  *object.EmeraldValue
	notBefore  *object.EmeraldValue
	notAfter   *object.EmeraldValue
	extensions []*object.EmeraldValue
	signerID   uint64
}

type opensslX509StoreData struct {
	certificates []*object.EmeraldValue
	errorCode    int64
	errorString  string
}

type opensslX509ExtensionFactoryData struct {
	subject *object.EmeraldValue
	issuer  *object.EmeraldValue
}

type opensslX509ExtensionData struct {
	name     string
	value    string
	critical bool
}

func opensslArrayValue(values []*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

var opensslRSASequence atomic.Uint64

func installOpenSSLX509(openssl *object.Module) {
	asn1Module := object.NewModule("OpenSSL::ASN1")
	asn1Module.Constants["UTF8STRING"] = newInt(opensslASN1UTF8String)
	asn1Module.Constants["IA5STRING"] = newInt(opensslASN1IA5String)
	openssl.Constants["ASN1"] = &object.EmeraldValue{Type: object.ValueModule, Data: asn1Module, Class: R.Classes["Module"]}

	pkeyModule := object.NewModule("OpenSSL::PKey")
	rsaClass := object.NewClass("OpenSSL::PKey::RSA")
	rsaClass.SuperClass = R.Classes["Object"]
	rsaClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: opensslRSAClassNew, Arity: -1})
	rsaClass.DefineMethod("public_key", &object.Method{Name: "public_key", Fn: opensslRSAPublicKey, Arity: 0})
	pkeyModule.Constants["RSA"] = &object.EmeraldValue{Type: object.ValueClass, Data: rsaClass, Class: R.Classes["Class"]}
	openssl.Constants["PKey"] = &object.EmeraldValue{Type: object.ValueModule, Data: pkeyModule, Class: R.Classes["Module"]}

	x509Module := object.NewModule("OpenSSL::X509")
	nameError := object.NewClass("OpenSSL::X509::NameError")
	nameError.SuperClass = R.Classes["StandardError"]
	x509Module.Constants["NameError"] = &object.EmeraldValue{Type: object.ValueClass, Data: nameError, Class: R.Classes["Class"]}

	nameClass := object.NewClass("OpenSSL::X509::Name")
	nameClass.SuperClass = R.Classes["Object"]
	nameClass.DefineClassMethod("parse", &object.Method{Name: "parse", Fn: opensslX509NameParse, Arity: 1})
	nameClass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: opensslX509NameToS, Arity: 0})
	nameClass.DefineMethod("to_a", &object.Method{Name: "to_a", Fn: opensslX509NameToA, Arity: 0})
	x509Module.Constants["Name"] = &object.EmeraldValue{Type: object.ValueClass, Data: nameClass, Class: R.Classes["Class"]}

	certificateClass := object.NewClass("OpenSSL::X509::Certificate")
	certificateClass.SuperClass = R.Classes["Object"]
	certificateClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: opensslX509CertificateNew, Arity: 0})
	certificateClass.DefineMethod("version=", &object.Method{Name: "version=", Fn: opensslX509CertificateSetVersion, Arity: 1})
	certificateClass.DefineMethod("serial=", &object.Method{Name: "serial=", Fn: opensslX509CertificateSetSerial, Arity: 1})
	certificateClass.DefineMethod("subject=", &object.Method{Name: "subject=", Fn: opensslX509CertificateSetSubject, Arity: 1})
	certificateClass.DefineMethod("issuer=", &object.Method{Name: "issuer=", Fn: opensslX509CertificateSetIssuer, Arity: 1})
	certificateClass.DefineMethod("subject", &object.Method{Name: "subject", Fn: opensslX509CertificateSubject, Arity: 0})
	certificateClass.DefineMethod("issuer", &object.Method{Name: "issuer", Fn: opensslX509CertificateIssuer, Arity: 0})
	certificateClass.DefineMethod("public_key=", &object.Method{Name: "public_key=", Fn: opensslX509CertificateSetPublicKey, Arity: 1})
	certificateClass.DefineMethod("not_before=", &object.Method{Name: "not_before=", Fn: opensslX509CertificateSetNotBefore, Arity: 1})
	certificateClass.DefineMethod("not_after=", &object.Method{Name: "not_after=", Fn: opensslX509CertificateSetNotAfter, Arity: 1})
	certificateClass.DefineMethod("not_before", &object.Method{Name: "not_before", Fn: opensslX509CertificateNotBefore, Arity: 0})
	certificateClass.DefineMethod("not_after", &object.Method{Name: "not_after", Fn: opensslX509CertificateNotAfter, Arity: 0})
	certificateClass.DefineMethod("sign", &object.Method{Name: "sign", Fn: opensslX509CertificateSign, Arity: 2})
	certificateClass.DefineMethod("add_extension", &object.Method{Name: "add_extension", Fn: opensslX509CertificateAddExtension, Arity: 1})
	x509Module.Constants["Certificate"] = &object.EmeraldValue{Type: object.ValueClass, Data: certificateClass, Class: R.Classes["Class"]}

	storeClass := object.NewClass("OpenSSL::X509::Store")
	storeClass.SuperClass = R.Classes["Object"]
	storeClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: opensslX509StoreNew, Arity: 0})
	storeClass.DefineMethod("add_cert", &object.Method{Name: "add_cert", Fn: opensslX509StoreAddCert, Arity: 1})
	storeClass.DefineMethod("verify", &object.Method{Name: "verify", Fn: opensslX509StoreVerify, Arity: 1})
	storeClass.DefineMethod("error", &object.Method{Name: "error", Fn: opensslX509StoreError, Arity: 0})
	storeClass.DefineMethod("error_string", &object.Method{Name: "error_string", Fn: opensslX509StoreErrorString, Arity: 0})
	x509Module.Constants["Store"] = &object.EmeraldValue{Type: object.ValueClass, Data: storeClass, Class: R.Classes["Class"]}

	extensionClass := object.NewClass("OpenSSL::X509::Extension")
	extensionClass.SuperClass = R.Classes["Object"]
	x509Module.Constants["Extension"] = &object.EmeraldValue{Type: object.ValueClass, Data: extensionClass, Class: R.Classes["Class"]}

	factoryClass := object.NewClass("OpenSSL::X509::ExtensionFactory")
	factoryClass.SuperClass = R.Classes["Object"]
	factoryClass.DefineClassMethod("new", &object.Method{Name: "new", Fn: opensslX509ExtensionFactoryNew, Arity: 0})
	factoryClass.DefineMethod("subject_certificate=", &object.Method{Name: "subject_certificate=", Fn: opensslX509ExtensionFactorySetSubject, Arity: 1})
	factoryClass.DefineMethod("issuer_certificate=", &object.Method{Name: "issuer_certificate=", Fn: opensslX509ExtensionFactorySetIssuer, Arity: 1})
	factoryClass.DefineMethod("create_extension", &object.Method{Name: "create_extension", Fn: opensslX509ExtensionFactoryCreate, Arity: -1})
	x509Module.Constants["ExtensionFactory"] = &object.EmeraldValue{Type: object.ValueClass, Data: factoryClass, Class: R.Classes["Class"]}

	openssl.Constants["X509"] = &object.EmeraldValue{Type: object.ValueModule, Data: x509Module, Class: R.Classes["Module"]}
}

func opensslX509NameError(message string) *object.EmeraldValue {
	class := R.Classes["StandardError"]
	if opensslValue := R.Classes["Object"].Constants["OpenSSL"]; opensslValue != nil && opensslValue.Type == object.ValueModule {
		if openssl, ok := opensslValue.Data.(*object.Module); ok {
			if x509Value := openssl.Constants["X509"]; x509Value != nil && x509Value.Type == object.ValueModule {
				if x509, ok := x509Value.Data.(*object.Module); ok {
					if value := x509.Constants["NameError"]; value != nil && value.Type == object.ValueClass {
						class, _ = value.Data.(*object.Class)
					}
				}
			}
		}
	}
	return newRuntimeException(class, message)
}

func opensslX509Class(opensslConstant, className string) *object.Class {
	opensslValue := R.Classes["Object"].Constants["OpenSSL"]
	if opensslValue == nil || opensslValue.Type != object.ValueModule {
		return nil
	}
	openssl, _ := opensslValue.Data.(*object.Module)
	moduleValue := openssl.Constants[opensslConstant]
	if moduleValue == nil || moduleValue.Type != object.ValueModule {
		return nil
	}
	module, _ := moduleValue.Data.(*object.Module)
	classValue := module.Constants[className]
	if classValue == nil || classValue.Type != object.ValueClass {
		return nil
	}
	class, _ := classValue.Data.(*object.Class)
	return class
}

func opensslX509NameParse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	raw, errVal := opensslStringArgument(args[0])
	if errVal != nil {
		return errVal
	}
	parts := []string{}
	if strings.HasPrefix(strings.TrimSpace(raw), "/") {
		parts = strings.Split(strings.TrimPrefix(strings.TrimSpace(raw), "/"), "/")
	} else if strings.Contains(raw, "=") {
		parts = strings.Split(raw, ",")
	} else {
		return NewTypeError("Cannot parse distinguished name")
	}
	recognized := map[string]bool{"C": true, "ST": true, "L": true, "O": true, "OU": true, "CN": true, "DC": true, "UID": true, "EMAILADDRESS": true}
	entries := make([]opensslX509NameEntry, 0, len(parts))
	for _, part := range parts {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" {
			return NewTypeError("Cannot parse distinguished name")
		}
		key := strings.ToUpper(strings.TrimSpace(pair[0]))
		if !recognized[key] {
			return opensslX509NameError("invalid field name: " + pair[0])
		}
		asn1Type := opensslASN1UTF8String
		if key == "DC" || key == "EMAILADDRESS" {
			asn1Type = opensslASN1IA5String
		}
		entries = append(entries, opensslX509NameEntry{key: key, value: strings.TrimSpace(pair[1]), asn1Type: asn1Type})
	}
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &opensslX509NameData{entries: entries}, Class: klass}
}

func opensslX509NameToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*opensslX509NameData)
	if !ok {
		return rubyString("")
	}
	var builder strings.Builder
	for _, entry := range data.entries {
		builder.WriteByte('/')
		builder.WriteString(entry.key)
		builder.WriteByte('=')
		builder.WriteString(entry.value)
	}
	return rubyString(builder.String())
}

func opensslX509NameToA(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*opensslX509NameData)
	if !ok {
		return opensslArrayValue(nil)
	}
	rows := make([]*object.EmeraldValue, 0, len(data.entries))
	for _, entry := range data.entries {
		rows = append(rows, opensslArrayValue([]*object.EmeraldValue{rubyString(entry.key), rubyString(entry.value), newInt(entry.asn1Type)}))
	}
	return opensslArrayValue(rows)
}

func opensslRSAClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	bitsValue := int64(2048)
	if len(args) > 1 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..1)", len(args)))
	}
	if len(args) == 1 {
		value, errVal := opensslIntegerArgument(args[0])
		if errVal != nil {
			return errVal
		}
		bitsValue = value
	}
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &opensslRSAData{id: opensslRSASequence.Add(1), bits: bitsValue}, Class: klass}
}

func opensslRSAPublicKey(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*opensslRSAData)
	if !ok {
		return R.NilVal
	}
	copyData := *data
	copyData.public = true
	return &object.EmeraldValue{Type: object.ValueObject, Data: &copyData, Class: receiver.Class}
}

func opensslX509CertificateNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &opensslX509CertificateData{}, Class: klass}
}

func opensslX509CertificateDataFrom(receiver *object.EmeraldValue) *opensslX509CertificateData {
	data, _ := receiver.Data.(*opensslX509CertificateData)
	return data
}

func opensslX509CertificateSetVersion(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	opensslX509CertificateDataFrom(receiver).version = args[0]
	return args[0]
}
func opensslX509CertificateSetSerial(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	opensslX509CertificateDataFrom(receiver).serial = args[0]
	return args[0]
}
func opensslX509CertificateSetSubject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	opensslX509CertificateDataFrom(receiver).subject = args[0]
	return args[0]
}
func opensslX509CertificateSetIssuer(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	opensslX509CertificateDataFrom(receiver).issuer = args[0]
	return args[0]
}
func opensslX509CertificateSubject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if value := opensslX509CertificateDataFrom(receiver).subject; value != nil {
		return value
	}
	return R.NilVal
}
func opensslX509CertificateIssuer(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if value := opensslX509CertificateDataFrom(receiver).issuer; value != nil {
		return value
	}
	return R.NilVal
}
func opensslX509CertificateSetPublicKey(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	opensslX509CertificateDataFrom(receiver).publicKey = args[0]
	return args[0]
}
func opensslX509CertificateSetNotBefore(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	opensslX509CertificateDataFrom(receiver).notBefore = args[0]
	return args[0]
}
func opensslX509CertificateSetNotAfter(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	opensslX509CertificateDataFrom(receiver).notAfter = args[0]
	return args[0]
}

func opensslX509CertificateNotBefore(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if value := opensslX509CertificateDataFrom(receiver).notBefore; value != nil {
		return value
	}
	return R.NilVal
}

func opensslX509CertificateNotAfter(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if value := opensslX509CertificateDataFrom(receiver).notAfter; value != nil {
		return value
	}
	return R.NilVal
}

func opensslX509CertificateSign(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := opensslX509CertificateDataFrom(receiver)
	if key, ok := args[0].Data.(*opensslRSAData); ok {
		data.signerID = key.id
	}
	return receiver
}

func opensslX509CertificateAddExtension(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := opensslX509CertificateDataFrom(receiver)
	data.extensions = append(data.extensions, args[0])
	return args[0]
}

func opensslX509StoreNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &opensslX509StoreData{errorString: "ok"}, Class: klass}
}

func opensslX509StoreAddCert(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*opensslX509StoreData)
	if opensslX509CertificateDataFrom(args[0]) == nil {
		return NewTypeError("wrong argument type")
	}
	data.certificates = append(data.certificates, args[0])
	return receiver
}

func opensslX509NameString(value *object.EmeraldValue) string {
	if value == nil {
		return ""
	}
	data, _ := value.Data.(*opensslX509NameData)
	if data == nil {
		return ""
	}
	var builder strings.Builder
	for _, entry := range data.entries {
		builder.WriteByte('/')
		builder.WriteString(entry.key)
		builder.WriteByte('=')
		builder.WriteString(entry.value)
	}
	return builder.String()
}

func opensslX509CertificateValidAt(data *opensslX509CertificateData, now time.Time) bool {
	if data == nil || data.notBefore == nil || data.notAfter == nil {
		return false
	}
	before, beforeOK := data.notBefore.Data.(*timeData)
	after, afterOK := data.notAfter.Data.(*timeData)
	if !beforeOK || !afterOK {
		return false
	}
	return !now.Before(before.value) && !now.After(after.value)
}

func opensslX509StoreVerify(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	store, _ := receiver.Data.(*opensslX509StoreData)
	leaf := opensslX509CertificateDataFrom(args[0])
	now := time.Now()
	if !opensslX509CertificateValidAt(leaf, now) {
		store.errorCode, store.errorString = 10, "certificate has expired"
		return R.FalseVal
	}
	issuerName := opensslX509NameString(leaf.issuer)
	trusted := false
	for _, candidateValue := range store.certificates {
		candidate := opensslX509CertificateDataFrom(candidateValue)
		if candidate == nil || opensslX509NameString(candidate.subject) != issuerName {
			continue
		}
		if !opensslX509CertificateValidAt(candidate, now) {
			store.errorCode, store.errorString = 10, "certificate has expired"
			return R.FalseVal
		}
		trusted = true
		break
	}
	if !trusted {
		store.errorCode, store.errorString = 20, "unable to get local issuer certificate"
		return R.FalseVal
	}
	store.errorCode, store.errorString = 0, "ok"
	return R.TrueVal
}

func opensslX509StoreError(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*opensslX509StoreData)
	return newInt(data.errorCode)
}

func opensslX509StoreErrorString(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*opensslX509StoreData)
	return rubyString(data.errorString)
}

func opensslX509ExtensionFactoryNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	klass, _ := receiver.Data.(*object.Class)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &opensslX509ExtensionFactoryData{}, Class: klass}
}

func opensslX509ExtensionFactorySetSubject(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	receiver.Data.(*opensslX509ExtensionFactoryData).subject = args[0]
	return args[0]
}

func opensslX509ExtensionFactorySetIssuer(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	receiver.Data.(*opensslX509ExtensionFactoryData).issuer = args[0]
	return args[0]
}

func opensslX509ExtensionFactoryCreate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 3 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 2..3)", len(args)))
	}
	name, errVal := opensslStringArgument(args[0])
	if errVal != nil {
		return errVal
	}
	value, errVal := opensslStringArgument(args[1])
	if errVal != nil {
		return errVal
	}
	critical := len(args) == 3 && args[2].IsTruthy()
	klass := opensslX509Class("X509", "Extension")
	return &object.EmeraldValue{Type: object.ValueObject, Data: &opensslX509ExtensionData{name: name, value: value, critical: critical}, Class: klass}
}
