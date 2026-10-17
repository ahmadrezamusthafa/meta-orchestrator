package api

import (
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// stackBrief tells the agent the tech stack of the task's repositories: how to verify a change in
// each, the conventions to keep and the commands it must never run (see stack.Brief).
func (r *Router) stackBrief(t *types.Task) string {
	repos := r.repoStacks(t.AssignedRepos)
	brief := stack.Brief(repos)
	if brief == "" || r.toolchain == nil {
		return brief
	}
	rep := r.toolchain.Report(toolchain.Requirements(repos), false)
	if missing := append(append([]string{}, rep.Missing...), rep.MissingRecommended...); len(missing) > 0 {
		brief += "Not installed on this machine: " + toolchain.Names(missing) + ". Skip the checks that need them and list " +
			"them in your summary. Do not install tools yourself — the operator installs them from the Toolchain panel.\n"
	}
	return brief
}

func (r *Router) repoStacks(names []string) []stack.Repo {
	paths := r.repoPaths(names)
	var out []stack.Repo
	for _, name := range names {
		if p := paths[name]; p != "" {
			out = append(out, stack.Repo{Name: name, Profile: stack.For(p)})
		}
	}
	return out
}

// repoLabels names each repository with its stack ("infra-live (OpenTofu, Terragrunt)"), so the
// complexity analyst sees infrastructure or identity work the task text may not mention.
func (r *Router) repoLabels(names []string) []string {
	stacks := map[string][]string{}
	for _, s := range r.repoStacks(names) {
		stacks[s.Name] = s.Profile.Names()
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if techs := stacks[n]; len(techs) > 0 {
			n += " (" + strings.Join(techs, ", ") + ")"
		}
		out = append(out, n)
	}
	return out
}
