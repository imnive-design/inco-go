package example

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

// --- Case 1: Anonymous function (closure) with @require ---

func ProcessWithCallback(db *DB) {
	handler := func(u *User) {
		// @require -nd u
		fmt.Println(u.Name)
	}

	u, _ := db.Query("SELECT 1")
	handler(u)
}

// --- Case 2: Multi-line @must (directive on its own line, statement spans multiple lines) ---

func FetchMultiLine(db *DB) *User {
	// @must
	res, _ := db.Query(
		"SELECT * FROM users WHERE id = ?",
	)

	fmt.Println("Fetched:", res.Name)
	return res
}

// --- Case 3: Nested closure with @require ---

func WithEnsure() (result *User) {
	// @require -nd result

	compute := func() *User {
		// @require -nd result
		return &User{Name: "inner"}
	}

	_ = compute
	return nil
}

// --- Case 4: @require -ret (return on violation, no panic) ---

func SafeGetUser(u *User) (result *User, ok bool) {
	// @require -ret -nd u
	return u, true
}

// --- Case 5: @require -ret with expression and unnamed returns ---

func SafeTransfer(amount int) (*QueryResult, error) {
	// @require -ret amount > 0
	return &QueryResult{RowsAffected: 1}, nil
}

// --- Case 6: @require -log (log + return on violation) ---

func LogAndReturn(u *User, name string) (result *User) {
	// @require -log -nd u
	// @require -log len(name) > 0, "name must not be empty"
	result = u
	result.Name = name
	return
}

// --- Case 7: @require -ret(expr, ...) custom return expressions ---

func FindUser(db *DB, id string) (*User, error) {
	// @require -ret(nil, ErrNotFound) -nd db
	// @require -ret(nil, fmt.Errorf("invalid id: %s", id)) len(id) > 0
	user, _ := db.Query("SELECT * FROM users WHERE id = ?")
	return user, nil
}

// --- Case 8: @require -ret(expr) single custom value ---

func GetDefault(x *int) int {
	// @require -ret(-1) -nd x
	return *x
}

// --- Case 9: @must -ret (return error instead of panic) ---

func SafeFetch(db *DB) (*User, error) {
	user, _ := db.Query("SELECT 1") // @must -ret
	return user, nil
}

// --- Case 10: @must -ret(expr, ...) custom return on error ---

func FetchOrDefault(db *DB) (*User, error) {
	user, _ := db.Query("SELECT 1") // @must -ret(&User{Name: "guest"}, nil)
	return user, nil
}
