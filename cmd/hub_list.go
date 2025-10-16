package cmd

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/grycap/oscar-cli/pkg/hub"
	"github.com/spf13/cobra"
)

type hubListOptions struct {
	owner      string
	repo       string
	rootPath   string
	ref        string
	outputJSON bool
	apiBase    string
}

func (o *hubListOptions) applyToClient() []hub.Option {
	options := []hub.Option{
		hub.WithOwner(o.owner),
		hub.WithRepo(o.repo),
		hub.WithRootPath(o.rootPath),
		hub.WithRef(o.ref),
	}
	if o.apiBase != "" {
		options = append(options, hub.WithBaseAPI(o.apiBase))
	}
	return options
}

func hubListFunc(cmd *cobra.Command, _ []string, opts *hubListOptions) error {
	client := hub.NewClient(opts.applyToClient()...)

	result, err := client.ListServices(cmd.Context())
	if err != nil {
		return err
	}

	if opts.outputJSON {
		payload := struct {
			Services []hub.Service `json:"services"`
			Warnings []hub.Warning `json:"warnings,omitempty"`
		}{
			Services: result.Services,
			Warnings: result.Warnings,
		}

		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(payload)
	}

	if len(result.Services) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No curated services found")
	} else {
		out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
		fmt.Fprintln(out, "SLUG\tNAME\tCREATOR")
		for _, svc := range result.Services {
			fmt.Fprintf(out, "%s\t%s\t%s\n", svc.Slug, svc.Name, svc.Creator)
		}
		out.Flush()
	}

	for _, warning := range result.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %v\n", warning.Path, warning.Err)
	}

	return nil
}

func makeHubListCmd() *cobra.Command {
	opts := &hubListOptions{
		owner:    "grycap",
		repo:     "oscar-hub",
		rootPath: "",
		ref:      "main",
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List curated OSCAR services available in OSCAR Hub",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return hubListFunc(cmd, args, opts)
		},
	}

	cmd.Flags().StringVar(&opts.owner, "owner", opts.owner, "GitHub owner that hosts the curated services")
	cmd.Flags().StringVar(&opts.repo, "repo", opts.repo, "GitHub repository that hosts the curated services")
	cmd.Flags().StringVar(&opts.rootPath, "path", opts.rootPath, "subdirectory inside the repository that contains the services")
	cmd.Flags().StringVar(&opts.ref, "ref", opts.ref, "Git reference (branch, tag, or commit) to query")
	cmd.Flags().BoolVar(&opts.outputJSON, "json", false, "print the list in JSON format")
	cmd.Flags().StringVar(&opts.apiBase, "api-base", "", "override the GitHub API base URL")
	if flag := cmd.Flags().Lookup("api-base"); flag != nil {
		flag.Hidden = true
	}

	return cmd
}
