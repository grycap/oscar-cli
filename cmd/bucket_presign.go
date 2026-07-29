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

func bucketPresignFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	operation, _ := cmd.Flags().GetString("operation")
	expires, _ := cmd.Flags().GetInt64("expires")
	contentType, _ := cmd.Flags().GetString("content-type")
	extraHeadersStr, _ := cmd.Flags().GetString("extra-headers")

	extraHeaders := make(map[string]string)
	if trimmed := strings.TrimSpace(extraHeadersStr); trimmed != "" {
		pairs := strings.Split(trimmed, ",")
		for _, pair := range pairs {
			kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(kv) == 2 {
				extraHeaders[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	presignReq := &storage.PresignRequest{
		ObjectKey:    args[1],
		Operation:    operation,
		ExpiresIn:    expires,
		ContentType:  contentType,
		ExtraHeaders: extraHeaders,
	}

	url, err := storage.PresignBucket(conf.Oscar[clusterName], args[0], presignReq)
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

	cmd.Flags().StringP("operation", "X", "download", "HTTP method for the presigned URL (get, put, head, delete)")
	cmd.Flags().Int64("expires", 0, "expiration time in seconds")
	cmd.Flags().String("content-type", "", "content type for the presigned request")
	cmd.Flags().String("extra-headers", "", "comma-separated extra headers (key=value,key=value)")

	return cmd
}
