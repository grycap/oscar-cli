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
	"github.com/grycap/oscar/v2/pkg/types"
)

const logsPath = "/system/logs"

// ListLogs returns a map with all the available logs from the given service
func ListLogs(c *cluster.Cluster, name string) (logMap map[string]*types.JobInfo, err error) {
	listLogsUrl, err := url.Parse(c.Endpoint)
	if err != nil {
		return logMap, cluster.ErrParsingEndpoint
	}
	listLogsUrl.Path = path.Join(listLogsUrl.Path, logsPath, name)

	req, err := http.NewRequest(http.MethodGet, listLogsUrl.String(), nil)
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
	getLogsUrl, err := url.Parse(c.Endpoint)
	if err != nil {
		return logs, cluster.ErrParsingEndpoint
	}
	getLogsUrl.Path = path.Join(getLogsUrl.Path, logsPath, svcName, jobName)

	if timestamps {
		q := getLogsUrl.Query()
		q.Set("timestamps", "true")
		getLogsUrl.RawQuery = q.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, getLogsUrl.String(), nil)
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
	removeLogUrl, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	removeLogUrl.Path = path.Join(removeLogUrl.Path, logsPath, svcName, jobName)

	req, err := http.NewRequest(http.MethodDelete, removeLogUrl.String(), nil)
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
	removeLogsUrl, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	removeLogsUrl.Path = path.Join(removeLogsUrl.Path, logsPath, svcName)

	if all {
		q := removeLogsUrl.Query()
		q.Set("all", "true")
		removeLogsUrl.RawQuery = q.Encode()
	}

	req, err := http.NewRequest(http.MethodDelete, removeLogsUrl.String(), nil)
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
