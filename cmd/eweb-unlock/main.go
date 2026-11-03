package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "已取消")
		} else {
			fmt.Fprintln(os.Stderr, "错误:", err)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, in *os.File, out io.Writer) error {
	flags := flag.NewFlagSet("eweb-unlock", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.Usage = func() {
		fmt.Fprintln(out, "用法: eweb-unlock [--verify-tls]")
		fmt.Fprintln(out, "填写设备网址和密码，开启开发者模式并验证 SSH 登录；不读写配置。")
		flags.PrintDefaults()
	}
	verifyTLS := flags.Bool("verify-tls", false, "验证设备 HTTPS 证书")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("直接运行 eweb-unlock 填写表单，无需子命令")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !term.IsTerminal(int(in.Fd())) {
		return errors.New("解锁表单需要交互终端，请直接在终端运行 eweb-unlock")
	}
	return runUnlockUI(ctx, in, out, func(ctx context.Context, endpoint, password string, progress func(string)) (*unlockResult, error) {
		return runUnlock(ctx, endpoint, password, *verifyTLS, verifySSH, progress)
	})
}
