package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

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
			state.focusDetailsPane()
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
