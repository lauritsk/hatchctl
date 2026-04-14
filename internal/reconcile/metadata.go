package reconcile

import (
	"context"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
	"github.com/lauritsk/hatchctl/internal/spec"
)

func mergedConfigWithRuntimeMetadata(resolved devcontainer.ResolvedConfig, runtimeImage string, metadata []spec.MetadataEntry) spec.MergedConfig {
	if isManagedImage(&resolved, runtimeImage) {
		merged := spec.MergeMetadata(spec.Config{}, metadata)
		merged.Config = resolved.Config
		return merged
	}
	return spec.MergeMetadata(resolved.Config, metadata)
}

func (e *Executor) runtimeMetadataFromImage(ctx context.Context, resolved devcontainer.ResolvedConfig, runtimeImage string) ([]spec.MetadataEntry, error) {
	if runtimeImage == "" {
		return devcontainer.FeaturesMetadata(resolved.Features), nil
	}
	inspect, err := e.engine.InspectImage(ctx, runtimeImage)
	if err != nil {
		if resolved.SourceKind == "compose" || isManagedImage(&resolved, runtimeImage) {
			return devcontainer.FeaturesMetadata(resolved.Features), nil
		}
		return nil, err
	}
	metadata, err := spec.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		return nil, err
	}
	return e.mergeSourceImageMetadata(ctx, resolved, runtimeImage, metadata)
}

func (e *Executor) runtimeMetadataFromContainer(ctx context.Context, resolved devcontainer.ResolvedConfig, inspect *backend.ContainerInspect) ([]spec.MetadataEntry, error) {
	if inspect == nil {
		return nil, nil
	}
	metadata, err := spec.MetadataFromLabel(inspect.Config.Labels[devcontainer.ImageMetadataLabel])
	if err != nil {
		return nil, err
	}
	return e.mergeSourceImageMetadata(ctx, resolved, inspect.Image, metadata)
}
