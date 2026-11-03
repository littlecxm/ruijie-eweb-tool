package devSta

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

// GetDevelopMode reports whether developer mode is enabled on the device.
func GetDevelopMode(ctx context.Context, c *client.Client, sid string) (bool, error) {
	var result struct {
		DevelopMode string `json:"developMode"`
	}
	if err := callDevelopMode(ctx, c, sid, "devSta.get", nil, &result); err != nil {
		return false, err
	}
	switch result.DevelopMode {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, errors.New("develop_mode response must contain developMode \"0\" or \"1\"")
	}
}

// ToggleDevelopMode explicitly enables or disables developer mode. It submits
// the setting once; enabling does not guarantee that SSH is ready yet.
func ToggleDevelopMode(ctx context.Context, c *client.Client, sid string, enabled bool) error {
	value := "0"
	if enabled {
		value = "1"
	}
	data := struct {
		DevelopMode string `json:"developMode"`
	}{DevelopMode: value}
	var result struct {
		Result string `json:"result"`
	}
	if err := callDevelopMode(ctx, c, sid, "devSta.set", data, &result); err != nil {
		return err
	}
	if result.Result != "success" {
		return fmt.Errorf("develop_mode setting rejected: result %q", result.Result)
	}
	return nil
}

func callDevelopMode(ctx context.Context, c *client.Client, sid, method string, data, result any) error {
	if strings.TrimSpace(sid) == "" {
		return errors.New("develop_mode requires a sid")
	}
	request := client.ApiCommonReq{
		Method: method,
		Params: DevStaReq{
			Module: "develop_mode", NoParse: false, Data: data, Device: "pc",
		},
	}
	if err := c.Call(ctx, "cmd", request, sid, result); err != nil {
		return fmt.Errorf("%s develop_mode: %w", method, err)
	}
	return nil
}
