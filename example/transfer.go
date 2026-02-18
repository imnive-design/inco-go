package example

import "fmt"

type Account struct {
	ID      string
	Balance int
}

type QueryResult struct {
	RowsAffected int
}

type dbConn struct{}

func (d *dbConn) Exec(query string) (*QueryResult, error) {
	return &QueryResult{RowsAffected: 1}, nil
}

var db = &dbConn{}

// Transfer demonstrates the full set of directives from the README.
func Transfer(from *Account, to *Account, amount int) {
	// @require -nd from, to
	// @require amount > 0, "amount must be positive"

	query := fmt.Sprintf("UPDATE accounts SET balance = balance - %d WHERE id = '%s'", amount, from.ID)
	res, _ := db.Exec(query) // @must

	fmt.Printf("Transfer %d from %s to %s, affected %d rows\n", amount, from.ID, to.ID, res.RowsAffected)
}
