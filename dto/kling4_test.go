package dto

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKling4RequestNormalizesDocumentedAliases(t *testing.T) {
	var req Kling4Request
	require.NoError(t, common.Unmarshal([]byte(`{"model":"kling-4.0","prompt":"A calm sea","duration":3,"resolution":"720p","image":"https://media.example/first.png","input_reference":"https://media.example/second.png","ratio":"21:9","n":1,"metadata":{}}`), &req))
	require.NoError(t, req.NormalizeAndValidate())
	assert.Equal(t, []string{"https://media.example/first.png"}, req.Images)
	require.NotNil(t, req.AspectRatio)
	assert.Equal(t, "21:9", *req.AspectRatio)
	assert.Equal(t, 1.0, req.ResolutionRatio())
	encoded, err := common.Marshal(req)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"kling-4.0","prompt":"A calm sea","duration":3,"resolution":"720p","images":["https://media.example/first.png"],"aspect_ratio":"21:9","n":1}`, string(encoded))
}

func TestKling4RequestPreservesCanonicalPrecedence(t *testing.T) {
	for _, input := range []string{
		`{"images":[],"image":"https://media.example/ignored.png","aspect_ratio":"","ratio":"16:9"}`,
		`{"images":[],"input_reference":"https://media.example/ignored.png","aspect_ratio":"","ratio":"16:9"}`,
	} {
		var req Kling4Request
		require.NoError(t, common.Unmarshal([]byte(`{"model":"kling-4.0","prompt":"Sea","duration":30,"resolution":"1080p",`+input[1:]), &req))
		require.NoError(t, req.NormalizeAndValidate())
		assert.Empty(t, req.Images)
		require.NotNil(t, req.AspectRatio)
		assert.Empty(t, *req.AspectRatio)
		assert.Equal(t, 1.4, req.ResolutionRatio())
		encoded, err := common.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "ignored.png")
	}
}

func TestKling4RequestFallsBackToInputReference(t *testing.T) {
	var req Kling4Request
	require.NoError(t, common.Unmarshal([]byte(`{"model":"kling-4.0","prompt":"Sea","duration":5,"resolution":"720p","image":"","input_reference":"https://media.example/reference.png"}`), &req))
	require.NoError(t, req.NormalizeAndValidate())
	assert.Equal(t, []string{"https://media.example/reference.png"}, req.Images)
	assert.Nil(t, req.InputReference)
}

func TestKling4RequestPreservesRepeatedReferencesAtInputLimits(t *testing.T) {
	var req Kling4Request
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"kling-4.0", "prompt":"Sea", "duration":5, "resolution":"720p",
		"images":[
			"https://m.example/i", "https://m.example/i", "https://m.example/i", "https://m.example/i", "https://m.example/i",
			"https://m.example/i", "https://m.example/i", "https://m.example/i", "https://m.example/i", "https://m.example/i"
		],
		"videos":["https://m.example/v", "https://m.example/v", "https://m.example/v", "https://m.example/v", "https://m.example/v"],
		"audios":["https://m.example/a", "https://m.example/a", "https://m.example/a"]
	}`), &req))
	require.NoError(t, req.NormalizeAndValidate())
	assert.Len(t, req.Images, 10)
	assert.Len(t, req.Videos, 5)
	assert.Len(t, req.Audios, 3)
	assert.Equal(t, req.Videos[0], req.Videos[4], "repeated references are separate billed inputs")
}

func TestKling4RequestRejectsInvalidJSONFieldTypes(t *testing.T) {
	for _, input := range []string{
		`{"duration":3.5}`,
		`{"duration":18446744073709551615}`,
		`{"duration":"5"}`,
		`{"n":1.5}`,
		`{"n":18446744073709551615}`,
		`{"n":false}`,
		`{"images":"https://media.example/reference.png"}`,
		`{"videos":[{"url":"https://media.example/reference.mp4"}]}`,
	} {
		t.Run(input, func(t *testing.T) {
			var req Kling4Request
			require.Error(t, common.Unmarshal([]byte(input), &req))
		})
	}
}

func TestKling4RequestRejectsInvalidContract(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
		value string
	}{
		{"wrong model", "model", `"kling-4.0-1080p"`},
		{"empty prompt", "prompt", `"  "`},
		{"missing duration", "duration", `null`},
		{"short duration", "duration", `2`},
		{"long duration", "duration", `31`},
		{"zero duration", "duration", `0`},
		{"missing resolution", "resolution", `""`},
		{"unsupported resolution", "resolution", `"4k"`},
		{"multiple outputs", "n", `2`},
		{"zero outputs", "n", `0`},
		{"undocumented metadata", "metadata", `{"duration":30}`},
		{"too many images", "images", `["https://m.example/a",` + strings.Repeat(`"https://m.example/a",`, 9) + `"https://m.example/a"]`},
		{"too many videos", "videos", `["https://m.example/a",` + strings.Repeat(`"https://m.example/a",`, 4) + `"https://m.example/a"]`},
		{"too many audios", "audios", `["https://m.example/a","https://m.example/a","https://m.example/a","https://m.example/a"]`},
		{"file URL", "images", `["file:///tmp/reference.png"]`},
		{"authenticated URL", "videos", `["https://user@m.example/reference.mp4"]`},
		{"empty URL", "audios", `[""]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"model": "kling-4.0", "prompt": "Sea", "duration": 5, "resolution": "720p"}
			var value any
			require.NoError(t, common.Unmarshal([]byte(tc.value), &value))
			input[tc.field] = value
			data, err := common.Marshal(input)
			require.NoError(t, err)
			var req Kling4Request
			require.NoError(t, common.Unmarshal(data, &req))
			require.Error(t, req.NormalizeAndValidate())
		})
	}
}
