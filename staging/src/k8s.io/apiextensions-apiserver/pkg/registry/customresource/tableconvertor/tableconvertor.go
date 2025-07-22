/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tableconvertor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"

	"k8s.io/klog/v2"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apiserver/pkg/cel/environment"

	"github.com/google/cel-go/common/types/ref"
	"k8s.io/apimachinery/pkg/api/meta"
	metatable "k8s.io/apimachinery/pkg/api/meta/table"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	celopenapi "k8s.io/apiserver/pkg/cel/common"

	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel/model"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/util/jsonpath"
)

var swaggerMetadataDescriptions = metav1.ObjectMeta{}.SwaggerDoc()

func UnstructuredToVal(unstructured interface{}, schema *schema.Structural) ref.Val {
	return celopenapi.UnstructuredToVal(unstructured, &model.Structural{Structural: schema})
}

const (
	// ScopedVarName is the variable name assigned to the locally scoped data element of a CEL validation
	// expression.
	ScopedVarName = "self"

	// OldScopedVarName is the variable name assigned to the existing value of the locally scoped data element of a
	// CEL validation expression.
	OldScopedVarName = "oldSelf"
)

// New creates a new table convertor for the provided CRD column definition. If the printer definition cannot be parsed,
// error will be returned along with a default table convertor.
func New(crdColumns []apiextensionsv1.CustomResourceColumnDefinition, s *schema.Structural) (rest.TableConvertor, error) {
	headers := []metav1.TableColumnDefinition{
		{Name: "Name", Type: "string", Format: "name", Description: swaggerMetadataDescriptions["name"]},
	}
	c := &convertor{
		headers: headers,
	}

	klog.V(1).Info("Inside tableconvertor New function")
	klog.V(1).Info(c)

	for _, col := range crdColumns {
		klog.V(1).Info(col)
		// klog.V(1).Info(path)
		// klog.V(1).Info(path.Parse(fmt.Sprintf("{%s}", col.JSONPath)))
		// TODO (Sreeram/Priyanka): Comment-Nov28
		// We need to add the CEL compilation logic bit here in place of JSONPath parsing when dealing with col.Expression

		if len(col.JSONPath) > 0 && len(col.Expression) == 0 {
			start := time.Now()
			path := jsonpath.New(col.Name)
			if err := path.Parse(fmt.Sprintf("{%s}", col.JSONPath)); err != nil {
				return c, fmt.Errorf("unrecognized column definition %q", col.JSONPath)
			}
			path.AllowMissingKeys(true)
			duration := time.Since(start)
			klog.Infof("Time taken for JSONPath execution %v: %s\n", col.JSONPath, duration)

			c.additionalColumns = append(c.additionalColumns, path)
		} else if len(col.Expression) > 0 && len(col.JSONPath) == 0 {
			// klog.V(1).Info("Inside the cel block in tableconverter.new()")
			// prog, err := crdcel.FinalColumnCompile(col.Expression)

			start := time.Now()
			compResult := cel.CompileColumn(col.Expression, s, model.SchemaDeclType(s, true), celconfig.PerCallLimit, environment.MustBaseEnvSet(environment.DefaultCompatibilityVersion(), true), cel.StoredExpressionsEnvLoader())
			duration := time.Since(start)
			klog.Infof("Time 6: Total time taken for entire CEL compilation %v: %s\n", col.Expression, duration)

			// klog.Infof("May 6 Error in CEL program compilation in tableconvertor: %v", compResult.Error)
			// klog.Infof("May 6 CEL program compresult in tableconvertor: %v", compResult)
			// klog.V(1).Infof("FINAL ERROR PRINTING: %v", err)
			// TODO (sreeram/Priyanka): Comment-Jan 6 2025
			// TLDR path = CEL Program (prog)
			// For JSONPath, path := jsonpath.New, similarly for CEL we're collecting the CEL prog, path := cel.CompileColumn() (function we wrote after copy pasting CompileColumns)
			// Next thing to take care is how path currently implements findResults, printResults, we need to implement those for cel prog as well so that we can append prog to c.additionalColumns
			if compResult.Error != nil {
				return c, fmt.Errorf("CEL compilation error %q", compResult.Error)
			}
			c.additionalColumns = append(c.additionalColumns, compResult)
		}
		// END Comment-Nov28

		desc := fmt.Sprintf("Custom resource definition column (in JSONPath format): %s", col.JSONPath)
		if len(col.Description) > 0 {
			desc = col.Description
		}

		c.headers = append(c.headers, metav1.TableColumnDefinition{
			Name:        col.Name,
			Type:        col.Type,
			Format:      col.Format,
			Description: desc,
			Priority:    col.Priority,
		})
	}

	klog.V(1).Info("Final c before returning")
	klog.V(1).Info(c)
	return c, nil
}

type columnPrinter interface {
	FindResults(data interface{}) ([][]reflect.Value, error)
	PrintResults(w io.Writer, results []reflect.Value) error
}

type convertor struct {
	headers           []metav1.TableColumnDefinition
	additionalColumns []columnPrinter
}

func (c *convertor) ConvertToTable(ctx context.Context, obj runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	table := &metav1.Table{}
	opt, ok := tableOptions.(*metav1.TableOptions)
	noHeaders := ok && opt != nil && opt.NoHeaders
	if !noHeaders {
		table.ColumnDefinitions = c.headers
	}

	if m, err := meta.ListAccessor(obj); err == nil {
		table.ResourceVersion = m.GetResourceVersion()
		table.Continue = m.GetContinue()
		table.RemainingItemCount = m.GetRemainingItemCount()
	} else {
		if m, err := meta.CommonAccessor(obj); err == nil {
			table.ResourceVersion = m.GetResourceVersion()
		}
	}

	var err error
	buf := &bytes.Buffer{}
	table.Rows, err = metatable.MetaToTableRow(obj, func(obj runtime.Object, m metav1.Object, name, age string) ([]interface{}, error) {
		cells := make([]interface{}, 1, 1+len(c.additionalColumns))
		cells[0] = name
		customHeaders := c.headers[1:]
		us, ok := obj.(runtime.Unstructured)
		if !ok {
			m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
			if err != nil {
				return nil, err
			}
			us = &unstructured.Unstructured{Object: m}
		}
		for i, column := range c.additionalColumns {
			// TODO (Sreeram/Priyanka): Comment-Nov28
			// We need to add the evaluation logic for compiled CEL expressions here in place of JSONPath equivalent of FindResults and PrintResults when dealing with col.Expression

			// klog.V(1).Info("Going to call FindResults now!!!")
			// klog.V(1).Infof("Column: %v", column)

			start := time.Now()
			results, err := column.FindResults(us.UnstructuredContent())
			if err != nil || len(results) == 0 || len(results[0]) == 0 {
				cells = append(cells, nil)
				continue
			}
			duration := time.Since(start)
			klog.Infof("Time taken for findResults for %v: %s\n", column, duration)

			// klog.V(1).Info("FindResults finished, going to do PrintResults now")
			// klog.V(1).Infof("FindResults result: %v", results)

			// as we only support simple JSON path, we can assume to have only one result (or none, filtered out above)
			value := results[0][0].Interface()
			if customHeaders[i].Type == "string" {
				klog.V(1).Info("Printing value we got from findResults")
				klog.V(1).Info(value)
				start := time.Now()
				if err := column.PrintResults(buf, []reflect.Value{reflect.ValueOf(value)}); err == nil {
					cells = append(cells, buf.String())
					buf.Reset()
				} else {
					cells = append(cells, nil)
				}
				duration := time.Since(start)
				klog.Infof("Time taken for printResults for %v: %s\n", column, duration)
			} else {
				// TODO (Sreeram/Priyanka): Comment-Nov28
				// Figure out cellForJSONValue function and if we need an equivalent cellForCELValue function
				// Look if this can be used for error handling for CEL evaluation errors.
				cells = append(cells, cellForJSONValue(customHeaders[i].Type, value))
			}
			// END Comment-Nov28
		}
		return cells, nil
	})
	return table, err
}

func cellForJSONValue(headerType string, value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch headerType {
	case "integer":
		switch typed := value.(type) {
		case int64:
			return typed
		case float64:
			return int64(typed)
		case json.Number:
			if i64, err := typed.Int64(); err == nil {
				return i64
			}
		}
	case "number":
		switch typed := value.(type) {
		case int64:
			return float64(typed)
		case float64:
			return typed
		case json.Number:
			if f, err := typed.Float64(); err == nil {
				return f
			}
		}
	case "boolean":
		if b, ok := value.(bool); ok {
			return b
		}
	case "string":
		if s, ok := value.(string); ok {
			return s
		}
	case "date":
		if typed, ok := value.(string); ok {
			var timestamp metav1.Time
			err := timestamp.UnmarshalQueryParameter(typed)
			if err != nil {
				return "<invalid>"
			}
			return metatable.ConvertToHumanReadableDateType(timestamp)
		}
	}

	return nil
}
