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

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path"
	"path/filepath"

	"github.com/goccy/go-yaml"
	"github.com/grycap/oscar-cli/pkg/cluster"
)

const (
	defaultConfig      = ".oscar-cli/config.yaml"
	configPath         = "/system/config"
	defaultMemory      = "256Mi"
	defaultLogLevel    = "INFO"
	defaultClusterName = "default-cluster"
)

var (
	errNoConfigFile       = errors.New("the configuration file doesn't exists. Please provide a valid one or create it with \"oscar-cli cluster add\"")
	errParsingConfigFile  = errors.New("the configuration file provided is not valid. Please provide a valid one or create it with \"oscar-cli cluster add\"")
	errCreatingConfigFile = errors.New("error creating the config file. Please check the path is correct and you have the appropriate permissions")
	clusterNotDefinedMsg  = "the cluster \"%s\" doesn't exist"
)

// Config stores the configuration of oscar-cli
type Config struct {
	Oscar   map[string]*cluster.Cluster `json:"oscar" binding:"required"`
	Default string                      `json:"default,omitempty"`
}

// GetDefaultConfigPath returns the default configuration file path
func GetDefaultConfigPath() (defaultConfigPath string, err error) {
	// Get the current user
	user, err := user.Current()
	if err != nil {
		return "", err
	}

	// Get the home dir of the user
	homeDir := user.HomeDir
	// Join the home dir with the default config path
	defaultConfigPath = path.Join(homeDir, defaultConfig)

	return defaultConfigPath, nil
}

// ReadConfig reads the configuration file
func ReadConfig(configPath string) (config *Config, err error) {
	// Read the config file
	content, err := os.ReadFile(configPath)
	if err != nil {
		// Return errNoConfigFile if the file doesn't exists
		return nil, errNoConfigFile
	}

	config = &Config{}
	// Unmarshal the config file (YAML or JSON)
	configExtension := filepath.Ext(configPath)
	if configExtension == ".yaml" || configExtension == ".yml" {
		if err = yaml.Unmarshal(content, config); err != nil {
			return nil, errParsingConfigFile
		}
	} else {
		// Default JSON
		if err = json.Unmarshal(content, config); err != nil {
			return nil, errParsingConfigFile
		}
	}

	return config, nil
}

func (config *Config) writeConfig(configPath string) (err error) {
	// Marshal the config content (YAML or JSON)
	configExtension := filepath.Ext(configPath)
	var configContent []byte
	if configExtension == ".yaml" || configExtension == ".yml" {
		configContent, err = yaml.Marshal(config)
		if err != nil {
			return err
		}
	} else {
		// Default JSON
		configContent, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return err
		}
	}

	// Create the config path (if required) and write the config file
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return errCreatingConfigFile
	}
	if err := os.WriteFile(configPath, configContent, 0600); err != nil {
		return errCreatingConfigFile
	}

	return nil
}

// AddCluster adds a new cluster to the config
func (config *Config) AddCluster(configPath string, id string, endpoint string, authUser string, authPassword string, oidcAccountName string, oidcRefreshToken string, sslVerify bool) error {
	// Add (or overwrite) the new cluster
	config.Oscar[id] = &cluster.Cluster{
		Endpoint:         endpoint,
		AuthUser:         authUser,
		AuthPassword:     authPassword,
		OIDCAccountName:  oidcAccountName,
		OIDCRefreshToken: oidcRefreshToken,
		SSLVerify:        sslVerify,
		Memory:           defaultMemory,
		LogLevel:         defaultLogLevel,
	}

	// If there is only one cluster set as default
	if len(config.Oscar) == 1 {
		config.Default = id
	}

	// Marshal and write the config file
	if err := config.writeConfig(configPath); err != nil {
		return err
	}

	return nil
}

// RemoveCluster removes a cluster from the config
func (config *Config) RemoveCluster(configPath, id string) error {
	// Check if the cluster id exists
	if err := config.CheckCluster(id); err != nil {
		return err
	}

	// Delete the cluster from config
	delete(config.Oscar, id)

	// Delete the identifier from default if set
	if config.Default == id {
		config.Default = ""
	}

	// Save the config
	if err := config.writeConfig(configPath); err != nil {
		return err
	}

	return nil
}

// CheckCluster checks if a cluster exists and return error if not
func (config *Config) CheckCluster(id string) error {
	if _, exists := config.Oscar[id]; !exists {
		return fmt.Errorf(clusterNotDefinedMsg, id)
	}
	return nil
}

func (config *Config) GetCluster(default_cluster bool, destinationClusterID string, clusterName string) (string, error) {
	if default_cluster {
		err := config.CheckCluster(config.Default)
		if err != nil {
			return "", err
		}
		return config.Default, nil
	} else if destinationClusterID != "" {
		err := config.CheckCluster(destinationClusterID)
		if err != nil {
			return "", err
		}
		return destinationClusterID, nil
	} else if clusterName == defaultClusterName {
		err := config.CheckCluster(config.Default)
		if err != nil {
			return "", err
		}
		return config.Default, nil
	}
	err := config.CheckCluster(clusterName)
	if err != nil {
		return "", err
	}
	return clusterName, nil

}

// SetDefault set a default cluster in the config file
func (config *Config) SetDefault(configPath, id string) error {
	// Check if the cluster id exists
	if err := config.CheckCluster(id); err != nil {
		return err
	}

	// Set as default
	config.Default = id

	// Save the config
	if err := config.writeConfig(configPath); err != nil {
		return err
	}

	return nil
}

func GetUserConfig(c *cluster.Cluster) (interface{}, error) {
	getServiceURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrMakingRequest
	}
	getServiceURL.Path = path.Join(getServiceURL.Path, configPath)
	req, err := http.NewRequest(http.MethodGet, getServiceURL.String(), nil)
	if err != nil {
		return nil, cluster.ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return nil, err
	}
	var response interface{}
	// Decode the response body into the info struct
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return nil, err
	}
	return response, nil
}
