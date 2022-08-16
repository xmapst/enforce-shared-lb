package config

import (
	"enforce-shared-lb/internal/utils"
	aliSlb "github.com/alibabacloud-go/slb-20140515/v3/client"
	huaweiElb "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v2/model"
	"github.com/sirupsen/logrus"
	tencentClb "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/clb/v20180317"
	"strings"
)

func (c *Configure) loadCloudConf() {
	if c.Labels == nil {
		c.Labels = map[string]string{
			"lb_address_type": "internet",
			"q1autoops_type":  "game-service",
		}
	}
	if strings.HasSuffix(c.KeyPrefix, ":") {
		c.KeyPrefix = strings.TrimSuffix(c.KeyPrefix, ":")
	}

	var ok bool
	c.CloudConf, ok = CloudConf[c.Cloud.Name]
	if !ok {
		logrus.Fatalf("%s Cloud Merchant is not supported yet", c.Cloud.Name)
	}
	err := utils.Json.Unmarshal(c.Cloud.Config, c.CloudConf)
	if err != nil {
		logrus.Fatalln(err)
	}
}

const (
	FakeCloud    = "fake"
	AlibabaCloud = "alibaba"
	HuaweiCloud  = "huawei"
	TencentCloud = "tencent"
)

var CloudConf = map[string]interface{}{
	FakeCloud:    new(AlibabaConf),
	AlibabaCloud: new(AlibabaConf),
	HuaweiCloud:  new(HuaweiConf),
	TencentCloud: new(TencentConf),
}

type AlibabaConf struct {
	aliSlb.CreateLoadBalancerRequest
}

type HuaweiConf struct {
	huaweiElb.CreateLoadbalancerReq
	Region    *string `json:"region,omitempty"`
	ProjectId *string `json:"project_id,omitempty"`
}

type TencentConf struct {
	tencentClb.CreateLoadBalancerRequest
	Region *string `json:"region,omitempty"`
}
