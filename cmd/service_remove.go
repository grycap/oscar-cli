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
	"time"

	"github.com/briandowns/spinner"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/spf13/cobra"
)

func serviceRemoveFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	for _, svcName := range args {
		msg := fmt.Sprintf(" Removing service \"%s\"", svcName)

		// Make and start the spinner
		s := spinner.New(spinner.CharSets[78], time.Millisecond*100)
		s.Suffix = msg
		s.FinalMSG = fmt.Sprintf("%s%s\n", successString, msg)
		s.Start()

		// Remove the service
		if err := service.RemoveService(conf.Oscar[cluster], svcName); err != nil {
			s.FinalMSG = fmt.Sprintf("%s%s\n", failureString, msg)
			s.Stop()
			return err
		}
		s.Stop()
	}

	return nil
}

func makeServiceRemoveCmd() *cobra.Command {
	serviceGetCmd := &cobra.Command{
		Use:     "remove SERVICE_NAME...",
		Short:   "Remove a service from the cluster",
		Args:    cobra.MinimumNArgs(1),
		Aliases: []string{"rm"},
		RunE:    serviceRemoveFunc,
	}

	serviceGetCmd.Flags().StringP("cluster", "c", "", "set the cluster")

	return serviceGetCmd
}
