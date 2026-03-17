package printer

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

var (
	JSONOutput bool
	Stdout     io.Writer = os.Stdout
	Stderr     io.Writer = os.Stderr
)

func PrintJSON(v interface{}) {
	enc := json.NewEncoder(Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func PrintError(msg string, args ...interface{}) {
	_, _ = fmt.Fprintf(Stderr, color.RedString("Error: ")+msg+"\n", args...)
}

func PrintSuccess(msg string, args ...interface{}) {
	_, _ = fmt.Fprintf(Stdout, color.GreenString("✓ ")+msg+"\n", args...)
}

func PrintInfo(msg string, args ...interface{}) {
	_, _ = fmt.Fprintf(Stdout, msg+"\n", args...)
}

func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header
	headerParts := make([]string, len(headers))
	for i, h := range headers {
		headerParts[i] = fmt.Sprintf("%-*s", widths[i], h)
	}
	headerLine := strings.Join(headerParts, "  ")
	_, _ = fmt.Fprintln(Stdout, color.New(color.Bold).Sprint(headerLine))

	// Print rows
	for _, row := range rows {
		parts := make([]string, len(headers))
		for i := range headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			parts[i] = fmt.Sprintf("%-*s", widths[i], cell)
		}
		_, _ = fmt.Fprintln(Stdout, strings.Join(parts, "  "))
	}
}

func FormatTime(ts int64) string {
	if ts == 0 {
		return ""
	}
	t := time.Unix(ts/1000, 0)
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if now.Sub(t) < 7*24*time.Hour {
		return t.Format("Mon 15:04")
	}
	return t.Format("2006-01-02 15:04")
}

func Truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func ChannelTypeName(t string) string {
	switch t {
	case "O":
		return "public"
	case "P":
		return "private"
	case "D":
		return "direct"
	case "G":
		return "group"
	default:
		return t
	}
}
