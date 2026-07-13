package process

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	DefaultMaxFrameBytes  = 1 << 20
	DefaultMaxStdoutBytes = 16 << 20
	DefaultMaxStderrBytes = 1 << 20
)

const (
	defaultTerminationGrace = 2 * time.Second
	defaultKillWait         = 2 * time.Second
)

// Protocol selects the framing applied to stdin and stdout.
type Protocol string

const (
	ProtocolJSONL          Protocol = "jsonl"
	ProtocolJSONRPCNDJSON  Protocol = "jsonrpc_ndjson"
	ProtocolCodexAppServer Protocol = "codex_app_server_ndjson"
)

// Config describes one explicitly selected Harness child process.
// Environment is complete rather than additive: the parent environment is
// never inherited. Every supplied key must also occur in AllowedEnvironment.
type Config struct {
	Executable                string
	ExpectedExecutableDigest  string
	Arguments                 []string
	WorkingDirectory          string
	AllowedWorkingDirectories []string
	Environment               map[string]string
	AllowedEnvironment        []string
	Protocol                  Protocol
	MaxFrameBytes             int
	MaxStdoutBytes            int64
	MaxStderrBytes            int64
	TerminationGrace          time.Duration
	KillWait                  time.Duration
}

type normalizedConfig struct {
	executable       string
	executableDigest string
	arguments        []string
	directory        string
	environment      []string
	protocol         Protocol
	maxFrameBytes    int
	maxStdoutBytes   int64
	maxStderrBytes   int64
	terminationGrace time.Duration
	killWait         time.Duration
}

func normalizeConfig(config Config) (normalizedConfig, error) {
	if !filepath.IsAbs(config.Executable) {
		return normalizedConfig{}, fmt.Errorf("%w: %w", ErrInvalidConfig, ErrExecutableNotAbsolute)
	}
	executable, err := filepath.EvalSymlinks(filepath.Clean(config.Executable))
	if err != nil {
		return normalizedConfig{}, fmt.Errorf("%w: resolve executable: %v", ErrExecutableNotRunnable, err)
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return normalizedConfig{}, fmt.Errorf("%w: %q", ErrExecutableNotRunnable, executable)
	}
	executableDigest, err := digestExecutable(executable)
	if err != nil {
		return normalizedConfig{}, fmt.Errorf("%w: hash executable: %v", ErrExecutableNotRunnable, err)
	}
	if config.ExpectedExecutableDigest != "" {
		if !validSHA256Digest(config.ExpectedExecutableDigest) {
			return normalizedConfig{}, fmt.Errorf("%w: expected executable digest must be canonical sha256", ErrInvalidConfig)
		}
		if subtle.ConstantTimeCompare([]byte(config.ExpectedExecutableDigest), []byte(executableDigest)) != 1 {
			return normalizedConfig{}, fmt.Errorf("%w: expected %s, actual %s", ErrExecutableDigestMismatch, config.ExpectedExecutableDigest, executableDigest)
		}
	}
	for index, argument := range config.Arguments {
		if strings.IndexByte(argument, 0) >= 0 {
			return normalizedConfig{}, fmt.Errorf("%w: argument %d contains NUL", ErrInvalidConfig, index)
		}
	}

	directory, err := allowedDirectory(config.WorkingDirectory, config.AllowedWorkingDirectories)
	if err != nil {
		return normalizedConfig{}, err
	}
	environment, err := allowedEnvironment(config.Environment, config.AllowedEnvironment)
	if err != nil {
		return normalizedConfig{}, err
	}
	if config.Protocol != ProtocolJSONL && config.Protocol != ProtocolJSONRPCNDJSON && config.Protocol != ProtocolCodexAppServer {
		return normalizedConfig{}, fmt.Errorf("%w: unsupported protocol %q", ErrInvalidConfig, config.Protocol)
	}

	maxFrameBytes := config.MaxFrameBytes
	if maxFrameBytes == 0 {
		maxFrameBytes = DefaultMaxFrameBytes
	}
	maxStdoutBytes := config.MaxStdoutBytes
	if maxStdoutBytes == 0 {
		maxStdoutBytes = DefaultMaxStdoutBytes
	}
	maxStderrBytes := config.MaxStderrBytes
	if maxStderrBytes == 0 {
		maxStderrBytes = DefaultMaxStderrBytes
	}
	terminationGrace := config.TerminationGrace
	if terminationGrace == 0 {
		terminationGrace = defaultTerminationGrace
	}
	killWait := config.KillWait
	if killWait == 0 {
		killWait = defaultKillWait
	}
	if maxFrameBytes < 1 || maxStdoutBytes < 1 || maxStderrBytes < 1 || terminationGrace < 0 || killWait < 0 {
		return normalizedConfig{}, fmt.Errorf("%w: limits and termination windows must be positive", ErrInvalidConfig)
	}
	if int64(maxFrameBytes) > maxStdoutBytes {
		return normalizedConfig{}, fmt.Errorf("%w: frame limit exceeds stdout limit", ErrInvalidConfig)
	}

	return normalizedConfig{
		executable: executable, executableDigest: executableDigest, arguments: append([]string(nil), config.Arguments...), directory: directory,
		environment: environment, protocol: config.Protocol, maxFrameBytes: maxFrameBytes,
		maxStdoutBytes: maxStdoutBytes, maxStderrBytes: maxStderrBytes,
		terminationGrace: terminationGrace, killWait: killWait,
	}, nil
}

func digestExecutable(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

func validSHA256Digest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func allowedDirectory(directory string, roots []string) (string, error) {
	if !filepath.IsAbs(directory) || len(roots) == 0 {
		return "", fmt.Errorf("%w: an absolute cwd and at least one allowed root are required", ErrWorkingDirectoryNotAllowed)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(directory))
	if err != nil {
		return "", fmt.Errorf("%w: resolve cwd: %v", ErrWorkingDirectoryNotAllowed, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: cwd is not a directory", ErrWorkingDirectoryNotAllowed)
	}
	for _, root := range roots {
		if !filepath.IsAbs(root) {
			continue
		}
		allowed, resolveErr := filepath.EvalSymlinks(filepath.Clean(root))
		if resolveErr != nil {
			continue
		}
		relative, relativeErr := filepath.Rel(allowed, resolved)
		if relativeErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrWorkingDirectoryNotAllowed, resolved)
}

func allowedEnvironment(values map[string]string, allowlist []string) ([]string, error) {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		if !validEnvironmentName(name) {
			return nil, fmt.Errorf("%w: invalid allowlist key %q", ErrInvalidConfig, name)
		}
		allowed[name] = struct{}{}
	}
	names := make([]string, 0, len(values))
	for name, value := range values {
		if !validEnvironmentName(name) || strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("%w: invalid environment entry %q", ErrInvalidConfig, name)
		}
		if _, ok := allowed[name]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrEnvironmentNotAllowed, name)
		}
		if sensitiveEnvironmentName(name) {
			return nil, fmt.Errorf("%w: %s", ErrSensitiveEnvironment, name)
		}
		if unsafeEnvironmentName(name) {
			return nil, fmt.Errorf("%w: %s", ErrUnsafeEnvironment, name)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, name+"="+values[name])
	}
	return result, nil
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for index, character := range name {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func sensitiveEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range []string{
		"API_KEY", "APIKEY", "ACCESS_KEY", "PRIVATE_KEY", "CLIENT_SECRET", "PASSWORD", "PASSWD",
		"TOKEN", "SECRET", "CREDENTIAL", "AUTH", "COOKIE",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	if upper == "KEY" || strings.HasSuffix(upper, "_KEY") {
		return true
	}
	return false
}

func unsafeEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "DYLD_") {
		return true
	}
	_, denied := map[string]struct{}{
		"LD_PRELOAD": {}, "LD_LIBRARY_PATH": {}, "BASH_ENV": {}, "ENV": {}, "SHELLOPTS": {},
		"NODE_OPTIONS": {}, "PYTHONPATH": {}, "PYTHONHOME": {}, "PYTHONSTARTUP": {}, "RUBYOPT": {}, "PERL5OPT": {},
	}[upper]
	return denied
}
