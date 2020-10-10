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
	return time.Now().After(w.lastRun.Add(w.interval))
}

func (w *writer) execute(r *kruntime) error {
	w.lastRun = time.Now()
	logger.Print("writer module executing")

	list, err := ioutil.ReadDir(w.datapath)
	if err != nil {
		return err
	}

	upath := list[rand.Intn(len(list))]
	buf, err := ioutil.ReadFile(path.Join(w.datapath, upath.Name()))
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
		p := rand.Intn(3)
		if p == 0 {
			for _, y := range val {
				r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :%v", w.channel, string(y)))
			}
		} else {
			msg := string(val[0])
			for _, y := range val[1:] {
				msg += " " + string(y)
			}
			r.ircout <- []byte(fmt.Sprintf("PRIVMSG %v :%v", w.channel, msg))
		}
	}

	return nil
}

func (w *writer) initialize() {
	logger.Print("writer initializing")
	w.lastRun = time.Now()
}
