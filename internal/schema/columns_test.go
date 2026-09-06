package schema

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEveryColumnHasOneLineDescription(t *testing.T) {
	var out bytes.Buffer
	if err := JSON(&out, true, false); err != nil {
		t.Fatal(err)
	}
	var schema Schema
	if err := json.Unmarshal(out.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	for table, columns := range schema.Tables {
		for _, column := range columns {
			if strings.TrimSpace(column.Doc) == "" || strings.ContainsAny(column.Doc, "\r\n") {
				t.Errorf("%s.%s description=%q", table, column.Name, column.Doc)
			}
		}
	}
	out.Reset()
	Text(&out, true, false)
	if out.Len() > 6*1024 {
		t.Fatalf("table schema is %d bytes, limit 6144", out.Len())
	}
	for table, columns := range schema.Tables {
		if !strings.Contains(out.String(), "\n"+table+"\n") {
			t.Errorf("text missing table %s", table)
		}
		for _, column := range columns {
			if !strings.Contains(out.String(), "\n"+column.Name+":"+column.Type+" "+column.Doc+"\n") {
				t.Errorf("text missing %s.%s", table, column.Name)
			}
		}
	}
}
