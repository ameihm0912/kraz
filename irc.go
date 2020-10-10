package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	IRC_META_REGISTER = iota
	IRC_META_NICKREGISTER
	IRC_META_RESET
)

var shouldReset = false

type sourceDescriptor struct {
	isServer bool
	server   string
	nick     string
	ident    string
	host     string
}

func (src *sourceDescriptor) isMe() bool {
	if !src.isServer &&
		src.nick == config.Nick {
		return true
	}
	return false
}

func irc_parse_source(src string) (sourceDescriptor, error) {
	var ret sourceDescriptor

	if src[0] != ':' || len(src) < 2 {
		return ret, fmt.Errorf("malformed source descriptor %v", src)
	}
	parts := strings.Split(src[1:], "!")
	if len(parts) != 2 {
		ret.isServer = true
		ret.server = src[1:]
		return ret, nil
	}
	ret.nick = parts[0]

	parts = strings.Split(parts[1], "@")
	if len(parts) != 2 {
		return ret, fmt.Errorf("error separating ident and host in %v", src)
	}
	ret.ident = parts[0]
	ret.host = parts[1]

	return ret, nil
}

func irc_handler(wg *sync.WaitGroup) {
	defer func() {
		logger.Print("irc handler exiting")
		wg.Done()
	}()

	logger.Print("irc handler starting")

	for {
		if shouldReset {
			// The intent of this code block is draining the IRC handler channels
			// if we have a notification we should reset state
			done := false
			for {
				select {
				case buf := <-runtime.ircin:
					logger.Printf("irc_input (discard): %v", string(buf))
				case meta := <-runtime.ircmeta:
					logger.Printf("irc_meta (discard): %v", string(meta))
				default:
					done = true
				}
				if done {
					break
				}
			}
			shouldReset = false

			// Notify entry we are ready and nothing remains in the channels
			logger.Print("irc_handler: ready for new connections")
			runtime.ircreset <- true
		}

		select {
		case buf := <-runtime.ircin:
			irc_input(buf)
		case meta := <-runtime.ircmeta:
			irc_meta(meta)
		case <-time.After(5 * time.Second):
			irc_periodic()
		}
	}
}

func irc_periodic() {
	if !runtime.registered {
		return
	}

	for i := range runtime.channel {
		x := &runtime.channel[i]
		if x.joined {
			continue
		}
		if x.join_sent.IsZero() ||
			time.Now().After(x.join_sent.Add(time.Duration(time.Second*30))) {
			x.join_sent = time.Now()
			logger.Printf("irc_periodic: attempting to join %v", x.name)
			runtime.ircout <- []byte(fmt.Sprintf("JOIN %v", x.name))
		}
	}

	for i := range runtime.modules {
		m := runtime.modules[i]
		if !m.shouldRun() {
			continue
		}
		err := m.execute(&runtime)
		if err != nil {
			logger.Printf("error in module %v: %v", m.getName(), err)
		}
	}
}

func irc_handle_join(src sourceDescriptor, args []string) {
	if len(args) < 3 {
		return
	}
	channame := args[2]
	if channame[0] == ':' {
		channame = channame[1:]
	}
	if src.isMe() {
		logger.Printf("marking %v as joined", channame)
		runtime.markChannelJoined(channame, true)
	}
}

func irc_handle_kick(src sourceDescriptor, args []string) {
	if len(args) < 4 {
		return
	}
	channame := args[2]
	kicked := args[3]

	if kicked == config.Nick {
		logger.Printf("marking %v as parted", channame)
		runtime.markChannelJoined(channame, false)
	}
}

func irc_handle_privmsg(src sourceDescriptor, args []string) {
	if len(args) < 4 {
		return
	}
	//recip := args[2]

	if args[3] == ":PING" {
		resp := strings.Join(args[3:], " ")
		runtime.ircout <- []byte(fmt.Sprintf("NOTICE %v %v",
			src.nick, resp))
	} else if args[3] == ":VERSION" {
		runtime.ircout <- []byte(fmt.Sprintf("NOTICE %v "+
			":VERSION kraz %v", src.nick, version))
	}
}

func irc_send_sasl_auth() {
	out := bytes.Join([][]byte{[]byte(config.SaslUser),
		[]byte(config.SaslUser), []byte(config.SaslPassword)}, []byte{0})
	enc := base64.StdEncoding.EncodeToString(out)
	runtime.ircout <- []byte(fmt.Sprintf("AUTHENTICATE %v", enc))
	runtime.ircout <- []byte("CAP END")
}

func irc_input(buf []byte) {
	args := strings.Split(string(buf), " ")

	if len(args) <= 1 {
		logger.Printf("ignoring input with insufficient arguments: %v", string(buf))
	}

	if args[0] == "PING" {
		runtime.ircout <- bytes.Replace(buf, []byte("PING"),
			[]byte("PONG"), 1)
		return
	} else if args[0] == "AUTHENTICATE" {
		if len(args) >= 1 && args[1] == "+" {
			// Send authentication messages
			irc_send_sasl_auth()
		}
	}

	var src sourceDescriptor
	var err error
	if args[0][0] == ':' {
		src, err = irc_parse_source(args[0])
		if err != nil {
			logger.Printf("error parsing source: %v", err)
			return
		}
	}

	switch args[1] {
	case "001":
		logger.Print("irc_input: registered")
		runtime.registered = true
	case "903":
		// Indication SASL authentication was successful, if so initiate nick
		// registration
		go func() {
			runtime.ircmeta <- IRC_META_NICKREGISTER
		}()
	case "JOIN":
		irc_handle_join(src, args)
	case "KICK":
		irc_handle_kick(src, args)
	case "PRIVMSG":
		irc_handle_privmsg(src, args)
	case "CAP":
		if len(args) >= 5 && args[3] == "ACK" && args[4] == ":sasl" {
			runtime.ircout <- []byte("AUTHENTICATE PLAIN")
		}
	}
}

func irc_meta(meta int) {
	switch meta {
	case IRC_META_REGISTER:
		logger.Print("irc_meta: got registration notification, beginning registration")

		// If we don't want SASL, we can just proceed to sending registration commands
		if config.SaslUser == "" {
			logger.Print("skipping SASL authentication")
			go func() {
				runtime.ircmeta <- IRC_META_NICKREGISTER
			}()
			return
		}
		logger.Print("attempting SASL authentication")
		runtime.ircout <- []byte("CAP REQ :sasl")
	case IRC_META_NICKREGISTER:
		runtime.ircout <- []byte(fmt.Sprintf("NICK %v", config.Nick))
		runtime.ircout <- []byte(fmt.Sprintf("USER %v @ host :%v", config.Nick,
			config.Nick))
	case IRC_META_RESET:
		logger.Print("irc_meta: got reset notification, cleaning up for a new connection")

		// Signal the writer routine it should exit
		runtime.net_writer_exit <- true

		runtime.resetStatus()
		shouldReset = true
	}
}
