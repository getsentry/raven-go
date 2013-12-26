package raven

import (
	"crypto/rand"
	"fmt"
	"io"
)

type uuid [16]byte

func (u uuid) String() string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

func newUUID() uuid {
	var u uuid
	io.ReadFull(rand.Reader, u[:])
	u[6] &= 0x0F // clear version
	u[6] |= 0x40 // set version to 4 (random uuid)
	u[8] &= 0x3F // clear variant
	u[8] |= 0x80 // set to IETF variant
	return u
}
