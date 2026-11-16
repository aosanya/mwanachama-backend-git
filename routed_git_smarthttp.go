package mwanachamagit

import (
	"errors"
	"net/http"
	"strings"
)

func (g *RoutedGitManager) SmartHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, rest, ok := splitRepositoryPath(r.URL.Path)
		if !ok {
			http.Error(w, "invalid repository path", http.StatusBadRequest)
			return
		}
		err := g.named(r.Context(), name)(func(file *RepoFile) error {
			inner := r.Clone(r.Context())
			inner.URL.Path = rest
			file.GitHTTPHandler().ServeHTTP(w, inner)
			return nil
		})
		if errors.Is(err, ErrRepoNotInitialised) || errors.Is(err, ErrRepositoryNotRegistered) {
			http.Error(w, "repository not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "internal error: "+err.Error(), http.StatusInternalServerError)
		}
	})
}

func splitRepositoryPath(path string) (name, rest string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/")
	index := strings.Index(trimmed, "/")
	if index < 0 {
		return "", "", false
	}
	name = strings.TrimSuffix(trimmed[:index], ".git")
	rest = trimmed[index:]
	return name, rest, name != "" && rest != "/"
}

var _ GitManager = (*RoutedGitManager)(nil)
