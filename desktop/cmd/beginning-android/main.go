package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rajkotpodman/beginning-android/desktop/internal/clipboard"
	"github.com/rajkotpodman/beginning-android/desktop/internal/config"
	"github.com/rajkotpodman/beginning-android/desktop/internal/network"
	"github.com/rajkotpodman/beginning-android/desktop/internal/relay"
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

	// 1. Start Embedded WebSocket Relay Server on port 8080
	relayServer := relay.NewServer(8080, cfg.PCName, cfg.PairCode, "beginning-android-release.apk")
	if err := relayServer.Start(); err != nil {
		log.Printf("[Relay] Warning: Could not start embedded relay server (may already be running): %v", err)
	} else {
		log.Println("[Relay] Embedded WebSocket Relay Server started successfully.")
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
	app := tray.NewTrayApp(cfg, client, relayServer)

	// Trap OS termination signals (Ctrl+C, SIGTERM)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("[Beginning Android] Received shutdown signal. Exiting gracefully...")
		client.Stop()
		if relayServer != nil {
			relayServer.Stop()
		}
		os.Exit(0)
	}()

	app.Run()

	// If systray exits (e.g. headless/non-interactive), stay alive continuously
	log.Println("[Beginning Android] Background sync and relay server running live.")
	select {}
}
