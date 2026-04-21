#!/bin/bash
set -eo pipefail

# ═══════════════════════════════════════════════════════════
# Virtual Private Node — Release Build Script
#
# Reads VERSION from virtual-private-node.sh automatically.
# Builds reproducible binary, creates tarball, generates
# checksums, and signs with GPG.
#
# Usage:
#   ./scripts/release.sh           # reads version from bootstrap script
#   ./scripts/release.sh 1.0.0     # override version
# ═══════════════════════════════════════════════════════════

BINARY="rlvpn"
OUTDIR="release"

# ── Resolve version ─────────────────────────────────────────

if [ -n "$1" ]; then
    VERSION="$1"
else
    VERSION=$(grep '^VERSION=' virtual-private-node.sh | head -1 | cut -d'"' -f2)
    if [ -z "$VERSION" ]; then
        echo "ERROR: Could not read VERSION from virtual-private-node.sh"
        echo "Usage: ./scripts/release.sh [version]"
        exit 1
    fi
    echo "  Read version from virtual-private-node.sh: ${VERSION}"
fi

echo ""
echo "  Building Virtual Private Node v${VERSION}"
echo ""

# ── Clean ───────────────────────────────────────────────────

rm -rf "$OUTDIR"
mkdir -p "$OUTDIR"

# ── Build ───────────────────────────────────────────────────

echo "  Building linux/amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o "${OUTDIR}/${BINARY}" ./cmd/
echo "  ✓ Binary built"

# ── Verify version is embedded ──────────────────────────────

EMBEDDED=$(go version -m "${OUTDIR}/${BINARY}" 2>/dev/null | grep "main.version" || true)
if [ -n "$EMBEDDED" ]; then
    echo "  ✓ Version embedded: ${EMBEDDED}"
fi

# ── Create tarball ──────────────────────────────────────────

TARBALL="${BINARY}-${VERSION}-amd64.tar.gz"
cd "$OUTDIR"
tar -czf "$TARBALL" "$BINARY"
rm "$BINARY"
echo "  ✓ Created ${TARBALL}"

# ── Generate checksums ──────────────────────────────────────

sha256sum "$TARBALL" > SHA256SUMS
echo "  ✓ Generated SHA256SUMS"
cat SHA256SUMS

# ── Sign checksums ──────────────────────────────────────────

echo ""
echo "  Signing SHA256SUMS..."
gpg --armor --detach-sign SHA256SUMS
echo "  ✓ Created SHA256SUMS.asc"

# ── Verify ──────────────────────────────────────────────────

echo ""
echo "  Verifying signature..."
gpg --verify SHA256SUMS.asc SHA256SUMS
echo "  ✓ Signature valid"

# ── Summary ─────────────────────────────────────────────────

echo ""
echo "  ═══════════════════════════════════════════════════"
echo ""
echo "  Release files in ./${OUTDIR}/:"
echo ""
ls -lh "$TARBALL" SHA256SUMS SHA256SUMS.asc
echo ""
echo "  Next steps:"
echo "    1. git add -A && git commit -m 'v${VERSION}'"
echo "    2. git tag -s v${VERSION} -m 'v${VERSION}'"
echo "    3. git push origin main && git push origin v${VERSION}"
echo "    4. gh release create v${VERSION} --title 'v${VERSION}' \\"
echo "         --generate-notes ${TARBALL} SHA256SUMS SHA256SUMS.asc"
echo ""
echo "  ═══════════════════════════════════════════════════"
echo ""