package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
)

type StatusResponseWriter struct {
	http.ResponseWriter
	StatusCode   int
	BytesWritten int64
}

func NewStatusResponseWriter(w http.ResponseWriter) *StatusResponseWriter {
	return &StatusResponseWriter{
		ResponseWriter: w,
		StatusCode:     http.StatusOK,
	}
}

func (rw *StatusResponseWriter) WriteHeader(code int) {
	rw.StatusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *StatusResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.BytesWritten += int64(n)
	return n, err
}

func (rw *StatusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("underlying ResponseWriter does not support hijacking")
}

func (rw *StatusResponseWriter) Flush() {
	if fl, ok := rw.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}
