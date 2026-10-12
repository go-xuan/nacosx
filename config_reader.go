package nacosx

import (
	"fmt"
	"reflect"

	"github.com/go-xuan/utilx/errorx"
	"github.com/go-xuan/utilx/marshalx"
	"github.com/nacos-group/nacos-sdk-go/vo"
	log "github.com/sirupsen/logrus"
)

// NewReader 创建nacos配置读取器，类型不在此处预填，统一由GetType按dataId后缀与显式类型解析
func NewReader(dataId string, listen ...bool) *Reader {
	return &Reader{
		DataId: dataId,
		Listen: len(listen) > 0 && listen[0],
	}
}

// Reader nacos配置读取器
type Reader struct {
	DataId string // 配置文件id
	Group  string // 配置所在分组
	Type   string // 配置文件类型
	Data   []byte // 配置文件内容
	Listen bool   // 是否启用监听
}

// ConfigParam 获取nacos配置参数
func (r *Reader) ConfigParam() vo.ConfigParam {
	return vo.ConfigParam{
		DataId:  r.DataId,
		Group:   defaultGroupOf(r.Group), // 未锚定分组时不能留空，否则SDK会拒绝读写
		Content: string(r.Data),
		Type:    vo.ConfigType(r.GetType()),
	}
}

// GetType 获取配置类型：优先按dataId后缀识别，其次显式类型，均无法识别时回退json
func (r *Reader) GetType() string {
	return resolveType(r.DataId, r.Type)
}

func (r *Reader) Anchor(group string) {
	if r.Group == "" {
		r.Group = group
	}
}

// Read 从nacos中读取配置
func (r *Reader) Read(v any) error {
	if r.Data == nil {
		if !Initialized() {
			return errorx.New("nacos not initialized")
		}
		client := GetClient()
		// 配置文件锚点为group分组
		r.Anchor(client.GetGroup())

		// 首次读取：ReadConfig 内部完成指针校验、空值校验与反序列化
		param := r.ConfigParam()
		data, err := client.ReadConfig(v, param)
		if err != nil {
			return errorx.Wrap(err, "read nacos config failed")
		}
		r.Data = data

		if r.Listen {
			// 回调在读取器内声明，才能同时更新目标结构体和Reader缓存
			param.OnChange = func(namespace, group, dataId, data string) {
				logger := log.WithField("dataId", dataId).
					WithField("group", group).
					WithField("namespace", namespace).
					WithField("length", len(data))
				logger.Info("the nacos config data has changed !!!")
				// 配置内容可能包含密码等敏感信息，只在Debug级别输出
				logger.WithField("data", data).Debug("the nacos config content")
				// 配置被删除时推送空内容，而yaml/toml等序列化器会把空内容当成解析成功，
				// 若继续处理会把缓存置空，导致后续Read命中缓存一直返回旧值且不报错
				if data == "" {
					logger.Warn("the nacos config content is empty, skip update")
					return
				}
				// 先解析到临时对象，成功后再整体替换，避免解析失败时目标结构体新旧混合
				value := reflect.ValueOf(v)
				target := reflect.New(value.Type().Elem())
				if err = marshalx.Apply(r.GetType()).Unmarshal([]byte(data), target.Interface()); err != nil {
					logger.WithError(err).Error("update nacos config failed")
					return
				}
				value.Elem().Set(target.Elem())
				// 解析成功后再刷新缓存，保证缓存与目标结构体一致
				r.Data = []byte(data)
			}
			// 监听注册失败不影响本次读取结果，仅记录日志，
			// 否则会让配置框架判定该读取器不可用，回退到本地文件配置
			if err = client.ListenConfig(param); err != nil {
				log.WithField("dataId", r.DataId).WithError(err).Warn("listen nacos config failed")
			}
		}
		return nil
	}

	// 命中缓存：配置内容无需重复拉取，但仍需反序列化到目标结构体
	if err := marshalx.Apply(r.GetType()).Unmarshal(r.Data, v); err != nil {
		return errorx.Wrap(err, "unmarshal nacos config failed")
	}
	return nil
}

// Location 配置文件位置
func (r *Reader) Location() string {
	return fmt.Sprintf("nacos@%s@%s", r.Group, r.DataId)
}

// Write 将配置写入nacos
func (r *Reader) Write(v any) error {
	if !Initialized() {
		return errorx.New("nacos not initialized")
	}
	client := GetClient()
	// 配置文件锚点为group分组
	r.Anchor(client.GetGroup())

	// 序列化配置
	data, err := marshalx.Apply(r.GetType()).Marshal(v)
	if err != nil {
		return errorx.Wrap(err, "marshal config failed")
	}

	// 发布配置，内容取本次序列化结果，不依赖缓存
	param := r.ConfigParam()
	param.Content = string(data)
	if err = client.PublishConfig(param); err != nil {
		return errorx.Wrap(err, "publish nacos config failed")
	}
	// 发布成功后再更新缓存，避免缓存内容与远端不一致
	r.Data = data
	return nil
}
