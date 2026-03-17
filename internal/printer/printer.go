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

func PrintJSON(value interface{}) {
	encoder := json.NewEncoder(Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func PrintError(message string, args ...interface{}) {
	_, _ = fmt.Fprintf(Stderr, color.RedString("Error: ")+message+"\n", args...)
}

func PrintSuccess(message string, args ...interface{}) {
	_, _ = fmt.Fprintf(Stdout, color.GreenString("✓ ")+message+"\n", args...)
}

func PrintInfo(message string, args ...interface{}) {
	_, _ = fmt.Fprintf(Stdout, message+"\n", args...)
}

func PrintTable(headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}

	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = len(header)
	}
	for _, row := range rows {
		for index, cell := range row {
			if index < len(widths) && len(cell) > widths[index] {
				widths[index] = len(cell)
			}
		}
	}

	headerParts := make([]string, len(headers))
	for index, header := range headers {
		headerParts[index] = fmt.Sprintf("%-*s", widths[index], header)
	}
	headerLine := strings.Join(headerParts, "  ")
	_, _ = fmt.Fprintln(Stdout, color.New(color.Bold).Sprint(headerLine))

	for _, row := range rows {
		parts := make([]string, len(headers))
		for index := range headers {
			cell := ""
			if index < len(row) {
				cell = row[index]
			}
			parts[index] = fmt.Sprintf("%-*s", widths[index], cell)
		}
		_, _ = fmt.Fprintln(Stdout, strings.Join(parts, "  "))
	}
}

func FormatTime(timestamp int64) string {
	if timestamp == 0 {
		return ""
	}
	parsed := time.Unix(timestamp/1000, 0)
	now := time.Now()
	if parsed.Year() == now.Year() && parsed.YearDay() == now.YearDay() {
		return parsed.Format("15:04")
	}
	if now.Sub(parsed) < 7*24*time.Hour {
		return parsed.Format("Mon 15:04")
	}
	return parsed.Format("2006-01-02 15:04")
}

func Truncate(source string, max int) string {
	source = strings.ReplaceAll(source, "\n", " ")
	if len(source) <= max {
		return source
	}
	return source[:max-3] + "..."
}

func ChannelTypeName(channelType string) string {
	switch channelType {
	case "O":
		return "public"
	case "P":
		return "private"
	case "D":
		return "direct"
	case "G":
		return "group"
	default:
		return channelType
	}
}
