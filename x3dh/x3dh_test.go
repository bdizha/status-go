package x3dh

import (
  "testing"
  "strings"

  "encoding/hex"

  "github.com/ethereum/go-ethereum/crypto"
  "github.com/stretchr/testify/assert"
)


func TestNewBundle(t *testing.T) {
  privateKey, err := crypto.GenerateKey()

  assert.Nil(t, err, "no error should be reported")

  bundle, _, err := NewBundle(privateKey)

  assert.Nil(t, err, "no error should be reported")

  recoveredSignature, err := crypto.SigToPub(
    crypto.Keccak256(bundle.signedPreKey),
    bundle.signature,
  )

  assert.Nil(t, err, "no error should be reported")

  assert.Equalf(
    t,
    &privateKey.PublicKey,
    recoveredSignature,
    "signature not valid",
  )

  assert.Equalf(
    t,
    "tahs",
    bundle.Serialize(),
    "should serialize",
  )
}


func TestProcessBundle(t *testing.T) {
  pubKey, err := hex.DecodeString("04760c4460e5336ac9bbd87952a3c7ec4363fc0a97bd31c86430806e287b437fd1b01abc6e1db640cf3106b520344af1d58b00b57823db3e1407cbc433e1b6d04d")

  assert.Nil(t, err, "no error should be reported")

  key0, err := crypto.ToECDSA([]byte(strings.Repeat("0", 32)))

  assert.Nil(t, err, "no error should be reported")

  key1, err := crypto.UnmarshalPubkey([]byte(pubKey))

  assert.Nil(t, err, "no error should be reported")

  compressedKey := crypto.CompressPubkey(key1)

  bundle := Bundle{identityKey: compressedKey}

  actual, err := ProcessBundle(&bundle, key0)

  assert.Nil(t, err, "no error should be reported")
  assert.Equalf(t, actual, "test", "unexpected")
}
