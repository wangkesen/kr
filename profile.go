package kr

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"golang.org/x/crypto/ssh"
)

type Profile struct {
	SSHWirePublicKey []byte  `json:"public_key_wire"`
	Email            string  `json:"email"`
	PGPPublicKey     *string `json:"pgp_pk"`
}

func (p Profile) AuthorizedKeyString() (authString string, err error) {
	pk, err := p.SSHPublicKey()
	if err != nil {
		return
	}
	authString = pk.Type() + " " + base64.StdEncoding.EncodeToString(p.SSHWirePublicKey) + " " + strings.Replace(p.Email, " ", "", -1)
	return
}

func (p Profile) SSHPublicKey() (pk ssh.PublicKey, err error) {
	return ssh.ParsePublicKey(p.SSHWirePublicKey)
}

func (p Profile) RSAPublicKey() (pk *rsa.PublicKey, err error) {
	return SSHWireRSAPublicKeyToRSAPublicKey(p.SSHWirePublicKey)
}

func (p Profile) PublicKeyFingerprint() []byte {
	digest := sha256.Sum256(p.SSHWirePublicKey)
	return digest[:]
}

func (p Profile) Equal(other Profile) bool {
	return bytes.Equal(p.SSHWirePublicKey, other.SSHWirePublicKey) && p.Email == other.Email
}
