package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	passwordauth "github.com/littlecxm/ruijie-eweb-tool/pkg/auth"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

const DefaultSessionLifetime = time.Hour

var passphrasePattern = regexp.MustCompile(`GibberishAES\s*\.\s*enc\s*\([^,;]*,\s*["']([0-9a-fA-F]{32})["']\s*\)`)

func ExtractLoginPassphrase(page []byte) (string, error) {
	matches := passphrasePattern.FindAllSubmatch(page, -1)
	if len(matches) == 0 {
		return "", errors.New("device login page does not contain an encryption passphrase")
	}
	passphrase := string(matches[0][1])
	for _, match := range matches[1:] {
		if string(match[1]) != passphrase {
			return "", errors.New("device login page contains conflicting encryption passphrases")
		}
	}
	return passphrase, nil
}

type LoginLockedError struct {
	Seconds int
	Count   int
}

func (e *LoginLockedError) Error() string {
	return fmt.Sprintf("device login is locked; try again in %d seconds", e.Seconds)
}

// Login authenticates admin and fetches the device identity before returning a
// usable session. Incorrect credentials and lockouts are never retried.
func Login(ctx context.Context, c *client.Client, password string) (*LoginResult, error) {
	if password == "" || !utf8.ValidString(password) || len(utf16.Encode([]rune(password))) > 64 {
		return nil, errors.New("password must contain 1 to 64 UTF-16 code units")
	}
	for attempt := 0; attempt < 2; attempt++ {
		page, err := c.Get(ctx, "/cgi-bin/luci/")
		if err != nil {
			return nil, fmt.Errorf("fetch login page: %w", err)
		}
		passphrase, err := ExtractLoginPassphrase(page)
		if err != nil {
			return nil, err
		}
		encrypted, err := passwordauth.GenerateAuthKey(password, passphrase)
		if err != nil {
			return nil, err
		}
		issuedAt := time.Now()
		params := passwordauth.LoginReq{
			Username: "admin", Time: strconv.FormatInt(issuedAt.Unix(), 10),
			Encry: true, Pwd: encrypted, IsCheckReadAgreement: "true",
		}
		var data LoginData
		if err := c.Call(ctx, "auth", client.ApiCommonReq{Method: "login", Params: params}, "", &data); err != nil {
			return nil, fmt.Errorf("login: %w", err)
		}
		if data.LockTime > 0 {
			return nil, &LoginLockedError{Seconds: data.LockTime, Count: data.Count}
		}
		if data.Reload {
			if attempt == 0 {
				continue
			}
			return nil, errors.New("device encryption passphrase changed again; please log in again")
		}
		if strings.TrimSpace(data.Token) == "" || strings.TrimSpace(data.SID) == "" {
			return nil, errors.New("login rejected: response is missing token or sid")
		}
		authority, err := GetAuthority(ctx, c, data.SID)
		if err != nil {
			return nil, err
		}
		sn := authority.Sysinfo.SerialNum
		if data.SN != "" && data.SN != sn {
			return nil, errors.New("login SN does not match sysinfo.serial_num")
		}
		return &LoginResult{
			Session:   Session{Token: data.Token, SID: data.SID, SN: sn, IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(DefaultSessionLifetime)},
			Authority: *authority,
		}, nil
	}
	return nil, errors.New("login did not complete")
}

func GetAuthority(ctx context.Context, c *client.Client, sid string) (*Authority, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, errors.New("getAuthority requires a sid")
	}
	var authority Authority
	if err := c.Call(ctx, "common", client.ApiCommonReq{Method: "getAuthority", Params: nil}, sid, &authority); err != nil {
		return nil, fmt.Errorf("getAuthority: %w", err)
	}
	if strings.TrimSpace(authority.Sysinfo.SerialNum) == "" {
		return nil, errors.New("getAuthority response is missing sysinfo.serial_num")
	}
	return &authority, nil
}
