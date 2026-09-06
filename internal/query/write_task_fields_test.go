package query

import "testing"

func TestParseTaskMetadataWrites(t *testing.T) {
	for _, sql := range []string{
		"INSERT INTO tasks (text, due, scheduled, priority, tags) VALUES ('ship', '2026-09-07', '2026-09-06', 5, 'work, #release')",
		"INSERT INTO tasks (text, due, scheduled, priority, tags) VALUES ('ship', NULL, NULL, NULL, NULL)",
		"UPDATE tasks SET text = 'ship', due = '2026-09-07', scheduled = '2026-09-06', priority = 1, tags = '#work docs/api' WHERE line = 1",
		"UPDATE tasks SET due = NULL, scheduled = NULL, priority = NULL, tags = NULL WHERE line = 1",
	} {
		if stmt, err := ParseWrite(sql); err != nil || stmt == nil {
			t.Errorf("rejected %q: statement=%#v error=%v", sql, stmt, err)
		}
	}
}

func TestRejectInvalidTaskMetadataWrites(t *testing.T) {
	for _, sql := range []string{
		"UPDATE tasks SET text = NULL WHERE line = 1",
		"UPDATE tasks SET due = 20260907 WHERE line = 1",
		"UPDATE tasks SET due = '2026-02-30' WHERE line = 1",
		"UPDATE tasks SET scheduled = '' WHERE line = 1",
		"UPDATE tasks SET priority = 0 WHERE line = 1",
		"UPDATE tasks SET priority = 6 WHERE line = 1",
		"UPDATE tasks SET priority = 1.5 WHERE line = 1",
		"UPDATE tasks SET tags = '' WHERE line = 1",
		"UPDATE tasks SET tags = '#ok, bad#tag' WHERE line = 1",
		"UPDATE tasks SET tags = 1 WHERE line = 1",
	} {
		if stmt, err := ParseWrite(sql); err == nil || stmt != nil {
			t.Errorf("accepted %q: statement=%#v error=%v", sql, stmt, err)
		}
	}
}
