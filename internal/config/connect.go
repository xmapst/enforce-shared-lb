package config

import (
	"context"
	"github.com/go-redis/redis/v8"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
)

var (
	RedisCli   *redis.Client
	KubeClient *kubernetes.Clientset
)

// newRedisClient new a redis client
func (c *Configure) newRedisClient() error {
	opt, err := redis.ParseURL(c.Redis)
	if err != nil {
		return err
	}

	RedisCli = redis.NewClient(opt)
	_, err = RedisCli.Ping(context.Background()).Result()
	if err != nil {
		return err
	}
	return nil
}

// newKubeClient new a kubernetes client
func (c *Configure) newKubeClient() error {
	var kubeConfig *rest.Config
	var err error
	// debug 模式
	if c.Debug {
		kubeConfPath := os.Getenv("KUBECONFIG")
		if kubeConfPath == "" {
			kubeConfPath = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
		var f []byte
		f, err = os.ReadFile(kubeConfPath)
		if err != nil {
			return err
		}
		kubeConfig, err = clientcmd.RESTConfigFromKubeConfig(f)
		if err != nil {
			return err
		}
	} else {
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			return err
		}
	}

	// configure kubernetes client burst, qps, timeout parameter
	kubeConfig.Burst = 1000
	kubeConfig.QPS = 500
	kubeConfig.Timeout = 0

	// new kubernetes client
	KubeClient, err = kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}
	return nil
}
