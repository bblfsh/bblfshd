package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/oklog/ulid"
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

var randPool = &sync.Pool{
	New: func() interface{} {
		return rand.NewSource(time.Now().UnixNano())
	},
}

// NewULID returns a new ULID, which is a lexically sortable UUID.
func NewULID() ulid.ULID {
	entropy := randPool.Get().(rand.Source)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.New(entropy))
	randPool.Put(entropy)

	return id
}
