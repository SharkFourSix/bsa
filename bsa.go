package bsa

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/georgysavva/scany/v2/sqlscan"
)

var (
	errorType = reflect.TypeOf((*error)(nil)).Elem()
)

type QueryLoader interface {
	Get(name string) (string, error)
}

type Querier interface {
	Query(q string, params ...any) (*sql.Rows, error)
}

type ContextQuerier interface {
	QueryContext(ctx context.Context, q string, params ...any) (*sql.Rows, error)
}

type Executioner interface {
	Exec(q string, params ...any) (sql.Result, error)
}

type ContextExecutioner interface {
	ExecContext(ctx context.Context, q string, params ...any) (sql.Result, error)
}

type DBResource interface {
	Querier
	ContextQuerier
	Executioner
	ContextExecutioner
}

func getErrorValue(err error) reflect.Value {
	if err == nil {
		return reflect.Zero(errorType)
	} else {
		return reflect.ValueOf(err)
	}
}

// Bind binds all the functions declared in the given struct.
//
// Some database systems (PostgreSQL) do not support the concept of last insert id. For such databases, the
// lastInsertIDSupport parameter must be false.
func Bind(ctx context.Context, handle DBResource, dst any, qLoder QueryLoader, lastInsertIDSupport bool) error {
	vType := reflect.TypeOf(dst)

	if vType.Kind() != reflect.Pointer {
		return fmt.Errorf("must be a pointer")
	}

	if vType.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("must be a pointer to a struct. found: %v", vType)
	}

	runner := func(typ reflect.Type, isExec, singleSelection bool, query string) func([]reflect.Value) []reflect.Value {
		numOut := typ.NumOut()
		if isExec {
			exec := func(args []reflect.Value) (int64, int64, error) {
				values := []any{}
				var (
					id, affected int64
					executor     DBResource = handle
				)
				if len(args) >= 1 {
					if tx, ok := args[0].Interface().(DBResource); ok {
						executor = tx
						for _, v := range args[1:] {
							values = append(values, v.Interface())
						}
					} else {
						for _, v := range args {
							values = append(values, v.Interface())
						}
					}
				} else {
					for _, v := range args {
						values = append(values, v.Interface())
					}
				}
				result, err := executor.ExecContext(ctx, query, values...)
				if err != nil {
					return 0, 0, err
				}
				if lastInsertIDSupport {
					if id, err = result.LastInsertId(); err != nil {
						return 0, 0, err
					}
				}
				if affected, err = result.RowsAffected(); err != nil {
					return 0, 0, err
				}
				return id, affected, nil
			}
			switch numOut {
			case 0: // nothing (will panic on error)
				return func(args []reflect.Value) (results []reflect.Value) {
					_, _, err := exec(args)
					if err != nil {
						panic(err)
					}
					return results
				}
			case 1: // error only
				return func(args []reflect.Value) (results []reflect.Value) {
					_, _, err := exec(args)
					results = append(results, getErrorValue(err))
					return
				}
			case 2: // int64,int64 will panic on error
				return func(args []reflect.Value) (results []reflect.Value) {
					id, affected, err := exec(args)
					if err != nil {
						panic(err)
					}
					results = append(results, reflect.ValueOf(id), reflect.ValueOf(affected))
					return
				}
			case 3: // int64,int64,error no panic
				return func(args []reflect.Value) (results []reflect.Value) {
					id, affected, err := exec(args)
					results = append(results, reflect.ValueOf(id), reflect.ValueOf(affected), getErrorValue(err))
					return
				}
			}
		} else {
			// select
			Query := func(args []reflect.Value, one bool, oType reflect.Type) (any, error) {
				values := []any{}
				executor := handle

				if len(args) >= 1 {
					if tx, ok := args[0].Interface().(DBResource); ok {
						executor = tx
						for _, v := range args[1:] {
							values = append(values, v.Interface())
						}
					} else {
						for _, v := range args {
							values = append(values, v.Interface())
						}
					}
				} else {
					for _, v := range args {
						values = append(values, v.Interface())
					}
				}
				rows, err := executor.QueryContext(ctx, query, values...)
				if err != nil {
					return nil, err
				}
				addressOf := func(v reflect.Value) reflect.Value {
					ptr := reflect.New(oType)
					ptr.Elem().Set(v)
					return ptr
				}
				if one {
					if oType.Kind() == reflect.Pointer {
						elem := oType.Elem()
						ptr := reflect.New(elem)
						err = sqlscan.ScanOne(ptr.Interface(), rows)
						if err != nil && errors.Is(err, sql.ErrNoRows) {
							return reflect.Zero(oType).Interface(), nil
						}
						return ptr.Interface(), err
					} else {
						dstRefValue := reflect.New(oType)
						err = sqlscan.ScanRow(dstRefValue.Interface(), rows)
						if err != nil && errors.Is(err, sql.ErrNoRows) {
							err = nil
						}
						return dstRefValue.Interface(), err
					}
				} else {
					valueInstance := reflect.MakeSlice(oType, 0, 0)
					valPointer := addressOf(valueInstance)
					err = sqlscan.ScanAll(valPointer.Interface(), rows)
					return valPointer.Elem().Interface(), err
				}
			}
			switch numOut {
			case 1: // type only, will panic
				return func(args []reflect.Value) []reflect.Value {
					rows, err := Query(args, singleSelection, typ.Out(0))
					if err != nil {
						panic(err)
					}
					return []reflect.Value{reflect.ValueOf(rows)}
				}
			case 2: // type, error no panic
				return func(args []reflect.Value) []reflect.Value {
					rows, err := Query(args, singleSelection, typ.Out(0))

					var rowsValue reflect.Value
					if rows == nil {
						rowsValue = reflect.Zero(typ.Out(0))
					} else {
						rowsValue = reflect.ValueOf(rows)
					}

					return []reflect.Value{rowsValue, getErrorValue(err)}
				}
			}
		}
		panic(fmt.Errorf("%v: unsupported return type", typ))
	}

	vType = vType.Elem()
	value := reflect.ValueOf(dst).Elem()

	resolveQuery := func(q, fnName string) (string, error) {
		spec, found := strings.CutPrefix(q, "file:")
		if found {
			if !strings.HasSuffix(spec, ".sql") {
				spec = spec + ".sql"
			}
			spec, err := qLoder.Get(spec)
			if err != nil {
				return "", errors.Join(fmt.Errorf("%s: failed to load file %s", fnName, q), err)
			}
			return spec, nil
		} else {
			return spec, nil
		}
	}

	for i := 0; i < vType.NumField(); i++ {
		field := vType.Field(i)
		fType := field.Type
		if fType.Kind() == reflect.Func && field.IsExported() {
			exec := field.Tag.Get("@exec")
			queryAll := field.Tag.Get("@query")
			queryOne := field.Tag.Get("@queryOne")

			if queryOne != "" {
				script, err := resolveQuery(queryOne, field.Name)
				if err != nil {
					return err
				}
				value.FieldByName(field.Name).
					Set(reflect.MakeFunc(fType, runner(fType, false, true, script)))
			} else if queryAll != "" {
				script, err := resolveQuery(queryAll, field.Name)
				if err != nil {
					return err
				}
				value.FieldByName(field.Name).
					Set(reflect.MakeFunc(fType, runner(fType, false, false, script)))
			} else if exec != "" {
				script, err := resolveQuery(exec, field.Name)
				if err != nil {
					return err
				}
				value.FieldByName(field.Name).
					Set(reflect.MakeFunc(fType, runner(fType, true, false, script)))
			} else {
				return fmt.Errorf("%s: function is defined but does not have any queries", field.Name)
			}
		}
	}
	return nil
}
