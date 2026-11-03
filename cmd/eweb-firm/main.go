package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/littlecxm/ruijie-eweb-tool/internal/tui"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/firm"
	"golang.org/x/term"
)

const usage = `Ruijie Eweb Firmware Tool — v1/v2
  eweb-firm dec <file> [-out <output>]
  eweb-firm enc <file> [--format v1|v2] [-out <output>] [--salt <hex>]

无参数启动全屏 TUI。解密自动识别 v1/v2；加密默认 v1。
默认输出追加 .decrypted / .encrypted；已有文件不会覆盖。
-o 和 --out 是 -out 的别名。
--salt 接受 16 个十六进制字符，仅适用于 v2 加密；通常省略以使用随机 Salt。
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) != 0 {
		return runCLI(args, out)
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("交互菜单需要终端，请直接运行 eweb-firm，或使用 enc/dec 子命令；用法见 --help")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return tui.Run(ctx, "eweb-firm", out, func(ctx context.Context, ui *tui.UI) error {
		return runWizardContext(ctx, ui, ui)
	})
}

// Validate the entire input before creating output. O_EXCL also prevents
// overwriting input when the same file is used as the output path.
func writeNewFile(path string, data []byte) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() {
		if file != nil {
			_ = file.Close()
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	n, err := file.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		return err
	}
	err = file.Close()
	file = nil
	return err
}

func processFile(input, output string, encrypt bool, format firm.FirmwareFormat, salt *[8]byte, out io.Writer) error {
	return processFileContext(context.Background(), input, output, encrypt, format, salt, out)
}

func processFileContext(ctx context.Context, input, output string, encrypt bool, format firm.FirmwareFormat, salt *[8]byte, out io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("读取输入文件: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var result []byte
	if encrypt {
		if salt != nil {
			if format != firm.FormatV2 {
				return errors.New("--salt 仅适用于 v2 加密")
			}
			result, err = firm.EncryptFirmwareWithSalt(data, *salt)
		} else {
			result, err = firm.EncryptFirmware(data, format)
		}
	} else {
		result, format, err = firm.DecryptFirmware(data)
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if output == "" {
		if encrypt {
			output = input + ".encrypted"
		} else {
			output = input + ".decrypted"
		}
	}
	if err := writeNewFile(output, result); err != nil {
		return fmt.Errorf("保存输出文件 (已有文件不会覆盖): %w", err)
	}
	action := "解密"
	if encrypt {
		action = "加密"
	}
	fmt.Fprintf(out, "%s成功 (%s): %s [%d bytes]\n", action, format, output, len(result))
	return nil
}

func runCLI(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	if args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(out, usage)
		return nil
	}
	if (args[0] != "enc" && args[0] != "dec") || len(args) < 2 || args[1] == "" {
		return errors.New(usage)
	}
	encrypt := args[0] == "enc"
	format := firm.FormatV1
	var destination string
	var salt *[8]byte
	seen := make(map[string]bool)
	for i := 2; i < len(args); i++ {
		option := args[i]
		if option == "-o" || option == "--out" {
			option = "-out"
		}
		if option != "-out" && option != "--format" && option != "--salt" {
			return fmt.Errorf("未知参数: %s", option)
		}
		if seen[option] || i+1 == len(args) || args[i+1] == "" {
			return fmt.Errorf("选项重复或缺少参数: %s", option)
		}
		seen[option] = true
		i++
		switch option {
		case "-out":
			destination = args[i]
		case "--format":
			if !encrypt || (args[i] != "v1" && args[i] != "v2") {
				return errors.New("--format 仅用于加密，值必须为 v1 或 v2")
			}
			format = firm.FirmwareFormat(args[i])
		case "--salt":
			if !encrypt {
				return errors.New("--salt 仅用于 v2 加密")
			}
			decoded, err := hex.DecodeString(args[i])
			if err != nil || len(decoded) != 8 {
				return errors.New("--salt 需要 16 个十六进制字符 (8 字节)")
			}
			salt = new([8]byte)
			copy(salt[:], decoded)
		}
	}
	if salt != nil && format != firm.FormatV2 {
		return errors.New("--salt 仅适用于 v2 加密")
	}
	return processFile(args[1], destination, encrypt, format, salt, out)
}
