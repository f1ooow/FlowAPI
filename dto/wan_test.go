package dto

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWanOfficialRequestContract(t *testing.T) {
	var r WanRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"wan3.0-video","input":{"media":[{"type":"reference_image","url":"https://example.com/image.png"}]},"parameters":{"audio":false,"seed":0,"prompt_extend":false,"watermark":false}}`), &r))
	require.NoError(t, r.NormalizeAndValidate())
	assert.Equal(t, "1080P", r.Parameters.Resolution)
	assert.Equal(t, "adaptive", r.Parameters.Ratio)
	assert.Equal(t, 5, *r.Parameters.Duration)
	b, err := common.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"audio":false`)
	assert.Contains(t, string(b), `"seed":0`)
	assert.Contains(t, string(b), `"prompt_extend":false`)
	for _, tc := range []struct{ name, body string }{
		{"smart duration", `{"prompt":"test"},"parameters":{"duration":-1}`},
		{"tail only", `{"media":[{"type":"last_frame","url":"https://example.com/x.png"}]}`},
		{"mixed mode", `{"media":[{"type":"first_frame","url":"https://example.com/x.png"},{"type":"reference_image","url":"https://example.com/x.png"}]}`},
		{"file and link", `{"media":[{"type":"file","url":"https://example.com/x.pdf"},{"type":"link","url":"https://example.com/article"}]}`},
		{"file no rewrite", `{"media":[{"type":"file","url":"https://example.com/x.pdf"}]},"parameters":{"prompt_extend":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var r WanRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"wan3.0-video","input":`+tc.body+`}`), &r))
			assert.Error(t, r.NormalizeAndValidate())
		})
	}
}
