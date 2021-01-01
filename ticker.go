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

const urlPrefix = "https://ca.finance.yahoo.com/quote/"

type symbolCacheEntry struct {
	currentPrice float64
	delta        string // Used for display only, just store the string value
	privmsg      string // Preconstructed PRIVMSG command output
}

var symbolCache map[string]symbolCacheEntry

var unitReplacer = strings.NewReplacer(",", "", "k", "000", "K", "000", "m", "000000", "M", "000000")

type ticker struct {
	symbols        []string
	channel        string
	interval       time.Duration
	executeOnJoin  bool
	forceShouldRun bool

	lastRun time.Time
}

func (t *ticker) initialize() {
	logger.Print("ticker initializing")
	symbolCache = make(map[string]symbolCacheEntry)
	t.lastRun = time.Now()
}

func fetchData(symbol string, t *ticker) error {
	url := urlPrefix + symbol
	logger.Printf("ticker requesting %v", url)

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if config.Http.UserAgent != "" {
		req.Header.Set("User-Agent", config.Http.UserAgent)
	}

	r, err := client.Do(req)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if r.StatusCode != 200 {
		return fmt.Errorf("request returned status code %v", r.StatusCode)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}

	buf := string(body)

	re := regexp.MustCompile("data-reactid=\"\\d+\">([0-9.]+)</span><span class")
	m := re.FindStringSubmatch(buf)
	if m == nil || len(m) < 2 {
		return fmt.Errorf("current value extraction failed for %v", symbol)
	}

	re = regexp.MustCompile("data-reactid=\"\\d+\">([+-]?[0-9.]+ \\([+-]?[0-9.%]+\\))</span><div id")
	n := re.FindStringSubmatch(buf)
	if n == nil || len(n) < 2 {
		return fmt.Errorf("value change extraction failed for %v", symbol)
	}

	curPrice, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return fmt.Errorf("ticker error in calc float conversion, %v", err)
	}

	newEnt := &symbolCacheEntry{
		currentPrice: curPrice,
		delta:        n[1],
		privmsg:      fmt.Sprintf("PRIVMSG %v :[ticker] %v %v %v", t.channel, symbol, m[1], n[1]),
	}
	symbolCache[symbol] = *newEnt

	return nil
}

func (t *ticker) execute(r *kruntime) error {
	t.lastRun = time.Now()
	logger.Print("ticker module executing")

	for _, x := range t.symbols {
		err := fetchData(x, t)
		if err != nil {
			logger.Printf("ticker error in fetch data: %v", err)
			continue
		}
		r.ircout <- []byte(symbolCache[x].privmsg)
	}

	return nil
}

func (t *ticker) getName() string {
	return "ticker"
}

func (t *ticker) shouldRun() bool {
	tm := time.Now()

	if t.forceShouldRun {
		t.forceShouldRun = false
		return true
	}

	if tm.Weekday() == 0 || tm.Weekday() == 6 {
		return false
	}

	if tm.UTC().Hour() < config.Ticker.ScheduleUTCStartHour ||
		tm.UTC().Hour() > config.Ticker.ScheduleUTCStopHour {
		return false
	}

	return tm.After(t.lastRun.Add(t.interval))
}

func (t *ticker) shouldRunOnJoin(channel string) bool {
	ret := t.executeOnJoin && t.channel == channel

	if ret {
		// We are going to execute on join; wind the lastRun counters back twice the
		// interval so shouldRun will return success
		t.lastRun = t.lastRun.Add(-2 * t.interval)
		t.forceShouldRun = true
		logger.Printf("ticker lastRun wound back to %v", t.lastRun)
	}

	return ret
}

func (t *ticker) handlesCommand(cmd string) bool {
	return cmd == "&calc" || cmd == "&ticker"
}

func (t *ticker) handleCommand(src sourceDescriptor, cmd string, args []string, r *kruntime) {
	switch cmd {
	case "&ticker":
		if len(args) < 5 {
			var cachedSymbols []string
			for symbol := range symbolCache {
				cachedSymbols = append(cachedSymbols, symbol)
			}
			sort.Strings(cachedSymbols)
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[ticker] available symbols: %v",
				t.channel, strings.Join(cachedSymbols, " ")))
			return
		}
		if v, ok := symbolCache[args[4]]; ok {
			r.ircout <- []byte(v.privmsg)
		}
	case "&calc":
		if len(args) < 6 {
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[calc] usage: &calc <symbol> <count>",
				t.channel))
			return
		}

		symbol := strings.ToUpper(args[4])

		if v, ok := symbolCache[symbol]; ok {
			units, err := strconv.Atoi(unitReplacer.Replace(args[5]))
			if err != nil {
				logger.Printf("ticker error in unit conversion, %v", err)
				return
			}

			if units <= 0 {
				return
			}

			rv := float64(units) * v.currentPrice
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :[calc] %v %v x $%v = $%v",
				t.channel, symbol, humanize.Comma(int64(units)),
				humanize.FormatFloat("#,###.####", v.currentPrice),
				humanize.FormatFloat("#,###.##", rv)))
		}
	}
}
