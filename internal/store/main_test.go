package store

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// Real costs make every login and account test slow, above all under -race.
	BcryptCost = bcrypt.MinCost
	os.Exit(m.Run())
}
