package network

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rajkotpodman/beginning-android/desktop/internal/clipboard"
	"github.com/rajkotpodman/beginning-android/desktop/internal/config"
	"github.com/rajkotpodman/beginning-android/desktop/internal/crypto"
)

type Envelope struct {
	Type       string `json:"type"`
	Seq        uint64 `json:"seq,omitempty"`
	ChunkIndex int    `json:"chunk_index,omitempty"`
	PublicKey  string `json:"public_key,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	Platform   string `json:"platform,omitempty"`
	Payload    string `json:"payload,omitempty"`
	Nonce      string `json:"nonce,omitempty"`
}

type Client struct {
	cfg          *config.Config
	pairCodeHash []byte
	pairSeed     []byte
	x25519Priv   *ecdh.PrivateKey
	x25519Pub    []byte

	sessionKey      []byte
	outgoingSeq     uint64
	lastIncomingSeq uint64

	conn        *websocket.Conn
	mu          sync.Mutex
	isRunning   bool
	currentFile *os.File

	OnStatusChange func(status string)
	OnPeerConnect  func(peerName string)
}

func NewClient(cfg *config.Config) *Client {
	pairCodeHash := crypto.PairCodeHash(cfg.PairCode)
	pairSeed := crypto.DerivePairSeed(cfg.PairCode)
	priv, pub, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		log.Printf("[Crypto] Error generating X25519 keypair: %v", err)
	}

	return &Client{
		cfg:          cfg,
		pairCodeHash: pairCodeHash,
		pairSeed:     pairSeed,
		x25519Priv:   priv,
		x25519Pub:    pub,
	}
}

func (c *Client) getActiveKey() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sessionKey) > 0 {
		return c.sessionKey
	}
	return c.pairSeed
}

func (c *Client) UpdatePairCode(pairCode string) {
	c.mu.Lock()
	c.cfg.PairCode = pairCode
	c.pairCodeHash = crypto.PairCodeHash(pairCode)
	c.pairSeed = crypto.DerivePairSeed(pairCode)
	c.sessionKey = nil
	priv, pub, _ := crypto.GenerateX25519KeyPair()
	c.x25519Priv = priv
	c.x25519Pub = pub
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
		c.sessionKey = nil
		c.lastIncomingSeq = 0
		c.mu.Unlock()

		if c.OnStatusChange != nil {
			c.OnStatusChange("Connected")
		}

		// Step 1: Send X25519 Public Key Exchange Announcement
		c.sendKeyExchange()
		c.readLoop(conn)

		c.mu.Lock()
		c.conn = nil
		c.sessionKey = nil
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
	// 1. Key Exchange
	if env.Type == "key_exchange" {
		if env.PublicKey != "" {
			peerPub, err := base64.StdEncoding.DecodeString(env.PublicKey)
			if err == nil {
				c.mu.Lock()
				sharedSecret, err := crypto.ComputeSharedSecret(c.x25519Priv, peerPub)
				if err == nil {
					sessionKey, err := crypto.DeriveSessionKey(sharedSecret, c.pairCodeHash)
					if err == nil {
						c.sessionKey = sessionKey
						log.Printf("[Crypto] E2EE Session Key Established with peer: %s (%s)", env.DeviceName, env.Platform)
					}
				}
				c.mu.Unlock()

				if env.DeviceName != "" && c.OnPeerConnect != nil {
					c.OnPeerConnect(env.DeviceName)
				}
			}
		}
		return
	}

	// 2. Replay Protection: Check sequential message counter
	if env.Seq > 0 {
		c.mu.Lock()
		if env.Seq <= c.lastIncomingSeq {
			c.mu.Unlock()
			log.Printf("[Crypto] Warning: Dropping replayed packet with seq=%d <= last=%d", env.Seq, c.lastIncomingSeq)
			return
		}
		c.lastIncomingSeq = env.Seq
		c.mu.Unlock()
	}

	key := c.getActiveKey()

	switch env.Type {
	case "pair":
		decrypted, err := crypto.DecryptString(key, env.Payload, env.Nonce)
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
		text, err := crypto.DecryptString(key, env.Payload, env.Nonce)
		if err == nil && text != "" {
			log.Printf("[Clipboard] Received live text (%d chars)", len(text))
			clipboard.SetText(text)
		}

	case "voice_stream":
		text, err := crypto.DecryptString(key, env.Payload, env.Nonce)
		if err == nil && text != "" {
			log.Printf("[Voice] Live speech transcription: %s", text)
			clipboard.SetText(text)
		}

	case "screenshot", "screenshot_meta", "screenshot_start", "file_start":
		decrypted, err := crypto.DecryptString(key, env.Payload, env.Nonce)
		if err != nil {
			return
		}
		var meta struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(decrypted), &meta) != nil {
			return
		}

		isScreenshot := env.Type == "screenshot" || env.Type == "screenshot_meta" || env.Type == "screenshot_start" || meta.Type == "screenshot"
		c.setupIncomingFile(meta.Name, isScreenshot)

	case "file_chunk":
		chunk, err := crypto.Decrypt(key, env.Payload, env.Nonce)
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
		c.sendRaw("pong", 0, "", "")
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
		if isScreenshot {
			cleanName = fmt.Sprintf("screenshot_%d.png", time.Now().Unix())
		} else {
			cleanName = fmt.Sprintf("file_%d", time.Now().Unix())
		}
	}

	destPath := filepath.Join(targetDir, cleanName)
	f, err := os.Create(destPath)
	if err != nil {
		log.Printf("[File] Error creating destination file: %v", err)
		return
	}
	c.currentFile = f
	if isScreenshot {
		log.Printf("[Screenshot] Receiving incoming screenshot to: %s", destPath)
	} else {
		log.Printf("[File] Receiving incoming file to: %s", destPath)
	}
}

func (c *Client) sendKeyExchange() {
	env := Envelope{
		Type:       "key_exchange",
		PublicKey:  base64.StdEncoding.EncodeToString(c.x25519Pub),
		DeviceName: c.cfg.PCName,
		Platform:   runtime.GOOS,
	}
	data, _ := json.Marshal(env)

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.WriteMessage(websocket.TextMessage, data)
	}
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

	// 2. Stream 64KB chunks: encrypted separately, sequential counter
	buf := make([]byte, 64*1024)
	chunkIdx := 0
	for {
		n, err := file.Read(buf)
		if n > 0 {
			seq := atomic.AddUint64(&c.outgoingSeq, 1)
			payload, nonce, encErr := crypto.Encrypt(c.getActiveKey(), buf[:n])
			if encErr != nil {
				return encErr
			}
			env := Envelope{
				Type:       "file_chunk",
				Seq:        seq,
				ChunkIndex: chunkIdx,
				Payload:    payload,
				Nonce:      nonce,
			}
			chunkIdx++
			data, _ := json.Marshal(env)
			c.mu.Lock()
			if c.conn != nil {
				c.conn.WriteMessage(websocket.TextMessage, data)
			}
			c.mu.Unlock()
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
	key := c.getActiveKey()
	seq := atomic.AddUint64(&c.outgoingSeq, 1)
	payload, nonce, err := crypto.Encrypt(key, data)
	if err != nil {
		return err
	}
	return c.sendRaw(msgType, seq, payload, nonce)
}

func (c *Client) sendRaw(msgType string, seq uint64, payload string, nonce string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	env := Envelope{
		Type:    msgType,
		Seq:     seq,
		Payload: payload,
		Nonce:   nonce,
	}
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}

	return c.conn.WriteMessage(websocket.TextMessage, data)
}
