package provider

import "enforce-shared-lb/internal/model"

// EventsService interface
type EventsService interface {
	Name() string
	// Init 初始化
	Init() error
	// StartWatch 开始监听
	StartWatch(chan model.Event) error
	// Close 关闭
	Close()
}

// EventInterface Events interface
type EventInterface EventsService

var EventList []EventInterface

func RegisterEvent(eventInterface EventInterface) {
	EventList = append(EventList, eventInterface)
}

// LoadBalancerService interface
type LoadBalancerService interface {
	// CreateClient 创建sdk client
	CreateClient() error
	// Create 创建负载均衡器
	Create() (string, error)
	// Delete 删除负载均衡器
	Delete(loadBalancerId string) error
	// Describe 查询
	Describe(loadBalancerId string) error
	// Annotation 绑定注解
	Annotation(string, map[string]string)
	// CheckAnnotation 检查注解是否已存在
	CheckAnnotation(map[string]string) bool
}

// LoadBalancerInterface LoadBalancer interface
type LoadBalancerInterface LoadBalancerService
