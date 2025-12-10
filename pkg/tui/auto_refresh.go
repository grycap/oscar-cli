package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (s *uiState) promptAutoRefresh() {
	s.mutex.Lock()
	if s.autoRefreshPromptVisible || s.searchVisible || s.confirmVisible || s.legendVisible || s.pages == nil {
		s.mutex.Unlock()
		return
	}
	s.autoRefreshPromptVisible = true
	s.autoRefreshFocus = s.app.GetFocus()
	prevPeriod := s.autoRefreshPeriod
	container := s.statusContainer
	s.mutex.Unlock()

	input := tview.NewInputField().
		SetLabel("Auto refresh seconds (0 to stop, default 10): ").
		SetFieldWidth(10)
	input.SetAcceptanceFunc(func(text string, last rune) bool {
		if last == 0 {
			return true
		}
		return last >= '0' && last <= '9'
	})
	if prevPeriod > 0 {
		input.SetText(fmt.Sprintf("%d", int(prevPeriod/time.Second)))
	}
	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			s.handleAutoRefreshInput(input.GetText())
		case tcell.KeyEscape:
			s.hideAutoRefreshPrompt()
		}
	})

	s.mutex.Lock()
	s.autoRefreshInput = input
	s.mutex.Unlock()

	s.queueUpdate(func() {
		container.Clear()
		container.SetTitle("Auto Refresh")
		input.SetBorder(false)
		container.AddItem(input, 0, 1, true)
	})
	s.app.SetFocus(input)
}

func (s *uiState) hideAutoRefreshPrompt() {
	s.mutex.Lock()
	if !s.autoRefreshPromptVisible {
		s.mutex.Unlock()
		return
	}
	s.autoRefreshPromptVisible = false
	input := s.autoRefreshInput
	s.autoRefreshInput = nil
	focus := s.autoRefreshFocus
	s.autoRefreshFocus = nil
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

func (s *uiState) handleAutoRefreshInput(value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		s.hideAutoRefreshPrompt()
		s.startAutoRefresh(10 * time.Second)
		s.setStatus("[green]Auto refresh every 10 second(s)")
		return
	}
	seconds, err := strconv.Atoi(trimmed)
	if err != nil {
		s.setStatus("[red]Enter a valid number of seconds")
		return
	}
	if seconds < 0 {
		s.setStatus("[red]Refresh period must not be negative")
		return
	}

	s.hideAutoRefreshPrompt()
	if seconds == 0 {
		if s.stopAutoRefresh() {
			s.setStatus("[yellow]Auto refresh disabled")
		} else {
			s.setStatus("[yellow]Auto refresh already disabled")
		}
		return
	}

	period := time.Duration(seconds) * time.Second
	s.startAutoRefresh(period)
	s.setStatus(fmt.Sprintf("[green]Auto refresh every %d second(s)", seconds))
}

func (s *uiState) startAutoRefresh(period time.Duration) {
	if period <= 0 {
		s.stopAutoRefresh()
		return
	}
	// Ensure previous ticker is stopped.
	s.stopAutoRefresh()

	parent := context.Background()
	s.mutex.Lock()
	if s.rootCtx != nil {
		parent = s.rootCtx
	}
	s.mutex.Unlock()

	ctx, cancel := context.WithCancel(parent)
	ticker := time.NewTicker(period)

	s.mutex.Lock()
	s.autoRefreshCancel = cancel
	s.autoRefreshTicker = ticker
	s.autoRefreshPeriod = period
	s.autoRefreshActive = true
	s.mutex.Unlock()

	go func() {
		s.refreshCurrent(context.Background())
		for {
			select {
			case <-ticker.C:
				s.refreshCurrent(context.Background())
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *uiState) stopAutoRefresh() bool {
	s.mutex.Lock()
	cancel := s.autoRefreshCancel
	active := s.autoRefreshActive
	s.autoRefreshCancel = nil
	s.autoRefreshTicker = nil
	s.autoRefreshPeriod = 0
	s.autoRefreshActive = false
	s.mutex.Unlock()

	if cancel != nil {
		cancel()
	}
	return active
}

func (s *uiState) decorateStatusText(base string) string {
	text := base
	s.mutex.Lock()
	active := s.autoRefreshActive
	period := s.autoRefreshPeriod
	s.mutex.Unlock()
	if active && period > 0 {
		seconds := int(period / time.Second)
		if seconds <= 0 {
			seconds = 1
		}
		indicator := fmt.Sprintf("[cyan]Auto refresh: every %d second(s)", seconds)
		if strings.TrimSpace(text) == "" {
			text = indicator
		} else {
			text = text + "\n" + indicator
		}
	}
	return text
}
