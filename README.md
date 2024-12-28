# bsa
Simplified database access in go. All the __*BS* *A*side__

### Show and tell

This library allows you to do this,

```go
type Repo struct {
    GetLocale(id int64) string `@queryOne:"select locale FROM customers where id = $1"`
} 

var myRepo *Repo

userLocale := myRepo.GetLocale(452, "en_US")
```

instead of this:

```go

func GetLocale(db *sql.DB, customerID int) (string, error) {
	var locale string

	query := `SELECT locale FROM customers WHERE id = $1`
	err := db.QueryRow(query, customerID).Scan(&locale)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no customer found with id %d", customerID)
		}
		return "", err
	}

	return locale, nil
}

```

### Features

1. __Transactions__: Transactions are not necessarily implemented. However, passing a `DBResource` as the first argument will make the function use that connection resource instead of the initial database connection. 

    That connection could then be an sql.Tx beforehand, effectively providing transaction support

    ```go
    type Repo struct {
        ProcessPurchase(dbRes DBResource, foo int64, bar int64) `@exec:"file:process_purchase"`
        UpdateBalances(dbRes DBResource, foo int64, bar int64) `@exec:"file:update_balances"`
        UpdateStock(dbRes DBResource, foo int64, bar int64) `@exec:"file:update_stock"`
    }

    var (
        db *sql.DB
        repo *Repo
    )

    tx, _ := db.Begin()
    repo.ProcessPurchase(tx,...)
    repo.UpdateBalances(tx,...)
    repo.UpdateStock(tx,...)
    tx.Commit()

    ```

2. __Allowed return type signatures__:

    a. `InsertUser(...)` : Functions that don't return anything will panic if an error occurs.
    
    b. `InsertUser(...) error` : The return type of a function returning a single value must be `error`. Any panics are caught and returned.
    
    c. `InsertUser(...) (int64,int64)` : The first value contains the last insert id, if the underlying database system supports this feature. If not, the value will be 0. Errors will cause the function to panic.
    
    d. `InsertUser(...) (int64,int64,error)` : Same as *c*, but errors will be returned.

    e. Query functions: Can only return `(*type, error)` which wraps panics into the returned error, or `(*type)` which panics on error.

3. __Directives__:

    a. `@query`: Corresponds to a `.Query`.

    b. `@queryOne`: Corresponds to a `.QueryRow` but only a single row is returned.

    c. `@exec`: Correspnds to a `.Exec`.

4. __Query mapping__: Allows loading complex queries from a `fs.FS`:

    Prefix the query or statement part with `file:` and set the rest to the path of the SQL file, without
    a file extension. The file will be resolved using the configured `QueryLoader`.

    ```go
    type Repo struct {
        DoSomething(dbRes DBResource, foo int64, bar int64) `@exec:"file:process_purchase"`
    }
    
    ```

5. __Struct field mapping__: Struct fields are mapped using `'db'` struct tags. The scanning library used is [scanny/sqlscan](https://github.com/georgysavva/scany). The db struct tag is not required unless the name of the struct field is different from the database column. 

    Check https://github.com/georgysavva/scany/tree/master/sqlscan for more details.

### Usage examples

Refer to tests in [bsa_test](bsa_test.go)