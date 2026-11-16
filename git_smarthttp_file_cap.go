package mwanachamagit

import (
	"errors"
	"io"

	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
)

var errPushTooLarge = errors.New("push body is over the limit")

type cappedBody struct {
	source  io.Reader
	left    int64
	tripped bool
}

func newCappedBody(source io.Reader, limit int64) *cappedBody {
	return &cappedBody{source: source, left: limit}
}

func (c *cappedBody) Read(p []byte) (int, error) {
	if c.tripped {
		return 0, errPushTooLarge
	}
	if int64(len(p)) > c.left+1 {
		p = p[:c.left+1]
	}
	n, err := c.source.Read(p)
	if int64(n) > c.left {
		n = int(c.left)
		c.left = 0
		c.tripped = true
		return n, errPushTooLarge
	}
	c.left -= int64(n)
	return n, err
}

func pushTooLargeStatus(limit int64, req *packp.ReferenceUpdateRequest) *packp.ReportStatus {
	message := pushTooLargeMessage(limit)
	status := packp.NewReportStatus()
	status.UnpackStatus = message
	if req == nil {
		return status
	}
	for _, cmd := range req.Commands {
		status.CommandStatuses = append(status.CommandStatuses, &packp.CommandStatus{ReferenceName: cmd.Name, Status: message})
	}
	return status
}
