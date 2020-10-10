package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"
)

const urlPrefix = "https://ca.finance.yahoo.com/quote/"

type ticker struct {
	symbols  []string
	channel  string
	interval time.Duration

	lastRun time.Time
}

func (t *ticker) initialize() {
	logger.Print("ticker initializing")
	t.lastRun = time.Now()
}

func fetchData(symbol string, channel string) (string, error) {
	url := urlPrefix + symbol
	logger.Printf("ticker requesting %v", url)

	r, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()

	if r.StatusCode != 200 {
		return "", fmt.Errorf("request returned status code %v", r.StatusCode)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	buf := string(body)

	re := regexp.MustCompile("data-reactid=\"\\d+\">([0-9.]+)</span><span class")
	m := re.FindStringSubmatch(buf)
	if m == nil || len(m) < 2 {
		return "", fmt.Errorf("current value extraction failed for %v", symbol)
	}

	re = regexp.MustCompile("data-reactid=\"\\d+\">([+-]?[0-9.]+ \\([+-]?[0-9.%]+\\))</span><div id")
	n := re.FindStringSubmatch(buf)
	if n == nil || len(n) < 2 {
		return "", fmt.Errorf("value change extraction failed for %v", symbol)
	}

	ret := fmt.Sprintf("PRIVMSG %v :[ticker] %v %v %v", channel, symbol, m[1], n[1])
	return ret, nil
}

func (t *ticker) execute(r *kruntime) error {
	t.lastRun = time.Now()
	logger.Print("ticker module executing")

	for _, x := range t.symbols {
		buf, err := fetchData(x, t.channel)
		if err != nil {
			logger.Printf("ticker error in fetch data: %v", err)
			continue
		}
		r.ircout <- []byte(buf)
	}

	return nil
}

func (t *ticker) getName() string {
	return "ticker"
}

func (t *ticker) shouldRun() bool {
	return time.Now().After(t.lastRun.Add(t.interval))
}
