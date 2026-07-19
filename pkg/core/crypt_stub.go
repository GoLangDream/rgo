//go:build !unix || !cgo

package core

func platformCrypt(password, salt string) (string, bool) {
	return "", false
}
