package tray

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/energye/systray"
	"github.com/rajkotpodman/beginning-android/desktop/internal/config"
	"github.com/rajkotpodman/beginning-android/desktop/internal/network"
)

type TrayApp struct {
	cfg        *config.Config
	client     *network.Client
	mStatus    *systray.MenuItem
	mPair      *systray.MenuItem
	mScreens   *systray.MenuItem
	mReceived  *systray.MenuItem
	mQuit      *systray.MenuItem
}

func NewTrayApp(cfg *config.Config, client *network.Client) *TrayApp {
	return &TrayApp{
		cfg:    cfg,
		client: client,
	}
}

func (t *TrayApp) Run() {
	systray.Run(t.onReady, t.onExit)
}

func (t *TrayApp) onReady() {
	systray.SetTitle("Beginning Android")
	systray.SetTooltip("Beginning Android - Live Sync")
	systray.SetIcon(getIconBytes())

	// Menu items required by spec:
	// 1. Status: Connected/Disconnected
	t.mStatus = systray.AddMenuItem("Status: Initializing...", "Current sync connection status")
	t.mStatus.Disable()

	systray.AddSeparator()

	// 2. Pair (enter 6-word code)
	t.mPair = systray.AddMenuItem("Pair Device...", "Pair with Android using 6-word code")

	// 3. Open Screenshots folder
	t.mScreens = systray.AddMenuItem("Open Screenshots folder", "Open auto-transferred screenshots directory")

	// 4. Open Received files folder
	t.mReceived = systray.AddMenuItem("Open Received files folder", "Open received files directory")

	systray.AddSeparator()

	// 5. Quit
	t.mQuit = systray.AddMenuItem("Quit", "Exit Beginning Android")

	// Hook network status updates to menu label
	t.client.OnStatusChange = func(status string) {
		t.mStatus.SetTitle(fmt.Sprintf("Status: %s", status))
		systray.SetTooltip(fmt.Sprintf("Beginning Android: %s", status))
	}

	t.client.OnPeerConnect = func(peerName string) {
		t.mStatus.SetTitle(fmt.Sprintf("Connected to: %s", peerName))
	}

	// Handle menu clicks via Click callback
	t.mPair.Click(func() {
		t.handlePairClick()
	})

	t.mScreens.Click(func() {
		dir, err := config.GetScreenshotsDir()
		if err == nil {
			openFolder(dir)
		}
	})

	t.mReceived.Click(func() {
		dir, err := config.GetReceivedDir()
		if err == nil {
			openFolder(dir)
		}
	})

	t.mQuit.Click(func() {
		systray.Quit()
	})
}

func (t *TrayApp) onExit() {
	log.Println("[Tray] Exiting Beginning Android...")
	t.client.Stop()
}

func (t *TrayApp) handlePairClick() {
	go func() {
		newCode := promptForPairCode(t.cfg.PairCode)
		if newCode != "" && newCode != t.cfg.PairCode {
			t.cfg.PairCode = newCode
			if err := config.SaveConfig(t.cfg); err != nil {
				log.Printf("[Config] Error saving config: %v", err)
			}
			t.client.UpdatePairCode(newCode)
			log.Printf("[Tray] Pair code updated to: %s", newCode)
		}
	}()
}

func promptForPairCode(currentCode string) string {
	// Try native input dialog on Windows via PowerShell
	if runtime.GOOS == "windows" {
		psScript := fmt.Sprintf(`
Add-Type -AssemblyName Microsoft.VisualBasic
[Microsoft.VisualBasic.Interaction]::InputBox("Enter the 6-word pair code shown on your Android device:", "Beginning Android Pairing", "%s")
`, currentCode)

		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
		out, err := cmd.Output()
		if err == nil {
			result := strings.TrimSpace(string(out))
			if result != "" {
				return result
			}
		}
	} else {
		// Try zenity on Linux
		if zenityPath, err := exec.LookPath("zenity"); err == nil {
			cmd := exec.Command(zenityPath, "--entry", "--title=Beginning Android Pairing",
				"--text=Enter 6-word pair code:", "--entry-text="+currentCode)
			out, err := cmd.Output()
			if err == nil {
				result := strings.TrimSpace(string(out))
				if result != "" {
					return result
				}
			}
		}
	}

	// Fallback: prompt in terminal if available
	fmt.Printf("\nEnter 6-word pair code (current: %s): ", currentCode)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func openFolder(path string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("explorer.exe", path).Start()
	case "linux":
		exec.Command("xdg-open", path).Start()
	}
}

// Minimal 16x16 ICO / PNG icon embedded bytes
func getIconBytes() []byte {
	// 16x16 teal square icon in PNG format
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0xf3, 0xff, 0x61, 0x00, 0x00, 0x00,
		0x24, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x60, 0xc0, 0x0a, 0x82,
		0x81, 0x81, 0x01, 0x04, 0x4c, 0xa1, 0xd0, 0x24, 0x42, 0x18, 0x18, 0x30,
		0x98, 0x42, 0x08, 0x82, 0x10, 0x30, 0x58, 0x02, 0x02, 0x0c, 0x00, 0x71,
		0x40, 0x08, 0x8d, 0x4c, 0xdb, 0x1d, 0x93, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}
