# 릴리스

`VERSION`이 `0.4.0`이면 Git tag는 `v0.4.0`, Docker image 이름은 `kanvas-v0.4.0:latest`, GitHub Release asset은 `kanvas-v0.4.0.tar.gz`입니다.

## 로컬 검증

```bash
make test
make release-tar
docker load < dist/kanvas-v0.4.0.tar.gz
```

## GitHub

```bash
git tag -a v0.4.0 -m 'Kanvas v0.4.0'
git push origin main v0.4.0
```

`.github/workflows/release.yml`은 tag와 `VERSION` 일치를 검증하고 linux/amd64 image를 빌드합니다. GitHub Release에 직접 첨부하는 프로젝트 산출물은 `kanvas-v<버전>.tar.gz` 하나뿐입니다. GitHub가 자동으로 보여주는 source archive는 플랫폼 기본 동작입니다.

릴리스 전에는 Go test, React production build, image user/entrypoint 검증이 실패 없이 끝나야 합니다.
