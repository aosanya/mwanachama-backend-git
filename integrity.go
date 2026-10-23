package mwanachamagit

import (
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/aosanya/mwanachama-backend-git/models"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
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

func verifyCommitObject(c models.Commit) error {
	if c.Data == "" || c.SHA == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(c.Data)
	if err != nil || plumbing.ComputeHash(plumbing.CommitObject, raw).String() != c.SHA {
		return fmt.Errorf("commit %s: %w", shortSHA(c.SHA), ErrContentTampered)
	}
	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.CommitObject)
	if _, err := obj.Write(raw); err != nil {
		return fmt.Errorf("commit %s: %w", shortSHA(c.SHA), ErrContentTampered)
	}
	var decoded object.Commit
	if err := decoded.Decode(obj); err != nil || decoded.Message != c.Message {
		return fmt.Errorf("commit %s: %w", shortSHA(c.SHA), ErrContentTampered)
	}
	return nil
}
