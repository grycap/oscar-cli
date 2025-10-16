package cmd

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/hub"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v3/pkg/types"
	"github.com/spf13/cobra"
)

type hubDeployOptions struct {
	owner    string
	repo     string
	rootPath string
	ref      string
	apiBase  string
	name     string
}

func (o *hubDeployOptions) applyToClient() []hub.Option {
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

func hubDeployFunc(cmd *cobra.Command, args []string, opts *hubDeployOptions) error {
	slug := args[0]

	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	clusterCfg := conf.Oscar[clusterName]

	client := hub.NewClient(opts.applyToClient()...)
	fdl, err := client.FetchFDL(cmd.Context(), slug)
	if err != nil {
		return err
	}

	clusterConfig, err := clusterCfg.GetClusterConfig()
	if err != nil {
		return err
	}

	serviceDef, err := buildServiceFromFDL(fdl, clusterName, clusterCfg, clusterConfig.MinIOProvider)
	if err != nil {
		return err
	}

	if opts.name != "" {
		serviceDef.Name = opts.name
	}

	action := "Creating"
	method := http.MethodPost
	if serviceExists(serviceDef, clusterCfg) {
		action = "Updating"
		method = http.MethodPut
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s service \"%s\" in cluster \"%s\"...\n", action, serviceDef.Name, clusterName)

	if err := service.ApplyService(serviceDef, clusterCfg, method); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Service \"%s\" deployed successfully.\n", serviceDef.Name)

	return nil
}

func makeHubDeployCmd() *cobra.Command {
	opts := &hubDeployOptions{
		owner:    "grycap",
		repo:     "oscar-hub",
		rootPath: "",
		ref:      "main",
	}

	defaultSource := fmt.Sprintf("Default curated source: https://github.com/%s/%s/tree/%s", opts.owner, opts.repo, opts.ref)

	cmd := &cobra.Command{
		Use:   "deploy SERVICE-SLUG",
		Short: "Deploy a curated OSCAR service into a cluster",
		Long:  "Deploy a curated OSCAR service into a cluster.\n" + defaultSource,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return hubDeployFunc(cmd, args, opts)
		},
	}

	cmd.Flags().StringVar(&opts.owner, "owner", opts.owner, "GitHub owner that hosts the curated services")
	cmd.Flags().StringVar(&opts.repo, "repo", opts.repo, "GitHub repository that hosts the curated services")
	cmd.Flags().StringVar(&opts.rootPath, "path", opts.rootPath, "subdirectory inside the repository that contains the services")
	cmd.Flags().StringVar(&opts.ref, "ref", opts.ref, "Git reference (branch, tag, or commit) to query")
	cmd.Flags().StringVar(&opts.apiBase, "api-base", "", "override the GitHub API base URL")
	cmd.Flags().StringVarP(&opts.name, "name", "n", "", "override the OSCAR service name during deployment")
	cmd.Flags().StringP("cluster", "c", "", "set the cluster")

	if flag := cmd.Flags().Lookup("api-base"); flag != nil {
		flag.Hidden = true
	}

	return cmd
}

func buildServiceFromFDL(fdl *service.FDL, clusterName string, clusterCfg *cluster.Cluster, minioProvider *types.MinIOProvider) (*types.Service, error) {
	for _, element := range fdl.Functions.Oscar {
		for _, svc := range element {
			if svc == nil {
				continue
			}

			svcCopy := *svc
			svcCopy.ClusterID = clusterName

			if svcCopy.Clusters == nil {
				svcCopy.Clusters = map[string]types.Cluster{}
			}
			svcCopy.Clusters[clusterName] = types.Cluster{
				Endpoint:     clusterCfg.Endpoint,
				AuthUser:     clusterCfg.AuthUser,
				AuthPassword: clusterCfg.AuthPassword,
				SSLVerify:    clusterCfg.SSLVerify,
			}

			if minioProvider != nil {
				if svcCopy.StorageProviders == nil {
					svcCopy.StorageProviders = &types.StorageProviders{}
				}
				if svcCopy.StorageProviders.MinIO == nil {
					svcCopy.StorageProviders.MinIO = map[string]*types.MinIOProvider{}
				}
				svcCopy.StorageProviders.MinIO[clusterName] = minioProvider
			}

			return &svcCopy, nil
		}
	}

	return nil, errors.New("the FDL does not contain an OSCAR service definition")
}
