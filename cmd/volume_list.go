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
	"github.com/grycap/oscar-cli/v2/pkg/volume"
	"github.com/spf13/cobra"
)

func volumeListFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	vols, err := volume.ListVolumes(conf.Oscar[clusterName])
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(vols)
	default:
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSIZE\tSTATUS\tOWNER")
		for _, v := range vols {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", v.Name, v.Size, v.Status.Phase, v.OwnerUser)
		}
		w.Flush()
		if len(vols) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No volumes found")
		}
	}

	return nil
}

func makeVolumeListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List managed volumes",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE:    volumeListFunc,
	}

	cmd.Flags().StringP("output", "o", "table", "output format (table, json)")

	return cmd
}
