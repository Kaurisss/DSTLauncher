package worldsettings

// LocationFromFiles 按游戏实际读取顺序判断某个分片的世界位置。
//
// 返回的 found 表示位置来自磁盘文件；没有世界配置文件时，位置来自 fallback。
func LocationFromFiles(clusterDir, shard, fallback string) (location string, found bool, err error) {
	levelData, exists, err := ReadLevelData(clusterDir, shard)
	if err != nil {
		return "", false, err
	}
	if exists {
		return levelData.Location(), true, nil
	}

	override, exists, err := ReadFile(clusterDir, shard, fallback)
	if err != nil {
		return "", false, err
	}
	if exists && override.WorldGenPreset != "" {
		return LocationFromPreset(override.WorldGenPreset), true, nil
	}
	return NormalizeLocation(fallback), false, nil
}
