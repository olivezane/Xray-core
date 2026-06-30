package aead

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

func TestOpenVMessAEADHeader(t *testing.T) {
	TestHeader := []byte("Test Header")
	key := KDF16([]byte("Demo Key for Auth ID Test"), "Demo Path for Auth ID Test")
	var keyw [16]byte
	copy(keyw[:], key)
	sealed := SealVMessAEADHeader(keyw, TestHeader)

	AEADR := bytes.NewReader(sealed)

	var authid [16]byte

	io.ReadFull(AEADR, authid[:])

	out, _, _, err := OpenVMessAEADHeader(keyw, authid, AEADR)

	fmt.Println(string(out))
	fmt.Println(err)
}

func TestOpenVMessAEADHeader2(t *testing.T) {
	TestHeader := []byte("Test Header")
	key := KDF16([]byte("Demo Key for Auth ID Test"), "Demo Path for Auth ID Test")
	var keyw [16]byte
	copy(keyw[:], key)
	sealed := SealVMessAEADHeader(keyw, TestHeader)

	AEADR := bytes.NewReader(sealed)

	var authid [16]byte

	io.ReadFull(AEADR, authid[:])

	out, _, readen, err := OpenVMessAEADHeader(keyw, authid, AEADR)
	requireEqual(t, len(sealed)-16-AEADR.Len(), readen)
	requireEqual(t, string(TestHeader), string(out))
	requireNil(t, err)
}

func TestOpenVMessAEADHeader4(t *testing.T) {
	for i := 0; i <= 60; i++ {
		TestHeader := []byte("Test Header")
		key := KDF16([]byte("Demo Key for Auth ID Test"), "Demo Path for Auth ID Test")
		var keyw [16]byte
		copy(keyw[:], key)
		sealed := SealVMessAEADHeader(keyw, TestHeader)
		var sealedm [16]byte
		copy(sealedm[:], sealed)
		sealed[i] ^= 0xff
		AEADR := bytes.NewReader(sealed)

		var authid [16]byte

		io.ReadFull(AEADR, authid[:])

		out, drain, readen, err := OpenVMessAEADHeader(keyw, authid, AEADR)
		requireEqual(t, len(sealed)-16-AEADR.Len(), readen)
		requireEqual(t, true, drain)
		requireNotNil(t, err)
		if err == nil {
			fmt.Println(">")
		}
		requireNil(t, out)
	}
}

func TestOpenVMessAEADHeader4Massive(t *testing.T) {
	for j := 0; j < 1000; j++ {
		for i := 0; i <= 60; i++ {
			TestHeader := []byte("Test Header")
			key := KDF16([]byte("Demo Key for Auth ID Test"), "Demo Path for Auth ID Test")
			var keyw [16]byte
			copy(keyw[:], key)
			sealed := SealVMessAEADHeader(keyw, TestHeader)
			var sealedm [16]byte
			copy(sealedm[:], sealed)
			sealed[i] ^= 0xff
			AEADR := bytes.NewReader(sealed)

			var authid [16]byte

			io.ReadFull(AEADR, authid[:])

			out, drain, readen, err := OpenVMessAEADHeader(keyw, authid, AEADR)
			requireEqual(t, len(sealed)-16-AEADR.Len(), readen)
			requireEqual(t, true, drain)
			requireNotNil(t, err)
			if err == nil {
				fmt.Println(">")
			}
			requireNil(t, out)
		}
	}
}
