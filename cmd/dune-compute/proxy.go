// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dune-cloud/dune-compute-sdk/dune"
)

const (
	dataQueueDepth = 128
	readChunk      = 32 * 1024
)

type outbound struct {
	frame []byte
	stop  bool
}

func openWebSocket(rawURL, token, headerName, apiKey string) (net.Conn, []byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, err
	}
	if parsed.Scheme != "wss" && parsed.Scheme != "ws" {
		return nil, nil, protocolError("tunnel URL must be a WebSocket URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, nil, protocolError("ssh token must not be placed in the URL")
	}
	if parsed.Hostname() == "" || headerName == "" || token == "" {
		return nil, nil, protocolError("incomplete tunnel address")
	}
	if strings.ContainsAny(token+headerName, "\r\n") {
		return nil, nil, protocolError("invalid token header")
	}
	if strings.ContainsAny(apiKey, "\r\n") {
		return nil, nil, protocolError("invalid api key")
	}
	host := parsed.Host
	if parsed.Port() == "" {
		port := "80"
		if parsed.Scheme == "wss" {
			port = "443"
		}
		host = net.JoinHostPort(parsed.Hostname(), port)
	}
	raw, err := (&net.Dialer{Timeout: 30 * time.Second}).Dial("tcp", host)
	if err != nil {
		return nil, nil, err
	}
	if tcp, ok := raw.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	conn := raw
	cleanup := true
	defer func() {
		if cleanup {
			_ = conn.Close()
		}
	}()
	if parsed.Scheme == "wss" {
		tlsConfig := &tls.Config{ServerName: parsed.Hostname()}
		if os.Getenv("DUNE_COMPUTE_SSH_INSECURE") == "1" {
			tlsConfig.InsecureSkipVerify = true
		}
		tlsConn := tls.Client(raw, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return nil, nil, err
		}
		conn = tlsConn
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, nil, err
	}
	keyB64 := base64.StdEncoding.EncodeToString(key)
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	lines := []string{
		fmt.Sprintf("GET %s HTTP/1.1", path),
		fmt.Sprintf("Host: %s", parsed.Host),
		"Upgrade: websocket",
		"Connection: Upgrade",
		fmt.Sprintf("Sec-WebSocket-Key: %s", keyB64),
		"Sec-WebSocket-Version: 13",
		fmt.Sprintf("%s: %s", headerName, token),
	}
	if apiKey != "" {
		lines = append(lines, "Authorization: Bearer "+apiKey)
	}
	lines = append(lines, "", "")
	request := strings.Join(lines, "\r\n")
	if err := writeFull(conn, []byte(request)); err != nil {
		return nil, nil, err
	}
	head, rest, err := readHTTPMessage(conn, 65536)
	if err != nil {
		return nil, nil, err
	}
	status, headers, err := headerMap(head)
	if err != nil {
		return nil, nil, err
	}
	parts := strings.Fields(status)
	if len(parts) < 2 || parts[1] != "101" {
		detail := strings.TrimSpace(string(rest))
		return nil, nil, protocolError(strings.TrimSpace(fmt.Sprintf("tunnel handshake failed (%s) %s", status, detail)))
	}
	if !strings.EqualFold(headers["upgrade"], "websocket") {
		return nil, nil, protocolError("tunnel handshake missing Upgrade")
	}
	if headers["sec-websocket-accept"] != acceptNonce(keyB64) {
		return nil, nil, protocolError("tunnel handshake failed the accept check")
	}
	cleanup = false
	return conn, rest, nil
}

func readHTTPMessage(conn net.Conn, limit int) ([]byte, []byte, error) {
	reader := bufio.NewReader(conn)
	var head []byte
	for {
		line, err := reader.ReadSlice('\n')
		if err != nil {
			return nil, nil, protocolError("connection closed during handshake")
		}
		head = append(head, line...)
		if len(head) > limit {
			return nil, nil, protocolError("handshake too large")
		}
		if bytesHasCRLFCRLF(head) {
			break
		}
	}
	split := indexCRLFCRLF(head)
	rest := append([]byte(nil), head[split+4:]...)
	buffered, _ := reader.Peek(reader.Buffered())
	rest = append(rest, buffered...)
	_, _ = reader.Discard(len(buffered))
	return head[:split], rest, nil
}

func bytesHasCRLFCRLF(buf []byte) bool { return indexCRLFCRLF(buf) >= 0 }

func indexCRLFCRLF(buf []byte) int {
	for i := 0; i+3 < len(buf); i++ {
		if buf[i] == '\r' && buf[i+1] == '\n' && buf[i+2] == '\r' && buf[i+3] == '\n' {
			return i
		}
	}
	return -1
}

func headerMap(head []byte) (string, map[string]string, error) {
	text := string(head)
	lines := strings.Split(text, "\r\n")
	if len(lines) == 0 || !strings.Contains(lines[0], " ") {
		return "", nil, protocolError("malformed handshake")
	}
	headers := map[string]string{}
	for _, line := range lines[1:] {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		headers[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(value)
	}
	return lines[0], headers, nil
}

func bridge(conn net.Conn, stdin, stdout *os.File, initial []byte) error {
	parser := &frameParser{}
	stream := &byteStream{}
	dataq := make(chan outbound, dataQueueDepth)
	control := make(chan outbound, 32)
	var stopFlag atomicBool
	var failOnce sync.Once
	var ferr error
	fail := func(err error) {
		if err == nil {
			return
		}
		failOnce.Do(func() { ferr = err })
		stopFlag.set()
	}

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		send := func(msg outbound) bool {
			if msg.stop {
				drainControl(control, conn)
				return true
			}
			if err := writeFull(conn, msg.frame); err != nil {
				fail(err)
				return true
			}
			return false
		}
		for {
			if msg, ok := tryRecv(control); ok {
				if send(msg) {
					return
				}
				continue
			}
			select {
			case msg := <-control:
				if send(msg) {
					return
				}
			case msg := <-dataq:
				if send(msg) {
					return
				}
			case <-time.After(50 * time.Millisecond):
				if stopFlag.get() && len(control) == 0 && len(dataq) == 0 {
					return
				}
			}
		}
	}()

	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		buf := make([]byte, readChunk)
		for !stopFlag.get() {
			n, err := stdin.Read(buf)
			if n > 0 {
				payload := append([]byte(nil), buf[:n]...)
				frame, frameErr := clientFrame(opBinary, payload)
				if frameErr != nil {
					fail(frameErr)
					return
				}
				dataq <- outbound{frame: frame}
			}
			if err != nil {
				if err != io.EOF {
					fail(err)
				}
				frame, frameErr := clientFrame(opClose, closePayload(1000))
				if frameErr != nil {
					fail(frameErr)
					return
				}
				dataq <- outbound{frame: frame}
				dataq <- outbound{stop: true}
				return
			}
		}
	}()

	socketDone := make(chan struct{})
	go func() {
		defer close(socketDone)
		defer func() {
			stopFlag.set()
			_ = stdout.Close()
		}()
		pending := initial
		buf := make([]byte, 65536)
		for !stopFlag.get() {
			var chunk []byte
			if len(pending) > 0 {
				chunk = pending
				pending = nil
			} else {
				n, err := conn.Read(buf)
				if n > 0 {
					chunk = append([]byte(nil), buf[:n]...)
				}
				if err != nil {
					if err != io.EOF {
						fail(err)
					}
					return
				}
				if n == 0 {
					return
				}
			}
			frames, err := parser.feed(chunk)
			if err != nil {
				fail(err)
				return
			}
			for _, fr := range frames {
				if err := handleFrame(fr, stream, stdout, control, &stopFlag); err != nil {
					fail(err)
					return
				}
				if stopFlag.get() {
					return
				}
			}
		}
	}()

	<-stdinDone
	<-writerDone
	<-socketDone
	_ = conn.Close()
	return ferr
}

func handleFrame(fr frame, stream *byteStream, stdout *os.File, control chan outbound, stopFlag *atomicBool) error {
	switch fr.opcode {
	case opPing:
		frame, err := clientFrame(opPong, fr.payload)
		if err != nil {
			return err
		}
		control <- outbound{frame: frame}
		return nil
	case opPong:
		return nil
	case opClose:
		payload := closePayload(1000)
		if len(fr.payload) >= 2 {
			payload = append([]byte(nil), fr.payload[:2]...)
		}
		frame, err := clientFrame(opClose, payload)
		if err != nil {
			return err
		}
		control <- outbound{frame: frame}
		stopFlag.set()
		return nil
	default:
		out, err := stream.push(fr.fin, fr.opcode, fr.payload)
		if err != nil {
			return err
		}
		return writeFull(stdout, out)
	}
}

func tryRecv(ch chan outbound) (outbound, bool) {
	select {
	case msg := <-ch:
		return msg, true
	default:
		return outbound{}, false
	}
}

func drainControl(control chan outbound, conn net.Conn) {
	for {
		select {
		case extra := <-control:
			if !extra.stop && len(extra.frame) > 0 {
				_ = writeFull(conn, extra.frame)
			}
		default:
			return
		}
	}
}

func runProxyFromEnv() error {
	rawURL := os.Getenv("DUNE_COMPUTE_SSH_WS_URL")
	token := os.Getenv("DUNE_COMPUTE_SSH_TOKEN")
	header := os.Getenv("DUNE_COMPUTE_SSH_TOKEN_HEADER")
	if header == "" {
		header = dune.TokenHeader
	}
	if rawURL == "" || token == "" {
		return protocolError("ssh proxy is missing its tunnel endpoint")
	}
	conn, rest, err := openWebSocket(rawURL, token, header, os.Getenv("DUNE_COMPUTE_API_KEY"))
	if err != nil {
		return err
	}
	return bridge(conn, os.Stdin, os.Stdout, rest)
}

func writeFull(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if n > 0 {
			payload = payload[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (a *atomicBool) set() {
	a.mu.Lock()
	a.v = true
	a.mu.Unlock()
}

func (a *atomicBool) get() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v
}
