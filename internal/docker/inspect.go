package docker

import (
	"context"
	"encoding/json"
	"fmt"
)

type InspectConfig struct {
	Labels map[string]string `json:"Labels"`
	User   string            `json:"User"`
	Env    []string          `json:"Env"`
}

type ImageInspect struct {
	ID           string        `json:"Id"`
	Architecture string        `json:"Architecture"`
	Os           string        `json:"Os"`
	Config       InspectConfig `json:"Config"`
}

type ContainerState struct {
	Status  string `json:"Status"`
	Running bool   `json:"Running"`
}

type ContainerInspect struct {
	ID     string         `json:"Id"`
	Name   string         `json:"Name"`
	Image  string         `json:"Image"`
	Config InspectConfig  `json:"Config"`
	State  ContainerState `json:"State"`
}

func (c *Client) InspectImage(ctx context.Context, image string) (ImageInspect, error) {
	data, err := c.CombinedOutput(ctx, "image", "inspect", image)
	if err != nil {
		return ImageInspect{}, err
	}
	var values []ImageInspect
	if err := json.Unmarshal([]byte(data), &values); err != nil {
		return ImageInspect{}, fmt.Errorf("parse docker image inspect: %w", err)
	}
	if len(values) == 0 {
		return ImageInspect{}, fmt.Errorf("image %q not found", image)
	}
	return values[0], nil
}

func (c *Client) InspectContainer(ctx context.Context, container string) (ContainerInspect, error) {
	data, err := c.CombinedOutput(ctx, "inspect", container)
	if err != nil {
		return ContainerInspect{}, err
	}
	var values []ContainerInspect
	if err := json.Unmarshal([]byte(data), &values); err != nil {
		return ContainerInspect{}, fmt.Errorf("parse docker inspect: %w", err)
	}
	if len(values) == 0 {
		return ContainerInspect{}, fmt.Errorf("container %q not found", container)
	}
	return values[0], nil
}
