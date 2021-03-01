package syncbox

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// constants for aws services
var (
	AWSDefaultRegion = os.Getenv("AWS_DEFAULT_REGION")
	S3BucketPrefix   = os.Getenv("SB_STORAGE_BUCHET")
)

// Storage structure that use AWS S3
type Storage struct {
	*Logger
	Session *session.Session
	Svc     *s3.S3
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
		Logger:  NewDefaultLogger(),
	}
}

// CreateBucket creates new bucket in S3
func (storage *Storage) CreateBucket(bucketName string) error {
	bucketName = S3BucketPrefix + bucketName
	result, err := storage.Svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: &bucketName,
	})
	if err != nil {
		storage.LogDebug("Failed to create bucket", err)
		return err
	}
	storage.LogDebug("create bucket result :%v\n", result)
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
		storage.LogDebug("Failed to upload data to %s/%s, %s\n", bucketName, objName, err)
		return err
	}
	storage.LogDebug("create object result: %v\n", result)
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
		storage.LogDebug("error on delete object: %v\n", err)
		return err
	}
	storage.LogDebug("delete object result: %v\n", result)
	return nil
}

// GetObject gets object from S3
func (storage *Storage) GetObject(bucketName string, objName string) ([]byte, error) {
	bucketName = S3BucketPrefix + bucketName
	params := &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objName),
	}

	resp, err := storage.Svc.GetObject(params)
	if err != nil {
		return nil, err
	}

	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return respBytes, nil
}

// Download pull the object from S3
func (storage *Storage) Download(path string, bucketName string, objName string) error {
	bucketName = S3BucketPrefix + bucketName
	file, err := os.Create("path")
	defer file.Close()
	if err != nil {
		storage.LogDebug("Failed to create file: %v\n", err)
		return err
	}

	downloader := s3manager.NewDownloader(storage.Session)
	numBytes, err := downloader.Download(file,
		&s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objName),
		})
	if err != nil {
		storage.LogDebug("Failed to download file: %v\n", err)
		return err
	}

	storage.LogDebug("Downloaded file %v: %v bytes\n", file.Name(), numBytes)
	return nil
}

func readInt64(data []byte) (ret int64) {
	buf := bytes.NewBuffer(data)
	binary.Read(buf, binary.LittleEndian, &ret)
	return
}

// ChecksumToNumString converts Checksum to string representation of int64
func ChecksumToNumString(checksum Checksum) string {
	return strconv.FormatInt(readInt64(checksum[:]), 10)
}
