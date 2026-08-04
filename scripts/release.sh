#!/usr/bin/env bash
#
# Build the sqlc-gen-kotlin wasm plugin and publish a GitHub release with the
# artifact and its checksum attached. Modelled on wileyatnelo/sqlc's
# scripts/release.sh; the differences are noted below.
#
# Usage:
#   scripts/release.sh <version>
#   scripts/release.sh v1.5.0
#   REPO=wileyatnelo/sqlc-gen-kotlin scripts/release.sh v1.5.0
#
# The version MUST already exist as a local git tag; this script never creates
# tags. Create it yourself first (e.g. `git tag v1.5.0`) at the commit you want
# to release, then run this. A tag containing "-" (e.g. v1.5.0-rc1) is published
# as a pre-release.
#
# Differs from sqlc's release script in three ways:
#   - One artifact, not a platform matrix: wasm is platform-independent.
#   - No version ldflag. The plugin has no version variable to embed -- the
#     `// versions:` header in generated code reports sqlc's version, not the
#     plugin's -- so the git tag is the only version identity.
#   - Delegates the build to `make`, rather than running `go build` itself. sqlc's
#     script has to inline the build because of the platform matrix and version
#     ldflag; here there is a single artifact, so reusing the Makefile target keeps
#     one definition of the build flags. Those flags include -trimpath, without
#     which the wasm embeds absolute build paths -- the same commit built from two
#     directories then produces two different hashes and a published artifact
#     cannot be reproduced or verified. Anyone can now check a release with
#     `make bin/sqlc-gen-kotlin.wasm && sha256sum bin/sqlc-gen-kotlin.wasm`.

set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: scripts/release.sh <version>   (e.g. v1.5.0)" >&2
  exit 1
fi

# Require the version to be an existing local tag; never create one.
if ! git rev-parse -q --verify "refs/tags/${VERSION}" >/dev/null; then
  echo "error: '${VERSION}' is not an existing local git tag." >&2
  echo "create it first at the commit you want to release, e.g.:" >&2
  echo "  git tag ${VERSION}" >&2
  exit 1
fi

# Build the tagged commit, not whatever happens to be checked out.
tag_commit="$(git rev-parse "${VERSION}^{commit}")"
head_commit="$(git rev-parse HEAD)"
if [ "$tag_commit" != "$head_commit" ]; then
  echo "error: HEAD (${head_commit}) is not at tag ${VERSION} (${tag_commit})." >&2
  echo "check out the tag before releasing:" >&2
  echo "  git checkout ${VERSION}" >&2
  exit 1
fi

# A dirty tree would ship code that is not in the tag.
if ! git diff-index --quiet HEAD --; then
  echo "error: working tree has uncommitted changes." >&2
  git status --short >&2
  exit 1
fi

# Default to this checkout's fork ("origin"), never upstream. Override with REPO=.
REPO="${REPO:-wileyatnelo/sqlc-gen-kotlin}"

ARTIFACT="sqlc-gen-kotlin.wasm"
DIST="dist"
rm -rf "$DIST"
mkdir -p "$DIST"

echo ">> building ${ARTIFACT} (wasip1/wasm)"
rm -f "bin/${ARTIFACT}"
make "bin/${ARTIFACT}"
cp "bin/${ARTIFACT}" "${DIST}/${ARTIFACT}"

echo ">> generating checksums"
(cd "$DIST" && sha256sum "$ARTIFACT" > checksums.txt)
sha256="$(cut -d' ' -f1 < "${DIST}/checksums.txt")"

# Pre-release when the tag carries a suffix like -rc1 / -beta.
prerelease_flag=()
if [[ "$VERSION" == *-* ]]; then
  prerelease_flag=(--prerelease)
fi

# Push the existing tag so the release references your commit. gh reuses the tag
# once it exists on the remote and will not create one.
echo ">> pushing tag ${VERSION} to ${REPO}"
git push origin "refs/tags/${VERSION}"

echo ">> creating release ${VERSION} on ${REPO}"
gh release create "$VERSION" \
  --repo "$REPO" \
  --title "$VERSION" \
  --generate-notes \
  "${prerelease_flag[@]}" \
  "${DIST}/${ARTIFACT}" \
  "${DIST}/checksums.txt"

echo ">> done: https://github.com/${REPO}/releases/tag/${VERSION}"
echo
echo "To consume this from api-v2, update GenerateSqlcConfigTask.kt:"
echo "  pluginUrl    = \"https://github.com/${REPO}/releases/download/${VERSION}/${ARTIFACT}\""
echo "  pluginSha256 = \"${sha256}\""
