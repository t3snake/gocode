package openaisiwc

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type callbackResult struct {
	code      string
	client_id string
	err       error
}

type callbackListener struct {
	uri       string
	results   chan callbackResult
	server    *http.Server
	state     string
	client_id string
	mutex     sync.Mutex
	completed bool
}

func startCallbackListener(state, client_id string) (*callbackListener, error) {
	socket, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	listener := &callbackListener{
		uri:       "http://" + socket.Addr().String() + "/auth/callback",
		results:   make(chan callbackResult, 1),
		state:     state,
		client_id: client_id,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/callback", listener.receiveCallback)
	listener.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := listener.server.Serve(socket); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case listener.results <- callbackResult{err: err}:
			default:
			}
		}
	}()
	return listener, nil
}

func (listener *callbackListener) receiveCallback(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	// ServeMux also routes HEAD to a GET handler. Only GET can finish sign-in.
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "Use GET for the sign-in callback.", http.StatusMethodNotAllowed)
		return
	}
	query := request.URL.Query()
	if query.Get("state") != listener.state {
		http.Error(writer, "Invalid sign-in state.", http.StatusBadRequest)
		return
	}

	// A second callback must not complete this attempt again, even after its result is read.
	listener.mutex.Lock()
	defer listener.mutex.Unlock()
	if listener.completed {
		http.Error(writer, "Sign-in callback already received.", http.StatusConflict)
		return
	}
	listener.completed = true

	result := callbackResult{code: query.Get("code"), client_id: query.Get("client_id")}
	if query.Get("error") != "" {
		result.err = fmt.Errorf("ChatGPT sign-in: %q", query.Get("error"))
	} else if result.code == "" {
		result.err = fmt.Errorf("callback did not return an authorization code")
	} else if listener.client_id != "" {
		if result.client_id != "" && result.client_id != listener.client_id {
			result.err = fmt.Errorf("callback client ID does not match the saved registration")
		}
		result.client_id = listener.client_id
	} else if result.client_id == "" || result.client_id == "dynamic_agent_client" {
		result.err = fmt.Errorf("callback did not return an issued client ID")
	}

	if result.err != nil {
		http.Error(writer, "Sign-in failed. Return to gocode.", http.StatusBadRequest)
	} else {
		fmt.Fprintln(writer, "Authorization received. Return to gocode to finish sign-in.")
	}
	select {
	case listener.results <- result:
	default:
	}
}
