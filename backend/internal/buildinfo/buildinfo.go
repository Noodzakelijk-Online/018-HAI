// Package buildinfo exposes build/version metadata. The Version/Commit/BuildTime
// values are intended to be set at link time via -ldflags; they default to
// development placeholders so the binary is always self-describing.
package buildinfo

import "runtime"

var (
	// Version is the semantic version, set at build time.
	Version = "dev"
	// Commit is the git commit, set at build time.
	Commit = "unknown"
	// BuildTime is the build timestamp, set at build time.
	BuildTime = "unknown"
)

// Info is a snapshot of build metadata.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
}

// Snapshot returns the current build metadata plus the Go runtime version.
func Snapshot() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
	}
}
