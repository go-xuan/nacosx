package nacosx

import (
	"github.com/go-xuan/configx"
	"github.com/go-xuan/utilx/errorx"
	log "github.com/sirupsen/logrus"
)

// Initialize 加载conf/nacos.yaml并初始化全局客户端
func Initialize() error {
	logger := log.WithField("package", "nacosx")
	// 保留根因错误，便于区分文件缺失、解析失败与客户端创建失败
	if err := configx.LoadConfigurator(&Config{}); err != nil {
		logger.WithError(err).Warn("initialize failed")
		return errorx.Wrap(err, "failed to initialize nacosx")
	}
	if !Initialized() {
		logger.Warn("initialize failed")
		return errorx.New("failed to initialize nacosx")
	}
	logger.Info("initialize success")
	return nil
}
