package cmdrun

import "sync"

// uidPool hands out one uid per running job. Two jobs never share one, which is
// what keeps one job out of another's /proc and lets a kill by uid reach exactly
// one job's processes.
type uidPool struct {
	mu    sync.Mutex
	start int
	count int
	used  map[int]struct{}
	// next rotates through the range, so a uid just released is the last to be
	// handed out again rather than the first.
	next int
}

func newUIDPool(start, count int) *uidPool {
	return &uidPool{start: start, count: count, used: make(map[int]struct{})}
}

func (p *uidPool) acquire() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i < p.count; i++ {
		uid := p.start + (p.next+i)%p.count
		if _, busy := p.used[uid]; busy {
			continue
		}
		p.used[uid] = struct{}{}
		p.next = (p.next + i + 1) % p.count
		return uid, nil
	}
	return -1, ErrNoFreeUID
}

func (p *uidPool) release(uid int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.used, uid)
}

func (p *uidPool) contains(uid int) bool {
	return uid >= p.start && uid < p.start+p.count
}
