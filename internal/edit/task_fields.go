package edit

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

var (
	taskWriteTagPattern       = regexp.MustCompile(`#[\p{L}\p{N}_/-]+`)
	taskDuePattern            = regexp.MustCompile(`📅[ \t]+\d{4}-\d{2}-\d{2}`)
	taskScheduledPattern      = regexp.MustCompile(`⏳[ \t]+\d{4}-\d{2}-\d{2}`)
	taskPriorityPattern       = regexp.MustCompile(`🔺|⏫|🔼|🔽|⏬`)
	taskMetadataSuffixPattern = regexp.MustCompile(`[ \t]+(?:#[\p{L}\p{N}_/-]+|[📅⏳🛫✅❌➕][ \t]+\d{4}-\d{2}-\d{2}|🔺|⏫|🔼|🔽|⏬|🔁|🆔|⛔|\[[^\]\r\n]+::[^\]\r\n]+\]|\([^\)\r\n]+::[^\)\r\n]+\))`)
	taskDataviewDuePattern    = regexp.MustCompile(`(?i)(?:\[[ \t]*due[ \t]*::[^\]\r\n]+\]|\([ \t]*due[ \t]*::[^\)\r\n]+\))`)
	taskDataviewSchedPattern  = regexp.MustCompile(`(?i)(?:\[[ \t]*scheduled[ \t]*::[^\]\r\n]+\]|\([ \t]*scheduled[ \t]*::[^\)\r\n]+\))`)
)

func taskInsertMetadata(values map[string]query.Value) (string, error) {
	tokens := []string{}
	if value, ok := values["tags"]; ok && value.Kind != query.ValueNull {
		tags, err := taskTags(value)
		if err != nil {
			return "", err
		}
		for _, tag := range tags {
			tokens = append(tokens, "#"+tag)
		}
	}
	for _, field := range []struct{ key, marker string }{{"due", "📅"}, {"scheduled", "⏳"}} {
		if value, ok := values[field.key]; ok && value.Kind != query.ValueNull {
			date, err := taskDate(field.key, value)
			if err != nil {
				return "", err
			}
			tokens = append(tokens, field.marker+" "+date)
		}
	}
	if value, ok := values["priority"]; ok && value.Kind != query.ValueNull {
		marker, err := taskPriority(value)
		if err != nil {
			return "", err
		}
		tokens = append(tokens, marker)
	}
	if len(tokens) == 0 {
		return "", nil
	}
	return " " + strings.Join(tokens, " "), nil
}

func updateTaskLine(raw string, values map[string]query.Value) (string, error) {
	line := raw
	_, state := values["state"]
	_, statusSet := values["status"]
	if state || statusSet {
		status, err := taskStatus(values)
		if err != nil {
			return "", err
		}
		match := checkboxPattern.FindStringSubmatchIndex(line)
		if match == nil {
			return "", fmt.Errorf("cannot locate checkbox")
		}
		line = line[:match[2]] + status + line[match[3]:]
	}
	prefix, body, err := taskLineBody(line)
	if err != nil {
		return "", err
	}
	if err := taskMetadataConflict(body, values); err != nil {
		return "", err
	}
	if value, ok := values["text"]; ok {
		text, err := taskWriteText(value)
		if err != nil {
			return "", err
		}
		if at := taskMetadataSuffixPattern.FindStringIndex(body); at != nil {
			body = text + body[at[0]:]
		} else {
			body = text
		}
	}
	for _, field := range []struct {
		key     string
		marker  string
		pattern *regexp.Regexp
	}{{"due", "📅", taskDuePattern}, {"scheduled", "⏳", taskScheduledPattern}} {
		value, ok := values[field.key]
		if !ok {
			continue
		}
		replacement := ""
		if value.Kind != query.ValueNull {
			date, err := taskDate(field.key, value)
			if err != nil {
				return "", err
			}
			replacement = field.marker + " " + date
		}
		body = replaceTaskTokens(body, field.pattern.FindAllStringIndex(body, -1), replacement)
	}
	if value, ok := values["priority"]; ok {
		replacement := ""
		if value.Kind != query.ValueNull {
			marker, err := taskPriority(value)
			if err != nil {
				return "", err
			}
			replacement = marker
		}
		body = replaceTaskTokens(body, taskPriorityPattern.FindAllStringIndex(body, -1), replacement)
	}
	if value, ok := values["tags"]; ok {
		replacement := ""
		if value.Kind != query.ValueNull {
			tags, err := taskTags(value)
			if err != nil {
				return "", err
			}
			canonical := make([]string, 0, len(tags))
			for _, tag := range tags {
				canonical = append(canonical, "#"+tag)
			}
			replacement = strings.Join(canonical, " ")
		}
		body = replaceNormalizedTaskTokens(body, taskTagSpans(body), replacement)
	}
	return prefix + body, nil
}

func taskLineBody(raw string) (string, string, error) {
	match := checkboxPattern.FindStringSubmatchIndex(raw)
	if match == nil {
		return "", "", fmt.Errorf("cannot locate checkbox")
	}
	start := match[1]
	for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t') {
		start++
	}
	return raw[:start], raw[start:], nil
}

func taskWriteText(value query.Value) (string, error) {
	if value.Kind != query.ValueString {
		return "", fmt.Errorf("task text must be a string")
	}
	text := value.String()
	if strings.TrimSpace(text) == "" || strings.ContainsAny(text, "\r\n") {
		return "", fmt.Errorf("task text must be one nonempty line")
	}
	if taskTextMarker.MatchString(text) {
		return "", fmt.Errorf("task text must contain content without a list marker or checkbox")
	}
	return text, nil
}

func taskMetadataConflict(text string, values map[string]query.Value) error {
	for _, field := range []struct {
		key     string
		pattern *regexp.Regexp
	}{{"due", taskDataviewDuePattern}, {"scheduled", taskDataviewSchedPattern}} {
		if _, targeted := values[field.key]; targeted && field.pattern.MatchString(text) {
			return fmt.Errorf("cannot write tasks.%s while non-canonical [%s:: ...] metadata is present", field.key, field.key)
		}
	}
	return nil
}

func taskDate(key string, value query.Value) (string, error) {
	if value.Kind != query.ValueString {
		return "", fmt.Errorf("task %s must be an ISO date string", key)
	}
	date := value.String()
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return "", fmt.Errorf("task %s must be an ISO date string", key)
	}
	return date, nil
}

func taskPriority(value query.Value) (string, error) {
	if value.Kind != query.ValueInt {
		return "", fmt.Errorf("task priority must be an integer from 1 to 5")
	}
	markers := map[int64]string{1: "⏬", 2: "🔽", 3: "🔼", 4: "⏫", 5: "🔺"}
	marker := markers[value.Data.(int64)]
	if marker == "" {
		return "", fmt.Errorf("task priority must be an integer from 1 to 5")
	}
	return marker, nil
}

func taskTags(value query.Value) ([]string, error) {
	if value.Kind != query.ValueString {
		return nil, fmt.Errorf("task tags must be a string")
	}
	parts := strings.Fields(strings.ReplaceAll(value.String(), ",", " "))
	if len(parts) == 0 {
		return nil, fmt.Errorf("task tags must contain at least one tag")
	}
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimPrefix(part, "#")
		canonical := "#" + part
		if taskWriteTagPattern.FindString(canonical) != canonical {
			return nil, fmt.Errorf("invalid task tag %q", part)
		}
		tags = append(tags, part)
	}
	return tags, nil
}

func taskTagSpans(body string) [][2]int {
	spans := [][2]int{}
	for _, span := range taskWriteTagPattern.FindAllStringIndex(body, -1) {
		if span[0] == 0 || body[span[0]-1] == ' ' || body[span[0]-1] == '\t' || body[span[0]-1] == '(' {
			spans = append(spans, [2]int{span[0], span[1]})
		}
	}
	return spans
}

func replaceTaskTokens(body string, spans [][]int, replacement string) string {
	normalized := make([][2]int, 0, len(spans))
	for _, span := range spans {
		normalized = append(normalized, [2]int{span[0], span[1]})
	}
	return replaceNormalizedTaskTokens(body, normalized, replacement)
}

func replaceNormalizedTaskTokens(body string, spans [][2]int, replacement string) string {
	if len(spans) == 0 {
		if replacement == "" {
			return body
		}
		separator := " "
		if body == "" || strings.HasSuffix(body, " ") || strings.HasSuffix(body, "\t") {
			separator = ""
		}
		return body + separator + replacement
	}
	if len(spans) == 1 && body[spans[0][0]:spans[0][1]] == replacement {
		return body
	}
	for i := len(spans) - 1; i >= 1; i-- {
		body = removeTaskToken(body, spans[i][0], spans[i][1])
	}
	first := spans[0]
	if replacement == "" {
		return removeTaskToken(body, first[0], first[1])
	}
	return body[:first[0]] + replacement + body[first[1]:]
}

func removeTaskToken(body string, start, end int) string {
	space := start
	for space > 0 && (body[space-1] == ' ' || body[space-1] == '\t') {
		space--
	}
	if space < start && (end == len(body) || body[end] == ' ' || body[end] == '\t') {
		start = space
	}
	return body[:start] + body[end:]
}

func taskMatchesWrite(task model.Task, values map[string]query.Value) bool {
	if value, ok := values["text"]; ok {
		_, body, err := taskLineBody(task.Raw)
		if err != nil || !strings.HasPrefix(body, value.String()) {
			return false
		}
		tail := body[len(value.String()):]
		if tail != "" && tail[0] != ' ' && tail[0] != '\t' {
			return false
		}
	}
	if _, state := values["state"]; state {
		status, err := taskStatus(values)
		if err != nil || task.Status != status {
			return false
		}
	} else if _, statusSet := values["status"]; statusSet {
		status, err := taskStatus(values)
		if err != nil || task.Status != status {
			return false
		}
	}
	for _, field := range []struct {
		key  string
		date *time.Time
	}{{"due", task.Due}, {"scheduled", task.Scheduled}} {
		value, ok := values[field.key]
		if !ok {
			continue
		}
		if value.Kind == query.ValueNull {
			if field.date != nil {
				return false
			}
		} else if field.date == nil || field.date.Format(time.DateOnly) != value.String() {
			return false
		}
	}
	if value, ok := values["priority"]; ok {
		want := int64(0)
		if value.Kind != query.ValueNull {
			want = value.Data.(int64)
		}
		if int64(task.Priority) != want {
			return false
		}
	}
	if value, ok := values["tags"]; ok {
		want := []string{}
		if value.Kind != query.ValueNull {
			var err error
			want, err = taskTags(value)
			if err != nil {
				return false
			}
		}
		if len(task.Tags) != len(want) {
			return false
		}
		for i := range want {
			if task.Tags[i] != want[i] {
				return false
			}
		}
	}
	return true
}
