// Command gencerts creates a self-signed CA + server certificate for the webhook.
//
//	go run ./scripts/gencerts -out config/webhook/certs
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	outDir := flag.String("out", "config/webhook/certs", "output directory")
	service := flag.String("service", "backup-operator-webhook", "webhook service name")
	namespace := flag.String("namespace", "backup-system", "webhook service namespace")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}

	cn := fmt.Sprintf("%s.%s.svc", *service, *namespace)
	dnsNames := []string{
		*service,
		fmt.Sprintf("%s.%s", *service, *namespace),
		cn,
		fmt.Sprintf("%s.%s.svc.cluster.local", *service, *namespace),
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fail(err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          mustSerial(),
		Subject:               pkix.Name{CommonName: "backup-operator-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		fail(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		fail(err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fail(err)
	}
	serverTpl := &x509.Certificate{
		SerialNumber: mustSerial(),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		fail(err)
	}

	mustWritePEM(filepath.Join(*outDir, "ca.crt"), "CERTIFICATE", caDER)
	mustWritePEM(filepath.Join(*outDir, "tls.crt"), "CERTIFICATE", serverDER)

	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		fail(err)
	}
	mustWritePEM(filepath.Join(*outDir, "tls.key"), "EC PRIVATE KEY", serverKeyDER)

	fmt.Printf("wrote certs to %s (CN=%s)\n", *outDir, cn)
}

func mustSerial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		fail(err)
	}
	return n
}

func mustWritePEM(path, typ string, der []byte) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: der}); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "gencerts: %v\n", err)
	os.Exit(1)
}
