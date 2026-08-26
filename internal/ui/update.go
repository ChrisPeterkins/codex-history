package ui

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type updateAvailableMsg struct {
	version string
}

// checkForUpdate queries the GitHub API for the latest release tag.
// Runs as a background Cmd so it never blocks the UI.
func checkForUpdate(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		// Don't check for dev builds
		if currentVersion == "dev" || currentVersion == "" {
			return nil
		}

		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get("https://api.github.com/repos/ChrisPeterkins/codex-history/releases/latest")
		if err != nil {
			return nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return nil
		}

		var release struct {
			TagName string `json:"tag_name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return nil
		}

		latest := strings.TrimPrefix(release.TagName, "v")
		current := strings.TrimPrefix(currentVersion, "v")

		if latest != "" && latest != current {
			return updateAvailableMsg{version: release.TagName}
		}

		return nil
	}
}
