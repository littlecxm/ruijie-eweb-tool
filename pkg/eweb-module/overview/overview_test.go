package overview

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

func TestOverviewMethods(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		method, data string
		invoke       func(*client.Client) (any, error)
		want         any
	}{
		{"getSysinfo", `{"serial_num":"SN001",".name":"cfg001","features":["a","b"]}`, func(c *client.Client) (any, error) { return GetSysinfo(ctx, c, "sid") }, Sysinfo{"serial_num": json.RawMessage(`"SN001"`), ".name": json.RawMessage(`"cfg001"`), "features": json.RawMessage(`["a","b"]`)}},
		{"getDeviceInfo", `{"local_time":"2026/11/03 12:34:56","uptime":123.5,"mem_total":65536.5,"mem_free":2048,"mem_free_total":4096.5,"mem_used":60000}`, func(c *client.Client) (any, error) { return GetDeviceInfo(ctx, c, "sid") }, &DeviceInfo{LocalTime: "2026/11/03 12:34:56", Uptime: 123.5, MemTotal: 65536.5, MemFree: 2048, MemFreeTotal: 4096.5, MemUsed: 60000}},
		{"getUptime", `0`, func(c *client.Client) (any, error) { return GetUptime(ctx, c, "sid") }, float64(0)},
		{"getHostName", `{"hostname":"Ruijie"}`, func(c *client.Client) (any, error) { return GetHostName(ctx, c, "sid") }, "Ruijie"},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/cgi-bin/luci/api/overview" || r.URL.Query().Get("auth") != "sid" {
					t.Error("incorrect overview request")
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"method":"`+tc.method+`","params":null}` {
					t.Errorf("request = %s", body)
				}
				io.WriteString(w, `{"code":0,"data":`+tc.data+`,"error":null}`)
			}))
			defer server.Close()
			c, err := client.New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer c.CloseIdleConnections()
			got, err := tc.invoke(c)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, %v; want %#v", got, err, tc.want)
			}
		})
	}
}

func TestOverviewRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{`{"code":0,"data":null}`, `{"code":0,"data":"123"}`, `{"code":0,"data":0,"error":{"message":"denied"}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, body) }))
		c, _ := client.New(server.URL)
		if _, err := GetUptime(context.Background(), c, "sid"); err == nil {
			t.Errorf("accepted %s", body)
		}
		c.CloseIdleConnections()
		server.Close()
	}
	c, _ := client.New("http://device")
	defer c.CloseIdleConnections()
	if _, err := GetHostName(context.Background(), c, " "); err == nil {
		t.Fatal("accepted missing SID")
	}
}
