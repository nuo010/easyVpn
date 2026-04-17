package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func LoadServerTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("data_tls requires both data_tls_cert_file and data_tls_key_file")
	}

	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load data TLS certificate: %w", err)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
	}, nil
}

func LoadClientTLSConfig(caFile, serverName string, insecureSkipVerify bool) (*tls.Config, error) {
	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if caFile != "" {
		data, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read data TLS CA file: %w", err)
		}
		if ok := rootCAs.AppendCertsFromPEM(data); !ok {
			return nil, fmt.Errorf("append data TLS CA certificates from %s: no certificates found", caFile)
		}
	}

	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		RootCAs:            rootCAs,
		ServerName:         serverName,
		InsecureSkipVerify: insecureSkipVerify,
	}, nil
}
