package core

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/GoLangDream/rgo/pkg/object"
)

func pathnameValue(path string) *object.EmeraldValue {
	return &object.EmeraldValue{Type: object.ValueString, Data: filepath.ToSlash(path), Class: R.Classes["Pathname"]}
}

func pathnameKernel(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) == 1 && args[0] != nil && args[0].Class == R.Classes["Pathname"] {
		return args[0]
	}
	return pathnameClassNew(receiver, args...)
}

func pathnameClassPwd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, err := os.Getwd()
	if err != nil {
		return newRuntimeException(R.Classes["SystemCallError"], err.Error())
	}
	return pathnameValue(path)
}

func pathnameCompare(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 1 {
		return R.NilVal
	}
	left, leftOK := pathnameString(receiver)
	right, rightOK := pathnameString(args[0])
	if !leftOK || !rightOK || args[0].Class != R.Classes["Pathname"] {
		return R.NilVal
	}
	result := int64(strings.Compare(left, right))
	return &object.EmeraldValue{Type: object.ValueInteger, Data: result, Class: R.Classes["Integer"]}
}

func pathnameHash(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	return CallMethod(rubyString(path), "hash")
}

func pathnameInspect(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	return rubyString("#<Pathname:" + path + ">")
}

func pathnameAbsolute(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	return boolValue(filepath.IsAbs(path))
}

func pathnameRelative(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	return boolValue(!filepath.IsAbs(path))
}

func pathnameRoot(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	return boolValue(path == string(filepath.Separator))
}

func pathnameEmpty(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	info, err := os.Stat(path)
	if err != nil {
		return newRuntimeException(R.Classes["SystemCallError"], err.Error())
	}
	if !info.IsDir() {
		return boolValue(info.Size() == 0)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return newRuntimeException(R.Classes["SystemCallError"], err.Error())
	}
	return boolValue(len(entries) == 0)
}

func pathnameExist(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	_, err := os.Stat(path)
	return boolValue(err == nil)
}

func pathnameFile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	info, err := os.Stat(path)
	return boolValue(err == nil && info.Mode().IsRegular())
}

func pathnameDirectory(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	info, err := os.Stat(path)
	return boolValue(err == nil && info.IsDir())
}

func pathnameJoin(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	result, ok := pathnameString(receiver)
	if !ok {
		return typeError("no implicit conversion into String")
	}
	for _, arg := range args {
		path, errVal := coercePath(arg)
		if errVal != nil {
			return errVal
		}
		if filepath.IsAbs(path) {
			result = filepath.Clean(path)
		} else {
			result = filepath.Clean(filepath.Join(result, path))
		}
	}
	return pathnameValue(result)
}

func pathnameParent(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	return pathnameValue(filepath.Dir(filepath.Clean(path)))
}

func pathnameCleanpath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) > 1 {
		return argumentError("wrong number of arguments")
	}
	path, _ := pathnameString(receiver)
	return pathnameValue(filepath.Clean(path))
}

func pathnameRealpath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return pathnameResolve(receiver, args)
}

func pathnameRealdirpath(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return pathnameResolve(receiver, args)
}

func pathnameResolve(receiver *object.EmeraldValue, args []*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	if len(args) > 1 {
		return argumentError("wrong number of arguments")
	}
	if len(args) == 1 {
		base, errVal := coercePath(args[0])
		if errVal != nil {
			return errVal
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(base, path)
		}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return newRuntimeException(R.Classes["SystemCallError"], err.Error())
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return newRuntimeException(R.Classes["SystemCallError"], err.Error())
	}
	return pathnameValue(abs)
}

func pathnameSub(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, _ := pathnameString(receiver)
	result := stringSub(rubyString(path), args...)
	if result == nil || result.Type != object.ValueString {
		return result
	}
	return pathnameValue(result.Data.(string))
}
