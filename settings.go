package mwanachamagit

import (
	"context"
	"fmt"
	"path"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

const (
	repositoriesSetting = "repositories"
	tagPatternsSetting  = "tag_patterns"
)

type DocumentStoreSettings interface {
	Repositories(ctx context.Context) ([]string, error)
	TagPatterns(ctx context.Context) ([]string, error)
}

func (m *gitManager) Repositories(ctx context.Context) ([]string, error) {
	return m.stringListSetting(ctx, repositoriesSetting)
}

func (m *gitManager) TagPatterns(ctx context.Context) ([]string, error) {
	return m.stringListSetting(ctx, tagPatternsSetting)
}

func (m *gitManager) stringListSetting(ctx context.Context, name string) ([]string, error) {
	s := m.store.Spec()
	if _, declared := s.Setting(name); !declared {
		return nil, nil
	}
	effective, err := spec.ResolveStored(m.db.WithContext(ctx), s, name)
	if err != nil {
		return nil, fmt.Errorf("setting %s: %w", name, err)
	}
	return ParseStringList(name, effective.Value)
}

func ParseStringList(name string, value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		if strs, isStrings := value.([]string); isStrings {
			return append([]string(nil), strs...), nil
		}
		return nil, fmt.Errorf("setting %s: %v is not a list of names", name, value)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		str, isString := item.(string)
		if !isString || str == "" {
			return nil, fmt.Errorf("setting %s: %v is not a name", name, item)
		}
		out = append(out, str)
	}
	return out, nil
}

func TagNameAllowed(patterns []string, name string) (bool, error) {
	if len(patterns) == 0 {
		return true, nil
	}
	for _, pattern := range patterns {
		matched, err := path.Match(pattern, name)
		if err != nil {
			return false, fmt.Errorf("tag pattern %q: %w", pattern, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}
