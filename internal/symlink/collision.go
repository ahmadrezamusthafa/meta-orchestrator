package symlink

import (
	"fmt"
	"strings"
)

// DependencyRef represents a package requirement discovered in a manifest.
type DependencyRef struct {
	RepoName         string `json:"repo_name"`
	PackageName      string `json:"package_name"`
	RequiredVersion  string `json:"required_version"`
	ProvidedVersion  string `json:"provided_version,omitempty"`
	ManifestPath     string `json:"manifest_path"`
}

// CollisionError details an incompatible cross-repo dependency constraint.
type CollisionError struct {
	PackageName string
	Consumer    string
	RequiredVer string
	Producer    string
	ProvidedVer string
}

func (e *CollisionError) Error() string {
	return fmt.Sprintf("dependency collision for '%s': %s requires %s, but %s provides %s",
		e.PackageName, e.Consumer, e.RequiredVer, e.Producer, e.ProvidedVer)
}

// CheckCollision evaluates if provided version satisfies required semver prefix.
func CheckCollision(required string, provided string) bool {
	normReq := strings.TrimLeft(required, "^~>=v")
	normProv := strings.TrimLeft(provided, "v")

	// If required specifies major version, ensure major versions match
	reqParts := strings.Split(normReq, ".")
	provParts := strings.Split(normProv, ".")

	if len(reqParts) > 0 && len(provParts) > 0 {
		if reqParts[0] != provParts[0] {
			return true // Major version collision
		}
	}
	return false
}
