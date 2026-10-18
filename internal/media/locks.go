package media

import "sync"

// HashLocks serialises work on one stored file: deciding that a file is
// unused and removing it must not interleave with an upload of the same
// content. The zero value is ready to use.
type HashLocks struct {
	mu   sync.Mutex
	held map[string]*hashLock
}

type hashLock struct {
	mu   sync.Mutex
	refs int
}

// Lock blocks until no one else holds sha, then returns the function that
// releases it. Entries are dropped once nobody holds or waits for them.
func (l *HashLocks) Lock(sha string) (unlock func()) {
	l.mu.Lock()
	if l.held == nil {
		l.held = map[string]*hashLock{}
	}
	h := l.held[sha]
	if h == nil {
		h = &hashLock{}
		l.held[sha] = h
	}
	h.refs++
	l.mu.Unlock()

	h.mu.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Unlock()
			l.mu.Lock()
			if h.refs--; h.refs == 0 {
				delete(l.held, sha)
			}
			l.mu.Unlock()
		})
	}
}
