package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	stdhash "hash"
	"math"
	"math/bits"

	"github.com/GoLangDream/rgo/pkg/object"
)

func installOpenSSLKDF(openssl *object.Module) {
	kdfModule := object.NewModule("OpenSSL::KDF")
	kdfModule.DefineMethod("pbkdf2_hmac", &object.Method{Name: "pbkdf2_hmac", Fn: opensslKDFPBKDF2HMAC, Arity: -1})
	kdfModule.DefineMethod("scrypt", &object.Method{Name: "scrypt", Fn: opensslKDFScrypt, Arity: -1})
	kdfError := object.NewClass("OpenSSL::KDF::KDFError")
	kdfError.SuperClass = R.Classes["StandardError"]
	kdfModule.Constants["KDFError"] = &object.EmeraldValue{Type: object.ValueClass, Data: kdfError, Class: R.Classes["Class"]}
	openssl.Constants["KDF"] = &object.EmeraldValue{Type: object.ValueModule, Data: kdfModule, Class: R.Classes["Module"]}
}

func opensslKDFError(message string) *object.EmeraldValue {
	class := R.Classes["StandardError"]
	if opensslValue := R.Classes["Object"].Constants["OpenSSL"]; opensslValue != nil && opensslValue.Type == object.ValueModule {
		if openssl, ok := opensslValue.Data.(*object.Module); ok {
			if kdfValue := openssl.Constants["KDF"]; kdfValue != nil && kdfValue.Type == object.ValueModule {
				if kdf, ok := kdfValue.Data.(*object.Module); ok {
					if errorValue := kdf.Constants["KDFError"]; errorValue != nil && errorValue.Type == object.ValueClass {
						class, _ = errorValue.Data.(*object.Class)
					}
				}
			}
		}
	}
	return newRuntimeException(class, message)
}

func opensslKDFArguments(args []*object.EmeraldValue, names []string) (*object.EmeraldValue, map[string]*object.EmeraldValue, *object.EmeraldValue) {
	if len(args) == 0 {
		return nil, nil, NewArgumentError("wrong number of arguments (given 0, expected 1)")
	}
	for _, argument := range args[1:] {
		if argument == nil || argument.Type != object.ValueHash {
			given := 1
			for _, candidate := range args[1:] {
				if candidate == nil || candidate.Type != object.ValueHash {
					given++
				}
			}
			if given == 0 {
				return nil, nil, NewArgumentError("wrong number of arguments (given 0, expected 1)")
			}
			return nil, nil, NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 1)", given))
		}
	}
	values := make(map[string]*object.EmeraldValue, len(names))
	for _, keywords := range args[1:] {
		for key, value := range valueToHashMap(keywords) {
			name := specName(key)
			known := false
			for _, expected := range names {
				if name == expected {
					known = true
					break
				}
			}
			if !known {
				return nil, nil, NewArgumentError("unknown keyword: :" + name)
			}
			values[name] = value
		}
	}
	missing := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := values[name]; !ok {
			missing = append(missing, ":"+name)
		}
	}
	if len(missing) == 1 {
		return nil, nil, NewArgumentError("missing keyword: " + missing[0])
	}
	if len(missing) > 1 {
		message := missing[0]
		for _, name := range missing[1:] {
			message += ", " + name
		}
		return nil, nil, NewArgumentError("missing keywords: " + message)
	}
	return args[0], values, nil
}

func opensslIntegerArgument(value *object.EmeraldValue) (int64, *object.EmeraldValue) {
	if integer, ok := numericBigIntValue(value); ok {
		if !integer.IsInt64() {
			return 0, NewRangeError("integer out of range")
		}
		return integer.Int64(), nil
	}
	if CallMethod != nil && value != nil && receiverHasCallableMethod(value, "to_int") {
		converted := CallMethod(value, "to_int")
		if converted != nil && converted.Type == object.ValueException {
			return 0, converted
		}
		if integer, ok := numericBigIntValue(converted); ok && integer.IsInt64() {
			return integer.Int64(), nil
		}
	}
	return 0, conversionTypeErrorToInteger(value)
}

func opensslKDFPBKDF2HMAC(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	passwordValue, keywords, errVal := opensslKDFArguments(args, []string{"salt", "iterations", "length", "hash"})
	if errVal != nil {
		return errVal
	}
	password, errVal := opensslStringArgument(passwordValue)
	if errVal != nil {
		return errVal
	}
	salt, errVal := opensslStringArgument(keywords["salt"])
	if errVal != nil {
		return errVal
	}
	iterations, errVal := opensslIntegerArgument(keywords["iterations"])
	if errVal != nil {
		return errVal
	}
	length, errVal := opensslIntegerArgument(keywords["length"])
	if errVal != nil {
		return errVal
	}
	spec, errVal := opensslDigestSpecArgument(keywords["hash"])
	if errVal != nil {
		return errVal
	}
	if iterations <= 0 {
		return opensslKDFError("PKCS5_PBKDF2_HMAC: invalid iteration count")
	}
	if length < 0 || uint64(length) > uint64(math.MaxInt) {
		return NewArgumentError("negative string size (or size too big)")
	}
	key := opensslPBKDF2([]byte(password), []byte(salt), int(iterations), int(length), spec.newHash)
	return stringWithEncoding(string(key), "ASCII-8BIT")
}

func opensslPBKDF2(password, salt []byte, iterations, length int, newHash func() stdhash.Hash) []byte {
	if length == 0 {
		return []byte{}
	}
	mac := hmac.New(newHash, password)
	hashLength := mac.Size()
	out := make([]byte, 0, length)
	var counter [4]byte
	for block := uint32(1); len(out) < length; block++ {
		binary.BigEndian.PutUint32(counter[:], block)
		mac.Reset()
		_, _ = mac.Write(salt)
		_, _ = mac.Write(counter[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac.Reset()
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for j := 0; j < hashLength; j++ {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:length]
}

func opensslKDFScrypt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	passwordValue, keywords, errVal := opensslKDFArguments(args, []string{"salt", "N", "r", "p", "length"})
	if errVal != nil {
		return errVal
	}
	password, errVal := opensslStringArgument(passwordValue)
	if errVal != nil {
		return errVal
	}
	salt, errVal := opensslStringArgument(keywords["salt"])
	if errVal != nil {
		return errVal
	}
	n, errVal := opensslIntegerArgument(keywords["N"])
	if errVal != nil {
		return errVal
	}
	r, errVal := opensslIntegerArgument(keywords["r"])
	if errVal != nil {
		return errVal
	}
	p, errVal := opensslIntegerArgument(keywords["p"])
	if errVal != nil {
		return errVal
	}
	length, errVal := opensslIntegerArgument(keywords["length"])
	if errVal != nil {
		return errVal
	}
	if length < 0 || uint64(length) > uint64(math.MaxInt) {
		return NewArgumentError("negative string size (or size too big)")
	}
	if n <= 1 || n&(n-1) != 0 || r <= 0 || p <= 0 || n > int64(math.MaxInt) || r > int64(math.MaxInt) || p > int64(math.MaxInt) {
		return opensslKDFError("EVP_PBE_scrypt")
	}
	if uint64(r)*uint64(p) >= 1<<30 || uint64(n)*uint64(r) > uint64(math.MaxInt)/(128*4) {
		return opensslKDFError("EVP_PBE_scrypt")
	}
	key, err := opensslScryptKey([]byte(password), []byte(salt), int(n), int(r), int(p), int(length))
	if err != nil {
		return opensslKDFError("EVP_PBE_scrypt")
	}
	return stringWithEncoding(string(key), "ASCII-8BIT")
}

func opensslScryptKey(password, salt []byte, n, r, p, length int) ([]byte, error) {
	if n <= 1 || n&(n-1) != 0 || r <= 0 || p <= 0 || uint64(r)*uint64(p) >= 1<<30 {
		return nil, fmt.Errorf("invalid scrypt parameters")
	}
	b := opensslPBKDF2(password, salt, 1, p*128*r, sha256.New)
	v := make([]uint32, 32*n*r)
	xy := make([]uint32, 64*r)
	for i := 0; i < p; i++ {
		opensslScryptSMix(b[i*128*r:], r, n, v, xy)
	}
	return opensslPBKDF2(password, b, 1, length, sha256.New), nil
}

func opensslScryptSMix(b []byte, r, n int, v, xy []uint32) {
	var tmp [16]uint32
	width := 32 * r
	x := xy[:width]
	y := xy[width:]
	for i := 0; i < width; i++ {
		x[i] = binary.LittleEndian.Uint32(b[i*4:])
	}
	for i := 0; i < n; i += 2 {
		copy(v[i*width:], x)
		opensslScryptBlockMix(&tmp, x, y, r)
		copy(v[(i+1)*width:], y)
		opensslScryptBlockMix(&tmp, y, x, r)
	}
	for i := 0; i < n; i += 2 {
		j := int(opensslScryptInteger(x, r) & uint64(n-1))
		for k := 0; k < width; k++ {
			x[k] ^= v[j*width+k]
		}
		opensslScryptBlockMix(&tmp, x, y, r)
		j = int(opensslScryptInteger(y, r) & uint64(n-1))
		for k := 0; k < width; k++ {
			y[k] ^= v[j*width+k]
		}
		opensslScryptBlockMix(&tmp, y, x, r)
	}
	for i, value := range x {
		binary.LittleEndian.PutUint32(b[i*4:], value)
	}
}

func opensslScryptInteger(b []uint32, r int) uint64 {
	index := (2*r - 1) * 16
	return uint64(b[index]) | uint64(b[index+1])<<32
}

func opensslScryptBlockMix(tmp *[16]uint32, input, output []uint32, r int) {
	copy(tmp[:], input[(2*r-1)*16:])
	for i := 0; i < 2*r; i += 2 {
		opensslScryptSalsaXOR(tmp, input[i*16:], output[i*8:])
		opensslScryptSalsaXOR(tmp, input[i*16+16:], output[i*8+r*16:])
	}
}

func opensslScryptSalsaXOR(tmp *[16]uint32, input, output []uint32) {
	for i := 0; i < 16; i++ {
		tmp[i] ^= input[i]
	}
	opensslSalsa208(tmp[:])
	copy(output, tmp[:])
}

func opensslSalsa208(b []uint32) {
	x := make([]uint32, 16)
	copy(x, b)
	for i := 0; i < 8; i += 2 {
		x[4] ^= bits.RotateLeft32(x[0]+x[12], 7)
		x[8] ^= bits.RotateLeft32(x[4]+x[0], 9)
		x[12] ^= bits.RotateLeft32(x[8]+x[4], 13)
		x[0] ^= bits.RotateLeft32(x[12]+x[8], 18)
		x[9] ^= bits.RotateLeft32(x[5]+x[1], 7)
		x[13] ^= bits.RotateLeft32(x[9]+x[5], 9)
		x[1] ^= bits.RotateLeft32(x[13]+x[9], 13)
		x[5] ^= bits.RotateLeft32(x[1]+x[13], 18)
		x[14] ^= bits.RotateLeft32(x[10]+x[6], 7)
		x[2] ^= bits.RotateLeft32(x[14]+x[10], 9)
		x[6] ^= bits.RotateLeft32(x[2]+x[14], 13)
		x[10] ^= bits.RotateLeft32(x[6]+x[2], 18)
		x[3] ^= bits.RotateLeft32(x[15]+x[11], 7)
		x[7] ^= bits.RotateLeft32(x[3]+x[15], 9)
		x[11] ^= bits.RotateLeft32(x[7]+x[3], 13)
		x[15] ^= bits.RotateLeft32(x[11]+x[7], 18)
		x[1] ^= bits.RotateLeft32(x[0]+x[3], 7)
		x[2] ^= bits.RotateLeft32(x[1]+x[0], 9)
		x[3] ^= bits.RotateLeft32(x[2]+x[1], 13)
		x[0] ^= bits.RotateLeft32(x[3]+x[2], 18)
		x[6] ^= bits.RotateLeft32(x[5]+x[4], 7)
		x[7] ^= bits.RotateLeft32(x[6]+x[5], 9)
		x[4] ^= bits.RotateLeft32(x[7]+x[6], 13)
		x[5] ^= bits.RotateLeft32(x[4]+x[7], 18)
		x[11] ^= bits.RotateLeft32(x[10]+x[9], 7)
		x[8] ^= bits.RotateLeft32(x[11]+x[10], 9)
		x[9] ^= bits.RotateLeft32(x[8]+x[11], 13)
		x[10] ^= bits.RotateLeft32(x[9]+x[8], 18)
		x[12] ^= bits.RotateLeft32(x[15]+x[14], 7)
		x[13] ^= bits.RotateLeft32(x[12]+x[15], 9)
		x[14] ^= bits.RotateLeft32(x[13]+x[12], 13)
		x[15] ^= bits.RotateLeft32(x[14]+x[13], 18)
	}
	for i := range b {
		b[i] += x[i]
	}
}
