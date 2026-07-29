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
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/grycap/oscar-cli/v2/pkg/cluster"
	"github.com/grycap/oscar/v4/pkg/types"
)

// GetDeploymentStatus returns the deployment status of a service.
func GetDeploymentStatus(c *cluster.Cluster, name string) (*types.ServiceDeploymentSummary, error) {
	getURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	getURL.Path = path.Join(getURL.Path, servicesPath, name, "deployment")

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

	var summary types.ServiceDeploymentSummary
	if err := json.NewDecoder(res.Body).Decode(&summary); err != nil {
		return nil, err
	}

	return &summary, nil
}

// GetDeploymentLogs returns the deployment logs of a service.
func GetDeploymentLogs(c *cluster.Cluster, name string) (*types.DeploymentLogStream, error) {
	getURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	getURL.Path = path.Join(getURL.Path, servicesPath, name, "deployment", "logs")

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

	var logStream types.DeploymentLogStream
	if err := json.NewDecoder(res.Body).Decode(&logStream); err != nil {
		return nil, err
	}

	return &logStream, nil
}
