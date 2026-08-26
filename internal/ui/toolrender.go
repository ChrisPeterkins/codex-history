package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/chrispeterkins/codex-history/internal/data"
)

// toolBadge renders a tool name badge with a color specific to the tool type.
func toolBadge(name string) string {
	bg, ok := toolBadgeColors[name]
	if !ok {
		bg = colorWarm
	}
	return toolBadgeStyle.Background(bg).Render(name)
}

// inputStr extracts a string value from a tool input map.
func inputStr(input map[string]interface{}, key string) string {
	if v, ok := input[key].(string); ok {
		return v
	}
	return ""
}

// toolCallSummary returns a brief description of what a tool call does.
func toolCallSummary(block data.ContentBlock) string {
	if block.Input == nil {
		return ""
	}
	switch block.ToolName {
	case "Bash":
		cmd := inputStr(block.Input, "command")
		if cmd != "" {
			cmd = strings.ReplaceAll(cmd, "\n", " ")
			if len(cmd) > maxCommandSummaryLen {
				cmd = cmd[:maxCommandSummaryLen-3] + "..."
			}
			return cmd
		}
	case "Read", "Write":
		if path := inputStr(block.Input, "file_path"); path != "" {
			return shortPath(path)
		}
	case "Edit":
		if path := inputStr(block.Input, "file_path"); path != "" {
			summary := shortPath(path)
			old := inputStr(block.Input, "old_string")
			new := inputStr(block.Input, "new_string")
			if old != "" && new != "" {
				summary += fmt.Sprintf(" (-%d/+%d)", strings.Count(old, "\n")+1, strings.Count(new, "\n")+1)
			}
			return summary
		}
	case "Glob":
		return inputStr(block.Input, "pattern")
	case "Grep":
		return inputStr(block.Input, "pattern")
	case "Agent":
		return inputStr(block.Input, "description")
	}
	return ""
}

// formatToolInput renders tool input as a readable string.
func formatToolInput(block data.ContentBlock) string {
	if block.Input == nil {
		return ""
	}
	switch block.ToolName {
	case "Bash":
		cmd := inputStr(block.Input, "command")
		if cmd == "" {
			return ""
		}
		lines := strings.Split(cmd, "\n")
		if len(lines) == 1 {
			return "$ " + cmd
		}
		var parts []string
		for i, l := range lines {
			if i == 0 {
				parts = append(parts, "$ "+l)
			} else {
				parts = append(parts, "> "+l)
			}
		}
		return strings.Join(parts, "\n")
	case "Edit":
		var parts []string
		if path := inputStr(block.Input, "file_path"); path != "" {
			parts = append(parts, "File: "+shortPath(path))
		}
		if diff := inputStr(block.Input, "diff"); diff != "" {
			parts = append(parts, colorizeDiff(diff))
			return strings.Join(parts, "\n")
		}
		old, new := inputStr(block.Input, "old_string"), inputStr(block.Input, "new_string")
		if old != "" && new != "" {
			parts = append(parts, renderDiff(old, new))
		}
		return strings.Join(parts, "\n")
	case "Write":
		path := inputStr(block.Input, "file_path")
		if path == "" {
			return ""
		}
		header := fmt.Sprintf("File: %s", shortPath(path))
		content := inputStr(block.Input, "content")
		if content == "" {
			return header
		}
		header = fmt.Sprintf("File: %s (%d lines)", shortPath(path), strings.Count(content, "\n")+1)
		preview := content
		if len(preview) > maxToolResultLen {
			preview = preview[:maxToolResultLen] + "\n... (truncated)"
		}
		if highlighted := highlightCode(preview, path); highlighted != "" {
			return header + "\n" + highlighted
		}
		var diffLines []string
		for _, l := range strings.Split(preview, "\n") {
			diffLines = append(diffLines, diffAddStyle.Render("+ "+l))
		}
		return header + "\n" + strings.Join(diffLines, "\n")
	case "Read":
		if path := inputStr(block.Input, "file_path"); path != "" {
			return "File: " + shortPath(path)
		}
	default:
		b, err := json.MarshalIndent(block.Input, "", "  ")
		if err == nil && len(b) < 500 {
			return string(b)
		}
	}
	return ""
}

func colorizeDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			lines[i] = diffAddStyle.Render(line)
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			lines[i] = diffRemoveStyle.Render(line)
		default:
			lines[i] = timestampStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

// renderDiff renders old/new strings as a unified diff with context lines.
func renderDiff(old, new string) string {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	prefixLen := 0
	minLen := min(len(oldLines), len(newLines))
	for prefixLen < minLen && oldLines[prefixLen] == newLines[prefixLen] {
		prefixLen++
	}

	suffixLen := 0
	for suffixLen < minLen-prefixLen &&
		oldLines[len(oldLines)-1-suffixLen] == newLines[len(newLines)-1-suffixLen] {
		suffixLen++
	}

	var lines []string

	// Context before
	contextStart := max(0, prefixLen-2)
	for i := contextStart; i < prefixLen; i++ {
		lines = append(lines, timestampStyle.Render("  "+oldLines[i]))
	}

	// Removed
	for i := prefixLen; i < len(oldLines)-suffixLen; i++ {
		lines = append(lines, diffRemoveStyle.Render("- "+oldLines[i]))
	}

	// Added
	for i := prefixLen; i < len(newLines)-suffixLen; i++ {
		lines = append(lines, diffAddStyle.Render("+ "+newLines[i]))
	}

	// Context after
	contextEnd := min(len(oldLines), len(oldLines)-suffixLen+2)
	for i := len(oldLines) - suffixLen; i < contextEnd; i++ {
		lines = append(lines, timestampStyle.Render("  "+oldLines[i]))
	}

	return strings.Join(lines, "\n")
}

// formatTimeGap returns a styled time gap indicator if the gap is significant.
func formatTimeGap(gap time.Duration, w int) string {
	if gap < 5*time.Minute {
		return ""
	}
	var label string
	switch {
	case gap < time.Hour:
		label = fmt.Sprintf("%dm", int(gap.Minutes()))
	case gap < 24*time.Hour:
		h := int(gap.Hours())
		m := int(gap.Minutes()) % 60
		if m > 0 {
			label = fmt.Sprintf("%dh %dm", h, m)
		} else {
			label = fmt.Sprintf("%dh", h)
		}
	default:
		d := int(gap.Hours() / 24)
		h := int(gap.Hours()) % 24
		if h > 0 {
			label = fmt.Sprintf("%dd %dh", d, h)
		} else {
			label = fmt.Sprintf("%dd", d)
		}
	}
	if gap < time.Hour {
		return systemMessageStyle.Width(w).Render("··· " + label + " ···")
	}
	return systemMessageStyle.Width(w).Render("── " + label + " ──")
}

// highlightCode applies syntax highlighting to code based on filename extension.
func highlightCode(code, filename string) string {
	if filepath.Ext(filename) == "" {
		return ""
	}
	lexer := lexers.Match(filename)
	if lexer == nil {
		return ""
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("monokai")
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		return ""
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return ""
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return ""
	}
	return buf.String()
}

// shortPath returns the last 2-3 segments of a file path.
func shortPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		return ".../" + strings.Join(parts[len(parts)-3:], "/")
	}
	return path
}

// hardWrap wraps lines that exceed maxWidth by inserting line breaks.
func hardWrap(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	var result []string
	for _, line := range strings.Split(s, "\n") {
		for len(line) > maxWidth {
			result = append(result, line[:maxWidth])
			line = line[maxWidth:]
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

// truncateStr truncates a string and replaces newlines with spaces.
func truncateStr(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

// formatSize formats byte counts as human-readable strings.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// formatTokenCount formats token counts (e.g., 1500 → "1.5k").
func formatTokenCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// clamp constrains val to the range [lo, hi].
func clamp(val, lo, hi int) int {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}
