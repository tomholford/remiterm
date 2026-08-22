package tui

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	_ "golang.org/x/image/webp"
)

const previewMaxBytes = 2 << 20

// decodeStill reads at most maxBytes and decodes a still image.
// GIF animation yields the first frame. Unknown or truncated payloads error.
func decodeStill(r io.Reader, maxBytes int64) (image.Image, string, error) {
	if maxBytes <= 0 {
		maxBytes = previewMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("image exceeds %d byte limit", maxBytes)
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	return img, format, nil
}
