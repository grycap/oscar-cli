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
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/spf13/cobra"
)

func bucketGetFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	bucketName := args[0]

	pageToken, _ := cmd.Flags().GetString("page")
	limit, _ := cmd.Flags().GetInt("limit")
	allPages, _ := cmd.Flags().GetBool("all")

	opts := &storage.BucketListOptions{
		PageToken:    strings.TrimSpace(pageToken),
		Limit:        limit,
		AutoPaginate: allPages,
	}

	result, err := storage.ListBucketObjectsWithOptions(conf.Oscar[clusterName], bucketName, opts)
	if err != nil {
		return err
	}

	prefix, _ := cmd.Flags().GetString("prefix")
	if trimmed := strings.TrimSpace(prefix); trimmed != "" {
		filtered := result.Objects[:0]
		for _, obj := range result.Objects {
			if strings.HasPrefix(obj.Name, trimmed) {
				filtered = append(filtered, obj)
			}
		}
		result.Objects = filtered
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		if err := bucketGetPrintJSON(cmd, result.Objects); err != nil {
			return err
		}
	case "table":
		bucketGetPrintTable(cmd, bucketName, result.Objects)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}

	if !allPages && result.NextPage != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "\nMore objects are available. Continue listing with --page %q or fetch everything with --all.\n", result.NextPage)
	}

	return nil
}

func bucketGetPrintJSON(cmd *cobra.Command, objects []*storage.BucketObject) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(objects)
}

func bucketGetPrintTable(cmd *cobra.Command, bucketName string, objects []*storage.BucketObject) {
	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE (B)\tLAST MODIFIED")
	for _, obj := range objects {
		lastModified := "-"
		if !obj.LastModified.IsZero() {
			lastModified = obj.LastModified.Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "%s\t%d\t%s\n", obj.Name, obj.Size, lastModified)
	}
	w.Flush()

	if len(objects) == 0 {
		fmt.Fprintf(out, "Bucket %q has no objects.\n", bucketName)
	}
}

func makeBucketGetCmd() *cobra.Command {
	bucketGetCmd := &cobra.Command{
		Use:     "get BUCKET_NAME",
		Short:   "Get the contents of a bucket",
		Long:    "Retrieve information about a specific OSCAR bucket using the cluster storage API.",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"g"},
		RunE:    bucketGetFunc,
	}

	bucketGetCmd.Flags().StringP("cluster", "c", "", "set the cluster")
	bucketGetCmd.Flags().StringP("output", "o", "table", "output format (table or json)")
	bucketGetCmd.Flags().String("prefix", "", "filter objects by key prefix")
	bucketGetCmd.Flags().String("page", "", "continuation token returned by a previous call")
	bucketGetCmd.Flags().Int("limit", 0, "maximum number of objects to request per call (default server limit)")
	bucketGetCmd.Flags().Bool("all", false, "automatically retrieve every page of results")

	return bucketGetCmd
}
