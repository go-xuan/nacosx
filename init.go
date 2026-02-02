package nacosx

import (
	"github.com/go-xuan/configx"
	log "github.com/sirupsen/logrus"
)

func init() {
	Init() // 初始化 nacos
}

func Init() {
	logger := log.WithField("package", "nacosx")
	if err := configx.LoadConfigurator(&Config{}); err == nil && Initialized() {
		logger.Info("initialized success")
		return
	}
	logger.Warn("initialized failed")
}
