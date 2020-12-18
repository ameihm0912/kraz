package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

var dataCache map[string]string
var priceCache map[string]float64
var calcCache float64
var unitReplacer = strings.NewReplacer(",", "", "k", "000", "K", "000", "m", "000000", "M", "000000")

type ticker struct {
	symbols  []string
	channel  string
	interval time.Duration

	lastRun time.Time
}

type krazroundtripper struct {
	rt http.RoundTripper
}

func (t *ticker) initialize() {
	logger.Print("ticker initializing")
	dataCache = make(map[string]string)
	priceCache = make(map[string]float64)
	t.lastRun = time.Now()
}

func (krt krazroundtripper) RoundTrip(req *http.Request) (*http.Response, error) {
	logger.Printf("ticker setting user agent to: %v", config.HTTP.UserAgent)
	req.Header.Add("User-Agent", config.HTTP.UserAgent)
	return krt.rt.RoundTrip(req)
}

func fetchData(symbol string, t *ticker) (string, error) {
	url := fmt.Sprintf(config.Ticker.Source, symbol)
	logger.Printf("ticker requesting %v", url)
	httpClient := &http.Client{
		Transport: krazroundtripper{rt: http.DefaultTransport},
	}
	r, err := httpClient.Get(url)
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

	calcCache, err = strconv.ParseFloat(m[1], 64)
	if err != nil {
		logger.Printf("ticker error in calc float conversion, %v", err)
	}
	priceCache[strings.ToUpper(symbol)] = calcCache

	ret := fmt.Sprintf("PRIVMSG %v :[ticker] %v %v %v", t.channel, symbol, m[1], n[1])

	dataCache[strings.ToUpper(symbol)] = ret

	return ret, nil
}

func (t *ticker) execute(r *kruntime) error {
	t.lastRun = time.Now()
	logger.Print("ticker module executing")

	for _, x := range t.symbols {
		_, err := fetchData(x, t)
		if err != nil {
			logger.Printf("ticker error in fetch data: %v", err)
			continue
		}
		r.ircout <- []byte(dataCache[x])
	}

	return nil
}

func (t *ticker) getName() string {
	return "ticker"
}

func (t *ticker) shouldRun() bool {
	tm := time.Now()

	// Update the symbol prices so we don't have to wait for the interval to trigger
	if calcCache <= 0 && config.Ticker.ExecuteOnJoin {
		return true
	}

	for _, x := range config.Ticker.ExcludeDays {
		if tm.Weekday() == x {
			return false
		}
	}
	if tm.UTC().Hour() < config.Ticker.ScheduleUTCStart || tm.UTC().Hour() > config.Ticker.ScheduleUTCStop {
		return false
	}

	return tm.After(t.lastRun.Add(t.interval))
}

func (t *ticker) handlesCommand(cmd string) bool {
	return cmd == "&calc" || cmd == "&ticker"
}

func (t *ticker) handleCommand(src sourceDescriptor, cmd string, args []string, r *kruntime) {
	switch cmd {
	case "&ticker":
		if len(args) < 5 {
			cachedSymbols := []string{}
			for symbol := range dataCache {
				cachedSymbols = append(cachedSymbols, symbol)
			}
			sort.Strings(cachedSymbols)
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[ticker] available symbols: %v", t.channel, strings.Join(cachedSymbols, " ")))
			return
		}

		symbol := strings.ToUpper(args[4])
		logger.Printf("ticker module loading cached data for %v", symbol)
		r.ircout <- []byte(dataCache[symbol])
	case "&calc":
		if len(args) < 5 {
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[calc] usage: &calc <symbol> [share count]", t.channel))
			return
		}

		symbol := strings.ToUpper(args[4])

		if _, ok := priceCache[symbol]; !ok {
			cachedSymbols := []string{}
			for symbol := range priceCache {
				cachedSymbols = append(cachedSymbols, symbol)
			}
			sort.Strings(cachedSymbols)
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[calc] available symbols: %v", t.channel, strings.Join(cachedSymbols, " ")))
			return
		}

		if len(args) < 6 {
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[calc] usage: &calc %v [share count]", t.channel, symbol))
			return
		}

		logger.Printf("ticker module loading cached price for %v", symbol)
		if priceCache[symbol] <= 0 {
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[calc] %v unavailable; pending ticker update",
				t.channel, symbol))
			return
		}

		v, err := strconv.Atoi(unitReplacer.Replace(args[5]))
		if err != nil || v <= 0 {
			return
		}
		rv := float64(v) * priceCache[symbol]
		r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[calc] %v %v x $%v = $%v",
			t.channel, symbol, humanize.Comma(int64(v)), humanize.FormatFloat("#,###.####", priceCache[symbol]), humanize.FormatFloat("#,###.##", rv)))
	}
}
