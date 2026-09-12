package servers

import (
	"strconv"
	"strings"
)

// metricsScript mengumpulkan vital sign server dalam SATU SSH round-trip
// (bukan satu perintah per metrik) — sama seperti homepoin, dipertahankan
// untuk paritas field yang ditampilkan. `top -bn1` butuh ~1 detik sampling;
// ini biaya yang sengaja diterima demi akurasi CPU%, tapi lihat catatan di
// Collector kenapa panggilan ini TIDAK dijalankan untuk semua server setiap
// tick — hanya untuk server yang statusnya sudah diketahui online & sedang
// "diperhatikan".
const metricsScript = `echo -n "METRICS:"; \
echo -n "OS_NAME=$(grep PRETTY_NAME /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"' | sed 's/[;=]//g' | xargs);"; \
echo -n "HOSTNAME=$(hostname -s 2>/dev/null || uname -n | cut -d. -f1);"; \
echo -n "UPTIME=$(cut -d. -f1 /proc/uptime 2>/dev/null);"; \
echo -n "MEM_USED=$(free -m | awk 'NR==2{print $3}');"; \
echo -n "MEM_TOTAL=$(free -m | awk 'NR==2{print $2}');"; \
echo -n "DISK_USED=$(df -BG / 2>/dev/null | awk 'NR==2{gsub(/G/,"",$3); print $3}');"; \
echo -n "DISK_TOTAL=$(df -BG / 2>/dev/null | awk 'NR==2{gsub(/G/,"",$2); print $2}');"; \
echo -n "CPU_CORES=$(nproc 2>/dev/null || echo 1);"; \
echo "CPU_USED_PCT=$(top -bn1 | grep '%Cpu' | sed -n 's/.*, *\([0-9.]*\)%* id.*/\1/p' | awk 'NF{printf "%.0f", 100-$1; exit} END{if(!NR) print 0}')"`

// applyMetricsOutput mengisi ServerStatus dari output script metrik remote.
func applyMetricsOutput(st *ServerStatus, stdout string) {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "METRICS:") {
			continue
		}
		payload := strings.TrimPrefix(line, "METRICS:")
		for _, part := range strings.Split(payload, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			key, val := kv[0], strings.TrimSpace(kv[1])
			switch key {
			case "OS_NAME":
				st.OSName = val
			case "HOSTNAME":
				st.Hostname = val
			case "UPTIME":
				st.UptimeSeconds = parseInt64(val)
			case "MEM_USED":
				st.MemUsedMB = parseInt64(val)
			case "MEM_TOTAL":
				st.MemTotalMB = parseInt64(val)
			case "DISK_USED":
				st.DiskUsedGB = parseFloat64(val)
			case "DISK_TOTAL":
				st.DiskTotalGB = parseFloat64(val)
			case "CPU_CORES":
				st.CPUCores = int(parseInt64(val))
			case "CPU_USED_PCT":
				st.CPUUsedPct = parseFloat64(val)
			}
		}
	}
}

func parseInt64(raw string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return n
}

func parseFloat64(raw string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return n
}
