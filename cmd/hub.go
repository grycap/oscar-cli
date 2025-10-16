package cmd

import "github.com/spf13/cobra"

func hubFunc(cmd *cobra.Command, args []string) {
	cmd.Help()
}

func makeHubCmd() *cobra.Command {
	hubCmd := &cobra.Command{
		Use:     "hub",
		Short:   "Interact with curated services from OSCAR Hub",
		Args:    cobra.NoArgs,
		Aliases: []string{"h"},
		Run:     hubFunc,
	}

	hubCmd.AddCommand(makeHubListCmd())
	hubCmd.AddCommand(makeHubDeployCmd())

	return hubCmd
}
