package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Message map[string]interface{}

var writeChan = make(chan Message, 200)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: mtunnel <port> [subdomain] [token]")
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

	targetURL := "http://127.0.0.1:" + port
	wsURL := "ws://103.160.212.145:3000"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	fmt.Println("Connected")

	go writer(conn)

	writeChan <- Message{
		"type":      "register",
		"subdomain": subdomain,
		"token":     token,
	}

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Fatal("Read error:", err)
		}

		msgType, _ := msg["type"].(string)

		if msgType == "assigned" {
			fmt.Println("🚀 URL:", msg["url"])
			continue
		}

		if msgType == "error" {
			fmt.Println("❌ Server error:", msg["message"])
			continue
		}

		if msgType == "ping" {
			writeChan <- Message{"type": "pong"}
			continue
		}

		if msgType != "request" {
			continue
		}

		go handleRequest(msg, targetURL)
	}
}

func writer(conn *websocket.Conn) {
	for msg := range writeChan {
		err := conn.WriteJSON(msg)
		if err != nil {
			fmt.Println("Write error:", err)
			return
		}
	}
}

func handleRequest(msg Message, target string) {
	id := msg["id"].(string)
	method := msg["method"].(string)
	path := msg["path"].(string)

	targetURL, _ := url.Parse(target)

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

	if headers, ok := msg["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			val, ok := v.(string)
			if !ok {
				continue
			}

			lower := strings.ToLower(k)
			if lower == "content-length" || lower == "host" {
				if lower == "host" {
					originalHost = val
				}
				continue
			}

			req.Header.Set(k, val)
		}
	}

	req.Host = targetURL.Host

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

	respBody := rec.body.Bytes()

	writeChan <- Message{
		"type":    "response",
		"id":      id,
		"status":  rec.status,
		"headers": headerToMap(rec.header),
		"body":    base64.StdEncoding.EncodeToString(respBody),
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
