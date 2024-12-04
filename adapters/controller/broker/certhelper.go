package broker

import (
	"crypto/x509"
	"github.com/rs/zerolog"
	"reflect"
)

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
