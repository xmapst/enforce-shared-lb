package service

import (
	"context"
	"enforce-shared-lb/internal/cache"
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/model"
	"enforce-shared-lb/internal/provider"
	"github.com/avast/retry-go/v4"
	"strconv"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

type Service struct {
	LB     provider.LoadBalancerInterface
	conf   *config.Configure
	client *kubernetes.Clientset
	GLock  map[string]*redsync.Mutex
}

func New() *Service {
	s := &Service{
		conf:   config.Conf,
		client: config.KubeClient,
		GLock:  map[string]*redsync.Mutex{},
	}
	return s
}

func (s *Service) RetryProcess(retryObjCh chan model.Event, obj model.Event) {
	err := retry.Do(
		func() (err error) {
			return s.Process(obj)
		},
		retry.Attempts(3),
		retry.DelayType(func(n uint, err error, config *retry.Config) time.Duration {
			max := time.Duration(n)
			if max > 8 {
				max = 8
			}
			duration := time.Second * max * max
			logrus.Errorf("apply service error: %s, %d retry in %s", err, n+1, duration)
			return duration
		}),
		retry.MaxDelay(time.Second*64),
	)
	if err != nil {
		retryObjCh <- obj
	}
}

func (s *Service) Process(obj model.Event) error {
	service, ok := obj.Data.(*corev1.Service)
	if !ok {
		logrus.Warning("data type is not corev1.Service")
		return nil
	}

	log := logrus.WithFields(logrus.Fields{
		"namespace":    service.Namespace,
		"name":         service.Name,
		"service_ip":   service.Spec.ClusterIP,
		"service_type": service.Spec.Type,
	})

	// skip service without label
	if s.skipLabel(service.Labels) {
		return nil
	}

	for k, v := range service.Spec.Ports {
		if v.Name == "" {
			service.Spec.Ports[k].Name = strconv.Itoa(int(v.Port))
		}
	}

	// 加锁处理
	// die waiting for lock
	for {
		err := s.GLock[service.Namespace].Lock()
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// unlock
	defer func(lock *redsync.Mutex) {
		_, err := lock.Unlock()
		if err != nil {
			log.Warning(err)
		}
	}(s.GLock[service.Namespace])

	switch obj.EventType {
	case model.EventTypeAdded, model.EventTypeModified:
		if s.skipService(service) {
			return nil
		}

		err := cache.DB.AddProject(service.Namespace)
		if err != nil {
			log.Warning(err)
		}

		// enable target port
		enTargetPort := s.getEnableTargetPort(service)

		// check service if exist from cache
		exist := s.checkExist(service, enTargetPort)
		if exist {
			return nil
		}

		// 获取可用LB
		var num = int64(len(service.Spec.Ports))
		var protocol = string(service.Spec.Ports[0].Protocol)
		var id string
		id, err = cache.DB.GetAvailableLoadBalancer(service.Namespace, num)
		if err != nil {
			log.Error(err)
			return err
		}

		// 获取新的LoadBalancer
		if id == "" {
			id, err = s.getNewLoadBalancer(service.Namespace)
			if err != nil {
				log.Error(err)
				return err
			}
		}

		// 获取已经使用的端口
		var cachePorts []cache.Port
		cachePorts, err = cache.DB.GetLoadBalancerUsingPorts(service.Namespace, id, protocol)
		if err != nil {
			log.Error(err)
			return err
		}

		// 对比ports和service.Spec.Ports
		backendPorts := s.translatePort(service.Spec.Ports)
		newPorts := cache.ComparePorts(cachePorts, backendPorts)
		service.Spec.Ports = s.translateServicePort(service.Spec.Ports, newPorts, enTargetPort)

		var usingPorts []cache.Port
		for _, v := range service.Spec.Ports {
			usingPorts = append(usingPorts, cache.Port{
				Name:       v.Name,
				Port:       v.Port,
				TargetPort: v.TargetPort.IntVal,
				Protocol:   string(v.Protocol),
			})
		}
		// 添加到到已使用集合中
		err = cache.DB.SetLoadBalancerUsingPorts(service.Namespace, id, protocol, usingPorts)
		if err != nil {
			log.Warning(err)
		}
		// 添加到后端集合
		err = cache.DB.AddBackend(service.Namespace, service.Name, id)
		if err != nil {
			log.Warning(err)
		}
		// 添加到到已使用集合中
		err = cache.DB.SetBackend(service.Namespace, service.Name, usingPorts)
		if err != nil {
			log.Warning(err)
		}
		// 增加使用量
		err = cache.DB.SetLoadBalancerAmount(service.Namespace, id, -num)
		if err != nil {
			log.Error(err)
			return err
		}
		// 应用到service
		return s.applyService(id, service)
	case model.EventTypeDeleted:
		var ports []cache.Port
		for _, v := range service.Spec.Ports {
			ports = append(ports, cache.Port{
				Name:       v.Name,
				Protocol:   string(v.Protocol),
				Port:       v.Port,
				TargetPort: v.TargetPort.IntVal,
			})
		}
		return cache.DB.Clean(service.Namespace, service.Name, ports)
	}
	return nil
}

func (s *Service) getNewLoadBalancer(project string) (string, error) {
	id, err := s.LB.Create()
	if err != nil {
		return "", err
	}
	// 修改可用数量
	err = cache.DB.SetLoadBalancerAmount(project, id, 0)
	if err != nil {
		return "", err
	}
	// 添加到到已使用集合中
	err = cache.DB.SetLoadBalancerUsingPorts(project, id, "UNKNOWN", nil)
	if err != nil {
		return "", err
	}
	// 添加到后端集合
	err = cache.DB.AddBackend(project, id, id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) checkExist(service *corev1.Service, enTargetPort bool) bool {
	log := logrus.WithFields(logrus.Fields{
		"namespace":    service.Namespace,
		"name":         service.Name,
		"service_ip":   service.Spec.ClusterIP,
		"service_type": service.Spec.Type,
	})
	// 获取当前使用的端口集合
	id, ports := cache.DB.GetBackendPorts(service.Namespace, service.Name)
	if ports != nil {
		service.Spec.Ports = s.translateServicePort(service.Spec.Ports, ports, enTargetPort)
		err := s.applyService(id, service)
		if err != nil {
			log.Error(err)
			return false
		}
		return true
	}
	return false
}

func (s *Service) translatePort(servicePort []corev1.ServicePort) (result []cache.Port) {
	for _, port := range servicePort {
		result = append(result, cache.Port{
			Name:       port.Name,
			Protocol:   string(port.Protocol),
			Port:       port.Port,
			TargetPort: port.TargetPort.IntVal,
		})
	}
	return result
}

func (s *Service) translateServicePort(servicePort []corev1.ServicePort, ports []cache.Port, targetPort bool) (result []corev1.ServicePort) {
	for _, port := range servicePort {
		for _, p := range ports {
			if port.Name == p.Name {
				res := corev1.ServicePort{
					Name:       port.Name,
					Protocol:   port.Protocol,
					Port:       p.Port,
					TargetPort: port.TargetPort,
				}
				if targetPort {
					res.TargetPort = intstr.FromInt(int(p.TargetPort))
				}
				result = append(result, res)
			}
		}
	}
	return result
}

func (s *Service) applyService(id string, service *corev1.Service) error {
	if service.Annotations == nil {
		service.Annotations = make(map[string]string)
	}
	s.LB.Annotation(id, service.Annotations)
	service.Spec.Type = corev1.ServiceTypeLoadBalancer
	service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
	_, err := s.client.CoreV1().Services(service.Namespace).Update(context.Background(), service, metav1.UpdateOptions{})
	if err != nil {
		logrus.Errorf("update service failed: %v", err)
		return err
	}
	return nil
}

func (s *Service) skipService(service *corev1.Service) bool {
	// skip services of type clusterIP
	if service.Spec.Type == corev1.ServiceTypeClusterIP {
		return false
	}

	if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
		// skip services with has externalIP
		if len(service.Spec.ExternalIPs) > 0 {
			return true
		}

		// skip services with has ingress
		if len(service.Status.LoadBalancer.Ingress) > 0 {
			return true
		}

		// skip services with has lb annotation and value is not empty
		return s.LB.CheckAnnotation(service.Annotations)
	}
	// skip services of other type
	return true
}

func (s *Service) getEnableTargetPort(service *corev1.Service) bool {
	if service.Annotations == nil {
		return false
	}
	if value, ok := service.Annotations["service.kubernetes.io/q1-enable-target_port"]; ok {
		if value == "true" {
			return true
		}
	}
	return false
}

// check if the service has labels
func (s *Service) skipLabel(labels map[string]string) bool {
	if labels == nil {
		return true
	}
	var skip bool
	for key, value := range s.conf.Labels {
		v, ok := labels[key]
		if !ok || v != value {
			skip = true
			continue
		}
		skip = false
	}
	return skip
}
