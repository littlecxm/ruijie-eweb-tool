package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"sort"

	passwordauth "github.com/littlecxm/ruijie-eweb-tool/pkg/auth"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/config"
	moduleauth "github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/auth"
)

const defaultEndpoint = "https://192.168.110.1"

const (
	newDevice  = "新增设备"
	exitWizard = "退出"
)

type prompter interface {
	Select(label string, options []string) (string, error)
	Input(label, defaultValue string) (string, error)
	Password(label string) (string, error)
}

type loginFunc func(context.Context, *client.Client, string) (*moduleauth.LoginResult, error)

func runWizard(ctx context.Context, cfg *config.Config, path string, verifyTLS bool, prompt prompter, login loginFunc, out io.Writer) error {
	sns := make([]string, 0, len(cfg.Device))
	for sn := range cfg.Device {
		sns = append(sns, sn)
	}
	sort.Strings(sns)
	choices := make([]string, 0, len(sns)+2)
	devices := make(map[string]string, len(sns))
	for _, sn := range sns {
		label := fmt.Sprintf("%s (%s)", sn, cfg.Device[sn].Endpoint)
		choices = append(choices, label)
		devices[label] = sn
	}
	choices = append(choices, newDevice, exitWizard)
	choice, err := prompt.Select("选择设备", choices)
	if err != nil {
		return err
	}
	if choice == exitWizard {
		return nil
	}
	var endpoint, password, selectedSN string
	needsPassword := true
	if choice == newDevice {
		endpoint, err = prompt.Input("设备网址", defaultEndpoint)
		if err != nil {
			return err
		}
	} else {
		var ok bool
		selectedSN, ok = devices[choice]
		if !ok {
			return errors.New("无效的设备选择")
		}
		stored := cfg.Device[selectedSN]
		endpoint = stored.Endpoint
		password, err = passwordauth.DecryptAuthKey(stored.Password, passwordauth.StoragePassphrase)
		needsPassword = err != nil || password == ""
		if needsPassword {
			fmt.Fprintln(out, "已保存的密码是旧格式或密文无效，请重新输入。")
		}
	}
	c, err := client.New(endpoint, client.WithTLSVerification(verifyTLS))
	if err != nil {
		return err
	}
	defer c.CloseIdleConnections()
	var result *moduleauth.LoginResult
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if needsPassword {
			password, err = prompt.Password("输入设备密码")
			if err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprintln(out, "正在登录并获取设备信息…")
		result, err = login(ctx, c, password)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var locked *moduleauth.LoginLockedError
		if needsPassword || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.As(err, &locked) {
			return err
		}
		fmt.Fprintf(out, "使用已保存密码登录失败：%v\n请重新输入设备密码。\n", err)
		needsPassword = true
	}
	if err := saveLogin(cfg, path, selectedSN, c.Endpoint(), password, result); err != nil {
		return err
	}
	model := result.Authority.Sysinfo.ModelFullName
	if model == "" {
		model = result.Authority.Sysinfo.Model
	}
	fmt.Fprintf(out, "登录成功：SN=%s，型号=%s，设备地址=%s\n配置已保存至 %s\n", result.Session.SN, model, c.Endpoint(), path)
	return runDeviceMenu(ctx, c, result.Session.SID, prompt, out)
}

func saveLogin(cfg *config.Config, path, selectedSN, endpoint, password string, result *moduleauth.LoginResult) error {
	if result == nil || result.Authority.Sysinfo.SerialNum == "" {
		return errors.New("设备未返回 sysinfo.serial_num，未保存配置")
	}
	sn := result.Authority.Sysinfo.SerialNum
	if result.Session.SN != sn || (selectedSN != "" && selectedSN != sn) {
		return errors.New("设备 SN 与所选设备不一致，未保存配置")
	}
	encrypted, err := passwordauth.GenerateAuthKey(password, passwordauth.StoragePassphrase)
	if err != nil {
		return err
	}
	next := *cfg
	next.Device = maps.Clone(cfg.Device)
	if next.Device == nil {
		next.Device = make(map[string]config.DeviceConfig)
	}
	next.Device[sn] = config.DeviceConfig{Endpoint: endpoint, Password: encrypted}
	if err := next.SaveToFile(path); err != nil {
		return fmt.Errorf("登录成功，但保存配置失败: %w", err)
	}
	*cfg = next
	return nil
}
