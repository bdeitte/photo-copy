package cli

import (
	"bufio"
	"fmt"
	"strings"
)

func promptUser(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return strings.TrimSpace(answer), nil
}

func promptYesNo(reader *bufio.Reader, prompt string) (bool, error) {
	answer, err := promptUser(reader, prompt)
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(answer)
	return answer == "y" || answer == "yes", nil
}
