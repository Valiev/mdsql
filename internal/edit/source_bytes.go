package edit

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

type physicalLine struct{ start, end, next int }
type sourceBytes struct {
	raw   []byte
	lines []physicalLine
	bom   int
}

func indexBytes(raw []byte) sourceBytes {
	s := sourceBytes{raw: raw}
	if bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf}) {
		s.bom = 3
	}
	for at := s.bom; at < len(raw); {
		line := physicalLine{start: at}
		for at < len(raw) && raw[at] != '\r' && raw[at] != '\n' {
			at++
		}
		line.end = at
		if at < len(raw) {
			if raw[at] == '\r' && at+1 < len(raw) && raw[at+1] == '\n' {
				at++
			}
			at++
		}
		line.next = at
		s.lines = append(s.lines, line)
	}
	return s
}
func (s sourceBytes) offset(line int) (int, error) {
	if line < 1 || line > len(s.lines)+1 {
		return 0, fmt.Errorf("line must be between 1 and %d", len(s.lines)+1)
	}
	if line == len(s.lines)+1 {
		return len(s.raw), nil
	}
	return s.lines[line-1].start, nil
}
func (s sourceBytes) text(line int) string { l := s.lines[line-1]; return string(s.raw[l.start:l.end]) }
func (s sourceBytes) eol(line int) string {
	// Prefer the line immediately before the insertion, then the following one.
	for i := line - 2; i >= 0; i-- {
		l := s.lines[i]
		if l.next > l.end {
			return string(s.raw[l.end:l.next])
		}
	}
	for i := line - 1; i < len(s.lines); i++ {
		l := s.lines[i]
		if l.next > l.end {
			return string(s.raw[l.end:l.next])
		}
	}
	return "\n"
}
func (s sourceBytes) terminated() bool {
	return len(s.lines) > 0 && s.lines[len(s.lines)-1].next > s.lines[len(s.lines)-1].end
}
func normalizeNewlines(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}
func trimOneEOL(text string) string {
	if strings.HasSuffix(text, "\r\n") {
		return text[:len(text)-2]
	}
	if strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\r") {
		return text[:len(text)-1]
	}
	return text
}

// insertion contains separators as part of the new bytes; old bytes never move
// into replacement ranges. A trailing input EOL is not a phantom empty line.
func (s sourceBytes) insertion(line int, text string, structured, child bool) ([]byte, int, error) {
	at, err := s.offset(line)
	if err != nil {
		return nil, 0, err
	}
	eol := s.eol(line)
	text = strings.ReplaceAll(normalizeNewlines(text), "\n", eol)
	text = trimOneEOL(text)
	prefix, suffix := "", ""
	if at == len(s.raw) && len(s.lines) > 0 && !s.terminated() {
		prefix = eol
	}
	if structured && !child && line > 1 && strings.TrimSpace(s.text(line-1)) != "" {
		prefix += eol
	}
	if at < len(s.raw) || s.terminated() || text == "" || strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\r") {
		suffix = eol
	}
	if structured && !child && at < len(s.raw) && strings.TrimSpace(s.text(line)) != "" {
		suffix += eol
	}
	first := line + len(indexBytes([]byte(prefix)).lines)
	// A separator terminating the existing EOF line does not add a new blank row.
	if at == len(s.raw) && len(s.lines) > 0 && !s.terminated() {
		first--
	}
	return []byte(prefix + text + suffix), first, nil
}

type bytePatch struct {
	start, end int
	data       []byte
}

func applyPatches(original []byte, patches []bytePatch) ([]byte, error) {
	sort.Slice(patches, func(i, j int) bool { return patches[i].start < patches[j].start })
	for i, p := range patches {
		if p.start < 0 || p.end < p.start || p.end > len(original) {
			return nil, fmt.Errorf("invalid patch range")
		}
		if i > 0 && (patches[i-1].end > p.start || patches[i-1].start == p.start) {
			return nil, fmt.Errorf("overlapping patches")
		}
	}
	out := bytes.Clone(original)
	for i := len(patches) - 1; i >= 0; i-- {
		p := patches[i]
		next := make([]byte, 0, len(out)-(p.end-p.start)+len(p.data))
		next = append(next, out[:p.start]...)
		next = append(next, p.data...)
		next = append(next, out[p.end:]...)
		out = next
	}
	return out, nil
}
