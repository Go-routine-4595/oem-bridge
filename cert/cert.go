package cert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/rs/zerolog"
	"os"
	"reflect"
)

func ShowCertificatePool(certPool *x509.CertPool, logger zerolog.Logger) {
	for _, certificate := range certPool.Subjects() {
		logger.Info().Int("size", len(certificate)).Msgf("Certificates in pool: %s\n", string(certificate))
	}
}

func ShowCertificatePoolFromFile(certPoolFile string, logger zerolog.Logger) {
	certPool := NewCertPoolWithAccess()

	certPoolPEM, err := os.ReadFile(certPoolFile)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read certificate pool")
		return
	}

	err = certPool.AddCert(certPoolPEM)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to add certificate to pool")
	}
	certPool.ListCerts()
}

func ShowCertificate(certFile string, logger zerolog.Logger) {
	f, err := os.Open(certFile)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to open certificate")
		return
	}

	defer f.Close()

	b, err := os.ReadFile(certFile)
	if err != nil {
		logger.Error().Err(err).Msgf("Failed to read certificate: %s \n", certFile)
		return
	}

	// Decode the PEMs block
	var (
		rest  []byte
		block *pem.Block
	)
	rest = b
	for len(rest) > 0 {
		block, rest = pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" {
			logger.Error().Msgf("Failed to decode PEM block containing the certificate: %s \n", certFile)
			return
		}

		certs, err := x509.ParseCertificates(block.Bytes)
		if err != nil {
			logger.Error().Err(err).Msgf("Failed to parse certificate: %s \n", certFile)
			return
		}

		for _, cert := range certs {
			ShowCertificateDetail(cert, logger)
		}
	}
}

func ShowCertificateDetail(cert *x509.Certificate, logger zerolog.Logger) {
	logger.Info().Msgf("Subject: %s\n", cert.Subject.String())
	logger.Info().Msgf("Issuer: %s\n", cert.Issuer.String())
	logger.Info().Msgf("Not Before: %s\n", cert.NotBefore.String())
	logger.Info().Msgf("Not After: %s\n", cert.NotAfter.String())
	logger.Info().Msgf("Key Usage: %s\n", cert.KeyUsage)
	logger.Info().Msgf("Ext Key Usage: %s\n", cert.ExtKeyUsage)
	logger.Info().Msgf("Basic Constraints Valid: %t\n", cert.IsCA)
	logger.Info().Msgf("DNS Names: %s\n", cert.DNSNames)
	logger.Info().Msgf("IP Addresses: %s\n", cert.IPAddresses)
	logger.Info().Msgf("Email Addresses: %s\n", cert.EmailAddresses)
	logger.Info().Msgf("URIs: %s\n", cert.URIs)
}

func listCertificates(certPool *x509.CertPool, logger zerolog.Logger) {
	poolValue := reflect.ValueOf(certPool).Elem()
	if poolValue.Kind() != reflect.Struct {
		logger.Error().Msg("unexpected type: expected a struct")
		return
	}

	// Retrieve the 'byName' field which is a map of name to certificates
	certsField := poolValue.FieldByName("byName")
	if !certsField.IsValid() {
		logger.Error().Msg("unexpected internal structure: no 'byName' field")
		return
	}

	certsMap := certsField.Interface().(map[string][]*x509.Certificate)
	for name, certs := range certsMap {
		logger.Debug().Msgf("Certificates for common name: %s\n", name)
		for _, cert := range certs {
			logger.Info().Msgf("  Subject: %s\n", cert.Subject)
			logger.Info().Msgf("  Issuer: %s\n", cert.Issuer)
		}
	}
}

// LoadCert loads and returns a configured tls.Config using the provided ControllerConfig for TLS settings.
// It reads the CA bundle, certificate, and key files specified in the config. If any file is missing, it returns an error.
// The function also handles loading X.509 key pairs and appending CA certificates to a new certificate pool.
// Note: InsecureSkipVerify is set to true regardless of the config setting.
func LoadCert(keyFile string, certFile string, bundleFile string) (*tls.Config, error) {
	if (keyFile == "" && certFile == "") || bundleFile == "" {
		return nil, fmt.Errorf("missing key, cert or ca bundle")
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load key pair: %v", err)
	}

	caCert, err := os.ReadFile(bundleFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA bundle: %v", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificates")
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}, nil
}
