package output

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"github.com/Valiev/mdsql/internal/query"
)

func (r *Renderer) renderGrouped(rows []query.Row, selected []string) error {
	if len(rows) == 0 {
		return nil
	}
	files := []string{}
	groups := map[string][]query.Row{}
	for _, row := range rows {
		file := row["file"].String()
		if _, ok := groups[file]; !ok {
			files = append(files, file)
		}
		groups[file] = append(groups[file], row)
	}
	keys := []string{}
	for _, key := range selectedKeys(rows[0], selected) {
		if key != "file" && !strings.HasPrefix(key, "__") {
			keys = append(keys, key)
		}
	}
	bw := bufio.NewWriter(r.Out)
	root := groupedRoot(files)
	directories := groupedDirectories(files, root)
	if root != "" {
		if _, err := fmt.Fprintf(bw, "root=%q\n", root); err != nil {
			return err
		}
	}
	writer := csv.NewWriter(bw)
	writer.Comma = '\t'
	if len(keys) > 0 {
		if err := writer.Write(keys); err != nil {
			return err
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return err
		}
	}
	directory := ""
	for i, file := range files {
		if directories[i] != directory {
			directory = directories[i]
			if _, err := fmt.Fprintf(bw, "dir=%q\n", directory); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(bw, "@%q\n", file[len(root)+len(directory):]); err != nil {
			return err
		}
		if len(keys) > 0 {
			for _, row := range groups[file] {
				values := make([]string, 0, len(keys))
				for _, key := range keys {
					value := scalarToText(r.displayValue(row[key], ""))
					if r.MaxLines > 0 {
						lines := strings.Split(value, "\n")
						if len(lines) > r.MaxLines {
							value = strings.Join(lines[:r.MaxLines], "\n") + fmt.Sprintf("\n… (+%d lines)", len(lines)-r.MaxLines)
						}
					}
					values = append(values, encodeForTSV(query.NewString(value)))
				}
				if err := writer.Write(values); err != nil {
					return err
				}
			}
			writer.Flush()
			if err := writer.Error(); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

func groupedRoot(files []string) string {
	if len(files) < 2 {
		return ""
	}
	common := files[0]
	for _, file := range files[1:] {
		i := 0
		for i < len(common) && i < len(file) && common[i] == file[i] {
			i++
		}
		common = common[:i]
	}
	root := common[:strings.LastIndexByte(common, '/')+1]
	if root == "" {
		return ""
	}
	saved := -len("root=" + strconv.Quote(root) + "\n")
	for _, file := range files {
		saved += len(strconv.Quote(file)) - len(strconv.Quote(file[len(root):]))
	}
	if saved <= 0 {
		return ""
	}
	return root
}

func groupedDirectories(files []string, root string) []string {
	directories := make([]string, len(files))
	for start := 0; start < len(files); {
		file := files[start][len(root):]
		directory := file[:strings.LastIndexByte(file, '/')+1]
		end := start + 1
		for end < len(files) {
			next := files[end][len(root):]
			if next[:strings.LastIndexByte(next, '/')+1] != directory {
				break
			}
			end++
		}
		if directory != "" {
			saved := -len("dir=" + strconv.Quote(directory) + "\n")
			if end < len(files) {
				saved -= len("dir=\"\"\n")
			}
			for _, file := range files[start:end] {
				saved += len(strconv.Quote(file[len(root):])) - len(strconv.Quote(file[len(root)+len(directory):]))
			}
			if saved > 0 {
				for i := start; i < end; i++ {
					directories[i] = directory
				}
			}
		}
		start = end
	}
	return directories
}
