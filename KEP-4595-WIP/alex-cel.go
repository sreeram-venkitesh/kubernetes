package alexcel

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/google/cel-go/cel"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/apiserver/pkg/cel/environment"
	"k8s.io/apiserver/pkg/cel/library"
)

type ColumnCompilationResult struct {
	Error          error
	MaxCost        uint64
	MaxCardinality uint64
	FieldPath      *field.Path
	Program        cel.Program
}

// CEL Cost Levers:
// Per Call Limit: The maximum cost that can be incurred by a single call to a CEL program.
// Max Budget: The maximum cost that can be incurred by all calls to CEL programs in a single request.

func CompileColumns(exprs []string, s *schema.Structural, declType *apiservercel.DeclType, perCallLimit uint64, baseEnvSet *environment.EnvSet, envLoader EnvLoader) ([]ColumnCompilationResult, error) {
	oldSelfEnvSet, _, err := prepareEnvSet(baseEnvSet, declType)
	if err != nil {
		return nil, err
	}
	estimator := newCostEstimator(declType)
	// compResults is the return value which saves a list of compilation results in the same order as x-kubernetes-validations rules.
	compResults := make([]ColumnCompilationResult, len(exprs))
	maxCardinality := maxCardinality(declType.MinSerializedSize)
	for i, rule := range exprs {
		ruleEnvSet := oldSelfEnvSet
		compResults[i] = compileColumn(s, rule, ruleEnvSet, envLoader, estimator, maxCardinality, perCallLimit)
	}

	return compResults, nil
}

func compileColumn(s *schema.Structural, rule string, envSet *environment.EnvSet, envLoader EnvLoader, estimator *library.CostEstimator, maxCardinality uint64, perCallLimit uint64) (compilationResult ColumnCompilationResult) {
	if len(strings.TrimSpace(rule)) == 0 {
		// include a compilation result, but leave both program and error nil per documented return semantics of this
		// function
		return
	}
	ruleEnv := envLoader.RuleEnv(envSet, rule)
	ast, issues := ruleEnv.Compile(rule)
	if issues != nil {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInvalid, Detail: "compilation failed: " + issues.String()}
		return
	}
	if ast.OutputType() != cel.BoolType {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInvalid, Detail: "cel expression must evaluate to a bool"}
		return
	}

	_, err := cel.AstToCheckedExpr(ast)
	if err != nil {
		// should be impossible since env.Compile returned no issues
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInternal, Detail: "unexpected compilation error: " + err.Error()}
		return
	}

	// TODO: Ideally we could configure the per expression limit at validation time and set it to the remaining overall budget, but we would either need a way to pass in a limit at evaluation time or move program creation to validation time
	prog, err := ruleEnv.Program(ast,
		cel.CostLimit(perCallLimit),
		cel.CostTracking(estimator),
		cel.InterruptCheckFrequency(celconfig.CheckFrequency),
	)
	if err != nil {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInvalid, Detail: "program instantiation failed: " + err.Error()}
		return
	}
	costEst, err := ruleEnv.EstimateCost(ast, estimator)
	if err != nil {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInternal, Detail: "cost estimation failed: " + err.Error()}
		return
	}
	compilationResult.MaxCost = costEst.Max
	compilationResult.MaxCardinality = maxCardinality
	compilationResult.Program = prog
}

func PrintColumns(compResults []ColumnCompilationResult, sts *schema.Structural, obj interface{}, remainingBudget int64) (field.ErrorList, []string) {
	var exprs []string
	activation, _ := validationActivationWithoutOldSelf(sts, obj, nil)
	var errs []*field.Error

	for _, compResult := range compResults {
		evalResult, evalDetails, err := compResult.Program.ContextEval(context.TODO(), activation)
		if evalDetails == nil {
			errs = append(errs, field.InternalError(compResult.FieldPath, fmt.Errorf("runtime cost could not be calculated, no further validation rules will be run")))
			return errs, nil
		} else {
			rtCost := evalDetails.ActualCost()
			if rtCost == nil {
				errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("runtime cost could not be calculated, no further validation rules will be run")))
				return errs, nil
			} else {
				if *rtCost > math.MaxInt64 || int64(*rtCost) > remainingBudget {
					errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("validation failed due to running out of cost budget, no further validation rules will be run")))
					return errs, nil
				}
				remainingBudget -= int64(*rtCost)
			}
		}
		if err != nil {
			// see types.Err for list of well defined error types
			if strings.HasPrefix(err.Error(), "no such overload") {
				// Most overload errors are caught by the compiler, which provides details on where exactly in the rule
				// error was found. Here, an overload error has occurred at runtime no details are provided, so we
				// append a more descriptive error message. This error can only occur when static type checking has
				// been bypassed. int-or-string is typed as dynamic and so bypasses compiler type checking.
				errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("'%v': call arguments did not match a supported operator, function or macro signature", err)))
			} else if strings.HasPrefix(err.Error(), "operation cancelled: actual cost limit exceeded") {
				errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("'%v': no further validation rules will be run due to call cost exceeds limit", err)))
				return errs, nil
			} else {
				// no such key: {key}, index out of bounds: {index}, integer overflow, division by zero, ...
				errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("%v", err)))
			}
			exprs = append(exprs, "")
			continue
		}

		str := evalResult.Value().(string)
		exprs = append(exprs, str)
	}
	return errs, exprs
}