package godev

func MergeConfig(base, overlay *DevcontainerConfig) *DevcontainerConfig {
	if base == nil && overlay == nil {
		return &DevcontainerConfig{}
	}
	if base == nil {
		return cloneConfig(overlay)
	}
	if overlay == nil {
		return cloneConfig(base)
	}
	merged := cloneConfig(base)
	if overlay.Name != "" {
		merged.Name = overlay.Name
	}
	if overlay.Image != "" {
		merged.Image = overlay.Image
	}
	merged.Build = mergeBuild(merged.Build, overlay.Build)
	merged.DockerComposeFile = append(merged.DockerComposeFile, overlay.DockerComposeFile...)
	if overlay.Service != "" {
		merged.Service = overlay.Service
	}
	merged.RunServices = append(merged.RunServices, overlay.RunServices...)
	if overlay.ShutdownAction != "" {
		merged.ShutdownAction = overlay.ShutdownAction
	}
	merged.ForwardPorts = append(merged.ForwardPorts, overlay.ForwardPorts...)
	merged.AppPort = append(merged.AppPort, overlay.AppPort...)
	merged.ContainerEnv = mergeStringMap(merged.ContainerEnv, overlay.ContainerEnv)
	merged.Mounts = append(merged.Mounts, overlay.Mounts...)
	if overlay.WorkspaceMount != "" {
		merged.WorkspaceMount = overlay.WorkspaceMount
	}
	if overlay.WorkspaceFolder != "" {
		merged.WorkspaceFolder = overlay.WorkspaceFolder
	}
	merged.RunArgs = append(merged.RunArgs, overlay.RunArgs...)
	merged.Privileged = merged.Privileged || overlay.Privileged
	merged.CapAdd = append(merged.CapAdd, overlay.CapAdd...)
	merged.SecurityOpt = append(merged.SecurityOpt, overlay.SecurityOpt...)
	merged.Init = mergeInit(merged.Init, overlay.Init)
	if overlay.ContainerUser != "" {
		merged.ContainerUser = overlay.ContainerUser
	}
	if overlay.RemoteUser != "" {
		merged.RemoteUser = overlay.RemoteUser
	}
	merged.RemoteEnv = mergeStringMap(merged.RemoteEnv, overlay.RemoteEnv)
	merged.Features = mergeFeatureSet(merged.Features, overlay.Features)
	merged.OverrideFeatureInstallOrder = append(merged.OverrideFeatureInstallOrder, overlay.OverrideFeatureInstallOrder...)
	if overlay.OverrideCommand != nil {
		merged.OverrideCommand = cloneBoolPtr(overlay.OverrideCommand)
	}
	if overlay.InitializeCommand != nil {
		merged.InitializeCommand = cloneLifecycleCommands(overlay.InitializeCommand)
	}
	if overlay.OnCreateCommand != nil {
		merged.OnCreateCommand = cloneLifecycleCommands(overlay.OnCreateCommand)
	}
	if overlay.UpdateContentCommand != nil {
		merged.UpdateContentCommand = cloneLifecycleCommands(overlay.UpdateContentCommand)
	}
	if overlay.PostCreateCommand != nil {
		merged.PostCreateCommand = cloneLifecycleCommands(overlay.PostCreateCommand)
	}
	if overlay.PostStartCommand != nil {
		merged.PostStartCommand = cloneLifecycleCommands(overlay.PostStartCommand)
	}
	if overlay.PostAttachCommand != nil {
		merged.PostAttachCommand = cloneLifecycleCommands(overlay.PostAttachCommand)
	}
	return merged
}

func cloneConfig(cfg *DevcontainerConfig) *DevcontainerConfig {
	if cfg == nil {
		return nil
	}
	out := *cfg
	out.Build = cloneBuild(cfg.Build)
	out.DockerComposeFile = cloneStringSlice(cfg.DockerComposeFile)
	out.RunServices = cloneStrings(cfg.RunServices)
	out.ForwardPorts = clonePortList(cfg.ForwardPorts)
	out.AppPort = clonePortList(cfg.AppPort)
	out.ContainerEnv = cloneStringMap(cfg.ContainerEnv)
	out.RemoteEnv = cloneStringMap(cfg.RemoteEnv)
	out.Mounts = cloneMounts(cfg.Mounts)
	out.RunArgs = cloneStrings(cfg.RunArgs)
	out.CapAdd = cloneStrings(cfg.CapAdd)
	out.SecurityOpt = cloneStrings(cfg.SecurityOpt)
	out.Features = cloneFeatureSet(cfg.Features)
	out.OverrideFeatureInstallOrder = cloneStrings(cfg.OverrideFeatureInstallOrder)
	out.Init = cloneBoolPtr(cfg.Init)
	out.OverrideCommand = cloneBoolPtr(cfg.OverrideCommand)
	out.InitializeCommand = cloneLifecycleCommands(cfg.InitializeCommand)
	out.OnCreateCommand = cloneLifecycleCommands(cfg.OnCreateCommand)
	out.UpdateContentCommand = cloneLifecycleCommands(cfg.UpdateContentCommand)
	out.PostCreateCommand = cloneLifecycleCommands(cfg.PostCreateCommand)
	out.PostStartCommand = cloneLifecycleCommands(cfg.PostStartCommand)
	out.PostAttachCommand = cloneLifecycleCommands(cfg.PostAttachCommand)
	return &out
}

func mergeBuild(base, overlay *DevcontainerBuild) *DevcontainerBuild {
	if base == nil && overlay == nil {
		return nil
	}
	if base == nil {
		return cloneBuild(overlay)
	}
	if overlay == nil {
		return cloneBuild(base)
	}
	merged := cloneBuild(base)
	if overlay.Dockerfile != "" {
		merged.Dockerfile = overlay.Dockerfile
	}
	if overlay.Context != "" {
		merged.Context = overlay.Context
	}
	merged.Args = mergeStringMap(merged.Args, overlay.Args)
	merged.CacheFrom = append(merged.CacheFrom, overlay.CacheFrom...)
	merged.Options = append(merged.Options, overlay.Options...)
	if overlay.Target != "" {
		merged.Target = overlay.Target
	}
	return merged
}

func mergeInit(base, overlay *bool) *bool {
	if base == nil && overlay == nil {
		return nil
	}
	value := (base != nil && *base) || (overlay != nil && *overlay)
	return &value
}

func cloneBuild(cfg *DevcontainerBuild) *DevcontainerBuild {
	if cfg == nil {
		return nil
	}
	out := *cfg
	out.Args = cloneStringMap(cfg.Args)
	out.CacheFrom = cloneStringSlice(cfg.CacheFrom)
	out.Options = cloneStrings(cfg.Options)
	return &out
}

func mergeStringMap(base, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := cloneStringMap(base)
	if merged == nil {
		merged = make(map[string]string, len(overlay))
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneStringSlice(values StringSlice) StringSlice {
	if len(values) == 0 {
		return nil
	}
	clone := make(StringSlice, len(values))
	copy(clone, values)
	return clone
}

func clonePortList(values PortList) PortList {
	if len(values) == 0 {
		return nil
	}
	clone := make(PortList, len(values))
	copy(clone, values)
	return clone
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	clone := make([]string, len(values))
	copy(clone, values)
	return clone
}

func cloneMounts(values []MountSpec) []MountSpec {
	if len(values) == 0 {
		return nil
	}
	clone := make([]MountSpec, len(values))
	copy(clone, values)
	return clone
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func mergeFeatureSet(base, overlay FeatureSet) FeatureSet {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := make(FeatureSet, len(base)+len(overlay))
	for key, options := range base {
		merged[key] = cloneFeatureOptions(options)
	}
	for key, options := range overlay {
		if existing, ok := merged[key]; ok {
			merged[key] = mergeFeatureOptions(existing, options)
			continue
		}
		merged[key] = cloneFeatureOptions(options)
	}
	return merged
}

func cloneFeatureSet(features FeatureSet) FeatureSet {
	if len(features) == 0 {
		return nil
	}
	clone := make(FeatureSet, len(features))
	for key, options := range features {
		clone[key] = cloneFeatureOptions(options)
	}
	return clone
}

func mergeFeatureOptions(base, overlay FeatureOptions) FeatureOptions {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := cloneFeatureOptions(base)
	if merged == nil {
		merged = make(FeatureOptions, len(overlay))
	}
	for key, value := range overlay {
		merged[key] = cloneFeatureOptionValue(value)
	}
	return merged
}

func cloneFeatureOptions(options FeatureOptions) FeatureOptions {
	if len(options) == 0 {
		return nil
	}
	clone := make(FeatureOptions, len(options))
	for key, value := range options {
		clone[key] = cloneFeatureOptionValue(value)
	}
	return clone
}

func cloneFeatureOptionValue(value FeatureOptionValue) FeatureOptionValue {
	clone := FeatureOptionValue{}
	if value.String != nil {
		text := *value.String
		clone.String = &text
	}
	if value.Bool != nil {
		flag := *value.Bool
		clone.Bool = &flag
	}
	return clone
}

func cloneLifecycleCommands(commands *LifecycleCommands) *LifecycleCommands {
	if commands == nil {
		return nil
	}
	clone := &LifecycleCommands{}
	if commands.Single != nil {
		single := *commands.Single
		clone.Single = &single
	}
	if len(commands.Parallel) > 0 {
		clone.Parallel = make([]NamedLifecycleCommand, len(commands.Parallel))
		copy(clone.Parallel, commands.Parallel)
		for idx := range clone.Parallel {
			if len(clone.Parallel[idx].Command.Exec) > 0 {
				exec := make([]string, len(clone.Parallel[idx].Command.Exec))
				copy(exec, clone.Parallel[idx].Command.Exec)
				clone.Parallel[idx].Command.Exec = exec
			}
		}
	}
	return clone
}
