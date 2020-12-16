package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const urlPrefix = "https://ca.finance.yahoo.com/quote/"

var calcCache float64

type ticker struct {
	symbols  []string
	channel  string
	interval time.Duration
	calc     string

	lastRun time.Time
}

func (t *ticker) initialize() {
	logger.Print("ticker initializing")
	t.lastRun = time.Now()
}

func fetchData(symbol string, t *ticker) (string, error) {
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

	if symbol == t.calc {
		calcCache, err = strconv.ParseFloat(m[1], 64)
		if err != nil {
			logger.Printf("ticker error in calc float conversion, %v", err)
		}
	}

	ret := fmt.Sprintf("PRIVMSG %v :[ticker] %v %v %v", t.channel, symbol, m[1], n[1])
	return ret, nil
}

func (t *ticker) execute(r *kruntime) error {
	t.lastRun = time.Now()
	logger.Print("ticker module executing")

	for _, x := range t.symbols {
		buf, err := fetchData(x, t)
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
	tm := time.Now()

	if tm.Weekday() == 0 || tm.Weekday() == 6 {
		return false
	}
	if tm.UTC().Hour() < 13 || tm.UTC().Hour() > 21 {
		return false
	}

	return tm.After(t.lastRun.Add(t.interval))
}

func (t *ticker) handlesCommand(cmd string) bool {
	return cmd == "&calc"
}

func (t *ticker) handleCommand(src sourceDescriptor, cmd string, args []string, r *kruntime) {
	if t.calc == "" || len(args) < 5 {
		return
	}
	v, err := strconv.Atoi(args[4])
	if err != nil || v <= 0 {
		return
	}
	rv := float64(v) * calcCache
	r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[ticker] calc %v %v == $%.2f",
		t.channel, t.calc, args[4], rv))
}
