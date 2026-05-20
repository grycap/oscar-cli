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

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/volume"
	"github.com/spf13/cobra"
)

func volumeGetFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	vol, err := volume.GetVolume(conf.Oscar[clusterName], args[0])
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(vol)
	default:
		out, err := yaml.Marshal(vol)
		if err != nil {
			return fmt.Errorf("failed to serialize volume: %w", err)
		}
		fmt.Print(string(out))
	}

	return nil
}

func makeVolumeGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get VOLUME_NAME",
		Short:   "Get details of a managed volume",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"g"},
		RunE:    volumeGetFunc,
	}

	cmd.Flags().StringP("output", "o", "yaml", "output format (yaml, json)")

	return cmd
}
