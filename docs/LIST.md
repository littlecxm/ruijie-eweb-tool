# Eweb Module List

## 概览

| API        | Method       | Module |
| ---------- | ------------ | ------ |
| api/common | getAuthority | nil    |

## 监控

| API     | Method     | Module    |
| ------- | ---------- | --------- |
| api/cmd | devSta.get | user_list |


| API          | Method    | Module |
| ------------ | --------- | ------ |
| api/overview | getUptime | nil    |


## 基本管理 `stepBaseSetting`

### WAN配置 `stepnetwork_wan`

URL: /admin/alone/network/network_wan

### PoE供电 `stepeg_poe`

URL: /admin/alone/network/eg_poe

| API     | Method     | Module |
| ------- | ---------- | ------ |
| api/cmd | devSta.get | poe    |

### 端口设置 `stepport_set`

URL: /admin/alone/network/port_set

| API     | Method        | Module      |
| ------- | ------------- | ----------- |
| api/cmd | devSta.get    | port_base   |
| api/cmd | devSta.get    | port_status |
| api/cmd | devConfig.get | portDesc    |

### AP指示灯 `stepled_cfg`

URL: /admin/alone/network/led_cfg

| API     | Method       | Module |
| ------- | ------------ | ------ |
| api/cmd | acConfig.get | devLed |

## 高级管理 `stepadvanced`

### DNS设置 `stepadvanced_dns`

URL: /admin/alone/advanced/advanced_dns

| API     | Method        | Module           |
| ------- | ------------- | ---------------- |
| api/cmd | devConfig.get | local_dns_server |

## 故障诊断 `stepdiagnose`

### 网络智检 `stepdiagnose_network`

URL: /admin/alone/diagnose/diagnose_network

| API     | Method     | Module   |
| ------- | ---------- | -------- |
| api/cmd | devSta.get | dev_diag |

### 故障告警 `stepdiagnose_alarm`

URL: /admin/alone/diagnose/diagnose_alarm

| API     | Method        | Module       |
| ------- | ------------- | ------------ |
| api/cmd | devSta.get    | alarm_map    |
| api/cmd | devConfig.get | alarm_upload |
| api/cmd | devSta.get    | ap_list_lite |

### 网络工具 `stepdiagnose_tool`

URL: /admin/alone/diagnose/diagnose_tool

| API          | Method         | Module |
| ------------ | -------------- | ------ |
| api/diagnose | getDiagnoseRes | nil    |

### 抓包诊断 `stepdiagnose_package`

URL: /admin/alone/diagnose/diagnose_package

| API          | Method          | Module |
| ------------ | --------------- | ------ |
| api/diagnose | getMemAndIfaces | nil    |

### 故障收集 `stepdiagnose_tech`

URL: /admin/alone/diagnose/diagnose_tech

HTTP POST Form Data

| API          | Method      | Module |
| ------------ | ----------- | ------ |
| api/download | techSupport | nil    |

#### 本地升级 `stepsystool_upgrade_tab-1`

URL: /admin/alone/systool/systool_upgrade?tab=1

| API     | Method     | Module       |
| ------- | ---------- | ------------ |
| api/cmd | devSta.set | develop_mode |

```json
{
    "method": "devSta.set",
    "params": {
        "module": "develop_mode",
        "noParse": false,
        "async": null,
        "remoteIp": false,
        "data": {
            "developMode": "1"
        },
        "device": "pc"
    }
}
```

### 云服务 `stepsystool_cloud`

URL: /admin/alone/systool/systool_cloud

| API     | Method       | Module           |
| ------- | ------------ | ---------------- |
| api/cmd | acConfig.get | network_group    |
| api/cmd | devSta.get   | cloudDomainCfg   |
| api/cmd | devSta.get   | cloud_serverlist |

### 连通性探测配置 `stepsystool_detect_config`

URL: /admin/alone/systool/detect_config

| API     | Method        | Module       |
| ------- | ------------- | ------------ |
| api/cmd | devConfig.get | connectivity |
