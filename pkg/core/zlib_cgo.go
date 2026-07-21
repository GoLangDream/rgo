//go:build cgo

package core

/*
#cgo LDFLAGS: -lz
#include <stdlib.h>
#include <zlib.h>

static int rgo_zlib_compress(const unsigned char *source, size_t source_len,
                             const unsigned char *dictionary, size_t dictionary_len,
                             int level, int window_bits,
                             unsigned char **output, size_t *output_len) {
    z_stream stream = {0};
    int result = deflateInit2(&stream, level, Z_DEFLATED, window_bits, 8, Z_DEFAULT_STRATEGY);
    if (result != Z_OK) return result;
	if (dictionary_len > 0) {
		result = deflateSetDictionary(&stream, dictionary, (uInt)dictionary_len);
		if (result != Z_OK) {
			deflateEnd(&stream);
			return result;
		}
	}

    size_t capacity = deflateBound(&stream, (uLong)source_len);
    unsigned char *buffer = (unsigned char *)malloc(capacity == 0 ? 1 : capacity);
    if (buffer == NULL) {
        deflateEnd(&stream);
        return Z_MEM_ERROR;
    }

    stream.next_in = (Bytef *)source;
    stream.avail_in = (uInt)source_len;
    stream.next_out = buffer;
    stream.avail_out = (uInt)capacity;
    result = deflate(&stream, Z_FINISH);
    if (result != Z_STREAM_END) {
        free(buffer);
        deflateEnd(&stream);
        return result;
    }

    *output = buffer;
    *output_len = stream.total_out;
    deflateEnd(&stream);
    return Z_OK;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func zlibCompressBytes(source []byte, level int, gzipFormat bool) ([]byte, error) {
	return zlibCompressBytesDictionary(source, nil, level, gzipFormat)
}

func zlibCompressBytesDictionary(source, dictionary []byte, level int, gzipFormat bool) ([]byte, error) {
	windowBits := C.int(15)
	if gzipFormat {
		windowBits = 31
	}
	var input *C.uchar
	if len(source) > 0 {
		input = (*C.uchar)(unsafe.Pointer(&source[0]))
	}
	var dictionaryInput *C.uchar
	if len(dictionary) > 0 {
		dictionaryInput = (*C.uchar)(unsafe.Pointer(&dictionary[0]))
	}
	var output *C.uchar
	var outputLength C.size_t
	result := C.rgo_zlib_compress(input, C.size_t(len(source)), dictionaryInput, C.size_t(len(dictionary)), C.int(level), windowBits, &output, &outputLength)
	if result != C.Z_OK {
		return nil, fmt.Errorf("zlib compression failed: %d", int(result))
	}
	defer C.free(unsafe.Pointer(output))
	return C.GoBytes(unsafe.Pointer(output), C.int(outputLength)), nil
}
