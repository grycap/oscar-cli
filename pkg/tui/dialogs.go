package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

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
