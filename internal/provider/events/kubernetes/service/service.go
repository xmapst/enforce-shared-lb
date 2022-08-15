package service

import (
	"context"
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/model"
	"enforce-shared-lb/internal/provider"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Service struct {
	// client kubernetes api 客户端
	client *kubernetes.Clientset
	// cancelFunc 取消函数
	cancelFunc context.CancelFunc
	// context 上下文
	context context.Context
}

func New() {
	if config.KubeClient == nil {
		return
	}
	provider.RegisterEvent(&Service{})
}

func (s *Service) Name() string {
	return "service"
}

func (s *Service) Init() (err error) {
	s.client = config.KubeClient
	s.context, s.cancelFunc = context.WithCancel(context.Background())
	return nil
}

func (s *Service) StartWatch(event chan model.Event) error {
	watcher, err := s.client.CoreV1().Services(metav1.NamespaceAll).Watch(s.context, metav1.ListOptions{})
	if err != nil {
		return err
	}
	// watch for changes to the service
	for {
		select {
		case <-s.context.Done():
			watcher.Stop()
			return nil
		case res := <-watcher.ResultChan():
			service, ok := res.Object.(*corev1.Service)
			if !ok {
				continue
			}
			event <- model.Event{
				BindType:  model.Service,
				EventType: string(res.Type),
				Project:   service.Namespace,
				Data:      service,
			}
		}
	}
}

func (s *Service) Close() {
	if s.cancelFunc != nil {
		s.cancelFunc()
	}
}
