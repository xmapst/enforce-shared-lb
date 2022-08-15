package processor

import (
	"context"
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/model"
	"enforce-shared-lb/internal/processor/service"
	"enforce-shared-lb/internal/provider"
	"enforce-shared-lb/internal/provider/loadbalancer"
	"fmt"
	"github.com/sirupsen/logrus"
)

type consumer struct {
	conf    *config.Configure
	service *service.Service
	lb      provider.LoadBalancerInterface
}

func Consumer(context context.Context, objCh chan model.Event) error {
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
			case <-context.Done():
				return
			case obj := <-objCh:
				if _, ok := c.service.GLock[obj.Project]; !ok {
					c.service.GLock[obj.Project] = config.GLock.NewMutex(fmt.Sprintf("%s:%s:lock", c.conf.KeyPrefix, obj.Project))
				}
				c.event(objCh, obj)
			}
		}
	}()
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
