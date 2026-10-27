package storage

import (
	"context"
	"testing"
)

// 直出 URL 契约：构造函数收到的 baseURL 是完整公开前缀（含 /cdn 路径段），
// 实现只拼接 object key。回归保护：曾出现 Local 再追加一次 /cdn 导致
// {base}/cdn/cdn/{key} 的双前缀 URL。
func TestLocalPresignGetUsesBaseURLAsFullPublicPrefix(t *testing.T) {
	s, err := NewLocal(t.TempDir(), "http://localhost:8080/cdn")
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	u, err := s.PresignGet(context.Background(), "ab/cd.bin")
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	want := "http://localhost:8080/cdn/ab/cd.bin"
	if u.String() != want {
		t.Fatalf("PresignGet = %q, want %q", u, want)
	}
}

func TestLocalPresignGetTrimsTrailingSlash(t *testing.T) {
	s, err := NewLocal(t.TempDir(), "http://localhost:8080/cdn/")
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	u, err := s.PresignGet(context.Background(), "ab/cd.bin")
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	want := "http://localhost:8080/cdn/ab/cd.bin"
	if u.String() != want {
		t.Fatalf("PresignGet = %q, want %q", u, want)
	}
}

// S3 在 baseURL 非空时不走 presigner，可在无网络环境锁定同一契约。
func TestS3PresignGetUsesBaseURLAsFullPublicPrefix(t *testing.T) {
	s, err := NewS3(context.Background(), Options{
		Endpoint:  "http://127.0.0.1:9000",
		Region:    "us-east-1",
		Bucket:    "hoard",
		AccessKey: "test",
		SecretKey: "test",
	}, "http://localhost:8080/cdn")
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}
	u, err := s.PresignGet(context.Background(), "ab/cd.bin")
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	want := "http://localhost:8080/cdn/ab/cd.bin"
	if u.String() != want {
		t.Fatalf("PresignGet = %q, want %q", u, want)
	}
}
