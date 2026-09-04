# oscar-cli TUI — Agent Context

## Project Overview
oscar-cli is a Go CLI tool with a TUI mode (`--interactive`). The TUI is built with [tview](https://github.com/rivo/tview) and [tcell](https://github.com/gdamore/tcell/v2). It displays Oscar clusters (left panel), a main table (services/buckets/volumes/logs), and a details panel (right). A status bar at the bottom shows prompts and a status line.

## Architecture
- **Entry point**: `Run(ctx, conf)` in `pkg/tui/app.go`
- **State struct**: `uiState` in `pkg/tui/state.go`
- **Modes**: `modeServices`, `modeBuckets`, `modeLogs`, `modeVolumes` (iota in state.go)
- **Panels**: `clusterList` (left), `serviceTable` (center), `detailsView` (right), `bucketObjectsTable` (sub-table in buckets mode), `statusContainer` (bottom bar)
- **Concurrency**: All UI reads/writes from goroutines use `s.mutex.Lock()`. Async work uses `defer/recover` for panic safety. UI updates use `s.queueUpdate(fn)`. Input handling is serialized via `atomic.CompareAndSwapInt32(&s.inputHandling, 0, 1)`.
- **SDK**: Oscar API types come from `github.com/grycap/oscar/v4/pkg/types`. Cluster config from `github.com/grycap/oscar-cli/pkg/cluster`. Service operations from `github.com/grycap/oscar-cli/pkg/service`. Storage from `github.com/grycap/oscar-cli/pkg/storage`.

## File Map

| File | Purpose |
|------|---------|
| `pkg/tui/app.go` | Main Run function, handleInput, all async show-* methods, quota wizard (5 steps), deploy status/logs |
| `pkg/tui/state.go` | State struct, panel modes, constants, legendText, statusHelpText |
| `pkg/tui/commands.go` | Command palette (`:`): registry, autocomplete, executeCommand |
| `pkg/tui/search.go` | Search (`/`): initiateSearch (auto-detects target based on focus+mode), show/hide/handleSearchInput |
| `pkg/tui/dialogs.go` | `legendTextForMode(mode)`, `toggleLegend`, `showLegendUnlocked`, `requestDeletion` |
| `pkg/tui/formatters.go` | formatQuota, formatDeploymentStatus, formatDeploymentLogs, formatMetricsSummary, formatServiceDetails, formatVolumeDetails, formatBucketDetails, formatClusterConfig, formatServiceLogs |
| `pkg/tui/buckets_view.go` | Bucket mode rendering, promptCreateBucket |
| `pkg/tui/volumes_view.go` | Volume mode rendering, promptCreateVolume |
| `pkg/tui/services_view.go` | Services mode rendering |
| `pkg/tui/logs_view.go` | Logs mode rendering |
| `pkg/cluster/quotas.go` | GetQuota(cfg, userID), UpdateQuota(cfg, userID, cpu, mem, volUpdate) |
| `pkg/service/deployment.go` | GetDeploymentStatus(cfg, name), GetDeploymentLogs(cfg, name) |

## Key Bindings (as of May 2026)

### General
- `q` — Quit
- `r` — Refresh current view
- `i` — Show cluster info
- `e` — Show cluster status
- `m` — Show cluster metrics
- `w` — Configure auto-refresh
- `b` — Switch to buckets view
- `s` — Switch to services view
- `v` — Switch to volumes view
- `f` — Focus details panel (toggle)
- `?` — Toggle help/legend (mode-specific keybindings shown)
- `:` — Open command palette
- `/` — Search (scoped to current mode)
- `←/→/Tab` — Switch pane
- `↑/↓` — Move selection

### Services mode
- `d/Del` — Delete selected service
- `l` — Open logs panel
- `g` — Get quota (if basic auth, prompts userID; otherwise fetches directly)
- `k` — Update quota (5-step wizard, only for basic auth)
- `t` — Show deployment status
- `h` — Show deployment logs
- `Enter` — Focus service details → logs

### Volumes mode
- `c` — Create volume
- `d/Del` — Delete volume

### Buckets mode
- `c` — Create bucket
- `d/Del` — Delete bucket
- `Enter` — Focus bucket objects
- `u` — Upload file (from bucket objects view)
- `o` — Reload bucket objects
- `n/p` — Next/previous page
- `a` — Load all objects

### Logs mode
- `l` — Open logs panel
- `d/Del` — Delete log

## Command Palette (`:`)

- Shows autocomplete with all commands sorted alphabetically.
- Blank entry (empty string) at top of suggestions when input is empty — pressing Enter on empty input closes the palette without executing.
- Backspace on empty input dismisses the palette.
- Enter with text executes the best match.
- User types with spaces, internal names use dots (e.g. "quota update" → `quota.update`). `executeCommand` normalizes.

### Registered Commands
```
help, refresh, cluster.info, cluster.status, cluster.metrics,
quota.get, quota.update, deploy.status, deploy.logs,
search, services, volumes, volume.create,
buckets, bucket.create
```

## Search (`/`)

`initiateSearch(ctx)` auto-detects target:
1. Based on focused element (clusterList → clusters, serviceTable → depends on mode, detailsView → details, bucketObjectsTable → buckets)
2. If target is clusters but mode is buckets/volumes/services, overrides to search that mode's data
3. Fallback: clusters if still none

Search targets: `searchTargetNone`, `searchTargetClusters`, `searchTargetServices`, `searchTargetLogs`, `searchTargetBuckets`, `searchTargetVolumes`, `searchTargetDetails`

## Quota System

- **Get**: `cluster.GetQuota(cfg, userID)` → GET `{endpoint}/system/quotas/user/{userID}`
  - If cluster has `auth_user`/`auth_password` (basic auth): prompts for userID first. Empty userID shows error.
  - Otherwise: fetches directly with empty userID.
  - Displayed via `formatQuota(name, quota)` showing resources (sorted, color-coded) and volume quotas.
- **Update**: `cluster.UpdateQuota(cfg, userID, cpu, mem, volUpdate)` → PUT `{endpoint}/system/quotas/user/{userID}`
  - Only available for basic auth clusters.
  - `promptUpdateQuota()` enters a 5-step wizard (userID → CPU → Memory → Volume Disk → Volume Count).
  - Each step input has `SetInputCapture` for Backspace → previous step.
  - Backspace on step 0 (userID) dismisses wizard.
  - `VolumeQuotaUpdate` struct with `Disk`, `Volumes`, `MaxDiskperVolume`, `MinDiskperVolume` (strings, omitempty).
  - If both volume disk and count are empty, `volUpdate` param is nil.
  - Also accessible via command palette `quota.update` with inline args: `quota update user cpu mem vol-disk vol-count` (no args → interactive wizard).

## Deploy Status/Logs

- **Status**: `service.GetDeploymentStatus(cfg, name)` → GET `{endpoint}/.../{name}/deployment`
  - Formatted by `formatDeploymentStatus(name, ds)` showing state (color-coded: green=yellow/red/blue), reason, last transition time, instances.
  - Guard nil `*metav1.Time` before formatting.
- **Logs**: `service.GetDeploymentLogs(cfg, name)` → GET `{endpoint}/.../{name}/deployment/logs`
  - Formatted by `formatDeploymentLogs(dl)` showing service name, available flag, message, entries.

## Important Patterns

### Adding a new keyboard shortcut
1. Add the case in `handleInput` (app.go ~line 556-741).
2. Update `legendTextForMode(mode)` in dialogs.go and `statusHelpText` in state.go (if global) or per-mode string.
3. Add guard check in input handling chain (e.g. `if s.somePromptVisible` before the switch).

### Adding a new command palette command
1. Add a `registerCommand(...)` call in `initCommands()` (commands.go ~line 27-118).
2. Register alphabetically within the file.

### Adding a new prompt/wizard
1. Add visible flag and saved-focus field to `uiState` in state.go.
2. Add guard in `handleInput` so the prompt intercepts keys before global bindings.
3. Create `promptShowXxx()` with `tview.NewInputField()`, `SetDoneFunc`, `SetInputCapture`.
4. Create `hideXxx()` that restores focus and clears container.
5. Optionally add Backspace-on-empty handling in `handleInput` at the `tcell.KeyBackspace` case.

### Prompt visibility guard order (in handleInput, app.go)
```
searchVisible → autoRefreshPromptVisible → createVolumePromptVisible →
createBucketPromptVisible → putFilePromptVisible → quotaPromptVisible →
updateQuotaPromptVisible → commandPaletteVisible
```

### Backspace handling
In the `tcell.KeyBackspace` case of `handleInput`, if the focused element is an `*tview.InputField` with empty text, dismiss the active prompt. Handles: quotaPrompt, updateQuotaPrompt, createVolumePrompt, createBucketPrompt, putFilePrompt, commandPalette, autoRefreshPrompt.
In quota update wizard steps, per-step `SetInputCapture` handles Backspace for step-back navigation.

### Async goroutine pattern
```go
go func(...) {
    defer func() {
        if r := recover(); r != nil {
            errMsg := fmt.Sprintf("unexpected error: %v", r)
            s.setStatus(fmt.Sprintf("[red]...: %s", errMsg))
            s.queueUpdate(func() {
                s.detailsView.SetText(fmt.Sprintf("[red]...: %s[-]", errMsg))
            })
        }
    }()
    result, err := someAPI(...)
    if err != nil {
        s.setStatus(...)
        s.queueUpdate(func() { ... })
        return
    }
    text := someFormatter(...)
    s.queueUpdate(func() {
        s.detailsView.SetText(text)
    })
}(...)
```

## types Package (oscar/v4)

Key types used:
- `types.Service` — service definition (Name, Image, CPU, Memory, Replicas, LogLevel)
- `types.QuotaResponse` — `UserID`, `ClusterQueue`, `Resources map[string]QuotaValues` (Max/Used int64), `Volumes *VolumeQuotaResponse`
- `types.VolumeQuotaResponse` — `Disk`, `Volumes` (both have Max/Used string), `MaxDiskperVolume`, `MinDiskperVolume`
- `types.QuotaUpdateRequest` — `CPU`, `Memory` (strings), `Volumes *VolumeQuotaUpdate`
- `types.VolumeQuotaUpdate` — `Disk`, `Volumes`, `MaxDiskperVolume`, `MinDiskperVolume` (strings, omitempty)
- `types.ServiceDeploymentSummary` — `State` (DeploymentState enum), `Reason`, `LastTransitionTime *metav1.Time`, `ActiveInstances`, `AffectedInstances`, `ResourceKind`
- `types.DeploymentLogStream` — `ServiceName`, `Available bool`, `Message`, `Entries []DeploymentLogEntry` (Timestamp, Message)
- `types.MetricsSummaryResponse` — `Start/End time.Time`, `Totals` (ServicesCountActive/Total, CPUHoursTotal, GPUHoursTotal, RequestsCount*, UsersCount, CountriesCount), `Sources []MetricsSummarySource`
- `types.ManagedVolume` — Name, Size, Status, Namespace, PVCName, OwnerUser, CreationMode, LifecyclePolicy, CreatedByService, Attachments
- `types.Info` — Version, GitCommit, Architecture, KubeVersion, ServerlessBackendInfo
- `types.JobsResponse` — list of jobs with pagination
- `types.Replica` — federation replica definition

---

# Non-TUI Project Structure

## Entry Point
`main.go` → `cmd.Execute()` (from `cmd/root.go`)

## CLI Framework
Uses **`github.com/spf13/cobra`**. All commands in `cmd/` package.

### Complete Command Tree
```
oscar-cli
├── version
├── cluster                         (cluster.go)
│   ├── add IDENTIFIER ENDPOINT     (cluster_add.go)
│   ├── delete IDENTIFIER           (cluster_remove.go)
│   ├── info                        (cluster_info.go)
│   ├── status                      (cluster_status.go)
│   ├── list                        (cluster_list.go)
│   └── default                     (cluster_default.go)
├── service                         (service.go)
│   ├── get SERVICE_NAME            (service_get.go)
│   ├── list                        (service_list.go)
│   ├── delete SERVICE_NAME...      (service_delete.go)
│   ├── run SERVICE_NAME            (service_run.go)
│   ├── job SERVICE_NAME            (service_job.go)
│   ├── logs                        (service_logs.go)
│   │   ├── list SERVICE_NAME       (service_logs_list.go)
│   │   ├── get SERVICE_NAME [JOB]  (service_logs_get.go)
│   │   └── delete SERVICE_NAME...  (service_logs_remove.go)
│   ├── get-file SERVICE_NAME ...   (service_getfile.go)
│   ├── put-file SERVICE_NAME ...   (service_putfile.go)
│   ├── delete-file SERVICE_NAME... (service_deletefile.go)
│   ├── list-files SERVICE_NAME...  (service_listfiles.go)
│   ├── system-logs                 (system_logs.go)
│   └── deployment                  (service_deployment.go)
│       ├── status SERVICE_NAME     (service_deployment_status.go)
│       └── logs SERVICE_NAME       (service_deployment_logs.go)
├── bucket                          (bucket.go)
│   ├── list                        (bucket_list.go)
│   ├── get BUCKET_NAME             (bucket_get.go)
│   ├── create BUCKET_NAME          (bucket_create.go)
│   ├── update BUCKET_NAME          (bucket_update.go)
│   ├── delete BUCKET_NAME          (bucket_delete.go)
│   ├── put-file BUCKET ...         (bucket_put_file.go)
│   ├── delete-file BUCKET ...      (bucket_delete_file.go)
│   └── presign BUCKET FILE         (bucket_presign.go)
├── hub                             (hub.go)
│   ├── list                        (hub_list.go)
│   ├── deploy SERVICE-SLUG         (hub_deploy.go)
│   └── validate SERVICE-SLUG       (hub_validate.go)
├── volume                          (volume.go)
│   ├── list                        (volume_list.go)
│   ├── create VOLUME_NAME          (volume_create.go)
│   ├── get VOLUME_NAME             (volume_get.go)
│   └── delete VOLUME_NAME          (volume_delete.go)
├── quota                           (quota.go)
│   ├── get [USER_ID]               (quota_get.go)
│   └── update USER_ID              (quota_update.go)
├── metrics                         (metrics.go)
│   ├── summary                     (metrics_summary.go)
│   ├── breakdown                   (metrics_breakdown.go)
│   └── service SERVICE_NAME        (metrics_service.go)
├── federation                      (federation.go)
│   ├── get SERVICE_NAME            (federation_get.go)
│   ├── create SERVICE_NAME         (federation_create.go)
│   ├── update SERVICE_NAME         (federation_update.go)
│   └── delete SERVICE_NAME         (federation_delete.go)
├── apply FDL_FILE                  (apply.go)
├── delete FDL_FILE                 (delete.go)
├── interactive                     (interactive.go)
└── health                          (health.go)
```

### root.go key vars
- `var Version string` — set via ldflags at build
- `var GitCommit string` — set via ldflags at build
- `func Execute()` — called from main.go

## Package Map (Non-TUI)

| Package | Path | Responsibility |
|---------|------|----------------|
| `cmd` | `cmd/` | All cobra command definitions (53 prod files) |
| `config` | `pkg/config/` | Config file read/write (YAML/JSON) at `~/.oscar-cli/config.yaml` |
| `cluster` | `pkg/cluster/` | Cluster API client: HTTP transport, auth, info/status/health/quotas/metrics |
| `service` | `pkg/service/` | Service CRUD, run/job, logs, deployment, federation, FDL parsing |
| `storage` | `pkg/storage/` | Bucket CRUD, file upload/download/list/delete via MinIO/S3/Onedata |
| `volume` | `pkg/volume/` | Managed volume CRUD |
| `hub` | `pkg/hub/` | OSCAR Hub integration: list curated services, deploy FDL, validate |
| `tui` | `pkg/tui/` | Terminal UI (covered above) |

## pkg/config — Configuration Management

**File**: `pkg/config/config.go`

- `Config.Oscar map[string]*cluster.Cluster` — cluster configurations keyed by name
- `Config.Default string` — default cluster name
- `GetDefaultConfigPath() (string, error)` → `~/.oscar-cli/config.yaml`
- `ReadConfig(configPath) (*Config, error)` — reads YAML or JSON
- `(c *Config) AddCluster(configPath, id, endpoint, authUser, authPass, oidcName, oidcToken, sslVerify) error`
- `(c *Config) RemoveCluster(configPath, id) error`
- `(c *Config) CheckCluster(id) error`
- `(c *Config) ClusterIDs() []string` — returns ordered cluster IDs
- `(c *Config) SetDefault(configPath, id) error`
- `(c *Config) GetCluster(defaultCluster, destClusterID, clusterName) (string, error)` — resolves target cluster from flags/FDL
- `GetUserConfig(c *cluster.Cluster) (interface{}, error)` — GET `/system/config`

## pkg/cluster — Cluster API Client

**Files**: `cluster.go`, `status_types.go`, `health.go`, `quotas.go`, `metrics.go`

### Cluster struct
```go
type Cluster struct {
    Endpoint         string
    AuthUser         string
    AuthPassword     string
    OIDCAccountName  string
    OIDCRefreshToken string
    SSLVerify        bool
    Memory           string
    LogLevel         string
}
```

### Key methods
- `(c *Cluster) GetClientSafe(args ...int) (*http.Client, error)` — creates authed HTTP client (Basic Auth, OIDC agent, or OIDC refresh token)
- `(c *Cluster) GetClient(args ...int) *http.Client` — panics on error
- `(c *Cluster) GetClusterInfo() (types.Info, error)` — GET `/system/info`
- `(c *Cluster) GetClusterConfig() (types.Config, error)` — GET `/system/config`
- `(c *Cluster) GetClusterStatus() (StatusInfo, error)` — GET `/system/status`
- `(c *Cluster) HealthCheck() (*HealthCheckResponse, error)` — GET `/health`
- `(c *Cluster) SetToken(client, token)` — injects bearer token round tripper

### Standalone functions
- `GetQuota(c, userID) (*types.QuotaResponse, error)` — GET `/system/quotas/user/{userID}`
- `UpdateQuota(c, userID, cpu, memory, volUpdate) (*types.QuotaResponse, error)` — PUT `/system/quotas/user/{userID}`
- `GetMetricsSummary(c, start, end) (*types.MetricsSummaryResponse, error)`
- `GetMetricsBreakdown(c, groupBy, start, end) (*types.MetricsBreakdownResponse, error)`
- `GetServiceMetrics(c, serviceName, start, end) (*types.ServiceMetricsResponse, error)`
- `CheckStatusCode(res *http.Response) error`

### Auth transport types
- `basicAuthRoundTripper` — `Authorization: Basic`
- `tokenRoundTripper` — `Authorization: Bearer`
- `RefreshToken`, `ResponseRefreshToken` — OIDC refresh token flow

## pkg/service — Service CRUD and Operations

**Files**: `service.go`, `logs.go`, `deployment.go`, `federation.go`

### FDL struct
```go
type FDL struct {
    Functions struct {
        Oscar []map[string]*types.Service
    }
    StorageProviders *types.StorageProviders
    Clusters         map[string]types.Cluster
}
```

### Key functions
- `ReadFDL(path) (*FDL, error)` — parses FDL file, embeds scripts
- `GetService(c, name) (*types.Service, error)` — GET `/system/services/{name}`
- `ListServices(c) ([]*types.Service, error)` — GET `/system/services`
- `ListServicesWithContext(ctx, c) ([]*types.Service, error)`
- `RemoveService(c, name) error` — DELETE `/system/services/{name}`
- `ApplyService(svc, c, method) error` — POST or PUT `/system/services`
- `RunService(c, name, token, endpoint, input,header) (io.ReadCloser, error)` — synchronous POST `/run/{name}`
- `JobService(c, name, token, endpoint, input) (io.ReadCloser, error)` — async POST `/job/{name}`
- `ListLogs(c, name, page) (types.JobsResponse, error)` — GET `/system/logs/{name}`
- `GetLogs(c, svcName, jobName, timestamps) (string, error)`
- `GetSystemLogs(c, timestamps, previous) (string, error)`
- `FindLatestJobName(c, svcName) (string, error)` — traverses pages to find newest
- `RemoveLog(c, svcName, jobName) error`
- `RemoveLogs(c, svcName, all) error`
- `GetDeploymentStatus(c, name) (*types.ServiceDeploymentSummary, error)` — GET `/system/services/{name}/deployment`
- `GetDeploymentLogs(c, name) (*types.DeploymentLogStream, error)` — GET `/system/services/{name}/deployment/logs`
- `GetFederation(c, serviceName) ([]types.Replica, error)`
- `CreateFederation(c, serviceName, replicas) error`
- `UpdateFederation(c, serviceName, replicas) error`
- `DeleteFederation(c, serviceName) error`

## pkg/storage — Storage Provider Operations

**Files**: `storage.go` (1226 lines), `progress.go`

### Key structs
```go
type BucketInfo struct { Name, Provider, Visibility string; AllowedUsers []string; Owner string }
type BucketObject struct { Name string; Size int64; LastModified time.Time; Owner string }
type BucketListOptions struct { PageToken string; Limit int; AutoPaginate bool }
type BucketListResult struct { Objects []*BucketObject; NextPage string; IsTruncated bool; ReturnedItems int }
type PresignRequest struct { ObjectKey, Operation string; ExpiresIn int64; ContentType string; ExtraHeaders map[string]string }
type TransferOption struct { ShowProgress bool }
```

### Key functions
- `ListBuckets(c) ([]*BucketInfo, error)` — GET `/system/buckets`
- `ListBucketsWithContext(ctx, c) ([]*BucketInfo, error)`
- `CreateBucket(c, name, visibility, allowedUsers) error` — POST
- `UpdateBucket(c, name, visibility, allowedUsers) error` — PUT
- `DeleteBucket(c, name) error` — DELETE
- `ListBucketObjectsWithOptions(c, bucketName, opts) (*BucketListResult, error)` — paginated
- `ListBucketObjectsWithOptionsContext(ctx, c, bucketName, opts) (*BucketListResult, error)`
- `PresignBucket(c, bucketName, req) (string, error)` — presigned URL
- `GetFileWithService(c, svc, provider, remotePath, localPath, opt) error`
- `PutFileWithService(c, svc, provider, localPath, remotePath, opt) error`
- `DeleteFileWithService(c, svcName, provider, remotePath) error`
- `ListFiles(c, svcName, provider, remotePath) ([]string, error)`
- `DefaultRemotePath(svc, provider, localPath) (string, error)`
- `DefaultOutputProvider(svc) (string, error)`
- `DefaultOutputPath(svc, provider) (string, error)`
- `ResolveLatestRemotePath(c, svc, provider, basePath) (string, error)`

## pkg/volume — Managed Volume Operations

**File**: `volume.go`

- `ListVolumes(c) ([]types.ManagedVolume, error)` — GET `/system/volumes`
- `CreateVolume(c, name, size) (*types.ManagedVolume, error)` — POST
- `GetVolume(c, name) (*types.ManagedVolume, error)` — GET `/system/volumes/{name}`
- `DeleteVolume(c, name) error` — DELETE

## pkg/hub — OSCAR Hub Integration

**Files**: `hub.go` (746 lines), `rocrate.go` (783 lines), `validate.go` (1602 lines)

### Key types
```go
type Client struct { owner, repo, rootPath, ref, baseAPI string; httpClient *http.Client }
type Service struct { Slug, Name, Description, Creator, URL, License, RepositoryURL, MetadataSource string }
type ListResult struct { Services []Service; Warnings []Warning }
type ROCrate struct { Context any; Graph []map[string]interface{}; index map[string]map[string]interface{} }
type AcceptanceTest struct { ID, Name, Command, ExpectedSubstring string; Inputs []TestInput; Steps []AcceptanceStep }
type AcceptanceStep struct { ID, Name, Command, ExpectedSubstring string; Inputs []TestInput }
type AcceptanceResult struct { Test AcceptanceTest; Passed bool; Output, Details string; Err error; StepResults []AcceptanceStepResult }
```

### Key functions
- `NewClient(opts ...Option) *Client`
- `(c *Client) ListServices(ctx) (*ListResult, error)` — lists curated services from `github.com/grycap/oscar-hub`
- `(c *Client) FetchFDL(ctx, slug) (*service.FDL, error)` — downloads FDL with embedded artifacts
- `LoadLocalFDL(localRoot, slug) (*service.FDL, error)`
- `ParseROCrate(raw) (*ROCrate, error)`
- `(c *ROCrate) AcceptanceTests() ([]AcceptanceTest, error)`
- `(c *Client) ValidateService(ctx, slug, clusterCfg, serviceNameOverride, localRoot) ([]AcceptanceResult, error)`
- `(c *Client) AcceptanceCommands(ctx, slug, serviceNameOverride, localRoot) ([]AcceptanceCommandSet, error)`
- `parseAcceptanceCommand(command) (parsedCommand, error)` — parses `oscar-cli service run/put-file/get-file/http`

## Build and CI

- **No Makefile** — builds via `go build`
- **GitHub Actions** (`.github/workflows/main.yaml`):
  - 5 build jobs: linux-amd64, linux-arm64, windows, darwin, darwin-arm64
  - Release job attaches binaries to GitHub releases
  - LDFLAGS inject version/commit:
    ```sh
    go build -ldflags "-s -w -X \"github.com/grycap/oscar-cli/cmd.Version=${VERSION}\" -X \"github.com/grycap/oscar-cli/cmd.GitCommit=${GITHUB_SHA::8}\""
    ```
- **Tests**: standard `go test` across all packages
- **Test files**: `main_test.go`, `cmd/*_test.go` (22 files), `pkg/config/config_test.go`, `pkg/cluster/cluster_test.go`, `pkg/service/service_test.go`, `pkg/storage/storage_test.go`, `pkg/hub/*_test.go` (3 files), `pkg/tui/app_test.go`

## Dependencies (Key Direct)

| Module | Purpose |
|--------|---------|
| `spf13/cobra` | CLI framework |
| `spf13/pflag` | Flag parsing |
| `aws/aws-sdk-go` | S3 client for storage |
| `gdamore/tcell/v2` | TUI terminal cell |
| `rivo/tview` | TUI widgets |
| `grycap/oscar/v4` | OSCAR API types |
| `indigo-dc/liboidcagent-go` | OIDC agent |
| `golang-jwt/jwt/v5` | JWT parsing |
| `goccy/go-yaml` | YAML parsing |
| `schollz/progressbar/v3` | Progress bars |
| `fatih/color` | Colored output |
| `briandowns/spinner` | Terminal spinners |
| `adrg/xdg` | XDG base paths |
| `grycap/cdmi-client-go` | Onedata CDMI client |
| `k8s.io/api`, `apimachinery`, `client-go` | K8s types |
| `sigs.k8s.io/kueue` | Kueue quota types |
