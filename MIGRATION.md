# Migrating from rlvpn to vpn (v0.6.3 → v0.7.0)

Virtual Private Node has a new home and a new name. The project moved to
`github.com/virtualprivatenode/vpn`, the binary is now `vpn`, the admin
user is now `vpn`, and the bootstrap script is gone — installation is a
subcommand of the signed binary itself: `sudo vpn install`.

This is a **one-time manual upgrade**. The built-in updater cannot cross
the rename (it looks for release files under the old name, which no longer
exist), and that is deliberate: it makes a half-migrated box impossible.
If your node shows an update it fails to download — this document is why.
Every release after this one updates through the TUI as before.

The whole procedure takes about 15 minutes of your attention. Your node is
offline for a few minutes in the middle — the same as a reboot.

---

## What this migration does NOT touch

Your node's identity and your money live in places the rename never
visits:

- **Bitcoin chain data** (`/var/lib/bitcoin`) — no re-sync, no re-download
- **LND wallet, channels, seed, macaroons** (`/var/lib/lnd`) — no channel
  closes, no force-closes, nothing to move or restore
- **Tor onion keys** (`/var/lib/tor`) — your onion addresses stay the same
- **Syncthing data and device pairings** (`/var/lib/syncthing`,
  `/etc/syncthing`) — paired phones keep working
- **SSH host keys** — your SSH client sees the same server, no warnings

**You do not need to close channels, move funds, or touch your seed.**
What happens to LND is a restart — the same thing every reboot already
does, and channels are built to survive that. Just don't migrate in the
middle of heavy payment activity: pick a quiet moment, since in-flight
payments during the downtime resolve by their normal timeout rules.

## Before you begin

- Log in as `ripsline` with your SSH key and **keep that session open for
  the entire procedure**. Exit the TUI to a shell.
- Confirm a channel backup exists (skip if you have no channels):

  ```
  ls -l /var/lib/lnd/data/chain/bitcoin/mainnet/channel.backup
  ```

  (Use `testnet4` in the path if that's your network.)
- Know how to reach your VPS provider's console. You should not need it —
  it's the standard fallback for any change that touches SSH.

## Step 1 — Download and verify the new binary

From your `ripsline` shell (downloads route through Tor, as always):

```
cd /tmp
torsocks wget https://github.com/virtualprivatenode/vpn/releases/download/v0.7.0/vpn-0.7.0-amd64.tar.gz
torsocks wget https://github.com/virtualprivatenode/vpn/releases/download/v0.7.0/SHA256SUMS
torsocks wget https://github.com/virtualprivatenode/vpn/releases/download/v0.7.0/SHA256SUMS.asc
```

Import the signing key — the **same key** that signed every rlvpn release:

```
torsocks wget -O signing-key.asc https://keys.openpgp.org/vks/v1/by-fingerprint/AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE
gpg --import signing-key.asc
```

Verify the signature and the checksum:

```
gpg --verify SHA256SUMS.asc SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
```

The signature must say **Good signature** with primary key fingerprint
`AFA0 EBAC DC9A 4C4A A7B0  154A C97C E10F 170B A5FE`, and the checksum
line must say `vpn-0.7.0-amd64.tar.gz: OK`. If either fails, stop and ask
before proceeding.

Install the binary:

```
tar -xzf vpn-0.7.0-amd64.tar.gz
sudo install -m 755 vpn /usr/local/bin/vpn
```

## Step 2 — Carry your settings over

Your existing settings file is compatible as-is. Copy it to the new
location (the installer fixes ownership):

```
sudo mkdir -p /etc/vpn
sudo cp /etc/rlvpn/config.json /etc/vpn/config.json
```

The installer reads this as your previous answers — network, prune size,
components, auto-unlock — so it won't ask you to re-decide things your
node already settled.

## Step 3 — Run the installer

```
sudo vpn install
```

What you'll see:

- **Environment checks** run first; on a working box they pass silently.
- **The access step** shows the SSH keys found on the box — including
  your existing `ripsline` keys, with fingerprints. Confirm to copy them
  to the new `vpn` user. You'll also set a login password for the `vpn`
  user (16 characters minimum) as a console fallback.
- **Component reinstall**: Bitcoin Core and LND are re-downloaded over
  Tor, re-verified, and reinstalled at the same versions. Services
  restart briefly — this is the downtime window. Your data directories
  are not touched.
- **SSH hardening** is rewritten under the new name. The installer reads
  your *current effective* SSH settings from the running service first,
  so whatever you had — including password login disabled, if you
  disabled it — stays exactly as it was. The old
  `00-rlvpn-hardening.conf` drop-in is removed automatically.

If the installer fails partway, re-run `sudo vpn install` — it resumes
from the first incomplete step and re-verifies Tor routing on every pass.
Nothing about your old setup has been removed, so the box is never
stranded.

## Step 4 — Verify the new login BEFORE any cleanup

Open a **new** terminal (leave the `ripsline` session open):

```
ssh vpn@<your-server-ip>
```

The TUI should launch. Check that your wallet, channels, and onion
addresses look right. If you don't use auto-unlock, unlock the wallet —
same as after any restart.

**Do not continue until this login works.** If it doesn't, go back to
your `ripsline` session (which still has full access) and fix it — or
ask. Nothing has been lost; the old setup still works at this point.

## Step 5 — Remove the old identity

From the still-open `ripsline` session, one command. It must be a single
command, in this order, because it removes ripsline's own admin rights
along the way:

```
sudo sh -c 'rm -f /usr/local/bin/rlvpn; rm -rf /etc/rlvpn; rm -f /var/log/rlvpn.log; rm -f /etc/sudoers.d/ripsline; userdel -r -f ripsline'
```

`userdel` will warn that the user is currently logged in — that's
expected and fine; your session keeps working until you close it. If you
want to keep the old log for your records, copy `/var/log/rlvpn.log`
somewhere first.

Then `exit`. The migration is complete.

Your box now has: the `vpn` binary, the `vpn` admin user — which by
design has **no sudo rights at all** (privileged operations go through
the node's own root helper) — and no trace of the old name.

## If something goes wrong

Until Step 5, the migration is reversible: the old binary, config, user,
and admin rights are all still in place, and logging in as `ripsline`
still works (it will still launch the old TUI). The one thing replaced
along the way is the SSH hardening file — and its settings are carried
into the new one, so behavior doesn't change.

The two rules that keep you safe:

1. Keep the `ripsline` session open until Step 4 passes.
2. Never run Step 5 before Step 4 passes.

## Moving to a different box instead

If you'd rather start on a fresh server, treat it as **moving a
Lightning node**, not as this upgrade: cooperatively close your channels
from the old box, wait for settlement, send the funds on-chain to a
wallet you control, install fresh on the new box, create a **new**
wallet, and reopen channels.

⚠️ **Do not use the channel backup (SCB) to migrate.** SCB restore is
disaster recovery, not a moving tool: it force-closes every channel,
your funds come back only after time-locks expire, and it depends on
your channel partners' cooperation. Worse, running two nodes from the
same seed — even briefly, even by accident — can lead to loss of funds.
If the old box is alive and healthy, close cooperatively; the backup is
for when it isn't.

## After migration

Routine updates return to the TUI: you'll see the update notice, confirm
it, and the node downloads and verifies the release itself — same
signing key, same verification, done for you. Any future release that
ever requires manual steps will say so clearly in its release notes;
this rename is expected to be the only one.