package cmd

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/clazic/sgis/internal/config"
	"github.com/spf13/cobra"
)

const updateRepo = "clazic/sgis"

var (
	updateCheckOnly bool
	updateForce     bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "sgis를 최신 버전으로 업데이트 (바이너리 + 스킬 파일)",
	Long: `sgis를 GitHub 최신 릴리스로 업데이트합니다.

바이너리(OS별)와 스킬 파일(SKILL.md, docs/, LEARNINGS.md, templates 등)을
함께 내려받아 설치된 스킬 디렉토리(~/.claude/skills/sgis, ~/.codex/skills/sgis)에 반영합니다.
다운로드 후 SHA256 체크섬을 검증한 뒤 바이너리를 교체합니다.

사용법:
  sgis update            최신 버전으로 업데이트 (바이너리 + 스킬)
  sgis update --check    업데이트 확인만 (설치하지 않음)
  sgis update --force    같은 버전이어도 강제 재설치`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runUpdate(); err != nil {
			fmt.Fprintf(os.Stderr, "오류: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheckOnly, "check", false, "업데이트 확인만 (설치하지 않음)")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "같은 버전이어도 강제 재설치")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate() error {
	current := appVersion
	fmt.Printf("현재 버전: %s\n", current)
	fmt.Println("최신 버전 확인 중...")

	latest, err := fetchLatestTag()
	if err != nil {
		return fmt.Errorf("최신 버전 확인 실패: %w", err)
	}
	fmt.Printf("최신 버전: %s\n", latest)

	if !updateForce && normalizeVer(current) == normalizeVer(latest) {
		fmt.Println("이미 최신 버전입니다.")
		return nil
	}
	if updateCheckOnly {
		fmt.Printf("새 버전 %s 사용 가능. `sgis update`로 설치하세요.\n", latest)
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("홈 디렉토리 확인 실패: %w", err)
	}

	binAsset, err := binaryAssetName()
	if err != nil {
		return err
	}
	skillAsset := fmt.Sprintf("sgis-skill-%s.tar.gz", latest)
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", updateRepo, latest)
	sha256SumsURL := base + "/SHA256SUMS"

	tmp, err := os.MkdirTemp("", ".sgis-update-*")
	if err != nil {
		return fmt.Errorf("임시 디렉토리 생성 실패: %w", err)
	}
	defer os.RemoveAll(tmp)

	// SHA256SUMS 다운로드
	sha256SumsPath := filepath.Join(tmp, "SHA256SUMS")
	fmt.Println("  체크섬 파일 다운로드 중...")
	if err := downloadFile(sha256SumsURL, sha256SumsPath); err != nil {
		return fmt.Errorf("SHA256SUMS 다운로드 실패: %w", err)
	}
	checksums, err := parseSHA256Sums(sha256SumsPath)
	if err != nil {
		return fmt.Errorf("SHA256SUMS 파싱 실패: %w", err)
	}

	skillTar := filepath.Join(tmp, "skill.tar.gz")
	binTmp := filepath.Join(tmp, "sgis-bin")

	// ── 바이너리 다운로드 + SHA256 검증 ──
	fmt.Printf("  바이너리 다운로드 중 (%s)...\n", binAsset)
	if err := downloadFile(base+"/"+binAsset, binTmp); err != nil {
		return fmt.Errorf("바이너리 다운로드 실패: %w", err)
	}

	// 바이너리 업데이트 확인 프롬프트
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("실행 바이너리 경로 확인 실패: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exePath); rerr == nil {
		exePath = resolved
	}

	if !promptYN(fmt.Sprintf("바이너리를 업데이트하겠습니까? (%s → %s) (y/N) ", current, latest)) {
		fmt.Println("  바이너리 업데이트를 건너뜁니다.")
	} else {
		// SHA256 검증 (swap 전)
		if expected, ok := checksums[binAsset]; ok {
			fmt.Println("  SHA256 체크섬 검증 중...")
			if err := verifySHA256(binTmp, expected); err != nil {
				return fmt.Errorf("바이너리 체크섬 검증 실패: %w", err)
			}
			fmt.Println("  체크섬 검증 완료.")
		} else {
			fmt.Fprintf(os.Stderr, "  경고: SHA256SUMS에서 %s 항목을 찾지 못했습니다. 검증 생략.\n", binAsset)
		}

		fmt.Printf("  바이너리 교체 중 (%s)...\n", exePath)
		if err := replaceBinary(binTmp, exePath); err != nil {
			return fmt.Errorf("바이너리 교체 실패: %w", err)
		}
		fmt.Printf("  바이너리: %s\n", exePath)
	}

	// ── 스킬 파일 다운로드 + 확인 프롬프트 ──
	if !promptYN(fmt.Sprintf("스킬 파일(SKILL.md, LEARNINGS.md, templates 등)을 업데이트하겠습니까? (y/N) ")) {
		fmt.Println("  스킬 파일 업데이트를 건너뜁니다.")
	} else {
		fmt.Println("  스킬 파일 다운로드 중...")
		if err := downloadFile(base+"/"+skillAsset, skillTar); err != nil {
			return fmt.Errorf("스킬 파일 다운로드 실패: %w", err)
		}

		// 스킬 tarball SHA256 검증
		if expected, ok := checksums[skillAsset]; ok {
			fmt.Println("  스킬 체크섬 검증 중...")
			if err := verifySHA256(skillTar, expected); err != nil {
				return fmt.Errorf("스킬 파일 체크섬 검증 실패: %w", err)
			}
			fmt.Println("  스킬 체크섬 검증 완료.")
		} else {
			fmt.Fprintf(os.Stderr, "  경고: SHA256SUMS에서 %s 항목을 찾지 못했습니다. 검증 생략.\n", skillAsset)
		}

		skillDirs := collectSkillDirs(home)
		for _, dest := range skillDirs {
			if err := extractTarGz(skillTar, dest); err != nil {
				fmt.Fprintf(os.Stderr, "  스킬 갱신 실패(%s): %v\n", dest, err)
				continue
			}
			fmt.Printf("  스킬: %s\n", dest)
		}
	}

	fmt.Printf("sgis %s → %s 업데이트 완료\n", current, latest)
	if runtime.GOOS == "windows" {
		fmt.Println("  (Windows) 새 바이너리는 다음 실행부터 적용됩니다. 이전 버전은 *.old로 보관됩니다.")
	}

	// 업데이트 완료 후 캐시 갱신
	_ = saveUpdateCache(latest)
	return nil
}

// promptYN TTY에서 y/N 프롬프트를 출력하고 사용자 응답을 반환합니다.
func promptYN(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		ans := strings.TrimSpace(scanner.Text())
		return strings.EqualFold(ans, "y")
	}
	return false
}

// parseSHA256Sums SHA256SUMS 파일을 파싱해 filename→hash 맵을 반환합니다.
// 형식: "<hash>  <filename>" (두 스페이스 또는 한 스페이스 모두 허용)
func parseSHA256Sums(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		hash := parts[0]
		// sha256sum 출력에서 파일명에 * 접두사(바이너리 모드)가 붙을 수 있음
		name := strings.TrimPrefix(parts[1], "*")
		result[name] = hash
	}
	return result, nil
}

// verifySHA256 파일의 SHA256 해시를 계산해 expected와 대조합니다.
func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("SHA256 불일치: expected=%s got=%s", expected, got)
	}
	return nil
}

// collectSkillDirs 존재하는 스킬 디렉토리를 수집합니다 (global + cwd project).
func collectSkillDirs(home string) []string {
	candidates := []string{
		filepath.Join(home, ".claude", "skills", "sgis"),
		filepath.Join(home, ".codex", "skills", "sgis"),
	}
	// cwd project 스킬
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, ".claude", "skills", "sgis"),
			filepath.Join(cwd, ".codex", "skills", "sgis"),
		)
	}
	var result []string
	for _, d := range candidates {
		if _, err := os.Stat(d); err == nil {
			result = append(result, d)
		}
	}
	// 존재하는 디렉토리가 없으면 global claude 기본
	if len(result) == 0 {
		result = append(result, filepath.Join(home, ".claude", "skills", "sgis"))
	}
	return result
}

// ── 자동 업데이트 알림 ──

type updateCache struct {
	LastCheck   time.Time `json:"last_check"`
	LatestKnown string    `json:"latest_known"`
}

func updateCachePath() string {
	return filepath.Join(config.ConfigDir(), "update-check.json")
}

func loadUpdateCache() (*updateCache, error) {
	data, err := os.ReadFile(updateCachePath())
	if err != nil {
		return &updateCache{}, nil
	}
	var c updateCache
	if err := json.Unmarshal(data, &c); err != nil {
		return &updateCache{}, nil
	}
	return &c, nil
}

func saveUpdateCache(latestKnown string) error {
	c := updateCache{LastCheck: time.Now(), LatestKnown: latestKnown}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(config.ConfigDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(updateCachePath(), data, 0o600)
}

// shouldCheckUpdate 업데이트 알림을 실행해야 할지 결정합니다.
func shouldCheckUpdate() bool {
	// 명시적 비활성화 (env)
	if os.Getenv("SGIS_NO_UPDATE_CHECK") != "" {
		return false
	}
	if cfg, err := config.Load(); err == nil && !cfg.UpdateCheck {
		return false
	}

	// stdout이 TTY가 아니면 스킵 (파이프/리다이렉트)
	fi, err := os.Stdout.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	// 24h 이내 이미 체크했으면 스킵
	cache, _ := loadUpdateCache()
	if time.Since(cache.LastCheck) < 24*time.Hour {
		return false
	}
	return true
}

// pendingUpdateNotice 백그라운드 체크 결과를 보관합니다.
var pendingUpdateNotice string
var pendingUpdateOnce sync.Once

// startBackgroundUpdateCheck PersistentPreRun에서 호출 — goroutine+타임아웃으로 비차단.
func startBackgroundUpdateCheck() {
	if !shouldCheckUpdate() {
		return
	}
	go func() {
		done := make(chan string, 1)
		go func() {
			tag, err := fetchLatestTagWithTimeout(3 * time.Second)
			if err != nil {
				done <- ""
				return
			}
			_ = saveUpdateCache(tag)
			done <- tag
		}()
		select {
		case tag := <-done:
			if tag != "" && normalizeVer(tag) != normalizeVer(appVersion) {
				pendingUpdateOnce.Do(func() {
					pendingUpdateNotice = tag
				})
			}
		case <-time.After(3 * time.Second):
		}
	}()
}

// printUpdateNotice 명령 종료 시점에 stderr로 업데이트 알림을 출력합니다.
// TTY에서만 호출됩니다 (shouldCheckUpdate가 이미 필터링).
func printUpdateNotice() {
	if pendingUpdateNotice == "" {
		return
	}
	tag := pendingUpdateNotice
	fmt.Fprintf(os.Stderr, "\n업데이트됨: %s → %s, 업데이트하시겠습니까? (y/N) ", appVersion, tag)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		ans := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(ans, "y") {
			if err := runUpdate(); err != nil {
				fmt.Fprintf(os.Stderr, "업데이트 실패: %v\n", err)
			}
			return
		}
	}
	// N 또는 무응답: 24h 침묵
	fmt.Fprintln(os.Stderr, "24시간 동안 알림을 표시하지 않습니다. (`sgis update`로 수동 업데이트)")
	_ = saveUpdateCache(tag)
}

// fetchLatestTagWithTimeout 타임아웃을 가진 버전으로 최신 태그를 가져옵니다.
func fetchLatestTagWithTimeout(timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 응답 코드 %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}

// fetchLatestTag GitHub 릴리스 API에서 최신 태그명을 가져옵니다.
func fetchLatestTag() (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateRepo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 응답 코드 %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("최신 릴리스 태그를 찾을 수 없습니다")
	}
	return payload.TagName, nil
}

// binaryAssetName 현재 OS/아키텍처에 맞는 릴리스 바이너리 자산명을 반환합니다.
func binaryAssetName() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		switch runtime.GOARCH {
		case "arm64":
			return "sgis-darwin-arm64", nil
		case "amd64":
			return "sgis-darwin-amd64", nil
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "sgis-linux-amd64", nil
		case "arm64":
			return "sgis-linux-arm64", nil
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "sgis-windows-amd64.exe", nil
		}
	}
	return "", fmt.Errorf("지원하지 않는 플랫폼: %s/%s (수동 설치 필요)", runtime.GOOS, runtime.GOARCH)
}

// downloadFile URL에서 파일을 받아 dest에 저장합니다 (리다이렉트 자동 추적).
func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("다운로드 응답 코드 %d (%s)", resp.StatusCode, url)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

// extractTarGz tar.gz를 dest에 추출합니다 (zip-slip 경로 이탈 방어, 기존 파일 덮어쓰기).
func extractTarGz(tarPath, dest string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	cleanDest := filepath.Clean(dest)
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." {
			continue
		}
		target := filepath.Join(cleanDest, name)
		// zip-slip 방어: 추출 경로가 dest 밖으로 벗어나지 않도록 검증
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("안전하지 않은 경로 항목: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

// replaceBinary 새 바이너리를 dst에 설치합니다.
// Unix: dst.new로 쓴 뒤 rename (원자 교체, 실행 중 프로세스는 기존 inode 유지).
// Windows: 실행 중 .exe는 직접 교체 불가 → 기존을 .old로 옮긴 뒤 새 파일 배치.
func replaceBinary(src, dst string) error {
	tmpDst := dst + ".new"
	if err := copyFile(src, tmpDst, 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(dst + ".old")
		if _, err := os.Stat(dst); err == nil {
			if err := os.Rename(dst, dst+".old"); err != nil {
				_ = os.Remove(tmpDst)
				return err
			}
		}
	}
	if err := os.Rename(tmpDst, dst); err != nil {
		_ = os.Remove(tmpDst)
		return err
	}
	return nil
}

// copyFile src를 dst로 복사하고 권한을 설정합니다.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}

// normalizeVer 비교용으로 "v" 접두사와 "-dirty"/빌드 메타데이터를 제거합니다.
func normalizeVer(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}
