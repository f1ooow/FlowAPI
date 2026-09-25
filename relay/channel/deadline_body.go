package channel

import (
	"context"
	"io"
)

type deadlineResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *deadlineResponseBody) Close() error {
	b.cancel()
	return b.ReadCloser.Close()
}
