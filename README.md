# Ruijie Eweb Tool

用于锐捷 eWeb 设备的工具

## 已测试设备

- RG-EG105G/RG-EG105G-P-E V2 [ReyeeOS 2.430.0.1924](https://www.ruijie.com.cn/fw/rj/292238/#)

## 命令工具

```sh
./eweb-tool --verify-tls
./eweb-tool --help
```

## SSH 解锁

```sh
./eweb-unlock
./eweb-unlock --verify-tls
./eweb-unlock --help
```

## 固件工具

```sh
./eweb-firm dec firmware.bin
./eweb-firm enc firmware.tar
./eweb-firm enc firmware.tar --format v2 -out firmware-v2.bin
./eweb-firm --help
```
