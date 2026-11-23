package source

import (
	"fmt"
	"path/filepath"
	"strings"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
)

// EffectiveEnvironment is the resolved environment configuration used for
// fetching and rendering. Revision is kept separate because it is an explicit
// pin and takes precedence over Branch.
type EffectiveEnvironment struct {
	Name     string
	Branch   string
	Revision string
	Overlay  string
	Data     map[string]string
}

// ResolveEnvironment validates and resolves environment, branch, revision,
// and overlay precedence for a ConfigSync.
func ResolveEnvironment(configSync *configsv1alpha1.ConfigSync) (EffectiveEnvironment, error) {
	gitSource := configSync.Spec.Source.Git
	if gitSource == nil {
		return EffectiveEnvironment{}, fmt.Errorf("only Git sources are supported; please set spec.source.git")
	}

	resolved := EffectiveEnvironment{
		Revision: strings.TrimSpace(gitSource.Revision),
		Branch:   strings.TrimSpace(gitSource.Branch),
		Data:     map[string]string{},
	}
	if configSync.Spec.Rollback != nil {
		revision := strings.TrimSpace(configSync.Spec.Rollback.Revision)
		if revision == "" {
			return EffectiveEnvironment{}, fmt.Errorf("spec.rollback.revision must not be empty")
		}
		resolved.Revision = revision
	}

	if configSync.Spec.Environment != nil {
		env := configSync.Spec.Environment
		if strings.TrimSpace(env.Name) != "" {
			resolved.Name = strings.TrimSpace(env.Name)
		}
		if strings.TrimSpace(env.Branch) != "" {
			resolved.Branch = strings.TrimSpace(env.Branch)
		}
		if strings.TrimSpace(env.Overlay) != "" {
			resolved.Overlay = strings.TrimSpace(env.Overlay)
		}
		for key, value := range env.Data {
			resolved.Data[strings.TrimSpace(key)] = value
		}
	}

	if resolved.Overlay == "" {
		resolved.Overlay = ""
	} else if filepath.IsAbs(resolved.Overlay) {
		return EffectiveEnvironment{}, fmt.Errorf("environment overlay must be repository-relative: %q", resolved.Overlay)
	} else {
		cleaned := filepath.Clean(filepath.FromSlash(resolved.Overlay))
		if cleaned == "." {
			cleaned = ""
		}
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(osSeparator)) {
			return EffectiveEnvironment{}, fmt.Errorf("environment overlay escapes the repository: %q", resolved.Overlay)
		}
		resolved.Overlay = cleaned
	}

	// A revision is an explicit immutable pin. Do not also pass a branch to
	// source checkout; this makes precedence deterministic.
	if resolved.Revision != "" {
		resolved.Branch = ""
	}

	return resolved, nil
}

const osSeparator = string(filepath.Separator)
