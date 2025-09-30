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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar/v3/pkg/types"
)

const servicesPath = "/system/services"
const runPath = "/run"
const jobPath = "/job"

// FDL represents a Functions Definition Language file
type FDL struct {
	Functions struct {
		Oscar []map[string]*types.Service `json:"oscar" binding:"required"`
	} `json:"functions" binding:"required"`
	StorageProviders *types.StorageProviders  `json:"storage_providers,omitempty"`
	Clusters         map[string]types.Cluster `json:"clusters,omitempty"`
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
		for clusterID, svc := range element {
			// Embed script
			scriptPath := getScriptPath(svc.Script, path)
			script, err := os.ReadFile(scriptPath)
			if err != nil {
				return fdl, fmt.Errorf("cannot load the script \"%s\" of service \"%s\", please check the path", scriptPath, svc.Name)
			}
			svc.Script = string(script)

			// Set ClusterID
			svc.ClusterID = clusterID

			// Embed StorageProviders
			svc.StorageProviders = fdl.StorageProviders

			// Embed Clusters
			svc.Clusters = fdl.Clusters
		}

	}

	return fdl, nil
}

// GetService gets a service from a cluster
func GetService(c *cluster.Cluster, name string) (svc *types.Service, err error) {
	getServiceURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return svc, cluster.ErrParsingEndpoint
	}
	getServiceURL.Path = path.Join(getServiceURL.Path, servicesPath, name)

	req, err := http.NewRequest(http.MethodGet, getServiceURL.String(), nil)
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
	getServicesURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return svcList, cluster.ErrParsingEndpoint
	}
	getServicesURL.Path = path.Join(getServicesURL.Path, servicesPath)

	req, err := http.NewRequest(http.MethodGet, getServicesURL.String(), nil)
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
	removeServiceURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	removeServiceURL.Path = path.Join(removeServiceURL.Path, servicesPath, name)

	req, err := http.NewRequest(http.MethodDelete, removeServiceURL.String(), nil)
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

	applyServiceURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	applyServiceURL.Path = path.Join(applyServiceURL.Path, servicesPath)
	// Marshal service
	svcBytes, err := json.Marshal(svc)
	if err != nil {
		return fmt.Errorf("cannot encode the service \"%s\", please check its definition", svc.Name)
	}
	reqBody := bytes.NewBuffer(svcBytes)

	// Make the request
	req, err := http.NewRequest(method, applyServiceURL.String(), reqBody)
	if err != nil {
		return cluster.ErrMakingRequest
	}

	client := c.GetClient()
	// Increase timeout to avoid errors due to daemonset execution
	if svc.ImagePrefetch {
		client = c.GetClient(400)
	}
	res, err := client.Do(req)
	if err != nil {
		return cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return err
	}

	return nil
}

// RunService invokes a service synchronously (a Serverless backend in the cluster is required)
func RunService(c *cluster.Cluster, name string, token string, endpoint string, input io.Reader) (responseBody io.ReadCloser, err error) {

	var runServiceURL *url.URL
	if token != "" {
		runServiceURL, err = url.Parse(endpoint)
	} else {
		runServiceURL, err = url.Parse(c.Endpoint)
	}

	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	runServiceURL.Path = path.Join(runServiceURL.Path, runPath, name)
	// Make the request
	req, err := http.NewRequest(http.MethodPost, runServiceURL.String(), input)
	if err != nil {
		return nil, cluster.ErrMakingRequest
	}

	var res *http.Response
	if token != "" {
		bearer := "Bearer " + strings.TrimSpace(token)
		req.Header.Add("Authorization", bearer)

		client := &http.Client{}
		res, err = client.Do(req)
	} else {

		// Get the service
		svc, err := GetService(c, name)
		if err != nil {
			return nil, err
		}
		// Add service's token if defined (OSCAR >= v2.2.0)
		if svc.Token != "" {
			bearer := "Bearer " + strings.TrimSpace(svc.Token)
			req.Header.Add("Authorization", bearer)
		}
		// Update cluster client timeout
		client := c.GetClient()
		client.Timeout = time.Second * 300

		// Update client transport to remove basic auth
		client.Transport = &http.Transport{
			// Enable/disable ssl verification
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !c.SSLVerify},
		}

		res, err = client.Do(req)
	}

	if err != nil {
		return nil, cluster.ErrSendingRequest
	}

	if err := cluster.CheckStatusCode(res); err != nil {
		return nil, err
	}

	return res.Body, nil
}

// JobService invokes a service asynchronously
func JobService(c *cluster.Cluster, name string, token string, endpoint string, input io.Reader) (responseBody io.ReadCloser, err error) {

	var jobServiceURL *url.URL
	if token != "" {
		jobServiceURL, err = url.Parse(endpoint)
	} else {
		jobServiceURL, err = url.Parse(c.Endpoint)
	}

	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	jobServiceURL.Path = path.Join(jobServiceURL.Path, jobPath, name)
	// Make the request
	req, err := http.NewRequest(http.MethodPost, jobServiceURL.String(), input)
	if err != nil {
		return nil, cluster.ErrMakingRequest
	}

	var res *http.Response
	if token != "" {
		bearer := "Bearer " + strings.TrimSpace(token)
		req.Header.Add("Authorization", bearer)

		client := &http.Client{}
		res, err = client.Do(req)
	} else {

		// Get the service
		svc, err := GetService(c, name)
		if err != nil {
			return nil, err
		}
		// Add service's token if defined (OSCAR >= v2.2.0)
		if svc.Token != "" {
			bearer := "Bearer " + strings.TrimSpace(svc.Token)
			req.Header.Add("Authorization", bearer)
		}
		// Update cluster client timeout
		client := c.GetClient()
		client.Timeout = time.Second * 300

		// Update client transport to remove basic auth
		client.Transport = &http.Transport{
			// Enable/disable ssl verification
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !c.SSLVerify},
		}

		res, err = client.Do(req)
	}

	if err != nil {
		return nil, cluster.ErrSendingRequest
	}

	if err := cluster.CheckStatusCode(res); err != nil {
		return nil, err
	}

	return res.Body, nil
}

func getScriptPath(scriptPath string, servicePath string) string {
	return filepath.Dir(servicePath) + "/" + scriptPath
}
