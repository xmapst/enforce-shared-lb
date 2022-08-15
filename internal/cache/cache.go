package cache

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type Cache struct {
	client           *redis.Client
	lock             *sync.Mutex
	keyPrefix        string
	maxNumOfBackends int64
}

var DB *Cache

func New(client *redis.Client, keyPrefix string, maxNumOfBackends int64) {
	DB = &Cache{
		client:           client,
		keyPrefix:        keyPrefix,
		maxNumOfBackends: maxNumOfBackends - 1,
		lock:             new(sync.Mutex),
	}
	go DB.autoClean(30)
}

/*
// 存项目名称, 使用无序集合
KEY: <prefix>:project
VAL: <project>

// 存后端名称，使用hash
KEY: <prefix>:<project>:backend
FILED: <name>
VAL: <LoadBalancerID>

// 存后端具体使用哪个slb以及端口相关信息, 使用无序集合
KEY: <prefix>:<project>:backend:<name>
VAL: <name>#<port>#<protocol>#<target_port>

// 存SLB的端口使用数量, 使用有序集合, 使用打分计算
KEY: <prefix>:<project>:loadbalancer:amount
VAL: <LoadBalancerID>
SCORE: <剩余量>

// 存SLB端口唯一, 使用无序集合
KEY: <prefix>:<project>:loadbalancer:<LoadBalancerID>:<protocol>
VAL: <port>
*/

func (c *Cache) ListProject() (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = fmt.Sprintf("%s:project", c.keyPrefix)
	return c.client.SMembers(context.Background(), key).Result()
}

func (c *Cache) AddProject(project string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = fmt.Sprintf("%s:project", c.keyPrefix)
	return c.client.SAdd(context.Background(), key, project).Err()
}

func (c *Cache) ListLoadBalancerAmount(project string) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project, "amount")
	members, err := c.client.ZRange(context.Background(), key, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	var data = make(map[string]float64)
	for _, v := range members {
		data[v] = c.client.ZScore(context.Background(), key, v).Val()
	}
	return data, nil

}

func (c *Cache) DeleteLoadBalancer(project, id string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project)
	return c.client.ZRem(context.Background(), key, id).Err()
}

func (c *Cache) ListLoadBalancer(project, id string) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project, id, "*")
	var cursor uint64
	var data []string
	for {
		var keys []string
		var err error
		keys, cursor, err = c.client.Scan(context.Background(), cursor, key, 1000).Result()
		if err != nil {
			logrus.Error(err)
			continue
		}
		for _, v := range keys {
			s := strings.Split(v, ":")
			data = append(data, s[len(s)-1])
		}
		if cursor == 0 {
			break
		}
	}

	return data, nil
}

func (c *Cache) DetailLoadBalancer(project, id, protocol string) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project, id, protocol)
	return c.client.SMembers(context.Background(), key).Result()
}

func (c *Cache) AddBackend(project, name, id string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project)
	return c.client.HSet(context.Background(), key, name, id).Err()
}

func (c *Cache) ListBackend(project string) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project)
	res, err := c.client.HGetAll(context.Background(), key).Result()
	if err != nil {
		return nil, err
	}
	for k, v := range res {
		if k == v {
			delete(res, k)
		}
	}
	return res, nil
}

type Port struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort int32  `json:"target_port"`
}

func (c *Cache) DetailBackend(project, name string) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project, name)
	members, err := c.client.SMembers(context.Background(), key).Result()
	if err != nil {
		return nil, err
	}
	var data []Port
	for _, v := range members {
		port, err := c.splitBackendPort(v)
		if err != nil {
			_ = c.client.SRem(context.Background(), key, v).Err()
		}
		data = append(data, *port)
	}
	return data, nil
}

func (c *Cache) splitBackendPort(str string) (*Port, error) {
	slice := strings.Split(str, "#")
	if len(slice) != 4 {
		return nil, fmt.Errorf("illegal data")
	}
	port, err := strconv.Atoi(slice[1])
	if err != nil {
		return nil, err
	}
	targetPort, err := strconv.Atoi(slice[3])
	if err != nil {
		return nil, err
	}
	return &Port{
		Name:       slice[0],
		Port:       int32(port),
		Protocol:   slice[2],
		TargetPort: int32(targetPort),
	}, nil
}

func (c *Cache) GetAvailableLoadBalancer(project string, num int64) (string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	// 获取分数范围内元素
	var key = c.loadBalancerKey(project, "amount")
	var op = &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", num),
		Max: fmt.Sprintf("%d", c.maxNumOfBackends),
	}
	res, err := c.client.ZRangeByScore(context.Background(), key, op).Result()
	if err != nil {
		return "", err
	}
	if len(res) <= 0 {
		return "", nil
	}
	// 取第一个
	return res[0], nil
}

func (c *Cache) SetLoadBalancerAmount(project, id string, increment int64) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	key := c.loadBalancerKey(project, "amount")
	if increment == 0 {
		// 设置初始分数
		return c.client.ZAdd(context.Background(), key, &redis.Z{
			Member: id,
			Score:  float64(c.maxNumOfBackends),
		}).Err()
	}
	return c.client.ZIncrBy(context.Background(), key, float64(increment), id).Err()
}

func (c *Cache) SetLoadBalancerUsingPorts(project, id, protocol string, ports []Port) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project, id, protocol)
	var members []interface{}
	if ports == nil {
		members = []interface{}{0}
	} else {
		for _, v := range ports {
			members = append(members, v.Port)
		}
	}
	// 添加成员
	return c.client.SAdd(context.Background(), key, members...).Err()
}

func (c *Cache) GetLoadBalancerUsingPorts(project, id, protocol string) ([]Port, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project, id, protocol)
	// 获取所有成员
	res, err := c.client.SMembers(context.Background(), key).Result()
	if err != nil {
		return nil, err
	}
	var ports []Port
	for _, v := range res {
		p, _ := strconv.ParseInt(v, 10, 64)
		ports = append(ports, Port{
			Port: int32(p),
		})
	}
	return ports, nil
}

func (c *Cache) GetBackendPorts(project, name string) (loadBalancerID string, res []Port) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project)
	var err error
	loadBalancerID, err = c.client.HGet(context.Background(), key, name).Result()
	if err != nil {
		if err != redis.Nil {
			logrus.Error(err)
		}
		return "", nil
	}
	key = c.backendKey(project, name)
	members, err := c.client.SMembers(context.Background(), key).Result()
	if err != nil {
		if err != redis.Nil {
			logrus.Error(err)
		}
		return "", nil
	}
	for _, v := range members {
		port, err := c.splitBackendPort(v)
		if err != nil {
			_ = c.client.HDel(context.Background(), key, v).Err()
			continue
		}
		res = append(res, *port)
	}
	return
}

func (c *Cache) SetBackend(project, name string, ports []Port) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project, name)
	for _, p := range ports {
		member := fmt.Sprintf("%s#%d#%s#%d", p.Name, p.Port, p.Protocol, p.TargetPort)
		err := c.client.SAdd(context.Background(), key, member).Err()
		if err != nil && err != redis.Nil {
			logrus.Error(err)
			return err
		}
	}
	return nil
}

func (c *Cache) Clean(project, name string, ports []Port) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	id, err := c.client.HGet(context.Background(), c.backendKey(project), name).Result()
	if err != nil && err != redis.Nil {
		logrus.Error(err)
		return err
	}

	// 删除缓存中后端
	err = c.client.Del(context.Background(), c.backendKey(project, name)).Err()
	if err != nil && err != redis.Nil {
		logrus.Error(err)
		return err
	}

	// 清理端口使用
	for _, v := range ports {
		err = c.client.SRem(context.Background(), c.loadBalancerKey(project, id, v.Protocol), v.Port).Err()
		if err != nil && err != redis.Nil {
			logrus.Warning(err)
		}
	}

	// 增加load balancer分数
	if id != "" {
		var increment = float64(int64(len(ports)))
		err = c.client.ZIncrBy(context.Background(), c.loadBalancerKey(project, "amount"), increment, id).Err()
		if err != nil && err != redis.Nil {
			logrus.Error(err)
			return err
		}
	}

	// 清理后端集合中成员
	err = c.client.HDel(context.Background(), c.backendKey(project), name).Err()
	if err != nil && err != redis.Nil {
		logrus.Error(err)
		return err
	}
	return nil
}

func (c *Cache) loadBalancerKey(project string, key ...string) string {
	return c.generateKey(project, "loadbalancer", key...)
}

func (c *Cache) backendKey(project string, key ...string) string {
	return c.generateKey(project, "backend", key...)
}

func (c *Cache) generateKey(project, style string, key ...string) string {
	if len(key) == 0 {
		return fmt.Sprintf("%s:%s:%s", c.keyPrefix, project, style)
	}
	return fmt.Sprintf("%s:%s:%s:%s", c.keyPrefix, project, style, strings.Join(key, ":"))
}

func (c *Cache) autoClean(interval int) {
	ticker := time.Tick(time.Duration(interval) * time.Second)
	for range ticker {
		c.cleanProject()
	}
}

func (c *Cache) cleanProject() {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = fmt.Sprintf("%s:project", c.keyPrefix)
	members, err := c.client.SMembers(context.Background(), key).Result()
	if err != nil {
		logrus.Warning(err)
		return
	}
	for _, member := range members {
		res := c.client.Exists(context.Background(), fmt.Sprintf("%s:%s:loadbalancer:amount", c.keyPrefix, member)).Val()
		if res == 1 {
			c.cleanBackendLoadBalancer(member)
			continue
		}
		err = c.client.SRem(context.Background(), key, member).Err()
		if err != nil {
			logrus.Warning(err)
		}
	}
}

func (c *Cache) cleanBackendLoadBalancer(project string) {
	//enforce_shared_lb:default:backend
	var key = c.backendKey(project)
	members, err := c.client.HGetAll(context.Background(), key).Result()
	if err != nil {
		logrus.Warning(err)
		return
	}
	for _, member := range members {
		err = c.client.ZScore(context.Background(), c.loadBalancerKey(project, "amount"), member).Err()
		if err == redis.Nil {
			err = c.client.HDel(context.Background(), key, member).Err()
			if err != nil && err != redis.Nil {
				logrus.Warning(err)
			}
		}
	}
}
