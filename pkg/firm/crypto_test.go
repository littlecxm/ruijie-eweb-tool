package firm

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func patterned(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i)
	}
	return data
}

// Digests were captured from the unmodified Ruijie_Toolkit.original.exe,
// using a fresh process for each input. The largest crosses its 4096-byte
// I/O boundary. These tests do not generate their own expected ciphertext.
func TestV1OriginalEXEVectors(t *testing.T) {
	vectors := []struct {
		size int
		hash string
	}{
		{0, "04bd4fcdaa6af7a3ca3540ff67ffff3c302538ad3eb7ffd1287836b0becd72a0"},
		{1, "0b8e13ded80697ccb3f4c81926e51029daca2375c58dfcbbd0495b4d5c694fa9"},
		{64, "cd2d6629f95b02f1e0b4a3eb95ea492d0191c46019503d912c68bebd05e16616"},
		{4097, "56519d73534e97dc7d9df7e20ddf742d79ecf5144f5fc43a74bd6eea28c92070"},
	}
	for _, v := range vectors {
		t.Run(fmt.Sprint(v.size), func(t *testing.T) {
			plain := patterned(v.size)
			got, err := EncryptFirmware(plain, FormatV1)
			if err != nil || fmt.Sprintf("%x", sha256.Sum256(got)) != v.hash {
				t.Fatalf("v1 original EXE mismatch: %v", err)
			}
			decrypted, format, err := DecryptFirmware(got)
			if err != nil || format != FormatV1 || !bytes.Equal(decrypted, plain) {
				t.Fatalf("v1 decryption failed: %v", err)
			}
		})
	}
}

// Independent Python hashlib + cryptography vectors, using the KDF
// parameters recovered from IDA and verified against both vendor samples.
func TestV2IndependentVectors(t *testing.T) {
	vectors := []struct {
		size int
		hash string
	}{
		{0, "414ae666d08470590ad4bfc4c9fde971462ee31ae802a466dbaa212cc5fdd768"},
		{1, "5eabddebf3edff89c9c931ed24b0baab77faad99f549f422bda49e3132e6ceda"},
		{16, "564d5300348676e89a3fd5d3e2c7ff18413992e98922664c9f8baaa9681f1b45"},
		{17, "929ed050cd8aaf8db4b17439e4e4d5e1193dd9ddd52081830d53bc03fa73221a"},
		{64, "53f2f5b50cc1d42e33a7ebe10a209c23e0ea605d768978c15acbbb464d942cca"},
		{4097, "6cd18e162f1499b037fa406a4566c0d062eada2df3be55f83e835ac8b596c571"},
	}
	salt := [8]byte{'4', '3', '3', '0', '6', '3', '6', '3'}
	for _, v := range vectors {
		t.Run(fmt.Sprint(v.size), func(t *testing.T) {
			plain := patterned(v.size)
			got, err := EncryptFirmwareWithSalt(plain, salt)
			if err != nil || fmt.Sprintf("%x", sha256.Sum256(got)) != v.hash {
				t.Fatalf("independent v2 ciphertext mismatch: %v", err)
			}
			decrypted, format, err := DecryptFirmware(got)
			if err != nil || format != FormatV2 || !bytes.Equal(decrypted, plain) {
				t.Fatalf("v2 decryption failed: %v", err)
			}
		})
	}
}

func TestV2VendorSamples(t *testing.T) {
	const dir = "EG_3.0(1)B11P430_EG105G-E-V2_13192420_encrypto_v2"
	vectors := []struct {
		name, hash string
		size       int
	}{
		{"EG_3.0(1)B11P430_EG105G-E-V2_13192420_sysupgrade_encrypto_v2.tar.bin", "39b3f5fc40f53e4f4f9976231cc4fef5c86af0333a69164ff78dbd3053b58617", 16455680},
		{"u-boot-eg105g-e-v2_encrypto_v2", "19adc2569d6b4cbedfc922c846d70d45ea8e8678e9fba003beef1502a4956144", 425004},
	}
	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, v.name))
			if os.IsNotExist(err) {
				t.Skip("optional vendor sample absent")
			}
			if err != nil {
				t.Fatal(err)
			}
			original := append([]byte(nil), raw...)
			plain, format, err := DecryptFirmware(raw)
			if err != nil || format != FormatV2 || len(plain) != v.size || fmt.Sprintf("%x", sha256.Sum256(plain)) != v.hash {
				t.Fatalf("vendor sample decryption mismatch: %v", err)
			}
			if !bytes.Equal(raw, original) {
				t.Fatal("decryption modified the input")
			}
			envelope, err := decodeV2(raw)
			if err != nil {
				t.Fatal(err)
			}
			var salt [8]byte
			copy(salt[:], envelope[8:16])
			reencrypted, err := EncryptFirmwareWithSalt(plain, salt)
			if err != nil || !bytes.Equal(reencrypted, original) {
				t.Fatalf("vendor ciphertext not reproduced byte for byte: %v", err)
			}
			if strings.HasSuffix(v.name, ".tar.bin") {
				archive := tar.NewReader(bytes.NewReader(plain))
				want := map[string]string{
					"sysupgrade-EG105G-E-V2/CONTROL": "ca21a67649a3a79d6c191b1b869199301320b64dd11914d12d8a32e14fd3d27f",
					"sysupgrade-EG105G-E-V2/root":    "428daeafb8fb7412950952d598fd6868371e652aa795a93064a0c8a8635ed316",
					"sysupgrade-EG105G-E-V2/kernel":  "0956eae4a99a7bd9b5df546a62f34534297246ec6b79c44fa9133c3b3b812aae",
				}
				for {
					header, err := archive.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					if header.Typeflag == tar.TypeDir {
						continue
					}
					hash := sha256.New()
					if _, err := io.Copy(hash, archive); err != nil {
						t.Fatal(err)
					}
					if hex.EncodeToString(hash.Sum(nil)) != want[header.Name] {
						t.Fatalf("tar member mismatch: %s", header.Name)
					}
					delete(want, header.Name)
				}
				if len(want) != 0 {
					t.Fatalf("missing tar members: %v", want)
				}
			}
		})
	}
}

func TestRejectMalformedCiphertext(t *testing.T) {
	// Fixed independent v2 vector for an empty plaintext.
	valid := []byte(MagicV2 + "\nU2FsdGVkX180MzMwNjM2M3RbCMk9AYAGpBPWjcuK4oU=\n")
	envelope, err := decodeV2(valid)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(body []byte) []byte {
		return []byte(MagicV2 + "\n" + base64.StdEncoding.EncodeToString(body) + "\n")
	}
	wrongMagic := append([]byte(nil), envelope...)
	wrongMagic[0] ^= 1
	// Flip the last byte in the penultimate ciphertext block. This
	// deterministically changes the final plaintext padding byte from
	// 0x10 to 0x11, which PKCS#7 must reject.
	longVector, err := EncryptFirmwareWithSalt(patterned(32), [8]byte{})
	if err != nil {
		t.Fatal(err)
	}
	badPadding, err := decodeV2(longVector)
	if err != nil {
		t.Fatal(err)
	}
	badPadding[len(badPadding)-17] ^= 1
	cases := [][]byte{
		nil, []byte("short"), []byte{0x1f, 0x8b, 0x08},
		[]byte(MagicV2), []byte(MagicV2 + "U2FsdGVk"),
		[]byte(MagicV2 + "\n!!!!"), valid[:len(valid)-3],
		encode(wrongMagic), encode(envelope[:31]), encode(badPadding),
	}
	for i, data := range cases {
		if _, _, err := DecryptFirmware(data); err == nil {
			t.Errorf("malformed case %d accepted", i)
		}
	}
	if _, err := EncryptFirmware(nil, "v3"); err == nil {
		t.Error("unsupported encryption format accepted")
	}
	// Header CRLF is supported without shifting the ciphertext by one.
	crlf := bytes.Replace(valid, []byte("\n"), []byte("\r\n"), -1)
	if plain, _, err := DecryptFirmware(crlf); err != nil || len(plain) != 0 {
		t.Fatalf("CRLF vector: %v", err)
	}
}

func TestV2RandomSalt(t *testing.T) {
	plain := patterned(64)
	first, err := EncryptFirmware(plain, FormatV2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncryptFirmware(plain, FormatV2)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("repeated v2 encryption reused salt")
	}
	for _, raw := range [][]byte{first, second} {
		got, _, err := DecryptFirmware(raw)
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("random-salt ciphertext failed to decrypt: %v", err)
		}
	}
}
