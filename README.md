# nacosx

Nacos 客户端封装，支持配置中心和服务发现，基于 configx 配置加载框架。

## 安装

```bash
go get github.com/go-xuan/nacosx
```

## 快速开始

在 `conf/nacos.yaml` 中配置：

```yaml
address: "127.0.0.1:8848"
username: "nacos"
password: "nacos"
namespace: "demo"
group: "DEFAULT_GROUP"
mode: 2  # 0-仅配置中心 1-仅服务发现 2-配置中心+服务发现
```

```go
import "github.com/go-xuan/nacosx"

// 创建 Nacos 配置读取器
reader := nacosx.NewReader("database.yaml")

// Nacos 客户端
client, err := nacosx.NewClient(config)
```

## 主要功能

- **配置读取** — `NewReader(dataId)` 创建 Reader，配合 configx 框架按优先级加载
- **服务发现** — 注册/注销服务实例，查询健康实例
- **配置监听** — 支持配置变更回调
