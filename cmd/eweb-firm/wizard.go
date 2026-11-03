package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/littlecxm/ruijie-eweb-tool/internal/tui"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/firm"
)

const (
	decryptFirmware   = "固件解密 (自动识别 v1/v2)"
	encryptFirmwareV1 = "固件加密 (v1)"
	encryptFirmwareV2 = "固件加密 (v2)"
	exitWizard        = "退出"
)

type prompter interface {
	Select(label string, options []string) (string, error)
	Input(label, defaultValue string) (string, error)
}

func runWizard(prompt prompter, out io.Writer) error {
	return runWizardContext(context.Background(), prompt, out)
}

func runWizardContext(ctx context.Context, prompt prompter, out io.Writer) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		choice, err := prompt.Select("选择操作", []string{decryptFirmware, encryptFirmwareV1, encryptFirmwareV2, exitWizard})
		if err != nil {
			return err
		}
		format := firm.FormatV1
		switch choice {
		case exitWizard:
			return nil
		case decryptFirmware, encryptFirmwareV1:
		case encryptFirmwareV2:
			format = firm.FormatV2
		default:
			return errors.New("无效的操作选择")
		}
		path, err := prompt.Input("文件路径 (可拖入或粘贴)", "")
		if err != nil {
			if errors.Is(err, tui.ErrCanceled) {
				continue
			}
			return err
		}
		path = normalizeFilePath(path)
		if path == "" {
			fmt.Fprintln(out, "错误: 输入文件路径不能为空")
			continue
		}
		fmt.Fprintln(out, "正在处理固件…")
		if err := processFileContext(ctx, path, "", choice != decryptFirmware, format, nil, out); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Fprintln(out, "错误:", err)
		}
	}
}
