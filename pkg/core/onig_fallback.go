//go:build !linux || !cgo

package core

func onigRegexpSearch(pattern, source, options string) ([]int, bool, string) {
	return nil, false, "Oniguruma unavailable"
}
