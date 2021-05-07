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

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/spf13/cobra"
)

func clusterFunc(cmd *cobra.Command, args []string) {
	cmd.Help()
}

func makeClusterCmd() *cobra.Command {
	clusterCmd := &cobra.Command{
		Use:     "cluster",
		Short:   "Manages the configuration of clusters",
		Args:    cobra.NoArgs,
		Aliases: []string{"c"},
		Run:     clusterFunc,
	}

	clusterCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath, "set the location of the config file (YAML or JSON)")

	// Add subcommands
	clusterCmd.AddCommand(makeClusterAddCmd())
	clusterCmd.AddCommand(makeClusterRemoveCmd())
	clusterCmd.AddCommand(makeClusterInfoCmd())
	clusterCmd.AddCommand(makeClusterListCmd())
	clusterCmd.AddCommand(makeClusterDefaultCmd())

	return clusterCmd
}

func getCluster(cmd *cobra.Command, conf *config.Config) (cluster string, err error) {
	// Check if the cluster flag is set
	cluster, _ = cmd.Flags().GetString("cluster")

	if cluster == "" {
		if conf.Default == "" {
			cmd.SilenceUsage = false
			return "", errors.New("cluster not set, please provide it or set a default one")
		}
		cluster = conf.Default
	}

	if err := conf.CheckCluster(cluster); err != nil {
		return "", err
	}

	return cluster, nil
}
