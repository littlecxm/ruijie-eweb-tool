package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/devSta"
	"github.com/pterm/pterm"
)

const listTerminals = "列出在线终端"

func printTerminals(out io.Writer, users *devSta.UserList) error {
	fmt.Fprintf(out, "在线终端：%d 台\n", users.Total)
	if len(users.List) == 0 {
		fmt.Fprintln(out, "暂无在线终端。")
		return nil
	}
	rows := pterm.TableData{{"名称", "IP", "MAC", "接入方式", "接入设备", "SSID", "频段", "端口"}}
	for _, terminal := range users.List {
		connection := terminal.ConnectType
		switch connection {
		case "wire":
			connection = "有线"
		case "wireless":
			connection = "无线"
		}
		rows = append(rows, []string{
			terminalText(terminal.Name, terminal.HostName),
			terminalText(terminal.UserIP), terminalText(terminal.MAC),
			terminalText(connection), terminalText(terminal.DeviceAliasName, terminal.SN),
			terminalText(terminal.SSID), terminalText(terminal.Band), terminalText(string(terminal.Port)),
		})
	}
	return pterm.DefaultTable.WithWriter(out).WithHasHeader().WithData(rows).Render()
}

func terminalText(values ...string) string {
	for _, value := range values {
		if value = strings.Join(strings.Fields(value), " "); value != "" {
			return value
		}
	}
	return "-"
}
