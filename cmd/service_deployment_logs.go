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
	"encoding/csv"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/v2/pkg/config"
	"github.com/grycap/oscar-cli/v2/pkg/service"
	"github.com/spf13/cobra"
)

func serviceDeploymentLogsFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	logStream, err := service.GetDeploymentLogs(conf.Oscar[clusterName], args[0])
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(logStream)
	case "csv":
		w := csv.NewWriter(cmd.OutOrStdout())
		w.Write([]string{"TIMESTAMP", "MESSAGE"})
		for _, entry := range logStream.Entries {
			w.Write([]string{entry.Timestamp, entry.Message})
		}
		w.Flush()
		return w.Error()
	case "table":
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "TIMESTAMP\tMESSAGE")
		for _, entry := range logStream.Entries {
			fmt.Fprintf(w, "%s\t%s\n", entry.Timestamp, entry.Message)
		}
		w.Flush()
	default:
		out, err := yaml.Marshal(logStream)
		if err != nil {
			return fmt.Errorf("failed to serialize deployment logs: %w", err)
		}
		fmt.Print(string(out))
	}

	return nil
}

func makeServiceDeploymentLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs SERVICE_NAME",
		Short:   "Get the deployment logs of a service",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"log", "l"},
		RunE:    serviceDeploymentLogsFunc,
	}

	cmd.Flags().StringP("output", "o", "yaml", "output format (yaml, json, table, csv)")

	return cmd
}
