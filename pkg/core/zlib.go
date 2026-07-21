package core

import (
	"bufio"
	"bytes"
	stdzlib "compress/zlib"
	"encoding/binary"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GoLangDream/rgo/pkg/object"
)

func zlibDeflateNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	level := -1
	if len(args) > 0 {
		value, ok := valueToInteger(args[0])
		if !ok {
			return typeError("no implicit conversion into Integer")
		}
		level = int(value)
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: &zlibDeflateData{level: level}, Class: R.Classes["Zlib::Deflate"]}
}

func zlibDeflateState(receiver *object.EmeraldValue) (*zlibDeflateData, *object.EmeraldValue) {
	data, ok := receiver.Data.(*zlibDeflateData)
	if !ok || data == nil {
		return nil, newRuntimeException(R.Classes["Zlib::StreamError"], "invalid stream")
	}
	return data, nil
}

func zlibDeflateClassDeflate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	_, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	level := -1
	if len(args) == 2 {
		value, ok := valueToInteger(args[1])
		if !ok {
			return typeError("no implicit conversion into Integer")
		}
		level = int(value)
	}
	compressed, err := zlibCompressBytes([]byte(raw), level, false)
	if err != nil {
		return newRuntimeException(R.Classes["Zlib::StreamError"], err.Error())
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		for start := 0; start < len(compressed); start += 16384 {
			end := start + 16384
			if end > len(compressed) {
				end = len(compressed)
			}
			if result := CallBlockWithArgs(CurrentBlockValue(), stringWithEncoding(string(compressed[start:end]), "BINARY")); result != nil && result.Type == object.ValueException {
				return result
			}
		}
		return R.NilVal
	}
	return stringWithEncoding(string(compressed), "BINARY")
}

func zlibDeflateInstanceDeflate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibDeflateState(receiver)
	if errVal != nil {
		return errVal
	}
	_, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	data.input.WriteString(raw)
	finish := len(args) > 1 && args[1] != nil && args[1].Type == object.ValueInteger && args[1].Data.(int64) == 4
	compressed, err := zlibCompressBytesDictionary([]byte(data.input.String()), []byte(data.dictionary), data.level, false)
	if err != nil {
		return newRuntimeException(R.Classes["Zlib::StreamError"], err.Error())
	}
	if finish {
		data.finished = true
		data.emitted = len(compressed)
		return stringWithEncoding(string(compressed), "BINARY")
	}
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		for len(compressed)-data.emitted >= 16384 {
			chunk := compressed[data.emitted : data.emitted+16384]
			data.emitted += 16384
			LastBlockResult = nil
			result := CallBlockWithArgs(CurrentBlockValue(), stringWithEncoding(string(chunk), "BINARY"))
			if LastBlockResult != nil {
				breakValue := LastBlockResult
				LastBlockResult = nil
				return breakValue
			}
			if result != nil && result.Type == object.ValueException {
				return result
			}
		}
	}
	return stringWithEncoding("", "BINARY")
}

func zlibDeflateAppend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := zlibDeflateInstanceDeflate(receiver, args[0])
	if result != nil && result.Type == object.ValueException {
		return result
	}
	return receiver
}

func zlibDeflateFinish(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibDeflateState(receiver)
	if errVal != nil {
		return errVal
	}
	compressed, err := zlibCompressBytesDictionary([]byte(data.input.String()), []byte(data.dictionary), data.level, false)
	if err != nil {
		return newRuntimeException(R.Classes["Zlib::StreamError"], err.Error())
	}
	if data.emitted > len(compressed) {
		data.emitted = len(compressed)
	}
	result := compressed[data.emitted:]
	data.emitted, data.finished = len(compressed), true
	return stringWithEncoding(string(result), "BINARY")
}

func zlibDeflateParams(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibDeflateState(receiver)
	if errVal != nil {
		return errVal
	}
	level, ok := valueToInteger(args[0])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	data.level = int(level)
	return R.NilVal
}

func zlibDeflateSetDictionary(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibDeflateState(receiver)
	if errVal != nil {
		return errVal
	}
	_, dictionary, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	data.dictionary = dictionary
	return args[0]
}

func zlibDeflateAdler(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibDeflateState(receiver)
	if errVal != nil {
		return errVal
	}
	return zlibAdler32(receiver, rubyString(data.input.String()))
}

func zlibStreamZero(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(0)
}
func zlibStreamDataType(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return newInt(0)
}
func zlibDeflateFinished(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*zlibDeflateData)
	return boolValue(data != nil && data.finished)
}
func zlibDeflateClosed(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*zlibDeflateData)
	return boolValue(data != nil && data.closed)
}
func zlibDeflateClose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*zlibDeflateData)
	if data != nil {
		data.closed = true
	}
	return R.NilVal
}

func inflateFirstStream(raw string) (string, string, bool) {
	source := bytes.NewReader([]byte(raw))
	buffered := bufio.NewReader(source)
	reader, err := stdzlib.NewReader(buffered)
	if err != nil {
		return "", "", false
	}
	decoded, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		return "", "", false
	}
	consumed := len(raw) - source.Len() - buffered.Buffered()
	if consumed < 0 || consumed > len(raw) {
		consumed = len(raw)
	}
	return string(decoded), raw[consumed:], true
}

func zlibInflateNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueObject, Data: &zlibInflateData{}, Class: R.Classes["Zlib::Inflate"]}
}

func zlibInflateClassInflate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	_, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	decoded, trailing, ok := inflateFirstStream(raw)
	if !ok {
		return newRuntimeException(R.Classes["Zlib::BufError"], "buffer error")
	}
	return stringWithEncoding(decoded+trailing, "BINARY")
}

func zlibInflateState(receiver *object.EmeraldValue) (*zlibInflateData, *object.EmeraldValue) {
	data, ok := receiver.Data.(*zlibInflateData)
	if !ok || data == nil {
		return nil, newRuntimeException(R.Classes["Zlib::StreamError"], "invalid stream")
	}
	return data, nil
}

func zlibInflateInstanceInflate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibInflateState(receiver)
	if errVal != nil {
		return errVal
	}
	if args[0] == nil || args[0].Type == object.ValueNil {
		result := data.pending
		data.pending = ""
		return stringWithEncoding(result, "BINARY")
	}
	_, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	if data.finished {
		return stringWithEncoding(raw, "BINARY")
	}
	data.input.WriteString(raw)
	decoded, trailing, complete := inflateFirstStream(data.input.String())
	if !complete {
		return stringWithEncoding("", "BINARY")
	}
	data.finished = true
	output := decoded + trailing
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CallBlockWithArgs != nil {
		for start := 0; start < len(output); start += 16384 {
			end := start + 16384
			if end > len(output) {
				end = len(output)
			}
			LastBlockResult = nil
			result := CallBlockWithArgs(CurrentBlockValue(), stringWithEncoding(output[start:end], "BINARY"))
			if LastBlockResult != nil {
				breakValue := LastBlockResult
				LastBlockResult = nil
				data.pending = output[end:]
				return breakValue
			}
			if result != nil && result.Type == object.ValueException {
				data.pending = output[end:]
				return result
			}
		}
		return R.NilVal
	}
	return stringWithEncoding(output, "BINARY")
}

func zlibInflateAppend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibInflateState(receiver)
	if errVal != nil {
		return errVal
	}
	result := zlibInflateInstanceInflate(receiver, args[0])
	if result != nil && result.Type == object.ValueException {
		return result
	}
	if result != nil && result.Type == object.ValueString {
		data.pending += result.Data.(string)
	}
	return receiver
}

func zlibInflateFinish(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibInflateState(receiver)
	if errVal != nil {
		return errVal
	}
	if !data.finished {
		return newRuntimeException(R.Classes["Zlib::BufError"], "buffer error")
	}
	result := data.pending
	data.pending = ""
	if BlockGivenCheck != nil && BlockGivenCheck() && CurrentBlockValue != nil && CurrentBlockValue() != nil && CallBlockWithArgs != nil {
		for start := 0; start < len(result); start += 16384 {
			end := start + 16384
			if end > len(result) {
				end = len(result)
			}
			blockResult := CallBlockWithArgs(CurrentBlockValue(), stringWithEncoding(result[start:end], "BINARY"))
			if blockResult != nil && blockResult.Type == object.ValueException {
				data.pending = result[end:]
				return blockResult
			}
		}
		return R.NilVal
	}
	return stringWithEncoding(result, "BINARY")
}

func zlibInflateFlushNextOut(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibInflateState(receiver)
	if errVal != nil {
		return errVal
	}
	result := data.pending
	data.pending = ""
	return stringWithEncoding(result, "BINARY")
}
func zlibInflateFinished(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*zlibInflateData)
	return boolValue(data != nil && data.finished)
}
func zlibInflateClosed(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*zlibInflateData)
	return boolValue(data != nil && data.closed)
}
func zlibInflateClose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, _ := receiver.Data.(*zlibInflateData)
	if data != nil {
		data.closed = true
	}
	return R.NilVal
}

func zlibGzipWriterNew(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 0 || args[0] == nil {
		return NewArgumentError("wrong number of arguments")
	}
	klass, _ := receiver.Data.(*object.Class)
	if klass == nil {
		klass = R.Classes["Zlib::GzipWriter"]
	}
	return &object.EmeraldValue{Type: object.ValueObject, Data: &gzipWriterData{io: args[0], mtime: time.Unix(0, 0)}, Class: klass}
}

func zlibGzipWriterWrap(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	writer := zlibGzipWriterNew(receiver, args...)
	if writer.Type == object.ValueException {
		return writer
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return writer
	}
	result := CallBlockWithArgs(CurrentBlockValue(), writer)
	closeResult := zlibGzipWriterClose(writer)
	if result != nil && result.Type == object.ValueException {
		return result
	}
	if closeResult != nil && closeResult.Type == object.ValueException {
		return closeResult
	}
	return result
}

func zlibGzipWriterState(receiver *object.EmeraldValue) (*gzipWriterData, *object.EmeraldValue) {
	data, ok := receiver.Data.(*gzipWriterData)
	if !ok || data == nil {
		return nil, newRuntimeException(R.Classes["Zlib::GzipFile::Error"], "invalid gzip stream")
	}
	return data, nil
}

func zlibGzipWriterWrite(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibGzipWriterState(receiver)
	if errVal != nil {
		return errVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["Zlib::GzipFile::Error"], "closed gzip stream")
	}
	_, raw, errVal := cgiStringArg(args[0])
	if errVal != nil {
		return errVal
	}
	data.headerWritten = true
	data.content.WriteString(raw)
	return newInt(int64(len(raw)))
}

func zlibGzipWriterAppend(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result := zlibGzipWriterWrite(receiver, args...)
	if result != nil && result.Type == object.ValueException {
		return result
	}
	return receiver
}

func zlibGzipWriterMtime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibGzipWriterState(receiver)
	if errVal != nil {
		return errVal
	}
	return newTimeValue(data.mtime)
}

func zlibGzipWriterSetMtime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibGzipWriterState(receiver)
	if errVal != nil {
		return errVal
	}
	if data.headerWritten {
		return newRuntimeException(R.Classes["Zlib::GzipFile::Error"], "header is already written")
	}
	if value, ok := valueToInteger(args[0]); ok {
		data.mtime = time.Unix(value, 0)
		return args[0]
	}
	if value, ok := args[0].Data.(*timeData); ok && value != nil {
		data.mtime = value.value
		return args[0]
	}
	return typeError("can't convert into time")
}

func zlibGzipWriterClose(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibGzipWriterState(receiver)
	if errVal != nil {
		return errVal
	}
	if data.closed {
		return data.io
	}
	compressed, err := zlibCompressBytes([]byte(data.content.String()), -1, true)
	if err != nil {
		return newRuntimeException(R.Classes["Zlib::GzipFile::Error"], err.Error())
	}
	if len(compressed) >= 8 {
		binary.LittleEndian.PutUint32(compressed[4:8], uint32(data.mtime.Unix()))
	}
	if CallMethod != nil {
		result := CallMethod(data.io, "write", stringWithEncoding(string(compressed), "BINARY"))
		if result != nil && result.Type == object.ValueException {
			return result
		}
		if receiverHasCallableMethod(data.io, "close") {
			if result = CallMethod(data.io, "close"); result != nil && result.Type == object.ValueException {
				return result
			}
		}
	}
	data.closed = true
	return data.io
}

func zlibGzipWriterClosed(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibGzipWriterState(receiver)
	if errVal != nil {
		return errVal
	}
	return boolValue(data.closed)
}

func zlibGzipWriterOrigName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return zlibGzipWriterHeaderValue(receiver, true)
}

func zlibGzipWriterComment(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return zlibGzipWriterHeaderValue(receiver, false)
}

func zlibGzipWriterHeaderValue(receiver *object.EmeraldValue, origName bool) *object.EmeraldValue {
	data, errVal := zlibGzipWriterState(receiver)
	if errVal != nil {
		return errVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["Zlib::GzipFile::Error"], "closed gzip stream")
	}
	if origName && data.origName != "" {
		return rubyString(data.origName)
	}
	if !origName && data.comment != "" {
		return rubyString(data.comment)
	}
	return R.NilVal
}

func zlibGzipWriterSetOrigName(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return zlibGzipWriterSetHeader(receiver, true, args[0])
}

func zlibGzipWriterSetComment(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return zlibGzipWriterSetHeader(receiver, false, args[0])
}

func zlibGzipWriterSetHeader(receiver *object.EmeraldValue, origName bool, value *object.EmeraldValue) *object.EmeraldValue {
	data, errVal := zlibGzipWriterState(receiver)
	if errVal != nil {
		return errVal
	}
	if data.closed {
		return newRuntimeException(R.Classes["Zlib::GzipFile::Error"], "closed gzip stream")
	}
	if data.headerWritten {
		return newRuntimeException(R.Classes["Zlib::GzipFile::Error"], "header is already written")
	}
	_, raw, errVal := cgiStringArg(value)
	if errVal != nil {
		return errVal
	}
	if origName {
		data.origName = raw
	} else {
		data.comment = raw
	}
	return value
}

func gzipReaderState(receiver *object.EmeraldValue) (*gzipReaderData, *object.EmeraldValue) {
	data, ok := receiver.Data.(*gzipReaderData)
	if !ok || data == nil {
		return nil, newRuntimeException(R.Classes["Zlib::GzipFile::Error"], "invalid gzip stream")
	}
	return data, nil
}

func zlibGzipReaderPos(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := gzipReaderState(receiver)
	if errVal != nil {
		return errVal
	}
	return newInt(int64(data.position))
}

func zlibGzipReaderLineno(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := gzipReaderState(receiver)
	if errVal != nil {
		return errVal
	}
	return newInt(int64(data.lineno))
}

func zlibGzipReaderEOF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := gzipReaderState(receiver)
	if errVal != nil {
		return errVal
	}
	return boolValue(data.pushback == "" && data.offset >= len(data.content))
}

func zlibGzipReaderGetc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := gzipReaderState(receiver)
	if errVal != nil {
		return errVal
	}
	remaining := data.pushback + data.content[data.offset:]
	if remaining == "" {
		return R.NilVal
	}
	length := 1
	if normalizedEncodingName(data.externalEncoding) == "UTF_8" {
		_, length = utf8.DecodeRuneInString(remaining)
		if length < 1 {
			length = 1
		}
	}
	return zlibGzipReaderRead(receiver, newInt(int64(length)))
}

func zlibGzipReaderReadpartial(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) < 1 || len(args) > 2 {
		return NewArgumentError("wrong number of arguments")
	}
	return zlibGzipReaderRead(receiver, args[0])
}

func zlibGzipReaderGets(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := gzipReaderState(receiver)
	if errVal != nil {
		return errVal
	}
	separator := "\n"
	if len(args) > 0 {
		_, separator, errVal = cgiStringArg(args[0])
		if errVal != nil {
			return errVal
		}
	}
	remaining := data.pushback + data.content[data.offset:]
	if remaining == "" {
		return R.NilVal
	}
	consume := len(remaining)
	paragraph := ""
	if separator == "" {
		trimmed := strings.TrimLeft(remaining, "\n")
		skipped := len(remaining) - len(trimmed)
		if end := strings.Index(trimmed, "\n\n"); end >= 0 {
			paragraph = trimmed[:end+2]
			consume = skipped + end + 2
			for consume < len(remaining) && remaining[consume] == '\n' {
				consume++
			}
		}
	} else if end := strings.Index(remaining, separator); end >= 0 {
		consume = end + len(separator)
	}
	value := zlibGzipReaderRead(receiver, newInt(int64(consume)))
	if value != nil && value.Type == object.ValueString && separator == "" {
		if paragraph != "" {
			value.Data = paragraph
		} else {
			value.Data = strings.TrimLeft(value.Data.(string), "\n")
		}
	}
	data.lineno++
	return value
}

func zlibGzipReaderEachValue(receiver *object.EmeraldValue, next func() *object.EmeraldValue) *object.EmeraldValue {
	for {
		value := next()
		if value == nil || value == R.NilVal || value.Type == object.ValueNil {
			return receiver
		}
		if value.Type == object.ValueException {
			return value
		}
		LastBlockResult = nil
		ForEachClearNext()
		result := CallBlockWithArgs(CurrentBlockValue(), value)
		if result != nil && result.Type == object.ValueException {
			return result
		}
		if LastBlockResult != nil {
			control := LastBlockResult
			LastBlockResult = nil
			return control
		}
		if ForEachConsumeNext() {
			continue
		}
	}
}

func zlibGzipReaderEnumerator(next func() *object.EmeraldValue) *object.EmeraldValue {
	return newGeneratorEnumerator(func() ([]*object.EmeraldValue, *object.EmeraldValue) {
		values := []*object.EmeraldValue{}
		for {
			value := next()
			if value == nil || value == R.NilVal || value.Type == object.ValueNil {
				return values, nil
			}
			if value.Type == object.ValueException {
				return values, value
			}
			values = append(values, value)
		}
	})
}

func zlibGzipReaderEachLine(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	next := func() *object.EmeraldValue { return zlibGzipReaderGets(receiver, args...) }
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CurrentBlockValue() == nil {
		return zlibGzipReaderEnumerator(next)
	}
	return zlibGzipReaderEachValue(receiver, next)
}

func zlibGzipReaderEachChar(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	next := func() *object.EmeraldValue { return zlibGzipReaderGetc(receiver) }
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CurrentBlockValue() == nil {
		return zlibGzipReaderEnumerator(next)
	}
	return zlibGzipReaderEachValue(receiver, next)
}

func zlibGzipReaderEachByte(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	next := func() *object.EmeraldValue {
		value := zlibGzipReaderRead(receiver, newInt(1))
		if value == nil || value == R.NilVal || value.Type != object.ValueString || value.Data.(string) == "" {
			return R.NilVal
		}
		return newInt(int64(value.Data.(string)[0]))
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CurrentBlockValue() == nil {
		return zlibGzipReaderEnumerator(next)
	}
	return zlibGzipReaderEachValue(receiver, next)
}

func zlibGzipReaderRewind(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := gzipReaderState(receiver)
	if errVal != nil {
		return errVal
	}
	if data.source != nil && CallMethod != nil && receiverHasCallableMethod(data.source, "seek") {
		if result := CallMethod(data.source, "seek", newInt(0)); result != nil && result.Type == object.ValueException {
			return result
		}
	}
	data.offset, data.position, data.lineno, data.pushback = 0, 0, 0, ""
	return newInt(0)
}

func zlibGzipReaderUngetc(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := gzipReaderState(receiver)
	if errVal != nil {
		return errVal
	}
	if args[0] == nil || args[0].Type == object.ValueNil {
		return R.NilVal
	}
	value := ""
	if integer, ok := valueToInteger(args[0]); ok {
		value = string(rune(integer))
	} else {
		_, value, errVal = cgiStringArg(args[0])
		if errVal != nil {
			return errVal
		}
	}
	data.pushback = value + data.pushback
	data.position -= len(value)
	return R.NilVal
}

func zlibGzipReaderUngetbyte(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if args[0] == nil || args[0].Type == object.ValueNil {
		return R.NilVal
	}
	value, ok := valueToInteger(args[0])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	return zlibGzipReaderUngetc(receiver, stringWithEncoding(string([]byte{byte(value)}), "BINARY"))
}

func zlibGzipReaderMtime(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	data, errVal := gzipReaderState(receiver)
	if errVal != nil {
		return errVal
	}
	return newTimeValue(data.mtime)
}
