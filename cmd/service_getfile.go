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
	"os"
	"path/filepath"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/spf13/cobra"
)

func serviceGetFileFunc(cmd *cobra.Command, args []string) error {
	serviceName := args[0]

	latestFlag := cmd.Flags().Lookup("download-latest-into")
	latestValue := ""
	latestRequested := false
	if latestFlag != nil && latestFlag.Changed {
		latestRequested = true
		latestValue = latestFlag.Value.String()
		if latestValue == latestFileNoOptSentinel {
			latestValue = ""
		}
	}

	provider, remotePath, localPath, remoteProvided, localProvided, err := parseGetFileArgs(args[1:], latestRequested)
	if err != nil {
		return err
	}

	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	noProgress, err := cmd.Flags().GetBool("no-progress")
	if err != nil {
		return err
	}

	var transferOpt *storage.TransferOption
	if noProgress {
		transferOpt = &storage.TransferOption{ShowProgress: false}
	}

	svc, err := service.GetService(conf.Oscar[cluster], serviceName)
	if err != nil {
		return err
	}

	if provider == "" {
		provider, err = storage.DefaultOutputProvider(svc)
		if err != nil {
			return err
		}
	}

	scopePath := remotePath
	if !remoteProvided {
		if !latestRequested {
			return fmt.Errorf("REMOTE_PATH argument is required")
		}
		scopePath, err = storage.DefaultOutputPath(svc, provider)
		if err != nil {
			return err
		}
	}

	if latestRequested {
		latestPath := scopePath
		if remoteProvided {
			latestPath = remotePath
		}
		resolved, err := storage.ResolveLatestRemotePath(conf.Oscar[cluster], svc, provider, latestPath)
		if err != nil {
			return err
		}
		remotePath = resolved

		if latestValue != "" && localProvided {
			return fmt.Errorf("--download-latest-into already defines a destination path; do not provide LOCAL_FILE as well")
		}

		if !localProvided {
			baseName := filepath.Base(remotePath)
			if baseName == "" || baseName == "." || baseName == "/" {
				return fmt.Errorf("unable to infer the file name from remote path %q", remotePath)
			}

			localPath = resolveLatestDestination(latestValue, baseName)
			localProvided = true
		}
	}

	if !localProvided {
		return fmt.Errorf("LOCAL_FILE argument is required")
	}

	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return err
	}
	localPath = absPath

	if err := ensureParentDir(localPath); err != nil {
		return err
	}

	err = storage.GetFileWithService(conf.Oscar[cluster], svc, provider, remotePath, localPath, transferOpt)
	if err != nil {
		return err
	}

	fmt.Fprintln(cmd.OutOrStdout(), localPath)

	return nil
}

func makeServiceGetFileCmd() *cobra.Command {
	serviceGetFileCmd := &cobra.Command{
		Use:   "get-file SERVICE_NAME [STORAGE_PROVIDER] [REMOTE_PATH] [LOCAL_FILE]",
		Short: "Get a file from a service's storage provider",
		Long: `Get a file from a service's storage provider.

The STORAGE_PROVIDER argument follows the format STORAGE_PROVIDER_TYPE.STORAGE_PROVIDER_NAME,
being the STORAGE_PROVIDER_TYPE one of the three supported storage providers (MinIO, S3 or Onedata)
and the STORAGE_PROVIDER_NAME is the identifier for the provider set in the service's definition.
If STORAGE_PROVIDER is omitted the first output provider defined in the service will be used.
When used together with --download-latest-into, REMOTE_PATH can be omitted and the default
output path of the selected provider will be employed.`,
		Args:    cobra.MinimumNArgs(1),
		Aliases: []string{"gf"},
		RunE:    serviceGetFileFunc,
	}

	serviceGetFileCmd.Flags().StringP("cluster", "c", "", "set the cluster")
	serviceGetFileCmd.Flags().Bool("no-progress", false, "disable progress bar output")
	serviceGetFileCmd.Flags().String("download-latest-into", "", "download the most recent file found under the remote path; optionally specify a destination directory or exact file path")
	if flag := serviceGetFileCmd.Flags().Lookup("download-latest-into"); flag != nil {
		flag.NoOptDefVal = latestFileNoOptSentinel
	}

	return serviceGetFileCmd
}

const latestFileNoOptSentinel = "__use_positional__"

func parseGetFileArgs(args []string, allowRemoteOmit bool) (provider, remotePath, localPath string, remoteProvided, localProvided bool, err error) {
	switch len(args) {
	case 0:
		if !allowRemoteOmit {
			return "", "", "", false, false, fmt.Errorf("invalid number of arguments")
		}
		return "", "", "", false, false, nil
	case 1:
		if looksLikeStorageProvider(args[0]) {
			if !allowRemoteOmit {
				return "", "", "", false, false, fmt.Errorf("REMOTE_PATH argument is required")
			}
			return args[0], "", "", false, false, nil
		}
		return "", args[0], "", true, false, nil
	case 2:
		if looksLikeStorageProvider(args[0]) {
			return args[0], args[1], "", true, false, nil
		}
		return "", args[0], args[1], true, true, nil
	case 3:
		if !looksLikeStorageProvider(args[0]) {
			return "", "", "", false, false, fmt.Errorf("invalid storage provider \"%s\"", args[0])
		}
		return args[0], args[1], args[2], true, true, nil
	default:
		return "", "", "", false, false, fmt.Errorf("invalid number of arguments")
	}
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func resolveLatestDestination(flagValue, baseName string) string {
	if flagValue == "" {
		return filepath.Join(".", baseName)
	}

	clean := filepath.Clean(flagValue)
	// Treat paths ending with separator (before cleaning) as directories
	if len(flagValue) > 0 && os.IsPathSeparator(flagValue[len(flagValue)-1]) {
		return filepath.Join(clean, baseName)
	}

	if info, err := os.Stat(clean); err == nil && info.IsDir() {
		return filepath.Join(clean, baseName)
	}

	if filepath.Ext(clean) != "" {
		return clean
	}

	return filepath.Join(clean, baseName)
}
