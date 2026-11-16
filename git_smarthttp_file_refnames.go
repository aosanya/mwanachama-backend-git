package mwanachamagit

import (
	"sort"

	gogitplumbing "github.com/go-git/go-git/v5/plumbing"
)

const (
	refusedInvalidBranchName = "not a valid branch name"
	refusedBranchClash       = "clashes with the existing branch "
)

type branchNamespace struct {
	branches    map[string]bool
	directories map[string]string
}

func newBranchNamespace() *branchNamespace {
	return &branchNamespace{branches: map[string]bool{}, directories: map[string]string{}}
}

func parentDirectories(name string) []string {
	var parents []string
	for i := 0; i < len(name); i++ {
		if name[i] == '/' {
			parents = append(parents, name[:i])
		}
	}
	return parents
}

func (n *branchNamespace) add(name string) {
	n.branches[name] = true
	for _, parent := range parentDirectories(name) {
		n.directories[parent] = name
	}
}

func (n *branchNamespace) clashWith(name string) (string, bool) {
	for _, parent := range parentDirectories(name) {
		if n.branches[parent] {
			return parent, true
		}
	}
	if under, isDirectory := n.directories[name]; isDirectory {
		return under, true
	}
	return "", false
}

func isValidBranchName(name string) bool {
	return gogitplumbing.NewBranchReferenceName(name).Validate() == nil
}

func usableBranchNames(defaultBranch string, names []string) []string {
	ordered := append([]string(nil), names...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if (ordered[i] == defaultBranch) != (ordered[j] == defaultBranch) {
			return ordered[i] == defaultBranch
		}
		return ordered[i] < ordered[j]
	})
	space := newBranchNamespace()
	var usable []string
	for _, name := range ordered {
		if !isValidBranchName(name) {
			continue
		}
		if _, clash := space.clashWith(name); clash {
			continue
		}
		space.add(name)
		usable = append(usable, name)
	}
	return usable
}

func branchNameRefusal(space *branchNamespace, name string) (string, bool) {
	if !isValidBranchName(name) {
		return refusedInvalidBranchName, true
	}
	if existing, clash := space.clashWith(name); clash {
		return refusedBranchClash + existing, true
	}
	return "", false
}

func (s *fileSnapshot) branchNames() []string {
	var names []string
	refs, err := s.storage.IterReferences()
	if err != nil {
		return nil
	}
	defer refs.Close()
	_ = refs.ForEach(func(ref *gogitplumbing.Reference) error {
		if ref.Name().IsBranch() {
			names = append(names, ref.Name().Short())
		}
		return nil
	})
	return names
}
