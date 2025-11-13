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

func bucketListFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	bucketName := args[0]
	objects, err := storage.ListBucketObjects(conf.Oscar[clusterName], bucketName)
	if err != nil {
		return err
	}

	prefix, _ := cmd.Flags().GetString("prefix")
	if trimmed := strings.TrimSpace(prefix); trimmed != "" {
		filtered := objects[:0]
		for _, obj := range objects {
			if strings.HasPrefix(obj.Name, trimmed) {
				filtered = append(filtered, obj)
			}
		}
		objects = filtered
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		return bucketListPrintJSON(cmd, objects)
	case "table":
		bucketListPrintTable(cmd, bucketName, objects)
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func bucketListPrintJSON(cmd *cobra.Command, objects []*storage.BucketObject) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(objects)
}

func bucketListPrintTable(cmd *cobra.Command, bucketName string, objects []*storage.BucketObject) {
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

func makeBucketListCmd() *cobra.Command {
	bucketListCmd := &cobra.Command{
		Use:     "list BUCKET_NAME",
		Short:   "List the contents of a bucket",
		Long:    "List the objects stored in an OSCAR bucket using the cluster storage API.",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"ls"},
		RunE:    bucketListFunc,
	}

	bucketListCmd.Flags().StringP("cluster", "c", "", "set the cluster")
	bucketListCmd.Flags().StringP("output", "o", "table", "output format (table or json)")
	bucketListCmd.Flags().String("prefix", "", "filter objects by key prefix")

	return bucketListCmd
}
