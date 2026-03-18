package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/webssh/manager/internal/middleware"
	sshutil "github.com/webssh/manager/internal/ssh"
	"github.com/webssh/manager/pkg/crypto"
	"golang.org/x/crypto/ssh"
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

	var host, username, encryptedCreds, authType string
	var port, ownerID int
	err = h.db.QueryRow(`
		SELECT n.host, n.port, n.username, c.auth_type, c.encrypted_value, n.user_id
		FROM nodes n
		JOIN credentials c ON n.credential_id = c.id
		WHERE n.id = ?`, nodeID).Scan(&host, &port, &username, &authType, &encryptedCreds, &ownerID)
	if err == sql.ErrNoRows {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if ownerID != user.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	key, err := base64.StdEncoding.DecodeString(user.EncryptionKey)
	if err != nil {
		http.Error(w, "Key error", http.StatusInternalServerError)
		return
	}

	creds, err := crypto.Decrypt(encryptedCreds, key)
	if err != nil {
		http.Error(w, "Decrypt error", http.StatusInternalServerError)
		return
	}

	authMethods, err := sshutil.BuildAuthMethod(authType, creds)
	if err != nil {
		http.Error(w, "Auth method error", http.StatusInternalServerError)
		return
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
	})
	if err != nil {
		http.Error(w, "SSH connection failed", http.StatusBadGateway)
		return
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		http.Error(w, "Session error", http.StatusBadGateway)
		return
	}
	defer sess.Close()

	cmd := `echo "HOSTNAME=$(hostname)" && ` +
		`echo "OS=$(cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d= -f2 | tr -d '"' || uname -s)" && ` +
		`echo "KERNEL=$(uname -r)" && ` +
		`echo "UPTIME=$(uptime -p 2>/dev/null || uptime)" && ` +
		`echo "CPU=$(cat /proc/cpuinfo 2>/dev/null | grep 'model name' | head -1 | cut -d: -f2 | xargs || sysctl -n machdep.cpu.brand_string 2>/dev/null || echo unknown)" && ` +
		`echo "CORES=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 0)" && ` +
		`echo "LOAD=$(cat /proc/loadavg 2>/dev/null | awk '{print $1\" \"$2\" \"$3}' || sysctl -n vm.loadavg 2>/dev/null | tr -d '{}' | xargs || echo 0)" && ` +
		`free -m 2>/dev/null | awk '/^Mem:/{print "MEM_TOTAL="$2" MEM_USED="$3}' || echo "MEM_TOTAL=0 MEM_USED=0" && ` +
		`df -h / 2>/dev/null | awk 'NR==2{print "DISK_TOTAL="$2" DISK_USED="$3" DISK_PCT="$5}' || echo "DISK_TOTAL=0 DISK_USED=0 DISK_PCT=0" && ` +
		`echo "IP=$(hostname -I 2>/dev/null | awk '{print $1}' || ipconfig getifaddr en0 2>/dev/null || echo unknown)"`

	out, err := sess.Output(cmd)
	if err != nil {
		// Return minimal info on command failure
		info := SysInfoResponse{Hostname: host, OS: "unknown", IPAddr: host}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info) //nolint:errcheck
		return
	}

	info := SysInfoResponse{}
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
						parseSysInfoKV(&info, kv[0], kv[1])
					}
				}
				continue
			}
		}

		kv := strings.SplitN(line, "=", 2)
		if len(kv) == 2 {
			parseSysInfoKV(&info, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info) //nolint:errcheck
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
	}
}
