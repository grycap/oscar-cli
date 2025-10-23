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
	"path"
	"time"

	"github.com/briandowns/spinner"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v3/pkg/types"
	"github.com/spf13/cobra"
)

func deleteFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	// Read file
	fdl, err := service.ReadFDL(args[0])
	if err != nil {
		return err
	}

	// Pre-loop to check all clusters and get its MinIO storage provider
	clusters := map[string]types.Cluster{}
	minioProviders := map[string]*types.MinIOProvider{}
	for _, element := range fdl.Functions.Oscar {
		for clusterName := range element {
			default_cluster, _ := cmd.Flags().GetBool("default")
			targetCluster, errCluster := conf.GetCluster(default_cluster, destinationClusterID, clusterName)
			if errCluster != nil {
				return errCluster
			}
			// Check if cluster is defined
			err := conf.CheckCluster(targetCluster)
			if err != nil {
				return err
			}

			// Get cluster info
			clusterInfo, err := conf.Oscar[targetCluster].GetClusterConfig()
			if err != nil {
				return err
			}

			// Append cluster
			clusters[targetCluster] = types.Cluster{
				Endpoint:     conf.Oscar[targetCluster].Endpoint,
				AuthUser:     conf.Oscar[targetCluster].AuthUser,
				AuthPassword: conf.Oscar[targetCluster].AuthPassword,
				SSLVerify:    conf.Oscar[targetCluster].SSLVerify,
			}

			// Append MinIO provider
			minioProviders[targetCluster] = clusterInfo.MinIOProvider
		}
	}

	fmt.Printf("Deleting file \"%s\"...\n", path.Base(args[0]))

	for _, element := range fdl.Functions.Oscar {
		for clusterName, svc := range element {
			default_cluster, _ := cmd.Flags().GetBool("default")
			targetCluster, errCluster := conf.GetCluster(default_cluster, destinationClusterID, clusterName)
			if errCluster != nil {
				return errCluster
			}
			msg := fmt.Sprintf(" Removing service \"%s\" in cluster \"%s\"", svc.Name, targetCluster)

			// Make and start the spinner
			s := spinner.New(spinner.CharSets[78], time.Millisecond*100)
			s.Suffix = msg
			s.FinalMSG = fmt.Sprintf("%s%s\n", successString, msg)
			s.Start()

			// Remove the service
			if err := service.RemoveService(conf.Oscar[targetCluster], svc.Name); err != nil {
				s.FinalMSG = fmt.Sprintf("%s%s\n", failureString, msg)
				s.Stop()
				return err
			}
			s.Stop()
		}
	}

	return nil
}

func makeDeleteCmd() *cobra.Command {
	applyCmd := &cobra.Command{
		Use:     "delete FDL_FILE",
		Short:   "Delete a FDL file to create or edit services in clusters",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"d"},
		RunE:    deleteFunc,
	}

	applyCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath, "set the location of the config file (YAML or JSON)")
	applyCmd.Flags().Bool("default", false, "override the cluster id defined in config file")

	return applyCmd
}
