package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
)

func promptChoiceConsole(question, title string, choices []string) (string, error) {
	printConsolePromptHeader(title)
	fmt.Fprintf(os.Stderr, "%s\n\nChoices:\n", question)
	for i, c := range choices {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, c)
	}
	fmt.Fprintf(os.Stderr, "\nEnter number or text: ")

	var ttyPath string
	if runtime.GOOS == "windows" {
		ttyPath = "CONIN$"
	} else {
		ttyPath = "/dev/tty"
	}

	tty, err := os.Open(ttyPath)
	if err != nil {
		return "", fmt.Errorf("cannot open console (%s): %w", ttyPath, err)
	}
	defer tty.Close()

	scanner := bufio.NewScanner(tty)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		for i, c := range choices {
			if input == fmt.Sprintf("%d", i+1) {
				return c, nil
			}
		}
		return input, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func printConsolePromptHeader(title string) {
	const (
		borderCols = 50
		label      = "  AI ASSISTANT QUESTION"
	)
	border := strings.Repeat("═", borderCols)
	padding := strings.Repeat(" ", borderCols-len(label))
	fmt.Fprintf(os.Stderr, "\n╔%s╗\n║%s%s║\n╚%s╝\n", border, label, padding, border)
	if title != "" && title != "AI Assistant" {
		fmt.Fprintf(os.Stderr, "[%s]\n", title)
	}
}

func promptConsole(question string) (string, error) {
	printConsolePromptHeader("AI Assistant")
	fmt.Fprintf(os.Stderr, "%s\n\nYour answer: ", question)
	os.Stderr.Sync()

	var ttyPath string
	if runtime.GOOS == "windows" {
		ttyPath = "CONIN$"
	} else {
		ttyPath = "/dev/tty"
	}

	tty, err := os.Open(ttyPath)
	if err == nil {
		defer tty.Close()
		scanner := bufio.NewScanner(tty)
		if scanner.Scan() {
			return scanner.Text(), nil
		}
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", nil
	}

	fmt.Fprintf(os.Stderr, "\n[ask_user] Unable to access terminal. Please check your MCP client configuration.\n")
	os.Stderr.Sync()
	return "", nil
}
