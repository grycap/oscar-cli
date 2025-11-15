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

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/spf13/cobra"
)

func clusterStatusFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	status, err := conf.Oscar[clusterName].GetClusterStatus()
	if err != nil {
		return err
	}

	output, err := yaml.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to serialize cluster status: %w", err)
	}

	fmt.Print(string(output))
	return nil
}

func makeClusterStatusCmd() *cobra.Command {
	clusterStatusCmd := &cobra.Command{
		Use:     "status",
		Short:   "Show status information of an OSCAR cluster",
		Args:    cobra.NoArgs,
		Aliases: []string{"s"},
		RunE:    clusterStatusFunc,
	}

	clusterStatusCmd.Flags().StringP("cluster", "c", "", "set the cluster")

	return clusterStatusCmd
}
