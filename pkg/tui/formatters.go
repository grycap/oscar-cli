package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/grycap/oscar/v4/pkg/types"
)

func truncateString(val string, limit int) string {
	if limit <= 0 || len(val) <= limit {
		return val
	}
	return val[:limit-1] + "…"
}

func defaultIfEmpty(val, fallback string) string {
	if strings.TrimSpace(val) == "" {
		return fallback
	}
	return val
}

func formatClusterInfo(clusterName string, info types.Info) string {
	builder := &strings.Builder{}
	if clusterName != "" {
		fmt.Fprintf(builder, "[yellow]Cluster:[-] %s\n", clusterName)
	}
	if info.Version != "" {
		fmt.Fprintf(builder, "[yellow]Version:[-] %s\n", info.Version)
	}
	if info.GitCommit != "" {
		fmt.Fprintf(builder, "[yellow]Commit:[-] %s\n", info.GitCommit)
	}
	if info.Architecture != "" {
		fmt.Fprintf(builder, "[yellow]Architecture:[-] %s\n", info.Architecture)
	}
	if info.KubeVersion != "" {
		fmt.Fprintf(builder, "[yellow]Kubernetes:[-] %s\n", info.KubeVersion)
	}
	if backend := info.ServerlessBackendInfo; backend != nil {
		if backend.Name != "" {
			fmt.Fprintf(builder, "[yellow]Serverless:[-] %s", backend.Name)
			if backend.Version != "" {
				fmt.Fprintf(builder, " %s", backend.Version)
			}
			builder.WriteByte('\n')
		} else if backend.Version != "" {
			fmt.Fprintf(builder, "[yellow]Serverless:[-] %s\n", backend.Version)
		}
	}
	out := strings.TrimRight(builder.String(), "\n")
	if out == "" {
		return "No cluster information available"
	}
	return out
}

func formatServiceLogs(serviceName, jobName, logs string) string {
	builder := &strings.Builder{}
	if serviceName != "" {
		fmt.Fprintf(builder, "[yellow]Service:[-] %s\n", serviceName)
	}
	if jobName != "" {
		fmt.Fprintf(builder, "[yellow]Job:[-] %s\n", jobName)
	}
	clean := strings.TrimSpace(logs)
	if clean == "" {
		builder.WriteString("No logs available")
		return builder.String()
	}
	builder.WriteString("\n")
	builder.WriteString(tview.Escape(clean))
	return builder.String()
}

func formatClusterConfig(name string, cfg *cluster.Cluster) string {
	title := strings.TrimSpace(name)
	if title == "" {
		title = "Cluster"
	}
	if cfg == nil {
		return fmt.Sprintf("[yellow]%s:[-]\n    configuration not available", title)
	}

	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]%s:[-]\n", title)
	appendField := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		fmt.Fprintf(builder, "    %s: %s\n", label, value)
	}

	appendField("endpoint", cfg.Endpoint)
	appendField("auth_user", cfg.AuthUser)
	if cfg.AuthPassword != "" {
		appendField("auth_password", maskSecret(cfg.AuthPassword))
	}
	appendField("oidc_account_name", cfg.OIDCAccountName)
	if cfg.OIDCRefreshToken != "" {
		appendField("oidc_refresh_token", trimToken(cfg.OIDCRefreshToken))
	}
	appendField("ssl_verify", strconv.FormatBool(cfg.SSLVerify))
	appendField("memory", strings.TrimSpace(cfg.Memory))
	appendField("log_level", strings.TrimSpace(cfg.LogLevel))

	return strings.TrimRight(builder.String(), "\n")
}

func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	const maxStars = 8
	if len(secret) <= maxStars {
		return strings.Repeat("*", len(secret))
	}
	return strings.Repeat("*", maxStars)
}

func trimToken(token string) string {
	if token == "" {
		return ""
	}
	firstLine := strings.Split(token, "\n")[0]
	const limit = 64
	if len(firstLine) > limit {
		return firstLine[:limit]
	}
	return firstLine
}

func formatServiceDetails(svc *types.Service) string {
	if svc == nil {
		return ""
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]Name:[-] %s\n", svc.Name)
	if svc.ClusterID != "" {
		fmt.Fprintf(builder, "[yellow]Cluster:[-] %s\n", svc.ClusterID)
	}
	if svc.Image != "" {
		fmt.Fprintf(builder, "[yellow]Image:[-] %s\n", svc.Image)
	}
	if svc.Memory != "" {
		fmt.Fprintf(builder, "[yellow]Memory:[-] %s\n", svc.Memory)
	}
	if svc.CPU != "" {
		fmt.Fprintf(builder, "[yellow]CPU:[-] %s\n", svc.CPU)
	}
	if replicas := len(svc.Replicas); replicas > 0 {
		fmt.Fprintf(builder, "[yellow]Replicas:[-] %d\n", replicas)
	}
	if svc.LogLevel != "" {
		fmt.Fprintf(builder, "[yellow]Log Level:[-] %s\n", svc.LogLevel)
	}
	return builder.String()
}

func formatBucketDetails(bucket *storage.BucketInfo) string {
	if bucket == nil {
		return ""
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]Name:[-] %s\n", bucket.Name)
	if bucket.Visibility != "" {
		fmt.Fprintf(builder, "[yellow]Visibility:[-] %s\n", bucket.Visibility)
	}
	if len(bucket.AllowedUsers) > 0 {
		fmt.Fprintf(builder, "[yellow]Allowed Users:[-] %s\n", strings.Join(bucket.AllowedUsers, ", "))
	}
	if bucket.Owner != "" {
		fmt.Fprintf(builder, "[yellow]Owner:[-] %s\n", bucket.Owner)
	}

	return builder.String()
}

func formatMetricsSummary(name string, summary *types.MetricsSummaryResponse) string {
	if summary == nil {
		return "No metrics available"
	}
	builder := &strings.Builder{}
	if name != "" {
		fmt.Fprintf(builder, "[yellow]Cluster:[-] %s\n", name)
	}
	if !summary.Start.IsZero() && !summary.End.IsZero() {
		fmt.Fprintf(builder, "[yellow]Period:[-] %s → %s\n",
			summary.Start.Format("2006-01-02 15:04"),
			summary.End.Format("2006-01-02 15:04"))
	}

	totals := summary.Totals
	builder.WriteString("\n[yellow]Services[-]\n")
	fmt.Fprintf(builder, "  Active: %d\n", totals.ServicesCountActive)
	fmt.Fprintf(builder, "  Total:  %d\n", totals.ServicesCountTotal)

	builder.WriteString("\n[yellow]CPU/GPU Hours[-]\n")
	fmt.Fprintf(builder, "  CPU: %.1f h\n", totals.CPUHoursTotal)
	fmt.Fprintf(builder, "  GPU: %.1f h\n", totals.GPUHoursTotal)

	builder.WriteString("\n[yellow]Requests[-]\n")
	fmt.Fprintf(builder, "  Total:  %d\n", totals.RequestsCountTotal)
	fmt.Fprintf(builder, "  Sync:   %d\n", totals.RequestsCountSync)
	fmt.Fprintf(builder, "  Async:  %d\n", totals.RequestsCountAsync)
	fmt.Fprintf(builder, "  Exposed: %d\n", totals.RequestsCountExposed)

	builder.WriteString("\n[yellow]Users[-]\n")
	fmt.Fprintf(builder, "  Total: %d\n", totals.UsersCount)
	if len(totals.Users) > 0 {
		builder.WriteString("  ")
		for i, u := range totals.Users {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(u)
		}
		builder.WriteByte('\n')
	}

	builder.WriteString("\n[yellow]Countries[-]\n")
	fmt.Fprintf(builder, "  Total: %d\n", totals.CountriesCount)
	if len(totals.Countries) > 0 {
		for _, c := range totals.Countries {
			fmt.Fprintf(builder, "  - %s: %d\n", c.Country, c.RequestCount)
		}
	}

	if len(summary.Sources) > 0 {
		builder.WriteString("\n[yellow]Sources[-]\n")
		for _, src := range summary.Sources {
			statusColor := "[green]"
			if src.Status != "healthy" {
				statusColor = "[red]"
			}
			fmt.Fprintf(builder, "  - %s: %s%s[-]", src.Name, statusColor, src.Status)
			if src.LastUpdated != nil && !src.LastUpdated.IsZero() {
				fmt.Fprintf(builder, " (%s)", src.LastUpdated.Format("2006-01-02 15:04"))
			}
			builder.WriteByte('\n')
			if src.Notes != "" {
				fmt.Fprintf(builder, "    %s\n", src.Notes)
			}
		}
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatVolumeDetails(vol *types.ManagedVolume) string {
	if vol == nil {
		return ""
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]Name:[-] %s\n", vol.Name)
	if vol.Size != "" {
		fmt.Fprintf(builder, "[yellow]Size:[-] %s\n", vol.Size)
	}
	if vol.Status.Phase != "" {
		colorTag := colorTagForPhase(vol.Status.Phase)
		fmt.Fprintf(builder, "[yellow]Status:[-] %s%s[-]\n", colorTag, vol.Status.Phase)
	}
	if vol.Status.Message != "" {
		fmt.Fprintf(builder, "[yellow]Message:[-] %s\n", vol.Status.Message)
	}
	if vol.Namespace != "" {
		fmt.Fprintf(builder, "[yellow]Namespace:[-] %s\n", vol.Namespace)
	}
	if vol.PVCName != "" {
		fmt.Fprintf(builder, "[yellow]PVC Name:[-] %s\n", vol.PVCName)
	}
	if vol.OwnerUser != "" {
		fmt.Fprintf(builder, "[yellow]Owner:[-] %s\n", vol.OwnerUser)
	}
	if vol.CreationMode != "" {
		fmt.Fprintf(builder, "[yellow]Creation Mode:[-] %s\n", vol.CreationMode)
	}
	if vol.LifecyclePolicy != "" {
		fmt.Fprintf(builder, "[yellow]Lifecycle Policy:[-] %s\n", vol.LifecyclePolicy)
	}
	if vol.CreatedByService != "" {
		fmt.Fprintf(builder, "[yellow]Created By:[-] %s\n", vol.CreatedByService)
	}
	if len(vol.Attachments) > 0 {
		fmt.Fprintf(builder, "[yellow]Attachments:[-]\n")
		for _, att := range vol.Attachments {
			fmt.Fprintf(builder, "  - %s → %s\n", att.ServiceName, att.MountPath)
		}
	}
	if vol.Status.AttachmentCount > 0 {
		fmt.Fprintf(builder, "[yellow]Attachment Count:[-] %d\n", vol.Status.AttachmentCount)
	}

	return builder.String()
}

func formatQuota(name string, quota *types.QuotaResponse) string {
	if quota == nil {
		return "No quota data available"
	}
	builder := &strings.Builder{}
	if name != "" {
		fmt.Fprintf(builder, "[yellow]Cluster:[-] %s\n", name)
	}
	fmt.Fprintf(builder, "[yellow]User:[-] %s\n", defaultIfEmpty(quota.UserID, "-"))
	if quota.ClusterQueue != "" {
		fmt.Fprintf(builder, "[yellow]Cluster Queue:[-] %s\n", quota.ClusterQueue)
	}

	// Sort resource keys for consistent output
	resKeys := make([]string, 0, len(quota.Resources))
	for k := range quota.Resources {
		resKeys = append(resKeys, k)
	}
	sort.Strings(resKeys)

	if len(resKeys) > 0 {
		builder.WriteString("\n[yellow]Resources[-]\n")
		for _, k := range resKeys {
			v := quota.Resources[k]
			colorMax := "[green]"
			colorUsed := "[green]"
			if v.Max > 0 && v.Used >= v.Max {
				colorUsed = "[red]"
			} else if v.Max > 0 && v.Used > v.Max/2 {
				colorUsed = "[yellow]"
			}
			fmt.Fprintf(builder, "  %s: %s%d[-] / %s%d[-]\n", k, colorUsed, v.Used, colorMax, v.Max)
		}
	}

	if v := quota.Volumes; v != nil {
		builder.WriteString("\n[yellow]Volumes[-]\n")
		if v.Disk.Max != "" || v.Disk.Used != "" {
			fmt.Fprintf(builder, "  Disk:    %s / %s\n",
				defaultIfEmpty(v.Disk.Used, "0"),
				defaultIfEmpty(v.Disk.Max, "-"))
		}
		if v.Volumes.Max != "" || v.Volumes.Used != "" {
			fmt.Fprintf(builder, "  Count:   %s / %s\n",
				defaultIfEmpty(v.Volumes.Used, "0"),
				defaultIfEmpty(v.Volumes.Max, "-"))
		}
		if v.MaxDiskperVolume != "" {
			fmt.Fprintf(builder, "  Max/vol: %s\n", v.MaxDiskperVolume)
		}
		if v.MinDiskperVolume != "" {
			fmt.Fprintf(builder, "  Min/vol: %s\n", v.MinDiskperVolume)
		}
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatDeploymentStatus(name string, ds *types.ServiceDeploymentSummary) string {
	if ds == nil {
		return "No deployment data available"
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]Service:[-] %s\n", name)

	color := "[green]"
	switch ds.State {
	case types.DeploymentStateReady:
		color = "[green]"
	case types.DeploymentStateDegraded:
		color = "[yellow]"
	case types.DeploymentStateFailed, types.DeploymentStateUnavailable:
		color = "[red]"
	default:
		color = "[blue]"
	}
	fmt.Fprintf(builder, "[yellow]State:[-] %s%s[-]\n", color, ds.State)

	if ds.Reason != "" {
		fmt.Fprintf(builder, "[yellow]Reason:[-] %s\n", ds.Reason)
	}

	if ds.LastTransitionTime != nil {
		fmt.Fprintf(builder, "[yellow]Last Transition:[-] %s\n", ds.LastTransitionTime.Time.Format("2006-01-02 15:04:05"))
	}

	if ds.ActiveInstances > 0 || ds.AffectedInstances > 0 {
		fmt.Fprintf(builder, "[yellow]Active Instances:[-] %d\n", ds.ActiveInstances)
		fmt.Fprintf(builder, "[yellow]Affected Instances:[-] %d\n", ds.AffectedInstances)
	}

	if ds.ResourceKind != "" {
		fmt.Fprintf(builder, "[yellow]Resource Kind:[-] %s\n", ds.ResourceKind)
	}

	return strings.TrimRight(builder.String(), "\n")
}

func formatDeploymentLogs(dl *types.DeploymentLogStream) string {
	if dl == nil {
		return "No deployment logs available"
	}
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "[yellow]Service:[-] %s\n", dl.ServiceName)
	fmt.Fprintf(builder, "[yellow]Available:[-] %t\n", dl.Available)
	if dl.Message != "" {
		fmt.Fprintf(builder, "[yellow]Message:[-] %s\n", dl.Message)
	}
	if len(dl.Entries) > 0 {
		builder.WriteString("\n[yellow]Entries:[-]\n")
		for _, e := range dl.Entries {
			if e.Timestamp != "" {
				fmt.Fprintf(builder, "  %s  ", e.Timestamp)
			} else {
				builder.WriteString("  ")
			}
			fmt.Fprintf(builder, "%s\n", e.Message)
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}
