//go:build linux

package hoststats

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func readMemory() (Capacity, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Capacity{}, err
	}
	return parseMemory(string(data))
}

func parseMemory(data string) (Capacity, error) {
	values := make(map[string]uint64)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || (fields[0] != "MemTotal:" && fields[0] != "MemAvailable:") {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return Capacity{}, fmt.Errorf("/proc/meminfo 内存字段格式错误")
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || kb > ^uint64(0)/1024 {
			return Capacity{}, fmt.Errorf("/proc/meminfo 内存数值无效")
		}
		values[fields[0]] = kb * 1024
	}
	total, totalOK := values["MemTotal:"]
	available, availableOK := values["MemAvailable:"]
	if !totalOK || !availableOK || total == 0 || available > total {
		return Capacity{}, fmt.Errorf("/proc/meminfo 缺少有效的 MemTotal / MemAvailable")
	}
	return Capacity{TotalBytes: total, AvailableBytes: available}, nil
}

func readDisk(dir string) (Capacity, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return Capacity{}, err
	}
	return Capacity{
		TotalBytes:     stat.Blocks * uint64(stat.Bsize),
		AvailableBytes: stat.Bavail * uint64(stat.Bsize),
	}, nil
}
