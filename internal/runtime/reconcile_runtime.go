package runtime

import (
	"context"

	"github.com/lauritsk/hatchctl/internal/devcontainer"
	ui "github.com/lauritsk/hatchctl/internal/display"
	"github.com/lauritsk/hatchctl/internal/engine/dockercli"
	workspaceplan "github.com/lauritsk/hatchctl/internal/plan"
	"github.com/lauritsk/hatchctl/internal/reconcile"
)

func (r *Runner) planDesiredImage(ctx context.Context, resolved devcontainer.ResolvedConfig) (reconcile.DesiredImage, error) {
	if resolved.SourceKind == "compose" {
		config, err := r.readComposeConfig(ctx, &resolved)
		if err != nil {
			return reconcile.DesiredImage{}, err
		}
		service, ok := config.Services[resolved.ComposeService]
		if !ok {
			return reconcile.DesiredImage{}, nil
		}
		if len(resolved.Features) > 0 {
			key, err := reconcile.ManagedImageKey(resolved, resolved.ImageName)
			if err != nil {
				return reconcile.DesiredImage{}, err
			}
			return reconcile.DesiredImage{TargetImage: resolved.ImageName, BuildMode: reconcile.ImageBuildModeFeatures, ReuseKey: key}, nil
		}
		targetImage := service.Image
		if targetImage == "" {
			targetImage = resolved.ComposeProject + "-" + resolved.ComposeService
		}
		if service.Build.Enabled() {
			return reconcile.DesiredImage{TargetImage: targetImage, BuildMode: reconcile.ImageBuildModeCompose}, nil
		}
		return reconcile.DesiredImage{TargetImage: targetImage, BuildMode: reconcile.ImageBuildModeNone, Verify: targetImage != ""}, nil
	}
	if len(resolved.Features) > 0 {
		key, err := reconcile.ManagedImageKey(resolved, resolved.ImageName)
		if err != nil {
			return reconcile.DesiredImage{}, err
		}
		return reconcile.DesiredImage{TargetImage: resolved.ImageName, BuildMode: reconcile.ImageBuildModeFeatures, ReuseKey: key}, nil
	}
	if resolved.Config.Image != "" {
		return reconcile.DesiredImage{TargetImage: resolved.Config.Image, BuildMode: reconcile.ImageBuildModeNone, Verify: true}, nil
	}
	key, err := reconcile.ManagedImageKey(resolved, resolved.ImageName)
	if err != nil {
		return reconcile.DesiredImage{}, err
	}
	return reconcile.DesiredImage{TargetImage: resolved.ImageName, BuildMode: reconcile.ImageBuildModeDocker, ReuseKey: key}, nil
}

func (r *Runner) reconcileImage(ctx context.Context, workspacePlan workspaceplan.WorkspacePlan, resolved devcontainer.ResolvedConfig, events ui.Sink) (string, reconcile.ImagePlan, error) {
	desired, err := r.planDesiredImage(ctx, resolved)
	if err != nil {
		return "", reconcile.ImagePlan{}, err
	}
	observed, err := reconcile.NewObserver(r.backend).Observe(ctx, reconcile.ObserveRequest{
		Plan:         workspacePlan,
		Resolved:     resolved,
		ImageRef:     desired.TargetImage,
		ObserveImage: desired.TargetImage != "",
	})
	if err != nil {
		return "", reconcile.ImagePlan{}, err
	}
	plan := reconcile.PlanImage(desired, observed.Image)
	switch plan.Action {
	case reconcile.ImageActionUseTarget:
		if plan.Verify && plan.TargetImage != "" {
			if err := r.verifyImageReference(ctx, plan.TargetImage, events); err != nil {
				return "", reconcile.ImagePlan{}, err
			}
		}
		return plan.TargetImage, plan, nil
	case reconcile.ImageActionReuseTarget:
		return plan.TargetImage, plan, nil
	case reconcile.ImageActionBuildTarget:
		image, err := r.buildManagedImage(ctx, resolved, plan, events)
		if err != nil {
			return "", reconcile.ImagePlan{}, err
		}
		return image, plan, nil
	default:
		return plan.TargetImage, plan, nil
	}
}

func (r *Runner) buildManagedImage(ctx context.Context, resolved devcontainer.ResolvedConfig, plan reconcile.ImagePlan, events ui.Sink) (string, error) {
	if resolved.SourceKind == "compose" {
		return r.ensureComposeImage(ctx, resolved, plan.ReuseKey, events)
	}
	if plan.BuildMode == reconcile.ImageBuildModeFeatures {
		return r.ensureImageWithFeatures(ctx, resolved, plan.ReuseKey, events)
	}
	return resolved.ImageName, r.buildDockerfileImage(ctx, resolved, resolved.ImageName, plan.ReuseKey, events)
}

func (r *Runner) desiredContainerKey(resolved devcontainer.ResolvedConfig, imagePlan reconcile.ImagePlan, image string, bridgeEnabled bool, sshAgent bool) (string, error) {
	imageIdentity := image
	if imagePlan.ReuseKey != "" {
		imageIdentity = imagePlan.ReuseKey
	}
	return reconcile.ContainerKey(resolved, imageIdentity, bridgeEnabled, sshAgent)
}

func (r *Runner) reconcileContainer(ctx context.Context, observed reconcile.ObservedState, resolved devcontainer.ResolvedConfig, image string, imagePlan reconcile.ImagePlan, bridgeEnabled bool, sshAgent bool, forceNew bool, events ui.Sink) (string, string, bool, error) {
	containerKey, err := r.desiredContainerKey(resolved, imagePlan, image, bridgeEnabled, sshAgent)
	if err != nil {
		return "", "", false, err
	}
	plan, err := reconcile.PlanContainer(observed, reconcile.DesiredContainer{ReuseKey: containerKey, ForceNew: forceNew})
	if err != nil {
		return "", "", false, err
	}
	switch plan.Action {
	case reconcile.ContainerActionReuse:
		return plan.ContainerID, containerKey, false, nil
	case reconcile.ContainerActionStart:
		stdout, stderr := r.progressWriters(events, phaseContainer, "Starting existing container "+plan.ContainerID, r.stdout, r.stderr)
		if err := r.backend.StartContainer(ctx, dockercli.StartContainerRequest{ContainerID: plan.ContainerID, Streams: dockercli.Streams{Stdout: stdout, Stderr: stderr}}); err != nil {
			return "", "", false, err
		}
		return plan.ContainerID, containerKey, false, nil
	case reconcile.ContainerActionReplace:
		if plan.NeedsCleanup && plan.ContainerID != "" {
			if err := r.removeContainer(ctx, plan.ContainerID, events); err != nil {
				return "", "", false, err
			}
		}
		containerID, err := r.createContainer(ctx, resolved, image, containerKey, bridgeEnabled, sshAgent, "", events)
		return containerID, containerKey, true, err
	case reconcile.ContainerActionCreate:
		containerID, err := r.createContainer(ctx, resolved, image, containerKey, bridgeEnabled, sshAgent, "", events)
		return containerID, containerKey, true, err
	default:
		return plan.ContainerID, containerKey, false, nil
	}
}

func (r *Runner) desiredLifecycleKey(resolved devcontainer.ResolvedConfig, containerKey string, dotfiles DotfilesOptions) (string, error) {
	return reconcile.LifecycleKey(resolved, containerKey, reconcile.DotfilesConfig{Repository: dotfiles.Repository, InstallCommand: dotfiles.InstallCommand, TargetPath: dotfiles.TargetPath})
}
