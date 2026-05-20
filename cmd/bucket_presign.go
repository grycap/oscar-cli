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
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/spf13/cobra"
)

func bucketPresignFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	expires, _ := cmd.Flags().GetInt("expires")

	url, err := storage.PresignBucket(conf.Oscar[clusterName], args[0], args[1], expires)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), url)

	return nil
}

func makeBucketPresignCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "presign BUCKET_NAME FILE_NAME",
		Short: "Generate a presigned URL for a file in a bucket",
		Args:  cobra.ExactArgs(2),
		RunE:  bucketPresignFunc,
	}

	cmd.Flags().Int("expires", 0, "expiration time in seconds")

	return cmd
}
