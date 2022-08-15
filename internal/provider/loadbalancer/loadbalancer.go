package loadbalancer

import (
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/provider"
	"enforce-shared-lb/internal/provider/loadbalancer/alibaba"
	"enforce-shared-lb/internal/provider/loadbalancer/fake"
	"enforce-shared-lb/internal/provider/loadbalancer/huawei"
	"enforce-shared-lb/internal/provider/loadbalancer/tencent"
)

func New() (lb provider.LoadBalancerInterface, err error) {
	switch config.Conf.Cloud.Name {
	case config.AlibabaCloud:
		lb = alibaba.New()
	case config.HuaweiCloud:
		lb = huawei.New()
	case config.TencentCloud:
		lb = tencent.New()
	default:
		lb = fake.New()
	}
	err = lb.CreateClient()
	return lb, err
}
