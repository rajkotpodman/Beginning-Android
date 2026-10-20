package relay

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local Wi-Fi / hotspot pairing
	},
}

type RoomManager struct {
	mu    sync.RWMutex
	rooms map[string]map[*websocket.Conn]bool
}

func NewRoomManager() *RoomManager {
	return &RoomManager{
		rooms: make(map[string]map[*websocket.Conn]bool),
	}
}

func (rm *RoomManager) Join(room string, conn *websocket.Conn) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.rooms[room]; !exists {
		rm.rooms[room] = make(map[*websocket.Conn]bool)
	}
	rm.rooms[room][conn] = true
	log.Printf("[Relay] Client joined room '%s' (total in room: %d)", room, len(rm.rooms[room]))
}

func (rm *RoomManager) Leave(room string, conn *websocket.Conn) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if conns, exists := rm.rooms[room]; exists {
		delete(conns, conn)
		log.Printf("[Relay] Client left room '%s' (remaining: %d)", room, len(conns))
		if len(conns) == 0 {
			delete(rm.rooms, room)
		}
	}
}

func (rm *RoomManager) Broadcast(room string, sender *websocket.Conn, msg []byte) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	conns, exists := rm.rooms[room]
	if !exists {
		return
	}

	for conn := range conns {
		if conn != sender {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("[Relay] Forward error in room '%s': %v", room, err)
			}
		}
	}
}

func (rm *RoomManager) Stats() (int, int) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	totalRooms := len(rm.rooms)
	totalConns := 0
	for _, c := range rm.rooms {
		totalConns += len(c)
	}
	return totalRooms, totalConns
}

type Server struct {
	port       int
	pcName     string
	pairCode   string
	apkPath    string
	manager    *RoomManager
	httpServer *http.Server
	udpConn    *net.UDPConn
	stopChan   chan struct{}
	mu         sync.Mutex

	latestVoiceText string
	latestVoicePeer string
	latestVoiceTime int64
	latestClipboard string
	connectedPeer   string
	sseClients      map[chan string]bool
	sseMu           sync.Mutex
	OnSendClipboard func(text string)
}

func NewServer(port int, pcName, pairCode, apkPath string) *Server {
	if port <= 0 {
		port = 8080
	}
	return &Server{
		port:       port,
		pcName:     pcName,
		pairCode:   pairCode,
		apkPath:    apkPath,
		manager:    NewRoomManager(),
		stopChan:   make(chan struct{}),
		sseClients: make(map[chan string]bool),
	}
}

func (s *Server) UpdatePairCode(code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairCode = code
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleHealth)
	mux.HandleFunc("/apk", s.handleApkDownload)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/ws/", s.handleWebSocket)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/api/clipboard", s.handleApiClipboard)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Minute,
		WriteTimeout: 10 * time.Minute,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind relay server to %s: %w", addr, err)
	}

	log.Printf("[Relay] Embedded WebSocket Relay Server listening on http://%s", addr)
	primaryIP := GetPrimaryLocalIP()
	if primaryIP != "" {
		log.Printf("[Relay] Mobile Endpoint: ws://%s:%d/ws", primaryIP, s.port)
	}

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[Relay] HTTP server error: %v", err)
		}
	}()

	go s.startUDPDiscovery()

	return nil
}

func (s *Server) Stop() {
	s.mu.Lock()
	select {
	case <-s.stopChan:
		s.mu.Unlock()
		return
	default:
		close(s.stopChan)
	}
	s.mu.Unlock()

	if s.httpServer != nil {
		s.httpServer.Close()
	}
	if s.udpConn != nil {
		s.udpConn.Close()
	}
	log.Printf("[Relay] Embedded Relay Server stopped.")
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	msgChan := make(chan string, 16)
	s.sseMu.Lock()
	s.sseClients[msgChan] = true
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, msgChan)
		close(msgChan)
		s.sseMu.Unlock()
	}()

	// Push current initial state
	s.mu.Lock()
	peer := s.connectedPeer
	vText := s.latestVoiceText
	vPeer := s.latestVoicePeer
	clip := s.latestClipboard
	code := s.pairCode
	s.mu.Unlock()

	initObj := map[string]interface{}{
		"type":           "init",
		"connected_peer": peer,
		"voice_text":     vText,
		"voice_peer":     vPeer,
		"clipboard":      clip,
		"pair_code":      code,
	}
	if b, err := json.Marshal(initObj); err == nil {
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	notify := r.Context().Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case msg := <-msgChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (s *Server) broadcastSSE(msg map[string]interface{}) {
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	payload := string(b)

	s.sseMu.Lock()
	defer s.sseMu.Unlock()
	for ch := range s.sseClients {
		select {
		case ch <- payload:
		default:
		}
	}
}

func (s *Server) BroadcastLiveVoice(peerName, text string) {
	s.mu.Lock()
	s.latestVoicePeer = peerName
	s.latestVoiceText = text
	s.latestVoiceTime = time.Now().UnixMilli()
	if peerName != "" {
		s.connectedPeer = peerName
	}
	s.mu.Unlock()

	s.broadcastSSE(map[string]interface{}{
		"type":           "voice",
		"peer":           peerName,
		"text":           text,
		"connected_peer": peerName,
		"timestamp":      time.Now().UnixMilli(),
	})
}

func (s *Server) BroadcastLiveClipboard(text string) {
	s.mu.Lock()
	s.latestClipboard = text
	s.mu.Unlock()

	s.broadcastSSE(map[string]interface{}{
		"type":      "clipboard",
		"text":      text,
		"timestamp": time.Now().UnixMilli(),
	})
}

func (s *Server) SetConnectedPeer(peerName string) {
	s.mu.Lock()
	s.connectedPeer = peerName
	s.mu.Unlock()

	s.broadcastSSE(map[string]interface{}{
		"type":           "peer",
		"connected_peer": peerName,
	})
}

func (s *Server) handleApiClipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid json", http.StatusBadRequest)
		return
	}

	s.BroadcastLiveClipboard(req.Text)
	if s.OnSendClipboard != nil {
		s.OnSendClipboard(req.Text)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Relay] Upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Extract room from path (e.g. /ws/lunar-willow-drift-...) or query (?room=...)
	path := strings.TrimPrefix(r.URL.Path, "/ws")
	room := strings.Trim(path, "/")
	if room == "" {
		room = r.URL.Query().Get("room")
	}
	if room == "" {
		s.mu.Lock()
		room = s.pairCode
		s.mu.Unlock()
	}

	s.manager.Join(room, conn)
	defer s.manager.Leave(room, conn)

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if messageType == websocket.TextMessage {
			var envelope map[string]interface{}
			if err := json.Unmarshal(p, &envelope); err == nil {
				envType, _ := envelope["type"].(string)

				if envType == "ping" {
					pong := map[string]interface{}{
						"type": "pong",
						"time": time.Now().UnixMilli(),
					}
					pongBytes, _ := json.Marshal(pong)
					_ = conn.WriteMessage(websocket.TextMessage, pongBytes)
					continue
				}

				if envType == "pair" {
					if targetRoom, ok := envelope["room"].(string); ok && targetRoom != "" && targetRoom != room {
						s.manager.Leave(room, conn)
						room = targetRoom
						s.manager.Join(room, conn)
					}
				}
			}

			s.manager.Broadcast(room, conn, p)
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	rooms, conns := s.manager.Stats()
	primaryIP := GetPrimaryLocalIP()
	allIPs := GetAllLocalIPs()

	s.mu.Lock()
	code := s.pairCode
	peer := s.connectedPeer
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":             "healthy",
		"service":            "beginning-android",
		"pc_name":            s.pcName,
		"pair_code":          code,
		"primary_ip":         primaryIP,
		"all_ips":            allIPs,
		"port":               s.port,
		"ws_url":             fmt.Sprintf("ws://%s:%d/ws", primaryIP, s.port),
		"active_rooms":       rooms,
		"active_connections": conns,
		"connected_peer":     peer,
	})
}

func (s *Server) handleApkDownload(w http.ResponseWriter, r *http.Request) {
	possiblePaths := []string{
		s.apkPath,
		"beginning-android-release.apk",
		filepath.Join("dist", "beginning-android-release.apk"),
		filepath.Join("..", "dist", "beginning-android-release.apk"),
		filepath.Join("android", "app", "build", "outputs", "apk", "release", "app-release.apk"),
	}

	var foundPath string
	for _, p := range possiblePaths {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				foundPath = p
				break
			}
		}
	}

	if foundPath == "" {
		http.Error(w, "Android APK artifact not found on PC. Please compile it first.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", "attachment; filename=\"beginning-android-release.apk\"")
	http.ServeFile(w, r, foundPath)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/pair" {
		http.NotFound(w, r)
		return
	}

	primaryIP := GetPrimaryLocalIP()
	s.mu.Lock()
	code := s.pairCode
	peer := s.connectedPeer
	voiceText := s.latestVoiceText
	clipText := s.latestClipboard
	s.mu.Unlock()

	wsURL := fmt.Sprintf("ws://%s:%d/ws", primaryIP, s.port)
	rooms, conns := s.manager.Stats()

	data := struct {
		PCName        string
		PairCode      string
		WsURL         string
		PrimaryIP     string
		Port          int
		ActiveRooms   int
		ActiveConns   int
		ConnectedPeer string
		VoiceText     string
		ClipboardText string
	}{
		PCName:        s.pcName,
		PairCode:      code,
		WsURL:         wsURL,
		PrimaryIP:     primaryIP,
		Port:          s.port,
		ActiveRooms:   rooms,
		ActiveConns:   conns,
		ConnectedPeer: peer,
		VoiceText:     voiceText,
		ClipboardText: clipText,
	}

	tmpl := template.Must(template.New("dashboard").Parse(dashboardHTML))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = tmpl.Execute(w, data)
}

func (s *Server) startUDPDiscovery() {
	addr, err := net.ResolveUDPAddr("udp", ":8082")
	if err != nil {
		log.Printf("[Discovery] Resolve UDP error: %v", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("[Discovery] Listen UDP 8082 error (discovery may be unavailable): %v", err)
		return
	}
	s.udpConn = conn

	// Periodic beacon goroutine
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()

		broadcastAddr, _ := net.ResolveUDPAddr("udp", "255.255.255.255:8082")

		for {
			select {
			case <-s.stopChan:
				return
			case <-ticker.C:
				primaryIP := GetPrimaryLocalIP()
				if primaryIP != "" && broadcastAddr != nil {
					s.mu.Lock()
					code := s.pairCode
					s.mu.Unlock()

					msg := fmt.Sprintf("BEGINNING_ANDROID_BEACON|ws://%s:%d/ws|%s|%s", primaryIP, s.port, s.pcName, code)
					_, _ = conn.WriteTo([]byte(msg), broadcastAddr)
				}
			}
		}
	}()

	// Incoming discovery listener
	buf := make([]byte, 1024)
	for {
		select {
		case <-s.stopChan:
			return
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, remoteAddr, err := conn.ReadFrom(buf)
		if err != nil {
			continue
		}

		query := string(buf[:n])
		if strings.HasPrefix(query, "DISCOVER_BEGINNING_ANDROID") || strings.HasPrefix(query, "BEGINNING_ANDROID_PING") {
			primaryIP := GetPrimaryLocalIP()
			s.mu.Lock()
			code := s.pairCode
			s.mu.Unlock()

			reply := fmt.Sprintf("BEGINNING_ANDROID_PONG|ws://%s:%d/ws|%s|%s", primaryIP, s.port, s.pcName, code)
			_, _ = conn.WriteTo([]byte(reply), remoteAddr)
		}
	}
}

func GetAllLocalIPs() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips
}

func GetPrimaryLocalIP() string {
	all := GetAllLocalIPs()
	var bestIP string
	for _, ip := range all {
		if strings.HasPrefix(ip, "192.168.75.") || strings.HasPrefix(ip, "192.168.93.") {
			continue // Skip VMware adapters
		}
		if strings.HasPrefix(ip, "10.") {
			return ip // Top priority: Mobile Hotspot subnet
		}
		if strings.HasPrefix(ip, "192.168.") {
			bestIP = ip
		} else if strings.HasPrefix(ip, "172.") && bestIP == "" {
			bestIP = ip
		}
	}
	if bestIP != "" {
		return bestIP
	}
	if len(all) > 0 {
		return all[0]
	}
	return "127.0.0.1"
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Beginning Android - Live Hub & Voice Conversion</title>
<style>
  :root {
    --bg: #0B1120;
    --card: #1E293B;
    --card-inner: #0F172A;
    --accent: #0D9488;
    --accent-hover: #14B8A6;
    --gemini-cyan: #38BDF8;
    --gemini-purple: #C084FC;
    --gemini-indigo: #818CF8;
    --text: #F8FAFC;
    --muted: #94A3B8;
    --border: rgba(255,255,255,0.08);
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    display: flex;
    justify-content: center;
    align-items: flex-start;
    min-height: 100vh;
    padding: 30px 16px 60px;
  }
  .container {
    max-width: 680px;
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }
  .card {
    background: var(--card);
    border-radius: 18px;
    padding: 24px;
    box-shadow: 0 20px 40px rgba(0,0,0,0.5);
    border: 1px solid var(--border);
  }
  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
  }
  .badge {
    background: var(--accent);
    color: white;
    font-size: 11px;
    font-weight: 700;
    padding: 5px 12px;
    border-radius: 20px;
    letter-spacing: 0.5px;
    text-transform: uppercase;
  }
  .badge-live {
    background: #0284C7;
    animation: pulse 1.8s infinite;
  }
  @keyframes pulse {
    0% { box-shadow: 0 0 0 0 rgba(56, 189, 248, 0.6); }
    70% { box-shadow: 0 0 0 10px rgba(56, 189, 248, 0); }
    100% { box-shadow: 0 0 0 0 rgba(56, 189, 248, 0); }
  }
  h1 { font-size: 22px; font-weight: 800; color: #fff; letter-spacing: -0.3px; }
  h2 { font-size: 17px; font-weight: 700; color: #fff; margin-bottom: 6px; display: flex; align-items: center; gap: 8px; }
  p.desc { font-size: 13px; color: var(--muted); margin-bottom: 16px; line-height: 1.5; }

  /* Live Voice Card */
  .voice-card {
    border: 1px solid rgba(56, 189, 248, 0.3);
    background: linear-gradient(180deg, #162032 0%, #111827 100%);
  }
  .canvas-box {
    width: 100%;
    height: 70px;
    border-radius: 12px;
    background: #070B14;
    overflow: hidden;
    position: relative;
    border: 1px solid rgba(255,255,255,0.06);
    margin-bottom: 12px;
  }
  canvas {
    width: 100%;
    height: 100%;
    display: block;
  }
  .transcription-box {
    background: var(--card-inner);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 16px;
    font-size: 15px;
    line-height: 1.6;
    color: #F1F5F9;
    min-height: 90px;
    max-height: 220px;
    overflow-y: auto;
    word-break: break-word;
    user-select: text;
  }
  .transcription-empty {
    color: #64748B;
    font-style: italic;
  }
  .voice-controls {
    display: flex;
    gap: 10px;
    margin-top: 12px;
  }
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    background: var(--accent);
    color: #fff;
    font-size: 14px;
    font-weight: 600;
    padding: 11px 16px;
    border-radius: 10px;
    text-decoration: none;
    border: none;
    cursor: pointer;
    transition: all 0.2s;
  }
  .btn:hover { background: var(--accent-hover); transform: translateY(-1px); }
  .btn-primary {
    background: linear-gradient(135deg, #0284C7 0%, #0D9488 100%);
    flex: 1;
  }
  .btn-outline {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--muted);
  }
  .btn-outline:hover {
    background: rgba(255,255,255,0.05);
    color: #fff;
  }
  .toast-banner {
    margin-top: 12px;
    padding: 10px 14px;
    background: rgba(13, 148, 136, 0.15);
    border: 1px solid rgba(13, 148, 136, 0.3);
    border-radius: 8px;
    font-size: 12px;
    color: #5EEAD4;
    line-height: 1.4;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  /* Info Fields */
  .grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    margin-bottom: 12px;
  }
  @media (max-width: 580px) {
    .grid { grid-template-columns: 1fr; }
  }
  .field {
    background: var(--card-inner);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 12px 14px;
  }
  .label { font-size: 11px; font-weight: 600; text-transform: uppercase; color: var(--muted); margin-bottom: 4px; letter-spacing: 0.5px; }
  .value { font-size: 14px; font-weight: 600; font-family: monospace; color: var(--gemini-cyan); word-break: break-all; }

  /* Guide Step Box */
  .guide-box {
    background: var(--card-inner);
    border-radius: 12px;
    padding: 16px;
    border: 1px solid var(--border);
  }
  .guide-step {
    display: flex;
    gap: 12px;
    margin-bottom: 12px;
    font-size: 13px;
    line-height: 1.5;
    color: #CBD5E1;
  }
  .guide-step:last-child { margin-bottom: 0; }
  .step-num {
    background: #334155;
    color: #38BDF8;
    font-weight: 700;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    flex-shrink: 0;
    margin-top: 2px;
  }

  /* Clipboard Input */
  .clip-row {
    display: flex;
    gap: 8px;
    margin-top: 10px;
  }
  .clip-input {
    flex: 1;
    background: var(--card-inner);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 10px 12px;
    color: #fff;
    font-size: 14px;
  }
  .clip-input:focus { outline: none; border-color: var(--accent); }
</style>
</head>
<body>

<div class="container">
  <!-- Card 1: Live Gemini Voice-to-Text on PC Screen -->
  <div class="card voice-card">
    <div class="header">
      <h2>🎙️ Live Gemini Voice Conversion</h2>
      <span class="badge badge-live" id="voiceStatusBadge">READY</span>
    </div>
    <p class="desc">
      Speak on your phone. Spoken words convert and stream right here in real time with live Gemini wave graphs.
    </p>

    <!-- 60 FPS HTML5 Canvas Gemini Waveform Graph -->
    <div class="canvas-box">
      <canvas id="waveCanvas"></canvas>
    </div>

    <!-- Live Transcribed Text -->
    <div class="transcription-box" id="transcriptionBox">
      <span class="transcription-empty" id="placeholderText">
        Tap "Start Continuous Voice" on your phone and speak. Your speech will stream live into this box...
      </span>
    </div>

    <div class="voice-controls">
      <button class="btn btn-primary" onclick="copySpokenText()">
        📋 Copy Spoken Text to PC Clipboard
      </button>
      <button class="btn btn-outline" onclick="clearSpokenText()">
        Clear
      </button>
    </div>

    <div class="toast-banner">
      <span>⚡</span>
      <div>
        <strong>Instant Windows Clipboard Sync:</strong> Any words spoken on phone are automatically placed on your PC clipboard. Press <code>Ctrl + V</code> in Notepad, Word, Chrome, WhatsApp, or any PC app to paste instantly!
      </div>
    </div>
  </div>

  <!-- Card 2: PC-Mobile Clipboard Bridge -->
  <div class="card">
    <div class="header">
      <h2>📋 Real-Time Shared Clipboard</h2>
      <span class="badge" style="background: #334155;" id="peerStatusBadge">
        {{if .ConnectedPeer}}{{.ConnectedPeer}}{{else}}Waiting for Phone{{end}}
      </span>
    </div>
    <p class="desc">300ms bi-directional clipboard sync between your Windows PC and Android phone.</p>

    <div class="field" style="margin-bottom: 10px;">
      <div class="label">Current Synced Text</div>
      <div class="value" id="currentClipboard" style="color: #A7F3D0; font-family: sans-serif; font-weight: normal; font-size: 14px;">
        {{if .ClipboardText}}{{.ClipboardText}}{{else}}No clipboard text synchronized yet{{end}}
      </div>
    </div>

    <div class="clip-row">
      <input type="text" id="pcTextInput" class="clip-input" placeholder="Type text to send to phone clipboard...">
      <button class="btn" onclick="sendToPhoneClipboard()">📲 Send to Phone</button>
    </div>
  </div>

  <!-- Card 3: Pair Info & APK Download -->
  <div class="card">
    <div class="header">
      <h1>Beginning Android</h1>
      <span class="badge">Relay Ready</span>
    </div>
    <p class="desc">Connection endpoints for your mobile hotspot / Wi-Fi.</p>

    <div class="grid">
      <div class="field">
        <div class="label">6-Word Pair Code</div>
        <div class="value" id="pairCode">{{.PairCode}}</div>
      </div>
      <div class="field">
        <div class="label">Connected PC</div>
        <div class="value" style="color: #A7F3D0;">{{.PCName}} ({{.PrimaryIP}})</div>
      </div>
    </div>

    <div class="field" style="margin-bottom: 14px;">
      <div class="label">Relay Server URL</div>
      <div class="value" id="relayUrl">{{.WsURL}}</div>
    </div>

    <div style="display: flex; gap: 10px;">
      <button class="btn btn-outline" style="flex: 1;" onclick="copyPairCode()">📋 Copy Pair Code</button>
      <a class="btn" style="flex: 1; text-align: center;" href="/apk" download>📲 Download Android APK</a>
    </div>
  </div>

  <!-- Card 4: How To Use Guide -->
  <div class="card">
    <h2>📖 How to Use Live Voice Conversion on PC</h2>
    <div class="guide-box" style="margin-top: 12px;">
      <div class="guide-step">
        <span class="step-num">1</span>
        <div><strong>Open Beginning Android on Phone:</strong> Confirm it shows <span style="color: #10B981; font-weight: bold;">Connected</span> in green.</div>
      </div>
      <div class="guide-step">
        <span class="step-num">2</span>
        <div><strong>Tap "Start Continuous Voice":</strong> Speak naturally in any language for as long as you want without stopping.</div>
      </div>
      <div class="guide-step">
        <span class="step-num">3</span>
        <div><strong>Watch Live Conversion on PC:</strong> Words appear live on this screen in real time with active audio wave graphs.</div>
      </div>
      <div class="guide-step">
        <span class="step-num">4</span>
        <div><strong>Press Ctrl + V in any PC app:</strong> The spoken text is automatically copied to your Windows clipboard ready to paste anywhere!</div>
      </div>
    </div>
  </div>
</div>

<script>
// --- Live Server-Sent Events (SSE) Stream ---
let liveVoiceText = {{if .VoiceText}}{{.VoiceText}}{{else}}""{{end}};
let isVoiceActive = false;
let waveIntensity = 0.08;
let lastSpeechTime = 0;

const transcriptionBox = document.getElementById('transcriptionBox');
const voiceStatusBadge = document.getElementById('voiceStatusBadge');
const peerStatusBadge = document.getElementById('peerStatusBadge');
const currentClipboard = document.getElementById('currentClipboard');

function updateVoiceDisplay(text, peer) {
  if (!text || text.trim() === "") return;
  liveVoiceText = text;
  transcriptionBox.innerHTML = '<span style="color:#F8FAFC; font-weight:500;">' + escapeHtml(text) + '</span>';
  transcriptionBox.scrollTop = transcriptionBox.scrollHeight;
  voiceStatusBadge.innerText = (peer ? peer : "PHONE") + " SPEAKING";
  voiceStatusBadge.style.background = "#0D9488";
  isVoiceActive = true;
  waveIntensity = 0.75;
  lastSpeechTime = Date.now();
}

function escapeHtml(str) {
  return str.replace(/[&<>"']/g, function(m) {
    return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' }[m];
  });
}

function connectSSE() {
  const evtSource = new EventSource('/events');
  evtSource.onmessage = function(e) {
    try {
      const data = JSON.parse(e.data);
      if (data.type === 'init') {
        if (data.connected_peer) {
          peerStatusBadge.innerText = "🟢 " + data.connected_peer;
          peerStatusBadge.style.background = "#0D9488";
        }
        if (data.voice_text) {
          updateVoiceDisplay(data.voice_text, data.voice_peer);
        }
        if (data.clipboard) {
          currentClipboard.innerText = data.clipboard;
        }
      } else if (data.type === 'voice') {
        updateVoiceDisplay(data.text, data.peer);
      } else if (data.type === 'clipboard') {
        currentClipboard.innerText = data.text;
      } else if (data.type === 'peer') {
        peerStatusBadge.innerText = "🟢 " + data.connected_peer;
        peerStatusBadge.style.background = "#0D9488";
      }
    } catch(err) {
      console.error(err);
    }
  };
  evtSource.onerror = function() {
    evtSource.close();
    setTimeout(connectSSE, 2000);
  };
}
connectSSE();

// --- 60 FPS HTML5 Canvas Gemini Waveform Visualizer ---
const canvas = document.getElementById('waveCanvas');
const ctx = canvas.getContext('2d');
let phase = 0;

function resizeCanvas() {
  const rect = canvas.getBoundingClientRect();
  canvas.width = rect.width * window.devicePixelRatio;
  canvas.height = rect.height * window.devicePixelRatio;
}
window.addEventListener('resize', resizeCanvas);
resizeCanvas();

function drawWave(ctx, w, h, centerY, amp, speed, freq, color, offset) {
  ctx.beginPath();
  ctx.lineWidth = 3 * window.devicePixelRatio;
  ctx.strokeStyle = color;
  ctx.lineCap = 'round';

  for (let x = 0; x <= w; x += 3) {
    const normX = x / w;
    const hann = Math.sin(Math.PI * normX);
    const y = centerY + Math.sin(normX * Math.PI * freq + phase * speed + offset) * amp * hann;
    if (x === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  }
  ctx.stroke();
}

function renderWaves() {
  requestAnimationFrame(renderWaves);
  if (!canvas.width || !canvas.height) return;

  const w = canvas.width;
  const h = canvas.height;
  const centerY = h / 2;

  ctx.clearRect(0, 0, w, h);

  // Decay wave intensity smoothly
  if (Date.now() - lastSpeechTime > 2000) {
    waveIntensity += (0.08 - waveIntensity) * 0.05;
    if (isVoiceActive && waveIntensity < 0.12) {
      isVoiceActive = false;
      voiceStatusBadge.innerText = "READY";
      voiceStatusBadge.style.background = "#0284C7";
    }
  } else {
    waveIntensity += (0.75 - waveIntensity) * 0.1;
  }

  phase += 0.04;
  const maxAmp = (h * 0.4) * waveIntensity;

  drawWave(ctx, w, h, centerY, maxAmp, 1.0, 3.5, '#38BDF8', 0);
  drawWave(ctx, w, h, centerY, maxAmp * 0.8, 1.3, 4.2, '#C084FC', 1.2);
  drawWave(ctx, w, h, centerY, maxAmp * 0.65, 0.8, 2.8, '#818CF8', 2.4);
  drawWave(ctx, w, h, centerY, maxAmp * 0.5, 1.1, 5.0, '#14B8A6', 3.6);
}
renderWaves();

// --- Interactive Actions ---
function copySpokenText() {
  if (!liveVoiceText || liveVoiceText.trim() === "") {
    alert("No spoken text yet. Speak on your phone first!");
    return;
  }
  navigator.clipboard.writeText(liveVoiceText).then(() => {
    alert("✅ Spoken text copied to PC clipboard!");
  });
}

function clearSpokenText() {
  liveVoiceText = "";
  transcriptionBox.innerHTML = '<span class="transcription-empty">Cleared. Speak on your phone to transcribe new text...</span>';
}

function copyPairCode() {
  const code = document.getElementById('pairCode').innerText.trim();
  navigator.clipboard.writeText(code).then(() => {
    alert("Pair code copied: " + code);
  });
}

function sendToPhoneClipboard() {
  const input = document.getElementById('pcTextInput');
  const text = input.value.trim();
  if (!text) return;

  fetch('/api/clipboard', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text: text })
  }).then(res => res.json()).then(() => {
    input.value = "";
    currentClipboard.innerText = text;
    alert("✅ Sent to phone clipboard!");
  }).catch(err => {
    alert("Error sending text: " + err);
  });
}
</script>
</body>
</html>`
