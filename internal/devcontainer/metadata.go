package devcontainer

import "github.com/lauritsk/hatchctl/internal/spec"

func FeaturesMetadata(features []ResolvedFeature) []spec.MetadataEntry {
	if len(features) == 0 {
		return nil
	}
	metadata := make([]spec.MetadataEntry, 0, len(features))
	for _, feature := range features {
		metadata = append(metadata, feature.Metadata)
	}
	return metadata
}
