package init

import (
	"github/go-xuan/nacosx"

	"github.com/go-xuan/quanx/configx"
	"github.com/go-xuan/utilx/errorx"
)

func init() {
	errorx.Panic(Init())
}

func Init() error {
	err := configx.LoadConfigurator(&nacosx.Config{})
	if err == nil && nacosx.Initialized() {
		return nil
	}
	return errorx.Wrap(err, "init nacos failed")
}
