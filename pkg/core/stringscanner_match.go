package core

import (
	"regexp"
	"strings"
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
