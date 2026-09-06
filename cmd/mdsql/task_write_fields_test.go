package main

import (
	"reflect"
	"runtime"
	"testing"
)

func TestTaskWriteFieldsEndToEndRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only")
	}
	const name = "tasks.md"
	root, file := contractFixture(t, name, "\ufeff# Tasks\r\n- [ ] seed\r\n")
	insert := "INSERT INTO tasks (section, text, due, scheduled, priority, tags) VALUES ('Tasks', 'ship', '2026-09-07', '2026-09-06', 5, 'work, #release')"
	contractReport(t, contractRun(t, root, name, insert), true, 1, 1)
	contractBytes(t, file, []byte("\ufeff# Tasks\r\n- [ ] seed\r\n- [ ] ship #work #release 📅 2026-09-07 ⏳ 2026-09-06 🔺\r\n"))

	rows := contractRows(t, root, name, "SELECT text, due, scheduled, priority, tags, state, raw FROM tasks WHERE line = 3")
	want := []map[string]any{{
		"text": "ship #work #release", "due": "2026-09-07", "scheduled": "2026-09-06",
		"priority": float64(5), "tags": []any{"work", "release"}, "state": "todo",
		"raw": "- [ ] ship #work #release 📅 2026-09-07 ⏳ 2026-09-06 🔺",
	}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("insert round-trip rows=%#v want=%#v", rows, want)
	}

	update := "UPDATE tasks SET text = 'launch', due = NULL, scheduled = '2026-09-08', priority = 1, tags = 'ops' WHERE line = 3"
	contractReport(t, contractRun(t, root, name, update), true, 1, 1)
	contractBytes(t, file, []byte("\ufeff# Tasks\r\n- [ ] seed\r\n- [ ] launch #ops ⏳ 2026-09-08 ⏬\r\n"))
	rows = contractRows(t, root, name, "SELECT text, due, scheduled, priority, tags, state, raw FROM tasks WHERE line = 3")
	want = []map[string]any{{
		"text": "launch #ops", "due": nil, "scheduled": "2026-09-08",
		"priority": float64(1), "tags": []any{"ops"}, "state": "todo",
		"raw": "- [ ] launch #ops ⏳ 2026-09-08 ⏬",
	}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("update round-trip rows=%#v want=%#v", rows, want)
	}
}
