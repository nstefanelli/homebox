package hasher

import (
	"context"
	"testing"
)

func TestTimingEqualizationHashIsValidArgon2(t *testing.T) {
	h := timingEqualizationHash()
	if h == "" {
		t.Fatal("timing equalization hash is empty")
	}
	if _, _, _, err := decodeHash(h); err != nil {
		t.Fatalf("timing equalization hash must be a decodable argon2id hash: %v", err)
	}
}

func TestStaticDummyHashIsValid(t *testing.T) {
	if _, _, _, err := decodeHash(staticDummyHash); err != nil {
		t.Fatalf("staticDummyHash must decode as argon2id: %v", err)
	}
}

func TestCheckDummyPasswordHashDoesRealWork(t *testing.T) {
	CheckDummyPasswordHashCtx(context.Background())
}
