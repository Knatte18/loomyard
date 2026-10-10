package dotgit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func mkdirs(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirs(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// plainClone lays out a `.git` directory under dir with HEAD, objects and refs.
func plainClone(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, ".git", "HEAD"), "ref: refs/heads/main\n")
	mkdirs(t, filepath.Join(dir, ".git", "objects"), filepath.Join(dir, ".git", "refs"))
}

// linkedWorktree lays out a worktree at dir whose gitfile holds gitdirLine, with an admin dir at adminDir that has a HEAD and a commondir file holding commondirLine.
func linkedWorktree(t *testing.T, dir, gitdirLine, adminDir, commondirLine string) {
	t.Helper()
	mkdirs(t, dir)
	writeFile(t, filepath.Join(dir, ".git"), "gitdir: "+gitdirLine+"\n")
	writeFile(t, filepath.Join(adminDir, "HEAD"), "ref: refs/heads/task\n")
	writeFile(t, filepath.Join(adminDir, "commondir"), commondirLine+"\n")
}

func TestRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		build          func(t *testing.T, root string) (dir string)
		wantGitDir     func(root string) string
		wantCommonDir  func(root string) string
		wantRepository bool
		wantNotRepo    bool
	}{
		{
			name: "git directory with HEAD objects and refs",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "repo"))
				return filepath.Join(root, "repo")
			},
			wantGitDir:     func(root string) string { return filepath.Join(root, "repo", ".git") },
			wantCommonDir:  func(root string) string { return filepath.Join(root, "repo", ".git") },
			wantRepository: true,
		},
		{
			name: "git directory without HEAD",
			build: func(t *testing.T, root string) string {
				dir := filepath.Join(root, "repo")
				mkdirs(t, filepath.Join(dir, ".git", "objects"), filepath.Join(dir, ".git", "refs"))
				return dir
			},
			wantGitDir:    func(root string) string { return filepath.Join(root, "repo", ".git") },
			wantCommonDir: func(root string) string { return filepath.Join(root, "repo", ".git") },
		},
		{
			name: "git directory without objects",
			build: func(t *testing.T, root string) string {
				dir := filepath.Join(root, "repo")
				writeFile(t, filepath.Join(dir, ".git", "HEAD"), "ref: refs/heads/main\n")
				mkdirs(t, filepath.Join(dir, ".git", "refs"))
				return dir
			},
			wantGitDir:    func(root string) string { return filepath.Join(root, "repo", ".git") },
			wantCommonDir: func(root string) string { return filepath.Join(root, "repo", ".git") },
		},
		{
			name: "git directory without refs",
			build: func(t *testing.T, root string) string {
				dir := filepath.Join(root, "repo")
				writeFile(t, filepath.Join(dir, ".git", "HEAD"), "ref: refs/heads/main\n")
				mkdirs(t, filepath.Join(dir, ".git", "objects"))
				return dir
			},
			wantGitDir:    func(root string) string { return filepath.Join(root, "repo", ".git") },
			wantCommonDir: func(root string) string { return filepath.Join(root, "repo", ".git") },
		},
		{
			name: "gitfile with absolute target and absolute commondir",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "main"))
				admin := filepath.Join(root, "main", ".git", "worktrees", "task")
				linkedWorktree(t, filepath.Join(root, "task"), admin, admin, filepath.Join(root, "main", ".git"))
				return filepath.Join(root, "task")
			},
			wantGitDir:     func(root string) string { return filepath.Join(root, "main", ".git", "worktrees", "task") },
			wantCommonDir:  func(root string) string { return filepath.Join(root, "main", ".git") },
			wantRepository: true,
		},
		{
			name: "gitfile with relative target and relative commondir",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "main"))
				admin := filepath.Join(root, "main", ".git", "worktrees", "task")
				linkedWorktree(t, filepath.Join(root, "task"), "../main/.git/worktrees/task", admin, "../..")
				return filepath.Join(root, "task")
			},
			wantGitDir:     func(root string) string { return filepath.Join(root, "main", ".git", "worktrees", "task") },
			wantCommonDir:  func(root string) string { return filepath.Join(root, "main", ".git") },
			wantRepository: true,
		},
		{
			name: "gitfile target missing",
			build: func(t *testing.T, root string) string {
				dir := filepath.Join(root, "task")
				writeFile(t, filepath.Join(dir, ".git"), "gitdir: "+filepath.Join(root, "gone")+"\n")
				return dir
			},
			wantNotRepo: true,
		},
		{
			name: "gitfile without a gitdir line",
			build: func(t *testing.T, root string) string {
				dir := filepath.Join(root, "task")
				writeFile(t, filepath.Join(dir, ".git"), "nonsense\n")
				return dir
			},
			wantNotRepo: true,
		},
		{
			name: "commondir target missing",
			build: func(t *testing.T, root string) string {
				admin := filepath.Join(root, "main", ".git", "worktrees", "task")
				linkedWorktree(t, filepath.Join(root, "task"), admin, admin, filepath.Join(root, "gone"))
				return filepath.Join(root, "task")
			},
			wantNotRepo: true,
		},
		{
			name: "no .git entry",
			build: func(t *testing.T, root string) string {
				mkdirs(t, filepath.Join(root, "plain"))
				return filepath.Join(root, "plain")
			},
			wantNotRepo: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			dir := tt.build(t, root)

			got, err := Read(dir)
			if tt.wantNotRepo {
				if !errors.Is(err, ErrNotRepository) {
					t.Fatalf("Read(%s) error = %v, want ErrNotRepository", dir, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Read(%s): %v", dir, err)
			}
			want := Geometry{GitDir: tt.wantGitDir(root), CommonDir: tt.wantCommonDir(root)}
			if got != want {
				t.Fatalf("Read(%s) = %+v, want %+v", dir, got, want)
			}
			if IsRepository(got) != tt.wantRepository {
				t.Fatalf("IsRepository(%+v) = %v, want %v", got, !tt.wantRepository, tt.wantRepository)
			}
		})
	}
}

func TestFindRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		build       func(t *testing.T, root string) (start string)
		wantRoot    func(root string) string
		wantNotRepo bool
	}{
		{
			name: "the root itself",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "repo"))
				return filepath.Join(root, "repo")
			},
			wantRoot: func(root string) string { return filepath.Join(root, "repo") },
		},
		{
			name: "a nested directory resolves to the root above",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "repo"))
				mkdirs(t, filepath.Join(root, "repo", "a", "b"))
				return filepath.Join(root, "repo", "a", "b")
			},
			wantRoot: func(root string) string { return filepath.Join(root, "repo") },
		},
		{
			name: "an empty .git directory inside a repository resolves outward",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "repo"))
				mkdirs(t, filepath.Join(root, "repo", "sub", ".git"))
				return filepath.Join(root, "repo", "sub")
			},
			wantRoot: func(root string) string { return filepath.Join(root, "repo") },
		},
		{
			name: "a linked worktree is its own root",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "main"))
				admin := filepath.Join(root, "main", ".git", "worktrees", "task")
				linkedWorktree(t, filepath.Join(root, "main", "task"), admin, admin, filepath.Join(root, "main", ".git"))
				mkdirs(t, filepath.Join(root, "main", "task", "src"))
				return filepath.Join(root, "main", "task", "src")
			},
			wantRoot: func(root string) string { return filepath.Join(root, "main", "task") },
		},
		{
			name: "a gitfile with a missing target inside a repository is refused",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "repo"))
				writeFile(t, filepath.Join(root, "repo", "pruned", ".git"), "gitdir: "+filepath.Join(root, "gone")+"\n")
				return filepath.Join(root, "repo", "pruned")
			},
			wantNotRepo: true,
		},
		{
			name: "a gitfile whose target is no repository is refused",
			build: func(t *testing.T, root string) string {
				plainClone(t, filepath.Join(root, "repo"))
				mkdirs(t, filepath.Join(root, "empty-admin"))
				writeFile(t, filepath.Join(root, "repo", "broken", ".git"), "gitdir: "+filepath.Join(root, "empty-admin")+"\n")
				return filepath.Join(root, "repo", "broken")
			},
			wantNotRepo: true,
		},
		{
			name: "a directory under no repository",
			build: func(t *testing.T, root string) string {
				mkdirs(t, filepath.Join(root, "plain", "deep"))
				return filepath.Join(root, "plain", "deep")
			},
			wantNotRepo: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			start := tt.build(t, root)

			gotRoot, gotGeometry, err := FindRoot(start)
			if tt.wantNotRepo {
				if !errors.Is(err, ErrNotRepository) {
					t.Fatalf("FindRoot(%s) error = %v, want ErrNotRepository", start, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindRoot(%s): %v", start, err)
			}
			if want := tt.wantRoot(root); gotRoot != want {
				t.Fatalf("FindRoot(%s) root = %s, want %s", start, gotRoot, want)
			}
			if !IsRepository(gotGeometry) {
				t.Fatalf("FindRoot(%s) geometry %+v is not a repository", start, gotGeometry)
			}
		})
	}
}
