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
	"io/ioutil"
	"net/http"
	"net/url"
	"path"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar/v3/pkg/types"
)

const logsPath = "/system/logs"

type JobsResponse struct {
	Jobs         map[string]*types.JobInfo `json:"jobs"`
	NextPage     string                    `json:"next_page,omitempty"`
	RemainingJob *int64                    `json:"remaining_jobs,omitempty"`
}

// ListLogs returns a map with all the available logs from the given service
func ListLogs(c *cluster.Cluster, name string, page string) (logMap JobsResponse, err error) {
	listLogsURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return logMap, cluster.ErrParsingEndpoint
	}
	listLogsURL.Path = path.Join(listLogsURL.Path, logsPath, name)
	query := listLogsURL.Query()
	query.Set("page", page)
	listLogsURL.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodGet, listLogsURL.String(), nil)
	if err != nil {
		return logMap, cluster.ErrMakingRequest
	}

	res, err := c.GetClient().Do(req)
	if err != nil {
		return logMap, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return logMap, err
	}

	// Decode the response body into the logMap
	err = json.NewDecoder(res.Body).Decode(&logMap)
	if err != nil {
		return logMap, err
	}

	return logMap, nil
}

// GetLogs get the logs from a service's job
func GetLogs(c *cluster.Cluster, svcName string, jobName string, timestamps bool) (logs string, err error) {
	getLogsURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return logs, cluster.ErrParsingEndpoint
	}
	getLogsURL.Path = path.Join(getLogsURL.Path, logsPath, svcName, jobName)

	if timestamps {
		q := getLogsURL.Query()
		q.Set("timestamps", "true")
		getLogsURL.RawQuery = q.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, getLogsURL.String(), nil)
	if err != nil {
		return logs, cluster.ErrMakingRequest
	}

	res, err := c.GetClient().Do(req)
	if err != nil {
		return logs, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return logs, err
	}

	// Read the response body
	byteLogs, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return logs, err
	}

	return string(byteLogs), nil
}

// RemoveLog removes the specified log (jobName) from a service in the cluster
func RemoveLog(c *cluster.Cluster, svcName, jobName string) error {
	removeLogURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	removeLogURL.Path = path.Join(removeLogURL.Path, logsPath, svcName, jobName)

	req, err := http.NewRequest(http.MethodDelete, removeLogURL.String(), nil)
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

// RemoveLogs removes completed or all logs (jobs) from a service in the cluster
func RemoveLogs(c *cluster.Cluster, svcName string, all bool) error {
	removeLogsURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	removeLogsURL.Path = path.Join(removeLogsURL.Path, logsPath, svcName)

	if all {
		q := removeLogsURL.Query()
		q.Set("all", "true")
		removeLogsURL.RawQuery = q.Encode()
	}

	req, err := http.NewRequest(http.MethodDelete, removeLogsURL.String(), nil)
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
