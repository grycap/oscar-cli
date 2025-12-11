package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/config"
)

// Run launches the interactive terminal user interface.
func Run(ctx context.Context, conf *config.Config) error {
	if conf == nil {
		return errors.New("interactive mode requires a configuration")
	}
	if len(conf.Oscar) == 0 {
		return errors.New("no clusters configured")
	}

	app := tview.NewApplication()
	state := &uiState{
		app:                app,
		conf:               conf,
		rootCtx:            ctx,
		statusView:         tview.NewTextView().SetDynamicColors(true),
		detailsView:        tview.NewTextView().SetDynamicColors(true),
		detailContainer:    tview.NewFlex().SetDirection(tview.FlexRow),
		serviceTable:       tview.NewTable().SetSelectable(true, false),
		bucketObjectsTable: tview.NewTable().SetSelectable(true, false),
		clusterList:        tview.NewList().ShowSecondaryText(false),
		mutex:              &sync.Mutex{},
		currentCluster:     "",
		failedClusters:     make(map[string]string),
		mode:               modeServices,
		bucketObjects:      make(map[string]*bucketObjectState),
		serviceDefinitions: make(map[string]string),
		logDetails:         make(map[string]string),
	}

	state.statusView.SetBorder(false)
	state.detailsView.SetBorder(true)
	state.detailsView.SetScrollable(true)
	state.detailsView.SetTitle("Details")
	state.detailsView.SetText("Select a cluster to view details")
	state.bucketObjectsTable.SetBorder(true)
	state.bucketObjectsTable.SetTitle("Bucket Objects")
	state.bucketObjectsTable.SetFixed(1, 0)
	state.detailContainer.AddItem(state.detailsView, 0, 1, false)
	state.serviceTable.SetBorder(true)
	state.serviceTable.SetTitle("Services")
	state.serviceTable.SetFixed(1, 0)
	state.clusterList.SetBorder(true)
	state.clusterList.SetTitle("Clusters")

	state.statusContainer = tview.NewFlex().SetDirection(tview.FlexColumn)
	state.statusContainer.SetBorder(true)
	state.statusContainer.SetTitle("Status")
	state.statusContainer.AddItem(state.statusView, 0, 1, false)

	clusterNames := conf.ClusterIDs()
	state.clusterNames = clusterNames
	defaultCluster := conf.Default
	if defaultCluster == "" && len(clusterNames) > 0 {
		defaultCluster = clusterNames[0]
	}
	if defaultCluster != "" {
		state.pendingCluster = defaultCluster
	}

	for _, name := range clusterNames {
		name := name
		state.clusterList.AddItem(name, "", 0, func() {
			state.selectCluster(ctx, name)
		})
	}

	state.clusterList.SetChangedFunc(func(index int, mainText, secondary string, shortcut rune) {
		if index < 0 || index >= len(clusterNames) {
			return
		}
		state.selectCluster(ctx, clusterNames[index])
	})

	state.serviceTable.SetSelectionChangedFunc(func(row, column int) {
		state.handleSelection(row, false)
	})
	state.serviceTable.SetSelectedFunc(func(row, column int) {
		state.handleSelection(row, true)
	})
	state.serviceTable.SetFocusFunc(func() {
		if state.modeIsServices() {
			state.markServicePanelVisited()
		}
	})

	layout := tview.NewFlex().
		AddItem(state.clusterList, 0, 1, true).
		AddItem(state.serviceTable, 0, 3, false)

	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(layout, 0, 4, true).
		AddItem(state.detailContainer, 0, 3, false).
		AddItem(state.statusContainer, 4, 0, false)

	state.statusView.SetText(state.decorateStatusText(statusHelpText))

	pages := tview.NewPages()
	pages.AddPage("main", root, true, true)
	state.pages = pages

	app.SetRoot(pages, true)
	app.SetFocus(state.clusterList)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if state.searchVisible {
			if event.Key() == tcell.KeyEsc {
				state.hideSearch()
				return nil
			}
			return event
		}
		if state.autoRefreshPromptVisible {
			if event.Key() == tcell.KeyEsc {
				state.hideAutoRefreshPrompt()
				return nil
			}
			return event
		}

		switch event.Key() {
		case tcell.KeyTab:
			if app.GetFocus() == state.clusterList {
				if state.modeIsServices() {
					state.markServicePanelVisited()
				}
				app.SetFocus(state.serviceTable)
			} else if state.modeIsBuckets() && app.GetFocus() == state.serviceTable {
				state.focusBucketObjectsTable()
			} else {
				app.SetFocus(state.clusterList)
			}
			return nil
		case tcell.KeyRight:
			if app.GetFocus() == state.clusterList {
				if state.modeIsServices() {
					state.markServicePanelVisited()
				}
				app.SetFocus(state.serviceTable)
				return nil
			}
			if state.modeIsBuckets() && app.GetFocus() == state.serviceTable {
				state.focusBucketObjectsTable()
				return nil
			}
		case tcell.KeyLeft:
			if app.GetFocus() == state.serviceTable {
				app.SetFocus(state.clusterList)
				return nil
			}
			if app.GetFocus() == state.bucketObjectsTable {
				app.SetFocus(state.serviceTable)
				return nil
			}
		case tcell.KeyBacktab:
			if app.GetFocus() == state.serviceTable {
				app.SetFocus(state.clusterList)
				return nil
			}
			if app.GetFocus() == state.bucketObjectsTable {
				app.SetFocus(state.serviceTable)
				return nil
			}
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if app.GetFocus() == state.detailsView {
				app.SetFocus(state.serviceTable)
				return nil
			}
			if app.GetFocus() == state.serviceTable {
				state.switchToServices(ctx)
				return nil
			}
		case tcell.KeyEnter:
			if app.GetFocus() == state.serviceTable {
				state.switchToLogs(ctx)
				return nil
			}
		}

		switch event.Rune() {
		case 'q', 'Q':
			app.Stop()
			return nil
		case 'r':
			state.refreshCurrent(ctx)
			return nil
		case 'w', 'W':
			state.promptAutoRefresh()
			return nil
		case 'b', 'B':
			state.switchToBuckets(ctx)
			return nil
		case 's', 'S':
			state.switchToServices(ctx)
			return nil
		case 'o', 'O':
			if state.modeIsBuckets() {
				state.reloadBucketObjects(ctx)
				state.focusBucketObjectsTable()
				return nil
			}
		case 'n', 'N':
			if state.modeIsBuckets() {
				state.nextBucketObjectsPage(ctx)
				return nil
			}
		case 'p', 'P':
			if state.modeIsBuckets() {
				state.previousBucketObjectsPage(ctx)
				return nil
			}
		case 'a', 'A':
			if state.modeIsBuckets() {
				state.loadAllBucketObjects(ctx)
				return nil
			}
		case 'd', 'D':
			if app.GetFocus() == state.serviceTable && state.modeIsServices() {
				state.requestDeletion()
				return nil
			}
		case 'v', 'V':
			state.queueUpdate(func() {
				if state.app.GetFocus() != state.detailsView {
					state.app.SetFocus(state.detailsView)
				} else {
					state.app.SetFocus(state.serviceTable)
				}
			})
			return nil
		case 'l', 'L':
			if app.GetFocus() == state.serviceTable {
				state.switchToLogs(ctx)
				return nil
			}
		case '?':
			state.toggleLegend()
			return nil
		case 'i', 'I':
			state.showClusterInfo()
			return nil
		case 't', 'T':
			state.showClusterStatus()
			return nil
		case '/':
			state.initiateSearch(ctx)
			return nil
		}
		return event
	})

	go func() {
		<-ctx.Done()
		state.stopAutoRefresh()
		app.Stop()
	}()

	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		state.mutex.Lock()
		if state.started {
			state.mutex.Unlock()
			return false
		}
		state.started = true
		pending := state.pendingCluster
		state.pendingCluster = ""
		state.mutex.Unlock()
		if pending != "" {
			if idx := indexOf(clusterNames, pending); idx >= 0 {
				go state.triggerClusterSelection(idx)
			}
		}
		return false
	})

	if err := app.Run(); err != nil {
		state.stopAutoRefresh()
		return err
	}
	state.stopAutoRefresh()
	return nil
}

func (s *uiState) selectCluster(ctx context.Context, name string) {
	s.mutex.Lock()
	if name == s.currentCluster && s.refreshing && s.loadingCluster == name {
		s.mutex.Unlock()
		return
	}
	if s.loadCancel != nil {
		s.loadCancel()
		s.loadCancel = nil
		s.refreshing = false
		s.loadingCluster = ""
	}
	if s.bucketCancel != nil {
		s.bucketCancel()
		s.bucketCancel = nil
	}
	if s.bucketObjectsCancel != nil {
		s.bucketObjectsCancel()
		s.bucketObjectsCancel = nil
	}
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = ""
	s.currentBucketObjectsKey = ""
	s.logEntries = nil
	s.currentLogsKey = ""
	s.currentLogJobKey = ""
	s.currentLogService = ""
	s.currentLogCluster = ""
	if s.mode == modeLogs {
		s.mode = modeServices
	}
	s.currentCluster = name
	mode := s.mode
	errMsg, blocked := s.failedClusters[name]
	s.mutex.Unlock()

	s.showClusterDetails(name)

	if mode == modeBuckets {
		if name == "" {
			s.setStatus("[red]Select a cluster to view buckets")
			s.queueUpdate(func() {
				s.showBucketMessage("Select a cluster to view buckets")
			})
			return
		}
		s.queueUpdate(func() {
			s.showBucketMessage("Loading buckets…")
		})
		go s.loadBuckets(ctx, name, false)
		return
	}

	if name == "" {
		s.queueUpdate(func() {
			s.showServiceMessage("Select a cluster to view services")
		})
		return
	}

	if blocked {
		s.setStatus(fmt.Sprintf("[red]%s", errMsg))
		s.queueUpdate(func() {
			s.showServiceMessage("Unable to load services")
		})
		go s.loadServices(ctx, name, true)
		return
	}

	go s.loadServices(ctx, name, false)
}

func (s *uiState) refreshCurrent(ctx context.Context) {
	s.mutex.Lock()
	name := s.currentCluster
	mode := s.mode
	delete(s.failedClusters, name)
	s.mutex.Unlock()
	if name == "" {
		return
	}
	if mode == modeBuckets {
		go s.loadBuckets(ctx, name, true)
	} else if mode == modeLogs {
		go s.loadLogs(ctx, name, s.currentLogService, true)
	} else {
		go s.loadServices(ctx, name, true)
	}
}

func (s *uiState) showClusterDetails(name string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		s.queueUpdate(func() {
			s.detailsView.SetText("Select a cluster to view details")
		})
		return
	}

	cfg := s.conf.Oscar[trimmed]
	text := formatClusterConfig(trimmed, cfg)
	s.queueUpdate(func() {
		s.detailsView.SetText(text)
	})
}

func (s *uiState) modeIsServices() bool {
	s.mutex.Lock()
	mode := s.mode
	s.mutex.Unlock()
	return mode == modeServices
}

func (s *uiState) modeIsBuckets() bool {
	s.mutex.Lock()
	mode := s.mode
	s.mutex.Unlock()
	return mode == modeBuckets
}

func (s *uiState) modeIsLogs() bool {
	s.mutex.Lock()
	mode := s.mode
	s.mutex.Unlock()
	return mode == modeLogs
}

func (s *uiState) focusDetailsPane() {
	s.queueUpdate(func() {
		s.app.SetFocus(s.detailsView)
	})
}

func (s *uiState) setStatus(message string) {
	s.mutex.Lock()
	started := s.started
	s.mutex.Unlock()
	text := s.decorateStatusText(message)
	if !started {
		s.statusView.SetText(text)
		return
	}
	s.queueUpdate(func() {
		s.statusView.SetText(text)
	})
}

func indexOf(values []string, target string) int {
	for i, v := range values {
		if v == target {
			return i
		}
	}
	return -1
}

func (s *uiState) triggerClusterSelection(index int) {
	s.queueUpdate(func() {
		s.clusterList.SetCurrentItem(index)
	})
}

func (s *uiState) handleSelection(row int, immediate bool) {
	s.mutex.Lock()
	mode := s.mode
	s.mutex.Unlock()
	if mode == modeBuckets {
		s.handleBucketSelection(row, immediate)
		return
	}
	if mode == modeLogs {
		s.handleLogSelection(row, immediate)
		return
	}
	s.handleServiceSelection(row, immediate)
}

func (s *uiState) queueUpdate(fn func()) {
	s.mutex.Lock()
	started := s.started
	s.mutex.Unlock()
	if !started {
		fn()
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// queueing can fail if the application has already stopped; ignore.
			}
		}()
		s.app.QueueUpdateDraw(fn)
	}()
}

func (s *uiState) showClusterInfo() {
	s.mutex.Lock()
	if s.confirmVisible || s.legendVisible {
		s.mutex.Unlock()
		return
	}
	clusterName := s.currentCluster
	s.mutex.Unlock()

	trimmedName := strings.TrimSpace(clusterName)
	if trimmedName == "" {
		s.setStatus("[red]Select a cluster to view its info")
		return
	}

	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil && trimmedName != clusterName {
		clusterCfg = s.conf.Oscar[trimmedName]
	}
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", trimmedName))
		return
	}

	displayName := trimmedName
	if displayName == "" {
		displayName = clusterName
	}

	s.setStatus(fmt.Sprintf("[yellow]Loading info for cluster %q…", displayName))

	go func(name string, cfg *cluster.Cluster) {
		info, err := cfg.GetClusterInfo()
		if err != nil {
			s.setStatus(fmt.Sprintf("[red]Failed to load info for %q: %v", name, err))
			return
		}
		s.setStatus(fmt.Sprintf("[green]Cluster info loaded for %q", name))
		text := formatClusterInfo(name, info)
		s.queueUpdate(func() {
			s.detailsView.SetText(text)
		})
	}(displayName, clusterCfg)
}

func (s *uiState) showClusterStatus() {
	s.mutex.Lock()
	if s.confirmVisible || s.legendVisible {
		s.mutex.Unlock()
		return
	}
	clusterName := s.currentCluster
	s.mutex.Unlock()

	trimmedName := strings.TrimSpace(clusterName)
	if trimmedName == "" {
		s.setStatus("[red]Select a cluster to view its status")
		return
	}

	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil && trimmedName != clusterName {
		clusterCfg = s.conf.Oscar[trimmedName]
	}
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", trimmedName))
		return
	}

	displayName := trimmedName
	if displayName == "" {
		displayName = clusterName
	}

	s.setStatus(fmt.Sprintf("[yellow]Loading status for cluster %q…", displayName))

	go func(name string, cfg *cluster.Cluster) {
		status, err := cfg.GetClusterStatus()
		if err != nil {
			s.setStatus(fmt.Sprintf("[red]Failed to load status for %q: %v", name, err))
			return
		}
		s.setStatus(fmt.Sprintf("[green]Cluster status loaded for %q", name))
		text := formatClusterStatus(name, status)
		s.queueUpdate(func() {
			s.detailsView.SetText(text)
		})
	}(displayName, clusterCfg)
}

func formatClusterStatus(clusterName string, status cluster.StatusInfo) string {
	builder := &strings.Builder{}
	if clusterName != "" {
		fmt.Fprintf(builder, "[yellow]Cluster:[-] %s\n", clusterName)
	}
	if count := status.Cluster.NodesCount; count > 0 {
		fmt.Fprintf(builder, "[yellow]Nodes:[-] %d total\n", count)
	}

	clusterMetrics := status.Cluster.Metrics
	if clusterMetrics.CPU.TotalFreeCores > 0 || clusterMetrics.CPU.MaxFreeOnNodeCores > 0 {
		fmt.Fprintf(builder, "[yellow]CPU:[-] free %d cores (max node %d)\n",
			clusterMetrics.CPU.TotalFreeCores, clusterMetrics.CPU.MaxFreeOnNodeCores)
	}
	if clusterMetrics.Memory.TotalFreeBytes > 0 || clusterMetrics.Memory.MaxFreeOnNodeBytes > 0 {
		fmt.Fprintf(builder, "[yellow]Memory:[-] free %s (max node %s)\n",
			humanizeBytes(clusterMetrics.Memory.TotalFreeBytes),
			humanizeBytes(clusterMetrics.Memory.MaxFreeOnNodeBytes))
	}
	if clusterMetrics.GPU.TotalGPU > 0 {
		fmt.Fprintf(builder, "[yellow]GPU:[-] %d available\n", clusterMetrics.GPU.TotalGPU)
	}

	if len(status.Cluster.Nodes) > 0 {
		builder.WriteString("[yellow]Node details:[-]\n")
		for _, node := range status.Cluster.Nodes {
			name := defaultIfEmpty(node.Name, "node")
			statusText := defaultIfEmpty(node.Status, "unknown")
			role := ""
			if node.IsInterlink {
				role = " (interlink)"
			}
			fmt.Fprintf(builder, "  - %s (%s)%s\n", name, statusText, role)
			if node.CPU.CapacityCores > 0 || node.CPU.UsageCores > 0 {
				fmt.Fprintf(builder, "      CPU: %d/%d cores used\n", node.CPU.UsageCores, node.CPU.CapacityCores)
			}
			if node.Memory.CapacityBytes > 0 || node.Memory.UsageBytes > 0 {
				fmt.Fprintf(builder, "      Memory: %s/%s\n",
					humanizeBytes(node.Memory.UsageBytes), humanizeBytes(node.Memory.CapacityBytes))
			}
			if node.GPU > 0 {
				fmt.Fprintf(builder, "      GPU: %d\n", node.GPU)
			}
			if len(node.Conditions) > 0 {
				conditions := make([]string, 0, len(node.Conditions))
				for _, cond := range node.Conditions {
					colorTag := "[green]"
					if cond.Status {
						colorTag = "[red]"
					}
					if strings.EqualFold(cond.Type, "Ready") && cond.Status {
						colorTag = "[green]"
					}
					conditions = append(conditions, fmt.Sprintf("%s%s=%t[-]", colorTag, cond.Type, cond.Status))
				}
				fmt.Fprintf(builder, "      Conditions: %s\n", strings.Join(conditions, ", "))
			}
		}
	}

	oscar := status.Oscar
	if oscar.DeploymentName != "" || oscar.JobsCount > 0 || oscar.Ready {
		state := "not ready"
		if oscar.Ready {
			state = "ready"
		}
		fmt.Fprintf(builder, "[yellow]OSCAR:[-] %s (%s)\n", defaultIfEmpty(oscar.DeploymentName, "manager"), state)
		deployment := oscar.Deployment
		if deployment.Replicas > 0 || deployment.ReadyReplicas > 0 || deployment.AvailableReplicas > 0 {
			fmt.Fprintf(builder, "    Replicas: %d total / %d ready / %d available\n",
				deployment.Replicas, deployment.ReadyReplicas, deployment.AvailableReplicas)
		}
		if !deployment.CreationTimestamp.IsZero() {
			fmt.Fprintf(builder, "    Created: %s\n", deployment.CreationTimestamp.UTC().Format(time.RFC3339))
		}
		if deployment.Strategy != "" {
			fmt.Fprintf(builder, "    Strategy: %s\n", deployment.Strategy)
		}
		if oscar.JobsCount > 0 {
			fmt.Fprintf(builder, "    Jobs: %d total\n", oscar.JobsCount)
		}
		if oscar.Pods.Total > 0 {
			fmt.Fprintf(builder, "    Pods: %d total", oscar.Pods.Total)
			if len(oscar.Pods.States) > 0 {
				stateKeys := make([]string, 0, len(oscar.Pods.States))
				for key := range oscar.Pods.States {
					stateKeys = append(stateKeys, key)
				}
				sort.Strings(stateKeys)
				parts := make([]string, 0, len(stateKeys))
				for _, key := range stateKeys {
					parts = append(parts, fmt.Sprintf("%s=%d", key, oscar.Pods.States[key]))
				}
				fmt.Fprintf(builder, " (%s)", strings.Join(parts, ", "))
			}
			builder.WriteByte('\n')
		}
		if oscar.OIDC.Enabled || len(oscar.OIDC.Issuers) > 0 || len(oscar.OIDC.Groups) > 0 {
			fmt.Fprintf(builder, "    OIDC: enabled=%t\n", oscar.OIDC.Enabled)
			if len(oscar.OIDC.Issuers) > 0 {
				fmt.Fprintf(builder, "          issuers: %s\n", strings.Join(oscar.OIDC.Issuers, ", "))
			}
			if len(oscar.OIDC.Groups) > 0 {
				fmt.Fprintf(builder, "          groups: %s\n", strings.Join(oscar.OIDC.Groups, ", "))
			}
		}
	}

	minio := status.MinIO
	if minio.BucketsCount > 0 || minio.TotalObjects > 0 {
		fmt.Fprintf(builder, "[yellow]MinIO:[-] %d bucket(s), %d object(s)\n", minio.BucketsCount, minio.TotalObjects)
	}

	out := strings.TrimRight(builder.String(), "\n")
	if out == "" {
		return "No cluster status available"
	}
	return out
}

func humanizeBytes(value int64) string {
	if value <= 0 {
		return "0 B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	f := float64(value)
	idx := 0
	for f >= 1024 && idx < len(units)-1 {
		f /= 1024
		idx++
	}
	if f >= 10 || idx == 0 {
		return fmt.Sprintf("%.0f %s", f, units[idx])
	}
	return fmt.Sprintf("%.1f %s", f, units[idx])
}
