package db

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"golang.org/x/text/unicode/norm"
	"modernc.org/sqlite"
)

// SearchFoldFunction is the SQL function search_fold(text), SearchFold for
// substring filters: `search_fold(name) LIKE ?` with the pattern folded the
// same way.
const SearchFoldFunction = "search_fold"

// searchFoldLetters folds the letters the people search folds too, so
// «петр» finds «Пётр» everywhere (see store.searchFold).
var searchFoldLetters = strings.NewReplacer("ё", "е", "й", "и")

// SearchFold lower-cases s with Unicode rules (SQLite's LIKE, NOCASE and
// lower() fold ASCII only, so «москва» did not find «Москва»), composes it
// and folds ё/й to е/и. Latin accents are kept.
func SearchFold(s string) string {
	return searchFoldLetters.Replace(strings.ToLower(norm.NFC.String(s)))
}

func init() {
	sqlite.MustRegisterDeterministicScalarFunction(SearchFoldFunction, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			switch v := args[0].(type) {
			case nil:
				return "", nil
			case string:
				return SearchFold(v), nil
			case []byte:
				return SearchFold(string(v)), nil
			default:
				return SearchFold(fmt.Sprint(v)), nil
			}
		})
}
