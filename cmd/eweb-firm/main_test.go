package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/firm"
)

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeInput(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIFirmwareRoundTrip(t *testing.T) {
	for _, format := range []firm.FirmwareFormat{firm.FormatV1, firm.FormatV2} {
		t.Run(string(format), func(t *testing.T) {
			plain := bytes.Repeat([]byte{0, 1, 0xff, '\n'}, 1025)
			input := writeInput(t, "firmware image.bin", plain)
			args := []string{"enc", input}
			if format == firm.FormatV2 {
				args = append(args, "--salt", "3433333036333633", "--format", "v2")
			}
			var output bytes.Buffer
			if err := run(args, &output); err != nil {
				t.Fatal(err)
			}
			encrypted := readFile(t, input+".encrypted")
			if detected, err := firm.DetectFirmwareFormat(encrypted); err != nil || detected != format {
				t.Fatalf("encrypted format = %q, %v; want %q", detected, err, format)
			}
			if format == firm.FormatV2 {
				want, err := firm.EncryptFirmwareWithSalt(plain, [8]byte{'4', '3', '3', '0', '6', '3', '6', '3'})
				if err != nil || !bytes.Equal(encrypted, want) {
					t.Fatalf("CLI did not use the supplied salt: %v", err)
				}
			}
			if err := run([]string{"dec", input + ".encrypted"}, &output); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(readFile(t, input+".encrypted.decrypted"), plain) || !bytes.Equal(readFile(t, input), plain) {
				t.Fatal("round trip changed the firmware or its input")
			}
			if strings.Count(output.String(), "("+string(format)+")") != 2 {
				t.Fatalf("success messages do not report the format: %s", output.String())
			}
		})
	}
}

func TestCLIOutputAliases(t *testing.T) {
	for _, alias := range []string{"-out", "--out", "-o"} {
		t.Run(alias, func(t *testing.T) {
			input := writeInput(t, "firmware", []byte("firmware"))
			destination := input + ".custom"
			if err := runCLI([]string{"enc", input, alias, destination}, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if plain, _, err := firm.DecryptFirmware(readFile(t, destination)); err != nil || string(plain) != "firmware" {
				t.Fatalf("custom output is incorrect: %v", err)
			}
			if _, err := os.Stat(input + ".encrypted"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("custom destination also created the default output: %v", err)
			}
		})
	}
}

func TestCLIRejectsInvalidOptionsBeforeWriting(t *testing.T) {
	input := writeInput(t, "firmware", []byte("firmware"))
	cases := [][]string{
		nil, {"enc"}, {"dec", ""}, {"unknown", input},
		{"enc", input, "--unknown"},
		{"enc", input, "-out"}, {"enc", input, "-out", ""},
		{"enc", input, "-o", input + ".out", "--out", input + ".other"},
		{"enc", input, "--format", "v3"},
		{"enc", input, "--format", "v1", "--format", "v2"},
		{"dec", input, "--format", "v2"},
		{"dec", input, "--salt", "3433333036333633"},
		{"enc", input, "--salt", "3433333036333633"},
		{"enc", input, "--format", "v2", "--salt", "1234"},
		{"enc", input, "--format", "v2", "--salt", "gggggggggggggggg"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var output bytes.Buffer
			if err := runCLI(args, &output); err == nil {
				t.Fatal("invalid arguments accepted")
			}
			if output.Len() != 0 {
				t.Fatalf("failed command printed success: %s", output.String())
			}
		})
	}
	entries, err := os.ReadDir(filepath.Dir(input))
	if err != nil || len(entries) != 1 {
		t.Fatalf("invalid command created output: %v, %v", entries, err)
	}
}

func TestCLIRefusesToOverwriteFiles(t *testing.T) {
	input := writeInput(t, "firmware", []byte("original firmware"))
	existing := input + ".encrypted"
	if err := os.WriteFile(existing, []byte("existing output"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"enc", input}, {"enc", input, "-out", input}} {
		if err := runCLI(args, &bytes.Buffer{}); !errors.Is(err, os.ErrExist) {
			t.Fatalf("overwriting an existing file must fail with ErrExist: %v", err)
		}
	}
	if string(readFile(t, input)) != "original firmware" || string(readFile(t, existing)) != "existing output" {
		t.Fatal("failed command changed an existing file")
	}
	link := input + ".symlink"
	if err := os.Symlink(input, link); err != nil {
		t.Fatal(err)
	}
	if err := runCLI([]string{"enc", input, "-out", link}, &bytes.Buffer{}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwriting a symlink must fail with ErrExist: %v", err)
	}
	if string(readFile(t, input)) != "original firmware" {
		t.Fatal("failed command overwrote the symlink target")
	}
}

func TestCLIInvalidInputDoesNotCreateOutput(t *testing.T) {
	input := writeInput(t, "firmware", []byte(firm.MagicV2+"\n!!!!"))
	for _, args := range [][]string{{"dec", input}, {"enc", input + ".missing"}} {
		if err := runCLI(args, &bytes.Buffer{}); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	entries, err := os.ReadDir(filepath.Dir(input))
	if err != nil || len(entries) != 1 {
		t.Fatalf("failed command created output: %v, %v", entries, err)
	}
}

func TestHelpDoesNotRequireTerminal(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var output bytes.Buffer
		if err := run([]string{flag}, &output); err != nil || output.String() != usage {
			t.Fatalf("help output = %q, %v", output.String(), err)
		}
	}
}
