package app

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strings"
)

func (a *App) Router() http.Handler {
	return a.logMiddleware(a.corsMiddleware(http.HandlerFunc(a.handleRequest)))
}

func (a *App) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, Origin, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "600")
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *App) handleRequest(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			a.log.Error("panic", "error", rec)
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}()

	path := strings.Trim(r.URL.Path, "/")
	segments := []string{}
	if path != "" {
		segments = strings.Split(path, "/")
	}

	if r.Method == http.MethodGet && path == "healthz" {
		a.handleHealth(w, r)
		return
	}
	if r.Method == http.MethodGet && path == "readyz" {
		a.handleReady(w, r)
		return
	}
	if r.Method == http.MethodGet && path == "ws" {
		a.handleWebsocket(w, r)
		return
	}

	if len(segments) >= 1 && segments[0] == "api" {
		if len(segments) >= 2 && segments[1] == "v1" {
			a.handleV1(w, r, segments[2:])
			return
		}
		a.handleAPI(w, r, segments[1:])
		return
	}

	if len(segments) >= 1 && segments[0] == "ocpi" {
		a.handleOCPI(w, r, segments[1:])
		return
	}

	if len(segments) >= 1 && segments[0] == "ocpp" {
		a.handleOCPPRoute(w, r, segments[1:])
		return
	}

	if a.serveUIIfEnabled(w, r, path) {
		return
	}

	http.NotFound(w, r)
}

func (a *App) handleAPI(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 0 {
		http.NotFound(w, r)
		return
	}

	switch segments[0] {
	case "chargers":
		if len(segments) == 1 {
			switch r.Method {
			case http.MethodGet:
				a.handleListChargers(w, r)
			case http.MethodPost:
				a.handleCreateCharger(w, r)
			default:
				http.NotFound(w, r)
			}
			return
		}
		chargerID := segments[1]
		if len(segments) == 2 {
			switch r.Method {
			case http.MethodGet:
				a.handleGetCharger(w, r, chargerID)
			case http.MethodDelete:
				a.handleDeleteCharger(w, r, chargerID)
			default:
				http.NotFound(w, r)
			}
			return
		}
		if len(segments) == 3 && segments[2] == "sessions" && r.Method == http.MethodPost {
			a.handleStartSession(w, r, chargerID)
			return
		}
	case "sessions":
		if len(segments) == 3 {
			sessionID := segments[1]
			switch segments[2] {
			case "stop":
				if r.Method == http.MethodPost {
					a.handleStopSession(w, r, sessionID)
					return
				}
			case "meter":
				if r.Method == http.MethodPost {
					a.handleMeterValue(w, r, sessionID)
					return
				}
			}
		}
	}

	http.NotFound(w, r)
}

func (a *App) handleOCPI(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 0 {
		http.NotFound(w, r)
		return
	}

	if len(segments) == 1 && segments[0] == "versions" && r.Method == http.MethodGet {
		a.handleVersions(w, r)
		return
	}

	if segments[0] != "2.2.1" {
		http.NotFound(w, r)
		return
	}

	if len(segments) == 1 && r.Method == http.MethodGet {
		a.handleEndpoints(w, r)
		return
	}

	if len(segments) >= 2 {
		switch segments[1] {
		case "credentials":
			if len(segments) == 2 {
				switch r.Method {
				case http.MethodGet:
					a.handleGetCredentials(w, r)
				case http.MethodPost:
					a.handlePostCredentials(w, r)
				case http.MethodPut:
					a.handlePutCredentials(w, r)
				default:
					http.NotFound(w, r)
				}
				return
			}
		case "locations":
			if len(segments) == 2 && r.Method == http.MethodGet {
				a.handleLocations(w, r)
				return
			}
			if len(segments) == 3 && r.Method == http.MethodGet {
				a.handleLocation(w, r, segments[2])
				return
			}
			if len(segments) == 4 && r.Method == http.MethodGet {
				a.handleEvse(w, r, segments[2], segments[3])
				return
			}
			if len(segments) == 5 && r.Method == http.MethodGet {
				a.handleConnector(w, r, segments[2], segments[3], segments[4])
				return
			}
		case "tariffs":
			if len(segments) == 2 && r.Method == http.MethodGet {
				a.handleTariffs(w, r)
				return
			}
		case "sessions":
			if len(segments) == 2 {
				switch r.Method {
				case http.MethodGet:
					a.handleSessions(w, r)
				case http.MethodPost:
					a.handleCreateSession(w, r)
				}
				return
			}
			if len(segments) == 3 {
				sessionID := segments[2]
				switch r.Method {
				case http.MethodGet:
					a.handleSession(w, r, sessionID)
				case http.MethodPatch:
					a.handlePatchSession(w, r, sessionID)
				}
				return
			}
		case "cdrs":
			if len(segments) == 2 {
				switch r.Method {
				case http.MethodGet:
					a.handleCDRs(w, r)
				case http.MethodPost:
					a.handleCreateCDR(w, r)
				}
				return
			}
		case "commands":
			if len(segments) == 3 && r.Method == http.MethodPost {
				a.handleCommand(w, r, segments[2])
				return
			}
		}
	}

	http.NotFound(w, r)
}

func (a *App) handleOCPPRoute(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) != 2 {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	protocol := segments[0]
	chargePointID := segments[1]
	a.handleOCPP(w, r, protocol, chargePointID)
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	count, err := r.ResponseWriter.Write(data)
	r.bytes += count
	return count, err
}

func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}
	return hijacker.Hijack()
}

func (r *responseRecorder) Flush() {
	flusher, ok := r.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
}
