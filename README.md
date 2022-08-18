# enforce-shared-lb

事因: 由于各云商对负载均衡器数量有所限制, 在k8s应用中, 有大批量的tcp需对外的服务时, 每个service对应一个负载均衡器是不现实的, 且造成浪费以费用高

基于云版本的k8s和负载均衡器特性, 自动强制为 `match` `labels` 的 `k8s` 集群 `CLusterIP` 类型的 `Service` 切换为使用共享负载均衡的 `Loadbalancer`

[算法](https://gist.github.com/xmapst/7d41516a644acd900bb3096573947e58): 保证同一个负载均衡下端口不冲突, 不同service使用相同的负载均衡器时, 如有端口冲突会自动处理为新的唯一端口

## 配置文件(以阿里云为例)

```json
{
  "addr": "0.0.0.0",
  "port": 8080,
  "channel_size": 1024,
  "redis": "redis://:123456@localhost:6379/0?pool_size=512&read_timeout=30s&write_timeout=30s&min_idle_conns=15",
  "key_prefix": "enforce_shared_lb",
  "labels": {
    "lb_address_type":"internet",
    "q1autoops_type":"game-service"
  },
  "cloud": {
    "name": "alibaba",
    "max": 51,
    "endpoint": "slb.aliyuncs.com",
    "access_key_id": "xxxxxxxxxxxx",
    "access_key_secret": "xxxxxxxxxxxxxxxxxxx",
    "config": {
      "RegionId": "cn-hangzhou",
      "AddressType": "internet",
      "InternetChargeType": "paybytraffic",
      "Bandwidth": 10,
      "LoadBalancerName": "lb-bp1o94dp5i6ea****",
      "VpcId": "vpc-bp1aevy8sofi8mh1****",
      "VSwitchId": "vsw-bp12mw1f8k3jgy****",
      "MasterZoneId": "cn-hangzhou-b",
      "SlaveZoneId": "cn-hangzhou-d",
      "LoadBalancerSpec": "slb.s1.small",
      "ResourceGroupId": "rg-atstuj3rtopt****",
      "PayType": "PayOnDemand",
      "PricingCycle": "month",
      "Duration": 1,
      "AutoPay": true,
      "AddressIPVersion": "ipv4",
      "Address": "192.168.XX.XX",
      "DeleteProtection": "on",
      "ModificationProtectionStatus": "ConsoleProtection",
      "ModificationProtectionReason": "Managed instance",
      "InstanceChargeType": "PayBySpec"
    }
  }
}
```

## 构建镜像

```shell
docker build -t enforce-shared-lb:latest .
# docker push enforce-shared-lb:latest
```

## 部署

```shell
kubectl create -f deploy/01-rbac.yml
kubectl create -f deploy/02-deployment.tml
kubectl create -f deploy/03-service.yml
```

## 唯一端口处理算法

基于同一个负载均衡器下的. 当然不同负载均衡器下是可以使用相同端口的, 举例:

+ A负载均衡器: [80, 81, 443, 444]
+ B负载均衡器: [80, 81, 443, 444]

这种情况是可以

### 情况一

labels匹配的新service的端口已存在使用中的, 此时需要运算出一个新端口给此service使用

```txt
使用中: [22, 23, 80, 443]
新service: [80, 443]
得到--->
新使用中列表: [22, 23, 80, 81, 443, 444]
新service使用: [81, 444]
```

### 情况二

labels匹配的新service的端口不存在使用中的, 此时不需要运算, 此service直接使用

```txt
使用中: [22, 23, 80, 443]
新service: [25, 8000]
得到--->
新使用中列表: [22, 23, 25, 80, 443, 8000]
新service使用: [25, 8000]
```

### 情况三

labels匹配的新service的端口中既有存在也有不存在使用中的, 此时需要将已存在运算出一个新的端口, 且该端口不能与当前service端口中不存在的冲突

```txt
使用中: [22, 23, 80, 443]
新service: [23, 24]
得到--->
新使用中列表: [22, 23, 24, 25, 80, 443]
新service使用: [25, 24]
```
