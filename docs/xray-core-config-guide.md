# Xray-core 配置完全指南

> 基于 [XTLS/Xray-core](https://github.com/XTLS/Xray-core) 源码分析编写  
> 配置文件格式：JSON

---

## 目录

1. [配置文件总体结构](#1-配置文件总体结构)
2. [log - 日志配置](#2-log---日志配置)
3. [dns - DNS 配置](#3-dns---dns-配置)
4. [routing - 路由配置](#4-routing---路由配置)
5. [inbounds - 入站配置](#5-inbounds---入站配置)
6. [outbounds - 出站配置](#6-outbounds---出站配置)
7. [transport / streamSettings - 传输配置](#7-transport--streamsettings---传输配置)
8. [policy - 策略配置](#8-policy---策略配置)
9. [api - API 配置](#9-api---api-配置)
10. [stats - 统计配置](#10-stats---统计配置)
11. [reverse - 反向代理配置](#11-reverse---反向代理配置)
12. [fakeDns - FakeDNS 配置](#12-fakedns---fakedns-配置)
13. [observatory - 连接观测配置](#13-observatory---连接观测配置)
14. [协议详解](#14-协议详解)
15. [TLS / REALITY 安全配置](#15-tls--reality-安全配置)

---

## 1. 配置文件总体结构

```json
{
  "log": {},
  "dns": {},
  "routing": {},
  "inbounds": [],
  "outbounds": [],
  "policy": {},
  "api": {},
  "stats": {},
  "reverse": {},
  "fakeDns": {},
  "observatory": {},
  "burstObservatory": {}
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `log` | LogConfig | 日志设置 |
| `dns` | DNSConfig | DNS 服务器设置 |
| `routing` | RouterConfig | 路由规则 |
| `inbounds` | \[InboundConfig\] | 入站连接配置数组 |
| `outbounds` | \[OutboundConfig\] | 出站连接配置数组 |
| `policy` | PolicyConfig | 本地策略 |
| `api` | APIConfig | 远程控制 API |
| `stats` | StatsConfig | 统计信息（空对象即可启用） |
| `reverse` | ReverseConfig | 反向代理 |
| `fakeDns` | FakeDNSConfig | FakeDNS 设置 |
| `observatory` | ObservatoryConfig | 连接观测 |
| `burstObservatory` | BurstObservatoryConfig | 突发连接观测 |

---

## 2. log - 日志配置

```json
{
  "log": {
    "access": "/var/log/xray/access.log",
    "error": "/var/log/xray/error.log",
    "loglevel": "warning",
    "dnsLog": false,
    "maskAddress": ""
  }
}
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `access` | string | `""` | 访问日志文件路径，留空则输出到 stdout，设为 `"none"` 可禁用 |
| `error` | string | `""` | 错误日志文件路径，留空则输出到 stderr |
| `loglevel` | string | `"warning"` | 日志级别：`"debug"` / `"info"` / `"warning"` / `"error"` / `"none"` |
| `dnsLog` | bool | `false` | 是否记录 DNS 查询日志 |
| `maskAddress` | string | `""` | 日志中 IP 地址的掩码处理方式 |

---

## 3. dns - DNS 配置

### 3.1 完整结构

```json
{
  "dns": {
    "servers": [],
    "hosts": {},
    "clientIp": "1.2.3.4",
    "tag": "dns-inbound",
    "queryStrategy": "UseIP",
    "disableCache": false,
    "serveStale": false,
    "serveExpiredTTL": 86400,
    "disableFallback": false,
    "disableFallbackIfMatch": false,
    "enableParallelQuery": false,
    "useSystemHosts": false
  }
}
```

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `servers` | \[NameServerConfig\] | `[]` | DNS 服务器列表 |
| `hosts` | object | `{}` | 静态域名-IP 映射（类似 hosts 文件） |
| `clientIp` | string | `""` | 向 DNS 服务器声明的客户端 IP（用于 EDNS Client Subnet） |
| `tag` | string | `""` | DNS 查询的入站标签，用于路由匹配 |
| `queryStrategy` | string | `"UseIP"` | 查询策略：`"UseIP"` / `"UseIPv4"` / `"UseIPv6"` / `"UseSystem"` |
| `disableCache` | bool | `false` | 禁用 DNS 缓存 |
| `serveStale` | bool | `false` | 缓存过期后仍返回旧记录 |
| `serveExpiredTTL` | uint32 | `0` | 过期记录可服务的最大秒数 |
| `disableFallback` | bool | `false` | 禁用 DNS 回退查询 |
| `disableFallbackIfMatch` | bool | `false` | 命中域名规则后禁用回退 |
| `enableParallelQuery` | bool | `false` | 启用并行 DNS 查询 |
| `useSystemHosts` | bool | `false` | 读取系统 hosts 文件 |

### 3.2 servers - DNS 服务器

支持简写和对象两种格式：

**简写格式：**
```json
"servers": ["8.8.8.8", "1.1.1.1", "localhost"]
```

**对象格式：**
```json
{
  "address": "8.8.8.8",
  "port": 53,
  "clientIp": "1.2.3.4",
  "skipFallback": false,
  "domains": ["domain:example.com", "geosite:cn"],
  "expectedIPs": ["geoip:cn"],
  "unexpectedIPs": [],
  "queryStrategy": "UseIP",
  "tag": "dns-remote",
  "timeoutMs": 5000,
  "disableCache": false,
  "serveStale": true,
  "serveExpiredTTL": 172800,
  "finalQuery": false
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `address` | string | DNS 服务器地址。支持：IP（UDP）、`https://` (DoH)、`h2c://` (h2c DoH)、`quic://` (DoQ)、`tcp://` (DoT)、`localhost`、`fakedns` |
| `port` | uint16 | 端口号，默认 53 |
| `clientIp` | string | 针对此服务器的 EDNS Client Subnet IP |
| `skipFallback` | bool | 此服务器不参与回退查询 |
| `domains` | \[string\] | 优先使用此服务器查询的域名列表 |
| `expectedIPs` | \[string\] | 期望返回的 IP 范围，不匹配时使用下一个服务器 |
| `unexpectedIPs` | \[string\] | 不期望的 IP 范围，命中则过滤掉 |
| `queryStrategy` | string | 此服务器的查询策略（覆盖全局） |
| `tag` | string | 此 DNS 服务器使用的路由出站标签 |
| `timeoutMs` | uint64 | 查询超时（毫秒） |
| `finalQuery` | bool | 使用此服务器查询后不再回退 |

### 3.3 hosts - 静态域名映射

```json
{
  "hosts": {
    "example.com": "127.0.0.1",
    "domain:example.com": "google.com",
    "geosite:category-ads-all": "127.0.0.1",
    "keyword:google": ["8.8.8.8", "8.8.4.4"],
    "regexp:.*\\.com": "8.8.4.4",
    "www.example.org": ["127.0.0.1", "127.0.0.2"]
  }
}
```

域名匹配类型前缀：
- **无前缀** - 精确匹配（`full:`）
- `domain:` - 子域名匹配
- `geosite:` - 使用 geosite 数据文件
- `keyword:` - 关键词匹配
- `regexp:` - 正则表达式匹配
- `full:` - 完整域名匹配
- `ext:file:tag` - 外部文件
- `dotless:` - 无点域名匹配

---

## 4. routing - 路由配置

### 4.1 完整结构

```json
{
  "routing": {
    "domainStrategy": "AsIs",
    "rules": [],
    "balancers": []
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `domainStrategy` | string | 域名解析策略 |
| `rules` | \[RoutingRule\] | 路由规则数组（按顺序匹配，匹配即停） |
| `balancers` | \[BalancingRule\] | 负载均衡器 |

**domainStrategy 可选值：**

| 值 | 说明 |
|----|------|
| `"AsIs"` | 不解析域名，直接按域名匹配路由规则 |
| `"IPIfNonMatch"` | 域名规则未匹配时，解析为 IP 再进行 IP 规则匹配 |
| `"IPOnDemand"` | 遇到 IP 规则时，立即解析域名进行匹配 |

### 4.2 rules - 路由规则

```json
{
  "ruleTag": "my-rule",
  "domain": ["geosite:cn", "domain:example.com"],
  "ip": ["geoip:cn", "10.0.0.0/8", "::1/128"],
  "port": "53,443,1000-2000",
  "sourcePort": "1234",
  "network": "tcp,udp",
  "source": ["10.0.0.0/8"],
  "user": ["user@email.com"],
  "inboundTag": ["inbound-tag"],
  "protocol": ["http", "tls", "bittorrent"],
  "attrs": {},
  "outboundTag": "direct",
  "balancerTag": ""
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `ruleTag` | string | 规则标签，用于调试和日志 |
| `domain` | \[string\] | 域名匹配列表 |
| `ip` | \[string\] | IP 匹配列表（CIDR 格式或 geoip） |
| `port` | string/int | 目标端口范围 |
| `sourcePort` | string/int | 来源端口范围 |
| `network` | string | 网络类型：`"tcp"` / `"udp"` / `"tcp,udp"` |
| `source` | \[string\] | 来源 IP 列表 |
| `user` | \[string\] | 用户邮箱匹配 |
| `inboundTag` | \[string\] | 入站标签匹配 |
| `protocol` | \[string\] | 协议嗅探匹配：`"http"` / `"tls"` / `"quic"` / `"bittorrent"` |
| `outboundTag` | string | 匹配后转发到此出站标签 |
| `balancerTag` | string | 匹配后使用此负载均衡器 |

**域名匹配格式：**

| 格式 | 示例 | 说明 |
|------|------|------|
| 纯字符串 | `"example.com"` | 子域名匹配 |
| `domain:` | `"domain:example.com"` | 子域名匹配 |
| `full:` | `"full:www.example.com"` | 精确匹配 |
| `keyword:` | `"keyword:google"` | 关键词匹配 |
| `regexp:` | `"regexp:.*\\.cn$"` | 正则匹配 |
| `geosite:` | `"geosite:cn"` | GeoSite 数据库 |
| `ext:` | `"ext:file.dat:tag"` | 外部数据文件 |

**IP 匹配格式：**

| 格式 | 示例 | 说明 |
|------|------|------|
| CIDR | `"10.0.0.0/8"` | CIDR 范围 |
| `geoip:` | `"geoip:cn"` | GeoIP 数据库 |
| `ext:` | `"ext:file.dat:tag"` | 外部数据文件 |

### 4.3 balancers - 负载均衡器

```json
{
  "tag": "balancer-name",
  "selector": ["proxy-a", "proxy-b"],
  "strategy": {
    "type": "random",
    "settings": {}
  },
  "fallbackTag": "direct"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `tag` | string | 均衡器标签 |
| `selector` | \[string\] | 出站标签前缀选择器 |
| `strategy` | object | 均衡策略（`"random"` / `"leastPing"` / `"leastLoad"`） |
| `fallbackTag` | string | 所有出站不可用时的回退标签 |

---

## 5. inbounds - 入站配置

### 5.1 通用结构

```json
{
  "protocol": "vless",
  "port": 443,
  "listen": "0.0.0.0",
  "tag": "inbound-tag",
  "settings": {},
  "streamSettings": {},
  "sniffing": {
    "enabled": true,
    "destOverride": ["http", "tls", "quic", "fakedns"],
    "domainsExcluded": [],
    "metadataOnly": false,
    "routeOnly": false
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `protocol` | string | 协议名称 |
| `port` | string/int | 监听端口（支持范围如 `"443-500"`） |
| `listen` | string | 监听地址，默认 `"0.0.0.0"` |
| `tag` | string | 入站标签，用于路由引用 |
| `settings` | object | 协议相关设置 |
| `streamSettings` | StreamConfig | 传输层设置 |
| `sniffing` | object | 流量嗅探设置 |

**支持的入站协议：**
- `vless` - VLESS 协议
- `vmess` - VMess 协议
- `trojan` - Trojan 协议
- `shadowsocks` - Shadowsocks 协议
- `socks` - SOCKS5 协议
- `http` - HTTP 代理
- `dokodemo-door` (或 `tunnel`) - 透明代理/任意门
- `wireguard` - WireGuard
- `mixed` - SOCKS/HTTP 混合代理
- `tun` - TUN 网络接口

### 5.2 sniffing - 流量嗅探

```json
{
  "enabled": true,
  "destOverride": ["http", "tls", "quic", "fakedns"],
  "domainsExcluded": ["domain:example.com"],
  "metadataOnly": false,
  "routeOnly": false
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `enabled` | bool | 启用嗅探 |
| `destOverride` | \[string\] | 嗅探类型：`"http"` / `"tls"` / `"quic"` / `"fakedns"` |
| `domainsExcluded` | \[string\] | 排除的域名列表 |
| `metadataOnly` | bool | 仅使用元数据嗅探（不读取内容） |
| `routeOnly` | bool | 仅用于路由，不覆盖目标地址 |

---

## 6. outbounds - 出站配置

### 6.1 通用结构

```json
{
  "protocol": "vless",
  "tag": "proxy",
  "settings": {},
  "streamSettings": {},
  "sendThrough": "0.0.0.0",
  "proxySettings": {
    "tag": "another-outbound",
    "transportLayer": false
  },
  "mux": {
    "enabled": false,
    "concurrency": 8,
    "xudpConcurrency": 16,
    "xudpProxyUDP443": "reject"
  },
  "targetStrategy": "AsIs"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `protocol` | string | 出站协议名称 |
| `tag` | string | 出站标签 |
| `settings` | object | 协议相关设置 |
| `streamSettings` | StreamConfig | 传输层设置 |
| `sendThrough` | string | 本地发送地址 |
| `proxySettings` | object | 链式代理设置 |
| `mux` | object | 多路复用设置 |
| `targetStrategy` | string | 域名策略：`"AsIs"` / `"UseIP"` / `"UseIPv4"` / `"UseIPv6"` 等 |

**targetStrategy 可选值：**

| 值 | 说明 |
|----|------|
| `"AsIs"` | 直接使用域名 |
| `"UseIP"` | 解析为 IP |
| `"UseIPv4"` / `"UseIPv6"` | 解析为指定版本 IP |
| `"UseIPv4v6"` / `"UseIPv6v4"` | 优先使用指定版本，回退另一版本 |
| `"ForceIP"` / `"ForceIPv4"` / `"ForceIPv6"` | 强制解析 |
| `"ForceIPv4v6"` / `"ForceIPv6v4"` | 强制解析并按优先级排序 |

**支持的出站协议：**
- `vless` - VLESS
- `vmess` - VMess
- `trojan` - Trojan
- `shadowsocks` - Shadowsocks
- `socks` - SOCKS5
- `http` - HTTP
- `freedom` (或 `direct`) - 直连
- `blackhole` (或 `block`) - 黑洞（丢弃流量）
- `dns` - DNS 出站
- `wireguard` - WireGuard
- `loopback` - 回环
- `hysteria` - Hysteria

### 6.2 mux - 多路复用

```json
{
  "enabled": true,
  "concurrency": 8,
  "xudpConcurrency": 16,
  "xudpProxyUDP443": "reject"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `enabled` | bool | 启用 Mux |
| `concurrency` | int16 | TCP 多路复用并发数（-1 完全禁用） |
| `xudpConcurrency` | int16 | XUDP 多路复用并发数 |
| `xudpProxyUDP443` | string | XUDP 对 UDP/443 的处理：`"reject"` / `"allow"` / `"skip"` |

---

## 7. transport / streamSettings - 传输配置

### 7.1 完整结构

```json
{
  "network": "tcp",
  "security": "tls",
  "tlsSettings": {},
  "realitySettings": {},
  "tcpSettings": {},
  "wsSettings": {},
  "httpupgradeSettings": {},
  "grpcSettings": {},
  "xhttpSettings": {},
  "sockopt": {}
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `network` | string | 传输协议 |
| `security` | string | 安全层：`""` / `"tls"` / `"reality"` / `"none"` |
| `tlsSettings` | TLSConfig | TLS 配置（当 security=tls） |
| `realitySettings` | REALITYConfig | REALITY 配置（当 security=reality） |
| `sockopt` | SocketConfig | 底层 Socket 选项 |

**network 传输协议可选值：**

| 值 | 说明 |
|----|------|
| `"tcp"` / `"raw"` | 原始 TCP |
| `"ws"` | WebSocket |
| `"grpc"` | gRPC |
| `"httpupgrade"` | HTTP Upgrade |
| `"splithttp"` / `"xhttp"` | Split HTTP (XHTTP) |

**REALITY 支持的传输层：**
- `"tcp"` (RAW)
- `"splithttp"` / `"xhttp"` (XHTTP)
- `"grpc"`

### 7.2 TCP (Raw) 传输

```json
{
  "tcpSettings": {
    "header": {
      "type": "none"
    },
    "acceptProxyProtocol": false
  }
}
```

header.type 伪装类型：`"none"` / `"http"`

### 7.3 WebSocket 传输

```json
{
  "wsSettings": {
    "host": "example.com",
    "path": "/ws-path",
    "headers": {
      "Host": "example.com"
    },
    "acceptProxyProtocol": false,
    "heartbeatPeriod": 0
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `host` | string | HTTP Host 头 |
| `path` | string | WebSocket 路径 |
| `headers` | object | 自定义 HTTP 头 |
| `heartbeatPeriod` | uint32 | 心跳间隔（秒） |

### 7.4 gRPC 传输

```json
{
  "grpcSettings": {
    "serviceName": "GunService",
    "multiMode": false,
    "authority": "",
    "idle_timeout": 60,
    "health_check_timeout": 20,
    "permit_without_stream": false,
    "initial_windows_size": 0
  }
}
```

### 7.5 HTTP Upgrade 传输

```json
{
  "httpupgradeSettings": {
    "host": "example.com",
    "path": "/path",
    "headers": {},
    "acceptProxyProtocol": false
  }
}
```

### 7.6 XHTTP (Split HTTP) 传输

```json
{
  "xhttpSettings": {
    "host": "example.com",
    "path": "/xhttp",
    "headers": {},
    "mode": "auto"
  }
}
```

### 7.7 sockopt - Socket 选项

```json
{
  "sockopt": {
    "mark": 255,
    "tcpFastOpen": false,
    "tproxy": "off",
    "domainStrategy": "AsIs",
    "dialerProxy": "",
    "acceptProxyProtocol": false,
    "tcpKeepAliveInterval": 0,
    "tcpKeepAliveIdle": 300,
    "tcpNoDelay": false,
    "tcpCongestion": "bbr",
    "interface": "",
    "tcpMptcp": false,
    "addressPortStrategy": "",
    "happyEyeballs": {
      "prioritizeIPv6": false,
      "tryDelayMs": 250,
      "interleave": 1,
      "maxConcurrentTry": 2
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `mark` | int | Linux SO_MARK |
| `tcpFastOpen` | bool/int | TCP Fast Open |
| `tproxy` | string | 透明代理方式：`"off"` / `"redirect"` / `"tproxy"` |
| `domainStrategy` | string | 出站连接的域名解析策略 |
| `dialerProxy` | string | 拨号使用的代理出站标签 |
| `tcpCongestion` | string | TCP 拥塞控制算法（如 `"bbr"`） |
| `interface` | string | 绑定的网卡接口 |
| `tcpMptcp` | bool | 启用 MPTCP |

---

## 8. policy - 策略配置

```json
{
  "policy": {
    "levels": {
      "0": {
        "handshake": 4,
        "connIdle": 300,
        "uplinkOnly": 2,
        "downlinkOnly": 5,
        "statsUserUplink": false,
        "statsUserDownlink": false,
        "statsUserOnline": false,
        "bufferSize": 10240
      }
    },
    "system": {
      "statsInboundUplink": false,
      "statsInboundDownlink": false,
      "statsOutboundUplink": false,
      "statsOutboundDownlink": false
    }
  }
}
```

### levels - 用户等级策略

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `handshake` | uint32 | `4` | 握手超时（秒） |
| `connIdle` | uint32 | `300` | 连接空闲超时（秒） |
| `uplinkOnly` | uint32 | `2` | 上行无数据后等待时间（秒） |
| `downlinkOnly` | uint32 | `5` | 下行无数据后等待时间（秒） |
| `statsUserUplink` | bool | `false` | 统计用户上行流量 |
| `statsUserDownlink` | bool | `false` | 统计用户下行流量 |
| `statsUserOnline` | bool | `false` | 统计用户在线状态 |
| `bufferSize` | int32 | `10240` | 每连接缓冲区大小（KB），`0` = 无缓冲，`-1` = 不限 |

### system - 系统级策略

| 字段 | 类型 | 说明 |
|------|------|------|
| `statsInboundUplink` | bool | 统计入站上行流量 |
| `statsInboundDownlink` | bool | 统计入站下行流量 |
| `statsOutboundUplink` | bool | 统计出站上行流量 |
| `statsOutboundDownlink` | bool | 统计出站下行流量 |

---

## 9. api - API 配置

```json
{
  "api": {
    "tag": "api",
    "listen": "127.0.0.1:8080",
    "services": [
      "HandlerService",
      "StatsService",
      "LoggerService",
      "RoutingService",
      "ObservatoryService"
    ]
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `tag` | string | API 入站标签 |
| `listen` | string | API 监听地址 |
| `services` | \[string\] | 启用的 API 服务 |

**可用服务：**

| 服务名 | 说明 |
|--------|------|
| `HandlerService` | 管理入站/出站处理器（添加/删除/修改） |
| `StatsService` | 查询统计数据 |
| `LoggerService` | 运行时重启日志 |
| `RoutingService` | 管理路由规则 |
| `ObservatoryService` | 连接观测服务 |

> **注意：** API 需要配合路由规则和 `dokodemo-door` 入站使用（见完整示例）。

---

## 10. stats - 统计配置

```json
{
  "stats": {}
}
```

空对象即可启用。需配合 `policy` 中开启 stats 相关选项使用。

---

## 11. reverse - 反向代理配置

```json
{
  "reverse": {
    "bridges": [
      {
        "tag": "bridge",
        "domain": "private.example.com"
      }
    ],
    "portals": [
      {
        "tag": "portal",
        "domain": "private.example.com"
      }
    ]
  }
}
```

| 组件 | 字段 | 说明 |
|------|------|------|
| bridges | `tag` | 桥接标签 |
| bridges | `domain` | 桥接使用的虚拟域名 |
| portals | `tag` | 门户标签 |
| portals | `domain` | 门户使用的虚拟域名 |

---

## 12. fakeDns - FakeDNS 配置

```json
{
  "fakeDns": {
    "ipPool": "198.18.0.0/15",
    "poolSize": 65535
  }
}
```

或多池配置：

```json
{
  "fakeDns": [
    { "ipPool": "198.18.0.0/15", "poolSize": 65535 },
    { "ipPool": "fc00::/18", "poolSize": 65535 }
  ]
}
```

---

## 13. observatory - 连接观测配置

### 标准观测

```json
{
  "observatory": {
    "subjectSelector": ["proxy"],
    "probeURL": "https://www.google.com/generate_204",
    "probeInterval": "10s",
    "enableConcurrency": true
  }
}
```

### 突发观测

```json
{
  "burstObservatory": {
    "subjectSelector": ["proxy"],
    "pingConfig": {
      "destination": "https://connectivitycheck.gstatic.com/generate_204",
      "interval": "1h",
      "connectivity": "1h",
      "timeout": "30s",
      "sampling": 3,
      "rounds": 1
    }
  }
}
```

---

## 14. 协议详解

### 14.1 VLESS（推荐）

**入站（服务端）：**

```json
{
  "protocol": "vless",
  "settings": {
    "clients": [
      {
        "id": "uuid-here",
        "flow": "xtls-rprx-vision",
        "level": 0,
        "email": "user@example.com"
      }
    ],
    "decryption": "none",
    "fallbacks": [
      { "dest": 80 },
      { "alpn": "h2", "dest": "/dev/shm/h2c.sock", "xver": 2 },
      { "path": "/ws", "dest": "serve-ws-none" }
    ]
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `clients` | array | 用户列表 |
| `clients[].id` | string | 用户 UUID |
| `clients[].flow` | string | 流控模式：`""` / `"xtls-rprx-vision"` / `"xtls-rprx-vision-udp443"` |
| `clients[].level` | uint32 | 用户等级 |
| `clients[].email` | string | 用户邮箱（用于统计） |
| `decryption` | string | 加密方式，目前只支持 `"none"` |
| `fallbacks` | array | 回落配置（当非 VLESS 流量到达时） |

**出站（客户端）：**

```json
{
  "protocol": "vless",
  "settings": {
    "vnext": [
      {
        "address": "server.example.com",
        "port": 443,
        "users": [
          {
            "id": "uuid-here",
            "flow": "xtls-rprx-vision",
            "encryption": "none",
            "level": 0
          }
        ]
      }
    ]
  }
}
```

简写格式（推荐）：
```json
{
  "protocol": "vless",
  "settings": {
    "address": "server.example.com",
    "port": 443,
    "id": "uuid-here",
    "flow": "xtls-rprx-vision",
    "encryption": "none"
  }
}
```

### 14.2 VMess

**入站：**

```json
{
  "protocol": "vmess",
  "settings": {
    "clients": [
      {
        "id": "uuid-here",
        "level": 0,
        "email": "user@example.com",
        "security": "auto"
      }
    ],
    "default": { "level": 0 }
  }
}
```

**出站：**

```json
{
  "protocol": "vmess",
  "settings": {
    "vnext": [
      {
        "address": "server.example.com",
        "port": 443,
        "users": [
          {
            "id": "uuid-here",
            "security": "auto",
            "level": 0
          }
        ]
      }
    ]
  }
}
```

VMess 安全类型（`security`）：
- `"auto"` - 自动选择
- `"aes-128-gcm"` - AES-128-GCM
- `"chacha20-poly1305"` - ChaCha20-Poly1305
- `"none"` - 不加密（不建议）
- `"zero"` - 不加密不校验

### 14.3 Trojan

**入站：**

```json
{
  "protocol": "trojan",
  "settings": {
    "clients": [
      {
        "password": "my-password",
        "level": 0,
        "email": "user@example.com"
      }
    ],
    "fallbacks": [
      { "dest": 80 }
    ]
  }
}
```

**出站：**

```json
{
  "protocol": "trojan",
  "settings": {
    "servers": [
      {
        "address": "server.example.com",
        "port": 443,
        "password": "my-password",
        "level": 0,
        "email": "user@example.com"
      }
    ]
  }
}
```

简写：
```json
{
  "protocol": "trojan",
  "settings": {
    "address": "server.example.com",
    "port": 443,
    "password": "my-password"
  }
}
```

### 14.4 Shadowsocks

**入站（传统）：**

```json
{
  "protocol": "shadowsocks",
  "settings": {
    "method": "aes-256-gcm",
    "password": "my-password",
    "network": "tcp,udp"
  }
}
```

**入站（Shadowsocks 2022 单用户）：**

```json
{
  "protocol": "shadowsocks",
  "settings": {
    "method": "2022-blake3-aes-128-gcm",
    "password": "base64-encoded-key",
    "network": "tcp,udp"
  }
}
```

**入站（Shadowsocks 2022 多用户）：**

```json
{
  "protocol": "shadowsocks",
  "settings": {
    "method": "2022-blake3-aes-128-gcm",
    "password": "server-key",
    "clients": [
      { "password": "user-key-1", "email": "user1@example.com" },
      { "password": "user-key-2", "email": "user2@example.com" }
    ],
    "network": "tcp,udp"
  }
}
```

**出站：**

```json
{
  "protocol": "shadowsocks",
  "settings": {
    "servers": [
      {
        "address": "server.example.com",
        "port": 8388,
        "method": "aes-256-gcm",
        "password": "my-password",
        "level": 0
      }
    ]
  }
}
```

**支持的加密方式：**

| 类型 | 方法 |
|------|------|
| 传统 | `aes-128-gcm`, `aes-256-gcm`, `chacha20-poly1305`, `xchacha20-poly1305`, `none` |
| 2022 | `2022-blake3-aes-128-gcm`, `2022-blake3-aes-256-gcm`, `2022-blake3-chacha20-poly1305` |

### 14.5 SOCKS5

**入站：**

```json
{
  "protocol": "socks",
  "settings": {
    "auth": "password",
    "accounts": [
      { "user": "username", "pass": "password" }
    ],
    "udp": true,
    "ip": "127.0.0.1",
    "userLevel": 0
  }
}
```

**出站：**

```json
{
  "protocol": "socks",
  "settings": {
    "servers": [
      {
        "address": "127.0.0.1",
        "port": 1080,
        "users": [
          { "user": "username", "pass": "password" }
        ]
      }
    ]
  }
}
```

### 14.6 HTTP

**入站：**

```json
{
  "protocol": "http",
  "settings": {
    "accounts": [
      { "user": "username", "pass": "password" }
    ],
    "allowTransparent": false,
    "userLevel": 0
  }
}
```

**出站：**

```json
{
  "protocol": "http",
  "settings": {
    "servers": [
      {
        "address": "proxy.example.com",
        "port": 8080,
        "users": [
          { "user": "username", "pass": "password" }
        ]
      }
    ]
  }
}
```

### 14.7 Freedom (Direct) - 直连

```json
{
  "protocol": "freedom",
  "settings": {
    "domainStrategy": "AsIs",
    "redirect": "",
    "fragment": {
      "packets": "tlshello",
      "length": "100-200",
      "interval": "10-20"
    }
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `domainStrategy` | string | 域名解析策略 |
| `redirect` | string | 重定向目标地址（`"host:port"`） |
| `fragment` | object | TCP 分片设置（用于绕过 DPI） |
| `fragment.packets` | string | 分片类型：`"tlshello"` / `""` / `"1-3"` |
| `fragment.length` | string | 每片长度范围 |
| `fragment.interval` | string | 发送间隔（毫秒） |

### 14.8 Blackhole (Block) - 黑洞

```json
{
  "protocol": "blackhole",
  "settings": {
    "response": {
      "type": "none"
    }
  }
}
```

`response.type`：`"none"`（默认）/ `"http"`（返回 403）

### 14.9 DNS 出站

```json
{
  "protocol": "dns",
  "settings": {
    "network": "tcp",
    "address": "8.8.8.8",
    "port": 53,
    "nonIPQuery": "drop"
  }
}
```

### 14.10 Dokodemo-door (透明代理)

```json
{
  "protocol": "dokodemo-door",
  "settings": {
    "address": "1.2.3.4",
    "port": 80,
    "network": "tcp,udp",
    "followRedirect": true,
    "userLevel": 0
  }
}
```

### 14.11 WireGuard

```json
{
  "protocol": "wireguard",
  "settings": {
    "secretKey": "your-private-key",
    "address": ["10.0.0.2/32", "fd00::2/128"],
    "peers": [
      {
        "publicKey": "peer-public-key",
        "preSharedKey": "",
        "endpoint": "server:51820",
        "keepAlive": 25,
        "allowedIPs": ["0.0.0.0/0", "::/0"]
      }
    ],
    "mtu": 1420,
    "reserved": [0, 0, 0],
    "domainStrategy": "ForceIP"
  }
}
```

### 14.12 Loopback - 回环

```json
{
  "protocol": "loopback",
  "settings": {
    "inboundTag": "inbound-tag"
  }
}
```

### 14.13 TUN 入站

```json
{
  "protocol": "tun",
  "settings": {
    "name": "xray0",
    "MTU": 1500,
    "userLevel": 0
  }
}
```

---

## 15. TLS / REALITY 安全配置

### 15.1 TLS 配置

```json
{
  "security": "tls",
  "tlsSettings": {
    "serverName": "example.com",
    "alpn": ["h2", "http/1.1"],
    "allowInsecure": false,
    "fingerprint": "chrome",
    "certificates": [
      {
        "certificateFile": "/path/to/cert.pem",
        "keyFile": "/path/to/key.pem"
      }
    ],
    "disableSystemRoot": false,
    "minVersion": "1.2",
    "maxVersion": "1.3",
    "rejectUnknownSni": false,
    "pinnedPeerCertSha256": "",
    "masterKeyLog": ""
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `serverName` | string | 服务器名称（SNI） |
| `alpn` | \[string\] | ALPN 协商协议列表 |
| `allowInsecure` | bool | 允许不安全证书（**不建议**） |
| `fingerprint` | string | TLS 客户端指纹伪装 |
| `certificates` | array | 证书列表 |
| `minVersion` / `maxVersion` | string | TLS 版本范围 |
| `rejectUnknownSni` | bool | 拒绝未知 SNI |
| `pinnedPeerCertSha256` | string | 证书固定（SHA256） |
| `masterKeyLog` | string | TLS 密钥日志文件路径 |

**fingerprint 可选值：**
`"chrome"`, `"firefox"`, `"safari"`, `"ios"`, `"android"`, `"edge"`, `"360"`, `"qq"`, `"random"`, `"randomized"`

**证书配置：**

```json
{
  "certificateFile": "/path/to/fullchain.pem",
  "keyFile": "/path/to/privkey.pem",
  "usage": "encipherment",
  "ocspStapling": 3600,
  "buildChain": false,
  "oneTimeLoading": false
}
```

`usage` 值：`"encipherment"` / `"verify"` / `"issue"`

### 15.2 REALITY 配置（推荐）

REALITY 是 TLS 的替代方案，无需域名和证书，通过代理真实网站的 TLS 握手来伪装。

**服务端配置：**

```json
{
  "security": "reality",
  "realitySettings": {
    "show": false,
    "dest": "www.example.com:443",
    "type": "tcp",
    "xver": 0,
    "serverNames": ["www.example.com", "example.com"],
    "privateKey": "your-x25519-private-key",
    "shortIds": ["", "0123456789abcdef"],
    "maxTimeDiff": 0,
    "minClientVer": "",
    "maxClientVer": ""
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `show` | bool | 显示调试信息 |
| `dest` | string/int | 回落目标网站（需为支持 TLS 1.3 和 H2 的真实网站） |
| `type` | string | 回落类型：`"tcp"` / `"unix"` |
| `xver` | uint64 | PROXY Protocol 版本（0/1/2） |
| `serverNames` | \[string\] | 允许的服务器名称列表 |
| `privateKey` | string | X25519 私钥 |
| `shortIds` | \[string\] | 短 ID 列表（十六进制，0-16 字符） |
| `maxTimeDiff` | uint64 | 最大时间差（毫秒），`0` = 不检查 |
| `minClientVer` / `maxClientVer` | string | 客户端版本范围限制 |
| `mldsa65Seed` | string | ML-DSA-65 种子（后量子安全，可选） |

**客户端配置：**

```json
{
  "security": "reality",
  "realitySettings": {
    "fingerprint": "chrome",
    "serverName": "www.example.com",
    "publicKey": "your-x25519-public-key",
    "shortId": "0123456789abcdef",
    "spiderX": "/"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `fingerprint` | string | **必填**，浏览器指纹伪装 |
| `serverName` | string | 目标网站 SNI |
| `publicKey` | string | 服务端公钥（对应 privateKey） |
| `shortId` | string | 短 ID（需在服务端 shortIds 列表中） |
| `spiderX` | string | 爬虫初始路径 |

**生成密钥对：**
```bash
xray x25519
```

---

## 附录：常用 GeoSite / GeoIP 标签

### GeoSite 常用标签

| 标签 | 说明 |
|------|------|
| `geosite:cn` | 中国大陆域名 |
| `geosite:geolocation-cn` | 中国大陆域名（含更多） |
| `geosite:geolocation-!cn` | 非中国大陆域名 |
| `geosite:category-ads-all` | 广告域名 |
| `geosite:google` | Google 相关域名 |
| `geosite:facebook` | Facebook 相关域名 |
| `geosite:twitter` | Twitter 相关域名 |
| `geosite:telegram` | Telegram 相关域名 |
| `geosite:youtube` | YouTube 相关域名 |
| `geosite:netflix` | Netflix 相关域名 |
| `geosite:apple` | Apple 相关域名 |
| `geosite:microsoft` | Microsoft 相关域名 |
| `geosite:private` | 私有域名（localhost 等） |

### GeoIP 常用标签

| 标签 | 说明 |
|------|------|
| `geoip:cn` | 中国大陆 IP |
| `geoip:private` | 私有 IP（10.0.0.0/8 等） |
| `geoip:us` | 美国 IP |
| `geoip:jp` | 日本 IP |
| `geoip:hk` | 香港 IP |
| `geoip:tw` | 台湾 IP |
| `geoip:sg` | 新加坡 IP |

---

> 本文档基于 Xray-core 源码分析，涵盖所有主要配置模块。具体版本功能请以官方文档为准。
