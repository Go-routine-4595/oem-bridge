package cert

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// Map of ExtKeyUsage to strings
var extKeyUsageMap = map[x509.ExtKeyUsage]string{
	x509.ExtKeyUsageAny:                            "Any",
	x509.ExtKeyUsageServerAuth:                     "ServerAuth",
	x509.ExtKeyUsageClientAuth:                     "ClientAuth",
	x509.ExtKeyUsageCodeSigning:                    "CodeSigning",
	x509.ExtKeyUsageEmailProtection:                "EmailProtection",
	x509.ExtKeyUsageIPSECEndSystem:                 "IPSECEndSystem",
	x509.ExtKeyUsageIPSECTunnel:                    "IPSECTunnel",
	x509.ExtKeyUsageIPSECUser:                      "IPSECUser",
	x509.ExtKeyUsageTimeStamping:                   "TimeStamping",
	x509.ExtKeyUsageOCSPSigning:                    "OCSPSigning",
	x509.ExtKeyUsageMicrosoftServerGatedCrypto:     "MicrosoftServerGatedCrypto",
	x509.ExtKeyUsageNetscapeServerGatedCrypto:      "NetscapeServerGatedCrypto",
	x509.ExtKeyUsageMicrosoftCommercialCodeSigning: "MicrosoftCommercialCodeSigning",
	x509.ExtKeyUsageMicrosoftKernelCodeSigning:     "MicrosoftKernelCodeSigning",
}

// Map of KeyUsage constants to their string representations
var keyUsageMap = map[x509.KeyUsage]string{
	x509.KeyUsageDigitalSignature:  "DigitalSignature",
	x509.KeyUsageContentCommitment: "ContentCommitment",
	x509.KeyUsageKeyEncipherment:   "KeyEncipherment",
	x509.KeyUsageDataEncipherment:  "DataEncipherment",
	x509.KeyUsageKeyAgreement:      "KeyAgreement",
	x509.KeyUsageCertSign:          "CertSign",
	x509.KeyUsageCRLSign:           "CRLSign",
	x509.KeyUsageEncipherOnly:      "EncipherOnly",
	x509.KeyUsageDecipherOnly:      "DecipherOnly",
}

func ShowCertificatePool(certPool *x509.CertPool) string {
	var res []string
	for _, certificate := range certPool.Subjects() {
		res = append(res, string(certificate))
	}
	return strings.Join(res, "\n")
}

func ShowCertificatePoolFromFile(certPoolFile string) (string, error) {
	certPool := NewCertPoolWithAccess()

	certPoolPEM, err := os.ReadFile(certPoolFile)
	if err != nil {
		return "", errors.Join(err, fmt.Errorf("Failed to read certificate pool: %s \n", certPoolFile))
	}

	err = certPool.AddCert(certPoolPEM)
	if err != nil {
		return "", errors.Join(err, fmt.Errorf("Failed to add certificate to pool: %s \n", certPoolFile))
	}

	return certPool.ListCerts(), nil
}

func ShowCertificate(certFile string) (string, error) {
	var res []string

	f, err := os.Open(certFile)
	if err != nil {
		return "", errors.Join(err, fmt.Errorf("Failed to open certificate: %s \n", certFile))
	}

	defer f.Close()

	b, err := os.ReadFile(certFile)
	if err != nil {
		return "", errors.Join(err, fmt.Errorf("Failed to read certificate: %s \n", certFile))
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
			return "", errors.New("failed to decode PEM block containing the certificate")
		}

		certs, err := x509.ParseCertificates(block.Bytes)
		if err != nil {
			return "", errors.Join(err, fmt.Errorf("Failed to parse certificate: %s \n", certFile))
		}

		for _, cert := range certs {
			res = append(res, ShowCertificateDetail(cert))
		}
	}
	return strings.Join(res, "\n"), nil
}

func ShowCertificateDetail(cert *x509.Certificate) string {
	var res []string

	res = append(res, fmt.Sprintf("Subject: %s", cert.Subject.String()))
	res = append(res, fmt.Sprintf("Issuer: %s", cert.Issuer.String()))
	res = append(res, fmt.Sprintf("Not Before: %s", cert.NotBefore.String()))
	res = append(res, fmt.Sprintf("Not After: %s", cert.NotAfter.String()))
	res = append(res, fmt.Sprintf("Key Usage: %s", keyUsageToStrings(cert.KeyUsage)))
	res = append(res, fmt.Sprintf("Ext Key Usage: %s", extKeyUsageToString(cert.ExtKeyUsage)))
	res = append(res, fmt.Sprintf("Basic Constraints Valid: %t", cert.IsCA))
	res = append(res, fmt.Sprintf("DNS Names: %s", cert.DNSNames))
	res = append(res, fmt.Sprintf("IP Addresses: %s", cert.IPAddresses))
	res = append(res, fmt.Sprintf("Email Addresses: %s", cert.EmailAddresses))
	res = append(res, fmt.Sprintf("URIs: %s", cert.URIs))

	return strings.Join(res, "\n")
}

func listCertificates(certPool *x509.CertPool) (string, error) {
	var list []string

	poolValue := reflect.ValueOf(certPool).Elem()
	if poolValue.Kind() != reflect.Struct {
		return "", errors.New("unexpected type: expected a struct")
	}

	// Retrieve the 'byName' field which is a map of name to certificates
	certsField := poolValue.FieldByName("byName")
	if !certsField.IsValid() {
		return "", errors.New("unexpected internal structure: no 'byName' field")
	}

	certsMap := certsField.Interface().(map[string][]*x509.Certificate)
	for name, certs := range certsMap {
		list = append(list, fmt.Sprintf("Subject: %s", name))
		for _, cert := range certs {
			list = append(list, fmt.Sprintf("  Subject: %s \n  Issuer: %s \n", cert.Subject.String(), cert.Issuer.String()))
		}
	}
	return strings.Join(list, "\n"), nil
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

	test := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: true,
	}
	return test, nil
}

// extKeyUsageToString converts an ExtKeyUsage to a string using the map.
func extKeyUsageToString(eku []x509.ExtKeyUsage) string {
	var usages []string
	for _, e := range eku {
		if usageStr, exists := extKeyUsageMap[e]; exists {
			usages = append(usages, usageStr)
		}
		usages = append(usages, "Unknown")
	}
	return strings.Join(usages, ", ")
}

// keyUsageToStrings converts a KeyUsage bitmask to a slice of strings
func keyUsageToStrings(ku x509.KeyUsage) string {
	var usages []string
	for bit, desc := range keyUsageMap {
		if ku&bit != 0 {
			usages = append(usages, desc)
		}
	}

	return strings.Join(usages, ", ")
}
