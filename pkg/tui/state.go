package tui

import (
	"context"
	"sync"
	"time"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/grycap/oscar/v4/pkg/types"
)

const legendText = `[yellow]Navigation[-]
  ↑/↓  Move selection
  ←/→ or Tab  Switch pane
  f  Focus details panel

[yellow]Actions[-]
  r  Refresh current view
  d or Del  Delete selected service/bucket/volume/object
  i  Show cluster info
  l  Open logs panel
  w  Configure auto refresh
  b  Switch to buckets view
  s  Switch to services view
  v  Switch to volumes view
  c  Create bucket/volume
  u  Upload file (bucket objects view)
  m  Show cluster metrics
  e  Show cluster status
  Enter  Focus bucket objects (bucket view)
  o  Reload bucket objects (bucket view)
  n/p  Next/previous bucket objects page
  a  Load all bucket objects
  q  Quit
  ?  Toggle this help`

const statusHelpText = "[yellow]Keys: [::b]q[::-] Quit · [::b]r[::-] Refresh · [::b]d/Del[::-] Delete svc/bucket/volume/object · [::b]i[::-] Cluster info · [::b]l[::-] Logs panel · [::b]w[::-] Auto refresh · [::b]b[::-] Buckets · [::b]s[::-] Services · [::b]v[::-] Volumes · [::b]c[::-] Create bucket/volume · [::b]u[::-] Upload file · [::b]m[::-] Metrics · [::b]e[::-] Status · [::b]g[::-] Quota · [::b]k[::-] Update quota · [::b]t[::-] Deploy status · [::b]h[::-] Deploy logs · [::b]f[::-] Focus details · [::b]Enter/n/p/a/o[::-] Bucket objects · [::b]?[::-] Help · [::b]←/→[::-] Switch pane · [::b]/[::-] Search"

type panelMode int

const (
	modeServices panelMode = iota
	modeBuckets
	modeLogs
	modeVolumes
)

var (
	serviceHeaders      = []string{"Name", "Image", "CPU", "Memory"}
	bucketHeaders       = []string{"Name", "Visibility", "Owner"}
	bucketObjectHeaders = []string{"Name", "Size (B)", "Last Modified"}
	logHeaders          = []string{"Job", "Status", "Started", "Finished"}
	volumeHeaders       = []string{"Name", "Size", "Status", "Attachments"}
)

type searchTarget int

const (
	searchTargetNone searchTarget = iota
	searchTargetClusters
	searchTargetServices
	searchTargetLogs
	searchTargetBuckets
	searchTargetVolumes
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
	volumes                  []types.ManagedVolume
	volumeCancel             context.CancelFunc
	volumeSeq                int
	volumeCluster            string
	createVolumePromptVisible bool
	createVolumeName         string
	createVolumeFocus        tview.Primitive
	createBucketPromptVisible bool
	createBucketName         string
	createBucketFocus        tview.Primitive
	putFilePromptVisible     bool
	putFileFocus             tview.Primitive
	quotaPromptVisible       bool
	quotaFocus               tview.Primitive
	updateQuotaPromptVisible bool
	updateQuotaStep          int
	updateQuotaUserID        string
	updateQuotaCPU           string
	updateQuotaVolumeDisk    string
	updateQuotaVolumeCount   string
	updateQuotaFocus         tview.Primitive
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
	inputHandling            int32
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
