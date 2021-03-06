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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/grycap/oscar/v2/pkg/types"
)

const infoPath = "/system/info"
const configPath = "/system/config"

var (
	// ErrParsingEndpoint error message for cluster endpoint parsing
	ErrParsingEndpoint = errors.New("error parsing the cluster endpoint, please check that you have typed it correctly")
	// ErrMakingRequest error message for making requests
	ErrMakingRequest = errors.New("error making the request")
	// ErrSendingRequest error message for sending requests
	ErrSendingRequest = errors.New("unable to communicate with the cluster, please check that the endpoint is well typed and accessible")
)

// Cluster defines the configuration of an OSCAR cluster
type Cluster struct {
	Endpoint     string `json:"endpoint" binding:"required"`
	AuthUser     string `json:"auth_user" binding:"required"`
	AuthPassword string `json:"auth_password" binding:"required"`
	SSLVerify    bool   `json:"ssl_verify" binding:"required"`
	Memory       string `json:"memory" binding:"required"`
	LogLevel     string `json:"log_level" binding:"required"`
}

type basicAuthRoundTripper struct {
	username  string
	password  string
	transport http.RoundTripper
}

// RoundTrip function to implement the RoundTripper interface adding basic auth headers
func (bart *basicAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Add basic auth to requests
	req.SetBasicAuth(bart.username, bart.password)
	return bart.transport.RoundTrip(req)
}

// GetClient returns an HTTP client to communicate with the cluster
func (cluster *Cluster) GetClient() *http.Client {
	var transport http.RoundTripper = &http.Transport{
		// Enable/disable ssl verification
		TLSClientConfig: &tls.Config{InsecureSkipVerify: !cluster.SSLVerify},
	}

	transport = &basicAuthRoundTripper{
		username:  cluster.AuthUser,
		password:  cluster.AuthPassword,
		transport: transport,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   time.Second * 20,
	}
}

// GetClusterInfo returns info from an OSCAR cluster
func (cluster *Cluster) GetClusterInfo() (info types.Info, err error) {
	getInfoURL, err := url.Parse(cluster.Endpoint)
	if err != nil {
		return info, ErrParsingEndpoint
	}
	getInfoURL.Path = path.Join(getInfoURL.Path, infoPath)

	req, err := http.NewRequest(http.MethodGet, getInfoURL.String(), nil)
	if err != nil {
		return info, ErrMakingRequest
	}

	res, err := cluster.GetClient().Do(req)
	if err != nil {
		return info, ErrSendingRequest
	}
	defer res.Body.Close()

	if err := CheckStatusCode(res); err != nil {
		return info, err
	}

	// Decode the response body into the info struct
	json.NewDecoder(res.Body).Decode(&info)

	return info, nil
}

// GetClusterConfig returns the config of an OSCAR cluster
func (cluster *Cluster) GetClusterConfig() (cfg types.Config, err error) {
	getConfigURL, err := url.Parse(cluster.Endpoint)
	if err != nil {
		return cfg, ErrParsingEndpoint
	}
	getConfigURL.Path = path.Join(getConfigURL.Path, configPath)

	req, err := http.NewRequest(http.MethodGet, getConfigURL.String(), nil)
	if err != nil {
		return cfg, ErrMakingRequest
	}

	res, err := cluster.GetClient().Do(req)
	if err != nil {
		return cfg, ErrSendingRequest
	}
	defer res.Body.Close()

	if err := CheckStatusCode(res); err != nil {
		return cfg, err
	}

	// Decode the response body into the info struct
	json.NewDecoder(res.Body).Decode(&cfg)

	return cfg, nil
}

// CheckStatusCode checks if a cluster response is valid and returns an appropriate error if not
func CheckStatusCode(res *http.Response) error {
	if res.StatusCode >= 200 && res.StatusCode <= 204 {
		return nil
	}
	if res.StatusCode == 401 {
		return errors.New("invalid credentials")
	}
	if res.StatusCode == 404 {
		return errors.New("not found")
	}
	if res.StatusCode == 502 {
		return errors.New("the service is not ready yet, please wait until it's ready or check if something failed")
	}
	// Create an error from the failed response body
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("cannot read the response: %v", err)
	}
	return errors.New(string(body))
}
