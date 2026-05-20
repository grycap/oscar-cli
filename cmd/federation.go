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
	"github.com/spf13/cobra"
)

func federationFunc(cmd *cobra.Command, args []string) {
	cmd.Help()
}

func makeFederationCmd() *cobra.Command {
	federationCmd := &cobra.Command{
		Use:   "federation",
		Short: "Manage service federation replicas",
		Args:  cobra.NoArgs,
		Run:   federationFunc,
	}

	federationCmd.PersistentFlags().StringP("cluster", "c", "", "set the cluster")
	federationCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath, "set the location of the config file (YAML or JSON)")

	federationCmd.AddCommand(makeFederationGetCmd())
	federationCmd.AddCommand(makeFederationCreateCmd())
	federationCmd.AddCommand(makeFederationUpdateCmd())
	federationCmd.AddCommand(makeFederationDeleteCmd())

	return federationCmd
}
