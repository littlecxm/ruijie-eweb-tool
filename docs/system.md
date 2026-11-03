# System API — 系统管理

根据固件源码 `usr-lib/lua/luci/modules/system.lua` 整理，覆盖该模块的全部 16 个非局部函数。
源码根目录为 `~/git/ruijie`；本文中的源码路径均相对于该目录。

本文采用统一的 HTTP API 文档结构。设备实际采用 RPC over HTTP：所有操作共用
`POST /cgi-bin/luci/api/system`，通过请求体的 `method` 选择操作。下文保留真实协议，
没有额外定义设备未实现的 REST 资源路径或 GET、PUT、DELETE 接口。

## 通用约定

### 请求地址与鉴权

```http
POST /cgi-bin/luci/api/system?auth=<sid> HTTP/1.1
Host: <device-host>
Content-Type: application/json
Accept: application/json
```

| 项目 | 说明 |
| --- | --- |
| Base URL | 设备的 HTTP(S) 地址，例如 `https://192.168.110.1` |
| API 路径 | `/cgi-bin/luci/api/system` |
| HTTP 方法 | `POST`，包括查询、修改和删除操作 |
| 鉴权 | 管理员会话；查询参数 `auth` 传入登录获得的 `sid` |
| 请求格式 | `application/json` |
| 响应格式 | `application/json` |
| 前端页面 | 系统管理 `stepsystool`；升级页 `/admin/alone/systool/systool_upgrade`，不是 API 地址 |

查询参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `auth` | string | 是 | 有效的登录会话 SID；由公共鉴权逻辑校验 |

请求体：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `method` | string | 是 | 本文列出的函数名，大小写敏感 |
| `params` | object / string / null | 按接口 | 通常为对象；无参数时传 `null` 或 `{}`，也可省略；`getRebootShell` 接收字符串 |
| `id` | string / number | 否 | 请求标识，响应原样返回；省略时响应为 `null` |

`params` 作为一个参数整体传给 Lua 函数，不按数组展开；不需要额外传入 `module` 或
`device`。省略 `params` 或传 `null` 时，分发器使用空 Lua table。
本文示例省略 `jsonrpc` 字段，与仓库现有客户端的调用格式一致。

文档导航和交互菜单总览见 [文档索引](README.md)。

只读调用示例：

```sh
curl --request POST \
  --url 'https://<device-host>/cgi-bin/luci/api/system?auth=<sid>' \
  --header 'Content-Type: application/json' \
  --header 'Accept: application/json' \
  --data '{"method":"getVersion","params":{"deviceType":"self"}}'
```

### 响应结构

```json
{
  "code": 0,
  "id": null,
  "data": {
    "result": true
  },
  "error": null
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | integer | RPC 包装层固定为 `0`，不代表业务操作成功 |
| `id` | string / number / null | 对应请求的 `id` |
| `data` | 任意 JSON 值 | 模块函数返回值，可能是对象、字符串、数字、布尔值或 `null` |
| `error` | object / null | RPC 错误；正常调用为 `null` |

**下文的“返回数据”均指完整响应中的 `data`。** 调用方需要依次判断 HTTP 状态、
`error` 以及接口自身的 `data.code` 或 `data.result`。
异步操作返回成功只表示代码已走到任务下发阶段，不表示重启、升级或恢复已经完成。

当前 `luci.json` 和 RPC 包装层还有以下序列化行为：

- 空 Lua table 编码为 `[]`，例如不检查新版本时的 `data.new`。
- 函数直接返回 `false` 或没有返回值时，RPC 将 `data` 编码为 `null`；
  对象内的布尔字段不受影响，例如 `{"result":false}` 和 `{"hasfile":false}`。
- Lua table 中值为 `nil` 的字段不出现在 JSON 对象中。

### 公共错误

| HTTP 状态 | 错误位置 | 值 | 含义 |
| --- | --- | --- | --- |
| `403` | `code` | `2` | 鉴权失败；返回 `msg: "403 Forbidden, auth is not passed"` |
| `403` | `code` | `2` | 请求检查失败；返回 `msg: "403 Forbidden, INVALID DATA"` |
| 通常 `200` | `error.code` | `-32700` | JSON 解析失败，`message: "Parse error."` |
| 通常 `200` | `error.code` | `-32600` | `method` 不是字符串，`message: "Invalid request."` |
| 通常 `200` | `error.code` | `-32601` | 找不到方法，`message: "Method not found."` |
| 通常 `200` | `error.code` | `-32602` | Lua 函数执行异常，例如参数类型错误；可能包含 `data`、`res` 诊断字段 |

鉴权失败的响应不使用 RPC 包装结构：

```json
{
  "code": 2,
  "msg": "403 Forbidden, auth is not passed"
}
```

RPC 方法不存在的响应示例：

```json
{
  "code": 0,
  "id": null,
  "data": null,
  "error": {
    "code": -32601,
    "message": "Method not found."
  }
}
```

## 接口索引

下列接口的 HTTP 方法均为 `POST`，路径均为 `/cgi-bin/luci/api/system`。

| 功能 | `method` | `params` | 行为 |
| --- | --- | --- | --- |
| 配置语言 | [`setLang`](#setlang) | `{ "lang": "zh_cn" }` | 写入并提交配置 |
| 查询设备版本 | [`getVersion`](#getversion) | `{ "deviceType": "self" }` | 读取当前版本，可检查新版本 |
| 读取设备时间 | [`asyncTime`](#asynctime) | `null` | 只读，当前实现不设置时间 |
| 查询会话超时 | [`getSessiontime`](#getsessiontime) | `null` | 只读 |
| 配置引导标识 | [`setStep`](#setstep) | `{ "steped": "1" }` | 写入并提交配置 |
| 查询剩余空间 | [`getOverlayFreeSpace`](#getoverlayfreespace) | `null` | 只读 |
| 重启设备 | [`reboot`](#reboot) | `null` | 异步重启 |
| 恢复出厂配置 | [`restoreConfiguration`](#restoreconfiguration) | `null` | 异步恢复，可能重启 |
| 提交固件升级 | [`upgradeConfirm`](#upgradeconfirm) | `{ "path": "/tmp/firmware.img" }` | 异步升级 |
| 取消固件升级 | [`upgradeCancel`](#upgradecancel) | `null` | 删除固定路径的升级文件 |
| 查询备份文件 | [`hasBackupFile`](#hasbackupfile) | `null` | 只读 |
| 恢复备份配置 | [`restoreConfig`](#restoreconfig) | `null` | 导入已有备份并异步重启 |
| 删除备份文件 | [`delRestoreConfig`](#delrestoreconfig) | `null` | 删除固定路径的备份文件 |
| 查询清理脚本 | [`hasKillRemaining`](#haskillremaining) | `null` | 只读 |
| 执行清理脚本 | [`doKillRemaining`](#dokillremaining) | `null` | 内部辅助函数，同步执行脚本 |
| 生成重启命令 | [`getRebootShell`](#getrebootshell) | `"user reboot"` | 内部辅助函数，仅返回命令文本 |

## eweb-tool 实现

Go 包为 `pkg/eweb-module/system`。以下函数均接收 `ctx`、`c *client.Client`、
`sid`；表中的参数为这三个公共参数之后的部分。全部 16 个公开方法均有客户端实现和菜单入口。

| RPC 方法 | Go 函数及额外参数 | 二级菜单位置 |
| --- | --- | --- |
| `setLang` | `SetLang(..., lang string) error` | 系统设置 → 设置 Web 语言 |
| `getVersion` | `GetVersion(..., deviceType string)` | 系统状态 → 当前版本 / 检查新版本 |
| `asyncTime` | `AsyncTime(...)` | 系统状态 → 设备时间 |
| `getSessiontime` | `GetSessionTime(...)` | 系统状态 → 会话超时 |
| `setStep` | `SetStep(..., steped string) error` | 系统设置 → 设置引导状态 |
| `getOverlayFreeSpace` | `GetOverlayFreeSpace(...)` | 系统状态 → Overlay 剩余空间 |
| `reboot` | `Reboot(...)` | 设备维护 → 重启设备 |
| `restoreConfiguration` | `RestoreConfiguration(...)` | 设备维护 → 恢复出厂配置 |
| `upgradeConfirm` | `UpgradeConfirm(..., options UpgradeOptions)` | 固件升级 → 提交固件升级 |
| `upgradeCancel` | `UpgradeCancel(...)` | 固件升级 → 删除待升级固件 |
| `hasBackupFile` | `HasBackupFile(...)` | 备份与恢复 → 检查设备备份文件 |
| `restoreConfig` | `RestoreConfig(..., localMAC bool)` | 备份与恢复 → 恢复设备备份 |
| `delRestoreConfig` | `DelRestoreConfig(...)` | 备份与恢复 → 删除设备备份文件 |
| `hasKillRemaining` | `HasKillRemaining(...)` | 设备维护 → 检查升级清理脚本 |
| `doKillRemaining` | `DoKillRemaining(...) error` | 设备维护 → 执行升级清理脚本 |
| `getRebootShell` | `GetRebootShell(..., reason string)` | 设备维护 → 查看重启命令 |

除表中标明仅返回 `error` 的函数外，其余返回相应数据及 `error`：版本为 `*Version`，
异步操作为 `*Operation`，备份状态为 `*BackupFile`，脚本存在性为 `bool`，
会话超时为 `int` 秒，其他文本接口为 `string`。
`Version.New` 保留为 `json.RawMessage`，兼容对象、数组、字符串等脚本输出。

客户端检查所有操作的 `data.code` 和设置的 `data.result`。缺少这些业务状态字段也返回错误，
不会仅凭外层 `code=0` 报告成功。`hasKillRemaining` 的 `null` 映射为 `false`，
`doKillRemaining` 接受合法的 `null` 返回值；缺少 `data` 仍视为协议错误。

`GetVersion` 使用空字符串表示仅查询当前版本，以 `self` / `bottom` 触发相应检查；
`SetStep` 只接受字符串 `"0"` / `"1"`。
`UpgradeOptions` 的 `Path` 必须是设备上的绝对路径，三个开关均为 Go `bool` 并编码为
JSON boolean；客户端会拦截空路径及固件过滤的命令控制字符。
`RestoreConfig` 的 `localMAC=true` 发送 URL 查询参数 `localMac=1`，不混入 JSON 请求体。

菜单中的升级提交只操作已经在设备上的固件，不负责上传或验证镜像。
重启、恢复出厂、恢复备份、升级和升级清理请求成功后结束当前会话；
删除、重启、恢复、升级及清理操作默认取消，确认后才发送请求，不自动重试。
这些交互确认属于命令行向导；Go API 供自动化直接调用，由调用方决定操作时机。

## 基础设置与状态

### setLang

配置 Web 语言，写入 UCI 的 `luci.main.lang` 并提交 `luci` 配置。

**请求参数**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `params.lang` | string | 否 | `"en"` | 必须存在于运行时的 `luci.modules.env.langMap` |

```json
{"method":"setLang","params":{"lang":"zh_cn"}}
```

**返回数据**

```json
{"result":true}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `result` | boolean | 语言存在时返回 `true`；不支持的语言返回 `false` |

语言表优先读取 UCI 的 `luci.languages`。缺少有效语言表时，EAP 的回退列表为
`en`、`zh_cn`；其他设备的回退列表为 `en`、`zh_cn`、`zh_ft`、`th`、`vi`、`es`、
`ru`、`tr`、`id`、`ar`、`de`、`fr`、`jp`、`es_MX`、`pt_BR`、`pl`、`it`、`yk`。
这些是源码回退值，设备实际可用语言以运行时配置为准。

### getVersion

读取 `/etc/openwrt_version`，按需调用云升级脚本检查新版本。

**请求参数**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `params.deviceType` | string | 否 | 无 | `"self"` 或 `"bottom"` 会执行版本检查；省略或其他值仅返回当前版本 |

```json
{"method":"getVersion","params":{"deviceType":"self"}}
```

**返回数据**（以下为省略 `deviceType`、不检查新版本的示例，版本号仅作示意）

```json
{"cur":"ReyeeOS 2.430.0.1924\n","new":[]}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `cur` | string，可缺省 | 版本文件的原始内容，可能带换行；读取失败时字段省略 |
| `new` | 任意 JSON 值 | 升级脚本输出经 `tool.decode()` 处理的结果；不检查时为空 table，在当前编码器下为 `[]` |

版本检查分别执行 `cloud_upgrade.sh --version-check self` 或
`cloud_upgrade.sh --version-check bottom`。`system.lua` 未定义二者的设备范围差异，
也未约定新版本字段结构。脚本输出不是可解析 JSON 时保留原始字符串，不能假定 `new`
始终为对象；此接口不会启动固件升级。

### asyncTime

读取设备本地时间。源码中设置时间的代码已经注释，传入 `time` 不会修改系统时间。

**请求参数：** 无，`params` 不参与当前实现。

```json
{"method":"asyncTime","params":null}
```

**返回数据**

```json
"2026/11/03 12:34:56"
```

返回类型为 string，格式为 `YYYY/MM/DD HH:mm:ss`，取自设备的本地时区，不包含时区标识。
结果经过首尾空白清理，不是 Unix 时间戳。

### getSessiontime

读取 Web 登录超时时间。

**请求参数：** 无。

```json
{"method":"getSessiontime","params":null}
```

**返回数据**

```json
"3600"
```

返回类型为 string / number，单位为秒。存在 UCI 配置 `luci.sauth.sessiontime` 时原样
返回，通常是字符串；未配置时返回数字 `3600`。此方法不会续期或修改会话。

### setStep

配置新用户引导标识，写入 UCI 的 `luci.main.steped` 并提交 `luci` 配置。

**请求参数**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `params.steped` | string | 是 | 无 | 仅接受字符串 `"0"`、`"1"`，分别表示未完成、已完成引导；不接受数字或布尔值 |

```json
{"method":"setStep","params":{"steped":"1"}}
```

**返回数据**

```json
{"result":true}
```

`result` 为 boolean。非法枚举值返回 `{"result":false}`；缺少 `steped` 或值为
`false` 时还会回显 `params`。例如 `params: null` 被分发器转换为空 table 后返回：

```json
{"result":false,"params":[]}
```

### getOverlayFreeSpace

查询设备 overlay 的剩余空间。

**请求参数：** 无。

```json
{"method":"getOverlayFreeSpace","params":null}
```

**返回数据**（数值仅作示意）

```json
"123456\n"
```

返回类型为 string，是 `df | grep overlayfs | awk -F' ' '{ print $4 }'` 的原始标准输出，
不进行数字转换或去除换行；没有匹配项时可能为空字符串。单位取决于设备 `df` 的输出
约定，源码未指定单位，不能直接按字节解释。

## 重启与恢复出厂

### reboot

异步下发设备重启命令。

**请求参数：** 无；重启原因固定为 `"user reboot"`，不能通过 `params` 自定义。

```json
{"method":"reboot","params":null}
```

**返回数据**（未启用重启原因脚本时的示例）

```json
{"code":0,"shell":"sleep 1; sync; reboot >/dev/null 2>&1;"}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | integer | 固定为 `0`，表示函数已完成任务下发流程 |
| `shell` | string | 实际下发的命令，随设备是否支持重启原因记录而变化 |

常规命令先等待 1 秒再同步并重启。操作可能中断当前连接，需要在设备启动后重新确认状态。

### restoreConfiguration

异步恢复出厂配置。

**请求参数：** 无。

```json
{"method":"restoreConfiguration","params":null}
```

**返回数据**（常规恢复分支，未启用重启原因脚本）

```json
{"code":0,"shell":"sleep 1; firstboot; sleep 2; sync; reboot"}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | integer | 固定为 `0`，不代表恢复已经完成 |
| `shell` | string | 常规恢复命令或设备统一框架的恢复命令 |

常规分支执行 `firstboot` 后重启，重启原因固定为 `"restore"`。当设备类型为 `msw`
（该方法默认值），且 `cmd.devSta.get({device="pc", module="devResetLocal"})` 返回
`exist == "1"` 时，改为下发 `dev_sta set -m devResetLocal '{}'`。
恢复出厂会清除配置，后续管理地址和会话可能变化。

## 固件升级

### upgradeConfirm

提交已经准备好的固件文件。上传与镜像校验由 `file` 模块的 `upgradeLocal` 完成，
不属于本接口；此接口不会上传文件。

**请求参数**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `params.path` | string | 是 | 无 | 设备上的固件路径，通常使用上传结果中的 `/tmp/firmware.img`；整网升级分支同样会校验此参数 |
| `params.check` | boolean | 否 | `false` | 为 `true` 时，先检查固定文件 `/tmp/firmware.img` 是否存在，不检查 `path` 指向的其他文件 |
| `params.isPersist` | boolean | 否 | `false` | 常规升级是否保留配置；`false` 会给 `sysupgrade` 添加 `-n` |
| `params.isEstAll` | boolean | 否 | `false` | 为 `true` 时使用整网升级命令 `sd_tipc_client_upgrade upgrade_req` |

```json
{
  "method": "upgradeConfirm",
  "params": {
    "path": "/tmp/firmware.img",
    "check": true,
    "isPersist": true,
    "isEstAll": false
  }
}
```

**返回数据**

```json
{"code":0}
```

| `data.code` | `data.msg` | 含义 |
| --- | --- | --- |
| `0` | 无 | 已进入异步升级任务下发流程 |
| `1` | `firmware not found, please try again.` | `check` 为真且 `/tmp/firmware.img` 不存在 |
| `-1` | `params illegal` | `path` 包含命令过滤字符 |

实现细节与调用约束：

- 入口首先停止固件自动删除任务，即使后续参数或文件检查失败，也已执行此步骤。
- `path` 不允许换行、回车、反引号、`&`、`$`、`;`、`|`。没有默认路径；缺失或类型
  不正确时可能产生 RPC 执行异常，而不是上表中的业务错误。
- 布尔参数按 Lua 真值处理：只有 `false`、`null` 或缺省为假，字符串 `"false"`、
  `"0"` 和数字 `0` 都为真。应发送 JSON boolean。
- 常规升级异步执行 `killall dropbear; sleep 2; /sbin/sysupgrade <选项> <固件路径>`；
  保留配置时不加 `-n`，不保留配置时添加 `-n`。路径通过 Lua `%q` 引用。
- 整网升级分支不在命令中使用 `path` 或 `isPersist`，但仍执行前置检查及 `path` 过滤。
  固件合法性、版本兼容性和空间校验结果应在上传阶段处理。

### upgradeCancel

停止固件自动删除任务，并删除 `/tmp/firmware.img`。

**请求参数：** 无；不支持指定其他文件路径。

```json
{"method":"upgradeCancel","params":null}
```

**返回数据**

```json
{"code":0,"msg":"delete fireware.img ok"}
```

`code` 为 integer，`msg` 为 string；消息保留源码中的 `fireware` 拼写。
函数不检查删除结果，也不终止已经启动的升级进程，因此不能把此方法视为正在进行的升级的回滚。

## 配置备份与恢复

本模块只查询、使用或删除固定路径 `/overlay/backup-config.tar.gz`，不负责创建或上传备份。

### hasBackupFile

检查备份配置文件是否存在。

**请求参数：** 无。

```json
{"method":"hasBackupFile","params":null}
```

**返回数据**

```json
{"hasfile":false,"path":"/overlay/backup-config.tar.gz"}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `hasfile` | boolean | 文件存在时为 `true`，否则为 `false` |
| `path` | string | 固定为 `/overlay/backup-config.tar.gz`，文件不存在时仍返回 |

### restoreConfig

调用 `file.restoreConfig(true)`，从 `/overlay/backup-config.tar.gz` 导入配置，随后异步重启。

**请求参数：** `params` 无有效字段；文件路径固定。

此方法还通过被调用的 `file.restoreConfig` 读取一个可选的 **URL 查询参数**：

| 参数 | 位置 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `localMac` | query | string | 否 | 无 | 仅 `"1"` 时给 `sysupgrade` 添加 `--local-mac`；不是 `params.localMac` |

```http
POST /cgi-bin/luci/api/system?auth=<sid>&localMac=1 HTTP/1.1
Content-Type: application/json
```

```json
{"method":"restoreConfig","params":null}
```

**返回数据**（成功分支示例，地址仅作示意）

```json
{
  "code": 0,
  "addr": "192.168.110.1",
  "_shell": "sync; sleep 1; sync; reboot >/dev/null 2>&1;"
}
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | integer | `0`：已下发重启；`1`：导入命令输出包含 `not supports` |
| `addr` | string | 成功分支返回 HTTP 环境中的 `SERVER_NAME`，缺失时为空字符串；不是解析出的恢复后地址 |
| `_shell` | string | 成功分支实际下发的重启命令，随设备环境变化 |
| `msg` | string | 失败分支返回导入命令的原始输出 |

失败分支返回形如 `{"code":1,"msg":"<包含 not supports 的命令输出>"}`。
导入命令为 `sysupgrade -r /overlay/backup-config.tar.gz`，带 `localMac=1` 时为
`sysupgrade --local-mac -r /overlay/backup-config.tar.gz`。源码未解释 `--local-mac`
的进一步语义。

函数不预先验证备份是否存在，也不检查导入命令退出码，仅根据输出是否包含
`not supports` 分支处理。成功分支还可能删除 `/backup/none_config_flag`，然后以
`"Import configuration"` 为原因重启。应先查询备份存在性，恢复后重新确认设备地址与配置。

### delRestoreConfig

删除 `/overlay/backup-config.tar.gz`。

**请求参数：** 无；不支持自定义路径。

```json
{"method":"delRestoreConfig","params":null}
```

**返回数据**

```json
"ok"
```

返回类型为 string。执行 `rm /overlay/backup-config.tar.gz` 后固定返回 `"ok"`，
不检查退出码；文件不存在或删除失败也没有单独的业务错误。

## 清理脚本与内部辅助函数

这些函数同样定义在 `system` 模块中。RPC 分发器没有方法白名单，能够解析模块内的
非局部函数；其中 `doKillRemaining`、`getRebootShell` 主要供其他 Lua 模块复用，
这里记录它们的真实参数与返回行为。

### hasKillRemaining

检查 `/etc/sysupgrade.d/kill_remaining.sh` 是否存在。

**请求参数：** 无。

```json
{"method":"hasKillRemaining","params":null}
```

**返回数据**（脚本存在）

```json
true
```

脚本存在时 `data` 为 `true`；不存在时 Lua 函数返回 `false`，经 RPC 包装后 `data`
为 `null`。因此 HTTP 调用端的实际返回类型为 boolean / null。

### doKillRemaining

存在清理脚本时，同步执行 `/etc/sysupgrade.d/kill_remaining.sh >/dev/null 2>&1`。

**请求参数：** 无。

```json
{"method":"doKillRemaining","params":null}
```

**返回数据**

```json
null
```

函数没有返回值。脚本不存在时跳过执行；脚本存在时丢弃输出，不返回退出码。
仅凭响应无法判断是否执行过脚本或脚本是否成功。

### getRebootShell

生成供其他操作拼接的重启命令文本，本函数不执行命令。

**请求参数**

| 参数 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `params` | string | 是 | 无 | 直接传入重启原因字符串；不是 `{ "reason": "..." }` 对象 |

```json
{"method":"getRebootShell","params":"user reboot"}
```

**返回数据**（设备不存在 `/etc/init.d/parse_reboot_reason`）

```json
"; sync; reboot"
```

返回类型为 string。当 `/etc/init.d/parse_reboot_reason` 存在且原因通过
`tool.includeXxs()` 检查时，会附加来源 IP 和原因，形如：

```json
"; sync; reboot -m 'web(192.168.110.2)' -r 'user reboot'"
```

来源 IP 由 `tool.getSafeRemoteIp()` 获取。原因包含换行、回车、反引号、`&`、`$`、`;`、
`|` 时不附加原因参数，仍返回基础命令；该检查不构成通用的 shell 字符串转义。
脚本存在时，缺失或非字符串的原因可能产生 RPC 执行异常。

## 源码依据与边界

| 源码文件 | 核对内容 |
| --- | --- |
| `usr-lib/lua/luci/modules/system.lua` | 本文全部接口、参数、默认值、返回值与操作分支 |
| `usr-lib/lua/luci/controller/web/api.lua` | `api/system` 路由、会话鉴权、模块绑定 |
| `usr-lib/lua/luci/utils/jsonrpc.lua` | 方法解析、参数整体传递、响应包装、RPC 错误 |
| `usr-lib/lua/luci/json.lua` | 空 table、`null` 与 JSON 编解码行为 |
| `usr-lib/lua/luci/modules/env.lua` | `setLang` 的语言表与设备回退列表 |
| `usr-lib/lua/luci/modules/file.lua` | 固件删除任务、上传前置流程、`restoreConfig(true)` 的真实行为 |
| `usr-lib/lua/luci/utils/tool.lua` | 命令过滤、JSON 输出解码、异步执行及请求检查 |

`getCurVersion`、`getNewVersion` 是局部函数，不能作为该模块的 RPC 方法直接调用。
本文依据本地源码整理，响应示例用于说明结构，未在真实设备上执行升级、重启、恢复或删除操作。
