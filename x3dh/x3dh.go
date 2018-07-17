package x3dh

import (
  "crypto/ecdsa"
  "fmt"

  "github.com/ethereum/go-ethereum/crypto"
  "github.com/ethereum/go-ethereum/crypto/ecies"
)

const (
  sskLen = 16
)


func (bundle Bundle) Serialize() string {
  return fmt.Sprintf(
    "{\"identity\": \"%x\", \"preKey\": \"%x\", \"sig\": \"%x\"}",
    bundle.identityKey,
    bundle.signedPreKey,
    bundle.signature,
  )
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
    identityKey: compressedIdentityKey,
    signedPreKey: compressedPreKey,
    signature: signature,
  }

  return &bundle, preKey, nil
}

func ProcessBundle(bundle *Bundle, prv *ecdsa.PrivateKey) ([]byte, error) {
  identityKey, err := crypto.DecompressPubkey(bundle.identityKey)

  if err != nil {
    return nil, err
  }

  prv0 := ecies.ImportECDSA(prv)
  identityKey0 := ecies.ImportECDSAPublic(identityKey)

  return prv0.GenerateShared(identityKey0, sskLen, sskLen)
}

type Bundle struct {
  identityKey     []byte
  signedPreKey    []byte
  signature       []byte
}


