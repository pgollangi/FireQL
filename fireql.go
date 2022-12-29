package fireql

import (
	"cloud.google.com/go/firestore"
	vkit "cloud.google.com/go/firestore/apiv1"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xwb1989/sqlparser"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"strconv"
	"strings"
)

// FireQL object is constructed to execute
// SQL queries on Firestore database.
// FireQL internally issue queries constructed from SQL
// on Firestore database using Google Firestore client library
type FireQL struct {
	projectId      string
	serviceAccount string
	defaultLimit   int
}

type QueryResult struct {
	Fields  []string
	Records []map[string]interface{}
}

// New creates new FireQL instance using the "projectId"
// passed. Accepts list of Option to pass other configuration params.
// Google Application Default Credentials are used by
// Firestore client library if service account is not passed via Option.
// See https://cloud.google.com/docs/authentication/client-libraries
// for more information about how Application Default Credentials are
// used by Google client libraries
func New(projectId string, options ...Option) (*FireQL, error) {
	fql := &FireQL{projectId: projectId}
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
func (fql *FireQL) Execute(query string) (*QueryResult, error) {
	stmt, err := sqlparser.Parse(query)
	if err != nil {
		return nil, err
	}
	switch stmt := stmt.(type) {
	case *sqlparser.Select:
		return fql.executeSelect(stmt)
	default:
		return nil,
			fmt.Errorf("unsupported sql statement %s. supported querties: SELECT",
				sqlparser.StmtType(sqlparser.Preview(query)))
	}
}

func (fql *FireQL) executeSelect(sQuery *sqlparser.Select) (*QueryResult, error) {
	from := sQuery.From
	if len(from) != 1 {
		return nil, errors.New("there must be a FROM collection")
	}
	qCollectionName := sqlparser.String(sQuery.From[0])

	ctx := context.Background()

	var firestoreOptions []option.ClientOption
	if len(fql.serviceAccount) > 0 {
		if !json.Valid([]byte(fql.serviceAccount)) {
			return nil, errors.New("invalid service account, it is expected to be a JSON")
		}

		creds, err := google.CredentialsFromJSON(ctx, []byte(fql.serviceAccount),
			vkit.DefaultAuthScopes()...,
		)
		if err != nil {
			return nil, fmt.Errorf("ServiceAccount: %v", err)
		}
		firestoreOptions = append(firestoreOptions, option.WithCredentials(creds))
	}

	fireClient, err := firestore.NewClient(ctx, fql.projectId, firestoreOptions...)
	if err != nil {
		return nil, err
	}
	defer fireClient.Close()

	fCollection := fireClient.Collection(qCollectionName)
	fQuery := fCollection.Query

	fQuery, selectedFields, err := fql.selectFields(fQuery, sQuery)
	if err != nil {
		return nil, err
	}
	fQuery, err = fql.addWhere(fQuery, sQuery)
	if err != nil {
		return nil, err
	}
	fQuery, err = fql.addLimit(fQuery, sQuery)
	if err != nil {
		return nil, err
	}
	fQuery, err = fql.addOrderBy(fQuery, sQuery)
	if err != nil {
		return nil, err
	}
	docs := fQuery.Documents(ctx)
	result, fields, err := fql.readResults(docs, selectedFields)
	if err != nil {
		return nil, err
	}

	return &QueryResult{Records: result, Fields: fields}, nil
}

func (fql *FireQL) readResults(docs *firestore.DocumentIterator, selectedFields map[string]string) ([]map[string]interface{}, []string, error) {
	var result []map[string]interface{}
	for {
		document, err := docs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error reading documents %v", err)
		}
		data := document.Data()

		if len(selectedFields) == 0 {
			for key, _ := range data {
				selectedFields[key] = key
			}
		}

		row := map[string]interface{}{}

		for selectedField, alias := range selectedFields {
			fieldPaths := strings.Split(selectedField, ".")
			var val interface{}
			val = data
			for _, fPath := range fieldPaths {
				val = val.(map[string]interface{})[fPath]
				if val == nil {
					return nil, nil, fmt.Errorf(`unknown field "%s" in doc "%s"`, selectedField, document.Ref.ID)
				}
			}
			if len(alias) == 0 {
				alias = selectedField
			}
			row[alias] = val
		}
		result = append(result, row)
	}

	var columns []string
	for field, alias := range selectedFields {
		if len(alias) == 0 {
			alias = field
		}
		columns = append(columns, alias)
	}

	return result, columns, nil
}

func (fql *FireQL) selectFields(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, map[string]string, error) {
	qSelects := sQuery.SelectExprs
	fields := map[string]string{}

selects:
	for _, qSelect := range qSelects {
		switch qSelect := qSelect.(type) {
		case *sqlparser.StarExpr:
			fields = map[string]string{}
			break selects
		case *sqlparser.AliasedExpr:
			field := qSelect.Expr.(*sqlparser.ColName).Name.String()
			alias := qSelect.As.String()
			fields[field] = alias
		}
	}

	if len(fields) > 0 {
		selects := make([]string, len(fields))
		i := 0
		for name, _ := range fields {
			selects[i] = name
			i++
		}
		fQuery = fQuery.Select(selects...)
	}
	return fQuery, fields, nil
}

func (fql *FireQL) addWhere(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, error) {
	var err error
	qWhere := sQuery.Where
	if qWhere != nil {
		if qWhere.Type == sqlparser.WhereStr {
			fQuery, err = fql.addWhereExpr(fQuery, sQuery, qWhere.Expr)
			if err != nil {
				return fQuery, err
			}
		} else {
			return fQuery, fmt.Errorf("unsupported WHERE type: %s", qWhere.Type)
		}
	}
	return fQuery, nil
}

func (fql *FireQL) addWhereExpr(fQuery firestore.Query, sQuery *sqlparser.Select, expr sqlparser.Expr) (firestore.Query, error) {
	var err error
	switch expr := expr.(type) {
	case *sqlparser.AndExpr:
		fQuery, err = fql.addWhereExpr(fQuery, sQuery, expr.Left)
		if err != nil {
			return fQuery, err
		}
		fQuery, err = fql.addWhereExpr(fQuery, sQuery, expr.Right)
		if err != nil {
			return fQuery, err
		}
	case *sqlparser.ComparisonExpr:
		val, err := fql.getValueFromExpr(expr.Right)
		if err != nil {
			return fQuery, err
		}
		fQuery = fQuery.Where(expr.Left.(*sqlparser.ColName).Name.String(),
			fql.getCompareOperator(expr.Operator), val)
	default:
		return fQuery, fmt.Errorf("unsupported WHERE clause: %s", sqlparser.String(expr))
	}
	return fQuery, nil
}

func (fql *FireQL) getCompareOperator(op string) string {
	switch op {
	case sqlparser.EqualStr:
		return "=="
	case sqlparser.NotInStr:
		return "not-in"
	}
	return op
}

func (fql *FireQL) getValueFromExpr(valExpr sqlparser.Expr) (interface{}, error) {
	switch valExpr := valExpr.(type) {
	case sqlparser.BoolVal:
		return valExpr, nil
	case *sqlparser.SQLVal:
		switch valExpr.Type {
		case sqlparser.IntVal:
			val, err := strconv.Atoi(string(valExpr.Val))
			if err != nil {
				return nil, err
			}
			return val, nil
		case sqlparser.FloatVal:
			val, err := strconv.ParseFloat(string(valExpr.Val), 64)
			if err != nil {
				return nil, err
			}
			return val, nil
		default:
			return string(valExpr.Val), nil
		}
	case sqlparser.ValTuple:
		values := make([]interface{}, len(valExpr))
		for idx, expr := range valExpr {
			val, err := fql.getValueFromExpr(expr)
			if err != nil {
				return nil, err
			}
			values[idx] = val
		}
		return values, nil
	case *sqlparser.ParenExpr:
		return fql.getValueFromExpr(valExpr.Expr)
	}
	return nil, nil
}

func (fql *FireQL) addLimit(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, error) {
	if sQuery.Limit != nil {
		// Offset not supported by Firestore
		rows, err := fql.getValueFromExpr(sQuery.Limit.Rowcount)
		if err != nil {
			return fQuery, err
		}
		fQuery = fQuery.Limit(rows.(int))
	} else if fql.defaultLimit > 0 {
		fQuery = fQuery.Limit(fql.defaultLimit)
	}
	return fQuery, nil
}

func (fql *FireQL) addOrderBy(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, error) {
	sOrders := sQuery.OrderBy
	for _, sOrder := range sOrders {
		column := sOrder.Expr.(*sqlparser.ColName).Name.String()
		direction := firestore.Asc
		if sOrder.Direction == sqlparser.DescScr {
			direction = firestore.Desc
		}
		fQuery = fQuery.OrderBy(column, direction)
	}
	return fQuery, nil
}
