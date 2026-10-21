package mwanachamagit

import "encoding/base64"

type Option func(*gitManager)

func WithMaxFileSize(bytes int64) Option {
	return func(m *gitManager) { m.maxFile = bytes }
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
