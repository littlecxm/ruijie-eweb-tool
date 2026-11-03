package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/littlecxm/ruijie-eweb-tool/internal/tui"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/firm"
)

func TestWizardCanceledPathReturnsToMenu(t *testing.T) {
	prompt := &fakePrompt{choices: []string{encryptFirmwareV1, exitWizard}, inputErr: tui.ErrCanceled}
	if err := runWizard(prompt, &bytes.Buffer{}); err != nil || len(prompt.choices) != 0 {
		t.Fatalf("cancel path: %v, choices=%v", err, prompt.choices)
	}
}

func TestCanceledFirmwareTaskCreatesNoOutput(t *testing.T) {
	input := writeInput(t, "firmware", []byte("firmware"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := processFileContext(ctx, input, "", true, firm.FormatV1, nil, &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	if _, err := os.Stat(input + ".encrypted"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled task wrote output: %v", err)
	}
}

type fakePrompt struct {
	choices   []string
	inputs    []string
	selectErr error
	inputErr  error
}

func (p *fakePrompt) Select(_ string, options []string) (string, error) {
	if p.selectErr != nil {
		return "", p.selectErr
	}
	if len(p.choices) == 0 {
		return "", errors.New("unexpected selection prompt")
	}
	choice := p.choices[0]
	p.choices = p.choices[1:]
	for _, option := range options {
		if option == choice {
			return choice, nil
		}
	}
	return "", errors.New("expected option missing from menu: " + choice)
}

func (p *fakePrompt) Input(_, _ string) (string, error) {
	if p.inputErr != nil {
		return "", p.inputErr
	}
	if len(p.inputs) == 0 {
		return "", errors.New("unexpected input prompt")
	}
	input := p.inputs[0]
	p.inputs = p.inputs[1:]
	return input, nil
}

func TestWizardFirmwareRoundTrip(t *testing.T) {
	for _, choice := range []string{encryptFirmwareV1, encryptFirmwareV2} {
		t.Run(choice, func(t *testing.T) {
			input := writeInput(t, "firmware image.bin", []byte("test firmware"))
			prompt := &fakePrompt{
				choices: []string{choice, decryptFirmware, exitWizard},
				inputs:  []string{"  \"" + input + "\"  ", "'" + input + ".encrypted'"},
			}
			var output bytes.Buffer
			if err := runWizard(prompt, &output); err != nil {
				t.Fatal(err)
			}
			if string(readFile(t, input+".encrypted.decrypted")) != "test firmware" {
				t.Fatal("wizard round trip changed the firmware")
			}
			if !strings.Contains(output.String(), "加密成功") || !strings.Contains(output.String(), "解密成功") {
				t.Fatalf("missing success feedback: %s", output.String())
			}
		})
	}
}

func TestWizardAcceptsPastedFilePaths(t *testing.T) {
	for _, tt := range []struct {
		name, filename, input string
	}{
		{"spaces", "firmware image.bin", `{dir}/firmware image.bin`},
		{"escaped spaces", "firmware image.bin", `{dir}/firmware\ image.bin`},
		{"escaped trailing space", "firmware.bin ", `{dir}/firmware.bin\ `},
		{"single quotes", "firmware image.bin", `'{dir}/firmware image.bin'`},
		{"double quotes", "firmware image.bin", `"{dir}/firmware image.bin"`},
		{"escaped symbols", "固件 (v2) [a] {b} & ! # $name;%.bin", `{dir}/固件\ \(v2\)\ \[a\]\ \{b\}\ \&\ \!\ \#\ \$name\;\%.bin`},
		{"escaped quotes", `firm'ware"image.bin`, `{dir}/firm\'ware\"image.bin`},
		{"escaped backslash", `firm\ware.bin`, `{dir}/firm\\ware.bin`},
		{"quoted apostrophe", "firm'ware image.bin", `'{dir}/firm'\''ware image.bin'`},
		{"mixed quotes", "firmware image.bin", `{dir}/firm"ware image".bin`},
		{"literal quotes", `"firmware".bin`, `{dir}/"firmware".bin`},
		{"literal apostrophe", "firm'ware.bin", `{dir}/firm'ware.bin`},
		{"literal backslash space", `firm\ ware.bin`, `{dir}/firm\ ware.bin`},
		{"literal escaped symbols", `firm\(ware\).bin`, `{dir}/firm\(ware\).bin`},
		{"quoted literal backslash", `firm\ware.bin`, `"{dir}/firm\ware.bin"`},
		{"quoted trailing space", "firmware.bin ", `'{dir}/firmware.bin '`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := writeInput(t, tt.filename, []byte("test firmware"))
			pasted := strings.ReplaceAll(tt.input, "{dir}", filepath.ToSlash(filepath.Dir(input)))
			prompt := &fakePrompt{
				choices: []string{encryptFirmwareV1, exitWizard},
				inputs:  []string{"  " + pasted + "  "},
			}
			var output bytes.Buffer
			if err := runWizard(prompt, &output); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "加密成功") {
				t.Fatalf("pasted path %q failed: %s", pasted, output.String())
			}
			plain, _, err := firm.DecryptFirmware(readFile(t, input+".encrypted"))
			if err != nil || string(plain) != "test firmware" {
				t.Fatalf("pasted path selected the wrong firmware: %q, %v", plain, err)
			}
		})
	}
}

func TestWizardRecoversFromFileErrors(t *testing.T) {
	input := writeInput(t, "firmware", []byte("unencrypted firmware"))
	prompt := &fakePrompt{
		choices: []string{decryptFirmware, decryptFirmware, encryptFirmwareV1, exitWizard},
		inputs:  []string{"  ", input, input},
	}
	var output bytes.Buffer
	if err := runWizard(prompt, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "输入文件路径不能为空") || !strings.Contains(output.String(), "missing upgrade_crypt_v1/v2 marker") {
		t.Fatalf("missing error feedback: %s", output.String())
	}
	readFile(t, input+".encrypted")
}

func TestWizardPropagatesPromptErrors(t *testing.T) {
	errInterrupted := errors.New("prompt interrupted")
	for _, prompt := range []*fakePrompt{
		{selectErr: errInterrupted},
		{choices: []string{decryptFirmware}, inputErr: errInterrupted},
	} {
		if err := runWizard(prompt, &bytes.Buffer{}); !errors.Is(err, errInterrupted) {
			t.Fatalf("prompt error was lost: %v", err)
		}
	}
}

func TestWizardExitDoesNotRequestInput(t *testing.T) {
	if err := runWizard(&fakePrompt{choices: []string{exitWizard}}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
}
