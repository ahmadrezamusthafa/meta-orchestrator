package symlink

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// PackageManifest represents standard npm package.json structure.
type PackageManifest struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
}

// SymlinkResult logs created symlink mapping.
type SymlinkResult struct {
	TargetLink   string `json:"target_link"`
	SourcePath   string `json:"source_path"`
	PackageName  string `json:"package_name"`
	ConsumerRepo string `json:"consumer_repo"`
}

// AutoSymlinkResolver discovers and establishes cross-repo filesystem links.
type AutoSymlinkResolver struct{}

// NewAutoSymlinkResolver creates a resolver.
func NewAutoSymlinkResolver() *AutoSymlinkResolver {
	return &AutoSymlinkResolver{}
}

// ResolveWorkspace inspects all repos in workspace and creates relative symlinks.
func (r *AutoSymlinkResolver) ResolveWorkspace(repoPaths map[string]string) ([]SymlinkResult, error) {
	var results []SymlinkResult

	// 1. Index provided packages
	providedPackages := make(map[string]PackageManifest)
	for _, repoPath := range repoPaths {
		pkgFile := filepath.Join(repoPath, "package.json")
		if data, err := os.ReadFile(pkgFile); err == nil {
			var m PackageManifest
			if err := json.Unmarshal(data, &m); err == nil && m.Name != "" {
				providedPackages[m.Name] = m
			}
		}
	}

	// 2. Discover consumers and establish symlinks
	for consumerName, consumerPath := range repoPaths {
		pkgFile := filepath.Join(consumerPath, "package.json")
		data, err := os.ReadFile(pkgFile)
		if err != nil {
			continue
		}

		var consumerManifest PackageManifest
		if err := json.Unmarshal(data, &consumerManifest); err != nil {
			continue
		}

		// Check all dependencies
		allDeps := make(map[string]string)
		for k, v := range consumerManifest.Dependencies {
			allDeps[k] = v
		}
		for k, v := range consumerManifest.DevDependencies {
			allDeps[k] = v
		}

		for depName, reqVersion := range allDeps {
			providerManifest, isProvided := providedPackages[depName]
			if !isProvided {
				continue
			}

			// Collision Check
			if CheckCollision(reqVersion, providerManifest.Version) {
				return nil, &CollisionError{
					PackageName: depName,
					Consumer:    consumerName,
					RequiredVer: reqVersion,
					Producer:    providerManifest.Name,
					ProvidedVer: providerManifest.Version,
				}
			}

			// Establish symlink inside node_modules
			nodeModulesDir := filepath.Join(consumerPath, "node_modules")
			if strings.Contains(depName, "/") {
				// Scoped package (e.g. @org/package)
				parts := strings.Split(depName, "/")
				nodeModulesDir = filepath.Join(nodeModulesDir, parts[0])
			}
			_ = os.MkdirAll(nodeModulesDir, 0755)

			linkPath := filepath.Join(consumerPath, "node_modules", depName)
			providerPath := repoPaths[providerManifest.Name]
			if providerPath == "" {
				// Search by matching package name in map
				for _, p := range repoPaths {
					if filepath.Base(p) == providerManifest.Name || strings.HasSuffix(p, filepath.Base(providerManifest.Name)) {
						providerPath = p
						break
					}
				}
			}

			_ = os.Remove(linkPath)
			if err := os.Symlink(providerPath, linkPath); err == nil {
				results = append(results, SymlinkResult{
					TargetLink:   linkPath,
					SourcePath:   providerPath,
					PackageName:  depName,
					ConsumerRepo: consumerName,
				})
			}
		}
	}

	return results, nil
}
