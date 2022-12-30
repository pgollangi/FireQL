package fireql

import (
	"fmt"
	selectStmt "github.com/pgollangi/fireql/pkg/select"
	"github.com/pgollangi/fireql/pkg/util"
	"github.com/xwb1989/sqlparser"
)

// FireQL object is constructed to execute
// SQL queries on Firestore database.
// FireQL internally issue queries constructed from SQL
// on Firestore database using Google Firestore client library
type FireQL struct {
	context *util.Context
}

// New creates new FireQL instance using the "projectId"
// passed. Accepts list of Option to pass other configuration params.
// Google Application Default Credentials are used by
// Firestore client library if service account is not passed via Option.
// See https://cloud.google.com/docs/authentication/client-libraries
// for more information about how Application Default Credentials are
// used by Google client libraries
func New(projectId string, options ...Option) (*FireQL, error) {
	fql := &FireQL{context: &util.Context{ProjectId: projectId}}
	for _, opt := range options {
		if err := opt(fql); err != nil {
			return nil, err
		}
	}
	return fql, nil
}

// Execute accepts SQL query as parameter, then parses, validates, construct
// and issue Firestore query to Google Firestore database.
// And parse results according field alias, return records.
func (fql *FireQL) Execute(query string) (*util.QueryResult, error) {
	stmtType := sqlparser.Preview(query)
	switch stmtType {
	case sqlparser.StmtSelect:
		return selectStmt.New(fql.context, query).Execute()
	default:
		return nil,
			fmt.Errorf("unsupported sql statement %s. supported querties: SELECT",
				sqlparser.StmtType(stmtType))
	}
}
