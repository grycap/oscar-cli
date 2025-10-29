package cmd

import (
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/tui"
	"github.com/spf13/cobra"
)

func makeInteractiveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "interactive",
		Short:   "Launch the interactive terminal UI",
		Aliases: []string{"ui"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			conf, err := config.ReadConfig(configPath)
			if err != nil {
				return err
			}

			return tui.Run(cmd.Context(), conf)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", defaultConfigPath, "set the location of the config file (YAML or JSON)")

	return cmd
}
