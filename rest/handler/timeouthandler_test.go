package handler

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/rest/internal/response"
)

func init() {
	log.SetOutput(io.Discard)
}

func TestTimeout(t *testing.T) {
	timeoutHandler := TimeoutHandler(time.Millisecond)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Minute)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
}

func TestWithinTimeout(t *testing.T) {
	timeoutHandler := TimeoutHandler(time.Second)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Millisecond)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestWithTimeoutTimedout(t *testing.T) {
	timeoutHandler := TimeoutHandler(time.Millisecond)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Millisecond * 10)
		_, err := w.Write([]byte(`foo`))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
}

func TestWithoutTimeout(t *testing.T) {
	timeoutHandler := TimeoutHandler(0)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestTimeoutPanic(t *testing.T) {
	timeoutHandler := TimeoutHandler(time.Minute)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("foo")
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	resp := httptest.NewRecorder()
	assert.Panics(t, func() {
		handler.ServeHTTP(resp, req)
	})
}

func TestTimeoutWebsocket(t *testing.T) {
	timeoutHandler := TimeoutHandler(time.Millisecond)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Millisecond * 10)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	req.Header.Set(headerUpgrade, valueWebsocket)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestTimeoutWroteHeaderTwice(t *testing.T) {
	timeoutHandler := TimeoutHandler(time.Minute)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(`hello`))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("foo", "bar")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestTimeoutWriteBadCode(t *testing.T) {
	timeoutHandler := TimeoutHandler(time.Minute)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(1000)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	resp := httptest.NewRecorder()
	assert.Panics(t, func() {
		handler.ServeHTTP(resp, req)
	})
}

func TestTimeoutClientClosed(t *testing.T) {
	timeoutHandler := TimeoutHandler(time.Minute)
	handler := timeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	cancel()
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(t, statusClientClosedRequest, resp.Code)
}

func TestTimeoutHijack(t *testing.T) {
	resp := httptest.NewRecorder()

	writer := &timeoutWriter{
		w: &response.WithCodeResponseWriter{
			Writer: resp,
		},
	}

	assert.NotPanics(t, func() {
		_, _, _ = writer.Hijack()
	})

	writer = &timeoutWriter{
		w: &response.WithCodeResponseWriter{
			Writer: mockedHijackable{resp},
		},
	}

	assert.NotPanics(t, func() {
		_, _, _ = writer.Hijack()
	})
}

func TestTimeoutPusher(t *testing.T) {
	handler := &timeoutWriter{
		w: mockedPusher{},
	}

	assert.Panics(t, func() {
		_ = handler.Push("any", nil)
	})

	handler = &timeoutWriter{
		w: httptest.NewRecorder(),
	}
	assert.Equal(t, http.ErrNotSupported, handler.Push("any", nil))
}

func TestTimeoutWroteTwice(t *testing.T) {
	writer := &timeoutWriter{
		w: &response.WithCodeResponseWriter{
			Writer: httptest.NewRecorder(),
		},
		h:   make(http.Header),
		req: httptest.NewRequest(http.MethodGet, "http://localhost", http.NoBody),
	}
	writer.writeHeaderLocked(http.StatusOK)
	writer.writeHeaderLocked(http.StatusOK)
}

type mockedPusher struct{}

func (m mockedPusher) Header() http.Header {
	panic("implement me")
}

func (m mockedPusher) Write(_ []byte) (int, error) {
	panic("implement me")
}

func (m mockedPusher) WriteHeader(_ int) {
	panic("implement me")
}

func (m mockedPusher) Push(_ string, _ *http.PushOptions) error {
	panic("implement me")
}
