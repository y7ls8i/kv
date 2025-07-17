package http

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/y7ls8i/kv/kv"
)

type (
	Value          string // base64 encoded string of binary
	LengthResponse struct {
		Length uint64 `json:"length"`
	}
	GetResponse struct {
		Value Value `json:"value"`
		OK    bool  `json:"ok"`
	}
	SubscribeResponse struct {
		Operation string `json:"operation"`
		Value     Value  `json:"value"`
	}
)

const (
	OperationAdd     = "ADD"
	OperationUpdate  = "UPDATE"
	OperationUDelete = "DELETE"
)

func lengthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")

	if len(pathParts) < 3 || strings.TrimSpace(pathParts[2]) == "" {
		http.Error(w, "Name required!", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(pathParts[2])

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(LengthResponse{Length: kv.Length(name)})
	if err != nil {
		http.Error(w, "Failed to encode JSON response", http.StatusInternalServerError)
		return
	}
}

func setHandler(w http.ResponseWriter, r *http.Request, name, key string) {
	defer func() {
		_ = r.Body.Close()
	}()

	body, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, r.Body))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusInternalServerError)
		return
	}
	kv.Set(name, key, body)
	w.WriteHeader(http.StatusNoContent)
}

func clearHandler(w http.ResponseWriter, r *http.Request, name string) {
	kv.Clear(name)
	w.WriteHeader(http.StatusNoContent)
}

func getHandler(w http.ResponseWriter, r *http.Request, name, key string) {
	var resp GetResponse
	v, ok := kv.Get(name, key)
	if ok {
		resp.OK = true
		resp.Value = Value(base64.StdEncoding.EncodeToString(v))
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

func deleteHandler(w http.ResponseWriter, r *http.Request, name, key string) {
	kv.Delete(name, key)
	w.WriteHeader(http.StatusNoContent)
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
		http.Error(w, "Name required!", http.StatusBadRequest)
		return
	}
	if len(pathParts) < 4 || strings.TrimSpace(pathParts[3]) == "" {
		http.Error(w, "Key required!", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(pathParts[2])
	key := strings.TrimSpace(pathParts[3])

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	subID, ch := kv.Subscribe(name, key)
	log.Printf("Subscribed name=%q key=%q id=%q", name, key, subID)

	defer func(name, key, subID string) {
		kv.Unsubscribe(name, subID)
		log.Printf("Unsubscribed name=%q key=%q id=%q", name, key, subID)
	}(name, key, subID)

	for {
		select {
		case <-r.Context().Done():
			return
		case change := <-ch:
			send := SubscribeResponse{Value: Value(base64.StdEncoding.EncodeToString(change.Value))}
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
		if len(pathParts) >= 4 {
			switch r.Method {
			case http.MethodGet:
				if pathParts[2] != "" && pathParts[3] != "" {
					getHandler(w, r, pathParts[2], pathParts[3])
					return
				}
			case http.MethodPost:
				if pathParts[2] != "" && pathParts[3] != "" {
					setHandler(w, r, pathParts[2], pathParts[3])
					return
				}
			case http.MethodDelete:
				if pathParts[2] != "" {
					if pathParts[3] == "" {
						clearHandler(w, r, pathParts[2])
					} else {
						deleteHandler(w, r, pathParts[2], pathParts[3])
					}
					return
				}
			}
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	})
	mux.HandleFunc("/subscribe/", subscribeHandler)
	mux.HandleFunc("/length/", lengthHandler)

	return mux
}
