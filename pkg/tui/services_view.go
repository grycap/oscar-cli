package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/cluster"
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

func (s *uiState) showServiceLogs() {
	s.mutex.Lock()
	if s.confirmVisible || s.legendVisible {
		s.mutex.Unlock()
		return
	}
	if s.mode != modeServices {
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

	s.setStatus(fmt.Sprintf("[yellow]Loading logs for %q…", serviceName))
	s.queueUpdate(func() {
		s.detailsView.SetText(fmt.Sprintf("Loading logs for %s…", serviceName))
	})

	go func(cName, svcName string, cfg *cluster.Cluster) {
		jobName, err := service.FindLatestJobName(cfg, svcName)
		if err != nil {
			if errors.Is(err, service.ErrNoLogsFound) {
				s.setStatus(fmt.Sprintf("[yellow]No logs found for %q", svcName))
				s.queueUpdate(func() {
					s.detailsView.SetText(formatServiceLogs(svcName, "", ""))
				})
				return
			}
			s.setStatus(fmt.Sprintf("[red]Failed to locate logs for %q: %v", svcName, err))
			return
		}

		logText, err := service.GetLogs(cfg, svcName, jobName, false)
		if err != nil {
			s.setStatus(fmt.Sprintf("[red]Failed to download logs for %q: %v", svcName, err))
			return
		}

		s.setStatus(fmt.Sprintf("[green]Loaded logs for %q", svcName))
		rendered := formatServiceLogs(svcName, jobName, logText)
		s.queueUpdate(func() {
			s.detailsView.SetText(rendered)
		})
	}(clusterName, serviceName, clusterCfg)
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
