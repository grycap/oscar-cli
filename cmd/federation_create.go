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
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v4/pkg/types"
	"github.com/spf13/cobra"
)

func federationCreateFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	replicaType, _ := cmd.Flags().GetString("type")
	clusterID, _ := cmd.Flags().GetString("cluster-id")
	serviceName, _ := cmd.Flags().GetString("service-name")
	url, _ := cmd.Flags().GetString("url")
	priority, _ := cmd.Flags().GetUint("priority")

	replica := types.Replica{
		Type:        replicaType,
		ClusterID:   clusterID,
		ServiceName: serviceName,
		URL:         url,
		SSLVerify:   true,
		Priority:    priority,
	}

	if err := service.CreateFederation(conf.Oscar[clusterName], args[0], []types.Replica{replica}); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Federation replica created for service %q\n", args[0])

	return nil
}

func makeFederationCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create SERVICE_NAME",
		Short: "Create a federation replica for a service",
		Args:  cobra.ExactArgs(1),
		RunE:  federationCreateFunc,
	}

	cmd.Flags().String("type", "oscar", "replica type (oscar or endpoint)")
	cmd.Flags().String("cluster-id", "", "cluster ID (for oscar type)")
	cmd.Flags().String("service-name", "", "service name in the replica cluster (for oscar type)")
	cmd.Flags().String("url", "", "endpoint URL (for endpoint type)")
	cmd.Flags().Uint("priority", 0, "delegation priority (0 = highest)")

	return cmd
}
