## Release Verification

Verify signatures before installation.

### Import the release signing key

```bash
gpg --keyserver hkps://keys.openpgp.org --recv-keys AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE
```

### Download the release files

```bash
VERSION="0.6.2"
wget -q "https://github.com/ripsline/virtual-private-node/releases/download/v${VERSION}/rlvpn-${VERSION}-amd64.tar.gz"
wget -q "https://github.com/ripsline/virtual-private-node/releases/download/v${VERSION}/SHA256SUMS"
wget -q "https://github.com/ripsline/virtual-private-node/releases/download/v${VERSION}/SHA256SUMS.asc"
```

### Verify the signature

```bash
gpg --verify SHA256SUMS.asc SHA256SUMS
```

### Verify the checksum

```bash
sha256sum --check --ignore-missing SHA256SUMS
```

The bootstrap script performs this verification automatically during
installation. This section is for users who want to verify manually
before running the one-liner.