#!/usr/bin/env sh
set -eu

RELEASE_REPO="ota-takeru/run-weaver"
REMOTE="origin"
BRANCH="main"
BUMP="patch"
VERSION=""
PUSH=0

usage() {
  cat <<'USAGE'
Usage: scripts/release.sh [--push] [--bump patch|minor|major] [--version vX.Y.Z] [--remote origin] [--branch main]

Prepares a run-weaver release tag. By default this is a dry run.
Use --push to create an annotated tag and push it to trigger the release workflow.
USAGE
}

die() {
  echo "release error: $*" >&2
  exit 1
}

cleanup() {
  if [ -n "${TMP_DIR:-}" ]; then
    rm -rf "$TMP_DIR"
  fi
}

need_value() {
  if [ "$#" -lt 2 ] || [ -z "$2" ]; then
    die "$1 requires a value"
  fi
}

is_semver_tag() {
  printf '%s\n' "$1" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'
}

release_preflight_builds() {
  tag="$1"
  TMP_DIR="$(mktemp -d)"
  LDFLAGS="-X github.com/ota-takeru/run-weaver/internal/cli.Version=$tag"

  mkdir -p \
    "$TMP_DIR/linux-amd64" \
    "$TMP_DIR/linux-arm64" \
    "$TMP_DIR/windows-amd64" \
    "$TMP_DIR/windows-arm64"

  GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$TMP_DIR/linux-amd64/run-weaver" ./cmd/run-weaver
  GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$TMP_DIR/linux-arm64/run-weaver" ./cmd/run-weaver
  GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$TMP_DIR/windows-amd64/run-weaver.exe" ./cmd/run-weaver
  GOOS=windows GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$TMP_DIR/windows-arm64/run-weaver.exe" ./cmd/run-weaver
}

trap cleanup EXIT

while [ "$#" -gt 0 ]; do
  case "$1" in
    --push)
      PUSH=1
      shift
      ;;
    --bump)
      need_value "$@"
      BUMP="$2"
      shift 2
      ;;
    --version)
      need_value "$@"
      VERSION="$2"
      shift 2
      ;;
    --remote)
      need_value "$@"
      REMOTE="$2"
      shift 2
      ;;
    --branch)
      need_value "$@"
      BRANCH="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "unknown argument: $1"
      ;;
  esac
done

case "$BUMP" in
  patch|minor|major) ;;
  *) die "--bump must be patch, minor, or major" ;;
esac

if [ -n "$VERSION" ] && ! is_semver_tag "$VERSION"; then
  die "--version must use vMAJOR.MINOR.PATCH format"
fi

REMOTE_URL="$(git remote get-url "$REMOTE" 2>/dev/null)" || die "git remote '$REMOTE' is not configured"
case "$REMOTE_URL" in
  *github.com[:/]"$RELEASE_REPO"*|*github.com/"$RELEASE_REPO"*)
    ;;
  *)
    die "remote '$REMOTE' must point to $RELEASE_REPO, got: $REMOTE_URL"
    ;;
esac

if [ -n "$(git status --porcelain)" ]; then
  die "worktree must be clean before preparing a release"
fi

CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
if [ "$CURRENT_BRANCH" != "$BRANCH" ]; then
  die "current branch must be '$BRANCH', got '$CURRENT_BRANCH'"
fi

git fetch --quiet "$REMOTE" "$BRANCH"
LOCAL_HEAD="$(git rev-parse HEAD)"
REMOTE_HEAD="$(git rev-parse "$REMOTE/$BRANCH")"
if [ "$LOCAL_HEAD" != "$REMOTE_HEAD" ]; then
  die "local HEAD must match $REMOTE/$BRANCH"
fi

REMOTE_TAGS="$(git ls-remote --tags "$REMOTE" 'v*' | awk '{print $2}' | sed 's#refs/tags/##; s#\^{}##' | sort -u)"

if [ -n "$VERSION" ]; then
  TAG="$VERSION"
else
  LATEST_TAG="$(printf '%s\n' "$REMOTE_TAGS" | awk '
    /^v[0-9]+\.[0-9]+\.[0-9]+$/ {
      split(substr($0, 2), parts, ".")
      major = parts[1] + 0
      minor = parts[2] + 0
      patch = parts[3] + 0
      if (!seen || major > bestMajor || (major == bestMajor && minor > bestMinor) || (major == bestMajor && minor == bestMinor && patch > bestPatch)) {
        seen = 1
        bestMajor = major
        bestMinor = minor
        bestPatch = patch
      }
    }
    END {
      if (seen) {
        printf "v%d.%d.%d\n", bestMajor, bestMinor, bestPatch
      }
    }
  ')"
  if [ -z "$LATEST_TAG" ]; then
    TAG="v0.1.0"
  else
    VERSION_BODY="${LATEST_TAG#v}"
    MAJOR="${VERSION_BODY%%.*}"
    REST="${VERSION_BODY#*.}"
    MINOR="${REST%%.*}"
    PATCH="${REST#*.}"
    case "$BUMP" in
      patch) PATCH=$((PATCH + 1)) ;;
      minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
      major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
    esac
    TAG="v${MAJOR}.${MINOR}.${PATCH}"
  fi
fi

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  die "local tag already exists: $TAG"
fi

if printf '%s\n' "$REMOTE_TAGS" | grep -Fx "$TAG" >/dev/null; then
  die "remote tag already exists: $TAG"
fi

gh auth status >/dev/null
go test ./...
release_preflight_builds "$TAG"

echo "release tag: $TAG"
echo "remote: $REMOTE ($REMOTE_URL)"
echo "branch: $BRANCH"
echo "workflow trigger: git push $REMOTE $TAG"
echo "preflight: go test ./... and release cross-builds passed"
echo

if [ "$PUSH" -eq 0 ]; then
  echo "dry run: no tag was created and nothing was pushed"
  echo "planned commands:"
  echo "  git tag -a $TAG -m 'Release $TAG'"
  echo "  git push $REMOTE $TAG"
  echo
  echo "Run with --push to create the tag and trigger GitHub Actions."
  exit 0
fi

git tag -a "$TAG" -m "Release $TAG"
git push "$REMOTE" "$TAG"

echo "pushed $TAG"
echo "Actions: https://github.com/$RELEASE_REPO/actions/workflows/release.yml"
echo "Release: https://github.com/$RELEASE_REPO/releases/tag/$TAG"
