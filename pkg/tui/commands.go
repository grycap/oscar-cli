package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type commandEntry struct {
	desc    string
	handler func(args []string)
}

var commandRegistry = map[string]commandEntry{}

func registerCommand(name, desc string, handler func(args []string)) {
	commandRegistry[name] = commandEntry{desc: desc, handler: handler}
}

func init() {
	// Commands are registered via init functions in each file
}

func (s *uiState) initCommands() {
	registerCommand("help", "Show this help", func(args []string) {
		s.toggleLegend()
	})
	registerCommand("refresh", "Refresh current view", func(args []string) {
		s.refreshCurrent(s.rootCtx)
	})
	registerCommand("cluster.info", "Show cluster info", func(args []string) {
		s.showClusterInfo()
	})
	registerCommand("cluster.status", "Show cluster status", func(args []string) {
		s.showClusterStatus()
	})
	registerCommand("cluster.metrics", "Show cluster metrics", func(args []string) {
		s.showMetricsSummary()
	})
	registerCommand("quota.get", "Get quota for a user [user]", func(args []string) {
		s.showQuotaPrompt()
	})
	registerCommand("quota.update", "Update quota: [user] [cpu] [mem] [vol-disk] [vol-count]", func(args []string) {
		if len(args) < 1 {
			s.promptUpdateQuota()
			return
		}
		userID := args[0]
		cpu := argOr(args, 1, "")
		mem := argOr(args, 2, "")
		volDisk := argOr(args, 3, "")
		volCount := argOr(args, 4, "")

		s.mutex.Lock()
		clusterCfg := s.conf.Oscar[s.currentCluster]
		s.mutex.Unlock()
		if clusterCfg == nil {
			s.setStatus("[red]No cluster selected or cluster not found")
			return
		}
		if clusterCfg.AuthUser == "" || clusterCfg.AuthPassword == "" {
			s.setStatus("[red]Quota update is only available for clusters with auth_user/auth_password")
			return
		}
		s.performUpdateQuota(clusterCfg, userID, cpu, mem, volDisk, volCount)
	})
	registerCommand("deploy.status", "Show deployment status <service>", func(args []string) {
		s.showServiceDeploymentStatus(s.rootCtx)
	})
	registerCommand("deploy.logs", "Show deployment logs <service>", func(args []string) {
		s.showServiceDeploymentLogs(s.rootCtx)
	})
	registerCommand("search", "Search <query>", func(args []string) {
		s.initiateSearch(s.rootCtx)
	})
	registerCommand("services", "Switch to services view", func(args []string) {
		s.switchToServices(s.rootCtx)
	})
	registerCommand("volumes", "Switch to volumes view", func(args []string) {
		s.switchToVolumes(s.rootCtx)
	})
	registerCommand("volume.create", "Create a volume", func(args []string) {
		s.mutex.Lock()
		if s.mode != modeVolumes {
			s.mutex.Unlock()
			s.switchToVolumes(s.rootCtx)
		} else {
			s.mutex.Unlock()
		}
		s.promptCreateVolume()
	})

	registerCommand("buckets", "Switch to buckets view", func(args []string) {
		s.switchToBuckets(s.rootCtx)
	})
	registerCommand("bucket.create", "Create a bucket", func(args []string) {
		s.mutex.Lock()
		if s.mode != modeBuckets {
			s.mutex.Unlock()
			s.switchToBuckets(s.rootCtx)
		} else {
			s.mutex.Unlock()
		}
		s.promptCreateBucket()
	})
}

func argOr(args []string, idx int, def string) string {
	if idx < len(args) {
		return args[idx]
	}
	return def
}

func (s *uiState) showCommandPalette() {
	s.mutex.Lock()
	if s.commandPaletteVisible || s.searchVisible || s.autoRefreshPromptVisible ||
		s.quotaPromptVisible || s.updateQuotaPromptVisible ||
		s.confirmVisible || s.legendVisible || s.pages == nil {
		s.mutex.Unlock()
		return
	}
	s.commandPaletteVisible = true
	s.commandPaletteFocus = s.app.GetFocus()
	container := s.statusContainer
	s.mutex.Unlock()

	s.setStatus("[yellow]Enter command (Tab for suggestions):[-]")
	input := tview.NewInputField()
	input.SetLabel(": ")
	input.SetFieldWidth(50)
		input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if (event.Key() == tcell.KeyBackspace || event.Key() == tcell.KeyBackspace2) && input.GetText() == "" {
			s.hideCommandPalette()
			return nil
		}
		return event
	})
	input.SetAutocompleteFunc(func(current string) []string {
		current = strings.TrimSpace(current)
		var suggestions []string
		if current == "" {
			suggestions = append(suggestions, "")
		}
		for name := range commandRegistry {
			if current == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(current)) {
				displayName := strings.ReplaceAll(name, ".", " ")
				suggestions = append(suggestions, displayName)
			}
		}
		sort.Strings(suggestions)
		return suggestions
	})
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			text := strings.TrimSpace(input.GetText())
			s.hideCommandPalette()
			if text != "" {
				s.executeCommand(text)
			}
		} else if key == tcell.KeyEscape {
			s.hideCommandPalette()
		}
	})

	s.queueUpdate(func() {
		container.Clear()
		container.SetTitle("Command Palette")
		input.SetBorder(false)
		container.AddItem(input, 0, 1, true)
	})
	s.app.SetFocus(input)
}

func (s *uiState) hideCommandPalette() {
	s.mutex.Lock()
	if !s.commandPaletteVisible {
		s.mutex.Unlock()
		return
	}
	s.commandPaletteVisible = false
	focus := s.commandPaletteFocus
	s.commandPaletteFocus = nil
	container := s.statusContainer
	s.mutex.Unlock()

	s.queueUpdate(func() {
		container.Clear()
		container.SetTitle("Status")
		container.AddItem(s.statusView, 0, 1, false)
		s.statusView.SetText(s.decorateStatusText(statusHelpText))
	})
	if focus != nil {
		s.app.SetFocus(focus)
	}
}

func (s *uiState) executeCommand(input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	// Normalize: replace spaces with dots for matching (user types "quota get", registry has "quota.get")
	normalized := strings.ToLower(strings.ReplaceAll(input, " ", "."))

	var bestMatch string
	var bestEntry commandEntry
	bestLen := 0

	for name, entry := range commandRegistry {
		lowerName := strings.ToLower(name)
		if strings.HasPrefix(normalized, lowerName) && len(lowerName) > bestLen {
			bestMatch = name
			bestEntry = entry
			bestLen = len(lowerName)
		}
	}

	if bestLen > 0 {
		cmdParts := strings.Split(bestMatch, ".")
		args := parts[len(cmdParts):]
		bestEntry.handler(args)
		return
	}

	s.setStatus(fmt.Sprintf("[red]Unknown command: %q. Type 'help' for available commands", parts[0]))
}
