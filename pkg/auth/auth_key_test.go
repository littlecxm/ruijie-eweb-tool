package auth

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// Vectors were independently generated with OpenSSL AES-256-CBC, -md md5,
// and salt 0001020304050607. Tests have no OpenSSL runtime dependency.
func TestOpenSSLCompatibility(t *testing.T) {
	cases := []struct{ password, passphrase, ciphertext string }{
		{"admin", "password", "U2FsdGVkX18AAQIDBAUGBz39o+yd79vxf0fdzvn+1HQ="},
		{"密码🙂", StoragePassphrase, "U2FsdGVkX18AAQIDBAUGBy2bMhMKKcDMfrAZZS5zHdY="},
		{"1234567890abcdef", "password", "U2FsdGVkX18AAQIDBAUGBzFJD1PWDQ0Ickqw8d+L5GPlbw9Ce2X0cVYiL8/fci58"},
		{"", "password", "U2FsdGVkX18AAQIDBAUGByPx2qU2vPDnvlcQJV3ZkF8="},
	}
	for _, tc := range cases {
		t.Run(tc.password, func(t *testing.T) {
			got, err := encrypt(tc.password, tc.passphrase, bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7}))
			if err != nil || got != tc.ciphertext {
				t.Fatalf("encrypt = %q, %v; want %q", got, err, tc.ciphertext)
			}
			plain, err := DecryptAuthKey(" \n"+tc.ciphertext+"\t", tc.passphrase)
			if err != nil || plain != tc.password {
				t.Fatalf("decrypt = %q, %v", plain, err)
			}
		})
	}
}

func TestRandomSalt(t *testing.T) {
	a, err := GenerateAuthKey("admin", StoragePassphrase)
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateAuthKey("admin", StoragePassphrase)
	if err != nil || a == b || strings.ContainsAny(a, " \r\n\t") {
		t.Fatalf("expected distinct whitespace-free salted passwords: %v", err)
	}
	if _, err := encrypt("admin", StoragePassphrase, bytes.NewReader(nil)); err == nil {
		t.Fatal("expected randomness failure")
	}
}

func TestInvalidCiphertexts(t *testing.T) {
	valid := "U2FsdGVkX18AAQIDBAUGBz39o+yd79vxf0fdzvn+1HQ="
	badPadding, _ := base64.StdEncoding.DecodeString("U2FsdGVkX18AAQIDBAUGBzFJD1PWDQ0Ickqw8d+L5GPlbw9Ce2X0cVYiL8/fci58")
	badPadding[31] ^= 0xff // The final padding byte in the second CBC block becomes 0xef.
	for _, input := range []string{"admin", "", "%%%%", base64.StdEncoding.EncodeToString([]byte("Salted__12345678")), valid[:len(valid)-4], base64.StdEncoding.EncodeToString(badPadding)} {
		if _, err := DecryptAuthKey(input, "password"); err == nil {
			t.Errorf("accepted invalid ciphertext %q", input)
		}
	}
	if _, err := DecryptAuthKey(valid, "wrong password"); err == nil {
		t.Fatal("accepted wrong passphrase")
	}
	if _, err := GenerateAuthKey(string([]byte{0xff}), "password"); err == nil {
		t.Fatal("accepted invalid UTF-8")
	}
}
