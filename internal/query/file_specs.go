package query

import "strings"

func (p *parser) parseFileSpecs() ([]string, error) {
	parenthesized := p.match(tLParen)
	if parenthesized {
		p.fileListStart = p.next().pos
	}
	var files []string
	for {
		if p.match(tString) {
			if p.peek().lit == "" {
				return nil, p.expected("non-empty file specification")
			}
			files = append(files, p.next().lit)
		} else if parenthesized {
			return nil, p.expected("quoted file specification in IN (...)")
		} else {
			var spec strings.Builder
			for p.peek().typ != tEOF && !p.match(tComma) && !isQueryClause(p.peek()) {
				spec.WriteString(p.next().lit)
			}
			if spec.Len() == 0 {
				return nil, p.expected("file specification after IN")
			}
			files = append(files, spec.String())
		}
		if p.match(tComma) {
			p.next()
			continue
		}
		if parenthesized {
			if !p.match(tRParen) {
				return nil, p.expected(", or ) after file specification")
			}
			p.fileListEnd = p.next().pos + 1
		}
		return files, nil
	}
}

func quotedFileSpecs(files []string) string {
	escape := strings.NewReplacer("\n", `\n`, "\t", `\t`, "'", `\'`)
	quoted := make([]string, len(files))
	for i, file := range files {
		quoted[i] = "'" + escape.Replace(file) + "'"
	}
	return strings.Join(quoted, ", ")
}
