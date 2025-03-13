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
			// Check if cluster is defined
			err := conf.CheckCluster(clusterName)
			if err != nil {
				return err
			}

			// Get cluster info
			clusterInfo, err := conf.Oscar[clusterName].GetClusterConfig()
			if err != nil {
				return err
			}

			// Append cluster
			clusters[clusterName] = types.Cluster{
				Endpoint:     conf.Oscar[clusterName].Endpoint,
				AuthUser:     conf.Oscar[clusterName].AuthUser,
				AuthPassword: conf.Oscar[clusterName].AuthPassword,
				SSLVerify:    conf.Oscar[clusterName].SSLVerify,
			}

			// Append MinIO provider
			minioProviders[clusterName] = clusterInfo.MinIOProvider
		}
	}

	fmt.Printf("Deleting file \"%s\"...\n", path.Base(args[0]))

	for _, element := range fdl.Functions.Oscar {
		for clusterName, svc := range element {

			msg := fmt.Sprintf(" Removing service \"%s\" in cluster \"%s\"", svc.Name, clusterName)

			// Make and start the spinner
			s := spinner.New(spinner.CharSets[78], time.Millisecond*100)
			s.Suffix = msg
			s.FinalMSG = fmt.Sprintf("%s%s\n", successString, msg)
			s.Start()

			// Remove the service
			if err := service.RemoveService(conf.Oscar[clusterName], svc.Name); err != nil {
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

	return applyCmd
}
