package config

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/go-redis/redis/v8"
	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	RedisCli   *redis.Client
	GLock      *redsync.Redsync
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

	pool := goredis.NewPool(RedisCli)
	GLock = redsync.New(pool)
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
		f, err = ioutil.ReadFile(kubeConfPath)
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
