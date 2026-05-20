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

package volume

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar/v4/pkg/types"
)

const volumesPath = "/system/volumes"

// ListVolumes returns the managed volumes in the cluster.
func ListVolumes(c *cluster.Cluster) ([]types.ManagedVolume, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, volumesPath)

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, cluster.ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return nil, err
	}

	var volumes []types.ManagedVolume
	if err := json.NewDecoder(res.Body).Decode(&volumes); err != nil {
		return nil, err
	}

	return volumes, nil
}

// CreateVolume creates a managed volume in the cluster.
func CreateVolume(c *cluster.Cluster, name, size string) (*types.ManagedVolume, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, errors.New("volume name is required")
	}
	trimmedSize := strings.TrimSpace(size)
	if trimmedSize == "" {
		return nil, errors.New("volume size is required")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, volumesPath)

	body := types.ManagedVolumeCreateRequest{
		Name: trimmedName,
		Size: trimmedSize,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("cannot encode volume request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, cluster.ErrMakingRequest
	}
	req.Header.Set("Content-Type", "application/json")

	client, err := c.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return nil, err
	}

	var vol types.ManagedVolume
	if err := json.NewDecoder(res.Body).Decode(&vol); err != nil {
		return nil, err
	}

	return &vol, nil
}

// GetVolume returns a specific managed volume.
func GetVolume(c *cluster.Cluster, name string) (*types.ManagedVolume, error) {
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return nil, errors.New("volume name is required")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, volumesPath, trimmed)

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, cluster.ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return nil, err
	}

	var vol types.ManagedVolume
	if err := json.NewDecoder(res.Body).Decode(&vol); err != nil {
		return nil, err
	}

	return &vol, nil
}

// DeleteVolume removes a managed volume from the cluster.
func DeleteVolume(c *cluster.Cluster, name string) error {
	if c == nil {
		return errors.New("cluster configuration not provided")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("volume name is required")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, volumesPath, trimmed)

	req, err := http.NewRequest(http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return cluster.ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		return cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	return cluster.CheckStatusCode(res)
}
