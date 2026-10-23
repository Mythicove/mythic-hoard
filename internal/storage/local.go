// Package storage 提供对象存储实现（本地磁盘 / S3 兼容），实现 shared/storage.Storage。
package storage

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/Mythicove/shared/storage"
)

// Local 是本地磁盘存储。
type Local struct {
	dir     string
	baseURL string
}

var _ storage.Storage = (*Local)(nil)

// NewLocal 构造本地存储；baseURL 用于 PresignGet 生成直出 URL。
func NewLocal(dir, baseURL string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Local{dir: dir, baseURL: baseURL}, nil
}

func (s *Local) Put(_ context.Context, o storage.Object) (string, error) {
	path := filepath.Join(s.dir, o.Key)
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, o.Body); err != nil {
		return "", err
	}
	return o.Key, nil
}

func (s *Local) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.dir, key))
}

func (s *Local) Delete(_ context.Context, key string) error {
	return os.Remove(filepath.Join(s.dir, key))
}

func (s *Local) PresignGet(_ context.Context, key string) (*url.URL, error) {
	return url.Parse(s.baseURL + "/cdn/" + key)
}
