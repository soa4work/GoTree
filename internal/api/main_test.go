package api

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/praetorianer777/gotree/internal/store"
)

func TestMain(m *testing.M) {
	// Real costs make every login and account test slow, above all under -race.
	store.BcryptCost = bcrypt.MinCost
	os.Exit(m.Run())
}
