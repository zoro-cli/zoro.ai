package runtime

import (
	"fmt"
	"github.com/gofrs/flock"
	"github.com/zoro-cli/zoro.ai/internal/app"
	"path/filepath"
)

type Lock struct{ f *flock.Flock }

func Acquire(root string) (*Lock, error) {
	f := flock.New(filepath.Join(root, ".zoro", "runtime", "zoro.lock"))
	ok, e := f.TryLock()
	if e != nil {
		return nil, fmt.Errorf("%w: %v", app.ErrLock, e)
	}
	if !ok {
		return nil, fmt.Errorf("%w: another Zoro operation is already active for this repository", app.ErrLock)
	}
	return &Lock{f}, nil
}
func (l *Lock) Release() { _ = l.f.Unlock() }
