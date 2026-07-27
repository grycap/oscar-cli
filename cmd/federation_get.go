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

	"github.com/grycap/oscar-cli/v2/pkg/config"
	"github.com/grycap/oscar-cli/v2/pkg/service"
	"github.com/spf13/cobra"
)

func federationGetFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	fed, err := service.GetFederation(conf.Oscar[clusterName], args[0])
	if err != nil {
		return err
	}
	replicas := fed.Members

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(fed)
	case "table":
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "Topology: ", fed.Topology)
		fmt.Fprintln(w, "Members: ", fed.Members.Len())
		fmt.Fprintln(w, "TYPE\tCLUSTER ID\tSERVICE NAME\tURL\tPRIORITY")
		for _, r := range replicas {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", r.Type, r.ClusterID, r.ServiceName, r.URL, r.Priority)
		}
		w.Flush()
		if len(replicas) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No federation members found")
		}
	default:
		fmt.Fprintln(cmd.OutOrStdout(), "Topology: ", fed.Topology)
		fmt.Fprintln(cmd.OutOrStdout(), "Members: ", fed.Members.Len())
		for _, r := range replicas {
			fmt.Fprintf(cmd.OutOrStdout(), "Type: %s\n", r.Type)
			fmt.Fprintf(cmd.OutOrStdout(), "Cluster ID: %s\n", r.ClusterID)
			fmt.Fprintf(cmd.OutOrStdout(), "Service Name: %s\n", r.ServiceName)
			fmt.Fprintf(cmd.OutOrStdout(), "URL: %s\n", r.URL)
			fmt.Fprintf(cmd.OutOrStdout(), "Priority: %d\n\n", r.Priority)
		}
		if len(replicas) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No federation members found")
		}
	}

	return nil
}

func makeFederationGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get SERVICE_NAME",
		Short:   "Get federated members of a service",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"get-federation", "g"},
		RunE:    federationGetFunc,
	}

	cmd.Flags().StringP("output", "o", "text", "output format (text, json, table)")

	return cmd
}
