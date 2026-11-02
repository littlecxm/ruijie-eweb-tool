package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	passwordauth "github.com/littlecxm/ruijie-eweb-tool/pkg/auth"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

const firstPassphrase = "91c6b8f77b213d7aac85c93aafa34cb0"
const secondPassphrase = "0123456789abcdef0123456789abcdef"

func TestLoginFlowAndKeyRotation(t *testing.T) {
	for _, rotate := range []bool{false, true} {
		t.Run(fmt.Sprint(rotate), func(t *testing.T) {
			var pages, logins, authorities int
			passphrase := firstPassphrase
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/cgi-bin/luci/":
					pages++
					if r.Method != http.MethodGet {
						t.Error("login page must use GET")
					}
					fmt.Fprintf(w, `<script>params.pwd = GibberishAES.enc(passwordEl.value, "%s").replace(/\s+/g, '');</script>`, passphrase)
				case "/cgi-bin/luci/api/auth":
					logins++
					var request struct {
						Method string                `json:"method"`
						Params passwordauth.LoginReq `json:"params"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || r.URL.RawQuery != "" || request.Method != "login" || request.Params.Username != "admin" || !request.Params.Encry || request.Params.IsCheckReadAgreement != "true" {
						t.Error("incorrect login request")
					}
					ts, err := strconv.ParseInt(request.Params.Time, 10, 64)
					if err != nil || time.Since(time.Unix(ts, 0)).Abs() > 5*time.Second {
						t.Error("incorrect Unix timestamp")
					}
					plain, err := passwordauth.DecryptAuthKey(request.Params.Pwd, passphrase)
					if err != nil || plain != "密码🙂" {
						t.Error("login did not use current passphrase")
					}
					if rotate && logins == 1 {
						passphrase = secondPassphrase
						io.WriteString(w, `{"code":0,"data":{"reload":true},"error":null}`)
						return
					}
					io.WriteString(w, `{"code":0,"id":null,"data":{"token":"private-token","sn":"SN001","sid":"session +/?"},"error":null}`)
				case "/cgi-bin/luci/api/common":
					authorities++
					body, _ := io.ReadAll(r.Body)
					if r.URL.Query().Get("auth") != "session +/?" || string(body) != `{"method":"getAuthority","params":null}` {
						t.Error("incorrect authority request")
					}
					io.WriteString(w, `{"code":0,"data":{"hostname":"Ruijie","deviceRole":"AC","sysinfo":{"serial_num":"SN001","modelFullName":"RG-EG105G-P-E V2"},"capacity":{"unknown":true}},"error":null}`)
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			c, _ := client.New(server.URL)
			defer c.CloseIdleConnections()
			result, err := Login(context.Background(), c, "密码🙂")
			if err != nil {
				t.Fatal(err)
			}
			wantCalls := 1
			if rotate {
				wantCalls = 2
			}
			if pages != wantCalls || logins != wantCalls || authorities != 1 {
				t.Fatalf("unexpected calls: %d/%d/%d", pages, logins, authorities)
			}
			if result.Session.SN != "SN001" || result.Session.Token != "private-token" || result.Session.SID != "session +/?" || result.Session.ExpiresAt.Sub(result.Session.IssuedAt) != time.Hour || result.Authority.Sysinfo.ModelFullName != "RG-EG105G-P-E V2" {
				t.Fatal("incorrect login result")
			}
		})
	}
}

func TestLoginFailures(t *testing.T) {
	cases := []struct {
		name, loginData, authorityData string
		wantLogins, wantAuthorities    int
	}{
		{"rejected", `{}`, ``, 1, 0},
		{"missing token", `{"sid":"sid"}`, ``, 1, 0},
		{"missing sid", `{"token":"token"}`, ``, 1, 0},
		{"locked", `{"lockTime":60,"count":5}`, ``, 1, 0},
		{"reload twice", `{"reload":true}`, ``, 2, 0},
		{"missing sysinfo", `{"token":"token","sid":"sid"}`, `{}`, 1, 1},
		{"blank serial", `{"token":"token","sid":"sid"}`, `{"sysinfo":{"serial_num":"  "}}`, 1, 1},
		{"mismatched serial", `{"token":"token","sid":"sid","sn":"A"}`, `{"sysinfo":{"serial_num":"B"}}`, 1, 1},
		{"null login", `null`, ``, 1, 0},
		{"string login", `"incorrect password"`, ``, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logins, authorities int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/cgi-bin/luci/":
					fmt.Fprintf(w, `GibberishAES.enc(passwordEl.value, '%s')`, firstPassphrase)
				case "/cgi-bin/luci/api/auth":
					logins++
					fmt.Fprintf(w, `{"code":0,"data":%s,"error":null}`, tc.loginData)
				case "/cgi-bin/luci/api/common":
					authorities++
					fmt.Fprintf(w, `{"code":0,"data":%s,"error":null}`, tc.authorityData)
				}
			}))
			defer server.Close()
			c, _ := client.New(server.URL)
			defer c.CloseIdleConnections()
			result, err := Login(context.Background(), c, "admin")
			if err == nil || result != nil || logins != tc.wantLogins || authorities != tc.wantAuthorities {
				t.Fatalf("unexpected result/calls: %v, %d/%d", err, logins, authorities)
			}
			if tc.name == "locked" {
				var locked *LoginLockedError
				if !errors.As(err, &locked) || locked.Seconds != 60 || locked.Count != 5 {
					t.Fatalf("lost lock information: %v", err)
				}
			}
		})
	}
}

func TestPassphraseExtraction(t *testing.T) {
	for _, page := range []string{
		`GibberishAES.enc(passwordEl.value,"` + firstPassphrase + `")`,
		`GibberishAES . enc ( passwordEl.value , '` + firstPassphrase + `' )`,
	} {
		if key, err := ExtractLoginPassphrase([]byte(page)); err != nil || key != firstPassphrase {
			t.Fatalf("extraction failed: %v", err)
		}
	}
	for _, page := range []string{"<html>missing key</html>", `GibberishAES.enc(x,window.dynamicKey)`, `GibberishAES.enc(x,"` + firstPassphrase + `");GibberishAES.enc(x,"` + secondPassphrase + `")`} {
		if _, err := ExtractLoginPassphrase([]byte(page)); err == nil {
			t.Fatal("accepted missing or ambiguous passphrase")
		}
	}
}

func TestMissingKeyAndInvalidPasswordDoNotLogin(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/cgi-bin/luci/" {
			t.Error("posted credentials without key")
		}
		io.WriteString(w, "no encryption key")
	}))
	defer server.Close()
	c, _ := client.New(server.URL)
	defer c.CloseIdleConnections()
	for _, password := range []string{"", strings.Repeat("a", 65), strings.Repeat("🙂", 33)} {
		if _, err := Login(context.Background(), c, password); err == nil {
			t.Fatal("accepted invalid password")
		}
	}
	if requests != 0 {
		t.Fatal("invalid password caused network request")
	}
	if _, err := Login(context.Background(), c, "admin"); err == nil || requests != 1 {
		t.Fatal("missing key did not stop login")
	}
}
