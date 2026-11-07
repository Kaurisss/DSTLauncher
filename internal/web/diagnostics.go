package web

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"dst-windows/internal/diagnostics"
)

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	id, err := strconv.Atoi(query(r, "room"))
	rm, ok := s.rooms.Get(id)
	if err != nil || !ok {
		respond(w, 200, nil, fmt.Errorf("请指定有效房间"))
		return
	}
	type source struct {
		Shard    string                `json:"shard"`
		State    string                `json:"state"`
		Findings []diagnostics.Finding `json:"findings"`
		Error    string                `json:"error,omitempty"`
	}
	result := struct {
		Time      time.Time             `json:"time"`
		Room      int                   `json:"room"`
		Checks    []diagnostics.Finding `json:"checks"`
		Shards    []source              `json:"shards"`
		Downloads source                `json:"downloads"`
	}{Time: time.Now(), Room: id, Checks: []diagnostics.Finding{}, Shards: []source{}}
	if rm.NeedsToken() && !rm.HasToken() {
		result.Checks = append(result.Checks, diagnostics.Finding{Code: "missing_token", Stage: "配置检查", Title: "尚未填写服务器令牌", Suggestion: "在房间设置中填写 Klei 服务器令牌后再启动在线房间。", Evidence: []string{"在线房间的服务器令牌为空"}, Mods: []diagnostics.Mod{}})
	}
	redact := func(lines []string) []string { return diagnostics.Redact(lines, rm.Token, rm.Password, rm.ClusterKey) }
	enrich := func(findings []diagnostics.Finding) {
		mods := s.modsFor(rm)
		for i := range findings {
			for j := range findings[i].Mods {
				name, _, _ := mods.ModInfo(findings[i].Mods[j].ID)
				findings[i].Mods[j].Name = name
			}
		}
	}
	states := map[string]string{}
	for _, st := range s.mgr.Status(rm.Key()) {
		states[st.Shard] = st.State
	}
	for _, shard := range rm.Shards {
		item := source{Shard: shard.Name, State: states[shard.Name], Findings: []diagnostics.Finding{}}
		if message := s.mgr.StartError(rm.Key(), shard.Name); message != "" {
			item.Findings = append(item.Findings, diagnostics.Finding{Code: "start_failed", Stage: "进程创建", Title: "最近一次启动失败", Suggestion: "检查服务器文件是否完整、安装路径是否有效，以及面板是否具有目录和执行权限。", Evidence: redact([]string{message}), Mods: []diagnostics.Mod{}})
		}
		lines, err := s.mgr.ReadLog(rm.Key(), shard.Name, 400)
		if err != nil {
			if !os.IsNotExist(err) {
				item.Error = "读取最近日志失败: " + err.Error()
			}
		} else {
			item.Findings = append(item.Findings, diagnostics.Analyze(redact(lines))...)
			enrich(item.Findings)
		}
		result.Shards = append(result.Shards, item)
	}
	update := s.mgr.ModsUpdateStatus()
	lines := append([]string{}, update.Output...)
	if update.Error != "" {
		lines = append(lines, "模组下载失败: "+update.Error)
	}
	result.Downloads = source{State: "stopped", Findings: diagnostics.Analyze(redact(lines))}
	if update.Running {
		result.Downloads.State = "running"
	}
	enrich(result.Downloads.Findings)
	respond(w, 200, result, nil)
}
