package hqlint

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var (
	areas        = []string{"00-GENESIS", "01-RESEARCH", "02-DESIGN", "03-IMPLEMENTATION", "04-JOURNEY"}
	genesisFiles = []string{"README.md", "vision.md", "constitution.md", "how-we-work.md"}
	legalStates  = map[string]bool{"active": true, "graduated": true, "abandoned": true}
	terminal     = map[string]bool{"graduated": true, "abandoned": true}

	episodeRe = regexp.MustCompile(`^\d{4}-[a-z0-9-]+\.md$`)
	stateRe   = regexp.MustCompile(`(?m)^\*\*State:\*\* *(\S+)`)
	linkRe    = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
)

// repoRoot walks up from this source file until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(self)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found walking up from %s", filepath.Dir(self))
		}
		dir = parent
	}
}

func hqDir(t *testing.T) string { return filepath.Join(repoRoot(t), "hq") }

func mustExist(t *testing.T, path, msg string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s (%s)", msg, path)
	}
}

// episodes returns the numbered-episode files in hq/04-JOURNEY (README.md and
// TEMPLATE.md excluded), sorted by name.
func episodes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(hqDir(t), "04-JOURNEY"))
	if err != nil {
		t.Fatalf("reading 04-JOURNEY: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == "README.md" || e.Name() == "TEMPLATE.md" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func TestAreasExistWithReadmes(t *testing.T) {
	hq := hqDir(t)
	mustExist(t, filepath.Join(hq, "README.md"), "missing hq/README.md")
	for _, a := range areas {
		mustExist(t, filepath.Join(hq, a, "README.md"), "missing area README")
	}
	for _, f := range genesisFiles {
		mustExist(t, filepath.Join(hq, "00-GENESIS", f), "missing GENESIS file")
	}
	mustExist(t, filepath.Join(hq, "01-RESEARCH", "TEMPLATE.md"), "missing 01-RESEARCH/TEMPLATE.md")
	mustExist(t, filepath.Join(hq, "04-JOURNEY", "TEMPLATE.md"), "missing 04-JOURNEY/TEMPLATE.md")
}

func TestResearchTopicsHaveLegalNonterminalStates(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join(hqDir(t), "01-RESEARCH"))
	if err != nil {
		t.Fatalf("reading 01-RESEARCH: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		readme := filepath.Join(hqDir(t), "01-RESEARCH", e.Name(), "README.md")
		data, err := os.ReadFile(readme)
		if err != nil {
			t.Errorf("%s: research topic without README.md", e.Name())
			continue
		}
		text := string(data)
		if !strings.HasPrefix(strings.TrimSpace(text), "# ") {
			t.Errorf("%s: README lacks a title", e.Name())
		}
		if !strings.Contains(text, "## Abstract") {
			t.Errorf("%s: README lacks an Abstract section", e.Name())
		}
		m := stateRe.FindStringSubmatch(text)
		if m == nil {
			t.Errorf("%s: README lacks a '**State:** ...' line", e.Name())
			continue
		}
		state := m[1]
		if !legalStates[state] {
			t.Errorf("%s: illegal state %q", e.Name(), state)
		}
		if terminal[state] {
			t.Errorf("%s: state %q is terminal but the folder lingers — "+
				"/research-graduate removes the topic folder on every outcome", e.Name(), state)
		}
	}
}

func TestJourneyEpisodesNumberedContiguously(t *testing.T) {
	names := episodes(t)
	var nums []int
	for _, n := range names {
		if !episodeRe.MatchString(n) {
			t.Errorf("file in hq/04-JOURNEY is not an NNNN-slug.md episode: %s", n)
			continue
		}
		num, _ := strconv.Atoi(n[:4])
		nums = append(nums, num)
	}
	seen := map[int]bool{}
	for _, n := range nums {
		if seen[n] {
			t.Errorf("duplicate episode number: %04d", n)
		}
		seen[n] = true
	}
	sort.Ints(nums)
	for i, n := range nums {
		if n != i+1 {
			t.Errorf("episode numbers not contiguous from 0001: got %v", nums)
			break
		}
	}
}

func TestJourneyEpisodesAreIndexed(t *testing.T) {
	index, err := os.ReadFile(filepath.Join(hqDir(t), "04-JOURNEY", "README.md"))
	if err != nil {
		t.Fatalf("reading 04-JOURNEY/README.md: %v", err)
	}
	for _, n := range episodes(t) {
		if !strings.Contains(string(index), n) {
			t.Errorf("episode missing from the hq/04-JOURNEY/README.md index: %s", n)
		}
	}
}

func TestEpisodesRecordReversalCondition(t *testing.T) {
	for _, n := range episodes(t) {
		data, err := os.ReadFile(filepath.Join(hqDir(t), "04-JOURNEY", n))
		if err != nil {
			t.Errorf("reading episode %s: %v", n, err)
			continue
		}
		if !strings.Contains(string(data), "Reversal condition:") {
			t.Errorf("episode without the required 'Reversal condition:' line: %s "+
				"(see hq/04-JOURNEY/TEMPLATE.md)", n)
		}
	}
}

func TestConstitutionSymlinkResolvesToGenesis(t *testing.T) {
	root := repoRoot(t)
	link := filepath.Join(root, ".specify", "memory", "constitution.md")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf(".specify/memory/constitution.md: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal(".specify/memory/constitution.md must be a symlink into GENESIS")
	}
	canonical := filepath.Join(root, "hq", "00-GENESIS", "constitution.md")
	gotResolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatalf("dangling symlink — speckit would re-copy its template over it: %v", err)
	}
	wantResolved, err := filepath.EvalSymlinks(canonical)
	if err != nil {
		t.Fatalf("canonical constitution missing: %v", err)
	}
	if gotResolved != wantResolved {
		t.Fatalf("symlink resolves to %s, not %s", gotResolved, wantResolved)
	}
	data, err := os.ReadFile(link)
	if err != nil {
		t.Fatalf("reading constitution through symlink: %v", err)
	}
	if !strings.Contains(string(data), "# Imp Framework Constitution") {
		t.Error("constitution missing its '# Imp Framework Constitution' title")
	}
}

func TestHqRelativeLinksResolve(t *testing.T) {
	hq := hqDir(t)
	var broken []string
	err := filepath.WalkDir(hq, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range linkRe.FindAllStringSubmatch(string(data), -1) {
			target := m[1]
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
				strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
				continue
			}
			rel := target
			if i := strings.Index(rel, "#"); i >= 0 {
				rel = rel[:i]
			}
			if rel == "" {
				continue
			}
			if _, statErr := os.Stat(filepath.Join(filepath.Dir(path), rel)); statErr != nil {
				broken = append(broken, filepath.Base(path)+" -> "+target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking hq/: %v", err)
	}
	if len(broken) > 0 {
		t.Errorf("broken relative markdown links inside hq/:\n%s", strings.Join(broken, "\n"))
	}
}
