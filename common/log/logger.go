/*
Copyright Zhigui.com. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package log

import (
	"fmt"

	"github.com/inconshreveable/log15"
	"github.com/zhigui-projects/go-hotstuff/api"
)

var defaultLogger api.Logger

func SetLogger(l api.Logger) {
	defaultLogger = l
}

func GetLogger(ctx ...interface{}) api.Logger {
	if defaultLogger == nil {
		defaultLogger = api.Logger(&DefaultLogger{New("logger", "hotstuff")})
	}
	if len(ctx) == 0 {
		return defaultLogger
	}
	return defaultLogger.New(ctx...)
}

// DefaultLogger is a default implementation of the Logger interface.
type DefaultLogger struct {
	log15.Logger
}

func (l *DefaultLogger) New(ctx ...interface{}) api.Logger {
	return &DefaultLogger{l.Logger.New(ctx...)}
}

func (l *DefaultLogger) Debug(v ...interface{}) {
	if len(v) > 0 {
		ctx := v[1:]
		l.Logger.Debug(v[0].(string), ctx...)
	}
}

func (l *DefaultLogger) Debugf(format string, v ...interface{}) {
	l.Logger.Debug(fmt.Sprintf(format, v...))
}

func (l *DefaultLogger) Error(v ...interface{}) {
	if len(v) > 0 {
		ctx := v[1:]
		l.Logger.Error(v[0].(string), ctx...)
	}
}

func (l *DefaultLogger) Errorf(format string, v ...interface{}) {
	l.Logger.Error(fmt.Sprintf(format, v...))
}

func (l *DefaultLogger) Info(v ...interface{}) {
	if len(v) > 0 {
		ctx := v[1:]
		l.Logger.Info(v[0].(string), ctx...)
	}
}

func (l *DefaultLogger) Infof(format string, v ...interface{}) {
	l.Logger.Info(fmt.Sprintf(format, v...))
}

func (l *DefaultLogger) Warning(v ...interface{}) {
	if len(v) > 0 {
		ctx := v[1:]
		l.Logger.Warn(v[0].(string), ctx...)
	}
}

func (l *DefaultLogger) Warningf(format string, v ...interface{}) {
	l.Logger.Warn(fmt.Sprintf(format, v...))
}

func (l *DefaultLogger) Fatal(v ...interface{}) {
	if len(v) > 0 {
		ctx := v[1:]
		l.Logger.Crit(v[0].(string), ctx...)
	}
}

func (l *DefaultLogger) Fatalf(format string, v ...interface{}) {
	l.Logger.Crit(fmt.Sprintf(format, v...))
}

func (l *DefaultLogger) Panic(v ...interface{}) {
	panic(fmt.Sprint(v...))
}

func (l *DefaultLogger) Panicf(format string, v ...interface{}) {
	panic(fmt.Sprintf(format, v...))
}
