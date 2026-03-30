package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var (
	// Version string variable to store the version of oscar-cli
	Version string
	// GitCommit string variable to store the git commit of the current oscar-cli build
	GitCommit string
)

func versionFunc(cmd *cobra.Command, args []string) {
	info, ok := debug.ReadBuildInfo()
	if Version != "" {
		fmt.Println("version:", Version)
	} else if ok && info.Main.Version != "" {
		fmt.Println("version:", info.Main.Version)
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
