// Copyright 2018 The dexon-consensus-core Authors
// This file is part of the dexon-consensus-core library.
//
// The dexon-consensus-core library is free software: you can redistribute it
// and/or modify it under the terms of the GNU Lesser General Public License as
// published by the Free Software Foundation, either version 3 of the License,
// or (at your option) any later version.
//
// The dexon-consensus-core library is distributed in the hope that it will be
// useful, but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Lesser
// General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the dexon-consensus-core library. If not, see
// <http://www.gnu.org/licenses/>.

package ecdsa

import (
	"crypto/ecdsa"

	ethcrypto "github.com/dexon-foundation/dexon/crypto"

	"github.com/dexon-foundation/dexon-consensus-core/common"
	"github.com/dexon-foundation/dexon-consensus-core/core/crypto"
)

const cryptoType = "ecdsa"

func init() {
	crypto.RegisterSigToPub(cryptoType, SigToPub)
}

// PrivateKey represents a private key structure used in geth and implments
// Crypto.PrivateKey interface.
type PrivateKey struct {
	privateKey ecdsa.PrivateKey
	publicKey  publicKey
}

// publicKey represents a public key structure used in geth and implements
// Crypto.PublicKey interface.
type publicKey struct {
	publicKey []byte
}

// NewPrivateKey creates a new PrivateKey structure.
func NewPrivateKey() (*PrivateKey, error) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	return &PrivateKey{
		privateKey: *key,
		publicKey:  *newPublicKey(key),
	}, nil
}

// newPublicKey creates a new PublicKey structure.
func newPublicKey(prvKey *ecdsa.PrivateKey) *publicKey {
	return &publicKey{
		publicKey: ethcrypto.CompressPubkey(&prvKey.PublicKey),
	}
}

// NewPublicKeyFromByteSlice constructs an eth.publicKey instance from
// a byte slice.
func NewPublicKeyFromByteSlice(b []byte) crypto.PublicKey {
	return publicKey{publicKey: b}
}

// decompressPubkey parses a public key in the 33-byte compressed format.
func decompressPubkey(pubkey []byte) (publicKey, error) {
	_, err := ethcrypto.DecompressPubkey(pubkey)
	return publicKey{
		publicKey: pubkey,
	}, err
}

// PublicKey returns the public key associate this private key.
func (prv *PrivateKey) PublicKey() crypto.PublicKey {
	return prv.publicKey
}

// Sign calculates an ECDSA signature.
//
// This function is susceptible to chosen plaintext attacks that can leak
// information about the private key that is used for signing. Callers must
// be aware that the given hash cannot be chosen by an adversery. Common
// solution is to hash any input before calculating the signature.
//
// The produced signature is in the [R || S || V] format where V is 0 or 1.
func (prv *PrivateKey) Sign(hash common.Hash) (
	sig crypto.Signature, err error) {
	s, err := ethcrypto.Sign(hash[:], &prv.privateKey)
	sig = crypto.Signature{
		Type:      cryptoType,
		Signature: s,
	}
	return
}

// VerifySignature checks that the given public key created signature over hash.
// The public key should be in compressed (33 bytes) or uncompressed (65 bytes)
// format.
// The signature should have the 64 byte [R || S] format.
func (pub publicKey) VerifySignature(
	hash common.Hash, signature crypto.Signature) bool {
	sig := signature.Signature
	if len(sig) == 65 {
		// The last byte is for ecrecover.
		sig = sig[:64]
	}
	return ethcrypto.VerifySignature(pub.publicKey, hash[:], sig)
}

// Compress encodes a public key to the 33-byte compressed format.
func (pub publicKey) Compress() []byte {
	return pub.publicKey
}

// Bytes returns the []byte representation of public key.
func (pub publicKey) Bytes() []byte {
	return pub.Compress()
}

// SigToPub returns the PublicKey that created the given signature.
func SigToPub(
	hash common.Hash, signature crypto.Signature) (crypto.PublicKey, error) {
	key, err := ethcrypto.SigToPub(hash[:], signature.Signature[:])
	if err != nil {
		return publicKey{}, err
	}
	return publicKey{publicKey: ethcrypto.CompressPubkey(key)}, nil
}