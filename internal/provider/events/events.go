package events

import (
	"enforce-shared-lb/internal/model"
	"enforce-shared-lb/internal/provider"
	"enforce-shared-lb/internal/provider/events/http"
	"enforce-shared-lb/internal/provider/events/kubernetes/service"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func register(router *gin.Engine) {
	service.New()
	http.New(router)
}

func Init(router *gin.Engine) {
	register(router)
	for _, e := range provider.EventList {
		if e == nil {
			continue
		}
		// 初始化
		err := e.Init()
		if err != nil {
			logrus.Warningf("init %s event plugins failed: %v", e.Name(), err)
		} else {
			logrus.Infof("init %s event plugins success", e.Name())
		}
	}
	return
}

func Start(eventCh chan model.Event) {
	for _, e := range provider.EventList {
		if e == nil {
			continue
		}
		go func(e provider.EventInterface) {
			err := e.StartWatch(eventCh)
			if err != nil {
				logrus.Error(err)
			}
		}(e)
	}
}

func Close() {
	if provider.EventList == nil {
		return
	}
	for _, e := range provider.EventList {
		if e == nil {
			continue
		}
		e.Close()
	}
}
