package upstream

import "strings"

type CredentialSecretIssue string

const (
	CredentialSecretInvalid        CredentialSecretIssue = "invalid_secret"
	CredentialSecretPrefixMismatch CredentialSecretIssue = "key_prefix_mismatch"
)

// CredentialSecretError deliberately contains no supplied credential value.
type CredentialSecretError struct {
	Issue CredentialSecretIssue
}

func (err *CredentialSecretError) Error() string {
	if err != nil && err.Issue == CredentialSecretPrefixMismatch {
		return "resolved credential does not match the required key prefix"
	}
	return "resolved credential has an invalid shape"
}

// ValidateResolvedSecret checks only non-secret shape metadata. The caller
// must not retain value after installing it into the provider transport.
func (profile CredentialProfile) ValidateResolvedSecret(value string) error {
	if value == "" || len(value) > 16*1024 || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
		return &CredentialSecretError{Issue: CredentialSecretInvalid}
	}
	for _, prefix := range profile.DeniedKeyPrefixes {
		if strings.HasPrefix(value, prefix) {
			return &CredentialSecretError{Issue: CredentialSecretPrefixMismatch}
		}
	}
	if len(profile.KeyPrefixes) == 0 {
		return nil
	}
	for _, prefix := range profile.KeyPrefixes {
		if strings.HasPrefix(value, prefix) {
			return nil
		}
	}
	return &CredentialSecretError{Issue: CredentialSecretPrefixMismatch}
}
