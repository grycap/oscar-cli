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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/grycap/oscar/v4/pkg/types"
)

func TestQuotaUpdateHelpListsFlags(t *testing.T) {
	stdout, stderr, err := runCommand(t, "quota", "update", "--help")
	if err != nil {
		t.Fatalf("quota update --help returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	flags := []string{
		"--cpu",
		"--memory",
		"--ephemeral-storage",
		"--gpu",
		"--volume-disk",
		"--volume-count",
		"--minio-buckets",
		"--minio-storage-per-bucket",
	}
	for _, flag := range flags {
		if !strings.Contains(stdout, flag) {
			t.Fatalf("expected %q in help output, got %q", flag, stdout)
		}
	}
}

func TestQuotaUpdateCommandSendsTypedRequest(t *testing.T) {
	const (
		username = "user"
		password = "pass"
		userID   = "alice"
	)

	var gotMethod, gotPath, gotContentType string
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading request body: %v", err)
		}
		if r.URL.Path != "/system/quotas/user/"+userID {
			http.NotFound(w, r)
			return
		}
		gotUser, gotPass, ok := r.BasicAuth()
		if !ok || gotUser != username || gotPass != password {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		quota := types.QuotaResponse{
			UserID: userID,
			MinIO: &types.MinIOQuotaResponse{
				Buckets:          types.MinIOBucketCountQuota{Max: 5, Used: 2},
				StoragePerBucket: types.MinIOStoragePerBucketQuota{Max: "1Gi"},
				StorageTotal:     types.MinIOStorageTotalUsage{Used: "3Gi"},
			},
		}
		if err := json.NewEncoder(w).Encode(quota); err != nil {
			t.Fatalf("encoding quota: %v", err)
		}
	}))
	defer server.Close()

	configFile := writeConfigFile(t, "alpha", server.URL)

	stdout, stderr, err := runCommand(t,
		"quota", "--config", configFile,
		"update", "alice",
		"--cluster", "alpha",
		"--cpu", "2",
		"--memory", "1Gi",
		"--ephemeral-storage", "10Gi",
		"--gpu", "1",
		"--minio-buckets", "5",
		"--minio-storage-per-bucket", "1Gi",
	)
	if err != nil {
		t.Fatalf("quota update command returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}

	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT request, got %s", gotMethod)
	}
	if gotPath != "/system/quotas/user/"+userID {
		t.Fatalf("expected path /system/quotas/user/%s, got %s", userID, gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected content type application/json, got %q", gotContentType)
	}

	var gotBodyMap map[string]any
	if err := json.Unmarshal(gotBody, &gotBodyMap); err != nil {
		t.Fatalf("decoding request body %q: %v", gotBody, err)
	}
	wantBody := map[string]any{
		"cpu":               "2",
		"memory":            "1Gi",
		"ephemeral_storage": "10Gi",
		"gpu":               "1",
		"minio": map[string]any{
			"buckets":            "5",
			"storage_per_bucket": "1Gi",
		},
	}
	if !reflect.DeepEqual(gotBodyMap, wantBody) {
		t.Fatalf("request body mismatch:\n got: %v\nwant: %v", gotBodyMap, wantBody)
	}

	if !strings.Contains(stdout, "user_id: alice") {
		t.Fatalf("expected user_id in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "minio:") {
		t.Fatalf("expected minio section in output, got %q", stdout)
	}
}
