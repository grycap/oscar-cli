package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rivo/tview"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/grycap/oscar/v3/pkg/types"
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
