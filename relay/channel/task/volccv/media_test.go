package volccv

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestPreprocessForLqirBoundsAndOutput(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, img))
	out, w, h, err := PreprocessForLqir(b.Bytes())
	require.NoError(t, err)
	assert.Positive(t, len(out))
	assert.Equal(t, 100, w)
	assert.Equal(t, 100, h)
	_, _, _, err = PreprocessForLqir(nil)
	require.Error(t, err)
}
