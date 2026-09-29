package mailer

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeSMTP speaks just enough SMTP to observe the client's commands. It
// never advertises AUTH — the shape every local dev server (Mailpit)
// has — so the sender must not attempt it without credentials.
type fakeSMTP struct {
	ln       net.Listener
	commands chan string
}

func startFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeSMTP{ln: ln, commands: make(chan string, 16)}
	go f.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeSMTP) serve() {
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	w := textproto.NewConn(conn)
	_ = w.PrintfLine("220 fake ESMTP")
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		f.commands <- cmd
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			// Capability list deliberately excludes AUTH.
			_ = w.PrintfLine("250-fake")
			_ = w.PrintfLine("250 SIZE 10485760")
		case strings.HasPrefix(cmd, "MAIL FROM"), strings.HasPrefix(cmd, "RCPT TO"):
			_ = w.PrintfLine("250 ok")
		case strings.HasPrefix(cmd, "DATA"):
			_ = w.PrintfLine("354 go ahead")
			// Consume the body silently until the terminating dot; replying
			// mid-DATA would corrupt the protocol.
			for {
				bodyLine, rerr := r.ReadString('\n')
				if rerr != nil {
					return
				}
				if strings.TrimSpace(bodyLine) == "." {
					break
				}
			}
			_ = w.PrintfLine("250 accepted")
		case strings.HasPrefix(cmd, "QUIT"):
			_ = w.PrintfLine("221 bye")
			return
		default:
			_ = w.PrintfLine("250 ok")
		}
	}
}

func (f *fakeSMTP) addr() string { return f.ln.Addr().String() }

func sendVia(t *testing.T, addr, username string) error {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("port: %v", err)
	}
	sender := NewSMTP(Config{Host: host, Port: port, From: "noreply@test", Username: username, Password: "pw"}, discardLogger())
	return sender.Send(t.Context(), Message{To: "dest@test", Subject: "probe", Body: "<p>hi</p>"})
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSMTPSkipsAuthWithoutCredentials(t *testing.T) {
	fake := startFakeSMTP(t)
	done := make(chan error, 1)
	go func() { done <- sendVia(t, fake.addr(), "") }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("send without credentials: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("send timed out")
	}

	sawAuth := false
	sawQuit := false
	for {
		select {
		case cmd := <-fake.commands:
			if strings.HasPrefix(cmd, "AUTH") {
				sawAuth = true
			}
			if strings.HasPrefix(cmd, "QUIT") {
				sawQuit = true
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no QUIT observed (auth=%v quit=%v)", sawAuth, sawQuit)
		}
		if sawQuit {
			break
		}
	}
	if sawAuth {
		t.Fatal("client must not attempt AUTH against a server that does not advertise it")
	}
}
