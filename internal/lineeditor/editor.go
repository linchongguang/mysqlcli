package lineeditor

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var ErrInterrupt = errors.New("输入被中断")

type Editor struct {
	input          *os.File
	output         io.Writer
	historyFile    string
	historyEnabled bool
	history        []string
}

func New(input *os.File, output io.Writer, historyFile string, historyEnabled bool) (*Editor, error) {
	editor := &Editor{
		input:          input,
		output:         output,
		historyFile:    historyFile,
		historyEnabled: historyEnabled && historyFile != "",
	}
	if err := editor.loadHistory(); err != nil {
		return nil, err
	}
	return editor, nil
}

func (e *Editor) ReadLine(prompt string) (string, error) {
	state, err := terminalState(e.input)
	if err != nil {
		return "", err
	}
	if err := setRaw(e.input); err != nil {
		return "", err
	}
	defer restoreTerminal(e.input, state)

	fmt.Fprint(e.output, prompt)
	reader := bufio.NewReader(e.input)
	line := make([]rune, 0, 128)
	position := 0
	historyIndex := len(e.history)
	draft := ""

	for {
		char, _, err := reader.ReadRune()
		if err != nil {
			return "", err
		}
		switch char {
		case '\r', '\n':
			fmt.Fprint(e.output, "\r\n")
			return string(line), nil
		case 3:
			fmt.Fprint(e.output, "^C\r\n")
			return "", ErrInterrupt
		case 4:
			if len(line) == 0 {
				fmt.Fprint(e.output, "\r\n")
				return "", io.EOF
			}
		case 1:
			position = 0
			e.redraw(prompt, line, position)
		case 5:
			position = len(line)
			e.redraw(prompt, line, position)
		case 8, 127:
			if position > 0 {
				line = append(line[:position-1], line[position:]...)
				position--
				e.redraw(prompt, line, position)
			}
		case 27:
			sequence, err := readEscapeSequence(reader)
			if err != nil {
				continue
			}
			switch sequence {
			case "[A":
				if len(e.history) > 0 && historyIndex > 0 {
					if historyIndex == len(e.history) {
						draft = string(line)
					}
					historyIndex--
					line = []rune(e.history[historyIndex])
					position = len(line)
					e.redraw(prompt, line, position)
				}
			case "[B":
				if historyIndex < len(e.history) {
					historyIndex++
					if historyIndex == len(e.history) {
						line = []rune(draft)
					} else {
						line = []rune(e.history[historyIndex])
					}
					position = len(line)
					e.redraw(prompt, line, position)
				}
			case "[C":
				if position < len(line) {
					position++
					e.redraw(prompt, line, position)
				}
			case "[D":
				if position > 0 {
					position--
					e.redraw(prompt, line, position)
				}
			}
		default:
			if char >= 32 && char != utf8.RuneError {
				line = append(line, 0)
				copy(line[position+1:], line[position:])
				line[position] = char
				position++
				e.redraw(prompt, line, position)
			}
		}
	}
}

func (e *Editor) AddHistory(statement string) error {
	statement = strings.TrimSpace(strings.Join(strings.Fields(statement), " "))
	if !e.historyEnabled || statement == "" || Sensitive(statement) {
		return nil
	}
	if len(e.history) > 0 && e.history[len(e.history)-1] == statement {
		return nil
	}
	e.history = append(e.history, statement)
	if len(e.history) > 1000 {
		e.history = e.history[len(e.history)-1000:]
	}
	if err := os.MkdirAll(filepath.Dir(e.historyFile), 0700); err != nil {
		return fmt.Errorf("创建历史目录: %w", err)
	}
	file, err := os.OpenFile(e.historyFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("打开历史文件: %w", err)
	}
	defer file.Close()
	if _, err := fmt.Fprintln(file, statement); err != nil {
		return fmt.Errorf("写入历史文件: %w", err)
	}
	return file.Chmod(0600)
}

func Sensitive(statement string) bool {
	upper := strings.ToUpper(statement)
	return strings.Contains(upper, "IDENTIFIED BY") ||
		strings.Contains(upper, "SET PASSWORD") ||
		strings.Contains(upper, "MASTER_PASSWORD") ||
		strings.Contains(upper, "SOURCE_PASSWORD")
}

func (e *Editor) loadHistory() error {
	if !e.historyEnabled {
		return nil
	}
	data, err := os.ReadFile(e.historyFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取历史文件: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			e.history = append(e.history, line)
		}
	}
	if len(e.history) > 1000 {
		e.history = e.history[len(e.history)-1000:]
	}
	return nil
}

func (e *Editor) redraw(prompt string, line []rune, position int) {
	fmt.Fprintf(e.output, "\r\x1b[2K%s%s", prompt, string(line))
	moveLeft := len(line) - position
	if moveLeft > 0 {
		fmt.Fprintf(e.output, "\x1b[%dD", moveLeft)
	}
}

func readEscapeSequence(reader *bufio.Reader) (string, error) {
	first, _, err := reader.ReadRune()
	if err != nil {
		return "", err
	}
	second, _, err := reader.ReadRune()
	if err != nil {
		return "", err
	}
	return string([]rune{first, second}), nil
}

func terminalState(file *os.File) (string, error) {
	command := exec.Command("stty", "-g")
	command.Stdin = file
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("读取终端状态: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func setRaw(file *os.File) error {
	command := exec.Command("stty", "raw", "-echo")
	command.Stdin = file
	if err := command.Run(); err != nil {
		return fmt.Errorf("进入终端行编辑模式: %w", err)
	}
	return nil
}

func restoreTerminal(file *os.File, state string) {
	command := exec.Command("stty", state)
	command.Stdin = file
	_ = command.Run()
}
