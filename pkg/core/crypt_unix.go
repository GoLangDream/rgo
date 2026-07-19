//go:build unix && cgo

package core

/*
#cgo LDFLAGS: -lcrypt
#define _GNU_SOURCE
#include <crypt.h>
#include <stdlib.h>
#include <string.h>

static char *rgo_crypt(const char *password, const char *salt) {
	struct crypt_data data;
	memset(&data, 0, sizeof(data));
	char *result = crypt_r(password, salt, &data);
	return result == NULL ? NULL : strdup(result);
}
*/
import "C"

import "unsafe"

func platformCrypt(password, salt string) (string, bool) {
	cPassword := C.CString(password)
	cSalt := C.CString(salt)
	defer C.free(unsafe.Pointer(cPassword))
	defer C.free(unsafe.Pointer(cSalt))

	result := C.rgo_crypt(cPassword, cSalt)
	if result == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result), true
}
