package restwebsocket

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

// reliableRestServer is the implementation of the RestServer interface
type reliableRestServer struct {
	SocketConnection
	rw   *restResponseWriter
	hdlr http.Handler
}

// NewRestServer creates a new rest api server
func NewRestServer(conn SocketConnection, handler http.Handler) RestServer {
	return &reliableRestServer{
		SocketConnection: conn,
		rw:               newRestResponseWriter(),
		hdlr:             handler,
	}
}

func (s *reliableRestServer) SocketResponseWriter() http.ResponseWriter {
	return s.rw
}

func (s *reliableRestServer) Serve() error {
	for {
		req := s.ReadRequest()

		if req == nil {
			log.Println("not a text message")
		}

		rw := s.rw
		s.hdlr.ServeHTTP(rw, req)
		response := rw.close()
		if err := s.WriteRaw(response); err != nil {
			log.Println("write message:", err)
		}
	}
}

type restResponseWriter struct {
	Status    int
	Buf       *bytes.Buffer
	HeaderMap http.Header
}

func (rw *restResponseWriter) Header() http.Header {
	return rw.HeaderMap
}

func (rw *restResponseWriter) WriteHeader(code int) {
	rw.Status = code
	// TODO: you should also write the headers here
}

func (rw *restResponseWriter) Write(b []byte) (int, error) {
	i, err := rw.Buf.Write(b)
	return i, err
}

func (rw *restResponseWriter) close() []byte {
	resp := &restResponse{
		Status:    rw.Status,
		Body:      rw.Buf.Bytes(),
		HeaderMap: rw.HeaderMap,
	}
	// Do the actual writing here
	b, _ := json.Marshal(resp)
	responseBody := string(resp.Body)
	log.Println("resp body: ", responseBody)
	return b

}

func newRestResponseWriter() *restResponseWriter {
	var b []byte
	return &restResponseWriter{
		Buf:       bytes.NewBuffer(b),
		HeaderMap: make(http.Header),
	}
}
