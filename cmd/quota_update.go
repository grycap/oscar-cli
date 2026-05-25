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

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar/v4/pkg/types"
	"github.com/spf13/cobra"
)

func quotaUpdateFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	cpu, _ := cmd.Flags().GetString("cpu")
	memory, _ := cmd.Flags().GetString("memory")
	volDisk, _ := cmd.Flags().GetString("volume-disk")
	volCount, _ := cmd.Flags().GetString("volume-count")
	volMaxDisk, _ := cmd.Flags().GetString("volume-max-disk")
	volMinDisk, _ := cmd.Flags().GetString("volume-min-disk")

	var volUpdate *types.VolumeQuotaUpdate
	if volDisk != "" || volCount != "" || volMaxDisk != "" || volMinDisk != "" {
		volUpdate = &types.VolumeQuotaUpdate{
			Disk:             volDisk,
			Volumes:          volCount,
			MaxDiskperVolume: volMaxDisk,
			MinDiskperVolume: volMinDisk,
		}
	}

	quota, err := cluster.UpdateQuota(conf.Oscar[clusterName], args[0], cpu, memory, volUpdate)
	if err != nil {
		return err
	}

	out, err := yaml.Marshal(quota)
	if err != nil {
		return fmt.Errorf("failed to serialize quota: %w", err)
	}
	fmt.Print(string(out))

	return nil
}

func makeQuotaUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update USER_ID",
		Short:   "Update quota for a user",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"u"},
		RunE:    quotaUpdateFunc,
	}

	cmd.Flags().String("cpu", "", "CPU quota (e.g. '2', '500m')")
	cmd.Flags().String("memory", "", "memory quota (e.g. '2Gi', '512Mi')")
	cmd.Flags().String("volume-disk", "", "volume disk quota")
	cmd.Flags().String("volume-count", "", "volume count quota")
	cmd.Flags().String("volume-max-disk", "", "max disk per volume")
	cmd.Flags().String("volume-min-disk", "", "min disk per volume")

	return cmd
}
