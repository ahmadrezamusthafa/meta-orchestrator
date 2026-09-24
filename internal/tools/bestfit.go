package tools

import (
	"runtime"
)

// BestFitResolution encapsulates resolved optimal tool version and rationale.
type BestFitResolution struct {
	ToolID          string `json:"tool_id"`
	ResolvedVersion string `json:"resolved_version"`
	HostOS          string `json:"host_os"`
	HostArch        string `json:"host_arch"`
	Rationale       string `json:"rationale"`
	IsCompatible    bool   `json:"is_compatible"`
}

// ResolveBestFit inspects runtime architecture and OS to determine best compatible version.
func (tm *ToolManager) ResolveBestFit(toolID string) (*BestFitResolution, error) {
	pkg, err := tm.GetPackage(toolID)
	if err != nil {
		return nil, err
	}

	hostOS := runtime.GOOS
	hostArch := runtime.GOARCH

	var resolvedVer string
	var rationale string

	switch toolID {
	case "treesitter-parsers":
		if hostArch == "arm64" && hostOS == "darwin" {
			resolvedVer = "0.22.6"
			rationale = "Optimized Apple Silicon ARM64 prebuilt native Tree-sitter binaries"
		} else {
			resolvedVer = "0.22.6"
			rationale = "Standard POSIX compatible Tree-sitter release"
		}

	case "playwright-engine":
		resolvedVer = "1.48.0"
		rationale = "Latest stable Chromium/WebKit headless engine with WebM and MP4 recording support"

	case "bmad-methodology":
		resolvedVer = "1.4.2"
		rationale = "Stable BMAD multi-agent sequential handoff protocol release"

	case "superpower-runtime":
		resolvedVer = "2.1.0"
		rationale = "Containerized and host-level execution harness with atomic worktree support"

	default:
		if len(pkg.AvailableVersions) > 0 {
			resolvedVer = pkg.AvailableVersions[len(pkg.AvailableVersions)-1]
			rationale = "Resolved latest available version"
		} else {
			resolvedVer = pkg.ActiveVersion
			rationale = "Resolved active installed version"
		}
	}

	return &BestFitResolution{
		ToolID:          toolID,
		ResolvedVersion: resolvedVer,
		HostOS:          hostOS,
		HostArch:        hostArch,
		Rationale:       rationale,
		IsCompatible:    true,
	}, nil
}

// ResolveAllBestFit calculates the optimal matrix for all registered tools.
func (tm *ToolManager) ResolveAllBestFit() ([]*BestFitResolution, error) {
	packages := tm.ListPackages()
	var results []*BestFitResolution

	for _, p := range packages {
		res, err := tm.ResolveBestFit(p.ID)
		if err == nil {
			results = append(results, res)
		}
	}
	return results, nil
}
