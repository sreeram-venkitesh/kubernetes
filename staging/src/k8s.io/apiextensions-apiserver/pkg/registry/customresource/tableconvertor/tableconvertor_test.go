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
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1beta1 "k8s.io/apimachinery/pkg/apis/meta/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/jsonpath"
	"k8s.io/klog/v2"
)

func Test_cellForJSONValue(t *testing.T) {
	tests := []struct {
		headerType string
		value      interface{}
		want       interface{}
	}{
		{"integer", int64(42), int64(42)},
		{"integer", float64(3.14), int64(3)},
		{"integer", true, nil},
		{"integer", "foo", nil},

		{"number", int64(42), float64(42)},
		{"number", float64(3.14), float64(3.14)},
		{"number", true, nil},
		{"number", "foo", nil},

		{"boolean", int64(42), nil},
		{"boolean", float64(3.14), nil},
		{"boolean", true, true},
		{"boolean", "foo", nil},

		{"string", int64(42), nil},
		{"string", float64(3.14), nil},
		{"string", true, nil},
		{"string", "foo", "foo"},

		{"date", int64(42), nil},
		{"date", float64(3.14), nil},
		{"date", true, nil},
		{"date", time.Now().Add(-time.Hour*12 - 30*time.Minute).UTC().Format(time.RFC3339), "12h"},
		{"date", time.Now().Add(+time.Hour*12 + 30*time.Minute).UTC().Format(time.RFC3339), "<invalid>"},
		{"date", "", "<unknown>"},

		{"unknown", "foo", nil},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%#v of type %s", tt.value, tt.headerType), func(t *testing.T) {
			if got := cellForJSONValue(tt.headerType, tt.value); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cellForJSONValue() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func BenchmarkNew_CEL_DeepComplex(b *testing.B) {
	crdColumns := []apiextensionsv1.CustomResourceColumnDefinition{
		{
			Name:       "highUtilNodes",
			Type:       "string",
			// Expression: `self.spec.environments.all(e, e.clusters.all(c, c.nodes.any(n, n.metrics.cpu > 90 && n.metrics.memory > 8000 && n.status == "Ready"))).clusters.nodes.id`,
			// Or a correct CEL expression to filter nodes accordingly
			// (example below)
			Expression: `self.spec.environments[].clusters[].nodes[exists(n, n.metrics.cpu > 90 && n.metrics.memory > 8000 && n.status == "Ready")].id`,
			// Adjust expression as needed for correctness
		},
	}

	s := validStructuralSchemaDeepComplex()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.io/v1",
			"kind":       "Complex",
			"metadata": map[string]interface{}{
				"name": "example",
			},
			"spec": map[string]interface{}{
				"environments": []map[string]interface{}{
					{
						"clusters": []map[string]interface{}{
							{
								"nodes": []map[string]interface{}{
									{
										"id":     "node-1",
										"status": "Ready",
										"metrics": map[string]interface{}{
											"cpu":    91,
											"memory": 8192,
										},
									},
									{
										"id":     "node-2",
										"status": "NotReady",
										"metrics": map[string]interface{}{
											"cpu":    75,
											"memory": 4096,
										},
									},
								},
							},
							{
								"nodes": []map[string]interface{}{
									{
										"id":     "node-3",
										"status": "Ready",
										"metrics": map[string]interface{}{
											"cpu":    95,
											"memory": 16000,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ctx := context.Background()

	convertor, err := New(crdColumns, s)
	if err != nil {
		b.Fatalf("Unexpected error creating convertor: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = convertor.ConvertToTable(ctx, obj, nil)

		if err != nil {
			b.Fatalf("Unexpected error converting to table: %v", err)
		}
	}
}

func BenchmarkNew_JSONPath_DeepComplex(b *testing.B) {
	crdColumns := []apiextensionsv1.CustomResourceColumnDefinition{
		{
			Name:     "highUtilNodes",
			Type:     "string",
			JSONPath: `$.spec.environments[*].clusters[*].nodes[?(@.metrics.memory > 8000)].id`,
		},
	}
	s := validStructuralSchemaDeepComplex()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.io/v1",
			"kind":       "Complex",
			"metadata": map[string]interface{}{
				"name": "example",
			},
			"spec": map[string]interface{}{
				"environments": []map[string]interface{}{
					{
						"clusters": []map[string]interface{}{
							{
								"nodes": []map[string]interface{}{
									{
										"id":     "node-1",
										"status": "Ready",
										"metrics": map[string]interface{}{
											"cpu":    91,
											"memory": 8192,
										},
									},
									{
										"id":     "node-2",
										"status": "NotReady",
										"metrics": map[string]interface{}{
											"cpu":    75,
											"memory": 4096,
										},
									},
								},
							},
							{
								"nodes": []map[string]interface{}{
									{
										"id":     "node-3",
										"status": "Ready",
										"metrics": map[string]interface{}{
											"cpu":    95,
											"memory": 16000,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	ctx := context.Background()

	convertor, err := New(crdColumns, s)
	if err != nil {
		b.Fatalf("Unexpected error creating convertor: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = convertor.ConvertToTable(ctx, obj, nil)
		if b.Elapsed() > 9 {
			klog.Info(b.Elapsed())
			klog.Info("Above 9!")
		}
		if err != nil {
			b.Fatalf("Unexpected error converting to table: %v", err)
		}
	}
}

func validStructuralSchemaDeepComplex() *schema.Structural {
	return &schema.Structural{
		Generic: schema.Generic{
			Type: "object",
		},
		Properties: map[string]schema.Structural{
			"spec": {
				Generic: schema.Generic{
					Type: "object",
				},
				Properties: map[string]schema.Structural{
					"environments": {
						Generic: schema.Generic{
							Type: "array",
						},
						Items: &schema.Structural{
							Generic: schema.Generic{
								Type: "object",
							},
							Properties: map[string]schema.Structural{
								"clusters": {
									Generic: schema.Generic{
										Type: "array",
									},
									Items: &schema.Structural{
										Generic: schema.Generic{
											Type: "object",
										},
										Properties: map[string]schema.Structural{
											"nodes": {
												Generic: schema.Generic{
													Type: "array",
												},
												Items: &schema.Structural{
													Generic: schema.Generic{
														Type: "object",
													},
													Properties: map[string]schema.Structural{
														"id": {
															Generic: schema.Generic{
																Type: "string",
															},
														},
														"status": {
															Generic: schema.Generic{
																Type: "string",
															},
														},
														"metrics": {
															Generic: schema.Generic{
																Type: "object",
															},
															Properties: map[string]schema.Structural{
																"cpu": {
																	Generic: schema.Generic{
																		Type: "integer",
																	},
																},
																"memory": {
																	Generic: schema.Generic{
																		Type: "integer",
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func Benchmark_CEL_Evaluation(b *testing.B) {
	crdColumns := []apiextensionsv1.CustomResourceColumnDefinition{
		{
			Name:       "celHosts",
			Type:       "string",
			Expression: `self.spec.servers.map(s, s.hosts.filter(h, h == "prod.example.com"))`,
		},
	}
	s := validStructuralSchema()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.istio.io/v1alpha1",
			"kind":       "Blah",
			"metadata": map[string]interface{}{
				"name": "blah",
			},
			"spec": map[string]interface{}{
				"servers": []map[string]interface{}{
					{"hosts": []string{"prod.example.com", "foo"}},
					{"hosts": []string{"bar", "baz"}},
				},
			},
		},
	}

	ctx := context.Background()

	convertor, err := New(crdColumns, s)
	if err != nil {
		b.Fatalf("Unexpected error creating convertor: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = convertor.ConvertToTable(ctx, obj, nil)
		if err != nil {
			b.Fatalf("Unexpected error converting to table: %v", err)
		}
	}
}

func Benchmark_JSONPath_Evaluation(b *testing.B) {
	crdColumns := []apiextensionsv1.CustomResourceColumnDefinition{
		{
			Name:     "jsonPathHosts",
			Type:     "string",
			JSONPath: `.spec.servers[*].hosts[?(@ == "prod.example.com")]`,
		},
	}
	s := validStructuralSchema()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.istio.io/v1alpha1",
			"kind":       "Blah",
			"metadata": map[string]interface{}{
				"name": "blah",
			},
			"spec": map[string]interface{}{
				"servers": []map[string]interface{}{
					{"hosts": []string{"prod.example.com", "foo"}},
					{"hosts": []string{"bar", "baz"}},
				},
			},
		},
	}

	ctx := context.Background()

	convertor, err := New(crdColumns, s)
	if err != nil {
		b.Fatalf("Unexpected error creating convertor: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = convertor.ConvertToTable(ctx, obj, nil)
		if err != nil {
			b.Fatalf("Unexpected error converting to table: %v", err)
		}
	}
}

func Benchmark_convertor_ConvertToTable(b *testing.B) {
	c := &convertor{
		headers: []metav1.TableColumnDefinition{
			{Name: "name", Type: "string"},
			{Name: "valueOnly", Type: "string"},
			{Name: "single1", Type: "string"},
			{Name: "single2", Type: "string"},
			{Name: "multi", Type: "string"},
		},
		additionalColumns: []columnPrinter{
			newJSONPath("valueOnly", "{.spec.servers[0].hosts[0]}"),
			newJSONPath("single1", "{.spec.servers[0].hosts}"),
			newJSONPath("single2", "{.spec.servers[1].hosts}"),
			newJSONPath("multi", "{.spec.servers[*].hosts}"),
		},
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.istio.io/v1alpha1",
			"kind":       "Blah",
			"metadata": map[string]interface{}{
				"name": "blah",
			},
			"spec": map[string]interface{}{
				"servers": []map[string]interface{}{
					{"hosts": []string{"foo"}},
					{"hosts": []string{"bar", "baz"}},
				},
			},
		},
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.ConvertToTable(ctx, obj, nil)
	}
}

func Test_convertor_ConvertToTable(t *testing.T) {
	type fields struct {
		headers           []metav1.TableColumnDefinition
		additionalColumns []columnPrinter
	}
	type args struct {
		ctx          context.Context
		obj          runtime.Object
		tableOptions runtime.Object
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *metav1.Table
		wantErr bool
	}{
		{
			name: "Return table for object",
			fields: fields{
				headers: []metav1.TableColumnDefinition{{Name: "name", Type: "string"}},
			},
			args: args{
				obj: &metav1beta1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{Name: "blah", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))},
				},
				tableOptions: nil,
			},
			want: &metav1.Table{
				ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "name", Type: "string"}},
				Rows: []metav1.TableRow{
					{
						Cells: []interface{}{"blah"},
						Object: runtime.RawExtension{
							Object: &metav1beta1.PartialObjectMetadata{
								ObjectMeta: metav1.ObjectMeta{Name: "blah", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))},
							},
						},
					},
				},
			},
		},
		{
			name: "Return table for list",
			fields: fields{
				headers: []metav1.TableColumnDefinition{{Name: "name", Type: "string"}},
			},
			args: args{
				obj: &metav1beta1.PartialObjectMetadataList{
					Items: []metav1beta1.PartialObjectMetadata{
						{ObjectMeta: metav1.ObjectMeta{Name: "blah", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))}},
						{ObjectMeta: metav1.ObjectMeta{Name: "blah-2", CreationTimestamp: metav1.NewTime(time.Unix(2, 0))}},
					},
				},
				tableOptions: nil,
			},
			want: &metav1.Table{
				ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "name", Type: "string"}},
				Rows: []metav1.TableRow{
					{
						Cells: []interface{}{"blah"},
						Object: runtime.RawExtension{
							Object: &metav1beta1.PartialObjectMetadata{
								ObjectMeta: metav1.ObjectMeta{Name: "blah", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))},
							},
						},
					},
					{
						Cells: []interface{}{"blah-2"},
						Object: runtime.RawExtension{
							Object: &metav1beta1.PartialObjectMetadata{
								ObjectMeta: metav1.ObjectMeta{Name: "blah-2", CreationTimestamp: metav1.NewTime(time.Unix(2, 0))},
							},
						},
					},
				},
			},
		},
		{
			name: "Accept TableOptions",
			fields: fields{
				headers: []metav1.TableColumnDefinition{{Name: "name", Type: "string"}},
			},
			args: args{
				obj: &metav1beta1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{Name: "blah", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))},
				},
				tableOptions: &metav1.TableOptions{},
			},
			want: &metav1.Table{
				ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "name", Type: "string"}},
				Rows: []metav1.TableRow{
					{
						Cells: []interface{}{"blah"},
						Object: runtime.RawExtension{
							Object: &metav1beta1.PartialObjectMetadata{
								ObjectMeta: metav1.ObjectMeta{Name: "blah", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))},
							},
						},
					},
				},
			},
		},
		{
			name: "Omit headers from TableOptions",
			fields: fields{
				headers: []metav1.TableColumnDefinition{{Name: "name", Type: "string"}},
			},
			args: args{
				obj: &metav1beta1.PartialObjectMetadata{
					ObjectMeta: metav1.ObjectMeta{Name: "blah", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))},
				},
				tableOptions: &metav1.TableOptions{NoHeaders: true},
			},
			want: &metav1.Table{
				Rows: []metav1.TableRow{
					{
						Cells: []interface{}{"blah"},
						Object: runtime.RawExtension{
							Object: &metav1beta1.PartialObjectMetadata{
								ObjectMeta: metav1.ObjectMeta{Name: "blah", CreationTimestamp: metav1.NewTime(time.Unix(1, 0))},
							},
						},
					},
				},
			},
		},
		{
			name: "Return table with additional column containing multiple string values",
			fields: fields{
				headers: []metav1.TableColumnDefinition{
					{Name: "name", Type: "string"},
					{Name: "valueOnly", Type: "string"},
					{Name: "single1", Type: "string"},
					{Name: "single2", Type: "string"},
					{Name: "multi", Type: "string"},
				},
				additionalColumns: []columnPrinter{
					newJSONPath("valueOnly", "{.spec.servers[0].hosts[0]}"),
					newJSONPath("single1", "{.spec.servers[0].hosts}"),
					newJSONPath("single2", "{.spec.servers[1].hosts}"),
					newJSONPath("multi", "{.spec.servers[*].hosts}"),
				},
			},
			args: args{
				obj: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "example.istio.io/v1alpha1",
						"kind":       "Blah",
						"metadata": map[string]interface{}{
							"name": "blah",
						},
						"spec": map[string]interface{}{
							"servers": []map[string]interface{}{
								{"hosts": []string{"foo"}},
								{"hosts": []string{"bar", "baz"}},
							},
						},
					},
				},
				tableOptions: nil,
			},
			want: &metav1.Table{
				ColumnDefinitions: []metav1.TableColumnDefinition{
					{Name: "name", Type: "string"},
					{Name: "valueOnly", Type: "string"},
					{Name: "single1", Type: "string"},
					{Name: "single2", Type: "string"},
					{Name: "multi", Type: "string"},
				},
				Rows: []metav1.TableRow{
					{
						Cells: []interface{}{
							"blah",
							"foo",
							`["foo"]`,
							`["bar","baz"]`,
							`["foo"]`, // TODO: TableConverter should be changed so that the response is this: `["foo"] ["bar","baz"]`,
						},
						Object: runtime.RawExtension{
							Object: &unstructured.Unstructured{
								Object: map[string]interface{}{
									"apiVersion": "example.istio.io/v1alpha1",
									"kind":       "Blah",
									"metadata": map[string]interface{}{
										"name": "blah",
									},
									"spec": map[string]interface{}{
										"servers": []map[string]interface{}{
											{"hosts": []string{"foo"}},
											{"hosts": []string{"bar", "baz"}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Return table with additional column containing multiple integer values as string",
			fields: fields{
				headers: []metav1.TableColumnDefinition{
					{Name: "name", Type: "string"},
					{Name: "valueOnly", Type: "string"},
					{Name: "single1", Type: "string"},
					{Name: "single2", Type: "string"},
					{Name: "multi", Type: "string"},
				},
				additionalColumns: []columnPrinter{
					newJSONPath("valueOnly", "{.spec.foo[0].bar[0]}"),
					newJSONPath("single1", "{.spec.foo[0].bar}"),
					newJSONPath("single2", "{.spec.foo[1].bar}"),
					newJSONPath("multi", "{.spec.foo[*].bar}"),
				},
			},
			args: args{
				obj: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "example.istio.io/v1alpha1",
						"kind":       "Blah",
						"metadata": map[string]interface{}{
							"name": "blah",
						},
						"spec": map[string]interface{}{
							"foo": []map[string]interface{}{
								{"bar": []int64{1}},
								{"bar": []int64{2, 3}},
							},
						},
					},
				},
				tableOptions: nil,
			},
			want: &metav1.Table{
				ColumnDefinitions: []metav1.TableColumnDefinition{
					{Name: "name", Type: "string"},
					{Name: "valueOnly", Type: "string"},
					{Name: "single1", Type: "string"},
					{Name: "single2", Type: "string"},
					{Name: "multi", Type: "string"},
				},
				Rows: []metav1.TableRow{
					{
						Cells: []interface{}{
							"blah",
							"1",
							"[1]",
							"[2,3]",
							"[1]", // TODO: TableConverter should be changed so that the response is this: `[1] [2,3]`,
						},
						Object: runtime.RawExtension{
							Object: &unstructured.Unstructured{
								Object: map[string]interface{}{
									"apiVersion": "example.istio.io/v1alpha1",
									"kind":       "Blah",
									"metadata": map[string]interface{}{
										"name": "blah",
									},
									"spec": map[string]interface{}{
										"foo": []map[string]interface{}{
											{"bar": []int64{1}},
											{"bar": []int64{2, 3}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Return table with additional column containing multiple integer values",
			fields: fields{
				headers: []metav1.TableColumnDefinition{
					{Name: "name", Type: "string"},
					{Name: "valueOnly", Type: "integer"},
					{Name: "single1", Type: "integer"},
					{Name: "single2", Type: "integer"},
					{Name: "multi", Type: "integer"},
				},
				additionalColumns: []columnPrinter{
					newJSONPath("valueOnly", "{.spec.foo[0].bar[0]}"),
					newJSONPath("single1", "{.spec.foo[0].bar}"),
					newJSONPath("single2", "{.spec.foo[1].bar}"),
					newJSONPath("multi", "{.spec.foo[*].bar}"),
				},
			},
			args: args{
				obj: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "example.istio.io/v1alpha1",
						"kind":       "Blah",
						"metadata": map[string]interface{}{
							"name": "blah",
						},
						"spec": map[string]interface{}{
							"foo": []map[string]interface{}{
								{"bar": []int64{1}},
								{"bar": []int64{2, 3}},
							},
						},
					},
				},
				tableOptions: nil,
			},
			want: &metav1.Table{
				ColumnDefinitions: []metav1.TableColumnDefinition{
					{Name: "name", Type: "string"},
					{Name: "valueOnly", Type: "integer"},
					{Name: "single1", Type: "integer"},
					{Name: "single2", Type: "integer"},
					{Name: "multi", Type: "integer"},
				},
				Rows: []metav1.TableRow{
					{
						Cells: []interface{}{
							"blah",
							int64(1),
							nil, // TODO: Seems like this should either return some data or return an error, not just be nil
							nil, // TODO: Seems like this should either return some data or return an error, not just be nil
							nil, // TODO: Seems like this should either return some data or return an error, not just be nil
						},
						Object: runtime.RawExtension{
							Object: &unstructured.Unstructured{
								Object: map[string]interface{}{
									"apiVersion": "example.istio.io/v1alpha1",
									"kind":       "Blah",
									"metadata": map[string]interface{}{
										"name": "blah",
									},
									"spec": map[string]interface{}{
										"foo": []map[string]interface{}{
											{"bar": []int64{1}},
											{"bar": []int64{2, 3}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &convertor{
				headers:           tt.fields.headers,
				additionalColumns: tt.fields.additionalColumns,
			}
			got, err := c.ConvertToTable(tt.args.ctx, tt.args.obj, tt.args.tableOptions)
			if (err != nil) != tt.wantErr {
				t.Errorf("convertor.ConvertToTable() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertor.ConvertToTable() = %s", cmp.Diff(tt.want, got))
			}
		})
	}
}

func validStructuralSchema() *schema.Structural {
	return &schema.Structural{
		Generic: schema.Generic{
			Type: "object",
		},
		Properties: map[string]schema.Structural{
			"spec": {
				Generic: schema.Generic{
					Type: "object",
				},
				Properties: map[string]schema.Structural{
					"servers": {
						Generic: schema.Generic{
							Type: "array",
						},
						Items: &schema.Structural{
							Generic: schema.Generic{
								Type: "object",
							},
							Properties: map[string]schema.Structural{
								"hosts": {
									Generic: schema.Generic{
										Type: "array",
									},
									Items: &schema.Structural{
										Generic: schema.Generic{
											Type: "string",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newJSONPath(name string, jsonPathExpression string) columnPrinter {
	jp := jsonpath.New(name)
	_ = jp.Parse(jsonPathExpression)
	return jp
}
