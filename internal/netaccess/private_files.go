package netaccess

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
)

func certificatePath(root string, policy *Policy) string {
	digest := sha256.Sum256([]byte(policy.Config.CertificateMode + ":" + policy.Origin.Hostname()))
	return filepath.Join(root, hex.EncodeToString(digest[:])+".pem")
}

func preparePrivateDirectory(root string) error {
	if err := os.MkdirAll(root, 0700); err != nil {
		return errors.New("cannot create private certificate storage")
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("certificate storage must be a real private directory")
	}
	if err := restrictCertificatePath(root); err != nil {
		return errors.New("cannot protect certificate storage permissions")
	}
	return nil
}

// One private PEM contains both the key and its chain. An atomic replacement
// cannot leave the listener with a new certificate and an old private key.
func writePrivate(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".certificate-*")
	if err != nil {
		return errors.New("cannot write private certificate storage")
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if err = restrictCertificatePath(name); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil || os.Rename(name, path) != nil {
		return errors.New("cannot save private certificate storage")
	}
	return nil
}

func readPrivate(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 128<<10 {
		return nil, errors.New("invalid private certificate file")
	}
	if err := restrictCertificatePath(path); err != nil {
		return nil, errors.New("cannot protect private certificate file")
	}
	return os.ReadFile(path) // #nosec G304 -- fixed internal names or a SHA-256 identifier under private certificate storage.
}

func encodeKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func loadOrCreateAccountKey(root string) (*ecdsa.PrivateKey, error) {
	path := filepath.Join(root, "acme-account.pem")
	data, err := readPrivate(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, errors.New("invalid stored certificate account key")
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			if key, ok := key.(*ecdsa.PrivateKey); ok {
				return key, nil
			}
		}
		return nil, errors.New("invalid stored certificate account key")
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot read stored certificate account key")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	data, err = encodeKey(key)
	if err != nil {
		return nil, err
	}
	return key, writePrivate(path, data)
}
