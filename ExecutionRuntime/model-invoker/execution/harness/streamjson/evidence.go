package streamjson

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	harnessprocess "github.com/Proview-China/rax/ExecutionRuntime/model-invoker/execution/harness/process"
)

// LaunchEvidence is a credential-free description of the exact process that
// the bridge is about to start. Environment values are committed into a
// digest but are never exposed.
type LaunchEvidence struct {
	RequestedExecutablePath  string                  `json:"requested_executable_path"`
	ActualExecutablePath     string                  `json:"actual_executable_path"`
	ExpectedExecutableDigest string                  `json:"expected_executable_digest"`
	ActualExecutableDigest   string                  `json:"actual_executable_digest"`
	Arguments                []string                `json:"arguments"`
	ArgumentsDigest          string                  `json:"arguments_digest"`
	EnvironmentNames         []string                `json:"environment_names,omitempty"`
	EnvironmentDigest        string                  `json:"environment_digest"`
	WorkingDirectory         string                  `json:"working_directory"`
	Protocol                 harnessprocess.Protocol `json:"protocol"`
}

// CloneProcessConfig freezes caller-owned maps and slices before an Adapter
// retains process configuration across Preflight/Open.
func CloneProcessConfig(config harnessprocess.Config) harnessprocess.Config {
	clone := config
	clone.Arguments = append([]string(nil), config.Arguments...)
	clone.AllowedWorkingDirectories = append([]string(nil), config.AllowedWorkingDirectories...)
	clone.AllowedEnvironment = append([]string(nil), config.AllowedEnvironment...)
	if config.Environment != nil {
		clone.Environment = make(map[string]string, len(config.Environment))
		for name, value := range config.Environment {
			clone.Environment[name] = value
		}
	}
	return clone
}

func (evidence LaunchEvidence) Clone() LaunchEvidence {
	clone := evidence
	clone.Arguments = append([]string(nil), evidence.Arguments...)
	clone.EnvironmentNames = append([]string(nil), evidence.EnvironmentNames...)
	return clone
}

func (evidence LaunchEvidence) Digest() (string, error) {
	return stableDigest(evidence)
}

func (evidence LaunchEvidence) Pinned() bool {
	return evidence.ExpectedExecutableDigest != "" && evidence.ExpectedExecutableDigest == evidence.ActualExecutableDigest
}

// ProbeLaunch resolves and hashes the configured executable, cwd, argv, and
// complete environment before process.Start repeats the security validation at
// the actual spawn boundary.
func ProbeLaunch(config harnessprocess.Config) (LaunchEvidence, error) {
	if config.Protocol != harnessprocess.ProtocolJSONL {
		return LaunchEvidence{}, fmt.Errorf("%w: process protocol must be jsonl", ErrInvalidConfig)
	}
	if !filepath.IsAbs(config.Executable) {
		return LaunchEvidence{}, fmt.Errorf("%w: executable path must be absolute", ErrInvalidConfig)
	}
	actualExecutable, err := filepath.EvalSymlinks(filepath.Clean(config.Executable))
	if err != nil {
		return LaunchEvidence{}, fmt.Errorf("%w: resolve executable: %v", ErrInvalidConfig, err)
	}
	executableDigest, err := digestFile(actualExecutable)
	if err != nil {
		return LaunchEvidence{}, fmt.Errorf("%w: digest executable: %v", ErrInvalidConfig, err)
	}
	actualDirectory, err := filepath.EvalSymlinks(filepath.Clean(config.WorkingDirectory))
	if err != nil {
		return LaunchEvidence{}, fmt.Errorf("%w: resolve cwd: %v", ErrInvalidConfig, err)
	}

	environmentNames := make([]string, 0, len(config.Environment))
	environmentEntries := make([]string, 0, len(config.Environment))
	for name, value := range config.Environment {
		environmentNames = append(environmentNames, name)
		environmentEntries = append(environmentEntries, name+"="+value)
	}
	sort.Strings(environmentNames)
	sort.Strings(environmentEntries)
	arguments := append([]string(nil), config.Arguments...)
	argumentsDigest, err := stableDigest(arguments)
	if err != nil {
		return LaunchEvidence{}, err
	}
	environmentDigest, err := stableDigest(environmentEntries)
	if err != nil {
		return LaunchEvidence{}, err
	}
	evidence := LaunchEvidence{
		RequestedExecutablePath:  config.Executable,
		ActualExecutablePath:     actualExecutable,
		ExpectedExecutableDigest: strings.TrimSpace(config.ExpectedExecutableDigest),
		ActualExecutableDigest:   executableDigest,
		Arguments:                arguments,
		ArgumentsDigest:          argumentsDigest,
		EnvironmentNames:         environmentNames,
		EnvironmentDigest:        environmentDigest,
		WorkingDirectory:         actualDirectory,
		Protocol:                 config.Protocol,
	}
	return evidence, nil
}

func digestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func stableDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("digest stream-json evidence: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
