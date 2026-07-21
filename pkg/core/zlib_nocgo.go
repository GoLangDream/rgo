//go:build !cgo

package core

import (
	"bytes"
	"compress/gzip"
	stdzlib "compress/zlib"
)

func zlibCompressBytes(source []byte, level int, gzipFormat bool) ([]byte, error) {
	return zlibCompressBytesDictionary(source, nil, level, gzipFormat)
}

func zlibCompressBytesDictionary(source, dictionary []byte, level int, gzipFormat bool) ([]byte, error) {
	var output bytes.Buffer
	if gzipFormat {
		writer, err := gzip.NewWriterLevel(&output, level)
		if err != nil {
			return nil, err
		}
		_, _ = writer.Write(source)
		if err := writer.Close(); err != nil {
			return nil, err
		}
		return output.Bytes(), nil
	}
	writer, err := stdzlib.NewWriterLevel(&output, level)
	if err != nil {
		return nil, err
	}
	_, _ = writer.Write(source)
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
