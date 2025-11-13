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
	"os"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/spf13/cobra"
)

var (
	configPath        string
	defaultConfigPath string
	rootCmd           *cobra.Command
)

func newRootCommand() *cobra.Command {
	resetPersistentState()

	cmd := &cobra.Command{
		Use:     "oscar-cli",
		Short:   "A CLI tool to interact with OSCAR clusters",
		Args:    cobra.NoArgs,
		Aliases: []string{"oscar", "ocli"},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Only display usage with args related errors
			cmd.SilenceUsage = true
		},
		Run: runFunc,
	}

	cmd.AddCommand(makeVersionCmd())
	cmd.AddCommand(makeClusterCmd())
	cmd.AddCommand(makeServiceCmd())
	cmd.AddCommand(makeHubCmd())
	cmd.AddCommand(makeApplyCmd())
	cmd.AddCommand(makeInteractiveCmd())
	cmd.AddCommand(makeDeleteCmd())

	return cmd
}

func runFunc(cmd *cobra.Command, args []string) {
	cmd.Help()
}

// Execute function to launch the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Set default config path
	var err error
	defaultConfigPath, err = config.GetDefaultConfigPath()
	if err != nil {
		os.Exit(1)
	}

	rootCmd = newRootCommand()
}

// NewRootCommand construct a fresh root command instance.
func NewRootCommand() *cobra.Command {
	return newRootCommand()
}

func resetPersistentState() {
	configPath = defaultConfigPath
	destinationClusterID = ""
}
