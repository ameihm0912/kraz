package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"sync"
	"time"
)

func net_connect(servers []string, verify bool) (*tls.Conn, error) {
	var ret *tls.Conn
	var err error

	tlsconf := &tls.Config{
		InsecureSkipVerify: false,
	}
	if !verify {
		tlsconf.InsecureSkipVerify = true
	}

	for _, s := range servers {
		logger.Printf("attempting connection to %v", s)

		ret, err = tls.Dial("tcp", s, tlsconf)
		if err == nil {
			logger.Printf("connection established to %v", s)
			cs := ret.ConnectionState()
			logger.Printf("cipher_suite: %v", cs.CipherSuite)
			return ret, nil
		}
		logger.Printf("error connecting to %v: %v", s, err)
	}
	return ret, fmt.Errorf("no servers were available")
}

func net_dispatch_available(store *bytes.Buffer) {
	for {
		idx := bytes.Index(store.Bytes(), []byte("\n"))
		if idx == -1 {
			break
		}
		b := bytes.Trim(store.Next(idx+1), "\r\n")
		// Dispatch the incoming command to the protocol handler
		logger.Printf("net_reader: server: %v", string(b))
		s := make([]byte, len(b))
		copy(s, b)
		runtime.ircin <- s
	}
}

func net_reader(wg *sync.WaitGroup, conn *tls.Conn) {
	defer func() {
		logger.Print("net_reader exiting")
		wg.Done()
	}()

	var store bytes.Buffer
	buf := make([]byte, 4096)

	store.Reset()
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			store.Write(buf[:n])
		}
		if err != nil {
			logger.Printf("read error: %v", err)
			// If a read error has occurred, treat it as fatal and dispatch a meta
			// notification to the protocol handler. Also dispatch any remaining data
			// we have in the store buffer.
			runtime.ircmeta <- IRC_META_RESET
			net_dispatch_available(&store)
			err = conn.Close()
			if err != nil {
				logger.Printf("error closing connection: %v", err)
			}
			return
		}

		net_dispatch_available(&store)
	}
}

func net_writer(wg *sync.WaitGroup, conn *tls.Conn) {
	defer func() {
		logger.Print("net_writer exiting")
		wg.Done()
	}()

	var lastWrite time.Time
	for {
		select {
		case buf := <-runtime.ircout:
			logger.Printf("net_writer: server: %v", string(buf))
			if !lastWrite.IsZero() && time.Now().Before(lastWrite.Add(1*time.Second)) {
				time.Sleep(1 * time.Second)
			}
			lastWrite = time.Now()
			_, err := conn.Write(append(buf, []byte{'\r', '\n'}...))
			if err != nil {
				logger.Printf("write error: %v", err)
			}
		case <-runtime.net_writer_exit:
			logger.Print("net_writer got signal to exit")
			return
		}
	}
}
