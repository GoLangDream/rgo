package core

import (
	"regexp"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

func stringScannerAnchoredSubmatch(data *stringScannerData, re *regexp.Regexp, sourcePattern string) []int {
	if data == nil || data.pos > int64(len(data.str)) {
		return nil
	}
	if !data.fixedAnchor {
		location := re.FindStringSubmatchIndex(data.str[data.pos:])
		if location == nil || location[0] != 0 {
			return nil
		}
		return location
	}
	if strings.Contains(sourcePattern, `\A`) && data.pos != 0 {
		return nil
	}
	if strings.Contains(sourcePattern, "^") && data.pos > 0 && data.str[data.pos-1] != '\n' {
		return nil
	}
	location := re.FindStringSubmatchIndex(data.str[data.pos:])
	if location == nil || location[0] != 0 {
		return nil
	}
	return location
}

func stringScannerRegexpSubmatch(data *stringScannerData, re *regexp.Regexp, raw *object.RRegexp, source string, anchored bool) ([]int, string) {
	if anchored && data != nil && data.fixedAnchor {
		if strings.Contains(raw.Pattern, `\A`) && data.pos != 0 {
			return nil, ""
		}
		if strings.Contains(raw.Pattern, "^") && data.pos > 0 && data.str[data.pos-1] != '\n' {
			return nil, ""
		}
	}
	if re != nil && !regexpNeedsOnig(raw.Pattern) {
		location := re.FindStringSubmatchIndex(source)
		if anchored && (location == nil || location[0] != 0) {
			return nil, ""
		}
		return location, ""
	}
	location, handled, errorText := onigRegexpSearch(regexpOnigCompatiblePattern(raw.Pattern), source, raw.Options)
	if handled {
		if errorText != "" {
			return nil, errorText
		}
		if anchored && (location == nil || location[0] != 0) {
			return nil, ""
		}
		return location, ""
	}
	if re == nil {
		_, err := compileRubyRegexp(raw)
		if err != nil {
			return nil, err.Error()
		}
		return nil, "unsupported regular expression"
	}
	location = re.FindStringSubmatchIndex(source)
	if anchored && (location == nil || location[0] != 0) {
		return nil, ""
	}
	return location, ""
}
