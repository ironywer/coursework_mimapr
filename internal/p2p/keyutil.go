package p2p

import (
	"os"

	crypto "github.com/libp2p/go-libp2p/core/crypto"
)

// LoadOrCreateKey loads a private key from the given path or generates a new
// Ed25519 key and saves it to the file if it does not exist.
func LoadOrCreateKey(path string) (crypto.PrivKey, error) {
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return crypto.UnmarshalPrivateKey(data)
	}

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)
	if err != nil {
		return nil, err
	}
	data, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return nil, err
	}
	return privKey, nil
}
