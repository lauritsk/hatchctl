package devcontainer

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const ImageMetadataLabel = "devcontainer.metadata"

type MetadataEntry struct {
	ID                   string            `json:"id,omitempty"`
	Init                 *bool             `json:"init,omitempty"`
	Privileged           *bool             `json:"privileged,omitempty"`
	CapAdd               []string          `json:"capAdd,omitempty"`
	SecurityOpt          []string          `json:"securityOpt,omitempty"`
	Mounts               []string          `json:"mounts,omitempty"`
	OnCreateCommand      LifecycleCommand  `json:"onCreateCommand,omitempty"`
	UpdateContentCommand LifecycleCommand  `json:"updateContentCommand,omitempty"`
	PostCreateCommand    LifecycleCommand  `json:"postCreateCommand,omitempty"`
	PostStartCommand     LifecycleCommand  `json:"postStartCommand,omitempty"`
	PostAttachCommand    LifecycleCommand  `json:"postAttachCommand,omitempty"`
	WaitFor              string            `json:"waitFor,omitempty"`
	RemoteUser           string            `json:"remoteUser,omitempty"`
	ContainerUser        string            `json:"containerUser,omitempty"`
	RemoteEnv            map[string]string `json:"remoteEnv,omitempty"`
	ContainerEnv         map[string]string `json:"containerEnv,omitempty"`
	OverrideCommand      *bool             `json:"overrideCommand,omitempty"`
	ForwardPorts         []string          `json:"forwardPorts,omitempty"`
	Customizations       map[string]any    `json:"customizations,omitempty"`
}

type MergedConfig struct {
	Config                Config
	Init                  bool
	Privileged            bool
	CapAdd                []string
	SecurityOpt           []string
	Mounts                []string
	RemoteUser            string
	ContainerUser         string
	RemoteEnv             map[string]string
	ContainerEnv          map[string]string
	OverrideCommand       *bool
	WaitFor               string
	ForwardPorts          []string
	OnCreateCommands      []LifecycleCommand
	UpdateContentCommands []LifecycleCommand
	PostCreateCommands    []LifecycleCommand
	PostStartCommands     []LifecycleCommand
	PostAttachCommands    []LifecycleCommand
	Customizations        map[string][]any
	Metadata              []MetadataEntry
}

func MetadataFromLabel(value string) ([]MetadataEntry, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var list []MetadataEntry
	if err := json.Unmarshal([]byte(value), &list); err == nil {
		return list, nil
	}
	var single MetadataEntry
	if err := json.Unmarshal([]byte(value), &single); err == nil {
		return []MetadataEntry{single}, nil
	}
	return nil, fmt.Errorf("parse %s label", ImageMetadataLabel)
}

func ConfigMetadata(config Config) MetadataEntry {
	return MetadataEntry{
		Init:                 config.Init,
		Privileged:           config.Privileged,
		CapAdd:               cloneSlice(config.CapAdd),
		SecurityOpt:          cloneSlice(config.SecurityOpt),
		Mounts:               cloneSlice(config.Mounts),
		OnCreateCommand:      config.OnCreateCommand,
		UpdateContentCommand: config.UpdateContentCommand,
		PostCreateCommand:    config.PostCreateCommand,
		PostStartCommand:     config.PostStartCommand,
		PostAttachCommand:    config.PostAttachCommand,
		WaitFor:              config.WaitFor,
		RemoteUser:           config.RemoteUser,
		ContainerUser:        config.ContainerUser,
		RemoteEnv:            cloneMap(config.RemoteEnv),
		ContainerEnv:         cloneMap(config.ContainerEnv),
		OverrideCommand:      config.OverrideCommand,
		Customizations:       config.Customizations,
	}
}

func MergeMetadata(config Config, metadata []MetadataEntry) MergedConfig {
	entries := append(cloneEntries(metadata), ConfigMetadata(config))
	reversed := reverseEntries(entries)
	merged := MergedConfig{
		Config:                config,
		CapAdd:                unionStrings(entries, func(entry MetadataEntry) []string { return entry.CapAdd }),
		SecurityOpt:           unionStrings(entries, func(entry MetadataEntry) []string { return entry.SecurityOpt }),
		Mounts:                mergeMounts(entries),
		RemoteEnv:             mergeStringMaps(entries, func(entry MetadataEntry) map[string]string { return entry.RemoteEnv }),
		ContainerEnv:          mergeStringMaps(entries, func(entry MetadataEntry) map[string]string { return entry.ContainerEnv }),
		ForwardPorts:          unionStrings(entries, func(entry MetadataEntry) []string { return entry.ForwardPorts }),
		OnCreateCommands:      collectCommands(entries, func(entry MetadataEntry) LifecycleCommand { return entry.OnCreateCommand }),
		UpdateContentCommands: collectCommands(entries, func(entry MetadataEntry) LifecycleCommand { return entry.UpdateContentCommand }),
		PostCreateCommands:    collectCommands(entries, func(entry MetadataEntry) LifecycleCommand { return entry.PostCreateCommand }),
		PostStartCommands:     collectCommands(entries, func(entry MetadataEntry) LifecycleCommand { return entry.PostStartCommand }),
		PostAttachCommands:    collectCommands(entries, func(entry MetadataEntry) LifecycleCommand { return entry.PostAttachCommand }),
		Customizations:        mergeCustomizations(entries),
		Metadata:              entries,
	}
	for _, entry := range entries {
		if entry.Init != nil && *entry.Init {
			merged.Init = true
		}
		if entry.Privileged != nil && *entry.Privileged {
			merged.Privileged = true
		}
	}
	merged.WaitFor = pickLastString(reversed, func(entry MetadataEntry) string { return entry.WaitFor })
	merged.RemoteUser = pickLastString(reversed, func(entry MetadataEntry) string { return entry.RemoteUser })
	merged.ContainerUser = pickLastString(reversed, func(entry MetadataEntry) string { return entry.ContainerUser })
	merged.OverrideCommand = pickLastBool(reversed, func(entry MetadataEntry) *bool { return entry.OverrideCommand })
	return merged
}

func SortedMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mergeStringMaps(entries []MetadataEntry, pick func(MetadataEntry) map[string]string) map[string]string {
	result := map[string]string{}
	for _, entry := range entries {
		for key, value := range pick(entry) {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func unionStrings(entries []MetadataEntry, pick func(MetadataEntry) []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, entry := range entries {
		for _, value := range pick(entry) {
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func collectCommands(entries []MetadataEntry, pick func(MetadataEntry) LifecycleCommand) []LifecycleCommand {
	var result []LifecycleCommand
	for _, entry := range entries {
		command := pick(entry)
		if !command.Empty() {
			result = append(result, command)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeMounts(entries []MetadataEntry) []string {
	type pair struct {
		target string
		value  string
	}
	seen := map[string]struct{}{}
	var reversed []pair
	for i := len(entries) - 1; i >= 0; i-- {
		for j := len(entries[i].Mounts) - 1; j >= 0; j-- {
			mount := entries[i].Mounts[j]
			target := mountTarget(mount)
			if target == "" {
				target = mount
			}
			if _, ok := seen[target]; ok {
				continue
			}
			seen[target] = struct{}{}
			reversed = append(reversed, pair{target: target, value: mount})
		}
	}
	if len(reversed) == 0 {
		return nil
	}
	result := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		result = append(result, reversed[i].value)
	}
	return result
}

func mountTarget(mount string) string {
	for _, part := range strings.Split(mount, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "target=") {
			return strings.TrimPrefix(part, "target=")
		}
		if strings.HasPrefix(part, "dst=") {
			return strings.TrimPrefix(part, "dst=")
		}
	}
	return ""
}

func mergeCustomizations(entries []MetadataEntry) map[string][]any {
	result := map[string][]any{}
	for _, entry := range entries {
		for key, value := range entry.Customizations {
			result[key] = append(result[key], value)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func pickLastString(entries []MetadataEntry, pick func(MetadataEntry) string) string {
	for _, entry := range entries {
		if value := pick(entry); value != "" {
			return value
		}
	}
	return ""
}

func pickLastBool(entries []MetadataEntry, pick func(MetadataEntry) *bool) *bool {
	for _, entry := range entries {
		if value := pick(entry); value != nil {
			copy := *value
			return &copy
		}
	}
	return nil
}

func cloneEntries(entries []MetadataEntry) []MetadataEntry {
	result := make([]MetadataEntry, len(entries))
	copy(result, entries)
	return result
}

func reverseEntries(entries []MetadataEntry) []MetadataEntry {
	result := cloneEntries(entries)
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func cloneSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
