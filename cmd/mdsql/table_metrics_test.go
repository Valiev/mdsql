package main

import (
	"encoding/json"
	"testing"
)

func TestTableMetricsAndDataCells(t *testing.T) {
	const source = "# Metrics\n\n| Name | Score |\n| :--- | ---: |\n| Alice | 10 |\n| Bob | |\n"
	for _, tc := range []struct {
		sql  string
		want string
	}{
		{"SELECT rows, columns FROM tables", `[{"rows":2,"columns":2}]`},
		{"SELECT header, value FROM cells WHERE NOT is_header AND header = 'Name'", `[{"header":"Name","value":"Alice"},{"header":"Name","value":"Bob"}]`},
	} {
		code, out, stderr := stdinRun(t, source, []string{"--strict", "--json", tc.sql})
		var got, want any
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("%s: code=%d out=%s err=%s", tc.sql, code, out, stderr)
		}
		if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
			t.Fatal(err)
		}
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		if code != 0 || stderr != "" || string(gotJSON) != string(wantJSON) {
			t.Errorf("%s: code=%d out=%s err=%s", tc.sql, code, out, stderr)
		}
	}
}
