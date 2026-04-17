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
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	certPath := flag.String("cert", "server-cert.pem", "output certificate path")
	keyPath := flag.String("key", "server-key.pem", "output private key path")
	commonName := flag.String("cn", "easyvpn-data", "certificate common name")
	dnsNames := flag.String("dns", "", "comma-separated DNS names")
	ipAddrs := flag.String("ip", "", "comma-separated IP addresses")
	days := flag.Int("days", 825, "certificate validity in days")
	flag.Parse()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatalf("generate private key: %v", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		log.Fatalf("generate serial number: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: *commonName,
		},
		NotBefore:             time.Now().Add(-5 * time.Minute).UTC(),
		NotAfter:              time.Now().Add(time.Duration(*days) * 24 * time.Hour).UTC(),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, raw := range splitList(*dnsNames) {
		template.DNSNames = append(template.DNSNames, raw)
	}
	for _, raw := range splitList(*ipAddrs) {
		ip := net.ParseIP(raw)
		if ip == nil {
			log.Fatalf("invalid IP address: %s", raw)
		}
		template.IPAddresses = append(template.IPAddresses, ip)
	}

	if len(template.DNSNames) == 0 && len(template.IPAddresses) == 0 {
		if ip := net.ParseIP(*commonName); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, *commonName)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		log.Fatalf("create certificate: %v", err)
	}

	if err := writePEM(*certPath, "CERTIFICATE", derBytes, 0o644); err != nil {
		log.Fatalf("write certificate: %v", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		log.Fatalf("marshal private key: %v", err)
	}
	if err := writePEM(*keyPath, "EC PRIVATE KEY", keyBytes, 0o600); err != nil {
		log.Fatalf("write private key: %v", err)
	}

	fmt.Printf("certificate: %s\nkey: %s\n", *certPath, *keyPath)
}

func splitList(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func writePEM(path, blockType string, derBytes []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer file.Close()

	return pem.Encode(file, &pem.Block{
		Type:  blockType,
		Bytes: derBytes,
	})
}
