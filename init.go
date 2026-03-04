package nacosx

import (
	"github.com/go-xuan/configx"
	"github.com/go-xuan/utilx/errorx"
	log "github.com/sirupsen/logrus"
)

func Initialize() error {
	logger := log.WithField("package", "nacosx")
	if err := configx.LoadConfigurator(&Config{}); err == nil && Initialized() {
		logger.Info("initialize success")
		return nil
	}
	logger.Warn("initialize failed")
	return errorx.New("initialize nacosx failed")
}
