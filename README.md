# Virtual Private Node

A one-command installer for a Bitcoin + Lightning node on Debian.
Bitcoin Core, LND, and Tor, configured and running in minutes.

After installation, manage your node with the beautiful TUI
or `bitcoin-cli`, `lncli`, and `systemctl`.
Private by default, simple by design. Your keys, your node.

## Screenshots

<p>
  <img src="docs/images/channels_home_dark.png" width="49%" />
  <img src="docs/images/channels_open_dark.png" width="49%" />
</p>
<p>
  <img src="docs/images/channels_home_light.png" width="49%" />
  <img src="docs/images/system_home_light.png" width="49%" />
</p>

## What gets installed

### Base (automatic)

- **Bitcoin Core** — pruned node, all P2P through Tor, GPG-verified with 5 independent signatures
- **LND** — Lightning Network daemon with Tor hidden services, installed Tor-only by default
- **Tor** — all traffic routed through Tor by default
- **UFW firewall** — deny all incoming except SSH
- **fail2ban** — brute force protection
- **Unattended upgrades** — automatic Debian security updates
- **NTP clock sync** — accurate time for block timestamps, HTLC timeouts, and macaroon expiry

### Optional (from the TUI)

- **Syncthing** — automatic LND channel backup to your local device

### Requirements

- Fresh Debian 13 Box
- 2 (v)CPU, 4+ GB RAM, 90+ GB SSD

### Privacy

- **Private channels by default.** Channel funding transactions are not linked to your node in the public graph. SCID alias hides the real channel ID from route hints when supported by the channel peer. Blinded paths (default on) go further by eliminating route hints entirely.
- **Blinded paths on invoices (default on).** Invoices use encrypted route data instead of plain hop hints. Senders can pay you without learning your node's pubkey, channel partners, or channel funding UTXOs.
- **Coin control for channel opens.** You choose which UTXOs fund each channel. One UTXO in, one channel out. No silent coin consolidation linking your channels on-chain.
- **Taproot channels (default on).** Cooperative channel closes produce a MuSig2 key-path spend, which looks identical to a regular single-sig transaction on-chain. Requires peer support.
- **Consistent P2TR address type.** All addresses (receive, change, close delivery, sweep) use the same bc1p format. P2TR has a smaller anonymity set than P2WPKH today, but matching LND's internal address type prevents change-detection fingerprints that would link your outputs regardless of anonymity set size.
- **No node alias.** Your node appears in the network graph with only its pubkey. No identifying name broadcast.
- **Tor-only by default.** All LND connections route through Tor hidden services. Your server IP is never published to the Lightning Network unless you explicitly upgrade to hybrid P2P mode.

### Quick Start

Download the signed `vpn` binary onto your Server (Box), verify
it, and run the installer:

```bash
cd /tmp
wget https://github.com/virtualprivatenode/vpn/releases/download/v0.7.0/vpn-0.7.0-amd64.tar.gz
wget https://github.com/virtualprivatenode/vpn/releases/download/v0.7.0/SHA256SUMS
wget https://github.com/virtualprivatenode/vpn/releases/download/v0.7.0/SHA256SUMS.asc

wget -O signing-key.asc https://keys.openpgp.org/vks/v1/by-fingerprint/AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE
gpg --import signing-key.asc
gpg --verify SHA256SUMS.asc SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing

tar -xzf vpn-0.7.0-amd64.tar.gz
sudo install -m 755 vpn /usr/local/bin/vpn
sudo vpn install
```

The signature must say **Good signature** with primary key
fingerprint `AFA0 EBAC DC9A 4C4A A7B0  154A C97C E10F 170B
A5FE`, and the checksum line must say `vpn-0.7.0-amd64.tar.gz:
OK`. If either fails, stop.

For testnet4:

```bash
sudo vpn install --testnet4
```

The installer checks the environment first and refuses — with a
full report, before changing anything — if the box is not one it
can trust. It then walks you through access setup and hardware
fit, installs Bitcoin Core and LND with every download after Tor
routed through Tor and verified before install, and drops you
straight into the node console. If a step fails or you
interrupt, just run `sudo vpn install` again — it resumes from
the first incomplete step.

**Access setup.** The installer creates a `vpn` admin user and
shows every SSH key it finds on the box — with fingerprints and
comments, and provider control lines excluded — for you to
confirm, replace, or extend before they are copied. You also set
a login password (16 characters minimum) as the console
fallback; whether password login over SSH stays enabled is
carried over exactly as the installer OBSERVED it on your box —
installing never silently changes it.

### Build from Source

```bash
sudo apt update && sudo apt install -y git wget sudo curl

cd /tmp
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
source ~/.profile

cd ~
git clone https://github.com/virtualprivatenode/vpn.git
cd vpn
go mod tidy
go build -o vpn ./cmd/
sudo ./vpn install
```

The installer places the binary at `/usr/local/bin/vpn` as its
first step.

### Wallet Creation

On first launch of the node console, you go straight to the wallet
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

### Dashboard

Every SSH login as `vpn` opens a terminal UI with a sidebar of
five sections plus a dark/light theme toggle:

- **Channels** — open channels with coin control; close and manage channels; view your Node Info (pubkey, URIs, QR codes for sharing); channel history
- **Wallet** — send and receive Lightning payments; payment history
- **On-Chain** — send and receive on-chain; UTXO coin control; transaction history with anchor sweep detection
- **Add-On** — install and manage Syncthing (channel backup)
- **System** — service status and logs; SSH key management and password auth toggle; auto-unlock configuration; P2P mode upgrade; self-update

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
systemctl status syncthing
```

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
later from **System → Services → LND → P2P Upgrade**:

- **Tor only** — maximum privacy, all connections through Tor
- **Hybrid (Tor + clearnet)** — better routing, your server IP is
  published to the Lightning Network

The upgrade is one-way — once your IP is published to the network
gossip, it cannot be retracted.

### Syncthing Channel Backups

Syncthing automatically syncs your LND `channel.backup` file to
your local device. No cloud services. No trust. If your Node dies,
recover your channels with your seed phrase and the backup file.

The sync connection is direct between your Node and your device
over an encrypted channel. Syncthing uses mutual TLS authentication
with device keys — only devices you explicitly approve can connect.
Discovery servers, relays, and NAT traversal are disabled.

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

- TUI runs as the unprivileged `vpn` admin user, which has **no sudo rights at all**. Privileged operations (service control, updates, config changes) go through a socket-activated root helper that serves a fixed menu of typed operations — no arbitrary commands, no arbitrary file reads — verifies the identity of every connecting process, and logs every operation to the system journal, which the admin user can read but not rewrite
- All connections through Tor (SOCKS5 port 9050)
- IPv6 disabled to prevent Tor bypass
- Stream isolation (separate circuit per connection)
- UFW firewall: SSH only, on the port(s) sshd actually listens on (+ 9735, 8080 for hybrid P2P, 22000 for Syncthing)
- Fail2ban: SSH brute-force protection
- Root SSH disabled by the installer
- SSH hardening: challenge-response, keyboard-interactive, and X11 forwarding disabled; password auth carried over exactly as OBSERVED on your box at install (toggle from System → SSH Keys once you've verified key auth works); login password changeable from the TUI
- Services run as dedicated bitcoin system user
- GPG signature verification for all software, any bad signature is a hard stop
- Unattended security upgrades with auto-reboot
- Base packages upgraded during install — behind the firewall, which comes up first — to close CVE windows on stale server images
- Syncthing backup sync: mutual TLS device approval, web UI only via Tor
- Bitcoin Core wallet disabled
- All downloads and apt operations route through Tor once Tor is up (verified by a hard gate before any download)
- Atomic config writes with fsync + rename (prevents corruption on power loss)
- Secure temp file creation with O_EXCL (prevents symlink attacks)
- Public IP detection uses kernel routing table (no external network calls)
- Mandatory seed confirmation ("I SAVED MY SEED") during wallet creation
- Auto-unlock (optional) uses a local password file with 0400 perms, never transmitted

### Privacy — Network Traffic

Downloading the `vpn` binary and signing key (Quick Start) is
ordinary clearnet traffic — Tor does not exist on the box yet.
After that, the installer makes two types of network calls:

**Phase 1 (clearnet, unavoidable):**
- `apt-get update` — Debian package index refresh
- one `apt-get install` — Tor, torsocks, ufw and base tools,
  from Debian package mirrors
- `apt-get upgrade` — Debian security updates (runs behind the
  freshly enabled firewall)
- NTP time sync enablement — ongoing clock sync queries to the
  Debian NTP pool (continues after install)

**Phase 2 (all through Tor, hard-gated):**
- Bitcoin Core and LND downloads
- Syncthing download (when Syncthing is installed)
- All subsequent apt operations

Before the first Phase-2 download, the installer verifies Tor
routing on Tor's own control port and refuses to continue if it
cannot confirm it — on every run, including resumes.

After install, the only ongoing clearnet traffic is NTP clock
sync (to the Debian NTP pool), Syncthing sync (port 22000) if
you install it, and LND P2P if you choose hybrid mode.
Everything else routes through Tor.

Verify Tor routing after install:
```bash
grep "Tor" /var/log/vpn.log
```

### Software Verification

All software is verified with GPG signatures and SHA256 checksums:

- **Bitcoin Core** — 5 trusted builder keys from
  [bitcoin-core/guix.sigs](https://github.com/bitcoin-core/guix.sigs).
  Requires 2 of 5 valid signatures. A bad signature (BADSIG) from any
  key is a hard stop.
- **LND** — Roasbeef's signing key verified against known fingerprint.
- **Syncthing** — pinned release binary, verified against the Syncthing
  release signing key's known fingerprint. The release checksums are
  clearsigned by the same key. The installer also writes Syncthing's
  entire configuration itself and refuses to start the daemon if any
  privacy setting does not verify.
- **vpn binary** — signed with a key hosted on an independent
  keyserver (not GitHub). You download and verify it yourself
  before running it (Quick Start above), and the built-in
  updater performs the same key and checksum verification for
  every later release. Hosting the key off GitHub means
  compromising one source does not compromise both the binary
  and the key.

Verification failure is a hard stop.

After installation, review the log:

```bash
cat /var/log/vpn.log
```

For manual binary verification before installation, see
[Release Verification](docs/verifying.md).

### Directory Layout

| Path | Contents |
| --- | --- |
| /etc/bitcoin/bitcoin.conf | Bitcoin Core configuration |
| /etc/lnd/lnd.conf | LND configuration |
| /etc/syncthing/ | Syncthing configuration |
| /etc/vpn/config.json | Install state and credentials |
| /var/lib/vpn/state/ | Node facts staged for the console (onion addresses, staged credentials) |
| /var/lib/bitcoin/ | Blockchain data |
| /var/lib/lnd/ | LND data and wallet |
| /var/lib/syncthing/lnd-backup/ | Auto-synced channel.backup |
| /var/log/vpn.log | Application log (install, verification, status) |

## License

Copyright (C) 2026 ripsline

This project is free software licensed under the
[GNU Affero General Public License v3.0](LICENSE).