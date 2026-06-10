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
	"encoding/csv"
	"encoding/json"
	"fmt"

	"github.com/grycap/oscar-cli/v2/pkg/config"
	"github.com/grycap/oscar-cli/v2/pkg/service"
	"github.com/spf13/cobra"
)

type systemLogEntry struct {
	Timestamp string `json:"timestamp"`
	Status    int    `json:"status"`
	Latency   string `json:"latency"`
	ClientIP  string `json:"client_ip"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	User      string `json:"user"`
	Raw       string `json:"raw"`
}

type systemLogsEnvelope struct {
	Logs []systemLogEntry `json:"logs"`
}

func systemLogsFunc(cmd *cobra.Command, args []string) error {
	conf, err := config.ReadConfig(configPath)
	if err != nil {
		return err
	}

	clusterName, err := getCluster(cmd, conf)
	if err != nil {
		return err
	}

	timestamps, _ := cmd.Flags().GetBool("timestamps")
	previous, _ := cmd.Flags().GetBool("previous")

	logs, err := service.GetSystemLogs(conf.Oscar[clusterName], timestamps, previous)
	if err != nil {
		return err
	}

	output, _ := cmd.Flags().GetString("output")
	switch output {
	case "csv":
		return printSystemLogsCSV(cmd, logs)
	case "json":
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, []byte(logs), "", "  "); err != nil {
			fmt.Print(logs)
			return nil
		}
		fmt.Print(pretty.String())
	default:
		fmt.Print(logs)
	}

	return nil
}

func printSystemLogsCSV(cmd *cobra.Command, logs string) error {
	var envelope systemLogsEnvelope
	if err := json.Unmarshal([]byte(logs), &envelope); err == nil && envelope.Logs != nil {
		w := csv.NewWriter(cmd.OutOrStdout())
		w.Write([]string{"TIMESTAMP", "METHOD", "PATH", "STATUS", "USER", "LATENCY", "CLIENT_IP", "RAW"})
		for _, entry := range envelope.Logs {
			status := fmt.Sprintf("%d", entry.Status)
			w.Write([]string{entry.Timestamp, entry.Method, entry.Path, status, entry.User, entry.Latency, entry.ClientIP, entry.Raw})
		}
		w.Flush()
		return w.Error()
	}

	var entries []systemLogEntry
	if err := json.Unmarshal([]byte(logs), &entries); err != nil {
		return fmt.Errorf("cannot parse system logs as JSON: %w", err)
	}

	w := csv.NewWriter(cmd.OutOrStdout())
	w.Write([]string{"TIMESTAMP", "METHOD", "PATH", "STATUS", "USER", "LATENCY", "CLIENT_IP", "RAW"})
	for _, entry := range entries {
		status := fmt.Sprintf("%d", entry.Status)
		w.Write([]string{entry.Timestamp, entry.Method, entry.Path, status, entry.User, entry.Latency, entry.ClientIP, entry.Raw})
	}
	w.Flush()
	return w.Error()
}

func makeSystemLogsCmd() *cobra.Command {
	systemLogsCmd := &cobra.Command{
		Use:   "system-logs",
		Short: "Get OSCAR manager system logs (Basic Auth only)",
		Args:  cobra.NoArgs,
		RunE:  systemLogsFunc,
	}

	systemLogsCmd.Flags().StringP("cluster", "c", "", "set the cluster")
	systemLogsCmd.Flags().BoolP("timestamps", "t", false, "include timestamps in the logs")
	systemLogsCmd.Flags().BoolP("previous", "p", false, "get logs from the previous terminated container")
	systemLogsCmd.Flags().StringP("output", "o", "text", "output format (text, json, csv)")

	return systemLogsCmd
}
