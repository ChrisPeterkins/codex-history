package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/chrispeterkins/codex-history/internal/data"
)

type renderResult struct {
	content   string
	userLines []int
	sections  []sectionLine
}

type sectionLine struct {
	key  string
	line int
}

type renderedMessage struct {
	content  string
	sections []sectionLine
}

func (m Model) renderConversation() renderResult {
	if len(m.messages) == 0 {
		w := m.conversationWidth() - conversationPadding
		boxContent := logoStyle.Render("◈  Codex History") + "\n\n" +
			emptyStyle.Render("Browse your Codex\nconversations")
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 3).
			Align(lipgloss.Center).
			Width(min(36, w)).
			Render(boxContent)
		hints := helpKeyStyle.Render("↑/↓") + " " + helpDescStyle.Render("navigate") +
			"  " + helpKeyStyle.Render("enter") + " " + helpDescStyle.Render("select") +
			"  " + helpKeyStyle.Render("?") + " " + helpDescStyle.Render("help")
		empty := "\n\n" + lipgloss.PlaceHorizontal(w, lipgloss.Center, box) +
			"\n\n" + lipgloss.PlaceHorizontal(w, lipgloss.Center, hints)
		return renderResult{content: empty}
	}

	w := m.conversationWidth() - conversationPadding
	if w < 1 {
		w = 1
	}
	var parts []string
	var userLines []int
	var sectionLines []sectionLine
	lineCount := 0
	appendPart := func(content string) int {
		start := lineCount
		if content != "" {
			parts = append(parts, content)
			lineCount += strings.Count(content, "\n") + 1
		}
		return start
	}

	// Compute and render stats header
	statsHeader := m.renderConversationStats(w)
	appendPart(statsHeader)

	hasRendered := false
	var lastTimestamp time.Time
	for _, msg := range m.messages {
		var rendered string
		var relativeSections []sectionLine

		// Insert time gap indicator if significant time passed
		if hasRendered && !msg.Timestamp.IsZero() && !lastTimestamp.IsZero() {
			gap := msg.Timestamp.Sub(lastTimestamp)
			if gapStr := formatTimeGap(gap, w); gapStr != "" {
				appendPart(gapStr)
			}
		}

		switch msg.Type {
		case "user":
			if msg.RawText != "" {
				if hasRendered {
					appendPart(turnDividerStyle.Render(strings.Repeat("─", w)))
				}
				userLines = append(userLines, lineCount)
				rendered = m.renderUserMessage(msg, w)
			}
		case "assistant":
			result := m.renderAssistantMessage(msg, w)
			rendered, relativeSections = result.content, result.sections
		case "system":
			if msg.Subtype == "turn_duration" && msg.DurationMs > 0 {
				rendered = m.renderSystemMessage(msg, w)
			}
		}

		if rendered != "" {
			start := appendPart(rendered)
			for _, section := range relativeSections {
				section.line += start
				sectionLines = append(sectionLines, section)
			}
			hasRendered = true
			if !msg.Timestamp.IsZero() {
				lastTimestamp = msg.Timestamp
			}
		}
	}

	return renderResult{
		content:   strings.Join(parts, "\n"),
		userLines: userLines,
		sections:  sectionLines,
	}
}

func (m Model) renderUserMessage(msg data.Message, w int) string {
	ts := timestampStyle.Render(msg.Timestamp.Format("15:04:05"))
	avatar := avatarUserStyle.Render("●")
	label := " " + avatar + " " + userLabelStyle.Render("You") + " " + ts

	text := msg.RawText
	if text == "" {
		return ""
	}

	// Render through glamour for inline code, bold, etc.
	mdRendered, err := m.renderer.Render(text)
	if err != nil || strings.TrimSpace(mdRendered) == "" {
		mdRendered = text
	}

	body := userBubbleStyle.Width(w).Render(mdRendered)
	return label + "\n" + body
}

func (m Model) renderAssistantMessage(msg data.Message, w int) renderedMessage {
	ts := timestampStyle.Render(msg.Timestamp.Format("15:04:05"))
	avatar := avatarAssistantStyle.Render("◆")
	label := " " + avatar + " " + assistantLabelStyle.Render("Codex") + " " + ts

	sections := []string{label}
	lines := 1
	var sectionLines []sectionLine
	appendSection := func(content, key string) {
		if content == "" {
			return
		}
		if key != "" {
			sectionLines = append(sectionLines, sectionLine{key: key, line: lines})
		}
		sections = append(sections, content)
		lines += strings.Count(content, "\n") + 1
	}

	for _, block := range msg.ContentBlocks {
		switch block.Type {
		case "text":
			if block.Text == "" {
				continue
			}
			mdRendered, err := m.renderer.Render(block.Text)
			if err != nil || strings.TrimSpace(mdRendered) == "" {
				mdRendered = block.Text
			}
			appendSection(assistantBubbleStyle.Width(w).Render(mdRendered), "")

		case "thinking":
			appendSection(m.renderThinkingBlock(block, msg.UUID, w), "thinking:"+msg.UUID)

		case "tool_use":
			appendSection(m.renderToolCall(block, msg, w), "tool:"+block.ToolID)
		}
	}

	if msg.Usage.OutputTokens > 0 {
		appendSection(m.renderTokenInfo(msg), "")
	}

	return renderedMessage{content: strings.Join(sections, "\n"), sections: sectionLines}
}

func (m Model) renderThinkingBlock(block data.ContentBlock, msgUUID string, w int) string {
	key := "thinking:" + msgUUID
	collapsed := m.isCollapsed(key)

	if collapsed {
		return thinkingGutterStyle.Render(thinkingHeaderStyle.Render("▸ Thinking..."))
	}

	header := thinkingHeaderStyle.Render("▾ Thinking")
	text := block.Thinking
	if text == "" {
		text = "(redacted)"
	}
	if len(text) > maxThinkingLen {
		text = text[:maxThinkingLen] + "\n... (truncated)"
	}
	body := thinkingBodyStyle.Width(w - 6).Render(text)
	return thinkingGutterStyle.Render(header + "\n" + body)
}

func (m Model) renderToolCall(block data.ContentBlock, msg data.Message, w int) string {
	key := "tool:" + block.ToolID
	collapsed := m.isCollapsed(key)

	// Build header with tool name and a brief summary
	summary := toolCallSummary(block)
	arrow := "▸"
	if !collapsed {
		arrow = "▾"
	}

	header := toolHeaderStyle.Render(fmt.Sprintf("%s %s", arrow, toolBadge(block.ToolName))) +
		" " + toolHeaderStyle.Render(summary)

	if collapsed {
		return toolGutterCollapsedStyle.Render(header)
	}

	var bodyParts []string

	// Show input
	inputStr := formatToolInput(block)
	if inputStr != "" {
		bodyParts = append(bodyParts, toolBodyStyle.Width(w-6).Render(inputStr))
	}

	// Find and show the paired result
	for _, pair := range msg.ToolPairs {
		if pair.Use.ToolID == block.ToolID {
			result := pair.Result.Content
			if result != "" {
				if len(result) > maxToolResultLen {
					result = result[:maxToolResultLen] + "\n... (truncated)"
				}
				result = hardWrap(result, w-8)
				if pair.Result.IsError {
					bodyParts = append(bodyParts, toolErrorStyle.Width(w-6).Render("Error: "+result))
				} else {
					bodyParts = append(bodyParts, toolBodyStyle.Width(w-6).Render(result))
				}
			}
			break
		}
	}

	content := header
	if len(bodyParts) > 0 {
		content += "\n" + strings.Join(bodyParts, "\n")
	}

	return toolGutterExpandedStyle.Render(content)
}

func (m Model) renderSystemMessage(msg data.Message, w int) string {
	dur := time.Duration(msg.DurationMs) * time.Millisecond
	var durStr string
	if dur >= time.Minute {
		durStr = fmt.Sprintf("%dm %ds", int(dur.Minutes()), int(dur.Seconds())%60)
	} else {
		durStr = fmt.Sprintf("%.1fs", dur.Seconds())
	}
	line := fmt.Sprintf("── turn completed in %s ──", durStr)
	return systemMessageStyle.Width(w).Render(line)
}

// renderConversationStats builds a stats summary line for the conversation header.
func (m Model) renderConversationStats(w int) string {
	if len(m.messages) == 0 {
		return ""
	}

	var msgCount, toolCount, totalTokens, totalDurationMs int
	for _, msg := range m.messages {
		if msg.Type == "user" || msg.Type == "assistant" {
			msgCount++
		}
		toolCount += len(msg.ToolPairs)
		totalTokens += msg.Usage.OutputTokens
		if msg.Type == "system" && msg.Subtype == "turn_duration" {
			totalDurationMs += msg.DurationMs
		}
	}

	var statParts []string
	if msgCount > 0 {
		statParts = append(statParts, fmt.Sprintf("%d messages", msgCount))
	}
	if toolCount > 0 {
		statParts = append(statParts, fmt.Sprintf("%d tool calls", toolCount))
	}
	if totalTokens > 0 {
		statParts = append(statParts, formatTokenCount(totalTokens)+" tokens")
	}
	if totalDurationMs > 0 {
		dur := time.Duration(totalDurationMs) * time.Millisecond
		if dur >= time.Minute {
			statParts = append(statParts, fmt.Sprintf("%dm %ds", int(dur.Minutes()), int(dur.Seconds())%60))
		} else {
			statParts = append(statParts, fmt.Sprintf("%.0fs", dur.Seconds()))
		}
	}

	if len(statParts) == 0 {
		return ""
	}

	result := tokenStyle.Render("  " + strings.Join(statParts, " · "))

	// Add file change summary
	fileNames := m.collectChangedFiles()
	if len(fileNames) > 0 {
		var fileStr string
		if len(fileNames) <= 3 {
			fileStr = strings.Join(fileNames, ", ")
		} else {
			fileStr = strings.Join(fileNames[:3], ", ") + fmt.Sprintf(" (+%d more)", len(fileNames)-3)
		}
		result += "\n" + tokenStyle.Render("  "+fileStr)
	}

	return result
}

// collectChangedFiles extracts unique file names from Edit/Write tool calls.
func (m Model) collectChangedFiles() []string {
	seen := make(map[string]bool)
	var names []string
	for _, msg := range m.messages {
		for _, pair := range msg.ToolPairs {
			if pair.Name != "Edit" && pair.Name != "Write" {
				continue
			}
			if path, ok := pair.Use.Input["file_path"].(string); ok {
				name := filepath.Base(path)
				if !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}
	return names
}

func (m Model) renderTokenInfo(msg data.Message) string {
	u := msg.Usage
	parts := []string{msg.Model}

	if u.OutputTokens > 0 {
		parts = append(parts, formatTokenCount(u.OutputTokens)+" out")
	}
	if u.CacheCreationInputTokens > 0 {
		parts = append(parts, formatTokenCount(u.CacheCreationInputTokens)+" cache create")
	}
	if u.CacheReadInputTokens > 0 {
		parts = append(parts, formatTokenCount(u.CacheReadInputTokens)+" cache read")
	}

	return tokenStyle.Render("  " + strings.Join(parts, " · "))
}

func (m Model) isCollapsed(key string) bool {
	collapsed, exists := m.collapsed[key]
	if !exists {
		return true // default: collapsed
	}
	return collapsed
}

// Helper functions (toolBadge, inputStr, toolCallSummary, formatToolInput,
// renderDiff, formatTimeGap, highlightCode, shortPath, hardWrap, truncateStr,
// formatSize, formatTokenCount, clamp) are in toolrender.go
