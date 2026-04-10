# Syncthing Setup Guide

Syncthing automatically syncs your LND `channel.backup` file from
your Virtual Private Node (Node) to your local device. If your Node
dies, you can recover your channels with your 24-word seed phrase
and this backup file.

Syncthing encrypts all connections using mutual TLS authentication.
Only devices you explicitly approve can connect. The `channel.backup`
file is useless without your seed phrase.

### Step 1 — Install Syncthing on Your Node

1. SSH into your Node as `ripsline`
2. Open the **Add-On** section in the sidebar
3. Select **Syncthing** and press Enter to install

### Step 2 — Install Syncthing on Your Local Computer/Phone

Download Syncthing for your operating system from
[syncthing.net](https://syncthing.net/downloads/).

- **macOS:** Download the `.dmg` or `brew install syncthing`
- **Windows:** Download the installer
- **Linux:** Install from your package manager
- **Android:** [researchxxl/syncthing-android](https://github.com/researchxxl/syncthing-android)

Start Syncthing. It opens a web UI at `http://127.0.0.1:8384`.

### Step 3 — Configure Local Syncthing

In your local Syncthing web UI, go to **Actions → Settings →
Connections** and disable the following:

- **Global Discovery** — off
- **Local Discovery** — off
- **Relaying** — off
- **NAT Traversal** — off

Click **Save**. This ensures your Syncthing only connects directly
to your Node by IP address, with no third-party relay or discovery
servers involved.

### Step 4 — Get Your Local Device ID

In your local Syncthing web UI:

1. Click **Actions** (top right)
2. Click **Show ID**
3. Copy the Device ID (looks like `XXXXXXX-XXXXXXX-...`)

### Step 5 — Pair Your Device on the Node

In the Node dashboard:

1. Open the **Add-On** section in the sidebar
2. Select **Syncthing** and press Enter to open the management tab
3. Select **Pair Device** and press Enter
4. Paste your local Device ID
5. Confirm with the Pair button

The Node adds your device and shares the backup folder automatically.

### Step 6 — Add the Node in Your Local Syncthing

In your local Syncthing web UI:

1. Click **Add Remote Device**
2. Paste the **Node Device ID** shown on the Syncthing post-pair
   screen in the TUI (or scan the QR code with a phone)
3. Under **Addresses**, replace `dynamic` with `tcp://<node-ip>:22000`
   using the same IP you SSH into
4. Click **Save**

### Step 7 — Accept the Backup Folder

After the devices connect, your local Syncthing will prompt you
to accept a shared folder called `lnd-backup`.

1. Click **Accept** (or **Add**)
2. Choose a local folder path, for example:
    - macOS: `~/lnd-backup`
    - Windows: `C:\Users\YourName\lnd-backup`
    - Linux: `~/lnd-backup`
3. Set the folder to **Receive Only**
4. Click **Save**

### Done

Your `channel.backup` file will sync automatically whenever:

- Your LND channel state changes (open, close, update)
- Your local device is online and connected

The sync happens in seconds. You don't need to keep your computer
on all the time — Syncthing will catch up the next time both
devices are online.

### Verify It's Working

Check that `channel.backup` appears in your local folder:

- **macOS/Linux:** `ls ~/lnd-backup/`
- **Windows:** Open the folder in Explorer

In the Node TUI, the Syncthing service should show as running in
the System section.

### Security

- Syncthing uses mutual TLS authentication — only devices you
  approve can connect
- The sync port (22000) rejects all unapproved devices immediately
- The `channel.backup` file is encrypted by LND and useless
  without your 24-word seed phrase
- Discovery and relay servers are disabled — your device connects
  directly to the Node by IP address
- The Syncthing web UI on the Node is only accessible via Tor
  (not exposed to clearnet)

### Troubleshooting

**Devices not connecting:**

- Verify both devices are running (green dot in web UI)
- Check that the Node address is correct: `tcp://<node-ip>:22000`
- Check firewall: `sudo ufw status` should show port 22000 open

**Folder not syncing:**

- Check that the folder is shared with both devices
- Node side should be **Send Only**
- Local side should be **Receive Only**
- Check Syncthing logs: **Actions → Logs** in the web UI

**Web UI access on Node:**

The Syncthing web UI is available over Tor for advanced
configuration. Open the Syncthing management tab in the Add-On
section and select **Web UI** — the onion address and credentials
are displayed there. Use Tor Browser to access it.