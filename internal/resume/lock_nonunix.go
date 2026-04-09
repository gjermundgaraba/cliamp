//go:build !unix

package resume

func withLock(fn func() error) error {
	return fn()
}
