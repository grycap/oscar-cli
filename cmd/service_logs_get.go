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

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/spf13/cobra"
)

func serviceLogsGetFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	showTimestamps, _ := cmd.Flags().GetBool("show-timestamps")

	logs, err := service.GetLogs(conf.Oscar[cluster], args[0], args[1], showTimestamps)
	if err != nil {
		return err
	}

	fmt.Println(logs)

	return nil
}

func makeServiceLogsGetCmd() *cobra.Command {
	serviceLogsGetCmd := &cobra.Command{
		Use:     "get SERVICE_NAME JOB_NAME",
		Short:   "Get the logs from a service",
		Args:    cobra.ExactArgs(2),
		Aliases: []string{"g"},
		RunE:    serviceLogsGetFunc,
	}

	serviceLogsGetCmd.Flags().BoolP("show-timestamps", "t", false, "show timestamps in the logs")

	return serviceLogsGetCmd
}
