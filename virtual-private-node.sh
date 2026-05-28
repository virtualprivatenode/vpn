#!/bin/bash
set -eo pipefail

# ═══════════════════════════════════════════════════════════
# Virtual Private Node — Bootstrap Script
#
# This script runs as root on a fresh Debian 13+ Box.
# It creates the ripsline user, downloads the rlvpn binary,
# configures auto-launch, and disables root SSH.
#
# Phase 1: Install Tor over clearnet (unavoidable)
# Phase 2: All remaining downloads route through Tor
#
# Usage:
#   curl -sL https://raw.githubusercontent.com/ripsline/virtual-private-node/main/virtual-private-node.sh | sudo bash
#   curl -sL https://raw.githubusercontent.com/ripsline/virtual-private-node/main/virtual-private-node.sh | sudo bash -s -- --testnet4
# ═══════════════════════════════════════════════════════════

VERSION="0.6.0"
BINARY_NAME="rlvpn"
ADMIN_USER="ripsline"

BASE_URL="https://github.com/ripsline/virtual-private-node/releases/download/v${VERSION}"
SIGNING_KEY_FP="AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE"

# ── Parse flags ──────────────────────────────────────────────

NETWORK="mainnet"
for arg in "$@"; do
    case "$arg" in
        --testnet4) NETWORK="testnet4" ;;
    esac
done

# ── Preflight checks ────────────────────────────────────────

if [ "$(id -u)" -ne 0 ]; then
    echo "ERROR: Run as root."
    echo "  curl -sL ripsline.com/virtual-private-node.sh | sudo bash"
    exit 1
fi

if ! grep -q "ID=debian" /etc/os-release 2>/dev/null; then
    echo "ERROR: This installer requires Debian."
    exit 1
fi

DEBIAN_VER=$(grep -oP 'VERSION_ID="\K[0-9]+' /etc/os-release 2>/dev/null || echo "0")
if [ "$DEBIAN_VER" -lt 13 ]; then
    echo "ERROR: Debian 13+ required."
    exit 1
fi

# ── Locate an SSH key source for the new admin user ────────
#
# Most VPS providers don't give you a root login — they
# provision a sudoer with the SSH key in its home dir. The
# recommended invocation is `curl … | sudo bash`, which sets
# $SUDO_USER directly. Operators who reach for `sudo su -`
# first lose $SUDO_USER (it gets wiped by su's login shell),
# so we have a cascade of fallbacks to recover the original
# user even then.
#
# Resolution order:
#   1. $SUDO_USER              (sudo bash — primary path)
#   2. logname                 (asks kernel for login name,
#                               survives sudo su -)
#   3. who (no args)           (reads utmp, first non-root)
#   4. /root/.ssh/authorized_keys  (bare-metal / legacy)
#
# Note: `who am i` was tried here originally but it depends
# on stdin being a tty, which curl|bash does not provide.
# logname and `who` (no args) do not have that problem.
#
# If none of the above yields keys, $SSH_KEY_SOURCE stays
# empty and the operator logs into ripsline with the random
# password we generate (printed at the end). They can then
# add a key via the TUI: System → SSH Keys.

SSH_KEY_SOURCE=""
CANDIDATE_USER=""

# 1. $SUDO_USER (sudo bash path)
if [ -n "${SUDO_USER:-}" ] && [ "$SUDO_USER" != "root" ]; then
    CANDIDATE_USER="$SUDO_USER"
fi

# 2. logname — kernel-level lookup of the login name
#    associated with the controlling terminal. Works through
#    sudo su - and through curl | bash.
if [ -z "$CANDIDATE_USER" ] && command -v logname &>/dev/null; then
    LN=$(logname 2>/dev/null || true)
    if [ -n "$LN" ] && [ "$LN" != "root" ]; then
        CANDIDATE_USER="$LN"
    fi
fi

# 3. who (no args) — reads utmp directly. Last resort for
#    unusual setups where logname fails.
if [ -z "$CANDIDATE_USER" ] && command -v who &>/dev/null; then
    WHO_USER=$(who 2>/dev/null | awk '$1 != "root" {print $1; exit}')
    if [ -n "$WHO_USER" ]; then
        CANDIDATE_USER="$WHO_USER"
    fi
fi

# Pick the authorized_keys file to copy from.
if [ -n "$CANDIDATE_USER" ] \
    && [ -s "/home/$CANDIDATE_USER/.ssh/authorized_keys" ]; then
    SSH_KEY_SOURCE="/home/$CANDIDATE_USER/.ssh/authorized_keys"
elif [ -s /root/.ssh/authorized_keys ]; then
    SSH_KEY_SOURCE="/root/.ssh/authorized_keys"
fi

echo ""
echo "  ╔══════════════════════════════════════════╗"
echo "  ║  Virtual Private Node — Bootstrap        ║"
echo "  ╚══════════════════════════════════════════╝"
echo ""
echo "  Network: ${NETWORK}"
if [ -n "$SSH_KEY_SOURCE" ]; then
    echo "  SSH key source: ${SSH_KEY_SOURCE}"
else
    echo "  SSH key source: (none — password login only)"
fi
echo ""

# ═══════════════════════════════════════════════════════════
# Phase 1: Clearnet — install Tor and essential dependencies
# This is the ONLY clearnet activity. After Tor starts,
# all remaining downloads go through torsocks.
# ═══════════════════════════════════════════════════════════

echo "  ── Phase 1: Installing Tor (clearnet) ──────"
echo ""

# Force non-interactive package operations throughout Phase 1.
# DEBIAN_FRONTEND suppresses debconf prompts entirely.
# NEEDRESTART_MODE=a tells needrestart (installed by default on
# Debian 13) to auto-restart services instead of showing the
# blue ncurses dialog mid-upgrade.
export DEBIAN_FRONTEND=noninteractive
export NEEDRESTART_MODE=a

apt-get update -qq

# Bring the base image current before doing anything else.
# Fresh VPS images are often weeks or months old and ship
# with unpatched CVEs. Upgrading now closes that window
# before the box starts listening on the network.
#
# confdef + confold tell dpkg to keep existing config files
# on conflict rather than prompting. On a fresh image with
# no user customizations this is the safe default.
echo "  Upgrading base packages (this may take a minute)..."
apt-get upgrade -y -qq \
    -o Dpkg::Options::="--force-confdef" \
    -o Dpkg::Options::="--force-confold"
echo "  ✓ Base packages upgraded"

apt-get install -y -qq sudo gnupg tor torsocks wget

# Ensure hostname resolves (prevents sudo delays)
if ! getent hosts "$(hostname)" >/dev/null 2>&1; then
    echo "127.0.0.1 $(hostname)" >> /etc/hosts
    echo "  ✓ Fixed hostname resolution"
fi

# Enable NTP clock sync. Bitcoin Core and LND depend on
# accurate time for block timestamps, HTLC timeouts, and
# macaroon expiry. systemd-timesyncd is present on Debian
# 13 by default and uses the standard Debian NTP pool,
# which serves UTC. No timezone setting — UTC is the
# correct default for a server.
if command -v timedatectl &>/dev/null; then
    timedatectl set-ntp true 2>/dev/null || true
    echo "  ✓ NTP clock sync enabled"
fi

# Start Tor and wait for it to bootstrap
systemctl enable tor
systemctl start tor
echo "  ✓ Tor installed and started"

# Wait for Tor to fully bootstrap (connect to network)
echo "  Waiting for Tor to bootstrap..."
for i in $(seq 1 30); do
    if torsocks curl -s --max-time 5 https://check.torproject.org/api/ip 2>/dev/null | grep -q '"IsTor":true'; then
        echo "  ✓ Tor bootstrapped — all further downloads via Tor"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "  WARNING: Tor bootstrap check timed out. Continuing anyway."
    fi
    sleep 2
done

echo ""
echo "  ── Phase 2: All downloads via Tor ──────────"
echo ""

# ═══════════════════════════════════════════════════════════
# Phase 2: Everything through Tor from here
# ═══════════════════════════════════════════════════════════

# Configure apt to use Tor for all future package operations
cat > /etc/apt/apt.conf.d/99-tor-proxy << 'APTEOF'
Acquire::http::Proxy "socks5h://127.0.0.1:9050";
Acquire::https::Proxy "socks5h://127.0.0.1:9050";
APTEOF
echo "  ✓ Configured apt to route through Tor"

# ── Download helper (always through torsocks) ───────────────

download() {
    local url="$1"
    local out="$2"
    local attempt=0
    local max_attempts=3

    while [ "$attempt" -lt "$max_attempts" ]; do
        if command -v wget &>/dev/null; then
            if torsocks wget -q -O "$out" "$url" 2>/dev/null; then
                return 0
            fi
        elif command -v curl &>/dev/null; then
            if torsocks curl -sLf -o "$out" "$url" 2>/dev/null; then
                return 0
            fi
        else
            echo "ERROR: Neither wget nor curl found."
            exit 1
        fi
        attempt=$((attempt + 1))
        if [ "$attempt" -lt "$max_attempts" ]; then
            echo "  Retry $((attempt))/$((max_attempts - 1))..."
            sleep 3
        fi
    done
    echo "ERROR: Download failed after $max_attempts attempts: $url"
    exit 1
}

# ── Create admin user ───────────────────────────────────────
#
# A fresh random password is generated on every run. On a
# re-run after a partial failure, this replaces the existing
# password so the final summary always prints a usable one.

set +o pipefail
PASSWORD=$(LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 25)
set -o pipefail
if [ ${#PASSWORD} -lt 25 ]; then
    echo "ERROR: Failed to generate secure password."
    exit 1
fi

if id "$ADMIN_USER" &>/dev/null; then
    echo "$ADMIN_USER:$PASSWORD" | chpasswd
    echo "  ✓ User $ADMIN_USER exists — password reset"
else
    adduser --disabled-password --gecos "Virtual Private Node" "$ADMIN_USER"
    echo "$ADMIN_USER:$PASSWORD" | chpasswd
    echo "  ✓ Created user $ADMIN_USER"
fi

# ── Passwordless sudo ───────────────────────────────────────

echo "$ADMIN_USER ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/$ADMIN_USER
chmod 440 /etc/sudoers.d/$ADMIN_USER
echo "  ✓ Configured passwordless sudo"

# ── Copy SSH keys from whichever source we found ────────────

if [ -n "$SSH_KEY_SOURCE" ]; then
    mkdir -p /home/$ADMIN_USER/.ssh
    cp "$SSH_KEY_SOURCE" /home/$ADMIN_USER/.ssh/authorized_keys
    chown -R $ADMIN_USER:$ADMIN_USER /home/$ADMIN_USER/.ssh
    chmod 700 /home/$ADMIN_USER/.ssh
    chmod 600 /home/$ADMIN_USER/.ssh/authorized_keys
    echo "  ✓ Copied SSH keys from $SSH_KEY_SOURCE"
fi

# ── Pre-seed network config ────────────────────────────────

install -d -m 0750 -o $ADMIN_USER -g $ADMIN_USER /etc/rlvpn
if [ ! -f /etc/rlvpn/config.json ]; then
    cat > /etc/rlvpn/config.json << CFGEOF
{
  "install_complete": false,
  "network": "${NETWORK}",
  "components": "bitcoin",
  "prune_size": 25,
  "p2p_mode": "tor",
  "auto_unlock": false,
  "lnd_installed": false,
  "wallet_created": false,
  "lit_installed": false,
  "syncthing_installed": false
}
CFGEOF
    echo "  ✓ Pre-seeded config (${NETWORK})"
fi

chown $ADMIN_USER:$ADMIN_USER /etc/rlvpn
chown $ADMIN_USER:$ADMIN_USER /etc/rlvpn/config.json

# ── Create log file ─────────────────────────────────────────

touch /var/log/rlvpn.log
chown $ADMIN_USER:$ADMIN_USER /var/log/rlvpn.log
chmod 0640 /var/log/rlvpn.log
echo "  ✓ Created log file"

# ── Download rlvpn tarball (via Tor) ────────────────────────

ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    *)
        echo "ERROR: Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

TARBALL="${BINARY_NAME}-${VERSION}-${ARCH}.tar.gz"

if command -v "$BINARY_NAME" &>/dev/null; then
    echo "  rlvpn binary already installed, skipping download."
else
    echo "  Downloading ${TARBALL} (via Tor)..."
    download "${BASE_URL}/${TARBALL}" "/tmp/${TARBALL}"

    if [ ! -s "/tmp/${TARBALL}" ]; then
        echo "ERROR: Download failed. Check your connection and try again."
        rm -f "/tmp/${TARBALL}"
        exit 1
    fi

    # ── Verify checksums + GPG signature ────────────────────────

    echo "  Downloading SHA256SUMS + signature (via Tor)..."
    download "${BASE_URL}/SHA256SUMS" "/tmp/SHA256SUMS"
    download "${BASE_URL}/SHA256SUMS.asc" "/tmp/SHA256SUMS.asc"

    echo "  Importing release signing key (via Tor)..."
    # Download key file through torsocks — no dirmngr, no keyserver protocol.
    # Key hosted on independent keyserver (not GitHub) so compromising
    # one source doesn't compromise both the binary and the key.
    KEY_FILE="/tmp/rlvpn-signing-key.asc"
    if ! download "https://keys.openpgp.org/vks/v1/by-fingerprint/${SIGNING_KEY_FP}" "$KEY_FILE"; then
        echo "ERROR: Could not download signing key."
        rm -f /tmp/${TARBALL} /tmp/SHA256SUMS /tmp/SHA256SUMS.asc
        exit 1
    fi
    if ! gpg --batch --import "$KEY_FILE" >/dev/null 2>&1; then
        echo "ERROR: Could not import signing key."
        rm -f /tmp/${TARBALL} /tmp/SHA256SUMS /tmp/SHA256SUMS.asc "$KEY_FILE"
        exit 1
    fi
    rm -f "$KEY_FILE"

    echo "  Verifying key fingerprint..."
    if ! gpg --batch --with-colons --list-keys "$SIGNING_KEY_FP" 2>/dev/null | grep -q "^fpr.*${SIGNING_KEY_FP}"; then
        echo "ERROR: Key fingerprint mismatch."
        rm -f /tmp/${TARBALL} /tmp/SHA256SUMS /tmp/SHA256SUMS.asc
        exit 1
    fi
    echo "  ✓ Key fingerprint verified"

    echo "  Verifying checksum signature..."
    if ! gpg --batch --verify /tmp/SHA256SUMS.asc /tmp/SHA256SUMS 2>/dev/null; then
        echo "ERROR: GPG signature verification failed."
        echo "  The download may be corrupted or tampered with."
        rm -f /tmp/${TARBALL} /tmp/SHA256SUMS /tmp/SHA256SUMS.asc
        exit 1
    fi
    echo "  ✓ Signature verified"

    echo "  Verifying checksum..."
    cd /tmp
    if ! sha256sum -c SHA256SUMS --ignore-missing 2>/dev/null | grep -q "${TARBALL}: OK"; then
        echo "ERROR: Checksum verification failed."
        rm -f /tmp/${TARBALL} /tmp/SHA256SUMS /tmp/SHA256SUMS.asc
        exit 1
    fi
    echo "  ✓ Checksum verified"
    cd - >/dev/null

    # ── Extract + install binary ────────────────────────────────

    tar -xzf "/tmp/${TARBALL}" -C /tmp

    if [ ! -s "/tmp/${BINARY_NAME}" ]; then
        echo "ERROR: Extracted binary not found."
        rm -f /tmp/${TARBALL} /tmp/SHA256SUMS /tmp/SHA256SUMS.asc
        exit 1
    fi

    install -m 755 "/tmp/${BINARY_NAME}" /usr/local/bin/$BINARY_NAME
    echo "  ✓ Installed rlvpn to /usr/local/bin/"

    # ── Cleanup ─────────────────────────────────────────────────

    rm -f /tmp/${TARBALL} /tmp/SHA256SUMS /tmp/SHA256SUMS.asc /tmp/${BINARY_NAME}
fi

# ── Auto-launch on ripsline login ───────────────────────────

cat > /home/$ADMIN_USER/.bash_profile << 'BASHEOF'
# Virtual Private Node — auto-launch
if [ -n "$SSH_CONNECTION" ] && [ -t 0 ]; then
    /usr/local/bin/rlvpn
fi

# Source .bashrc after rlvpn (wrappers may have been added)
[ -f ~/.bashrc ] && source ~/.bashrc
BASHEOF
chown $ADMIN_USER:$ADMIN_USER /home/$ADMIN_USER/.bash_profile
echo "  ✓ Configured auto-launch"

# ── Harden SSH daemon ───────────────────────────────────────
#
# Disables root login and several legacy/unused auth paths.
# Does NOT disable password authentication — that's a
# one-way lockout risk and belongs in the TUI's System tab,
# where the operator can verify key auth works first.

mkdir -p /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/00-rlvpn-hardening.conf << 'SSHEOF'
# Virtual Private Node — SSH hardening
# Password auth is intentionally left untouched here and is
# managed from the TUI (System tab) after key verification.
PermitRootLogin no
PubkeyAuthentication yes
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
X11Forwarding no
SSHEOF
chmod 644 /etc/ssh/sshd_config.d/00-rlvpn-hardening.conf                                                                                                                                     
echo "  ✓ Created sshd hardening drop-in"                                                                                                                                                    
                                                                                                                                                                                            
systemctl restart sshd 2>/dev/null || systemctl restart ssh                                                                                                                                  
echo "  ✓ Applied SSH hardening (root login disabled)"

# ── Log bootstrap completion ────────────────────────────────

echo "[$(date -u '+%Y-%m-%d %H:%M:%S UTC')] [bootstrap] Bootstrap v${VERSION} complete. Tor routing: active" >> /var/log/rlvpn.log
chown $ADMIN_USER:$ADMIN_USER /var/log/rlvpn.log

# ── Print instructions ──────────────────────────────────────

echo ""
echo "  ═══════════════════════════════════════════════════"
echo ""
echo "  Bootstrap complete."
echo ""
echo "  Open a NEW terminal and connect:"
echo ""
echo "    ssh $ADMIN_USER@<your-server-ip-address>"
if [ -n "$SSH_KEY_SOURCE" ]; then
    echo ""
    echo "    Your SSH key has been copied — key auth should"
    echo "    just work. Fallback password (save it!): $PASSWORD"
else
    echo "    Password: $PASSWORD"
fi
echo ""
echo "  The node installer will start automatically."
echo "  Network: ${NETWORK}"
echo ""
echo "  ⚠️  Save this password. Root SSH is now disabled."
echo "  ⚠️  Recovery: use your VPS provider's console."
if [ -z "$SSH_KEY_SOURCE" ]; then
    echo ""
    echo "  To switch to key auth: log in with the password,"
    echo "  then add your key via the TUI: System → SSH Keys."
fi
echo ""
echo "  All downloads are routed through Tor."
echo ""
echo "  ═══════════════════════════════════════════════════"
echo ""