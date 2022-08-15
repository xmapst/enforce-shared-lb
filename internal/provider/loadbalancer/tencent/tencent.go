package tencent

import (
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/provider"
	"enforce-shared-lb/internal/utils"
	"fmt"
	"time"

	"github.com/alibabacloud-go/tea/tea"
	"github.com/sirupsen/logrus"
	clb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

type tencentCloud struct {
	client          *clb.Client
	endpoint        *string
	accessKeyId     *string
	accessKeySecret *string
	conf            *config.TencentConf
	request         *clb.CreateLoadBalancerRequest
}

func New() provider.LoadBalancerInterface {
	t := &tencentCloud{
		endpoint:        config.Conf.Cloud.Endpoint,
		accessKeyId:     config.Conf.Cloud.AccessKeySecret,
		accessKeySecret: config.Conf.Cloud.AccessKeySecret,
		conf:            config.Conf.CloudConf.(*config.TencentConf),
		request:         config.Conf.CloudConf.(*clb.CreateLoadBalancerRequest),
	}
	return t
}

func (t *tencentCloud) CreateClient() (err error) {
	credential := common.NewCredential(
		*t.accessKeyId,
		*t.accessKeySecret,
	)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = *t.endpoint
	t.client, err = clb.NewClient(credential, *t.conf.Region, cpf)
	return err
}

func (t *tencentCloud) Create() (string, error) {
	var request = *t.request
	request.LoadBalancerName = tea.String(fmt.Sprintf("%s-%d", *t.request.LoadBalancerType, time.Now().Unix()))
	var resp *clb.CreateLoadBalancerResponse
	fn := func() (err error) {
		resp, err = t.client.CreateLoadBalancer(t.request)
		if err != nil {
			logrus.Error(err)
			return err
		}
		logrus.Info(resp.ToJsonString())
		return nil
	}
	err := utils.Retry(3, "创建腾讯云CLB失败", fn)
	if err != nil {
		return "", err
	}
	// 存在某些场景，如创建出现延迟时，此字段可能返回为空
	if resp.Response.LoadBalancerIds == nil {
		resp.Response.LoadBalancerIds, err = t.DescribeTaskStatus(resp.Response.DealName)
		if err != nil {
			return "", err
		}
	}
	return *resp.Response.LoadBalancerIds[0], nil
}

func (t *tencentCloud) DescribeTaskStatus(dealName *string) (loadBalancerIds []*string, err error) {
	var resp *clb.DescribeTaskStatusResponse
	fn := func() error {
		resp, err = t.client.DescribeTaskStatus(&clb.DescribeTaskStatusRequest{
			DealName: dealName,
		})
		if err != nil {
			return err
		}
		logrus.Info(resp.ToJsonString())
		status := *resp.Response.Status
		if status == 2 {
			return fmt.Errorf("%s订单进行中", *dealName)
		}
		loadBalancerIds = resp.Response.LoadBalancerIds
		return nil
	}
	_ = utils.Retry(0, "查询腾讯CLB创建状态失败", fn)
	if *resp.Response.Status != 0 {
		return nil, fmt.Errorf("腾讯CLB创建失败, request_id=%s", *resp.Response.RequestId)
	}
	return loadBalancerIds, nil
}

func (t *tencentCloud) Delete(id string) error {
	return nil
}

func (t *tencentCloud) Describe(id string) error {
	return nil
}

func (t *tencentCloud) Annotation(id string, annotation map[string]string) {
	annotation["service.kubernetes.io/tke-existed-lbid"] = id
}

func (t *tencentCloud) CheckAnnotation(annotation map[string]string) bool {
	if _, ok := annotation["service.kubernetes.io/tke-existed-lbid"]; ok {
		if annotation["service.kubernetes.io/tke-existed-lbid"] != "" {
			return true
		}
	}
	return false
}
