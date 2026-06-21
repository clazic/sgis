//go:build !windows

package auth

import (
	"os"

	"golang.org/x/sys/unix"
)

// acquireLock은 Unix에서 syscall.Flock을 사용해 배타적 advisory 락을 획득합니다.
// NFS 등 비지원 파일시스템에서는 ENOTSUP/EOPNOTSUPP 에러가 반환됩니다.
// 호출자는 에러 시 lock-free 폴백을 수행해야 합니다.
func acquireLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX)
}

// releaseLock은 Unix에서 flock 락을 해제합니다.
func releaseLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
