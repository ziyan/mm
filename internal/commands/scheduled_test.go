package commands

import (
	"testing"
	"time"
)

func TestParseScheduleTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(int64) bool
	}{
		{
			name:    "duration 1h",
			input:   "1h",
			wantErr: false,
			check: func(ms int64) bool {
				expected := time.Now().Add(time.Hour).UnixMilli()
				diff := ms - expected
				return diff > -5000 && diff < 5000
			},
		},
		{
			name:    "duration 30m",
			input:   "30m",
			wantErr: false,
			check: func(ms int64) bool {
				expected := time.Now().Add(30 * time.Minute).UnixMilli()
				diff := ms - expected
				return diff > -5000 && diff < 5000
			},
		},
		{
			name:    "duration 1h30m",
			input:   "1h30m",
			wantErr: false,
			check: func(ms int64) bool {
				expected := time.Now().Add(90 * time.Minute).UnixMilli()
				diff := ms - expected
				return diff > -5000 && diff < 5000
			},
		},
		{
			name:    "absolute datetime",
			input:   "2030-01-15T14:30",
			wantErr: false,
			check: func(ms int64) bool {
				return ms > 0
			},
		},
		{
			name:    "absolute datetime with seconds",
			input:   "2030-01-15T14:30:00",
			wantErr: false,
			check: func(ms int64) bool {
				return ms > 0
			},
		},
		{
			name:    "time only",
			input:   "14:30",
			wantErr: false,
			check: func(ms int64) bool {
				return ms > 0
			},
		},
		{
			name:    "invalid format",
			input:   "not-a-time",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScheduleTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseScheduleTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil && !tt.check(got) {
				t.Errorf("parseScheduleTime(%q) = %d, check failed", tt.input, got)
			}
		})
	}
}
