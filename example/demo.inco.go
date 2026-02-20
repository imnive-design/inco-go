package example

import "fmt"

type User struct {
	Name string
	Age  int
}

type DB struct {
	connected bool
}

func (db *DB) Query(q string) (*User, error) {
	return &User{Name: "test", Age: 25}, nil
}

// --- Case 1: Default action (panic) with expression ---

func CreateUser(name string, age int) {
	// @require len(name) > 0
	// @require age > 0

	fmt.Printf("Creating user: %s, age %d\n", name, age)
}

// --- Case 2: Panic with custom message ---

func GetUser(u *User) {
	// @require u != nil panic("user must not be nil")

	fmt.Println(u.Name)
}

// --- Case 3: @must — error must not occur ---

func FetchUser(db *DB, id string) (*User, error) {
	// @require db != nil
	// @require len(id) > 0 panic("empty id")

	user, _ := db.Query("SELECT * FROM users WHERE id = ?") // @must
	return user, nil
}
