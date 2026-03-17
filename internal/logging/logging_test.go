package logging

import (
	"testing"
)

func TestSetup(t *testing.T) {
	// Should not panic with valid levels
	levels := []string{"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}
	for _, level := range levels {
		Setup(level)
	}

	// Should not panic with invalid level (falls back to INFO)
	Setup("INVALID")
	Setup("")
}

func TestLoggerExists(t *testing.T) {
	if Log == nil {
		t.Error("Log should not be nil")
	}
}
