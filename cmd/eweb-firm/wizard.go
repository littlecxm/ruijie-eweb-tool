package main

import (
	"errors"
	"io"
	"strings"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/firm"
	"github.com/pterm/pterm"
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

type terminalPrompt struct{}

func (terminalPrompt) Select(label string, options []string) (string, error) {
	return pterm.DefaultInteractiveSelect.WithOptions(options).Show(label)
}

func (terminalPrompt) Input(label, defaultValue string) (string, error) {
	return pterm.DefaultInteractiveTextInput.WithDefaultValue(defaultValue).Show(label)
}

func runWizard(prompt prompter, out io.Writer) error {
	for {
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
			return err
		}
		path = strings.TrimSpace(path)
		if len(path) >= 2 && ((path[0] == '"' && path[len(path)-1] == '"') || (path[0] == '\'' && path[len(path)-1] == '\'')) {
			path = path[1 : len(path)-1]
		}
		if path == "" {
			pterm.Error.WithWriter(out).Println("输入文件路径不能为空")
			continue
		}
		if err := processFile(path, "", choice != decryptFirmware, format, nil, out); err != nil {
			pterm.Error.WithWriter(out).Println(err)
		}
	}
}
