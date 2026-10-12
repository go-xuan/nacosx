package nacosx

import (
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-xuan/configx"
	"github.com/go-xuan/utilx/errorx"
	"github.com/go-xuan/utilx/stringx"
	"github.com/nacos-group/nacos-sdk-go/clients"
	"github.com/nacos-group/nacos-sdk-go/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/common/constant"
	"github.com/nacos-group/nacos-sdk-go/vo"
	log "github.com/sirupsen/logrus"
)

const (
	OnlyConfig      = iota // 仅启用配置中心
	OnlyNaming             // 仅启用服务发现
	ConfigAndNaming        // 启用配置中心和服务发现

	defaultPort = 8848 // nacos默认端口
)

// Config nacos连接配置
type Config struct {
	Address   string `yaml:"address" json:"address"`      // nacos服务地址,多个以英文逗号分割
	Username  string `yaml:"username" json:"username"`    // 用户名
	Password  string `yaml:"password" json:"password"`    // 密码
	AccessKey string `yaml:"access_key" json:"accessKey"` // ak
	SecretKey string `yaml:"secret_key" json:"secretKey"` // sk
	Namespace string `yaml:"namespace" json:"namespace"`  // 命名空间
	Group     string `yaml:"group" json:"group"`          // 配置分组
	Mode      int    `yaml:"mode" json:"mode"`            // 模式（0-仅配置中心；1-仅服务发现；2-配置中心和服务发现）
}

// LogFields 日志字段
func (c *Config) LogFields() map[string]any {
	fields := make(map[string]any)
	fields["address"] = c.Address
	fields["group"] = defaultGroupOf(c.Group) // 记录实际生效的分组，未配置时为默认分组
	fields["namespace"] = c.Namespace
	fields["mode"] = c.Mode
	return fields
}

func (c *Config) Readers() []configx.Reader {
	return []configx.Reader{
		configx.NewFileReader("nacos.yaml"),
	}
}

// Valid 配置是否可用：命名空间与至少一个有效服务地址必须显式配置
func (c *Config) Valid() bool {
	if strings.TrimSpace(c.Namespace) == "" {
		return false
	}
	// 地址按逗号分割，只要存在非空段就可用（" , " 这类写法不能算配了地址）
	for _, addr := range stringx.Split(c.Address, ",") {
		if addr != "" {
			return true
		}
	}
	return false
}

// defaultGroupOf 分组为空时按nacos默认分组兜底：
// 服务发现的GroupName为空时SDK本就会填DEFAULT_GROUP，
// 而配置读写遇到空分组会直接报"param.group can not be empty"，故统一兜底
func defaultGroupOf(group string) string {
	if group = strings.TrimSpace(group); group != "" {
		return group
	}
	return constant.DEFAULT_GROUP
}

func (c *Config) Execute() error {
	if _client == nil {
		// 底层已按字段记录并包装错误，此处不再重复记录
		client, err := NewClient(c)
		if err != nil {
			return err
		}
		_client = client
		log.WithFields(c.LogFields()).Info("init nacos success")
	}
	return nil
}

// AddressUrl nacos访问地址，取首个非空地址；未指定路径时补默认上下文，
// 地址自带路径（如 http://host:8848/nacos）时原样返回，避免重复拼接
func (c *Config) AddressUrl() string {
	for _, addr := range stringx.Split(c.Address, ",") {
		if addr == "" {
			continue
		}
		if !strings.Contains(addr, "://") {
			addr = "http://" + addr
		}
		parsed, err := url.Parse(addr)
		if err != nil {
			continue // 地址非法时跳过，尝试下一个
		}
		if parsed.Path == "" || parsed.Path == "/" {
			parsed.Path = "/nacos"
		}
		return strings.TrimSuffix(parsed.String(), "/")
	}
	return ""
}

// EnableNaming 是否开启服务注册
func (c *Config) EnableNaming() bool {
	return c.Mode == OnlyNaming || c.Mode == ConfigAndNaming
}

// ClientConfig nacos客户端配置
func (c *Config) ClientConfig() *constant.ClientConfig {
	return &constant.ClientConfig{
		NamespaceId:         namespaceId(c.Namespace),
		AccessKey:           c.AccessKey,
		SecretKey:           c.SecretKey,
		Username:            c.Username,
		Password:            c.Password,
		TimeoutMs:           10 * 1000,
		BeatInterval:        3 * 1000,
		NotLoadCacheAtStart: true,
		LogDir:              filepath.Join("resource", ".nacos/log"),
		CacheDir:            filepath.Join("resource", ".nacos/cache"),
	}
}

// namespaceId 归一化命名空间：nacos公共命名空间的tenant为空串（sdk文档要求public场景传空串），
// 而配置里习惯写public，此处统一转换，避免把public当作命名空间ID发给服务端
func namespaceId(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if strings.EqualFold(namespace, "public") {
		return ""
	}
	return namespace
}

// ServerConfigs nacos服务中间件配置
func (c *Config) ServerConfigs() []constant.ServerConfig {
	var configs []constant.ServerConfig
	for _, addr := range stringx.Split(c.Address, ",") {
		if addr == "" { // 跳过空地址，避免生成非法的服务配置
			continue
		}
		configs = append(configs, serverConfigOf(addr))
	}
	if len(configs) == 0 {
		log.Error("the address of nacos cannot be empty")
	}
	return configs
}

// serverConfigOf 解析单个服务地址，兼容host:port、[ipv6]:port与scheme://host:port/path三种写法；
// scheme与path分别落到SDK的Scheme与ContextPath上（两者为空时SDK按http、/nacos兜底），
// 与 AddressUrl 的口径保持一致；端口缺失或非法时回退默认端口，
// ipv6地址补回中括号（SDK按 scheme://ip:port 拼接）
func serverConfigOf(addr string) constant.ServerConfig {
	scheme, contextPath := "", ""
	if before, after, ok := strings.Cut(addr, "://"); ok {
		scheme, addr = before, after
	}
	if idx := strings.Index(addr, "/"); idx >= 0 {
		contextPath, addr = addr[idx:], addr[:idx]
	}
	host, port := addr, ""
	if h, p, err := net.SplitHostPort(addr); err == nil {
		host, port = h, p
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return constant.ServerConfig{
		Scheme:      scheme,
		ContextPath: strings.TrimSuffix(contextPath, "/"), // 空值时由SDK兜底为/nacos
		IpAddr:      host,
		Port:        parsePort(port, addr),
	}
}

// parsePort 解析端口：未指定或超出范围时回退默认端口
func parsePort(port, addr string) uint64 {
	if port == "" {
		return defaultPort
	}
	value := stringx.ParseInt64(port, 0)
	if value <= 0 || value > 65535 {
		log.Warnf("invalid port %q in nacos address %s, fallback to %d", port, addr, defaultPort)
		value = defaultPort
	}
	return uint64(value)
}

func (c *Config) ClientParam() vo.NacosClientParam {
	return vo.NacosClientParam{
		ClientConfig:  c.ClientConfig(),
		ServerConfigs: c.ServerConfigs(),
	}
}

// ConfigClient 初始化Nacos配置中心客户端
func (c *Config) ConfigClient(param vo.NacosClientParam) (config_client.IConfigClient, error) {
	logger := log.WithFields(c.LogFields())
	client, err := clients.NewConfigClient(param)
	if err != nil {
		logger.WithError(err).Error("create nacos config client failed")
		return nil, errorx.Wrap(err, "create nacos config client failed")
	}
	logger.Info("create nacos config client success")
	return client, nil
}

// NamingClient 初始化Nacos服务发现客户端
func (c *Config) NamingClient(param vo.NacosClientParam) (naming_client.INamingClient, error) {
	logger := log.WithFields(c.LogFields())
	client, err := clients.NewNamingClient(param)
	if err != nil {
		logger.WithError(err).Error("create nacos naming client failed")
		return nil, errorx.Wrap(err, "create nacos naming client failed")
	}
	logger.Info("create nacos naming client success")
	return client, nil
}
