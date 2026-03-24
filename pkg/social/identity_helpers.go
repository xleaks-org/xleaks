package social

import (
	"fmt"

	"github.com/xleaks-org/xleaks/pkg/identity"
)

func activeIdentity(kp *identity.KeyPair) (*identity.KeyPair, error) {
	if kp == nil {
		return nil, fmt.Errorf("identity not unlocked")
	}

	pubkey := kp.PublicKeyBytes()
	if len(pubkey) == 0 || isAllZero(pubkey) {
		return nil, fmt.Errorf("identity not unlocked")
	}

	return kp, nil
}

func hasIdentity(kp *identity.KeyPair) bool {
	if kp == nil {
		return false
	}
	return len(kp.PublicKeyBytes()) > 0 && !isAllZero(kp.PublicKeyBytes())
}

func isAllZero(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}
