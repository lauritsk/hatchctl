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

func ConfigMetadata(config Config) MetadataEntry {
	return spec.ConfigMetadata(config)
}

func MergeMetadata(config Config, metadata []MetadataEntry) MergedConfig {
	return spec.MergeMetadata(config, metadata)
}

func SortedMapKeys(values map[string]string) []string {
	return spec.SortedMapKeys(values)
}
