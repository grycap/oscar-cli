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

	"github.com/spf13/cobra"
)

var (
	// Version string variable to store the version of oscar-cli
	Version string
	// GitCommit string variable to store the git commit of the current oscar-cli build
	GitCommit string
)

func versionFunc(cmd *cobra.Command, args []string) {
	if Version != "" {
		fmt.Println("version:", Version)
	} else {
		fmt.Println("version: devel")
	}
	fmt.Println("git commit:", GitCommit)
}

func makeVersionCmd() *cobra.Command {
	versionCmd := &cobra.Command{
		Use:     "version",
		Short:   "Print the version",
		Args:    cobra.NoArgs,
		Aliases: []string{"v"},
		Run:     versionFunc,
	}

	return versionCmd
}
