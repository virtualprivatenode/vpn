# Reproducible builds

Reproducibility is a goal of the Virtual Private Node project.
As of v0.1.1 and later, it is possible to recreate the exact binary
published in the GitHub releases.

Because the project is a single statically-linked Go binary with no
bundled runtime, reproducibility is straightforward compared to
projects that bundle a JVM or native installers.

## Reproducing a release

### Prerequisites

On Debian 13+:

```bash
apt install -y git wget curl sudo
```

### Install Go

Because Go embeds its version in the binary, it is essential to have
the same version of Go installed when recreating the release. The
required version is specified in the `go.mod` file at the root of
the repository.

Note: Do not install Go using a system package manager (e.g. apt,
snap, brew). Distribution packages may patch the toolchain or use
different default flags, which will produce a non-reproducible build.

#### Go from the official downloads

Go is available for all supported platforms from
[go.dev/dl](https://go.dev/dl/).

```bash
GO_VERSION="1.26.1"  # check go.mod for exact version
wget "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
export PATH=$PATH:/usr/local/go/bin
```

#### Verify the installation

```bash
go version
```

### Building the binary

First, assign a temporary variable in your shell for the specific
release you want to build:

```bash
GIT_TAG="v0.4.0"
```

The project can then be cloned as follows:

```bash
git clone --branch "${GIT_TAG}" --depth 1 \
    https://github.com/ripsline/virtual-private-node.git
```

If you already have the repo cloned, fetch all new updates and
checkout the release:

```bash
cd virtual-private-node
git fetch --all --tags
git checkout "${GIT_TAG}"
```

Change into the project folder and build:

```bash
cd virtual-private-node
VERSION="${GIT_TAG#v}"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o rlvpn ./cmd/
```

The binary will be placed in the current directory as `rlvpn`.

### Verifying the binary is identical

Download the released binary from
[GitHub Releases](https://github.com/ripsline/virtual-private-node/releases):

```bash
VERSION="0.4.0"

wget -q "https://github.com/ripsline/virtual-private-node/releases/download/v${VERSION}/rlvpn-${VERSION}-amd64.tar.gz"
wget -q "https://github.com/ripsline/virtual-private-node/releases/download/v${VERSION}/SHA256SUMS"
wget -q "https://github.com/ripsline/virtual-private-node/releases/download/v${VERSION}/SHA256SUMS.asc"
```

Import the release signing key from the OpenPGP keyserver:

```bash
gpg --keyserver hkps://keys.openpgp.org --recv-keys AFA0EBACDC9A4C4AA7B0154AC97CE10F170BA5FE
```

Verify the signature:

```bash
gpg --verify SHA256SUMS.asc SHA256SUMS
```

Verify the checksum:

```bash
sha256sum --check --ignore-missing SHA256SUMS
```

Extract the released binary and compare:

```bash
tar -xzf "rlvpn-${VERSION}-amd64.tar.gz"
mv rlvpn rlvpn-release

sha256sum rlvpn-release rlvpn
```

Both hashes should be identical.

If there is output showing different hashes, please open an issue
with detailed instructions to reproduce, including build system
platform and Go version.

## Troubleshooting

If the checksums do not match, check the following:

| Cause | Fix |
| --- | --- |
| Different Go version | Check `go.mod` and use the exact version listed |
| Missing `-trimpath` | Local filesystem paths get embedded in the binary |
| CGO enabled | Set `CGO_ENABLED=0` explicitly |
| Different `ldflags` | Must include `-s -w -X main.version=VERSION` |
| OS/arch mismatch | Must build with `GOOS=linux GOARCH=amd64` |
| Go installed via apt/snap | Use the official tarball from go.dev/dl |
| Missing `-X main.version` | Version string will differ in the binary |

You can inspect the build metadata embedded in each binary:

```bash
go version -m rlvpn-release
go version -m rlvpn
```

Both outputs should be identical.

## What this proves

| Check | What it verifies |
| --- | --- |
| GPG signature | Release was signed by the project maintainer |
| SHA256 checksum | Download was not corrupted or tampered with |
| Reproducible build | Binary was built from the published source code |

All three together prove the binary you are running is exactly what
the source code says it is, signed by who it claims to be from, and
delivered without modification.