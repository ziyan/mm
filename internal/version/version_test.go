package version

import "testing"

func TestVersionReturnsDefault(t *testing.T) {
	if got := Version(); got != "0.1.0" {
		t.Errorf("Version() = %q, want %q", got, "0.1.0")
	}
}

func TestCommitReturnsDefault(t *testing.T) {
	if got := Commit(); got != "unknown" {
		t.Errorf("Commit() = %q, want %q", got, "unknown")
	}
}

func TestUserAgentFormat(t *testing.T) {
	if got := UserAgent(); got != "mm/0.1.0+unknown" {
		t.Errorf("UserAgent() = %q, want %q", got, "mm/0.1.0+unknown")
	}
}

func TestUserAgentReflectsOverrides(t *testing.T) {
	originalVersion := version
	originalCommit := commit
	defer func() {
		version = originalVersion
		commit = originalCommit
	}()

	version = "2.3.4"
	commit = "abc1234"

	if got := Version(); got != "2.3.4" {
		t.Errorf("Version() = %q, want %q", got, "2.3.4")
	}
	if got := Commit(); got != "abc1234" {
		t.Errorf("Commit() = %q, want %q", got, "abc1234")
	}
	if got := UserAgent(); got != "mm/2.3.4+abc1234" {
		t.Errorf("UserAgent() = %q, want %q", got, "mm/2.3.4+abc1234")
	}
}
