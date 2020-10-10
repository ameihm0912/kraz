package main

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type tickerCfg struct {
	Symbols  []string
	Interval string
	Channel  string
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

	return &ret, ret.validate()
}
