package network

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rajkotpodman/beginning-android/desktop/internal/clipboard"
	"github.com/rajkotpodman/beginning-android/desktop/internal/config"
	"github.com/rajkotpodman/beginning-android/desktop/internal/crypto"
)

type Envelope struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Nonce   string `json:"nonce"`
}

type Client struct {
	cfg       *config.Config
	aesKey    []byte
	conn      *websocket.Conn
	mu        sync.Mutex
	isRunning bool

	// File receiving state
	currentFile *os.File

	OnStatusChange func(status string)
	OnPeerConnect  func(peerName string)
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		cfg:    cfg,
		aesKey: crypto.DeriveKey(cfg.PairCode),
	}
}

func (c *Client) UpdatePairCode(pairCode string) {
	c.mu.Lock()
	c.cfg.PairCode = pairCode
	c.aesKey = crypto.DeriveKey(pairCode)
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		conn.Close()
	}
}

func (c *Client) Start() {
	c.mu.Lock()
	if c.isRunning {
		c.mu.Unlock()
		return
	}
	c.isRunning = true
	c.mu.Unlock()

	go c.connectLoop()
}

func (c *Client) Stop() {
	c.mu.Lock()
	c.isRunning = false
	if c.conn != nil {
		c.conn.Close()
	}
	if c.currentFile != nil {
		c.currentFile.Close()
		c.currentFile = nil
	}
	c.mu.Unlock()
}

func (c *Client) connectLoop() {
	retryDelay := 1 * time.Second

	for {
		c.mu.Lock()
		running := c.isRunning
		pairCode := c.cfg.PairCode
		relayURL := c.cfg.RelayURL
		c.mu.Unlock()

		if !running {
			return
		}

		if pairCode == "" {
			if c.OnStatusChange != nil {
				c.OnStatusChange("Not Paired")
			}
			time.Sleep(2 * time.Second)
			continue
		}

		if c.OnStatusChange != nil {
			c.OnStatusChange("Connecting...")
		}

		cleanURL := strings.TrimRight(relayURL, "/")
		roomURL := fmt.Sprintf("%s/%s", cleanURL, pairCode)

		conn, _, err := websocket.DefaultDialer.Dial(roomURL, nil)
		if err != nil {
			log.Printf("[Network] Connection failed: %v. Retrying in %v...", err, retryDelay)
			if c.OnStatusChange != nil {
				c.OnStatusChange("Disconnected")
			}
			time.Sleep(retryDelay)
			if retryDelay < 30*time.Second {
				retryDelay *= 2
			}
			continue
		}

		retryDelay = 1 * time.Second
		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()

		if c.OnStatusChange != nil {
			c.OnStatusChange("Connected")
		}

		c.sendPairAnnouncement()
		c.readLoop(conn)

		c.mu.Lock()
		c.conn = nil
		if c.currentFile != nil {
			c.currentFile.Close()
			c.currentFile = nil
		}
		c.mu.Unlock()

		if c.OnStatusChange != nil {
			c.OnStatusChange("Disconnected")
		}
	}
}

func (c *Client) readLoop(conn *websocket.Conn) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[Network] Read error: %v", err)
			return
		}

		var env Envelope
		if err := json.Unmarshal(message, &env); err != nil {
			continue
		}

		c.handleIncomingEnvelope(&env)
	}
}

func (c *Client) handleIncomingEnvelope(env *Envelope) {
	switch env.Type {
	case "pair":
		decrypted, err := crypto.DecryptString(c.aesKey, env.Payload, env.Nonce)
		if err == nil {
			var meta map[string]interface{}
			if json.Unmarshal([]byte(decrypted), &meta) == nil {
				peerName, _ := meta["device_name"].(string)
				if peerName != "" && c.OnPeerConnect != nil {
					c.OnPeerConnect(peerName)
				}
			}
		}

	case "clipboard":
		text, err := crypto.DecryptString(c.aesKey, env.Payload, env.Nonce)
		if err == nil && text != "" {
			log.Printf("[Clipboard] Received live text (%d chars)", len(text))
			clipboard.SetText(text)
		}

	case "voice_stream":
		text, err := crypto.DecryptString(c.aesKey, env.Payload, env.Nonce)
		if err == nil && text != "" {
			log.Printf("[Voice] Live speech transcription: %s", text)
			clipboard.SetText(text)
		}

	case "file_start", "screenshot_meta":
		decrypted, err := crypto.DecryptString(c.aesKey, env.Payload, env.Nonce)
		if err != nil {
			return
		}
		var meta struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		}
		if json.Unmarshal([]byte(decrypted), &meta) != nil {
			return
		}

		c.setupIncomingFile(meta.Name, env.Type == "screenshot_meta")

	case "file_chunk":
		chunk, err := crypto.Decrypt(c.aesKey, env.Payload, env.Nonce)
		if err != nil || c.currentFile == nil {
			return
		}
		c.currentFile.Write(chunk)

	case "file_end":
		if c.currentFile != nil {
			path := c.currentFile.Name()
			c.currentFile.Close()
			c.currentFile = nil
			log.Printf("[File] Successfully received and saved: %s", path)
		}

	case "ping":
		c.sendRaw("pong", "", "")
	}
}

func (c *Client) setupIncomingFile(name string, isScreenshot bool) {
	if c.currentFile != nil {
		c.currentFile.Close()
		c.currentFile = nil
	}

	var targetDir string
	lowerName := strings.ToLower(name)
	if isScreenshot || strings.Contains(lowerName, "screenshot") {
		targetDir, _ = config.GetScreenshotsDir()
	} else {
		targetDir, _ = config.GetReceivedDir()
	}

	cleanName := filepath.Base(name)
	if cleanName == "" || cleanName == "." {
		cleanName = fmt.Sprintf("file_%d", time.Now().Unix())
	}

	destPath := filepath.Join(targetDir, cleanName)
	f, err := os.Create(destPath)
	if err != nil {
		log.Printf("[File] Error creating destination file: %v", err)
		return
	}
	c.currentFile = f
	log.Printf("[File] Receiving incoming file to: %s", destPath)
}

func (c *Client) sendPairAnnouncement() {
	payload := map[string]interface{}{
		"device_name": c.cfg.PCName,
		"platform":    runtime.GOOS,
	}
	data, _ := json.Marshal(payload)
	c.SendEncrypted("pair", data)
}

func (c *Client) SendClipboard(text string) error {
	return c.SendEncrypted("clipboard", []byte(text))
}

func (c *Client) SendFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	// 1. Send file start
	meta := map[string]interface{}{
		"name": filepath.Base(filePath),
		"size": info.Size(),
	}
	metaBytes, _ := json.Marshal(meta)
	if err := c.SendEncrypted("file_start", metaBytes); err != nil {
		return err
	}

	// 2. Stream chunks (64 KB)
	buf := make([]byte, 64*1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			if err := c.SendEncrypted("file_chunk", buf[:n]); err != nil {
				return err
			}
			time.Sleep(5 * time.Millisecond)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// 3. Send file end
	return c.SendEncrypted("file_end", []byte{})
}

func (c *Client) SendEncrypted(msgType string, data []byte) error {
	payload, nonce, err := crypto.Encrypt(c.aesKey, data)
	if err != nil {
		return err
	}
	return c.sendRaw(msgType, payload, nonce)
}

func (c *Client) sendRaw(msgType string, payload string, nonce string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	env := Envelope{
		Type:    msgType,
		Payload: payload,
		Nonce:   nonce,
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}

	return c.conn.WriteMessage(websocket.TextMessage, data)
}
