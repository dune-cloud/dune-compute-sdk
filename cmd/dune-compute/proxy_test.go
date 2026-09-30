// Copyright Dune Cloud Inc.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const tokenHeader = "X-CloudStack-Ssh-Token"

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "ssh-proxy" {
		if err := runProxyFromEnv(); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestBridgePreservesAFragmentedStreamWithPings(t *testing.T) {
	payload := make([]byte, 256+64*1024)
	for i := 0; i < 256; i++ {
		payload[i] = byte(i)
	}
	if _, err := rand.Read(payload[256:]); err != nil {
		t.Fatal(err)
	}
	port, errors := serve(t, echoSession)
	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutRead, stdoutWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		conn, rest, err := openWebSocket(fmt.Sprintf("ws://127.0.0.1:%d/client/ssh", port), "good-token", tokenHeader, "")
		if err != nil {
			errCh <- err
			return
		}
		errCh <- bridge(conn, stdinRead, stdoutWrite, rest)
	}()
	if _, err := stdinWrite.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = stdinWrite.Close()
	got, err := io.ReadAll(stdoutRead)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if len(errors) > 0 {
		t.Fatal(errors[0])
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %d bytes, want %d", len(got), len(payload))
	}
}

func TestHandshakeSendsBearerAPIKey(t *testing.T) {
	const apiKey = "test-api-key"
	var mu sync.Mutex
	var got string
	port, errors := serve(t, func(conn net.Conn, rest []byte, headers map[string]string) {
		mu.Lock()
		got = headers["authorization"]
		mu.Unlock()
		echoSession(conn, rest, headers)
	})
	conn, _, err := openWebSocket(fmt.Sprintf("ws://127.0.0.1:%d/client/ssh", port), "good-token", tokenHeader, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	mu.Lock()
	defer mu.Unlock()
	if got != "Bearer "+apiKey {
		t.Fatalf("authorization = %q", got)
	}
	if len(errors) > 0 {
		t.Fatal(errors[0])
	}
}

func TestHandshakeOmitsBearerWhenAPIKeyIsEmpty(t *testing.T) {
	var mu sync.Mutex
	var got string
	var present bool
	port, _ := serve(t, func(conn net.Conn, rest []byte, headers map[string]string) {
		value, ok := headers["authorization"]
		mu.Lock()
		got, present = value, ok
		mu.Unlock()
		echoSession(conn, rest, headers)
	})
	conn, _, err := openWebSocket(fmt.Sprintf("ws://127.0.0.1:%d/client/ssh", port), "good-token", tokenHeader, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	mu.Lock()
	defer mu.Unlock()
	if present || got != "" {
		t.Fatalf("authorization = %q present=%v", got, present)
	}
}

func TestHandshakeRejectsAnAPIKeyWithANewline(t *testing.T) {
	if _, _, err := openWebSocket("ws://127.0.0.1:1/client/ssh", "good-token", tokenHeader, "bad\nkey"); err == nil {
		t.Fatal("newline in api key was accepted")
	}
}

func TestProxyFromEnvSendsBearerAPIKey(t *testing.T) {
	const apiKey = "test-api-key"
	var mu sync.Mutex
	var got string
	port, _ := serve(t, func(conn net.Conn, _ []byte, headers map[string]string) {
		mu.Lock()
		got = headers["authorization"]
		mu.Unlock()
		_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
		_ = conn.Close()
	})
	t.Setenv("DUNE_COMPUTE_SSH_WS_URL", fmt.Sprintf("ws://127.0.0.1:%d/client/ssh", port))
	t.Setenv("DUNE_COMPUTE_SSH_TOKEN", "good-token")
	t.Setenv("DUNE_COMPUTE_SSH_TOKEN_HEADER", tokenHeader)
	t.Setenv("DUNE_COMPUTE_API_KEY", apiKey)
	if err := runProxyFromEnv(); err == nil {
		t.Fatal("handshake failure was ignored")
	}
	mu.Lock()
	defer mu.Unlock()
	if got != "Bearer "+apiKey {
		t.Fatalf("authorization = %q", got)
	}
}

func TestBadAcceptIsRejected(t *testing.T) {
	port, _ := serve(t, func(conn net.Conn, _ []byte, _ map[string]string) {
		_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: not-the-accept-value\r\n\r\n"))
		_ = conn.Close()
	})
	if _, _, err := openWebSocket(fmt.Sprintf("ws://127.0.0.1:%d/client/ssh", port), "good-token", tokenHeader, ""); err == nil {
		t.Fatal("bad accept was accepted")
	}
}

func TestOpenSSHRoundTripAndExitCode(t *testing.T) {
	sshd, sshPort, work := startSSHD(t)
	defer func() {
		_ = sshd.Process.Kill()
		_, _ = sshd.Process.Wait()
	}()
	var dials atomicCount
	port, errors := serve(t, func(conn net.Conn, rest []byte, headers map[string]string) {
		handshakeThenSplice(conn, rest, headers, sshPort, &dials)
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(),
		fmt.Sprintf("DUNE_COMPUTE_SSH_WS_URL=ws://127.0.0.1:%d/client/ssh", port),
		"DUNE_COMPUTE_SSH_TOKEN=good-token",
		"DUNE_COMPUTE_SSH_TOKEN_HEADER="+tokenHeader,
	)
	base := []string{
		"ssh", "-F", "/dev/null",
		"-o", "ProxyCommand=" + shellQuote(exe) + " ssh-proxy",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "PreferredAuthentications=publickey",
		"-i", filepath.Join(work, "user"),
		"-l", account.Username,
		"instance-1",
	}
	ok := runSSHCommand(t, env, append(append([]string{}, base...), "--", "echo", "ok"))
	if ok.Code != 0 {
		t.Fatalf("echo failed: %s", ok.Stderr)
	}
	if strings.TrimSpace(ok.Stdout) != "ok" {
		t.Fatalf("stdout = %q", ok.Stdout)
	}
	code := runSSHCommand(t, env, append(append([]string{}, base...), "--", "exit", "7"))
	if code.Code != 7 {
		t.Fatalf("exit = %d stderr=%s", code.Code, code.Stderr)
	}
	blob := make([]byte, 256+128*1024)
	for i := 0; i < 256; i++ {
		blob[i] = byte(i)
	}
	if _, err := rand.Read(blob[256:]); err != nil {
		t.Fatal(err)
	}
	copied := runSSHCommand(t, env, append(append([]string{}, base...), "--", "cat"), blob)
	if copied.Code != 0 {
		t.Fatalf("cat failed: %s", copied.Stderr)
	}
	if !bytes.Equal(copied.StdoutBytes, blob) {
		t.Fatalf("cat bytes %d", len(copied.StdoutBytes))
	}
	before := dials.get()
	deniedEnv := append([]string{}, env...)
	for i, value := range deniedEnv {
		if strings.HasPrefix(value, "DUNE_COMPUTE_SSH_TOKEN=") {
			deniedEnv[i] = "DUNE_COMPUTE_SSH_TOKEN=other-token"
		}
	}
	denied := runSSHCommand(t, deniedEnv, append(append([]string{}, base...), "--", "echo", "ok"))
	if denied.Code == 0 {
		t.Fatal("wrong token was accepted")
	}
	if dials.get() != before {
		t.Fatal("rejected token still dialed sshd")
	}
	if len(errors) > 0 {
		t.Fatal(errors[0])
	}
}

type commandResult struct {
	Code        int
	Stdout      string
	Stderr      string
	StdoutBytes []byte
}

func runSSHCommand(t *testing.T, env []string, args []string, stdin ...[]byte) commandResult {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin[0])
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return commandResult{Code: code, Stdout: out.String(), Stderr: errb.String(), StdoutBytes: out.Bytes()}
}

func serve(t *testing.T, session func(net.Conn, []byte, map[string]string)) (int, []error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	var mu sync.Mutex
	var errors []error
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				if tcp, ok := conn.(*net.TCPConn); ok {
					_ = tcp.SetNoDelay(true)
				}
				head, rest, err := readHTTPMessage(conn, 65536)
				if err != nil {
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
					_ = conn.Close()
					return
				}
				_, headers, err := headerMap(head)
				if err != nil || headers["sec-websocket-key"] == "" {
					_ = conn.Close()
					return
				}
				session(conn, rest, headers)
			}()
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, errors
}

func echoSession(conn net.Conn, rest []byte, headers map[string]string) {
	defer conn.Close()
	if headers[strings.ToLower(tokenHeader)] != "good-token" {
		_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
		return
	}
	accept := acceptNonce(headers["sec-websocket-key"])
	_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n"))
	parser := &frameParser{}
	stream := &byteStream{}
	var incoming []byte
	pending := rest
	for {
		var chunk []byte
		if len(pending) > 0 {
			chunk = pending
			pending = nil
		} else {
			buf := make([]byte, 65536)
			n, err := conn.Read(buf)
			if n > 0 {
				chunk = buf[:n]
			}
			if err != nil {
				return
			}
		}
		frames, err := parser.feed(chunk)
		if err != nil {
			return
		}
		finished := false
		for _, fr := range frames {
			if fr.opcode == opPing || fr.opcode == opPong {
				if fr.opcode == opPing {
					encoded, err := encodeFrame(opPong, fr.payload, true, nil)
					if err == nil {
						_, _ = conn.Write(encoded)
					}
				}
				continue
			}
			if fr.opcode == opClose {
				finished = true
				break
			}
			piece, err := stream.push(fr.fin, fr.opcode, fr.payload)
			if err != nil {
				return
			}
			incoming = append(incoming, piece...)
		}
		if finished {
			break
		}
	}
	mid := len(incoming) / 2
	var wire []byte
	for _, chunk := range []struct {
		op  int
		fin bool
		p   []byte
	}{
		{opBinary, false, incoming[:mid]},
		{opPing, true, []byte{1}},
		{opContinuation, true, incoming[mid:]},
		{opClose, true, closePayload(1000)},
	} {
		encoded, err := encodeFrame(chunk.op, chunk.p, chunk.fin, nil)
		if err != nil {
			return
		}
		wire = append(wire, encoded...)
	}
	for len(wire) > 0 {
		_, _ = conn.Write(wire[:1])
		wire = wire[1:]
	}
}

func handshakeThenSplice(conn net.Conn, rest []byte, headers map[string]string, sshPort int, dials *atomicCount) {
	if headers[strings.ToLower(tokenHeader)] != "good-token" {
		_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 9\r\nConnection: close\r\n\r\nforbidden"))
		_ = conn.Close()
		return
	}
	accept := acceptNonce(headers["sec-websocket-key"])
	_, _ = conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n"))
	upstream, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", sshPort))
	if err != nil {
		_ = conn.Close()
		return
	}
	if tcp, ok := upstream.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}
	dials.add()
	spliceWebSocket(conn, rest, upstream)
	_ = upstream.Close()
}

func spliceWebSocket(client net.Conn, rest []byte, upstream net.Conn) {
	parser := &frameParser{}
	stream := &byteStream{}
	var stop atomicBool
	var sendMu sync.Mutex
	sendClient := func(payload []byte) {
		sendMu.Lock()
		_, _ = client.Write(payload)
		sendMu.Unlock()
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer stop.set()
		pending := rest
		for !stop.get() {
			var chunk []byte
			if len(pending) > 0 {
				chunk = pending
				pending = nil
			} else {
				buf := make([]byte, 65536)
				n, err := client.Read(buf)
				if n > 0 {
					chunk = buf[:n]
				}
				if err != nil {
					break
				}
			}
			frames, err := parser.feed(chunk)
			if err != nil {
				break
			}
			for _, fr := range frames {
				switch fr.opcode {
				case opPing:
					encoded, err := encodeFrame(opPong, fr.payload, true, nil)
					if err == nil {
						sendClient(encoded)
					}
				case opPong:
				case opClose:
					if tcp, ok := upstream.(*net.TCPConn); ok {
						_ = tcp.CloseWrite()
					}
					return
				default:
					piece, err := stream.push(fr.fin, fr.opcode, fr.payload)
					if err != nil {
						return
					}
					if _, err := upstream.Write(piece); err != nil {
						return
					}
				}
			}
		}
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		defer stop.set()
		buf := make([]byte, 32768)
		for !stop.get() {
			n, err := upstream.Read(buf)
			if n > 0 {
				encoded, encErr := encodeFrame(opBinary, append([]byte(nil), buf[:n]...), true, nil)
				if encErr == nil {
					sendClient(encoded)
				}
				ping, pingErr := encodeFrame(opPing, []byte{1}, true, nil)
				if pingErr == nil {
					sendClient(ping)
				}
			}
			if err != nil {
				break
			}
		}
		if encoded, err := encodeFrame(opClose, closePayload(1000), true, nil); err == nil {
			sendClient(encoded)
		}
	}()
	wg.Wait()
	_ = client.Close()
}

func startSSHD(t *testing.T) (*exec.Cmd, int, string) {
	t.Helper()
	if _, err := os.Stat("/usr/sbin/sshd"); err != nil {
		t.Skip("sshd is not installed")
	}
	work := t.TempDir()
	if err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-f", filepath.Join(work, "host"), "-N", "").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-f", filepath.Join(work, "user"), "-N", "").Run(); err != nil {
		t.Fatal(err)
	}
	pub, err := os.ReadFile(filepath.Join(work, "user.pub"))
	if err != nil {
		t.Fatal(err)
	}
	auth := filepath.Join(work, "authorized_keys")
	if err := os.WriteFile(auth, pub, 0o644); err != nil {
		t.Fatal(err)
	}
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()
	config := filepath.Join(work, "sshd_config")
	text := fmt.Sprintf(`Port %d
ListenAddress 127.0.0.1
HostKey %s
PidFile %s
AuthorizedKeysFile %s
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
PubkeyAuthentication yes
AuthenticationMethods publickey
PermitRootLogin no
StrictModes no
PrintMotd no
X11Forwarding no
AllowTcpForwarding no
UseDNS no
`, port, filepath.Join(work, "host"), filepath.Join(work, "sshd.pid"), auth)
	if err := os.WriteFile(config, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	check := exec.Command("/usr/sbin/sshd", "-t", "-f", config)
	if out, err := check.CombinedOutput(); err != nil {
		t.Skipf("sshd rejected the test config: %s", out)
	}
	cmd := exec.Command("/usr/sbin/sshd", "-D", "-e", "-f", config)
	log, err := os.Create(filepath.Join(work, "sshd.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return cmd, port, work
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	body, _ := os.ReadFile(filepath.Join(work, "sshd.log"))
	t.Fatalf("sshd did not listen: %s", body)
	return nil, 0, ""
}

type atomicCount struct {
	mu sync.Mutex
	n  int
}

func (c *atomicCount) add() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *atomicCount) get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
