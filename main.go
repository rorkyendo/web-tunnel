package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Message map[string]interface{}

var ErrUnauthorized = errors.New("unauthorized token")

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	if strings.EqualFold(os.Args[1], "tcp") {
		runTCPMode()
		return
	}

	runHTTPMode()
}

func printUsage() {
	fmt.Println("Usage (HTTP): mtunnel <port> [subdomain] [token] [upstream-host]")
	fmt.Println("Usage (TCP):  mtunnel tcp <local-port> [remote-port] [token] [upstream-host]")
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func argOrEnv(args []string, idx int, envName, def string) string {
	if len(os.Args) > idx && os.Args[idx] != "" {
		return os.Args[idx]
	}
	if v := os.Getenv(envName); v != "" {
		return v
	}
	return def
}

func reconnectDelay(attempt int, base, max time.Duration) time.Duration {
	delay := base
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= max {
			return max
		}
	}
	return delay
}

func dialWS(wsURL string, readTimeout time.Duration) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})
	return conn, nil
}

// ===================== HTTP MODE =====================

func runHTTPMode() {
	port := os.Args[1]
	subdomain := ""
	if len(os.Args) > 2 {
		subdomain = os.Args[2]
	}

	token := argOrEnv(os.Args, 3, "MTUNNEL_TOKEN", "")
	upstreamHost := argOrEnv(os.Args, 4, "MTUNNEL_UPSTREAM_HOST", "localhost")

	targetURL := "http://" + upstreamHost + ":" + port
	wsURL := os.Getenv("MTUNNEL_SERVER")
	if wsURL == "" {
		wsURL = "ws://163.223.104.100:3000"
	}

	readTimeout := time.Duration(envInt("MTUNNEL_READ_TIMEOUT_SEC", 300)) * time.Second
	reconnectBase := time.Duration(envInt("MTUNNEL_RECONNECT_BASE_SEC", 2)) * time.Second
	reconnectMax := time.Duration(envInt("MTUNNEL_RECONNECT_MAX_SEC", 30)) * time.Second

	attempt := 0

	for {
		err := runSession(wsURL, targetURL, subdomain, token, readTimeout)
		if err != nil {
			fmt.Println("⚠️ Disconnected:", err)
			if errors.Is(err, ErrUnauthorized) {
				fmt.Println("❌ Stopped reconnect: unauthorized token")
				return
			}
		}

		delay := reconnectDelay(attempt, reconnectBase, reconnectMax)
		fmt.Println("🔁 Reconnecting in", delay)
		time.Sleep(delay)
		attempt++
	}
}

func runSession(wsURL, targetURL, subdomain, token string, readTimeout time.Duration) error {
	conn, err := dialWS(wsURL, readTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	writeChan := make(chan Message, 200)
	done := make(chan struct{})
	writerErr := make(chan error, 1)
	go writer(conn, writeChan, done, writerErr)
	defer close(done)

	fmt.Println("Connected (mtunnel v0.6)")

	if !enqueue(writeChan, done, Message{
		"type":      "register",
		"proto":     "http",
		"subdomain": subdomain,
		"token":     token,
	}) {
		return errors.New("failed to register tunnel")
	}

	for {
		select {
		case err := <-writerErr:
			return err
		default:
		}

		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			return err
		}

		conn.SetReadDeadline(time.Now().Add(readTimeout))

		msgType, _ := msg["type"].(string)

		if msgType == "assigned" {
			fmt.Println("🚀 URL:", msg["url"])
			continue
		}

		if msgType == "error" {
			message, _ := msg["message"].(string)
			fmt.Println("❌ Server error:", message)
			if strings.Contains(strings.ToLower(message), "unauthorized") {
				return ErrUnauthorized
			}
			return errors.New(message)
		}

		if msgType == "ping" {
			enqueue(writeChan, done, Message{"type": "pong"})
			continue
		}

		if msgType != "request" {
			continue
		}

		go handleRequest(msg, targetURL, writeChan, done)
	}
}

func handleRequest(msg Message, target string, writeChan chan<- Message, done <-chan struct{}) {
	id := msg["id"].(string)
	method := msg["method"].(string)
	path := msg["path"].(string)
	debug := os.Getenv("MTUNNEL_DEBUG") == "1"

	targetURL, _ := url.Parse(target)
	upstreamHostHeader := os.Getenv("MTUNNEL_UPSTREAM_HOST_HEADER")

	bodyEncoded := msg["body"].(string)
	bodyBytes, _ := base64.StdEncoding.DecodeString(bodyEncoded)

	if path == "" {
		path = "/"
	} else if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "?") {
		path = "/" + path
	}

	upstreamURL, _ := url.Parse(targetURL.String() + path)

	req, _ := http.NewRequest(method, upstreamURL.String(), bytes.NewReader(bodyBytes))
	originalHost := ""
	originalReferer := ""

	if headers, ok := msg["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			val, ok := v.(string)
			if !ok {
				continue
			}

			lower := strings.ToLower(k)
			if lower == "content-length" || lower == "host" {
				if lower == "host" {
					originalHost = strings.TrimSpace(strings.Split(val, ",")[0])
				}
				continue
			}

			if lower == "referer" {
				originalReferer = val
			}

			req.Header.Set(k, val)
		}
	}

	req.Host = targetURL.Host
	if originalHost != "" {
		req.Host = originalHost
	}
	if upstreamHostHeader != "" {
		req.Host = upstreamHostHeader
	}

	if req.Header.Get("X-Forwarded-Host") == "" && originalHost != "" {
		req.Header.Set("X-Forwarded-Host", originalHost)
	}
	if req.Header.Get("X-Forwarded-Proto") == "" {
		req.Header.Set("X-Forwarded-Proto", "https")
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(rw, "Bad Gateway")
	}

	rec := newRecorder()

	proxy.ServeHTTP(rec, req)
	if debug || rec.status >= 300 || path == "/404.html" {
		location := rec.header.Get("Location")
		if location != "" && originalReferer != "" {
			fmt.Printf("[proxy] %s %s host=%s -> %d location=%s referer=%s\n", method, path, req.Host, rec.status, location, originalReferer)
		} else if location != "" {
			fmt.Printf("[proxy] %s %s host=%s -> %d location=%s\n", method, path, req.Host, rec.status, location)
		} else if originalReferer != "" {
			fmt.Printf("[proxy] %s %s host=%s -> %d referer=%s\n", method, path, req.Host, rec.status, originalReferer)
		} else {
			fmt.Printf("[proxy] %s %s host=%s -> %d\n", method, path, req.Host, rec.status)
		}
	}

	respBody := rec.body.Bytes()

	enqueue(writeChan, done, Message{
		"type":    "response",
		"id":      id,
		"status":  rec.status,
		"headers": headerToMap(rec.header),
		"body":    base64.StdEncoding.EncodeToString(respBody),
	})
}

// ===================== TCP MODE =====================

func runTCPMode() {
	if len(os.Args) < 3 {
		printUsage()
		return
	}

	localPort := os.Args[2]

	remotePort := envInt("MTUNNEL_REMOTE_PORT", 0)
	if len(os.Args) > 3 {
		if n, err := strconv.Atoi(os.Args[3]); err == nil && n > 0 {
			remotePort = n
		}
	}

	token := argOrEnv(os.Args, 4, "MTUNNEL_TOKEN", "")
	upstreamHost := argOrEnv(os.Args, 5, "MTUNNEL_UPSTREAM_HOST", "localhost")
	target := net.JoinHostPort(upstreamHost, localPort)

	wsURL := os.Getenv("MTUNNEL_SERVER")
	if wsURL == "" {
		wsURL = "ws://163.223.104.100:3000"
	}

	readTimeout := time.Duration(envInt("MTUNNEL_READ_TIMEOUT_SEC", 300)) * time.Second
	reconnectBase := time.Duration(envInt("MTUNNEL_RECONNECT_BASE_SEC", 2)) * time.Second
	reconnectMax := time.Duration(envInt("MTUNNEL_RECONNECT_MAX_SEC", 30)) * time.Second

	attempt := 0

	for {
		err := runTCPSession(wsURL, target, token, remotePort, readTimeout)
		if err != nil {
			fmt.Println("⚠️ Disconnected:", err)
			if errors.Is(err, ErrUnauthorized) {
				fmt.Println("❌ Stopped reconnect: unauthorized token")
				return
			}
		}

		delay := reconnectDelay(attempt, reconnectBase, reconnectMax)
		fmt.Println("🔁 Reconnecting in", delay)
		time.Sleep(delay)
		attempt++
	}
}

func runTCPSession(wsURL, target, token string, remotePort int, readTimeout time.Duration) error {
	conn, err := dialWS(wsURL, readTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	writeChan := make(chan Message, 200)
	done := make(chan struct{})
	writerErr := make(chan error, 1)
	go writer(conn, writeChan, done, writerErr)
	defer close(done)

	fmt.Println("Connected (mtunnel v0.6 tcp)")

	if !enqueue(writeChan, done, Message{
		"type":        "register",
		"proto":       "tcp",
		"token":       token,
		"remote_port": remotePort,
	}) {
		return errors.New("failed to register tunnel")
	}

	var mu sync.Mutex
	localConns := map[string]net.Conn{}

	removeConn := func(connID string) net.Conn {
		mu.Lock()
		defer mu.Unlock()
		c := localConns[connID]
		delete(localConns, connID)
		return c
	}

	defer func() {
		mu.Lock()
		for _, c := range localConns {
			c.Close()
		}
		localConns = map[string]net.Conn{}
		mu.Unlock()
	}()

	for {
		select {
		case err := <-writerErr:
			return err
		default:
		}

		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			return err
		}

		conn.SetReadDeadline(time.Now().Add(readTimeout))

		msgType, _ := msg["type"].(string)
		connID, _ := msg["conn_id"].(string)

		switch msgType {
		case "assigned":
			fmt.Println("🚀 TCP URL:", msg["url"])
			fmt.Println("   Forwarding to", target)
		case "error":
			message, _ := msg["message"].(string)
			fmt.Println("❌ Server error:", message)
			if strings.Contains(strings.ToLower(message), "unauthorized") {
				return ErrUnauthorized
			}
			return errors.New(message)
		case "ping":
			enqueue(writeChan, done, Message{"type": "pong"})
		case "tcp-open":
			if connID == "" {
				continue
			}
			local, err := net.DialTimeout("tcp", target, 10*time.Second)
			if err != nil {
				fmt.Println("⚠️ Local dial failed:", err)
				enqueue(writeChan, done, Message{"type": "tcp-close", "conn_id": connID})
				continue
			}
			mu.Lock()
			localConns[connID] = local
			mu.Unlock()
			go pumpTCP(local, connID, writeChan, done, removeConn)
		case "tcp-data":
			encoded, _ := msg["data"].(string)
			mu.Lock()
			local := localConns[connID]
			mu.Unlock()
			if local == nil {
				continue
			}
			chunk, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				continue
			}
			if _, err := local.Write(chunk); err != nil {
				removeConn(connID)
				local.Close()
				enqueue(writeChan, done, Message{"type": "tcp-close", "conn_id": connID})
			}
		case "tcp-close":
			if local := removeConn(connID); local != nil {
				local.Close()
			}
		}
	}
}

func pumpTCP(local net.Conn, connID string, writeChan chan<- Message, done <-chan struct{}, removeConn func(string) net.Conn) {
	buf := make([]byte, 16*1024)
	for {
		n, err := local.Read(buf)
		if n > 0 {
			if !enqueue(writeChan, done, Message{
				"type":    "tcp-data",
				"conn_id": connID,
				"data":    base64.StdEncoding.EncodeToString(buf[:n]),
			}) {
				return
			}
		}
		if err != nil {
			removeConn(connID)
			local.Close()
			enqueue(writeChan, done, Message{"type": "tcp-close", "conn_id": connID})
			return
		}
	}
}

// ===================== SHARED =====================

func writer(conn *websocket.Conn, writeChan <-chan Message, done <-chan struct{}, writerErr chan<- error) {
	for {
		select {
		case <-done:
			return
		case msg := <-writeChan:
			err := conn.WriteJSON(msg)
			if err != nil {
				writerErr <- err
				return
			}
		}
	}
}

func enqueue(writeChan chan<- Message, done <-chan struct{}, msg Message) bool {
	select {
	case <-done:
		return false
	case writeChan <- msg:
		return true
	}
}

type recorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newRecorder() *recorder {
	return &recorder{
		header: make(map[string][]string),
		status: 200,
	}
}

func (r *recorder) Header() http.Header {
	return r.header
}

func (r *recorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *recorder) WriteHeader(statusCode int) {
	r.status = statusCode
}

func headerToMap(h http.Header) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}
