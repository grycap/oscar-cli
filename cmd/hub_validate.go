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
	"strings"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/hub"
	"github.com/spf13/cobra"
)

type hubValidateOptions struct {
	owner    string
	repo     string
	rootPath string
	ref      string
	apiBase  string
	name     string
}

func (o *hubValidateOptions) applyToClient() []hub.Option {
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

func hubValidateFunc(cmd *cobra.Command, args []string, opts *hubValidateOptions) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterID, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	client := hub.NewClient(opts.applyToClient()...)
	results, err := client.ValidateService(cmd.Context(), args[0], conf.Oscar[clusterID], opts.name)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Acceptance tests for %s (%d)\n", args[0], len(results))

	passed := 0
	for _, result := range results {
		name := result.Test.Name
		if strings.TrimSpace(name) == "" {
			name = result.Test.ID
		}

		status := "FAIL"
		if result.Passed {
			status = "PASS"
			passed++
		}
		fmt.Fprintf(out, "- [%s] %s\n", status, name)

		if result.Err != nil {
			fmt.Fprintf(out, "  Error: %v\n", result.Err)
			continue
		}

		if result.Details != "" {
			fmt.Fprintf(out, "  Details: %s\n", result.Details)
		}

		if strings.TrimSpace(result.Test.ExpectedSubstring) != "" {
			fmt.Fprintf(out, "  Expect: %q\n", result.Test.ExpectedSubstring)
		}

		if strings.TrimSpace(result.Output) != "" {
			fmt.Fprintf(out, "  Output preview: %s\n", result.Output)
		}
	}

	if passed != len(results) {
		return fmt.Errorf("%d of %d acceptance tests failed", len(results)-passed, len(results))
	}

	return nil
}

func makeHubValidateCmd() *cobra.Command {
	opts := &hubValidateOptions{
		owner:    "grycap",
		repo:     "oscar-hub",
		rootPath: "crates",
		ref:      "main",
	}

	cmd := &cobra.Command{
		Use:     "validate SERVICE_SLUG",
		Short:   "Run acceptance tests defined in the OSCAR Hub RO-Crate metadata",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"test", "check"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return hubValidateFunc(cmd, args, opts)
		},
	}

	cmd.Flags().StringP("cluster", "c", "", "set the target cluster")
	cmd.Flags().StringVar(&opts.owner, "owner", opts.owner, "GitHub owner that hosts the curated services")
	cmd.Flags().StringVar(&opts.repo, "repo", opts.repo, "GitHub repository that hosts the curated services")
	cmd.Flags().StringVar(&opts.rootPath, "path", opts.rootPath, "subdirectory inside the repository that contains the services")
	cmd.Flags().StringVar(&opts.ref, "ref", opts.ref, "Git reference (branch, tag, or commit) to query")
	cmd.Flags().StringVar(&opts.apiBase, "api-base", "", "override the GitHub API base URL")
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "override the OSCAR service name during validation")
	if flag := cmd.Flags().Lookup("api-base"); flag != nil {
		flag.Hidden = true
	}

	return cmd
}
