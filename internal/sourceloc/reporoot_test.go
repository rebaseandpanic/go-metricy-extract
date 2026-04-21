package sourceloc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRepoRoot_FindsGitDir(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	sub := filepath.Join(project, "sub")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := ResolveRepoRoot(sub)
	if got != project {
		t.Errorf("got %q, want %q", got, project)
	}
}

func TestResolveRepoRoot_FindsGoMod(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	sub := filepath.Join(project, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	got := ResolveRepoRoot(sub)
	if got != project {
		t.Errorf("got %q, want %q", got, project)
	}
}

func TestResolveRepoRoot_PrefersGitOverGoMod(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	got := ResolveRepoRoot(project)
	if got != project {
		t.Errorf("got %q, want %q", got, project)
	}
}

// TestResolveRepoRoot_GitOuterWinsOverGoModInner pins the "outer VCS root
// beats nearest module" behaviour that the two-pass algorithm produces.
// Structure: repo/.git, repo/service/go.mod, repo/service/cmd. Starting
// from cmd, .git at repo is found during the first sweep, so we return
// repo even though go.mod sits closer.
func TestResolveRepoRoot_GitOuterWinsOverGoModInner(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	svc := filepath.Join(repo, "service")
	cmd := filepath.Join(svc, "cmd")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(cmd, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := ResolveRepoRoot(cmd)
	if got != repo {
		t.Errorf("got %q, want %q (outer .git should win)", got, repo)
	}
}

func TestResolveRepoRoot_NoMarkersReturnsStart(t *testing.T) {
	root := t.TempDir()
	random := filepath.Join(root, "random")
	if err := os.MkdirAll(random, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := ResolveRepoRoot(random)
	if got != random {
		t.Errorf("got %q, want %q", got, random)
	}
}

func TestMakeRelative_InsideRoot(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	path := filepath.Join(project, "sub", "file.go")
	got := MakeRelative(path, project)
	want := "sub/file.go"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMakeRelative_OutsideRoot(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	outside := filepath.Join(root, "other", "file.go")
	got := MakeRelative(outside, project)
	if got != outside {
		t.Errorf("got %q, want absolute passthrough %q", got, outside)
	}
}

func TestMakeRelative_EqualsRoot(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := MakeRelative(project, project)
	// filepath.Rel(x, x) == "." — we preserve that. Pinning here so future
	// refactors can't silently drift to "" or the absolute path.
	if got != "." {
		t.Errorf("got %q, want %q", got, ".")
	}
}

func TestMakeRelative_EmptyRepoRootPassesThrough(t *testing.T) {
	got := MakeRelative("/abs/path/file.go", "")
	if got != "/abs/path/file.go" {
		t.Errorf("got %q, want unchanged absolute path", got)
	}
}

// TestResolveRepoRoot_FindsGitFile pins the submodule / git-worktree layout
// where .git is a *file* (containing "gitdir: ...") rather than a directory.
// walkUpFor uses os.Lstat, which treats any entry name match as a hit — a
// regression to os.Stat + IsDir() would break submodule monorepos silently.
func TestResolveRepoRoot_FindsGitFile(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	sub := filepath.Join(project, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// .git as a plain file — the submodule / worktree pattern.
	if err := os.WriteFile(filepath.Join(project, ".git"), []byte("gitdir: ../.git/modules/project\n"), 0o644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
	got := ResolveRepoRoot(sub)
	if got != project {
		t.Errorf("got %q, want %q (.git as a file should still mark the repo root)", got, project)
	}
}

// TestResolveRepoRoot_StartIsRepoRoot pins that a start directory which
// itself contains the .git marker resolves to itself — not to its parent.
// A regression that lifts to filepath.Dir(cur) *before* the Lstat check
// would return the parent; this test catches that drift.
func TestResolveRepoRoot_StartIsRepoRoot(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got := ResolveRepoRoot(project)
	if got != project {
		t.Errorf("got %q, want %q (start dir with .git should resolve to self)", got, project)
	}
}

// TestMakeRelative_ParentEscape pins the guard on reporoot.go: when
// filepath.Rel returns exactly "..", the path is the parent of repoRoot —
// which is outside and must pass through untouched. A brittle prefix check
// like strings.HasPrefix(rel, "..") (no separator) would miss this exact-
// match case if the later separator-based check were ever removed.
func TestMakeRelative_ParentEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo", "child")
	// MakeRelative(root-of-"repo/child", "repo") → rel == "..", must passthrough.
	parent := filepath.Dir(root)
	got := MakeRelative(parent, root)
	if got != parent {
		t.Errorf("got %q, want passthrough %q (rel==\"..\" is an escape)", got, parent)
	}
}
