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
	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/spf13/cobra"
)

func quotaGetFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	userID := ""
	if len(args) > 0 {
		userID = args[0]
	}

	quota, err := cluster.GetQuota(conf.Oscar[clusterName], userID)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(quota)
	if err != nil {
		return fmt.Errorf("failed to serialize quota: %w", err)
	}
	fmt.Print(string(out))

	return nil
}

func makeQuotaGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "get [USER_ID]",
		Short:   "Get quota for a user",
		Args:    cobra.MaximumNArgs(1),
		Aliases: []string{"g"},
		RunE:    quotaGetFunc,
	}
}
