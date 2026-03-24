package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/storage"
)

func (s *uiState) setCurrentBucketObjectsKey(key string) {
	s.mutex.Lock()
	s.currentBucketObjectsKey = key
	s.mutex.Unlock()
}

func makeBucketObjectsKey(clusterName, bucketName string) string {
	return fmt.Sprintf("%s\x00%s", clusterName, bucketName)
}

func (s *uiState) presentBucketObjects(clusterName, bucketName string) {
	if clusterName == "" || bucketName == "" {
		s.showBucketObjectsPrompt("Select a bucket to list objects")
		return
	}
	key := makeBucketObjectsKey(clusterName, bucketName)
	s.mutex.Lock()
	state := s.bucketObjects[key]
	s.mutex.Unlock()
	if state != nil && len(state.Objects) > 0 {
		s.renderBucketObjects(bucketName, state)
		s.updateBucketObjectsStatus(bucketName, state)
		return
	}
	s.showBucketObjectsLoading(bucketName)
	go s.fetchBucketObjects(s.rootCtx, clusterName, bucketName, &bucketObjectRequest{})
}

func (s *uiState) ensureBucketObjectsPaneUnlocked() {
	if s.bucketObjectsVisible {
		return
	}
	s.bucketObjectsVisible = true
	s.detailContainer.AddItem(s.bucketObjectsTable, 0, 2, false)
}

func (s *uiState) hideBucketObjectsPane() {
	s.queueUpdate(func() {
		if !s.bucketObjectsVisible {
			return
		}
		s.bucketObjectsVisible = false
		s.detailContainer.RemoveItem(s.bucketObjectsTable)
	})
}

func (s *uiState) focusBucketObjectsTable() {
	s.queueUpdate(func() {
		s.ensureBucketObjectsPaneUnlocked()
		rowCount := s.bucketObjectsTable.GetRowCount()
		if rowCount > 1 {
			row, _ := s.bucketObjectsTable.GetSelection()
			if row <= 0 || row >= rowCount {
				s.bucketObjectsTable.Select(1, 0)
			}
		}
		s.app.SetFocus(s.bucketObjectsTable)
	})
}

func (s *uiState) showBucketObjectsPrompt(message string) {
	s.queueUpdate(func() {
		s.ensureBucketObjectsPaneUnlocked()
		s.bucketObjectsTable.SetTitle("Bucket Objects")
		setBucketObjectTableHeader(s.bucketObjectsTable)
		fillMessageRow(s.bucketObjectsTable, len(bucketObjectHeaders), message)
		s.bucketObjectsTable.Select(0, 0)
	})
}

func (s *uiState) showBucketObjectsLoading(bucketName string) {
	title := "Bucket Objects"
	if bucketName != "" {
		title = fmt.Sprintf("Bucket Objects (%s)", bucketName)
	}
	s.queueUpdate(func() {
		s.ensureBucketObjectsPaneUnlocked()
		s.bucketObjectsTable.SetTitle(title)
		setBucketObjectTableHeader(s.bucketObjectsTable)
		fillMessageRow(s.bucketObjectsTable, len(bucketObjectHeaders), "Loading objects…")
		s.bucketObjectsTable.Select(0, 0)
	})
}

func (s *uiState) showBucketObjectsError(bucketName string) {
	title := "Bucket Objects"
	if bucketName != "" {
		title = fmt.Sprintf("Bucket Objects (%s)", bucketName)
	}
	s.queueUpdate(func() {
		s.ensureBucketObjectsPaneUnlocked()
		s.bucketObjectsTable.SetTitle(title)
		setBucketObjectTableHeader(s.bucketObjectsTable)
		fillMessageRow(s.bucketObjectsTable, len(bucketObjectHeaders), "Unable to load objects")
		s.bucketObjectsTable.Select(0, 0)
	})
}

func (s *uiState) renderBucketObjects(bucketName string, state *bucketObjectState) {
	if state == nil {
		s.showBucketObjectsPrompt("Select a bucket to list objects")
		return
	}
	title := "Bucket Objects"
	if bucketName != "" {
		title = fmt.Sprintf("Bucket Objects (%s)", bucketName)
	}
	if state.Auto {
		title += " [all]"
	}
	s.queueUpdate(func() {
		s.ensureBucketObjectsPaneUnlocked()
		s.bucketObjectsTable.SetTitle(title)
		setBucketObjectTableHeader(s.bucketObjectsTable)
		if len(state.Objects) == 0 {
			fillMessageRow(s.bucketObjectsTable, len(bucketObjectHeaders), "No objects found")
			s.bucketObjectsTable.Select(0, 0)
			return
		}
		for i, obj := range state.Objects {
			row := i + 1
			lastModified := "-"
			if !obj.LastModified.IsZero() {
				lastModified = obj.LastModified.Format("2006-01-02 15:04:05")
			}
			s.bucketObjectsTable.SetCell(row, 0, tview.NewTableCell(obj.Name).
				SetSelectable(true).
				SetExpansion(5)).
				SetCell(row, 1, tview.NewTableCell(strconv.FormatInt(obj.Size, 10)).
					SetSelectable(false).
					SetExpansion(2)).
				SetCell(row, 2, tview.NewTableCell(lastModified).
					SetSelectable(false).
					SetExpansion(3))
		}
		row, _ := s.bucketObjectsTable.GetSelection()
		if row <= 0 || row > len(state.Objects) {
			s.bucketObjectsTable.Select(1, 0)
		}
	})
}

func (s *uiState) updateBucketObjectsStatus(bucketName string, state *bucketObjectState) {
	if state == nil {
		return
	}
	count := len(state.Objects)
	if state.Auto {
		s.setStatus(fmt.Sprintf("[green]Loaded %d object(s) from %s", count, bucketName))
		return
	}
	if state.NextPage != "" && state.IsTruncated {
		msg := fmt.Sprintf("[yellow]%s: showing %d object(s). Press 'n' for next page", bucketName, count)
		if len(state.PrevTokens) > 0 {
			msg += ", 'p' for previous"
		}
		s.setStatus(msg)
		return
	}
	if len(state.PrevTokens) > 0 {
		s.setStatus(fmt.Sprintf("[green]%s: showing %d object(s). Press 'p' for previous page", bucketName, count))
		return
	}
	s.setStatus(fmt.Sprintf("[green]%s: showing %d object(s)", bucketName, count))
}

func (s *uiState) currentBucketSelection() (string, *storage.BucketInfo) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.mode != modeBuckets {
		return "", nil
	}
	clusterName := s.currentCluster
	row, _ := s.serviceTable.GetSelection()
	if row <= 0 || row-1 >= len(s.bucketInfos) {
		return clusterName, nil
	}
	return clusterName, s.bucketInfos[row-1]
}

func (s *uiState) reloadBucketObjects(ctx context.Context) {
	clusterName, bucket := s.currentBucketSelection()
	if clusterName == "" || bucket == nil {
		s.setStatus("[yellow]Select a bucket to reload objects")
		return
	}
	s.setCurrentBucketObjectsKey(makeBucketObjectsKey(clusterName, bucket.Name))
	s.showBucketObjectsLoading(bucket.Name)
	go s.fetchBucketObjects(ctx, clusterName, bucket.Name, &bucketObjectRequest{})
}

func (s *uiState) nextBucketObjectsPage(ctx context.Context) {
	clusterName, bucket := s.currentBucketSelection()
	if clusterName == "" || bucket == nil {
		s.setStatus("[yellow]Select a bucket to load more objects")
		return
	}
	key := makeBucketObjectsKey(clusterName, bucket.Name)
	s.mutex.Lock()
	state := s.bucketObjects[key]
	s.mutex.Unlock()
	if state == nil || state.NextPage == "" {
		s.setStatus(fmt.Sprintf("[yellow]No additional objects for %s", bucket.Name))
		return
	}
	prevTokens := append([]string(nil), state.PrevTokens...)
	prevTokens = append(prevTokens, state.CurrentToken)
	s.setCurrentBucketObjectsKey(key)
	s.showBucketObjectsLoading(bucket.Name)
	go s.fetchBucketObjects(ctx, clusterName, bucket.Name, &bucketObjectRequest{
		Token:      state.NextPage,
		PrevTokens: prevTokens,
	})
}

func (s *uiState) previousBucketObjectsPage(ctx context.Context) {
	clusterName, bucket := s.currentBucketSelection()
	if clusterName == "" || bucket == nil {
		s.setStatus("[yellow]Select a bucket to load previous objects")
		return
	}
	key := makeBucketObjectsKey(clusterName, bucket.Name)
	s.mutex.Lock()
	state := s.bucketObjects[key]
	s.mutex.Unlock()
	if state == nil || len(state.PrevTokens) == 0 {
		s.setStatus(fmt.Sprintf("[yellow]%s is already at the first page", bucket.Name))
		return
	}
	prevTokens := append([]string(nil), state.PrevTokens...)
	token := prevTokens[len(prevTokens)-1]
	prevTokens = prevTokens[:len(prevTokens)-1]
	s.setCurrentBucketObjectsKey(key)
	s.showBucketObjectsLoading(bucket.Name)
	go s.fetchBucketObjects(ctx, clusterName, bucket.Name, &bucketObjectRequest{
		Token:      token,
		PrevTokens: prevTokens,
	})
}

func (s *uiState) loadAllBucketObjects(ctx context.Context) {
	clusterName, bucket := s.currentBucketSelection()
	if clusterName == "" || bucket == nil {
		s.setStatus("[yellow]Select a bucket to load all objects")
		return
	}
	s.setCurrentBucketObjectsKey(makeBucketObjectsKey(clusterName, bucket.Name))
	s.showBucketObjectsLoading(bucket.Name)
	go s.fetchBucketObjects(ctx, clusterName, bucket.Name, &bucketObjectRequest{
		Token: "",
		Auto:  true,
	})
}

func (s *uiState) fetchBucketObjects(ctx context.Context, clusterName, bucketName string, req *bucketObjectRequest) {
	if req == nil {
		req = &bucketObjectRequest{}
	}
	clusterCfg := s.conf.Oscar[clusterName]
	if clusterCfg == nil {
		s.setStatus(fmt.Sprintf("[red]Cluster %q configuration not found", clusterName))
		s.showBucketObjectsError(bucketName)
		return
	}

	opts := &storage.BucketListOptions{
		PageToken:    strings.TrimSpace(req.Token),
		AutoPaginate: req.Auto,
	}
	key := makeBucketObjectsKey(clusterName, bucketName)

	s.mutex.Lock()
	if s.bucketObjectsCancel != nil {
		s.bucketObjectsCancel()
	}
	s.bucketObjectsSeq++
	seq := s.bucketObjectsSeq
	ctxFetch, cancel := context.WithTimeout(ctx, 20*time.Second)
	s.bucketObjectsCancel = cancel
	s.mutex.Unlock()

	result, err := storage.ListBucketObjectsWithOptionsContext(ctxFetch, clusterCfg, bucketName, opts)
	cancel()

	if err != nil {
		s.mutex.Lock()
		if seq == s.bucketObjectsSeq {
			s.bucketObjectsCancel = nil
		}
		activeKey := s.currentBucketObjectsKey
		s.mutex.Unlock()
		s.setStatus(fmt.Sprintf("[red]Unable to load objects for %s: %v", bucketName, err))
		if activeKey == key {
			s.showBucketObjectsError(bucketName)
		}
		return
	}

	if result == nil {
		result = &storage.BucketListResult{}
	}
	state := &bucketObjectState{
		Objects:       append([]*storage.BucketObject(nil), result.Objects...),
		NextPage:      result.NextPage,
		PrevTokens:    append([]string(nil), req.PrevTokens...),
		CurrentToken:  opts.PageToken,
		IsTruncated:   result.IsTruncated,
		Auto:          opts.AutoPaginate,
		ReturnedItems: result.ReturnedItems,
	}
	if state.Objects == nil {
		state.Objects = []*storage.BucketObject{}
	}

	s.mutex.Lock()
	if seq != s.bucketObjectsSeq {
		s.mutex.Unlock()
		return
	}
	s.bucketObjectsCancel = nil
	s.bucketObjects[key] = state
	activeKey := s.currentBucketObjectsKey
	s.mutex.Unlock()

	if activeKey == key {
		s.renderBucketObjects(bucketName, state)
		s.updateBucketObjectsStatus(bucketName, state)
	}
}
