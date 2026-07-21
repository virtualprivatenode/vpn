// internal/installer/rpcauth.go

package installer

// bitcoind RPC credentials for this node's own tooling (the
// TUI's chain status probe and the bitcoin-cli shell wrapper).
//
// The mechanism is bitcoind's rpcauth option: bitcoin.conf
// carries only a salted HMAC of the password — an attacker who
// reads the conf learns nothing usable — while the cleartext
// password is staged once on the board (root:vpn 0640) for the
// admin user's clients. Compared to bitcoind's cookie file,
// the static credential survives bitcoind restarts without
// re-reading anything, and gives this node's tooling its own
// RPC identity (which is what would make per-user method
// whitelisting possible later). LND is unaffected: it keeps
// its cookie-file configuration, which bitcoind still writes.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/virtualprivatenode/vpn/internal/helper"
	"github.com/virtualprivatenode/vpn/internal/paths"
)

// BitcoindRPCUser is the RPC identity the node's own tooling
// authenticates as.
const BitcoindRPCUser = "vpn"

// generateRPCAuth produces an rpcauth= line and the matching
// cleartext password. It reproduces Bitcoin Core's reference
// generator (share/rpcauth/rpcauth.py) exactly:
//
//   - password: 32 random bytes, unpadded URL-safe base64;
//   - salt: 16 random bytes as 32 lowercase hex chars;
//   - HMAC-SHA256 keyed by the hex salt STRING (not the raw
//     bytes — reversing this yields a line that never
//     authenticates), message = the password bytes.
func generateRPCAuth(user string) (line, password string, err error) {
	var pw [32]byte
	if _, err := rand.Read(pw[:]); err != nil {
		return "", "", fmt.Errorf("generate RPC password: %w", err)
	}
	password = base64.RawURLEncoding.EncodeToString(pw[:])

	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", "", fmt.Errorf("generate RPC salt: %w", err)
	}
	saltHex := hex.EncodeToString(salt[:])

	mac := hmac.New(sha256.New, []byte(saltHex))
	mac.Write([]byte(password))
	line = fmt.Sprintf("rpcauth=%s:%s$%s",
		user, saltHex, hex.EncodeToString(mac.Sum(nil)))
	return line, password, nil
}

// writeRPCAuthCredential regenerates the credential pair and
// installs BOTH halves: the hashed line is returned for the
// bitcoin.conf write, the cleartext is staged on the board.
// The two are only ever replaced together — a conf line
// without its staged password (or vice versa) would strand
// every client on an auth error.
func writeRPCAuthCredential() (rpcauthLine string, err error) {
	line, password, err := generateRPCAuth(BitcoindRPCUser)
	if err != nil {
		return "", err
	}
	if err := helper.WriteBoard(paths.StateBitcoindRPCPass,
		[]byte(password+"\n")); err != nil {
		return "", err
	}
	return line, nil
}
