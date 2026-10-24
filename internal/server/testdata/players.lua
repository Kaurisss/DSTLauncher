local captured
local printed
local responses
package.preload["json"] = function()
    return {encode_compliant = function(result) captured = result table.insert(responses, result) return "{}" end}
end
print = function(line) printed = line end

local function fixture()
    captured = nil
    printed = nil
    responses = {}
    TheWorld = {ismastersim = true}
    local function player(id, ghost, days)
        local inst = {
            userid = id, components = {age = {GetDisplayAgeInDays = function() return days end}},
            Transform = {GetWorldPosition = function() return 10, 0, 20 end},
            HasTag = function(_, tag) return tag == "playerghost" and ghost end,
            PushEvent = function(self, event) self.event = event end,
            SnapCamera = function(self) self.snapped = true end,
        }
        inst.Transform.SetPosition = function(_, x, y, z) inst.position = {x, y, z} end
        inst.Physics = {Teleport = function(_, x, y, z) inst.position = {x, y, z} end}
        return inst
    end
    AllPlayers = {player("KU_alive", false, 12), player("KU_ghost", true, 3)}
    TheNet = {
        GetClientTable = function() return {
            {userid = "KU_host", name = "Host", performance = {}},
            {userid = "KU_alive", name = "中文]: <script>\n" .. string.char(1), prefab = "wilson", playerage = 9, admin = true},
            {userid = "KU_ghost", name = "ghost", prefab = "wendy", playerage = 2},
            {userid = "KU_select", name = "select"},
            {userid = "KU_remote", name = "another shard", prefab = "willow", playerage = 72},
        } end,
        Kick = function(self, id) self.kicked = id end,
        Ban = function(self, id) self.banned = id end,
    }
end

local function expectReply()
    assert(printed == "fixture:{}", "reply marker missing")
    assert(captured ~= nil, "no response")
    return captured
end

local function getPlayers()
    local players = {}
    for _, result in ipairs(responses) do
        if result.player then table.insert(players, result.player) end
    end
    return players
end
