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

const quotasPath = "/system/quotas/user"

// GetQuota returns the quota for the specified user.
func GetQuota(c *Cluster, userID string) (*types.QuotaResponse, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}
	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, quotasPath, userID)
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
//
// The request fields map one-to-one to the JSON body sent to the API.
// Fields that are zero-valued (empty strings, nil pointers) are not
// modified on the server; empty values for CPU, memory and ephemeral
// storage are always serialized because the API contract does not mark
// them omitempty.
func UpdateQuota(c *Cluster, userID string, req *types.QuotaUpdateRequest) (*types.QuotaResponse, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return nil, errors.New("user ID is required")
	}
	if req == nil {
		req = &types.QuotaUpdateRequest{}
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, quotasPath, trimmed)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cannot encode quota request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPut, endpoint.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, ErrMakingRequest
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client, err := c.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(httpReq)
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
