package version

import "fmt"

// version and commit are set at build time via -ldflags.
var (
	version = "0.1.0"
	commit  = "unknown"
)

func Version() string { return version }

func Commit() string { return commit }

func UserAgent() string {
	return fmt.Sprintf("mm/%s+%s", version, commit)
}
