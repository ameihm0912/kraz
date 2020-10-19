package main

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"path"
	"strings"
	"time"
)

type writer struct {
	channel  string
	datapath string
	interval time.Duration

	lastRun time.Time
}

func (w *writer) getName() string {
	return "writer"
}

func (w *writer) shouldRun() bool {
	return w.interval.Seconds() != 0 && time.Now().After(w.lastRun.Add(w.interval))
}

func (w *writer) handlesCommand(cmd string) bool {
	return cmd == "&w"
}

func (w *writer) handleCommand(src sourceDescriptor, cmd string, args []string, r *kruntime) {
	target := args[2]

	list, err := w.availableEntries()
	if err != nil {
		logger.Printf("writer error getting available entries, %v", err)
		return
	}

	if len(args) >= 5 {
		if args[4] == "list" {
			buf := strings.Join(list, " ")
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :writer: available: %v", target, buf))
		} else {
			found := false
			for _, x := range list {
				if x == args[4] {
					found = true
				}
			}
			if !found {
				logger.Printf("writer source %v not available", args[4])
				return
			}
			w.write(target, args[4], r)
		}
	} else {
		upath := list[rand.Intn(len(list))]
		w.write(target, upath, r)
	}
}

func (w *writer) availableEntries() ([]string, error) {
	list, err := ioutil.ReadDir(w.datapath)
	if err != nil {
		return nil, err
	}

	ret := make([]string, len(list))
	for i, x := range list {
		ret[i] = x.Name()
	}

	return ret, nil
}

func (w *writer) write(target string, source string, r *kruntime) error {
	buf, err := ioutil.ReadFile(path.Join(w.datapath, source))
	if err != nil {
		return err
	}

	parts := strings.Split(string(buf), "\n")
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(parts), func(i, j int) { parts[i], parts[j] = parts[j], parts[i] })

	for _, x := range parts {
		val := string(x)
		if val == "" {
			continue
		}
		p := rand.Intn(5)
		if p == 0 && len(val) <= 6 {
			for _, y := range val {
				r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :%v", target, string(y)))
			}
		} else {
			b := rand.Intn(6) == 0
			msg := string(val[0])
			for _, y := range val[1:] {
				msg += " " + string(y)
			}
			if b {
				msg = "" + msg + ""
			}
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :%v", target, msg))
		}
	}

	return nil
}

func (w *writer) execute(r *kruntime) error {
	w.lastRun = time.Now()
	logger.Print("writer module executing")

	rand.Seed(time.Now().UnixNano())
	val := rand.Intn(10)
	if val > 0 {
		logger.Printf("writer skipping, %v != 0", val)
		return nil
	}

	list, err := w.availableEntries()
	if err != nil {
		return err
	}

	upath := list[rand.Intn(len(list))]
	w.write(w.channel, upath, r)

	return nil
}

func (w *writer) initialize() {
	logger.Print("writer initializing")
	w.lastRun = time.Now()
}
