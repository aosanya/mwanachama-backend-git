package mwanachamagit

import "regexp"

var (
	shaPattern     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	refNamePattern = regexp.MustCompile(`^[^\x00-\x20~^:?*\[\\]+$`)
)

func isSHA(s string) bool { return shaPattern.MatchString(s) }

func isRefName(s string) bool {
	return refNamePattern.MatchString(s) &&
		!regexp.MustCompile(`(^/|/$|//|\.\.|@\{|\.lock$|^\.|/\.)`).MatchString(s)
}

var patterns = map[string]func(string) bool{
	"sha":      isSHA,
	"ref_name": isRefName,
}
