package main

import "testing"

func TestUnquoteFilePath(t *testing.T) {
	for _, tt := range []struct {
		name, input, want string
		valid             bool
	}{
		{"empty", "", "", true},
		{"quoted empty", `""`, "", true},
		{"double quoted escapes", "\"/tmp/firm\\$ware\\`image\\\"\\\\.bin\"", "/tmp/firm$ware`image\"\\.bin", true},
		{"single quoted literals", `'/tmp/firm\ $ware.bin'`, `/tmp/firm\ $ware.bin`, true},
		{"double quoted literal escape", `"/tmp/firm\ ware.bin"`, `/tmp/firm\ ware.bin`, true},
		{"Windows separators", `C:\Users\name\firmware.bin`, `C:\Users\name\firmware.bin`, true},
		{"quoted Windows separators", `"C:\Users\name\firmware image.bin"`, `C:\Users\name\firmware image.bin`, true},
		{"trailing backslash", `/tmp/firmware\`, `/tmp/firmware\`, true},
		{"no variable expansion", `/tmp/$HOME.bin`, `/tmp/$HOME.bin`, true},
		{"no command substitution", "/tmp/$(echo firmware)`echo image`.bin", "/tmp/$(echo firmware)`echo image`.bin", true},
		{"unterminated single quote", `'/tmp/firmware.bin`, "", false},
		{"unterminated double quote", `"/tmp/firmware.bin`, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := unquoteFilePath(tt.input)
			if valid != tt.valid || (valid && got != tt.want) {
				t.Fatalf("unquoteFilePath(%q) = %q, %v; want %q, %v", tt.input, got, valid, tt.want, tt.valid)
			}
		})
	}
}
