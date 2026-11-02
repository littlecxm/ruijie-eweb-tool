package auth

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
)

const sshPwdPrefix = "\xa2\xaa\x1f\xf6\xe9\xf4\x45\x0f\xf0\xee\x32\xbb\x47\x62\xa5\xd4"
const sshPwdSuffix = "\x32\x03\x08\x51\x73\x56\x77\x91\xe1\x6f\xa2\x5b\x4c\xc2\xd5\x7d"

func SshPasswdGenerate(sn string) (string, error) {
	if i := strings.IndexByte(sn, 0); i >= 0 {
		sn = sn[:i]
	}
	if len(sn) >= 14 {
		return "", fmt.Errorf("SN must be at most 13 bytes: the original function reads beyond its buffer at 14 bytes")
	}

	digest := md5.Sum([]byte(sshPwdPrefix + sn + sshPwdSuffix))
	return hex.EncodeToString(digest[:8]), nil
}
