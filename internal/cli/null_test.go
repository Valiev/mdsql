package cli

import "testing"

func TestNullOverride(t *testing.T) {
	for _, format := range []string{"text", "grouped", "csv", "tsv"} {
		for _, marker := range []string{"", "NULL", "-", "--json"} {
			cfg, err := ParseArgs([]string{"--format", format, "--null", marker, "SELECT file FROM files", "note.md"})
			if err != nil || cfg.Null == nil || *cfg.Null != marker {
				t.Fatalf("format=%s marker=%q cfg=%+v err=%v", format, marker, cfg, err)
			}
		}
	}
	for _, flags := range [][]string{{"--null"}, {"--json", "--null=NULL"}, {"--format", "plain", "--null=NULL"}} {
		if _, err := ParseArgs(flags); err == nil {
			t.Fatalf("expected error: %v", flags)
		}
	}
}
