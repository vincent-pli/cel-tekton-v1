/*
Copyright 2019 The Knative Authors

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

package v1alpha1

import (
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"

	// duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/kmeta"
)

// VariableStore is a context or variables storage to help caculate the CEL expression.
//
// +genclient
// +genreconciler:krshapedlogic=false
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VariableStore struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec holds the desired state of the VariableStore (from the client).
	// +optional
	Spec VariableStoreSpec `json:"spec,omitempty"`
}

var (
	// Check that VariableStore can be validated and defaulted.
	_ apis.Validatable   = (*VariableStore)(nil)
	_ apis.Defaultable   = (*VariableStore)(nil)
	_ kmeta.OwnerRefable = (*VariableStore)(nil)
)

// VariableStoreRunReason represents a reason for the Run "Succeeded" condition
type VariableStoreRunReason string

const (
	// ReasonCouldntGet indicates that the associated VariableStore couldn't be retrieved
	ReasonCouldntGet VariableStoreRunReason = "CouldntGet"

	// ReasonNoParaminRun indicates that the declared Param cound not find in Run
	ReasonNoParaminRun VariableStoreRunReason = "CoundntGetParamValue"

	// ReasonCoundntGetOriginalVariables indicates that variables cound not be query from Redis
	ReasonCoundntGetOriginalVariables VariableStoreRunReason = "CoundntGetOriginalVariables"

	// ReasonCoundntSaveOriginalVariables indicates that variables cound not be save to Redis
	ReasonCoundntSaveOriginalVariables VariableStoreRunReason = "CoundntSaveOriginalVariables"

	// ReasonCoundntExtendEnv indicates that variables cound not be added to env
	ReasonCoundntExtendEnv VariableStoreRunReason = "CoundntExtendEnv"

	// ReasonSyntaxError indicates that the reason for failure status is that a CEL expression couldn't be parsed
	ReasonSyntaxError VariableStoreRunReason = "SyntaxError"

	// ReasonEvaluationError indicates that the reason for failure status is that a CEL expression couldn't be evaluated
	// typically due to evaluation environment or executable program
	ReasonEvaluationError VariableStoreRunReason = "EvaluationError"

	// ReasonEvaluationSuccess indicates that the reason for the success status is that all CEL expressions were
	// evaluated successfully and the results were produced
	ReasonEvaluationSuccess VariableStoreRunReason = "EvaluationSuccess"
)

func (e VariableStoreRunReason) String() string {
	return string(e)
}

// VariableStoreSpec holds the desired state of the VariableStore (from the client).
type VariableStoreSpec struct {
	Params []v1beta1.ParamSpec `json:"params,omitempty"`
	// Vars holds the predefined variables and these variabls will be the context for next caculation.
	Vars []v1beta1.Param `json:"vars,omitempty"`
	// Vars holds the predefined variables and these variabls will be the context for next caculation.
	Results []v1beta1.Param `json:"results,omitempty"`
}

const (
	// AddressableServiceConditionReady is set when the revision is starting to materialize
	// runtime resources, and becomes true when those resources are ready.
	AddressableServiceConditionReady = apis.ConditionReady
)

// AddressableServiceList is a list of AddressableService resources
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type VariableStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VariableStore `json:"items"`
}
