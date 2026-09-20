// internal/installer/rpcauth.go

package installer

// bitcoind RPC credentials for the two local clients: this
// node's own tooling (the TUI and bitcoin-cli wrapper), and LND.
//
// The mechanism is bitcoind's rpcauth option: bitcoin.conf
// carries only a salted HMAC of the password — an attacker who
// reads the conf learns nothing usable — while the cleartext
// password is staged once on the board (root:vpn 0640) for the
// admin user's clients. Compared to bitcoind's cookie file,
// the static credential survives bitcoind restarts without
// re-reading anything, and gives this node's tooling its own
// RPC identity (which is what would make per-user method
// whitelisting possible later). LND receives an independent
// rpcauth identity and keeps its cleartext half only in the
// protected LND configuration; it never reads bitcoind's data
// directory, cookie, or configuration file.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/virtualprivatenode/vpn/internal/host"
)

const (
	// BitcoindRPCUser is the identity the node's own tooling uses.
	BitcoindRPCUser = "vpn"
)

type nodeRPCAuthCredentials struct {
	lines       []string
	lndPassword string
}

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

// writeRPCAuthCredentials regenerates both local identities.
// The TUI password is staged on the board; the LND password is
// returned only long enough for the caller to write lnd.conf.
func writeRPCAuthCredentials() (nodeRPCAuthCredentials, error) {
	uiLine, uiPassword, err := generateRPCAuth(BitcoindRPCUser)
	if err != nil {
		return nodeRPCAuthCredentials{}, err
	}
	lndLine, lndPassword, err := generateRPCAuth(host.LNDBitcoindRPCUser)
	if err != nil {
		return nodeRPCAuthCredentials{}, err
	}
	if err := host.StageBitcoindRPCPassword(uiPassword); err != nil {
		return nodeRPCAuthCredentials{}, err
	}
	return nodeRPCAuthCredentials{
		lines:       []string{uiLine, lndLine},
		lndPassword: lndPassword,
	}, nil
}
