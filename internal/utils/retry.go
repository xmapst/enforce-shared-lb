package utils

import (
	"github.com/avast/retry-go/v4"
	"github.com/sirupsen/logrus"
	"time"
)

func Retry(num uint, msg string, fn func() error) error {
	return retry.Do(
		fn,
		retry.Attempts(num),
		retry.DelayType(func(n uint, err error, config *retry.Config) time.Duration {
			max := time.Duration(n)
			if max > 8 {
				max = 8
			}
			duration := time.Second * max * max
			logrus.Warnf("%s, 第[%d]次重试, %v", msg, n, err)
			return duration
		}),
	)
}
