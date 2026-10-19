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
}

func NewServer(port int, pcName, pairCode, apkPath string) *Server {
	if port <= 0 {
		port = 8080
	}
	return &Server{
		port:     port,
		pcName:   pcName,
		pairCode: pairCode,
		apkPath:  apkPath,
		manager:  NewRoomManager(),
		stopChan: make(chan struct{}),
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
	s.mu.Unlock()

	wsURL := fmt.Sprintf("ws://%s:%d/ws", primaryIP, s.port)
	rooms, conns := s.manager.Stats()

	data := struct {
		PCName      string
		PairCode    string
		WsURL       string
		PrimaryIP   string
		Port        int
		ActiveRooms int
		ActiveConns int
	}{
		PCName:      s.pcName,
		PairCode:    code,
		WsURL:       wsURL,
		PrimaryIP:   primaryIP,
		Port:        s.port,
		ActiveRooms: rooms,
		ActiveConns: conns,
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
	// Prioritize 10.x.x.x (typical mobile hotspot) or 192.168.x.x (home Wi-Fi)
	// Exclude typical VMware / VirtualBox ranges (192.168.75.x, 192.168.93.x)
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
<title>Beginning Android - Pair Hub</title>
<style>
  :root {
    --bg: #0F172A;
    --card: #1E293B;
    --accent: #0D9488;
    --accent-hover: #14B8A6;
    --text: #F8FAFC;
    --muted: #94A3B8;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    display: flex;
    justify-content: center;
    align-items: center;
    min-height: 100vh;
    padding: 20px;
  }
  .card {
    background: var(--card);
    border-radius: 16px;
    padding: 32px;
    max-width: 520px;
    width: 100%;
    box-shadow: 0 20px 40px rgba(0,0,0,0.4);
    border: 1px solid rgba(255,255,255,0.08);
  }
  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 24px;
  }
  .badge {
    background: var(--accent);
    color: white;
    font-size: 11px;
    font-weight: 700;
    padding: 4px 10px;
    border-radius: 20px;
    letter-spacing: 0.5px;
    text-transform: uppercase;
  }
  h1 { font-size: 22px; font-weight: 700; color: #fff; }
  p.desc { font-size: 14px; color: var(--muted); margin-bottom: 24px; line-height: 1.5; }
  .field {
    background: rgba(15, 23, 42, 0.7);
    border: 1px solid rgba(255,255,255,0.1);
    border-radius: 10px;
    padding: 14px 16px;
    margin-bottom: 16px;
  }
  .label { font-size: 11px; font-weight: 600; text-transform: uppercase; color: var(--muted); margin-bottom: 6px; letter-spacing: 0.5px; }
  .value { font-size: 16px; font-weight: 600; font-family: monospace; color: #38BDF8; word-break: break-all; }
  .btn {
    display: inline-block;
    width: 100%;
    text-align: center;
    background: var(--accent);
    color: #fff;
    font-size: 15px;
    font-weight: 600;
    padding: 14px;
    border-radius: 10px;
    text-decoration: none;
    border: none;
    cursor: pointer;
    transition: background 0.2s;
    margin-top: 10px;
  }
  .btn:hover { background: var(--accent-hover); }
  .btn-outline {
    background: transparent;
    border: 1px solid var(--accent);
    color: var(--accent-hover);
    margin-top: 8px;
  }
  .btn-outline:hover { background: rgba(13, 148, 136, 0.15); }
  .notice {
    font-size: 12px;
    color: var(--muted);
    text-align: center;
    margin-top: 20px;
    line-height: 1.4;
  }
</style>
</head>
<body>
<div class="card">
  <div class="header">
    <h1>Beginning Android</h1>
    <span class="badge">Relay Ready</span>
  </div>
  <p class="desc">PC and Android are ready to pair on your mobile hotspot or Wi-Fi network.</p>

  <div class="field">
    <div class="label">6-Word Pair Code</div>
    <div class="value" id="pairCode">{{.PairCode}}</div>
  </div>

  <div class="field">
    <div class="label">Relay Server URL</div>
    <div class="value" id="relayUrl">{{.WsURL}}</div>
  </div>

  <div class="field">
    <div class="label">Connected PC</div>
    <div class="value" style="color: #A7F3D0;">{{.PCName}} ({{.PrimaryIP}})</div>
  </div>

  <button class="btn" onclick="copyConfig()">📋 Copy Pair Code</button>
  <a class="btn btn-outline" href="/apk" download>📲 Download Android APK to Phone</a>

  <div class="notice">
    Hotspot password: <code>9898048483</code><br>
    Once connected, syncing remains live in background continuously.
  </div>
</div>

<script>
function copyConfig() {
  const code = document.getElementById('pairCode').innerText.trim();
  navigator.clipboard.writeText(code).then(() => {
    alert('Pair code copied: ' + code);
  });
}
</script>
</body>
</html>`
