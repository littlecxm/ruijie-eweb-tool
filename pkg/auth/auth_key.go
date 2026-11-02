package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" // Required by the device's GibberishAES/OpenSSL protocol.
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// StoragePassphrase is the stable passphrase used for config.toml passwords.
// It is separate from the changing passphrase advertised by the device.
const StoragePassphrase = ""

// GenerateAuthKey encrypts a UTF-8 password in GibberishAES's salted format.
func GenerateAuthKey(password, passphrase string) (string, error) {
	return encrypt(password, passphrase, rand.Reader)
}

func encrypt(password, passphrase string, random io.Reader) (string, error) {
	if !utf8.ValidString(password) || !utf8.ValidString(passphrase) {
		return "", errors.New("password and passphrase must be valid UTF-8")
	}
	salt := make([]byte, 8)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key, iv := deriveKey(passphrase, salt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("initialize password cipher: %w", err)
	}
	padding := aes.BlockSize - len([]byte(password))%aes.BlockSize
	plaintext := append([]byte(password), bytes.Repeat([]byte{byte(padding)}, padding)...)
	result := append([]byte("Salted__"), salt...)
	result = append(result, make([]byte, len(plaintext))...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(result[16:], plaintext)
	return base64.StdEncoding.EncodeToString(result), nil
}

// DecryptAuthKey decrypts and validates a GibberishAES password ciphertext.
func DecryptAuthKey(encrypted, passphrase string) (string, error) {
	encrypted = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, encrypted)
	data, err := base64.StdEncoding.Strict().DecodeString(encrypted)
	if err != nil || len(data) < 32 || string(data[:8]) != "Salted__" || (len(data)-16)%aes.BlockSize != 0 {
		return "", errors.New("invalid encrypted password format")
	}
	key, iv := deriveKey(passphrase, data[8:16])
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("initialize password cipher: %w", err)
	}
	plaintext := make([]byte, len(data)-16)
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, data[16:])
	padding := int(plaintext[len(plaintext)-1])
	if padding < 1 || padding > aes.BlockSize || !bytes.Equal(plaintext[len(plaintext)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return "", errors.New("invalid encrypted password padding")
	}
	plaintext = plaintext[:len(plaintext)-padding]
	if !utf8.Valid(plaintext) {
		return "", errors.New("decrypted password is not valid UTF-8")
	}
	return string(plaintext), nil
}

// deriveKey implements OpenSSL EVP_BytesToKey with MD5 and one iteration.
func deriveKey(passphrase string, salt []byte) ([]byte, []byte) {
	material := make([]byte, 0, 48)
	var previous []byte
	for len(material) < 48 {
		h := md5.New()
		h.Write(previous)
		h.Write([]byte(passphrase))
		h.Write(salt)
		previous = h.Sum(nil)
		material = append(material, previous...)
	}
	return material[:32], material[32:48]
}
