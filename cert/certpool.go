package cert

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

const (
	certificate = "CERTIFICATE"
	key         = "KEY"
)

// CertPoolWithAccess is a wrapper over x509.CertPool
type CertPoolWithAccess struct {
	pool         *x509.CertPool
	certificates []*x509.Certificate
}

// NewCertPoolWithAccess creates a new CertPoolWithAccess
func NewCertPoolWithAccess() *CertPoolWithAccess {
	return &CertPoolWithAccess{
		pool:         x509.NewCertPool(),
		certificates: make([]*x509.Certificate, 0),
	}
}

// AddCert adds a certificate to the pool and stores it for later access
func (c *CertPoolWithAccess) AddCert(certPEM []byte) error {
	var (
		block *pem.Block
		rest  []byte
	)

	rest = certPEM
	for len(rest) > 0 {
		block, rest = pem.Decode(rest)
		if block == nil || block.Type != certificate {
			return fmt.Errorf("failed to decode PEM block containing the certificate")
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return err
		}

		c.pool.AddCert(cert)
		c.certificates = append(c.certificates, cert)
	}
	return nil
}

// ListCerts prints the subjects of all certificates in the pool
func (c *CertPoolWithAccess) ListCerts() string {
	var (
		res []string
	)
	for _, cert := range c.certificates {
		res = append(res, fmt.Sprintf("Subject: %s", cert.Subject.String()))
	}
	return strings.Join(res, "\n")
}
