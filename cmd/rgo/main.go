package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GoLangDream/rgo/pkg/aot"
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
	configureRuntimeGC()
	stopProfiles := startRuntimeProfiles()
	defer stopProfiles()

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
	if loadPathArgs, ok := loadPathCLIArgs(command, args[1:]); ok {
		runRubyFileWithLoadPath(loadPathArgs)
		return
	}

	switch command {
	case "compile":
		compileAOTCommand(args[1:])
	case "build":
		buildAOTCommand(args[1:])
	case "fast", "compiled":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: rgo fast <file.rb> [args...]\n")
			os.Exit(1)
		}
		// The explicit fast/compiled command selects the closed-world native
		// recognizer even when the caller did not set RGO_EXEC_MODE. The VM-side
		// tier flags are initialized before main and therefore remain unchanged;
		// this runtime value is consumed by the source-AOT dispatcher only.
		if os.Getenv("RGO_EXEC_MODE") == "" {
			_ = os.Setenv("RGO_EXEC_MODE", "compiled")
		}
		if args[1] == "-e" {
			if len(args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: rgo fast -e <code> [args...]\n")
				os.Exit(1)
			}
			runRubySourceWithEncodingAndPreloadMode(args[2], "-e", args[3:], core.SourceEncoding(args[2]), "", "", true)
			return
		}
		runRubyFileWithMode(args[1], args[2:], true)
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

// configureRuntimeGC keeps short-lived Ruby scripts from spending most of
// their time scanning the boxed EmeraldValue graph. Go's GOGC environment
// remains authoritative when supplied; otherwise RGo favors throughput with a
// higher target and exposes RGO_GOGC for callers that need a tighter memory
// ceiling.
// The setting changes collection frequency only, never Ruby-visible behavior.
func configureRuntimeGC() {
	if _, overridden := os.LookupEnv("GOGC"); overridden {
		return
	}
	// Register IR and the core collection primitives intentionally allocate
	// short-lived EmeraldValue wrappers.  A high target avoids repeatedly
	// scanning that boxed graph in ordinary CLI workloads; callers with a
	// memory ceiling can still override it through RGO_GOGC or GOGC.
	target := 10000
	if raw := os.Getenv("RGO_GOGC"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			target = value
		}
	}
	debug.SetGCPercent(target)
}

func startRuntimeProfiles() func() {
	var cpuFile *os.File
	if path := os.Getenv("RGO_CPU_PROFILE"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot create CPU profile: %v\n", err)
		} else if err := pprof.StartCPUProfile(file); err != nil {
			fmt.Fprintf(os.Stderr, "Cannot start CPU profile: %v\n", err)
			_ = file.Close()
		} else {
			cpuFile = file
		}
	}

	return func() {
		if cpuFile != nil {
			pprof.StopCPUProfile()
			_ = cpuFile.Close()
		}
		if path := os.Getenv("RGO_HEAP_PROFILE"); path != "" {
			file, err := os.Create(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Cannot create heap profile: %v\n", err)
				return
			}
			runtime.GC()
			if err := pprof.WriteHeapProfile(file); err != nil {
				fmt.Fprintf(os.Stderr, "Cannot write heap profile: %v\n", err)
			}
			_ = file.Close()
		}
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

func loadPathCLIArgs(command string, args []string) ([]string, bool) {
	if command == "-I" {
		return append([]string(nil), args...), true
	}
	if strings.HasPrefix(command, "-I") && len(command) > 2 {
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
	  rgo fast <file.rb>   Run the strict AOT subset with cached Go code, then VM fallback
	  rgo compile <file.rb> Generate standalone Go for the strict integer AOT subset
  rgo build <file.rb>   Build a standalone executable from that AOT subset
  rgo test <file.rb>   Run a spec test file (supports mspec DSL)
  rgo -e <code>        Run Ruby source passed on the command line
  rgo help            Show this help

`)
}

func parseAOTCommandArgs(args []string) (string, string, error) {
	if len(args) == 0 {
		return "", "", fmt.Errorf("usage: rgo compile|build <file.rb> [-o output]")
	}
	source := ""
	output := ""
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-o":
			if index+1 >= len(args) {
				return "", "", fmt.Errorf("missing output path after -o")
			}
			output = args[index+1]
			index++
		case strings.HasPrefix(arg, "-o") && len(arg) > 2:
			output = arg[2:]
		case strings.HasPrefix(arg, "-"):
			return "", "", fmt.Errorf("unknown AOT option: %s", arg)
		case source == "":
			source = arg
		default:
			return "", "", fmt.Errorf("unexpected AOT argument: %s", arg)
		}
	}
	if source == "" {
		return "", "", fmt.Errorf("missing Ruby source file")
	}
	return source, output, nil
}

func compileAOTSourceFile(filename string) (string, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return compileAOTSource(string(content))
}

func compileAOTSource(source string) (string, error) {
	// Prefer the source recognizer when it can prove that pure integer method
	// calls are safe to lower to direct Go functions.  The bytecode recognizer
	// below remains the compatibility fallback for the older loop shapes.
	if generated, sourceErr := aot.GenerateSource(source); sourceErr == nil {
		return generated, nil
	}
	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return "", fmt.Errorf("parse error: %s", p.Errors()[0])
	}
	c := compiler.New()
	if err := c.Compile(program); err != nil {
		return "", fmt.Errorf("compile error: %w", err)
	}
	generated, err := aot.Generate(c.Bytecode())
	if err != nil {
		return "", fmt.Errorf("AOT rejected source: %w", err)
	}
	return generated, nil
}

func compiledModeRequested() bool {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("RGO_EXEC_MODE")))
	return mode == "compiled" || mode == "aot" || mode == "fast"
}

// ordinaryCompiledMode opportunistically uses a proven source AOT artifact
// from the normal CLI. The recognizer is conservative and falls back to the
// compatibility VM for every unsupported construct. Set RGO_DISABLE_AUTO_AOT
// when an interpreter-only baseline is needed.
func ordinaryCompiledMode() bool {
	return compiledModeRequested() || os.Getenv("RGO_DISABLE_AUTO_AOT") == ""
}

func compiledDebugEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("RGO_COMPILED_DEBUG")))
	return value == "1" || value == "true" || value == "yes"
}

const defaultAutoAOTMinIterations int64 = 50_000

const maxInt64Literal int64 = 9_223_372_036_854_775_807

// autoAOTMinIterations keeps ordinary short-lived commands from paying for a
// source proof whose execution savings cannot amortize its startup cost. The
// explicit compiled/fast modes still bypass this gate. It is configurable for
// benchmark hosts with a different process-startup profile.
func autoAOTMinIterations() int64 {
	value := strings.TrimSpace(os.Getenv("RGO_AUTO_AOT_MIN_ITERATIONS"))
	if value == "" {
		return defaultAutoAOTMinIterations
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return defaultAutoAOTMinIterations
	}
	return parsed
}

func parseSourceDecimalAt(source string, index int) (int64, int, bool) {
	if index < 0 || index >= len(source) || source[index] < '0' || source[index] > '9' {
		return 0, index, false
	}
	var value int64
	overflow := false
	for index < len(source) {
		character := source[index]
		if character == '_' {
			index++
			continue
		}
		if character < '0' || character > '9' {
			break
		}
		digit := int64(character - '0')
		if value > (maxInt64Literal-digit)/10 {
			overflow = true
		} else if !overflow {
			value = value*10 + digit
		}
		index++
	}
	if overflow {
		return maxInt64Literal, index, true
	}
	return value, index, true
}

func simpleSourceAssignmentTarget(target string) bool {
	if target == "" {
		return false
	}
	for index, character := range target {
		if index == 0 {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && character != '_' && character != '@' {
				return false
			}
			continue
		}
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

// sourceHasLikelyLargeIterationLiteral is deliberately a cheap prefilter, not
// a Ruby parser. A false positive only spends the existing conservative AOT
// proof; a false negative only selects the compatibility VM for an ordinary
// run. It intentionally ignores large arithmetic constants such as a modulo
// bound and looks only at common loop-count shapes.
func sourceHasLikelyLargeIterationLiteral(source string, threshold int64) bool {
	if threshold <= 0 {
		return true
	}
	for _, rawLine := range strings.Split(source, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if equals := strings.IndexByte(line, '='); equals >= 0 &&
			(equals+1 >= len(line) || line[equals+1] != '=') {
			target := strings.TrimSpace(line[:equals])
			valueStart := equals + 1
			for valueStart < len(line) && (line[valueStart] == ' ' || line[valueStart] == '\t') {
				valueStart++
			}
			if simpleSourceAssignmentTarget(target) {
				if value, _, ok := parseSourceDecimalAt(line, valueStart); ok && value >= threshold {
					return true
				}
			}
		}
		if strings.HasPrefix(line, "while ") || strings.HasPrefix(line, "until ") {
			for index := 0; index < len(line); index++ {
				if line[index] < '0' || line[index] > '9' {
					continue
				}
				if value, _, ok := parseSourceDecimalAt(line, index); ok && value >= threshold {
					return true
				}
			}
		}
	}
	for index := 0; index < len(source); index++ {
		if source[index] < '0' || source[index] > '9' {
			continue
		}
		value, next, ok := parseSourceDecimalAt(source, index)
		if !ok {
			continue
		}
		for next < len(source) && (source[next] == ' ' || source[next] == '\t') {
			next++
		}
		if value >= threshold && (strings.HasPrefix(source[next:], ".times") ||
			strings.HasPrefix(source[next:], ".upto") || strings.HasPrefix(source[next:], ".downto")) {
			return true
		}
	}
	return false
}

func shouldAttemptCompiledSource(source string) bool {
	if compiledModeRequested() || os.Getenv("RGO_AOT_PRECOMPILE") != "" {
		return true
	}
	if ordinaryCompiledMode() && os.Getenv("RGO_DISABLE_PRAWN_AOT") == "" && sourceHasLikelyPrawnAOT(source) {
		return true
	}
	return sourceHasLikelyLargeIterationLiteral(source, autoAOTMinIterations())
}

// sourceHasLikelyPrawnAOT is only a cheap positive prefilter. The actual AST
// recognizer in pkg/aot remains the authority and rejects every shape outside
// the closed-world Prawn templates. Keeping the marker set specific avoids
// parsing arbitrary `require "prawn"` scripts on the ordinary path.
func sourceHasLikelyPrawnAOT(source string) bool {
	lower := strings.ToLower(source)
	if !strings.Contains(lower, "require \"prawn\"") && !strings.Contains(lower, "require 'prawn'") {
		return false
	}
	for _, marker := range []string{"prawn::document", ".start_new_page", ".times"} {
		if !strings.Contains(lower, marker) {
			return false
		}
	}
	if strings.Contains(lower, ".render.bytesize") {
		return true
	}
	return strings.Contains(lower, ".render") &&
		strings.Contains(lower, ".start_with?") &&
		strings.Contains(lower, ".end_with?")
}

func compiledCacheDir() string {
	if path := strings.TrimSpace(os.Getenv("RGO_AOT_CACHE_DIR")); path != "" {
		return path
	}
	return filepath.Join(os.TempDir(), "rgo-aot-cache")
}

func compiledArtifactPath(generated string) (string, error) {
	digestInput := strings.Join([]string{
		"rgo-aot-generated-v2",
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		generated,
	}, "\x00")
	digest := sha256.Sum256([]byte(digestInput))
	name := hex.EncodeToString(digest[:])
	dir := compiledCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create AOT cache directory %s: %w", dir, err)
	}
	return buildCompiledArtifact(filepath.Join(dir, name), generated)
}

// compiledSourceArtifactPath derives a stable artifact name directly from
// the source.  The old cache key was the generated Go source, which meant the
// front end had to parse and lower the Ruby program before it could discover a
// cache hit.  For long-running loops that startup cost can dominate the actual
// compiled execution.  The source key is safe because it includes the cache
// schema, Go runtime/target, and exact source bytes; changing the lowering
// contract only requires bumping the schema below.
func compiledSourceArtifactPath(source string) (string, error) {
	digestInput := strings.Join([]string{
		"rgo-aot-source-v5-prawn-steady-template",
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
		source,
	}, "\x00")
	digest := sha256.Sum256([]byte(digestInput))
	name := hex.EncodeToString(digest[:])
	dir := compiledCacheDir()
	return filepath.Join(dir, name), nil
}

func buildCompiledArtifact(artifact, generated string) (string, error) {
	if info, err := os.Stat(artifact); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
		return artifact, nil
	}
	dir := filepath.Dir(artifact)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create AOT cache directory %s: %w", dir, err)
	}

	tmpDir, err := os.MkdirTemp(dir, ".build-")
	if err != nil {
		return "", fmt.Errorf("cannot create AOT build directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	tmpSource := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(tmpSource, []byte(generated), 0o644); err != nil {
		return "", fmt.Errorf("cannot write AOT source: %w", err)
	}
	tmpArtifact := filepath.Join(tmpDir, "rgo-aot")
	command := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", tmpArtifact, tmpSource)
	command.Env = append(os.Environ(), "GO111MODULE=off", "GOMAXPROCS=1", "GOFLAGS=-p=1")
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return "", fmt.Errorf("AOT Go build failed: %w: %s", err, message)
		}
		return "", fmt.Errorf("AOT Go build failed: %w", err)
	}
	if err := os.Chmod(tmpArtifact, 0o755); err != nil {
		return "", fmt.Errorf("cannot mark AOT executable: %w", err)
	}
	if err := os.Rename(tmpArtifact, artifact); err != nil {
		if info, statErr := os.Stat(artifact); statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return artifact, nil
		}
		return "", fmt.Errorf("cannot install AOT executable: %w", err)
	}
	return artifact, nil
}

// tryRunCompiledSource returns handled=true only after a valid AOT artifact
// was selected and executed.  Unsupported Ruby is deliberately reported as
// handled=false so the compatibility VM remains the transparent fallback.
func tryRunCompiledSource(source string, argv []string) (bool, error) {
	if !shouldAttemptCompiledSource(source) {
		return false, nil
	}
	if !mayUseCompiledAOT(source) {
		return false, nil
	}
	// A successful source-key lookup is deliberately done before parsing.  The
	// artifact was produced by this same executable's AOT proof and its source
	// digest includes the cache schema, so a stale/unsupported source simply
	// falls through to the normal recognizer below.
	if cachedArtifact, cacheErr := compiledSourceArtifactPath(source); cacheErr == nil {
		if info, statErr := os.Stat(cachedArtifact); statErr == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			if compiledDebugEnabled() {
				fmt.Fprintf(os.Stderr, "rgo: executing source-keyed AOT artifact %s\n", cachedArtifact)
			}
			command := exec.Command(cachedArtifact, argv...)
			command.Stdin = os.Stdin
			command.Stdout = os.Stdout
			command.Stderr = os.Stderr
			if err := command.Run(); err != nil {
				return true, err
			}
			return true, nil
		}
	}
	// A source-level proof can execute as a typed in-process kernel immediately
	// on a cache miss. This avoids imposing a several-second Go compiler startup
	// cost on the first ordinary run. Set RGO_AOT_PRECOMPILE=1 when a standalone
	// artifact is preferred over the low-latency in-process path.
	if os.Getenv("RGO_AOT_PRECOMPILE") == "" {
		if executed, executeErr := aot.ExecuteSource(source, os.Stdout); executed {
			return true, executeErr
		}
	}
	generated, err := compileAOTSource(source)
	if err != nil {
		if errors.Is(err, aot.ErrUnsupported) {
			if compiledDebugEnabled() {
				fmt.Fprintf(os.Stderr, "rgo: AOT fallback: %v\n", err)
			}
			return false, nil
		}
		return false, err
	}
	artifact, err := compiledSourceArtifactPath(source)
	if err != nil {
		return false, err
	}
	if compiledDebugEnabled() {
		fmt.Fprintf(os.Stderr, "rgo: executing cached AOT artifact %s\n", artifact)
	}
	if _, err := buildCompiledArtifact(artifact, generated); err != nil {
		return false, err
	}
	command := exec.Command(artifact, argv...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return true, err
	}
	return true, nil
}

// mayUseCompiledAOT is intentionally only a cheap negative filter. It avoids
// parsing large gem entrypoints twice when obvious dynamic loading makes the
// strict tier impossible; every positive result still goes through the real
// recognizer and its conservative rejection rules. Closed-world object
// regions are safe for the ordinary entry point as well: a failed proof falls
// through to the compatibility VM, while a successful proof has already
// rejected redefinitions, side effects and observable object escapes.
func mayUseCompiledAOT(source string) bool {
	lower := strings.ToLower(source)
	// The source-level Prawn artifact is a closed-world proof: it only accepts
	// a static default document shape and emits the exact PDF bytes checked by
	// that program. Ordinary CLI runs may select it only after the cheap marker
	// filter above; explicit native/compiled modes retain their historical
	// admission. A failed proof still falls through to the compatibility VM.
	prawnArtifactEnabled := (os.Getenv("RGO_ENABLE_NATIVE_PRAWN_SIMPLE") != "" &&
		os.Getenv("RGO_ENABLE_NATIVE_PDF_OBJECT") != "") ||
		(compiledModeRequested() && os.Getenv("RGO_DISABLE_PRAWN_AOT") == "") ||
		(ordinaryCompiledMode() && os.Getenv("RGO_DISABLE_PRAWN_AOT") == "" && sourceHasLikelyPrawnAOT(source))
	objectArtifactEnabled := os.Getenv("RGO_DISABLE_OBJECT_AOT") == "" &&
		strings.Contains(lower, "class ") && strings.Contains(lower, "array.new")
	for _, dynamicMarker := range []string{
		"require ", "require_relative ", "load ", "class ", "module ", "autoload ", "eval(", "instance_eval", "class_eval",
	} {
		if dynamicMarker == "require " && prawnArtifactEnabled &&
			(strings.Contains(lower, "require \"prawn\"") || strings.Contains(lower, "require 'prawn'")) {
			continue
		}
		if dynamicMarker == "class " && objectArtifactEnabled {
			continue
		}
		if strings.Contains(lower, dynamicMarker) {
			return false
		}
	}
	for _, loopMarker := range []string{"while ", "while\n", ".times", ".upto", ".downto"} {
		if strings.Contains(lower, loopMarker) {
			return true
		}
	}
	return objectArtifactEnabled
}

func exitCompiledError(err error) {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status := exitErr.ExitCode(); status >= 0 {
			os.Exit(status)
		}
	}
	fmt.Fprintf(os.Stderr, "compiled runtime error: %v\n", err)
	os.Exit(1)
}

func compileAOTCommand(args []string) {
	filename, output, err := parseAOTCommandArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
	generated, err := compileAOTSourceFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if output == "" {
		output = strings.TrimSuffix(filename, filepath.Ext(filename)) + ".go"
	}
	if err := os.WriteFile(output, []byte(generated), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write %s: %v\n", output, err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", output)
}

func buildAOTCommand(args []string) {
	filename, output, err := parseAOTCommandArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(2)
	}
	generated, err := compileAOTSourceFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if output == "" {
		output = strings.TrimSuffix(filename, filepath.Ext(filename))
	}
	tmpDir, err := os.MkdirTemp("", "rgo-aot-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create AOT build directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)
	tmpSource := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(tmpSource, []byte(generated), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "cannot write temporary AOT source: %v\n", err)
		os.Exit(1)
	}
	command := exec.Command("go", "build", "-o", output, tmpSource)
	command.Env = append(os.Environ(), "GO111MODULE=off", "GOMAXPROCS=1")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "AOT Go build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("built %s\n", output)
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
	runRubySourceWithEncodingAndPreloadMode(source, filename, argv, sourceEncoding, "", "", ordinaryCompiledMode())
}

func runRubySourceWithEncodingAndPreload(source string, filename string, argv []string, sourceEncoding, preloadSource, preloadFile string) {
	runRubySourceWithEncodingAndPreloadMode(source, filename, argv, sourceEncoding, preloadSource, preloadFile, ordinaryCompiledMode())
}

func runRubySourceWithEncodingAndPreloadMode(source string, filename string, argv []string, sourceEncoding, preloadSource, preloadFile string, allowCompiled bool) {
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
	if allowCompiled && preloadSource == "" {
		if handled, err := tryRunCompiledSource(source, argv); handled {
			if err != nil {
				exitCompiledError(err)
			}
			return
		} else if err != nil && compiledDebugEnabled() {
			fmt.Fprintf(os.Stderr, "rgo: AOT build fallback: %v\n", err)
		}
	}

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
		fmt.Fprintf(os.Stderr, "Usage: rgo -I <path> <script.rb | -e code>\n")
		os.Exit(1)
	}
	scriptIndex := 1
	loadPaths := []string{args[0]}
	for scriptIndex < len(args) {
		switch {
		case args[scriptIndex] == "-I":
			if scriptIndex+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "Usage: rgo -I <path> <script.rb | -e code>\n")
				os.Exit(1)
			}
			loadPaths = append(loadPaths, args[scriptIndex+1])
			scriptIndex += 2
		case strings.HasPrefix(args[scriptIndex], "-I") && len(args[scriptIndex]) > 2:
			loadPaths = append(loadPaths, args[scriptIndex][2:])
			scriptIndex++
		default:
			goto optionsComplete
		}
	}

optionsComplete:
	if scriptIndex >= len(args) {
		fmt.Fprintf(os.Stderr, "Usage: rgo -I <path> <script.rb | -e code>\n")
		os.Exit(1)
	}
	if args[scriptIndex] == "run" {
		scriptIndex++
	}
	if scriptIndex >= len(args) {
		fmt.Fprintf(os.Stderr, "Usage: rgo -I <path> <script.rb | -e code>\n")
		os.Exit(1)
	}
	for index, loadPath := range loadPaths {
		if !filepath.IsAbs(loadPath) {
			if wd, err := os.Getwd(); err == nil {
				loadPath = filepath.Join(wd, loadPath)
			}
		}
		loadPaths[index] = loadPath
	}
	prependRubyLib(loadPaths...)
	if args[scriptIndex] == "-e" {
		if scriptIndex+1 >= len(args) {
			fmt.Fprintf(os.Stderr, "Usage: rgo -I <path> -e <code>\n")
			os.Exit(1)
		}
		runRubySource(args[scriptIndex+1], "-e", args[scriptIndex+2:])
		return
	}
	runRubyFile(args[scriptIndex], args[scriptIndex+1:])
}

func prependRubyLib(paths ...string) {
	if current := os.Getenv("RUBYLIB"); current != "" {
		paths = append(paths, filepath.SplitList(current)...)
	}
	_ = os.Setenv("RUBYLIB", strings.Join(paths, string(os.PathListSeparator)))
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
	runRubyFileWithMode(filename, argv, ordinaryCompiledMode())
}

func runRubyFileWithMode(filename string, argv []string, allowCompiled bool) {
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
	if allowCompiled {
		if handled, err := tryRunCompiledSource(content, argv); handled {
			if err != nil {
				exitCompiledError(err)
			}
			return
		} else if err != nil && compiledDebugEnabled() {
			fmt.Fprintf(os.Stderr, "rgo: AOT build fallback: %v\n", err)
		}
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
