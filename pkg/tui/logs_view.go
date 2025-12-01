package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v3/pkg/types"
)

type logEntry struct {
	Name string
	Info *types.JobInfo
}

func (s *uiState) switchToLogs(ctx context.Context) {
	if s.searchVisible {
		s.hideSearch()
	}

	s.mutex.Lock()
	if s.confirmVisible || s.legendVisible {
		s.mutex.Unlock()
		return
	}
	if s.mode == modeBuckets {
		s.mutex.Unlock()
		s.setStatus("[red]Logs are only available in services view")
		return
	}

	row, _ := s.serviceTable.GetSelection()
	if row <= 0 || row-1 >= len(s.currentServices) {
		s.mutex.Unlock()
		s.setStatus("[red]Select a service to view logs")
		return
	}
	svcPtr := s.currentServices[row-1]
	clusterName := s.currentCluster
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = ""
	s.mutex.Unlock()

	if svcPtr == nil {
		s.setStatus("[red]Select a service to view logs")
		return
	}
	serviceName := strings.TrimSpace(svcPtr.Name)
	if serviceName == "" {
		s.setStatus("[red]Select a service to view logs")
		return
	}

	clusterName = strings.TrimSpace(clusterName)
	if clusterName == "" {
		s.setStatus("[red]Select a cluster to view logs")
		return
	}

	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		return
	}

	key := makeLogKey(clusterName, serviceName)
	s.mutex.Lock()
	s.mode = modeLogs
	s.currentLogsKey = key
	s.currentLogService = serviceName
	s.currentLogCluster = clusterName
	s.logEntries = nil
	s.currentLogJobKey = ""
	s.mutex.Unlock()

	s.setStatus(fmt.Sprintf("[yellow]Loading logs for %q…", serviceName))
	s.queueUpdate(func() {
		s.showLogMessage(serviceName, "Loading logs…")
		s.detailsView.SetText("Select a log entry to view contents")
	})

	parent := ctx
	if parent == nil {
		parent = context.Background()
	}
	go s.loadLogs(parent, clusterName, serviceName, false)
}

func (s *uiState) loadLogs(ctx context.Context, clusterName, serviceName string, force bool) {
	if strings.TrimSpace(clusterName) == "" || strings.TrimSpace(serviceName) == "" {
		return
	}

	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		s.queueUpdate(func() {
			s.showLogMessage(serviceName, "Cluster not found")
		})
		return
	}

	key := makeLogKey(clusterName, serviceName)

	s.mutex.Lock()
	cachedKey := s.currentLogsKey
	cachedEntries := append([]*logEntry(nil), s.logEntries...)
	if !force && cachedKey == key && len(cachedEntries) > 0 {
		s.mutex.Unlock()
		if s.modeIsLogs() {
			s.renderLogTable(cachedEntries, serviceName)
		}
		return
	}
	s.logSeq++
	seq := s.logSeq
	s.currentLogsKey = key
	s.currentLogService = serviceName
	s.currentLogCluster = clusterName
	s.logEntries = nil
	s.mutex.Unlock()

	s.setStatus(fmt.Sprintf("[yellow]Loading logs for %q…", serviceName))
	s.queueUpdate(func() {
		s.showLogMessage(serviceName, "Loading logs…")
	})

	page := ""
	entries := map[string]*types.JobInfo{}

	for {
		select {
		case <-ctx.Done():
			s.setStatus("[red]Log loading cancelled")
			return
		default:
		}

		resp, err := service.ListLogs(clusterCfg, serviceName, page)
		if err != nil {
			s.setStatus(fmt.Sprintf("[red]Unable to load logs for %q: %v", serviceName, err))
			s.queueUpdate(func() {
				s.showLogMessage(serviceName, "Unable to load logs")
			})
			return
		}

		for name, info := range resp.Jobs {
			entries[name] = info
		}

		if strings.TrimSpace(resp.NextPage) == "" {
			break
		}
		page = resp.NextPage
	}

	list := make([]*logEntry, 0, len(entries))
	for name, info := range entries {
		list = append(list, &logEntry{
			Name: name,
			Info: info,
		})
	}

	sort.Slice(list, func(i, j int) bool {
		ti := jobTimestamp(list[i].Info)
		tj := jobTimestamp(list[j].Info)
		switch {
		case ti.IsZero() && tj.IsZero():
			return list[i].Name > list[j].Name
		case ti.IsZero():
			return false
		case tj.IsZero():
			return true
		case ti.Equal(tj):
			return list[i].Name > list[j].Name
		default:
			return ti.After(tj)
		}
	})

	s.mutex.Lock()
	if seq != s.logSeq || s.currentLogsKey != key {
		s.mutex.Unlock()
		return
	}
	s.logEntries = list
	mode := s.mode
	currentCluster := s.currentCluster
	currentService := s.currentLogService
	s.mutex.Unlock()

	if mode == modeLogs && currentCluster == clusterName && currentService == serviceName {
		s.renderLogTable(list, serviceName)
		s.setStatus(fmt.Sprintf("[green]Loaded %d log(s) for %q", len(list), serviceName))
	}
}

func (s *uiState) renderLogTable(entries []*logEntry, serviceName string) {
	s.queueUpdate(func() {
		s.serviceTable.SetTitle(fmt.Sprintf("Logs: %s", serviceName))
		setLogTableHeader(s.serviceTable)
		if len(entries) == 0 {
			fillMessageRow(s.serviceTable, len(logHeaders), "No logs found")
			s.detailsView.SetText("No logs available")
			return
		}
		for idx, entry := range entries {
			row := idx + 1
			status := "-"
			started := "-"
			finished := "-"
			if entry.Info != nil {
				status = defaultIfEmpty(entry.Info.Status, "-")
				started = formatLogTime(jobStartTime(entry.Info))
				finished = formatLogTime(jobFinishTime(entry.Info))
			}
			s.serviceTable.SetCell(row, 0, tview.NewTableCell(entry.Name).
				SetSelectable(true).
				SetExpansion(4)).
				SetCell(row, 1, tview.NewTableCell(status).
					SetExpansion(2)).
				SetCell(row, 2, tview.NewTableCell(started).
					SetExpansion(3)).
				SetCell(row, 3, tview.NewTableCell(finished).
					SetExpansion(3))
		}
		row, col := s.serviceTable.GetSelection()
		if row <= 0 || row > len(entries) {
			s.serviceTable.Select(1, 0)
		} else {
			s.serviceTable.Select(row, col)
		}
	})
}

func (s *uiState) showLogMessage(serviceName, message string) {
	s.serviceTable.SetTitle(fmt.Sprintf("Logs: %s", serviceName))
	setLogTableHeader(s.serviceTable)
	fillMessageRow(s.serviceTable, len(logHeaders), message)
}

func (s *uiState) handleLogSelection(row int, immediate bool) {
	s.mutex.Lock()
	if s.mode != modeLogs {
		if s.detailTimer != nil {
			s.detailTimer.Stop()
			s.detailTimer = nil
		}
		s.mutex.Unlock()
		return
	}
	entries := append([]*logEntry(nil), s.logEntries...)
	clusterName := s.currentLogCluster
	serviceName := s.currentLogService
	seq := s.logSeq
	if row <= 0 || row-1 >= len(entries) {
		if s.detailTimer != nil {
			s.detailTimer.Stop()
			s.detailTimer = nil
		}
		s.lastSelection = ""
		s.mutex.Unlock()
		s.queueUpdate(func() {
			s.detailsView.SetText("Select a log entry to view contents")
		})
		return
	}
	entry := entries[row-1]
	token := fmt.Sprintf("log:%s:%d:%d", entry.Name, row, seq)
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = token
	s.mutex.Unlock()

	if immediate {
		s.showLogDetails(clusterName, serviceName, entry.Name)
		return
	}

	timer := time.AfterFunc(500*time.Millisecond, func() {
		s.mutex.Lock()
		if s.lastSelection != token {
			s.mutex.Unlock()
			return
		}
		s.detailTimer = nil
		s.mutex.Unlock()
		s.showLogDetails(clusterName, serviceName, entry.Name)
	})

	s.mutex.Lock()
	if s.lastSelection == token {
		s.detailTimer = timer
	} else {
		timer.Stop()
	}
	s.mutex.Unlock()
}

func (s *uiState) showLogDetails(clusterName, serviceName, jobName string) {
	clusterName = strings.TrimSpace(clusterName)
	serviceName = strings.TrimSpace(serviceName)
	jobName = strings.TrimSpace(jobName)
	if clusterName == "" || serviceName == "" || jobName == "" {
		s.queueUpdate(func() {
			s.detailsView.SetText("Select a log entry to view contents")
		})
		return
	}

	key := makeLogDetailKey(clusterName, serviceName, jobName)
	s.mutex.Lock()
	cached := s.logDetails[key]
	s.logDetailSeq++
	seq := s.logDetailSeq
	s.currentLogJobKey = key
	s.mutex.Unlock()

	if cached != "" {
		s.queueUpdate(func() {
			s.detailsView.SetText(cached)
		})
		return
	}

	s.queueUpdate(func() {
		s.detailsView.SetText(fmt.Sprintf("Loading log %s…", jobName))
	})
	s.setStatus(fmt.Sprintf("[yellow]Loading log %q…", jobName))

	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		return
	}

	go func(seq int, key string) {
		logText, err := service.GetLogs(clusterCfg, serviceName, jobName, false)
		if err != nil {
			s.setStatus(fmt.Sprintf("[red]Failed to load log %q: %v", jobName, err))
			return
		}

		rendered := formatServiceLogs(serviceName, jobName, logText)
		s.mutex.Lock()
		if seq != s.logDetailSeq {
			s.mutex.Unlock()
			return
		}
		s.logDetails[key] = rendered
		active := s.currentLogJobKey == key
		s.mutex.Unlock()

		if active {
			s.queueUpdate(func() {
				s.detailsView.SetText(rendered)
			})
			s.setStatus(fmt.Sprintf("[green]Loaded log %q", jobName))
		}
	}(seq, key)
}

func jobTimestamp(info *types.JobInfo) time.Time {
	if info == nil {
		return time.Time{}
	}
	if info.CreationTime != nil {
		return info.CreationTime.Time
	}
	if info.StartTime != nil {
		return info.StartTime.Time
	}
	if info.FinishTime != nil {
		return info.FinishTime.Time
	}
	return time.Time{}
}

func jobStartTime(info *types.JobInfo) time.Time {
	if info == nil {
		return time.Time{}
	}
	if info.StartTime != nil {
		return info.StartTime.Time
	}
	if info.CreationTime != nil {
		return info.CreationTime.Time
	}
	return time.Time{}
}

func jobFinishTime(info *types.JobInfo) time.Time {
	if info == nil || info.FinishTime == nil {
		return time.Time{}
	}
	return info.FinishTime.Time
}

func formatLogTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func makeLogKey(clusterName, serviceName string) string {
	return fmt.Sprintf("%s\x00%s", clusterName, serviceName)
}

func makeLogDetailKey(clusterName, serviceName, jobName string) string {
	return fmt.Sprintf("%s\x00%s\x00%s", clusterName, serviceName, jobName)
}
