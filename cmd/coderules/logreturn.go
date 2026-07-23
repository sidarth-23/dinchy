package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// loggingSink names methods that count as error logging. An empty typeName matches
// package-level functions; otherwise it matches methods on a pointer to the named type.
type loggingSink struct {
	packagePath string
	typeName    string
	methods     []string
}

var loggingSinks = []loggingSink{
	{packagePath: "github.com/sidarth-23/dinchy/internal/platform/logging", methods: []string{"Error", "HTTPError", "Panic"}},
	{packagePath: "log/slog", typeName: "Logger", methods: []string{"Error", "ErrorContext"}},
}

var (
	skippedPackagePaths = []string{"github.com/sidarth-23/dinchy/internal/platform/logging"}
	skippedPathMarkers  = []string{"/internal/platform/store/sqlcgen/"}
	skippedFileSuffixes = []string{"_test.go"}
)

type functionUnit struct {
	name               string
	pos                token.Pos
	body               *ast.BlockStmt
	info               *types.Info
	doc                *ast.CommentGroup
	comments           []*ast.CommentGroup
	errorResultIndexes []int
}

// runLogReturn enforces the project rule "never log and return the same error." It loads
// the packages matched by args with full type information and flags every function that
// both calls an error-logging method (see loggingSinks) and returns a non-nil error, unless
// the function carries a //dinchy:allow-logreturn directive with a reason. Findings are
// joined into one error; a nil return means the scanned tree is clean.
//
//dinchy:allow-logreturn validator entrypoint reports violations by returning an error
func runLogReturn(args []string) error {
	patterns := args
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	dir, patterns := loadDirAndPatterns(patterns)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Fset: token.NewFileSet(),
		Dir:  dir,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return err
	}

	violations := make([]string, 0)
	for _, pkg := range pkgs {
		if shouldSkipPackage(pkg) {
			continue
		}
		for _, pkgErr := range pkg.Errors {
			violations = append(violations, pkgErr.Error())
		}
		for i, file := range pkg.Syntax {
			filename := fileNameForSyntax(pkg, i, file)
			if shouldSkipFile(filename) {
				continue
			}
			for _, unit := range collectTypedFunctionUnits(pkg, file) {
				violations = append(violations, analyzeFunctionUnit(pkg.Fset, filename, unit)...)
			}
		}
	}

	if len(violations) == 0 {
		return nil
	}
	return errors.New(strings.Join(violations, "\n"))
}

func loadDirAndPatterns(patterns []string) (string, []string) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	if len(patterns) != 1 {
		return cwd, patterns
	}

	pattern := patterns[0]
	if strings.HasSuffix(pattern, "/...") || strings.HasSuffix(pattern, `\...`) {
		base := strings.TrimSuffix(pattern, "/...")
		base = strings.TrimSuffix(base, `\...`)
		if filepath.IsAbs(base) {
			return base, []string{"./..."}
		}
	}
	if !filepath.IsAbs(pattern) {
		return cwd, patterns
	}
	info, statErr := os.Stat(pattern)
	if statErr != nil {
		return filepath.Dir(pattern), []string{"."}
	}
	if info.IsDir() {
		return pattern, []string{"."}
	}
	return filepath.Dir(pattern), []string{"."}
}

func shouldSkipPackage(pkg *packages.Package) bool {
	if pkg == nil {
		return true
	}
	if slices.Contains(skippedPackagePaths, pkg.PkgPath) {
		return true
	}
	return pathContainsAny(pkg.PkgPath, skippedPathMarkers)
}

func shouldSkipFile(filename string) bool {
	if filename == "" {
		return true
	}
	for _, suffix := range skippedFileSuffixes {
		if strings.HasSuffix(filename, suffix) {
			return true
		}
	}
	return pathContainsAny(filename, skippedPathMarkers)
}

func pathContainsAny(path string, markers []string) bool {
	slashed := filepath.ToSlash(path)
	for _, marker := range markers {
		if strings.Contains(slashed, marker) {
			return true
		}
	}
	return false
}

func fileNameForSyntax(pkg *packages.Package, index int, file *ast.File) string {
	if pkg != nil && index >= 0 && index < len(pkg.GoFiles) {
		return pkg.GoFiles[index]
	}
	if pkg != nil && index >= 0 && index < len(pkg.CompiledGoFiles) {
		return pkg.CompiledGoFiles[index]
	}
	if pkg != nil && pkg.Fset != nil && file != nil {
		return pkg.Fset.Position(file.Pos()).Filename
	}
	return ""
}

func collectTypedFunctionUnits(pkg *packages.Package, file *ast.File) []functionUnit {
	if pkg == nil || file == nil {
		return nil
	}
	units := make([]functionUnit, 0)
	ast.Inspect(file, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body == nil {
				return true
			}
			units = append(units, functionUnit{
				name:               funcDeclDisplayName(fn),
				pos:                fn.Pos(),
				body:               fn.Body,
				info:               pkg.TypesInfo,
				doc:                fn.Doc,
				comments:           file.Comments,
				errorResultIndexes: errorResultIndexesFromSignature(signatureForFuncDecl(pkg.TypesInfo, fn)),
			})
			return true
		case *ast.FuncLit:
			if fn.Body == nil {
				return true
			}
			units = append(units, functionUnit{
				name:               "func literal",
				pos:                fn.Type.Func,
				body:               fn.Body,
				info:               pkg.TypesInfo,
				comments:           file.Comments,
				errorResultIndexes: errorResultIndexesFromSignature(signatureForFuncLit(pkg.TypesInfo, fn)),
			})
			return true
		default:
			return true
		}
	})
	return units
}

func errorResultIndexesFromSignature(sig *types.Signature) []int {
	if sig == nil || sig.Results() == nil {
		return nil
	}
	errorType := types.Universe.Lookup("error").Type()
	results := sig.Results()
	indexes := make([]int, 0, results.Len())
	for i := 0; i < results.Len(); i++ {
		if types.Identical(results.At(i).Type(), errorType) {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func signatureForFuncDecl(info *types.Info, decl *ast.FuncDecl) *types.Signature {
	if info == nil || decl == nil || decl.Name == nil {
		return nil
	}
	obj := info.Defs[decl.Name]
	if obj == nil {
		return nil
	}
	sig, _ := obj.Type().(*types.Signature)
	return sig
}

func signatureForFuncLit(info *types.Info, lit *ast.FuncLit) *types.Signature {
	if info == nil || lit == nil || lit.Type == nil {
		return nil
	}
	typ := info.TypeOf(lit.Type)
	if typ == nil {
		return nil
	}
	sig, _ := typ.(*types.Signature)
	return sig
}

func funcDeclDisplayName(decl *ast.FuncDecl) string {
	if decl == nil || decl.Name == nil {
		return "func"
	}
	return decl.Name.Name
}

func analyzeFunctionUnit(fset *token.FileSet, filename string, unit functionUnit) []string {
	violations := make([]string, 0, 2)
	found, reason := allowDirective(fset, unit)
	if found && reason == "" {
		violations = append(violations, formatFinding(fset, filename, unit.pos, "allow-logreturn directive requires a reason"))
	}
	if len(unit.errorResultIndexes) == 0 {
		return violations
	}
	if found && reason != "" {
		return violations
	}
	if logreturn := logReturnViolation(fset, filename, unit); logreturn != "" {
		violations = append(violations, logreturn)
	}
	return violations
}

// allowDirective reports whether the unit carries an allow-logreturn directive and the
// reason attached to it. reason is empty when the directive is present without one.
func allowDirective(fset *token.FileSet, unit functionUnit) (found bool, reason string) {
	for _, group := range directiveGroups(fset, unit) {
		if ok, r := commentGroupDirective(group); ok {
			return true, r
		}
	}
	return false, ""
}

func directiveGroups(fset *token.FileSet, unit functionUnit) []*ast.CommentGroup {
	groups := make([]*ast.CommentGroup, 0, 2)
	if unit.doc != nil {
		groups = append(groups, unit.doc)
	}
	if len(unit.comments) == 0 {
		return groups
	}
	startLine := fset.Position(unit.pos).Line
	endLine := fset.Position(unit.body.End()).Line
	for _, group := range unit.comments {
		if group == nil || group == unit.doc {
			continue
		}
		line := fset.Position(group.Pos()).Line
		if line == startLine || line == endLine {
			groups = append(groups, group)
		}
	}
	return groups
}

func commentGroupDirective(group *ast.CommentGroup) (found bool, reason string) {
	if group == nil {
		return false, ""
	}
	for _, comment := range group.List {
		if comment == nil {
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(comment.Text), "//"))
		reason, ok := strings.CutPrefix(text, "dinchy:allow-logreturn")
		if !ok {
			continue
		}
		return true, strings.TrimSpace(reason)
	}
	return false, ""
}

func logReturnViolation(fset *token.FileSet, filename string, unit functionUnit) string {
	hasLog := false
	hasErrorReturn := false
	ast.Inspect(unit.body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if isLoggingCall(unit, x) {
				hasLog = true
			}
		case *ast.ReturnStmt:
			if isErrorReturn(unit.errorResultIndexes, x) {
				hasErrorReturn = true
			}
		}
		return !hasLog || !hasErrorReturn
	})
	if !hasLog || !hasErrorReturn {
		return ""
	}
	return formatFinding(fset, filename, unit.pos, fmt.Sprintf("function %q logs an error and also returns one; log at the owning boundary or return only", unit.name))
}

func formatFinding(fset *token.FileSet, filename string, pos token.Pos, message string) string {
	return fmt.Sprintf("%s:%d: %s", filename, fset.Position(pos).Line, message)
}

func isErrorReturn(indexes []int, stmt *ast.ReturnStmt) bool {
	if len(indexes) == 0 || stmt == nil {
		return false
	}
	if len(stmt.Results) == 0 {
		return true
	}
	for _, index := range indexes {
		if index >= len(stmt.Results) {
			continue
		}
		if !isNilIdent(stmt.Results[index]) {
			return true
		}
	}
	return false
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

func isLoggingCall(unit functionUnit, call *ast.CallExpr) bool {
	if call == nil || unit.info == nil {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	for _, sink := range loggingSinks {
		if sink.matches(unit.info, sel) {
			return true
		}
	}
	return false
}

func (s loggingSink) matches(info *types.Info, sel *ast.SelectorExpr) bool {
	if !slices.Contains(s.methods, sel.Sel.Name) {
		return false
	}
	if s.typeName == "" {
		obj, ok := info.Uses[sel.Sel].(*types.Func)
		return ok && obj.Pkg() != nil && obj.Pkg().Path() == s.packagePath
	}
	selInfo, ok := info.Selections[sel]
	return ok && isPointerToNamed(selInfo.Recv(), s.packagePath, s.typeName)
}

func isPointerToNamed(t types.Type, packagePath, name string) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == packagePath && obj.Name() == name
}
