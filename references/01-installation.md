# 01 — 설치 및 초기 설정

## 요구 사항

- 인터넷 연결 (설치 및 API 호출)
- SGIS Open API 자격증명 (consumerKey + consumerSecret)
  - 발급: https://sgis.mods.go.kr/developer/html/newOpenApi/guide/guide/getApiKey.html

별도 런타임(Node.js, Python 등) 불필요 — 단일 바이너리.

---

## 설치

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.sh | sh
```

설치 후 PATH 확인:
```bash
command -v sgis       # /Users/<user>/.local/bin/sgis 가 나와야 함
sgis --version
```

PATH에 `~/.local/bin`이 없으면 셸 프로파일에 추가:
```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc   # zsh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc   # bash
source ~/.zshrc
```

특정 버전 지정:
```bash
SGIS_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/clazic/sgis/master/scripts/install.ps1 | iex
```

설치 후 새 터미널 열고:
```powershell
sgis --version
```

한글 깨짐 시: `chcp 65001`

---

## 자격증명 설정

```bash
# 대화형 설정 (권장)
sgis config set-credential <CONSUMER_KEY> <CONSUMER_SECRET>

# 환경변수 (CI/서버)
export SGIS_CONSUMER_KEY="<KEY>"
export SGIS_CONSUMER_SECRET="<SECRET>"

# 설정 확인
sgis config get
```

---

## 설치 확인

```bash
sgis --version
sgis code stage        # 시도 17개 목록이 나오면 정상
```

---

## 제거

```bash
# macOS/Linux
curl -fsSL https://raw.githubusercontent.com/clazic/sgis/master/scripts/uninstall.sh | sh

# Windows
irm https://raw.githubusercontent.com/clazic/sgis/master/scripts/uninstall.ps1 | iex
```

---

## 업데이트

```bash
sgis update           # 최신 버전으로 업데이트
sgis update --check   # 업데이트 확인만
```

---

## 설치 경로 요약

| 항목 | macOS/Linux | Windows |
|------|-------------|---------|
| 바이너리 | `~/.local/bin/sgis` | `%LOCALAPPDATA%\Programs\sgis\sgis.exe` |
| 스킬 파일 | `~/.claude/skills/sgis/` | `%USERPROFILE%\.claude\skills\sgis\` |
| 설정 | `~/.sgis/config.yaml` | `%USERPROFILE%\.sgis\config.yaml` |
| 토큰 캐시 | `~/.sgis/token.json` | `%USERPROFILE%\.sgis\token.json` |
