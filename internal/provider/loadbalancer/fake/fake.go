package fake

import (
	"enforce-shared-lb/internal/provider"

	"github.com/google/uuid"
)

type fake struct{}

func New() provider.LoadBalancerInterface {
	return &fake{}
}

func (f *fake) CreateClient() error {
	return nil
}

func (f *fake) Create() (string, error) { return uuid.New().String(), nil }

func (f *fake) Delete(id string) error { return nil }

func (f *fake) Describe(id string) error { return nil }

func (f *fake) Annotation(id string, annotation map[string]string) {
	annotation["service.kubernetes.io/fake-cloud-loadbalancer-id"] = id
}

func (f *fake) CheckAnnotation(annotation map[string]string) bool {
	if _, ok := annotation["service.kubernetes.io/fake-cloud-loadbalancer-id"]; ok {
		if annotation["service.kubernetes.io/fake-cloud-loadbalancer-id"] != "" {
			return true
		}
	}
	return false
}
