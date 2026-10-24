local function execute()
    if not TheWorld or not TheWorld.ismastersim then
        return {error = "not_ready"}
    end
    local clients = {}
    for _, client in ipairs(TheNet:GetClientTable() or {}) do
        if client.performance == nil and type(client.userid) == "string" and client.userid ~= "" then
            clients[client.userid] = client
        end
    end
    local entities = {}
    for _, player in ipairs(AllPlayers or {}) do
        if player.userid then entities[player.userid] = player end
    end
    if request.action == "list" then
        local count = 0
        for userid, player in pairs(entities) do
            local client = clients[userid]
            if client ~= nil then
                local name = string.gsub(tostring(client.name or ""), "[%z\1-\8\11\12\14-\31]", "")
                local days = tonumber(client.playerage)
                if player.components.age then
                    days = player.components.age:GetDisplayAgeInDays()
                end
                local item = {
                    userid = userid, name = name, prefab = client.prefab or "",
                    days = days and math.max(0, math.floor(days)) or nil,
                    ghost = player:HasTag("playerghost"),
                    inWorld = true, admin = client.admin == true,
                }
                print(request.tag .. ":" .. require("json").encode_compliant({player = item}))
                count = count + 1
            end
        end
        return {count = count}
    end
    if clients[request.userid] == nil then return {error = "not_found"} end
    if request.action == "kick" then
        TheNet:Kick(request.userid)
    elseif request.action == "ban" then
        TheNet:Ban(request.userid)
    else
        local player = entities[request.userid]
        if player == nil then return {error = "not_in_world"} end
        if request.action == "revive" then
            if not player:HasTag("playerghost") then return {error = "not_ghost"} end
            player:PushEvent("respawnfromghost")
        elseif request.action == "teleport" then
            local target = entities[request.target]
            if target == nil or clients[request.target] == nil then return {error = "target_missing"} end
            local x, y, z = target.Transform:GetWorldPosition()
            if player.Physics then
                player.Physics:Teleport(x, y, z)
            else
                player.Transform:SetPosition(x, y, z)
            end
            player:SnapCamera()
        else
            return {error = "invalid_action"}
        end
    end
    return {ok = true}
end
local ok, result = pcall(execute)
if not ok then
    print("DST_PANEL_PLAYER_ERROR: " .. tostring(result))
    result = {error = "execution_failed"}
end
print(request.tag .. ":" .. require("json").encode_compliant(result))
