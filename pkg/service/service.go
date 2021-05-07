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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar/v2/pkg/types"
)

const servicesPath = "/system/services"

// FDL represents a Functions Definition Language file
type FDL struct {
	Functions struct {
		Oscar []map[string]*types.Service `json:"oscar" binding:"required"`
	} `json:"functions" binding:"required"`
	StorageProviders *types.StorageProviders `json:"storage_providers,omitempty"`
}

// ReadFDL reads the content of FDL file and returns a valid FDL struct with the scripts and StorageProviders embedded into the services
func ReadFDL(path string) (fdl *FDL, err error) {
	fdl = &FDL{}
	// Read the file
	content, err := os.ReadFile(path)
	if err != nil {
		return fdl, errors.New("cannot read the file, please check the path")
	}

	// Unmarshal the FDL
	err = yaml.Unmarshal(content, fdl)
	if err != nil {
		return fdl, errors.New("the FDL file is not valid, please check its definition")
	}

	for _, element := range fdl.Functions.Oscar {
		for _, svc := range element {
			// Embed script
			scriptPath := svc.Script
			script, err := os.ReadFile(scriptPath)
			if err != nil {
				return fdl, fmt.Errorf("cannot load the script \"%s\" of service \"%s\", please check the path", scriptPath, svc.Name)
			}
			svc.Script = string(script)

			// Embed StorageProviders
			svc.StorageProviders = fdl.StorageProviders
		}

	}

	return fdl, nil
}

// GetService gets a service from a cluster
func GetService(c *cluster.Cluster, name string) (svc *types.Service, err error) {
	getServiceUrl, err := url.Parse(c.Endpoint)
	if err != nil {
		return svc, cluster.ErrParsingEndpoint
	}
	getServiceUrl.Path = path.Join(getServiceUrl.Path, servicesPath, name)

	req, err := http.NewRequest(http.MethodGet, getServiceUrl.String(), nil)
	if err != nil {
		return svc, cluster.ErrMakingRequest
	}

	res, err := c.GetClient().Do(req)
	if err != nil {
		return svc, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return svc, err
	}

	// Decode the response body into the info struct
	err = json.NewDecoder(res.Body).Decode(&svc)
	if err != nil {
		return svc, err
	}

	return svc, nil
}

// ListServices gets a service from a cluster
func ListServices(c *cluster.Cluster) (svcList []*types.Service, err error) {
	getServicesUrl, err := url.Parse(c.Endpoint)
	if err != nil {
		return svcList, cluster.ErrParsingEndpoint
	}
	getServicesUrl.Path = path.Join(getServicesUrl.Path, servicesPath)

	req, err := http.NewRequest(http.MethodGet, getServicesUrl.String(), nil)
	if err != nil {
		return svcList, cluster.ErrMakingRequest
	}

	res, err := c.GetClient().Do(req)
	if err != nil {
		return svcList, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return svcList, err
	}

	// Decode the response body into the info struct
	err = json.NewDecoder(res.Body).Decode(&svcList)
	if err != nil {
		return svcList, err
	}

	return svcList, nil
}

// RemoveService removes a service from a cluster
func RemoveService(c *cluster.Cluster, name string) error {
	removeServiceUrl, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	removeServiceUrl.Path = path.Join(removeServiceUrl.Path, servicesPath, name)

	req, err := http.NewRequest(http.MethodDelete, removeServiceUrl.String(), nil)
	if err != nil {
		return cluster.ErrMakingRequest
	}

	res, err := c.GetClient().Do(req)
	if err != nil {
		return cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return err
	}

	return nil
}

// ApplyService creates or edit a service in the specified cluster
func ApplyService(svc *types.Service, c *cluster.Cluster, method string) error {
	// Check valid methods (only POST and PUT are allowed)
	if method != http.MethodPost && method != http.MethodPut {
		return errors.New("invalid method")
	}

	applyServiceUrl, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	applyServiceUrl.Path = path.Join(applyServiceUrl.Path, servicesPath)

	// Marshal service
	svcBytes, err := json.Marshal(svc)
	if err != nil {
		return fmt.Errorf("cannot encode the service \"%s\", please check its definition", svc.Name)
	}
	reqBody := bytes.NewBuffer(svcBytes)

	// Make the request
	req, err := http.NewRequest(method, applyServiceUrl.String(), reqBody)
	if err != nil {
		return cluster.ErrMakingRequest
	}

	res, err := c.GetClient().Do(req)
	if err != nil {
		return cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return err
	}

	return nil
}
