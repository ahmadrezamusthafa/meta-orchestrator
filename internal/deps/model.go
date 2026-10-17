// Package deps reports a repository's project dependencies (React, Go modules, Flutter packages,
// …) per ecosystem and installs them the way the repository's own lockfile says, with the
// repository's own package manager.
//
// Installing never changes versions: it reproduces the lockfile (npm ci, pnpm install
// --frozen-lockfile, go mod download, …), so running it again is also how local drift is rolled
// back. Changing a version is a code change and goes through a task (see the API layer).
//
// Ecosystems and package managers come from internal/stack (Profile.Packages); this package adds
// lockfiles, install commands, installed state and dependency lists.
package deps

import "time"

// Ecosystem is one dependency manifest of a repository.
type Ecosystem struct {
	Ecosystem string `json:"ecosystem"` // node, go, flutter, dart, ruby, python, jvm
	Manager   string `json:"manager"`   // npm, pnpm, yarn, bun, go, flutter, dart, bundler, pip, maven, gradle
	Manifest  string `json:"manifest"`  // repository-relative
	Lockfile  string `json:"lockfile"`  // repository-relative, "" when there is none
	// Installed reports whether the dependencies on disk match the lockfile.
	Installed bool `json:"installed"`
	// Drift is a sentence when the installed state disagrees with the lockfile, else "".
	Drift   string  `json:"drift"`
	Install Install `json:"install"`
	// MissingTool is the toolchain id to install when the manager binary is missing, else "".
	MissingTool  string       `json:"missing_tool"`
	Dependencies []Dependency `json:"dependencies"`

	// Dir is the repository-relative folder the install command runs in.
	Dir string `json:"-"`
	// Missing is set when the install command's binary cannot be found, with or without a
	// toolchain entry to install it from.
	Missing bool `json:"-"`
	// bin is the binary to execute for Install.Command[0] (absolute for wrappers).
	bin string
}

// Install is how the ecosystem's dependencies are installed.
type Install struct {
	Command []string `json:"command"`
	// Auto is set when the orchestrator runs the command itself; otherwise it is shown to copy.
	Auto bool   `json:"auto"`
	Note string `json:"note"`
}

// Dependency is one declared dependency.
type Dependency struct {
	Name      string `json:"name"`
	Wanted    string `json:"wanted"`    // the manifest's range or constraint
	Installed string `json:"installed"` // what is installed (or locked), "" when unknown
	Latest    string `json:"latest"`    // only filled on request
	Dev       bool   `json:"dev"`
	Key       bool   `json:"key"` // a framework or tool worth showing first
}

// Job is one install run.
type Job struct {
	ID          string    `json:"id"`
	Package     string    `json:"package"` // "repo:<repo>"
	Action      string    `json:"action"`  // "install"
	Version     string    `json:"version"`
	FromVersion string    `json:"from_version"`
	State       string    `json:"state"` // queued, running, succeeded, failed
	Log         []string  `json:"log"`
	Message     string    `json:"message"`
	Started     time.Time `json:"started"`
	Finished    time.Time `json:"finished"`
}

// Job states.
const (
	StateQueued    = "queued"
	StateRunning   = "running"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
)

func (j *Job) clone() Job {
	cp := *j
	cp.Log = append([]string{}, j.Log...)
	return cp
}

// Done reports whether the job has finished.
func (j Job) Done() bool { return j.State == StateSucceeded || j.State == StateFailed }
