package netaccess

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func serialNumber() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func localAuthority(root string) (*tls.Certificate, error) {
	path := filepath.Join(root, "local-authority.pem")
	data, err := readPrivate(path)
	if errors.Is(err, os.ErrNotExist) {
		data, err = createLocalAuthority()
		if err == nil {
			err = writePrivate(path, data)
		}
	}
	if err != nil {
		return nil, errors.New("cannot read or create the local certificate authority")
	}
	pair, err := tls.X509KeyPair(data, data)
	if err != nil {
		return nil, errors.New("the stored local certificate authority is invalid")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || !leaf.IsCA || time.Now().Before(leaf.NotBefore) || time.Until(leaf.NotAfter) < 24*time.Hour {
		return nil, errors.New("the local certificate authority is invalid or expiring; use a new data directory and enroll its trust certificate")
	}
	pair.Leaf = leaf
	return &pair, nil
}

func createLocalAuthority() ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "MagicHandy local trust"},
		NotBefore: time.Now().Add(-5 * time.Minute), NotAfter: time.Now().AddDate(5, 0, 0),
		IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyPEM, err := encodeKey(key)
	return append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), keyPEM...), err
}

func issueLocalCertificate(root, host string) ([]byte, error) {
	ca, err := localAuthority(root)
	if err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := serialNumber()
	if err != nil {
		return nil, err
	}
	expires := time.Now().AddDate(0, 3, 0)
	if ca.Leaf.NotAfter.Before(expires) {
		expires = ca.Leaf.NotAfter
	}
	template := &x509.Certificate{SerialNumber: serial, NotBefore: time.Now().Add(-5 * time.Minute), NotAfter: expires,
		IPAddresses: []net.IP{net.ParseIP(host)}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.Leaf, &key.PublicKey, ca.PrivateKey)
	if err != nil {
		return nil, err
	}
	keyPEM, err := encodeKey(key)
	return append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), keyPEM...), err
}

// LocalTrustCertificate downloads only the public root, never its signing key.
func (a *Automation) LocalTrustCertificate() ([]byte, error) {
	data, err := readPrivate(filepath.Join(a.root, "local-authority.pem"))
	if err != nil {
		return nil, errors.New("prepare a LAN certificate before downloading its trust certificate")
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("local trust certificate is unavailable")
	}
	return pem.EncodeToMemory(block), nil
}
