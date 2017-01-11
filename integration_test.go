// +build integration

package restwebsocket

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

var (
	socketserver *WebsocketServer
	socketclient *WebsocketClient
	done         chan struct{}
)

func simpleHandler(rw http.ResponseWriter, req *http.Request) {
	rw.Write([]byte("Hello, Dolores!"))
}

func chunkedHandler(rw http.ResponseWriter, req *http.Request) {
	flusher, ok := rw.(http.Flusher)
	if !ok {
		panic("expected http.ResponseWriter to be an http.Flusher")
	}
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(rw, "Chunk #%d\n", i)
		flusher.Flush()
		time.Sleep(1 * time.Second)
	}
}

func closeNotifiedChunkedHandler(rw http.ResponseWriter, req *http.Request) {
	flusher, ok := rw.(http.Flusher)
	if !ok {
		panic("expected http.ResponseWriter to be an http.Flusher")
	}
	notify := rw.(http.CloseNotifier).CloseNotify()
	for i := 1; i <= 10; i++ {

		select {
		case <-notify:
			log.Println("connection closed...exiting handler")
			return
		default:
			fmt.Fprintf(rw, "Chunk #%d\n", i)
			flusher.Flush()
			time.Sleep(1 * time.Second)
		}
	}
}

func startSocketServer() (*WebsocketServer, chan struct{}) {
	wsServer := NewWebSocketServer(":9090", 100, true, nil, nil, nil)
	ch, err := wsServer.Accept()
	if err != nil {
		log.Println("accept: ", err)
	}
	m := http.NewServeMux()
	m.HandleFunc("/simple/", simpleHandler)
	m.HandleFunc("/chunked/", chunkedHandler)
	m.HandleFunc("/closenotifiedchunked/", closeNotifiedChunkedHandler)
	done := make(chan struct{})
	log.Println("socketserver waiting for connection")
	go func() {
		defer log.Printf("closing socketserver")
		for {
			select {
			case conn := <-ch:
				conn.Serve(context.Background(), m)
				//time.Sleep(5 * time.Second)
				//conn.Close()
			case <-done:
				return
			}
		}
	}()
	return wsServer, done
}

func TestMain(m *testing.M) {
	socketserver, done = startSocketServer()
	u, _ := url.Parse("ws://localhost:9090/")
	socketclient = NewWebSocketClient(u, true, nil, nil, nil)
	code := m.Run()
	close(done)
	os.Exit(code)
}

func TestSimpleHandlerSuccess(t *testing.T) {
	err := socketclient.Connect()
	assert.Nil(t, err)
	req, _ := http.NewRequest(http.MethodGet, "ws://localhost:9090/simple/", nil)
	for i := 0; i < 4; i++ {
		err := socketclient.Connection().WriteRequest(req)
		assert.Nil(t, err)
		resp, err := socketclient.Connection().ReadResponse()
		assert.Nil(t, err)
		assert.NotNil(t, resp)
		b, err := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		assert.Nil(t, err)
		assert.Equal(t, "Hello, Dolores!", string(b))
	}
	beforeCount := socketserver.activeConnectionCount()
	socketclient.Connection().Close()
	// give some time for the server to unregister connection
	// ToDo find a better way to do this
	time.Sleep(1 * time.Second)
	assert.Equal(t, beforeCount-1, socketserver.activeConnectionCount())
}

func TestSimpleHandlerFailureClientSide(t *testing.T) {
	err := socketclient.Connect()
	// disabling failure detection at the socketclient side
	socketclient.Connection().heartBeat.stop()
	assert.Nil(t, err)
	// force close the underlying network connection
	beforeCount := socketserver.activeConnectionCount()
	socketclient.conn.conn.UnderlyingConn().Close()
	// the server is expected to detect that and remove the connection from the connection map
	success := make(chan bool, 1)
	go func() {
		for {
			if socketserver.activeConnectionCount() == beforeCount-1 {
				success <- true
				return
			}
			time.Sleep(1 * time.Second)
		}
	}()
	select {
	case <-time.After(30 * time.Second):
		t.Fatal("timed out")
	case <-success:
	}
}

func TestSimpleHandlerFailureServerSide(t *testing.T) {
	time.Sleep(3 * time.Second)
	err := socketclient.Connect()
	// disabling failure detection at the socketclient side
	assert.Nil(t, err)
	// force close the underlying network connection
	//beforeConn := socketclient.Connection()
	socketclient.conn.conn.UnderlyingConn().Close()
	time.Sleep(10 * time.Second)
	// client should try to reconnect with backoff
	success := make(chan bool, 1)
	go func() {
		for {
			if err := socketclient.Connection().conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err == nil {
				success <- true
				return
			}
			time.Sleep(1 * time.Second)
		}
	}()
	select {
	case <-time.After(20 * time.Second):
		t.Fatal("timed out")
	case <-success:
	}
}
