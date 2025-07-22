package cel

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	apiservercel "k8s.io/apiserver/pkg/cel"
	"k8s.io/apiserver/pkg/cel/environment"
	"k8s.io/apiserver/pkg/cel/library"

	"github.com/google/cel-go/common/types/ref"
	"k8s.io/klog/v2"

	"github.com/google/cel-go/cel"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	celconfig "k8s.io/apiserver/pkg/apis/cel"
)

type ColumnCompilationResult struct {
	Error          error
	MaxCost        uint64
	MaxCardinality uint64
	FieldPath      *field.Path
	Program        cel.Program
}

func (c ColumnCompilationResult) FindResults(data interface{}) ([][]reflect.Value, error) {
	klog.V(1).Info("Inside FindResults of cel")

	// activation, _ := validationActivationWithoutOldSelf(data.(*schema.Structural), nil, nil)
	klog.V(1).Info(data)
	vars := map[string]interface{}{
		"self": data,
	}

	evalResult, det, err := c.Program.Eval(vars)

	if err != nil {
		klog.V(1).Info("Error happened inside FindResults evaluation")
		klog.V(1).Info(det)
		klog.V(1).Info(err)
	}

	reflectSlice := [][]reflect.Value{
		{reflect.ValueOf(evalResult)},
	}

	klog.V(1).Info("Printing reflectSlice")
	klog.V(1).Info(reflectSlice)
	return reflectSlice, nil
}

func (c ColumnCompilationResult) PrintResults(w io.Writer, results []reflect.Value) error {

	klog.V(1).Info("Inside cel PrintResults")

	// Iterate over the reflect.Values in the results slice
	for _, result := range results {
		// Convert the reflect.Value into a string
		klog.V(1).Info("Inside FindResults for loop")
		klog.V(1).Info(result)
		var str string
		switch result.Kind() {
		case reflect.String:
			str = result.String() // If it's a string, just use it
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			str = fmt.Sprintf("%d", result.Int()) // If it's an integer, convert to string
		case reflect.Float32, reflect.Float64:
			str = fmt.Sprintf("%f", result.Float()) // If it's a float, convert to string
		case reflect.Bool:
			str = fmt.Sprintf("%v", result.Bool()) // If it's a bool, convert to string
		default:
			str = fmt.Sprintf("%v", result.Interface()) // Use the default string representation for other types
		}

		// Convert the string to a byte slice
		_, err := w.Write([]byte(str))
		if err != nil {
			klog.V(1).Info("Error inside cel printresults")
			klog.V(1).Info(err)
			return err // Return the error if the write failed
		}
	}

	// No error, return nil
	return nil
}

func Eval(prg cel.Program,
	vars any) (out ref.Val, det *cel.EvalDetails, err error) {
	varMap, isMap := vars.(map[string]any)
	fmt.Println("------ input ------")
	if !isMap {
		fmt.Printf("(%T)\n", vars)
	} else {
		for k, v := range varMap {
			switch val := v.(type) {
			case proto.Message:
				bytes, err := prototext.Marshal(val)
				if err != nil {
					klog.V(1).Infof("failed to marshal proto to text: %v", val)
				}
				fmt.Printf("%s = %s", k, string(bytes))
			case map[string]any:
				b, _ := json.MarshalIndent(v, "", "  ")
				fmt.Printf("%s = %v\n", k, string(b))
			case uint64:
				fmt.Printf("%s = %vu\n", k, v)
			default:
				fmt.Printf("%s = %v\n", k, v)
			}
		}
	}
	fmt.Println()
	out, det, err = prg.Eval(vars)
	return out, det, err
}

// CEL Cost Levers:
// Per Call Limit: The maximum cost that can be incurred by a single call to a CEL program.
// Max Budget: The maximum cost that can be incurred by all calls to CEL programs in a single request.

func CompileColumn(expr string, s *schema.Structural, declType *apiservercel.DeclType, perCallLimit uint64, baseEnvSet *environment.EnvSet, envLoader EnvLoader) ColumnCompilationResult {
	klog.V(1).Infof("expr: %v", expr)
	klog.V(1).Infof("schema: %v", s)
	klog.V(1).Infof("declType: %v", declType)
	klog.V(1).Infof("perCallLimit: %v", perCallLimit)
	klog.V(1).Infof("baseEnvSet: %v", baseEnvSet)
	klog.V(1).Infof("envLoader: %v", envLoader)

	// INFO (PS): oldSelfEnvSet might be required only in the context of CRD Validation Ratcheting
	// where we use both oldSelf and self to calculate the diff between the old crd and new crd to
	// decide what all fields we need to validate.
	// TODO: If needed go through the ratcheting KEP and its alpha PRs to confirm if oldSelfEnvSet logic was
	// added as part of the ratcheting KEP.

	// benchmarking starts here
	start := time.Now()
	oldSelfEnvSet, _, err := prepareEnvSet(baseEnvSet, declType)
	if err != nil {
		compResult := ColumnCompilationResult{Error: err}
		return compResult
	}
	estimator := newCostEstimator(declType)
	maxCardinality := maxCardinality(declType.MinSerializedSize)
	ruleEnvSet := oldSelfEnvSet
	duration := time.Since(start)

	klog.Infof("Time 1: Time taken for setting up env and cost estimator for %s: %s\n", expr, duration)
	// klog.V(1).Infof("\n\n ruleEnvSet: %v", ruleEnvSet)
	compResult := compileColumnExpression(s, expr, ruleEnvSet, envLoader, estimator, maxCardinality, perCallLimit)
	// klog.V(1).Infof("\n\n compResult: %v", compResult)
	if compResult.Error != nil {
		return compResult
	}
	return compResult
}

func compileColumnExpression(s *schema.Structural, rule string, envSet *environment.EnvSet, envLoader EnvLoader, estimator *library.CostEstimator, maxCardinality uint64, perCallLimit uint64) (compilationResult ColumnCompilationResult) {
	if len(strings.TrimSpace(rule)) == 0 {
		// include a compilation result, but leave both program and error nil per documented return semantics of this
		// function
		return
	}
	ruleEnv := envLoader.RuleEnv(envSet, rule)
	start := time.Now()
	ast, issues := ruleEnv.Compile(rule)
	duration := time.Since(start)
	klog.Infof("Time 2: Time taken for CEL compilation for %s: %s\n", rule, duration)

	if issues != nil {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInvalid, Detail: "compilation failed: " + issues.String()}
		return
	}

	// klog.V(1).Infof("PRINTING AST OUTPUT TYPE: %v", ast.OutputType())
	// klog.V(1).Infof("PRINTING CEL STRING TYPE: %v", cel.StringType)
	// klog.V(1).Infof("PRINTING CEL BOOL TYPE: %v", cel.BoolType)
	// klog.V(1).Infof("PRINTING CEL INT TYPE: %v", cel.IntType)
	// klog.V(1).Infof("PRINTING CEL INT TYPE: %v", cel.ListType(cel.IntType))

	// klog.V(1).Infof("ast.OutputType() == cel.StringType: %v", ast.OutputType().IsExactType(cel.StringType))
	// klog.V(1).Infof("ast.OutputType() == cel.ListType(cel.IntType): %v", ast.OutputType().IsExactType(cel.ListType(cel.IntType)))

	// start = time.Now()
	// klog.Infof("Printing ast.outputType %s", ast.OutputType())
	// klog.Infof("Printing unstructuredToVal %s", UnstructuredToVal(nil, s))

	// if ast.OutputType() != cel.StringType &&
	// 	ast.OutputType() != cel.BoolType &&
	// 	ast.OutputType() != cel.IntType &&
	// 	// ast.OutputType() != cel.DoubleType &&
	// 	// ast.OutputType() != cel.DurationType &&
	// 	//    ast.OutputType() != cel.ListType(cel.IntType) &&
	// 	!ast.OutputType().IsExactType(cel.ListType(cel.IntType)) &&
	// 	!ast.OutputType().IsExactType(cel.ListType(cel.StringType)) &&
	// 	// !ast.OutputType().IsExactType(cel.ListType(cel.DynType)) &&
	// 	// !ast.OutputType().IsExactType(cel.ListType(cel.MapType(cel.StringType, cel.StringType))) &&
	// 	// !ast.OutputType().IsExactType(cel.ListType()) &&
	// 	// !ast.OutputType().IsExactType(cel.ListType(cel.MapType(cel.StringType, cel.StringType))) &&
	// 	ast.OutputType() != cel.DynType {
	// 	// if ast.OutputType() != cel.DynType {
	// 	compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInvalid, Detail: "cel expression must evaluate to a valid type"}
	// 	return
	// }
	// duration = time.Since(start)
	// klog.Infof("Time3: Time taken for if chain in CEL compilation function for %s: %s\n", rule, duration)

	start = time.Now()
	_, err := cel.AstToCheckedExpr(ast)
	if err != nil {
		// should be impossible since env.Compile returned no issues
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInternal, Detail: "unexpected compilation error: " + err.Error()}
		return
	}
	duration = time.Since(start)
	klog.Infof("Time 3: Time taken for astToCheckedExpr for %s: %s\n", rule, duration)

	// TODO: Ideally we could configure the per expression limit at validation time and
	// set it to the remaining overall budget, but we would either need a way to pass in
	// a limit at evaluation time or move program creation to validation time
	start = time.Now()
	prog, err := ruleEnv.Program(ast,
		cel.CostLimit(perCallLimit),
		cel.CostTracking(estimator),
		cel.InterruptCheckFrequency(celconfig.CheckFrequency),
	)
	if err != nil {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInvalid, Detail: "program instantiation failed: " + err.Error()}
		return
	}
	duration = time.Since(start)
	klog.Infof("Time 4: Time taken for program generation for %s: %s\n", rule, duration)

	start = time.Now()
	costEst, err := ruleEnv.EstimateCost(ast, estimator)
	if err != nil {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInternal, Detail: "cost estimation failed: " + err.Error()}
		return
	}
	duration = time.Since(start)
	klog.Infof("Time 5: Time taken for cost estimation for %s: %s\n", rule, duration)

	compilationResult.MaxCost = costEst.Max
	compilationResult.MaxCardinality = maxCardinality
	compilationResult.Program = prog
	return compilationResult
}
