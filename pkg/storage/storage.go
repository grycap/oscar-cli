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

package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v3/pkg/types"
)

var DefaultStorageProvider = []string{"minio.default", "minio"}

// BucketInfo describes a storage bucket accessible from a cluster.
type BucketInfo struct {
	Name         string
	Provider     string
	Visibility   string
	AllowedUsers []string
	Owner        string
	CreationDate time.Time
}

// BucketObject describes an object stored inside a bucket.
type BucketObject struct {
	Name         string
	Size         int64
	LastModified time.Time
	Owner        string
}

// ListBuckets returns the buckets available through the cluster MinIO provider.
func ListBuckets(c *cluster.Cluster) ([]*BucketInfo, error) {
	return ListBucketsWithContext(context.Background(), c)
}

// ListBucketsWithContext returns the buckets available through the cluster MinIO provider using the given context.
func ListBucketsWithContext(ctx context.Context, c *cluster.Cluster) ([]*BucketInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, "system", "buckets")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
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

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	type bucketPayload struct {
		Name         string   `json:"bucket_name"`
		Visibility   string   `json:"visibility"`
		AllowedUsers []string `json:"allowed_users"`
		Owner        string   `json:"owner"`
		Provider     string   `json:"provider"`
		CreationDate string   `json:"creation_date"`
	}

	var (
		raw      []bucketPayload
		envelope struct {
			Buckets []bucketPayload `json:"buckets"`
		}
	)

	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Buckets != nil {
		raw = envelope.Buckets
	} else {
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, err
		}
	}

	buckets := make([]*BucketInfo, 0, len(raw))
	for _, item := range raw {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		info := &BucketInfo{
			Name:         name,
			Provider:     defaultProviderLabel(item.Provider),
			Visibility:   strings.TrimSpace(item.Visibility),
			AllowedUsers: append([]string(nil), item.AllowedUsers...),
			Owner:        strings.TrimSpace(item.Owner),
		}
		if ts := strings.TrimSpace(item.CreationDate); ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				info.CreationDate = t
			} else if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				info.CreationDate = t
			}
		}
		buckets = append(buckets, info)
	}

	sort.Slice(buckets, func(i, j int) bool {
		return strings.ToLower(buckets[i].Name) < strings.ToLower(buckets[j].Name)
	})

	return buckets, nil
}

// ListBucketObjects returns the objects stored inside a bucket using the default context.
func ListBucketObjects(c *cluster.Cluster, bucketName string) ([]*BucketObject, error) {
	return ListBucketObjectsWithContext(context.Background(), c, bucketName)
}

// ListBucketObjectsWithContext returns the objects stored inside a bucket using the provided context.
func ListBucketObjectsWithContext(ctx context.Context, c *cluster.Cluster, bucketName string) ([]*BucketObject, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return nil, errors.New("cluster configuration not provided")
	}
	trimmedBucket := strings.TrimSpace(bucketName)
	if trimmedBucket == "" {
		return nil, errors.New("bucket name is required")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, cluster.ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, "system", "buckets", trimmedBucket)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
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

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	rawObjects, err := decodeBucketObjectsPayload(body)
	if err != nil {
		return nil, err
	}

	objects := make([]*BucketObject, 0, len(rawObjects))
	for _, item := range rawObjects {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.Key)
		}
		if name == "" {
			name = strings.TrimSpace(item.ObjectName)
		}
		if name == "" {
			continue
		}
		size := item.Size
		if size == 0 {
			size = item.SizeBytes
		}
		object := &BucketObject{
			Name:  name,
			Size:  size,
			Owner: strings.TrimSpace(item.Owner),
		}
		if ts := strings.TrimSpace(item.LastModified); ts != "" {
			if t, ok := parseBucketTimestamp(ts); ok {
				object.LastModified = t
			}
		}
		objects = append(objects, object)
	}

	sort.Slice(objects, func(i, j int) bool {
		return strings.ToLower(objects[i].Name) < strings.ToLower(objects[j].Name)
	})

	return objects, nil
}

type bucketObjectPayload struct {
	Name         string `json:"name"`
	Key          string `json:"key"`
	ObjectName   string `json:"object_name"`
	Size         int64  `json:"size"`
	SizeBytes    int64  `json:"size_bytes"`
	LastModified string `json:"last_modified"`
	Owner        string `json:"owner"`
}

func decodeBucketObjectsPayload(body []byte) ([]bucketObjectPayload, error) {
	var direct []bucketObjectPayload
	if err := json.Unmarshal(body, &direct); err == nil {
		if direct == nil {
			direct = []bucketObjectPayload{}
		}
		return direct, nil
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}

	if list, ok := bucketObjectListFromMap(envelope); ok {
		return list, nil
	}

	if bucketNode, ok := envelope["bucket"]; ok {
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(bucketNode, &nested); err == nil {
			if list, ok := bucketObjectListFromMap(nested); ok {
				return list, nil
			}
		}
	}

	return []bucketObjectPayload{}, nil
}

func bucketObjectListFromMap(m map[string]json.RawMessage) ([]bucketObjectPayload, bool) {
	candidates := []string{"objects", "files", "contents", "data"}
	for _, key := range candidates {
		raw, ok := m[key]
		if !ok {
			continue
		}
		var list []bucketObjectPayload
		if err := json.Unmarshal(raw, &list); err != nil {
			continue
		}
		if list == nil {
			list = []bucketObjectPayload{}
		}
		return list, true
	}
	return nil, false
}

var bucketTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05 -0700 MST",
}

func parseBucketTimestamp(ts string) (time.Time, bool) {
	for _, layout := range bucketTimeLayouts {
		if t, err := time.Parse(layout, ts); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// DeleteBucket removes a bucket from the specified cluster.
func DeleteBucket(c *cluster.Cluster, name string) error {
	if c == nil {
		return errors.New("cluster configuration not provided")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("bucket name is required")
	}

	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return cluster.ErrParsingEndpoint
	}
	endpoint.Path = path.Join(endpoint.Path, "system", "buckets", trimmed)

	req, err := http.NewRequest(http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return cluster.ErrMakingRequest
	}

	client, err := c.GetClientSafe()
	if err != nil {
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		return cluster.ErrSendingRequest
	}
	defer res.Body.Close()

	if err := cluster.CheckStatusCode(res); err != nil {
		return err
	}

	return nil
}

func defaultProviderLabel(provider string) string {
	trimmed := strings.TrimSpace(provider)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}

func getProvider(c *cluster.Cluster, providerString string, providers *types.StorageProviders) (interface{}, error) {
	if providerString == "minio" || providerString == "minio.default" {
		config, err := config.GetUserConfig(c)
		if err != nil {
			return nil, cluster.ErrMakingRequest
		}
		minio_value := config.(map[string]interface{})["minio_provider"].(map[string]interface{})
		minio_response := types.MinIOProvider{
			AccessKey: minio_value["access_key"].(string),
			SecretKey: minio_value["secret_key"].(string),
			Region:    minio_value["region"].(string),
			Endpoint:  minio_value["endpoint"].(string),
			Verify:    minio_value["verify"].(bool),
		}
		return &minio_response, nil
	}

	// Check the format of STORAGE_PROVIDER
	provSlice := strings.SplitN(strings.TrimSpace(providerString), types.ProviderSeparator, 2)
	if len(provSlice) != 2 {
		return nil, fmt.Errorf("the STORAGE_PROVIDER \"%s\" is not valid. It must have the form <PROVIDER_NAME>.<PROVIDER_IDENTIFIER>\nExample: minio.my_minio", providerString)
	}

	// Check if the Provider is defined and return it
	var ok = false
	var prov interface{}
	switch provSlice[0] {
	case types.MinIOName:
		prov, ok = providers.MinIO[provSlice[1]]
	case types.S3Name:
		prov, ok = providers.S3[provSlice[1]]
	case types.OnedataName:
		prov, ok = providers.Onedata[provSlice[1]]
	}

	if !ok {
		return prov, fmt.Errorf("the STORAGE_PROVIDER \"%s\" is not defined in the service", providerString)
	}

	return prov, nil
}

// DefaultRemotePath builds the remote path for an upload when only the provider's configured path is available.
func DefaultRemotePath(svc *types.Service, provider, localPath string) (string, error) {
	if svc == nil {
		return "", errors.New("service definition not provided")
	}

	providerPath := ""
	for _, input := range svc.Input {
		if slices.Contains(DefaultStorageProvider, input.Provider) {
			providerPath = input.Path
			break
		}
	}

	if strings.TrimSpace(providerPath) == "" {
		return "", fmt.Errorf("service \"%s\" does not define an input path for storage provider \"%s\"", svc.Name, provider)
	}

	cleaned := strings.Trim(providerPath, " /")
	filename := filepath.Base(localPath)
	if filename == "." || filename == "/" {
		return "", fmt.Errorf("cannot determine file name for \"%s\"", localPath)
	}

	if cleaned == "" {
		return filename, nil
	}

	return path.Join(cleaned, filename), nil
}

// GetFile downloads a file from a storage provider
func GetFile(c *cluster.Cluster, svcName, providerString, remotePath, localPath string, opt *TransferOption) error {
	// Get the service definition
	svc, err := service.GetService(c, svcName)
	if err != nil {
		return err
	}

	return GetFileWithService(c, svc, providerString, remotePath, localPath, opt)
}

// GetFileWithService downloads a file using a pre-fetched service definition.
func GetFileWithService(c *cluster.Cluster, svc *types.Service, providerString, remotePath, localPath string, opt *TransferOption) error {
	if svc == nil {
		return errors.New("service definition not provided")
	}

	// Get the provider (as an interface)
	prov, err := getProvider(c, providerString, svc.StorageProviders)
	if err != nil {
		return err
	}

	// Create the file
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("unable to create the file \"%s\"", localPath)
	}
	defer file.Close()

	remotePath = strings.Trim(remotePath, " /")
	// Split buckets and folders from remotePath
	splitPath := strings.SplitN(remotePath, "/", 2)
	if len(splitPath) == 1 {
		splitPath = append(splitPath, "")
	}

	showProgress := resolveShowProgress(opt)

	switch v := prov.(type) {
	case types.S3Provider:
		var total int64
		if showProgress {
			head, err := v.GetS3Client().HeadObject(&s3.HeadObjectInput{
				Bucket: aws.String(splitPath[0]),
				Key:    aws.String(splitPath[1]),
			})
			if err == nil && head.ContentLength != nil {
				total = *head.ContentLength
			}
		}

		progressOptions := newTransferOptions(downloadDescription(remotePath), total, showProgress)
		bar := buildProgressBar(progressOptions)
		defer finishProgressBar(bar)

		writer := io.WriterAt(file)
		if bar != nil {
			writer = newProgressWriterAt(file, bar)
		}

		downloader := s3manager.NewDownloaderWithClient(v.GetS3Client())
		_, err := downloader.Download(writer, &s3.GetObjectInput{
			Bucket: aws.String(splitPath[0]),
			Key:    aws.String(splitPath[1]),
		})
		if err != nil {
			return err
		}
	case *types.MinIOProvider:
		var total int64
		if showProgress {
			head, err := v.GetS3Client().HeadObject(&s3.HeadObjectInput{
				Bucket: aws.String(splitPath[0]),
				Key:    aws.String(splitPath[1]),
			})
			if err == nil && head.ContentLength != nil {
				total = *head.ContentLength
			}
		}

		progressOptions := newTransferOptions(downloadDescription(remotePath), total, showProgress)
		bar := buildProgressBar(progressOptions)
		defer finishProgressBar(bar)

		writer := io.WriterAt(file)
		if bar != nil {
			writer = newProgressWriterAt(file, bar)
		}

		// Repeat s3 code for correct type assertion
		downloader := s3manager.NewDownloaderWithClient(v.GetS3Client())
		_, err := downloader.Download(writer, &s3.GetObjectInput{
			Bucket: aws.String(splitPath[0]),
			Key:    aws.String(splitPath[1]),
		})
		if err != nil {
			return err
		}
	case *types.OnedataProvider:
		remotePath = path.Join(v.Space, remotePath)
		content, err := v.GetCDMIClient().GetObject(remotePath)
		if err != nil {
			return err
		}
		writer := io.Writer(file)
		if _, err := io.Copy(writer, content); err != nil {
			return err
		}
	default:
		return errors.New("invalid provider")
	}

	return nil
}

// DefaultOutputProvider returns the first output storage provider defined in the service.
func DefaultOutputProvider(svc *types.Service) (string, error) {
	if svc == nil {
		return "", errors.New("service definition not provided")
	}

	for _, output := range svc.Output {
		provider := strings.TrimSpace(output.Provider)
		if provider != "" {
			return provider, nil
		}
	}

	return "", fmt.Errorf("service \"%s\" does not define any output storage providers", svc.Name)
}

// DefaultOutputPath returns the configured output path for the provided storage provider.
func DefaultOutputPath(svc *types.Service, provider string) (string, error) {
	if svc == nil {
		return "", errors.New("service definition not provided")
	}

	provider = strings.TrimSpace(provider)
	var firstPath string

	for _, output := range svc.Output {
		outputProvider := strings.TrimSpace(output.Provider)
		outputPath := strings.TrimSpace(output.Path)

		if outputPath == "" {
			continue
		}

		if firstPath == "" {
			firstPath = outputPath
		}

		if provider == "" || outputProvider == provider {
			return outputPath, nil
		}
	}

	if provider == "" && firstPath != "" {
		return firstPath, nil
	}

	if provider != "" {
		return "", fmt.Errorf("service \"%s\" does not define an output path for storage provider \"%s\"", svc.Name, provider)
	}

	return "", fmt.Errorf("service \"%s\" does not define any output paths", svc.Name)
}

// ResolveLatestRemotePath returns the path to the most recently modified file under the provided remote path.
func ResolveLatestRemotePath(c *cluster.Cluster, svc *types.Service, providerString, basePath string) (string, error) {
	if svc == nil {
		return "", errors.New("service definition not provided")
	}

	basePath = strings.Trim(basePath, " /")
	if basePath == "" {
		return "", errors.New("remote path cannot be empty")
	}

	prov, err := getProvider(c, providerString, svc.StorageProviders)
	if err != nil {
		return "", err
	}

	splitPath := strings.SplitN(basePath, "/", 2)
	if len(splitPath) == 1 {
		splitPath = append(splitPath, "")
	}

	bucket := strings.TrimSpace(splitPath[0])
	if bucket == "" {
		return "", errors.New("remote path must include the bucket name")
	}
	prefix := strings.TrimLeft(splitPath[1], "/")

	var s3Client *s3.S3
	switch v := prov.(type) {
	case types.S3Provider:
		s3Client = v.GetS3Client()
	case *types.MinIOProvider:
		s3Client = v.GetS3Client()
	default:
		return "", errors.New("--download-latest-into is only supported for S3 or MinIO providers")
	}

	input := &s3.ListObjectsInput{
		Bucket: aws.String(bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
	}

	var latest *s3.Object
	err = s3Client.ListObjectsPages(input, func(page *s3.ListObjectsOutput, last bool) bool {
		for _, obj := range page.Contents {
			if obj == nil || obj.Key == nil || obj.LastModified == nil {
				continue
			}
			if obj.Size != nil && *obj.Size == 0 && strings.HasSuffix(*obj.Key, "/") {
				continue
			}
			if latest == nil || obj.LastModified.After(*latest.LastModified) {
				latest = obj
			}
		}
		return true
	})
	if err != nil {
		return "", err
	}

	if latest == nil || latest.Key == nil {
		return "", fmt.Errorf("no files found under \"%s\"", basePath)
	}

	key := strings.TrimLeft(*latest.Key, "/")
	return path.Join(bucket, key), nil
}

// PutFile uploads a file to a storage provider
func PutFile(c *cluster.Cluster, svcName, providerString, localPath, remotePath string, opt *TransferOption) error {
	svc, err := service.GetService(c, svcName)
	if err != nil {
		return err
	}
	return putFile(c, svc, providerString, localPath, remotePath, opt)
}

// PutFileWithService uploads a file using a pre-fetched service definition.
func PutFileWithService(c *cluster.Cluster, svc *types.Service, providerString, localPath, remotePath string, opt *TransferOption) error {
	return putFile(c, svc, providerString, localPath, remotePath, opt)
}

func putFile(c *cluster.Cluster, svc *types.Service, providerString, localPath, remotePath string, opt *TransferOption) error {
	if svc == nil {
		return errors.New("service definition not provided")
	}

	prov, err := getProvider(c, providerString, svc.StorageProviders)
	if err != nil {
		return err
	}

	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("unable to read the file \"%s\"", localPath)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	fileSize := int64(0)
	if err == nil {
		fileSize = fileInfo.Size()
	}

	remotePath = strings.Trim(remotePath, " /")
	// Split buckets and folders from remotePath
	splitPath := strings.SplitN(remotePath, "/", 2)
	if len(splitPath) == 1 {
		splitPath = append(splitPath, "")
	}

	showProgress := resolveShowProgress(opt)
	progressOptions := newTransferOptions(uploadDescription(localPath), fileSize, showProgress)
	bar := buildProgressBar(progressOptions)
	defer finishProgressBar(bar)

	reader := io.ReadSeeker(file)
	if bar != nil {
		reader = newProgressReadSeeker(file, bar)
	}

	switch v := prov.(type) {
	case types.S3Provider:
		uploader := s3manager.NewUploaderWithClient(v.GetS3Client())
		_, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(splitPath[0]),
			Key:    aws.String(splitPath[1]),
			Body:   reader,
		})
		if err != nil {
			return err
		}
	case *types.MinIOProvider:
		uploader := s3manager.NewUploaderWithClient(v.GetS3Client())
		_, err := uploader.Upload(&s3manager.UploadInput{
			Bucket: aws.String(splitPath[0]),
			Key:    aws.String(splitPath[1]),
			Body:   reader,
		})
		if err != nil {
			return err
		}
	case *types.OnedataProvider:
		remotePath = path.Join(v.Space, remotePath)
		err := v.GetCDMIClient().CreateObject(remotePath, reader, true)
		if err != nil {
			return err
		}
	default:
		return errors.New("invalid provider")
	}

	return nil
}

// DeleteFile uploads a file to a storage provider
func DeleteFile(c *cluster.Cluster, svcName, providerString, remotePath string) error {
	// Get the service definition
	svc, err := service.GetService(c, svcName)
	if err != nil {
		return err
	}

	// Get the provider (as an interface)
	prov, err := getProvider(c, providerString, svc.StorageProviders)
	if err != nil {
		return err
	}

	remotePath = strings.Trim(remotePath, " /")
	// Split buckets and folders from remotePath
	splitPath := strings.SplitN(remotePath, "/", 2)
	if len(splitPath) == 1 {
		splitPath = append(splitPath, "")
	}

	switch v := prov.(type) {
	case *types.MinIOProvider:
		// Repeat s3 code for correct type assertion
		v.GetS3Client().DeleteObject(
			&s3.DeleteObjectInput{
				Bucket: aws.String(splitPath[0]),
				Key:    aws.String(splitPath[1]),
			},
		)
	default:
		return errors.New("invalid provider")
	}

	return nil
}

// ListFiles list files from a storage provider
func ListFiles(c *cluster.Cluster, svcName, providerString, remotePath string) (list []string, err error) {
	// Get the service definition
	svc, err := service.GetService(c, svcName)
	if err != nil {
		return list, err
	}

	// Get the provider (as an interface)
	prov, err := getProvider(c, providerString, svc.StorageProviders)
	if err != nil {
		return list, err
	}

	remotePath = strings.Trim(remotePath, " /")
	// Split buckets and folders from remotePath
	splitPath := strings.SplitN(remotePath, "/", 2)
	if len(splitPath) == 1 {
		splitPath = append(splitPath, "")
	}

	switch v := prov.(type) {
	case *types.S3Provider:
		res, err := v.GetS3Client().ListObjects(&s3.ListObjectsInput{
			Bucket: aws.String(splitPath[0]),
			Prefix: aws.String(splitPath[1]),
		})
		if err != nil {
			return list, err
		}
		for _, obj := range res.Contents {
			nameFile := strings.TrimPrefix(*obj.Key, fmt.Sprintf("%s/", splitPath[1]))
			dateFile := *obj.LastModified
			if *obj.Size == 0 {
				list = append(list, nameFile)
			} else {
				list = append(list, nameFile+" \t"+dateFile.String())
			}
		}
	case *types.MinIOProvider:
		// Repeat s3 code for correct type assertion
		res, err := v.GetS3Client().ListObjects(&s3.ListObjectsInput{
			Bucket: aws.String(splitPath[0]),
			Prefix: aws.String(splitPath[1]),
		})
		if err != nil {
			return list, err
		}
		for _, obj := range res.Contents {
			nameFile := strings.TrimPrefix(*obj.Key, fmt.Sprintf("%s/", splitPath[1]))
			dateFile := *obj.LastModified
			if *obj.Size == 0 {
				list = append(list, nameFile)
			} else {
				list = append(list, nameFile+" \t"+dateFile.String())
			}

		}
	case *types.OnedataProvider:
		remotePath = path.Join(v.Space, remotePath)
		list, err = v.GetCDMIClient().ReadContainer(remotePath)
		if err != nil {
			return list, err
		}
	default:
		return list, errors.New("invalid provider")
	}

	return list, nil
}
