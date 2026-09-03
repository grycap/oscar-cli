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
func GetFederation(c *cluster.Cluster, serviceName string) (types.FederationResponse, error) {
	var federation types.FederationResponse
	getURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return federation, cluster.ErrParsingEndpoint
	}
	getURL.Path = path.Join(getURL.Path, federationPath, serviceName)

	req, err := http.NewRequest(http.MethodGet, getURL.String(), nil)
	if err != nil {
		return federation, cluster.ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return federation, err
	}

	res, err := client.Do(req)
	if err != nil {
		return federation, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return federation, err
	}

	if err := json.NewDecoder(res.Body).Decode(&federation); err != nil {
		return federation, err
	}

	return federation, nil
}

// CreateFederation creates federated members for a service.
func CreateFederation(c *cluster.Cluster, serviceName string, replicas []types.Replica) error {
	return federationRequest(c, http.MethodPost, serviceName, types.FederationRequest{Members: replicas})
}

// UpdateFederation updates federated members for a service.
func UpdateFederation(c *cluster.Cluster, serviceName string, replicas []types.Replica) error {
	return federationRequest(c, http.MethodPut, serviceName, types.FederationRequest{Update: replicas})
}

// DeleteFederation deletes federated members for a service.
func DeleteFederation(c *cluster.Cluster, serviceName string, replicas []types.Replica) error {
	return federationRequest(c, http.MethodDelete, serviceName, types.FederationRequest{Members: replicas, Delete: true})
}

func federationRequest(c *cluster.Cluster, method, serviceName string, payload types.FederationRequest) error {
	reqURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	reqURL.Path = path.Join(reqURL.Path, federationPath, serviceName)

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	body := bytes.NewBuffer(bodyBytes)

	req, err := http.NewRequest(method, reqURL.String(), body)
	if err != nil {
		return cluster.ErrMakingRequest
	}
	req.Header.Set("Content-Type", "application/json")

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
