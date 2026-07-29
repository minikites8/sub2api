package repository

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type s3GeneratedMediaStore struct {
	client *s3.Client
	bucket string
}

func NewS3GeneratedMediaStoreFactory() service.GeneratedMediaObjectStoreFactory {
	return func(ctx context.Context, cfg *service.GeneratedMediaStorageConfig) (service.GeneratedMediaObjectStore, error) {
		region := cfg.Region
		if region == "" {
			region = "auto"
		}
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion(region),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
		)
		if err != nil {
			return nil, fmt.Errorf("load object storage config: %w", err)
		}
		client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
			if cfg.Endpoint != "" {
				options.BaseEndpoint = &cfg.Endpoint
			}
			options.UsePathStyle = cfg.ForcePathStyle
			options.APIOptions = append(options.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
			options.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		})
		return &s3GeneratedMediaStore{client: client, bucket: cfg.Bucket}, nil
	}
}

func (s *s3GeneratedMediaStore) Upload(ctx context.Context, key string, body io.Reader, contentType string, contentLength int64) (int64, error) {
	counter := &generatedMediaCountingReader{reader: body}
	input := &s3.PutObjectInput{Bucket: &s.bucket, Key: &key, Body: counter, ContentType: &contentType}
	if contentLength >= 0 {
		input.ContentLength = &contentLength
	}
	finish := servertiming.ObserveDependency(ctx, "object_storage")
	_, err := s.client.PutObject(ctx, input)
	finish()
	if err != nil {
		return counter.read, fmt.Errorf("object storage PutObject: %w", err)
	}
	return counter.read, nil
}

func (s *s3GeneratedMediaStore) HeadBucket(ctx context.Context) error {
	finish := servertiming.ObserveDependency(ctx, "object_storage")
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: &s.bucket})
	finish()
	if err != nil {
		return fmt.Errorf("object storage HeadBucket: %w", err)
	}
	return nil
}

type generatedMediaCountingReader struct {
	reader io.Reader
	read   int64
}

func (r *generatedMediaCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += int64(n)
	return n, err
}
