package docker

import (
	"fmt"
	"path/filepath"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// SkillVolumeMount represents a bind mount configuration for a skill into a container.
type SkillVolumeMount struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only"`
	Format        string `json:"format"`
}

// GenerateSkillMounts computes the required volume mounts for registered polyglot skills.
func GenerateSkillMounts(skills []*types.UniversalSkillContract, taskID string) []SkillVolumeMount {
	var mounts []SkillVolumeMount

	for _, s := range skills {
		if s.SourceLocation == "" {
			continue
		}

		containerDest := fmt.Sprintf("/opt/skills/%s", s.Name)
		readOnly := true
		// Superpower host tools might require writable scratch
		if s.SourceFormat == types.SkillFormatSuperpower {
			readOnly = false
		}

		mounts = append(mounts, SkillVolumeMount{
			HostPath:      s.SourceLocation,
			ContainerPath: containerDest,
			ReadOnly:      readOnly,
			Format:        string(s.SourceFormat),
		})
	}

	// Always mount MCP bridge and task artifacts
	mounts = append(mounts, SkillVolumeMount{
		HostPath:      filepath.Join(".sdlc", "artifacts", taskID),
		ContainerPath: "/sdlc/artifacts",
		ReadOnly:      false,
		Format:        "artifacts_bridge",
	})

	return mounts
}
