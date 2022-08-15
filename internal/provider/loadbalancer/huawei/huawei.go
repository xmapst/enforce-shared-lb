package huawei

import (
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/provider"
	"enforce-shared-lb/internal/utils"
	"fmt"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	elb "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v2/model"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v2/region"
	"github.com/sirupsen/logrus"
	"time"
)

type huaweiCloud struct {
	client          *elb.ElbClient
	endpoint        *string
	accessKeyId     *string
	accessKeySecret *string
	conf            *config.HuaweiConf
	request         *model.CreateLoadbalancerRequest
	id              string
	name            string
}

func New() provider.LoadBalancerInterface {
	h := &huaweiCloud{
		endpoint:        config.Conf.Cloud.Endpoint,
		accessKeyId:     config.Conf.Cloud.AccessKeyId,
		accessKeySecret: config.Conf.Cloud.AccessKeySecret,
		conf:            config.Conf.CloudConf.(*config.HuaweiConf),
		request: &model.CreateLoadbalancerRequest{
			Body: &model.CreateLoadbalancerRequestBody{
				Loadbalancer: config.Conf.CloudConf.(*model.CreateLoadbalancerReq),
			},
		},
	}
	return h
}

func (h *huaweiCloud) CreateClient() (err error) {
	auth := basic.NewCredentialsBuilder().
		WithAk(*h.accessKeyId).
		WithSk(*h.accessKeySecret).
		Build()
	h.client = elb.NewElbClient(
		elb.ElbClientBuilder().
			WithRegion(region.ValueOf(*h.conf.Region)).
			WithCredential(auth).
			Build(),
	)

	return nil
}

func (h *huaweiCloud) Create() (string, error) {
	var request = *h.request
	request.Body.Loadbalancer.Name = tea.String(fmt.Sprintf("%s-%d", *h.request.Body.Loadbalancer.Name, time.Now().Unix()))
	var loadBalancerId *string
	fn := func() error {
		resp, err := h.client.CreateLoadbalancer(&request)
		if err != nil {
			return err
		}
		logrus.Info(resp.String())
		loadBalancerId = &resp.Loadbalancer.Id
		return nil
	}
	err := utils.Retry(3, "创建阿里云SLB失败", fn)
	if err != nil {
		return *loadBalancerId, err
	}
	return *loadBalancerId, nil
}

func (h *huaweiCloud) Delete(id string) error { return nil }

func (h *huaweiCloud) Describe(id string) error { return nil }

func (h *huaweiCloud) Annotation(id string, annotation map[string]string) {
	annotation["kubernetes.io/elb.subnet-id"] = id
}

func (h *huaweiCloud) CheckAnnotation(annotation map[string]string) bool {
	if _, ok := annotation["kubernetes.io/elb.subnet-id"]; ok {
		if annotation["kubernetes.io/elb.subnet-id"] != "" {
			return true
		}
	}
	return false
}
