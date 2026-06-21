//go:build windows

package auth

import (
	"os"

	"golang.org/x/sys/windows"
)

// acquireLock은 Windows에서 LockFileEx를 사용해 배타적 advisory 락을 획득합니다.
// 비지원 환경에서는 에러가 반환됩니다.
// 호출자는 에러 시 lock-free 폴백을 수행해야 합니다.
func acquireLock(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol)
}

// releaseLock은 Windows에서 UnlockFileEx로 락을 해제합니다.
func releaseLock(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
