package agollo

import (
	"fmt"
	"io"
	"io/ioutil"
)

type Logger interface {
	Log(kvs ...interface{})
}

type LoggerOption func(*logger)

func LoggerWriter(w io.Writer) LoggerOption {
	return func(l *logger) {
		l.w = w
	}
}

func NewLogger(opts ...LoggerOption) Logger {
	l := &logger{}
	for _, opt := range opts {
		opt(l)
	}

	if l.w == nil {
		l.w = ioutil.Discard
	}

	return l
}

type logger struct {
	w io.Writer
}

func (l *logger) Log(kvs ...interface{}) {
	fmt.Fprintln(l.w, kvs...)
}
