package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/webssh/manager/internal/middleware"
	gossh "golang.org/x/crypto/ssh"
)

type SysInfoHandler struct {
	db *sql.DB
}

func NewSysInfoHandler(db *sql.DB) *SysInfoHandler {
	return &SysInfoHandler{db: db}
}

type SysInfoResponse struct {
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Kernel    string `json:"kernel"`
	Uptime    string `json:"uptime"`
	CPUModel  string `json:"cpu_model"`
	CPUCores  int    `json:"cpu_cores"`
	LoadAvg   string `json:"load_avg"`
	MemTotal  int64  `json:"mem_total"`
	MemUsed   int64  `json:"mem_used"`
	DiskTotal string `json:"disk_total"`
	DiskUsed  string `json:"disk_used"`
	DiskPct   string `json:"disk_pct"`
	IPAddr    string `json:"ip_addr"`
	GPUModel  string `json:"gpu_model"`
	Stale     bool   `json:"stale,omitempty"`
}

// sysInfoShellCmd is the shell command used to collect system information remotely.
const sysInfoShellCmd = `echo "HOSTNAME=$(hostname)" && ` +
	`echo "OS=$(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d= -f2 | tr -d '"' || uname -s)" && ` +
	`echo "KERNEL=$(uname -r)" && ` +
	`echo "UPTIME=$(uptime -p 2>/dev/null || uptime)" && ` +
	`echo "CPU=$(cat /proc/cpuinfo 2>/dev/null | grep 'model name' | head -1 | cut -d: -f2 | xargs || sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown)" && ` +
	`echo "CORES=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 0)" && ` +
	`echo "LOAD=$(cat /proc/loadavg 2>/dev/null | awk '{print $1" "$2" "$3}' || sysctl -n vm.loadavg 2>/dev/null | tr -d '{}' | xargs || echo 0)" && ` +
	`if [ "$(uname)" = "Darwin" ]; then ` +
	`MEM_TOTAL=$(($(sysctl -n hw.memsize) / 1024 / 1024)) && ` +
	`PAGESIZE=$(sysctl -n hw.pagesize) && ` +
	`FREE_PAGES=$(vm_stat | grep "Pages free:" | awk '{print $3}' | tr -d '.') && ` +
	`SPEC_PAGES=$(vm_stat | grep "Pages speculative:" | awk '{print $3}' | tr -d '.') && ` +
	`FREE_MEM=$((($FREE_PAGES + $SPEC_PAGES) * $PAGESIZE / 1024 / 1024)) && ` +
	`MEM_USED=$(($MEM_TOTAL - $FREE_MEM)) && ` +
	`echo "MEM_TOTAL=$MEM_TOTAL MEM_USED=$MEM_USED"; ` +
	`else ` +
	`free -m 2>/dev/null | awk '/^Mem:/{print "MEM_TOTAL="$2" MEM_USED="$3}' || echo "MEM_TOTAL=0 MEM_USED=0"; ` +
	`fi && ` +
	`df -h / 2>/dev/null | awk 'NR==2{print "DISK_TOTAL="$2" DISK_USED="$3" DISK_PCT="$5}' | tr -d '%' || echo "DISK_TOTAL=0 DISK_USED=0 DISK_PCT=0" && ` +
	`echo "IP=$(hostname -I 2>/dev/null | awk '{print $1}' || ifconfig 2>/dev/null | grep 'inet ' | grep -v 127.0.0.1 | awk '{print $2}' | head -n 1 || echo unknown)" && ` +
	`echo "GPU=$(system_profiler SPDisplaysDataType 2>/dev/null | grep 'Chipset Model' | cut -d: -f2 | xargs | head -n 1 || lspci 2>/dev/null | grep -i -E 'vga|3d|2d' | cut -d: -f3 | xargs | head -n 1 || nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -n 1 || echo unknown)"`

// collectSysInfo connects via an existing SSH client and collects system info.
// Returns the data or an error; does not write to DB.
func collectSysInfo(client *gossh.Client) (*SysInfoResponse, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("open SSH session: %w", err)
	}
	defer sess.Close()

	out, err := sess.Output(sysInfoShellCmd)
	if err != nil {
		return nil, fmt.Errorf("run sysinfo command: %w", err)
	}

	info := &SysInfoResponse{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle lines with multiple KEY=VALUE pairs (e.g. "MEM_TOTAL=X MEM_USED=Y")
		if strings.Contains(line, " ") && strings.Contains(line, "=") {
			tokens := strings.Fields(line)
			allKV := true
			for _, t := range tokens {
				if !strings.Contains(t, "=") {
					allKV = false
					break
				}
			}
			if allKV {
				for _, t := range tokens {
					kv := strings.SplitN(t, "=", 2)
					if len(kv) == 2 {
						parseSysInfoKV(info, kv[0], kv[1])
					}
				}
				continue
			}
		}

		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			parseSysInfoKV(info, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}

	return info, nil
}

// collectLocalSysInfo executes the sysInfoShellCmd locally on the server.
func collectLocalSysInfo() (*SysInfoResponse, error) {
	cmd := exec.Command("/bin/sh", "-c", sysInfoShellCmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("run local sysinfo command: %w", err)
	}

	info := &SysInfoResponse{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Handle lines with multiple KEY=VALUE pairs (e.g. "MEM_TOTAL=X MEM_USED=Y")
		if strings.Contains(line, " ") && strings.Contains(line, "=") {
			tokens := strings.Fields(line)
			allKV := true
			for _, t := range tokens {
				if !strings.Contains(t, "=") {
					allKV = false
					break
				}
			}
			if allKV {
				for _, t := range tokens {
					kv := strings.SplitN(t, "=", 2)
					if len(kv) == 2 {
						parseSysInfoKV(info, kv[0], kv[1])
					}
				}
				continue
			}
		}

		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			parseSysInfoKV(info, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}

	return info, nil
}

// collectAndStoreSysInfo connects to the node described by routing, collects
// system info, and persists it to the node_sysinfo table. Intended to be
// called asynchronously after node creation. Errors are logged and silently
// ignored.
func collectAndStoreSysInfo(db *sql.DB, nodeID int, routing *nodeSSHRouting) {
	if routing.DialConfig.TargetHost == "localhost-shell" {
		info, err := collectLocalSysInfo()
		if err != nil {
			log.Printf("sysinfo collect node %d (local): %v", nodeID, err)
			return
		}
		if _, err := db.Exec(`
			INSERT OR REPLACE INTO node_sysinfo
				(node_id, hostname, os, kernel, uptime, cpu_model, cpu_cores, load_avg,
				 mem_total, mem_used, disk_total, disk_used, disk_pct, ip_addr, gpu_model, collected_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			nodeID,
			info.Hostname, info.OS, info.Kernel, info.Uptime,
			info.CPUModel, info.CPUCores, info.LoadAvg,
			info.MemTotal, info.MemUsed,
			info.DiskTotal, info.DiskUsed, info.DiskPct,
			info.IPAddr, info.GPUModel,
		); err != nil {
			log.Printf("sysinfo collect node %d (local): persist: %v", nodeID, err)
		}
		return
	}

	client, err := dialNodeSSH(routing)
	if err != nil {
		log.Printf("sysinfo collect node %d: dial: %v", nodeID, err)
		return
	}
	defer client.Close()

	info, err := collectSysInfo(client)
	if err != nil {
		log.Printf("sysinfo collect node %d: collect: %v", nodeID, err)
		return
	}

	if _, err := db.Exec(`
		INSERT OR REPLACE INTO node_sysinfo
			(node_id, hostname, os, kernel, uptime, cpu_model, cpu_cores, load_avg,
			 mem_total, mem_used, disk_total, disk_used, disk_pct, ip_addr, gpu_model, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		nodeID,
		info.Hostname, info.OS, info.Kernel, info.Uptime,
		info.CPUModel, info.CPUCores, info.LoadAvg,
		info.MemTotal, info.MemUsed,
		info.DiskTotal, info.DiskUsed, info.DiskPct,
		info.IPAddr, info.GPUModel,
	); err != nil {
		log.Printf("sysinfo collect node %d: persist: %v", nodeID, err)
	}
}

func (h *SysInfoHandler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// path: /api/nodes/{id}/sysinfo → parts: ["api","nodes","{id}","sysinfo"]
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	nodeID, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	// Verify the node exists and belongs to the authenticated user before
	// loading any cached data or attempting an SSH refresh.
	var ownerID int
	var host string
	if err := h.db.QueryRow("SELECT user_id, host FROM nodes WHERE id = ?", nodeID).Scan(&ownerID, &host); err == sql.ErrNoRows {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if ownerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Load cached data from node_sysinfo table.
	var cached *SysInfoResponse
	var cachedRow SysInfoResponse
	scanErr := h.db.QueryRow(`
		SELECT hostname, os, kernel, uptime, cpu_model, cpu_cores, load_avg,
		       mem_total, mem_used, disk_total, disk_used, disk_pct, ip_addr, gpu_model
		FROM node_sysinfo
		WHERE node_id = ?`, nodeID).Scan(
		&cachedRow.Hostname, &cachedRow.OS, &cachedRow.Kernel, &cachedRow.Uptime,
		&cachedRow.CPUModel, &cachedRow.CPUCores, &cachedRow.LoadAvg,
		&cachedRow.MemTotal, &cachedRow.MemUsed,
		&cachedRow.DiskTotal, &cachedRow.DiskUsed, &cachedRow.DiskPct,
		&cachedRow.IPAddr, &cachedRow.GPUModel,
	)
	if scanErr == nil {
		cached = &cachedRow
	}

	if host == "localhost-shell" {
		info, err := collectLocalSysInfo()
		if err != nil {
			if cached != nil {
				cached.Stale = true
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(cached) //nolint:errcheck
				return
			}
			http.Error(w, "Local sysinfo collection failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Persist refreshed data.
		_, dbErr := h.db.Exec(`
			INSERT OR REPLACE INTO node_sysinfo
				(node_id, hostname, os, kernel, uptime, cpu_model, cpu_cores, load_avg,
				 mem_total, mem_used, disk_total, disk_used, disk_pct, ip_addr, gpu_model, collected_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			nodeID,
			info.Hostname, info.OS, info.Kernel, info.Uptime,
			info.CPUModel, info.CPUCores, info.LoadAvg,
			info.MemTotal, info.MemUsed,
			info.DiskTotal, info.DiskUsed, info.DiskPct,
			info.IPAddr, info.GPUModel,
		)
		if dbErr != nil {
			log.Printf("sysinfo Get (local): persist refreshed sysinfo for node %d: %v", nodeID, dbErr)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info) //nolint:errcheck
		return
	}

	key, err := base64.StdEncoding.DecodeString(user.EncryptionKey)
	if err != nil {
		// Key error — fall back to cache if available
		if cached != nil {
			cached.Stale = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cached) //nolint:errcheck
			return
		}
		http.Error(w, "Key error", http.StatusInternalServerError)
		return
	}

	routing, err := loadNodeSSHRouting(h.db, nodeID, user.ID, key)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Node not found", http.StatusNotFound)
			return
		}
		if cached != nil {
			cached.Stale = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cached) //nolint:errcheck
			return
		}
		http.Error(w, "Failed to load node configuration", http.StatusInternalServerError)
		return
	}

	client, err := dialNodeSSH(routing)
	if err != nil {
		// SSH failed — return stale cache if available, otherwise error.
		if cached != nil {
			cached.Stale = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cached) //nolint:errcheck
			return
		}
		http.Error(w, "SSH connection failed", http.StatusBadGateway)
		return
	}
	defer client.Close()

	info, err := collectSysInfo(client)
	if err != nil {
		// Command failure — return stale cache if available, otherwise minimal info.
		if cached != nil {
			cached.Stale = true
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cached) //nolint:errcheck
			return
		}
		minimal := SysInfoResponse{Hostname: routing.DialConfig.TargetHost, OS: "unknown", IPAddr: routing.DialConfig.TargetHost}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(minimal) //nolint:errcheck
		return
	}

	// Persist refreshed data.
	_, dbErr := h.db.Exec(`
		INSERT OR REPLACE INTO node_sysinfo
			(node_id, hostname, os, kernel, uptime, cpu_model, cpu_cores, load_avg,
			 mem_total, mem_used, disk_total, disk_used, disk_pct, ip_addr, gpu_model, collected_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		nodeID,
		info.Hostname, info.OS, info.Kernel, info.Uptime,
		info.CPUModel, info.CPUCores, info.LoadAvg,
		info.MemTotal, info.MemUsed,
		info.DiskTotal, info.DiskUsed, info.DiskPct,
		info.IPAddr, info.GPUModel,
	)
	if dbErr != nil {
		log.Printf("sysinfo Get: persist refreshed sysinfo for node %d: %v", nodeID, dbErr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info) //nolint:errcheck
}

// GetCached returns the last-cached sysinfo for a node without performing a
// live SSH refresh. Returns 404 if no cached data exists.
func (h *SysInfoHandler) GetCached(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// path: /api/nodes/{id}/sysinfo/cached
	if len(parts) < 4 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	nodeID, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid node ID", http.StatusBadRequest)
		return
	}

	// Verify ownership
	var ownerID int
	if err := h.db.QueryRow("SELECT user_id FROM nodes WHERE id = ?", nodeID).Scan(&ownerID); err == sql.ErrNoRows {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if ownerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Return cached row or 404
	var cached SysInfoResponse
	err = h.db.QueryRow(`
		SELECT hostname, os, kernel, uptime, cpu_model, cpu_cores, load_avg,
		       mem_total, mem_used, disk_total, disk_used, disk_pct, ip_addr, gpu_model
		FROM node_sysinfo WHERE node_id = ?`, nodeID).Scan(
		&cached.Hostname, &cached.OS, &cached.Kernel, &cached.Uptime,
		&cached.CPUModel, &cached.CPUCores, &cached.LoadAvg,
		&cached.MemTotal, &cached.MemUsed,
		&cached.DiskTotal, &cached.DiskUsed, &cached.DiskPct, &cached.IPAddr, &cached.GPUModel,
	)
	if err == sql.ErrNoRows {
		http.Error(w, "No cached data", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cached) //nolint:errcheck
}

func parseSysInfoKV(info *SysInfoResponse, k, v string) {
	switch k {
	case "HOSTNAME":
		info.Hostname = v
	case "OS":
		info.OS = v
	case "KERNEL":
		info.Kernel = v
	case "UPTIME":
		info.Uptime = v
	case "CPU":
		info.CPUModel = v
	case "CORES":
		info.CPUCores, _ = strconv.Atoi(v)
	case "LOAD":
		info.LoadAvg = v
	case "MEM_TOTAL":
		info.MemTotal, _ = strconv.ParseInt(v, 10, 64)
	case "MEM_USED":
		info.MemUsed, _ = strconv.ParseInt(v, 10, 64)
	case "DISK_TOTAL":
		info.DiskTotal = v
	case "DISK_USED":
		info.DiskUsed = v
	case "DISK_PCT":
		info.DiskPct = v
	case "IP":
		info.IPAddr = v
	case "GPU":
		info.GPUModel = v
	}
}
