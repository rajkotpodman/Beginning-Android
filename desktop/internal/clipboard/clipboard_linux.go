//go:build !windows

package clipboard

import (
	"bytes"
	"os/exec"
	"strings"
)

func readSystemClipboard() (string, error) {
	// 1. Try Wayland wl-paste
	if path, err := exec.LookPath("wl-paste"); err == nil {
		cmd := exec.Command(path, "-n")
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
	}

	// 2. Try xclip
	if path, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(path, "-selection", "clipboard", "-o")
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
	}

	// 3. Try xsel
	if path, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command(path, "--clipboard", "--output")
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
	}

	return "", nil
}

func writeSystemClipboard(text string) error {
	// 1. Try Wayland wl-copy
	if path, err := exec.LookPath("wl-copy"); err == nil {
		cmd := exec.Command(path)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// 2. Try xclip
	if path, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command(path, "-selection", "clipboard")
		cmd.Stdin = bytes.NewReader([]byte(text))
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// 3. Try xsel
	if path, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command(path, "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return nil
}
