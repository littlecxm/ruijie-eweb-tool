package main

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	sshUser           = "root"
	sshPort           = "54133"
	sshReadyTimeout   = 15 * time.Second
	sshAttemptTimeout = 3 * time.Second
	sshRetryInterval  = time.Second
)

type sshChecker struct {
	dial           func(context.Context, string, string) (net.Conn, error)
	wait           func(context.Context, time.Duration) error
	readyTimeout   time.Duration
	attemptTimeout time.Duration
	retryInterval  time.Duration
}

func newSSHChecker() sshChecker {
	return sshChecker{
		dial: (&net.Dialer{}).DialContext, wait: waitForSSHRetry,
		readyTimeout: sshReadyTimeout, attemptTimeout: sshAttemptTimeout,
		retryInterval: sshRetryInterval,
	}
}

func verifySSH(ctx context.Context, address, password string) error {
	return newSSHChecker().verify(ctx, address, password)
}

func (v sshChecker) verify(ctx context.Context, address, password string) error {
	ctx, cancel := context.WithTimeout(ctx, v.readyTimeout)
	defer cancel()
	config := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{ssh.Password(password)},
		// This one-shot local-device check deliberately keeps no known_hosts.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		// Match OpenSSH's +ssh-rsa: prefer modern host-key algorithms, while
		// allowing older devices that only offer an RSA/SHA-1 signature.
		HostKeyAlgorithms: append(ssh.SupportedAlgorithms().HostKeys, ssh.KeyAlgoRSA),
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, v.attemptTimeout)
		err := v.attempt(attemptCtx, address, config)
		cancelAttempt()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			return nil
		}
		if !retryableSSHError(err) {
			return err
		}
		if err := v.wait(ctx, v.retryInterval); err != nil {
			return err
		}
	}
}

func (v sshChecker) attempt(ctx context.Context, address string, config *ssh.ClientConfig) error {
	conn, err := v.dial(ctx, "tcp", address)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	// ClientConfig.Timeout only covers TCP dialing. Closing the underlying
	// socket also interrupts a stalled banner, key exchange or authentication.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	connection, channels, requests, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return ssh.NewClient(connection, channels, requests).Close()
}

func retryableSSHError(err error) bool {
	var networkError net.Error
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		(errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary()))
}

func waitForSSHRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
