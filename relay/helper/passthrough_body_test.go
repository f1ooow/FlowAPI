package helper

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPassthroughBodyPreservesUnmappedAndNonJSONBodies(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, payload, target string
		mapped                             bool
	}{
		{"no match", "application/json", " {\n\"model\" : \"client\", \"n\":1e2 }\n", "client", false},
		{"same target", "application/json", " {\"model\":\"client\"} ", "client", true},
		{"incidental normalization is not a redirect", "application/json", " {\"model\":\"client\"} ", "normalized", false},
		{"multipart", "multipart/form-data", "--boundary\r\nraw model=client\r\n", "upstream", true},
		{"form", "application/x-www-form-urlencoded", "model=client&unknown=1", "upstream", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage, err := common.CreateBodyStorage([]byte(tc.payload))
			require.NoError(t, err)
			defer storage.Close()
			info := &relaycommon.RelayInfo{OriginModelName: "client", ChannelMeta: &relaycommon.ChannelMeta{
				IsModelMapped: tc.mapped, UpstreamModelName: tc.target,
			}}
			body, closer, err := NewPassthroughBody(storage, info, tc.contentType)
			require.NoError(t, err)
			assert.Nil(t, closer, "no-op must reuse the existing storage")
			actual, err := io.ReadAll(body)
			require.NoError(t, err)
			assert.Equal(t, tc.payload, string(actual))
			reader, err := body.NewReader()
			require.NoError(t, err)
			defer reader.Close()
			replayed, err := io.ReadAll(reader)
			require.NoError(t, err)
			assert.Equal(t, actual, replayed)
		})
	}
}

func TestNewPassthroughBodyRedirectPreservesFieldsAndIndependentAttempts(t *testing.T) {
	previous := common.GetDiskCacheConfig()
	t.Cleanup(func() { common.SetDiskCacheConfig(previous) })
	for _, disk := range []bool{false, true} {
		name := "memory"
		if disk {
			name = "disk"
		}
		t.Run(name, func(t *testing.T) {
			common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 64, Path: t.TempDir()})
			payload := []byte(`{"model":"client","unknown":9007199254740993,"decimal":1.2300000000000000001,"nested":{"model":"nested-model","n":18446744073709551615},"zero":0,"false":false,"null":null}`)
			storage, err := common.CreateBodyStorage(payload)
			require.NoError(t, err)
			defer storage.Close()
			require.Equal(t, disk, storage.IsDisk())
			prefix := make([]byte, 7)
			_, err = io.ReadFull(storage, prefix)
			require.NoError(t, err)

			var original map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(payload, &original))
			delete(original, "model")
			for _, target := range []string{"upstream-a", "upstream-b"} {
				info := &relaycommon.RelayInfo{OriginModelName: "client", ChannelMeta: &relaycommon.ChannelMeta{
					IsModelMapped: true, UpstreamModelName: target,
				}}
				body, closer, err := NewPassthroughBody(storage, info, "application/json")
				require.NoError(t, err)
				require.NotNil(t, closer)
				defer closer.Close()
				a, err := body.NewReader()
				require.NoError(t, err)
				defer a.Close()
				b, err := body.NewReader()
				require.NoError(t, err)
				defer b.Close()
				head := make([]byte, 5)
				_, err = io.ReadFull(a, head)
				require.NoError(t, err)
				actual, err := io.ReadAll(b)
				require.NoError(t, err)
				tail, err := io.ReadAll(a)
				require.NoError(t, err)
				assert.Equal(t, actual, append(head, tail...))
				assert.EqualValues(t, len(actual), body.Size())
				var fields map[string]json.RawMessage
				require.NoError(t, common.Unmarshal(actual, &fields))
				assert.Equal(t, `"`+target+`"`, string(fields["model"]))
				delete(fields, "model")
				assert.Equal(t, original, fields, "all unknown values must retain their exact numeric values")
				require.NoError(t, closer.Close())
			}
			rest, err := io.ReadAll(storage)
			require.NoError(t, err)
			assert.Equal(t, payload, append(prefix, rest...), "rewrite must not mutate storage or its cursor")
		})
	}
}

func TestNewPassthroughBodyRejectsInvalidJSONObject(t *testing.T) {
	for _, payload := range []string{`{"model":`, `[]`, `null`, `"client"`, `42`, `true`, `{"model":"client"} {}`} {
		t.Run(payload, func(t *testing.T) {
			storage, err := common.CreateBodyStorage([]byte(payload))
			require.NoError(t, err)
			defer storage.Close()
			info := &relaycommon.RelayInfo{OriginModelName: "client", ChannelMeta: &relaycommon.ChannelMeta{
				IsModelMapped: true, UpstreamModelName: "upstream",
			}}
			body, closer, err := NewPassthroughBody(storage, info, "application/json")
			require.Error(t, err)
			assert.Nil(t, body)
			assert.Nil(t, closer)
		})
	}
}
