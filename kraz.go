package main

import (
	"crypto/tls"
	"flag"
	"log"
	"os"
	"sync"
	"time"
)

const version = "0.0.1"

var config *cfg
var runtime kruntime
var logger *log.Logger

type channelStatus struct {
	name      string
	joined    bool
	join_sent time.Time // Last time a JOIN was sent for this channel
}

type kruntime struct {
	connected bool

	ircin    chan []byte
	ircout   chan []byte
	ircmeta  chan int
	ircreset chan bool // Used to indicate the IRC handler is reset and ready

	net_writer_exit chan bool

	registered bool

	channel []channelStatus

	modules []module
}

func (k *kruntime) addModule(m module) {
	logger.Printf("registering module: %v", m.getName())
	m.initialize()
	k.modules = append(k.modules, m)
}

func (k *kruntime) markChannelJoined(name string, status bool) {
	for i := range k.channel {
		if k.channel[i].name == name {
			k.channel[i].joined = status
			return
		}
	}
}

func (k *kruntime) resetStatus() {
	for i := range k.channel {
		k.channel[i].joined = false
		k.channel[i].join_sent = time.Time{}
	}
	k.registered = false
}

func (k *kruntime) stateInit() {
	logger.Print("initializing runtime state")

	k.connected = false
	k.registered = false

	k.ircin = make(chan []byte, 512)
	k.ircout = make(chan []byte, 512)
	k.ircmeta = make(chan int) // We don't buffer meta commands
	k.ircreset = make(chan bool)

	k.net_writer_exit = make(chan bool)

	for _, x := range config.Channels {
		logger.Printf("configuring for %v", x)
		runtime.channel = append(runtime.channel,
			channelStatus{
				name:   x,
				joined: false,
			})
	}
}

func init() {
	logger = log.New(os.Stdout, "kraz: ", log.Ltime|log.Ldate|log.LUTC)
}

func entry(wg *sync.WaitGroup) {
	defer func() {
		logger.Print("main thread exiting")
		wg.Done()
	}()

	logger.Print("main thread starting")

	for {
		var conn *tls.Conn
		var err error
		// Check our connection status and see if we need to establish or not
		if !runtime.connected {
			conn, err = net_connect(config.Servers, config.VerifyCert)
			if err != nil {
				logger.Printf("connection error: %v: sleeping for retry", err)
				time.Sleep(5 * time.Second)
				continue
			}
			runtime.connected = true
		}

		// Signal the protocol handler we have a valid connection and we want to
		// send our registration
		runtime.ircmeta <- IRC_META_REGISTER

		var wg sync.WaitGroup
		wg.Add(2)
		go net_reader(&wg, conn)
		go net_writer(&wg, conn)
		wg.Wait()
		runtime.connected = false

		// If we get here, the network threads have exited but we want to make sure the IRC
		// protocol handler is ready for a new connection, wait until we get a signal from it
		<-runtime.ircreset
	}
}

func execute(confpath string) {
	logger.Print("initializing")

	var err error
	config, err = loadCfg(confpath)
	if err != nil {
		log.Fatalf("error loading configuration: %v", err)
	}

	runtime.stateInit()

	err = moduleRegistration()
	if err != nil {
		log.Fatalf("error during module registration: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go entry(&wg)
	go irc_handler(&wg)
	wg.Wait()
}

func main() {
	confpath := flag.String("c", "./kraz.yaml", "path to configuration")
	flag.Parse()
	execute(*confpath)
}
