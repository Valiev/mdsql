package edit

import (
	"bytes"
	"fmt"
	"strings"
)

func unifiedDiff(path string, before, after []byte) string {
	if bytes.Equal(before, after) {
		return ""
	}
	// Unified diff counts LF-delimited lines; CR is part of line content.
	split := func(data []byte) [][]byte {
		if len(data) == 0 {
			return nil
		}
		lines := bytes.SplitAfter(data, []byte("\n"))
		if len(lines[len(lines)-1]) == 0 {
			lines = lines[:len(lines)-1]
		}
		return lines
	}
	old, newLines := split(before), split(after)
	prefix := 0
	for prefix < len(old) && prefix < len(newLines) && bytes.Equal(old[prefix], newLines[prefix]) {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(newLines)-prefix && bytes.Equal(old[len(old)-suffix-1], newLines[len(newLines)-suffix-1]) {
		suffix++
	}
	start := prefix - 3
	if start < 0 {
		start = 0
	}
	contextAfter := suffix
	if contextAfter > 3 {
		contextAfter = 3
	}
	oldEnd, newEnd := len(old)-suffix+contextAfter, len(newLines)-suffix+contextAfter
	rangeText := func(start, count int) string {
		if count == 0 {
			return fmt.Sprintf("%d,0", start)
		}
		return fmt.Sprintf("%d,%d", start+1, count)
	}
	var out strings.Builder
	name := path
	if strings.ContainsAny(name, "\r\n\t\"") {
		name = fmt.Sprintf("%q", name)
	}
	fmt.Fprintf(&out, "--- %s\n+++ %s\n@@ -%s +%s @@\n", name, name, rangeText(start, oldEnd-start), rangeText(start, newEnd-start))
	write := func(prefix byte, lines [][]byte) {
		for _, line := range lines {
			out.WriteByte(prefix)
			out.Write(line)
			if line[len(line)-1] != '\n' {
				out.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	write(' ', old[start:prefix])
	write('-', old[prefix:len(old)-suffix])
	write('+', newLines[prefix:len(newLines)-suffix])
	write(' ', old[len(old)-suffix:oldEnd])
	return out.String()
}
