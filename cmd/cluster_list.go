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

	"github.com/fatih/color"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/spf13/cobra"
)

func clusterListFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	if len(conf.Oscar) == 0 {
		fmt.Println("There are no defined clusters in the config file")
	} else {
		def := conf.Default

		// Configure bold font
		bold := color.New(color.Bold)

		// Print the clusters
		for k, v := range conf.Oscar {
			if k == def {
				// Print the default bold
				bold.Printf("%s (%s) (Default)\n", k, v.Endpoint)
			} else {
				fmt.Printf("%s (%s)\n", k, v.Endpoint)
			}
		}
	}

	return nil
}

func makeClusterListCmd() *cobra.Command {
	clusterListCmd := &cobra.Command{
		Use:     "list",
		Short:   "List the configured OSCAR clusters",
		Args:    cobra.NoArgs,
		Aliases: []string{"ls"},
		RunE:    clusterListFunc,
	}

	return clusterListCmd
}
