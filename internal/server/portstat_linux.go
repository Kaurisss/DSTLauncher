//go:build linux

package server

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// portOwnedBy checks the UDP socket inodes held by pid against Linux's network
// tables. Reading /proc does not bind or disturb the game port while a shard is
// starting.
func portOwnedBy(port, pid int) bool {
	if port <= 0 || port > 65535 || pid <= 0 {
		return false
	}

	inodes, err := processSocketInodes(pid)
	if err != nil || len(inodes) == 0 {
		return false
	}
	return udpTableHasSocket("/proc/net/udp", port, inodes) ||
		udpTableHasSocket("/proc/net/udp6", port, inodes)
}

func processSocketInodes(pid int) (map[string]struct{}, error) {
	entries, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "fd"))
	if err != nil {
		return nil, err
	}

	inodes := make(map[string]struct{})
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "fd", entry.Name()))
		if err != nil {
			continue
		}
		if !strings.HasPrefix(target, "socket:[") || !strings.HasSuffix(target, "]") {
			continue
		}
		inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
		if inode != "" {
			inodes[inode] = struct{}{}
		}
	}
	return inodes, nil
}

func udpTableHasSocket(path string, port int, inodes map[string]struct{}) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// /proc/net/udp{,6}: local_address is column 2 and inode is column 10.
		if len(fields) < 10 {
			continue
		}
		address := strings.Split(fields[1], ":")
		if len(address) != 2 {
			continue
		}
		value, err := strconv.ParseUint(address[1], 16, 16)
		if err != nil || int(value) != port {
			continue
		}
		if _, ok := inodes[fields[9]]; ok {
			return true
		}
	}
	return false
}
