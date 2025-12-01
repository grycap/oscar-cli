package tui

import (
	"context"
	"sync"
	"time"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/grycap/oscar/v3/pkg/types"
)

const legendText = `[yellow]Navigation[-]
  ↑/↓  Move selection
  ←/→ or Tab  Switch pane
  v  Focus details panel

[yellow]Actions[-]
  r  Refresh current view
  d  Delete selected item
  i  Show cluster info
  l  Open logs panel
  w  Configure auto refresh
  b  Switch to buckets view
  s  Switch to services view
  Enter  Focus bucket objects (bucket view)
  o  Reload bucket objects (bucket view)
  n/p  Next/previous bucket objects page
  a  Load all bucket objects
  q  Quit
  ?  Toggle this help`

const statusHelpText = "[yellow]Keys: [::b]q[::-] Quit · [::b]r[::-] Refresh · [::b]d[::-] Delete selection · [::b]i[::-] Cluster info · [::b]l[::-] Logs panel · [::b]w[::-] Auto refresh · [::b]b[::-] Buckets · [::b]s[::-] Services · [::b]v[::-] Focus details · [::b]Enter/n/p/a/o[::-] Bucket objects · [::b]?[::-] Help · [::b]←/→[::-] Switch pane · [::b]/[::-] Search"

type panelMode int

const (
	modeServices panelMode = iota
	modeBuckets
	modeLogs
)

var (
	serviceHeaders      = []string{"Name", "Image", "CPU", "Memory"}
	bucketHeaders       = []string{"Name", "Visibility", "Owner"}
	bucketObjectHeaders = []string{"Name", "Size (B)", "Last Modified"}
	logHeaders          = []string{"Job", "Status", "Started", "Finished"}
)

type searchTarget int

const (
	searchTargetNone searchTarget = iota
	searchTargetClusters
	searchTargetServices
	searchTargetLogs
	searchTargetBuckets
	searchTargetDetails
)

type uiState struct {
	app                *tview.Application
	conf               *config.Config
	rootCtx            context.Context
	statusView         *tview.TextView
	detailsView        *tview.TextView
	detailContainer    *tview.Flex
	serviceTable       *tview.Table
	bucketObjectsTable *tview.Table
	clusterList        *tview.List
	statusContainer    *tview.Flex
	pages              *tview.Pages
	mutex              *sync.Mutex

	clusterNames             []string
	currentCluster           string
	currentServices          []*types.Service
	refreshing               bool
	started                  bool
	pendingCluster           string
	loadingCluster           string
	failedClusters           map[string]string
	loadCancel               context.CancelFunc
	loadSeq                  int
	detailTimer              *time.Timer
	lastSelection            string
	legendVisible            bool
	confirmVisible           bool
	savedFocus               tview.Primitive
	mode                     panelMode
	bucketInfos              []*storage.BucketInfo
	bucketCancel             context.CancelFunc
	bucketSeq                int
	bucketCluster            string
	bucketObjectsVisible     bool
	bucketObjects            map[string]*bucketObjectState
	currentBucketObjectsKey  string
	bucketObjectsCancel      context.CancelFunc
	bucketObjectsSeq         int
	searchVisible            bool
	searchInput              *tview.InputField
	searchTarget             searchTarget
	originalFocus            tview.Primitive
	serviceDefinitions       map[string]string
	serviceDefinitionSeq     int
	currentServiceDefinition string
	autoRefreshCancel        context.CancelFunc
	autoRefreshTicker        *time.Ticker
	autoRefreshPeriod        time.Duration
	autoRefreshActive        bool
	autoRefreshPromptVisible bool
	autoRefreshInput         *tview.InputField
	autoRefreshFocus         tview.Primitive
	servicePanelVisited      bool
	logEntries               []*logEntry
	logDetails               map[string]string
	logSeq                   int
	logDetailSeq             int
	currentLogsKey           string
	currentLogJobKey         string
	currentLogService        string
	currentLogCluster        string
}

type bucketObjectState struct {
	Objects       []*storage.BucketObject
	NextPage      string
	PrevTokens    []string
	CurrentToken  string
	IsTruncated   bool
	Auto          bool
	ReturnedItems int
}

type bucketObjectRequest struct {
	Token      string
	PrevTokens []string
	Auto       bool
}
