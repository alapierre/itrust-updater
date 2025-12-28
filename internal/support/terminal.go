package support

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func ReadPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println() // Add a newline after the password entry
	return strings.TrimSpace(string(bytePassword)), nil
}
