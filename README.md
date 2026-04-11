# Virtual Private Node

A one-command installer for a private Lightning node on Debian —
Bitcoin Core, LND, and Tor, configured and running in minutes.

After installation, manage your node with the beautiful terminal UI
or `bitcoin-cli`, `lncli`, and `systemctl`.
No wrappers, no abstractions. Your keys, your node.

## Screenshots

<table>
  <tr>
    <td><img src="docs/images/create_wallet_dark.png" alt="Create Wallet Screen (Dark)" /></td>
    <td><img src="docs/images/create_wallet_light.png" alt="Create Wallet Screen (Light)" /></td>
  </tr>
  <tr>
    <td><img src="docs/images/channels_dark.png" alt="Channels Dashboard (Dark)" /></td>
    <td><img src="docs/images/channels_light.png" alt="Channels Dashboard (Light)" /></td>
  </tr>
</table>

## What gets installed

### Base (automatic)

- **Bitcoin Core** — pruned node, Tor-only P2P, GPG-verified with 5 independent signatures
- **LND** — Lightning Network daemon with Tor hidden services, installed Tor-only by default
- **Tor** — all traffic routed through Tor by default
- **UFW firewall** — deny all incoming except SSH
- **fail2ban** — brute force protection
- **Unattended upgrades** — automatic Debian security updates
- **NTP clock sync** — accurate time for block timestamps, HTLC timeouts, and macaroon expiry

### Optional (from the TUI)

- **Syncthing** — automatic LND channel backup to your local device
- **LndHub.go** — Lightning accounts

### Requirements

- Fresh Debian 13+ Box
- 2 (v)CPU, 4+ GB RAM, 90+ GB SSD
- [Mynymbox VPS with exact specs](https://client.mynymbox.io/store/custom/custom-vps-2-4-90-nl?aff=8)

### Quick Start

SSH into your Server (computer) and run:

```bash
curl -sL https://raw.githubusercontent.com/ripsline/virtual-private-node/main/virtual-private-node.sh | sudo bash
```

> [!NOTE]
> Some downloads route through Tor and can occasionally fail on the first attempt. The script is idempotent and safe to rerun. If you hit an error, just run the above command again.

This creates a `ripsline` user, copies your SSH key across automatically,
downloads the `rlvpn` binary, installs Bitcoin Core + LND + Tor, and
hardens the SSH daemon. Follow the on-screen instructions to SSH in as
`ripsline` — Bitcoin Core begins syncing and the TUI opens to the wallet
creation flow.

For testnet4:

```bash
curl -sL https://raw.githubusercontent.com/ripsline/virtual-private-node/main/virtual-private-node.sh | sudo bash -s -- --testnet4
```

**SSH key discovery.** The bootstrap tries several ways to find an
existing SSH key to copy to the new `ripsline` user: `$SUDO_USER`'s
`authorized_keys`, then `logname`, then `who`, then `/root/.ssh/`.
This works for `curl | sudo bash`, `sudo su -` followed by curl, and
bare-metal root installs. If no key is found, a random password is
printed at the end and you can add a key later from the TUI.

### Dashboard

Every SSH login as `ripsline` opens a terminal UI with a sidebar of
five sections plus a dark/light theme toggle:

- **Channels** — open, close, and manage Lightning channels; view your Node Info (pubkey, URIs, QR codes for sharing); channel history
- **Wallet** — send and receive Lightning payments; payment history
- **On-Chain** — send and receive on-chain; UTXO coin control; transaction history with anchor sweep detection
- **Add-On** — install and manage Syncthing (channel backup) and LndHub (Lightning accounts)
- **System** — service status and logs; auto-unlock configuration; P2P mode upgrade; self-update

Detail views open in tabs within each section. Press `ctrl+c` to quit
and drop to a shell:

```bash
bitcoin-cli getblockchaininfo
bitcoin-cli getpeerinfo

lncli getinfo
lncli walletbalance

# Services
systemctl status bitcoind
systemctl status tor@default
systemctl status lnd
systemctl status lndhub
systemctl status syncthing
```

### Build from Source

```bash
apt update && apt install -y git wget sudo curl

cd /tmp
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
source ~/.profile

cd ~
git clone https://github.com/ripsline/virtual-private-node.git
cd virtual-private-node
go mod tidy
go build -o rlvpn ./cmd/
sudo install -m 755 ./rlvpn /usr/local/bin/rlvpn
curl -sL https://raw.githubusercontent.com/ripsline/virtual-private-node/main/virtual-private-node.sh | sudo bash
```

The bootstrap script detects that `rlvpn` is already installed and
skips the download.

### Software Verification

All software is verified with GPG signatures and SHA256 checksums:

- **Bitcoin Core** — 5 trusted builder keys from
  [bitcoin-core/guix.sigs](https://github.com/bitcoin-core/guix.sigs).
  Requires 2 of 5 valid signatures. A bad signature (BADSIG) from any
  key is a hard stop.
- **LND** — Roasbeef's signing key verified against known fingerprint.
- **LndHub.go** — built from source at pinned release tag (v1.0.2).
  No prebuilt binary is used. The Go toolchain compiles directly from
  the [getAlby/lndhub.go](https://github.com/getAlby/lndhub.go) repository.
- **rlvpn binary** — signed with a key hosted on an independent
  keyserver (not GitHub). The bootstrap downloads the key file directly
  through Tor rather than using keyserver protocols, so compromising
  one source does not compromise both the binary and the key.

Verification failure is a hard stop.

After installation, review the log:

```bash
cat /var/log/rlvpn.log
```

For manual binary verification before installation, see
[Release Verification](docs/verifying.md).

### Wallet Creation

On first TUI launch after bootstrap, you'll go straight to the wallet
creation flow:

1. Read the privacy and seed warnings, press Proceed
2. Wait for LND to become ready
3. Type a wallet password
4. Write down your 24-word seed on paper
5. Type `I SAVED MY SEED` to confirm

The confirmation phrase is required — there is no skip. Once confirmed,
the flow transitions into auto-unlock configuration so you don't have
to manually unlock on every reboot.

A note on cancellation: pressing `ctrl+c` during the password prompt is
a legitimate escape hatch (no seed has been generated yet, nothing is
written to disk). Once you've seen your seed, `ctrl+c` is blocked by
design — the only way forward is typing the confirmation phrase.

### Connecting Zeus Wallet

Open the **Wallet** section in the TUI for Zeus pairing — scan a QR
code or copy the connection string. Both Tor and clearnet pairings
are supported if your node is in hybrid P2P mode.

#### Tor only (default)
1. Open the Wallet section → Pair Wallet
2. In Zeus: Advanced Set-Up → LND (REST)
3. Scan the QR code, or copy the server address, REST port (8080),
   and macaroon

#### Clearnet + Tor (hybrid mode)
1. Upgrade to hybrid P2P mode from System → P2P Upgrade
2. Open the Wallet section → Pair Wallet
3. Both clearnet (IP:8080) and Tor connection strings are available
4. First clearnet connection: accept the self-signed certificate
   warning — the connection is encrypted with LND's auto-refreshed
   TLS certificate

Note: Clearnet is faster. Tor is more private. Both use the same macaroon.

### Sharing Your Node

The **Channels** section has a **Node Info** tab that displays
everything a peer needs to open a channel with you:

- Node alias, pubkey, LND version
- Peer count, active channels, node capacity
- Outbound, inbound, on-chain, and total spendable balances
- QR codes for your advertised URIs (Tor, clearnet, or both)
- A `Copy URIs` button that drops to a shell view with clean
  clearnet/Tor section labels for easy copy-paste

### P2P Mode

LND is installed Tor-only by default. You can upgrade to hybrid mode
later from **System → P2P Upgrade**:

- **Tor only** — maximum privacy, all connections through Tor
- **Hybrid (Tor + clearnet)** — better routing, your server IP is
  published to the Lightning Network

The upgrade is one-way — once your IP is published to the network
gossip, it cannot be retracted.

### LndHub — Lightning Accounts

LndHub.go provides separate Lightning wallet accounts backed by your
LND node. Create accounts for family, friends, or AI agents from the
Add-On section. Each account gets isolated credentials and connects
via Zeus or any LndHub-compatible wallet.

**How it works:**

1. Install LndHub from the Add-On section
2. Create accounts from the LndHub management screen
3. Share the login, password, and server address with the user
4. They connect Zeus: Advanced Set-Up → LndHub → enter credentials
5. Fund their account by paying an invoice they generate

**Privacy model:**

- Passwords are shown once at creation and never stored
- The admin cannot see user balances through the TUI
- Account deactivation records the balance for refund purposes
- LndHub uses a dedicated macaroon with minimal LND permissions
  (info:read, invoices:read/write, offchain:read/write)

**Built from source:**

LndHub.go is cloned from GitHub at a pinned release tag and compiled
on your server using the Go toolchain. No prebuilt binaries are
downloaded. PostgreSQL is installed as the database backend.

**Clearnet note:** Clearnet connections (hybrid P2P mode) are encrypted
via a TLS reverse proxy. Tor connections use HTTP through the encrypted
Tor tunnel. Both are secure in transit.

### Syncthing Channel Backups

Syncthing automatically syncs your LND `channel.backup` file to
your local device. No cloud services. No trust. If your Node dies,
recover your channels with your seed phrase and the backup file.

The sync connection is direct between your Node and your device
over an encrypted channel. Syncthing uses mutual TLS authentication
with device keys — only devices you explicitly approve can connect.
Discovery servers and relays are disabled.

**Setup summary:**

1. Install Syncthing on your device from [syncthing.net](https://syncthing.net)
2. Disable discovery, relays, and NAT traversal in local Syncthing settings
3. Pair your device from the Add-On section in the dashboard
4. Add the Node as a remote device in your local Syncthing
5. Accept the backup folder share and set it to Receive Only

Your `channel.backup` syncs automatically whenever both devices are
online. The Syncthing web UI on the Node is accessible over Tor for
advanced configuration.

For the full setup guide, see
[Syncthing Setup Guide](docs/syncthing.md).

### Security

- TUI runs as unprivileged user, sudo per-action (not root)
- All connections through Tor (SOCKS5 port 9050)
- IPv6 disabled to prevent Tor bypass
- Stream isolation (separate circuit per connection)
- UFW firewall: SSH only (+ 9735, 8080 for hybrid P2P, 3000 for LndHub hybrid, 22000 for Syncthing)
- Fail2ban: SSH brute-force protection
- Root SSH disabled after bootstrap
- SSH hardening: challenge-response, keyboard-interactive, and X11 forwarding disabled (password auth left on by default — toggle from the TUI once you've verified key auth works)
- Services run as dedicated bitcoin system user
- GPG signature verification for all software
- Signing key hosted on independent keyserver with pinned fingerprint, downloaded as a file through Tor
- Bad signature detection — any BADSIG is a hard stop
- Unattended security upgrades with auto-reboot
- Base packages upgraded during bootstrap to close CVE windows on stale server images
- LND channel backup auto-synced via Syncthing (mutual TLS, direct connection, no cloud)
- Syncthing sync port (22000) rejects unapproved devices via mutual TLS before any data exchange
- Syncthing web UI accessible only via Tor
- Bitcoin Core wallet disabled
- All downloads after Tor installation route through torsocks
- apt package manager configured to use Tor SOCKS proxy
- Atomic config writes with fsync + rename (prevents corruption on power loss)
- Secure temp file creation with O_EXCL (prevents symlink attacks)
- Database queries protected by strict input validation
- LndHub TLS proxy: rate limited (10 req/s), X-Forwarded-For stripped
- Public IP detection uses kernel routing table (no external network calls)
- Mandatory seed confirmation ("I SAVED MY SEED") during wallet creation
- Auto-unlock (optional) uses a local password file with 0400 perms, never transmitted

### Privacy — Network Traffic

The bootstrap script makes two types of network calls:

**Phase 1 (clearnet, unavoidable):**
- `apt-get update` — Debian package index refresh
- `apt-get upgrade` — Debian security updates
- `apt-get install tor torsocks gnupg sudo wget` — Debian package mirrors
- NTP time sync enablement — ongoing clock sync queries to the Debian NTP pool (continues after bootstrap)

**Phase 2 (all through Tor):**
- rlvpn binary download from GitHub
- GPG signing key file download from independent keyserver
- Bitcoin Core and LND downloads
- Go toolchain download (when LndHub is installed)
- Syncthing repository key (when Syncthing is installed)
- All subsequent apt operations

After bootstrap, the only ongoing clearnet traffic is NTP clock sync
(to the Debian NTP pool), Syncthing sync (port 22000) if you install
it, and LND P2P if you choose hybrid mode. Everything else routes
through Tor.

Verify Tor routing after install:
```bash
grep "Tor" /var/log/rlvpn.log
```

### Architecture

```
User SSH → ripsline@<server-ip-address> → rlvpn TUI (non-root)
                             ↓
              sudo per-action → systemctl, bitcoin-cli, lncli
              ctrl+c → shell with bitcoin-cli, lncli wrappers

Services (systemd, run as bitcoin user):
  tor.service        → SOCKS proxy, hidden services
  bitcoind.service   → pruned node, Tor-routed, wallet disabled
  lnd.service        → Lightning daemon, Tor-only by default
  lndhub.service     → Lightning accounts (add-on, built from source)
  lndhub-proxy       → TLS reverse proxy for LndHub (hybrid mode only)
  syncthing.service  → channel backup sync (add-on)
```

### Directory Layout

| Path | Contents |
| --- | --- |
| /etc/bitcoin/bitcoin.conf | Bitcoin Core configuration |
| /etc/lnd/lnd.conf | LND configuration |
| /etc/syncthing/ | Syncthing configuration |
| /etc/lndhub/lndhub.env | LndHub configuration and secrets |
| /etc/rlvpn/config.json | Install state and credentials |
| /var/lib/bitcoin/ | Blockchain data |
| /var/lib/lnd/ | LND data and wallet |
| /var/lib/lndhub/ | LndHub data |
| /var/lib/syncthing/lnd-backup/ | Auto-synced channel.backup |
| /var/log/rlvpn.log | Application log (install, verification, status) |

## License

Copyright (C) 2026 ripsline

This project is free software licensed under the
[GNU Affero General Public License v3.0](LICENSE).