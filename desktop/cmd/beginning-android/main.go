package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/rajkotpodman/beginning-android/desktop/internal/clipboard"
	"github.com/rajkotpodman/beginning-android/desktop/internal/config"
	"github.com/rajkotpodman/beginning-android/desktop/internal/network"
	"github.com/rajkotpodman/beginning-android/desktop/internal/tray"
)

func main() {
	// Parse CLI arguments
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "send", "--send-file":
			if len(os.Args) < 3 {
				fmt.Println("Usage: beginning-android send <file_path>")
				os.Exit(1)
			}
			filePath := os.Args[2]
			handleCLISend(filePath)
			return

		case "pair":
			if len(os.Args) < 3 {
				fmt.Println("Usage: beginning-android pair <6-word-code>")
				os.Exit(1)
			}
			handleCLIPair(os.Args[2])
			return

		case "--help", "-h":
			printUsage()
			return
		}
	}

	// Flag support: beginning-android -send <file>
	sendFileFlag := flag.String("send-file", "", "Send a file to paired Android device")
	pairCodeFlag := flag.String("pair", "", "Set 6-word pairing code")
	flag.Parse()

	if *sendFileFlag != "" {
		handleCLISend(*sendFileFlag)
		return
	}
	if *pairCodeFlag != "" {
		handleCLIPair(*pairCodeFlag)
		return
	}

	// Normal System Tray Background Mode
	runTrayApp()
}

func printUsage() {
	fmt.Println("Beginning Android Desktop Client")
	fmt.Println("Usage:")
	fmt.Println("  beginning-android             Launch background sync and system tray")
	fmt.Println("  beginning-android send <file> Send a file directly to paired Android device")
	fmt.Println("  beginning-android pair <code> Configure 6-word pairing code")
}

func handleCLIPair(code string) {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	cfg.PairCode = code
	if err := config.SaveConfig(cfg); err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}
	fmt.Printf("Pair code successfully updated to: %s\n", code)
}

func handleCLISend(filePath string) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		log.Fatalf("Invalid file path: %v", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		log.Fatalf("File not found: %s", absPath)
	}
	if info.IsDir() {
		log.Fatalf("Path is a directory, please select a file: %s", absPath)
	}

	cfg, err := config.LoadConfig()
	if err != nil || cfg.PairCode == "" {
		log.Fatalf("Device not paired yet. Run 'beginning-android pair <code_from_android>' first.")
	}

	fmt.Printf("Connecting to relay and sending: %s (%d bytes)...\n", filepath.Base(absPath), info.Size())

	client := network.NewClient(cfg)
	client.Start()

	// Wait for connection
	connected := make(chan bool)
	client.OnStatusChange = func(status string) {
		if status == "Connected" {
			select {
			case connected <- true:
			default:
			}
		}
	}

	select {
	case <-connected:
		fmt.Println("Connected to relay. Transferring file...")
		if err := client.SendFile(absPath); err != nil {
			log.Fatalf("Error sending file: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
		fmt.Println("File sent successfully!")
		client.Stop()
	case <-time.After(15 * time.Second):
		client.Stop()
		log.Fatalf("Connection timed out. Ensure relay server is running.")
	}
}

func runTrayApp() {
	log.Println("[Beginning Android] Starting desktop client...")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	client := network.NewClient(cfg)

	// Start 300ms Clipboard Watcher
	clipboard.StartWatcher(300*time.Millisecond, func(text string) {
		log.Printf("[Clipboard] Outgoing text copied (%d chars), syncing to Android...", len(text))
		client.SendClipboard(text)
	})

	// Start Network Client
	client.Start()

	// Launch System Tray UI
	app := tray.NewTrayApp(cfg, client)
	app.Run()
}
