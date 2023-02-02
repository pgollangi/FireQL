package support

import (
	"fmt"
	"github.com/Knetic/govaluate"
	"reflect"
	"strings"
)

type Function func(data []interface{}) (interface{}, error)

type FunctionRegistration struct {
	function       Function
	requiredParams int
}

var functions = map[string]*FunctionRegistration{}

func init() {
	functions = map[string]*FunctionRegistration{
		"LENGTH": &FunctionRegistration{
			function:       Length,
			requiredParams: 1,
		},
	}
}

func ExecFunc(name string, data []interface{}) (interface{}, error) {
	err := ValidateFunc(name, data)
	if err != nil {
		return nil, err
	}
	return functions[strings.ToUpper(name)].function(data)
}

func ValidateFunc(name string, params interface{}) error {
	name = strings.ToUpper(name)
	funRegistration := functions[name]
	if funRegistration == nil {
		return fmt.Errorf(`unknown function "%s"`, name)
	}
	switch v := params.(type) {
	case []interface{}:
		if len(v) < funRegistration.requiredParams {
			return fmt.Errorf(`insufficient params to "%s" function. expects %d params`, name, funRegistration.requiredParams)
		}
	}

	return nil
}

func Length(data []interface{}) (result interface{}, err error) {
	defer func() {
		if e := recover(); e != nil {
			result = nil
			err = fmt.Errorf(`LENGTH of type "%v" is not supported`, e.(*reflect.ValueError).Kind)
		}
	}()
	return float64(reflect.ValueOf(data[0]).Len()), nil
}

func GetEvalFunctions() map[string]govaluate.ExpressionFunction {
	result := map[string]govaluate.ExpressionFunction{}
	for name, fun := range functions {
		result[name] = func(args ...interface{}) (interface{}, error) {
			return fun.function(args)
		}
	}
	return result
}
