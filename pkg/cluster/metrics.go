/*
Copyright (C) GRyCAP - I3M - UPV

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cluster

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"

	"github.com/grycap/oscar/v4/pkg/types"
)

const metricsPath = "/system/metrics"

// GetMetricsSummary returns the metrics summary for the cluster.
func GetMetricsSummary(c *Cluster, start, end string) (*types.MetricsSummaryResponse, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, metricsPath)

	query := endpoint.Query()
	if start != "" {
		query.Set("start", start)
	}
	if end != "" {
		query.Set("end", end)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, ErrSendingRequest
	}
	defer res.Body.Close()

	if err := CheckStatusCode(res); err != nil {
		return nil, err
	}

	var summary types.MetricsSummaryResponse
	if err := json.NewDecoder(res.Body).Decode(&summary); err != nil {
		return nil, err
	}

	return &summary, nil
}

// GetMetricsBreakdown returns the metrics breakdown grouped by the specified key.
func GetMetricsBreakdown(c *Cluster, groupBy, start, end string) (*types.MetricsBreakdownResponse, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, metricsPath, "breakdown")

	query := endpoint.Query()
	if groupBy != "" {
		query.Set("group_by", groupBy)
	}
	if start != "" {
		query.Set("start", start)
	}
	if end != "" {
		query.Set("end", end)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, ErrSendingRequest
	}
	defer res.Body.Close()

	if err := CheckStatusCode(res); err != nil {
		return nil, err
	}

	var breakdown types.MetricsBreakdownResponse
	if err := json.NewDecoder(res.Body).Decode(&breakdown); err != nil {
		return nil, err
	}

	return &breakdown, nil
}

// GetServiceMetrics returns the metrics for a specific service.
func GetServiceMetrics(c *Cluster, serviceName, start, end string) (*types.ServiceMetricsResponse, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}
	if serviceName == "" {
		return nil, errors.New("service name is required")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, metricsPath, serviceName)

	query := endpoint.Query()
	if start != "" {
		query.Set("start", start)
	}
	if end != "" {
		query.Set("end", end)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, ErrSendingRequest
	}
	defer res.Body.Close()

	if err := CheckStatusCode(res); err != nil {
		return nil, err
	}

	var svcMetrics types.ServiceMetricsResponse
	if err := json.NewDecoder(res.Body).Decode(&svcMetrics); err != nil {
		return nil, err
	}

	return &svcMetrics, nil
}
