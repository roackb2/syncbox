package syncbox

import (
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// constants for aws services
const (
	AWSDefaultRegion = "us-east-1"
	S3BucketPrefix   = "syncbox-user-bucket-"
)

// Storage structure that use AWS S3
type Storage struct {
	Session *session.Session
	Svc     *s3.S3
	Logger  *Logger
}

// NewStorage instantiate Storage
func NewStorage() *Storage {
	sess := session.New(
		&aws.Config{
			Region: aws.String(AWSDefaultRegion),
		},
	)

	return &Storage{
		Session: sess,
		Svc:     s3.New(sess),
		Logger:  NewLogger(DefaultAppPrefix, GlobalLogInfo, GlobalLogError, GlobalLogDebug),
	}
}

// CreateBucket creates new bucket in S3
func (storage *Storage) CreateBucket(bucketName string) error {
	bucketName = S3BucketPrefix + bucketName
	result, err := storage.Svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: &bucketName,
	})
	if err != nil {
		storage.Logger.LogDebug("Failed to create bucket", err)
		return err
	}
	storage.Logger.LogDebug("create bucket result :%v\n", result)
	return nil
}

// CreateObject creates a object and put content in it
func (storage *Storage) CreateObject(bucketName string, objName string, content string) error {
	bucketName = S3BucketPrefix + bucketName
	result, err := storage.Svc.PutObject(&s3.PutObjectInput{
		Body:   strings.NewReader(content),
		Bucket: &bucketName,
		Key:    &objName,
	})
	if err != nil {
		storage.Logger.LogDebug("Failed to upload data to %s/%s, %s\n", bucketName, objName, err)
		return err
	}
	storage.Logger.LogDebug("create object result: %v\n", result)
	return nil
}

// DeleteObject deletes object in bucketName
func (storage *Storage) DeleteObject(bucketName string, objName string) error {
	bucketName = S3BucketPrefix + bucketName
	result, err := storage.Svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objName),
	})
	if err != nil {
		storage.Logger.LogDebug("error on delete object: %v\n", err)
		return err
	}
	storage.Logger.LogDebug("delete object result: %v\n", result)
	return nil
}

// Download pull the object from S3
func (storage *Storage) Download(path string, bucketName string, objName string) error {
	bucketName = S3BucketPrefix + bucketName
	file, err := os.Create("path")
	defer file.Close()
	if err != nil {
		storage.Logger.LogDebug("Failed to create file: %v\n", err)
		return err
	}

	downloader := s3manager.NewDownloader(storage.Session)
	numBytes, err := downloader.Download(file,
		&s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objName),
		})
	if err != nil {
		storage.Logger.LogDebug("Failed to download file: %v\n", err)
		return err
	}

	storage.Logger.LogDebug("Downloaded file %v: %v bytes\n", file.Name(), numBytes)
	return nil
}
