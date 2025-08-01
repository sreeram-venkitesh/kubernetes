/*
Copyright 2025 The Kubernetes Authors.

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

package zeroroneooftypedef

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

func Test(t *testing.T) {
	st := localSchemeBuilder.Test(t)

	st.Value(&Struct{
		Tasks: TaskList{
			{Name: "other", State: "Other"},
		},
	}).ExpectValid()

	st.Value(&Struct{
		Tasks: TaskList{
			{Name: "succeeded", State: "Succeeded"},
			{Name: "other", State: "Other"},
		},
	}).ExpectValid()

	st.Value(&Struct{
		Tasks: TaskList{},
	}).ExpectValid()

	invalidBothSet := &Struct{
		Tasks: TaskList{
			{Name: "succeeded", State: "Succeeded"},
			{Name: "failed", State: "Failed"},
		},
	}

	st.Value(invalidBothSet).ExpectMatches(
		field.ErrorMatcher{}.ByType().ByField().ByOrigin(),
		field.ErrorList{
			field.Invalid(field.NewPath("tasks"), nil, "").WithOrigin("zeroOrOneOf"),
		},
	)

	// Test ratcheting.
	st.Value(invalidBothSet).OldValue(invalidBothSet).ExpectValid()
}
