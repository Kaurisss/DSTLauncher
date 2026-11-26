package server

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// logPath 返回分片日志路径。
// 必须带上集群标识：所有房间共用同一个 -conf_dir，
// 若只用 "<confDir>_<shard>" 命名，Cluster_1 与 Cluster_2 的 Master 日志会互相覆盖。
func (m *Manager) logPath(c *Cluster, shard string) string {
	return m.logPathFor(c.Key(), shard)
}

func (m *Manager) logPathFor(clusterKey, shard string) string {
	return filepath.Join(m.logDir, fmt.Sprintf("%s_%s.log", clusterKey, shard))
}

// LogTailer 是一个同时写入文件、保留内存尾部、并把每一行回调出去的分流器。
//
//   - 写文件：保留历史日志，供进程停止后仍可回看
//   - 内存环形缓冲：前端拉取最近日志时比读文件快
//   - onLine 回调：把服务器 stdout 的每一行实时广播给 Web 前端（SSE）
//
// 之所以要按行切分而不是原样透传：stdout 是字节流，一次 Write 可能包含
// 半行或好几行，直接推送会让前端出现半截断行。
type LogTailer struct {
	file    *os.File
	mu      sync.Mutex
	buf     []byte
	maxLen  int
	closed  bool
	onLine  func(string)
	partial []byte // 尚未收到换行符的残留行
}

const defaultTailSize = 512 * 1024 // 保留最近 512KB

// 单行超过该长度即丢弃，防止服务器输出无换行的刷屏内容把内存撑爆
const maxLineLen = 16 * 1024

// NewLogTailer 打开日志文件并创建尾随缓冲。onLine 可为 nil。
func NewLogTailer(path string, onLine func(string)) *LogTailer {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		// 日志目录创建失败不应阻断进程启动
		_ = err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		f = nil
	}
	return &LogTailer{file: f, maxLen: defaultTailSize, onLine: onLine}
}

// Write 实现 io.Writer：写文件 + 更新内存缓冲 + 按行回调。
func (l *LogTailer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		_, _ = l.file.Write(p)
	}

	l.buf = append(l.buf, p...)
	if len(l.buf) > l.maxLen {
		// 重新分配底层数组，避免切片不断右移导致底层数组无限增长
		l.buf = append([]byte(nil), l.buf[len(l.buf)-l.maxLen:]...)
	}

	if l.onLine != nil {
		l.partial = append(l.partial, p...)
		for {
			i := bytes.IndexByte(l.partial, '\n')
			if i < 0 {
				break
			}
			line := strings.TrimRight(string(l.partial[:i]), "\r")
			l.partial = append([]byte(nil), l.partial[i+1:]...)
			l.onLine(line)
		}
		if len(l.partial) > maxLineLen {
			// 超长且迟迟不换行，直接丢弃，避免内存堆积
			l.partial = nil
		}
	}
	return len(p), nil
}

// Tail 返回最近的 n 字节日志。
func (l *LogTailer) Tail(n int) string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if n > len(l.buf) {
		n = len(l.buf)
	}
	return string(l.buf[len(l.buf)-n:])
}

// Lines 返回最近 n 行日志。
func (l *LogTailer) Lines(n int) []string {
	tail := l.Tail(64 * 1024)
	all := strings.Split(strings.ReplaceAll(tail, "\r\n", "\n"), "\n")
	// 去掉末尾的空行
	for len(all) > 0 && strings.TrimSpace(all[len(all)-1]) == "" {
		all = all[:len(all)-1]
	}
	if n > 0 && n < len(all) {
		all = all[len(all)-n:]
	}
	return all
}

// Close 关闭日志文件。
func (l *LogTailer) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// ReadLog 从磁盘读取指定分片的最近 n 行日志（用于进程已停止的情况）。
func (m *Manager) ReadLog(clusterKey, shard string, n int) ([]string, error) {
	path := m.logPathFor(clusterKey, shard)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return lines, err
	}
	return lines, nil
}
