package tui

import (
	"context"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

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
		s.statusView.SetText(s.decorateStatusText(statusHelpText))
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
