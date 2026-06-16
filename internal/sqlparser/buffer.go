package sqlparser

import "strings"

type Buffer struct {
	lines     []string
	delimiter string
}

func NewBuffer() *Buffer {
	return &Buffer{delimiter: ";"}
}

func (b *Buffer) Append(line string) {
	b.lines = append(b.lines, line)
}

func (b *Buffer) String() string {
	return strings.Join(b.lines, "\n")
}

func (b *Buffer) Empty() bool {
	return len(b.lines) == 0 || strings.TrimSpace(b.String()) == ""
}

func (b *Buffer) Clear() {
	b.lines = nil
}

func (b *Buffer) Delimiter() string {
	return b.delimiter
}

func (b *Buffer) SetDelimiter(delimiter string) bool {
	delimiter = strings.TrimSpace(delimiter)
	if delimiter == "" || strings.ContainsAny(delimiter, " \t\r\n") {
		return false
	}
	b.delimiter = delimiter
	return true
}

func (b *Buffer) Complete() bool {
	statement := b.String()
	if strings.HasSuffix(strings.TrimSpace(statement), "\\g") || strings.HasSuffix(strings.TrimSpace(statement), "\\G") {
		return true
	}
	return hasTerminator(statement, b.delimiter)
}

func (b *Buffer) Statement() (string, bool) {
	statement := strings.TrimSpace(b.String())
	vertical := false
	if strings.HasSuffix(statement, "\\G") {
		vertical = true
		statement = strings.TrimSpace(strings.TrimSuffix(statement, "\\G"))
	} else if strings.HasSuffix(statement, "\\g") {
		statement = strings.TrimSpace(strings.TrimSuffix(statement, "\\g"))
	} else {
		statement = strings.TrimSpace(strings.TrimSuffix(statement, b.delimiter))
	}
	return statement, vertical
}

func hasTerminator(statement string, delimiter string) bool {
	var quote rune
	inLineComment := false
	inBlockComment := false
	escaped := false
	runes := []rune(statement)
	delimiterRunes := []rune(delimiter)

	for index := 0; index < len(runes); index++ {
		char := runes[index]
		next := rune(0)
		if index+1 < len(runes) {
			next = runes[index+1]
		}
		if inLineComment {
			if char == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if char == '*' && next == '/' {
				inBlockComment = false
				index++
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == quote {
				if next == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		if char == '#' || (char == '-' && next == '-' && (index+2 >= len(runes) || runes[index+2] == ' ')) {
			inLineComment = true
			continue
		}
		if char == '/' && next == '*' {
			inBlockComment = true
			index++
			continue
		}
		if char == '\'' || char == '"' || char == '`' {
			quote = char
			continue
		}
		if matchesAt(runes, delimiterRunes, index) {
			return strings.TrimSpace(string(runes[index+len(delimiterRunes):])) == ""
		}
	}
	return false
}

func matchesAt(value []rune, target []rune, index int) bool {
	if len(target) == 0 || index+len(target) > len(value) {
		return false
	}
	for targetIndex := range target {
		if value[index+targetIndex] != target[targetIndex] {
			return false
		}
	}
	return true
}
