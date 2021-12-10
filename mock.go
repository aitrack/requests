package requests

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
)

type MockResponseWriter struct {
	header http.Header

	writer io.Writer
}

func (m *MockResponseWriter) Header() http.Header {
	return m.header
}

func (m *MockResponseWriter) Write(b []byte) (int, error) {
	return m.writer.Write(b)
}

func (m *MockResponseWriter) WriteHeader(statusCode int) {
}

func NewMockResponseWriter(buf *bytes.Buffer) http.ResponseWriter {
	result := MockResponseWriter{header: make(http.Header), writer: buf}

	return &result
}

func NewMockRequest(params map[string]string) *http.Request {
	u := "http://temp"
	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Add(k, v)
		}
		u = u + "?" + values.Encode()
	}

	result, _ := http.NewRequest("", u, nil)
	return result
}
