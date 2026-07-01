# Xray-core 增强功能

以下是在上游 [XTLS/Xray-core](https://github.com/XTLS/Xray-core) 基础上新增的功能。

## TUN 系统协议栈

TUN 入站新增 `stackType` 配置项，可选 `"gvisor"`（默认）或 `"system"`。system 模式用原生方式处理 TCP/IP 包，不依赖 gVisor，占用更低。

```json
{
  "inbounds": [
    {
      "protocol": "tun",
      "settings": {
        "name": "xray0",
        "stackType": "system",
        "MTU": 1500
      }
    }
  ]
}
```

支持平台：Linux、Windows、macOS、Android、FreeBSD。

## QUIC v2 嗅探

QUIC 嗅探新增对 version 2 的支持。如果已在 `destOverride` 中启用了 `"quic"`，则无需更改配置即可自动识别 QUIC v2 流量。

## WebRTC 流量识别

新增基于 STUN 协议的 WebRTC 嗅探，在开启了流量嗅探的入站上自动生效，无需额外配置。

```json
{
  "inbounds": [
    {
      "sniffing": {
        "enabled": true,
        "destOverride": ["http", "tls", "quic"]
      }
    }
  ]
}
```

## 日志按天轮转

日志写入文件时，可通过 `logKeepDays` 指定保留天数，到期自动清理历史日志。

```json
{
  "log": {
    "loglevel": "warning",
    "logKeepDays": 7
  }
}
```

## 配置严格校验

JSON 解码器启用 `DisallowUnknownFields`，配置文件中的未知字段（如拼写错误 `"inbouns"`、`"loglevel"` 错写成 `"logLevel"`）会直接报错并提示位置，不再被静默忽略。

```json
{
  "inbouns": []   // ← 报错：unknown field "inbouns"
}
```

协议特定配置（`inbounds[].settings`、`routing.rules[]` 等）由下游 Build 方法校验，不受此影响。
