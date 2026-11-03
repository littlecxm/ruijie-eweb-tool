# Overview API — 设备概览

根据 `usr-lib/lua/luci/modules/overview.lua` 整理，覆盖全部 4 个公开方法。
通用协议、鉴权、空值语义和菜单结构见 [文档索引](README.md)。

## 接口索引

所有方法使用 `POST /cgi-bin/luci/api/overview?auth=<sid>`，没有有效请求参数，
发送 `params:null`。以下“返回数据”均指 RPC 响应中的 `data`。

| 功能 | RPC 方法 | Go 函数 | 二级菜单项 |
| --- | --- | --- | --- |
| 系统配置概览 | `getSysinfo` | `overview.GetSysinfo` | 设备概览 → 系统配置概览 |
| 运行与内存信息 | `getDeviceInfo` | `overview.GetDeviceInfo` | 设备概览 → 设备运行信息 |
| 开机后运行时长 | `getUptime` | `overview.GetUptime` | 设备概览 → 运行时间 |
| 主机名 | `getHostName` | `overview.GetHostName` | 设备概览 → 主机名 |

Go 函数均接收 `ctx context.Context`、`c *client.Client`、`sid string`。
这 4 个接口只读取设备状态，不修改配置。

## getSysinfo

读取 UCI `sysinfo` 配置中第一个类型为 `sysinfo` 的 section，并返回 `get_all` 结果。

```json
{"method":"getSysinfo","params":null}
```

返回数据示例：

```json
{
  ".name": "cfg001",
  ".type": "sysinfo",
  "serial_num": "SN001",
  "dev_type": "egw"
}
```

字段取决于设备实际 UCI 配置，示例不表示固定 schema，也不保证包含登录接口
`getAuthority.sysinfo` 中的所有字段。UCI 元数据可能包含 `.name`、`.type`、`.index`、
`.anonymous`，option 也可能是列表；Go 使用 `map[string]json.RawMessage` 保留原始类型，
菜单按键名排序显示。

找不到 section 或读取失败时可能没有结果，经 RPC 包装为 `null`；客户端将其报告为
数据缺失错误，不伪造空的设备信息。

## getDeviceInfo

调用 `luci.sys.sysinfo()`、`luci.sys.uptime()` 和 `system.asyncTime()` 组装运行信息。

```json
{"method":"getDeviceInfo","params":null}
```

返回数据示例：

```json
{
  "local_time": "2026/11/03 12:34:56",
  "uptime": 12345,
  "mem_total": 65536,
  "mem_free": 2048,
  "mem_free_total": 16384,
  "mem_used": 49152
}
```

| 字段 | 类型 / 单位 | 固件含义 |
| --- | --- | --- |
| `local_time` | string | 设备本地时间，格式 `YYYY/MM/DD HH:mm:ss`，无时区标识 |
| `uptime` | number / 秒 | 开机后的运行时长 |
| `mem_total` | number / KiB | `MemTotal + SwapTotal / 2` |
| `mem_free` | number / KiB | `MemFree` |
| `mem_free_total` | number / KiB | `MemFree + SwapFree / 2 + Active(file) + Inactive(file) + SReclaimable` |
| `mem_used` | number / KiB | `MemTotal - mem_free_total`，这里的 `MemTotal` 是物理内存原始值 |

内存值来自 `/proc/meminfo`；存在除以 2 的计算，因此 Go 使用 `float64` 接收。
源码注释中的“可用存储”实际对应内存，不能解释为磁盘空间。
启用 Swap 时，`mem_used + mem_free_total` 不一定等于 API 的 `mem_total`，
界面保留固件的计算值，不据此推导使用率。

## getUptime

```json
{"method":"getUptime","params":null}
```

返回数据为数值，例如 `12345`，单位为秒。直接返回 `nixio.sysinfo().uptime`。
Go 返回 `float64`；`0` 是合法数值，不视为无数据。

## getHostName

```json
{"method":"getHostName","params":null}
```

返回数据：

```json
{"hostname":"Ruijie"}
```

读取 UCI `system` 配置中第一个 `system` section 的 `hostname`。
未配置时返回 `{"hostname":""}`，不会因此省略字段。Go 函数直接返回主机名字符串。

## 请求示例

```sh
curl --request POST \
  --url 'https://<device-host>/cgi-bin/luci/api/overview?auth=<sid>' \
  --header 'Content-Type: application/json' \
  --data '{"method":"getDeviceInfo","params":null}'
```

## 源码依据

| 固件文件 | 核对内容 |
| --- | --- |
| `usr-lib/lua/luci/modules/overview.lua` | 全部 4 个公开方法及字段计算 |
| `usr-lib/lua/luci/sys.lua` | `/proc/meminfo` 字段来源及 uptime |
| `usr-lib/lua/luci/modules/system.lua` | `asyncTime` 的本地时间格式 |
| `usr-lib/lua/luci/utils/jsonrpc.lua` | RPC 响应包装与顶层空值 |

上述路径均相对于 `~/git/ruijie`。
