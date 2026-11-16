package mwanachamagit

import (
	"encoding/base64"
	"fmt"
)

const (
	defaultMaxPushBytes = 32 << 20
	bytesPerMiB         = 1 << 20
)

type Option func(*gitManager)

func WithMaxFileSize(bytes int64) Option {
	return func(m *gitManager) { m.maxFile = bytes }
}

func WithMaxPushSize(bytes int64) Option {
	return func(m *gitManager) { m.maxPush = bytes }
}

func (m *gitManager) pushLimit() int64 {
	if m.maxPush > 0 {
		return m.maxPush
	}
	return defaultMaxPushBytes
}

func pushTooLargeMessage(limit int64) string {
	size := fmt.Sprintf("%d byte", limit)
	if limit%bytesPerMiB == 0 {
		size = fmt.Sprintf("%d MiB", limit/bytesPerMiB)
	}
	return fmt.Sprintf("this push is over the %s limit; push fewer files at a time", size)
}

func (m *gitManager) checkFileSize(req WriteFileRequest) error {
	if m.maxFile <= 0 {
		return nil
	}
	if contentSize(req) > m.maxFile {
		return ErrFileTooLarge
	}
	return nil
}

func contentSize(req WriteFileRequest) int64 {
	if req.Encoding == "base64" {
		if raw, err := base64.StdEncoding.DecodeString(req.Content); err == nil {
			return int64(len(raw))
		}
	}
	return int64(len(req.Content))
}
