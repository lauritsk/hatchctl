package devcontainer

import "github.com/lauritsk/hatchctl/internal/spec"

const ImageMetadataLabel = spec.ImageMetadataLabel

type (
	MetadataEntry = spec.MetadataEntry
	MergedConfig  = spec.MergedConfig
)

func MetadataFromLabel(value string) ([]MetadataEntry, error) {
	return spec.MetadataFromLabel(value)
}

func MetadataLabelValue(entries []MetadataEntry) (string, error) {
	return spec.MetadataLabelValue(entries)
}

func MergeMetadata(config Config, metadata []MetadataEntry) MergedConfig {
	return spec.MergeMetadata(config, metadata)
}
