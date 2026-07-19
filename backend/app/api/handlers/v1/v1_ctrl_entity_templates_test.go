package v1

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTemplatePhotoContent(t *testing.T) {
	var validPNG bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&validPNG, img))

	mimeType, err := validateTemplatePhotoContent(validPNG.Bytes())
	require.NoError(t, err)
	assert.Equal(t, "image/png", mimeType)

	_, err = validateTemplatePhotoContent([]byte("plain text"))
	require.Error(t, err)

	// A magic prefix alone must not be accepted as an image.
	_, err = validateTemplatePhotoContent([]byte{0xff, 0xd8, 0xff, 0xe0})
	require.Error(t, err)
	assert.ErrorContains(t, err, "valid image")
}
