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

package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v3/pkg/types"
	"github.com/spf13/cobra"
)

const timeFormat = "2006-01-02 15:04:05"

func serviceLogsListFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	statusSlice, _ := cmd.Flags().GetStringSlice("status")

	logMap, err := service.ListLogs(conf.Oscar[cluster], args[0])
	if err != nil {
		return err
	}

	printLogMap(logMap, statusSlice)

	return nil
}

func printLogMap(logMap map[string]*types.JobInfo, statusSlice []string) {
	if len(logMap) == 0 {
		fmt.Println("This service has no logs")
	} else {
		// Prepare tabwriter
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 0, 8, 2, '\t', 0)
		// Print header
		fmt.Fprintln(w, "JOB NAME\tSTATUS\tCREATION TIME\tSTART TIME\tFINISH TIME")

		for jobName, jobInfo := range logMap {
			// Filter by status
			if len(statusSlice) > 0 {
				found := false
				for _, status := range statusSlice {
					if strings.EqualFold(status, jobInfo.Status) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// Prepare times
			creationTime := ""
			if jobInfo.CreationTime != nil {
				creationTime = jobInfo.CreationTime.Format(timeFormat)
			}
			startTime := ""
			if jobInfo.StartTime != nil {
				startTime = jobInfo.StartTime.Format(timeFormat)
			}
			finishTime := ""
			if jobInfo.FinishTime != nil {
				finishTime = jobInfo.FinishTime.Format(timeFormat)
			}

			// Print job's logs
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", jobName, jobInfo.Status, creationTime, startTime, finishTime)
		}
		w.Flush()
	}
}

func makeServiceLogsListCmd() *cobra.Command {
	serviceLogsListCmd := &cobra.Command{
		Use:     "list SERVICE_NAME",
		Short:   "List the logs from a service",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"ls"},
		RunE:    serviceLogsListFunc,
	}

	serviceLogsListCmd.Flags().StringSliceP("status", "s", []string{}, "filter by status (Pending, Running, Succeeded or Failed), multiple values can be specified by a comma-separated string")

	return serviceLogsListCmd
}
