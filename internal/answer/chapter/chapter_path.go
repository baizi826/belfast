package chapter

import "github.com/ggmolly/belfast/internal/protobuf"

type chapterCellKey struct {
	Row    uint32
	Column uint32
}

// chapterCellBlocksMove 对应客户端 chapterleveldata.lua 的 considerAsObstacle(SubjectPlayer, row, col)：
// 格子上有「flag==active 的敌方附件」（存活小怪/精英/伏击/BOSS/多阶段BOSS…）就算障碍 ——
// 客户端给它 PrioObstacle(=1000)，A* 直接当墙，所以活怪堵路、跨不过去（只能停在它上面开打）。
// 被击沉(flag==disabled)的怪格不是障碍，可以走过去。
func chapterCellBlocksMove(state *protobuf.CURRENTCHAPTERINFO, row, column uint32) bool {
	_, cell := findChapterCellAt(state, chapterPos{Row: row, Column: column})
	if cell == nil || cell.GetItemFlag() != chapterCellActive {
		return false
	}
	return isChapterEnemyAttachment(cell.GetItemType())
}

func findMovePath(grids []chapterGrid, state *protobuf.CURRENTCHAPTERINFO, start chapterPos, end chapterPos) []chapterPos {
	if start == end {
		return []chapterPos{start}
	}
	walkable := make(map[chapterCellKey]bool, len(grids))
	for _, grid := range grids {
		if grid.Walkable {
			walkable[chapterCellKey{Row: grid.Row, Column: grid.Column}] = true
		}
	}
	startKey := chapterCellKey{Row: start.Row, Column: start.Column}
	endKey := chapterCellKey{Row: end.Row, Column: end.Column}
	if !walkable[startKey] || !walkable[endKey] {
		return nil
	}
	// 终点豁免：客户端把终点的优先级强制成 PrioNormal（considerAsStayPoint），
	// 所以终点可以是怪格 —— 走过去就是打它。中途的活怪不豁免。
	blocked := func(key chapterCellKey) bool {
		if !walkable[key] {
			return true
		}
		if key == startKey || key == endKey {
			return false
		}
		return chapterCellBlocksMove(state, key.Row, key.Column)
	}
	queue := []chapterCellKey{startKey}
	visited := map[chapterCellKey]bool{startKey: true}
	parent := map[chapterCellKey]chapterCellKey{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == endKey {
			break
		}
		neighbors := make([]chapterCellKey, 0, 4)
		neighbors = append(neighbors, chapterCellKey{Row: current.Row + 1, Column: current.Column})
		if current.Row > 1 {
			neighbors = append(neighbors, chapterCellKey{Row: current.Row - 1, Column: current.Column})
		}
		neighbors = append(neighbors, chapterCellKey{Row: current.Row, Column: current.Column + 1})
		if current.Column > 1 {
			neighbors = append(neighbors, chapterCellKey{Row: current.Row, Column: current.Column - 1})
		}
		for _, neighbor := range neighbors {
			if visited[neighbor] || blocked(neighbor) {
				continue
			}
			visited[neighbor] = true
			parent[neighbor] = current
			queue = append(queue, neighbor)
		}
	}
	if !visited[endKey] {
		return nil
	}
	path := []chapterPos{}
	current := endKey
	for {
		path = append(path, chapterPos{Row: current.Row, Column: current.Column})
		if current == startKey {
			break
		}
		current = parent[current]
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
