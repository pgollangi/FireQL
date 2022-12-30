package _select

import (
	"cloud.google.com/go/firestore"
	vkit "cloud.google.com/go/firestore/apiv1"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pgollangi/fireql/pkg/util"
	"github.com/xwb1989/sqlparser"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"strconv"
	"strings"
)

type SelectStatement struct {
	context  *util.Context
	rawQuery string
}

type SelectResult struct {
	Fields  []string
	Records []map[string]interface{}
}

func New(context *util.Context, rawQuery string) *SelectStatement {
	return &SelectStatement{
		context,
		rawQuery,
	}
}

func (sel *SelectStatement) Execute() (*util.QueryResult, error) {
	stmt, err := sqlparser.Parse(sel.rawQuery)
	if err != nil {
		return nil, err
	}

	sQuery := stmt.(*sqlparser.Select)

	from := sQuery.From
	if len(from) != 1 {
		return nil, errors.New("there must be a FROM collection")
	}
	qCollectionName := sqlparser.String(sQuery.From[0])

	fireClient, err := sel.newFireClient()
	if err != nil {
		return nil, err
	}
	defer fireClient.Close()

	fCollection := fireClient.Collection(qCollectionName)
	fQuery := fCollection.Query

	fQuery, selectedFields, err := sel.selectFields(fQuery, sQuery)
	if err != nil {
		return nil, err
	}
	fQuery, err = sel.addWhere(fQuery, sQuery)
	if err != nil {
		return nil, err
	}
	fQuery, err = sel.addLimit(fQuery, sQuery)
	if err != nil {
		return nil, err
	}
	fQuery, err = sel.addOrderBy(fQuery, sQuery)
	if err != nil {
		return nil, err
	}
	docs := fQuery.Documents(context.Background())
	result, fields, err := sel.readResults(docs, selectedFields)
	if err != nil {
		return nil, err
	}

	return &util.QueryResult{Records: result, Fields: fields}, nil
}

func (sel *SelectStatement) readResults(docs *firestore.DocumentIterator, selectedFields map[string]string) ([]map[string]interface{}, []string, error) {
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
			for key := range data {
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

func (sel *SelectStatement) selectFields(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, map[string]string, error) {
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
		for name := range fields {
			selects[i] = name
			i++
		}
		fQuery = fQuery.Select(selects...)
	}
	return fQuery, fields, nil
}

func (sel *SelectStatement) addWhere(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, error) {
	var err error
	qWhere := sQuery.Where
	if qWhere != nil {
		if qWhere.Type == sqlparser.WhereStr {
			fQuery, err = sel.addWhereExpr(fQuery, sQuery, qWhere.Expr)
			if err != nil {
				return fQuery, err
			}
		} else {
			return fQuery, fmt.Errorf("unsupported WHERE type: %s", qWhere.Type)
		}
	}
	return fQuery, nil
}

func (sel *SelectStatement) addWhereExpr(fQuery firestore.Query, sQuery *sqlparser.Select, expr sqlparser.Expr) (firestore.Query, error) {
	var err error
	switch expr := expr.(type) {
	case *sqlparser.AndExpr:
		fQuery, err = sel.addWhereExpr(fQuery, sQuery, expr.Left)
		if err != nil {
			return fQuery, err
		}
		fQuery, err = sel.addWhereExpr(fQuery, sQuery, expr.Right)
		if err != nil {
			return fQuery, err
		}
	case *sqlparser.ComparisonExpr:
		val, err := sel.getValueFromExpr(expr.Right)
		if err != nil {
			return fQuery, err
		}
		fQuery = fQuery.Where(expr.Left.(*sqlparser.ColName).Name.String(),
			sel.getCompareOperator(expr.Operator), val)
	default:
		return fQuery, fmt.Errorf("unsupported WHERE clause: %s", sqlparser.String(expr))
	}
	return fQuery, nil
}

func (sel *SelectStatement) getCompareOperator(op string) string {
	switch op {
	case sqlparser.EqualStr:
		return "=="
	case sqlparser.NotInStr:
		return "not-in"
	}
	return op
}

func (sel *SelectStatement) getValueFromExpr(valExpr sqlparser.Expr) (interface{}, error) {
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
			val, err := sel.getValueFromExpr(expr)
			if err != nil {
				return nil, err
			}
			values[idx] = val
		}
		return values, nil
	case *sqlparser.ParenExpr:
		return sel.getValueFromExpr(valExpr.Expr)
	}
	return nil, nil
}

func (sel *SelectStatement) addLimit(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, error) {
	if sQuery.Limit != nil {
		// Offset not supported by Firestore
		rows, err := sel.getValueFromExpr(sQuery.Limit.Rowcount)
		if err != nil {
			return fQuery, err
		}
		fQuery = fQuery.Limit(rows.(int))
	} else if sel.context.DefaultLimit > 0 {
		fQuery = fQuery.Limit(sel.context.DefaultLimit)
	}
	return fQuery, nil
}

func (sel *SelectStatement) addOrderBy(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, error) {
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

func (sel *SelectStatement) newFireClient() (*firestore.Client, error) {
	ctx := context.Background()

	var firestoreOptions []option.ClientOption
	if len(sel.context.ServiceAccount) > 0 {
		if !json.Valid([]byte(sel.context.ServiceAccount)) {
			return nil, errors.New("invalid service account, it is expected to be a JSON")
		}

		creds, err := google.CredentialsFromJSON(ctx, []byte(sel.context.ServiceAccount),
			vkit.DefaultAuthScopes()...,
		)
		if err != nil {
			return nil, fmt.Errorf("ServiceAccount: %v", err)
		}
		firestoreOptions = append(firestoreOptions, option.WithCredentials(creds))
	}

	return firestore.NewClient(ctx, sel.context.ProjectId, firestoreOptions...)
}
