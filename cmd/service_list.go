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
	"text/tabwriter"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/spf13/cobra"
)

func serviceListFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	svcList, err := service.ListServices(conf.Oscar[cluster])
	if err != nil {
		return err
	}

	if len(svcList) == 0 {
		fmt.Println("There are no services in the cluster")
	} else {
		// Prepare tabwriter
		w := new(tabwriter.Writer)
		w.Init(os.Stdout, 0, 8, 2, '\t', 0)
		// Print header
		fmt.Fprintln(w, "NAME\tCONTAINER\tCPU\tMEMORY")
		// Print services
		for _, s := range svcList {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Image, s.CPU, s.Memory)
		}
		w.Flush()
	}

	return nil
}

func makeServiceListCmd() *cobra.Command {
	serviceListCmd := &cobra.Command{
		Use:     "list",
		Short:   "List the available services in a cluster",
		Args:    cobra.NoArgs,
		Aliases: []string{"ls"},
		RunE:    serviceListFunc,
	}

	serviceListCmd.Flags().StringP("cluster", "c", "", "set the cluster")

	return serviceListCmd
}
