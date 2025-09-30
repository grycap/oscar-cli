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
	"net/http"
	"path"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v3/pkg/types"
	"github.com/spf13/cobra"
)

var (
	failureString        = color.New(color.FgRed).Sprint("✗ ")
	successString        = color.New(color.FgGreen).Sprint("✓ ")
	destinationClusterID string
)

func applyFunc(cmd *cobra.Command, args []string) error {
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

	if destinationClusterID != "" {
		if err := conf.CheckCluster(destinationClusterID); err != nil {
			return err
		}
	}

	// Pre-loop to check all clusters and get its MinIO storage provider
	clusters := map[string]types.Cluster{}
	minioProviders := map[string]*types.MinIOProvider{}
	for _, element := range fdl.Functions.Oscar {
		for clusterName := range element {
			targetCluster := clusterName
			if destinationClusterID != "" {
				targetCluster = destinationClusterID
			}
			if _, exists := clusters[targetCluster]; exists {
				continue
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

	fmt.Printf("Applying file \"%s\"...\n", path.Base(args[0]))

	for _, element := range fdl.Functions.Oscar {
		for clusterName, svc := range element {
			targetCluster := clusterName
			if destinationClusterID != "" {
				targetCluster = destinationClusterID
			}

			svc.ClusterID = targetCluster

			msg := fmt.Sprintf(" Creating service \"%s\" in cluster \"%s\"", svc.Name, targetCluster)
			method := http.MethodPost

			// Make and start the spinner
			s := spinner.New(spinner.CharSets[78], time.Millisecond*100)
			s.Suffix = msg
			s.FinalMSG = fmt.Sprintf("%s%s\n", successString, msg)
			s.Start()

			// Add (and overwrite) clusters
			if svc.Clusters == nil {
				// Initialize map
				svc.Clusters = map[string]types.Cluster{}
			}
			for cn, c := range clusters {
				svc.Clusters[cn] = c
			}

			// Add (and overwrite) MinIO providers
			if svc.StorageProviders == nil {
				// Initialize StorageProviders
				svc.StorageProviders = &types.StorageProviders{}
			}
			if svc.StorageProviders.MinIO == nil {
				// Initialize map
				svc.StorageProviders.MinIO = map[string]*types.MinIOProvider{}
			}

			// Check if service exists in cluster in order to create or edit it
			if exists := serviceExists(svc, conf.Oscar[targetCluster]); exists {
				msg = fmt.Sprintf(" Editing service \"%s\" in cluster \"%s\"", svc.Name, targetCluster)
				method = http.MethodPut
				s.Suffix = msg
				s.FinalMSG = fmt.Sprintf("%s%s\n", successString, msg)
			}

			// Apply the service
			err = service.ApplyService(svc, conf.Oscar[targetCluster], method)
			if err != nil {
				s.FinalMSG = fmt.Sprintf("%s%s\n", failureString, msg)
				s.Stop()
				return err
			}
			s.Stop()
		}
	}

	return nil
}

func serviceExists(svc *types.Service, c *cluster.Cluster) bool {
	_, err := service.GetService(c, svc.Name)
	return err == nil
}

func makeApplyCmd() *cobra.Command {
	applyCmd := &cobra.Command{
		Use:     "apply FDL_FILE",
		Short:   "Apply a FDL file to create or edit services in clusters",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"a"},
		RunE:    applyFunc,
	}

	applyCmd.PersistentFlags().StringVar(&configPath, "config", defaultConfigPath, "set the location of the config file (YAML or JSON)")
	applyCmd.Flags().StringVarP(&destinationClusterID, "cluster", "c", "", "override the cluster id defined in the FDL file")

	return applyCmd
}
