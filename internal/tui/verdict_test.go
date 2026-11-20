package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/publish"
)

// verdictModel opens review on a ready draft whose f-001 is a blocking issue, gives it verdict, applies change, and
// presses p.
func verdictModel(t *testing.T, viewer, author, verdict string, height int, change func(d *draft.Draft) error) *Model {
	t.Helper()
	dir := readyFixture(t, viewer, author)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		d.Verdict = verdict
		if change != nil {
			return change(d)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "C"}, 100, height)
	m.Update(keyOf("p"))
	if m.view != viewAction {
		t.Fatalf("p did not open the action step: view %v notice %q", m.view, m.notice)
	}
	return m
}

func excludeBlocking(d *draft.Draft) error {
	_, err := draft.Exclude(d, "f-001", testNow)
	return err
}

func wantStart(t *testing.T, m *Model, action string) {
	t.Helper()
	if got := publish.Actions[m.pick]; got != action {
		t.Fatalf("start %q, want %q:\n%s", got, action, m.View().Content)
	}
}

func TestActionStepStartsOnTheVerdict(t *testing.T) {
	for _, c := range []struct {
		verdict, description string
		change               func(d *draft.Draft) error
	}{
		{"approve", "post the findings and approve the pull request", excludeBlocking},
		{"request-changes", "post the findings and request changes", nil},
		{"comment", "post the findings without a verdict", nil},
	} {
		t.Run(c.verdict, func(t *testing.T) {
			m := verdictModel(t, "reviewer", "author", c.verdict, 24, c.change)
			wantStart(t, m, c.verdict)
			text := paneText(m.View().Content)
			if !strings.Contains(text, "recommended "+m.glyphs.Sep+" "+c.description) || strings.Count(text, "recommended") != 1 {
				t.Fatalf("the %s row is not the one marked recommended, or a reason shows:\n%s", c.verdict, m.View().Content)
			}
		})
	}
}

func TestActionStepWithoutAVerdictIsUnchanged(t *testing.T) {
	m := verdictModel(t, "reviewer", "author", "", 24, nil)
	wantStart(t, m, "comment")
	if strings.Contains(m.View().Content, "recommended") {
		t.Fatalf("a mark or reason without a verdict:\n%s", m.View().Content)
	}
}

func TestActionStepOverridesTheVerdictAndSaysWhy(t *testing.T) {
	for _, c := range []struct {
		name, viewer, verdict, start, why string
		change                            func(d *draft.Draft) error
	}{
		{"approve with a blocking finding", "reviewer", "approve", "request-changes", "agent recommended approve; f-001 is blocking", nil},
		{"request-changes with nothing blocking", "reviewer", "request-changes", "comment", "agent recommended request-changes; no published finding is blocking", excludeBlocking},
		{"approve on the human's own pull request", "author", "approve", "comment", "agent recommended approve; you authored this pull request", excludeBlocking},
		{"approve with a blocking finding on the human's own pull request", "author", "approve", "comment", "agent recommended approve; you authored this pull request", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := verdictModel(t, c.viewer, "author", c.verdict, 24, c.change)
			wantStart(t, m, c.start)
			if text := paneText(m.View().Content); !strings.Contains(text, c.why) {
				t.Fatalf("no reason %q:\n%s", c.why, m.View().Content)
			}
			// The reason belongs to the step, not to the key: it stays while the cursor moves.
			m.Update(keyOf("k"))
			if text := paneText(m.View().Content); !strings.Contains(text, c.why) {
				t.Fatalf("the reason went with a key:\n%s", m.View().Content)
			}
		})
	}
}

func TestUnavailableRecommendedRowKeepsItsMark(t *testing.T) {
	m := verdictModel(t, "reviewer", "author", "approve", 24, nil)
	want := "recommended " + m.glyphs.Sep + " unavailable: cannot approve while publishable findings are blocking: f-001"
	if text := paneText(m.View().Content); !strings.Contains(text, want) {
		t.Fatalf("no %q:\n%s", want, m.View().Content)
	}
}

func TestEnterOnTheStartIsNeverRefused(t *testing.T) {
	m := verdictModel(t, "author", "author", "approve", 24, nil)
	m.Update(keyOf("enter"))
	if m.view != viewInline || m.action != "comment" {
		t.Fatalf("enter on the start: view %v action %q notice %q", m.view, m.action, m.notice)
	}
}

// The start is worked out when p opens the step, not when the human comes back to it.
func TestBackFromInlineKeepsTheHumansAction(t *testing.T) {
	m := verdictModel(t, "reviewer", "author", "approve", 24, nil)
	wantStart(t, m, "request-changes")
	for _, key := range []string{"k", "k", "enter", "esc"} {
		m.Update(keyOf(key))
	}
	wantStart(t, m, "comment")
	m.Update(keyOf("esc"))
	m.Update(keyOf("p"))
	wantStart(t, m, "request-changes")
}

// Short of rows the choices give way, and the reason is kept whole, as a refusal is. A refused enter on approve puts a
// refusal under the reason, so both compete for the rows.
func TestReasonStaysWholeWhenTheStepIsShort(t *testing.T) {
	const why = "agent recommended approve; f-001 is blocking"
	cut := false
	for _, width := range []int{60, 100} {
		for height := 12; height <= 24; height++ {
			m := verdictModel(t, "reviewer", "author", "approve", 24, nil)
			m.Update(keyOf("k"))
			m.Update(keyOf("enter"))
			m.Update(tea.WindowSizeMsg{Width: width, Height: height})
			view := m.View().Content
			text := paneText(view)
			if !strings.Contains(text, why) || !strings.Contains(text, "cannot approve while publishable findings are blocking: f-001; use") {
				t.Fatalf("%dx%d lost the reason or the refusal:\n%s", width, height, view)
			}
			shown := slices.DeleteFunc(slices.Clone(publish.Actions), func(a string) bool { return !strings.Contains(view, " "+a+" ") })
			cut = cut || len(shown) < len(publish.Actions)
		}
	}
	if !cut {
		t.Fatal("no size cut the choices, so the test never saw the reason under pressure")
	}
}
