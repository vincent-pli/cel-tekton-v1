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

package variablestore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1"
	"github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	runreconciler "github.com/tektoncd/pipeline/pkg/client/injection/reconciler/pipeline/v1alpha1/run"
	listersalpha "github.com/tektoncd/pipeline/pkg/client/listers/pipeline/v1alpha1"
	variablestorev1alpha1 "github.com/vincentpli/cel-tekton/pkg/apis/variablestores/v1alpha1"
	variableclientset "github.com/vincentpli/cel-tekton/pkg/client/clientset/versioned"
	"go.uber.org/zap"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/reconciler"
	"knative.dev/pkg/tracker"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/ext"

	"github.com/go-redis/redis/v8"
	"github.com/tektoncd/pipeline/pkg/reconciler/events"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reconciler implements addressableservicereconciler.Interface for
// AddressableService resources.
type Reconciler struct {
	// Tracker builds an index of what resources are watching other resources
	// so that we can immediately react to changes tracked resources.
	Tracker tracker.Interface

	//Clientset about resources
	variablestoreClientSet variableclientset.Interface

	// Listers index properties about resources
	runLister listersalpha.RunLister

	// rdb index properties about resources
	rdb *redis.Client
}

const (
	PrefixParam = "params"
	PrefixVar   = "vars"
)

// Check that our Reconciler implements Interface
var _ runreconciler.Interface = (*Reconciler)(nil)

var redisClient *redis.Client

func init() {
}

// ReconcileKind implements Interface.ReconcileKind.
func (r *Reconciler) ReconcileKind(ctx context.Context, run *v1alpha1.Run) reconciler.Event {
	var merr error
	logger := logging.FromContext(ctx)
	logger.Infof("Reconciling Run %s/%s at %v", run.Namespace, run.Name, time.Now())

	// Check that the Run references a VariableStore CRD.  The logic is controller.go should ensure that only this type of Run
	// is reconciled this controller but it never hurts to do some bullet-proofing.
	if run.Spec.Ref == nil ||
		run.Spec.Ref.APIVersion != variablestorev1alpha1.SchemeGroupVersion.String() ||
		run.Spec.Ref.Kind != "VariableStore" {
		logger.Errorf("Received control for a Run %s/%s that does not reference a VariableStore custom CRD", run.Namespace, run.Name)
		return nil
	}

	// If the Run has not started, initialize the Condition and set the start time.
	if !run.HasStarted() {
		logger.Infof("Starting new Run %s/%s", run.Namespace, run.Name)
		run.Status.InitializeConditions()
		// In case node time was not synchronized, when controller has been scheduled to other nodes.
		if run.Status.StartTime.Sub(run.CreationTimestamp.Time) < 0 {
			logger.Warnf("Run %s createTimestamp %s is after the Run started %s", run.Name, run.CreationTimestamp, run.Status.StartTime)
			run.Status.StartTime = &run.CreationTimestamp
		}

		// Emit events. During the first reconcile the status of the Run may change twice
		// from not Started to Started and then to Running, so we need to sent the event here
		// and at the end of 'Reconcile' again.
		// We also want to send the "Started" event as soon as possible for anyone who may be waiting
		// on the event to perform user facing initialisations, such has reset a CI check status
		afterCondition := run.Status.GetCondition(apis.ConditionSucceeded)
		events.Emit(ctx, nil, afterCondition, run)
	}

	if run.IsDone() {
		logger.Infof("Run %s/%s is done", run.Namespace, run.Name)
		return nil
	}

	// Store the condition before reconcile
	beforeCondition := run.Status.GetCondition(apis.ConditionSucceeded)

	// Reconcile the Run
	if err := r.reconcile(ctx, run); err != nil {
		logger.Errorf("Reconcile error: %v", err.Error())
		merr = multierror.Append(merr, err)
	}

	afterCondition := run.Status.GetCondition(apis.ConditionSucceeded)
	events.Emit(ctx, beforeCondition, afterCondition, run)

	// Only transient errors that should retry the reconcile are returned.
	return merr
}

func (r *Reconciler) reconcile(ctx context.Context, run *v1alpha1.Run) error {
	logger := logging.FromContext(ctx)
	variablestore, err := r.getVariableStore(ctx, run)
	if err != nil {
		logger.Errorf("Error retrieving VariableStore for Run %s/%s: %s", run.Namespace, run.Name, err)
		run.Status.MarkRunFailed(variablestorev1alpha1.ReasonCouldntGet.String(),
			"Error retrieving VariableStore for Run %s/%s: %s",
			run.Namespace, run.Name, err)
		return nil
	}

	// Create a program environment configured with the standard library of CEL functions and macros
	env, err := cel.NewEnv(cel.Declarations(), ext.Strings(), ext.Encoders())
	if err != nil {
		logger.Errorf("Couldn't create a program env with standard library of CEL functions & macros when reconciling Run %s/%s: %v", run.Namespace, run.Name, err)
		return err
	}

	var runResults []v1alpha1.RunResult
	contextExpressions := map[string]interface{}{}

	// All variables declared in VariableStore.params will be the add to env
	var params = map[string]interface{}{}
	for _, paramSpec := range variablestore.Spec.Params {
		param := getParam(paramSpec.Name, run.Spec.Params)

		if param == nil {
			logger.Errorf("The param: %s defined in VariableStore: %s/%s is not in Run: %s/%s", paramSpec.Name, variablestore.Namespace, variablestore.Name, run.Name, run.Namespace)
			run.Status.MarkRunFailed(variablestorev1alpha1.ReasonNoParaminRun.String(),
				"The param: %s defined in VariableStore: %s/%s is not in Run: %s/%s", paramSpec.Name, variablestore.Namespace, variablestore.Name, run.Name, run.Namespace)
			return nil
		}

		params[nameConvert(param.Name)] = param.Value.StringVal
	}

	if len(params) != 0 {
		contextExpressions[PrefixParam] = params
		env, err = env.Extend(cel.Declarations(decls.NewVar(PrefixParam, decls.NewMapType(decls.String, decls.Dyn))))
		if err != nil {
			logger.Errorf("CEL expression %s could not be add to context env when reconciling Run %s/%s: %v", PrefixParam, run.Namespace, run.Name, err)
			run.Status.MarkRunFailed(variablestorev1alpha1.ReasonCoundntExtendEnv.String(),
				"CEL expression %s could not be add to context env", PrefixParam, err)
			return nil
		}
	}

	// Get original variables stored in Redis  TODO
	redisKey = run.ObjectMeta.Labels["tekton.dev/pipelineRun"]
	originalVars, err := r.getVariables(redisKey, logger)
	if err != nil {
		logger.Errorf("Get original variables for VariableStore: %s/%s hit error: %v", variablestore.Name, variablestore.Namespace, err)
		run.Status.MarkRunFailed(variablestorev1alpha1.ReasonCoundntGetOriginalVariables.String(),
			"Get original variables for VariableStore: %s/%s hit error: %v", variablestore.Name, variablestore.Namespace, err)
		return nil
	}

	var vars = map[string]interface{}{}
	for key, value := range originalVars {
		vars[nameConvert(key)] = value
	}
	contextExpressions[PrefixVar] = vars

	env, err = env.Extend(cel.Declarations(decls.NewVar(PrefixVar, decls.NewMapType(decls.String, decls.Dyn))))
	if err != nil {
		logger.Errorf("CEL expression %s could not be add to context env when reconciling Run %s/%s: %v", PrefixVar, run.Namespace, run.Name, err)
		run.Status.MarkRunFailed(variablestorev1alpha1.ReasonCoundntExtendEnv.String(),
			"CEL expression %s could not be add to context env", PrefixVar, err)
		return nil
	}

	for _, variable := range variablestore.Spec.Vars {
		// Combine the Parse and Check phases CEL program compilation to produce an Ast and associated issues
		ast, iss := env.Compile(variable.Value.StringVal)
		if iss.Err() != nil {
			logger.Errorf("CEL expression %s could not be parsed when reconciling Run %s/%s: %v", variable.Name, run.Namespace, run.Name, iss.Err())
			run.Status.MarkRunFailed(variablestorev1alpha1.ReasonSyntaxError.String(),
				"CEL expression %s could not be parsed", variable.Name, iss.Err())
			return nil
		}

		// Generate an evaluable instance of the Ast within the environment
		prg, err := env.Program(ast)
		if err != nil {
			logger.Errorf("CEL expression %s could not be evaluated when reconciling Run %s/%s: %v", variable.Name, run.Namespace, run.Name, err)
			run.Status.MarkRunFailed(variablestorev1alpha1.ReasonEvaluationError.String(),
				"CEL expression %s could not be evaluated", variable.Name, err)
			return nil
		}

		// Evaluate the CEL expression (Ast)
		out, _, err := prg.Eval(contextExpressions)
		if err != nil {
			logger.Errorf("CEL expression %s could not be evaluated when reconciling Run %s/%s: %v", variable.Name, run.Namespace, run.Name, err)
			run.Status.MarkRunFailed(variablestorev1alpha1.ReasonEvaluationError.String(),
				"CEL expression %s could not be evaluated", variable.Name, err)
			return nil
		}

		// Evaluation of CEL expression was successful
		logger.Infof("CEL expression %s evaluated successfully when reconciling Run %s/%s", variable.Name, run.Namespace, run.Name)

		varMap, ok := contextExpressions[PrefixVar].(map[string]interface{})
		if ok {
			varMap[nameConvert(variable.Name)] = fmt.Sprintf("%s", out.ConvertToType(types.StringType).Value())
		}
	}

	// Handle Result of Run
	for _, result := range variablestore.Spec.Results {
		// Combine the Parse and Check phases CEL program compilation to produce an Ast and associated issues
		ast, iss := env.Compile(result.Value.StringVal)
		if iss.Err() != nil {
			logger.Errorf("CEL expression %s could not be parsed when reconciling Run %s/%s: %v", result.Name, run.Namespace, run.Name, iss.Err())
			run.Status.MarkRunFailed(variablestorev1alpha1.ReasonSyntaxError.String(),
				"CEL expression %s could not be parsed", result.Name, iss.Err())
			return nil
		}

		// Generate an evaluable instance of the Ast within the environment
		prg, err := env.Program(ast)
		if err != nil {
			logger.Errorf("CEL expression %s could not be evaluated when reconciling Run %s/%s: %v", result.Name, run.Namespace, run.Name, err)
			run.Status.MarkRunFailed(variablestorev1alpha1.ReasonEvaluationError.String(),
				"CEL expression %s could not be evaluated", result.Name, err)
			return nil
		}

		// Evaluate the CEL expression (Ast)
		out, _, err := prg.Eval(contextExpressions)
		if err != nil {
			logger.Errorf("CEL expression %s could not be evaluated when reconciling Run %s/%s: %v", result.Name, run.Namespace, run.Name, err)
			run.Status.MarkRunFailed(variablestorev1alpha1.ReasonEvaluationError.String(),
				"CEL expression %s could not be evaluated", result.Name, err)
			return nil
		}

		// Evaluation of CEL expression was successful
		logger.Infof("CEL expression %s evaluated successfully when reconciling Run %s/%s", result.Name, run.Namespace, run.Name)

		runResults = append(runResults, v1alpha1.RunResult{
			Name:  result.Name,
			Value: fmt.Sprintf("%s", out.ConvertToType(types.StringType).Value()),
		})

	}

	//need record all item in vars, since need to store them to Redis TODO
	err = r.saveVariables(redisKey, contextExpressions[PrefixParam])
	if err != nil {
		logger.Errorf("Save variables to Redis hit exception when reconciling Run %s/%s: %v", run.Namespace, run.Name, err)
		run.Status.MarkRunFailed(variablestorev1alpha1.ReasonCoundntSaveOriginalVariables.String(),
			"Save variables to Redis hit exception when reconciling Run %s/%s: %v", run.Namespace, run.Name, err)
		return nil
	}

	run.Status.Results = append(run.Status.Results, runResults...)
	run.Status.MarkRunSucceeded(variablestorev1alpha1.ReasonEvaluationSuccess.String(),
		"CEL expressions were evaluated successfully")

	return nil
}

func (r *Reconciler) getVariableStore(ctx context.Context, run *v1alpha1.Run) (*variablestorev1alpha1.VariableStore, error) {
	var variablestore *variablestorev1alpha1.VariableStore

	if run.Spec.Ref != nil && run.Spec.Ref.Name != "" {
		// Use the k8 client to get the VariableStore rather than the lister.  This avoids a timing issue where
		// the VariableStore is not yet in the lister cache if it is created at nearly the same time as the Run.
		// See https://github.com/tektoncd/pipeline/issues/2740 for discussion on this issue.
		vs, err := r.variablestoreClientSet.CustomV1alpha1().VariableStores(run.Namespace).Get(ctx, run.Spec.Ref.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		variablestore = vs
	} else {
		return nil, fmt.Errorf("Missing spec.ref.name for Run %s", fmt.Sprintf("%s/%s", run.Namespace, run.Name))
	}

	return variablestore, nil
}

func getParam(paramName string, params []v1beta1.Param) *v1beta1.Param {
	for _, param := range params {
		if paramName == param.Name {
			return &param
		}
	}

	return nil
}

func nameConvert(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

// Fake TODO
func (r *Reconciler) getVariables(key string, logger *zap.SugaredLogger) (map[string]string, error) {
	variables := map[string]string{}
	val, err := r.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		logger.Infof("There is no variables in redis for pr: %s/%s", run.Namespace, key)
		return variables
	} else if err != nil {
		return nil, err
	}

	err = json.Unmarshal([]byte(val), &variables)
	if err != nil {
		return nil, err
	}

	return variables
	// return map[string]string{
	// 	"job_severity": "Sev-1",
	// }, nil
}

// Fake TODO
func (r *Reconciler) saveVariables(key string, vars interface{}) error {
	varMap, ok := var.(map[string]string{})
	if !ok {
		return fmt.Errorf("Variables is not a map[string]string")
	}
	variables, err := json.Marshal(varMap)
	if err != nil {
		return err
	}

    err := r.rdb.Set(context.Background(), key, variables, 0).Err()
    if err != nil {
        return err
    }
	
	return nil
}
