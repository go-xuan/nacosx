# nacosx

Nacos 客户端封装，支持配置中心与服务发现，基于 configx 配置加载框架，可同时作为其他模块的远程配置源。

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
namespace: "public"          # 命名空间
group: "DEFAULT_GROUP"       # 配置分组，可选，不填按 DEFAULT_GROUP 处理
mode: 2                      # 0-仅配置中心 1-仅服务发现 2-配置中心+服务发现
```

```go
import "github.com/go-xuan/nacosx"

func main() {
    nacosx.Initialize()                       // 加载 conf/nacos.yaml 并创建客户端
    client := nacosx.GetClient()              // 获取全局客户端
    group := client.GetGroup()                // DEFAULT_GROUP
}
```

`nacosx.Config` 实现了 `configx.Configurator`，因此也可以交给配置框架统一编排：

```go
configx.LoadConfigurator(&nacosx.Config{})
```

> nacosx 自身的配置走本地文件（`conf/nacos.yaml`），不支持从 nacos 读取自身配置——这是配置中心的引导（bootstrap）配置。

## 配置说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `address` | string | Nacos 服务地址，多个以英文逗号分隔（`host:port,host:port`），省略端口默认 `8848`，兼容 `[ipv6]:port` 与 `scheme://host:port/path`（如 `https://nacos.io:8848/custom`，协议与路径会写到 SDK 的 `Scheme` / `ContextPath`，未指定时由 SDK 兜底为 `http` / `/nacos`）；可用 `Config.AddressUrl()` 取首个地址的访问地址（已自带路径时原样返回，不会重复拼 `/nacos`） |
| `username` | string | 用户名 |
| `password` | string | 密码 |
| `access_key` | string | 阿里云 AK（开启鉴权时使用） |
| `secret_key` | string | 阿里云 SK（开启鉴权时使用） |
| `namespace` | string | 命名空间 ID，**必填**；公共命名空间填 `public`（等价于空串，见下） |
| `group` | string | 配置分组，**可选**，不填按 `DEFAULT_GROUP` 处理，同时作为 Reader 的默认锚点 |
| `mode` | int | 模式：`0`-仅配置中心 `1`-仅服务发现 `2`-两者都启用 |

`address` / `namespace` 是必须显式配置的两项（去除首尾空白后判断，`address` 还需要至少 1 个非空地址段：`","`、`" , "` 这类写法都算没配），构成 `Valid()` 的判断条件，任一为空则配置器不会初始化客户端；`NewClient` 套用同一条件并返回明确错误 —— 这两项为空只会由 SDK 报出含义模糊的 `server configs not found in properties`，要等拿到客户端之后才暴露。`group` 是可选配置，为空时统一按 nacos 默认分组 `DEFAULT_GROUP` 处理：服务发现的 `GroupName` 为空时 SDK 本就会自行兜底，配置读取遇到空分组则会被 SDK 直接拒绝，所以在 `GetGroup()` 与 `Reader.ConfigParam()` 里统一兜底。

> 注：SDK 只在**读**配置时校验空分组（返回 `param.group can not be empty`）；写路径（`PublishConfig` / `DeleteConfig`）的空分组校验少了 `return`，会把空分组原样发给服务端，详见「已知限制」。

> Nacos 公共命名空间的 tenant 为空串（SDK 文档要求 public 场景传空串），因此 `namespace: public` 会被归一化成空串，无需在配置里写空值。

### 运行模式

| Mode | 常量 | 初始化的客户端 |
|------|------|--------------|
| `0` | `nacosx.OnlyConfig` | 仅 `configClient`，可读写/监听配置 |
| `1` | `nacosx.OnlyNaming` | 仅 `namingClient`，可注册/发现服务 |
| `2` | `nacosx.ConfigAndNaming` | 两者都初始化 |

模式未启用对应的客户端时，调用 `GetConfigClient()` / `GetNamingClient()` 会 panic。可用 `Config.EnableNaming()` 判断当前是否启用服务发现。

## 包级函数

| 函数 | 说明 |
|------|------|
| `Initialize() error` | 读取 `conf/nacos.yaml` 并初始化全局客户端 |
| `NewClient(*Config) (*Client, error)` | 用指定配置创建独立客户端（不写入全局；`Config.Valid()` 不通过时直接返回错误，不创建任何底层客户端） |
| `Initialized() bool` | 全局客户端是否已初始化 |
| `GetClient() *Client` | 获取全局客户端，未初始化时 panic |

## Client 接口

### 配置中心

| 方法 | 说明 |
|------|------|
| `GetConfig(param, publishIfNotExist)` | 获取配置内容；`publishIfNotExist` 为 true 且配置不存在时发布空配置 |
| `ReadConfig(config any, param)` | 获取并反序列化到 `config`（必须传非空指针），返回原始字节；反序列化为**整体替换**（先解析到临时对象再替换目标值，内容中缺失的字段会被重置为零值） |
| `PublishConfig(param)` | 发布/更新配置 |
| `DeleteConfig(param)` | 删除配置 |
| `ListenConfig(param)` | 监听配置变更，回调取自调用方传入的 `param.OnChange`，回调为空时直接返回错误 |
| `CancelListenConfig(param)` | 取消监听 |
| `SearchConfig(param)` | 模糊搜索配置，返回 `*model.ConfigPage` |

### 服务发现

nacosx 未对服务发现做二次封装，直接暴露原生客户端：

```go
naming := nacosx.GetClient().GetNamingClient()

// 注册实例
ok, err := naming.RegisterInstance(vo.RegisterInstanceParam{
    Ip:          "127.0.0.1",
    Port:        8888,
    ServiceName: "demo-server",
    GroupName:   nacosx.GetClient().GetGroup(),
    Weight:      1,
    Enable:      true,
    Healthy:     true,
    Ephemeral:   true,
})

// 注销实例
_, err = naming.DeregisterInstance(vo.DeregisterInstanceParam{
    Ip:          "127.0.0.1",
    Port:        8888,
    ServiceName: "demo-server",
    GroupName:   nacosx.GetClient().GetGroup(),
})

// 查询健康实例
instances, err := naming.SelectInstances(vo.SelectInstancesParam{
    ServiceName: "demo-server",
    GroupName:   nacosx.GetClient().GetGroup(),
    HealthyOnly: true,
})
```

### Client 其他方法

| 方法 | 说明 |
|------|------|
| `GetGroup()` | 获取配置分组，未配置分组时为 `DEFAULT_GROUP` |
| `GetConfigClient()` | 获取原生配置中心客户端 |
| `GetNamingClient()` | 获取原生服务发现客户端 |

## 配置读取器 Reader

`Reader` 实现了 `configx.Reader`，用于把 nacos 作为配置来源接入 configx 的多源加载流程。典型用法是放在文件读取器之前，实现"远端优先、本地兜底"：

```go
func (c *Config) Readers() []configx.Reader {
    return []configx.Reader{
        nacosx.NewReader("database.yaml"),       // 优先从 nacos 读取
        configx.NewFileReader("database.yaml"),  // 回退到本地文件
        configx.NewTagReader(),                  // 再回退到结构体 default 标签
    }
}
```

```go
// 创建读取器，第二个参数控制是否监听配置变更（可选，默认 false）
reader := nacosx.NewReader("database.yaml", true)

// 手动读取
var db DBConfig
if err := reader.Read(&db); err != nil {
    // ...
}
```

| 方法 | 说明 |
|------|------|
| `NewReader(dataId string, listen ...bool)` | 创建读取器，配置类型由 `GetType()` 按 dataId 后缀解析，不预先写死 |
| `Read(v any) error` | 从 nacos 读取并反序列化到 `v`（必须传非空指针）；命中缓存时不重复拉取；监听注册失败只记日志、不影响返回值 |
| `Write(v any) error` | 序列化 `v` 并**整体覆盖**发布到 nacos，发布成功后才更新缓存 |
| `Anchor(group string)` | 设置分组锚点（分组为空时生效，由 configx 调用） |
| `Location() string` | 返回位置标识，形如 `nacos@DEFAULT_GROUP@database.yaml` |
| `ConfigParam() vo.ConfigParam` | 构造 nacos 配置参数；分组未锚定且未配置时按 `DEFAULT_GROUP` 兜底（SDK 不接受空分组） |
| `GetType() string` | 获取配置类型：优先按 dataId 后缀识别，其次显式类型，均无法识别时回退 `json`；只有已知格式（`json`/`yaml`/`yml`/`toml`/`properties`/`msgpack`）才被识别，`com.example.app` 这类后缀会被忽略；`yml` 归一为 `yaml` |

也可以直接构造结构体指定分组：

```go
reader := &nacosx.Reader{
    DataId: "rand.json",
    Group:  "DEFAULT_GROUP",
    Listen: true,
}
```

## 配置监听

### 原生监听

`Client.ListenConfig` 是对 SDK 的透传，回调由调用方自己提供：

```go
var cfg AppConfig

client := nacosx.GetClient()
param := vo.ConfigParam{
    DataId: "app.yaml",
    Group:  client.GetGroup(),
    Type:   vo.YAML,
}
param.OnChange = func(namespace, group, dataId, data string) {
    if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
        log.Printf("update config failed: %v", err)
        return
    }
    log.Printf("config changed: %s@%s", group, dataId)
}

if err := client.ListenConfig(param); err != nil {
    log.Fatal(err)
}

// 不再需要时取消监听
defer client.CancelListenConfig(param)
```

> SDK 对同一个 `dataId` + `group` 只保留首个注册的监听，重复注册不会替换回调。

### Reader 自动监听

`Reader` 在 `Read` 中自行注册回调（`NewReader("app.yaml", true)` 或 `&Reader{Listen: true}`），因此能同时维护目标结构体与缓存：

- 以 `dataId` / `group` / `namespace` / `length` 为字段打印一条 `the nacos config data has changed !!!` 日志，配置内容本身只在 `Debug` 级别输出（避免数据库密码等进入 `Info` 日志）；
- 首次读取、命中缓存、监听回调三条路径都走同一套反序列化逻辑：先解析到临时对象，成功后才整体替换目标结构体 `v`（内容中缺失的字段会被重置为零值，不是与旧值就地合并）；回调成功后再刷新 `Reader.Data`，所以变更后再调用同一个 `Reader` 的 `Read` 命中缓存拿到的也是最新内容；`v` 必须是非空指针，否则直接返回错误而不是 panic；
- 解析失败只记录错误日志，`v` 与 `Reader.Data` 都保持原值，不会出现新旧字段混合；内容为空（nacos 上配置被删除）时同样只记日志、不更新 —— 因为 `yaml`/`toml` 等序列化器会把空内容当成解析成功，若继续处理会把缓存置空，导致后续 `Read` 命中缓存一直返回删除前的旧值且不报错。

`Reader` 不提供取消监听的接口（没有保存注册参数），需要时用 `client.CancelListenConfig(reader.ConfigParam())`。

回调由 SDK 在独立 goroutine 中执行，会并发写入 `v` 与 `Reader.Data`，读取侧如需并发访问请自行加锁。

## 客户端细节

`Config.ClientConfig()` 中固定了以下参数，如需调整可直接修改该方法返回的结构体：

| 参数 | 值 | 说明 |
|------|-----|------|
| `TimeoutMs` | `10000` | 请求超时 |
| `BeatInterval` | `3000` | 心跳间隔 |
| `NotLoadCacheAtStart` | `true` | 启动时不加载本地缓存 |
| `LogDir` | `resource/.nacos/log` | SDK 日志目录 |
| `CacheDir` | `resource/.nacos/cache` | SDK 缓存目录 |

## 主要功能

- **配置读写** — 获取、发布、删除、模糊搜索配置
- **配置监听** — `Client` 透传原生监听能力，`Reader` 开启 `Listen` 后自动反序列化到目标结构体、刷新缓存并记录日志
- **服务发现** — 通过 `GetNamingClient()` 使用原生的注册/注销/查询能力
- **多源配置** — 作为 `configx.Reader` 接入，与本地文件、环境变量、结构体标签按优先级组合
- **多实例支持** — `NewClient` 可创建任意多个独立客户端（如 `quanx-cmd` 的配置迁移场景）

## 已知限制

- **无法优雅关闭** — 底层 `nacos-sdk-go v1.1.6` 的配置/服务发现客户端未提供 `Close` 接口，长轮询与心跳 goroutine 只能随进程退出而结束。
- **全局客户端不可替换** — `Config.Execute()` 在客户端已存在时直接返回，重复加载不会重建连接。
- **服务发现未封装** — 需自行使用 `GetNamingClient()` 返回的原生客户端。
- **监听回调不传播错误** — 反序列化失败仅记录日志，调用方无法感知。
- **重复监听不生效** — SDK 对同一 `dataId` + `group` 只保留首个注册的回调，第二个监听会被静默忽略；取消监听同样是按 `dataId` + `group` + `namespace` 整体移除，无法区分注册方。
- **监听注册不重试** — `Reader` 只在首次 `Read` 时注册监听，失败仅记录日志，后续 `Read` 命中缓存不会再尝试。
- **`Write` 是整体覆盖** — 只序列化传入结构体后发布，远端存在而结构体中缺失的配置项会被抹掉，且不校验配置是否已存在。
- **监听回调并发写入** — SDK 在独立 goroutine 中执行回调，会并发写入目标结构体与 `Reader.Data`，调用方需自行保证读取侧的并发安全。
- **反序列化为整体替换** — 首次读取、缓存读取与监听回调都按「解析到临时对象后整体替换」处理，nacos 上配置里缺失的字段会被重置为零值，而不是保留结构体中的旧值。
- **SDK 写路径不校验空分组** — `nacos-sdk-go v1.1.6` 的 `PublishConfig` / `DeleteConfig` 在分组或内容为空时只构造错误、没有 `return`，错误随即被覆盖，空分组会被原样发给服务端（读路径 `GetConfig` 会正常返回 `param.group can not be empty`）。nacosx 侧已通过 `DEFAULT_GROUP` 兜底规避，但直接使用 `GetConfigClient()` 时仍需自行保证分组非空。
