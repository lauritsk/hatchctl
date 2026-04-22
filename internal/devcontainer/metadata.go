package devcontainer

func FeaturesMetadata(features []ResolvedFeature) []MetadataEntry {
	if len(features) == 0 {
		return nil
	}
	metadata := make([]MetadataEntry, 0, len(features))
	for _, feature := range features {
		metadata = append(metadata, feature.Metadata)
	}
	return metadata
}
