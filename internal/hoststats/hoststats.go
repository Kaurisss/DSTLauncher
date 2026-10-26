// Package hoststats samples host memory and the filesystem holding the save directory.
package hoststats

import (
	"fmt"
	"os"
	"path/filepath"
)

// Capacity uses bytes available to this process, including filesystem quotas.
type Capacity struct {
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
}

type Snapshot struct {
	ContainerMemory      *Capacity `json:"containerMemory,omitempty"`
	ContainerMemoryError string    `json:"containerMemoryError,omitempty"`
	Memory               *Capacity `json:"memory"`
	Disk                 *Capacity `json:"disk"`
	DiskPath             string    `json:"diskPath"`
	MemoryError          string    `json:"memoryError,omitempty"`
	DiskError            string    `json:"diskError,omitempty"`
}

// Read keeps independent failures separate so a disk error never hides memory.
func Read(saveDir string) Snapshot {
	result := Snapshot{DiskPath: saveDir}
	container, containerErr := readContainerMemory()
	if containerErr != nil {
		result.ContainerMemoryError = containerErr.Error()
	} else {
		result.ContainerMemory = container
	}
	memory, err := readMemory()
	if err != nil {
		result.MemoryError = err.Error()
	} else {
		result.Memory = &memory
	}
	dir, err := existingDir(saveDir)
	if err == nil {
		var disk Capacity
		disk, err = readDisk(dir)
		if err == nil {
			result.Disk = &disk
		}
	}
	if err != nil {
		result.DiskError = err.Error()
	}
	return result
}

// A new installation may not have a save directory yet; do not create it to sample.
func existingDir(path string) (string, error) {
	dir, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for {
		info, err := os.Stat(dir)
		if err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("存档路径不是目录: %s", dir)
			}
			return dir, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		dir = parent
	}
}
