package main

import (
	"context"
	"enforce-shared-lb/internal/api"
	"enforce-shared-lb/internal/cache"
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/model"
	"enforce-shared-lb/internal/processor"
	"enforce-shared-lb/internal/provider/events"
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"net/http"
	"os"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"
)

var (
	eventCh    chan model.Event
	cancelFunc context.CancelFunc
	ctx        context.Context
	server     *http.Server
)

func init() {
	// init logrus
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetReportCaller(true)
	logrus.SetFormatter(&logrus.TextFormatter{
		TimestampFormat: time.RFC3339,
		DisableColors:   true,
		CallerPrettyfier: func(frame *runtime.Frame) (function string, file string) {
			file = fmt.Sprintf("%s:%d", path.Base(frame.File), frame.Line)
			_f := strings.Split(frame.Function, ".")
			function = _f[len(_f)-1]
			return
		},
	})
	// register signal handlers
	registerSignalHandlers()
}

func main() {
	kingpin.Parse()
	// load config
	config.Init()
	cache.New(config.RedisCli, config.Conf.KeyPrefix, config.Conf.Cloud.Max)
	router := api.Router()
	// init events
	events.Init(router)
	eventCh = make(chan model.Event, config.Conf.ChannelSize)
	ctx, cancelFunc = context.WithCancel(context.Background())
	// run event consumer
	logrus.Infoln("start event consumer")
	err := processor.Consumer(ctx, eventCh)
	if err != nil {
		logrus.Fatalln(err)
	}
	// run event producer
	logrus.Infoln("start event producer")
	events.Start(eventCh)
	// start http server
	server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Conf.Addr, config.Conf.Port),
		WriteTimeout: time.Second * 180,
		ReadTimeout:  time.Second * 180,
		IdleTimeout:  time.Second * 180,
		Handler:      router,
	}
	logrus.Infof("start http server, listen %s", server.Addr)
	err = server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		logrus.Error(err)
	}
}

func registerSignalHandlers() {
	logrus.Infoln("register signal handlers")
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-sigs
		logrus.Infoln("received signal, exiting...")
		if server != nil {
			// 关闭http server
			shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second*15)
			defer cancel()
			err := server.Shutdown(shutdownCtx)
			if err != nil {
				logrus.Error(err)
			}
		}
		// 关闭事件接收器
		events.Close()
		// 关闭事件消费者
		cancelFunc()
		close(eventCh)
		// 关闭redis连接
		_ = config.RedisCli.Close()
		os.Exit(0)
	}()
}
