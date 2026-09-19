package ui

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/term"
)

const Banner = `
 ██████╗███████╗██████╗ 
██╔════╝██╔════╝╚════██╗
██║     ███████╗ █████╔╝
██║     ╚════██║ ╚═══██╗
╚██████╗███████║██████╔╝
 ╚═════╝╚══════╝╚═════╝ 

Ctrl+Shift+3 — text community CLI
`

var (
	inputOnce   sync.Once
	scannerOnce sync.Once
	inputFile   *os.File
	scanner     *bufio.Scanner
)

// interactiveInput returns a terminal for prompts. When cs3 is started via
// curl|bash, stdin is the script pipe (already at EOF); fall back to /dev/tty.
func interactiveInput() *os.File {
	inputOnce.Do(func() {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			inputFile = os.Stdin
			return
		}
		name := "/dev/tty"
		if runtime.GOOS == "windows" {
			name = "CONIN$"
		}
		f, err := os.OpenFile(name, os.O_RDWR, 0)
		if err != nil {
			inputFile = os.Stdin
			return
		}
		inputFile = f
	})
	return inputFile
}

func inputScanner() *bufio.Scanner {
	scannerOnce.Do(func() {
		scanner = bufio.NewScanner(interactiveInput())
		// Allow long paste (thoughts up to ~10k runes).
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
	})
	return scanner
}

func PrintBanner() {
	fmt.Print(Banner)
}

func Println(a ...any) {
	fmt.Println(a...)
}

func Printf(format string, a ...any) {
	fmt.Printf(format, a...)
}

func ReadLine(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	sc := inputScanner()
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("eof")
	}
	return strings.TrimSpace(sc.Text()), nil
}

// ReadMultiline reads lines until a line equals endMarker (typically ".").
// Trailing blank lines are trimmed; internal blank lines are kept.
// EOF before the end marker is an error so a partial paste is not submitted.
func ReadMultiline(endMarker string) (string, error) {
	if endMarker == "" {
		endMarker = "."
	}
	sc := inputScanner()
	var lines []string
	for {
		if !sc.Scan() {
			if err := sc.Err(); err != nil {
				return "", err
			}
			return "", fmt.Errorf("eof")
		}
		line := strings.TrimRight(sc.Text(), "\r")
		if line == endMarker {
			for len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			return strings.Join(lines, "\n"), nil
		}
		lines = append(lines, line)
	}
}

func ReadPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	in := interactiveInput()
	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		sc := inputScanner()
		if !sc.Scan() {
			return "", fmt.Errorf("password required")
		}
		return strings.TrimSpace(sc.Text()), nil
	}
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Confirm(prompt string) (bool, error) {
	s, err := ReadLine(prompt + " [y/N]: ")
	if err != nil {
		return false, err
	}
	s = strings.ToLower(s)
	return s == "y" || s == "yes", nil
}

func Truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return string(r)
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}
