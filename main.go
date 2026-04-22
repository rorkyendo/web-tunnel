package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Message map[string]interface{}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: mtunnel <port> [subdomain] [token] [upstream-host]")
		return
	}

	port := os.Args[1]
	subdomain := ""
	if len(os.Args) > 2 {
		subdomain = os.Args[2]
	}

	token := os.Getenv("MTUNNEL_TOKEN")
	if len(os.Args) > 3 {
		token = os.Args[3]
	}

	upstreamHost := os.Getenv("MTUNNEL_UPSTREAM_HOST")
	if upstreamHost == "" {
		upstreamHost = "localhost"
	}
	if len(os.Args) > 4 {
		upstreamHost = os.Args[4]
	}

	targetURL := "http://" + upstreamHost + ":" + port
	wsURL := "ws://103.160.212.54:3000"

	readTimeout := 300 * time.Second
	if timeoutSec := os.Getenv("MTUNNEL_READ_TIMEOUT_SEC"); timeoutSec != "" {
		if n, err := strconv.Atoi(timeoutSec); err == nil && n > 0 {
			readTimeout = time.Duration(n) * time.Second
		}
	}

	reconnectBase := 2 * time.Second
	if baseSec := os.Getenv("MTUNNEL_RECONNECT_BASE_SEC"); baseSec != "" {
		if n, err := strconv.Atoi(baseSec); err == nil && n > 0 {
			reconnectBase = time.Duration(n) * time.Second
		}
	}

	reconnectMax := 30 * time.Second
	if maxSec := os.Getenv("MTUNNEL_RECONNECT_MAX_SEC"); maxSec != "" {
		if n, err := strconv.Atoi(maxSec); err == nil && n > 0 {
			reconnectMax = time.Duration(n) * time.Second
		}
	}

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

		delay := reconnectBase
		for i := 0; i < attempt; i++ {
			delay *= 2
			if delay >= reconnectMax {
				delay = reconnectMax
				break
			}
		}

		fmt.Println("🔁 Reconnecting in", delay)
		time.Sleep(delay)
		attempt++
	}
}

var ErrUnauthorized = errors.New("unauthorized token")

func runSession(wsURL, targetURL, subdomain, token string, readTimeout time.Duration) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(readTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(readTimeout))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})

	writeChan := make(chan Message, 200)
	done := make(chan struct{})
	writerErr := make(chan error, 1)
	go writer(conn, writeChan, done, writerErr)
	defer close(done)

	fmt.Println("Connected (mtunnel v0.5)")

	if !enqueue(writeChan, done, Message{
		"type":      "register",
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
		header: make(http.Header),
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
