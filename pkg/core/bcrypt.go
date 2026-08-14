package core

// The bcrypt primitive below follows golang.org/x/crypto/bcrypt's BSD-licensed
// implementation, but exposes the fixed-salt operation required by bcrypt-ruby's
// native extension API.

import (
	"encoding/base64"
	"fmt"
	"strconv"

	"github.com/GoLangDream/rgo/pkg/object"
	"golang.org/x/crypto/blowfish"
)

const (
	bcryptMinCost         = 4
	bcryptMaxCost         = 31
	bcryptSaltBytes       = 16
	bcryptEncodedSaltSize = 22
)

var bcryptEncoding = base64.NewEncoding("./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

var bcryptMagicCipherData = []byte("OrpheanBeholderScryDoubt")

func installBCryptExtension(objectClass *object.Class) {
	if objectClass == nil {
		return
	}

	var bcryptModule *object.Module
	bcryptValue := objectClass.Constants["BCrypt"]
	if bcryptValue != nil && bcryptValue.Type == object.ValueModule {
		bcryptModule, _ = bcryptValue.Data.(*object.Module)
	}
	if bcryptModule == nil {
		bcryptModule = object.NewModule("BCrypt")
		bcryptValue = &object.EmeraldValue{Type: object.ValueModule, Data: bcryptModule, Class: R.Classes["Module"]}
		objectClass.DefineConstant("BCrypt", bcryptValue)
		AssignConstantName(classEmeraldValue(objectClass), "BCrypt", bcryptValue)
	}

	engineValue := bcryptModule.Constants["Engine"]
	var engine *object.Class
	if engineValue != nil && engineValue.Type == object.ValueClass {
		engine, _ = engineValue.Data.(*object.Class)
	}
	if engine == nil {
		engine = object.NewClass("BCrypt::Engine")
		engine.SuperClass = R.Classes["Object"]
		engineValue = &object.EmeraldValue{Type: object.ValueClass, Data: engine, Class: R.Classes["Class"]}
		bcryptModule.Constants["Engine"] = engineValue
		AssignConstantName(bcryptValue, "Engine", engineValue)
	}

	engine.DefineClassMethod("__bc_salt", &object.Method{Name: "__bc_salt", Fn: bcryptExtSalt, Arity: 3})
	engine.DefineClassMethod("__bc_crypt", &object.Method{Name: "__bc_crypt", Fn: bcryptExtCrypt, Arity: 2})
}

func bcryptExtSalt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 3 {
		return NewArgumentError("wrong number of arguments")
	}
	if args[0] == nil || args[0].Type != object.ValueString ||
		args[1] == nil || args[1].Type != object.ValueInteger ||
		args[2] == nil || args[2].Type != object.ValueString {
		return typeError("wrong argument type")
	}
	prefix := args[0].Data.(string)
	cost := int(args[1].Data.(int64))
	random := []byte(args[2].Data.(string))
	if len(random) < bcryptSaltBytes || cost < bcryptMinCost || cost > bcryptMaxCost {
		return R.NilVal
	}
	return rubyString(fmt.Sprintf("%s%02d$%s", prefix, cost, bcryptBase64Encode(random[:bcryptSaltBytes])))
}

func bcryptExtCrypt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	if args[0] == nil || args[0].Type != object.ValueString ||
		args[1] == nil || args[1].Type != object.ValueString {
		return R.NilVal
	}
	hashed, err := bcryptHashWithSetting([]byte(args[0].Data.(string)), args[1].Data.(string))
	if err != nil {
		return R.NilVal
	}
	return rubyString(hashed)
}

func bcryptHashWithSetting(password []byte, setting string) (string, error) {
	if len(setting) < 29 || setting[0] != '$' || setting[1] != '2' || setting[3] != '$' || setting[6] != '$' {
		return "", fmt.Errorf("invalid bcrypt setting")
	}
	cost, err := strconv.Atoi(setting[4:6])
	if err != nil || cost < bcryptMinCost || cost > bcryptMaxCost {
		return "", fmt.Errorf("invalid bcrypt cost")
	}
	salt := []byte(setting[7:29])
	decodedSalt, err := bcryptBase64Decode(salt)
	if err != nil || len(decodedSalt) != bcryptSaltBytes {
		return "", fmt.Errorf("invalid bcrypt salt")
	}

	key := make([]byte, len(password)+1)
	copy(key, password)
	cipher, err := blowfish.NewSaltedCipher(key, decodedSalt)
	if err != nil {
		return "", err
	}
	rounds := uint64(1) << uint(cost)
	for round := uint64(0); round < rounds; round++ {
		blowfish.ExpandKey(key, cipher)
		blowfish.ExpandKey(decodedSalt, cipher)
	}

	cipherData := append([]byte(nil), bcryptMagicCipherData...)
	for offset := 0; offset < len(cipherData); offset += blowfish.BlockSize {
		for round := 0; round < 64; round++ {
			cipher.Encrypt(cipherData[offset:offset+blowfish.BlockSize], cipherData[offset:offset+blowfish.BlockSize])
		}
	}
	return setting[:29] + bcryptBase64Encode(cipherData[:23]), nil
}

func bcryptBase64Encode(source []byte) string {
	target := make([]byte, bcryptEncoding.EncodedLen(len(source)))
	bcryptEncoding.Encode(target, source)
	for len(target) > 0 && target[len(target)-1] == '=' {
		target = target[:len(target)-1]
	}
	return string(target)
}

func bcryptBase64Decode(source []byte) ([]byte, error) {
	padded := append([]byte(nil), source...)
	for len(padded)%4 != 0 {
		padded = append(padded, '=')
	}
	target := make([]byte, bcryptEncoding.DecodedLen(len(padded)))
	written, err := bcryptEncoding.Decode(target, padded)
	if err != nil {
		return nil, err
	}
	return target[:written], nil
}
