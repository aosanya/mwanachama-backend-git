package mwanachamagit

import (
	"fmt"
	"io"

	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/revlist"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

func uploadPackResponse(store storer.EncodedObjectStorer, req *packp.UploadPackRequest) (*packp.UploadPackResponse, error) {
	if req.IsEmpty() {
		return nil, transport.ErrEmptyUploadPackRequest
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := checkUploadCapabilities(req.Capabilities); err != nil {
		return nil, err
	}
	haves, err := revlist.Objects(store, req.Haves, nil)
	if err != nil {
		return nil, err
	}
	wanted, err := revlist.Objects(store, req.Wants, haves)
	if err != nil {
		return nil, err
	}
	reader, writer := io.Pipe()
	go func() {
		_, err := packfile.NewEncoder(writer, store, false).Encode(wanted, 0)
		writer.CloseWithError(err)
	}()
	return packp.NewUploadPackResponseWithPackfile(req, reader), nil
}

func checkUploadCapabilities(requested *capability.List) error {
	supported := capability.NewList()
	if err := supported.Set(capability.Agent, capability.DefaultAgent()); err != nil {
		return err
	}
	if err := supported.Set(capability.OFSDelta); err != nil {
		return err
	}
	for _, c := range requested.All() {
		if !supported.Supports(c) {
			return fmt.Errorf("unsupported capability: %s", c)
		}
	}
	return nil
}
