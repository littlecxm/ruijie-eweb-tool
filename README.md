# Ruijie Eweb Tool

用于锐捷 eWeb 设备的工具

## 已测试设备

- RG-EG105G/RG-EG105G-P-E V2 `[ReyeeOS 2.430.0.1924](https://www.ruijie.com.cn/fw/rj/292238/#)`

## 命令工具

```sh
go build -o eweb-tool ./cmd/eweb-tool
```

```sh
./eweb-tool --config /path/to/config.toml
./eweb-tool --verify-tls
./eweb-tool --help
```

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

```sh
./eweb-firm dec firmware.bin
./eweb-firm enc firmware.tar
./eweb-firm enc firmware.tar --format v2 -out firmware-v2.bin
./eweb-firm --help
```
