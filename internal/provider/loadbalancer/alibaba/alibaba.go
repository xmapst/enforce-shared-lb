package alibaba

import (
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/provider"
	"enforce-shared-lb/internal/utils"
	"fmt"
	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	slb "github.com/alibabacloud-go/slb-20140515/v3/client"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/sirupsen/logrus"
	"time"
)

type aliCloud struct {
	client          *slb.Client
	endpoint        *string
	accessKeyId     *string
	accessKeySecret *string
	conf            *config.AlibabaConf
	request         *slb.CreateLoadBalancerRequest
}

func New() provider.LoadBalancerInterface {
	a := &aliCloud{
		endpoint:        config.Conf.Cloud.Endpoint,
		accessKeyId:     config.Conf.Cloud.AccessKeyId,
		accessKeySecret: config.Conf.Cloud.AccessKeySecret,
		conf:            config.Conf.CloudConf.(*config.AlibabaConf),
		request:         config.Conf.CloudConf.(*slb.CreateLoadBalancerRequest),
	}
	return a
}

func (a *aliCloud) CreateClient() (err error) {
	_config := &openapi.Config{
		Endpoint:        a.endpoint,
		AccessKeyId:     a.accessKeyId,
		AccessKeySecret: a.accessKeySecret,
	}
	a.client, err = slb.NewClient(_config)
	return err
}

func (a *aliCloud) Create() (string, error) {
	var request = *a.request
	request.LoadBalancerName = tea.String(fmt.Sprintf("%s-%d", *a.request.LoadBalancerName, time.Now().Unix()))
	var loadBalancerId *string
	fn := func() error {
		resp, err := a.client.CreateLoadBalancer(&request)
		if err != nil {
			return err
		}
		logrus.Info(resp.String())
		loadBalancerId = resp.Body.LoadBalancerId
		return nil
	}
	err := utils.Retry(3, "创建阿里云SLB失败", fn)
	if err != nil {
		return *loadBalancerId, err
	}
	return *loadBalancerId, nil
}

func (a *aliCloud) Delete(id string) error { return nil }

func (a *aliCloud) Describe(id string) error { return nil }

func (a *aliCloud) Annotation(id string, annotation map[string]string) {
	annotation["service.beta.kubernetes.io/alibaba-cloud-loadbalancer-id"] = id
}

func (a *aliCloud) CheckAnnotation(annotation map[string]string) bool {
	if _, ok := annotation["service.beta.kubernetes.io/alibaba-cloud-loadbalancer-id"]; ok {
		if annotation["service.beta.kubernetes.io/alibaba-cloud-loadbalancer-id"] != "" {
			return true
		}
	}
	return false
}
