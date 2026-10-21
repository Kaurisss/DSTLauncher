"""生成《岛屿冒险》海难世界规则页的显示顺序（按游戏内顺序）。"""

# 分组顺序与组内顺序，按游戏内海难「世界规则」页三列从左到右、从上到下抄录。
SHIPWRECKED_SETTINGS_ORDER = {
    "global": [
        "specialevent", "autumn", "winter", "spring", "summer", "day",
        "spawnmode", "ghostenabled", "portalresurection", "ghostsanitydrain",
        "resettime", "beefaloheat", "krampus", "mild", "hurricane", "monsoon",
        "dry", "poison",
    ],
    "events": [
        "crow_carnival", "hallowed_nights", "winters_feast", "year_of_the_gobbler",
        "year_of_the_varg", "year_of_the_pig", "year_of_the_carrat",
        "year_of_the_beefalo", "year_of_the_catcoon", "year_of_the_bunnyman",
        "year_of_the_dragonfly", "year_of_the_snake", "year_of_the_knight",
    ],
    "survivors": [
        "extrastartingitems", "seasonalstartingitems", "spawnprotection",
        "dropeverythingondespawn", "healthpenalty", "lessdamagetaken",
        "temperaturedamage", "hunger", "darkness", "shadowcreatures",
        "brightmarecreatures",
    ],
    "misc": [
        "crocodog", "yellowcrocodog", "bluecrocodog", "dst_boats", "ia_boats",
        "ia_drowning", "whalehunt", "floods", "oceanwaves", "tides",
        "dragoonegg", "alternatewhalehunt", "lightning", "waterencounters", "weather",
    ],
    "resources": [
        "regrowth", "jungletree_regrowth", "basicresource_regrowth",
        "twiggytrees_regrowth", "coral_brain_rock_regrowth", "palmtree_regrowth",
        "sandhill_regrowth", "sweet_potato_regrowth", "mangrovetree_regrowth",
        "reeds_regrowth", "flowers_regrowth", "bioluminescence_regrowth",
        "seashell_regrowth",
    ],
    "animals": [
        "bunnymen_setting", "crab_setting", "rainbowjellyfish_setting",
        "jellyfish_setting", "ballphin_setting", "fishermerm_setting",
        "solofish_setting", "pigs_setting", "sharkitten_setting",
        "primeape_setting", "grassgekkos", "bees_setting", "butterfly",
        "wildbores_setting", "fishschools", "birds", "moles_setting",
        "lobster_setting",
    ],
    "monsters": [
        "swordfish_setting", "dragoon_setting", "stungray_setting", "wasps",
        "mosquito", "chessnavy_setting", "snake_setting", "spiders_setting",
        "spider_warriors", "lureplants", "merms",
    ],
    "giants": [
        "klaus", "eyeofterror", "daywalker2", "fruitfly", "liefs", "kraken",
        "tigershark", "beequeen", "spiderqueen", "twister",
    ],
}

# 世界生成页的分组顺序与组内顺序（按游戏内页面截图）。
SHIPWRECKED_WORLDGEN_GROUP_ORDER = ["misc", "resources", "animals", "monsters"]

SHIPWRECKED_WORLDGEN_ORDER = {
    "misc": [
        "task_set", "start_location", "world_size", "touchstone", "boons",
        "prefabswaps_start", "junkyard", "balatro", "volcano", "volcanoisland",
        "bermudatriangle", "terrariumchest",
    ],
    "resources": [
        "marshbush", "shipwrecked_trees", "rock", "limpets", "coral_brain_rock",
        "crate", "sapling", "poisonhole", "waterygrave", "shipwreck", "sandhill",
        "berrybush", "seaweed", "tidalpool", "tar_pool", "magma_rocks", "flint",
        "coral", "sweet_potato", "bamboo", "reeds", "flowers", "grass",
        "bioluminescence", "bush_vine", "mushroom", "seashell", "mussel_farm",
    ],
    "animals": [
        "crabhole", "rainbowjellyfish", "jellyfish", "ox", "ballphin", "seagull",
        "fishermerm", "doydoy", "solofish", "primeape", "bees", "wildbores",
        "fishinhole", "lobster",
    ],
    "monsters": [
        "swordfish", "stungray", "angrybees", "merm", "flup", "snake",
        "spiders", "tallbirds",
    ],
}

# 世界生成页里游戏不展示、但清单里存在的项：它们只在「火山岛」开启时出现。
#
# 模组用 VOLCANO_ISLAND_OPTIONS + HasVolcanoIsland() 控制：当前值
# volcanoisland="none" 时整组不列出来。所以这里不能直接删（删了以后用户开了
# 火山岛也没法调），而是在后端按当前值动态隐藏。
SHIPWRECKED_VOLCANO_ISLAND_ONLY = [
    "coffeebush", "elephantcactus", "rock_obsidian", "rock_charcoal",
    "volcano_shrub", "dragoon",
]

# 世界规则里的火山岛专属项（同一个开关控制）
SHIPWRECKED_VOLCANO_ISLAND_ONLY_SETTINGS = [
    "daywalker", "rock_obsidian_regrowth", "rock_charcoal_regrowth",
    "volcano_shrub_regrowth", "magmarock_regrowth",
]

# ---------------- 火山（volcanoworld）----------------
# 火山是副世界，游戏不列季节/活动/冒险家（它们带 master_controlled），
# 所以只有这五组。分组与组内顺序按游戏内截图抄录。
VOLCANO_GROUP_ORDER = ["misc", "resources", "animals", "monsters", "giants"]

VOLCANO_SETTINGS_ORDER = {
    "misc": ["dst_boats", "ia_boats", "dragoonegg", "lightning", "weather"],
    "resources": [
        "regrowth", "rock_charcoal_regrowth", "volcano_shrub_regrowth",
        "magmarock_regrowth", "rock_obsidian_regrowth",
    ],
    "animals": [
        "bunnymen_setting", "fishermerm_setting", "pigs_setting",
        "primeape_setting", "bees_setting", "wildbores_setting", "moles_setting",
    ],
    "monsters": [
        "dragoon_setting", "snake_setting", "spiders_setting",
        "spider_warriors", "merms",
    ],
    "giants": ["fruitfly", "liefs", "daywalker", "spiderqueen"],
}

VOLCANO_WORLDGEN_GROUP_ORDER = ["misc", "resources", "monsters"]

VOLCANO_WORLDGEN_ORDER = {
    "misc": ["task_set", "start_location"],
    "resources": [
        "coffeebush", "rock_charcoal", "volcano_shrub", "magma_rocks",
        "elephantcactus", "rock_obsidian",
    ],
    "monsters": ["dragoon"],
}

# 分组顺序也照截图：全局、活动、冒险家、世界、资源再生、生物、敌对生物、巨兽
SHIPWRECKED_GROUP_ORDER = [
    "global", "events", "survivors", "misc",
    "resources", "animals", "monsters", "giants",
]

# 游戏内页面不展示、但原版清单里存在的项。
# 它们现在是「火山岛开启时才出现」的那批，具体名单见上面两个常量；
# 这里只为向后兼容保留一个合并视图，实际隐藏由后端按 volcanoisland 当前值决定。
SHIPWRECKED_HIDDEN = (
    SHIPWRECKED_VOLCANO_ISLAND_ONLY + SHIPWRECKED_VOLCANO_ISLAND_ONLY_SETTINGS
)
