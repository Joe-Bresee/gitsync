package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
	syncerrors "github.com/joe-bresee/gitsync/internal/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FetchSource(configSync *configsv1alpha1.ConfigSync, ctx context.Context, c client.Client) (string, string, string, string, error) {
	if configSync.Spec.Source.Git == nil {
		return "", "", "", "", fmt.Errorf("only Git sources are supported; please set spec.source.git")
	}

	environment, err := ResolveEnvironment(configSync)
	if err != nil {
		return "", "", "", "", err
	}

	authSecretRef := configSync.Spec.Source.Git.AuthSecretRef
	if authSecretRef != nil && strings.TrimSpace(authSecretRef.Namespace) == "" {
		authSecretRef = &configsv1alpha1.ObjectRef{
			Name:      authSecretRef.Name,
			Namespace: configSync.Namespace,
		}
	}

	revisionSHA, sourcePath, commitMsg, err := cloneOrUpdate(
		ctx,
		c,
		configSync.Spec.Source.Git.RepoURL,
		environment.Revision,
		environment.Branch,
		configSync.Spec.Source.Git.AuthMethod,
		authSecretRef,
	)
	if err != nil {
		return "", "", "", "", syncerrors.Wrap("fetch Git source", configSync.Spec.Source.Git.RepoURL, true, err)
	}

	configuredPath := configSync.Spec.Source.Git.Path
	if environment.Overlay != "" {
		configuredPath = filepath.Join(configuredPath, environment.Overlay)
	}
	resolvedPath, err := resolveGitSourcePath(sourcePath, configuredPath)
	if err != nil {
		return "", "", "", "", err
	}

	return revisionSHA, resolvedPath, sourcePath, commitMsg, nil
}

func resolveGitSourcePath(repoPath, configuredPath string) (string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" {
		return "", fmt.Errorf("spec.source.git.path must not be empty")
	}
	if filepath.IsAbs(configuredPath) {
		return "", fmt.Errorf("spec.source.git.path must be repository-relative: %q", configuredPath)
	}

	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve repository cache path: %w", err)
	}

	cleanConfiguredPath := filepath.Clean(filepath.FromSlash(configuredPath))
	sourcePath := filepath.Join(repoPath, cleanConfiguredPath)
	rel, err := filepath.Rel(repoPath, sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve source path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("spec.source.git.path escapes the repository: %q", configuredPath)
	}

	resolvedRepoPath, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve repository cache path: %w", err)
	}
	resolvedSourcePath, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return "", fmt.Errorf("source path %q does not exist: %w", configuredPath, err)
	}
	rel, err = filepath.Rel(resolvedRepoPath, resolvedSourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve source path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("spec.source.git.path resolves outside the repository: %q", configuredPath)
	}

	return resolvedSourcePath, nil
}
