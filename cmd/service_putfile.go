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
	"strings"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar-cli/pkg/storage"
	"github.com/grycap/oscar/v3/pkg/types"
	"github.com/spf13/cobra"
)

const defaultStorageProvider = "minio.default"

func servicePutFileFunc(cmd *cobra.Command, args []string) error {
	serviceName := args[0]

	provider, localFile, remoteFile, remoteProvided, err := parsePutFileArgs(args[1:])
	if err != nil {
		return err
	}

	if err := validateLocalFile(localFile); err != nil {
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

	svc, err := service.GetService(conf.Oscar[cluster], serviceName)
	if err != nil {
		return err
	}

	if !remoteProvided {
		remoteFile, err = storage.DefaultRemotePath(svc, provider, localFile)
		if err != nil {
			return err
		}
	}

	err = storage.PutFileWithService(conf.Oscar[cluster], svc, provider, localFile, remoteFile)
	if err != nil {
		return err
	}

	return nil
}

func makeServicePutFileCmd() *cobra.Command {
	servicePutFileCmd := &cobra.Command{
		Use:   "put-file SERVICE_NAME [STORAGE_PROVIDER] LOCAL_FILE [REMOTE_FILE]",
		Short: "Put a file in a service's storage provider",
		Long: `Put a file in a service's storage provider.
		
The STORAGE_PROVIDER argument follows the format STORAGE_PROVIDER_TYPE.STORAGE_PROVIDER_NAME,
being the STORAGE_PROVIDER_TYPE one of the three supported storage providers (MinIO, S3 or Onedata)
and the STORAGE_PROVIDER_NAME is the identifier for the provider set in the service's definition.
If STORAGE_PROVIDER is omitted the default value "minio.default" is used.
If REMOTE_FILE is omitted the command uploads the file to the configured input path of that provider using the local file name.`,
		Args:    cobra.RangeArgs(2, 4),
		Aliases: []string{"pf"},
		RunE:    servicePutFileFunc,
	}

	servicePutFileCmd.Flags().StringP("cluster", "c", "", "set the cluster")

	return servicePutFileCmd
}

func parsePutFileArgs(args []string) (provider, localFile, remoteFile string, remoteProvided bool, err error) {
	switch len(args) {
	case 1:
		return defaultStorageProvider, args[0], "", false, nil
	case 2:
		if looksLikeStorageProvider(args[0]) {
			return args[0], args[1], "", false, nil
		}
		return defaultStorageProvider, args[0], args[1], true, nil
	case 3:
		if !looksLikeStorageProvider(args[0]) {
			return "", "", "", false, fmt.Errorf("invalid storage provider \"%s\"", args[0])
		}
		return args[0], args[1], args[2], true, nil
	default:
		return "", "", "", false, fmt.Errorf("invalid number of arguments")
	}
}

func looksLikeStorageProvider(value string) bool {
	parts := strings.SplitN(value, types.ProviderSeparator, 2)
	if len(parts) != 2 {
		return false
	}
	switch parts[0] {
	case types.MinIOName, types.S3Name, types.OnedataName, types.WebDavName:
		return true
	default:
		return false
	}
}

func validateLocalFile(localPath string) error {
	if !fileExists(localPath) {
		return fmt.Errorf("local file \"%s\" does not exist or is not accessible", localPath)
	}
	return nil
}

func fileExists(target string) bool {
	info, err := os.Stat(target)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
