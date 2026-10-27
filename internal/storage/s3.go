package storage

import (
	"context"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/Mythicove/shared/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Options 是 S3 存储配置。
type Options struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// S3 是 S3 兼容存储。
type S3 struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
	baseURL   string
}

var _ storage.Storage = (*S3)(nil)

// NewS3 构造 S3 存储。baseURL 是完整公开前缀（含 /cdn 之类路径段，由调用方拼装），
// 非空时 PresignGet 直接返回 baseURL + key 的公开 URL；为空才走 S3 presign。
// 实现只拼接 object key，不得再追加自己的路径段（与 NewLocal 契约一致）。
func NewS3(ctx context.Context, opts Options, baseURL string) (*S3, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(opts.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(opts.AccessKey, opts.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if opts.Endpoint != "" {
			o.BaseEndpoint = aws.String(opts.Endpoint)
			o.UsePathStyle = true
		}
	})
	return &S3{
		client:    client,
		presigner: s3.NewPresignClient(client),
		bucket:    opts.Bucket,
		baseURL:   baseURL,
	}, nil
}

func (s *S3) Put(ctx context.Context, o storage.Object) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &s.bucket,
		Key:         &o.Key,
		Body:        o.Body,
		ContentType: &o.ContentType,
	})
	return o.Key, err
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	return err
}

func (s *S3) PresignGet(ctx context.Context, key string) (*url.URL, error) {
	if s.baseURL != "" {
		return url.Parse(strings.TrimRight(s.baseURL, "/") + "/" + key)
	}
	req, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key},
		s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return nil, err
	}
	return url.Parse(req.URL)
}
