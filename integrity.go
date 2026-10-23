package mwanachamagit

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/aosanya/mwanachama-backend-git/models"
	"github.com/go-git/go-git/v5/plumbing"
)

var ErrContentTampered = errors.New("stored content does not match its recorded hash")

func gitBlobSHA(content []byte) string {
	return plumbing.ComputeHash(plumbing.BlobObject, content).String()
}

func verifyBlobContent(blob models.Blob) error {
	if blob.SHA == "" {
		return nil
	}
	if gitBlobSHA([]byte(blob.Content)) == blob.SHA {
		return nil
	}
	if blob.Encoding == "base64" {
		if raw, err := base64.StdEncoding.DecodeString(blob.Content); err == nil && gitBlobSHA(raw) == blob.SHA {
			return nil
		}
	}
	return fmt.Errorf("blob %s at %q: %w", shortSHA(blob.SHA), blob.Path, ErrContentTampered)
}
