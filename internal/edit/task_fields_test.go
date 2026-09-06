package edit

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTaskInsertMetadataRoundTrip(t *testing.T) {
	raw := "# Tasks\n- [ ] old\n"
	after, matched, changed := corePlan(t, raw, "INSERT INTO tasks (section, text, due, scheduled, priority, tags) VALUES ('Tasks', 'ship', '2026-09-07', '2026-09-06', 5, 'work, #release')")
	want := "# Tasks\n- [ ] old\n- [ ] ship #work #release 📅 2026-09-07 ⏳ 2026-09-06 🔺\n"
	if after != want || matched != 1 || changed != 1 {
		t.Fatalf("got %q (%d,%d), want %q", after, matched, changed, want)
	}
	task := coreDoc(t, after).Tasks[1]
	if task.Due == nil || task.Due.Format(time.DateOnly) != "2026-09-07" ||
		task.Scheduled == nil || task.Scheduled.Format(time.DateOnly) != "2026-09-06" ||
		task.Priority != 5 || !reflect.DeepEqual(task.Tags, []string{"work", "release"}) {
		t.Fatalf("round-trip task=%#v", task)
	}
}

func TestTaskUpdateTextPreservesPrefixAndMetadataSuffix(t *testing.T) {
	raw := "\ufeff# Tasks\r\n\t*   [/] Old title #keep 📅 2026-09-07 ⏳ 2026-09-06 ⏫ 🆔 task-1 [owner:: me]\r\n"
	after, matched, changed := corePlan(t, raw, "UPDATE tasks SET text = 'New title' WHERE line = 2")
	want := "\ufeff# Tasks\r\n\t*   [/] New title #keep 📅 2026-09-07 ⏳ 2026-09-06 ⏫ 🆔 task-1 [owner:: me]\r\n"
	if after != want || matched != 1 || changed != 1 {
		t.Fatalf("got %q (%d,%d), want %q", after, matched, changed, want)
	}
}

func TestTaskUpdateMetadataPreservesOtherFieldsAndState(t *testing.T) {
	raw := "- [x] old #a 📅 2026-09-01 ⏳ 2026-09-02 ⏫ 🆔 task-1 [owner:: me]"
	after, matched, changed := corePlan(t, raw, "UPDATE tasks SET due = '2026-09-07', priority = 1, tags = '#x, y' WHERE line = 1")
	want := "- [x] old #x #y 📅 2026-09-07 ⏳ 2026-09-02 ⏬ 🆔 task-1 [owner:: me]"
	if after != want || matched != 1 || changed != 1 {
		t.Fatalf("got %q (%d,%d), want %q", after, matched, changed, want)
	}
	task := coreDoc(t, after).Tasks[0]
	if task.State != "done" || task.Scheduled == nil || task.Scheduled.Format(time.DateOnly) != "2026-09-02" || task.ID != "task-1" || task.Fields["owner"] != "me" {
		t.Fatalf("unrelated metadata changed: %#v", task)
	}
}

func TestTaskUpdateMetadataNullClearsCanonicalTokens(t *testing.T) {
	raw := "- [x] old #a 📅 2026-09-01 ⏳ 2026-09-02 ⏫"
	after, matched, changed := corePlan(t, raw, "UPDATE tasks SET due = NULL, scheduled = NULL, priority = NULL, tags = NULL WHERE line = 1")
	if after != "- [x] old" || matched != 1 || changed != 1 {
		t.Fatalf("got %q (%d,%d)", after, matched, changed)
	}
	task := coreDoc(t, after).Tasks[0]
	if task.Due != nil || task.Scheduled != nil || task.Priority != 0 || len(task.Tags) != 0 || task.State != "done" {
		t.Fatalf("clear round-trip task=%#v", task)
	}
}

func TestTaskMetadataCanonicalNoOp(t *testing.T) {
	raw := "- [/] old #a #b 📅 2026-09-07 ⏳ 2026-09-06 🔼"
	after, matched, changed := corePlan(t, raw, "UPDATE tasks SET tags = 'a b', due = '2026-09-07', scheduled = '2026-09-06', priority = 3 WHERE line = 1")
	if after != raw || matched != 1 || changed != 0 {
		t.Fatalf("got %q (%d,%d)", after, matched, changed)
	}
}

func TestTaskMetadataOnlyUpdateKeepsCheckbox(t *testing.T) {
	raw := "- [/] old"
	after, matched, changed := corePlan(t, raw, "UPDATE tasks SET due = '2026-09-07' WHERE line = 1")
	if after != "- [/] old 📅 2026-09-07" || matched != 1 || changed != 1 {
		t.Fatalf("got %q (%d,%d)", after, matched, changed)
	}
}

func TestTaskCanonicalDateWriteRejectsDataviewConflict(t *testing.T) {
	for _, sql := range []string{
		"UPDATE tasks SET due = '2026-09-07' WHERE line = 1",
		"UPDATE tasks SET due = NULL WHERE line = 1",
		"UPDATE tasks SET scheduled = '2026-09-07' WHERE line = 1",
	} {
		raw := "- [ ] old [due:: 2026-09-01] (scheduled:: 2026-09-02)"
		stmt := coreStmt(t, sql)
		if err := validateStatement(stmt); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := planText([]byte(raw), stmt, coreDoc(t, raw)); err == nil || !strings.Contains(err.Error(), "non-canonical") {
			t.Fatalf("SQL=%q error=%v", sql, err)
		}
	}
}
