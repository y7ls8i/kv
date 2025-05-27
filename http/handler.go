package http

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/y7ls8i/kv/kv"
)

type LengthResponse struct {
	Length uint64 `json:"length"`
}

func lengthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(LengthResponse{Length: kv.Length()})
	if err != nil {
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
		return
	}
}

func setHandler(w http.ResponseWriter, r *http.Request, k string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	kv.Set(k, body)
	w.WriteHeader(http.StatusNoContent)
}

func clearHandler(w http.ResponseWriter, r *http.Request) {
	kv.Clear()
	w.WriteHeader(http.StatusNoContent)
}

type GetResponse struct {
	Value *string `json:"value,omitempty"`
	OK    bool    `json:"ok"`
}

func getHandler(w http.ResponseWriter, r *http.Request, k string) {
	var resp GetResponse
	v, ok := kv.Get(k)
	if ok {
		resp.OK = true
		resp.Value = addr(string(v))
	} else {
		resp.OK = false
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(resp)
	if err != nil {
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
		return
	}
}

func deleteHandler(w http.ResponseWriter, r *http.Request, k string) {
	kv.Delete(k)
	w.WriteHeader(http.StatusNoContent)
}

const (
	OperationAdd     = "ADD"
	OperationUpdate  = "UPDATE"
	OperationUDelete = "DELETE"
)

type SubscribeResponse struct {
	Operation string  `json:"operation"`
	Value     *string `json:"value,omitempty"`
}

func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	// Ensure the response writer is flushed after each event
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")

	if len(pathParts) < 3 || strings.TrimSpace(pathParts[2]) == "" {
		http.Error(w, "Key required!", http.StatusBadRequest)
		return
	}
	key := strings.TrimSpace(pathParts[2])

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	subID, ch := kv.Subscribe(key)
	log.Printf("Subscribed key=%q id=%q", key, subID)

	defer func(key, subID string) {
		kv.Unsubscribe(subID)
		log.Printf("Unsubscribed key=%q id=%q", key, subID)
	}(key, subID)

	for {
		select {
		case <-r.Context().Done():
			return
		case change := <-ch:
			var sendValue *string
			if len(change.Value) > 0 {
				sendValue = addr(string(change.Value))
			}
			send := SubscribeResponse{Value: sendValue}
			switch change.Op {
			case kv.OperationAdd:
				send.Operation = OperationAdd
			case kv.OperationUpdate:
				send.Operation = OperationUpdate
			case kv.OperationDelete:
				send.Operation = OperationUDelete
			}
			if err := json.NewEncoder(w).Encode(send); err != nil {
				http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
				return
			}
			flusher.Flush()
		}
	}
}

func NewServeMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/values/", func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) >= 3 {
			switch r.Method {
			case http.MethodGet:
				getHandler(w, r, pathParts[2])
				return
			case http.MethodPost:
				setHandler(w, r, pathParts[2])
				return
			case http.MethodDelete:
				if pathParts[2] == "" {
					clearHandler(w, r)
				} else {
					deleteHandler(w, r, pathParts[2])
				}
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/subscribe/", subscribeHandler)
	mux.HandleFunc("/length", lengthHandler)

	return mux
}

func addr[T any](value T) *T {
	return &value
}
