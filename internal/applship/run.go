package applship

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func run(name string, args ...string) error {
	fmt.Fprintf(os.Stderr, "$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

func exists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
