package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// prompt reads a line of input from stdin with the given label.
func prompt(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no input")
	}
	return strings.TrimSpace(scanner.Text()), nil
}

// promptYesNo asks a yes/no question. defaultYes controls the default when the user presses Enter.
func promptYesNo(label string, defaultYes bool) (bool, error) {
	fmt.Fprintf(os.Stderr, "%s ", label)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return defaultYes, nil
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	switch answer {
	case "":
		return defaultYes, nil
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid answer %q — expected y or n", answer)
	}
}

// promptPassword reads a password from the terminal without echoing.
func promptPassword(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr) // newline after hidden input
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}
