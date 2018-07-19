package x3dh

import (
  "testing"
  "strings"

  "encoding/hex"

  "github.com/ethereum/go-ethereum/crypto/ecies"
  "github.com/ethereum/go-ethereum/crypto"
  "github.com/stretchr/testify/assert"
)

const (
  privateKey1 = "00000000000000000000000000000000"
  privateKey2 = "11111111111111111111111111111111"

  pubKey = "04760c4460e5336ac9bbd87952a3c7ec4363fc0a97bd31c86430806e287b437fd1b01abc6e1db640cf3106b520344af1d58b00b57823db3e1407cbc433e1b6d04d"
  jsonBundle = "{\"identity\":\"Ai7VV/WtM2sxpJhX5OlmSVSsMzhaogqT4tZL/n8I9RJ3\",\"signedPreKey\":\"A3YMRGDlM2rJu9h5UqPH7ENj/AqXvTHIZDCAbih7Q3/R\",\"signature\":\"V/WbDiANpbmAV8aB3KUKDMBP/htduOdcpGbA9rcJljNTDATz15iUTaD80Lge653ER2JTihIy01DjAo7fhjk/WwE=\"}"
)

var sharedKey = []byte{0x42, 0xe9, 0xa9, 0x42, 0xdb, 0xc9, 0x32, 0x9e, 0xaa, 0x3d, 0x3f, 0xa7, 0x75, 0x56, 0x7f, 0x4a, 0x3f, 0x8d, 0x2e, 0xb, 0xb0, 0x6b, 0x67, 0x3a, 0x97, 0x79, 0x76, 0xd6, 0xa5, 0x9b, 0x5f, 0xa9}

func testBundle() (*Bundle, error) {
  privateKey, err := crypto.ToECDSA([]byte(privateKey1))
  if err != nil {
    return nil, err
  }

  byteSignedPreKey, err := hex.DecodeString("04760c4460e5336ac9bbd87952a3c7ec4363fc0a97bd31c86430806e287b437fd1b01abc6e1db640cf3106b520344af1d58b00b57823db3e1407cbc433e1b6d04d")
  if err != nil {
    return nil, err
  }

  signedPreKey, err := crypto.UnmarshalPubkey([]byte(byteSignedPreKey))
  if err != nil {
    return nil, err
  }

  compressedPreKey := crypto.CompressPubkey(signedPreKey)

  signature, err := crypto.Sign(crypto.Keccak256(compressedPreKey), privateKey)

  bundle := Bundle {
    Identity: crypto.CompressPubkey(&privateKey.PublicKey),
    SignedPreKey: compressedPreKey,
    Signature: signature,
  }

  return &bundle, nil;
}


func TestNewBundle(t *testing.T) {
  privateKey, err := crypto.ToECDSA([]byte(strings.Repeat("0", 32)))

  assert.Nil(t, err, "Private key should be generated without errors")

  bundle, _, err := NewBundle(privateKey)

  assert.Nil(t, err, "Bundle should be generated without errors")

  recoveredPublicKey, err := crypto.SigToPub(
    crypto.Keccak256(bundle.GetSignedPreKey()),
    bundle.Signature,
  )

  assert.Nil(t, err, "Public key should be recovered from the bundle successfully")

  assert.Equalf(
    t,
    &privateKey.PublicKey,
    recoveredPublicKey,
    "The correct public key should be recovered",
  )
}

func TestToJSON(t *testing.T) {

  bundle, err := testBundle()

  assert.Nil(t, err, "Test bundle should be generated without errors")

  actualJsonBundle, err := bundle.ToJSON()

  assert.Nil(t, err, "no error should be reported")

  assert.Equalf(
    t,
    jsonBundle,
    actualJsonBundle,
    "The correct bundle should be generated",
  )
}

func TestFromJSON(t *testing.T) {

  expectedBundle, err := testBundle()

  assert.Nil(t, err, "Test bundle should be generated without errors")

  actualBundle, err := FromJSON(jsonBundle)

  assert.Nil(t, err, "Bundle should be unmarshaled without errors")

  assert.Equalf(
    t,
    expectedBundle,
    actualBundle,
    "The correct bundle should be generated",
  )
}

func TestX3dh(t *testing.T) {
  bundle, err := testBundle()

  bundleIdentityKey, err := crypto.DecompressPubkey(bundle.GetIdentity())
  assert.Nil(t, err, "Bundle identity key should be generated without errors")

  bundleSignedPreKey, err := crypto.DecompressPubkey(bundle.GetIdentity())
  assert.Nil(t, err, "Bundle signed pre key should be generated without errors")

  privateKey, err := crypto.ToECDSA([]byte(privateKey1))
  assert.Nil(t, err, "private key should be generated without errors")

  ephemeralKey1, err := crypto.ToECDSA([]byte(privateKey1))
  assert.Nil(t, err, "ephemeral key should be generated without errors")

  x3dh, err := x3dh(
    ecies.ImportECDSAPublic(bundleIdentityKey),
    ecies.ImportECDSAPublic(bundleSignedPreKey),
    ecies.ImportECDSA(privateKey),
    ecies.ImportECDSA(ephemeralKey1),
  )

  assert.Equalf(t, sharedKey, x3dh, "Should generate the correct key")
}

func TestProcessBundle(t *testing.T) {
  bundle, err := testBundle()

  assert.Nil(t, err, "Test bundle should be generated without errors")

  privateKey, err := crypto.ToECDSA([]byte(privateKey2))

  assert.Nil(t, err, "Private key should be imported without errors")

  actual, err := ProcessBundle(bundle, privateKey)

  assert.Nil(t, err, "no error should be reported")
  assert.NotNil(t, actual, "A key should be generated")
}
