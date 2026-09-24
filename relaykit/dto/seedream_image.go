package dto

const (
	Seedream40    = "doubao-seedream-4-0-250828"
	Seedream45    = "doubao-seedream-4-5-251128"
	Seedream50    = "doubao-seedream-5-0-260128"
	Seedream50Pro = "doubao-seedream-5-0-pro-260628"
	// Pro decomposition returns one base image and up to sixteen layers.
	MaxSeedreamLayerImages        = 17
	SeedreamLayerImagePriceFactor = 0.5
)

type SequentialImageGenerationOptions struct {
	MaxImages *uint `json:"max_images,omitempty"`
}

func IsSeedreamModel(model string) bool {
	switch model {
	case Seedream40, Seedream45, Seedream50, Seedream50Pro, "seedream-4-0-250828":
		return true
	default:
		return false
	}
}

func (i *ImageRequest) IsSeedreamLayerDecomposition() bool {
	return i != nil && i.Model == Seedream50Pro && i.LayerDecomposition != nil && *i.LayerDecomposition
}
