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

	result, err := storage.ListBuckets(conf.Oscar[clusterName])
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		if err := bucketListPrintJSON(cmd, result); err != nil {
			return err
		}
	case "table":
		bucketListPrintTable(cmd, result)
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}

	return nil
}

func bucketListPrintJSON(cmd *cobra.Command, objects []*storage.BucketInfo) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(objects)
}

func bucketListPrintTable(cmd *cobra.Command, objects []*storage.BucketInfo) {
	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 0, 8, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVISIBILITY\tALLOWED USERS\tOWNER")
	for _, obj := range objects {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", obj.Name, obj.Visibility, obj.AllowedUsers, obj.Owner)
	}
	w.Flush()

	if len(objects) == 0 {
		fmt.Fprintf(out, "There is no Bucket.\n")
	}
	w.Flush()

	if len(objects) == 0 {
		fmt.Fprintf(out, "There is no Bucket.\n")
	}
}

func makeBucketListCmd() *cobra.Command {
	bucketListCmd := &cobra.Command{
		Use:     "list",
		Short:   "List the buckets of a cluster",
		Long:    "List the buckets of a OSCAR cluster using the cluster storage API.",
		Args:    cobra.ExactArgs(0),
		Aliases: []string{"ls"},
		RunE:    bucketListFunc,
	}

	bucketListCmd.Flags().StringP("cluster", "c", "", "set the cluster")
	bucketListCmd.Flags().StringP("output", "o", "table", "output format (table or json)")

	return bucketListCmd
}
