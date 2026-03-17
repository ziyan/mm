package printer

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name string
		ts   int64
		want string
	}{
		{"zero", 0, ""},
		{"today", time.Now().Truncate(time.Hour).UnixMilli(), time.Now().Truncate(time.Hour).Format("15:04")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTime(tt.ts)
			if got != tt.want {
				t.Errorf("FormatTime(%d) = %q, want %q", tt.ts, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"this is a long string", 10, "this is..."},
		{"hello\nworld", 20, "hello world"},
	}
	for _, tt := range tests {
		got := Truncate(tt.s, tt.max)
		if got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
		}
	}
}

func TestChannelTypeName(t *testing.T) {
	tests := map[string]string{
		"O": "public",
		"P": "private",
		"D": "direct",
		"G": "group",
		"X": "X",
	}
	for input, want := range tests {
		got := ChannelTypeName(input)
		if got != want {
			t.Errorf("ChannelTypeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	Stdout = &buf

	headers := []string{"NAME", "VALUE"}
	rows := [][]string{
		{"foo", "bar"},
		{"hello", "world"},
	}
	PrintTable(headers, rows)

	output := buf.String()
	if !strings.Contains(output, "NAME") {
		t.Error("table output missing header NAME")
	}
	if !strings.Contains(output, "foo") {
		t.Error("table output missing row 'foo'")
	}
	if !strings.Contains(output, "world") {
		t.Error("table output missing row 'world'")
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	Stdout = &buf

	data := map[string]string{"key": "value"}
	PrintJSON(data)

	output := buf.String()
	if !strings.Contains(output, `"key"`) {
		t.Error("JSON output missing key")
	}
	if !strings.Contains(output, `"value"`) {
		t.Error("JSON output missing value")
	}
}

func TestPrintEmptyTable(t *testing.T) {
	var buf bytes.Buffer
	Stdout = &buf

	PrintTable([]string{"A", "B"}, nil)
	if buf.Len() != 0 {
		t.Error("empty table should produce no output")
	}
}
