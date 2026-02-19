package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type processStatus struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid,omitempty"`
	Error   string `json:"error,omitempty"`
}

type statusPayload struct {
	OK        bool                     `json:"ok"`
	Time      string                   `json:"time"`
	Processes map[string]processStatus `json:"processes"`
}

func main() {
	listenAddr := envOrDefault("HEALTH_LISTEN_ADDR", ":8080")
	xrayPIDFile := envOrDefault("XRAY_PID_FILE", "/var/run/agent-svc-plus/xray.pid")
	agentPIDFile := envOrDefault("AGENT_PID_FILE", "/var/run/agent-svc-plus/agent.pid")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"service": "agent-runtime-healthz",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		processes := map[string]processStatus{
			"xray":  readProcessStatus(xrayPIDFile),
			"agent": readProcessStatus(agentPIDFile),
		}
		ready := processes["xray"].Running && processes["agent"].Running
		code := http.StatusOK
		if !ready {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, statusPayload{
			OK:        ready,
			Time:      time.Now().UTC().Format(time.RFC3339),
			Processes: processes,
		})
	})
	mux.HandleFunc("/debug/processes", func(w http.ResponseWriter, _ *http.Request) {
		processes := map[string]processStatus{
			"xray":  readProcessStatus(xrayPIDFile),
			"agent": readProcessStatus(agentPIDFile),
		}
		writeJSON(w, http.StatusOK, statusPayload{
			OK:        processes["xray"].Running && processes["agent"].Running,
			Time:      time.Now().UTC().Format(time.RFC3339),
			Processes: processes,
		})
	})

	log.Printf("health server listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		log.Fatal(err)
	}
}

func envOrDefault(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return v
}

func readProcessStatus(pidFile string) processStatus {
	content, err := os.ReadFile(pidFile)
	if err != nil {
		return processStatus{Running: false, Error: "pid file not found"}
	}
	pidStr := strings.TrimSpace(string(content))
	if pidStr == "" {
		return processStatus{Running: false, Error: "pid file empty"}
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return processStatus{Running: false, Error: "pid is not a number"}
	}

	if err := syscall.Kill(pid, 0); err != nil {
		return processStatus{Running: false, PID: pid, Error: err.Error()}
	}
	return processStatus{Running: true, PID: pid}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
