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

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/volume"
	"github.com/spf13/cobra"
)

func volumeCreateFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	size, _ := cmd.Flags().GetString("size")

	vol, err := volume.CreateVolume(conf.Oscar[clusterName], args[0], size)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Volume %q created (size: %s)\n", vol.Name, vol.Size)

	return nil
}

func makeVolumeCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create VOLUME_NAME",
		Short: "Create a managed volume",
		Args:  cobra.ExactArgs(1),
		RunE:  volumeCreateFunc,
	}

	cmd.Flags().String("size", "", "volume size (e.g. 1Gi, 10Gi)")

	return cmd
}
