package _select

import (
	"cloud.google.com/go/firestore"
	vkit "cloud.google.com/go/firestore/apiv1"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pgollangi/fireql/pkg/support"
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
	return sel.readResults(docs, selectedFields)
}

func (sel *SelectStatement) readResults(docs *firestore.DocumentIterator, selectedColumns []*selectColumn) (*util.QueryResult, error) {
	document, err := docs.Next()
	if err == iterator.Done {
		var columns []string
		for _, column := range selectedColumns {
			columns = append(columns, column.alias)
		}
		return &util.QueryResult{Columns: columns, Records: [][]interface{}{}}, nil
	}

	// Insert COLUMNS for START (*) selection
	starIdx := -1
	for idx, column := range selectedColumns {
		if column.colType == Star {
			starIdx = idx
			break
		}
	}
	if starIdx != -1 {
		// Remove star Select as we insert real columns
		selectedColumns = append(selectedColumns[:starIdx], selectedColumns[starIdx+1:]...)
		data := document.Data()
		for key := range data {
			newCol := &selectColumn{
				field:   key,
				alias:   key,
				colType: Field,
			}
			if len(selectedColumns) == starIdx {
				selectedColumns = append(selectedColumns, newCol)
			} else {
				selectedColumns = append(selectedColumns[:starIdx+1], selectedColumns[starIdx:]...)
				selectedColumns[starIdx] = newCol
			}
			starIdx++
		}
	}

	var columns []string
	var rows [][]interface{}

	for _, column := range selectedColumns {
		columns = append(columns, column.alias)
	}

	for {
		row := make([]interface{}, len(columns))
		rows = append(rows, row)

		data := document.Data()

		for idx, column := range selectedColumns {
			val, err := readColumnValue(document, &data, column)
			if err != nil {
				return nil, err
			}
			row[idx] = val
		}
		document, err = docs.Next()
		if err == iterator.Done {
			break
		}
	}

	return &util.QueryResult{Columns: columns, Records: rows}, nil
}

func readColumnValue(document *firestore.DocumentSnapshot, data *map[string]interface{}, column *selectColumn) (interface{}, error) {
	var val interface{}
	switch column.colType {
	case Field:
		fieldPaths := strings.Split(column.field, ".")
		var colData interface{}
		colData = *data
		for _, fPath := range fieldPaths {
			colData = colData.(map[string]interface{})[fPath]
			if colData == nil {
				return nil, fmt.Errorf(`unknown field "%s" in doc "%s"`, column.field, document.Ref.ID)
			}
		}
		val = colData
		break
	case Function:
		params := make([]interface{}, len(column.params))
		for i, param := range column.params {
			paramVal, err := readColumnValue(document, data, param)
			if err != nil {
				return nil, err
			}
			params[i] = paramVal

		}
		funcVal, err := support.ExecFunc(column.field, params)
		if err != nil {
			return nil, err
		}
		val = funcVal
		break
	}
	return val, nil
}

type ColumnType int

const (
	Field    ColumnType = 0
	Function            = 1
	Star                = 2
)

type selectColumn struct {
	field   string
	alias   string
	colType ColumnType
	params  []*selectColumn
}

func (sel *SelectStatement) selectFields(fQuery firestore.Query, sQuery *sqlparser.Select) (firestore.Query, []*selectColumn, error) {
	qSelects := sQuery.SelectExprs

	columns, err := sel.collectSelectColumns(qSelects)
	if err != nil {
		return fQuery, nil, err
	}

	selects := sel.collectSelectFields(columns)
	if len(selects) > 0 {
		fQuery = fQuery.Select(selects...)
	}
	return fQuery, columns, nil
}

func (sel *SelectStatement) collectSelectFields(columns []*selectColumn) []string {
	var fields []string
loop:
	for _, col := range columns {
		switch col.colType {
		case Field:
			fields = append(fields, col.field)
			break
		case Function:
			paramFields := sel.collectSelectFields(col.params)
			fields = append(fields, paramFields...)
			break
		case Star:
			// Don't select fields on firestore.Query to return all fields
			fields = []string{}
			break loop
		}
	}
	return fields
}

func (sel *SelectStatement) collectSelectColumns(qSelects sqlparser.SelectExprs) ([]*selectColumn, error) {

	var columns []*selectColumn
	for _, qSelect := range qSelects {
		switch qSelect := qSelect.(type) {
		case *sqlparser.StarExpr:
			columns = append(columns, &selectColumn{
				field:   "*",
				colType: Star,
			})
			break
		case *sqlparser.AliasedExpr:
			switch colExpr := qSelect.Expr.(type) {
			case *sqlparser.ColName:
				field := colExpr.Name.String()
				alias := qSelect.As.String()
				if alias == "" {
					alias = field
				}
				columns = append(columns, &selectColumn{
					field:   field,
					alias:   alias,
					colType: Field,
				})
				break
			case *sqlparser.FuncExpr:
				name := colExpr.Name.String()
				alias := qSelect.As.String()
				if alias == "" {
					alias = name
				}
				params, err := sel.collectSelectColumns(colExpr.Exprs)
				if err != nil {
					return nil, err
				}
				err = support.ValidateFunc(name, params)
				if err != nil {
					return nil, err
				}
				columns = append(columns, &selectColumn{
					field:   name,
					alias:   alias,
					colType: Function,
					params:  params,
				})
				break
			}
			break
		}
	}

	return columns, nil
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
