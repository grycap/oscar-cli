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
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/spf13/cobra"
)

func serviceGetFileFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	err = storage.GetFile(conf.Oscar[cluster], args[0], args[1], args[2], args[3])
	if err != nil {
		return err
	}

	return nil
}

func makeServiceGetFileCmd() *cobra.Command {
	serviceGetFileCmd := &cobra.Command{
		Use:   "get-file SERVICE_NAME STORAGE_PROVIDER REMOTE_FILE LOCAL_FILE",
		Short: "Get a file from a service's storage provider",
		Long: `Get a file from a service's storage provider.

The STORAGE_PROVIDER argument follows the format STORAGE_PROVIDER_TYPE.STORAGE_PROVIDER_NAME,
being the STORAGE_PROVIDER_TYPE one of the three supported storage providers (MinIO, S3 or Onedata)
and the STORAGE_PROVIDER_NAME is the identifier for the provider set in the service's definition.`,
		Args:    cobra.ExactArgs(4),
		Aliases: []string{"gf"},
		RunE:    serviceGetFileFunc,
	}

	serviceGetFileCmd.Flags().StringP("cluster", "c", "", "set the cluster")

	return serviceGetFileCmd
}
