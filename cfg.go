package main

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type httpCfg struct {
	UserAgent string
}

type tickerCfg struct {
	Symbols              []string
	Interval             string
	Channel              string
	Calc                 string
	ExecuteOnJoin        bool
	ScheduleUTCStartHour int
	ScheduleUTCStopHour  int
}

type writerCfg struct {
	Channel  string
	Datapath string
	Interval string
}

type cfg struct {
	Nick         string
	Servers      []string
	Channels     []string
	VerifyCert   bool
	SaslUser     string
	SaslPassword string

	Http   httpCfg
	Ticker tickerCfg
	Writer writerCfg
}

func (c *cfg) validate() error {
	return nil
}

func loadCfg(confpath string) (*cfg, error) {
	logger.Printf("loading configuration from %v", confpath)
	buf, err := ioutil.ReadFile(confpath)
	if err != nil {
		return nil, err
	}

	var ret cfg
	err = yaml.Unmarshal(buf, &ret)
	if err != nil {
		return nil, err
	}

	if ret.Ticker.ScheduleUTCStartHour == 0 {
		ret.Ticker.ScheduleUTCStartHour = 13
	}
	if ret.Ticker.ScheduleUTCStopHour == 0 {
		ret.Ticker.ScheduleUTCStopHour = 21
	}

	return &ret, ret.validate()
}
