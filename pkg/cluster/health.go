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
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const healthPath = "/health"

// HealthCheckResponse contains the result of a health check.
type HealthCheckResponse struct {
	Status   string `json:"status"`
	RawBody  string `json:"raw_body,omitempty"`
}

// HealthCheck performs a health check against the OSCAR cluster.
func (cluster *Cluster) HealthCheck() (*HealthCheckResponse, error) {
	healthURL, err := url.Parse(cluster.Endpoint)
	if err != nil {
		return nil, ErrParsingEndpoint
	}
	healthURL.Path = path.Join(healthURL.Path, healthPath)

	req, err := http.NewRequest(http.MethodGet, healthURL.String(), nil)
	if err != nil {
		return nil, ErrMakingRequest
	}

	client, err := cluster.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, ErrSendingRequest
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	status := "ok"
	if res.StatusCode != http.StatusOK {
		status = "unhealthy"
	}

	return &HealthCheckResponse{
		Status:  status,
		RawBody: strings.TrimSpace(string(body)),
	}, nil
}
