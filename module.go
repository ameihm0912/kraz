package main

import (
	"time"
)

type module interface {
	getName() string
	shouldRun() bool
	shouldRunOnJoin(string) bool
	execute(*kruntime) error
	initialize()
	handlesCommand(string) bool
	handleCommand(sourceDescriptor, string, []string, *kruntime)
}

func moduleRegistration() error {
	var err error

	if config.Ticker.Interval != "" {
		t := ticker{}
		t.symbols = config.Ticker.Symbols
		t.channel = config.Ticker.Channel
		t.interval, err = time.ParseDuration(config.Ticker.Interval)
		if err != nil {
			return err
		}
		t.executeOnJoin = config.Ticker.ExecuteOnJoin
		runtime.addModule(&t)
	}

	if config.Writer.Interval != "" {
		t := writer{}
		t.channel = config.Writer.Channel
		t.datapath = config.Writer.Datapath
		t.interval, err = time.ParseDuration(config.Writer.Interval)
		if err != nil {
			return err
		}
		runtime.addModule(&t)
	}

	return nil
}
