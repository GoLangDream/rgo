package core

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/GoLangDream/rgo/pkg/object"
)

var fileUtilsCommands = []string{
	"cd", "chdir", "chmod", "compare_file", "copy", "copy_file", "cp",
	"ln", "ln_s", "mkdir", "mkdir_p", "mkpath", "move", "mv", "pwd",
	"remove", "remove_dir", "remove_entry", "rm", "rm_f", "rm_r", "rm_rf",
	"rmdir", "touch", "uptodate?",
}

func installFileUtilsModule(objectClass *object.Class) {
	if objectClass == nil {
		return
	}
	if existing := objectClass.Constants["FileUtils"]; existing != nil && existing.Type == object.ValueModule {
		return
	}

	mod := object.NewModule("FileUtils")
	define := func(name string, fn func(*object.EmeraldValue, ...*object.EmeraldValue) *object.EmeraldValue, arity int) {
		mod.DefineMethod(name, &object.Method{Name: name, Fn: fn, Arity: arity})
	}
	define("commands", fileUtilsCommandsMethod, 0)
	define("options_of", fileUtilsOptionsOf, 1)
	define("pwd", fileUtilsPwd, 0)
	define("getwd", fileUtilsPwd, 0)
	define("cd", fileUtilsCd, -1)
	define("chdir", fileUtilsCd, -1)
	define("mkdir", fileUtilsMkdir, -1)
	define("mkdir_p", fileUtilsMkdirP, -1)
	define("mkpath", fileUtilsMkdirP, -1)
	define("rmdir", fileUtilsRmdir, -1)
	define("rm", fileUtilsRm, -1)
	define("remove", fileUtilsRm, -1)
	define("rm_f", fileUtilsRmF, -1)
	define("rm_r", fileUtilsRmR, -1)
	define("rm_rf", fileUtilsRmRF, -1)
	define("remove_dir", fileUtilsRmRF, -1)
	define("remove_entry", fileUtilsRmRF, -1)
	define("cp", fileUtilsCopyFile, -1)
	define("copy", fileUtilsCopyFile, -1)
	define("copy_file", fileUtilsCopyFile, -1)
	define("mv", fileUtilsMove, -1)
	define("move", fileUtilsMove, -1)
	define("ln", fileUtilsLink, -1)
	define("ln_s", fileUtilsSymlink, -1)
	define("chmod", fileUtilsChmod, -1)
	define("touch", fileUtilsTouch, -1)
	define("compare_file", fileUtilsCompareFile, 2)
	define("uptodate?", fileUtilsUptodate, 2)

	moduleValue := &object.EmeraldValue{Type: object.ValueModule, Data: mod, Class: R.Classes["Module"]}
	objectClass.DefineConstant("FileUtils", moduleValue)
	AssignConstantName(&object.EmeraldValue{Type: object.ValueClass, Data: objectClass, Class: R.Classes["Class"]}, "FileUtils", moduleValue)
}

func fileUtilsCommandsMethod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	values := make([]*object.EmeraldValue, 0, len(fileUtilsCommands))
	for _, name := range fileUtilsCommands {
		values = append(values, rubyString(name))
	}
	return &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: R.Classes["Array"]}
}

func fileUtilsOptionsOf(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	// FileUtilsExt uses this metadata only to wrap verbose/noop options. Native
	// methods already accept and ignore those options, so no wrapper is needed.
	return &object.EmeraldValue{Type: object.ValueArray, Data: []*object.EmeraldValue{}, Class: R.Classes["Array"]}
}

func fileUtilsArgs(args []*object.EmeraldValue) []*object.EmeraldValue {
	if len(args) > 0 && args[len(args)-1] != nil && args[len(args)-1].Type == object.ValueHash {
		return args[:len(args)-1]
	}
	return args
}

func fileUtilsPaths(args []*object.EmeraldValue) ([]string, *object.EmeraldValue) {
	args = fileUtilsArgs(args)
	paths := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != nil && arg.Type == object.ValueArray {
			nested, errVal := fileUtilsPaths(arg.Data.([]*object.EmeraldValue))
			if errVal != nil {
				return nil, errVal
			}
			paths = append(paths, nested...)
			continue
		}
		path, errVal := coercePath(arg)
		if errVal != nil {
			return nil, errVal
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func fileUtilsPwd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	path, err := os.Getwd()
	if err != nil {
		return errnoForPathError(err)
	}
	return rubyString(path)
}

func fileUtilsCd(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	args = fileUtilsArgs(args)
	if len(args) != 1 {
		return NewArgumentError("wrong number of arguments")
	}
	path, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	oldPath, err := os.Getwd()
	if err != nil {
		return errnoForPathError(err)
	}
	if err := os.Chdir(path); err != nil {
		return errnoForPathError(err)
	}
	if BlockGivenCheck == nil || !BlockGivenCheck() || CurrentBlockValue == nil || CallBlockWithArgs == nil {
		return R.NilVal
	}
	result := CallBlockWithArgs(CurrentBlockValue(), rubyString(path))
	if restoreErr := os.Chdir(oldPath); restoreErr != nil && (result == nil || result.Type != object.ValueException) {
		return errnoForPathError(restoreErr)
	}
	return result
}

func fileUtilsMkdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	paths, errVal := fileUtilsPaths(args)
	if errVal != nil {
		return errVal
	}
	for _, path := range paths {
		if err := os.Mkdir(path, os.FileMode(0777&^currentFileUmask)); err != nil {
			return errnoForPathError(err)
		}
	}
	return R.NilVal
}

func fileUtilsMkdirP(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	paths, errVal := fileUtilsPaths(args)
	if errVal != nil {
		return errVal
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, os.FileMode(0777&^currentFileUmask)); err != nil {
			return errnoForPathError(err)
		}
	}
	return R.NilVal
}

func fileUtilsRmdir(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return fileUtilsRemovePaths(args, false, false)
}

func fileUtilsRm(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return fileUtilsRemovePaths(args, false, false)
}

func fileUtilsRmF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return fileUtilsRemovePaths(args, false, true)
}

func fileUtilsRmR(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return fileUtilsRemovePaths(args, true, false)
}

func fileUtilsRmRF(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return fileUtilsRemovePaths(args, true, true)
}

func fileUtilsRemovePaths(args []*object.EmeraldValue, recursive, force bool) *object.EmeraldValue {
	paths, errVal := fileUtilsPaths(args)
	if errVal != nil {
		return errVal
	}
	for _, path := range paths {
		var err error
		if recursive {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
		if err != nil && !(force && os.IsNotExist(err)) {
			return errnoForPathError(err)
		}
	}
	return R.NilVal
}

func fileUtilsCopyFile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	args = fileUtilsArgs(args)
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	source, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	destination, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		destination = filepath.Join(destination, filepath.Base(source))
	}
	input, err := os.Open(source)
	if err != nil {
		return errnoForPathError(err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return errnoForPathError(err)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return errnoForPathError(err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return errnoForPathError(copyErr)
	}
	if closeErr != nil {
		return errnoForPathError(closeErr)
	}
	return R.NilVal
}

func fileUtilsMove(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	args = fileUtilsArgs(args)
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	source, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	destination, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	if info, err := os.Stat(destination); err == nil && info.IsDir() {
		destination = filepath.Join(destination, filepath.Base(source))
	}
	if err := os.Rename(source, destination); err != nil {
		return errnoForPathError(err)
	}
	return R.NilVal
}

func fileUtilsLink(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return fileUtilsCreateLink(args, false)
}

func fileUtilsSymlink(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	return fileUtilsCreateLink(args, true)
}

func fileUtilsCreateLink(args []*object.EmeraldValue, symbolic bool) *object.EmeraldValue {
	args = fileUtilsArgs(args)
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	source, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	destination, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	var err error
	if symbolic {
		err = os.Symlink(source, destination)
	} else {
		err = os.Link(source, destination)
	}
	if err != nil {
		return errnoForPathError(err)
	}
	return R.NilVal
}

func fileUtilsChmod(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	args = fileUtilsArgs(args)
	if len(args) < 2 {
		return NewArgumentError("wrong number of arguments")
	}
	mode, ok := valueToInteger(args[0])
	if !ok {
		return typeError("no implicit conversion into Integer")
	}
	paths, errVal := fileUtilsPaths(args[1:])
	if errVal != nil {
		return errVal
	}
	for _, path := range paths {
		if err := os.Chmod(path, unixModeToFileMode(os.FileMode(mode&07777))); err != nil {
			return errnoForPathError(err)
		}
	}
	return R.NilVal
}

func fileUtilsTouch(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	paths, errVal := fileUtilsPaths(args)
	if errVal != nil {
		return errVal
	}
	now := time.Now()
	for _, path := range paths {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, os.FileMode(0666&^currentFileUmask))
		if err != nil {
			return errnoForPathError(err)
		}
		if err := file.Close(); err != nil {
			return errnoForPathError(err)
		}
		if err := os.Chtimes(path, now, now); err != nil {
			return errnoForPathError(err)
		}
	}
	return R.NilVal
}

func fileUtilsCompareFile(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	left, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	right, errVal := coercePath(args[1])
	if errVal != nil {
		return errVal
	}
	leftData, err := os.ReadFile(left)
	if err != nil {
		return errnoForPathError(err)
	}
	rightData, err := os.ReadFile(right)
	if err != nil {
		return errnoForPathError(err)
	}
	return boolValue(bytes.Equal(leftData, rightData))
}

func fileUtilsUptodate(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
	if len(args) != 2 {
		return NewArgumentError("wrong number of arguments")
	}
	target, errVal := coercePath(args[0])
	if errVal != nil {
		return errVal
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return R.FalseVal
		}
		return errnoForPathError(err)
	}
	sources, errVal := fileUtilsPaths([]*object.EmeraldValue{args[1]})
	if errVal != nil {
		return errVal
	}
	for _, source := range sources {
		sourceInfo, err := os.Stat(source)
		if err != nil {
			return errnoForPathError(err)
		}
		if sourceInfo.ModTime().After(targetInfo.ModTime()) {
			return R.FalseVal
		}
	}
	return R.TrueVal
}
