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

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/spf13/cobra"
)

func serviceDeploymentStatusFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	summary, err := service.GetDeploymentStatus(conf.Oscar[clusterName], args[0])
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	case "table":
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "STATE\tREASON\tACTIVE\tAFFECTED\tKIND\tLAST TRANSITION")
		transition := "-"
		if summary.LastTransitionTime != nil {
			transition = summary.LastTransitionTime.Time.Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t%s\n",
			summary.State, summary.Reason, summary.ActiveInstances,
			summary.AffectedInstances, summary.ResourceKind, transition)
		w.Flush()
	default:
		out, err := yaml.Marshal(summary)
		if err != nil {
			return fmt.Errorf("failed to serialize deployment status: %w", err)
		}
		fmt.Print(string(out))
	}

	return nil
}

func makeServiceDeploymentStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status SERVICE_NAME",
		Short:   "Get the deployment status of a service",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"st"},
		RunE:    serviceDeploymentStatusFunc,
	}

	cmd.Flags().StringP("output", "o", "yaml", "output format (yaml, json, table)")

	return cmd
}
