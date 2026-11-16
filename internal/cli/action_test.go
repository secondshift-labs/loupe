package cli

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Pinning the install action's sha pins the binary only while its version default is the release it shipped in.
func TestInstallAction(t *testing.T) {
	var config struct {
		Packages map[string]struct {
			ExtraFiles []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"extra-files"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "release-please-config.json")), &config); err != nil {
		t.Fatalf("release-please-config.json: %v", err)
	}
	generic := false
	for _, f := range config.Packages["."].ExtraFiles {
		generic = generic || (f.Type == "generic" && f.Path == "action.yml")
	}
	if !generic {
		t.Error("release-please-config.json MUST list action.yml as a generic extra-file")
	}

	pin := "default: v" + releaseVersion(t) + " # x-release-please-version"
	if !strings.Contains(readRepoFile(t, "action.yml"), pin) {
		t.Errorf("action.yml MUST carry %q", pin)
	}
}

// usesOf returns each uses: value in a workflow or action file with its line comment, in file order.
func usesOf(t *testing.T, rel string) (uses, comments []string) {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(readRepoFile(t, rel)), &doc); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				if k, v := n.Content[i], n.Content[i+1]; k.Value == "uses" && v.Kind == yaml.ScalarNode {
					uses, comments = append(uses, v.Value), append(comments, v.LineComment)
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&doc)
	if len(uses) == 0 {
		t.Fatalf("%s has no uses:", rel)
	}
	return uses, comments
}

// publish.yml holds the write token, so it MUST run only GitHub's artifact and checkout actions and loupe's own install.
func TestPublishWorkflowRunsNoCallerCode(t *testing.T) {
	const rel = ".github/workflows/publish.yml"
	var workflow struct {
		On   map[string]any `yaml:"on"`
		Jobs map[string]struct {
			Steps []struct {
				Uses string         `yaml:"uses"`
				With map[string]any `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, rel)), &workflow); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
	if _, ok := workflow.On["workflow_call"]; !ok || len(workflow.On) != 1 {
		t.Errorf("%s MUST declare on: workflow_call only, got %v", rel, workflow.On)
	}
	allowed := map[string]bool{"actions/checkout": true, "actions/upload-artifact": true, "actions/download-artifact": true}
	uses, _ := usesOf(t, rel)
	for _, u := range uses {
		name, _, _ := strings.Cut(u, "@")
		if !allowed[name] && u != "./.loupe" {
			t.Errorf("%s uses %s, which is not on the allowlist", rel, u)
		}
	}
	checkouts := 0
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if !strings.HasPrefix(step.Uses, "actions/checkout@") {
				continue
			}
			checkouts++
			if step.With["repository"] != "secondshift-labs/loupe" || step.With["sparse-checkout"] != "action.yml" {
				t.Errorf("%s's checkout MUST be secondshift-labs/loupe's action.yml alone, got %v", rel, step.With)
			}
		}
	}
	if checkouts != 1 {
		t.Errorf("%s has %d checkout steps, want 1", rel, checkouts)
	}
}

func TestCIPiecesPinBySHA(t *testing.T) {
	pinned := regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}$`)
	version := regexp.MustCompile(`^# v[0-9]+\.[0-9]+\.[0-9]+$`)
	for _, rel := range []string{"ci/capture/action.yml", "ci/save/action.yml", ".github/workflows/publish.yml"} {
		uses, comments := usesOf(t, rel)
		for i, u := range uses {
			if u == "./.loupe" {
				continue
			}
			if !pinned.MatchString(u) || !version.MatchString(comments[i]) {
				t.Errorf("%s: uses %s %s MUST pin a 40-character sha with a version comment", rel, u, comments[i])
			}
		}
	}
}

// ci/capture installs the release named in the root action.yml at its own commit, so a version of its own could drift
// from the caller's pin.
func TestCapturePieceCarriesNoVersion(t *testing.T) {
	for _, rel := range []string{"ci/capture/action.yml", "ci/capture/capture.sh"} {
		text := readRepoFile(t, rel)
		for _, banned := range []string{"x-release-please-version", "secondshift-labs/loupe@"} {
			if strings.Contains(text, banned) {
				t.Errorf("%s MUST NOT contain %q", rel, banned)
			}
		}
	}
}

// Inside a called workflow, github.workflow_sha is the caller's commit and job.workflow_sha is the called workflow's
// own (docs/github-facts.md, 2026-11-15). Checking loupe out at the caller's commit would fail or install the wrong
// release.
func TestPublishWorkflowReadsItsOwnCommit(t *testing.T) {
	text := readRepoFile(t, ".github/workflows/publish.yml")
	if strings.Contains(text, "github.workflow_sha") {
		t.Error("publish.yml MUST NOT read github.workflow_sha, which names the caller's commit")
	}
	if !strings.Contains(text, "${{ toJSON(job) }}") || !strings.Contains(text, ".workflow_sha") {
		t.Error("publish.yml MUST read its own commit from the job context's workflow_sha")
	}
}

// A job with no timeout runs for GitHub's default of 6 hours when an API call or an agent hangs.
func TestCIJobsHaveTimeouts(t *testing.T) {
	text := readRepoFile(t, ".github/workflows/publish.yml")
	if !strings.Contains(text, "timeout-minutes:") {
		t.Error("publish.yml's publish job MUST set timeout-minutes")
	}
	doc := readRepoFile(t, "docs/ci-action.md")
	start := strings.Index(doc, "name: review\n")
	if start < 0 {
		t.Fatal("docs/ci-action.md has no starter file")
	}
	starter := doc[start:]
	starter = starter[:strings.Index(starter, "```")]
	if n := strings.Count(starter, "timeout-minutes:"); n != 2 {
		t.Errorf("the starter file sets timeout-minutes on %d jobs, want 2 (review and unlabel)", n)
	}
}
