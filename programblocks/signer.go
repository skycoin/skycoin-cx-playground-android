package programblocks

import (
	"errors"
	"fmt"

	"github.com/skycoin/skycoin/src/cipher"
)

// ErrSignerNotConfirmed is the error of a program whose signature is valid
// but whose signer is not trusted and whose run the user did not confirm.
var ErrSignerNotConfirmed = errors.New("the signer is not trusted and the user did not confirm the run")

// KeyListed reports whether key is one of keys.
func KeyListed(keys []cipher.PubKey, key cipher.PubKey) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

// ConfirmSigner applies D27 to the signer of a verified manifest: a key in
// trusted runs without asking, and any other key runs only when confirm
// returns true. A nil confirm refuses every key that is not trusted.
func ConfirmSigner(signer cipher.PubKey, trusted []cipher.PubKey, confirm func(cipher.PubKey) bool) error {
	if KeyListed(trusted, signer) || (confirm != nil && confirm(signer)) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrSignerNotConfirmed, signer.Hex())
}
