package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func sshTestServer(t *testing.T, legacyRSA, incompatibleCipher bool) (string, <-chan error) {
	t.Helper()
	var key any
	var err error
	if legacyRSA {
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	} else {
		_, key, err = ed25519.GenerateKey(rand.Reader)
	}
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if legacyRSA {
		signer, err = ssh.NewSignerWithAlgorithms(signer.(ssh.AlgorithmSigner), []string{ssh.KeyAlgoRSA})
		if err != nil {
			t.Fatal(err)
		}
	}
	config := &ssh.ServerConfig{PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if conn.User() != "root" || string(password) != "calculated-password" {
			return nil, errors.New("authentication rejected")
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	if incompatibleCipher {
		config.Ciphers = []string{ssh.InsecureCipherAES128CBC}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan error, 1)
	go func() {
		raw, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer raw.Close()
		connection, channels, requests, err := ssh.NewServerConn(raw, config)
		if err != nil {
			done <- err
			return
		}
		defer connection.Close()
		go ssh.DiscardRequests(requests)
		for channel := range channels {
			t.Error("SSH check requested a session or another channel")
			_ = channel.Reject(ssh.Prohibited, "authentication check only")
		}
		done <- connection.Wait()
	}()
	return listener.Addr().String(), done
}

func waitSSHServer(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("SSH connection was not closed")
		return nil
	}
}

func TestSSHPasswordAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		legacy, cipher bool
		wantError      bool
	}{
		{name: "modern", password: "calculated-password"},
		{name: "ssh-rsa only", password: "calculated-password", legacy: true},
		{name: "wrong password", password: "wrong-password", wantError: true},
		{name: "algorithm negotiation failed", password: "calculated-password", cipher: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			address, done := sshTestServer(t, tc.legacy, tc.cipher)
			checker := newSSHChecker()
			dials := 0
			checker.dial = func(ctx context.Context, network, target string) (net.Conn, error) {
				dials++
				if network != "tcp" || target != "device:54133" {
					t.Errorf("unexpected SSH target: %s %s", network, target)
				}
				return (&net.Dialer{}).DialContext(ctx, network, address)
			}
			checker.wait = func(context.Context, time.Duration) error {
				t.Error("authentication and algorithm errors must not be retried")
				return errors.New("unexpected retry")
			}
			err := checker.verify(context.Background(), "device:54133", tc.password)
			if (err != nil) != tc.wantError || dials != 1 {
				t.Fatalf("error = %v, dial count = %d", err, dials)
			}
			if err != nil && strings.Contains(err.Error(), tc.password) {
				t.Fatal("SSH error exposed password")
			}
			waitSSHServer(t, done)
			files, err := os.ReadDir(".")
			if err != nil || len(files) != 0 {
				t.Fatal("SSH check wrote a file")
			}
		})
	}
}

func TestSSHWaitsForStartup(t *testing.T) {
	address, done := sshTestServer(t, false, false)
	checker := newSSHChecker()
	dials, waits := 0, 0
	checker.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		dials++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 3*time.Second || time.Until(deadline) < 2*time.Second {
			t.Error("attempt must be limited to three seconds")
		}
		if dials < 3 {
			return nil, &net.OpError{Op: "dial", Net: network, Err: syscall.ECONNREFUSED}
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	checker.wait = func(ctx context.Context, delay time.Duration) error {
		waits++
		deadline, ok := ctx.Deadline()
		if delay != time.Second || !ok || time.Until(deadline) > 15*time.Second || time.Until(deadline) < 14*time.Second {
			t.Error("retry must wait one second within a fifteen-second budget")
		}
		return nil // Skip real waiting while exercising the same retry loop.
	}
	if err := checker.verify(context.Background(), "device:54133", "calculated-password"); err != nil || dials != 3 || waits != 2 {
		t.Fatalf("error = %v, dials = %d, waits = %d", err, dials, waits)
	}
	waitSSHServer(t, done)
}

func TestSSHHandshakeTimeoutAndCancellation(t *testing.T) {
	for _, cancelHandshake := range []bool{false, true} {
		t.Run(map[bool]string{false: "overall deadline", true: "cancel handshake"}[cancelHandshake], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			checker := newSSHChecker()
			checker.readyTimeout = 40 * time.Millisecond
			checker.dial = func(context.Context, string, string) (net.Conn, error) {
				client, server := net.Pipe()
				t.Cleanup(func() { _ = server.Close() })
				go func() {
					// Read the client's banner but never supply a server banner.
					buffer := make([]byte, 256)
					_, _ = server.Read(buffer)
					if cancelHandshake {
						cancel()
					}
					_, _ = io.Copy(io.Discard, server)
				}()
				return client, nil
			}
			err := checker.verify(ctx, "device:54133", "password")
			want := context.DeadlineExceeded
			if cancelHandshake {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("error = %v; want %v", err, want)
			}
		})
	}
}

func TestSSHAttemptDeadline(t *testing.T) {
	checker := newSSHChecker()
	checker.attemptTimeout = 20 * time.Millisecond
	checker.dial = func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		t.Cleanup(func() { _ = server.Close() })
		go io.Copy(io.Discard, server)
		return client, nil
	}
	stopped := errors.New("test stopped after timed-out attempt")
	checker.wait = func(context.Context, time.Duration) error { return stopped }
	if err := checker.verify(context.Background(), "device:54133", "password"); !errors.Is(err, stopped) {
		t.Fatalf("stalled handshake was not interrupted and retried: %v", err)
	}
}

func TestSSHCancelsDialAndRetry(t *testing.T) {
	for _, dialing := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		checker := newSSHChecker()
		checker.dial = func(attemptCtx context.Context, _, _ string) (net.Conn, error) {
			if dialing {
				cancel()
				<-attemptCtx.Done()
				return nil, attemptCtx.Err()
			}
			return nil, syscall.ECONNREFUSED
		}
		checker.wait = func(ctx context.Context, delay time.Duration) error {
			cancel()
			return waitForSSHRetry(ctx, delay)
		}
		if err := checker.verify(ctx, "device:54133", "password"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation = %v", err)
		}
		cancel()
	}
}
