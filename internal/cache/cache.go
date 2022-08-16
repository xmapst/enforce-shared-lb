package cache

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Cache struct {
	client           *redis.Client
	lock             *sync.Mutex
	keyPrefix        string
	maxNumOfBackends int64
	ctx              context.Context
}

var DB *Cache

func New(client *redis.Client, keyPrefix string, maxNumOfBackends int64) {
	DB = &Cache{
		client:           client,
		keyPrefix:        keyPrefix,
		maxNumOfBackends: maxNumOfBackends - 1,
		lock:             new(sync.Mutex),
		ctx:              context.Background(),
	}
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
	return c.client.SMembers(c.ctx, key).Result()
}

func (c *Cache) AddProject(project string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = fmt.Sprintf("%s:project", c.keyPrefix)
	return c.client.SAdd(c.ctx, key, project).Err()
}

func (c *Cache) ListLoadBalancerAmount(project string) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project, "amount")
	members, err := c.client.ZRange(c.ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	var data = make(map[string]float64)
	for _, v := range members {
		data[v] = c.client.ZScore(c.ctx, key, v).Val()
	}
	return data, nil

}

func (c *Cache) DeleteLoadBalancer(project, id string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project)
	return c.client.ZRem(c.ctx, key, id).Err()
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
		keys, cursor, err = c.client.Scan(c.ctx, cursor, key, 1000).Result()
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
	return c.client.SMembers(c.ctx, key).Result()
}

func (c *Cache) AddBackend(project, name, id string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project)
	return c.client.HSet(c.ctx, key, name, id).Err()
}

func (c *Cache) ListBackend(project string) (interface{}, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project)
	res, err := c.client.HGetAll(c.ctx, key).Result()
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
	members, err := c.client.SMembers(c.ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var data []Port
	for _, v := range members {
		port, err := c.splitBackendPort(v)
		if err != nil {
			_ = c.client.SRem(c.ctx, key, v).Err()
			continue
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
	res, err := c.client.ZRangeByScore(c.ctx, key, op).Result()
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
		return c.client.ZAdd(c.ctx, key, &redis.Z{
			Member: id,
			Score:  float64(c.maxNumOfBackends),
		}).Err()
	}
	return c.client.ZIncrBy(c.ctx, key, float64(increment), id).Err()
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
	return c.client.SAdd(c.ctx, key, members...).Err()
}

func (c *Cache) GetLoadBalancerUsingPorts(project, id, protocol string) ([]*Port, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.loadBalancerKey(project, id, protocol)
	// 获取所有成员
	res, err := c.client.SMembers(c.ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var ports []*Port
	for _, v := range res {
		p, _ := strconv.ParseInt(v, 10, 64)
		ports = append(ports, &Port{
			Port: int32(p),
		})
	}
	return ports, nil
}

func (c *Cache) GetBackendPorts(project, name string) (loadBalancerID string, res []*Port) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project)
	var err error
	loadBalancerID, err = c.client.HGet(c.ctx, key, name).Result()
	if err != nil {
		if err != redis.Nil {
			logrus.Error(err)
		}
		return "", nil
	}
	key = c.backendKey(project, name)
	members, err := c.client.SMembers(c.ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			logrus.Error(err)
		}
		return "", nil
	}
	for _, v := range members {
		port, err := c.splitBackendPort(v)
		if err != nil {
			_ = c.client.HDel(c.ctx, key, v).Err()
			continue
		}
		res = append(res, port)
	}
	return
}

func (c *Cache) SetBackend(project, name string, ports []Port) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	var key = c.backendKey(project, name)
	var members []interface{}
	for _, p := range ports {
		member := fmt.Sprintf("%s#%d#%s#%d", p.Name, p.Port, p.Protocol, p.TargetPort)
		members = append(members, member)
	}
	err := c.client.SAdd(c.ctx, key, members...).Err()
	if err != nil && err != redis.Nil {
		logrus.Error(err)
		return err
	}
	return nil
}

func (c *Cache) Clean(project, name string, ports []Port) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	id, err := c.client.HGet(c.ctx, c.backendKey(project), name).Result()
	if err != nil && err != redis.Nil {
		logrus.Error(err)
		return err
	}

	// 增加load balancer分数
	if id != "" {
		var increment = float64(int64(len(ports)))
		err = c.client.ZIncrBy(c.ctx, c.loadBalancerKey(project, "amount"), increment, id).Err()
		if err != nil && err != redis.Nil {
			logrus.Error(err)
			return err
		}
	}

	c.cleanPorts(project, id, ports)
	err = c.cleanBackend(project, name)
	if err != nil {
		return err
	}
	return c.cleanBackendSet(project, name)
}

func (c *Cache) cleanPorts(project, id string, ports []Port) {
	// 清理端口使用
	var wg = new(sync.WaitGroup)
	for _, v := range ports {
		wg.Add(1)
		go func(v Port) {
			defer wg.Done()
			err := c.client.SRem(c.ctx, c.loadBalancerKey(project, id, v.Protocol), v.Port).Err()
			if err != nil && err != redis.Nil {
				logrus.Warning(err)
			}
		}(v)
	}
	wg.Wait()
}

func (c *Cache) cleanBackend(project, name string) error {
	// 删除缓存中后端
	err := c.client.Del(c.ctx, c.backendKey(project, name)).Err()
	if err != nil && err != redis.Nil {
		logrus.Error(err)
		return err
	}
	return nil
}

func (c *Cache) cleanBackendSet(project, name string) error {
	// 清理后端集合中成员
	err := c.client.HDel(c.ctx, c.backendKey(project), name).Err()
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

func (c *Cache) Recycle(interval time.Duration, ch chan<- string) {
	go func() {
		ticker := time.Tick(interval * time.Second)
		for range ticker {
			go func() {
				c.lock.Lock()
				defer c.lock.Unlock()
				var key = fmt.Sprintf("%s:project", c.keyPrefix)
				members, err := c.client.SMembers(c.ctx, key).Result()
				if err != nil {
					logrus.Warning(err)
					return
				}
				var wg = new(sync.WaitGroup)
				for _, member := range members {
					wg.Add(1)
					go c.cleanBackendLoadBalancer(wg, member, ch)
				}
				wg.Wait()
			}()
		}
	}()
}

func (c *Cache) cleanBackendLoadBalancer(wg *sync.WaitGroup, project string, ch chan<- string) {
	defer wg.Done()
	var key = c.backendKey(project)
	members, err := c.client.HGetAll(c.ctx, key).Result()
	if err != nil && err != redis.Nil {
		logrus.Warning(err)
		return
	}
	for _, member := range members {
		wg.Add(1)
		go func(member string) {
			defer wg.Done()
			// check
			var cursor uint64
			var data []string
			for {
				var err error
				var keys []string
				keys, cursor, err = c.client.Scan(c.ctx, cursor, c.loadBalancerKey(project, member, "*"), 1000).Result()
				if err != nil {
					logrus.Error(err)
					continue
				}
				data = append(data, keys...)
				if cursor == 0 {
					break
				}
			}
			if len(data) != 0 {
				return
			}
			// 清理分数
			err = c.client.ZRem(c.ctx, c.loadBalancerKey(project, "amount"), member).Err()
			if err != nil && err != redis.Nil {
				logrus.Warning(err)
			}
			// 清理后端列表
			err = c.client.HDel(c.ctx, key, member).Err()
			if err != nil && err != redis.Nil {
				logrus.Warning(err)
			}
			ch <- member
		}(member)
	}
}
