package overview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

// Sysinfo preserves device-specific UCI options and section metadata.
type Sysinfo map[string]json.RawMessage

// DeviceInfo memory values are in KiB, including the firmware's swap adjustment.
type DeviceInfo struct {
	LocalTime    string  `json:"local_time"`
	Uptime       float64 `json:"uptime"`
	MemTotal     float64 `json:"mem_total"`
	MemFree      float64 `json:"mem_free"`
	MemFreeTotal float64 `json:"mem_free_total"`
	MemUsed      float64 `json:"mem_used"`
}

type HostName struct {
	Hostname string `json:"hostname"`
}

func GetSysinfo(ctx context.Context, c *client.Client, sid string) (Sysinfo, error) {
	return get[Sysinfo](ctx, c, sid, "getSysinfo")
}

func GetDeviceInfo(ctx context.Context, c *client.Client, sid string) (*DeviceInfo, error) {
	info, err := get[DeviceInfo](ctx, c, sid, "getDeviceInfo")
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetUptime returns elapsed seconds since boot.
func GetUptime(ctx context.Context, c *client.Client, sid string) (float64, error) {
	return get[float64](ctx, c, sid, "getUptime")
}

func GetHostName(ctx context.Context, c *client.Client, sid string) (string, error) {
	info, err := get[HostName](ctx, c, sid, "getHostName")
	return info.Hostname, err
}

func get[T any](ctx context.Context, c *client.Client, sid, method string) (T, error) {
	var result T
	if strings.TrimSpace(sid) == "" {
		return result, errors.New("overview requires a sid")
	}
	if err := c.Call(ctx, "overview", client.ApiCommonReq{Method: method, Params: nil}, sid, &result); err != nil {
		return result, fmt.Errorf("overview.%s: %w", method, err)
	}
	return result, nil
}
