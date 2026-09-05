package lint

import (
	"testing"
	"time"

	"github.com/frankbesch/memlint/internal/config"
)

func runStamps(root string, cfg *config.Stamps) Result {
	return Run(root, &config.Config{Stamps: cfg})
}

// A "last verified" stamp must be within max_age_days of the file's last
// commit: a file that changed after its stamp is stale evidence.
func TestStamps(t *testing.T) {
	requireGit(t)
	today := time.Now().UTC().Format("2006-01-02")
	old := time.Now().UTC().AddDate(0, 0, -40).Format("2006-01-02")
	root := newRepo(t, map[string]string{
		"fresh.md":   "# a\nLast verified: " + today + "\n",
		"stale.md":   "# b\nlast verified " + old + "\n",
		"nostamp.md": "# c\nno stamp here\n",
	})
	cfg := &config.Stamps{Files: []string{"fresh.md", "stale.md", "nostamp.md"}, MaxAgeDays: 30}
	res := runStamps(root, cfg)
	wantCounts(t, res, 1, 1)
	if !hasFinding(res, "stamps", "stale.md", "stamp "+old+" is") {
		t.Errorf("stale stamp must be YELLOW stamps/stale:\n%s", dump(res))
	}
	if !hasFinding(res, "stamps", "nostamp.md", "no stamp") {
		t.Errorf("missing stamp must be RED stamps/missing:\n%s", dump(res))
	}
	if f := findingAt(res, "stale.md"); f.Line != 2 || f.Code != "stamps/stale" {
		t.Errorf("stale finding should sit on the stamp line, got %+v", f)
	}
}

// Age is measured against the last commit touching the file, not today:
// an old note with an equally old stamp is not stale.
func TestStampsAgeIsRelativeToLastCommit(t *testing.T) {
	requireGit(t)
	root := writeTree(t, map[string]string{"old.md": "Last verified: 2024-01-10\n"})
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "add", "-A")
	cmd := []string{"-c", "commit.gpgsign=false", "commit", "-q", "-m", "old", "--date", "2024-01-20T00:00:00Z"}
	git(t, root, cmd...)
	wantCounts(t, runStamps(root, &config.Stamps{Files: []string{"old.md"}, MaxAgeDays: 30}), 0, 0)

	// Uncommitted edits count as "now": the file changed after its stamp.
	writeFile(t, root, "old.md", "Last verified: 2024-01-10\nedited today\n")
	res := runStamps(root, &config.Stamps{Files: []string{"old.md"}, MaxAgeDays: 30})
	wantCounts(t, res, 0, 1)
}

func TestStampsNoHistoryIsYellow(t *testing.T) {
	requireGit(t)
	root := writeTree(t, map[string]string{"a.md": "Last verified: 2020-01-01\n"})
	res := runStamps(root, &config.Stamps{Files: []string{"a.md"}, MaxAgeDays: 30})
	wantCounts(t, res, 0, 1)
	wantMessage(t, res, "no git history")
}

func findingAt(res Result, path string) Finding {
	for _, f := range res.Findings {
		if f.Path == path {
			return f
		}
	}
	return Finding{}
}
