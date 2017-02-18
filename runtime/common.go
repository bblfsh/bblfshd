package runtime 

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type Digest []byte

func NewDigest(s string) Digest {
	b, _ := hex.DecodeString(s)

	return Digest(b)
}

func ComputeDigest(input ...string) Digest {
	h := sha256.New()
	for _, i := range input {
		h.Write([]byte(i))
	}

	return Digest(h.Sum(nil))
}

func (d Digest) IsZero() bool {
	return len(d) == 0
}

func (d Digest) String() string {
	return fmt.Sprintf("%x", []byte(d))
}
