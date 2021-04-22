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
	"github.com/spf13/cobra"
)

func clusterDefaultFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	def := conf.Default

	// Check the set flag
	set, _ := cmd.Flags().GetString("set")
	if set == "" {
		if conf.Default == "" {
			fmt.Println("There is no default cluster, please set it with the \"--set\" flag")
		} else {
			fmt.Println(def)
		}
	} else {
		// Check if the passed cluster exists
		if err := conf.SetDefault(configPath, set); err != nil {
			return err
		}
		fmt.Printf("The cluster \"%s\" has been set as default successfully\n", set)
	}

	return nil
}

func makeClusterDefaultCmd() *cobra.Command {
	clusterDefaultCmd := &cobra.Command{
		Use:     "default",
		Short:   "Show or set the default cluster",
		Args:    cobra.NoArgs,
		Aliases: []string{"d"},
		RunE:    clusterDefaultFunc,
	}

	clusterDefaultCmd.Flags().String("set", "", "set a default cluster by passing its IDENTIFIER")

	return clusterDefaultCmd
}
