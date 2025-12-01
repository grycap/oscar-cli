package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v3/pkg/types"
)

func (s *uiState) markServicePanelVisited() {
	s.mutex.Lock()
	already := s.servicePanelVisited
	s.servicePanelVisited = true
	row, _ := s.serviceTable.GetSelection()
	s.mutex.Unlock()
	if already {
		return
	}
	if row > 0 {
		s.handleSelection(row, true)
		return
	}
	s.setServiceDetailsText("Select a service to inspect details")
}

func (s *uiState) serviceDetailsEnabled() bool {
	s.mutex.Lock()
	visited := s.servicePanelVisited
	s.mutex.Unlock()
	return visited
}

func (s *uiState) setServiceDetailsText(text string) {
	if !s.serviceDetailsEnabled() {
		return
	}
	s.queueUpdate(func() {
		s.detailsView.SetText(text)
	})
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
	s.currentLogsKey = ""
	s.currentLogJobKey = ""
	s.currentLogService = ""
	s.currentLogCluster = ""
	s.logEntries = nil
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
	services := s.currentServices
	clusterName := s.currentCluster
	s.mutex.Unlock()

	s.hideBucketObjectsPane()
	s.showClusterDetails(clusterName)

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
	enabled := s.servicePanelVisited
	if row <= 0 || row-1 >= len(s.currentServices) {
		if s.detailTimer != nil {
			s.detailTimer.Stop()
			s.detailTimer = nil
		}
		s.lastSelection = ""
		s.mutex.Unlock()
		if enabled {
			s.setServiceDetailsText("Select a service to inspect details")
		}
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

	if !enabled {
		return
	}

	if immediate {
		s.showServiceDefinition(s.currentCluster, svc.Name)
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
		s.showServiceDefinition(s.currentCluster, svc.Name)
	})

	s.mutex.Lock()
	if s.lastSelection == token {
		s.detailTimer = timer
	} else {
		timer.Stop()
	}
	s.mutex.Unlock()
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
	s.setServiceDetailsText("Select a service to inspect details")
	s.refreshCurrent(context.Background())
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

func (s *uiState) showServiceMessage(message string) {
	s.serviceTable.SetTitle("Services")
	setServiceTableHeader(s.serviceTable)
	fillMessageRow(s.serviceTable, len(serviceHeaders), message)
}

func (s *uiState) searchServices(query string) bool {
	s.mutex.Lock()
	services := append([]*types.Service(nil), s.currentServices...)
	s.mutex.Unlock()
	for idx, svc := range services {
		if svc == nil {
			continue
		}
		fields := []string{svc.Name, svc.Image, svc.CPU, svc.Memory}
		if containsQuery(strings.Join(fields, " "), query) {
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

func (s *uiState) showServiceDefinition(clusterName, serviceName string) {
	clusterName = strings.TrimSpace(clusterName)
	serviceName = strings.TrimSpace(serviceName)
	if clusterName == "" || serviceName == "" {
		s.setServiceDetailsText("Select a service to inspect details")
		return
	}

	key := makeServiceDefinitionKey(clusterName, serviceName)
	s.mutex.Lock()
	cached := s.serviceDefinitions[key]
	s.serviceDefinitionSeq++
	seq := s.serviceDefinitionSeq
	s.currentServiceDefinition = key
	s.mutex.Unlock()

	if cached != "" {
		s.queueUpdate(func() {
			s.detailsView.SetText(cached)
		})
		return
	}

	loadingText := fmt.Sprintf("Loading definition for %s…", serviceName)
	s.queueUpdate(func() {
		s.detailsView.SetText(loadingText)
	})
	s.setStatus(fmt.Sprintf("[yellow]Loading definition for %q…", serviceName))

	go s.fetchServiceDefinition(clusterName, serviceName, key, seq)
}

func (s *uiState) fetchServiceDefinition(clusterName, serviceName, key string, seq int) {
	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		return
	}

	def, err := service.GetService(clusterCfg, serviceName)
	if err != nil {
		s.setStatus(fmt.Sprintf("[red]Failed to load definition for %q: %v", serviceName, err))
		return
	}

	rendered, err := formatServiceDefinition(def)
	if err != nil {
		s.setStatus(fmt.Sprintf("[red]Failed to format definition for %q: %v", serviceName, err))
		return
	}

	s.mutex.Lock()
	if seq != s.serviceDefinitionSeq {
		s.mutex.Unlock()
		return
	}
	s.serviceDefinitions[key] = rendered
	active := s.currentServiceDefinition
	s.mutex.Unlock()

	if active == key {
		s.queueUpdate(func() {
			s.detailsView.SetText(rendered)
		})
		s.setStatus(fmt.Sprintf("[green]Loaded definition for %q", serviceName))
	}
}

func makeServiceDefinitionKey(clusterName, serviceName string) string {
	return fmt.Sprintf("%s\x00%s", clusterName, serviceName)
}

func formatServiceDefinition(svc *types.Service) (string, error) {
	if svc == nil {
		return "", nil
	}

	data, err := json.Marshal(svc)
	if err != nil {
		return "", err
	}
	var val interface{}
	if err := json.Unmarshal(data, &val); err != nil {
		return "", err
	}
	builder := &strings.Builder{}
	colorizeJSON(builder, val, 0)
	return builder.String(), nil
}

func colorizeJSON(builder *strings.Builder, val interface{}, level int) {
	indent := strings.Repeat("  ", level)
	switch v := val.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		builder.WriteString("{\n")
		for i, k := range keys {
			builder.WriteString(indent + "  ")
			builder.WriteString("[yellow]")
			builder.WriteString(tview.Escape(k))
			builder.WriteString("[-]: ")
			colorizeJSON(builder, v[k], level+1)
			if i < len(keys)-1 {
				builder.WriteString(",")
			}
			builder.WriteString("\n")
		}
		builder.WriteString(indent + "}")
	case []interface{}:
		builder.WriteString("[\n")
		for i, item := range v {
			builder.WriteString(indent + "  ")
			colorizeJSON(builder, item, level+1)
			if i < len(v)-1 {
				builder.WriteString(",")
			}
			builder.WriteString("\n")
		}
		builder.WriteString(indent + "]")
	case string:
		builder.WriteString("[green]\"")
		builder.WriteString(tview.Escape(v))
		builder.WriteString("\"[-]")
	case float64, int, int64, uint64, float32:
		builder.WriteString("[cyan]")
		builder.WriteString(fmt.Sprintf("%v", v))
		builder.WriteString("[-]")
	case bool:
		builder.WriteString("[magenta]")
		builder.WriteString(fmt.Sprintf("%v", v))
		builder.WriteString("[-]")
	case nil:
		builder.WriteString("[gray]null[-]")
	default:
		builder.WriteString(tview.Escape(fmt.Sprintf("%v", v)))
	}
}
