package usecase

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	"example.com/pisag_go/ports"
)

// WithOracleSelfSigned: integration/dev only
// ORACLE_CA_PATH の PEM(oracle.crt) を読み込み、"oracle CAのみ" を信頼する CertPool を作って policy.TLSRootCAs に設定する。
// - InsecureSkipVerify は使わない
// - SystemCertPool へ append もしない（= 明示CAのみ信頼 / システムCAをバイパス）
func WithOracleSelfSigned(policy ports.Policy) (ports.Policy, error) {
	caPath := strings.TrimSpace(os.Getenv("ORACLE_CA_PATH"))
	if caPath == "" {
		return policy, fmt.Errorf("ORACLE_CA_PATH is not set")
	}

	pem, err := os.ReadFile(caPath)
	if err != nil {
		return policy, fmt.Errorf("read ORACLE_CA_PATH: %w", err)
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(pem); !ok {
		return policy, errors.New("failed to append certs from PEM")
	}

	policy.TLSRootCAs = pool
	return policy, nil
}
