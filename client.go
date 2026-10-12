package nacosx

import (
	"reflect"
	"strings"

	"github.com/go-xuan/utilx/errorx"
	"github.com/go-xuan/utilx/filex"
	"github.com/go-xuan/utilx/marshalx"
	"github.com/nacos-group/nacos-sdk-go/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/model"
	"github.com/nacos-group/nacos-sdk-go/vo"
)

// 客户端
var _client *Client

// GetClient 获取客户端
func GetClient() *Client {
	if _client == nil {
		panic("nacos client not initialized, please check the relevant config")
	}
	return _client
}

// Initialized 是否初始化
func Initialized() bool {
	return _client != nil
}

// NewClient 创建nacos客户端
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, errorx.New("the nacos config cannot be nil")
	}
	// 与配置框架使用同一套校验：地址与命名空间缺失时客户端都不可用。
	// 这两项为空只会由SDK报出含义模糊的"server configs not found in properties"，
	// 要等调用方拿到客户端之后才会暴露，此处提前拦截；分组是可选项，为空时按默认分组兜底
	if !config.Valid() {
		return nil, errorx.New("the nacos config is invalid: address and namespace cannot be empty")
	}
	client := &Client{config: config}
	var param = config.ClientParam()
	var err error
	switch config.Mode {
	case OnlyConfig: // 仅初始化配置中心
		if client.configClient, err = config.ConfigClient(param); err != nil {
			return nil, errorx.Wrap(err, "init nacos config client failed")
		}
	case OnlyNaming: // 仅初始化服务发现
		if client.namingClient, err = config.NamingClient(param); err != nil {
			return nil, errorx.Wrap(err, "init nacos naming client failed")
		}
	case ConfigAndNaming: // 初始化配置中心和服务发现
		if client.configClient, err = config.ConfigClient(param); err != nil {
			return nil, errorx.Wrap(err, "init nacos config client failed")
		}
		if client.namingClient, err = config.NamingClient(param); err != nil {
			return nil, errorx.Wrap(err, "init nacos naming client failed")
		}
	default: // 模式非法时直接报错，避免返回一个所有客户端均为nil的实例
		return nil, errorx.Sprintf("invalid nacos mode: %d, expected %d(config only), %d(naming only) or %d(both)",
			config.Mode, OnlyConfig, OnlyNaming, ConfigAndNaming)
	}
	return client, nil
}

// Client nacos客户端
type Client struct {
	config       *Config                     // nacos配置
	configClient config_client.IConfigClient // nacos配置中心客户端
	namingClient naming_client.INamingClient // nacos服务发现客户端
}

// GetGroup 获取分组，未配置分组时为nacos默认分组
func (c *Client) GetGroup() string {
	return defaultGroupOf(c.config.Group)
}

// GetConfigClient 获取配置中心客户端
func (c *Client) GetConfigClient() config_client.IConfigClient {
	if c.configClient == nil {
		panic("the nacos config client has not been initialized")
	}
	return c.configClient
}

// GetNamingClient 获取服务发现客户端
func (c *Client) GetNamingClient() naming_client.INamingClient {
	if c.namingClient == nil {
		panic("the nacos naming client has not been initialized")
	}
	return c.namingClient
}

// PublishConfig 发布nacos配置
func (c *Client) PublishConfig(param vo.ConfigParam) error {
	if _, err := c.GetConfigClient().PublishConfig(param); err != nil {
		return errorx.Wrap(err, "nacos config client publish config failed")
	}
	return nil
}

// GetConfig 获取nacos配置
func (c *Client) GetConfig(param vo.ConfigParam, publishIfNotExist bool) (string, error) {
	content, err := c.GetConfigClient().GetConfig(param)
	if err != nil {
		return "", errorx.Wrap(err, "get nacos config failed")
	}
	// 如果配置不存在，则发布配置
	if content == "" && publishIfNotExist {
		if err = c.PublishConfig(param); err != nil {
			return "", errorx.Wrap(err, "publish nacos config failed")
		}
	}
	return content, nil
}

// DeleteConfig 删除nacos配置
func (c *Client) DeleteConfig(param vo.ConfigParam) error {
	if _, err := c.GetConfigClient().DeleteConfig(param); err != nil {
		return errorx.Wrap(err, "nacos config client delete config failed")
	}
	return nil
}

// ReadConfig 读取nacos配置
func (c *Client) ReadConfig(config any, param vo.ConfigParam) ([]byte, error) {
	// 配置值必须是非空指针：否则既无法解析，也无从判断该不该发起请求
	if _, err := ptrValue(config); err != nil {
		return nil, err
	}
	// 读取配置文件内容
	content, err := c.GetConfig(param, false)
	if err != nil {
		return nil, errorx.Wrap(err, "read nacos config failed")
	}
	if content == "" {
		return nil, errorx.Sprintf("read nacos config empty: group=%s, dataId=%s", param.Group, param.DataId)
	}
	// 解析配置文件内容
	data := []byte(content)
	if err := unmarshal(resolveType(param.DataId, string(param.Type)), data, config); err != nil {
		return nil, errorx.Wrap(err, "unmarshal nacos config failed")
	}
	return data, nil
}

// ptrValue 校验并返回目标值：必须是非空指针，否则反序列化时可能panic（yaml遇到typed-nil指针会崩）
func ptrValue(v any) (reflect.Value, error) {
	value := reflect.ValueOf(v)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return value, errorx.New("the scanned object must be of non-nil pointer type")
	}
	return value, nil
}

// unmarshal 反序列化配置内容到目标指针：先解析到临时对象，成功后再整体替换目标值。
// 缓存、首次读取与监听回调三处统一走这里，保证更新语义一致（整体替换而非就地合并），
// 也避免解析失败时目标结构体出现新旧字段混合
func unmarshal(format string, data []byte, v any) error {
	value, err := ptrValue(v)
	if err != nil {
		return err
	}
	target := reflect.New(value.Type().Elem())
	if err = marshalx.Apply(format).Unmarshal(data, target.Interface()); err != nil {
		return err
	}
	value.Elem().Set(target.Elem())
	return nil
}

// resolveType 解析配置类型：dataId后缀是内容格式的约定，识别到时优先于调用方传入的类型，
// 其次显式类型，均无法识别时回退json；同时把yml归一为yaml，与nacos-sdk-go的ConfigType常量保持一致
func resolveType(dataId, explicit string) string {
	// 只有已知格式的后缀才算类型，com.example.app这类dataId的后缀不是格式，不能覆盖显式类型
	if suffix := filex.GetSuffix(dataId); isFormat(suffix) {
		explicit = suffix
	}
	if !isFormat(explicit) {
		// 显式类型也不是已知格式时同样回退json，避免把未知类型透传给序列化器
		return marshalx.JSON
	}
	if strings.EqualFold(explicit, marshalx.YML) {
		return marshalx.YAML
	}
	return strings.ToLower(explicit)
}

// isFormat 是否为序列化器支持的配置格式
func isFormat(name string) bool {
	switch strings.ToLower(name) {
	case marshalx.JSON, marshalx.YML, marshalx.YAML, marshalx.TOML, marshalx.PROPERTIES, marshalx.MSGPACK:
		return true
	}
	return false
}

// ListenConfig 监听nacos配置
func (c *Client) ListenConfig(param vo.ConfigParam) error {
	// SDK不校验回调，为空时会在配置变更触发的goroutine里panic，此处提前拦截
	if param.OnChange == nil {
		return errorx.New("the onChange callback of nacos config cannot be nil")
	}
	if err := c.GetConfigClient().ListenConfig(param); err != nil {
		return errorx.Wrap(err, "listen nacos config failed")
	}
	return nil
}

// CancelListenConfig 取消监听nacos配置
func (c *Client) CancelListenConfig(param vo.ConfigParam) error {
	if err := c.GetConfigClient().CancelListenConfig(param); err != nil {
		return errorx.Wrap(err, "cancel listen nacos config failed")
	}
	return nil
}

// SearchConfig 搜索nacos配置
func (c *Client) SearchConfig(param vo.SearchConfigParam) (*model.ConfigPage, error) {
	page, err := c.GetConfigClient().SearchConfig(param)
	if err != nil {
		return nil, errorx.Wrap(err, "search nacos config failed")
	}
	if page == nil { // 未搜索到配置时可能返回nil，避免调用方解引用panic
		page = &model.ConfigPage{}
	}
	return page, nil
}
