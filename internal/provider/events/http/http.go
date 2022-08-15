package http

import (
	"enforce-shared-lb/internal/config"
	"enforce-shared-lb/internal/model"
	"enforce-shared-lb/internal/provider"
	"enforce-shared-lb/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
)

type Http struct {
	router *gin.Engine
}

func New(router *gin.Engine) {
	if config.Conf.Port == 0 {
		return
	}
	provider.RegisterEvent(&Http{
		router: router,
	})
}

func (h *Http) Name() string {
	return "http"
}

func (h *Http) Init() error {
	return nil
}

var eventTypeMap = map[string]string{
	http.MethodPost:   model.EventTypeAdded,
	http.MethodPut:    model.EventTypeModified,
	http.MethodDelete: model.EventTypeDeleted,
}

func (h *Http) StartWatch(event chan model.Event) error {
	logrus.Info("http endpoint /events")
	h.router.Any("/events", func(c *gin.Context) {
		c.JSON(http.StatusOK, utils.Response(http.StatusOK, nil, "TODO: http event input"))
		//value, ok := eventTypeMap[r.Method]
		//if !ok {
		//	h.render(w, http.StatusBadRequest, "Bad Request")
		//	return
		//}
		//defer func(Body io.ReadCloser) {
		//	err := Body.Close()
		//	if err != nil {
		//		logrus.Error(err)
		//	}
		//}(r.Body)
		//body, err := ioutil.ReadAll(r.Body)
		//if err != nil || string(body) == "" {
		//	h.render(w, http.StatusBadRequest, "Bad Request")
		//	return
		//}
		//var service = &corev1.Service{}
		//err = json.Unmarshal(body, service)
		//if err != nil {
		//	h.render(w, http.StatusBadRequest, "Bad Request")
		//	return
		//}
		//h.render(w, http.StatusOK, nil)
		//event <- model.Event{
		//	BindType:  model.Http,
		//	EventType: value,
		//	Data:      string(body),
		//}
	})
	return nil
}

func (h *Http) Close() {}
