package core

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GoLangDream/rgo/pkg/object"
)

const (
	dateItalyStart   = int64(2299161)
	dateEnglandStart = int64(2361222)
)

type dateInfinityData struct{ sign int64 }

func installDateTimeClass(objectClass, dateClass *object.Class) {
	if objectClass == nil || dateClass == nil {
		return
	}
	if existing, ok := objectClass.Constants["DateTime"]; ok && existing != nil {
		return
	}
	klass := object.NewClass("DateTime")
	klass.SuperClass = dateClass
	klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: dateTimeClassNew, Arity: -1})
	klass.DefineClassMethod("civil", &object.Method{Name: "civil", Fn: dateTimeClassNew, Arity: -1})
	klass.DefineClassMethod("jd", &object.Method{Name: "jd", Fn: dateClassJD, Arity: -1})
	klass.DefineClassMethod("now", &object.Method{Name: "now", Fn: dateTimeClassNow, Arity: -1})
	klass.DefineClassMethod("parse", &object.Method{Name: "parse", Fn: dateTimeClassParse, Arity: -1})
	klass.DefineClassMethod("rfc2822", &object.Method{Name: "rfc2822", Fn: dateTimeClassRFC2822, Arity: -1})
	klass.DefineClassMethod("rfc822", &object.Method{Name: "rfc822", Fn: dateTimeClassRFC2822, Arity: -1})
	klass.DefineMethod("hour", &object.Method{Name: "hour", Fn: dateTimeHour, Arity: 0})
	klass.DefineMethod("min", &object.Method{Name: "min", Fn: dateTimeMinute, Arity: 0})
	klass.DefineMethod("minute", &object.Method{Name: "minute", Fn: dateTimeMinute, Arity: 0})
	klass.DefineMethod("sec", &object.Method{Name: "sec", Fn: dateTimeSecond, Arity: 0})
	klass.DefineMethod("second", &object.Method{Name: "second", Fn: dateTimeSecond, Arity: 0})
	klass.DefineMethod("sec_fraction", &object.Method{Name: "sec_fraction", Fn: dateTimeSecondFraction, Arity: 0})
	klass.DefineMethod("second_fraction", &object.Method{Name: "second_fraction", Fn: dateTimeSecondFraction, Arity: 0})
	klass.DefineMethod("offset", &object.Method{Name: "offset", Fn: dateTimeOffset, Arity: 0})
	klass.DefineMethod("zone", &object.Method{Name: "zone", Fn: dateTimeZone, Arity: 0})
	klass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: dateTimeToS, Arity: 0})
	klass.DefineMethod("to_date", &object.Method{Name: "to_date", Fn: dateTimeToDate, Arity: 0})
	klass.DefineMethod("to_datetime", &object.Method{Name: "to_datetime", Fn: func(r *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue { return r }, Arity: 0})
	klass.DefineMethod("to_time", &object.Method{Name: "to_time", Fn: dateTimeToTime, Arity: 0})
	klass.DefineMethod("+", &object.Method{Name: "+", Fn: dateTimePlus, Arity: 1})
	klass.DefineMethod("-", &object.Method{Name: "-", Fn: dateTimeMinus, Arity: 1})
	R.Classes["DateTime"] = klass
	value := &object.EmeraldValue{Type: object.ValueClass, Data: klass, Class: R.Classes["Class"]}
	objectClass.DefineConstant("DateTime", value)
	AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "DateTime", value)
	if timeClass := R.Classes["Time"]; timeClass != nil {
		timeClass.DefineMethod("to_datetime", &object.Method{Name: "to_datetime", Fn: timeToDateTime, Arity: 0})
	}
	objectClass.DefineMethod("new_datetime", &object.Method{Name: "new_datetime", Fn: dateSpecNewDateTime, Arity: -1, Visibility: "private"})
}

func dateSpecNewDateTime(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	defaults := []*object.EmeraldValue{newInt(2000), newInt(1), newInt(1), newInt(0), newInt(0), newInt(0), newInt(0), newInt(dateItalyStart)}
	if len(args) > 0 && args[0] != nil && args[0].Type == object.ValueHash {
		hash := valueToHashMap(args[0])
		for i, name := range []string{"year", "month", "day", "hour", "minute", "second", "offset", "start"} {
			if value, ok := hashLookup(hash, rubySymbol(name)); ok {
				defaults[i] = value
			}
		}
	}
	receiver := &object.EmeraldValue{Type: object.ValueClass, Data: R.Classes["DateTime"], Class: R.Classes["Class"]}
	return dateTimeClassNew(receiver, defaults...)
}

func dateTimeClassNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 8 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..8)", len(args)))
	}
	values := []int64{-4712, 1, 1, 0, 0}
	for i := 0; i < len(args) && i < 5; i++ {
		if args[i] == nil || args[i].Type != object.ValueInteger {
			return argumentError("invalid date")
		}
		values[i] = args[i].Data.(int64)
	}
	year, month, day, hour, minute := values[0], values[1], values[2], values[3], values[4]
	if month < 0 {
		month += 13
	}
	if month < 1 || month > 12 {
		return argumentError("invalid date")
	}
	start := float64(dateItalyStart)
	if len(args) > 7 {
		var ok bool
		start, ok = dateStartValue(args[7])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	if day < 0 {
		day += dateDaysInMonth(year, month, start) + 1
	}
	jd, ok := dateCivilToJD(year, month, day, start)
	if !ok {
		return argumentError("invalid date")
	}
	if hour < -24 || hour > 24 {
		return argumentError("invalid hour")
	}
	if hour < 0 {
		hour += 24
	} else if hour == 24 {
		hour = 0
		jd++
	}
	if minute < -60 || minute >= 60 {
		return argumentError("invalid minute")
	}
	if minute < 0 {
		minute += 60
	}
	seconds := new(big.Rat)
	if len(args) > 5 {
		var valid bool
		seconds, valid = dateNumericRat(args[5])
		if !valid {
			return argumentError("invalid second")
		}
	}
	if seconds.Cmp(big.NewRat(-60, 1)) < 0 || seconds.Cmp(big.NewRat(60, 1)) >= 0 {
		return argumentError("invalid second")
	}
	if seconds.Sign() < 0 {
		seconds.Add(seconds, big.NewRat(60, 1))
	}
	second := dateRatFloor(seconds)
	fraction := new(big.Rat).Sub(seconds, new(big.Rat).SetInt64(second))
	offset := new(big.Rat)
	if len(args) > 6 {
		var valid bool
		offset, valid = dateOffsetRat(args[6])
		if !valid || offset.Cmp(big.NewRat(-1, 1)) < 0 || offset.Cmp(big.NewRat(1, 1)) > 0 {
			return argumentError("invalid offset")
		}
	}
	localSeconds := new(big.Rat).SetInt64(hour*3600 + minute*60 + second)
	localSeconds.Add(localSeconds, fraction)
	dayFraction := new(big.Rat).Quo(localSeconds, big.NewRat(86400, 1))
	instant := new(big.Rat).Add(new(big.Rat).SetInt64(jd), dayFraction)
	instant.Sub(instant, offset)
	result := newDateTimeFromInstant(instant, offset, start, dateReceiverClass(receiver))
	result.Data.(*dateData).secFraction = new(big.Rat).Set(fraction)
	return result
}

func dateNumericRat(value *object.EmeraldValue) (*big.Rat, bool) {
	if value == nil {
		return nil, false
	}
	if integer, ok := numericBigIntValue(value); ok && value.Type == object.ValueInteger {
		return new(big.Rat).SetInt(integer), true
	}
	if value.Type == object.ValueFloat {
		return new(big.Rat).SetFloat64(value.Data.(float64)), true
	}
	if rational, ok := value.Data.(*rationalData); ok && rational.denominator.Sign() != 0 {
		return new(big.Rat).SetFrac(new(big.Int).Set(rational.numerator), new(big.Int).Set(rational.denominator)), true
	}
	return nil, false
}
func dateOffsetRat(value *object.EmeraldValue) (*big.Rat, bool) {
	if value != nil && value.Type == object.ValueString {
		match := regexp.MustCompile(`^([+-])(\d{2}):(\d{2})(?::(\d{2}))?$`).FindStringSubmatch(value.Data.(string))
		if match == nil {
			return nil, false
		}
		h, _ := strconv.ParseInt(match[2], 10, 64)
		m, _ := strconv.ParseInt(match[3], 10, 64)
		s := int64(0)
		if match[4] != "" {
			s, _ = strconv.ParseInt(match[4], 10, 64)
		}
		total := h*3600 + m*60 + s
		if match[1] == "-" {
			total = -total
		}
		return big.NewRat(total, 86400), true
	}
	return dateNumericRat(value)
}
func dateRatFloor(value *big.Rat) int64 {
	q := new(big.Int).Quo(value.Num(), value.Denom())
	r := new(big.Int).Rem(value.Num(), value.Denom())
	if value.Sign() < 0 && r.Sign() != 0 {
		q.Sub(q, big.NewInt(1))
	}
	return q.Int64()
}

func newDateTimeFromInstant(instant, offset *big.Rat, start float64, class *object.Class) *object.EmeraldValue {
	if class == nil {
		class = R.Classes["DateTime"]
	}
	local := new(big.Rat).Add(new(big.Rat).Set(instant), offset)
	jd := dateRatFloor(local)
	fractionDay := new(big.Rat).Sub(local, new(big.Rat).SetInt64(jd))
	seconds := new(big.Rat).Mul(fractionDay, big.NewRat(86400, 1))
	whole := dateRatFloor(seconds)
	fraction := new(big.Rat).Sub(seconds, new(big.Rat).SetInt64(whole))
	year, month, day := dateJDToCivil(jd, start)
	return &object.EmeraldValue{Type: object.ValueObject, Class: class, Data: &dateData{year: year, month: month, day: day, jd: jd, start: start, hour: whole / 3600, minute: (whole % 3600) / 60, second: whole % 60, secFraction: fraction, offset: new(big.Rat).Set(offset), instant: new(big.Rat).Set(instant)}}
}

func dateTimeHour(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.hour)
}
func dateTimeMinute(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.minute)
}
func dateTimeSecond(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.second)
}
func dateTimeSecondFraction(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	if d.secFraction == nil {
		return newInt(0)
	}
	return newRationalValue(d.secFraction.Num(), d.secFraction.Denom())
}
func dateTimeOffset(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	if d.offset == nil {
		return newInt(0)
	}
	return newRationalValue(d.offset.Num(), d.offset.Denom())
}
func dateTimeZone(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return rubyString(dateOffsetString(d.offset, true))
}
func dateOffsetString(offset *big.Rat, colon bool) string {
	seconds := int64(0)
	if offset != nil {
		value := new(big.Rat).Mul(offset, big.NewRat(86400, 1))
		seconds = dateRatFloor(value)
	}
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	if colon {
		return fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, (seconds%3600)/60)
	}
	return fmt.Sprintf("%s%02d%02d", sign, seconds/3600, (seconds%3600)/60)
}
func dateTimeToS(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return rubyString(fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d%s", d.year, d.month, d.day, d.hour, d.minute, d.second, dateOffsetString(d.offset, true)))
}
func dateTimeToDate(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newDateValueFromJD(d.jd, d.start, R.Classes["Date"])
}

func dateTimeClassNow(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return argumentError("wrong number of arguments")
	}
	now := time.Now().In(effectiveLocalLocation())
	_, seconds := now.Zone()
	offset := big.NewRat(int64(seconds), 86400)
	start := float64(dateItalyStart)
	if len(args) == 1 {
		var ok bool
		start, ok = dateStartValue(args[0])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	jd := dateGregorianToJD(int64(now.Year()), int64(now.Month()), int64(now.Day()))
	localSeconds := big.NewRat(int64(now.Hour()*3600+now.Minute()*60+now.Second()), 1)
	localSeconds.Add(localSeconds, big.NewRat(int64(now.Nanosecond()), 1e9))
	instant := new(big.Rat).Add(new(big.Rat).SetInt64(jd), new(big.Rat).Quo(localSeconds, big.NewRat(86400, 1)))
	instant.Sub(instant, offset)
	return newDateTimeFromInstant(instant, offset, start, dateReceiverClass(receiver))
}

func dateTimeClassParse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 0 && args[0] != nil {
		input, err := dateParseString(args[0])
		if err != nil {
			return err
		}
		if match := regexp.MustCompile(`^(-?\d+)-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})([+-]\d{2}:\d{2})?$`).FindStringSubmatch(strings.TrimSpace(input)); match != nil {
			values := make([]*object.EmeraldValue, 0, 8)
			for i := 1; i <= 6; i++ {
				value, e := strconv.ParseInt(match[i], 10, 64)
				if e != nil {
					return dateInvalidError()
				}
				values = append(values, newInt(value))
			}
			if match[7] != "" {
				values = append(values, rubyString(match[7]))
			}
			return dateTimeClassNew(receiver, values...)
		}
	}
	parsed := dateClassParse(receiver, args...)
	if parsed == nil || parsed.Type == object.ValueException {
		return parsed
	}
	d, ok := parsed.Data.(*dateData)
	if !ok {
		return dateInvalidError()
	}
	return dateTimeClassNew(receiver, newInt(d.year), newInt(d.month), newInt(d.day))
}
func dateTimeClassRFC2822(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 || args[0] == nil || args[0] == R.NilVal {
		return dateInvalidError()
	}
	input, err := dateParseString(args[0])
	if err != nil {
		return err
	}
	if strings.TrimSpace(input) == "" {
		return dateInvalidError()
	}
	return dateInvalidError()
}

func installDateMethods(klass *object.Class) {
	klass.DefineClassMethod("new", &object.Method{Name: "new", Fn: dateClassNew, Arity: -1})
	klass.DefineClassMethod("civil", &object.Method{Name: "civil", Fn: dateCivilValue, Arity: -1})
	klass.DefineClassMethod("jd", &object.Method{Name: "jd", Fn: dateClassJD, Arity: -1})
	klass.DefineClassMethod("ordinal", &object.Method{Name: "ordinal", Fn: dateClassOrdinal, Arity: -1})
	klass.DefineClassMethod("commercial", &object.Method{Name: "commercial", Fn: dateClassCommercial, Arity: -1})
	klass.DefineClassMethod("parse", &object.Method{Name: "parse", Fn: dateClassParse, Arity: -1})
	klass.DefineClassMethod("strptime", &object.Method{Name: "strptime", Fn: dateClassStrptime, Arity: -1})
	klass.DefineClassMethod("iso8601", &object.Method{Name: "iso8601", Fn: dateClassISO8601, Arity: -1})
	klass.DefineClassMethod("_iso8601", &object.Method{Name: "_iso8601", Fn: dateClassUnderscoreISO8601, Arity: -1})
	klass.DefineClassMethod("gregorian_leap?", &object.Method{Name: "gregorian_leap?", Fn: dateClassGregorianLeap, Arity: 1})
	klass.DefineClassMethod("leap?", &object.Method{Name: "leap?", Fn: dateClassGregorianLeap, Arity: 1})
	klass.DefineClassMethod("julian_leap?", &object.Method{Name: "julian_leap?", Fn: dateClassJulianLeap, Arity: 1})
	klass.DefineClassMethod("valid_civil?", &object.Method{Name: "valid_civil?", Fn: dateClassValidCivil, Arity: -1})
	klass.DefineClassMethod("valid_date?", &object.Method{Name: "valid_date?", Fn: dateClassValidCivil, Arity: -1})
	klass.DefineClassMethod("valid_ordinal?", &object.Method{Name: "valid_ordinal?", Fn: dateClassValidOrdinal, Arity: -1})
	klass.DefineClassMethod("valid_commercial?", &object.Method{Name: "valid_commercial?", Fn: dateClassValidCommercial, Arity: -1})
	klass.DefineClassMethod("today", &object.Method{Name: "today", Fn: dateClassToday, Arity: -1})

	klass.DefineMethod("==", &object.Method{Name: "==", Fn: dateEqual, Arity: 1})
	klass.DefineMethod("eql?", &object.Method{Name: "eql?", Fn: dateEqual, Arity: 1})
	klass.DefineMethod("===", &object.Method{Name: "===", Fn: dateCompare, Arity: 1})
	klass.DefineMethod("<=>", &object.Method{Name: "<=>", Fn: dateCompare, Arity: 1})
	klass.DefineMethod("<", &object.Method{Name: "<", Fn: dateLess, Arity: 1})
	klass.DefineMethod("<=", &object.Method{Name: "<=", Fn: dateLessEqual, Arity: 1})
	klass.DefineMethod(">", &object.Method{Name: ">", Fn: dateGreater, Arity: 1})
	klass.DefineMethod(">=", &object.Method{Name: ">=", Fn: dateGreaterEqual, Arity: 1})
	klass.DefineMethod("hash", &object.Method{Name: "hash", Fn: dateHash, Arity: 0})
	klass.DefineMethod("year", &object.Method{Name: "year", Fn: dateYear, Arity: 0})
	klass.DefineMethod("month", &object.Method{Name: "month", Fn: dateMonth, Arity: 0})
	klass.DefineMethod("mon", &object.Method{Name: "mon", Fn: dateMonth, Arity: 0})
	klass.DefineMethod("day", &object.Method{Name: "day", Fn: dateDay, Arity: 0})
	klass.DefineMethod("mday", &object.Method{Name: "mday", Fn: dateDay, Arity: 0})
	klass.DefineMethod("yday", &object.Method{Name: "yday", Fn: dateYDay, Arity: 0})
	klass.DefineMethod("wday", &object.Method{Name: "wday", Fn: dateWDay, Arity: 0})
	klass.DefineMethod("cwday", &object.Method{Name: "cwday", Fn: dateCWDay, Arity: 0})
	klass.DefineMethod("cweek", &object.Method{Name: "cweek", Fn: dateCWeek, Arity: 0})
	klass.DefineMethod("cwyear", &object.Method{Name: "cwyear", Fn: dateCWYear, Arity: 0})
	klass.DefineMethod("jd", &object.Method{Name: "jd", Fn: dateJD, Arity: 0})
	klass.DefineMethod("ajd", &object.Method{Name: "ajd", Fn: dateAJD, Arity: 0})
	klass.DefineMethod("mjd", &object.Method{Name: "mjd", Fn: dateMJD, Arity: 0})
	klass.DefineMethod("amjd", &object.Method{Name: "amjd", Fn: dateMJD, Arity: 0})
	klass.DefineMethod("ld", &object.Method{Name: "ld", Fn: dateLD, Arity: 0})
	klass.DefineMethod("day_fraction", &object.Method{Name: "day_fraction", Fn: dateDayFraction, Arity: 0})
	klass.DefineMethod("start", &object.Method{Name: "start", Fn: dateStart, Arity: 0})
	klass.DefineMethod("gregorian?", &object.Method{Name: "gregorian?", Fn: dateGregorian, Arity: 0})
	klass.DefineMethod("julian?", &object.Method{Name: "julian?", Fn: dateJulian, Arity: 0})
	klass.DefineMethod("leap?", &object.Method{Name: "leap?", Fn: dateLeap, Arity: 0})
	klass.DefineMethod("+", &object.Method{Name: "+", Fn: datePlus, Arity: 1})
	klass.DefineMethod("-", &object.Method{Name: "-", Fn: dateMinus, Arity: 1})
	klass.DefineMethod(">>", &object.Method{Name: ">>", Fn: dateShiftMonths, Arity: 1})
	klass.DefineMethod("<<", &object.Method{Name: "<<", Fn: dateShiftMonthsBack, Arity: 1})
	klass.DefineMethod("succ", &object.Method{Name: "succ", Fn: dateSucc, Arity: 0})
	klass.DefineMethod("next", &object.Method{Name: "next", Fn: dateSucc, Arity: 0})
	klass.DefineMethod("next_day", &object.Method{Name: "next_day", Fn: dateNextDay, Arity: -1})
	klass.DefineMethod("prev_day", &object.Method{Name: "prev_day", Fn: datePrevDay, Arity: -1})
	klass.DefineMethod("next_month", &object.Method{Name: "next_month", Fn: dateNextMonth, Arity: -1})
	klass.DefineMethod("prev_month", &object.Method{Name: "prev_month", Fn: datePrevMonth, Arity: -1})
	klass.DefineMethod("next_year", &object.Method{Name: "next_year", Fn: dateNextYear, Arity: -1})
	klass.DefineMethod("prev_year", &object.Method{Name: "prev_year", Fn: datePrevYear, Arity: -1})
	klass.DefineMethod("new_start", &object.Method{Name: "new_start", Fn: dateNewStart, Arity: -1})
	klass.DefineMethod("italy", &object.Method{Name: "italy", Fn: dateItaly, Arity: 0})
	klass.DefineMethod("england", &object.Method{Name: "england", Fn: dateEngland, Arity: 0})
	klass.DefineMethod("julian", &object.Method{Name: "julian", Fn: dateAllJulian, Arity: 0})
	klass.DefineMethod("gregorian", &object.Method{Name: "gregorian", Fn: dateAllGregorian, Arity: 0})
	klass.DefineMethod("to_s", &object.Method{Name: "to_s", Fn: dateToS, Arity: 0})
	klass.DefineMethod("inspect", &object.Method{Name: "inspect", Fn: dateInspect, Arity: 0})
	klass.DefineMethod("strftime", &object.Method{Name: "strftime", Fn: dateStrftime, Arity: -1})
	klass.DefineMethod("step", &object.Method{Name: "step", Fn: dateStep, Arity: -1})
	klass.DefineMethod("upto", &object.Method{Name: "upto", Fn: dateUpto, Arity: 1})
	klass.DefineMethod("downto", &object.Method{Name: "downto", Fn: dateDownto, Arity: 1})
	for i, name := range []string{"sunday?", "monday?", "tuesday?", "wednesday?", "thursday?", "friday?", "saturday?"} {
		weekday := int64(i)
		klass.DefineMethod(name, &object.Method{Name: name, Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			data, ok := receiver.Data.(*dateData)
			return boolValue(ok && dateMod(data.jd+1, 7) == weekday)
		}, Arity: 0})
	}

	klass.DefineConstant("ITALY", newInt(dateItalyStart))
	klass.DefineConstant("ENGLAND", newInt(dateEnglandStart))
	installDateInfinity(klass)
	klass.DefineConstant("JULIAN", newDateInfinityValue(1))
	klass.DefineConstant("GREGORIAN", newDateInfinityValue(-1))
	defineDateNameConstants(klass)
	errorClass := object.NewClass("Date::Error")
	errorClass.SuperClass = R.Classes["ArgumentError"]
	R.Classes["Date::Error"] = errorClass
	klass.DefineConstant("Error", &object.EmeraldValue{Type: object.ValueClass, Data: errorClass, Class: R.Classes["Class"]})
	if timeClass := R.Classes["Time"]; timeClass != nil {
		timeClass.DefineMethod("to_date", &object.Method{Name: "to_date", Fn: timeToDate, Arity: 0})
	}
}

func defineDateNameConstants(klass *object.Class) {
	define := func(name string, values []string, leadingNil bool) {
		items := make([]*object.EmeraldValue, 0, len(values)+1)
		if leadingNil {
			items = append(items, R.NilVal)
		}
		for _, value := range values {
			items = append(items, frozenRubyConstantString(value))
		}
		klass.DefineConstant(name, &object.EmeraldValue{Type: object.ValueArray, Data: items, Class: R.Classes["Array"], Frozen: true})
	}
	define("MONTHNAMES", []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}, true)
	define("ABBR_MONTHNAMES", []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}, true)
	define("DAYNAMES", []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}, false)
	define("ABBR_DAYNAMES", []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}, false)
}

func dateCivilValue(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 4 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..4)", len(args)))
	}
	year, month, day := int64(-4712), int64(1), int64(1)
	var ok bool
	if len(args) > 0 {
		year, ok = valueToInteger(args[0])
		if !ok {
			return NewTypeError("invalid year")
		}
	}
	if len(args) > 1 {
		month, ok = valueToInteger(args[1])
		if !ok {
			return NewTypeError("invalid month")
		}
	}
	if len(args) > 2 {
		day, ok = valueToInteger(args[2])
		if !ok {
			return NewTypeError("invalid day")
		}
	}
	start := float64(dateItalyStart)
	if len(args) > 3 {
		start, ok = dateStartValue(args[3])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	if month < 0 {
		month += 13
	}
	if month < 1 || month > 12 {
		return argumentError("invalid date")
	}
	if day < 0 {
		day += dateDaysInMonth(year, month, start) + 1
	}
	jd, valid := dateCivilToJD(year, month, day, start)
	if !valid {
		return argumentError("invalid date")
	}
	return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
}

func dateClassJD(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 2 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..2)", len(args)))
	}
	jd := int64(0)
	var ok bool
	if len(args) > 0 {
		jd, ok = valueToInteger(args[0])
		if !ok {
			return NewTypeError("invalid julian day")
		}
	}
	start := float64(dateItalyStart)
	if len(args) > 1 {
		start, ok = dateStartValue(args[1])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
}

func dateClassOrdinal(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 3 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..3)", len(args)))
	}
	if len(args) == 0 {
		return dateClassJD(receiver)
	}
	year, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("invalid year")
	}
	yday := int64(1)
	if len(args) > 1 {
		yday, ok = valueToInteger(args[1])
		if !ok {
			return NewTypeError("invalid day")
		}
	}
	start := float64(dateItalyStart)
	if len(args) > 2 {
		start, ok = dateStartValue(args[2])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	jd, valid := dateOrdinalJD(year, yday, start)
	if !valid {
		return argumentError("invalid date")
	}
	return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
}

func dateOrdinalJD(year, yday int64, start float64) (int64, bool) {
	first, ok := dateCivilToJD(year, 1, 1, start)
	if !ok {
		return 0, false
	}
	next, ok := dateCivilToJD(year+1, 1, 1, start)
	if !ok {
		return 0, false
	}
	length := next - first
	if yday < 0 {
		yday += length + 1
	}
	if yday < 1 || yday > length {
		return 0, false
	}
	return first + yday - 1, true
}

func dateClassCommercial(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 4 {
		return NewArgumentError(fmt.Sprintf("wrong number of arguments (given %d, expected 0..4)", len(args)))
	}
	if len(args) == 0 {
		return dateClassJD(receiver)
	}
	year, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("invalid year")
	}
	week, day := int64(1), int64(1)
	if len(args) > 1 {
		week, ok = valueToInteger(args[1])
		if !ok {
			return NewTypeError("invalid week")
		}
	}
	if len(args) > 2 {
		day, ok = valueToInteger(args[2])
		if !ok {
			return NewTypeError("invalid day")
		}
	}
	start := float64(dateItalyStart)
	if len(args) > 3 {
		start, ok = dateStartValue(args[3])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	jd, valid := dateCommercialJD(year, week, day, start)
	if !valid {
		return argumentError("invalid date")
	}
	return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
}

func dateCommercialJD(year, week, day int64, start float64) (int64, bool) {
	if day < 0 {
		day += 8
	}
	if day < 1 || day > 7 || week == 0 {
		return 0, false
	}
	jan4, ok := dateCivilToJD(year, 1, 4, start)
	if !ok {
		return 0, false
	}
	jan4day := dateMod(jan4+1, 7)
	if jan4day == 0 {
		jan4day = 7
	}
	week1 := jan4 - jan4day + 1
	nextJan4, ok := dateCivilToJD(year+1, 1, 4, start)
	if !ok {
		return 0, false
	}
	nextDay := dateMod(nextJan4+1, 7)
	if nextDay == 0 {
		nextDay = 7
	}
	weekCount := (nextJan4 - nextDay + 1 - week1) / 7
	if week < 0 {
		week += weekCount + 1
	}
	if week < 1 || week > weekCount {
		return 0, false
	}
	return week1 + (week-1)*7 + day - 1, true
}

var dateOrdinalSuffix = regexp.MustCompile(`(?i)(\d)(st|nd|rd|th)\b`)

func dateParseString(value *object.EmeraldValue) (string, *object.EmeraldValue) {
	if value != nil && value.Type == object.ValueString {
		return value.Data.(string), nil
	}
	if value != nil && receiverHasCallableMethod(value, "to_str") && CallMethod != nil {
		converted := CallMethod(value, "to_str")
		if converted != nil && converted.Type == object.ValueException {
			return "", converted
		}
		if converted != nil && converted.Type == object.ValueString {
			return converted.Data.(string), nil
		}
	}
	return "", NewTypeError("no implicit conversion into String")
}

func dateClassParse(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 3 {
		return argumentError("wrong number of arguments")
	}
	input := "-4712-01-01"
	if len(args) > 0 {
		var err *object.EmeraldValue
		input, err = dateParseString(args[0])
		if err != nil {
			return err
		}
	}
	complete := true
	if len(args) > 1 && args[1] != nil && args[1].Type == object.ValueBool {
		complete = args[1].Data.(bool)
	}
	start := float64(dateItalyStart)
	if len(args) > 2 {
		var ok bool
		start, ok = dateStartValue(args[2])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	year, month, day, ordinal, ok := dateParseParts(input, complete)
	if !ok {
		return dateInvalidError()
	}
	if ordinal != 0 {
		jd, valid := dateOrdinalJD(year, ordinal, start)
		if !valid {
			return dateInvalidError()
		}
		return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
	}
	jd, valid := dateCivilToJD(year, month, day, start)
	if !valid {
		return dateInvalidError()
	}
	return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
}

func dateInvalidError() *object.EmeraldValue {
	class := R.Classes["Date::Error"]
	if class == nil {
		class = R.Classes["ArgumentError"]
	}
	return newRuntimeException(class, "invalid date")
}

func dateParseParts(source string, complete bool) (year, month, day, ordinal int64, ok bool) {
	s := strings.TrimSpace(strings.ToLower(source))
	if s == "" {
		return 0, 0, 0, 0, false
	}
	now := time.Now()
	year, month, day = int64(now.Year()), int64(now.Month()), 1
	weekdays := map[string]int64{"sunday": 7, "sun": 7, "monday": 1, "mon": 1, "tuesday": 2, "tue": 2, "wednesday": 3, "wed": 3, "thursday": 4, "thu": 4, "friday": 5, "fri": 5, "saturday": 6, "sat": 6}
	if wanted, exists := weekdays[s]; exists {
		todayJD := dateGregorianToJD(int64(now.Year()), int64(now.Month()), int64(now.Day()))
		current := dateMod(todayJD+1, 7)
		if current == 0 {
			current = 7
		}
		jd := todayJD + wanted - current
		year, month, day = dateJDToCivilUsing(jd, true)
		return year, month, day, 0, true
	}
	months := map[string]int64{"january": 1, "jan": 1, "february": 2, "feb": 2, "march": 3, "mar": 3, "april": 4, "apr": 4, "may": 5, "june": 6, "jun": 6, "july": 7, "jul": 7, "august": 8, "aug": 8, "september": 9, "sep": 9, "sept": 9, "october": 10, "oct": 10, "november": 11, "nov": 11, "december": 12, "dec": 12}
	if m, exists := months[s]; exists {
		return year, m, 1, 0, true
	}
	ordinalOnly := dateOrdinalSuffix.ReplaceAllString(s, "$1")
	if matched, _ := regexp.MatchString(`^\d{1,2}$`, ordinalOnly); matched && ordinalOnly != s {
		value, _ := strconv.ParseInt(ordinalOnly, 10, 64)
		return year, month, value, 0, true
	}
	if matched, _ := regexp.MatchString(`^\d+$`, s); matched {
		switch len(s) {
		case 1:
			return 0, 0, 0, 0, false
		case 2:
			value, _ := strconv.ParseInt(s, 10, 64)
			return year, month, value, 0, true
		case 3:
			value, _ := strconv.ParseInt(s, 10, 64)
			return year, 0, 0, value, true
		case 4:
			m, _ := strconv.ParseInt(s[:2], 10, 64)
			d, _ := strconv.ParseInt(s[2:], 10, 64)
			return year, m, d, 0, true
		case 5:
			y, _ := strconv.ParseInt(s[:2], 10, 64)
			o, _ := strconv.ParseInt(s[2:], 10, 64)
			return dateCompleteYear(y, complete), 0, 0, o, true
		case 6:
			y, _ := strconv.ParseInt(s[:2], 10, 64)
			m, _ := strconv.ParseInt(s[2:4], 10, 64)
			d, _ := strconv.ParseInt(s[4:], 10, 64)
			return dateCompleteYear(y, complete), m, d, 0, true
		case 7:
			y, _ := strconv.ParseInt(s[:4], 10, 64)
			o, _ := strconv.ParseInt(s[4:], 10, 64)
			return y, 0, 0, o, true
		case 8:
			y, _ := strconv.ParseInt(s[:4], 10, 64)
			m, _ := strconv.ParseInt(s[4:6], 10, 64)
			d, _ := strconv.ParseInt(s[6:], 10, 64)
			return y, m, d, 0, true
		}
	}
	negativeFirst := strings.HasPrefix(s, "-")
	negativeLast, _ := regexp.MatchString(`[./\s-]-\d`, s)
	s = dateOrdinalSuffix.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "--", "-")
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == '/' || r == '-' || r == ' ' || r == ',' })
	if len(fields) == 0 {
		return 0, 0, 0, 0, false
	}
	monthIndex := -1
	for i, field := range fields {
		if value, exists := months[field]; exists {
			month, monthIndex = value, i
		}
	}
	parse := func(value string) (int64, bool) { v, err := strconv.ParseInt(value, 10, 64); return v, err == nil }
	if monthIndex >= 0 {
		numbers := make([]int64, 0, 2)
		for i, field := range fields {
			if i != monthIndex {
				if value, valid := parse(field); valid {
					numbers = append(numbers, value)
				}
			}
		}
		if len(numbers) == 0 {
			return year, month, 1, 0, true
		}
		if len(numbers) == 1 {
			if numbers[0] > 31 {
				return numbers[0], month, 1, 0, true
			}
			return year, month, numbers[0], 0, true
		}
		if monthIndex == 1 && (numbers[0] > 31 || len(fields[0]) >= 4) {
			year, day = numbers[0], numbers[1]
		} else {
			day, year = numbers[0], numbers[1]
		}
		if negativeLast {
			year = -year
		}
		if negativeFirst && monthIndex > 0 {
			year = -year
		}
		return year, month, day, 0, true
	}
	if len(fields) == 3 {
		a, aok := parse(fields[0])
		b, bok := parse(fields[1])
		c, cok := parse(fields[2])
		if !aok || !bok || !cok {
			return 0, 0, 0, 0, false
		}
		if negativeFirst {
			a = -a
		}
		if len(fields[0]) >= 4 || negativeFirst {
			return a, b, c, 0, true
		}
		if len(fields[2]) >= 4 {
			return c, b, a, 0, true
		}
		return dateCompleteYear(a, complete), b, c, 0, true
	}
	return 0, 0, 0, 0, false
}

func dateCompleteYear(year int64, complete bool) int64 {
	if !complete {
		return year
	}
	if year >= 0 && year <= 68 {
		return year + 2000
	}
	if year >= 69 && year <= 99 {
		return year + 1900
	}
	return year
}

func dateClassStrptime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 3 {
		return argumentError("wrong number of arguments")
	}
	if len(args) == 0 {
		return dateClassJD(receiver)
	}
	input, err := dateParseString(args[0])
	if err != nil {
		return err
	}
	format := "%F"
	if len(args) > 1 {
		if args[1] == nil || args[1].Type != object.ValueString {
			return NewTypeError("no implicit conversion into String")
		}
		format = args[1].Data.(string)
	}
	start := float64(dateItalyStart)
	if len(args) > 2 {
		var ok bool
		start, ok = dateStartValue(args[2])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	parts, ok := dateStrptimeParts(input, format)
	if !ok {
		return dateInvalidError()
	}
	now := time.Now()
	year, month, day := int64(now.Year()), int64(now.Month()), int64(1)
	_, hasY := parts['Y']
	_, hasShortY := parts['y']
	_, hasCentury := parts['C']
	if hasY || hasShortY || hasCentury {
		month = 1
	}
	if y, exists := parts['Y']; exists {
		year = y
	}
	if y, exists := parts['y']; exists {
		if c, has := parts['C']; has {
			year = c*100 + y
		} else {
			year = dateCompleteYear(y, true)
		}
	} else if c, exists := parts['C']; exists {
		year = c * 100
	}
	if m, exists := parts['m']; exists {
		month = m
	}
	if d, exists := parts['d']; exists {
		day = d
	}
	if ordinal, exists := parts['j']; exists {
		jd, valid := dateOrdinalJD(year, ordinal, start)
		if !valid {
			return dateInvalidError()
		}
		return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
	}
	if _, hasG := parts['G']; hasG {
		year = parts['G']
		week := int64(1)
		if v, exists := parts['V']; exists {
			week = v
		}
		weekday := int64(1)
		if v, exists := parts['u']; exists {
			weekday = v
		}
		jd, valid := dateCommercialJD(year, week, weekday, start)
		if !valid {
			return dateInvalidError()
		}
		return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
	}
	if g, has := parts['g']; has {
		year = dateCompleteYear(g, true)
		week := int64(1)
		if v, exists := parts['V']; exists {
			week = v
		}
		weekday := int64(1)
		if v, exists := parts['u']; exists {
			weekday = v
		}
		jd, valid := dateCommercialJD(year, week, weekday, start)
		if !valid {
			return dateInvalidError()
		}
		return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
	}
	if week, has := parts['V']; has {
		todayJD := dateGregorianToJD(int64(now.Year()), int64(now.Month()), int64(now.Day()))
		todayData := &dateData{jd: todayJD, start: start}
		commercialYear, _ := dateCommercialParts(todayData)
		weekday := int64(1)
		if value, exists := parts['u']; exists {
			weekday = value
		}
		jd, valid := dateCommercialJD(commercialYear, week, weekday, start)
		if !valid {
			return dateInvalidError()
		}
		return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
	}
	if week, hasU := parts['U']; hasU {
		jd, valid := dateWeekNumberJD(year, week, false, parts)
		if !valid {
			return dateInvalidError()
		}
		return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
	}
	if week, hasW := parts['W']; hasW {
		jd, valid := dateWeekNumberJD(year, week, true, parts)
		if !valid {
			return dateInvalidError()
		}
		return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
	}
	if weekday, has := parts['w']; has && parts['m'] == 0 && parts['d'] == 0 {
		first := dateGregorianToJD(year, 1, 1)
		firstW := dateMod(first+1, 7)
		jd := first + dateMod(weekday-firstW, 7)
		return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
	}
	if weekday, has := parts['a']; has && parts['Y'] == 0 && parts['y'] == 0 && parts['m'] == 0 && parts['d'] == 0 {
		today := dateGregorianToJD(int64(now.Year()), int64(now.Month()), int64(now.Day()))
		sunday := today - dateMod(today+1, 7)
		return newDateValueFromJD(sunday+weekday, start, dateReceiverClass(receiver))
	}
	jd, valid := dateCivilToJD(year, month, day, start)
	if !valid {
		return dateInvalidError()
	}
	return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
}

func dateStrptimeParts(input, format string) (map[byte]int64, bool) {
	replacements := map[string]string{"%c": "%a %b %e %H:%M:%S %Y", "%D": "%m/%d/%y", "%F": "%Y-%m-%d", "%v": "%e-%b-%Y", "%x": "%m/%d/%y", "%+": "%a %b %e %H:%M:%S %Z %Y"}
	for key, value := range replacements {
		format = strings.ReplaceAll(format, key, value)
	}
	var pattern strings.Builder
	pattern.WriteString("^")
	directives := []byte{}
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			if format[i] == ' ' {
				pattern.WriteString(`\s+`)
			} else {
				pattern.WriteString(regexp.QuoteMeta(string(format[i])))
			}
			continue
		}
		i++
		if i >= len(format) {
			return nil, false
		}
		directive := format[i]
		directives = append(directives, directive)
		switch directive {
		case 'A', 'a':
			pattern.WriteString(`([A-Za-z]+)`)
		case 'B', 'b', 'h':
			pattern.WriteString(`([A-Za-z]+)`)
		case 'e':
			pattern.WriteString(`\s*(\d{1,2})`)
		case 'Y', 'G':
			pattern.WriteString(`(-?\d+)`)
		case 'y', 'g', 'C', 'd', 'm', 'U', 'W', 'V', 'u', 'w':
			pattern.WriteString(`(\d{1,2})`)
		case 'j':
			pattern.WriteString(`(\d{1,3})`)
		case 'H', 'M', 'S':
			pattern.WriteString(`(\d{1,2})`)
		case 'Z', 'z':
			pattern.WriteString(`([+\-:\dA-Za-z]+)`)
		case '%':
			pattern.WriteString(`(%)`)
		default:
			pattern.WriteString(`(.+?)`)
		}
	}
	pattern.WriteString("$")
	match := regexp.MustCompile(pattern.String()).FindStringSubmatch(input)
	if match == nil {
		return nil, false
	}
	parts := map[byte]int64{}
	months := map[string]int64{"january": 1, "jan": 1, "february": 2, "feb": 2, "march": 3, "mar": 3, "april": 4, "apr": 4, "may": 5, "june": 6, "jun": 6, "july": 7, "jul": 7, "august": 8, "aug": 8, "september": 9, "sep": 9, "october": 10, "oct": 10, "november": 11, "nov": 11, "december": 12, "dec": 12}
	weekdays := map[string]int64{"sunday": 0, "sun": 0, "monday": 1, "mon": 1, "tuesday": 2, "tue": 2, "wednesday": 3, "wed": 3, "thursday": 4, "thu": 4, "friday": 5, "fri": 5, "saturday": 6, "sat": 6}
	for i, directive := range directives {
		raw := strings.ToLower(strings.TrimSpace(match[i+1]))
		switch directive {
		case 'A', 'a':
			value, exists := weekdays[raw]
			if !exists {
				return nil, false
			}
			parts['a'] = value
		case 'B', 'b', 'h':
			value, exists := months[raw]
			if !exists {
				return nil, false
			}
			parts['m'] = value
		case 'e':
			value, e := strconv.ParseInt(raw, 10, 64)
			if e != nil {
				return nil, false
			}
			parts['d'] = value
		case 'd':
			value, e := strconv.ParseInt(raw, 10, 64)
			if e != nil {
				return nil, false
			}
			parts['d'] = value
		case 'H', 'M', 'S', 'Z', 'z', '%':
			continue
		default:
			value, e := strconv.ParseInt(raw, 10, 64)
			if e != nil {
				return nil, false
			}
			parts[directive] = value
		}
	}
	return parts, true
}

func dateWeekNumberJD(year, week int64, monday bool, parts map[byte]int64) (int64, bool) {
	if week < 0 || week > 53 {
		return 0, false
	}
	jan1 := dateGregorianToJD(year, 1, 1)
	janW := dateMod(jan1+1, 7)
	target := int64(0)
	if monday {
		target = 1
	}
	first := jan1 + dateMod(target-janW, 7)
	if week == 0 {
		first -= 7
	}
	weekday := target
	if value, ok := parts['w']; ok {
		weekday = value
	}
	if value, ok := parts['u']; ok {
		weekday = dateMod(value, 7)
	}
	return first + (week-1)*7 + dateMod(weekday-target, 7), true
}

func dateClassISO8601(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == R.NilVal {
		return dateInvalidError()
	}
	if len(args) > 2 {
		return argumentError("wrong number of arguments")
	}
	input, err := dateParseString(args[0])
	if err != nil {
		return err
	}
	start := float64(dateItalyStart)
	if len(args) == 2 {
		var ok bool
		start, ok = dateStartValue(args[1])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	input = strings.TrimSpace(input)
	var year, month, day int64
	var parseErr error
	if match := regexp.MustCompile(`^(-?\d+)-(\d{2})-(\d{2})$`).FindStringSubmatch(input); match != nil {
		year, parseErr = strconv.ParseInt(match[1], 10, 64)
		if parseErr == nil {
			month, _ = strconv.ParseInt(match[2], 10, 64)
			day, _ = strconv.ParseInt(match[3], 10, 64)
		}
	} else if len(input) == 8 {
		year, parseErr = strconv.ParseInt(input[:4], 10, 64)
		if parseErr == nil {
			month, _ = strconv.ParseInt(input[4:6], 10, 64)
			day, _ = strconv.ParseInt(input[6:], 10, 64)
		}
	} else {
		parseErr = fmt.Errorf("invalid")
	}
	if parseErr != nil {
		return dateInvalidError()
	}
	jd, ok := dateCivilToJD(year, month, day, start)
	if !ok {
		return dateInvalidError()
	}
	return newDateValueFromJD(jd, start, dateReceiverClass(receiver))
}

func dateClassUnderscoreISO8601(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == R.NilVal {
		return emptyHashValue()
	}
	if len(args) > 1 {
		return argumentError("wrong number of arguments")
	}
	input, err := dateParseString(args[0])
	if err != nil {
		return err
	}
	match := regexp.MustCompile(`^(-?\d+)-(\d{2})-(\d{2})$`).FindStringSubmatch(strings.TrimSpace(input))
	if match == nil {
		return emptyHashValue()
	}
	year, yerr := strconv.ParseInt(match[1], 10, 64)
	month, merr := strconv.ParseInt(match[2], 10, 64)
	day, derr := strconv.ParseInt(match[3], 10, 64)
	if yerr != nil || merr != nil || derr != nil {
		return emptyHashValue()
	}
	result := emptyHashValue()
	hash := result.Data.(*object.RHash)
	for _, pair := range []struct {
		name  string
		value int64
	}{{"year", year}, {"mon", month}, {"mday", day}} {
		key := rubySymbol(pair.name)
		hash.Keys = append(hash.Keys, key)
		hash.Pairs[key] = newInt(pair.value)
	}
	return result
}

func dateReceiverClass(receiver *object.EmeraldValue) *object.Class {
	if receiver != nil && receiver.Type == object.ValueClass {
		if class, ok := receiver.Data.(*object.Class); ok {
			return class
		}
	}
	return R.Classes["Date"]
}

func newDateValueFromJD(jd int64, start float64, class *object.Class) *object.EmeraldValue {
	if class == nil {
		class = R.Classes["Date"]
	}
	year, month, day := dateJDToCivil(jd, start)
	return &object.EmeraldValue{Type: object.ValueObject, Data: &dateData{year: year, month: month, day: day, jd: jd, start: start}, Class: class}
}

func dateStartValue(value *object.EmeraldValue) (float64, bool) {
	if value == nil {
		return 0, false
	}
	if infinity, ok := value.Data.(*dateInfinityData); ok {
		if infinity.sign > 0 {
			return math.Inf(1), true
		}
		return math.Inf(-1), true
	}
	if value.Type == object.ValueFloat {
		return value.Data.(float64), true
	}
	integer, ok := valueToInteger(value)
	return float64(integer), ok
}

func dateCivilToJD(year, month, day int64, start float64) (int64, bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, false
	}
	gregorian := dateGregorianToJD(year, month, day)
	julian := dateJulianToJD(year, month, day)
	var jd int64
	var greg bool
	if float64(gregorian) >= start {
		jd, greg = gregorian, true
	} else if float64(julian) < start {
		jd = julian
	} else {
		return 0, false
	}
	y, m, d := dateJDToCivilUsing(jd, greg)
	return jd, y == year && m == month && d == day
}

func dateGregorianToJD(year, month, day int64) int64 {
	a := dateFloorDiv(14-month, 12)
	y := year + 4800 - a
	m := month + 12*a - 3
	return day + dateFloorDiv(153*m+2, 5) + 365*y + dateFloorDiv(y, 4) - dateFloorDiv(y, 100) + dateFloorDiv(y, 400) - 32045
}

func dateJulianToJD(year, month, day int64) int64 {
	a := dateFloorDiv(14-month, 12)
	y := year + 4800 - a
	m := month + 12*a - 3
	return day + dateFloorDiv(153*m+2, 5) + 365*y + dateFloorDiv(y, 4) - 32083
}

func dateJDToCivil(jd int64, start float64) (int64, int64, int64) {
	return dateJDToCivilUsing(jd, float64(jd) >= start)
}

func dateJDToCivilUsing(jd int64, gregorian bool) (int64, int64, int64) {
	var b, c, d, e, m int64
	if gregorian {
		a := jd + 32044
		b = dateFloorDiv(4*a+3, 146097)
		c = a - dateFloorDiv(146097*b, 4)
		d = dateFloorDiv(4*c+3, 1461)
		e = c - dateFloorDiv(1461*d, 4)
		m = dateFloorDiv(5*e+2, 153)
		day := e - dateFloorDiv(153*m+2, 5) + 1
		month := m + 3 - 12*dateFloorDiv(m, 10)
		year := 100*b + d - 4800 + dateFloorDiv(m, 10)
		return year, month, day
	}
	c = jd + 32082
	d = dateFloorDiv(4*c+3, 1461)
	e = c - dateFloorDiv(1461*d, 4)
	m = dateFloorDiv(5*e+2, 153)
	day := e - dateFloorDiv(153*m+2, 5) + 1
	month := m + 3 - 12*dateFloorDiv(m, 10)
	year := d - 4800 + dateFloorDiv(m, 10)
	return year, month, day
}

func dateFloorDiv(a, b int64) int64 {
	q, r := a/b, a%b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}

func dateMod(a, b int64) int64 { return a - dateFloorDiv(a, b)*b }

func dateDaysInMonth(year, month int64, start float64) int64 {
	if month == 2 {
		greg := float64(dateGregorianToJD(year, month, 1)) >= start
		if (greg && dateGregorianLeapYear(year)) || (!greg && dateJulianLeapYear(year)) {
			return 29
		}
		return 28
	}
	if month == 4 || month == 6 || month == 9 || month == 11 {
		return 30
	}
	return 31
}

func dateGregorianLeapYear(year int64) bool {
	return dateMod(year, 4) == 0 && (dateMod(year, 100) != 0 || dateMod(year, 400) == 0)
}
func dateJulianLeapYear(year int64) bool { return dateMod(year, 4) == 0 }

func dateDataValue(receiver *object.EmeraldValue) (*dateData, bool) {
	data, ok := receiver.Data.(*dateData)
	return data, ok && data != nil
}

func dateYear(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.year)
}
func dateMonth(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.month)
}
func dateDay(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.day)
}
func dateJD(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.jd)
}
func dateMJD(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.jd - 2400001)
}
func dateLD(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.jd - 2299160)
}
func dateDayFraction(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(0)
}
func dateAJD(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newRationalValue(big.NewInt(2*d.jd-1), big.NewInt(2))
}

func dateYDay(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	result := d.day
	for month := int64(1); month < d.month; month++ {
		result += dateDaysInMonth(d.year, month, d.start)
	}
	return newInt(result)
}
func dateWDay(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(dateMod(d.jd+1, 7))
}
func dateCWDay(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	w := dateMod(d.jd+1, 7)
	if w == 0 {
		w = 7
	}
	return newInt(w)
}

func dateCommercialParts(d *dateData) (int64, int64) {
	cwday := dateMod(d.jd+1, 7)
	if cwday == 0 {
		cwday = 7
	}
	thursday := d.jd + 4 - cwday
	year, _, _ := dateJDToCivilUsing(thursday, true)
	jan4 := dateGregorianToJD(year, 1, 4)
	jan4w := dateMod(jan4+1, 7)
	if jan4w == 0 {
		jan4w = 7
	}
	week1 := jan4 - (jan4w - 1)
	return year, dateFloorDiv(d.jd-week1, 7) + 1
}
func dateCWYear(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	y, _ := dateCommercialParts(d)
	return newInt(y)
}
func dateCWeek(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	_, w := dateCommercialParts(d)
	return newInt(w)
}

func dateStart(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	if math.IsInf(d.start, 1) {
		return newDateInfinityValue(1)
	}
	if math.IsInf(d.start, -1) {
		return newDateInfinityValue(-1)
	}
	return newInt(int64(d.start))
}
func dateGregorian(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return boolValue(float64(d.jd) >= d.start)
}
func dateJulian(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return boolValue(float64(d.jd) < d.start)
}
func dateLeap(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	if float64(d.jd) >= d.start {
		return boolValue(dateGregorianLeapYear(d.year))
	}
	return boolValue(dateJulianLeapYear(d.year))
}

func dateCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	d, _ := dateDataValue(receiver)
	other, ok := args[0].Data.(*dateData)
	var right int64
	if ok {
		right = other.jd
	} else {
		right, ok = valueToInteger(args[0])
		if !ok {
			return R.NilVal
		}
	}
	if d.jd < right {
		return newInt(-1)
	}
	if d.jd > right {
		return newInt(1)
	}
	return newInt(0)
}
func dateComparisonPair(receiver *object.EmeraldValue, args []*object.EmeraldValue) (int64, int64, bool) {
	if len(args) != 1 {
		return 0, 0, false
	}
	left, _ := dateDataValue(receiver)
	if right, ok := args[0].Data.(*dateData); ok {
		return left.jd, right.jd, true
	}
	value, ok := valueToInteger(args[0])
	return left.jd, value, ok
}
func dateLess(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	l, r, ok := dateComparisonPair(receiver, args)
	if !ok {
		return NewTypeError("comparison failed")
	}
	return boolValue(l < r)
}
func dateLessEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	l, r, ok := dateComparisonPair(receiver, args)
	if !ok {
		return NewTypeError("comparison failed")
	}
	return boolValue(l <= r)
}
func dateGreater(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	l, r, ok := dateComparisonPair(receiver, args)
	if !ok {
		return NewTypeError("comparison failed")
	}
	return boolValue(l > r)
}
func dateGreaterEqual(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	l, r, ok := dateComparisonPair(receiver, args)
	if !ok {
		return NewTypeError("comparison failed")
	}
	return boolValue(l >= r)
}
func dateHash(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newInt(d.jd)
}

func datePlus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	d, _ := dateDataValue(receiver)
	days, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("expected numeric")
	}
	return newDateValueFromJD(d.jd+days, d.start, receiver.Class)
}
func dateMinus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	d, _ := dateDataValue(receiver)
	if other, ok := args[0].Data.(*dateData); ok {
		return newRationalValue(big.NewInt(d.jd-other.jd), big.NewInt(1))
	}
	days, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("expected numeric")
	}
	return newDateValueFromJD(d.jd-days, d.start, receiver.Class)
}
func dateShiftMonths(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	months, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("expected integer")
	}
	return dateShiftMonthsBy(receiver, months)
}
func dateShiftMonthsBack(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	months, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("expected integer")
	}
	return dateShiftMonthsBy(receiver, -months)
}
func dateShiftMonthsBy(receiver *object.EmeraldValue, months int64) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	total := d.year*12 + d.month - 1 + months
	year := dateFloorDiv(total, 12)
	month := dateMod(total, 12) + 1
	day := d.day
	if last := dateDaysInMonth(year, month, d.start); day > last {
		day = last
	}
	for day > 0 {
		if jd, ok := dateCivilToJD(year, month, day, d.start); ok {
			return newDateValueFromJD(jd, d.start, receiver.Class)
		}
		day--
	}
	return argumentError("invalid date")
}
func dateSucc(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newDateValueFromJD(d.jd+1, d.start, receiver.Class)
}
func dateOptionalCount(args []*object.EmeraldValue) (int64, *object.EmeraldValue) {
	if len(args) > 1 {
		return 0, argumentError("wrong number of arguments")
	}
	if len(args) == 0 {
		return 1, nil
	}
	n, ok := valueToInteger(args[0])
	if !ok {
		return 0, NewTypeError("expected integer")
	}
	return n, nil
}
func dateNextDay(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n, e := dateOptionalCount(args)
	if e != nil {
		return e
	}
	d, _ := dateDataValue(receiver)
	return newDateValueFromJD(d.jd+n, d.start, receiver.Class)
}
func datePrevDay(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n, e := dateOptionalCount(args)
	if e != nil {
		return e
	}
	d, _ := dateDataValue(receiver)
	return newDateValueFromJD(d.jd-n, d.start, receiver.Class)
}
func dateNextMonth(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n, e := dateOptionalCount(args)
	if e != nil {
		return e
	}
	return dateShiftMonthsBy(receiver, n)
}
func datePrevMonth(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n, e := dateOptionalCount(args)
	if e != nil {
		return e
	}
	return dateShiftMonthsBy(receiver, -n)
}
func dateNextYear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n, e := dateOptionalCount(args)
	if e != nil {
		return e
	}
	return dateShiftMonthsBy(receiver, 12*n)
}
func datePrevYear(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	n, e := dateOptionalCount(args)
	if e != nil {
		return e
	}
	return dateShiftMonthsBy(receiver, -12*n)
}

func dateNewStart(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return argumentError("wrong number of arguments")
	}
	start := float64(dateItalyStart)
	var ok bool
	if len(args) == 1 {
		start, ok = dateStartValue(args[0])
		if !ok {
			return NewTypeError("invalid start")
		}
	}
	d, _ := dateDataValue(receiver)
	return newDateValueFromJD(d.jd, start, receiver.Class)
}

func dateWithStart(receiver *object.EmeraldValue, start float64) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return newDateValueFromJD(d.jd, start, receiver.Class)
}

func dateItaly(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	return dateWithStart(receiver, float64(dateItalyStart))
}
func dateEngland(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	return dateWithStart(receiver, float64(dateEnglandStart))
}
func dateAllJulian(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	return dateWithStart(receiver, math.Inf(1))
}
func dateAllGregorian(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	return dateWithStart(receiver, math.Inf(-1))
}

func dateToS(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return rubyString(fmt.Sprintf("%04d-%02d-%02d", d.year, d.month, d.day))
}
func dateInspect(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	return rubyString(fmt.Sprintf("#<Date: %04d-%02d-%02d ((%dj,0s,0n),+0s,%dj)>", d.year, d.month, d.day, d.jd, int64(d.start)))
}

func dateStep(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return argumentError("wrong number of arguments")
	}
	start, _ := dateDataValue(receiver)
	finish, ok := args[0].Data.(*dateData)
	if !ok {
		return NewTypeError("expected Date")
	}
	step := int64(1)
	if len(args) == 2 {
		step, ok = valueToInteger(args[1])
		if !ok {
			return NewTypeError("expected numeric")
		}
	}
	if step == 0 {
		return argumentError("step cannot be 0")
	}
	build := func() ([]*object.EmeraldValue, *object.EmeraldValue) {
		values := []*object.EmeraldValue{}
		for jd := start.jd; (step > 0 && jd <= finish.jd) || (step < 0 && jd >= finish.jd); jd += step {
			values = append(values, newDateValueFromJD(jd, start.start, receiver.Class))
		}
		return values, nil
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CurrentBlockValue() == nil {
		return newGeneratorEnumerator(build)
	}
	values, err := build()
	if err != nil {
		return err
	}
	for _, value := range values {
		result := CallBlockWithArgs(CurrentBlockValue(), value)
		if result != nil && result.Type == object.ValueException {
			return result
		}
		if LastBlockResult != nil {
			control := LastBlockResult
			LastBlockResult = nil
			return control
		}
	}
	return receiver
}
func dateUpto(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return dateStep(receiver, args...)
}
func dateDownto(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	return dateStep(receiver, args[0], newInt(-1))
}

func timeToDate(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*timeData)
	if !ok || data == nil {
		return NewTypeError("expected Time")
	}
	year, month, day := data.value.Date()
	jd := dateGregorianToJD(int64(year), int64(month), int64(day))
	return newDateValueFromJD(jd, float64(dateItalyStart), R.Classes["Date"])
}

func dateTimePlus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	days, ok := dateNumericRat(args[0])
	if !ok {
		return NewTypeError("expected numeric")
	}
	d, _ := dateDataValue(receiver)
	instant := new(big.Rat).Add(new(big.Rat).Set(d.instant), days)
	return newDateTimeFromInstant(instant, d.offset, d.start, receiver.Class)
}
func dateTimeMinus(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	d, _ := dateDataValue(receiver)
	if other, ok := args[0].Data.(*dateData); ok && other.instant != nil {
		difference := new(big.Rat).Sub(d.instant, other.instant)
		return newRationalValue(difference.Num(), difference.Denom())
	}
	days, ok := dateNumericRat(args[0])
	if !ok {
		return NewTypeError("expected numeric")
	}
	instant := new(big.Rat).Sub(new(big.Rat).Set(d.instant), days)
	return newDateTimeFromInstant(instant, d.offset, d.start, receiver.Class)
}

func dateTimeToTime(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := dateDataValue(receiver)
	local := new(big.Rat).Add(new(big.Rat).Set(d.instant), d.offset)
	jd := dateRatFloor(local)
	fraction := new(big.Rat).Sub(local, new(big.Rat).SetInt64(jd))
	secondsRat := new(big.Rat).Mul(fraction, big.NewRat(86400, 1))
	seconds := dateRatFloor(secondsRat)
	sub := new(big.Rat).Sub(secondsRat, new(big.Rat).SetInt64(seconds))
	nanosRat := new(big.Rat).Mul(sub, big.NewRat(1e9, 1))
	nanos := dateRatFloor(nanosRat)
	year, month, day := dateJDToCivilUsing(jd, true)
	offsetSeconds := dateRatFloor(new(big.Rat).Mul(d.offset, big.NewRat(86400, 1)))
	location := time.FixedZone(dateOffsetString(d.offset, true), int(offsetSeconds))
	value := time.Date(int(year), time.Month(month), int(day), int(seconds/3600), int((seconds%3600)/60), int(seconds%60), int(nanos), location)
	return newTimeValue(value)
}

func timeToDateTime(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	data, ok := receiver.Data.(*timeData)
	if !ok || data == nil {
		return NewTypeError("expected Time")
	}
	year, month, day := data.value.Date()
	hour, minute, second := data.value.Clock()
	_, offsetSeconds := data.value.Zone()
	offset := big.NewRat(int64(offsetSeconds), 86400)
	jd := dateGregorianToJD(int64(year), int64(month), int64(day))
	localSeconds := big.NewRat(int64(hour*3600+minute*60+second), 1)
	localSeconds.Add(localSeconds, big.NewRat(int64(data.value.Nanosecond()), 1e9))
	instant := new(big.Rat).Add(new(big.Rat).SetInt64(jd), new(big.Rat).Quo(localSeconds, big.NewRat(86400, 1)))
	instant.Sub(instant, offset)
	return newDateTimeFromInstant(instant, offset, float64(dateItalyStart), R.Classes["DateTime"])
}

func dateStrftime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return argumentError("wrong number of arguments")
	}
	format := "%F"
	d, _ := dateDataValue(receiver)
	if d.instant != nil {
		format = "%FT%T%Z"
	}
	if len(args) == 1 {
		if args[0] == nil || args[0].Type != object.ValueString {
			return NewTypeError("no implicit conversion into String")
		}
		format = args[0].Data.(string)
	}
	return rubyString(dateFormat(d, format))
}

func dateFormat(d *dateData, format string) string {
	dayNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	monthNames := []string{"", "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	var out strings.Builder
	for i := 0; i < len(format); {
		if format[i] != '%' || i+1 >= len(format) {
			out.WriteByte(format[i])
			i++
			continue
		}
		i++
		if format[i] == '%' {
			out.WriteByte('%')
			i++
			continue
		}
		colonCount := 0
		for i < len(format) && format[i] == ':' {
			colonCount++
			i++
		}
		flags := ""
		for i < len(format) && strings.ContainsRune("^_0-", rune(format[i])) {
			flags += string(format[i])
			i++
		}
		widthStart := i
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}
		width := 0
		if i > widthStart {
			width, _ = strconv.Atoi(format[widthStart:i])
		}
		if i >= len(format) {
			out.WriteByte('%')
			out.WriteString(flags)
			break
		}
		spec := format[i]
		i++
		value, raw, defaultWidth, defaultPad := dateFormatValue(d, spec, dayNames, monthNames)
		if spec == 'N' {
			digits := width
			if digits == 0 {
				digits = 9
			}
			value = dateFractionDigits(d, digits)
			raw = value
			width = 0
			defaultWidth = 0
		}
		if spec == 'z' && colonCount > 0 {
			value = dateOffsetStringDetailed(d.offset, colonCount)
			raw = value
		}
		if strings.Contains(flags, "^") {
			value, raw = strings.ToUpper(value), strings.ToUpper(raw)
		}
		if strings.Contains(flags, "-") {
			value = raw
		} else if width > 0 {
			pad := byte(' ')
			for j := 0; j < len(flags); j++ {
				if flags[j] == '0' {
					pad = '0'
				} else if flags[j] == '_' {
					pad = ' '
				}
			}
			value = datePad(raw, width, pad)
		} else if defaultWidth > 0 {
			pad := defaultPad
			for j := 0; j < len(flags); j++ {
				if flags[j] == '0' {
					pad = '0'
				} else if flags[j] == '_' {
					pad = ' '
				}
			}
			value = datePad(raw, defaultWidth, pad)
		}
		out.WriteString(value)
	}
	return out.String()
}

func datePad(value string, width int, pad byte) string {
	if len(value) >= width {
		return value
	}
	return strings.Repeat(string(pad), width-len(value)) + value
}

func dateInstantValue(d *dateData) *big.Rat {
	if d != nil && d.instant != nil {
		return new(big.Rat).Set(d.instant)
	}
	if d == nil {
		return new(big.Rat)
	}
	return new(big.Rat).SetInt64(d.jd)
}
func dateFractionDigits(d *dateData, digits int) string {
	if digits < 1 {
		return ""
	}
	fraction := new(big.Rat)
	if d != nil && d.secFraction != nil {
		fraction.Set(d.secFraction)
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	scaled := new(big.Rat).Mul(fraction, new(big.Rat).SetInt(scale))
	value := dateRatFloor(scaled)
	return fmt.Sprintf("%0*d", digits, value)
}
func dateOffsetStringDetailed(offset *big.Rat, colons int) string {
	seconds := int64(0)
	if offset != nil {
		seconds = dateRatFloor(new(big.Rat).Mul(offset, big.NewRat(86400, 1)))
	}
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	if colons >= 2 {
		return fmt.Sprintf("%s%02d:%02d:%02d", sign, seconds/3600, (seconds%3600)/60, seconds%60)
	}
	return fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, (seconds%3600)/60)
}

func dateFormatValue(d *dateData, spec byte, days, months []string) (value, raw string, width int, pad byte) {
	wday := dateMod(d.jd+1, 7)
	cwday := wday
	if cwday == 0 {
		cwday = 7
	}
	cwyear, cweek := dateCommercialParts(d)
	yday := d.day
	for m := int64(1); m < d.month; m++ {
		yday += dateDaysInMonth(d.year, m, d.start)
	}
	setNumber := func(n int64, w int, p byte) (string, string, int, byte) {
		r := strconv.FormatInt(n, 10)
		return datePad(r, w, p), r, w, p
	}
	switch spec {
	case 'A':
		raw = days[wday]
		return raw, raw, 0, ' '
	case 'a':
		raw = days[wday][:3]
		return raw, raw, 0, ' '
	case 'B':
		raw = months[d.month]
		return raw, raw, 0, ' '
	case 'b', 'h':
		raw = months[d.month][:3]
		return raw, raw, 0, ' '
	case 'C':
		return setNumber(dateFloorDiv(d.year, 100), 2, '0')
	case 'd':
		return setNumber(d.day, 2, '0')
	case 'e':
		return setNumber(d.day, 2, ' ')
	case 'G':
		return setNumber(cwyear, 4, '0')
	case 'g':
		return setNumber(dateMod(cwyear, 100), 2, '0')
	case 'H':
		return setNumber(d.hour, 2, '0')
	case 'I':
		hour := d.hour % 12
		if hour == 0 {
			hour = 12
		}
		return setNumber(hour, 2, '0')
	case 'j':
		return setNumber(yday, 3, '0')
	case 'k':
		return setNumber(d.hour, 2, ' ')
	case 'l':
		hour := d.hour % 12
		if hour == 0 {
			hour = 12
		}
		return setNumber(hour, 2, ' ')
	case 'M':
		return setNumber(d.minute, 2, '0')
	case 'S':
		return setNumber(d.second, 2, '0')
	case 'L':
		raw = dateFractionDigits(d, 3)
		return raw, raw, 0, ' '
	case 'N':
		raw = dateFractionDigits(d, 9)
		return raw, raw, 0, ' '
	case 'm':
		return setNumber(d.month, 2, '0')
	case 'n':
		return "\n", "\n", 0, ' '
	case 'P':
		if d.hour >= 12 {
			return "pm", "pm", 0, ' '
		}
		return "am", "am", 0, ' '
	case 'p':
		if d.hour >= 12 {
			return "PM", "PM", 0, ' '
		}
		return "AM", "AM", 0, ' '
	case 's':
		seconds := new(big.Rat).Mul(dateInstantValue(d), big.NewRat(86400, 1))
		seconds.Sub(seconds, big.NewRat(2440588*86400, 1))
		raw = strconv.FormatInt(dateRatFloor(seconds), 10)
		return raw, raw, 0, ' '
	case 'Q':
		millis := new(big.Rat).Mul(dateInstantValue(d), big.NewRat(86400000, 1))
		millis.Sub(millis, big.NewRat(2440588*86400000, 1))
		raw = strconv.FormatInt(dateRatFloor(millis), 10)
		return raw, raw, 0, ' '
	case 't':
		return "\t", "\t", 0, ' '
	case 'U':
		return setNumber((yday+6-wday)/7, 2, '0')
	case 'u':
		raw = strconv.FormatInt(cwday, 10)
		return raw, raw, 0, ' '
	case 'V':
		return setNumber(cweek, 2, '0')
	case 'W':
		monIndex := dateMod(wday+6, 7)
		return setNumber((yday+6-monIndex)/7, 2, '0')
	case 'w':
		raw = strconv.FormatInt(wday, 10)
		return raw, raw, 0, ' '
	case 'Y':
		return setNumber(d.year, 4, '0')
	case 'y':
		return setNumber(dateMod(d.year, 100), 2, '0')
	case 'z':
		raw = dateOffsetString(d.offset, false)
		return raw, raw, 0, ' '
	case 'Z':
		raw = dateOffsetString(d.offset, true)
		return raw, raw, 0, ' '
	case 'c':
		raw = dateFormat(d, "%a %b %e %H:%M:%S %Y")
		return raw, raw, 0, ' '
	case 'D', 'x':
		raw = dateFormat(d, "%m/%d/%y")
		return raw, raw, 0, ' '
	case 'F':
		raw = dateFormat(d, "%Y-%m-%d")
		return raw, raw, 0, ' '
	case 'R':
		raw = fmt.Sprintf("%02d:%02d", d.hour, d.minute)
		return raw, raw, 0, ' '
	case 'r':
		raw = dateFormat(d, "%I:%M:%S %p")
		return raw, raw, 0, ' '
	case 'T', 'X':
		raw = fmt.Sprintf("%02d:%02d:%02d", d.hour, d.minute, d.second)
		return raw, raw, 0, ' '
	case 'v':
		raw = dateFormat(d, "%e-%^b-%Y")
		return raw, raw, 0, ' '
	case '+':
		raw = dateFormat(d, "%a %b %e %H:%M:%S %Z %Y")
		return raw, raw, 0, ' '
	default:
		raw = "%" + string(spec)
		return raw, raw, 0, ' '
	}
}

func dateClassGregorianLeap(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	y, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("expected integer")
	}
	return boolValue(dateGregorianLeapYear(y))
}
func dateClassJulianLeap(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	y, ok := valueToInteger(args[0])
	if !ok {
		return NewTypeError("expected integer")
	}
	return boolValue(dateJulianLeapYear(y))
}
func dateClassValidCivil(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 3 || len(args) > 4 {
		return boolValue(false)
	}
	y, yok := valueToInteger(args[0])
	m, mok := valueToInteger(args[1])
	d, dok := valueToInteger(args[2])
	start := float64(dateItalyStart)
	sok := true
	if len(args) == 4 {
		start, sok = dateStartValue(args[3])
	}
	if !yok || !mok || !dok || !sok {
		return boolValue(false)
	}
	_, ok := dateCivilToJD(y, m, d, start)
	return boolValue(ok)
}
func dateClassValidOrdinal(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 2 || len(args) > 3 {
		return R.FalseVal
	}
	year, yok := valueToInteger(args[0])
	yday, dok := valueToInteger(args[1])
	start, sok := float64(dateItalyStart), true
	if len(args) == 3 {
		start, sok = dateStartValue(args[2])
	}
	if !yok || !dok || !sok {
		return R.FalseVal
	}
	_, ok := dateOrdinalJD(year, yday, start)
	return boolValue(ok)
}
func dateClassValidCommercial(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 3 || len(args) > 4 {
		return R.FalseVal
	}
	year, yok := valueToInteger(args[0])
	week, wok := valueToInteger(args[1])
	day, dok := valueToInteger(args[2])
	start, sok := float64(dateItalyStart), true
	if len(args) == 4 {
		start, sok = dateStartValue(args[3])
	}
	if !yok || !wok || !dok || !sok {
		return R.FalseVal
	}
	_, ok := dateCommercialJD(year, week, day, start)
	return boolValue(ok)
}
func dateClassToday(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return argumentError("wrong number of arguments")
	}
	now := time.Now()
	values := []*object.EmeraldValue{newInt(int64(now.Year())), newInt(int64(now.Month())), newInt(int64(now.Day()))}
	if len(args) == 1 {
		values = append(values, args[0])
	}
	return dateCivilValue(receiver, values...)
}

func installDateInfinity(dateClass *object.Class) {
	infinity := object.NewClass("Date::Infinity")
	infinity.SuperClass = R.Classes["Numeric"]
	infinity.DefineClassMethod("new", &object.Method{Name: "new", Fn: dateInfinityNew, Arity: -1})
	infinity.DefineMethod("<=>", &object.Method{Name: "<=>", Fn: dateInfinityCompare, Arity: 1})
	infinity.DefineMethod("abs", &object.Method{Name: "abs", Fn: func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
		return newDateInfinityValue(1)
	}, Arity: 0})
	infinity.DefineMethod("+@", &object.Method{Name: "+@", Fn: func(r *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue { return r }, Arity: 0})
	infinity.DefineMethod("-@", &object.Method{Name: "-@", Fn: dateInfinityNegate, Arity: 0})
	infinity.DefineMethod("zero?", &object.Method{Name: "zero?", Fn: func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue { return R.FalseVal }, Arity: 0})
	infinity.DefineMethod("finite?", &object.Method{Name: "finite?", Fn: func(_ *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue { return R.FalseVal }, Arity: 0})
	infinity.DefineMethod("infinite?", &object.Method{Name: "infinite?", Fn: dateInfinityInfinite, Arity: 0})
	infinity.DefineMethod("nan?", &object.Method{Name: "nan?", Fn: dateInfinityNaN, Arity: 0})
	infinity.DefineMethod("d", &object.Method{Name: "d", Fn: dateInfinityD, Arity: 0})
	infinity.DefineMethod("coerce", &object.Method{Name: "coerce", Fn: dateInfinityCoerce, Arity: 1})
	R.Classes["Date::Infinity"] = infinity
	value := &object.EmeraldValue{Type: object.ValueClass, Data: infinity, Class: R.Classes["Class"]}
	dateClass.DefineConstant("Infinity", value)
}
func newDateInfinityValue(sign int64) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueObject, Data: &dateInfinityData{sign: sign}, Class: R.Classes["Date::Infinity"]}
}
func dateInfinityNew(_ *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return argumentError("wrong number of arguments")
	}
	sign := int64(1)
	if len(args) == 1 {
		n, ok := valueToInteger(args[0])
		if !ok {
			return NewTypeError("expected numeric")
		}
		if n < 0 {
			sign = -1
		} else if n == 0 {
			sign = 0
		}
	}
	return newDateInfinityValue(sign)
}
func dateInfinityD(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*dateInfinityData)
	return newInt(d.sign)
}
func dateInfinityCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	d, _ := receiver.Data.(*dateInfinityData)
	other, ok := args[0].Data.(*dateInfinityData)
	if !ok {
		return R.NilVal
	}
	if d.sign < other.sign {
		return newInt(-1)
	}
	if d.sign > other.sign {
		return newInt(1)
	}
	return newInt(0)
}
func dateInfinityNegate(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*dateInfinityData)
	return newDateInfinityValue(-d.sign)
}
func dateInfinityInfinite(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*dateInfinityData)
	if d.sign == 0 {
		return R.NilVal
	}
	return newInt(d.sign)
}
func dateInfinityNaN(receiver *object.EmeraldValue, _ ...*object.EmeraldValue) *object.EmeraldValue {
	d, _ := receiver.Data.(*dateInfinityData)
	return boolValue(d.sign == 0)
}
func dateInfinityCoerce(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return argumentError("wrong number of arguments")
	}
	d, _ := receiver.Data.(*dateInfinityData)
	left := -d.sign
	if d.sign == 0 {
		left = 0
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{newInt(left), newInt(d.sign)}, Class: R.Classes["Array"]}
}
