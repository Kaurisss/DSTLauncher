package diagnostics

import (
	"regexp"
	"strings"
)

type Finding struct {
	Code       string   `json:"code"`
	Stage      string   `json:"stage"`
	Title      string   `json:"title"`
	Suggestion string   `json:"suggestion"`
	Evidence   []string `json:"evidence"`
	Mods       []Mod    `json:"mods"`
}

type Mod struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

var workshopID = regexp.MustCompile(`(?i)(?:workshop[-/\\]|content[/\\]322330[/\\])(\d+)`)
var downloadID = regexp.MustCompile(`(?i)(?:publishedfileid|item|mod)[ :=#]+(\d{6,})`)

var rules = []struct {
	code, stage, title, suggestion string
	pattern                        *regexp.Regexp
}{
	{"token", "身份验证", "服务器令牌验证失败", "在房间设置中重新填写有效的 Klei 服务器令牌，并检查服务器能否连接 Klei。", regexp.MustCompile(`(?i)(invalid.*(?:server.*token|cluster.*token)|(?:server.*token|cluster.*token).*(?:invalid|missing|failed)|E_INVALID_TOKEN|E_INVALID_SESSION)`)},
	{"port", "网络监听", "端口绑定失败", "检查游戏、Steam 和分片协调端口是否被其他进程占用，调整后重试。", regexp.MustCompile(`(?i)(address already in use|bind.*(?:failed|error)|(?:failed|unable) to bind|WSAEADDRINUSE)`)},
	{"download", "模组下载", "模组下载或更新失败", "检查网络及工坊条目是否公开，确认磁盘空间后重新下载列出的模组。", regexp.MustCompile(`(?i)(download.*(?:fail|error)|(?:fail|error).*download|workshop.*(?:fail|error)|模组(?:下载|更新)失败|以下模组未能下载|仍有.*未下载|未能下载)`)},
	{"lua", "脚本加载或运行", "Lua 脚本报错", "查看错误堆栈及最近修改的模组配置。日志明确指向模组时，可先停用该模组并复测。", regexp.MustCompile(`(?i)(LUA ERROR|Error loading mod|stack traceback:)`)},
	{"memory", "内存分配", "内存分配失败", "减少同时运行的分片或模组，检查宿主机及容器的可用内存。", regexp.MustCompile(`(?i)(out of memory|not enough memory|bad_alloc|memory allocation.*fail|cannot allocate memory)`)},
	{"master", "分片连接", "子世界连接主世界失败", "确认主世界已就绪，并检查主世界地址、协调端口和集群密钥是否一致。", regexp.MustCompile(`(?i)((?:failed|unable) to connect.*master|master.*connection.*(?:failed|refused)|could not find.*master)`)},
	{"version", "版本验证", "客户端与服务器版本不一致", "更新服务器及相关模组，再重启仍在运行旧版本的分片。", regexp.MustCompile(`(?i)(version mismatch|server is out of date|client is out of date|incompatible.*version)`)},
}

// Analyze only attributes mods from the failing line/stack, not earlier mod-loading messages.
func Analyze(lines []string) []Finding {
	result := []Finding{}
	seen := map[string]int{}
	for i, line := range lines {
		for _, rule := range rules {
			if !rule.pattern.MatchString(line) {
				continue
			}
			end := i + 1
			if rule.code == "lua" {
				end = min(len(lines), i+12)
			}
			context := lines[i:end]
			mods := []Mod{}
			ids := map[string]bool{}
			for _, evidence := range context {
				if rule.code == "lua" && !strings.Contains(strings.ToLower(evidence), ".lua") && !rule.pattern.MatchString(evidence) {
					continue
				}
				matches := workshopID.FindAllStringSubmatch(evidence, -1)
				if rule.code == "download" {
					matches = append(matches, downloadID.FindAllStringSubmatch(evidence, -1)...)
				}
				for _, match := range matches {
					if !ids[match[1]] {
						ids[match[1]] = true
						mods = append(mods, Mod{ID: "workshop-" + match[1]})
					}
				}
			}
			key := rule.code
			for _, mod := range mods {
				key += ":" + mod.ID
			}
			finding := Finding{rule.code, rule.stage, rule.title, rule.suggestion, append([]string{}, context...), mods}
			if at, ok := seen[key]; ok {
				result[at] = finding
			} else {
				seen[key] = len(result)
				result = append(result, finding)
			}
		}
	}
	return result
}

func Redact(lines []string, secrets ...string) []string {
	result := make([]string, len(lines))
	for i, line := range lines {
		for _, secret := range secrets {
			if secret != "" {
				line = strings.ReplaceAll(line, secret, "[已隐藏]")
			}
		}
		result[i] = line
	}
	return result
}
