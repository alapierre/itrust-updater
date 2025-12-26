package logging

import (
	"github.com/sirupsen/logrus"
)

func Component(name string) *logrus.Entry {
	return logrus.WithField("component", name)
}

func App(name string) *logrus.Entry {
	return Component(name)
}
