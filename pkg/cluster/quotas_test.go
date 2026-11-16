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

package cluster

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

func quotaTestResponse() types.QuotaResponse {
	return types.QuotaResponse{
		UserID: "alice",
		Resources: map[string]types.QuotaValues{
			"cpu": {Max: 2, Used: 1},
		},
		MinIO: &types.MinIOQuotaResponse{
			Buckets:          types.MinIOBucketCountQuota{Max: 5, Used: 2},
			StoragePerBucket: types.MinIOStoragePerBucketQuota{Max: "1Gi"},
			StorageTotal:     types.MinIOStorageTotalUsage{Used: "3Gi"},
		},
	}
}

func TestGetQuota(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system/quotas/user/alice" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(quotaTestResponse()); err != nil {
			t.Fatalf("encoding quota: %v", err)
		}
	}))
	defer server.Close()

	c := &Cluster{
		Endpoint:     server.URL,
		AuthUser:     "user",
		AuthPassword: "pass",
		SSLVerify:    true,
	}

	quota, err := GetQuota(c, "alice")
	if err != nil {
		t.Fatalf("GetQuota returned error: %v", err)
	}
	if quota.UserID != "alice" {
		t.Fatalf("expected user ID alice, got %s", quota.UserID)
	}
	if quota.MinIO == nil {
		t.Fatal("expected MinIO section in quota response, got nil")
	}
	if quota.MinIO.Buckets.Max != 5 || quota.MinIO.Buckets.Used != 2 {
		t.Fatalf("unexpected buckets quota: %+v", quota.MinIO.Buckets)
	}
	if quota.MinIO.StoragePerBucket.Max != "1Gi" {
		t.Fatalf("expected storage per bucket 1Gi, got %s", quota.MinIO.StoragePerBucket.Max)
	}
	if quota.MinIO.StorageTotal.Used != "3Gi" {
		t.Fatalf("expected total storage used 3Gi, got %s", quota.MinIO.StorageTotal.Used)
	}
}

func TestUpdateQuota(t *testing.T) {
	const userID = "alice"

	var gotPath, gotMethod, gotContentType string
	var gotBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		if err := json.NewEncoder(w).Encode(quotaTestResponse()); err != nil {
			t.Fatalf("encoding quota: %v", err)
		}
	}))
	defer server.Close()

	c := &Cluster{
		Endpoint:     server.URL,
		AuthUser:     "user",
		AuthPassword: "pass",
		SSLVerify:    true,
	}

	cases := []struct {
		name     string
		req      *types.QuotaUpdateRequest
		wantBody map[string]any
	}{
		{
			name: "cpu only",
			req:  &types.QuotaUpdateRequest{CPU: "2"},
			wantBody: map[string]any{
				"cpu": "2", "memory": "", "ephemeral_storage": "",
			},
		},
		{
			name: "memory only",
			req:  &types.QuotaUpdateRequest{Memory: "1Gi"},
			wantBody: map[string]any{
				"cpu": "", "memory": "1Gi", "ephemeral_storage": "",
			},
		},
		{
			name: "ephemeral storage only",
			req:  &types.QuotaUpdateRequest{EphemeralStorage: "10Gi"},
			wantBody: map[string]any{
				"cpu": "", "memory": "", "ephemeral_storage": "10Gi",
			},
		},
		{
			name: "gpu only",
			req:  &types.QuotaUpdateRequest{GPU: "2"},
			wantBody: map[string]any{
				"cpu": "", "memory": "", "ephemeral_storage": "", "gpu": "2",
			},
		},
		{
			name: "minio buckets only",
			req:  &types.QuotaUpdateRequest{MinIO: &types.MinIOQuotaUpdate{Buckets: "5"}},
			wantBody: map[string]any{
				"cpu": "", "memory": "", "ephemeral_storage": "",
				"minio": map[string]any{"buckets": "5"},
			},
		},
		{
			name: "minio storage per bucket only",
			req:  &types.QuotaUpdateRequest{MinIO: &types.MinIOQuotaUpdate{StoragePerBucket: "1Gi"}},
			wantBody: map[string]any{
				"cpu": "", "memory": "", "ephemeral_storage": "",
				"minio": map[string]any{"storage_per_bucket": "1Gi"},
			},
		},
		{
			name: "full update",
			req: &types.QuotaUpdateRequest{
				CPU:              "2",
				Memory:           "1Gi",
				EphemeralStorage: "10Gi",
				GPU:              "1",
				Volumes: &types.VolumeQuotaUpdate{
					Disk:             "5Gi",
					Volumes:          "3",
					MaxDiskperVolume: "2Gi",
					MinDiskperVolume: "1Gi",
				},
				MinIO: &types.MinIOQuotaUpdate{Buckets: "5", StoragePerBucket: "1Gi"},
			},
			wantBody: map[string]any{
				"cpu": "2", "memory": "1Gi", "ephemeral_storage": "10Gi", "gpu": "1",
				"volumes": map[string]any{"disk": "5Gi", "volumes": "3", "max_disk_per_volume": "2Gi", "min_disk_per_volume": "1Gi"},
				"minio":   map[string]any{"buckets": "5", "storage_per_bucket": "1Gi"},
			},
		},
		{
			name: "nil request uses empty body",
			req:  nil,
			wantBody: map[string]any{
				"cpu": "", "memory": "", "ephemeral_storage": "",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotPath, gotMethod, gotContentType, gotBody = "", "", "", nil

			var gotBodyMap map[string]any
			quota, err := UpdateQuota(c, userID, tc.req)
			if err != nil {
				t.Fatalf("UpdateQuota returned error: %v", err)
			}
			if err := json.Unmarshal(gotBody, &gotBodyMap); err != nil {
				t.Fatalf("decoding request body %q: %v", gotBody, err)
			}
			if !reflect.DeepEqual(gotBodyMap, tc.wantBody) {
				t.Fatalf("request body mismatch:\n got: %v\nwant: %v", gotBody, tc.wantBody)
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

			// The response is decoded with the full quota payload, including MinIO.
			if quota.UserID != userID {
				t.Fatalf("expected user ID %s, got %s", userID, quota.UserID)
			}
			if quota.MinIO == nil {
				t.Fatal("expected MinIO section in quota response, got nil")
			}
			if quota.MinIO.Buckets.Max != 5 || quota.MinIO.Buckets.Used != 2 {
				t.Fatalf("unexpected buckets quota: %+v", quota.MinIO.Buckets)
			}
			if quota.MinIO.StoragePerBucket.Max != "1Gi" {
				t.Fatalf("expected storage per bucket 1Gi, got %s", quota.MinIO.StoragePerBucket.Max)
			}
			if quota.MinIO.StorageTotal.Used != "3Gi" {
				t.Fatalf("expected total storage used 3Gi, got %s", quota.MinIO.StorageTotal.Used)
			}
		})
	}
}

func TestUpdateQuotaValidation(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	c := &Cluster{
		Endpoint:     server.URL,
		AuthUser:     "user",
		AuthPassword: "pass",
		SSLVerify:    true,
	}

	if _, err := UpdateQuota(nil, "alice", &types.QuotaUpdateRequest{CPU: "2"}); err == nil ||
		err.Error() != "cluster configuration not provided" {
		t.Fatalf("expected missing cluster error, got %v", err)
	}

	cases := []struct {
		name string
		uid  string
	}{
		{"empty user ID", ""},
		{"whitespace user ID", "   "},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := UpdateQuota(c, tc.uid, &types.QuotaUpdateRequest{CPU: "2"})
			if err == nil || err.Error() != "user ID is required" {
				t.Fatalf("expected user ID required error, got %v", err)
			}
		})
	}

	if requests != 0 {
		t.Fatalf("expected no request to be sent, got %d", requests)
	}
}

func TestUpdateQuotaError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &Cluster{
		Endpoint:     server.URL,
		AuthUser:     "user",
		AuthPassword: "pass",
		SSLVerify:    true,
	}

	_, err := UpdateQuota(c, "alice", &types.QuotaUpdateRequest{CPU: "2"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected boom error, got %v", err)
	}
}
