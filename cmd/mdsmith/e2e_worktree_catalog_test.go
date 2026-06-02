package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitInRepo runs a git command inside dir with a fixed identity so the
// test does not depend on the host's global git config.
func gitInRepo(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{
		"-C", dir,
		"-c", "user.email=test@example.com",
		"-c", "user.name=mdsmith-test",
		"-c", "commit.gpgsign=false",
		"-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

// TestE2E_Fix_PreservesCatalogInIgnoredWorktree is the regression test
// for the worktree catalog-emptying bug.
//
// It builds a real Git superproject whose .gitignore and .mdsmith.yml
// both ignore ".claude/worktrees/**", then adds a linked worktree under
// that ignored path (mirroring how the agent harness lays out
// .claude/worktrees/agent-x). Running `fix` and `check` from inside the
// worktree on its own PLAN.md must NOT empty the populated
// `<?catalog?>` over plan/*.md: the worktree is its own working tree, so
// the superproject's ignore rule must not classify the worktree's files
// as ignored.
//
// Before the fix, the catalog glob resolved to zero files and `fix`
// rewrote the section body to empty; this test asserts the rows survive.
func TestE2E_Fix_PreservesCatalogInIgnoredWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()

	// Superproject config: ignore the worktrees directory both in git and
	// in mdsmith's own ignore list, exactly as the real repo does.
	writeFixture(t, repo, ".gitignore", ".claude/worktrees/\n")
	writeFixture(t, repo, ".mdsmith.yml",
		"rules:\n  first-line-heading: false\n  line-length: false\n"+
			"ignore:\n  - \".claude/worktrees/**\"\n")

	// A small plan directory and an index file whose catalog enumerates it.
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "plan"), 0o755))
	writeFixture(t, repo, "plan/alpha.md", "# Alpha\n\nFirst plan.\n")
	writeFixture(t, repo, "plan/beta.md", "# Beta\n\nSecond plan.\n")
	planSrc := "<?catalog\nglob: \"plan/*.md\"\nrow: \"- [{filename}]({filename})\"\n?>\n" +
		"- [plan/alpha.md](plan/alpha.md)\n- [plan/beta.md](plan/beta.md)\n<?/catalog?>\n"
	writeFixture(t, repo, "PLAN.md", planSrc)

	// Make it a real Git working tree and commit, so a linked worktree
	// can be added (git worktree requires at least one commit).
	gitInRepo(t, repo, "init")
	gitInRepo(t, repo, "add", "-A")
	gitInRepo(t, repo, "commit", "-m", "initial")

	// Add a linked worktree under the ignored .claude/worktrees/ path.
	// Its absolute path therefore contains the ".claude/worktrees"
	// segment the superproject's .gitignore matches.
	worktree := filepath.Join(repo, ".claude", "worktrees", "agent-x")
	gitInRepo(t, repo, "worktree", "add", "--detach", worktree, "HEAD")

	// Sanity: the worktree boundary marker is a .git FILE, not a dir.
	gitInfo, err := os.Lstat(filepath.Join(worktree, ".git"))
	require.NoError(t, err, "worktree .git must exist")
	assert.False(t, gitInfo.IsDir(),
		"worktree .git must be a file (gitdir pointer), confirming a working-tree boundary")

	wtPlan := filepath.Join(worktree, "PLAN.md")

	// `check` from inside the worktree must not report the section as out
	// of date (it would, if the glob spuriously resolved to zero files).
	_, checkErr, checkCode := runBinaryInDir(t, worktree, "",
		"check", "--no-color", "PLAN.md")
	assert.Equal(t, 0, checkCode,
		"check from inside the worktree should pass; stderr: %s", checkErr)
	assert.NotContains(t, checkErr, "out of date",
		"catalog must not read as out of date from inside the worktree; stderr: %s", checkErr)

	// `fix` from inside the worktree must leave the populated catalog
	// intact rather than emptying it.
	_, fixErr, _ := runBinaryInDir(t, worktree, "",
		"fix", "--no-color", "PLAN.md")

	got, err := os.ReadFile(wtPlan)
	require.NoError(t, err, "reading PLAN.md after fix; fix stderr: %s", fixErr)
	gotStr := string(got)

	assert.Contains(t, gotStr, "plan/alpha.md",
		"fix emptied the catalog: alpha row missing.\nfix stderr: %s\nfile:\n%s", fixErr, gotStr)
	assert.Contains(t, gotStr, "plan/beta.md",
		"fix emptied the catalog: beta row missing.\nfix stderr: %s\nfile:\n%s", fixErr, gotStr)
	assert.Equal(t, 2, strings.Count(gotStr, "](plan/"),
		"expected both catalog rows preserved.\nfile:\n%s", gotStr)
}
