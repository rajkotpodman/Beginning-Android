package clipboard

import (
	"sync"
	"time"
)

var (
	mu          sync.Mutex
	lastText    string
	suppressJob bool
)

// SetText sets the system clipboard and updates internal cache to prevent echo loop
func SetText(text string) error {
	mu.Lock()
	suppressJob = true
	lastText = text
	mu.Unlock()

	err := writeSystemClipboard(text)

	go func() {
		time.Sleep(500 * time.Millisecond)
		mu.Lock()
		suppressJob = false
		mu.Unlock()
	}()

	return err
}

// GetText reads the system clipboard
func GetText() (string, error) {
	return readSystemClipboard()
}

// StartWatcher polls clipboard every 300ms and invokes callback if changed
func StartWatcher(pollInterval time.Duration, onChange func(text string)) {
	go func() {
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for range ticker.C {
			mu.Lock()
			suppressed := suppressJob
			mu.Unlock()

			if suppressed {
				continue
			}

			current, err := readSystemClipboard()
			if err != nil || current == "" {
				continue
			}

			mu.Lock()
			if current != lastText {
				lastText = current
				mu.Unlock()
				onChange(current)
			} else {
				mu.Unlock()
			}
		}
	}()
}
