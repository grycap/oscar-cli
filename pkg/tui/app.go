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
	"github.com/grycap/oscar/v3/pkg/types"
)

const legendText = `[yellow]Navigation[-]
  ↑/↓  Move selection
  ←/→ or Tab  Switch pane

[yellow]Actions[-]
  r  Refresh services
  d  Delete selected service
  q  Quit
  ?  Toggle this help`

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
	}

	state.statusView.SetBorder(true)
	state.statusView.SetTitle("Status")
	state.detailsView.SetBorder(true)
	state.detailsView.SetTitle("Service Details")
	state.detailsView.SetText("Select a service to inspect details")
	state.serviceTable.SetBorder(true)
	state.serviceTable.SetTitle("Services")
	state.serviceTable.SetFixed(1, 0)
	state.clusterList.SetBorder(true)
	state.clusterList.SetTitle("Clusters")

	clusterNames := sortedClusters(conf.Oscar)
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
		state.handleServiceSelection(row, false)
	})
	state.serviceTable.SetSelectedFunc(func(row, column int) {
		state.handleServiceSelection(row, true)
	})

	layout := tview.NewFlex().
		AddItem(state.clusterList, 0, 1, true).
		AddItem(state.serviceTable, 0, 3, false)

	root := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(layout, 0, 1, true).
		AddItem(state.detailsView, 12, 1, false).
		AddItem(state.statusView, 3, 1, false)

	state.statusView.SetText("[yellow]Keys: [::b]q[::-] Quit · [::b]r[::-] Refresh · [::b]d[::-] Delete · [::b]?[::-] Help · [::b]←/→[::-] Switch pane")

	pages := tview.NewPages()
	pages.AddPage("main", root, true, true)
	state.pages = pages

	app.SetRoot(pages, true)
	app.SetFocus(state.clusterList)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
		case 'd', 'D':
			if app.GetFocus() == state.serviceTable {
				state.requestDeletion()
				return nil
			}
		case '?':
			state.toggleLegend()
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
	pages           *tview.Pages
	mutex           *sync.Mutex
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
	}
	if s.detailTimer != nil {
		s.detailTimer.Stop()
		s.detailTimer = nil
	}
	s.lastSelection = ""
	s.currentCluster = name
	if errMsg, blocked := s.failedClusters[name]; blocked {
		s.mutex.Unlock()
		s.queueUpdate(func() {
			s.showServiceMessage("Unable to load services")
		})
		s.setStatus(fmt.Sprintf("[red]%s", errMsg))
		s.mutex.Lock()
		s.currentServices = nil
		s.mutex.Unlock()
		return
	}
	s.mutex.Unlock()

	s.queueUpdate(func() {
		s.detailsView.SetText("Select a service to inspect details")
	})

	go s.loadServices(ctx, name, false)
}

func (s *uiState) refreshCurrent(ctx context.Context) {
	s.mutex.Lock()
	name := s.currentCluster
	delete(s.failedClusters, name)
	s.mutex.Unlock()
	if name == "" {
		return
	}
	go s.loadServices(ctx, name, true)
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

	s.queueUpdate(func() {
		setServiceTableHeader(s.serviceTable)
		if len(servicesList) == 0 {
			s.showServiceMessage("No services found")
			return
		}
		for i, svc := range servicesList {
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
		s.mutex.Lock()
		defer s.mutex.Unlock()
		if loadVersion == s.loadSeq {
			if s.currentCluster == name {
				s.currentServices = servicesList
				delete(s.failedClusters, name)
			}
			s.refreshing = false
			s.loadingCluster = ""
			s.loadCancel = nil
		}
	})
	cancel()
	s.setStatus(fmt.Sprintf("[green]Loaded %d service(s) for %s", len(servicesList), name))
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

func setServiceTableHeader(table *tview.Table) {
	table.Clear()
	headers := []string{"Name", "Image", "CPU", "Memory"}
	for col, header := range headers {
		table.SetCell(0, col, tview.NewTableCell(header).
			SetTextColor(tcell.ColorWhite).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold))
	}
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
	if s.confirmVisible || s.legendVisible || s.pages == nil {
		s.mutex.Unlock()
		return
	}
	row, _ := s.serviceTable.GetSelection()
	if row <= 0 || row-1 >= len(s.currentServices) {
		s.mutex.Unlock()
		s.setStatus("[red]Select a service to delete")
		return
	}
	svcPtr := s.currentServices[row-1]
	clusterName := s.currentCluster
	if svcPtr == nil || clusterName == "" {
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

func formatServiceDetails(svc *types.Service) string {
	if svc == nil {
		return ""
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]Name:[-] %s\n", svc.Name)
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

func (s *uiState) showServiceMessage(message string) {
	setServiceTableHeader(s.serviceTable)
	s.serviceTable.SetCell(1, 0, tview.NewTableCell(message).
		SetAlign(tview.AlignCenter).
		SetSelectable(false).
		SetExpansion(8))
	s.serviceTable.SetCell(1, 1, tview.NewTableCell("").SetSelectable(false))
	s.serviceTable.SetCell(1, 2, tview.NewTableCell("").SetSelectable(false))
	s.serviceTable.SetCell(1, 3, tview.NewTableCell("").SetSelectable(false))
}
