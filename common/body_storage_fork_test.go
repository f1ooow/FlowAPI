package common

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

func TestForkBodyStorageOwnsCursorAndSurvivesParentClose(t *testing.T) {
	for _, disk := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "disk"}[disk], func(t *testing.T) {
			var original BodyStorage
			if disk {
				storage, err := newDiskStorage([]byte("payload"), t.TempDir())
				require.NoError(t, err)
				original = storage
			} else {
				original = newMemoryStorage([]byte("payload"))
			}
			a, err := ForkBodyStorage(original)
			require.NoError(t, err)
			defer a.Close()
			b, err := ForkBodyStorage(original)
			require.NoError(t, err)
			defer b.Close()
			require.NoError(t, original.Close())
			prefix := make([]byte, 3)
			_, err = io.ReadFull(a, prefix)
			require.NoError(t, err)
			assert.Equal(t, "pay", string(prefix))
			other, err := io.ReadAll(b)
			require.NoError(t, err)
			assert.Equal(t, "payload", string(other))
			replay, err := a.NewReader()
			require.NoError(t, err)
			defer replay.Close()
			full, err := io.ReadAll(replay)
			require.NoError(t, err)
			assert.Equal(t, "payload", string(full))
			tail, err := io.ReadAll(a)
			require.NoError(t, err)
			assert.Equal(t, "load", string(tail))
		})
	}
}
