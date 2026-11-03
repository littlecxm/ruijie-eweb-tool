package firm

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

type FirmwareFormat string

const (
	FormatV1     FirmwareFormat = "v1"
	FormatV2     FirmwareFormat = "v2"
	MagicV1                     = "upgrade_crypt_v1!@2021"
	MagicV2                     = "upgrade_crypt_v2!@2023"
	v2Iterations                = 2023
	v2Password                  = "BR0khBR0khsHi4MkNGrJ421Yf&j8ceh6"
)

// DetectFirmwareFormat accepts only the version markers used by platform.sh.
func DetectFirmwareFormat(data []byte) (FirmwareFormat, error) {
	switch {
	case bytes.HasPrefix(data, []byte(MagicV1)):
		return FormatV1, nil
	case bytes.HasPrefix(data, []byte(MagicV2)):
		return FormatV2, nil
	default:
		return "", errors.New("missing upgrade_crypt_v1/v2 marker; select an encrypted firmware file from the package")
	}
}

// transformV1 matches sd-upgrade-crypto 0x400BDC and the original EXE's
// decrypt routine at 0x140001634. A new state is used for each file.
func transformV1(data []byte) []byte {
	state := [8]byte{1, 0, 0, 0, 1, 0, 1, 0}
	out := make([]byte, len(data))
	for i, value := range data {
		feedback := (state[0] + state[4] + state[5] + state[6]) & 1
		// The six-iteration loop leaves state[6] unchanged. Shifting seven
		// bytes instead would silently break compatibility with the device.
		copy(state[:6], state[1:7])
		state[7] = feedback
		var key byte
		for j, bit := range state {
			key |= bit << uint(j)
		}
		out[i] = value ^ key
	}
	return out
}

// deriveV2 matches EVP_BytesToKey(AES-256-CBC, MD5, salt, password,
// 32, 2023, key, iv), not PBKDF2.
func deriveV2(salt [8]byte) (key [32]byte, iv [16]byte) {
	var material [48]byte
	var previous []byte
	for offset := 0; offset < len(material); offset += md5.Size {
		hash := md5.New()
		hash.Write(previous)
		hash.Write([]byte(v2Password))
		hash.Write(salt[:])
		var digest [md5.Size]byte
		copy(digest[:], hash.Sum(nil))
		for i := 1; i < v2Iterations; i++ {
			digest = md5.Sum(digest[:])
		}
		copy(material[offset:], digest[:])
		previous = append(previous[:0], digest[:]...)
	}
	copy(key[:], material[:32])
	copy(iv[:], material[32:])
	return
}

func decodeV2(data []byte) (envelope []byte, err error) {
	// platform.sh removes the first line with sed '1d': the 22-byte
	// marker is followed by LF, so the Base64 body starts at byte 23.
	if !bytes.HasPrefix(data, []byte(MagicV2)) {
		return nil, errors.New("not a v2 firmware file")
	}
	body := data[len(MagicV2):]
	if bytes.HasPrefix(body, []byte("\r\n")) {
		body = body[2:]
	} else if bytes.HasPrefix(body, []byte("\n")) {
		body = body[1:]
	} else {
		return nil, errors.New("missing newline after the v2 firmware header")
	}
	// Go ignores CR/LF in the OpenSSL 64-column Base64. Strict also
	// validates the unused bits before an '=' padding character.
	envelope = make([]byte, base64.StdEncoding.DecodedLen(len(body)))
	n, err := base64.StdEncoding.Strict().Decode(envelope, body)
	if err != nil {
		return nil, fmt.Errorf("invalid or truncated v2 Base64 data: %w", err)
	}
	envelope = envelope[:n]
	if len(envelope) < 16+aes.BlockSize || !bytes.HasPrefix(envelope, []byte("Salted__")) {
		return nil, errors.New("invalid v2 Salted__ header or ciphertext too short")
	}
	if (len(envelope)-16)%aes.BlockSize != 0 {
		return nil, errors.New("v2 AES ciphertext length is not a multiple of 16 bytes")
	}
	return envelope, nil
}

// DecryptFirmware selects v1/v2 and leaves data unchanged. V1 has no
// integrity check. V2 validates envelope and PKCS#7 padding; the vendor
// format contains no cryptographic authentication tag.
func DecryptFirmware(data []byte) ([]byte, FirmwareFormat, error) {
	format, err := DetectFirmwareFormat(data)
	if err != nil {
		return nil, "", err
	}
	if format == FormatV1 {
		return transformV1(data[len(MagicV1):]), format, nil
	}
	envelope, err := decodeV2(data)
	if err != nil {
		return nil, format, err
	}
	var salt [8]byte
	copy(salt[:], envelope[8:16])
	key, iv := deriveV2(salt)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, format, err
	}
	body := envelope[16:]
	plain := make([]byte, len(body))
	cipher.NewCBCDecrypter(block, iv[:]).CryptBlocks(plain, body)
	padding := int(plain[len(plain)-1])
	if padding < 1 || padding > aes.BlockSize {
		return nil, format, errors.New("invalid v2 padding; ciphertext may be corrupted")
	}
	for _, value := range plain[len(plain)-padding:] {
		if int(value) != padding {
			return nil, format, errors.New("invalid v2 padding; ciphertext may be corrupted")
		}
	}
	return plain[:len(plain)-padding], format, nil
}

// EncryptFirmware uses fresh random salt for v2. The vendor's reader
// accepts all 8-byte salts; ASCII-only salt is not required.
func EncryptFirmware(plain []byte, format FirmwareFormat) ([]byte, error) {
	if format == FormatV1 {
		return append([]byte(MagicV1), transformV1(plain)...), nil
	}
	if format != FormatV2 {
		return nil, fmt.Errorf("unsupported firmware format: %q", format)
	}
	var salt [8]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	return EncryptFirmwareWithSalt(plain, salt)
}

// EncryptFirmwareWithSalt generates v2 with caller-supplied salt, useful
// for byte-identical reproduction of an existing firmware sample.
func EncryptFirmwareWithSalt(plain []byte, salt [8]byte) ([]byte, error) {
	key, iv := deriveV2(salt)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	envelope := make([]byte, 16+len(plain)+padding)
	copy(envelope, "Salted__")
	copy(envelope[8:16], salt[:])
	body := envelope[16:]
	copy(body, plain)
	for i := len(plain); i < len(body); i++ {
		body[i] = byte(padding)
	}
	cipher.NewCBCEncrypter(block, iv[:]).CryptBlocks(body, body)
	encoded := base64.StdEncoding.EncodeToString(envelope)
	out := make([]byte, 0, len(MagicV2)+1+len(encoded)+(len(encoded)+63)/64)
	out = append(out, MagicV2...)
	out = append(out, '\n')
	for len(encoded) > 64 {
		out = append(out, encoded[:64]...)
		out = append(out, '\n')
		encoded = encoded[64:]
	}
	out = append(out, encoded...)
	out = append(out, '\n')
	return out, nil
}
