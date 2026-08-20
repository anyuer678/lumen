package task

// PriorityQueue 优先级队列
type PriorityQueue []*Task

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// 优先级越高（数字越小）越优先
	if pq[i].Priority != pq[j].Priority {
		return pq[i].Priority < pq[j].Priority
	}
	// 优先级相同，先创建的先执行
	return pq[i].CreatedAt.Before(pq[j].CreatedAt)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *PriorityQueue) Push(x interface{}) {
	*pq = append(*pq, x.(*Task))
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*pq = old[0 : n-1]
	return item
}

// Peek 查看队首任务（不移除）
func (pq PriorityQueue) Peek() *Task {
	if len(pq) == 0 {
		return nil
	}
	return pq[0]
}
