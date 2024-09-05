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
	"io/ioutil"
	"os"

	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/spf13/cobra"
)

func serviceRunFunc(cmd *cobra.Command, args []string) error {
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
	}
	if token != "" && endpoint == "" {
		// Error missing endpoint
	}
	// Parse input (only --input or --text-input are allowed) (AND one of them is required)
	inputFile, _ := cmd.Flags().GetString("input")
	textInput, _ := cmd.Flags().GetString("text-input")
	outputFile, _ := cmd.Flags().GetString("output")
	if inputFile == "" && textInput == "" {
		return errors.New("you must specify \"--input\" or \"--text-input\" flag")
	}
	if inputFile != "" && textInput != "" {
		return errors.New("you only can specify one of \"--input\" or \"--text-input\" flags")
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
	resBody, err := service.RunService(conf.Oscar[cluster], args[0], token, endpoint, reader)
	if err != nil {
		return err
	}
	defer resBody.Close()

	// Create temp file to store the result body
	tmpfile, err := ioutil.TempFile("", "")
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()
	if err != nil {
		return errors.New("unable to create a temporary file to store the result")
	}
	_, err = io.Copy(tmpfile, resBody)
	if err != nil {
		return errors.New("unable to copy the response")
	}

	// Decode the result body
	tmpfile.Seek(0, 0)
	decoder := base64.NewDecoder(base64.StdEncoding, tmpfile)

	// Parse output (store file if --output is set)
	var out *os.File

	if outputFile != "" {
		// Create the file if --output is set
		out, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("unable to create the file \"%s\"", outputFile)
		}
	} else {
		// Create a temporary file
		out, err = ioutil.TempFile("", "")
		if err != nil {
			return errors.New("unable to create a temporary file to decode the result")
		}
		defer os.Remove(out.Name())
	}
	defer out.Close()

	// Copy the decoder stream into out
	_, err = io.Copy(out, decoder)
	if err != nil {
		// If resBody can't be decoded copy it directly
		// Seek tmpfile and out to start from the beginning
		tmpfile.Seek(0, 0)
		out.Seek(0, 0)
		_, err = io.Copy(out, tmpfile)
		if err != nil {
			return errors.New("unable to copy the response")
		}
	}

	if outputFile == "" {
		// Copy out to stdout
		out.Seek(0, 0)
		_, err = io.Copy(os.Stdout, out)
		if err != nil {
			return errors.New("unable to print the result")
		}
	}

	return nil
}

func makeServiceRunCmd() *cobra.Command {
	serviceRunCmd := &cobra.Command{
		Use:     "run SERVICE_NAME {--input | --text-input}",
		Short:   "Invoke a service synchronously (a Serverless backend in the cluster is required)",
		Args:    cobra.ExactArgs(1),
		Aliases: []string{"invoke", "r"},
		RunE:    serviceRunFunc,
	}

	serviceRunCmd.Flags().StringP("cluster", "c", "", "set the cluster")
	serviceRunCmd.Flags().StringP("endpoint", "e", "", "endpoint of a non registered cluster")
	serviceRunCmd.Flags().StringP("token", "t", "", "token of the service")
	serviceRunCmd.Flags().StringP("file-input", "f", "", "input file for the request")
	serviceRunCmd.Flags().StringP("text-input", "i", "", "text input string for the request")
	serviceRunCmd.Flags().StringP("output", "o", "", "file path to store the output")

	return serviceRunCmd
}
