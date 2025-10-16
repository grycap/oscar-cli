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
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/grycap/oscar-cli/pkg/cluster"
	"github.com/grycap/oscar-cli/pkg/config"
	"github.com/grycap/oscar-cli/pkg/service"
	"github.com/grycap/oscar/v3/pkg/types"
)

var DefaultStorageProvider = []string{"minio.default", "minio"}

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
