//go:build linux

package server

import (
	"os"
	"testing"
	"time"
)

func TestQueryProcessSelf(t *testing.T) {
	burnCPU(20 * time.Millisecond)

	cpu1, mem1, err := queryProcess(os.Getpid())
	if err != nil {
		t.Fatalf("采样失败: %v", err)
	}
	if mem1 == 0 {
		t.Error("内存占用为 0，驻留内存应非零")
	}
	if mem1 > 64<<30 {
		t.Errorf("内存占用异常: %d 字节", mem1)
	}
	if cpu1 <= 0 {
		t.Error("累计 CPU 时间应大于 0")
	}

	burnCPU(20 * time.Millisecond)
	cpu2, _, err := queryProcess(os.Getpid())
	if err != nil {
		t.Fatalf("二次采样失败: %v", err)
	}
	if cpu2 < cpu1 {
		t.Errorf("累计 CPU 时间倒退了: %v -> %v", cpu1, cpu2)
	}
}

func TestQueryProcessRejectsMissingPID(t *testing.T) {
	cpu, mem, err := queryProcess(0x7fffffff)
	if err == nil {
		t.Errorf("预期不存在的 PID 报错，得到 cpu=%v mem=%d", cpu, mem)
	}
}

func TestProcessCPUTicksHandlesParenthesesInName(t *testing.T) {
	stat := "123 (name with ) parentheses) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16"
	user, system, err := processCPUTicks(stat)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if user != 11 || system != 12 {
		t.Errorf("ticks = (%d, %d), want (11, 12)", user, system)
	}
}

func TestProcResourceThrottlesShortInterval(t *testing.T) {
	manager := &Manager{}
	process := &process{pid: os.Getpid(), state: StateRunning}

	_, first := manager.procResource(process)
	if first != 0 {
		t.Errorf("首次采样占用率应为 0，实际 %v", first)
	}
	_, second := manager.procResource(process)
	if second != 0 {
		t.Errorf("间隔过短时应复用上次结果（0），实际 %v", second)
	}

	process.mu.RLock()
	before := process.res.at
	process.mu.RUnlock()
	time.Sleep(minSampleInterval + 50*time.Millisecond)
	manager.procResource(process)

	process.mu.RLock()
	after := process.res.at
	process.mu.RUnlock()
	if !after.After(before) {
		t.Errorf("超出最小采样间隔后时间戳未推进: %v -> %v", before, after)
	}
}

func burnCPU(duration time.Duration) {
	deadline := time.Now().Add(duration)
	var value uint64
	for time.Now().Before(deadline) {
		value++
	}
	if value == 0 {
		panic("CPU loop did not run")
	}
}
