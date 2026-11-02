package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/config"
	moduleauth "github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/auth"
	"github.com/pterm/pterm"
	"golang.org/x/term"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("eweb-tool", flag.ContinueOnError)
	configPath := flags.String("config", "config.toml", "配置文件路径")
	verifyTLS := flags.Bool("verify-tls", false, "验证设备 HTTPS 证书")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("直接运行 eweb-tool 进入向导，无需子命令")
	}
	cfg, err := config.LoadFromFile(*configPath)
	if errors.Is(err, os.ErrNotExist) {
		cfg, err = config.New(), nil
	}
	if err != nil {
		return err
	}
	if cfg.App.Debug {
		pterm.EnableDebugMessages()
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("登录向导需要交互终端，请直接在终端运行 eweb-tool")
	}
	pterm.DefaultHeader.WithFullWidth().Println("Ruijie Eweb Tool")
	return runWizard(ctx, cfg, *configPath, *verifyTLS, terminalPrompt{}, moduleauth.Login, os.Stdout)
}
