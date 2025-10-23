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
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/spf13/cobra"
)

var DEFAULT_PROVIDER = "minio.default"

func serviceJobFunc(cmd *cobra.Command, args []string) error {
	// Read the config file
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	cluster, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}
	// Check if the cluster has auth
	endpoint, _ := cmd.Flags().GetString("endpoint")
	token, _ := cmd.Flags().GetString("token")

	if endpoint != "" && token == "" {
		// Error missing token
		return errors.New("you must specify a service token with the flag \"--token\"")
	}
	if token != "" && endpoint == "" {
		// Error missing endpoint
		return errors.New("you must specify a the cluster endpoint with the flag \"--endpoint\"")
	}
	// Parse input (only --input or --text-input are allowed) (AND one of them is required)
	inputFile, _ := cmd.Flags().GetString("file-input")
	textInput, _ := cmd.Flags().GetString("text-input")

	if inputFile == "" && textInput == "" {
		return errors.New("you must specify \"--file-input\" or \"--text-input\" flag")
	}
	if inputFile != "" && textInput != "" {
		return errors.New("you only can specify one of \"--file-input\" or \"--text-input\" flags")
	}

	var inputReader io.Reader = bytes.NewBufferString(textInput)

	if inputFile != "" {
		// Open the file
		file, err := os.Open(inputFile)
		defer file.Close()
		if err != nil {
			return fmt.Errorf("unable to read the file \"%s\"", inputFile)
		}
		// Set the file as the inputReader
		inputReader = file
	}

	// Make pipe to encode file stream
	reader, writer := io.Pipe()
	encoder := base64.NewEncoder(base64.StdEncoding, writer)

	// Copy the file to the encoder in a goroutine to avoid blocking the execution
	go func() {
		_, err := io.Copy(encoder, inputReader)
		encoder.Close()
		if err != nil {
			writer.CloseWithError(err)
		}
		writer.Close()
	}()
	// Make the request
	resBody, err := service.JobService(conf.Oscar[cluster], args[0], token, endpoint, reader)
	if err != nil {
		return err
	}
	defer resBody.Close()

	return nil
}

func makeServiceJobCmd() *cobra.Command {
	serviceRunCmd := &cobra.Command{
		Use:     "job SERVICE_NAME {--file-input | --text-input}",
		Short:   "Invoke a service asynchronously (only compatible with MinIO providers)",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"job", "j"},
		RunE:    serviceJobFunc,
	}

	serviceRunCmd.Flags().StringP("cluster", "c", "", "set the cluster")
	serviceRunCmd.Flags().StringP("endpoint", "e", "", "endpoint of a non registered cluster")
	serviceRunCmd.Flags().StringP("token", "t", "", "token of the service")
	serviceRunCmd.Flags().StringP("file-input", "f", "", "input file for the request")
	serviceRunCmd.Flags().StringP("text-input", "i", "", "text input string for the request")

	return serviceRunCmd
}
