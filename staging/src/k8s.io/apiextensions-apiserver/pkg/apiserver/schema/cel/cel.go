package cel

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	// "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	// "k8s.io/apimachinery/pkg/util/validation/field"
	// celconfig "k8s.io/apiserver/pkg/apis/cel"
	// apiservercel "k8s.io/apiserver/pkg/cel"
	// "k8s.io/apiserver/pkg/cel/environment"
	// "k8s.io/apiserver/pkg/cel/library"
	// "k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	// structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"

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
	// Program        cel.Program
	Program *celProgram
}

type celProgram struct {
	Program cel.Program
}

func (c celProgram) FindResults(data interface{}) ([][]reflect.Value, error) {
	klog.V(1).Info("Inside FindResults of cel")
	// out, det, err := Eval(c.Program, cel.NoVars())

	// activationFactory, _ := validationActivationWithoutOldSelf(data.(*schema.Structural), nil, nil)
	klog.V(1).Info(data)
	vars := map[string]interface{}{
		"self": data,
	}

	evalResult, det, err := c.Program.Eval(vars)
	// if err != nil {
	// return c, fmt.Errorf("invalid CEL expression %q: %v", col.Expression, err)
	// }

	klog.V(1).Infof("HEHEHE EvalResult in tableconvertor: %v", evalResult)
	klog.V(1).Infof("HEHEHE eval err in tableconvertor: %v", err)

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

func (c celProgram) PrintResults(w io.Writer, results []reflect.Value) error {
	// return errors.New("this is an error")
	// Iterate over the reflect.Values in the results slice
	klog.V(1).Info("Inside cel PrintResults")
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

// func FinalColumnCompile(rule string) (celProgram, error) {
// 	// klog.V(1).Info("Inside FinalColumnCompile function")
// 	// // Jan 7
// 	// // Here we will need to update this env with the env that Kubernetes uses
// 	// // Look into prepareEnvSet() function in the cel package in this path
// 	// // We can possible reuse Alex's code to prepare env
// 	// env, err := cel.NewEnv()
// 	// klog.V(1).Info("Created cel.NewEnv()")
// 	// if err != nil {
// 	// 	klog.V(1).Info("env error: %v", err)
// 	// }
// 	// // Check that the expression compiles and returns a String.
// 	// // ast, iss := env.Parse(fmt.Sprintf("\"%s\"", rule))
// 	// // ast, iss := env.Parse(`size("test")`)
// 	// // klog.V(1).Info("Parsed cel expression to get ast")
// 	// // // Report syntactic errors, if present.
// 	// // if iss.Err() != nil {
// 	// // 	klog.V(1).Info(iss.Err())
// 	// // }
// 	// // // Type-check the expression for correctness.
// 	// // checked, iss := env.Check(ast)
// 	// // klog.V(1).Info("Checked the cel ast")
// 	// // // Report semantic errors, if present.
// 	// // if iss.Err() != nil {
// 	// // 	klog.V(1).Info(iss.Err())
// 	// // }
// 	// ast, iss := env.Compile(rule)
// 	// if iss.Err() != nil {
// 	// 	klog.V(1).Info(iss.Err())
// 	// }
// 	// // Check the output type is a string.
// 	// if ast.OutputType() != cel.AnyType {
// 	// 	klog.V(1).Infof(
// 	// 		"Got %v, wanted %v result type",
// 	// 		ast.OutputType(), cel.AnyType)
// 	// }
// 	// // Plan the program.
// 	// program, err := env.Program(ast)
// 	// klog.V(1).Info("Got cel program")
// 	// celProg := celProgram{Program: program}
// 	// return celProg, err
// 	// Evaluate the program without any additional arguments.
// 	env, err := cel.NewEnv(
// 		cel.Variable("self", cel.MapType(cel.StringType, cel.DynType)),
// 		cel.Function("string", cel.Overload("any_to_string", []*cel.Type{cel.DynType}, cel.StringType)))

// 	if err != nil {
// 		return celProgram{}, err
// 	}

// 	ast, iss := env.Compile(rule)
// 	if iss.Err() != nil {
// 		return celProgram{}, iss.Err()
// 	}

// 	// Program should return a string type
// 	if ast.OutputType() != cel.StringType {
// 		return celProgram{}, fmt.Errorf("expression must evaluate to a string, got %v", ast.OutputType())
// 	}

// 	program, err := env.Program(ast)
// 	if err != nil {
// 		return celProgram{}, err
// 	}

// 	return celProgram{Program: program}, nil
// }

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

// TODO Comment-Jan 06
// We've updated teh CompileColumns to handle just a single CEL expression at a time
// TODO Jan 17
// We uncommented CompileColumn and compileColumn to use in the validation for the cel expression
// but we haven't implemented it as of now. We have uncommented these functions and resolved all errors
// that were shown in THIS file.
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

	start := time.Now()
	oldSelfEnvSet, _, err := prepareEnvSet(baseEnvSet, declType)
	if err != nil {
		compResult := ColumnCompilationResult{Error: err}
		return compResult
	}
	estimator := newCostEstimator(declType)
	// compResults is the return value which saves a list of compilation results in the same order as x-kubernetes-validations rules.
	// compResults := make([]ColumnCompilationResult, len(exprs))
	maxCardinality := maxCardinality(declType.MinSerializedSize)
	ruleEnvSet := oldSelfEnvSet
	duration := time.Since(start)

	klog.Infof("May3: Time 1: Time taken for costEstimator for %s: %s\n", expr, duration)
	// klog.V(1).Infof("\n\n ruleEnvSet: %v", ruleEnvSet)
	compResult := compileColumnExpression(s, expr, ruleEnvSet, envLoader, estimator, maxCardinality, perCallLimit)
	// klog.V(1).Infof("\n\n compResult: %v", compResult)
	if compResult.Error != nil {
		return compResult
	}
	return compResult
}

// // End of comment-Jan 06

// func CompileColumns(exprs []string, s *schema.Structural, declType *apiservercel.DeclType, perCallLimit uint64, baseEnvSet *environment.EnvSet, envLoader EnvLoader) ([]ColumnCompilationResult, error) {
// 	oldSelfEnvSet, _, err := prepareEnvSet(baseEnvSet, declType)
// 	if err != nil {
// 		return nil, err
// 	}
// 	estimator := newCostEstimator(declType)
// 	// compResults is the return value which saves a list of compilation results in the same order as x-kubernetes-validations rules.
// 	compResults := make([]ColumnCompilationResult, len(exprs))
// 	maxCardinality := maxCardinality(declType.MinSerializedSize)
// 	for i, rule := range exprs {
// 		ruleEnvSet := oldSelfEnvSet
// 		compResults[i] = compileColumn(s, rule, ruleEnvSet, envLoader, estimator, maxCardinality, perCallLimit)
// 	}

// 	return compResults, nil
// }

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
	klog.Infof("May3: Time2: Time taken for CEL compilation for %s: %s\n", rule, duration)

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

	start = time.Now()
	klog.Infof("printing ast.outputType %s", ast.OutputType())
	if ast.OutputType() != cel.StringType &&
		ast.OutputType() != cel.BoolType &&
		ast.OutputType() != cel.IntType &&
		// ast.OutputType() != cel.DoubleType &&
		// ast.OutputType() != cel.DurationType &&
		//    ast.OutputType() != cel.ListType(cel.IntType) &&
		!ast.OutputType().IsExactType(cel.ListType(cel.IntType)) &&
		!ast.OutputType().IsExactType(cel.ListType(cel.StringType)) &&
		// !ast.OutputType().IsExactType(cel.ListType(cel.DynType)) &&
		// !ast.OutputType().IsExactType(cel.ListType(cel.MapType(cel.StringType, cel.StringType))) &&
		// !ast.OutputType().IsExactType(cel.ListType()) &&
		// !ast.OutputType().IsExactType(cel.ListType(cel.MapType(cel.StringType, cel.StringType))) &&
		ast.OutputType() != cel.DynType {
		// if ast.OutputType() != cel.DynType {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInvalid, Detail: "cel expression must evaluate to a valid type"}
		return
	}
	duration = time.Since(start)
	klog.Infof("May3: Time3: Time taken for if chain in CEL compilation function for %s: %s\n", rule, duration)

	start = time.Now()
	_, err := cel.AstToCheckedExpr(ast)
	if err != nil {
		// should be impossible since env.Compile returned no issues
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInternal, Detail: "unexpected compilation error: " + err.Error()}
		return
	}
	duration = time.Since(start)
	klog.Infof("May3: Time4: Time taken for astToCheckedExpr for %s: %s\n", rule, duration)

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
	klog.Infof("May3: Time5: Time taken for program generation for %s: %s\n", rule, duration)

	start = time.Now()
	costEst, err := ruleEnv.EstimateCost(ast, estimator)
	if err != nil {
		compilationResult.Error = &apiservercel.Error{Type: apiservercel.ErrorTypeInternal, Detail: "cost estimation failed: " + err.Error()}
		return
	}
	duration = time.Since(start)
	klog.Infof("May3: Time6: Time taken for cost estimation for %s: %s\n", rule, duration)
	// out, _, _ := eval(prog, s)
	// klog.V(1).Infof("HEHEHEH Program evaluation: %v", out)
	compilationResult.MaxCost = costEst.Max
	compilationResult.MaxCardinality = maxCardinality
	compilationResult.Program = &celProgram{Program: prog}
	return compilationResult
}

// func PrintColumns(compResults []ColumnCompilationResult, sts *schema.Structural, obj interface{}, remainingBudget int64) (field.ErrorList, []string) {
// 	var exprs []string
// 	activation, _ := validationActivationWithoutOldSelf(sts, obj, nil)
// 	var errs []*field.Error

// 	for _, compResult := range compResults {
// 		evalResult, evalDetails, err := compResult.Program.ContextEval(context.TODO(), activation)
// 		if evalDetails == nil {
// 			errs = append(errs, field.InternalError(compResult.FieldPath, fmt.Errorf("runtime cost could not be calculated, no further validation rules will be run")))
// 			return errs, nil
// 		} else {
// 			rtCost := evalDetails.ActualCost()
// 			if rtCost == nil {
// 				errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("runtime cost could not be calculated, no further validation rules will be run")))
// 				return errs, nil
// 			} else {
// 				if *rtCost > math.MaxInt64 || int64(*rtCost) > remainingBudget {
// 					errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("validation failed due to running out of cost budget, no further validation rules will be run")))
// 					return errs, nil
// 				}
// 				remainingBudget -= int64(*rtCost)
// 			}
// 		}
// 		if err != nil {
// 			// see types.Err for list of well defined error types
// 			if strings.HasPrefix(err.Error(), "no such overload") {
// 				// Most overload errors are caught by the compiler, which provides details on where exactly in the rule
// 				// error was found. Here, an overload error has occurred at runtime no details are provided, so we
// 				// append a more descriptive error message. This error can only occur when static type checking has
// 				// been bypassed. int-or-string is typed as dynamic and so bypasses compiler type checking.
// 				errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("'%v': call arguments did not match a supported operator, function or macro signature", err)))
// 			} else if strings.HasPrefix(err.Error(), "operation cancelled: actual cost limit exceeded") {
// 				errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("'%v': no further validation rules will be run due to call cost exceeds limit", err)))
// 				return errs, nil
// 			} else {
// 				// no such key: {key}, index out of bounds: {index}, integer overflow, division by zero, ...
// 				errs = append(errs, field.Invalid(compResult.FieldPath, sts.Type, fmt.Sprintf("%v", err)))
// 			}
// 			exprs = append(exprs, "")
// 			continue
// 		}

// 		str := evalResult.Value().(string)
// 		exprs = append(exprs, str)
// 	}
// 	return errs, exprs
// }
