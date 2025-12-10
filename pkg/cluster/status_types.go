package cluster

import "time"

// StatusInfo aggregates the information returned by /system/status.
type StatusInfo struct {
	Cluster ClusterStatus `json:"cluster"`
	Oscar   OscarStatus   `json:"oscar"`
	MinIO   MinioStatus   `json:"minio"`
}

type ClusterStatus struct {
	NodesCount int64          `json:"nodes_count"`
	Metrics    ClusterMetrics `json:"metrics"`
	Nodes      []NodeDetail   `json:"nodes"`
}

type ClusterMetrics struct {
	CPU    CPUMetrics    `json:"cpu"`
	Memory MemoryMetrics `json:"memory"`
	GPU    GPUMetrics    `json:"gpu"`
}

type CPUMetrics struct {
	TotalFreeCores     int64 `json:"total_free_cores"`
	MaxFreeOnNodeCores int64 `json:"max_free_on_node_cores"`
}

type MemoryMetrics struct {
	TotalFreeBytes     int64 `json:"total_free_bytes"`
	MaxFreeOnNodeBytes int64 `json:"max_free_on_node_bytes"`
}

type GPUMetrics struct {
	TotalGPU int64 `json:"total_gpu"`
}

type NodeDetail struct {
	Name        string                `json:"name"`
	CPU         NodeResource          `json:"cpu"`
	Memory      NodeResource          `json:"memory"`
	GPU         int64                 `json:"gpu"`
	IsInterlink bool                  `json:"is_interlink"`
	Status      string                `json:"status"`
	Conditions  []NodeConditionSimple `json:"conditions"`
}

type NodeResource struct {
	CapacityCores int64 `json:"capacity_cores,omitempty"`
	UsageCores    int64 `json:"usage_cores,omitempty"`
	CapacityBytes int64 `json:"capacity_bytes,omitempty"`
	UsageBytes    int64 `json:"usage_bytes,omitempty"`
}

type NodeConditionSimple struct {
	Type   string `json:"type"`
	Status bool   `json:"status"`
}

type OscarStatus struct {
	DeploymentName string          `json:"deployment_name"`
	Ready          bool            `json:"ready"`
	Deployment     OscarDeployment `json:"deployment"`
	JobsCount      int             `json:"jobs_count"`
	Pods           PodStates       `json:"pods"`
	OIDC           OIDCInfo        `json:"oidc"`
}

type OscarDeployment struct {
	AvailableReplicas int32             `json:"available_replicas"`
	ReadyReplicas     int32             `json:"ready_replicas"`
	Replicas          int32             `json:"replicas"`
	CreationTimestamp time.Time         `json:"creation_timestamp"`
	Strategy          string            `json:"strategy"`
	Labels            map[string]string `json:"labels"`
}

type PodStates struct {
	Total  int            `json:"total"`
	States map[string]int `json:"states"`
}

type OIDCInfo struct {
	Enabled bool     `json:"enabled"`
	Issuers []string `json:"issuers"`
	Groups  []string `json:"groups"`
}

type MinioStatus struct {
	BucketsCount int `json:"buckets_count"`
	TotalObjects int `json:"total_objects"`
}
