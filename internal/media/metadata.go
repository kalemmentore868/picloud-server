package media

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

func ImageDimensions(path string, mediaType string) (*int64, *int64) {
	if mediaType != TypePhoto {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer file.Close()

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, nil
	}
	width := int64(cfg.Width)
	height := int64(cfg.Height)
	return &width, &height
}
