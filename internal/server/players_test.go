package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func playersFixture(t *testing.T, reply func(*Manager, string, string, string)) (*Manager, *Cluster) {
	t.Helper()
	m := NewManager(t.TempDir(), t.TempDir(), t.TempDir())
	c := &Cluster{ID: 2, Master: "Surface", Shards: []string{"Surface", "Below"}}
	m.Register(c)
	markerRE := regexp.MustCompile(`DST_PANEL_PLAYERS_[0-9a-f]{24}`)
	for _, name := range c.Shards {
		shard := name
		m.procs[procKey(c.Key(), shard)] = &process{
			cluster: c, shard: shard, state: StateRunning,
			cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}},
			stdin: rollbackInput{write: func(command string) {
				reply(m, c.Key(), shard, markerRE.FindString(command))
			}},
		}
		m.procs[procKey(c.Key(), shard)].observePlayerLog("[00:00:01]: Spawning player at: [Load] (0.00, 0.00, 0.00)")
	}
	return m, c
}

func TestPlayersWaitForCurrentShardSpawnLog(t *testing.T) {
	calls := 0
	m, c := playersFixture(t, func(m *Manager, cluster, shard, marker string) {
		calls++
		if shard != "Surface" {
			t.Errorf("尚无玩家生成日志的分片不应查询: %s", shard)
		}
		m.emit(cluster, shard, "log", marker+`:{"count":0}`)
	})
	surface := m.procs[procKey(c.Key(), "Surface")]
	below := m.procs[procKey(c.Key(), "Below")]
	surface.playersReady.Store(false)
	below.playersReady.Store(false)
	for _, line := range []string{
		"[00:00:01]: Server is now ready",
		"[00:00:02]: Client authenticated: KU_test",
		"[00:00:03]: [Say] test: Spawning player at: [Load] (0, 0, 0)",
		"[00:00:04]: RemoteCommandInput: print('Spawning player at: [Load]')",
	} {
		surface.observePlayerLog(line)
	}
	got, err := m.Players(context.Background(), c.Key())
	if err != nil || len(got.Players) != 0 || len(got.Errors) != 0 || calls != 0 {
		t.Fatalf("仅进程运行、就绪或认证不能触发玩家查询: %+v, %v, commands=%d", got, err, calls)
	}
	if err := m.ActOnPlayer(context.Background(), c.Key(), PlayerAction{Shard: "Surface", UserID: "KU_test", Action: "kick"}); err == nil || calls != 0 {
		t.Fatalf("未见玩家进入时也不能通过操作接口发送命令: %v, commands=%d", err, calls)
	}
	surface.observePlayerLog("[00:00:05]: Spawning player at: [Load] (1.00, 0.00, 2.00)")
	for _, status := range m.Status(c.Key()) {
		if status.PlayersReady != (status.Shard == "Surface") {
			t.Fatalf("就绪标记必须按分片隔离: %+v", status)
		}
	}
	if _, err := m.Players(context.Background(), c.Key()); err != nil || calls != 1 {
		t.Fatalf("只有确认玩家生成的分片可查询: %v, commands=%d", err, calls)
	}
	m.procs[procKey(c.Key(), "Surface")] = &process{
		cluster: c, shard: "Surface", state: StateRunning, cmd: surface.cmd, stdin: surface.stdin,
	}
	surface.observePlayerLog("[00:01:00]: Spawning player at: [Load] (1.00, 0.00, 2.00)")
	if _, err := m.Players(context.Background(), c.Key()); err != nil || calls != 1 {
		t.Fatalf("重启后不能沿用旧进程的玩家日志: %v, commands=%d", err, calls)
	}
}

func TestPlayersCorrelatesShardsAndKeepsPartialFailures(t *testing.T) {
	m, c := playersFixture(t, func(m *Manager, cluster, shard, marker string) {
		m.emit(cluster, shard, "log", "RemoteCommandInput: "+marker+`:{"count":0}`)
		m.emit("Cluster_999", shard, "log", marker+`:{"count":0}`)
		m.emit(cluster, "Wrong", "log", marker+`:{"count":0}`)
		if shard == "Below" {
			m.emit(cluster, shard, "log", "[00:00:30]: "+marker+`:{"error":"not_ready"}`)
			return
		}
		m.emit(cluster, shard, "log", marker+`:{"player":{"userid":"KU_test","name":"中文 ]: <script>","prefab":"wendy","days":3,"ghost":true,"inWorld":true}}`)
		m.emit(cluster, shard, "log", "[00:00:30]: "+marker+`:{"count":1}`)
	})
	got, err := m.Players(context.Background(), c.Key())
	if err != nil || len(got.Players) != 1 || got.Players[0].Shard != "Surface" || got.Players[0].Name != "中文 ]: <script>" {
		t.Fatalf("应只接受当前房间/分片的真实回复: %+v, %v", got, err)
	}
	if got.Errors["Below"] == "" {
		t.Fatal("失败分片不应显示为空列表")
	}
	if len(m.subs) != 0 {
		t.Fatal("查询结束应清理日志订阅")
	}
}

func TestPlayersMergesMigrationSnapshotsByIDNotName(t *testing.T) {
	m, c := playersFixture(t, func(m *Manager, cluster, shard, marker string) {
		m.emit(cluster, shard, "log", marker+`:{"player":{"userid":"KU_moving","name":"same nickname","inWorld":true}}`)
		count := 1
		if shard == "Surface" {
			m.emit(cluster, shard, "log", marker+`:{"player":{"userid":"KU_other","name":"same nickname","inWorld":true}}`)
			count++
		}
		m.emit(cluster, shard, "log", fmt.Sprintf("%s:{\"count\":%d}", marker, count))
	})
	got, err := m.Players(context.Background(), c.Key())
	if err != nil || len(got.Errors) != 0 || len(got.Players) != 2 {
		t.Fatalf("同一玩家跨分片只列一次，同名的另一位玩家仍保留: %+v, %v", got, err)
	}
	ids := map[string]bool{}
	for _, player := range got.Players {
		if ids[player.UserID] {
			t.Fatalf("重复的玩家 ID: %s", player.UserID)
		}
		ids[player.UserID] = true
	}
}

func TestPlayerRequestsDoNotMixConcurrentReplies(t *testing.T) {
	m, c := playersFixture(t, func(m *Manager, cluster, shard, marker string) {
		go m.emit(cluster, shard, "log", marker+`:{"ok":true}`)
	})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.ActOnPlayer(context.Background(), c.Key(), PlayerAction{Shard: "Surface", UserID: "KU_test", Action: "kick"}); err != nil {
				t.Errorf("独立回执串线: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestPlayerActionValidationAndTimeout(t *testing.T) {
	calls := 0
	m, c := playersFixture(t, func(*Manager, string, string, string) { calls++ })
	for _, action := range []PlayerAction{
		{Shard: "Surface", UserID: `KU_";c_regenerateworld();--`, Action: "kick"},
		{Shard: "../Other", UserID: "KU_test", Action: "kick"},
		{Shard: "Surface", UserID: "KU_test", Action: "delete"},
		{Shard: "Surface", UserID: "KU_test", Action: "teleport", Target: "KU_test"},
		{Shard: "Surface", UserID: "KU_test", Action: "teleport"},
		{Shard: "Surface", UserID: "KU_test", Action: "revive", Target: "KU_other"},
	} {
		if err := m.ActOnPlayer(context.Background(), c.Key(), action); err == nil {
			t.Errorf("非法操作未被拒绝: %+v", action)
		}
	}
	if calls != 0 {
		t.Fatal("校验失败时仍写入了控制台")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := m.ActOnPlayer(ctx, c.Key(), PlayerAction{Shard: "Surface", UserID: "KU_test", Action: "ban"}); err == nil || !strings.Contains(err.Error(), "勿立即重复提交") {
		t.Fatalf("没有回执不能当作执行成功: %v", err)
	}
	m.procs[procKey(c.Key(), "Surface")].exited.Store(true)
	if err := m.ActOnPlayer(context.Background(), c.Key(), PlayerAction{Shard: "Surface", UserID: "KU_test", Action: "kick"}); err == nil {
		t.Fatal("未拒绝已停止分片")
	}
}

func TestPlayerGameRejectionsAndEmptyList(t *testing.T) {
	for _, payload := range []string{`{"error":"not_found"}`, `{"error":"not_ghost"}`, `{"error":"target_missing"}`, `{}`, "broken"} {
		t.Run(payload, func(t *testing.T) {
			m, c := playersFixture(t, func(m *Manager, cluster, shard, marker string) { m.emit(cluster, shard, "log", marker+":"+payload) })
			if err := m.ActOnPlayer(context.Background(), c.Key(), PlayerAction{Shard: "Surface", UserID: "KU_test", Action: "revive"}); err == nil {
				t.Fatal("把游戏拒绝或无效回复当成了成功")
			}
		})
	}
	m, c := playersFixture(t, func(m *Manager, cluster, shard, marker string) {
		m.emit(cluster, shard, "log", marker+`:{"count":0}`)
	})
	got, err := m.Players(context.Background(), c.Key())
	if err != nil || len(got.Players) != 0 || len(got.Errors) != 0 || got.Players == nil {
		t.Fatalf("真正空列表应成功且保持 JSON 数组: %+v, %v", got, err)
	}
}

func TestPlayersRejectsIncompleteStream(t *testing.T) {
	m, c := playersFixture(t, func(m *Manager, cluster, shard, marker string) {
		m.emit(cluster, shard, "log", marker+`:{"count":1}`)
	})
	got, err := m.Players(context.Background(), c.Key())
	if err != nil || len(got.Errors) != 2 || len(got.Players) != 0 {
		t.Fatalf("丢失行不能被当作空列表: %+v, %v", got, err)
	}
}

func TestPlayersLuaBehavior(t *testing.T) {
	lua := os.Getenv("DST_LUA_EXECUTABLE")
	if lua == "" {
		var err error
		lua, err = exec.LookPath("lua")
		if err != nil {
			t.Skip("设置 DST_LUA_EXECUTABLE 或安装 Lua 5.1 以执行游戏 Lua 回归")
		}
	}
	fixture, err := os.ReadFile("testdata/players.lua")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, before, action, userid, target, check string
	}{
		{"list", "", "list", "", "", `local r=expectReply(); local players=getPlayers(); assert(r.count==2 and #players==2); local byid={}; for _,p in ipairs(players) do byid[p.userid]=p end; assert(byid.KU_host==nil); assert(byid.KU_alive.days==12 and byid.KU_alive.admin); assert(not string.find(byid.KU_alive.name,string.char(1),1,true)); assert(byid.KU_ghost.ghost and byid.KU_ghost.inWorld); assert(byid.KU_select==nil and byid.KU_remote==nil)`},
		{"shared_list_without_local_players", "AllPlayers={}", "list", "", "", `assert(expectReply().count==0 and #getPlayers()==0)`},
		{"shared_list_with_only_ghost_local", "AllPlayers={AllPlayers[2]}", "list", "", "", `local players=getPlayers(); assert(expectReply().count==1 and #players==1 and players[1].userid=="KU_ghost")`},
		{"kick", "", "kick", "KU_alive", "", `assert(expectReply().ok and TheNet.kicked=="KU_alive")`},
		{"ban", "", "ban", "KU_ghost", "", `assert(expectReply().ok and TheNet.banned=="KU_ghost")`},
		{"revive", "", "revive", "KU_ghost", "", `assert(expectReply().ok and AllPlayers[2].event=="respawnfromghost")`},
		{"not_ghost", "", "revive", "KU_alive", "", `assert(expectReply().error=="not_ghost" and AllPlayers[1].event==nil)`},
		{"left", "", "kick", "KU_left", "", `assert(expectReply().error=="not_found" and TheNet.kicked==nil)`},
		{"selection", "", "revive", "KU_select", "", `assert(expectReply().error=="not_in_world")`},
		{"teleport", "", "teleport", "KU_alive", "KU_ghost", `assert(expectReply().ok and AllPlayers[1].position[1]==10 and AllPlayers[1].position[3]==20 and AllPlayers[1].snapped)`},
		{"teleport_without_physics", "AllPlayers[1].Physics=nil", "teleport", "KU_alive", "KU_ghost", `assert(expectReply().ok and AllPlayers[1].position[1]==10 and AllPlayers[1].snapped)`},
		{"target_disconnected", `TheNet.GetClientTable=function() return {{userid="KU_alive"}} end`, "teleport", "KU_alive", "KU_ghost", `assert(expectReply().error=="target_missing" and AllPlayers[1].position==nil)`},
		{"target_left", "", "teleport", "KU_alive", "KU_left", `assert(expectReply().error=="target_missing" and AllPlayers[1].position==nil)`},
		{"not_ready", "TheWorld=nil", "list", "", "", `assert(expectReply().error=="not_ready")`},
		{"exception", "TheNet.Kick=function() error('failure') end", "kick", "KU_alive", "", `assert(expectReply().error=="execution_failed")`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action := PlayerAction{Action: tc.action, UserID: tc.userid, Target: tc.target}
			command := playerCommand("fixture", action)
			if strings.ContainsAny(command, "\r\n") {
				t.Fatal("控制台命令必须是单行")
			}
			script := fmt.Sprintf("%s\nfixture()\n%s\n%s\n%s\n", fixture, tc.before, command, tc.check)
			path := filepath.Join(t.TempDir(), "players.lua")
			if err := os.WriteFile(path, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command(lua, path).CombinedOutput(); err != nil {
				t.Fatalf("Lua 行为回归失败: %v\n%s", err, output)
			}
		})
	}
}
