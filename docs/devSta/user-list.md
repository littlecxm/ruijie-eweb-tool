# 在线终端列表

使用登录获得的 SID 请求：

```http
POST /cgi-bin/luci/api/cmd?auth=<sid>
Content-Type: application/json
```

```json
{"method":"devSta.get","params":{"module":"user_list","noParse":true,"async":null,"remoteIp":false,"data":{"devType":"all","dataType":"timely"},"device":"pc"}}
```

`devType=all` 查询有线和无线终端；`dataType=timely` 请求及时数据。
`remoteIp=false` 使用当前设备，`async=null` 保留固件默认行为。

## 返回与解析

LuCI RPC 外层包含 `code`、`data`、`error`。`noParse=true` 时，成功的模块返回值
保持 JSON 文本形式，所以 `data` 通常是一个需要再次解码的字符串：

```json
{"code":0,"id":null,"data":"{\"code\":\"0\",\"total\":\"0\",\"list\":[]}","error":null}
```

上例仅说明空列表的编码结构。客户端也兼容 `data` 已解码为对象的响应。
外层成功后仍需检查模块自身的 `code`（存在时）；支持字符串或整数错误码。
`total` 支持字符串或整数，缺省时使用 `list` 的记录数。缺少列表、无效 JSON、
无效数量和非零模块错误码均返回错误，不作为“暂无在线终端”处理。

终端常用字段：

| 字段 | 含义 |
| --- | --- |
| `mac`、`userIp` | 终端 MAC、IP |
| `name`、`hostName` | 备注名称、主机名；显示时优先使用备注 |
| `connectType` | `wire`：有线，`wireless`：无线 |
| `sn`、`deviceAliasName` | 接入设备 SN、别名；显示时优先使用别名 |
| `ssid`、`band` | 无线 SSID、频段 |
| `port` | 有线端口，兼容字符串或数字 |
| `onlinetime`、`rssi` | 固件返回的在线时长、信号值，保留原始文本或数字，不换算单位 |
| `hardwareType`、`osType`、`manufacture` | 终端类型、系统、厂商 |

不同固件和接入方式可能省略部分字段，界面以 `-` 显示空值。

## 固件调用链

依据本地 `~/git/ruijie` 中的固件文件：

- `usr-lib/lua/luci/modules/cmd.lua`：`devSta.get` 调用 `dev_sta.fetch`。
  模块调用成功后，`noParse=true` 直接返回底层 `data`，否则由 LuCI 解码 JSON。
- `usr-lib/lua/dev_sta.lua`：通过 `libuflua.client_call` 调用内部模块，调用类型为 `2`。
- `usr-sbin/user_list_rlog.sh`：执行 `ubus call user_list get`，支持同样的
  `devType` / `dataType` 参数，并将 `/tmp/user_list/user_list_tmp.json` 复制为 `wiresta.json`。
- `usr-sbin/user_list.lua`：查询 `ubus user_list`；输出不是有效 JSON 时，
  回退读取 `/tmp/user_list/user_list_tmp.json`，最后提取 `total`。
- `usr-sbin/user_list.elf` 的可见字符串包含 `user_list_get_data_callback`、
  `dev_sta get -m sta_list`、`sta_list_tmp.json`、`user_list_tmp.json` 及上述终端字段，
  支持由内部 ELF 进程采集、汇总并维护文件的判断。

`sta_list_tmp.json` 是采集过程中使用的数据之一；查询全部终端应使用 `user_list` 接口。
`usr-local/lua/dev_config/user_list_note.lua` 管理终端备注，不负责返回全部在线终端。
仅凭这些 Lua 源码和 ELF 字符串，不能断定最终结果文件具体由哪一层读取；
可以确定 LuCI 通过内部模块取得数据再封装为 RPC 响应。客户端直接使用 HTTP API。

此功能使用模拟 HTTP 设备验证，尚未在真实设备上验证。
