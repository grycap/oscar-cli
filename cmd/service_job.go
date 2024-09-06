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
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/spf13/cobra"
)

var DEFAULT_PROVIDER = "minio.default"

func serviceJobFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}
	// Parse input (only --input or --text-input are allowed) (AND one of them is required)
	inputFilePath, _ := cmd.Flags().GetString("input")
	if inputFilePath == "" {
		return errors.New("you must specify \"--input\" or \"--text-input\" flag")
	}
	// Make the request
	err = storage.PutFile(conf.Oscar[cluster], args[0], DEFAULT_PROVIDER, inputFilePath, "")
	if err != nil {
		return err
	}

	return nil
}

func makeServiceJobCmd() *cobra.Command {
	serviceRunCmd := &cobra.Command{
		Use:     "job SERVICE_NAME --input",
		Short:   "Invoke a service asynchronously (only compatible with MinIO providers)",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"job", "j"},
		RunE:    serviceRunFunc,
	}

	serviceRunCmd.Flags().StringP("file-input", "f", "", "input file for the request")

	return serviceRunCmd
}
