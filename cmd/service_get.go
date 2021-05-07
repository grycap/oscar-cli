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
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/spf13/cobra"
)

func serviceGetFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	svc, err := service.GetService(conf.Oscar[cluster], args[0])
	if err != nil {
		return err
	}

	yaml, _ := yaml.Marshal(svc)
	fmt.Print(string(yaml))

	return nil
}

func makeServiceGetCmd() *cobra.Command {
	serviceGetCmd := &cobra.Command{
		Use:     "get SERVICE_NAME",
		Short:   "Get the definition of a service",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"g"},
		RunE:    serviceGetFunc,
	}

	serviceGetCmd.Flags().StringP("cluster", "c", "", "set the cluster")

	return serviceGetCmd
}
