package main

import (
	"bytes"
	"context"
	"errors"
	"os"
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
