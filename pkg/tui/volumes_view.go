package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/v2/pkg/volume"
	"github.com/grycap/oscar/v4/pkg/types"
)

func (s *uiState) switchToVolumes(ctx context.Context) {
	if s.searchVisible {
		s.hideSearch()
	}
	s.mutex.Lock()
	if s.confirmVisible || s.legendVisible {
		s.mutex.Unlock()
		return
	}
	if s.mode == modeVolumes {
		s.mutex.Unlock()
		return
	}
	s.mode = modeVolumes
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
	s.mutex.Unlock()

	s.hideBucketObjectsPane()

	clusterName := s.currentCluster
	if clusterName == "" {
		s.setStatus("[red]Select a cluster to view volumes")
		s.queueUpdate(func() {
			s.showVolumeMessage("Select a cluster to view volumes")
		})
		s.showClusterDetails(clusterName)
		return
	}

	s.showClusterDetails(clusterName)
	s.queueUpdate(func() {
		s.showVolumeMessage("Loading volumes…")
	})

	s.mutex.Lock()
	cached := s.volumes
	cachedCluster := s.volumeCluster
	s.mutex.Unlock()
	if len(cached) > 0 && cachedCluster == clusterName {
		s.renderVolumeTable(cached)
		s.setStatus(fmt.Sprintf("[green]Loaded %d volume(s) for %s", len(cached), clusterName))
		return
	}

	go s.loadVolumes(ctx, clusterName, false)
}

func (s *uiState) loadVolumes(ctx context.Context, name string, force bool) {
	if name == "" {
		return
	}

	clusterCfg := s.conf.Oscar[name]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", name))
		s.queueUpdate(func() {
			s.showVolumeMessage("Cluster not found")
		})
		return
	}

	s.setStatus(fmt.Sprintf("[yellow]Loading volumes for cluster %s…", name))
	s.mutex.Lock()
	keepTable := force && s.currentCluster == name && len(s.volumes) > 0
	s.mutex.Unlock()
	if !keepTable {
		s.queueUpdate(func() {
			s.showVolumeMessage("Loading volumes…")
		})
	}

	s.mutex.Lock()
	if s.volumeCancel != nil {
		s.volumeCancel()
		s.volumeCancel = nil
	}
	s.volumeSeq++
	seq := s.volumeSeq
	s.mutex.Unlock()

	vols, err := volume.ListVolumes(clusterCfg)
	if err != nil {
		s.setStatus(fmt.Sprintf("[red]Unable to load volumes for %s: %v", name, err))
		s.mutex.Lock()
		if seq == s.volumeSeq {
			s.volumes = nil
			s.volumeCancel = nil
			s.volumeCluster = ""
		}
		s.mutex.Unlock()
		s.queueUpdate(func() {
			s.showVolumeMessage("Unable to load volumes")
		})
		return
	}

	s.mutex.Lock()
	if seq != s.volumeSeq {
		s.mutex.Unlock()
		return
	}
	s.volumes = vols
	s.volumeCancel = nil
	s.volumeCluster = name
	mode := s.mode
	currentCluster := s.currentCluster
	s.mutex.Unlock()

	if mode == modeVolumes && currentCluster == name {
		s.renderVolumeTable(vols)
		s.setStatus(fmt.Sprintf("[green]Loaded %d volume(s) for %s", len(vols), name))
	}
}

func (s *uiState) handleVolumeSelection(row int, immediate bool) {
	s.mutex.Lock()
	if s.mode != modeVolumes {
		s.mutex.Unlock()
		return
	}
	var vol *types.ManagedVolume
	if row > 0 && row-1 < len(s.volumes) {
		vol = &s.volumes[row-1]
	}
	s.mutex.Unlock()

	if vol == nil {
		s.queueUpdate(func() {
			s.detailsView.SetText("Select a volume to inspect details")
		})
		return
	}

	s.queueUpdate(func() {
		s.detailsView.SetText(formatVolumeDetails(vol))
	})
}

func (s *uiState) renderVolumeTable(vols []types.ManagedVolume) {
	s.queueUpdate(func() {
		s.serviceTable.SetTitle("Volumes")
		setVolumeTableHeader(s.serviceTable)
		if len(vols) == 0 {
			fillMessageRow(s.serviceTable, len(volumeHeaders), "No volumes found")
			s.detailsView.SetText("Select a volume to inspect details")
			return
		}
		for i, vol := range vols {
			row := i + 1
			color := volumePhaseColor(vol.Status.Phase)
			attachments := "0"
			if vol.Status.AttachmentCount > 0 {
				attachments = fmt.Sprintf("%d", vol.Status.AttachmentCount)
			}
			nameCell := tview.NewTableCell(vol.Name).
				SetSelectable(true).
				SetExpansion(4)
			sizeCell := tview.NewTableCell(defaultIfEmpty(vol.Size, "-")).
				SetExpansion(2)
			statusCell := tview.NewTableCell(defaultIfEmpty(vol.Status.Phase, "-")).
				SetExpansion(2).
				SetTextColor(color)
			attCell := tview.NewTableCell(attachments).
				SetExpansion(2)
			s.serviceTable.SetCell(row, 0, nameCell).
				SetCell(row, 1, sizeCell).
				SetCell(row, 2, statusCell).
				SetCell(row, 3, attCell)
		}
		row, col := s.serviceTable.GetSelection()
		if row <= 0 || row > len(vols) {
			s.serviceTable.Select(1, 0)
		} else {
			s.serviceTable.Select(row, col)
		}
	})
}

func (s *uiState) showVolumeMessage(message string) {
	s.serviceTable.SetTitle("Volumes")
	setVolumeTableHeader(s.serviceTable)
	fillMessageRow(s.serviceTable, len(volumeHeaders), message)
}

func (s *uiState) performVolumeDeletion(clusterName, volumeName string) {
	s.setStatus(fmt.Sprintf("[yellow]Deleting volume %q...", volumeName))
	s.mutex.Lock()
	s.lastSelection = ""
	s.mutex.Unlock()
	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		return
	}
	if err := volume.DeleteVolume(clusterCfg, volumeName); err != nil {
		s.setStatus(fmt.Sprintf("[red]Failed to delete volume %q: %v", volumeName, err))
		return
	}
	s.setStatus(fmt.Sprintf("[green]Volume %q deleted", volumeName))
	s.queueUpdate(func() {
		s.detailsView.SetText("Select a volume to inspect details")
	})
	s.refreshCurrent(context.Background())
}

func (s *uiState) searchVolumes(query string) bool {
	s.mutex.Lock()
	vols := append([]types.ManagedVolume(nil), s.volumes...)
	s.mutex.Unlock()
	for idx, vol := range vols {
		haystack := strings.ToLower(vol.Name + " " + vol.Size + " " + vol.Status.Phase)
		if strings.Contains(haystack, query) {
			row := idx + 1
			s.queueUpdate(func() {
				s.serviceTable.Select(row, 0)
				s.handleVolumeSelection(row, false)
			})
			return true
		}
	}
	return false
}

func (s *uiState) promptCreateVolume() {
	s.mutex.Lock()
	if s.createVolumePromptVisible || s.searchVisible || s.autoRefreshPromptVisible ||
		s.confirmVisible || s.legendVisible || s.pages == nil ||
		s.currentCluster == "" {
		s.mutex.Unlock()
		return
	}
	s.createVolumePromptVisible = true
	s.createVolumeName = ""
	s.createVolumeFocus = s.app.GetFocus()
	container := s.statusContainer
	s.mutex.Unlock()

	input := tview.NewInputField().
		SetLabel("Volume name: ").
		SetFieldWidth(30)
	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			s.handleCreateVolumeName(strings.TrimSpace(input.GetText()))
		case tcell.KeyEscape:
			s.hideCreateVolumePrompt()
		}
	})

	s.queueUpdate(func() {
		container.Clear()
		container.SetTitle("Create Volume")
		input.SetBorder(false)
		container.AddItem(input, 0, 1, true)
	})
	s.app.SetFocus(input)
}

func (s *uiState) handleCreateVolumeName(name string) {
	if name == "" {
		s.setStatus("[red]Volume name cannot be empty")
		return
	}

	s.mutex.Lock()
	s.createVolumeName = name
	container := s.statusContainer
	s.mutex.Unlock()

	input := tview.NewInputField().
		SetLabel("Volume size (e.g. 5Gi): ").
		SetFieldWidth(15)
	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			s.handleCreateVolumeSize(name, strings.TrimSpace(input.GetText()))
		case tcell.KeyEscape:
			s.hideCreateVolumePrompt()
		}
	})

	s.queueUpdate(func() {
		container.Clear()
		container.SetTitle("Create Volume")
		input.SetBorder(false)
		container.AddItem(input, 0, 1, true)
	})
	s.app.SetFocus(input)
}

func (s *uiState) handleCreateVolumeSize(name, size string) {
	if size == "" {
		s.setStatus("[red]Volume size cannot be empty")
		return
	}

	s.hideCreateVolumePrompt()
	s.setStatus(fmt.Sprintf("[yellow]Creating volume %q (%s)...", name, size))

	clusterName := s.currentCluster
	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		return
	}

	go func() {
		_, err := volume.CreateVolume(clusterCfg, name, size)
		if err != nil {
			s.setStatus(fmt.Sprintf("[red]Failed to create volume %q: %v", name, err))
			return
		}
		s.setStatus(fmt.Sprintf("[green]Volume %q created", name))
		s.refreshCurrent(context.Background())
	}()
}

func (s *uiState) hideCreateVolumePrompt() {
	s.mutex.Lock()
	if !s.createVolumePromptVisible {
		s.mutex.Unlock()
		return
	}
	s.createVolumePromptVisible = false
	s.createVolumeName = ""
	focus := s.createVolumeFocus
	s.createVolumeFocus = nil
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
