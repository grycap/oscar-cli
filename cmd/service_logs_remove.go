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
	"errors"
	"fmt"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/spf13/cobra"
)

func serviceLogsRemoveFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	all, _ := cmd.Flags().GetBool("all")
	succeeded, _ := cmd.Flags().GetBool("succeeded")

	if succeeded {
		err := service.RemoveLogs(conf.Oscar[cluster], args[0], false)
		if err == nil {
			fmt.Printf("All succeeded jobs from service \"%s\" removed successfully\n", args[0])
		}
		return err
	}

	if all {
		err := service.RemoveLogs(conf.Oscar[cluster], args[0], true)
		if err == nil {
			fmt.Printf("All jobs from service \"%s\" removed successfully\n", args[0])
		}
		return err
	}

	for _, jobName := range args[1:] {
		err := service.RemoveLog(conf.Oscar[cluster], args[0], jobName)
		if err != nil {
			return err
		}
		fmt.Printf("Job \"%s\" from service \"%s\" removed successfully\n", jobName, args[0])
	}

	return nil
}

func checkServiceLogsRemoveArgs(cmd *cobra.Command, args []string) error {
	err := cobra.MinimumNArgs(1)(cmd, args)
	if err != nil {
		return err
	}

	all, _ := cmd.Flags().GetBool("all")
	succeeded, _ := cmd.Flags().GetBool("succeeded")

	if all && succeeded {
		return errors.New("only one of \"--all\" or \"--succeeded\" flags can be set")
	}

	if all || succeeded {
		return cobra.ExactArgs(1)(cmd, args)
	}

	return cobra.MinimumNArgs(2)(cmd, args)
}

func makeServiceLogsRemoveCmd() *cobra.Command {
	serviceLogsRemoveCmd := &cobra.Command{
		Use:     "remove SERVICE_NAME {JOB_NAME... | --succeeded | --all}",
		Short:   "Remove a service's job along with its logs",
		Args:    checkServiceLogsRemoveArgs,
		Aliases: []string{"rm"},
		RunE:    serviceLogsRemoveFunc,
	}

	serviceLogsRemoveCmd.Flags().BoolP("all", "a", false, "remove all logs from the service")
	serviceLogsRemoveCmd.Flags().BoolP("succeeded", "s", false, "remove succeeded logs from the service")

	return serviceLogsRemoveCmd
}
