package collector

import "sync"

type Pauser struct {
	value bool
	mu    sync.RWMutex
}

func (p *Pauser) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.value = true
}

func (p *Pauser) UnPause() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.value = false
}

func (p *Pauser) Value() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.value
}

func NewPauser() *Pauser {
	return new(Pauser)
}
