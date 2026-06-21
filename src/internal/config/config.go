// Package config는 SGIS CLI의 설정 관리를 담당합니다.
// 설정 파일은 ~/.sgis/config.yaml에 저장됩니다.
// 환경변수 SGIS_CONSUMER_KEY / SGIS_CONSUMER_SECRET는 config.yaml보다 우선됩니다.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// configDirOverride는 테스트용으로 설정 디렉토리를 오버라이드하는 변수입니다.
// 테스트에서만 사용하며, 실제 ~/.sgis 디렉토리를 오염시키지 않기 위해 존재합니다.
var configDirOverride string

// Config는 SGIS CLI의 전체 설정을 나타냅니다.
type Config struct {
	ConsumerKey    string `mapstructure:"consumer_key"`
	ConsumerSecret string `mapstructure:"consumer_secret"`
	DefaultFormat  string `mapstructure:"default_format"`
	UpdateCheck    bool   `mapstructure:"update_check"`
}

// ConfigDir은 설정 디렉토리 경로를 반환합니다.
// 테스트 환경에서는 configDirOverride가 우선됩니다.
func ConfigDir() string {
	if configDirOverride != "" {
		return configDirOverride
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".sgis")
	}
	return filepath.Join(homeDir, ".sgis")
}

// ConfigFilePath는 설정 파일의 전체 경로를 반환합니다.
func ConfigFilePath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// EnsureConfigDir은 설정 디렉토리를 생성합니다 (이미 존재하면 무시).
func EnsureConfigDir() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("설정 디렉토리 생성 실패 (%s): %w", dir, err)
	}
	return nil
}

// Load는 설정 파일을 로드합니다.
// 우선순위: 환경변수(SGIS_CONSUMER_KEY/SGIS_CONSUMER_SECRET) > config.yaml > 기본값
func Load() (*Config, error) {
	v := viper.New()

	// 기본값 설정
	v.SetDefault("consumer_key", "")
	v.SetDefault("consumer_secret", "")
	v.SetDefault("default_format", "table")
	v.SetDefault("update_check", true)

	// 설정 파일 경로 설정
	configFile := ConfigFilePath()
	if _, err := os.Stat(configFile); err == nil {
		v.SetConfigFile(configFile)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("설정 파일 읽기 실패: %w", err)
		}
	}

	// 설정을 구조체로 언마샬링
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("설정 파싱 실패: %w", err)
	}

	// 환경변수가 있으면 최우선으로 적용 (절대 로그에 출력하지 않음)
	if v := os.Getenv("SGIS_CONSUMER_KEY"); v != "" {
		cfg.ConsumerKey = v
	}
	if v := os.Getenv("SGIS_CONSUMER_SECRET"); v != "" {
		cfg.ConsumerSecret = v
	}

	return cfg, nil
}

// GetCredentials는 ConsumerKey와 ConsumerSecret을 반환합니다.
// 어느 하나라도 없으면 에러를 반환합니다.
func GetCredentials() (key, secret string, err error) {
	cfg, err := Load()
	if err != nil {
		return "", "", err
	}
	if cfg.ConsumerKey == "" || cfg.ConsumerSecret == "" {
		return "", "", fmt.Errorf("서비스 ID/보안 Key가 설정되지 않았습니다\n%s", NoCredentialMessage())
	}
	return cfg.ConsumerKey, cfg.ConsumerSecret, nil
}

// SetCredentials는 서비스 ID와 보안 Key를 config.yaml에 저장합니다 (0o600).
func SetCredentials(key, secret string) error {
	if key == "" {
		return fmt.Errorf("서비스 ID는 비워둘 수 없습니다")
	}
	if secret == "" {
		return fmt.Errorf("보안 Key는 비워둘 수 없습니다")
	}

	// 기존 설정 로드 (다른 필드 보존)
	cfg, err := Load()
	if err != nil {
		// 로드 실패해도 새로 생성 가능
		cfg = &Config{DefaultFormat: "table", UpdateCheck: true}
	}
	cfg.ConsumerKey = key
	cfg.ConsumerSecret = secret

	if err := EnsureConfigDir(); err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(ConfigFilePath())
	v.SetConfigType("yaml")
	v.Set("consumer_key", cfg.ConsumerKey)
	v.Set("consumer_secret", cfg.ConsumerSecret)
	v.Set("default_format", cfg.DefaultFormat)
	v.Set("update_check", cfg.UpdateCheck)

	if err := v.WriteConfig(); err != nil {
		if err := v.SafeWriteConfigAs(ConfigFilePath()); err != nil {
			return fmt.Errorf("설정 파일 저장 실패: %w", err)
		}
	}

	// 0o600 권한 강제 적용
	if err := os.Chmod(ConfigFilePath(), 0o600); err != nil {
		return fmt.Errorf("설정 파일 권한 설정 실패: %w", err)
	}
	return nil
}

// HasCredentials는 서비스 ID와 보안 Key가 모두 설정되어 있는지 확인합니다.
func HasCredentials() bool {
	if os.Getenv("SGIS_CONSUMER_KEY") != "" && os.Getenv("SGIS_CONSUMER_SECRET") != "" {
		return true
	}
	cfg, err := Load()
	if err != nil {
		return false
	}
	return cfg.ConsumerKey != "" && cfg.ConsumerSecret != ""
}

// NoCredentialMessage는 자격증명이 없을 때 표시할 안내 메시지를 반환합니다.
func NoCredentialMessage() string {
	return `서비스 ID/보안 Key가 설정되지 않았습니다.

설정 방법:

1. 환경변수 사용 (우선):
   export SGIS_CONSUMER_KEY="your_consumer_key"
   export SGIS_CONSUMER_SECRET="your_consumer_secret"

2. 명령어로 설정:
   sgis config set-credential <서비스 ID> <보안 Key>

서비스 ID/보안 Key는 SGIS 개발자 포털에서 발급받을 수 있습니다.
https://sgis.kostat.go.kr/developer/html/newOpenApi/api/develop/apiUsageApp.html
`
}

// SetDefaultFormat은 기본 출력 형식을 설정합니다.
func SetDefaultFormat(format string) error {
	validFormats := []string{"table", "json", "csv", "geojson", "xlsx"}
	valid := false
	for _, f := range validFormats {
		if f == format {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("유효하지 않은 형식: %s (지원: table, json, csv, geojson, xlsx)", format)
	}

	cfg, err := Load()
	if err != nil {
		cfg = &Config{UpdateCheck: true}
	}
	cfg.DefaultFormat = format

	if err := EnsureConfigDir(); err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(ConfigFilePath())
	v.SetConfigType("yaml")
	v.Set("consumer_key", cfg.ConsumerKey)
	v.Set("consumer_secret", cfg.ConsumerSecret)
	v.Set("default_format", cfg.DefaultFormat)
	v.Set("update_check", cfg.UpdateCheck)

	if err := v.WriteConfig(); err != nil {
		if err := v.SafeWriteConfigAs(ConfigFilePath()); err != nil {
			return fmt.Errorf("설정 파일 저장 실패: %w", err)
		}
	}
	return nil
}

// SetConfigDirForTesting은 테스트 실행 시 설정 디렉토리를 오버라이드합니다.
// 테스트에서 실제 ~/.sgis를 오염시키지 않기 위해 사용합니다.
// 테스트 완료 후 빈 문자열을 전달하여 원상복구할 수 있습니다.
func SetConfigDirForTesting(dir string) {
	configDirOverride = dir
}
