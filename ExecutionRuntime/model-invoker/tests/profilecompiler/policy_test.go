package profilecompiler_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/profile"
)

func TestScopedPolicyMergeUsesConstraintAlgebra(t *testing.T) {
	_, profiles := representativeRegistry(t)
	base := profileByID(t, profiles, profile.ProfileOpenAIDirect).DefaultPolicy
	base.Verification = profile.VerificationObserved

	layer := profile.PolicyLayer{
		ID: "workspace.policy", Scope: profile.PolicyScopeWorkspace,
		Identity: base.Identity,
		AllowedMechanismIDs: profile.StringSetConstraint{
			Specified: true, Values: []string{"openai.caller.apply_patch", "openai.caller.process", "openai.responses.json_schema"},
		},
		DeniedMechanismIDs: []string{"openai.caller.write_file"},
		MaxWallTime:        60 * time.Second, MaxActions: 4, Verification: profile.VerificationStrong,
		Filesystem: &profile.FilesystemPolicyLayer{
			WritablePaths: profile.PathSetConstraint{
				Specified: true, Values: []string{"/workspace/internal/config/config.go"},
			},
			DeniedPaths: []string{"/workspace/internal/config/private"},
			MaxFileSize: 64 * 1024, RequireBackup: true,
		},
		Process: &profile.ProcessPolicyLayer{
			AllowedArgv: profile.ArgvSetConstraint{
				Specified: true, Values: [][]string{{"go", "test", "./internal/config"}},
			},
			NetworkAccess: profile.NetworkDenied, MaxTimeout: 60 * time.Second,
		},
		Network: &profile.NetworkPolicyLayer{Mode: profile.NetworkUnrestricted},
		Secret: &profile.SecretPolicyLayer{
			DeniedEnvironmentNames: []string{"EXTRA_SECRET"}, RequireRedaction: true,
		},
	}
	merged, err := profile.MergeRuntimePolicy(base, layer)
	if err != nil {
		t.Fatalf("MergeRuntimePolicy() error = %v", err)
	}
	if got := merged.AllowedMechanismIDs.Values; len(got) != 3 {
		t.Fatalf("allowed mechanism intersection = %#v", got)
	}
	if merged.MaxWallTime != 60*time.Second || merged.MaxActions != 4 ||
		merged.Verification != profile.VerificationStrong {
		t.Fatalf("stricter limits not preserved: %#v", merged)
	}
	if len(merged.Filesystem.WritablePaths.Values) != 1 ||
		merged.Filesystem.MaxFileSize != 64*1024 || !merged.Filesystem.RequireBackup {
		t.Fatalf("filesystem merge = %#v", merged.Filesystem)
	}
	if merged.Network.Mode != profile.NetworkDenied {
		t.Fatalf("network relaxation succeeded: %q", merged.Network.Mode)
	}
	if len(merged.Secret.DeniedEnvironmentNames) != 2 {
		t.Fatalf("secret deny union = %#v", merged.Secret.DeniedEnvironmentNames)
	}
}

func TestScopedPathConstraintKeepsNarrowerChildRoot(t *testing.T) {
	_, profiles := representativeRegistry(t)
	base := profileByID(t, profiles, profile.ProfileOpenAIDirect).DefaultPolicy
	base.Filesystem.WritablePaths = profile.PathSetConstraint{
		Specified: true,
		Values:    []string{"/workspace", "/workspace/generated"},
	}
	base.Process.AllowedCWDs = profile.PathSetConstraint{Specified: true, Values: []string{"/workspace"}}

	merged, err := profile.MergeRuntimePolicy(base, profile.PolicyLayer{
		ID: "task.path-narrowing", Scope: profile.PolicyScopeTask, Identity: base.Identity,
		Filesystem: &profile.FilesystemPolicyLayer{WritablePaths: profile.PathSetConstraint{
			Specified: true, Values: []string{"/workspace/internal/config"},
		}},
		Process: &profile.ProcessPolicyLayer{AllowedCWDs: profile.PathSetConstraint{
			Specified: true, Values: []string{"/workspace/internal"},
		}},
	})
	if err != nil {
		t.Fatalf("MergeRuntimePolicy() error = %v", err)
	}
	if got := merged.Filesystem.WritablePaths.Values; len(got) != 1 || got[0] != "/workspace/internal/config" {
		t.Fatalf("writable path intersection = %#v", got)
	}
	if got := merged.Process.AllowedCWDs.Values; len(got) != 1 || got[0] != "/workspace/internal" {
		t.Fatalf("cwd intersection = %#v", got)
	}
}

func TestScopedPolicyCannotRelaxPermissionOrIdentity(t *testing.T) {
	_, profiles := representativeRegistry(t)
	base := profileByID(t, profiles, profile.ProfileOpenAIDirect).DefaultPolicy

	_, err := profile.MergeRuntimePolicy(base, profile.PolicyLayer{
		ID: "task.relax", Scope: profile.PolicyScopeTask, Identity: base.Identity,
		Filesystem: &profile.FilesystemPolicyLayer{FollowSymlinks: profile.PermissionAllow},
	})
	if err == nil {
		t.Fatal("FollowSymlinks relaxation error = nil")
	}

	forged := base.Identity
	forged.Provider = "anthropic"
	if _, err := profile.MergeRuntimePolicy(base, profile.PolicyLayer{
		ID: "task.identity", Scope: profile.PolicyScopeTask, Identity: forged,
	}); err == nil {
		t.Fatal("Provider identity override error = nil")
	}
}

func TestCompilerRejectsRequestThatExceedsTypedPolicyBudget(t *testing.T) {
	compiler, _ := compilerForTest(t)
	input := paperCompileInput(profile.ProfileOpenAIDirect)
	input.Request.Budget.MaxWallTime = 121 * time.Second
	_, err := compiler.Compile(input)
	assertProfileErrorCode(t, err, profile.ErrorPolicyRejected)
}

func TestScopeOrderIsDeterministicAndTaskPreferenceIsMostSpecific(t *testing.T) {
	_, profiles := representativeRegistry(t)
	base := profileByID(t, profiles, profile.ProfileOpenAIDirect).DefaultPolicy
	organization := profile.PolicyLayer{
		ID: "organization.policy", Scope: profile.PolicyScopeOrganization, Identity: base.Identity,
		MaxActions: 6, MechanismPreferenceWeight: map[string]int{"openai.caller.apply_patch": 1},
	}
	task := profile.PolicyLayer{
		ID: "task.policy", Scope: profile.PolicyScopeTask, Identity: base.Identity,
		MaxActions: 3, MechanismPreferenceWeight: map[string]int{"openai.caller.apply_patch": 9},
	}
	forward, err := profile.MergeRuntimePolicy(base, organization, task)
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := profile.MergeRuntimePolicy(base, task, organization)
	if err != nil {
		t.Fatal(err)
	}
	forwardDigest, err := forward.Digest()
	if err != nil {
		t.Fatal(err)
	}
	reverseDigest, err := reverse.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if forwardDigest != reverseDigest || forward.MaxActions != 3 ||
		forward.MechanismPreferenceWeight["openai.caller.apply_patch"] != 9 {
		t.Fatalf("scope merge drifted: forward=%#v reverse=%#v", forward, reverse)
	}
}
