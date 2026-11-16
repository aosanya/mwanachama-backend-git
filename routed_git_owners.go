package mwanachamagit

import "sync"

const maxRememberedOwners = 4096

type fileOwners struct {
	mu    sync.Mutex
	names map[string]string
}

func (o *fileOwners) get(key string) (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	name, ok := o.names[key]
	return name, ok
}

func (o *fileOwners) remember(key, fileName string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.names == nil || len(o.names) >= maxRememberedOwners {
		o.names = map[string]string{}
	}
	o.names[key] = fileName
}

func (o *fileOwners) forget(key string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.names, key)
}

func (o *fileOwners) clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.names = nil
}
