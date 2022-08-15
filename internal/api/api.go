package api

import (
	"bytes"
	"encoding/json"
	"enforce-shared-lb/internal/cache"
	"enforce-shared-lb/internal/utils"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type baseUri struct {
	Project string `uri:"project" binding:"required"`
}

type loadBalancerUri struct {
	baseUri
	ID string `uri:"id" binding:"required"`
}

type detailLoadBalancerUri struct {
	loadBalancerUri
	Protocol string `uri:"protocol" binding:"required"`
}

type backendUri struct {
	baseUri
	Name string `uri:"name" binding:"required"`
}

func Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(cors.Default(), gin.Recovery(), logger())
	r.Any("/", func(c *gin.Context) {
		routers := r.Routes()
		var paths []string
		for _, v := range routers {
			if v.Path == "/" {
				continue
			}
			paths = append(paths, fmt.Sprintf(`%s: <a href="%s">%s</a>`, v.Method, v.Path, v.Path))
		}
		data := strings.Join(paths, "<br />")
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(data))
	})
	r.GET("/health", func(c *gin.Context) {
		c.SecureJSON(http.StatusOK, utils.Response(http.StatusOK, nil, "running"))
	})
	api := r.Group("/api")
	{
		api.GET("project", func(c *gin.Context) {
			response(c, func() (interface{}, error) {
				return cache.DB.ListProject()
			})
		})
		api.GET(":project/loadbalancer", func(c *gin.Context) {
			var query baseUri
			err := c.ShouldBindUri(&query)
			if err != nil {
				c.SecureJSON(http.StatusOK, utils.Response(http.StatusBadRequest, nil, err.Error()))
				return
			}
			response(c, func() (interface{}, error) {
				return cache.DB.ListLoadBalancerAmount(query.Project)
			})
		})
		api.GET(":project/loadbalancer/:id", func(c *gin.Context) {
			var query loadBalancerUri
			err := c.ShouldBindUri(&query)
			if err != nil {
				c.SecureJSON(http.StatusOK, utils.Response(http.StatusBadRequest, nil, err.Error()))
				return
			}
			response(c, func() (interface{}, error) {
				return cache.DB.ListLoadBalancer(query.Project, query.ID)
			})
		})
		api.GET(":project/loadbalancer/:id/:protocol", func(c *gin.Context) {
			var query detailLoadBalancerUri
			err := c.ShouldBindUri(&query)
			if err != nil {
				c.SecureJSON(http.StatusOK, utils.Response(http.StatusBadRequest, nil, err.Error()))
				return
			}
			response(c, func() (interface{}, error) {
				return cache.DB.DetailLoadBalancer(query.Project, query.ID, query.Protocol)
			})
		})
		api.GET(":project/backend", func(c *gin.Context) {
			var query baseUri
			err := c.ShouldBindUri(&query)
			if err != nil {
				c.SecureJSON(http.StatusOK, utils.Response(http.StatusBadRequest, nil, err.Error()))
				return
			}
			response(c, func() (interface{}, error) {
				return cache.DB.ListBackend(query.Project)
			})
		})
		api.GET(":project/backend/:name", func(c *gin.Context) {
			var query backendUri
			err := c.ShouldBindUri(&query)
			if err != nil {
				c.SecureJSON(http.StatusOK, utils.Response(http.StatusBadRequest, nil, err.Error()))
				return
			}
			response(c, func() (interface{}, error) {
				return cache.DB.DetailBackend(query.Project, query.Name)
			})
		})
	}
	return r
}

func response(c *gin.Context, fn func() (interface{}, error)) {
	var ws *websocket.Conn
	if websocket.IsWebSocketUpgrade(c.Request) {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logrus.Error(err)
			c.SecureJSON(http.StatusOK, utils.Response(http.StatusBadRequest, nil, err))
			return
		}
	}
	buf := &bytes.Buffer{}
	for {
		buf.Reset()
		res, err := fn()
		if err != nil {
			logrus.Error(err)
			continue
		}
		if err = json.NewEncoder(buf).Encode(res); err != nil {
			logrus.Error(err)
			return
		}
		if ws == nil {
			c.SecureJSON(http.StatusOK, utils.Response(http.StatusOK, res, nil))
			return
		} else {
			err = ws.WriteMessage(websocket.TextMessage, buf.Bytes())
		}
		if err != nil {
			logrus.Error(err)
			return
		}
		time.Sleep(1 * time.Second)
	}
}

func logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// other handler can change c.Path so:
		path := c.Request.URL.Path
		start := time.Now()
		c.Next()
		stop := time.Since(start)
		latency := int(math.Ceil(float64(stop.Nanoseconds()) / 1000000.0))
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		clientUserAgent := c.Request.UserAgent()
		referer := c.Request.Referer()
		method := c.Request.Method
		dataLength := c.Writer.Size()
		if dataLength < 0 {
			dataLength = 0
		}

		entry := logrus.WithFields(logrus.Fields{
			"status_code": statusCode,
			"latency":     latency, // time to process
			"client_ip":   clientIP,
			"method":      method,
			"path":        path,
			"referer":     referer,
			"length":      dataLength,
			"user_agent":  clientUserAgent,
		})

		if len(c.Errors) > 0 {
			entry.Error(c.Errors.ByType(gin.ErrorTypePrivate).String())
		} else {
			entry.Info("none")
		}
	}
}
