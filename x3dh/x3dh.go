package x3dh

import (
  "fmt"
  "errors"
  "crypto/ecdsa"

  "github.com/golang/protobuf/jsonpb"
  "github.com/ethereum/go-ethereum/crypto"
  "github.com/ethereum/go-ethereum/crypto/ecies"
)

const (
  sskLen = 16
)

func (bundle Bundle) ToJSON() (string, error) {
  ma := jsonpb.Marshaler{};
  return ma.MarshalToString(&bundle);
}

func FromJSON(str string) (*Bundle, error) {
  var bundle Bundle;
  err := jsonpb.UnmarshalString(str, &bundle);
  return &bundle, err
}

func NewBundle(identity *ecdsa.PrivateKey) (* Bundle, * ecdsa.PrivateKey, error) {
  preKey, err := crypto.GenerateKey()

  if err != nil {
    return nil, nil, err
  }

  compressedPreKey  := crypto.CompressPubkey(&preKey.PublicKey)
  compressedIdentityKey  := crypto.CompressPubkey(&identity.PublicKey)

  signature, err := crypto.Sign(crypto.Keccak256(compressedPreKey), identity)

  if err != nil {
    return nil, nil, err
  }

  bundle := Bundle {
    Identity: compressedIdentityKey,
    SignedPreKey: compressedPreKey,
    Signature: signature,
  }

  return &bundle, preKey, nil
}

func verifyBundle(bundle *Bundle, bundleIdentityKey *ecdsa.PublicKey) error {

  recoveredKey, err := crypto.SigToPub(
    crypto.Keccak256(bundle.GetSignedPreKey()),
    bundle.GetSignature(),
  )

  if err != nil {
    return err
  }

  fmt.Printf("%x %x", recoveredKey.Y, bundleIdentityKey.Y);

  if crypto.PubkeyToAddress(*recoveredKey) != crypto.PubkeyToAddress(*bundleIdentityKey) {
    return errors.New("Identity key and signature mismatch")
  }

  return nil
}

func x3dh(
  bundleIdentityKey *ecies.PublicKey,
  bundleSignedPreKey *ecies.PublicKey,
  myIdentityKey *ecies.PrivateKey,
  myEphemeralKey *ecies.PrivateKey,
) ([]byte, error) {
  dh1, err := myIdentityKey.GenerateShared(
    bundleSignedPreKey,
    sskLen,
    sskLen,
  )
  if err != nil {
    return nil, err
  }

  dh2, err := myEphemeralKey.GenerateShared(
    bundleIdentityKey,
    sskLen,
    sskLen,
  )
  if err != nil {
    return nil, err
  }

  dh3, err := myEphemeralKey.GenerateShared(
    bundleSignedPreKey,
    sskLen,
    sskLen,
  )
  if err != nil {
    return nil, err
  }

  secretInput := append(append(dh1, dh2...), dh3...)

  sharedSecret := crypto.Keccak256(secretInput)

  return sharedSecret, nil
}

func ProcessBundle(bundle *Bundle, prv *ecdsa.PrivateKey) ([]byte, error) {

  bundleIdentityKey, err := crypto.DecompressPubkey(bundle.GetIdentity())
  if err != nil {
    return nil, err
  }

  bundleSignedPreKey, err := crypto.DecompressPubkey(bundle.GetIdentity())
  if err != nil {
    return nil, err
  }

  err = verifyBundle(bundle, bundleIdentityKey);

  if err != nil {
    return nil, err
  }

  ephemeralKey1, err := crypto.GenerateKey()

  return x3dh(
    ecies.ImportECDSAPublic(bundleIdentityKey),
    ecies.ImportECDSAPublic(bundleSignedPreKey),
    ecies.ImportECDSA(prv),
    ecies.ImportECDSA(ephemeralKey1),
  )
}
