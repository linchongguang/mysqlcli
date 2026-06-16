package metacommand

import (
	"fmt"
	"strings"
	"unicode"
)

type Command struct {
	Name string
	Args []string
}

func Parse(line string) (Command, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "\\") {
		return Command{}, fmt.Errorf("快捷命令必须以反斜杠开头")
	}
	fields, err := splitFields(strings.TrimPrefix(line, "\\"))
	if err != nil {
		return Command{}, err
	}
	if len(fields) == 0 {
		return Command{}, fmt.Errorf("快捷命令不能为空")
	}
	return Command{Name: strings.ToLower(fields[0]), Args: fields[1:]}, nil
}

func splitFields(value string) ([]string, error) {
	var result []string
	var current strings.Builder
	var quote rune
	escaped := false

	flush := func() {
		if current.Len() > 0 {
			result = append(result, current.String())
			current.Reset()
		}
	}

	for _, char := range value {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if unicode.IsSpace(char) {
			flush()
			continue
		}
		current.WriteRune(char)
	}
	if quote != 0 {
		return nil, fmt.Errorf("快捷命令参数引号未闭合")
	}
	if escaped {
		current.WriteRune('\\')
	}
	flush()
	return result, nil
}
