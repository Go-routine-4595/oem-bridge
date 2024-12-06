package broker

import (
	"crypto/x509"
	"encoding/pem"
	"github.com/rs/zerolog"
	"os"
	"reflect"
)

func showCertificatePool(certPool *x509.CertPool, logger zerolog.Logger) {
	for _, certificate := range certPool.Subjects() {
		logger.Debug().Int("size", len(certificate)).Msgf("Certificates in pool: %s\n", string(certificate))
	}
}

func showCertificate(certFile string, logger zerolog.Logger) {
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

	// Decode the PEM block
	block, _ := pem.Decode(b)
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
		showCertificateDetail(cert, logger)
	}
}

func showCertificateDetail(cert *x509.Certificate, logger zerolog.Logger) {
	logger.Debug().Msgf("Subject: %s\n", cert.Subject)
	logger.Debug().Msgf("Issuer: %s\n", cert.Issuer)
	logger.Debug().Msgf("Not Before: %s\n", cert.NotBefore)
	logger.Debug().Msgf("Not After: %s\n", cert.NotAfter)
	logger.Debug().Msgf("Key Usage: %s\n", cert.KeyUsage)
	logger.Debug().Msgf("Ext Key Usage: %s\n", cert.ExtKeyUsage)
	logger.Debug().Msgf("Basic Constraints Valid: %t\n", cert.IsCA)
	logger.Debug().Msgf("DNS Names: %s\n", cert.DNSNames)
	logger.Debug().Msgf("IP Addresses: %s\n", cert.IPAddresses)
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
			logger.Debug().Msgf("  Subject: %s\n", cert.Subject)
			logger.Debug().Msgf("  Issuer: %s\n", cert.Issuer)
		}
	}
}
