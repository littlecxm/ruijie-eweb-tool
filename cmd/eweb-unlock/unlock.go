package main

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"charm.land/lipgloss/v2"
	passwordauth "github.com/littlecxm/ruijie-eweb-tool/pkg/auth"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
	moduleauth "github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/auth"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/devSta"
)

type sshVerifier func(context.Context, string, string) error

type unlockResult struct {
	host, password string
}

var keywordStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("204")).Background(lipgloss.Color("235"))

func runUnlock(ctx context.Context, endpoint, password string, verifyTLS bool, verify sshVerifier, progress func(string)) (*unlockResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if progress == nil {
		progress = func(string) {}
	}
	c, err := client.New(endpoint, client.WithTLSVerification(verifyTLS))
	if err != nil {
		return nil, fmt.Errorf("设备网址: %w", err)
	}
	defer c.CloseIdleConnections()
	// Use the normalized origin, discarding the Web port for SSH. JoinHostPort
	// also handles bracketed IPv6 addresses correctly.
	u, _ := url.Parse(c.Endpoint())
	host := u.Hostname()
	address := net.JoinHostPort(host, sshPort)
	progress("正在登录并获取设备信息…")
	result, err := moduleauth.Login(ctx, c, password)
	if err != nil {
		return nil, fmt.Errorf("登录失败: %w", err)
	}
	progress("正在开启开发者模式…")
	if err := devSta.ToggleDevelopMode(ctx, c, result.Session.SID, true); err != nil {
		return nil, fmt.Errorf("开启开发者模式失败: %w", err)
	}
	sshPassword, err := passwordauth.SshPasswdGenerate(result.Session.SN)
	if err != nil {
		return nil, fmt.Errorf("计算 SSH 密码失败: %w", err)
	}
	progress(fmt.Sprintf("正在验证 SSH 登录 %s（最多等待 15 秒）…", address))
	if err := verify(ctx, address, sshPassword); err != nil {
		return nil, fmt.Errorf("开发者模式请求已提交，SSH 验证失败: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &unlockResult{host: host, password: sshPassword}, nil
}

func renderUnlockResult(result *unlockResult) string {
	// Keep the command safe to paste even when the endpoint uses a hostname
	// containing shell metacharacters. IPv6 literals need brackets for ssh.
	commandHost := result.host
	if strings.Contains(commandHost, ":") {
		commandHost = "[" + commandHost + "]"
	}
	command := fmt.Sprintf("ssh -oHostKeyAlgorithms=+ssh-rsa -p %s %s", sshPort, shellQuote(sshUser+"@"+commandHost))
	return fmt.Sprintf("解锁成功\n主机: %s\n用户名: %s\n端口: %s\nSSH 密码: %s\n连接命令: %s\n",
		keywordStyle.Render(result.host), keywordStyle.Render(sshUser),
		keywordStyle.Render(sshPort), keywordStyle.Render(result.password), keywordStyle.Render(command))
}

func shellQuote(value string) string {
	// Single-quote the destination; endpoint validation permits hostnames with
	// characters that a shell would otherwise interpret.
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
