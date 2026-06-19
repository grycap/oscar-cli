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

package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/grycap/oscar-cli/v2/pkg/cluster"
	"github.com/grycap/oscar/v4/pkg/types"
)

const federationPath = "/system/federation"

// GetFederation returns the federated members of a service.
func GetFederation(c *cluster.Cluster, serviceName string) ([]types.Replica, error) {
	getURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	getURL.Path = path.Join(getURL.Path, federationPath, serviceName)

	req, err := http.NewRequest(http.MethodGet, getURL.String(), nil)
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

	var replicas []types.Replica
	if err := json.NewDecoder(res.Body).Decode(&replicas); err != nil {
		return nil, err
	}

	return replicas, nil
}

// CreateFederation creates federated members for a service.
func CreateFederation(c *cluster.Cluster, serviceName string, replicas []types.Replica) error {
	return federationRequest(c, http.MethodPost, serviceName, replicas)
}

// UpdateFederation updates federated members for a service.
func UpdateFederation(c *cluster.Cluster, serviceName string, replicas []types.Replica) error {
	return federationRequest(c, http.MethodPut, serviceName, replicas)
}

// DeleteFederation deletes federated members for a service.
func DeleteFederation(c *cluster.Cluster, serviceName string) error {
	return federationRequest(c, http.MethodDelete, serviceName, nil)
}

func federationRequest(c *cluster.Cluster, method, serviceName string, replicas []types.Replica) error {
	reqURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	reqURL.Path = path.Join(reqURL.Path, federationPath, serviceName)

	var body *bytes.Buffer
	if replicas != nil {
		bodyBytes, err := json.Marshal(replicas)
		if err != nil {
			return err
		}
		body = bytes.NewBuffer(bodyBytes)
	}

	req, err := http.NewRequest(method, reqURL.String(), body)
	if err != nil {
		return cluster.ErrMakingRequest
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
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
