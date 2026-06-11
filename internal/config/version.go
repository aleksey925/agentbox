package config

import (
	"fmt"
	"regexp"
)

// versionPattern bounds a version/tag to a single, safe path component. A version
// becomes a directory name under ~/.agentbox/bin/<agent>/ and arrives from
// untrusted sources - network responses and `agent use` arguments - so it must
// not carry path separators.
var versionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidateVersion rejects a version that could escape the agent's bin dir when
// joined into a path. The charset already forbids separators; "." and ".." are
// the only in-charset values that traverse, so they are rejected explicitly.
func ValidateVersion(version string) error {
	if version == "." || version == ".." || !versionPattern.MatchString(version) {
		return fmt.Errorf("invalid version %q", version)
	}
	return nil
}
