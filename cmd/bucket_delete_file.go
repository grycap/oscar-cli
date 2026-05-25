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
	"github.com/grycap/oscar-cli/v2/pkg/config"
	"github.com/grycap/oscar-cli/v2/pkg/storage"
	"github.com/spf13/cobra"
)

func bucketDeleteFileFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	bucketName := args[0]
	remotePath := args[1]
	pathBucket := bucketName + "/" + remotePath

	return storage.DeleteFile(conf.Oscar[clusterName], storage.DefaultStorageProvider[0], pathBucket)

}

func makeBucketDeleteFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete-file BUCKET_NAME REMOTE_PATH",
		Short:   "Delete a file from a MinIO bucket",
		Long:    "Delete a file at the specified remote path from a MinIO bucket.",
		Args:    cobra.ExactArgs(2),
		Aliases: []string{"df"},
		RunE:    bucketDeleteFileFunc,
	}

	cmd.Flags().StringP("cluster", "c", "", "set the cluster")

	return cmd
}
