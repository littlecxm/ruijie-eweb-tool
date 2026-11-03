package devSta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

func developModeClient(t *testing.T, wantRequest, response string, status int) (*client.Client, *int) {
	t.Helper()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/cgi-bin/luci/api/cmd" || r.URL.Query().Get("auth") != "test sid+/?" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("incorrect authenticated cmd request")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != wantRequest {
			t.Errorf("request = %s; want %s", body, wantRequest)
		}
		if status != 0 {
			w.WriteHeader(status)
		}
		io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	c, err := client.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.CloseIdleConnections)
	return c, &calls
}

func TestGetDevelopMode(t *testing.T) {
	const request = `{"method":"devSta.get","params":{"module":"develop_mode","noParse":false,"async":null,"remoteIp":false,"device":"pc"}}`
	for _, tc := range []struct {
		data      string
		want      bool
		wantError bool
	}{
		{data: `{"developMode":"0"}`},
		{data: `{"developMode":"1"}`, want: true},
		{data: `{"developMode":"2"}`, wantError: true},
		{data: `{"developMode":1}`, wantError: true},
		{data: `{"developMode":null}`, wantError: true},
		{data: `{}`, wantError: true},
		{data: `null`, wantError: true},
		{data: `[]`, wantError: true},
	} {
		t.Run(tc.data, func(t *testing.T) {
			c, calls := developModeClient(t, request, `{"code":0,"id":null,"error":null,"data":`+tc.data+`}`, 0)
			got, err := GetDevelopMode(context.Background(), c, "test sid+/?")
			if got != tc.want || (err != nil) != tc.wantError || *calls != 1 {
				t.Fatalf("GetDevelopMode = %v, %v; calls = %d", got, err, *calls)
			}
		})
	}
}

func TestToggleDevelopMode(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			value := "0"
			if enabled {
				value = "1"
			}
			request := `{"method":"devSta.set","params":{"module":"develop_mode","noParse":false,"async":null,"remoteIp":false,"data":{"developMode":"` + value + `"},"device":"pc"}}`
			for _, tc := range []struct {
				name, response string
				status         int
				wantError      bool
			}{
				{name: "success", response: `{"code":0,"id":null,"error":null,"data":{"result":"success"}}`},
				{name: "rejected", response: `{"code":0,"data":{"result":"failed"}}`, wantError: true},
				{name: "boolean", response: `{"code":0,"data":{"result":true}}`, wantError: true},
				{name: "missing result", response: `{"code":0,"data":{}}`, wantError: true},
				{name: "null data", response: `{"code":0,"data":null}`, wantError: true},
				{name: "empty table", response: `{"code":0,"data":[]}`, wantError: true},
				{name: "missing data", response: `{"code":0}`, wantError: true},
				{name: "RPC error", response: `{"code":1,"error":"denied"}`, wantError: true},
				{name: "HTTP error", status: 403, wantError: true},
				{name: "invalid envelope", response: `not JSON`, wantError: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					c, calls := developModeClient(t, request, tc.response, tc.status)
					err := ToggleDevelopMode(context.Background(), c, "test sid+/?", enabled)
					if (err != nil) != tc.wantError || *calls != 1 {
						t.Fatalf("error = %v, calls = %d", err, *calls)
					}
				})
			}
		})
	}
}

func TestDevelopModeSessionAndCancellation(t *testing.T) {
	c, err := client.New("https://device")
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	for _, call := range []func(context.Context, string) error{
		func(ctx context.Context, sid string) error { _, err := GetDevelopMode(ctx, c, sid); return err },
		func(ctx context.Context, sid string) error { return ToggleDevelopMode(ctx, c, sid, true) },
	} {
		if err := call(context.Background(), " "); err == nil {
			t.Fatal("accepted missing SID")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := call(ctx, "private-session")
		if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private-session") {
			t.Fatalf("cancellation lost cause or exposed SID: %v", err)
		}
	}
}
