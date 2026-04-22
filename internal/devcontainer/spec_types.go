package devcontainer

import (
	"encoding/json"
	"fmt"
	"sort"
)

type Config struct {
	Name                 string            `json:"name,omitempty"`
	Image                string            `json:"image,omitempty"`
	DockerFile           string            `json:"dockerFile,omitempty"`
	WorkspaceFolder      string            `json:"workspaceFolder,omitempty"`
	WorkspaceMount       string            `json:"workspaceMount,omitempty"`
	Mounts               []string          `json:"mounts,omitempty"`
	ContainerEnv         map[string]string `json:"containerEnv,omitempty"`
	RemoteEnv            map[string]string `json:"remoteEnv,omitempty"`
	ContainerUser        string            `json:"containerUser,omitempty"`
	RemoteUser           string            `json:"remoteUser,omitempty"`
	RunArgs              []string          `json:"runArgs,omitempty"`
	ForwardPorts         ForwardPorts      `json:"forwardPorts,omitempty"`
	InitializeCommand    LifecycleCommand  `json:"initializeCommand,omitempty"`
	OnCreateCommand      LifecycleCommand  `json:"onCreateCommand,omitempty"`
	UpdateContentCommand LifecycleCommand  `json:"updateContentCommand,omitempty"`
	PostCreateCommand    LifecycleCommand  `json:"postCreateCommand,omitempty"`
	PostStartCommand     LifecycleCommand  `json:"postStartCommand,omitempty"`
	PostAttachCommand    LifecycleCommand  `json:"postAttachCommand,omitempty"`
	WaitFor              string            `json:"waitFor,omitempty"`
	OverrideCommand      *bool             `json:"overrideCommand,omitempty"`
	UpdateRemoteUserUID  *bool             `json:"updateRemoteUserUID,omitempty"`
	Init                 *bool             `json:"init,omitempty"`
	Privileged           *bool             `json:"privileged,omitempty"`
	CapAdd               []string          `json:"capAdd,omitempty"`
	SecurityOpt          []string          `json:"securityOpt,omitempty"`
	Build                *BuildConfig      `json:"build,omitempty"`
	Customizations       map[string]any    `json:"customizations,omitempty"`
	Features             map[string]any    `json:"features,omitempty"`
	DockerComposeFile    any               `json:"dockerComposeFile,omitempty"`
	Service              string            `json:"service,omitempty"`
	Raw                  map[string]any    `json:"-"`
}

type BuildConfig struct {
	Dockerfile string            `json:"dockerfile,omitempty"`
	Context    string            `json:"context,omitempty"`
	Target     string            `json:"target,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	CacheFrom  any               `json:"cacheFrom,omitempty"`
	Options    []string          `json:"options,omitempty"`
}

type LifecycleCommand struct {
	Kind   string
	Value  string
	Args   []string
	Steps  map[string]LifecycleCommand
	Exists bool
}

type ForwardPorts []string

type LifecycleStep struct {
	Name    string
	Command LifecycleCommand
}

func (p ForwardPorts) MarshalJSON() ([]byte, error) {
	values := make([]any, 0, len(p))
	for _, port := range p {
		if port == "" {
			continue
		}
		if normalized, ok := parseLocalhostPort(port); ok {
			values = append(values, normalized)
			continue
		}
		values = append(values, port)
	}
	if len(values) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(values)
}

func (p *ForwardPorts) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*p = nil
		return nil
	}
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	ports, err := NormalizeForwardPorts(raw)
	if err != nil {
		return err
	}
	*p = ports
	return nil
}

func (c LifecycleCommand) MarshalJSON() ([]byte, error) {
	switch c.Kind {
	case "string":
		return json.Marshal(c.Value)
	case "array":
		return json.Marshal(c.Args)
	case "object":
		if c.Steps == nil {
			return []byte("{}"), nil
		}
		return json.Marshal(c.Steps)
	default:
		return []byte("null"), nil
	}
}

func (c *LifecycleCommand) UnmarshalJSON(data []byte) error {
	c.Exists = true
	if string(data) == "null" {
		*c = LifecycleCommand{}
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		c.Kind = "string"
		c.Value = s
		return nil
	}

	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		c.Kind = "array"
		c.Args = arr
		return nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	c.Kind = "object"
	c.Steps = make(map[string]LifecycleCommand, len(obj))
	for key, raw := range obj {
		var child LifecycleCommand
		if err := json.Unmarshal(raw, &child); err != nil {
			return fmt.Errorf("parse lifecycle command %q: %w", key, err)
		}
		c.Steps[key] = child
	}
	return nil
}

func (c LifecycleCommand) Empty() bool {
	return !c.Exists || c.Kind == ""
}

func (c LifecycleCommand) SortedSteps() []LifecycleStep {
	if c.Kind != "object" {
		return nil
	}
	keys := make([]string, 0, len(c.Steps))
	for key := range c.Steps {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	steps := make([]LifecycleStep, 0, len(keys))
	for _, key := range keys {
		steps = append(steps, LifecycleStep{Name: key, Command: c.Steps[key]})
	}
	return steps
}
