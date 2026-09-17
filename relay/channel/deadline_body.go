package channel

import (
	"context"
	"fmt"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"io"
)

type deadlineResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

const maxHedgeDrainBytes = 16 << 20

type hedgeResponseBody struct {
	io.ReadCloser
	attempt *relaycommon.HedgeAttempt
	drained int64
}

func (b *hedgeResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if b.attempt.Detached.Load() {
		b.drained += int64(n)
		if b.drained > maxHedgeDrainBytes {
			return n, fmt.Errorf("hedge usage collection exceeded byte limit")
		}
	}
	return n, err
}

func (b *deadlineResponseBody) Close() error {
	b.cancel()
	return b.ReadCloser.Close()
}
