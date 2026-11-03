package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
)

type Version struct {
	Current string          `json:"cur"`
	New     json.RawMessage `json:"new"`
}

// Operation reports acceptance by the device, not completion of an async task.
type Operation struct {
	Code         *int   `json:"code"`
	Message      string `json:"msg"`
	Shell        string `json:"shell,omitempty"`
	RestoreShell string `json:"_shell,omitempty"`
	Address      string `json:"addr,omitempty"`
}

type BackupFile struct {
	HasFile bool   `json:"hasfile"`
	Path    string `json:"path"`
}

type UpgradeOptions struct {
	Path      string `json:"path"`
	Check     bool   `json:"check"`
	IsPersist bool   `json:"isPersist"`
	IsEstAll  bool   `json:"isEstAll"`
}

func SetLang(ctx context.Context, c *client.Client, sid, lang string) error {
	if strings.TrimSpace(lang) == "" {
		return errors.New("language must not be empty")
	}
	return set(ctx, c, sid, "setLang", map[string]string{"lang": lang})
}

// GetVersion accepts "" for the current version, or "self" / "bottom" to
// execute the device's corresponding cloud version check.
func GetVersion(ctx context.Context, c *client.Client, sid, deviceType string) (*Version, error) {
	if deviceType != "" && deviceType != "self" && deviceType != "bottom" {
		return nil, errors.New("deviceType must be empty, self or bottom")
	}
	params := struct {
		DeviceType string `json:"deviceType,omitempty"`
	}{deviceType}
	result, err := call[Version](ctx, c, sid, "getVersion", params, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// AsyncTime only reads local device time in this firmware; it does not set it.
func AsyncTime(ctx context.Context, c *client.Client, sid string) (string, error) {
	return call[string](ctx, c, sid, "asyncTime", nil, nil)
}

func GetRebootShell(ctx context.Context, c *client.Client, sid, reason string) (string, error) {
	return call[string](ctx, c, sid, "getRebootShell", reason, nil)
}

func RestoreConfiguration(ctx context.Context, c *client.Client, sid string) (*Operation, error) {
	return operation(ctx, c, sid, "restoreConfiguration", nil, nil)
}

func Reboot(ctx context.Context, c *client.Client, sid string) (*Operation, error) {
	return operation(ctx, c, sid, "reboot", nil, nil)
}

func HasKillRemaining(ctx context.Context, c *client.Client, sid string) (bool, error) {
	raw, err := rawCall(ctx, c, sid, "hasKillRemaining")
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(string(raw)) {
	case "null", "false": // LuCI converts a top-level false result to null.
		return false, nil
	case "true":
		return true, nil
	default:
		return false, errors.New("invalid hasKillRemaining result")
	}
}

func DoKillRemaining(ctx context.Context, c *client.Client, sid string) error {
	raw, err := rawCall(ctx, c, sid, "doKillRemaining")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(raw)) != "null" {
		return errors.New("invalid doKillRemaining result")
	}
	return nil
}

// UpgradeConfirm submits an image already present on the device. It does not
// upload or validate the image. Check tests only /tmp/firmware.img on the device.
func UpgradeConfirm(ctx context.Context, c *client.Client, sid string, options UpgradeOptions) (*Operation, error) {
	if !path.IsAbs(options.Path) || strings.ContainsAny(options.Path, "\n\r`&$;|\x00") {
		return nil, errors.New("firmware path must be an absolute device path without command control characters")
	}
	return operation(ctx, c, sid, "upgradeConfirm", options, nil)
}

func UpgradeCancel(ctx context.Context, c *client.Client, sid string) (*Operation, error) {
	return operation(ctx, c, sid, "upgradeCancel", nil, nil)
}

// GetSessionTime returns seconds and accepts both UCI text and numeric defaults.
func GetSessionTime(ctx context.Context, c *client.Client, sid string) (int, error) {
	value, err := call[json.Number](ctx, c, sid, "getSessiontime", nil, nil)
	if err != nil {
		return 0, err
	}
	seconds, err := strconv.Atoi(value.String())
	if err != nil || seconds < 0 {
		return 0, errors.New("invalid getSessiontime result")
	}
	return seconds, nil
}

func HasBackupFile(ctx context.Context, c *client.Client, sid string) (*BackupFile, error) {
	result, err := call[BackupFile](ctx, c, sid, "hasBackupFile", nil, nil)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// RestoreConfig imports the existing fixed-path backup. localMAC controls the
// URL query parameter, not the JSON params object.
func RestoreConfig(ctx context.Context, c *client.Client, sid string, localMAC bool) (*Operation, error) {
	var query url.Values
	if localMAC {
		query = url.Values{"localMac": {"1"}}
	}
	return operation(ctx, c, sid, "restoreConfig", nil, query)
}

func DelRestoreConfig(ctx context.Context, c *client.Client, sid string) (string, error) {
	return call[string](ctx, c, sid, "delRestoreConfig", nil, nil)
}

func SetStep(ctx context.Context, c *client.Client, sid, steped string) error {
	if steped != "0" && steped != "1" {
		return errors.New("steped must be 0 or 1")
	}
	return set(ctx, c, sid, "setStep", map[string]string{"steped": steped})
}

// GetOverlayFreeSpace preserves df output. The firmware does not specify units.
func GetOverlayFreeSpace(ctx context.Context, c *client.Client, sid string) (string, error) {
	return call[string](ctx, c, sid, "getOverlayFreeSpace", nil, nil)
}

func set(ctx context.Context, c *client.Client, sid, method string, params any) error {
	result, err := call[struct {
		Result *bool `json:"result"`
	}](ctx, c, sid, method, params, nil)
	if err != nil {
		return err
	}
	if result.Result == nil {
		return fmt.Errorf("system.%s: response is missing result", method)
	}
	if !*result.Result {
		return fmt.Errorf("system.%s: device rejected the setting", method)
	}
	return nil
}

func operation(ctx context.Context, c *client.Client, sid, method string, params any, query url.Values) (*Operation, error) {
	result, err := call[Operation](ctx, c, sid, method, params, query)
	if err != nil {
		return nil, err
	}
	if result.Code == nil {
		return nil, fmt.Errorf("system.%s: response is missing code", method)
	}
	if *result.Code != 0 {
		return nil, fmt.Errorf("system.%s: %w", method, &client.APIError{Code: *result.Code, Message: result.Message})
	}
	return &result, nil
}

func call[T any](ctx context.Context, c *client.Client, sid, method string, params any, query url.Values) (T, error) {
	var result T
	if strings.TrimSpace(sid) == "" {
		return result, errors.New("system requires a sid")
	}
	if err := c.CallWithQuery(ctx, "system", client.ApiCommonReq{Method: method, Params: params}, sid, query, &result); err != nil {
		return result, fmt.Errorf("system.%s: %w", method, err)
	}
	return result, nil
}

func rawCall(ctx context.Context, c *client.Client, sid, method string) (json.RawMessage, error) {
	if strings.TrimSpace(sid) == "" {
		return nil, errors.New("system requires a sid")
	}
	data, err := c.CallRaw(ctx, "system", client.ApiCommonReq{Method: method, Params: nil}, sid)
	if err != nil {
		return nil, fmt.Errorf("system.%s: %w", method, err)
	}
	return data, nil
}
