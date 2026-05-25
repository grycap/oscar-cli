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
	"encoding/json"
	"fmt"

	"github.com/grycap/oscar-cli/v2/pkg/config"
	"github.com/spf13/cobra"
)

func healthFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	health, err := conf.Oscar[clusterName].HealthCheck()
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(health); err != nil {
			return err
		}
	default:
		fmt.Fprintf(cmd.OutOrStdout(), "Health: %s\n", health.Status)
		if health.RawBody != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Response: %s\n", health.RawBody)
		}
	}

	return nil
}

func makeHealthCmd() *cobra.Command {
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Check the health status of an OSCAR cluster",
		Args:  cobra.NoArgs,
		RunE:  healthFunc,
	}

	healthCmd.Flags().StringP("cluster", "c", "", "set the cluster")
	healthCmd.Flags().StringP("output", "o", "text", "output format (json)")

	return healthCmd
}
