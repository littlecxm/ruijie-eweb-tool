# eWeb API 与命令向导

本文档按设备真实的 LuCI RPC 模块组织。源码依据为 `~/git/ruijie` 中的固件文件，
示例数据用于说明协议；新增接口已通过模拟 HTTP 设备验证，尚未做真机验证。

## 模块索引

| 模块 | 文档 | Go 包 | 覆盖范围 |
| --- | --- | --- | --- |
| `overview` | [设备概览](overview.md) | `pkg/eweb-module/overview` | 全部 4 个公开方法 |
| `system` | [系统管理](system.md) | `pkg/eweb-module/system` | 全部 16 个公开方法 |
| `cmd / devSta` | [在线终端](devSta/user-list.md) | `pkg/eweb-module/devSta` | `devSta.get / user_list` |

[模块与页面清单](LIST.md) 同时包含已实现和待实现的接口，不能仅凭列入清单判断实现状态。

## 通用请求

```http
POST /cgi-bin/luci/api/<module>?auth=<sid>
Content-Type: application/json
```

```json
{"method":"getUptime","params":null}
```

`sid` 来自登录会话；`params` 作为一个整体传入 Lua 方法。`overview` 和 `system`
不需要 `module`、`device`、`noParse` 等 `cmd` 模块参数。

```json
{"code":0,"id":null,"data":123,"error":null}
```

先检查 HTTP 状态、外层 `code` / `error`，再检查业务返回值。`system` 的操作结果
需要检查 `data.code`，设置结果需要检查 `data.result`。LuCI 会将直接返回的 `false`
以及无返回值转换为 `data:null`；对象内的 `false` 保持原值，空 Lua table 编码为 `[]`。

`client.Call` 解析普通结果并拒绝缺失或 `null` 数据；`client.CallRaw` 保留合法的 `null`，
供 `hasKillRemaining` / `doKillRemaining` 使用。`client.CallWithQuery` 支持恢复配置的
`localMac` 查询参数，`auth` 始终由 SID 参数管理。

## 登录后的菜单

```text
列出在线终端
设备概览 → 设备运行信息 / 系统配置概览 / 运行时间 / 主机名
系统状态 → 当前版本 / 检查新版本 self、bottom / 设备时间 / 会话超时 / Overlay 剩余空间
系统设置 → 设置 Web 语言 / 设置引导状态
备份与恢复 → 检查设备备份文件 / 恢复设备备份 / 删除设备备份文件
固件升级 → 提交固件升级 / 删除待升级固件
设备维护 → 重启设备 / 恢复出厂配置 / 检查或执行升级清理脚本 / 查看重启命令
退出
```

各二级菜单可返回上级；普通查询和设置执行后留在当前菜单，API 错误允许重新选择操作。
重启、恢复出厂、恢复备份、升级提交、执行升级清理脚本均在设备确认请求成功后结束会话。
异步提交成功不表示设备操作已经完成。变更请求不会自动重试。

会清除文件、重启设备或影响服务的操作先展示目标设备及影响，默认选择“取消”。
恢复备份和执行升级清理脚本会先检查对应文件是否存在。

固件菜单只提交**已经上传并完成校验的设备端镜像**，不提供上传接口。
默认保留配置、仅升级本机；自定义镜像路径时，不使用仅针对 `/tmp/firmware.img` 的存在检查。
“删除待升级固件”删除固定路径镜像，不能停止已经开始的升级。
