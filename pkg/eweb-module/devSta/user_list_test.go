package devSta

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

func TestGetUserList(t *testing.T) {
	const payload = `{"code":"0","total":"2","list":[{"mac":"7c:c2:94:d1:84:ad","userIp":"192.168.60.106","sn":"SN001","name":"手机","hostName":"phone","connectType":"wireless","ssid":"CiGA","band":"2.4G","rssi":-56,"onlinetime":"123"},{"mac":"00:11:22:33:44:55","userIp":"192.168.60.107","hostName":"desktop","connectType":"wire","port":3,"rssi":"","onlinetime":456}]}`
	for _, stringData := range []bool{true, false} {
		t.Run(map[bool]string{true: "noParse string", false: "object"}[stringData], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/cgi-bin/luci/api/cmd" || r.URL.Query().Get("auth") != "test sid+/?" {
					t.Error("incorrect authenticated cmd request")
				}
				body, _ := io.ReadAll(r.Body)
				want := `{"method":"devSta.get","params":{"module":"user_list","noParse":true,"async":null,"remoteIp":false,"data":{"devType":"all","dataType":"timely"},"device":"pc"}}`
				if string(body) != want {
					t.Errorf("request = %s", body)
				}
				var data any = json.RawMessage(payload)
				if stringData {
					data = payload
				}
				json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data, "error": nil})
			}))
			defer server.Close()
			c, err := client.New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer c.CloseIdleConnections()
			users, err := GetUserList(context.Background(), c, "test sid+/?")
			if err != nil {
				t.Fatal(err)
			}
			if users.Total != 2 || len(users.List) != 2 {
				t.Fatalf("unexpected user list: %+v", users)
			}
			wireless, wired := users.List[0], users.List[1]
			if wireless.UserIP != "192.168.60.106" || wireless.SSID != "CiGA" || wireless.Band != "2.4G" || wireless.RSSI != "-56" || wireless.OnlineTime != "123" {
				t.Errorf("wireless terminal = %+v", wireless)
			}
			if wired.Port != "3" || wired.RSSI != "" || wired.OnlineTime != "456" || wired.HostName != "desktop" {
				t.Errorf("wired terminal = %+v", wired)
			}
		})
	}
}

func TestDecodeUserList(t *testing.T) {
	for _, payload := range []string{
		`{"code":0,"total":0,"list":[]}`,
		`{"code":"0","total":"0","list":[]}`,
		`{"list":[]}`,
	} {
		users, err := decodeUserList(json.RawMessage(payload))
		if err != nil || users.Total != 0 || len(users.List) != 0 {
			t.Fatalf("empty list %s: %+v, %v", payload, users, err)
		}
	}
	users, err := decodeUserList(json.RawMessage(`{"list":[{"mac":"00:11:22:33:44:55"}]}`))
	if err != nil || users.Total != 1 {
		t.Fatalf("missing total: %+v, %v", users, err)
	}
	for _, payload := range []string{
		`null`, `[]`, `{}`, `"not JSON"`, `""`,
		`{"code":null,"list":[]}`, `{"code":true,"list":[]}`,
		`{"code":"invalid","list":[]}`,
		`{"code":0}`, `{"list":null}`, `{"list":{}}`,
		`{"total":-1,"list":[]}`, `{"total":"many","list":[]}`,
		`{"total":1.5,"list":[]}`, `{"total":null,"list":[]}`,
		`{"list":[{"rssi":true}]}`, `{"list":[{"port":{}}]}`,
	} {
		if _, err := decodeUserList(json.RawMessage(payload)); err == nil {
			t.Errorf("accepted invalid payload: %s", payload)
		}
	}
	for _, payload := range []string{
		`{"code":"1","msg":"collection failed"}`,
		`{"code":1,"err":"collection failed"}`,
	} {
		_, err := decodeUserList(json.RawMessage(payload))
		var apiErr *client.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != 1 || apiErr.Message != "collection failed" {
			t.Errorf("module failure = %v", err)
		}
	}
}

func TestGetUserListSessionAndCancellation(t *testing.T) {
	c, err := client.New("https://device")
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseIdleConnections()
	if _, err := GetUserList(context.Background(), c, " "); err == nil {
		t.Fatal("accepted missing SID")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetUserList(ctx, c, "private-session")
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "private-session") {
		t.Fatalf("cancellation lost cause or exposed SID: %v", err)
	}
}
