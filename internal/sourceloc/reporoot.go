package sourceloc

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveRepoRoot resolves a starting directory to the repository root.
//
// Priority:
//  1. walk upward looking for a .git directory or file → return that dir
//  2. walk upward looking for a go.mod file             → return that dir
//  3. no match                                          → return start
//
// The two lookups run as separate upward sweeps: a .git anywhere above
// start wins over a go.mod that would only be found via the second sweep.
// This matches the intent "the outer VCS root, not the nearest module."
func ResolveRepoRoot(start string) string {
	if dir, ok := walkUpFor(start, ".git"); ok {
		return dir
	}
	if dir, ok := walkUpFor(start, "go.mod"); ok {
		return dir
	}
	return start
}

// walkUpFor walks from start toward the filesystem root, returning the
// first directory that contains an entry named target (any type — file,
// directory, or symlink). The second return is false when no match is found
// before the walk reaches the filesystem root.
func walkUpFor(start, target string) (string, bool) {
	cur := start
	for {
		candidate := filepath.Join(cur, target)
		if _, err := os.Lstat(candidate); err == nil {
			return cur, true
		}
		parent := filepath.Dir(cur)
		// filepath.Dir on a root returns the root itself — this is the
		// canonical termination signal.
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

// MakeRelative returns path relative to repoRoot with forward-slash
// separators. If path is not inside repoRoot (or filepath.Rel returns an
// error), the input path is returned unchanged. An empty repoRoot is
// treated as "no remap requested" and also returns path unchanged.
//
// Returns "." when path == repoRoot (inherited from filepath.Rel); callers
// that treat the result as a file path should handle that case.
func MakeRelative(path, repoRoot string) string {
	if repoRoot == "" {
		return path
	}
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return path
	}
	// filepath.Rel happily produces "../outside/x.go" when path escapes
	// repoRoot; we treat that as "not inside" and pass through untouched.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(rel)
}
