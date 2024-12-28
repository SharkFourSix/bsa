package bsa_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/SharkFourSix/bsa"
	_ "github.com/mattn/go-sqlite3"
)

func createInMemoryDb() (*sql.DB, error) {
	dba, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	initializationStrings := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA ignore_check_constraints = OFF",
		"PRAGMA strict = ON",
		"CREATE TABLE IF NOT EXISTS users(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, age INTEGER NOT NULL CHECK (age IS NOT NULL))",
	}

	for _, q := range initializationStrings {
		if _, err = dba.Exec(q); err != nil {
			dba.Close()
			return nil, err
		}
	}

	return dba, nil
}

func diskFileQueryLoader() bsa.QueryLoader {
	return bsa.NewFSQueryLoader(os.DirFS("."), "./test/")
}

type User struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
	Age  int    `db:"age"`
}

type UserRepository struct {
	AddUser               func(name string, age int) (lastInsertID int64, rowsAffected int64, err error)                     `@exec:"insert into users(name, age) values ($1, $2)"`
	SelectUser            func(id int64) (*User, error)                                                                      `@queryOne:"select * from users where id = $1"`
	SelectAllUsers        func() []*User                                                                                     `@query:"select * from users"`
	InsertUserQueryLoader func() (int64, int64)                                                                              `@exec:"file:insert_user_steve"`
	AddUserTxWithError    func(bsa.DBResource, string, int) error                                                            `@exec:"insert into users(name, age) values (?, NULL)"`
	AddUserTx             func(res bsa.DBResource, name string, age int) (lastInsertID int64, rowsAffected int64, err error) `@exec:"insert into users(name, age) values (?, ?)"`
}

func TestQueryOne(t *testing.T) {
	var (
		userRepo     = UserRepository{}
		lastInsertID = int64(0)
		user         *User
	)

	db, err := createInMemoryDb()
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	if err = bsa.Bind(context.Background(), db, &userRepo, diskFileQueryLoader(), true); err != nil {
		t.Fatal(err)
	}

	if lastInsertID, _, err = userRepo.AddUser("john", 65); err != nil {
		t.Fatal(err)
	}

	if user, err = userRepo.SelectUser(lastInsertID); err != nil {
		t.Fatal(err)
	}

	if user.Name != "john" {
		t.Fatalf("Names don't match")
	}
}

func TestQuery(t *testing.T) {
	var (
		userRepo = UserRepository{}
		users    []*User
	)

	db, err := createInMemoryDb()
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	if err = bsa.Bind(context.Background(), db, &userRepo, diskFileQueryLoader(), true); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		if _, _, err = userRepo.AddUser(fmt.Sprintf("user%d", i), 25+i); err != nil {
			t.Fatal(err)
		}
	}

	users = userRepo.SelectAllUsers()

	if len(users) != 5 {
		t.Fatalf("Users empty")
	}
}

func TestTransaction(t *testing.T) {
	var (
		userRepo = UserRepository{}
		tx       *sql.Tx
		users    []*User
	)

	db, err := createInMemoryDb()
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	if err = bsa.Bind(context.Background(), db, &userRepo, diskFileQueryLoader(), true); err != nil {
		t.Fatal(err)
	}

	if tx, err = db.Begin(); err != nil {
		t.Fatal(err)
	}

	defer tx.Rollback()

	_, _, _ = userRepo.AddUserTx(tx, "john", 45)
	err = userRepo.AddUserTxWithError(tx, "jane", 90)
	if err != nil {
		tx.Rollback()

		// select users
		users = userRepo.SelectAllUsers()
		if len(users) == 0 {
			return
		}
	}
	t.Fatal("transaction statements slipped through")
}

func TestQueryLoader(t *testing.T) {
	var (
		userRepo = UserRepository{}
		user     *User
		id       int64
	)

	db, err := createInMemoryDb()
	if err != nil {
		t.Fatal(err)
	}

	defer db.Close()

	if err = bsa.Bind(context.Background(), db, &userRepo, diskFileQueryLoader(), true); err != nil {
		t.Fatal(err)
	}

	id, _ = userRepo.InsertUserQueryLoader()
	user, err = userRepo.SelectUser(id)
	if err != nil {
		t.Fatal(err)
	}

	if user.Name != "steve" {
		t.Fatalf("expected 'steve', got :%s", user.Name)
	}
}
