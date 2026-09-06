package parser

import (
	"strings"

	"github.com/Valiev/mdsql/internal/model"
)

func parseTask(file, content, raw string, line int, container string) (model.Task, bool) {
	match := reTask.FindStringSubmatch(content)
	if match == nil {
		return model.Task{}, false
	}
	indent, status, body := match[1], match[2], match[3]
	task := model.Task{
		Block:     model.Block{File: file, Kind: model.KindList, Start: line, End: line, Line: line, Text: body, Raw: raw, Nested: len(indent) > 0, Level: len(indent) / 2},
		Text:      stripFormatting(body),
		Depth:     len(indent) / 2,
		Fields:    map[string]interface{}{},
		Container: container,
	}
	switch status {
	case " ":
		task.State, task.Status, task.Done = "todo", status, false
	case "x", "X":
		task.State, task.Status, task.Done = "done", status, true
	case "/":
		task.State, task.Status, task.Done = "in_progress", status, false
	case "-":
		task.State, task.Status, task.Done = "cancelled", status, false
	default:
		task.State, task.Status, task.Done = "other", status, false
	}
	parseTaskMeta(&task)
	task.Text = strings.TrimSpace(task.Text)
	return task, true
}

func extractQuoteTasks(doc *model.Document, lines []string, start, end int) [][2]int {
	var codeRanges [][2]int
	var containers []string
	fence, fenceDepth := "", 0
	for i := start; i < end; i++ {
		content, depth := quoteContent(lines[i])
		line := i + 1
		if depth < fenceDepth {
			fence = ""
		}
		if fence != "" {
			codeRanges[len(codeRanges)-1][1] = line
			closing, tail := parseCodeFence(content)
			if depth == fenceDepth && len(closing) >= len(fence) && closing[0] == fence[0] && tail == "" {
				fence = ""
			}
			continue
		}
		if len(containers) > depth {
			containers = containers[:depth]
		}
		if depth == 0 {
			continue
		}
		for len(containers) < depth {
			container := "blockquote"
			if len(containers) > 0 {
				container = containers[len(containers)-1]
			}
			containers = append(containers, container)
		}
		if opening, _ := parseCodeFence(content); opening != "" {
			fence, fenceDepth = opening, depth
			codeRanges = append(codeRanges, [2]int{line, line})
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(content), "[!") {
			if kind, _, _ := parseCalloutFirstLine([]string{content}); kind != "" {
				containers[depth-1] = "callout"
			}
		}
		if task, ok := parseTask(doc.Path, content, lines[i], line, containers[depth-1]); ok {
			doc.Tasks = append(doc.Tasks, task)
		}
	}
	return codeRanges
}

func quoteContent(line string) (string, int) {
	depth := 0
	for {
		trim := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(trim, ">") {
			return line, depth
		}
		line = trim[1:]
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			line = line[1:]
		}
		depth++
	}
}
