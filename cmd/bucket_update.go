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
	"strings"

	"github.com/grycap/oscar-cli/v2/pkg/config"
	"github.com/grycap/oscar-cli/v2/pkg/storage"
	"github.com/spf13/cobra"
)

func bucketUpdateFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	visibility, _ := cmd.Flags().GetString("visibility")
	allowedUsersStr, _ := cmd.Flags().GetString("allowed-users")

	var allowedUsers []string
	if trimmed := strings.TrimSpace(allowedUsersStr); trimmed != "" {
		allowedUsers = strings.Split(trimmed, ",")
		for i := range allowedUsers {
			allowedUsers[i] = strings.TrimSpace(allowedUsers[i])
		}
	}

	if err := storage.UpdateBucket(conf.Oscar[clusterName], args[0], visibility, allowedUsers); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Bucket %q updated\n", args[0])

	return nil
}

func makeBucketUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update BUCKET_NAME",
		Short: "Update a bucket's visibility and allowed users",
		Args:  cobra.ExactArgs(1),
		RunE:  bucketUpdateFunc,
	}

	cmd.Flags().String("visibility", "", "bucket visibility (public, private)")
	cmd.Flags().String("allowed-users", "", "comma-separated list of allowed users")

	return cmd
}
