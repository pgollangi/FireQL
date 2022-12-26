# FireQL
Go library and interactive CLI tool to query Google Firestore resources using SQL syntax.

## Usage

`FireQL` available as Go library and interactive command-line tool.

### Go Library
An example of querying collections using SQL syntax:
```go
import (
    "github.com/pgollangi/fireql"
)

func main() {
    query, _ := fireql.NewFireQL("GCP_PROJECT_ID")
    result, err := query.Execute("Select Id, Email, FullName as Name, `Address.City` as City from users LIMIT 10")
    if err != nil {
        panic(err)
    }
    _ = result
}
```
Replace `GCP_PROJECT_ID` with your Google ProjectId.

### Command-Line

## Installation

### Go

### Homebrew

### Scoop (for windows)

### Manual
You can alternately download suitable binary for your OS at the [releases page](https://github.com/pgollangi/fireql/releases).

## Limitations
All of [firestore query limitations](https://firebase.google.com/docs/firestore/query-data/queries#query_limitations) are applicable when running queries using `FireQL`.

In additional to that:

- Only `SELECT` queries for now. Support for `INSERT`, `UPDATE`, and `DELETE` might come in the future.
- Only `AND` conditions supported in `WHERE` clause. 
- No support for `JOIN`s.
- `LIMIT` doesn't accept an `OFFSET`, only a single number.
- No support of `GROUP BY` and aggregate function `COUNT`.
- 

## Roadmap
- Expand support all logical conditions in `WHERE` clause by internally issuing multiple query requests to Firestore and merge results locally before returning.
- `GROUP BY` support
- Support other DML queries

## Contributing
Thanks for considering contributing to this project!

Please read the Contributions and Code of conduct.

Feel free to open an issue or submit a pull request!
## Licence

`FireQL` is open-sourced software licensed under the [MIT](LICENCE) license.
