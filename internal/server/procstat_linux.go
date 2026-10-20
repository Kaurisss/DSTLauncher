//go:build linux

package server

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// /proc/<pid>/stat reports CPU time in USER_HZ. Linux exposes USER_HZ as 100
	// on the x86_64 platforms supported by the first Docker release.
	linuxClockTicks = 100

	// CPU is derived from two cumulative samples; very short intervals make the
	// value noisy when multiple browser tabs poll status at the same time.
	minSampleInterval = 900 * time.Millisecond
)

// queryProcess reads a Linux process's aggregate CPU time and resident memory
// directly from procfs, avoiding an extra runtime dependency.
func queryProcess(pid int) (cpu time.Duration, memBytes uint64, err error) {
	if pid <= 0 {
		return 0, 0, fmt.Errorf("PID 必须为正数: %d", pid)
	}

	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, 0, err
	}
	userTicks, systemTicks, err := processCPUTicks(string(stat))
	if err != nil {
		return 0, 0, err
	}

	statm, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(statm))
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("/proc/%d/statm 格式不完整", pid)
	}
	residentPages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("解析 /proc/%d/statm 失败: %w", pid, err)
	}

	return ticksDuration(userTicks + systemTicks), residentPages * uint64(os.Getpagesize()), nil
}

func processCPUTicks(stat string) (user, system uint64, err error) {
	// comm is wrapped in parentheses and may itself contain spaces or ')', so the
	// final ')' is the only safe separator before the fixed field sequence.
	closeParen := strings.LastIndex(stat, ")")
	if closeParen < 0 {
		return 0, 0, fmt.Errorf("/proc stat 缺少进程名结束标记")
	}
	fields := strings.Fields(stat[closeParen+1:])
	// Field 3 is fields[0]; utime/stime are fields 14/15, hence [11]/[12].
	if len(fields) < 13 {
		return 0, 0, fmt.Errorf("/proc stat 字段不足")
	}
	user, err = strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("解析 utime 失败: %w", err)
	}
	system, err = strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("解析 stime 失败: %w", err)
	}
	return user, system, nil
}

func ticksDuration(ticks uint64) time.Duration {
	seconds, remainder := ticks/linuxClockTicks, ticks%linuxClockTicks
	return time.Duration(seconds)*time.Second + time.Duration(remainder)*time.Second/linuxClockTicks
}

// procResource refreshes and returns memory plus CPU percentage. GOMAXPROCS is
// used instead of host CPU count because modern Go adjusts it to the container's
// CPU quota on Linux.
func (m *Manager) procResource(p *process) (uint64, float64) {
	cpu, mem, err := queryProcess(p.pid)
	if err != nil {
		return 0, 0
	}

	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()

	prev := p.res
	elapsed := now.Sub(prev.at)
	if !prev.at.IsZero() && elapsed < minSampleInterval {
		return mem, prev.pct
	}

	pct := 0.0
	if !prev.at.IsZero() && elapsed > 0 {
		delta := (cpu - prev.cpu).Seconds()
		if delta < 0 {
			delta = 0
		}
		workers := runtime.GOMAXPROCS(0)
		if workers < 1 {
			workers = 1
		}
		pct = delta / elapsed.Seconds() * 100 / float64(workers)
	}

	p.res = procSample{at: now, cpu: cpu, pct: pct}
	return mem, pct
}
