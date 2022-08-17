package processor

import (
	"context"
	"enforce-shared-lb/internal/cache"
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/model"
	"enforce-shared-lb/internal/processor/service"
	"enforce-shared-lb/internal/provider"
	"enforce-shared-lb/internal/provider/loadbalancer"
	"github.com/sirupsen/logrus"
)

type consumer struct {
	conf    *config.Configure
	service *service.Service
	lb      provider.LoadBalancerInterface
}

func Consumer(ctx context.Context, objCh chan model.Event) error {
	lb, err := loadbalancer.New()
	if err != nil {
		logrus.Error(err)
		return err
	}
	var c = &consumer{
		conf:    config.Conf,
		service: service.New(),
		lb:      lb,
	}
	c.service.LB = lb

	go func() {
		for {
			select {
			case <-ctx.Done():
				logrus.Infoln("stop Consumer")
				return
			case obj := <-objCh:
				c.event(objCh, obj)
			}
		}
	}()
	if c.conf.AutoClean {
		ch := make(chan string, c.conf.ChannelSize)
		cache.DB.Recycle(300, ch)
		go func() {
			for id := range ch {
				logrus.Infoln("clean idle loadBalancer", id)
				err = c.service.LB.Delete(id)
				if err != nil {
					logrus.Error(err)
				}
			}
		}()
	}
	return nil
}

func (c *consumer) event(objCh chan model.Event, obj model.Event) {
	switch obj.BindType {
	case model.Service:
		// asynchronously process service event
		c.service.RetryProcess(objCh, obj)
	case model.Http:
		logrus.Info(obj.Data)
	default:
		logrus.Warningf("source %s is not supported", obj.BindType)
		return
	}
}
