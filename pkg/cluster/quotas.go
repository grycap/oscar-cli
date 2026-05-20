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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/grycap/oscar/v4/pkg/types"
)

const quotasPath = "/system/quotas"

// GetQuota returns the quota for the specified user.
func GetQuota(c *Cluster, userID string) (*types.QuotaResponse, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil, errors.New("user ID is required")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, quotasPath, trimmed)

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

	var quota types.QuotaResponse
	if err := json.NewDecoder(res.Body).Decode(&quota); err != nil {
		return nil, err
	}

	return &quota, nil
}

// UpdateQuota updates the quota for the specified user.
func UpdateQuota(c *Cluster, userID, cpu, memory string, volumes *types.VolumeQuotaUpdate) (*types.QuotaResponse, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil, errors.New("user ID is required")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, quotasPath, trimmed)

	reqBody := types.QuotaUpdateRequest{
		CPU:    cpu,
		Memory: memory,
		Volumes: volumes,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("cannot encode quota request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, endpoint.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, ErrMakingRequest
	}
	req.Header.Set("Content-Type", "application/json")

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

	var quota types.QuotaResponse
	if err := json.NewDecoder(res.Body).Decode(&quota); err != nil {
		return nil, err
	}

	return &quota, nil
}
