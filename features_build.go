package devcontainer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/client"
)

const featureImageBaseDir = "/usr/local/share/devcontainer/features"

func buildFeaturesImage(ctx context.Context, cli *client.Client, baseImage, baseUser, workspaceRoot, devcontainerID string, cfg *DevcontainerConfig, features []*ResolvedFeature, vars map[string]string) (string, error) {
	if len(features) == 0 {
		return baseImage, nil
	}
	contextDir, err := os.MkdirTemp("", "godev-features-build-*")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = os.RemoveAll(contextDir)
	}()
	featuresDir := filepath.Join(contextDir, "features")
	if err := os.MkdirAll(featuresDir, 0o755); err != nil {
		return "", err
	}
	extraEnv := featureUserEnv(cfg, baseUser)
	for idx, feature := range features {
		dirName := fmt.Sprintf("%02d-%s", idx+1, sanitizeName(feature.Metadata.ID))
		source := feature.FeatureDir
		dest := filepath.Join(featuresDir, dirName)
		if err := copyDir(source, dest); err != nil {
			return "", err
		}
		feature.ImageDir = path.Join(featureImageBaseDir, dirName)
		entrypoint, err := featureEntrypointPath(feature, vars)
		if err != nil {
			return "", err
		}
		if entrypoint != "" && !strings.HasPrefix(entrypoint, feature.ImageDir) {
			return "", fmt.Errorf("feature entrypoint must be under %s", feature.ImageDir)
		}
		envFile := renderFeatureEnvFile(feature.Options.Values, extraEnv)
		if err := os.WriteFile(filepath.Join(dest, "devcontainer-features.env"), []byte(envFile), 0o644); err != nil {
			return "", err
		}
	}
	dockerfile := buildFeaturesDockerfile(baseImage, baseUser, features, vars)
	if err := os.WriteFile(filepath.Join(contextDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return "", err
	}
	buildContext, err := tarDirectory(contextDir)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = buildContext.Close()
	}()
	tag := featuresImageTag(workspaceRoot, devcontainerID, features)
	resp, err := cli.ImageBuild(ctx, buildContext, build.ImageBuildOptions{
		Dockerfile: "Dockerfile",
		Tags:       []string{tag},
		Remove:     true,
	})
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return "", err
	}
	return tag, nil
}

func buildFeaturesDockerfile(baseImage, baseUser string, features []*ResolvedFeature, vars map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "FROM %s\n", baseImage)
	b.WriteString("USER root\n")
	b.WriteString("WORKDIR /\n")
	fmt.Fprintf(&b, "COPY features/ %s/\n", featureImageBaseDir)
	for _, feature := range features {
		command := featureInstallCommand(feature, vars)
		fmt.Fprintf(&b, "RUN %s\n", command)
	}
	if baseUser != "" && baseUser != "root" {
		fmt.Fprintf(&b, "USER %s\n", baseUser)
	}
	return b.String()
}

func featureInstallCommand(feature *ResolvedFeature, vars map[string]string) string {
	entrypoint, _ := featureEntrypointPath(feature, vars)
	entrypointCommand := ""
	if entrypoint != "" {
		entrypointCommand = fmt.Sprintf("chmod +x %s && ", entrypoint)
	}
	return fmt.Sprintf("set -e; cd %s; chmod +x install.sh; set -a; . ./devcontainer-features.env; set +a; %s./install.sh", feature.ImageDir, entrypointCommand)
}

func featuresImageTag(workspaceRoot, devcontainerID string, features []*ResolvedFeature) string {
	base := sanitizeName(filepath.Base(workspaceRoot))
	if base == "" {
		base = "devcontainer"
	}
	hashInput := make([]string, 0, len(features))
	for _, feature := range features {
		hashInput = append(hashInput, feature.DependencyKey)
	}
	seed := strings.Join(hashInput, ",")
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("godev-%s-%s-features-%s:latest", base, devcontainerID, hex.EncodeToString(sum[:8]))
}

func featureUserEnv(cfg *DevcontainerConfig, baseUser string) map[string]string {
	containerUser := cfg.ContainerUser
	if containerUser == "" {
		containerUser = baseUser
	}
	if containerUser == "" {
		containerUser = "root"
	}
	remoteUser := cfg.RemoteUser
	if remoteUser == "" {
		remoteUser = containerUser
	}
	containerHome := resolveUserHome(containerUser)
	remoteHome := resolveUserHome(remoteUser)
	return map[string]string{
		"_CONTAINER_USER":      containerUser,
		"_REMOTE_USER":         remoteUser,
		"_CONTAINER_USER_HOME": containerHome,
		"_REMOTE_USER_HOME":    remoteHome,
	}
}

func resolveUserHome(user string) string {
	user = strings.TrimSpace(user)
	if user == "" || user == "root" || user == "0" {
		return "/root"
	}
	if strings.Contains(user, ":") {
		user = strings.SplitN(user, ":", 2)[0]
	}
	return "/home/" + user
}

func copyDir(source, dest string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			_ = srcFile.Close()
			return err
		}
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			_ = dstFile.Close()
			_ = srcFile.Close()
			return err
		}
		if err := dstFile.Close(); err != nil {
			_ = srcFile.Close()
			return err
		}
		return srcFile.Close()
	})
}

func imageDefaultUser(ctx context.Context, cli *client.Client, imageRef string) (string, error) {
	inspect, err := cli.ImageInspect(ctx, imageRef)
	if err != nil {
		return "", err
	}
	if inspect.Config == nil {
		return "", nil
	}
	return inspect.Config.User, nil
}
