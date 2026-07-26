package core

import (
	"hash/fnv"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

const (
	bigDecimalFinite = iota
	bigDecimalNaN
	bigDecimalPositiveInfinity
	bigDecimalNegativeInfinity
)

type bigDecimalData struct {
	coefficient  *big.Int
	exponent     int
	special      int
	negativeZero bool
	precision    int
}

var (
	bigDecimalGlobalLimit int
	bigDecimalRoundMode   int64 = 3
	bigDecimalExceptions        = map[int64]bool{}
)

func installBigDecimalClass(objectClass *object.Class) {
	if objectClass == nil {
		return
	}
	if existing := objectClass.Constants["BigDecimal"]; existing != nil && existing.Type == object.ValueClass {
		installBigMathModule(objectClass)
		return
	}

	class := object.NewClass("BigDecimal")
	class.SuperClass = R.Classes["Numeric"]
	class.DefineMethod("==", &object.Method{Name: "==", Fn: bigDecimalEqual, Arity: 1})
	class.DefineMethod("eql?", &object.Method{Name: "eql?", Fn: bigDecimalEql, Arity: 1})
	class.DefineMethod("<=>", &object.Method{Name: "<=>", Fn: bigDecimalCompare, Arity: 1})
	class.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: bigDecimalToS, Arity: -1})
	class.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: bigDecimalToS, Arity: 0})
	class.DefineMethod("finite?", &object.Method{Name: "finite?", Fn: bigDecimalFiniteMethod, Arity: 0})
	class.DefineMethod("nan?", &object.Method{Name: "nan?", Fn: bigDecimalNaNMethod, Arity: 0})
	class.DefineMethod("infinite?", &object.Method{Name: "infinite?", Fn: bigDecimalInfiniteMethod, Arity: 0})
	class.DefineMethod("zero?", &object.Method{Name: "zero?", Fn: bigDecimalZero, Arity: 0})
	class.DefineMethod("sign", &object.Method{Name: "sign", Fn: bigDecimalSign, Arity: 0})
	class.DefineMethod("precision", &object.Method{Name: "precision", Fn: bigDecimalPrecision, Arity: 0})
	class.DefineMethod("to_f", &object.Method{Name: "to_f", Fn: bigDecimalToF, Arity: 0})
	class.DefineMethod("to_i", &object.Method{Name: "to_i", Fn: bigDecimalToI, Arity: 0})
	class.DefineMethod("to_int", &object.Method{Name: "to_int", Fn: bigDecimalToI, Arity: 0})
	class.DefineMethod("+", &object.Method{Name: "+", Fn: bigDecimalAdd, Arity: 1})
	class.DefineMethod("add", &object.Method{Name: "add", Fn: bigDecimalAdd, Arity: 2})
	class.DefineMethod("-", &object.Method{Name: "-", Fn: bigDecimalSubtract, Arity: 1})
	class.DefineMethod("sub", &object.Method{Name: "sub", Fn: bigDecimalSubtract, Arity: 2})
	class.DefineMethod("*", &object.Method{Name: "*", Fn: bigDecimalMultiply, Arity: 1})
	class.DefineMethod("mult", &object.Method{Name: "mult", Fn: bigDecimalMultiply, Arity: 2})
	class.DefineMethod("multiply", &object.Method{Name: "multiply", Fn: bigDecimalMultiply, Arity: 2})
	class.DefineMethod("abs", &object.Method{Name: "abs", Fn: bigDecimalAbs, Arity: 0})
	class.DefineMethod("+@", &object.Method{Name: "+@", Fn: bigDecimalUnaryPlus, Arity: 0})
	class.DefineMethod("-@", &object.Method{Name: "-@", Fn: bigDecimalUnaryMinus, Arity: 0})
	class.DefineMethod("coerce", &object.Method{Name: "coerce", Fn: bigDecimalCoerce, Arity: 1})
	class.DefineMethod("===", &object.Method{Name: "===", Fn: bigDecimalEqual, Arity: 1})
	class.DefineMethod("exponent", &object.Method{Name: "exponent", Fn: bigDecimalExponent, Arity: 0})
	class.DefineMethod("fix", &object.Method{Name: "fix", Fn: bigDecimalFix, Arity: 0})
	class.DefineMethod("frac", &object.Method{Name: "frac", Fn: bigDecimalFrac, Arity: 0})
	class.DefineMethod("to_r", &object.Method{Name: "to_r", Fn: bigDecimalToR, Arity: 0})
	class.DefineMethod("hash", &object.Method{Name: "hash", Fn: bigDecimalHash, Arity: 0})
	class.DefineMethod("/", &object.Method{Name: "/", Fn: bigDecimalQuo, Arity: 1})
	class.DefineMethod("quo", &object.Method{Name: "quo", Fn: bigDecimalQuo, Arity: 1})
	class.DefineMethod("divide", &object.Method{Name: "divide", Fn: bigDecimalQuo, Arity: 1})
	class.DefineMethod("div", &object.Method{Name: "div", Fn: bigDecimalDiv, Arity: -1})
	class.DefineMethod("divmod", &object.Method{Name: "divmod", Fn: bigDecimalDivmod, Arity: 1})
	class.DefineMethod("%", &object.Method{Name: "%", Fn: bigDecimalModulo, Arity: 1})
	class.DefineMethod("modulo", &object.Method{Name: "modulo", Fn: bigDecimalModulo, Arity: 1})
	class.DefineMethod("remainder", &object.Method{Name: "remainder", Fn: bigDecimalRemainder, Arity: 1})
	class.DefineMethod("floor", &object.Method{Name: "floor", Fn: bigDecimalFloor, Arity: -1})
	class.DefineMethod("truncate", &object.Method{Name: "truncate", Fn: bigDecimalTruncate, Arity: -1})
	class.DefineMethod(">", &object.Method{Name: ">", Fn: bigDecimalGreaterThan, Arity: 1})
	class.DefineMethod(">=", &object.Method{Name: ">=", Fn: bigDecimalGreaterEqual, Arity: 1})
	class.DefineMethod("<", &object.Method{Name: "<", Fn: bigDecimalLessThan, Arity: 1})
	class.DefineMethod("<=", &object.Method{Name: "<=", Fn: bigDecimalLessEqual, Arity: 1})
	class.DefineMethod("round", &object.Method{Name: "round", Fn: bigDecimalRound, Arity: -1})
	class.DefineMethod("ceil", &object.Method{Name: "ceil", Fn: bigDecimalCeil, Arity: -1})
	class.DefineMethod("split", &object.Method{Name: "split", Fn: bigDecimalSplit, Arity: 0})
	class.DefineMethod("**", &object.Method{Name: "**", Fn: bigDecimalPower, Arity: 1})
	class.DefineMethod("power", &object.Method{Name: "power", Fn: bigDecimalPower, Arity: 1})
	class.DefineMethod("sqrt", &object.Method{Name: "sqrt", Fn: bigDecimalSqrt, Arity: 1})
	class.DefineClassMethod("limit", &object.Method{Name: "limit", Fn: bigDecimalLimit, Arity: -1})
	class.DefineClassMethod("mode", &object.Method{Name: "mode", Fn: bigDecimalMode, Arity: -1})

	classValue := &object.EmeraldValue{Type: object.ValueClass, Data: class, Class: R.Classes["Class"]}
	objectClass.DefineConstant("BigDecimal", classValue)
	R.Classes["BigDecimal"] = class
	AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "BigDecimal", classValue)
	installBigMathModule(objectClass)

	constants := map[string]int64{
		"BASE": 1_000_000_000, "EXCEPTION_ALL": 0xff, "EXCEPTION_INFINITY": 0x01,
		"EXCEPTION_NaN": 0x02, "EXCEPTION_UNDERFLOW": 0x04, "EXCEPTION_OVERFLOW": 0x01,
		"EXCEPTION_ZERODIVIDE": 0x10, "ROUND_MODE": 0x100, "ROUND_UP": 1,
		"ROUND_DOWN": 2, "ROUND_HALF_UP": 3, "ROUND_HALF_DOWN": 4, "ROUND_CEILING": 5,
		"ROUND_FLOOR": 6, "ROUND_HALF_EVEN": 7, "SIGN_NaN": 0,
		"SIGN_POSITIVE_ZERO": 1, "SIGN_NEGATIVE_ZERO": -1, "SIGN_POSITIVE_FINITE": 2,
		"SIGN_NEGATIVE_FINITE": -2, "SIGN_POSITIVE_INFINITE": 3, "SIGN_NEGATIVE_INFINITE": -3,
	}
	for name, value := range constants {
		class.DefineConstant(name, newInt(value))
	}
	class.DefineConstant("VERSION", rubyString("3.3.1"))
	class.DefineConstant("NAN", bigDecimalValue(&bigDecimalData{special: bigDecimalNaN}))
	class.DefineConstant("INFINITY", bigDecimalValue(&bigDecimalData{special: bigDecimalPositiveInfinity}))

	method := &object.Method{Name: "BigDecimal", Fn: bigDecimalKernel, Arity: -1, Visibility: "private"}
	objectClass.DefineMethod("BigDecimal", method)
	if kernel := R.Classes["Kernel"]; kernel != nil {
		kernel.DefineMethod("BigDecimal", method)
		kernel.DefineClassMethod("BigDecimal", &object.Method{Name: "BigDecimal", Fn: bigDecimalKernel, Arity: -1})
	}
}

func installBigMathModule(objectClass *object.Class) {
	if objectClass == nil || objectClass.Constants["BigMath"] != nil {
		return
	}
	bigMath := object.NewModule("BigMath")
	bigMath.DefineMethod("log", &object.Method{Name: "log", Fn: bigMathLog, Arity: 2})
	objectClass.DefineConstant("BigMath", &object.EmeraldValue{Type: object.ValueModule, Data: bigMath, Class: R.Classes["Module"]})
}

func bigDecimalKernel(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	args, exceptionFalse := kernelConversionExceptionFalse(args)
	if len(args) == 0 || len(args) > 3 {
		return kernelConversionResult(NewArgumentError("wrong number of arguments"), exceptionFalse)
	}
	precision := 0
	if len(args) >= 2 {
		parsed, ok := valueToInteger(args[1])
		if !ok || parsed < 0 {
			return kernelConversionResult(NewArgumentError("invalid precision"), exceptionFalse)
		}
		precision = int(parsed)
	}
	if rational, ok := rationalValueData(args[0]); ok {
		if len(args) < 2 {
			return kernelConversionResult(NewArgumentError("can't omit precision for a Rational"), exceptionFalse)
		}
		return bigDecimalValue(bigDecimalFromRat(new(big.Rat).SetFrac(rational.numerator, rational.denominator), precision))
	}
	data, ok := bigDecimalFromValue(args[0])
	if !ok {
		return kernelConversionResult(NewArgumentError("invalid value for BigDecimal"), exceptionFalse)
	}
	return kernelConversionResult(bigDecimalResult(data, false), exceptionFalse)
}

func installBigDecimalUtil() {
	installBigDecimalClass(R.Classes["Object"])
	method := func(className string, fn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue) {
		if class := R.Classes[className]; class != nil {
			class.DefineMethod("to_d", &object.Method{Name: "to_d", Fn: fn, Arity: -1})
		}
	}
	method("Integer", bigDecimalNumericToD)
	method("Float", bigDecimalNumericToD)
	method("String", bigDecimalStringToD)
	method("Rational", bigDecimalRationalToD)
	method("NilClass", bigDecimalNilToD)
	method("BigDecimal", bigDecimalSelfToD)
	if class := R.Classes["BigDecimal"]; class != nil {
		class.DefineMethod("to_digits", &object.Method{Name: "to_digits", Fn: bigDecimalToDigits, Arity: 0})
	}
}

func bigDecimalFromRat(value *big.Rat, precision int) *bigDecimalData {
	if precision <= 0 {
		precision = len(new(big.Int).Abs(value.Num()).String()) + len(value.Denom().String()) + 16
	}
	float := new(big.Float).SetPrec(uint(precision*4 + 64)).SetMode(big.ToNearestEven).SetRat(value)
	text := float.Text('e', precision-1)
	data, _ := parseBigDecimal(text)
	return data
}

func bigMathLog(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	precisionValue, ok := valueToInteger(args[1])
	if !ok || precisionValue <= 0 {
		return NewArgumentError("precision must be positive")
	}
	data, ok := bigDecimalNumericFromValue(args[0])
	if !ok || data.special != bigDecimalFinite {
		return NewTypeError("can't convert into BigDecimal")
	}
	rational := bigDecimalToRat(data)
	if rational == nil || rational.Sign() <= 0 {
		return mathDomainError("log")
	}
	precision := int(precisionValue)
	bits := uint(precision*4 + 128)
	value := new(big.Float).SetPrec(bits).SetMode(big.ToNearestEven).SetRat(rational)
	mantissa := new(big.Float).SetPrec(bits)
	exponent := value.MantExp(mantissa)
	result := bigFloatNaturalLogSeries(mantissa, bits)
	if exponent != 0 {
		two := new(big.Float).SetPrec(bits).SetInt64(2)
		logTwo := bigFloatNaturalLogSeries(two, bits)
		scale := new(big.Float).SetPrec(bits).SetInt64(int64(exponent))
		result.Add(result, scale.Mul(scale, logTwo))
	}
	parsed, valid := parseBigDecimal(result.Text('e', precision+8))
	if !valid {
		return NewArgumentError("invalid value for BigDecimal")
	}
	return bigDecimalValue(parsed)
}

func bigFloatNaturalLogSeries(value *big.Float, bits uint) *big.Float {
	one := new(big.Float).SetPrec(bits).SetInt64(1)
	numerator := new(big.Float).SetPrec(bits).Sub(value, one)
	denominator := new(big.Float).SetPrec(bits).Add(value, one)
	z := new(big.Float).SetPrec(bits).Quo(numerator, denominator)
	zSquared := new(big.Float).SetPrec(bits).Mul(z, z)
	term := new(big.Float).SetPrec(bits).Set(z)
	sum := new(big.Float).SetPrec(bits).Set(z)
	threshold := new(big.Float).SetPrec(bits).SetMantExp(one, -int(bits))
	absolute := new(big.Float).SetPrec(bits)
	for divisor := int64(3); ; divisor += 2 {
		term.Mul(term, zSquared)
		addend := new(big.Float).SetPrec(bits).Quo(term, new(big.Float).SetPrec(bits).SetInt64(divisor))
		sum.Add(sum, addend)
		absolute.Abs(addend)
		if absolute.Cmp(threshold) <= 0 {
			break
		}
	}
	return sum.Mul(sum, new(big.Float).SetPrec(bits).SetInt64(2))
}

func bigDecimalNumericToD(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := bigDecimalFromValue(receiver)
	if !ok {
		return NewTypeError("cannot convert to BigDecimal")
	}
	return bigDecimalValue(data)
}

func bigDecimalStringToD(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	raw := receiver.Data.(string)
	for end := len(raw); end > 0; end-- {
		if data, ok := parseBigDecimal(raw[:end]); ok {
			return bigDecimalValue(data)
		}
	}
	return bigDecimalValue(normalizeBigDecimal(big.NewInt(0), 0, false))
}

func bigDecimalRationalToD(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := rationalValueData(receiver)
	if !ok {
		return NewTypeError("cannot convert to BigDecimal")
	}
	precision := 0
	if len(args) > 1 {
		return NewArgumentError("wrong number of arguments")
	}
	if len(args) == 1 {
		value, valid := valueToInteger(args[0])
		if !valid || value < 0 {
			return NewArgumentError("invalid precision")
		}
		precision = int(value)
	}
	return bigDecimalValue(bigDecimalFromRat(new(big.Rat).SetFrac(data.numerator, data.denominator), precision))
}

func bigDecimalNilToD(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalValue(normalizeBigDecimal(big.NewInt(0), 0, false))
}

func bigDecimalSelfToD(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func bigDecimalToDigits(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite {
		return bigDecimalASCIIString(bigDecimalString(data))
	}
	return bigDecimalASCIIString(bigDecimalFixedString(data))
}

func bigDecimalFixedString(data *bigDecimalData) string {
	digits := new(big.Int).Abs(data.coefficient).String()
	sign := ""
	if data.coefficient.Sign() < 0 || data.negativeZero {
		sign = "-"
	}
	position := len(digits) + data.exponent
	switch {
	case position <= 0:
		return sign + "0." + strings.Repeat("0", -position) + digits
	case position >= len(digits):
		return sign + digits + strings.Repeat("0", position-len(digits)) + ".0"
	default:
		return sign + digits[:position] + "." + digits[position:]
	}
}

func bigDecimalFromValue(value *object.EmeraldValue) (*bigDecimalData, bool) {
	if value == nil {
		return nil, false
	}
	if data := bigDecimalDataFrom(value); data != nil {
		return cloneBigDecimalData(data), true
	}
	if rational, ok := rationalValueData(value); ok {
		precision := len(new(big.Int).Abs(rational.numerator).String()) + len(rational.denominator.String()) + 48
		return bigDecimalFromRat(new(big.Rat).SetFrac(rational.numerator, rational.denominator), precision), true
	}
	switch value.Type {
	case object.ValueString:
		return parseBigDecimal(value.Data.(string))
	case object.ValueInteger:
		coefficient := new(big.Int)
		if value.BigInt != nil {
			coefficient.Set(value.BigInt)
		} else {
			coefficient.SetInt64(value.Data.(int64))
		}
		return normalizeBigDecimal(coefficient, 0, false), true
	case object.ValueFloat:
		f := value.Data.(float64)
		switch {
		case math.IsNaN(f):
			return &bigDecimalData{special: bigDecimalNaN}, true
		case math.IsInf(f, 1):
			return &bigDecimalData{special: bigDecimalPositiveInfinity}, true
		case math.IsInf(f, -1):
			return &bigDecimalData{special: bigDecimalNegativeInfinity}, true
		}
		data, ok := parseBigDecimal(strconv.FormatFloat(f, 'g', -1, 64))
		if ok && f == 0 && math.Signbit(f) {
			data.negativeZero = true
		}
		return data, ok
	default:
		if CallMethod != nil && receiverHasCallableMethod(value, "to_str") {
			converted := CallMethod(value, "to_str")
			if converted != nil && converted.Type == object.ValueString {
				return parseBigDecimal(converted.Data.(string))
			}
		}
		return nil, false
	}
}

func bigDecimalNumericFromValue(value *object.EmeraldValue) (*bigDecimalData, bool) {
	if value == nil || value.Type == object.ValueString {
		return nil, false
	}
	if bigDecimalDataFrom(value) != nil || value.Type == object.ValueInteger || value.Type == object.ValueFloat {
		return bigDecimalFromValue(value)
	}
	if _, ok := rationalValueData(value); ok {
		return bigDecimalFromValue(value)
	}
	return nil, false
}

func parseBigDecimal(input string) (*bigDecimalData, bool) {
	s := strings.TrimSpace(input)
	switch strings.ToLower(s) {
	case "nan":
		return &bigDecimalData{special: bigDecimalNaN}, true
	case "infinity", "+infinity":
		return &bigDecimalData{special: bigDecimalPositiveInfinity}, true
	case "-infinity":
		return &bigDecimalData{special: bigDecimalNegativeInfinity}, true
	}
	if s == "" {
		return nil, false
	}
	negative := false
	if s[0] == '+' || s[0] == '-' {
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return nil, false
	}
	for i, ch := range s {
		if ch == '_' && (i == 0 || i+1 == len(s) || s[i-1] < '0' || s[i-1] > '9' || s[i+1] < '0' || s[i+1] > '9') {
			return nil, false
		}
	}
	s = strings.ReplaceAll(s, "_", "")
	exponent := 0
	if index := strings.IndexAny(s, "eEdD"); index >= 0 {
		if strings.IndexAny(s[index+1:], "eEdD") >= 0 {
			return nil, false
		}
		exponentText := s[index+1:]
		exponentValue := new(big.Int)
		if _, ok := exponentValue.SetString(exponentText, 10); !ok {
			return nil, false
		}
		if !exponentValue.IsInt64() {
			if exponentValue.Sign() > 0 {
				if negative {
					return &bigDecimalData{special: bigDecimalNegativeInfinity}, true
				}
				return &bigDecimalData{special: bigDecimalPositiveInfinity}, true
			}
			return normalizeBigDecimal(big.NewInt(0), 0, negative), true
		}
		parsed := exponentValue.Int64()
		if parsed > int64(^uint(0)>>1) || parsed < -int64(^uint(0)>>1)-1 {
			if parsed > 0 {
				if negative {
					return &bigDecimalData{special: bigDecimalNegativeInfinity}, true
				}
				return &bigDecimalData{special: bigDecimalPositiveInfinity}, true
			}
			return normalizeBigDecimal(big.NewInt(0), 0, negative), true
		}
		exponent = int(parsed)
		s = s[:index]
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 || (len(parts) == 1 && parts[0] == "") || (len(parts) == 2 && parts[0] == "" && parts[1] == "") {
		return nil, false
	}
	fractionDigits := 0
	if len(parts) == 2 {
		fractionDigits = len(parts[1])
		s = parts[0] + parts[1]
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return nil, false
		}
	}
	coefficient := new(big.Int)
	if _, ok := coefficient.SetString(s, 10); !ok {
		return nil, false
	}
	if negative {
		coefficient.Neg(coefficient)
	}
	data := normalizeBigDecimal(coefficient, exponent-fractionDigits, negative)
	data.precision = len(strings.TrimLeft(s, "0"))
	if data.precision == 0 {
		data.precision = 1
	}
	return data, true
}

func normalizeBigDecimal(coefficient *big.Int, exponent int, negativeZero bool) *bigDecimalData {
	coefficient = new(big.Int).Set(coefficient)
	if coefficient.Sign() == 0 {
		return &bigDecimalData{coefficient: coefficient, exponent: 0, negativeZero: negativeZero, precision: 1}
	}
	ten := big.NewInt(10)
	quotient := new(big.Int)
	remainder := new(big.Int)
	for {
		quotient.QuoRem(coefficient, ten, remainder)
		if remainder.Sign() != 0 {
			break
		}
		coefficient.Set(quotient)
		exponent++
	}
	return &bigDecimalData{coefficient: coefficient, exponent: exponent, precision: len(new(big.Int).Abs(coefficient).String())}
}

func bigDecimalValue(data *bigDecimalData) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueObject, Data: data, Class: R.Classes["BigDecimal"]}
}

func bigDecimalDataFrom(value *object.EmeraldValue) *bigDecimalData {
	if value == nil || value.Class != R.Classes["BigDecimal"] {
		return nil
	}
	data, _ := value.Data.(*bigDecimalData)
	return data
}

func cloneBigDecimalData(data *bigDecimalData) *bigDecimalData {
	copy := *data
	if data.coefficient != nil {
		copy.coefficient = new(big.Int).Set(data.coefficient)
	}
	return &copy
}

func bigDecimalToRat(data *bigDecimalData) *big.Rat {
	if data == nil || data.special != bigDecimalFinite {
		return nil
	}
	rat := new(big.Rat).SetInt(data.coefficient)
	power := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt(data.exponent))), nil)
	if data.exponent >= 0 {
		rat.Mul(rat, new(big.Rat).SetInt(power))
	} else {
		rat.Quo(rat, new(big.Rat).SetInt(power))
	}
	return rat
}

func bigDecimalEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return R.FalseVal
	}
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(args[0])
	if !ok || left.special == bigDecimalNaN || right.special == bigDecimalNaN {
		return R.FalseVal
	}
	if left.special != right.special {
		return R.FalseVal
	}
	if left.special != bigDecimalFinite {
		return R.TrueVal
	}
	return boolValue(bigDecimalCmpData(left, right) == 0)
}

func bigDecimalEql(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalEqual(receiver, args...)
}

func bigDecimalCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return R.NilVal
	}
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(args[0])
	if !ok {
		if args[0] != nil && CallMethod != nil && receiverHasCallableMethod(args[0], "coerce") {
			return bigDecimalApplyCoerce(receiver, args[0], "<=>")
		}
		return R.NilVal
	}
	if left.special == bigDecimalNaN || right.special == bigDecimalNaN {
		return R.NilVal
	}
	if left.special != bigDecimalFinite || right.special != bigDecimalFinite {
		return newInt(int64(bigDecimalSpecialCompare(left.special, right.special)))
	}
	return newInt(int64(bigDecimalCmpData(left, right)))
}

func bigDecimalCmpData(left, right *bigDecimalData) int {
	if left.special != bigDecimalFinite || right.special != bigDecimalFinite {
		return bigDecimalSpecialCompare(left.special, right.special)
	}
	leftSign, rightSign := left.coefficient.Sign(), right.coefficient.Sign()
	if leftSign < rightSign {
		return -1
	}
	if leftSign > rightSign {
		return 1
	}
	if leftSign == 0 {
		return 0
	}
	leftDigits := new(big.Int).Abs(left.coefficient).String()
	rightDigits := new(big.Int).Abs(right.coefficient).String()
	leftMagnitude := len(leftDigits) + left.exponent
	rightMagnitude := len(rightDigits) + right.exponent
	comparison := 0
	if leftMagnitude < rightMagnitude {
		comparison = -1
	} else if leftMagnitude > rightMagnitude {
		comparison = 1
	} else {
		width := len(leftDigits)
		if len(rightDigits) > width {
			width = len(rightDigits)
		}
		comparison = strings.Compare(leftDigits+strings.Repeat("0", width-len(leftDigits)), rightDigits+strings.Repeat("0", width-len(rightDigits)))
	}
	if leftSign < 0 {
		return -comparison
	}
	return comparison
}

func bigDecimalSpecialCompare(left, right int) int {
	rank := func(value int) int {
		switch value {
		case bigDecimalNegativeInfinity:
			return -1
		case bigDecimalPositiveInfinity:
			return 1
		default:
			return 0
		}
	}
	l, r := rank(left), rank(right)
	if l < r {
		return -1
	}
	if l > r {
		return 1
	}
	return 0
}

func bigDecimalToS(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	if data == nil {
		return bigDecimalASCIIString("")
	}
	switch data.special {
	case bigDecimalNaN:
		return bigDecimalASCIIString("NaN")
	case bigDecimalPositiveInfinity:
		return bigDecimalASCIIString("Infinity")
	case bigDecimalNegativeInfinity:
		return bigDecimalASCIIString("-Infinity")
	}
	if data.coefficient.Sign() == 0 {
		if data.negativeZero {
			return bigDecimalASCIIString("-0.0")
		}
		return bigDecimalASCIIString("0.0")
	}
	fixed, leading, group := false, "", 0
	if len(args) == 1 && args[0] != nil {
		if args[0].Type == object.ValueInteger {
			value, _ := valueToInteger(args[0])
			if value > 0 {
				group = int(value)
			}
		} else if args[0].Type == object.ValueString {
			format := args[0].Data.(string)
			fixed = strings.Contains(strings.ToUpper(format), "F")
			if strings.Contains(format, "+") {
				leading = "+"
			} else if strings.Contains(format, " ") {
				leading = " "
			}
			digitText := strings.Trim(format, "+- FfEe")
			if parsed, err := strconv.Atoi(digitText); err == nil && parsed > 0 {
				group = parsed
			}
		}
	}
	if fixed {
		formatted := bigDecimalFixedString(data)
		negative := strings.HasPrefix(formatted, "-")
		body := strings.TrimPrefix(formatted, "-")
		parts := strings.SplitN(body, ".", 2)
		if group > 0 {
			parts[0] = bigDecimalGroupRight(parts[0], group)
			parts[1] = bigDecimalGroupLeft(parts[1], group)
		}
		if negative {
			leading = "-"
		}
		return bigDecimalASCIIString(leading + parts[0] + "." + parts[1])
	}
	digits := new(big.Int).Abs(data.coefficient).String()
	sign := leading
	if data.coefficient.Sign() < 0 {
		sign = "-"
	}
	if group > 0 {
		digits = bigDecimalGroupLeft(digits, group)
	}
	exponent := len(new(big.Int).Abs(data.coefficient).String()) + data.exponent
	return bigDecimalASCIIString(sign + "0." + digits + "e" + strconv.Itoa(exponent))
}

func bigDecimalGroupLeft(value string, size int) string {
	if size <= 0 || len(value) <= size {
		return value
	}
	parts := make([]string, 0, (len(value)+size-1)/size)
	for len(value) > size {
		parts = append(parts, value[:size])
		value = value[size:]
	}
	parts = append(parts, value)
	return strings.Join(parts, " ")
}

func bigDecimalGroupRight(value string, size int) string {
	if size <= 0 || len(value) <= size {
		return value
	}
	first := len(value) % size
	if first == 0 {
		first = size
	}
	parts := []string{value[:first]}
	for index := first; index < len(value); index += size {
		parts = append(parts, value[index:index+size])
	}
	return strings.Join(parts, " ")
}

func bigDecimalASCIIString(value string) *object.EmeraldValue {
	result := rubyString(value)
	result.Encoding = "US-ASCII"
	return result
}

func bigDecimalFiniteMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(bigDecimalDataFrom(receiver).special == bigDecimalFinite)
}

func bigDecimalNaNMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return boolValue(bigDecimalDataFrom(receiver).special == bigDecimalNaN)
}

func bigDecimalInfiniteMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	switch bigDecimalDataFrom(receiver).special {
	case bigDecimalPositiveInfinity:
		return newInt(1)
	case bigDecimalNegativeInfinity:
		return newInt(-1)
	default:
		return R.NilVal
	}
}

func bigDecimalZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	return boolValue(data.special == bigDecimalFinite && data.coefficient.Sign() == 0)
}

func bigDecimalSign(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	switch data.special {
	case bigDecimalNaN:
		return newInt(0)
	case bigDecimalPositiveInfinity:
		return newInt(3)
	case bigDecimalNegativeInfinity:
		return newInt(-3)
	}
	if data.coefficient.Sign() == 0 {
		if data.negativeZero {
			return newInt(-1)
		}
		return newInt(1)
	}
	if data.coefficient.Sign() < 0 {
		return newInt(-2)
	}
	return newInt(2)
}

func bigDecimalPrecision(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(int64(bigDecimalDataFrom(receiver).precision))
}

func bigDecimalToF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	switch data.special {
	case bigDecimalNaN:
		return newFloat(math.NaN())
	case bigDecimalPositiveInfinity:
		return newFloat(math.Inf(1))
	case bigDecimalNegativeInfinity:
		return newFloat(math.Inf(-1))
	}
	value, _ := bigDecimalToRat(data).Float64()
	if value == 0 && data.negativeZero {
		value = math.Copysign(0, -1)
	}
	return newFloat(value)
}

func bigDecimalToI(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite {
		return newRuntimeException(R.Classes["FloatDomainError"], bigDecimalString(data))
	}
	rat := bigDecimalToRat(data)
	integer := new(big.Int).Quo(rat.Num(), rat.Denom())
	if integer.IsInt64() {
		return newInt(integer.Int64())
	}
	return NewIntegerFromBigInt(integer)
}

func bigDecimalString(data *bigDecimalData) string {
	switch data.special {
	case bigDecimalNaN:
		return "NaN"
	case bigDecimalPositiveInfinity:
		return "Infinity"
	case bigDecimalNegativeInfinity:
		return "-Infinity"
	default:
		return "BigDecimal"
	}
}

func bigDecimalExponent(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite || data.coefficient.Sign() == 0 {
		return newInt(0)
	}
	digits := len(new(big.Int).Abs(data.coefficient).String())
	return newInt(int64(digits + data.exponent))
}

func bigDecimalFix(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 0 {
		return NewArgumentError("wrong number of arguments")
	}
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite || data.exponent >= 0 {
		return bigDecimalValue(cloneBigDecimalData(data))
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-data.exponent)), nil)
	coefficient := new(big.Int).Quo(data.coefficient, divisor)
	result := normalizeBigDecimal(coefficient, 0, data.negativeZero || data.coefficient.Sign() < 0)
	return bigDecimalValue(result)
}

func bigDecimalFrac(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite {
		return bigDecimalValue(cloneBigDecimalData(data))
	}
	fixed := bigDecimalDataFrom(bigDecimalFix(receiver))
	return bigDecimalValue(bigDecimalAddData(data, negateBigDecimalData(fixed)))
}

func bigDecimalToR(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite {
		return newRuntimeException(R.Classes["FloatDomainError"], bigDecimalString(data))
	}
	rat := bigDecimalToRat(data)
	return newRationalValue(rat.Num(), rat.Denom())
}

func bigDecimalHash(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	hasher := fnv.New64a()
	if data.special != bigDecimalFinite {
		_, _ = hasher.Write([]byte(strconv.Itoa(data.special)))
	} else if data.coefficient.Sign() == 0 {
		_, _ = hasher.Write([]byte("0"))
	} else {
		_, _ = hasher.Write([]byte(data.coefficient.String()))
		_, _ = hasher.Write([]byte{'e'})
		_, _ = hasher.Write([]byte(strconv.Itoa(data.exponent)))
	}
	return NewIntegerFromBigInt(new(big.Int).SetUint64(hasher.Sum64()))
}

func bigDecimalQuo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	precision := bigDecimalGlobalLimit
	if precision <= 0 {
		precision = 36
	}
	return bigDecimalDivideValue(receiver, args[0], precision, false)
}

func bigDecimalDiv(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	if len(args) == 1 {
		left := bigDecimalDataFrom(receiver)
		right, ok := bigDecimalNumericFromValue(args[0])
		if !ok {
			return bigDecimalApplyCoerce(receiver, args[0], "div")
		}
		if left.special != bigDecimalFinite || right.special == bigDecimalNaN {
			return newRuntimeException(R.Classes["FloatDomainError"], "NaN or Infinity")
		}
		if right.special != bigDecimalFinite {
			return newInt(0)
		}
		if right.coefficient.Sign() == 0 {
			return newRuntimeException(R.Classes["ZeroDivisionError"], "divided by 0")
		}
		return NewIntegerFromBigInt(bigDecimalRatQuotient(bigDecimalToRat(left), bigDecimalToRat(right), true))
	}
	precision, ok := valueToInteger(args[1])
	if !ok || precision < 0 {
		return NewArgumentError("invalid precision")
	}
	effectivePrecision := int(precision)
	if effectivePrecision == 0 {
		effectivePrecision = bigDecimalGlobalLimit
	}
	if effectivePrecision == 0 {
		effectivePrecision = 36
	}
	return bigDecimalDivideValue(receiver, args[0], effectivePrecision, true)
}

func bigDecimalDivideValue(receiver, divisor *object.EmeraldValue, precision int, explicitPrecision bool) *object.EmeraldValue {
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(divisor)
	if !ok {
		return bigDecimalApplyCoerce(receiver, divisor, "/")
	}
	if left.special == bigDecimalNaN || right.special == bigDecimalNaN || (left.special != bigDecimalFinite && right.special != bigDecimalFinite) {
		return bigDecimalResult(&bigDecimalData{special: bigDecimalNaN}, false)
	}
	if right.special != bigDecimalFinite {
		return bigDecimalResult(normalizeBigDecimal(big.NewInt(0), 0, bigDecimalNegative(left) != bigDecimalNegative(right)), false)
	}
	if right.coefficient.Sign() == 0 {
		if left.special == bigDecimalFinite && left.coefficient.Sign() == 0 {
			return bigDecimalResult(&bigDecimalData{special: bigDecimalNaN}, true)
		}
		negative := bigDecimalNegative(left) != bigDecimalNegative(right)
		if negative {
			return bigDecimalResult(&bigDecimalData{special: bigDecimalNegativeInfinity}, true)
		}
		return bigDecimalResult(&bigDecimalData{special: bigDecimalPositiveInfinity}, true)
	}
	if left.special != bigDecimalFinite {
		negative := bigDecimalNegative(left) != bigDecimalNegative(right)
		if negative {
			return bigDecimalResult(&bigDecimalData{special: bigDecimalNegativeInfinity}, false)
		}
		return bigDecimalResult(&bigDecimalData{special: bigDecimalPositiveInfinity}, false)
	}
	if precision <= 0 {
		precision = 36
	}
	rat := new(big.Rat).Quo(bigDecimalToRat(left), bigDecimalToRat(right))
	return bigDecimalResult(bigDecimalFromRat(rat, precision), false)
}

func bigDecimalDivmod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(args[0])
	if !ok {
		return bigDecimalApplyCoerce(receiver, args[0], "divmod")
	}
	if right.special == bigDecimalFinite && right.coefficient.Sign() == 0 {
		return newRuntimeException(R.Classes["ZeroDivisionError"], "divided by 0")
	}
	if left.special == bigDecimalNaN || right.special == bigDecimalNaN {
		nan := bigDecimalValue(&bigDecimalData{special: bigDecimalNaN})
		return bigDecimalPair(nan, bigDecimalValue(&bigDecimalData{special: bigDecimalNaN}))
	}
	if left.special != bigDecimalFinite {
		quotient := cloneBigDecimalData(left)
		if bigDecimalNegative(right) {
			quotient = negateBigDecimalData(quotient)
		}
		return bigDecimalPair(bigDecimalValue(quotient), bigDecimalValue(&bigDecimalData{special: bigDecimalNaN}))
	}
	if right.special != bigDecimalFinite {
		if !bigDecimalNegative(left) == !bigDecimalNegative(right) {
			return bigDecimalPair(bigDecimalValue(normalizeBigDecimal(big.NewInt(0), 0, false)), receiver)
		}
		return bigDecimalPair(bigDecimalValue(normalizeBigDecimal(big.NewInt(-1), 0, false)), bigDecimalValue(cloneBigDecimalData(right)))
	}
	leftRat, rightRat := bigDecimalToRat(left), bigDecimalToRat(right)
	divisionPrecision := bigDecimalGlobalLimit
	if divisionPrecision <= 0 {
		divisionPrecision = 36
	}
	roundedDivision := bigDecimalFromRat(new(big.Rat).Quo(leftRat, rightRat), divisionPrecision)
	quotient := bigDecimalRatQuotient(bigDecimalToRat(roundedDivision), big.NewRat(1, 1), true)
	remainder := new(big.Rat).Sub(leftRat, new(big.Rat).Mul(new(big.Rat).SetInt(quotient), rightRat))
	return bigDecimalPair(bigDecimalValue(normalizeBigDecimal(new(big.Int).Set(quotient), 0, false)), bigDecimalValue(bigDecimalFromExactRat(remainder)))
}

func bigDecimalModulo(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := bigDecimalDivmod(receiver, args...)
	if result == nil || result.Type != object.ValueArray {
		return result
	}
	values := result.Data.([]*object.EmeraldValue)
	return values[1]
}

func bigDecimalRemainder(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(args[0])
	if !ok {
		return bigDecimalApplyCoerce(receiver, args[0], "remainder")
	}
	if right.special == bigDecimalFinite && right.coefficient.Sign() == 0 {
		return newRuntimeException(R.Classes["ZeroDivisionError"], "divided by 0")
	}
	if left.special != bigDecimalFinite || right.special == bigDecimalNaN {
		return bigDecimalValue(&bigDecimalData{special: bigDecimalNaN})
	}
	if right.special != bigDecimalFinite {
		return receiver
	}
	leftRat, rightRat := bigDecimalToRat(left), bigDecimalToRat(right)
	quotient := bigDecimalRatQuotient(leftRat, rightRat, false)
	remainder := new(big.Rat).Sub(leftRat, new(big.Rat).Mul(new(big.Rat).SetInt(quotient), rightRat))
	return bigDecimalValue(bigDecimalFromExactRat(remainder))
}

func bigDecimalRatQuotient(left, right *big.Rat, floor bool) *big.Int {
	value := new(big.Rat).Quo(left, right)
	quotient := new(big.Int).Quo(value.Num(), value.Denom())
	if floor && value.Sign() < 0 && new(big.Int).Rem(value.Num(), value.Denom()).Sign() != 0 {
		quotient.Sub(quotient, big.NewInt(1))
	}
	return quotient
}

func bigDecimalFromExactRat(value *big.Rat) *bigDecimalData {
	denominator := new(big.Int).Set(value.Denom())
	twoCount, fiveCount := 0, 0
	rem := new(big.Int)
	for new(big.Int).QuoRem(denominator, big.NewInt(2), rem); rem.Sign() == 0; new(big.Int).QuoRem(denominator, big.NewInt(2), rem) {
		denominator.Quo(denominator, big.NewInt(2))
		twoCount++
	}
	for new(big.Int).QuoRem(denominator, big.NewInt(5), rem); rem.Sign() == 0; new(big.Int).QuoRem(denominator, big.NewInt(5), rem) {
		denominator.Quo(denominator, big.NewInt(5))
		fiveCount++
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return bigDecimalFromRat(value, 36)
	}
	places := twoCount
	if fiveCount > places {
		places = fiveCount
	}
	coefficient := new(big.Int).Set(value.Num())
	if twoCount < places {
		coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(places-twoCount)), nil))
	}
	if fiveCount < places {
		coefficient.Mul(coefficient, new(big.Int).Exp(big.NewInt(5), big.NewInt(int64(places-fiveCount)), nil))
	}
	return normalizeBigDecimal(coefficient, -places, false)
}

func bigDecimalPair(first, second *object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{first, second}, Class: R.Classes["Array"]}
}

func bigDecimalFloor(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalQuantize(receiver, args, true)
}

func bigDecimalTruncate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalQuantize(receiver, args, false)
}

func bigDecimalQuantize(receiver *object.EmeraldValue, args []*object.EmeraldValue, floor bool) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError("wrong number of arguments")
	}
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite {
		if len(args) == 0 || floor {
			return newRuntimeException(R.Classes["FloatDomainError"], bigDecimalString(data))
		}
		return receiver
	}
	places := 0
	if len(args) == 1 {
		parsed, ok := valueToInteger(args[0])
		if !ok {
			return NewTypeError("not an integer")
		}
		places = int(parsed)
	}
	targetExponent := -places
	result := cloneBigDecimalData(data)
	if data.exponent < targetExponent {
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(targetExponent-data.exponent)), nil)
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(data.coefficient, divisor, remainder)
		if floor && data.coefficient.Sign() < 0 && remainder.Sign() != 0 {
			quotient.Sub(quotient, big.NewInt(1))
		}
		result = normalizeBigDecimal(quotient, targetExponent, false)
	}
	if len(args) == 0 {
		return NewIntegerFromBigInt(bigDecimalRatQuotient(bigDecimalToRat(result), big.NewRat(1, 1), floor))
	}
	return bigDecimalValue(result)
}

func bigDecimalGreaterThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalRelational(receiver, args, ">", func(value int) bool { return value > 0 })
}

func bigDecimalGreaterEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalRelational(receiver, args, ">=", func(value int) bool { return value >= 0 })
}

func bigDecimalLessThan(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalRelational(receiver, args, "<", func(value int) bool { return value < 0 })
}

func bigDecimalLessEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalRelational(receiver, args, "<=", func(value int) bool { return value <= 0 })
}

func bigDecimalRelational(receiver *object.EmeraldValue, args []*object.EmeraldValue, method string, predicate func(int) bool) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(args[0])
	if !ok {
		if args[0] != nil && CallMethod != nil && receiverHasCallableMethod(args[0], "coerce") {
			return bigDecimalApplyCoerce(receiver, args[0], method)
		}
		return NewArgumentError("comparison of BigDecimal with value failed")
	}
	if left.special == bigDecimalNaN || right.special == bigDecimalNaN {
		return R.FalseVal
	}
	return boolValue(predicate(bigDecimalCmpData(left, right)))
}

func bigDecimalLimit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError("wrong number of arguments")
	}
	old := bigDecimalGlobalLimit
	if len(args) == 1 && args[0] != nil && args[0].Type != object.ValueNil {
		value, ok := valueToInteger(args[0])
		if !ok || value < 0 {
			return NewArgumentError("argument must be positive")
		}
		bigDecimalGlobalLimit = int(value)
	}
	return newInt(int64(old))
}

func bigDecimalMode(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	flag, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("first argument must be an Integer")
	}
	if flag == 0x100 {
		if len(args) == 1 {
			return newInt(bigDecimalRoundMode)
		}
		mode, valid := bigDecimalRoundingMode(args[1])
		if !valid {
			return NewArgumentError("invalid rounding mode")
		}
		bigDecimalRoundMode = mode
		return newInt(mode)
	}
	if len(args) == 1 {
		return boolValue(bigDecimalExceptions[flag])
	}
	bigDecimalExceptions[flag] = args[1] != nil && args[1] != R.FalseVal && args[1].Type != object.ValueNil
	return boolValue(bigDecimalExceptions[flag])
}

func bigDecimalModeEnabled(flag int64) bool {
	return bigDecimalExceptions[flag] || bigDecimalExceptions[0xff]
}

func bigDecimalResult(data *bigDecimalData, zeroDivision bool) *object.EmeraldValue {
	if data != nil {
		if data.special == bigDecimalNaN && bigDecimalModeEnabled(0x02) {
			return newRuntimeException(R.Classes["FloatDomainError"], "Computation results in NaN")
		}
		if (data.special == bigDecimalPositiveInfinity || data.special == bigDecimalNegativeInfinity) && (bigDecimalModeEnabled(0x01) || zeroDivision && bigDecimalModeEnabled(0x10)) {
			return newRuntimeException(R.Classes["FloatDomainError"], "Computation results in Infinity")
		}
	}
	return bigDecimalValue(data)
}

func bigDecimalRoundingMode(value *object.EmeraldValue) (int64, bool) {
	if mode, ok := valueToInteger(value); ok && mode >= 1 && mode <= 7 {
		return mode, true
	}
	if value == nil || value.Type != object.ValueSymbol {
		return 0, false
	}
	name, _ := value.Data.(string)
	switch name {
	case "up":
		return 1, true
	case "down", "truncate":
		return 2, true
	case "half_up", "default":
		return 3, true
	case "half_down":
		return 4, true
	case "ceiling", "ceil":
		return 5, true
	case "floor":
		return 6, true
	case "half_even", "banker":
		return 7, true
	default:
		return 0, false
	}
}

func bigDecimalRound(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite {
		if len(args) == 0 {
			return newRuntimeException(R.Classes["FloatDomainError"], bigDecimalString(data))
		}
		return receiver
	}
	places := 0
	if len(args) >= 1 {
		value, ok := valueToInteger(args[0])
		if !ok {
			return NewTypeError("not an integer")
		}
		places = int(value)
	}
	mode := bigDecimalRoundMode
	if len(args) == 2 {
		var ok bool
		mode, ok = bigDecimalRoundingMode(args[1])
		if !ok {
			name := "unknown"
			if args[1] != nil && args[1].Type == object.ValueSymbol {
				name, _ = args[1].Data.(string)
			}
			return NewArgumentError("invalid rounding mode (" + name + ")")
		}
	}
	result := bigDecimalRoundPlaces(data, places, mode)
	if len(args) == 0 {
		return NewIntegerFromBigInt(bigDecimalRatQuotient(bigDecimalToRat(result), big.NewRat(1, 1), false))
	}
	return bigDecimalValue(result)
}

func bigDecimalCeil(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return NewArgumentError("wrong number of arguments")
	}
	data := bigDecimalDataFrom(receiver)
	if data.special != bigDecimalFinite {
		return newRuntimeException(R.Classes["FloatDomainError"], bigDecimalString(data))
	}
	places := 0
	if len(args) == 1 {
		value, ok := valueToInteger(args[0])
		if !ok {
			return NewTypeError("not an integer")
		}
		places = int(value)
	}
	result := bigDecimalRoundPlaces(data, places, 5)
	if len(args) == 0 {
		return NewIntegerFromBigInt(bigDecimalRatQuotient(bigDecimalToRat(result), big.NewRat(1, 1), false))
	}
	return bigDecimalValue(result)
}

func bigDecimalSplit(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := bigDecimalDataFrom(receiver)
	sign := int64(1)
	digits := "0"
	exponent := int64(0)
	switch data.special {
	case bigDecimalNaN:
		sign = 0
		digits = "NaN"
	case bigDecimalPositiveInfinity:
		digits = "Infinity"
	case bigDecimalNegativeInfinity:
		sign = -1
		digits = "Infinity"
	default:
		if data.coefficient.Sign() < 0 || data.negativeZero {
			sign = -1
		}
		if data.coefficient.Sign() != 0 {
			digits = new(big.Int).Abs(data.coefficient).String()
			exponent = int64(len(digits) + data.exponent)
		}
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{newInt(sign), rubyString(digits), newInt(10), newInt(exponent)}, Class: R.Classes["Array"]}
}

func bigDecimalPower(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	exponent, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("exponent must be an Integer")
	}
	data := bigDecimalDataFrom(receiver)
	if exponent == 0 {
		return bigDecimalValue(normalizeBigDecimal(big.NewInt(1), 0, false))
	}
	if data.special == bigDecimalNaN {
		return bigDecimalResult(&bigDecimalData{special: bigDecimalNaN}, false)
	}
	if data.special != bigDecimalFinite {
		if exponent < 0 {
			return bigDecimalValue(normalizeBigDecimal(big.NewInt(0), 0, false))
		}
		negative := data.special == bigDecimalNegativeInfinity && exponent%2 != 0
		if negative {
			return bigDecimalValue(&bigDecimalData{special: bigDecimalNegativeInfinity})
		}
		return bigDecimalValue(&bigDecimalData{special: bigDecimalPositiveInfinity})
	}
	if data.coefficient.Sign() == 0 && exponent < 0 {
		negative := data.negativeZero && exponent%2 != 0
		if negative {
			return bigDecimalValue(&bigDecimalData{special: bigDecimalNegativeInfinity})
		}
		return bigDecimalValue(&bigDecimalData{special: bigDecimalPositiveInfinity})
	}
	if exponent > 0 {
		maxInt := int64(^uint(0) >> 1)
		if data.exponent != 0 && (exponent > 0 && (int64(data.exponent) > maxInt/exponent || int64(data.exponent) < (-maxInt-1)/exponent)) {
			negative := data.coefficient.Sign() < 0 && exponent%2 != 0
			if data.exponent > 0 {
				if negative {
					return bigDecimalResult(&bigDecimalData{special: bigDecimalNegativeInfinity}, false)
				}
				return bigDecimalResult(&bigDecimalData{special: bigDecimalPositiveInfinity}, false)
			}
			return bigDecimalResult(normalizeBigDecimal(big.NewInt(0), 0, negative), false)
		}
		coefficient := new(big.Int).Exp(data.coefficient, big.NewInt(exponent), nil)
		result := normalizeBigDecimal(coefficient, data.exponent*int(exponent), false)
		return bigDecimalResult(bigDecimalRoundSignificant(result, bigDecimalGlobalLimit, bigDecimalRoundMode), false)
	}
	positiveExponent := -exponent
	coefficient := new(big.Int).Exp(data.coefficient, big.NewInt(positiveExponent), nil)
	positive := normalizeBigDecimal(coefficient, data.exponent*int(positiveExponent), false)
	rat := new(big.Rat).Inv(bigDecimalToRat(positive))
	precision := data.precision + 48
	if precision < 64 {
		precision = 64
	}
	return bigDecimalValue(bigDecimalFromRat(rat, precision))
}

func bigDecimalSqrt(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	precision, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("precision must be an Integer")
	}
	if precision < 0 {
		return NewArgumentError("precision must be positive")
	}
	data := bigDecimalDataFrom(receiver)
	if data.special == bigDecimalPositiveInfinity {
		return receiver
	}
	if data.special != bigDecimalFinite || data.coefficient.Sign() < 0 {
		return newRuntimeException(R.Classes["FloatDomainError"], bigDecimalString(data))
	}
	if data.coefficient.Sign() == 0 {
		return bigDecimalValue(normalizeBigDecimal(big.NewInt(0), 0, false))
	}
	if precision < 1 {
		precision = 1
	}
	bits := uint(precision*4 + 96)
	value := new(big.Float).SetPrec(bits).SetMode(big.ToNearestEven).SetRat(bigDecimalToRat(data))
	root := new(big.Float).SetPrec(bits).SetMode(big.ToNearestEven).Sqrt(value)
	parsed, valid := parseBigDecimal(root.Text('e', int(precision)+2))
	if !valid {
		return NewArgumentError("cannot compute square root")
	}
	return bigDecimalValue(bigDecimalRoundSignificant(parsed, int(precision)+1, bigDecimalRoundMode))
}

func bigDecimalRoundPlaces(data *bigDecimalData, places int, mode int64) *bigDecimalData {
	if data == nil || data.special != bigDecimalFinite {
		return cloneBigDecimalData(data)
	}
	targetExponent := -places
	if data.exponent >= targetExponent {
		return cloneBigDecimalData(data)
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(targetExponent-data.exponent)), nil)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(data.coefficient, divisor, remainder)
	if remainder.Sign() == 0 {
		return normalizeBigDecimal(quotient, targetExponent, data.negativeZero)
	}
	negative := data.coefficient.Sign() < 0
	absRemainder := new(big.Int).Abs(remainder)
	twiceRemainder := new(big.Int).Lsh(new(big.Int).Set(absRemainder), 1)
	halfComparison := twiceRemainder.Cmp(divisor)
	increment := false
	switch mode {
	case 1:
		increment = true
	case 2:
		increment = false
	case 3:
		increment = halfComparison >= 0
	case 4:
		increment = halfComparison > 0
	case 5:
		increment = !negative
	case 6:
		increment = negative
	case 7:
		increment = halfComparison > 0 || (halfComparison == 0 && quotient.Bit(0) == 1)
	}
	if increment {
		if negative {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return normalizeBigDecimal(quotient, targetExponent, negative && quotient.Sign() == 0)
}

func bigDecimalRoundSignificant(data *bigDecimalData, precision int, mode int64) *bigDecimalData {
	if precision <= 0 || data == nil || data.special != bigDecimalFinite || data.coefficient.Sign() == 0 {
		return data
	}
	exponent := len(new(big.Int).Abs(data.coefficient).String()) + data.exponent
	return bigDecimalRoundPlaces(data, precision-exponent, mode)
}

func bigDecimalOperationPrecision(args []*object.EmeraldValue) (int, *object.EmeraldValue) {
	precision := bigDecimalGlobalLimit
	if len(args) == 2 {
		value, ok := valueToInteger(args[1])
		if !ok {
			return 0, NewTypeError("precision must be an Integer")
		}
		if value < 0 {
			return 0, NewArgumentError("precision must be positive")
		}
		if value > 0 {
			precision = int(value)
		}
	}
	return precision, nil
}

func bigDecimalAdd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(args[0])
	if !ok {
		return bigDecimalApplyCoerce(receiver, args[0], "+")
	}
	result := bigDecimalAddData(left, right)
	precision, errVal := bigDecimalOperationPrecision(args)
	if errVal != nil {
		return errVal
	}
	return bigDecimalResult(bigDecimalRoundSignificant(result, precision, bigDecimalRoundMode), false)
}

func bigDecimalSubtract(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(args[0])
	if !ok {
		return bigDecimalApplyCoerce(receiver, args[0], "-")
	}
	result := bigDecimalAddData(left, negateBigDecimalData(right))
	precision, errVal := bigDecimalOperationPrecision(args)
	if errVal != nil {
		return errVal
	}
	return bigDecimalResult(bigDecimalRoundSignificant(result, precision, bigDecimalRoundMode), false)
}

func bigDecimalAddData(left, right *bigDecimalData) *bigDecimalData {
	if left.special == bigDecimalNaN || right.special == bigDecimalNaN {
		return &bigDecimalData{special: bigDecimalNaN}
	}
	if left.special != bigDecimalFinite || right.special != bigDecimalFinite {
		if left.special != bigDecimalFinite && right.special != bigDecimalFinite && left.special != right.special {
			return &bigDecimalData{special: bigDecimalNaN}
		}
		if left.special != bigDecimalFinite {
			return cloneBigDecimalData(left)
		}
		return cloneBigDecimalData(right)
	}
	exponent := left.exponent
	if right.exponent < exponent {
		exponent = right.exponent
	}
	leftCoefficient := scaleBigDecimalCoefficient(left.coefficient, left.exponent-exponent)
	rightCoefficient := scaleBigDecimalCoefficient(right.coefficient, right.exponent-exponent)
	coefficient := new(big.Int).Add(leftCoefficient, rightCoefficient)
	return normalizeBigDecimal(coefficient, exponent, false)
}

func bigDecimalMultiply(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	left := bigDecimalDataFrom(receiver)
	right, ok := bigDecimalNumericFromValue(args[0])
	if !ok {
		return bigDecimalApplyCoerce(receiver, args[0], "*")
	}
	if left.special == bigDecimalNaN || right.special == bigDecimalNaN {
		return bigDecimalResult(&bigDecimalData{special: bigDecimalNaN}, false)
	}
	if left.special != bigDecimalFinite || right.special != bigDecimalFinite {
		if (left.special == bigDecimalFinite && left.coefficient.Sign() == 0) || (right.special == bigDecimalFinite && right.coefficient.Sign() == 0) {
			return bigDecimalResult(&bigDecimalData{special: bigDecimalNaN}, false)
		}
		negative := bigDecimalNegative(left) != bigDecimalNegative(right)
		if negative {
			return bigDecimalResult(&bigDecimalData{special: bigDecimalNegativeInfinity}, false)
		}
		return bigDecimalResult(&bigDecimalData{special: bigDecimalPositiveInfinity}, false)
	}
	coefficient := new(big.Int).Mul(left.coefficient, right.coefficient)
	negativeZero := coefficient.Sign() == 0 && bigDecimalNegative(left) != bigDecimalNegative(right)
	result := normalizeBigDecimal(coefficient, left.exponent+right.exponent, negativeZero)
	precision, errVal := bigDecimalOperationPrecision(args)
	if errVal != nil {
		return errVal
	}
	return bigDecimalResult(bigDecimalRoundSignificant(result, precision, bigDecimalRoundMode), false)
}

func bigDecimalAbs(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data := cloneBigDecimalData(bigDecimalDataFrom(receiver))
	data.negativeZero = false
	if data.special == bigDecimalNegativeInfinity {
		data.special = bigDecimalPositiveInfinity
	}
	if data.coefficient != nil {
		data.coefficient.Abs(data.coefficient)
	}
	return bigDecimalValue(data)
}

func bigDecimalUnaryPlus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return receiver
}

func bigDecimalUnaryMinus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return bigDecimalValue(negateBigDecimalData(bigDecimalDataFrom(receiver)))
}

func negateBigDecimalData(data *bigDecimalData) *bigDecimalData {
	result := cloneBigDecimalData(data)
	switch result.special {
	case bigDecimalPositiveInfinity:
		result.special = bigDecimalNegativeInfinity
	case bigDecimalNegativeInfinity:
		result.special = bigDecimalPositiveInfinity
	case bigDecimalFinite:
		result.coefficient.Neg(result.coefficient)
		if result.coefficient.Sign() == 0 {
			result.negativeZero = !result.negativeZero
		}
	}
	return result
}

func bigDecimalNegative(data *bigDecimalData) bool {
	if data.special == bigDecimalNegativeInfinity {
		return true
	}
	return data.special == bigDecimalFinite && (data.coefficient.Sign() < 0 || data.negativeZero)
}

func scaleBigDecimalCoefficient(coefficient *big.Int, places int) *big.Int {
	if places <= 0 {
		return new(big.Int).Set(coefficient)
	}
	power := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil)
	return new(big.Int).Mul(coefficient, power)
}

func bigDecimalCoerce(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	converted, ok := bigDecimalNumericFromValue(args[0])
	if !ok {
		return NewTypeError("cannot coerce value to BigDecimal")
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{bigDecimalValue(converted), receiver}, Class: R.Classes["Array"]}
}

func bigDecimalApplyCoerce(receiver, other *object.EmeraldValue, method string) *object.EmeraldValue {
	if other == nil || CallMethod == nil || !receiverHasCallableMethod(other, "coerce") {
		return NewTypeError("cannot coerce value to BigDecimal")
	}
	coerced := CallMethod(other, "coerce", receiver)
	if coerced == nil || coerced.Type == object.ValueException {
		return coerced
	}
	if coerced.Type != object.ValueArray {
		return NewTypeError("coerce must return [x, y]")
	}
	values, _ := coerced.Data.([]*object.EmeraldValue)
	if len(values) != 2 {
		return NewTypeError("coerce must return [x, y]")
	}
	return CallMethod(values[0], method, values[1])
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
