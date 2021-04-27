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

func servicePutFileFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	err = storage.PutFile(conf.Oscar[cluster], args[0], args[1], args[2], args[3])
	if err != nil {
		return err
	}

	return nil
}

// TODO: document STORAGE_PROVIDER identifier format (<STORAGE_PROVIDER_TYPE>.<STORAGE_PROVIDER_NAME>)
func makeServicePutFileCmd() *cobra.Command {
	servicePutFileCmd := &cobra.Command{
		Use:     "put-file SERVICE_NAME STORAGE_PROVIDER LOCAL_FILE REMOTE_FILE",
		Short:   "Put a file in a service's storage provider",
		Args:    cobra.ExactArgs(4),
		Aliases: []string{"pf"},
		RunE:    servicePutFileFunc,
	}

	servicePutFileCmd.Flags().StringP("cluster", "c", "", "set the cluster")

	return servicePutFileCmd
}
