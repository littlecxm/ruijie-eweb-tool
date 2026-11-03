# Ruijie Eweb Tool Agents Documentation

需要 Go 1.26.4 或更新版本。

## 命令工具

```sh
go build -o eweb-tool ./cmd/eweb-tool
./eweb-tool
```

启动后进入 TUI，选择已保存设备、**新增设备**或退出。新增设备的设备网址默认是
`https://192.168.110.1`，可修改为实际地址，
账号固定为 `admin`，密码输入使用掩码。选择已有设备后直接使用保存的密码登录，
登录失败时再要求输入新密码；保存的密码无效或无法解密时也会要求重新输入。
登录成功后进入操作菜单，可列出在线终端，或进入**设备概览、系统设置**二级菜单。
`system` 的查询、设置、备份恢复、固件升级和设备维护操作统一放在**系统设置**中；各二级菜单都可返回上级。
再次选择查询项可刷新数据。
列表包含有线和无线终端，显示名称、IP、MAC、接入方式、接入设备、SSID、频段和端口。

```sh
./eweb-tool --config /path/to/config.toml
./eweb-tool --verify-tls
./eweb-tool --help
```

默认兼容设备的自签名 HTTPS 证书；`--verify-tls` 开启证书验证。请求超时为 15 秒。
向导需要交互终端；自动化程序可调用下方 Go API。
删除文件、重启、恢复、升级及升级清理操作需要在向导中确认，默认取消。
可能中断设备连接的操作成功提交后结束会话，待设备完成操作后重新登录。
系统设置中的固件升级操作提交已在设备上的镜像，需要提前完成上传和校验。

## SSH 解锁

```sh
go build -o eweb-unlock ./cmd/eweb-unlock
./eweb-unlock
./eweb-unlock --verify-tls
./eweb-unlock --help
```

## 固件工具

```sh
go build -o eweb-firm ./cmd/eweb-firm
./eweb-firm
```

无参数启动全屏 TUI，可选择固件解密、v1 加密、v2 加密或退出。
解密自动识别 v1/v2；文件路径支持空格、单/双引号及终端拖入时的反斜杠转义（如 `\ `、`\(`、`\)`）。
处理成功或失败后均返回菜单，结果保留在可滚动区域；输入路径时 `Esc` 可返回菜单。

也可直接使用命令行，无需交互终端：

```sh
./eweb-firm dec firmware.bin
./eweb-firm enc firmware.tar
./eweb-firm enc firmware.tar --format v2 -out firmware-v2.bin
./eweb-firm --help
```

## 配置

`eweb-tool` 的配置默认保存在当前目录的 `config.toml`。文件不存在时，首次登录并成功获取设备
信息后自动创建。设备以 `getAuthority` 返回的 `sysinfo.serial_num` 为索引；
更新一个设备时保留其他设备和 app 设置。参考
[配置示例](pkg/config/config.toml.example)。

`password` 保存带随机 Salt 的 GibberishAES 密文。配置包含可用于重新登录的凭据，
通过原子替换写入，文件权限为 `0600`。旧配置的明文密码或损坏密文会要求重新输入，
并在成功登录后更新。解析错误、登录失败、SN 不一致和写入失败均不会覆盖原配置。

## 会话

每次登录读取设备登录页的当前加密口令。配置使用固定存储口令；登录时在内存中解密，
再使用当前设备口令加密发送。设备要求刷新口令时最多重试一次。向导使用保存的密码登录失败后，
允许手动输入新密码再登录一次，成功后更新配置；取消、超时或设备锁定时直接返回。
Go API 不会重试密码错误或锁定。
token、sid 和默认 3600 秒有效期仅保留在返回的内存会话中。
