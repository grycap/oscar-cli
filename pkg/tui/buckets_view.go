package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/storage"
)

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
	s.currentBucketObjectsKey = ""
	s.mutex.Unlock()

	clusterName := s.currentCluster
	if clusterName == "" {
		s.setStatus("[red]Select a cluster to view buckets")
		s.queueUpdate(func() {
			s.showBucketMessage("Select a cluster to view buckets")
		})
		s.showBucketObjectsPrompt("Select a bucket to list objects")
		s.showClusterDetails(clusterName)
		return
	}

	s.showClusterDetails(clusterName)
	s.queueUpdate(func() {
		s.showBucketMessage("Loading buckets…")
	})
	s.showBucketObjectsPrompt("Select a bucket to list objects")

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

func (s *uiState) handleBucketSelection(row int, immediate bool) {
	s.mutex.Lock()
	if s.mode != modeBuckets {
		s.mutex.Unlock()
		return
	}
	clusterName := s.currentCluster
	var bucket *storage.BucketInfo
	if row > 0 && row-1 < len(s.bucketInfos) {
		bucket = s.bucketInfos[row-1]
	}
	s.mutex.Unlock()

	if bucket == nil {
		s.queueUpdate(func() {
			s.detailsView.SetText("Select a bucket to inspect details")
		})
		s.setCurrentBucketObjectsKey("")
		s.showBucketObjectsPrompt("Select a bucket to list objects")
		return
	}

	s.queueUpdate(func() {
		s.detailsView.SetText(formatBucketDetails(bucket))
	})
	s.setCurrentBucketObjectsKey(makeBucketObjectsKey(clusterName, bucket.Name))
	s.presentBucketObjects(clusterName, bucket.Name)
	if immediate {
		s.focusBucketObjectsTable()
	}
}

func (s *uiState) renderBucketTable(buckets []*storage.BucketInfo) {
	s.queueUpdate(func() {
		s.serviceTable.SetTitle("Buckets")
		setBucketTableHeader(s.serviceTable)
		if len(buckets) == 0 {
			fillMessageRow(s.serviceTable, len(bucketHeaders), "No buckets found")
			s.detailsView.SetText("Select a bucket to inspect details")
			s.showBucketObjectsPrompt("Select a bucket to list objects")
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

func (s *uiState) showBucketMessage(message string) {
	s.serviceTable.SetTitle("Buckets")
	setBucketTableHeader(s.serviceTable)
	fillMessageRow(s.serviceTable, len(bucketHeaders), message)
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

func (s *uiState) searchBuckets(query string) bool {
	s.mutex.Lock()
	buckets := append([]*storage.BucketInfo(nil), s.bucketInfos...)
	s.mutex.Unlock()
	for idx, bucket := range buckets {
		if bucket == nil {
			continue
		}
		haystack := strings.ToLower(bucket.Name + " " + bucket.Owner + " " + bucket.Visibility)
		if strings.Contains(haystack, query) {
			row := idx + 1
			s.queueUpdate(func() {
				s.serviceTable.Select(row, 0)
				s.handleBucketSelection(row, false)
			})
			return true
		}
	}
	return false
}
