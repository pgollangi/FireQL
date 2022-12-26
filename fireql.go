package fireql

import (
	"cloud.google.com/go/firestore"
	"context"
	"errors"
	"fmt"
	"github.com/xwb1989/sqlparser"
	"google.golang.org/api/iterator"
	"strconv"
	"strings"
)

type FireQL struct {
	projectId string
}

type QueryResult struct {
	Fields  []string
	Records []map[string]interface{}
}

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

	fireClient, err := firestore.NewClient(ctx, fql.projectId)
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
					return nil, nil, fmt.Errorf(`unknown field "%s"`, selectedField)
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
	for _, alias := range selectedFields {
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
		fQuery = fQuery.Where(expr.Left.(*sqlparser.ColName).Name.String(), expr.Operator, val)
	default:
		return fQuery, fmt.Errorf("unsupported WHERE clause: %s", sqlparser.String(expr))
	}
	return fQuery, nil
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
	}
	return fQuery, nil
}

func (fql *FireQL) addOrderBy(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, error) {
	return fQuery, nil
}

func NewFireQL(projectId string) (*FireQL, error) {
	return &FireQL{projectId}, nil
}
