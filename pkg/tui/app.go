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
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/grycap/oscar/v3/pkg/types"
)

const legendText = `[yellow]Navigation[-]
  ↑/↓  Move selection
  ←/→ or Tab  Switch pane

[yellow]Actions[-]
  r  Refresh current view
  d  Delete selected item
  i  Show cluster info
  b  Switch to buckets view
  s  Switch to services view
  q  Quit
  ?  Toggle this help`

type panelMode int

const (
	modeServices panelMode = iota
	modeBuckets
)

var (
	serviceHeaders = []string{"Name", "Image", "CPU", "Memory"}
	bucketHeaders  = []string{"Name", "Visibility", "Owner"}
)

const statusHelpText = "[yellow]Keys: [::b]q[::-] Quit · [::b]r[::-] Refresh · [::b]d[::-] Delete selection · [::b]i[::-] Cluster info · [::b]b[::-] Buckets · [::b]s[::-] Services · [::b]?[::-] Help · [::b]←/→[::-] Switch pane · [::b]/[::-] Search"

type searchTarget int

const (
	searchTargetNone searchTarget = iota
	searchTargetClusters
	searchTargetServices
	searchTargetBuckets
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
		app:            app,
		conf:           conf,
		statusView:     tview.NewTextView().SetDynamicColors(true),
		detailsView:    tview.NewTextView().SetDynamicColors(true),
		serviceTable:   tview.NewTable().SetSelectable(true, false),
		clusterList:    tview.NewList().ShowSecondaryText(false),
		mutex:          &sync.Mutex{},
		currentCluster: "",
		failedClusters: make(map[string]string),
		mode:           modeServices,
	}

	state.statusView.SetBorder(false)
	state.detailsView.SetBorder(true)
	state.detailsView.SetTitle("Details")
	state.detailsView.SetText("Select a service to inspect details")
	state.serviceTable.SetBorder(true)
	state.serviceTable.SetTitle("Services")
	state.serviceTable.SetFixed(1, 0)
	state.clusterList.SetBorder(true)
	state.clusterList.SetTitle("Clusters")

	state.statusContainer = tview.NewFlex().SetDirection(tview.FlexColumn)
	state.statusContainer.SetBorder(true)
	state.statusContainer.SetTitle("Status")
	state.statusContainer.AddItem(state.statusView, 0, 1, false)

	clusterNames := sortedClusters(conf.Oscar)
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

	layout := tview.NewFlex().
		AddItem(state.clusterList, 0, 1, true).
		AddItem(state.serviceTable, 0, 3, false)

	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(layout, 0, 1, true).
		AddItem(state.detailsView, 12, 1, false).
		AddItem(state.statusContainer, 3, 1, false)

	state.statusView.SetText(statusHelpText)

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

		switch event.Key() {
		case tcell.KeyTab:
			if app.GetFocus() == state.clusterList {
				app.SetFocus(state.serviceTable)
			} else {
				app.SetFocus(state.clusterList)
			}
			return nil
		case tcell.KeyRight:
			if app.GetFocus() == state.clusterList {
				app.SetFocus(state.serviceTable)
				return nil
			}
		case tcell.KeyLeft:
			if app.GetFocus() == state.serviceTable {
				app.SetFocus(state.clusterList)
				return nil
			}
		case tcell.KeyBacktab:
			if app.GetFocus() == state.serviceTable {
				app.SetFocus(state.clusterList)
				return nil
			}
		}

		switch event.Rune() {
		case 'q', 'Q':
			app.Stop()
			return nil
		case 'r', 'R':
			state.refreshCurrent(ctx)
			return nil
		case 'b', 'B':
			state.switchToBuckets(ctx)
			return nil
		case 's', 'S':
			state.switchToServices(ctx)
			return nil
		case 'd', 'D':
			if app.GetFocus() == state.serviceTable {
				state.requestDeletion()
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
		return err
	}
	return nil
}

type uiState struct {
	app             *tview.Application
	conf            *config.Config
	statusView      *tview.TextView
	detailsView     *tview.TextView
	serviceTable    *tview.Table
	clusterList     *tview.List
	statusContainer *tview.Flex
	pages           *tview.Pages
	mutex           *sync.Mutex

	clusterNames    []string
	currentCluster  string
	currentServices []*types.Service
	refreshing      bool
	started         bool
	pendingCluster  string
	loadingCluster  string
	failedClusters  map[string]string
	loadCancel      context.CancelFunc
	loadSeq         int
	detailTimer     *time.Timer
	lastSelection   string
	legendVisible   bool
	confirmVisible  bool
	savedFocus      tview.Primitive
	mode            panelMode
	bucketInfos     []*storage.BucketInfo
	bucketCancel    context.CancelFunc
	bucketSeq       int
	bucketCluster   string
	searchVisible   bool
	searchInput     *tview.InputField
	searchTarget    searchTarget
	originalFocus   tview.Primitive
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
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = ""
	s.currentCluster = name
	mode := s.mode
	errMsg, blocked := s.failedClusters[name]
	s.mutex.Unlock()

	if mode == modeBuckets {
		if name == "" {
			s.setStatus("[red]Select a cluster to view buckets")
			s.queueUpdate(func() {
				s.showBucketMessage("Select a cluster to view buckets")
				s.detailsView.SetText("Select a bucket to inspect details")
			})
			return
		}
		s.queueUpdate(func() {
			s.showBucketMessage("Loading buckets…")
			s.detailsView.SetText("Select a bucket to inspect details")
		})
		go s.loadBuckets(ctx, name, false)
		return
	}

	if name == "" {
		s.queueUpdate(func() {
			s.showServiceMessage("Select a cluster to view services")
			s.detailsView.SetText("Select a service to inspect details")
		})
		return
	}

	if blocked {
		s.setStatus(fmt.Sprintf("[red]%s", errMsg))
		s.queueUpdate(func() {
			s.showServiceMessage("Unable to load services")
			s.detailsView.SetText("Select a service to inspect details")
		})
		go s.loadServices(ctx, name, true)
		return
	}

	s.queueUpdate(func() {
		s.detailsView.SetText("Select a service to inspect details")
	})

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
	} else {
		go s.loadServices(ctx, name, true)
	}
}

func (s *uiState) switchToBuckets(ctx context.Context) {
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
		return
	}
	s.mode = modeBuckets
	if s.loadCancel != nil {
		s.loadCancel()
		s.loadCancel = nil
		s.refreshing = false
		s.loadingCluster = ""
	}
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = ""
	s.mutex.Unlock()

	clusterName := s.currentCluster
	if clusterName == "" {
		s.setStatus("[red]Select a cluster to view buckets")
		s.queueUpdate(func() {
			s.showBucketMessage("Select a cluster to view buckets")
			s.detailsView.SetText("Select a bucket to inspect details")
		})
		return
	}

	s.queueUpdate(func() {
		s.showBucketMessage("Loading buckets…")
		s.detailsView.SetText("Select a bucket to inspect details")
	})

	s.mutex.Lock()
	cached := s.bucketInfos
	cachedCluster := s.bucketCluster
	s.mutex.Unlock()
	if len(cached) > 0 && cachedCluster == clusterName {
		s.renderBucketTable(cached)
		s.setStatus(fmt.Sprintf("[green]Loaded %d bucket(s) for %s", len(cached), clusterName))
		return
	}

	go s.loadBuckets(ctx, clusterName, false)
}

func (s *uiState) switchToServices(ctx context.Context) {
	if s.searchVisible {
		s.hideSearch()
	}
	s.mutex.Lock()
	if s.confirmVisible || s.legendVisible {
		s.mutex.Unlock()
		return
	}
	if s.mode == modeServices {
		s.mutex.Unlock()
		return
	}
	s.mode = modeServices
	if s.bucketCancel != nil {
		s.bucketCancel()
		s.bucketCancel = nil
	}
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = ""
	services := s.currentServices
	clusterName := s.currentCluster
	s.mutex.Unlock()

	s.queueUpdate(func() {
		s.detailsView.SetText("Select a service to inspect details")
	})

	if len(services) > 0 {
		s.renderServiceTable(services)
		s.setStatus(fmt.Sprintf("[green]Loaded %d service(s) for %s", len(services), clusterName))
		return
	}

	if clusterName == "" {
		s.queueUpdate(func() {
			s.showServiceMessage("Select a cluster to view services")
		})
		return
	}

	s.queueUpdate(func() {
		s.showServiceMessage("Loading…")
	})
	go s.loadServices(ctx, clusterName, true)
}

func (s *uiState) loadServices(ctx context.Context, name string, force bool) {
	if name == "" {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			s.mutex.Lock()
			s.refreshing = false
			s.loadingCluster = ""
			s.mutex.Unlock()
			s.setStatus(fmt.Sprintf("[red]Unexpected error while loading services for %s: %v", name, r))
		}
	}()

	cfg, ok := s.conf.Oscar[name]
	if !ok || cfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q not found in configuration", name))
		s.mutex.Lock()
		s.refreshing = false
		s.loadingCluster = ""
		s.currentServices = nil
		s.failedClusters[name] = fmt.Sprintf("Cluster %q not found in configuration", name)
		s.mutex.Unlock()
		s.queueUpdate(func() {
			s.showServiceMessage("Cluster not found")
		})
		return
	}

	s.setStatus(fmt.Sprintf("[yellow]Loading services for cluster %s…", name))
	s.queueUpdate(func() {
		s.showServiceMessage("Loading…")
	})

	s.mutex.Lock()
	if s.refreshing && !force && s.loadingCluster == name {
		s.mutex.Unlock()
		return
	}
	if s.loadCancel != nil {
		s.loadCancel()
		s.loadCancel = nil
	}
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = ""
	s.loadSeq++
	loadVersion := s.loadSeq
	ctxFetch, cancel := context.WithTimeout(ctx, 15*time.Second)
	s.refreshing = true
	s.loadingCluster = name
	s.loadCancel = cancel
	s.mutex.Unlock()

	servicesList, err := service.ListServicesWithContext(ctxFetch, cfg)
	if err != nil {
		message := fmt.Sprintf("Unable to load services for %s: %v", name, err)
		s.setStatus(fmt.Sprintf("[red]%s", message))
		s.mutex.Lock()
		if loadVersion == s.loadSeq {
			s.failedClusters[name] = message
			s.refreshing = false
			s.loadingCluster = ""
			s.currentServices = nil
			s.loadCancel = nil
		}
		s.mutex.Unlock()
		s.queueUpdate(func() {
			s.showServiceMessage("Unable to load services")
		})
		cancel()
		return
	}
	if ctx.Err() != nil {
		s.mutex.Lock()
		if loadVersion == s.loadSeq {
			s.refreshing = false
			s.loadingCluster = ""
			s.currentServices = nil
			s.loadCancel = nil
		}
		s.mutex.Unlock()
		cancel()
		return
	}

	cancel()
	s.mutex.Lock()
	if loadVersion != s.loadSeq {
		s.mutex.Unlock()
		return
	}
	if s.currentCluster == name {
		s.currentServices = servicesList
		delete(s.failedClusters, name)
	}
	s.refreshing = false
	s.loadingCluster = ""
	s.loadCancel = nil
	currentMode := s.mode
	s.mutex.Unlock()

	if currentMode == modeServices && s.currentCluster == name {
		s.renderServiceTable(servicesList)
		s.setStatus(fmt.Sprintf("[green]Loaded %d service(s) for %s", len(servicesList), name))
	}
}

func (s *uiState) loadBuckets(ctx context.Context, name string, force bool) {
	if name == "" {
		return
	}

	clusterCfg := s.conf.Oscar[name]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", name))
		s.queueUpdate(func() {
			s.showBucketMessage("Cluster not found")
		})
		return
	}

	s.setStatus(fmt.Sprintf("[yellow]Loading buckets for cluster %s…", name))
	s.queueUpdate(func() {
		s.showBucketMessage("Loading buckets…")
	})

	s.mutex.Lock()
	if s.bucketCancel != nil {
		s.bucketCancel()
		s.bucketCancel = nil
	}
	s.bucketSeq++
	seq := s.bucketSeq
	ctxFetch, cancel := context.WithTimeout(ctx, 15*time.Second)
	s.bucketCancel = cancel
	s.mutex.Unlock()

	buckets, err := storage.ListBucketsWithContext(ctxFetch, clusterCfg)
	cancel()
	if err != nil {
		s.setStatus(fmt.Sprintf("[red]Unable to load buckets for %s: %v", name, err))
		s.mutex.Lock()
		if seq == s.bucketSeq {
			s.bucketInfos = nil
			s.bucketCancel = nil
			s.bucketCluster = ""
		}
		s.mutex.Unlock()
		s.queueUpdate(func() {
			s.showBucketMessage("Unable to load buckets")
		})
		return
	}

	s.mutex.Lock()
	if seq != s.bucketSeq {
		s.mutex.Unlock()
		return
	}
	s.bucketInfos = buckets
	s.bucketCancel = nil
	s.bucketCluster = name
	mode := s.mode
	currentCluster := s.currentCluster
	s.mutex.Unlock()

	if mode == modeBuckets && currentCluster == name {
		s.renderBucketTable(buckets)
		s.setStatus(fmt.Sprintf("[green]Loaded %d bucket(s) for %s", len(buckets), name))
	}
}

func (s *uiState) setStatus(message string) {
	s.mutex.Lock()
	started := s.started
	s.mutex.Unlock()
	if !started {
		s.statusView.SetText(message)
		return
	}
	s.queueUpdate(func() {
		s.statusView.SetText(message)
	})
}

func sortedClusters(m map[string]*cluster.Cluster) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
		s.handleBucketSelection(row)
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

func (s *uiState) handleServiceSelection(row int, immediate bool) {
	s.mutex.Lock()
	if s.mode != modeServices {
		if s.detailTimer != nil {
			s.detailTimer.Stop()
			s.detailTimer = nil
		}
		s.mutex.Unlock()
		return
	}
	if row <= 0 || row-1 >= len(s.currentServices) {
		if s.detailTimer != nil {
			s.detailTimer.Stop()
			s.detailTimer = nil
		}
		s.lastSelection = ""
		s.mutex.Unlock()
		return
	}
	svcPtr := s.currentServices[row-1]
	if svcPtr == nil {
		s.mutex.Unlock()
		return
	}
	svc := *svcPtr
	token := fmt.Sprintf("%s-%d-%d", svc.Name, row, s.loadSeq)
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = token
	s.mutex.Unlock()

	if immediate {
		s.queueUpdate(func() {
			s.detailsView.SetText(formatServiceDetails(&svc))
		})
		return
	}

	timer := time.AfterFunc(1*time.Second, func() {
		s.mutex.Lock()
		if s.lastSelection != token {
			s.mutex.Unlock()
			return
		}
		s.detailTimer = nil
		s.mutex.Unlock()
		s.queueUpdate(func() {
			s.detailsView.SetText(formatServiceDetails(&svc))
		})
	})

	s.mutex.Lock()
	if s.lastSelection == token {
		s.detailTimer = timer
	} else {
		timer.Stop()
	}
	s.mutex.Unlock()
}

func (s *uiState) handleBucketSelection(row int) {
	s.mutex.Lock()
	if s.mode != modeBuckets {
		s.mutex.Unlock()
		return
	}
	if row <= 0 || row-1 >= len(s.bucketInfos) {
		s.mutex.Unlock()
		s.queueUpdate(func() {
			s.detailsView.SetText("Select a bucket to inspect details")
		})
		return
	}
	bucket := s.bucketInfos[row-1]
	s.mutex.Unlock()
	s.queueUpdate(func() {
		s.detailsView.SetText(formatBucketDetails(bucket))
	})
}

func (s *uiState) toggleLegend() {
	s.mutex.Lock()
	visible := s.legendVisible
	confirm := s.confirmVisible
	s.mutex.Unlock()
	if visible {
		s.queueUpdate(func() {
			s.hideLegendUnlocked()
		})
		return
	}
	if confirm || s.pages == nil {
		return
	}
	s.queueUpdate(func() {
		s.showLegendUnlocked()
	})
}

func (s *uiState) showLegendUnlocked() {
	if s.pages == nil {
		return
	}
	s.mutex.Lock()
	if s.legendVisible {
		s.mutex.Unlock()
		return
	}
	s.legendVisible = true
	s.savedFocus = s.app.GetFocus()
	s.mutex.Unlock()
	modal := tview.NewModal().
		SetText(legendText).
		AddButtons([]string{"Close"})
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		s.hideLegendUnlocked()
	})
	s.pages.AddAndSwitchToPage("legend", modal, true)
}

func (s *uiState) hideLegendUnlocked() {
	if s.pages == nil {
		return
	}
	s.mutex.Lock()
	if !s.legendVisible {
		s.mutex.Unlock()
		return
	}
	s.legendVisible = false
	focus := s.savedFocus
	s.savedFocus = nil
	s.mutex.Unlock()
	s.pages.RemovePage("legend")
	if focus != nil {
		s.app.SetFocus(focus)
	}
}

func (s *uiState) requestDeletion() {
	s.mutex.Lock()
	mode := s.mode
	if s.confirmVisible || s.legendVisible || s.pages == nil {
		s.mutex.Unlock()
		return
	}
	row, _ := s.serviceTable.GetSelection()
	clusterName := s.currentCluster
	switch mode {
	case modeServices:
		if row <= 0 || row-1 >= len(s.currentServices) || clusterName == "" {
			s.mutex.Unlock()
			s.setStatus("[red]Select a service to delete")
			return
		}
		svcPtr := s.currentServices[row-1]
		if svcPtr == nil {
			s.mutex.Unlock()
			s.setStatus("[red]Select a service to delete")
			return
		}
		if s.detailTimer != nil {
			s.detailTimer.Stop()
			s.detailTimer = nil
		}
		svcName := svcPtr.Name
		s.mutex.Unlock()

		prompt := fmt.Sprintf("Delete service %q from cluster %q?", svcName, clusterName)
		s.queueUpdate(func() {
			s.showConfirmation(prompt, func() {
				go s.performDeletion(clusterName, svcName)
			})
		})
	case modeBuckets:
		if row <= 0 || row-1 >= len(s.bucketInfos) || clusterName == "" {
			s.mutex.Unlock()
			s.setStatus("[red]Select a bucket to delete")
			return
		}
		bucket := s.bucketInfos[row-1]
		if bucket == nil || strings.TrimSpace(bucket.Name) == "" {
			s.mutex.Unlock()
			s.setStatus("[red]Select a bucket to delete")
			return
		}
		bucketName := bucket.Name
		s.mutex.Unlock()

		prompt := fmt.Sprintf("Delete bucket %q from cluster %q?", bucketName, clusterName)
		s.queueUpdate(func() {
			s.showConfirmation(prompt, func() {
				go s.performBucketDeletion(clusterName, bucketName)
			})
		})
	default:
		s.mutex.Unlock()
		s.setStatus("[red]Deletion not available in this view")
	}
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

func (s *uiState) showConfirmation(text string, onConfirm func()) {
	if s.pages == nil {
		return
	}
	s.mutex.Lock()
	if s.confirmVisible {
		s.mutex.Unlock()
		return
	}
	s.confirmVisible = true
	s.savedFocus = s.app.GetFocus()
	s.mutex.Unlock()
	modal := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Cancel", "Delete"})
	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if buttonLabel == "Delete" {
			onConfirm()
		}
		s.hideConfirmationUnlocked()
	})
	s.pages.AddAndSwitchToPage("confirm", modal, true)
}

func (s *uiState) hideConfirmationUnlocked() {
	if s.pages == nil {
		return
	}
	s.mutex.Lock()
	if !s.confirmVisible {
		s.mutex.Unlock()
		return
	}
	s.confirmVisible = false
	focus := s.savedFocus
	s.savedFocus = nil
	s.mutex.Unlock()
	s.pages.RemovePage("confirm")
	if focus != nil {
		s.app.SetFocus(focus)
	}
}

func (s *uiState) performDeletion(clusterName, svcName string) {
	s.setStatus(fmt.Sprintf("[yellow]Deleting service %q...", svcName))
	s.mutex.Lock()
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = ""
	s.mutex.Unlock()
	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		return
	}
	if err := service.RemoveService(clusterCfg, svcName); err != nil {
		s.setStatus(fmt.Sprintf("[red]Failed to delete service %q: %v", svcName, err))
		return
	}
	s.setStatus(fmt.Sprintf("[green]Service %q deleted", svcName))
	s.queueUpdate(func() {
		s.detailsView.SetText("Select a service to inspect details")
	})
	s.refreshCurrent(context.Background())
}

func (s *uiState) performBucketDeletion(clusterName, bucketName string) {
	s.setStatus(fmt.Sprintf("[yellow]Deleting bucket %q...", bucketName))
	s.mutex.Lock()
	s.lastSelection = ""
	s.mutex.Unlock()
	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		return
	}
	if err := storage.DeleteBucket(clusterCfg, bucketName); err != nil {
		s.setStatus(fmt.Sprintf("[red]Failed to delete bucket %q: %v", bucketName, err))
		return
	}
	s.setStatus(fmt.Sprintf("[green]Bucket %q deleted", bucketName))
	s.queueUpdate(func() {
		s.detailsView.SetText("Select a bucket to inspect details")
	})
	s.refreshCurrent(context.Background())
}

func (s *uiState) initiateSearch(ctx context.Context) {
	_ = ctx
	s.mutex.Lock()
	if s.searchVisible || s.confirmVisible || s.legendVisible || s.pages == nil {
		s.mutex.Unlock()
		return
	}
	focus := s.app.GetFocus()
	mode := s.mode
	s.mutex.Unlock()

	target := searchTargetNone
	switch focus {
	case s.clusterList:
		target = searchTargetClusters
	case s.serviceTable:
		if mode == modeBuckets {
			target = searchTargetBuckets
		} else {
			target = searchTargetServices
		}
	}

	if target == searchTargetNone {
		return
	}

	s.mutex.Lock()
	switch target {
	case searchTargetClusters:
		if len(s.clusterNames) == 0 {
			s.mutex.Unlock()
			s.setStatus("[yellow]No clusters to search")
			return
		}
	case searchTargetServices:
		if len(s.currentServices) == 0 {
			s.mutex.Unlock()
			s.setStatus("[yellow]No services to search")
			return
		}
	case searchTargetBuckets:
		if len(s.bucketInfos) == 0 {
			s.mutex.Unlock()
			s.setStatus("[yellow]No buckets to search")
			return
		}
	}
	s.mutex.Unlock()

	s.showSearch(target)
}

func (s *uiState) showSearch(target searchTarget) {
	s.mutex.Lock()
	if s.searchVisible || s.pages == nil {
		s.mutex.Unlock()
		return
	}
	s.searchVisible = true
	s.searchTarget = target
	s.originalFocus = s.app.GetFocus()
	container := s.statusContainer
	s.mutex.Unlock()

	label := "Search: "
	switch target {
	case searchTargetClusters:
		label = "Clusters: "
	case searchTargetServices:
		label = "Services: "
	case searchTargetBuckets:
		label = "Buckets: "
	}

	input := tview.NewInputField().
		SetLabel(label).
		SetFieldWidth(30)
	input.SetChangedFunc(func(text string) {
		s.handleSearchInput(text)
	})
	input.SetDoneFunc(func(key tcell.Key) {
		s.hideSearch()
	})

	s.mutex.Lock()
	s.searchInput = input
	s.mutex.Unlock()

	s.queueUpdate(func() {
		container.Clear()
		container.SetTitle("Search")
		input.SetBorder(false)
		container.AddItem(input, 0, 1, true)
	})
	s.app.SetFocus(input)
}

func (s *uiState) hideSearch() {
	s.mutex.Lock()
	if !s.searchVisible {
		s.mutex.Unlock()
		return
	}
	s.searchVisible = false
	s.searchTarget = searchTargetNone
	input := s.searchInput
	s.searchInput = nil
	focus := s.originalFocus
	s.originalFocus = nil
	container := s.statusContainer
	s.mutex.Unlock()

	s.queueUpdate(func() {
		container.Clear()
		container.SetTitle("Status")
		container.AddItem(s.statusView, 0, 1, false)
		s.statusView.SetText(statusHelpText)
	})
	if focus != nil {
		s.app.SetFocus(focus)
	}

	if input != nil {
		input.SetText("")
	}
}

func (s *uiState) handleSearchInput(query string) {
	s.mutex.Lock()
	target := s.searchTarget
	s.mutex.Unlock()
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return
	}
	lower := strings.ToLower(trimmed)
	var found bool
	switch target {
	case searchTargetClusters:
		found = s.searchClusters(lower)
	case searchTargetServices:
		found = s.searchServices(lower)
	case searchTargetBuckets:
		found = s.searchBuckets(lower)
	}
	if !found {
		s.setStatus("[yellow]No matches found")
	}
}

func (s *uiState) searchClusters(query string) bool {
	s.mutex.Lock()
	names := append([]string(nil), s.clusterNames...)
	s.mutex.Unlock()
	for idx, name := range names {
		if strings.Contains(strings.ToLower(name), query) {
			s.queueUpdate(func() {
				s.clusterList.SetCurrentItem(idx)
			})
			return true
		}
	}
	return false
}

func (s *uiState) searchServices(query string) bool {
	s.mutex.Lock()
	services := append([]*types.Service(nil), s.currentServices...)
	s.mutex.Unlock()
	for idx, svc := range services {
		if svc == nil {
			continue
		}
		if strings.Contains(strings.ToLower(svc.Name), query) {
			row := idx + 1
			s.queueUpdate(func() {
				s.serviceTable.Select(row, 0)
				s.handleServiceSelection(row, true)
			})
			return true
		}
	}
	return false
}

func (s *uiState) searchBuckets(query string) bool {
	s.mutex.Lock()
	buckets := append([]*storage.BucketInfo(nil), s.bucketInfos...)
	s.mutex.Unlock()
	for idx, bucket := range buckets {
		if bucket == nil {
			continue
		}
		haystack := strings.ToLower(bucket.Name + " " + bucket.Owner)
		if strings.Contains(haystack, query) {
			row := idx + 1
			s.queueUpdate(func() {
				s.serviceTable.Select(row, 0)
				s.handleBucketSelection(row)
			})
			return true
		}
	}
	return false
}

func truncateString(val string, limit int) string {
	if limit <= 0 || len(val) <= limit {
		return val
	}
	return val[:limit-1] + "…"
}

func defaultIfEmpty(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

func formatClusterInfo(clusterName string, info types.Info) string {
	builder := &strings.Builder{}
	if clusterName != "" {
		fmt.Fprintf(builder, "[yellow]Cluster:[-] %s\n", clusterName)
	}
	if info.Version != "" {
		fmt.Fprintf(builder, "[yellow]Version:[-] %s\n", info.Version)
	}
	if info.GitCommit != "" {
		fmt.Fprintf(builder, "[yellow]Commit:[-] %s\n", info.GitCommit)
	}
	if info.Architecture != "" {
		fmt.Fprintf(builder, "[yellow]Architecture:[-] %s\n", info.Architecture)
	}
	if info.KubeVersion != "" {
		fmt.Fprintf(builder, "[yellow]Kubernetes:[-] %s\n", info.KubeVersion)
	}
	if backend := info.ServerlessBackendInfo; backend != nil {
		if backend.Name != "" {
			fmt.Fprintf(builder, "[yellow]Serverless:[-] %s", backend.Name)
			if backend.Version != "" {
				fmt.Fprintf(builder, " %s", backend.Version)
			}
			builder.WriteByte('\n')
		} else if backend.Version != "" {
			fmt.Fprintf(builder, "[yellow]Serverless:[-] %s\n", backend.Version)
		}
	}
	out := strings.TrimRight(builder.String(), "\n")
	if out == "" {
		return "No cluster information available"
	}
	return out
}

func formatServiceDetails(svc *types.Service) string {
	if svc == nil {
		return ""
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]Name:[-] %s\n", svc.Name)
	if svc.ClusterID != "" {
		fmt.Fprintf(builder, "[yellow]Cluster:[-] %s\n", svc.ClusterID)
	}
	if svc.Image != "" {
		fmt.Fprintf(builder, "[yellow]Image:[-] %s\n", svc.Image)
	}
	if svc.Memory != "" {
		fmt.Fprintf(builder, "[yellow]Memory:[-] %s\n", svc.Memory)
	}
	if svc.CPU != "" {
		fmt.Fprintf(builder, "[yellow]CPU:[-] %s\n", svc.CPU)
	}
	if replicas := len(svc.Replicas); replicas > 0 {
		fmt.Fprintf(builder, "[yellow]Replicas:[-] %d\n", replicas)
	}
	if svc.LogLevel != "" {
		fmt.Fprintf(builder, "[yellow]Log Level:[-] %s\n", svc.LogLevel)
	}
	return builder.String()
}

func formatBucketDetails(bucket *storage.BucketInfo) string {
	if bucket == nil {
		return ""
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]Name:[-] %s\n", bucket.Name)
	if bucket.Visibility != "" {
		fmt.Fprintf(builder, "[yellow]Visibility:[-] %s\n", bucket.Visibility)
	}
	if len(bucket.AllowedUsers) > 0 {
		fmt.Fprintf(builder, "[yellow]Allowed Users:[-] %s\n", strings.Join(bucket.AllowedUsers, ", "))
	}
	if bucket.Owner != "" {
		fmt.Fprintf(builder, "[yellow]Owner:[-] %s\n", bucket.Owner)
	}

	if !bucket.CreationDate.IsZero() {
		fmt.Fprintf(builder, "[yellow]Created:[-] %s\n", bucket.CreationDate.Format("2006-01-02 15:04"))
	}
	return builder.String()
}

func (s *uiState) showServiceMessage(message string) {
	s.serviceTable.SetTitle("Services")
	setServiceTableHeader(s.serviceTable)
	fillMessageRow(s.serviceTable, len(serviceHeaders), message)
}

func (s *uiState) showBucketMessage(message string) {
	s.serviceTable.SetTitle("Buckets")
	setBucketTableHeader(s.serviceTable)
	fillMessageRow(s.serviceTable, len(bucketHeaders), message)
}

func (s *uiState) renderServiceTable(services []*types.Service) {
	s.queueUpdate(func() {
		s.serviceTable.SetTitle("Services")
		setServiceTableHeader(s.serviceTable)
		if len(services) == 0 {
			fillMessageRow(s.serviceTable, len(serviceHeaders), "No services found")
			return
		}
		for i, svc := range services {
			row := i + 1
			s.serviceTable.SetCell(row, 0, tview.NewTableCell(svc.Name).
				SetExpansion(2).
				SetSelectable(true)).
				SetCell(row, 1, tview.NewTableCell(truncateString(svc.Image, 40)).
					SetExpansion(4)).
				SetCell(row, 2, tview.NewTableCell(defaultIfEmpty(svc.CPU, "-")).
					SetExpansion(1)).
				SetCell(row, 3, tview.NewTableCell(defaultIfEmpty(svc.Memory, "-")).
					SetExpansion(1))
		}
		row, col := s.serviceTable.GetSelection()
		if row <= 0 || row > len(services) {
			s.serviceTable.Select(1, 0)
		} else {
			s.serviceTable.Select(row, col)
		}
	})
}

func (s *uiState) renderBucketTable(buckets []*storage.BucketInfo) {
	s.queueUpdate(func() {
		s.serviceTable.SetTitle("Buckets")
		setBucketTableHeader(s.serviceTable)
		if len(buckets) == 0 {
			fillMessageRow(s.serviceTable, len(bucketHeaders), "No buckets found")
			s.detailsView.SetText("Select a bucket to inspect details")
			return
		}
		for i, bucket := range buckets {
			row := i + 1
			color := bucketVisibilityColor(bucket.Visibility)
			nameCell := tview.NewTableCell(bucket.Name).
				SetSelectable(true).
				SetExpansion(4)
			visCell := tview.NewTableCell(defaultIfEmpty(bucket.Visibility, "-")).
				SetExpansion(2).
				SetTextColor(color)
			ownerCell := tview.NewTableCell(defaultIfEmpty(bucket.Owner, "-")).
				SetExpansion(5)
			s.serviceTable.SetCell(row, 0, nameCell).
				SetCell(row, 1, visCell).
				SetCell(row, 2, ownerCell)
		}
		row, col := s.serviceTable.GetSelection()
		if row <= 0 || row > len(buckets) {
			s.serviceTable.Select(1, 0)
		} else {
			s.serviceTable.Select(row, col)
		}
	})
}

func setServiceTableHeader(table *tview.Table) {
	setTableHeader(table, serviceHeaders)
}

func setBucketTableHeader(table *tview.Table) {
	setTableHeader(table, bucketHeaders)
}

func setTableHeader(table *tview.Table, headers []string) {
	table.Clear()
	for col, header := range headers {
		table.SetCell(0, col, tview.NewTableCell(header).
			SetTextColor(tcell.ColorWhite).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold))
	}
}

func fillMessageRow(table *tview.Table, columns int, message string) {
	table.SetCell(1, 0, tview.NewTableCell(message).
		SetAlign(tview.AlignCenter).
		SetSelectable(false).
		SetExpansion(columns))
	for col := 1; col < columns; col++ {
		table.SetCell(1, col, tview.NewTableCell("").SetSelectable(false))
	}
}

func bucketVisibilityColor(vis string) tcell.Color {
	switch strings.ToLower(strings.TrimSpace(vis)) {
	case "restricted":
		return tcell.ColorYellow
	case "private":
		return tcell.ColorRed
	case "public":
		return tcell.ColorGreen
	default:
		return tcell.ColorWhite
	}
}
