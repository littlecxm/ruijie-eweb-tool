package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCallRequest(t *testing.T) {
	const sid = "session +/?"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/cgi-bin/luci/api/common" || r.URL.Query().Get("auth") != sid || r.Header.Get("Content-Type") != "application/json" {
			t.Error("incorrect authenticated JSON request")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"method":"getAuthority","params":null}` {
			t.Errorf("unexpected request: %s", body)
		}
		io.WriteString(w, `{"code":0,"id":null,"data":{"value":"ok"},"error":null}`)
	}))
	defer server.Close()
	c, err := New(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	if c.http.Timeout != 15*time.Second || c.Endpoint() != server.URL {
		t.Fatal("incorrect defaults")
	}
	var result struct{ Value string }
	if err := c.Call(context.Background(), "common", ApiCommonReq{Method: "getAuthority"}, sid, &result); err != nil || result.Value != "ok" {
		t.Fatalf("Call = %+v, %v", result, err)
	}
}

func TestResponseFailures(t *testing.T) {
	cases := []struct {
		name, body  string
		wantCode    int
		wantMessage string
	}{
		{"business", `{"code":1,"error":{"code":1,"msg":"denied"}}`, 1, "denied"},
		{"message", `{"code":2,"message":"expired","error":null}`, 2, "expired"},
		{"string error", `{"code":3,"error":"denied"}`, 3, "denied"},
		{"zero with error", `{"code":0,"error":{"message":"rejected"}}`, 0, "rejected"},
		{"missing code", `{"data":{}}`, 0, ""},
		{"null code", `{"code":null,"data":{}}`, 0, ""},
		{"missing data", `{"code":0}`, 0, ""},
		{"null data", `{"code":0,"data":null}`, 0, ""},
		{"malformed", `<html>not JSON</html>`, 0, ""},
		{"wrong data type", `{"code":0,"data":"secret response"}`, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, tc.body) }))
			defer server.Close()
			c, _ := New(server.URL)
			defer c.CloseIdleConnections()
			var result struct{ Value string }
			err := c.Call(context.Background(), "auth", ApiCommonReq{Method: "login"}, "", &result)
			if err == nil || strings.Contains(err.Error(), "secret response") {
				t.Fatalf("expected safe error, got %v", err)
			}
			if tc.wantMessage != "" {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.Code != tc.wantCode || apiErr.Message != tc.wantMessage {
					t.Fatalf("unexpected business error: %v", err)
				}
			}
		})
	}
}

func TestTLSVerification(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	for _, verify := range []bool{false, true} {
		c, _ := New(server.URL, WithTLSVerification(verify))
		_, err := c.Get(context.Background(), "/cgi-bin/luci/")
		c.CloseIdleConnections()
		if (err != nil) != verify {
			t.Errorf("verify=%v, error=%v", verify, err)
		}
	}
}

func TestRequestTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	c, _ := New(server.URL)
	defer c.CloseIdleConnections()
	c.http.Timeout = 20 * time.Millisecond
	if _, err := c.Get(context.Background(), "/"); err == nil {
		t.Fatal("request did not time out")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Call(ctx, "common", ApiCommonReq{}, "private-session", &json.RawMessage{}); !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private-session") {
		t.Fatalf("canceled request leaked SID or lost cause: %v", err)
	}
}

func TestOriginAndHTTPFailures(t *testing.T) {
	for _, endpoint := range []string{"", "ftp://device", "https://user:password@device", "https://device/path", "https://device?auth=secret", "https://device#fragment"} {
		if _, err := New(endpoint); err == nil {
			t.Errorf("accepted endpoint %q", endpoint)
		}
	}
	var reached bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		reached = true
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "private-session")
	}))
	defer server.Close()
	c, _ := New(server.URL)
	defer c.CloseIdleConnections()
	if _, err := c.Get(context.Background(), "https://another-device/"); err == nil {
		t.Fatal("accepted another origin")
	}
	if _, err := c.Get(context.Background(), "/redirect"); err == nil || reached {
		t.Fatal("followed device redirect")
	}
	if err := c.Call(context.Background(), "common", ApiCommonReq{}, "private-session", nil); err == nil || strings.Contains(err.Error(), "private-session") {
		t.Fatalf("HTTP error leaked response body: %v", err)
	}
}
