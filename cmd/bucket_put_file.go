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

func bucketPutFunc(cmd *cobra.Command, args []string) error {
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
	localPath := args[1]
	remotePath := args[2]
	pathBucket := bucketName + "/" + remotePath

	noProgress, err := cmd.Flags().GetBool("no-progress")
	if err != nil {
		return err
	}

	var transferOpt *storage.TransferOption
	if noProgress {
		transferOpt = &storage.TransferOption{ShowProgress: false}
	}

	return storage.PutFile(conf.Oscar[clusterName], storage.DefaultStorageProvider[0], localPath, pathBucket, transferOpt)
}

func makeBucketPutFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "put-file BUCKET_NAME LOCAL_FILE REMOTE_PATH",
		Short:   "Upload a file to a MinIO bucket",
		Long:    "Upload a local file to a MinIO bucket at the specified remote path.",
		Args:    cobra.ExactArgs(3),
		Aliases: []string{"pf"},
		RunE:    bucketPutFunc,
	}

	cmd.Flags().Bool("no-progress", false, "disable progress bar output")

	return cmd
}
