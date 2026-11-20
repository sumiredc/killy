package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// render converts the small Markdown subset the privacy policy uses into HTML: `#` and `##`
// headings, paragraphs, `-` lists of one-line items, pipe tables, and the inline forms `code`,
// **bold**, [text](https://…) and <https://…>. Anything else is an error rather than being passed
// through, so a construct this does not understand never reaches the published page unnoticed.
//
// The shape it returns describes the document without its words (block kinds, list and table
// sizes, link targets), so two translations can be checked for the same structure.
func render(src string) (out string, shape []string, err error) {
	var b strings.Builder
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")

	var para []string
	var paraStart int
	var urls []string

	inline := func(text string, line int) (string, error) {
		p := inlineParser{src: text, line: line}
		h, err := p.parse(false)
		urls = append(urls, p.urls...)
		return h, err
	}
	flushPara := func() error {
		if len(para) == 0 {
			return nil
		}
		urls = nil
		h, err := inline(strings.Join(para, " "), paraStart)
		if err != nil {
			return err
		}
		b.WriteString("<p>" + h + "</p>\n")
		shape = append(shape, "p"+linkShape(urls))
		para = nil
		return nil
	}

	for i := 0; i < len(lines); i++ {
		n := i + 1
		line := strings.TrimRight(lines[i], " \t")
		switch {
		case line == "":
			if err := flushPara(); err != nil {
				return "", nil, err
			}
		case strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## "):
			if err := flushPara(); err != nil {
				return "", nil, err
			}
			level := 1
			if strings.HasPrefix(line, "## ") {
				level = 2
			}
			urls = nil
			h, err := inline(strings.TrimSpace(line[level:]), n)
			if err != nil {
				return "", nil, err
			}
			fmt.Fprintf(&b, "<h%d>%s</h%d>\n", level, h, level)
			shape = append(shape, fmt.Sprintf("h%d%s", level, linkShape(urls)))
		case strings.HasPrefix(line, "- "):
			if err := flushPara(); err != nil {
				return "", nil, err
			}
			urls = nil
			items := 0
			b.WriteString("<ul>\n")
			for ; i < len(lines) && strings.HasPrefix(strings.TrimRight(lines[i], " \t"), "- "); i++ {
				h, err := inline(strings.TrimSpace(strings.TrimRight(lines[i], " \t")[2:]), i+1)
				if err != nil {
					return "", nil, err
				}
				b.WriteString("<li>" + h + "</li>\n")
				items++
			}
			i--
			b.WriteString("</ul>\n")
			shape = append(shape, fmt.Sprintf("ul %d%s", items, linkShape(urls)))
		case strings.HasPrefix(line, "|"):
			if err := flushPara(); err != nil {
				return "", nil, err
			}
			start := i
			for i < len(lines) && strings.HasPrefix(strings.TrimRight(lines[i], " \t"), "|") {
				i++
			}
			rows := lines[start:i]
			i--
			h, rowsN, cols, tableURLs, err := renderTable(rows, start+1)
			if err != nil {
				return "", nil, err
			}
			b.WriteString(h)
			shape = append(shape, fmt.Sprintf("table %dx%d%s", rowsN, cols, linkShape(tableURLs)))
		default:
			if unsupportedStart.MatchString(line) || (strings.HasPrefix(line, "<") && !strings.HasPrefix(line, "<https://")) || line[0] == ' ' || line[0] == '\t' {
				return "", nil, fmt.Errorf("line %d: unsupported Markdown block: %q", n, line)
			}
			if strings.HasPrefix(line, "#") {
				return "", nil, fmt.Errorf("line %d: only # and ## headings are supported: %q", n, line)
			}
			if len(para) == 0 {
				paraStart = n
			}
			para = append(para, strings.TrimSpace(line))
		}
	}
	if err := flushPara(); err != nil {
		return "", nil, err
	}
	return b.String(), shape, nil
}

var (
	unsupportedStart = regexp.MustCompile("^(>|```|~~~|\\d+[.)] |[*+] |---|===|!\\[)")
	tableSeparator   = regexp.MustCompile(`^\|(\s*:?-+:?\s*\|)+$`)
)

func linkShape(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	return " " + strings.Join(urls, " ")
}

// renderTable renders a pipe table whose first row is the header and second the separator. It
// returns the number of body rows and of columns.
func renderTable(rows []string, firstLine int) (h string, bodyRows, cols int, urls []string, err error) {
	if len(rows) < 3 || !tableSeparator.MatchString(strings.TrimSpace(rows[1])) {
		return "", 0, 0, nil, fmt.Errorf("line %d: a table needs a header row, a |---| separator and at least one row", firstLine)
	}
	var b strings.Builder
	b.WriteString("<div class=\"table-wrap\"><table>\n")
	for r, row := range rows {
		if r == 1 {
			continue
		}
		cells := splitRow(row)
		if r == 0 {
			cols = len(cells)
		} else if len(cells) != cols {
			return "", 0, 0, nil, fmt.Errorf("line %d: this row has %d cells, the header has %d", firstLine+r, len(cells), cols)
		}
		tag := "td"
		if r == 0 {
			tag = "th"
			b.WriteString("<thead>")
		} else if r == 2 {
			b.WriteString("<tbody>")
		}
		b.WriteString("<tr>")
		for _, c := range cells {
			p := inlineParser{src: c, line: firstLine + r}
			ch, err := p.parse(false)
			if err != nil {
				return "", 0, 0, nil, err
			}
			urls = append(urls, p.urls...)
			fmt.Fprintf(&b, "<%s>%s</%s>", tag, ch, tag)
		}
		b.WriteString("</tr>")
		if r == 0 {
			b.WriteString("</thead>\n")
		} else {
			b.WriteString("\n")
		}
	}
	b.WriteString("</tbody></table></div>\n")
	return b.String(), len(rows) - 2, cols, urls, nil
}

func splitRow(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimSuffix(strings.TrimPrefix(row, "|"), "|")
	cells := strings.Split(row, "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

// inlineParser turns the inline forms of one block into HTML and records the link targets.
type inlineParser struct {
	src  string
	line int
	urls []string
}

func (p *inlineParser) fail(format string, a ...any) error {
	return fmt.Errorf("line %d: %s in %q", p.line, fmt.Sprintf(format, a...), p.src)
}

// parse converts p.src. In a link's text (inLink) a nested link is an error.
func (p *inlineParser) parse(inLink bool) (string, error) {
	var b strings.Builder
	s := p.src
	for i := 0; i < len(s); {
		switch {
		case s[i] == '`':
			end := strings.IndexByte(s[i+1:], '`')
			if end < 0 {
				return "", p.fail("unclosed `")
			}
			b.WriteString("<code>" + html.EscapeString(s[i+1:i+1+end]) + "</code>")
			i += end + 2
		case strings.HasPrefix(s[i:], "**"):
			end := strings.Index(s[i+2:], "**")
			if end < 0 {
				return "", p.fail("unclosed **")
			}
			inner := inlineParser{src: s[i+2 : i+2+end], line: p.line}
			h, err := inner.parse(inLink)
			if err != nil {
				return "", err
			}
			p.urls = append(p.urls, inner.urls...)
			b.WriteString("<strong>" + h + "</strong>")
			i += end + 4
		case s[i] == '[':
			if inLink {
				return "", p.fail("a link inside a link")
			}
			close := strings.Index(s[i:], "](")
			if close < 0 {
				return "", p.fail("unclosed [")
			}
			endURL := strings.IndexByte(s[i+close:], ')')
			if endURL < 0 {
				return "", p.fail("unclosed (")
			}
			target := s[i+close+2 : i+close+endURL]
			if err := checkURL(target); err != nil {
				return "", p.fail("%v", err)
			}
			inner := inlineParser{src: s[i+1 : i+close], line: p.line}
			h, err := inner.parse(true)
			if err != nil {
				return "", err
			}
			p.urls = append(p.urls, target)
			b.WriteString(`<a href="` + html.EscapeString(target) + `">` + h + "</a>")
			i += close + endURL + 1
		case s[i] == '<':
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				return "", p.fail("unclosed <")
			}
			target := s[i+1 : i+end]
			if err := checkURL(target); err != nil {
				return "", p.fail("%v", err)
			}
			p.urls = append(p.urls, target)
			b.WriteString(`<a href="` + html.EscapeString(target) + `">` + html.EscapeString(target) + "</a>")
			i += end + 1
		case s[i] == '*' || s[i] == ']':
			return "", p.fail("unsupported %q", s[i])
		default:
			j := i
			for j < len(s) && !strings.ContainsRune("`*[]<", rune(s[j])) {
				j++
			}
			b.WriteString(html.EscapeString(s[i:j]))
			i = j
		}
	}
	return b.String(), nil
}

// checkURL accepts only absolute https links: a relative link would break on the published page.
func checkURL(u string) error {
	if !strings.HasPrefix(u, "https://") || strings.ContainsAny(u, " \"<>") {
		return fmt.Errorf("link %q must be an absolute https:// URL", u)
	}
	return nil
}
