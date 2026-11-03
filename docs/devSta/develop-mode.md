# 开发者模式

使用登录获得的 SID，请求 `POST /cgi-bin/luci/api/cmd?auth=<sid>`。
所有请求均设置 `Content-Type: application/json`。

## 查询

```json
{"method":"devSta.get","params":{"module":"develop_mode","noParse":false,"async":null,"remoteIp":false,"device":"pc"}}
```

```json
{"code":0,"id":null,"error":null,"data":{"developMode":"0"}}
```

`GetDevelopMode(ctx, c, sid) (bool, error)` 将字符串 `"0"` / `"1"` 分别映射为关闭 / 开启。
查询不发送 `data`。缺少状态、未知状态或字段类型错误均返回错误。

## 开启与关闭

```json
{"method":"devSta.set","params":{"module":"develop_mode","noParse":false,"async":null,"remoteIp":false,"data":{"developMode":"1"},"device":"pc"}}
```

关闭时仅将 `developMode` 改为字符串 `"0"`，两种设置均返回：

```json
{"code":0,"id":null,"error":null,"data":{"result":"success"}}
```

`ToggleDevelopMode(ctx, c, sid, enabled bool) error` 根据参数明确设置状态：`true` 开启，
`false` 关闭，不会自动反转当前状态。只有 `data.result` 为字符串 `"success"` 时设置成功；
布尔值、缺失字段和 `null` 均不视为成功。HTTP 和 RPC 错误由通用客户端检查。
设置请求不会自动重试。

`eweb-unlock` 复用登录接口取得 SID 和 SN，调用 `ToggleDevelopMode(..., true)` 后计算密码，
最多等待 15 秒验证 `root` 用户、`54133` 端口的 SSH 密码认证。成功的设置响应并不代表 SSH
已经就绪；只有认证成功才显示 SSH 密码。API 协议依据实际请求/响应样例，测试使用模拟设备。
