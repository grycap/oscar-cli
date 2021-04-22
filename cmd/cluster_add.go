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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/spf13/cobra"
)

func clusterAddFunc(cmd *cobra.Command, args []string) error {
	// Get the arguments
	identifier := args[0]
	endpoint := args[1]
	username := args[2]
	var pass string
	var err error
	passStdin, _ := cmd.Flags().GetBool("password-stdin")
	if passStdin {
		if len(args) != 3 {
			cmd.SilenceUsage = false
			return errors.New("if the \"--password-stdin\" flag is set only 3 arguments are allowed")
		}
		pass, err = readPassStdin()
		if err != nil {
			return err
		}
	} else {
		if len(args) != 4 {
			cmd.SilenceUsage = false
			return errors.New("you must provide the password")
		}
		pass = args[3]
	}

	conf, err := config.ReadConfig(configPath)
	if err != nil {
		conf = &config.Config{
			Oscar: map[string]*cluster.Cluster{},
		}
	}

	disableSSL, _ := cmd.Flags().GetBool("disable-ssl")

	err = conf.AddCluster(configPath, identifier, endpoint, username, pass, !disableSSL)
	if err != nil {
		return err
	}

	fmt.Printf("Cluster \"%s\" successfully stored. To modify the default values, please edit the file %s\n", identifier, configPath)

	return nil
}

func makeClusterAddCmd() *cobra.Command {
	clusterAddCmd := &cobra.Command{
		Use:     "add IDENTIFIER ENDPOINT USERNAME {PASSWORD | --password-stdin}",
		Short:   "Add a new existing cluster to oscar-cli",
		Args:    cobra.RangeArgs(3, 4),
		Aliases: []string{"a"},
		RunE:    clusterAddFunc,
	}

	clusterAddCmd.Flags().Bool("disable-ssl", false, "disable verification of ssl certificates for the added cluster")
	clusterAddCmd.Flags().Bool("password-stdin", false, "take the password from stdin")

	return clusterAddCmd
}

func readPassStdin() (string, error) {
	bytes, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}

	return strings.Trim(string(bytes), "\n"), nil
}
