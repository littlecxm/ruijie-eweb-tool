package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/littlecxm/ruijie-eweb-tool/internal/tui"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/client"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/devSta"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/overview"
	"github.com/littlecxm/ruijie-eweb-tool/pkg/eweb-module/system"
)

const (
	backMenu      = "返回上级菜单"
	cancelAction  = "取消"
	confirmAction = "确认执行"
)

type menuAction struct {
	label string
	run   func() (endSession bool, err error)
}

type deviceMenu struct {
	ctx    context.Context
	c      *client.Client
	sid    string
	prompt prompter
	out    io.Writer
}

// Interaction failures leave the wizard; device/API failures keep its menu open.
type interactionError struct{ error }

func (e interactionError) Unwrap() error { return e.error }

func runDeviceMenu(ctx context.Context, c *client.Client, sid string, prompt prompter, out io.Writer) error {
	m := &deviceMenu{ctx: ctx, c: c, sid: sid, prompt: prompt, out: out}
	_, err := m.loop("选择操作", exitWizard, []menuAction{
		{listTerminals, func() (bool, error) {
			fmt.Fprintln(out, "正在获取在线终端…")
			users, err := devSta.GetUserList(ctx, c, sid)
			if err != nil {
				return false, err
			}
			return false, printTerminals(out, users)
		}},
		m.submenu("设备概览", m.overviewActions()),
		m.submenu("系统状态", m.statusActions()),
		m.submenu("系统设置", m.settingsActions()),
		m.submenu("备份与恢复", m.backupActions()),
		m.submenu("固件升级", m.firmwareActions()),
		m.submenu("设备维护", m.maintenanceActions()),
	})
	return err
}

func (m *deviceMenu) loop(title, leave string, actions []menuAction) (bool, error) {
	options := make([]string, 0, len(actions)+1)
	for _, action := range actions {
		options = append(options, action.label)
	}
	options = append(options, leave)
	for {
		if err := m.ctx.Err(); err != nil {
			return false, err
		}
		choice, err := m.choose(title, options)
		if err != nil {
			return false, err
		}
		if choice == leave {
			return false, nil
		}
		action := actions[slices.Index(options, choice)]
		end, err := action.run()
		if err != nil {
			if m.ctx.Err() != nil {
				return false, m.ctx.Err()
			}
			if errors.Is(err, tui.ErrCanceled) {
				continue
			}
			var interaction interactionError
			if errors.As(err, &interaction) {
				return false, err
			}
			fmt.Fprintf(m.out, "%s失败：%v\n", action.label, err)
			continue
		}
		if end {
			return true, nil
		}
	}
}

func (m *deviceMenu) submenu(title string, actions []menuAction) menuAction {
	return menuAction{title, func() (bool, error) { return m.loop(title, backMenu, actions) }}
}

func (m *deviceMenu) choose(label string, options []string) (string, error) {
	choice, err := m.prompt.Select(label, options)
	if err != nil {
		return "", interactionError{err}
	}
	if !slices.Contains(options, choice) {
		return "", interactionError{errors.New("无效的操作选择")}
	}
	return choice, nil
}

func (m *deviceMenu) input(label, value string) (string, error) {
	value, err := m.prompt.Input(label, value)
	if err != nil {
		return "", interactionError{err}
	}
	return strings.TrimSpace(value), nil
}

func (m *deviceMenu) confirm(message string) (bool, error) {
	choice, err := m.choose(fmt.Sprintf("设备 %s：%s", m.c.Endpoint(), message), []string{cancelAction, confirmAction})
	return choice == confirmAction, err
}

func queryAction[T any](m *deviceMenu, label string, fn func() (T, error)) menuAction {
	return menuAction{label, func() (bool, error) {
		result, err := fn()
		if err != nil {
			return false, err
		}
		return false, m.printResult(label, result)
	}}
}

func (m *deviceMenu) overviewActions() []menuAction {
	return []menuAction{
		queryAction(m, "设备运行信息", func() (*overview.DeviceInfo, error) { return overview.GetDeviceInfo(m.ctx, m.c, m.sid) }),
		queryAction(m, "系统配置概览", func() (overview.Sysinfo, error) { return overview.GetSysinfo(m.ctx, m.c, m.sid) }),
		queryAction(m, "运行时间", func() (float64, error) { return overview.GetUptime(m.ctx, m.c, m.sid) }),
		queryAction(m, "主机名", func() (string, error) { return overview.GetHostName(m.ctx, m.c, m.sid) }),
	}
}

func (m *deviceMenu) statusActions() []menuAction {
	return []menuAction{
		queryAction(m, "当前版本", func() (*system.Version, error) { return system.GetVersion(m.ctx, m.c, m.sid, "") }),
		queryAction(m, "检查新版本（self）", func() (*system.Version, error) { return system.GetVersion(m.ctx, m.c, m.sid, "self") }),
		queryAction(m, "检查新版本（bottom）", func() (*system.Version, error) { return system.GetVersion(m.ctx, m.c, m.sid, "bottom") }),
		queryAction(m, "设备时间", func() (string, error) { return system.AsyncTime(m.ctx, m.c, m.sid) }),
		queryAction(m, "会话超时", func() (int, error) { return system.GetSessionTime(m.ctx, m.c, m.sid) }),
		queryAction(m, "Overlay 剩余空间（设备原始值）", func() (string, error) { return system.GetOverlayFreeSpace(m.ctx, m.c, m.sid) }),
	}
}

func (m *deviceMenu) settingsActions() []menuAction {
	return []menuAction{
		{"设置 Web 语言", func() (bool, error) {
			lang, err := m.input("语言代码（以设备支持为准）", "zh_cn")
			if err != nil {
				return false, err
			}
			if err := system.SetLang(m.ctx, m.c, m.sid, lang); err != nil {
				return false, err
			}
			fmt.Fprintln(m.out, "Web 语言已设置。")
			return false, nil
		}},
		{"设置引导状态", func() (bool, error) {
			choice, err := m.choose("引导状态", []string{"已完成", "未完成"})
			if err != nil {
				return false, err
			}
			steped := "1"
			if choice == "未完成" {
				steped = "0"
			}
			if err := system.SetStep(m.ctx, m.c, m.sid, steped); err != nil {
				return false, err
			}
			fmt.Fprintln(m.out, "引导状态已设置。")
			return false, nil
		}},
	}
}

func (m *deviceMenu) backupActions() []menuAction {
	return []menuAction{
		queryAction(m, "检查设备备份文件", func() (*system.BackupFile, error) { return system.HasBackupFile(m.ctx, m.c, m.sid) }),
		{"恢复设备备份", m.restoreBackup},
		m.confirmed("删除设备备份文件", "将删除设备上的 /overlay/backup-config.tar.gz，是否继续？", false, func() error {
			_, err := system.DelRestoreConfig(m.ctx, m.c, m.sid)
			return err
		}),
	}
}

func (m *deviceMenu) firmwareActions() []menuAction {
	return []menuAction{
		{"提交固件升级", m.upgrade},
		m.confirmed("删除待升级固件", "将删除设备上的 /tmp/firmware.img；此操作不能中止已启动的升级，是否继续？", false, func() error {
			_, err := system.UpgradeCancel(m.ctx, m.c, m.sid)
			return err
		}),
	}
}

func (m *deviceMenu) maintenanceActions() []menuAction {
	return []menuAction{
		m.confirmed("重启设备", "将重启设备并断开当前连接，是否继续？", true, func() error {
			_, err := system.Reboot(m.ctx, m.c, m.sid)
			return err
		}),
		m.confirmed("恢复出厂配置", "将清空设备配置并可能重启，管理地址可能变化，是否继续？", true, func() error {
			_, err := system.RestoreConfiguration(m.ctx, m.c, m.sid)
			return err
		}),
		queryAction(m, "检查升级清理脚本", func() (bool, error) { return system.HasKillRemaining(m.ctx, m.c, m.sid) }),
		{"执行升级清理脚本", m.cleanup},
		{"查看重启命令", func() (bool, error) {
			reason, err := m.input("重启原因（仅查看命令）", "user reboot")
			if err != nil {
				return false, err
			}
			shell, err := system.GetRebootShell(m.ctx, m.c, m.sid, reason)
			if err != nil {
				return false, err
			}
			return false, m.printResult("重启命令（未执行）", shell)
		}},
	}
}

func (m *deviceMenu) confirmed(label, warning string, endSession bool, fn func() error) menuAction {
	return menuAction{label, func() (bool, error) {
		ok, err := m.confirm(warning)
		if err != nil || !ok {
			return false, err
		}
		if err := fn(); err != nil {
			return false, err
		}
		fmt.Fprintln(m.out, "设备已返回请求成功；请按需检查实际执行结果。")
		if endSession {
			fmt.Fprintln(m.out, "当前会话结束，请在设备操作完成后重新登录。")
		}
		return endSession, nil
	}}
}

func (m *deviceMenu) restoreBackup() (bool, error) {
	backup, err := system.HasBackupFile(m.ctx, m.c, m.sid)
	if err != nil {
		return false, err
	}
	if !backup.HasFile {
		return false, errors.New("设备上没有可恢复的备份文件")
	}
	choice, err := m.choose("恢复选项", []string{"默认恢复", "启用 localMac 选项"})
	if err != nil {
		return false, err
	}
	return m.confirmed("恢复设备备份", "将导入设备上的 /overlay/backup-config.tar.gz 并重启，是否继续？", true, func() error {
		_, err := system.RestoreConfig(m.ctx, m.c, m.sid, choice == "启用 localMac 选项")
		return err
	}).run()
}

func (m *deviceMenu) upgrade() (bool, error) {
	fmt.Fprintln(m.out, "提交已在设备上的固件；请先完成固件上传和校验。")
	imagePath, err := m.input("设备上的固件路径", "/tmp/firmware.img")
	if err != nil {
		return false, err
	}
	persist, err := m.choose("升级后配置", []string{"保留配置", "不保留配置"})
	if err != nil {
		return false, err
	}
	scope, err := m.choose("升级范围", []string{"本机", "整网"})
	if err != nil {
		return false, err
	}
	options := system.UpgradeOptions{Path: imagePath, Check: imagePath == "/tmp/firmware.img", IsPersist: persist == "保留配置", IsEstAll: scope == "整网"}
	warning := fmt.Sprintf("将提交升级，固件=%s，范围=%s，%s，是否继续？", imagePath, scope, persist)
	if options.IsEstAll {
		warning = fmt.Sprintf("将提交整网升级；设备整网升级流程决定镜像和配置处理，是否继续？（固件参数=%s）", imagePath)
	}
	return m.confirmed("提交固件升级", warning, true, func() error {
		_, err := system.UpgradeConfirm(m.ctx, m.c, m.sid, options)
		return err
	}).run()
}

func (m *deviceMenu) cleanup() (bool, error) {
	exists, err := system.HasKillRemaining(m.ctx, m.c, m.sid)
	if err != nil {
		return false, err
	}
	if !exists {
		fmt.Fprintln(m.out, "设备没有升级清理脚本。")
		return false, nil
	}
	return m.confirmed("执行升级清理脚本", "将执行设备升级前清理脚本，可能停止服务或断开连接，是否继续？", true, func() error {
		return system.DoKillRemaining(m.ctx, m.c, m.sid)
	}).run()
}

func (m *deviceMenu) printResult(label string, value any) error {
	rows := [][]string{{"项目", "值"}}
	switch v := value.(type) {
	case *overview.DeviceInfo:
		rows = append(rows, []string{"设备时间", v.LocalTime}, []string{"运行时间", fmt.Sprintf("%g 秒", v.Uptime)},
			[]string{"总内存（含固件 Swap 调整）", fmt.Sprintf("%g KiB", v.MemTotal)},
			[]string{"空闲内存", fmt.Sprintf("%g KiB", v.MemFree)},
			[]string{"可用内存（含可回收部分）", fmt.Sprintf("%g KiB", v.MemFreeTotal)},
			[]string{"已用内存（固件计算值）", fmt.Sprintf("%g KiB", v.MemUsed)})
	case overview.Sysinfo:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			text := string(v[key])
			var s string
			if json.Unmarshal(v[key], &s) == nil {
				text = s
			}
			rows = append(rows, []string{terminalText(key), terminalText(text)})
		}
	case *system.BackupFile:
		rows = append(rows, []string{"备份文件存在", yesNo(v.HasFile)}, []string{"设备路径", v.Path})
	case *system.Version:
		fmt.Fprintf(m.out, "当前版本：%s\n", terminalText(v.Current))
		if len(v.New) == 0 {
			return nil
		}
		return m.printJSON("版本检查结果", v.New)
	case string:
		_, err := fmt.Fprintf(m.out, "%s：%s\n", label, terminalText(v))
		return err
	case bool:
		_, err := fmt.Fprintf(m.out, "%s：%s\n", label, yesNo(v))
		return err
	case float64:
		_, err := fmt.Fprintf(m.out, "%s：%g 秒\n", label, v)
		return err
	case int:
		_, err := fmt.Fprintf(m.out, "%s：%d 秒\n", label, v)
		return err
	default:
		return m.printJSON(label, value)
	}
	return tui.WriteTable(m.out, rows)
}

func (m *deviceMenu) printJSON(label string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(m.out, "%s：\n%s\n", label, data)
	return err
}

func yesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}
