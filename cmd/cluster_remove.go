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
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/spf13/cobra"
)

func clusterRemoveFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	if err := conf.RemoveCluster(configPath, args[0]); err != nil {
		return err
	}

	return nil
}

func makeClusterRemoveCmd() *cobra.Command {
	clusterRemoveCmd := &cobra.Command{
		Use:     "delete IDENTIFIER",
		Short:   "Delete a cluster from the configuration file",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"d", "del", "remove", "rm"},
		RunE:    clusterRemoveFunc,
	}

	return clusterRemoveCmd
}
