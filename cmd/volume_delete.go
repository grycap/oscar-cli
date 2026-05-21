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

func volumeDeleteFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	if err := volume.DeleteVolume(conf.Oscar[clusterName], args[0]); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Volume %q deleted\n", args[0])

	return nil
}

func makeVolumeDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete VOLUME_NAME",
		Short:   "Delete a managed volume",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"rm"},
		RunE:    volumeDeleteFunc,
	}

	return cmd
}
