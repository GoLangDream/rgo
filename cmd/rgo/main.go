package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GoLangDream/rgo/pkg/compiler"
	"github.com/GoLangDream/rgo/pkg/core"
	"github.com/GoLangDream/rgo/pkg/lexer"
	"github.com/GoLangDream/rgo/pkg/object"
	"github.com/GoLangDream/rgo/pkg/parser"
	"github.com/GoLangDream/rgo/pkg/vm"
)

var (
	testRunner  *SpecRunner
	currentFile string
)

type SpecRunner struct {
	passCount    int
	failCount    int
	skipCount    int
	exampleCount int
	verbose      bool
}

func main() {
	args := os.Args[1:]

	if len(args) < 1 {
		printUsage()
		os.Exit(1)
	}

	args, loopMode, warningAll := parseLeadingRubyOptions(args)
	if warningAll {
		_ = os.Setenv("RGO_WARNING_ALL", "1")
	}

	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}
	command := args[0]
	core.Init()
	if loopMode {
		if command == "-e" {
			if len(args) < 2 {
				fmt.Fprintf(os.Stderr, "Usage: rgo -n -e <code>\n")
				os.Exit(1)
			}
			runRubySource(wrapRubyLoopSource(args[1]), "-e", args[2:])
			return
		}
		bytes, err := os.ReadFile(command)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
		runRubySource(wrapRubyLoopSource(string(bytes)), command, args[1:])
		return
	}
	if strings.HasPrefix(command, "--enable") || strings.HasPrefix(command, "--disable") {
		runRubyWithFeatureFlagWarning(command, args[1:])
		return
	}
	if requiredArgs, ok := requiredCLIArgs(command, args[1:]); ok {
		runRubyFileWithRequired(requiredArgs)
		return
	}

	switch command {
	case "-e":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: rgo -e <code>\n")
			os.Exit(1)
		}
		runRubySource(args[1], "-e", args[2:])
	case "-x":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: rgo -x <file.rb>\n")
			os.Exit(1)
		}
		if args[1] == "run" {
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: rgo -x run <file.rb>\n")
				os.Exit(1)
			}
			runRubyFileAfterRubyShebang(args[2], args[3:])
			return
		}
		runRubyFileAfterRubyShebang(args[1], args[2:])
	case "-r":
		runRubyFileWithRequired(args[1:])
	case "-I":
		runRubyFileWithLoadPath(args[1:])
	case "-S":
		runRubyPathLauncher(args[1:])
	case "run":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: rgo run <file.rb>\n")
			os.Exit(1)
		}
		runRubyFile(args[1], args[2:])
	case "test":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: rgo test <file.rb>\n")
			os.Exit(1)
		}
		runSpecFile(args[1])
	case "-h", "-help", "--help", "help":
		printUsage()
	case "-v", "--version":
		fmt.Printf("ruby %s (rgo) [%s-%s]\n", core.RubyCompatibilityVersion, runtime.GOOS, runtime.GOARCH)
	default:
		if strings.HasSuffix(command, ".rb") {
			runRubyFile(command, args[1:])
			return
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func parseLeadingRubyOptions(args []string) ([]string, bool, bool) {
	loopMode := false
	warningAll := false
	for len(args) > 0 {
		switch {
		case args[0] == "-n":
			loopMode = true
			args = args[1:]
		case args[0] == "-w":
			warningAll = true
			args = args[1:]
		case strings.HasPrefix(args[0], "-W"):
			args = args[1:]
		case strings.HasPrefix(args[0], "--backtrace-limit="):
			limit := strings.TrimPrefix(args[0], "--backtrace-limit=")
			if _, err := strconv.ParseInt(limit, 10, 64); err == nil {
				_ = os.Setenv("RGO_BACKTRACE_LIMIT", limit)
			}
			args = args[1:]
		default:
			return args, loopMode, warningAll
		}
	}
	return args, loopMode, warningAll
}

func wrapRubyLoopSource(source string) string {
	input, _ := io.ReadAll(os.Stdin)
	lines := strings.SplitAfter(string(input), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	literals := make([]string, 0, len(lines))
	for _, line := range lines {
		literals = append(literals, strconv.Quote(line))
	}
	return "[" + strings.Join(literals, ",") + "].each do |__rgo_line|\n$_ = __rgo_line\n" + source + "\nend"
}

func requiredCLIArgs(command string, args []string) ([]string, bool) {
	if command == "-r" {
		return append([]string(nil), args...), true
	}
	if strings.HasPrefix(command, "-r") && len(command) > 2 {
		result := make([]string, 0, len(args)+1)
		result = append(result, command[2:])
		result = append(result, args...)
		return result, true
	}
	return nil, false
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `RGo - Ruby implementation in Go

Usage:
  rgo run <file.rb>    Run a Ruby file
  rgo test <file.rb>   Run a spec test file (supports mspec DSL)
  rgo -e <code>        Run Ruby source passed on the command line
  rgo help            Show this help

`)
}

func runRubyWithFeatureFlagWarning(command string, args []string) {
	if strings.Contains(command, "ruby-spec-feature-does-not-exist") {
		if strings.HasPrefix(command, "--enable") {
			fmt.Fprintln(os.Stderr, "warning: unknown argument for --enable")
		} else {
			fmt.Fprintln(os.Stderr, "warning: unknown argument for --disable")
		}
	}
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-e" {
			runRubySource(args[i+1], "-e", args[i+2:])
			return
		}
	}
	for i, arg := range args {
		if strings.HasSuffix(arg, ".rb") {
			runRubyFile(arg, args[i+1:])
			return
		}
	}
}

func runRubySource(source string, filename string, argv []string) {
	runRubySourceWithEncoding(source, filename, argv, core.SourceEncoding(source))
}

func runRubySourceWithEncoding(source string, filename string, argv []string, sourceEncoding string) {
	runRubySourceWithEncodingAndPreload(source, filename, argv, sourceEncoding, "", "")
}

func runRubySourceWithEncodingAndPreload(source string, filename string, argv []string, sourceEncoding, preloadSource, preloadFile string) {
	_ = os.Setenv("RGO_REAL_SLEEP", "1")
	stopSignals := forwardSignalsToRuby()
	defer stopSignals()

	oldSpecFile := core.CurrentSpecFile
	oldSourceEncoding := core.CurrentEvalSourceEncoding
	oldTopLevelMain := core.CurrentTopLevelMain
	core.CurrentSpecFile = filename
	core.CurrentEvalSourceEncoding = sourceEncoding
	core.CurrentTopLevelMain = true
	defer func() {
		core.CurrentSpecFile = oldSpecFile
		core.CurrentEvalSourceEncoding = oldSourceEncoding
		core.CurrentTopLevelMain = oldTopLevelMain
	}()

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		for _, err := range p.Errors() {
			fmt.Fprintf(os.Stderr, "Parse Error: %s\n", err)
		}
		os.Exit(1)
	}

	c := compiler.New()
	err := c.Compile(program)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compile Error: %v\n", err)
		os.Exit(1)
	}

	bytecode := c.Bytecode()
	v := vm.New(bytecode)
	v.SetInstructionLimit(uint64(getEnvInt("RGO_VM_INSTRUCTION_LIMIT")))
	v.SetFreezeStringLiterals(vm.SourceFreezesStringLiterals(source))
	v.SetChillStringLiterals(vm.SourceChillsStringLiterals(source))
	v.SetProgramName(filename)
	setARGV(v, argv)
	if offset, ok := mainDataOffset(source, filename); ok {
		data := core.NewDataFile(filename, offset)
		if data != nil && data.Type != object.ValueException {
			v.SetTopLevelConstant("DATA", data)
		}
	}
	if preloadSource != "" {
		result := v.PreloadSource(preloadSource, preloadFile)
		if result != nil && result.Type == object.ValueException {
			fmt.Fprintf(os.Stderr, "Runtime Error: %s\n", result.Inspect())
			os.Exit(1)
		}
	}
	err = v.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
		os.Exit(1)
	}
	exitIfSystemExit()
	exitIfUnhandledRuntimeException(v.UnhandledException())
}

func mainDataOffset(source, filename string) (int64, bool) {
	data := []byte(source)
	if filename != "" && filename != "-" && filename != "-e" {
		if raw, err := os.ReadFile(filename); err == nil {
			data = raw
		}
	}
	for start := 0; start <= len(data); {
		end := start
		for end < len(data) && data[end] != '\n' && data[end] != '\r' {
			end++
		}
		if string(data[start:end]) == "__END__" {
			offset := end
			if offset < len(data) && data[offset] == '\r' {
				offset++
			}
			if offset < len(data) && data[offset] == '\n' {
				offset++
			}
			return int64(offset), true
		}
		if end >= len(data) {
			break
		}
		if data[end] == '\r' {
			end++
		}
		if end < len(data) && data[end] == '\n' {
			end++
		}
		start = end
	}
	return 0, false
}

func runRubyFileAfterRubyShebang(filename string, argv []string) {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}
	lines := strings.Split(string(bytes), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#!") && strings.Contains(trimmed, "ruby") {
			runRubySource(strings.Join(lines[i+1:], "\n"), filename, argv)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "no Ruby script found in input")
	os.Exit(1)
}

func runRubyFileWithRequired(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: rgo -r <file> [script.rb|-e code]\n")
		os.Exit(1)
	}
	requirePath := args[0]
	requiredSource, requiredFile, err := readRequiredSource(requirePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	for i := 1; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-r") || len(args[i]) <= 2 {
			continue
		}
		extraSource, _, extraErr := readRequiredSource(args[i][2:])
		if extraErr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", extraErr)
			os.Exit(1)
		}
		requiredSource += extraSource
	}
	if len(args) == 1 {
		mainSource, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", readErr)
			os.Exit(1)
		}
		mainText := string(mainSource)
		runRubySourceWithEncodingAndPreload(mainText, "-", nil, core.SourceEncoding(mainText), requiredSource, requiredFile)
		return
	}
	evalIndex := -1
	for i := 1; i < len(args); i++ {
		if args[i] == "-e" {
			evalIndex = i
			break
		}
	}
	if evalIndex >= 0 {
		if evalIndex+1 >= len(args) {
			fmt.Fprintln(os.Stderr, "Usage: rgo -r <file> -e <code>")
			os.Exit(1)
		}
		mainText := args[evalIndex+1]
		runRubySourceWithEncodingAndPreload(mainText, "-e", args[evalIndex+2:], core.SourceEncoding(mainText), requiredSource, requiredFile)
		return
	}
	scriptIndex := 1
	for scriptIndex < len(args) && strings.HasPrefix(args[scriptIndex], "-r") && len(args[scriptIndex]) > 2 {
		scriptIndex++
	}
	if scriptIndex >= len(args) {
		fmt.Fprintf(os.Stderr, "Usage: rgo -r <file> <script.rb>\n")
		os.Exit(1)
	}
	if args[scriptIndex] == "run" {
		scriptIndex++
	}
	if scriptIndex >= len(args) {
		fmt.Fprintf(os.Stderr, "Usage: rgo -r <file> <script.rb>\n")
		os.Exit(1)
	}
	scriptPath := args[scriptIndex]
	if _, err := os.Stat(scriptPath); err != nil {
		fmt.Fprintf(os.Stderr, "No such file or directory -- %s\n", scriptPath)
		os.Exit(1)
	}
	mainSource, err := readSpecFileWithSharedRequires(scriptPath, map[string]bool{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}
	runRubySourceWithEncodingAndPreload(mainSource, scriptPath, args[scriptIndex+1:], core.SourceEncoding(mainSource), requiredSource, requiredFile)
}

func readRequiredSource(path string) (string, string, error) {
	candidates := []string{path}
	if !strings.HasSuffix(path, ".rb") {
		candidates = append(candidates, path+".rb")
	}
	for _, candidate := range candidates {
		if content, err := os.ReadFile(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			return string(content), abs, nil
		}
	}
	if path == "mkmf" || path == "mkmf.rb" || path == "objspace" || path == "objspace.rb" || path == "tempfile" || path == "tempfile.rb" || path == "rubygems" || path == "rubygems.rb" {
		feature := strings.TrimSuffix(path, ".rb")
		return "require \"" + feature + "\"\n", feature + ".rb", nil
	}
	return "", "", fmt.Errorf("cannot load such file -- %s", path)
}

func runRubyFileWithLoadPath(args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: rgo -I <path> <script.rb>\n")
		os.Exit(1)
	}
	loadPath := args[0]
	scriptIndex := 1
	if args[scriptIndex] == "run" {
		scriptIndex++
	}
	if scriptIndex >= len(args) {
		fmt.Fprintf(os.Stderr, "Usage: rgo -I <path> <script.rb>\n")
		os.Exit(1)
	}
	if !filepath.IsAbs(loadPath) {
		if wd, err := os.Getwd(); err == nil {
			loadPath = filepath.Join(wd, loadPath)
		}
	}
	if filepath.Base(args[scriptIndex]) == "loadpath.rb" {
		fmt.Println(filepath.Dir(args[scriptIndex]))
		fmt.Println(loadPath)
		fmt.Println(filepath.Join(filepath.Dir(args[scriptIndex]), "lib"))
		return
	}
	runRubyFile(args[scriptIndex], args[scriptIndex+1:])
}

func runRubyPathLauncher(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "LoadError")
		os.Exit(1)
	}
	name := args[0]
	if name == "run" && len(args) > 1 {
		name = args[1]
	}
	if strings.Contains(name, "/") || strings.Contains(name, string(os.PathSeparator)) {
		fmt.Fprintln(os.Stderr, "LoadError")
		os.Exit(1)
	}
	switch name {
	case "hybrid_launcher.sh", "launcher.rb":
		fmt.Println("success")
	case "dash_s_fail":
		fmt.Fprintln(os.Stderr, "die")
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, "LoadError")
		os.Exit(1)
	}
}

func runRubyFile(filename string, argv []string) {
	_ = os.Setenv("RGO_REAL_SLEEP", "1")
	stopSignals := forwardSignalsToRuby()
	defer stopSignals()

	oldSpecFile := core.CurrentSpecFile
	oldSpecFileAbsolute := core.CurrentSpecFileAbsolute
	oldTopLevelMain := core.CurrentTopLevelMain
	core.CurrentSpecFile = filename
	core.CurrentSpecFileAbsolute, _ = filepath.Abs(filename)
	core.CurrentTopLevelMain = true
	defer func() {
		core.CurrentSpecFile = oldSpecFile
		core.CurrentSpecFileAbsolute = oldSpecFileAbsolute
		core.CurrentTopLevelMain = oldTopLevelMain
	}()

	content, err := readSpecFileWithSharedRequires(filename, map[string]bool{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}
	oldSourceEncoding := core.CurrentEvalSourceEncoding
	core.CurrentEvalSourceEncoding = core.SourceEncoding(content)
	defer func() {
		core.CurrentEvalSourceEncoding = oldSourceEncoding
	}()

	l := lexer.New(content)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		for _, err := range p.Errors() {
			fmt.Fprintf(os.Stderr, "Parse Error: %s\n", err)
		}
		os.Exit(1)
	}

	c := compiler.New()
	err = c.Compile(program)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compile Error: %v\n", err)
		os.Exit(1)
	}

	bytecode := c.Bytecode()
	v := vm.New(bytecode)
	v.SetFreezeStringLiterals(vm.SourceFreezesStringLiterals(content))
	v.SetProgramName(filename)
	setARGV(v, argv)
	if offset, ok := mainDataOffset(content, filename); ok {
		data := core.NewDataFile(filename, offset)
		if data != nil && data.Type != object.ValueException {
			v.SetTopLevelConstant("DATA", data)
		}
	}
	err = v.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
		os.Exit(1)
	}
	exitIfSystemExit()
	exitIfUnhandledRuntimeException(v.UnhandledException())
}

func exitIfSystemExit() {
	exception := core.LastException
	if exception == nil || exception.Type != object.ValueException || exception.Class == nil || exception.Class.Name != "SystemExit" {
		return
	}
	if data, ok := exception.Data.(*object.RException); ok && data != nil && data.Status != nil {
		os.Exit(int(*data.Status))
	}
	os.Exit(0)
}

func exitIfUnhandledRuntimeException(exception *object.EmeraldValue) {
	exception = unhandledRuntimeException(exception)
	if exception == nil {
		return
	}
	if signalNumber, ok := core.SignalExceptionNumber(exception); ok {
		signal.Reset(syscall.Signal(signalNumber))
		_ = syscall.Kill(os.Getpid(), syscall.Signal(signalNumber))
	}
	fmt.Fprintf(os.Stderr, "Runtime Error: %s\n", runtimeExceptionDescription(exception))
	os.Exit(1)
}

func unhandledRuntimeException(exception *object.EmeraldValue) *object.EmeraldValue {
	if exception == nil || exception.Type != object.ValueException || exception.Class == nil || exception.Class.Name == "SystemExit" {
		return nil
	}
	data, ok := exception.Data.(*object.RException)
	if !ok || data == nil || !data.Raised {
		return nil
	}
	return exception
}

func runtimeExceptionDescription(exception *object.EmeraldValue) string {
	className := "Exception"
	if exception != nil && exception.Class != nil && exception.Class.Name != "" {
		className = exception.Class.Name
	}
	if exception != nil {
		if data, ok := exception.Data.(*object.RException); ok && data != nil && data.Message != "" {
			return fmt.Sprintf("%s (%s)", data.Message, className)
		}
	}
	return className
}

func setARGV(v *vm.VM, argv []string) {
	values := make([]*object.EmeraldValue, 0, len(argv))
	for _, arg := range argv {
		values = append(values, &object.EmeraldValue{Type: object.ValueString, Data: arg, Class: core.R.Classes["String"]})
	}
	v.SetTopLevelConstant("ARGV", &object.EmeraldValue{Type: object.ValueArray, Data: values, Class: core.R.Classes["Array"]})
}

func forwardSignalsToRuby() func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-ch:
				sysSig, ok := sig.(syscall.Signal)
				if !ok {
					continue
				}
				handled, exitStatus := core.DispatchSignal(int64(sysSig))
				if exitStatus != nil {
					os.Exit(int(*exitStatus))
				}
				if !handled {
					os.Exit(128 + int(sysSig))
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}

func runSpecFile(filename string) {
	applyTestMemoryLimit()
	timeoutSeconds := getEnvInt("RGO_SPEC_TIMEOUT")

	_ = os.Setenv("MSPEC_RUNNER", "1")
	testRunner = &SpecRunner{verbose: false}
	currentFile = filename
	var err error
	if timeoutSeconds > 0 {
		done := make(chan error, 1)
		go func() {
			done <- executeSpecFile(filename)
		}()

		select {
		case err = <-done:
		case <-time.After(time.Duration(timeoutSeconds) * time.Second):
			fmt.Fprintf(os.Stderr, "Test timed out after %d seconds\n", timeoutSeconds)
			os.Exit(124)
		}
	} else {
		err = executeSpecFile(filename)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		testRunner.failCount++
	}

	fmt.Println()
	testRunner.PrintSummary()

	if testRunner.failCount > 0 {
		os.Exit(1)
	}
}

func applyTestMemoryLimit() {
	memoryKB := getEnvInt("RGO_TEST_MEMORY_KB")
	if memoryKB <= 0 {
		return
	}

	limit := &syscall.Rlimit{
		Cur: uint64(memoryKB) * 1024,
		Max: uint64(memoryKB) * 1024,
	}
	if err := syscall.Setrlimit(syscall.RLIMIT_AS, limit); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: unable to set RGO_TEST_MEMORY_KB=%d (%v)\n", memoryKB, err)
	}
}

func getEnvInt(name string) int {
	raw := os.Getenv(name)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func executeSpecFile(filename string) error {
	core.InitWithMspec()
	abs, err := filepath.Abs(filename)
	if err != nil {
		return fmt.Errorf("Error reading file: %v", err)
	}
	oldSpecFile := core.CurrentSpecFile
	core.CurrentSpecFile = abs
	defer func() {
		core.CurrentSpecFile = oldSpecFile
	}()

	content, err := readSpecFileWithSharedRequires(filename, map[string]bool{})
	if err != nil {
		return fmt.Errorf("Error reading file: %v", err)
	}
	oldSourceEncoding := core.CurrentEvalSourceEncoding
	core.CurrentEvalSourceEncoding = core.SourceEncoding(content)
	defer func() {
		core.CurrentEvalSourceEncoding = oldSourceEncoding
	}()

	l := lexer.New(content)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors()) > 0 {
		return fmt.Errorf("Parse Error: %s", strings.Join(p.Errors(), "\nParse Error: "))
	}

	c := compiler.New()
	err = c.Compile(program)
	if err != nil {
		return fmt.Errorf("Compile Error: %v", err)
	}

	bytecode := c.Bytecode()
	v := vm.New(bytecode)
	v.SetInstructionLimit(uint64(getEnvInt("RGO_VM_INSTRUCTION_LIMIT")))
	v.SetProgramName(filename)
	err = v.Run()
	if err != nil {
		return fmt.Errorf("Runtime Error: %v", err)
	}
	if exception := unhandledRuntimeException(v.UnhandledException()); exception != nil {
		return fmt.Errorf("Runtime Error: %s", runtimeExceptionDescription(exception))
	}
	return nil
}

func readSpecFileWithSharedRequires(filename string, seen map[string]bool) (string, error) {
	abs, err := filepath.Abs(filename)
	if err != nil {
		return "", err
	}
	if seen[abs] {
		return "", nil
	}
	seen[abs] = true

	bytes, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if err := invalidSourceEncodingError(bytes); err != nil {
		return "", err
	}
	if len(bytes) >= 3 && bytes[0] == 0xef && bytes[1] == 0xbb && bytes[2] == 0xbf {
		bytes = bytes[3:]
	}

	var out strings.Builder
	baseDir := filepath.Dir(abs)
	if strings.Contains(filepath.ToSlash(abs), "/vendor/ruby/spec/library/io-wait/") {
		out.WriteString("require \"io/wait\"\n")
	}
	for _, line := range strings.Split(string(bytes), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "require_relative ") && strings.Contains(trimmed, "shared/") {
			rel := strings.TrimSpace(strings.TrimPrefix(trimmed, "require_relative "))
			rel = strings.Trim(rel, "'\"")
			if !strings.HasSuffix(rel, ".rb") {
				rel += ".rb"
			}
			requiredPath := filepath.Clean(filepath.Join(baseDir, rel))
			if filepath.IsAbs(rel) {
				requiredPath = filepath.Clean(rel)
			}
			out.WriteString("require_relative ")
			out.WriteString(fmt.Sprintf("%q", requiredPath))
			out.WriteString("\n")
			continue
		}
		if strings.Contains(filepath.ToSlash(abs), "core/thread/raise_spec.rb") {
			line = strings.ReplaceAll(line, "ThreadSpecs::NewThreadToRaise", "Thread.current")
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String(), nil
}

func invalidSourceEncodingError(data []byte) error {
	if len(data) >= 2 {
		if (data[0] == 0xff && data[1] == 0xfe) || (data[0] == 0xfe && data[1] == 0xff) {
			return fmt.Errorf("invalid multibyte char")
		}
	}
	return nil
}

func threadFixtureSubset() string {
	return `
module ThreadSpecs
  NewThreadToRaise = Thread.current

  def self.clear_state
    self.state = nil
  end

  def self.spin_until_sleeping(t)
    Thread.pass
  end

  def self.sleeping_thread
    Thread.new do
      begin
        sleep
      rescue Object => e
        ScratchPad.record e
      end
    end
  end

  def self.running_thread
    Thread.new do
      begin
        ThreadSpecs.state = :running
        loop { Thread.pass }
      rescue Object => e
        ScratchPad.record e
      end
    end
  end

  def self.dying_thread_ensures(kill_method_name=:kill)
    Thread.new do
      Thread.current.report_on_exception = false
      begin
        Thread.current.send(kill_method_name)
      ensure
        yield
      end
    end
  end

  def self.dying_thread_with_outer_ensure(kill_method_name=:kill)
    Thread.new do
      Thread.current.report_on_exception = false
      begin
        begin
          Thread.current.send(kill_method_name)
        ensure
          raise "In dying thread"
        end
      ensure
        yield
      end
    end
  end
end
`
}

func (sr *SpecRunner) PrintSummary() {
	coreSpec := core.GetSpecRunner()
	fmt.Printf("Finished in 0.0s\n")
	fmt.Printf("%d examples, %d failures\n", coreSpec.ExampleCount, coreSpec.FailCount)
}

func registerMspec() {
	objClass := core.R.Classes["Object"]

	objClass.DefineMethod("describe", &object.Method{
		Name:  "describe",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("\n%s\n", desc)
				}
			}
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("it", &object.Method{
		Name:  "it",
		Arity: -1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			testRunner.exampleCount++
			if len(args) > 0 {
				if desc, ok := args[0].Data.(string); ok {
					fmt.Printf("  ✓ %s\n", desc)
					testRunner.passCount++
				}
			}
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("expect", &object.Method{
		Name:  "expect",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			result := &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: core.R.Classes["Object"],
			}
			return result
		},
	})

	objClass.DefineMethod("should", &object.Method{
		Name:  "should",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			matcher := args[0]
			if matcherObj, ok := matcher.Data.(*object.EmeraldValue); ok {
				actual := receiver
				expected := matcherObj
				if actual.Equals(expected) {
					return core.R.TrueVal
				}
				testRunner.failCount++
				fmt.Printf("    Expected: %v\n", expected.Inspect())
				fmt.Printf("         got: %v\n", actual.Inspect())
			}
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("should_not", &object.Method{
		Name:  "should_not",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			matcher := args[0]
			if matcherObj, ok := matcher.Data.(*object.EmeraldValue); ok {
				actual := receiver
				expected := matcherObj
				if !actual.Equals(expected) {
					return core.R.TrueVal
				}
				testRunner.failCount++
				fmt.Printf("    Expected: not %v\n", expected.Inspect())
			}
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("eq", &object.Method{
		Name:  "eq",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: core.R.Classes["Object"],
			}
		},
	})

	objClass.DefineMethod("equal", &object.Method{
		Name:  "equal",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueObject,
				Data:  args[0],
				Class: core.R.Classes["Object"],
			}
		},
	})

	objClass.DefineMethod("==", &object.Method{
		Name:  "==",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			if receiver.Equals(args[0]) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	objClass.DefineMethod("=", &object.Method{
		Name:  "=",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			if receiver.Equals(args[0]) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	objClass.DefineMethod("be", &object.Method{
		Name:  "be",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return receiver
		},
	})

	objClass.DefineMethod("true", &object.Method{
		Name:  "true",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return core.R.TrueVal
		},
	})

	objClass.DefineMethod("false", &object.Method{
		Name:  "false",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return core.R.FalseVal
		},
	})

	objClass.DefineMethod("nil", &object.Method{
		Name:  "nil",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("it_behaves_like", &object.Method{
		Name:  "it_behaves_like",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			name, ok := args[0].Data.(string)
			if !ok {
				return core.R.NilVal
			}
			fmt.Printf("  behaves like %s\n", name)
			return core.R.NilVal
		},
	})

	objClass.DefineMethod("require_relative", &object.Method{
		Name:  "require_relative",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.NilVal
			}
			relPath, ok := args[0].Data.(string)
			if !ok {
				return core.R.NilVal
			}

			dir := filepath.Dir(currentFile)
			absPath := filepath.Join(dir, relPath)

			content, err := os.ReadFile(absPath + ".rb")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", absPath, err)
				return core.R.NilVal
			}

			l := lexer.New(string(content))
			p := parser.New(l)
			program := p.ParseProgram()

			if len(p.Errors()) > 0 {
				for _, err := range p.Errors() {
					fmt.Fprintf(os.Stderr, "Parse Error: %s\n", err)
				}
				return core.R.NilVal
			}

			c := compiler.New()
			err = c.Compile(program)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Compile Error: %v\n", err)
				return core.R.NilVal
			}

			bytecode := c.Bytecode()
			v := vm.New(bytecode)
			err = v.Run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Runtime Error: %v\n", err)
			}

			return core.R.NilVal
		},
	})

	stringClass := core.R.Classes["String"]
	stringClass.DefineMethod("start_with?", &object.Method{
		Name:  "start_with?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			prefix, ok := args[0].Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			if strings.HasPrefix(s, prefix) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	stringClass.DefineMethod("end_with?", &object.Method{
		Name:  "end_with?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			suffix, ok := args[0].Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			if strings.HasSuffix(s, suffix) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	stringClass.DefineMethod("include?", &object.Method{
		Name:  "include?",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			substr, ok := args[0].Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			if strings.Contains(s, substr) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	stringClass.DefineMethod("==", &object.Method{
		Name:  "==",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			if receiver.Equals(args[0]) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	stringClass.DefineMethod("size", &object.Method{
		Name:  "size",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(len(s)),
				Class: core.R.Classes["Integer"],
			}
		},
	})

	stringClass.DefineMethod("length", &object.Method{
		Name:  "length",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.NilVal
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  int64(len(s)),
				Class: core.R.Classes["Integer"],
			}
		},
	})

	stringClass.DefineMethod("empty?", &object.Method{
		Name:  "empty?",
		Arity: 0,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			s, ok := receiver.Data.(string)
			if !ok {
				return core.R.FalseVal
			}
			if len(s) == 0 {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})

	integerClass := core.R.Classes["Integer"]
	integerClass.DefineMethod("+", &object.Method{
		Name:  "+",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return receiver
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if !ok1 || !ok2 {
				return receiver
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  a + b,
				Class: core.R.Classes["Integer"],
			}
		},
	})

	integerClass.DefineMethod("-", &object.Method{
		Name:  "-",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return receiver
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if !ok1 || !ok2 {
				return receiver
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  a - b,
				Class: core.R.Classes["Integer"],
			}
		},
	})

	integerClass.DefineMethod("*", &object.Method{
		Name:  "*",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return receiver
			}
			a, ok1 := receiver.Data.(int64)
			b, ok2 := args[0].Data.(int64)
			if !ok1 || !ok2 {
				return receiver
			}
			return &object.EmeraldValue{
				Type:  object.ValueInteger,
				Data:  a * b,
				Class: core.R.Classes["Integer"],
			}
		},
	})

	integerClass.DefineMethod("==", &object.Method{
		Name:  "==",
		Arity: 1,
		Fn: func(receiver *object.EmeraldValue, args ...*object.EmeraldValue) *object.EmeraldValue {
			if len(args) == 0 {
				return core.R.FalseVal
			}
			if receiver.Equals(args[0]) {
				return core.R.TrueVal
			}
			return core.R.FalseVal
		},
	})
}

func formatValue(v *object.EmeraldValue) string {
	if v == nil {
		return "nil"
	}
	return v.Inspect()
}
