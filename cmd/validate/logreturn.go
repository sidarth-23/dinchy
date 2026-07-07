package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	loggingPackagePath     = "github.com/sidarth-23/dinchy/internal/platform/logging"
	sqlcGeneratedDirMarker = "/internal/platform/store/sqlcgen/"
	allowLogReturnPrefix   = "dinchy:allow-logreturn"
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

//dinchy:allow-logreturn validator entrypoint reports violations by returning an error
func runValidateLogReturn(args []string) error {
	patterns := args
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	if target, recursive, ok := syntaxFallbackTarget(patterns[0]); ok && len(patterns) == 1 {
		if recursive {
			return runValidateLogReturnDir(target)
		}
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			return runValidateLogReturnDir(target)
		}
		return runValidateLogReturnFile(target)
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

func syntaxFallbackTarget(pattern string) (target string, recursive, ok bool) {
	if pattern == "" {
		return "", false, false
	}
	if strings.HasSuffix(pattern, "/...") || strings.HasSuffix(pattern, `\...`) {
		base := strings.TrimSuffix(pattern, "/...")
		base = strings.TrimSuffix(base, `\...`)
		if filepath.IsAbs(base) {
			return base, true, true
		}
		return "", false, false
	}
	if !filepath.IsAbs(pattern) {
		return "", false, false
	}
	return pattern, false, true
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

func runValidateLogReturnFile(path string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}
	violations := make([]string, 0)
	for _, unit := range collectSyntaxFunctionUnits(file) {
		violations = append(violations, analyzeFunctionUnit(fset, path, unit)...)
	}
	if len(violations) == 0 {
		return nil
	}
	return errors.New(strings.Join(violations, "\n"))
}

func runValidateLogReturnDir(dir string) error {
	fset := token.NewFileSet()
	violations := make([]string, 0)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if shouldSkipPath(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || shouldSkipPath(path) {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		for _, unit := range collectSyntaxFunctionUnits(file) {
			violations = append(violations, analyzeFunctionUnit(fset, path, unit)...)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		return nil
	}
	return errors.New(strings.Join(violations, "\n"))
}

func shouldSkipPackage(pkg *packages.Package) bool {
	if pkg == nil {
		return true
	}
	if pkg.PkgPath == loggingPackagePath {
		return true
	}
	return strings.Contains(filepath.ToSlash(pkg.PkgPath), sqlcGeneratedDirMarker)
}

func shouldSkipFile(filename string) bool {
	if filename == "" {
		return true
	}
	if strings.HasSuffix(filename, "_test.go") {
		return true
	}
	return strings.Contains(filepath.ToSlash(filename), sqlcGeneratedDirMarker)
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

func shouldSkipPath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), sqlcGeneratedDirMarker)
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

func collectSyntaxFunctionUnits(file *ast.File) []functionUnit {
	if file == nil {
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
				doc:                fn.Doc,
				comments:           file.Comments,
				errorResultIndexes: errorResultIndexesFromFieldList(fn.Type.Results),
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
				comments:           file.Comments,
				errorResultIndexes: errorResultIndexesFromFieldList(fn.Type.Results),
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

func errorResultIndexesFromFieldList(fields *ast.FieldList) []int {
	if fields == nil {
		return nil
	}
	indexes := make([]int, 0)
	position := 0
	for _, field := range fields.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		if isErrorTypeExpr(field.Type) {
			for i := 0; i < count; i++ {
				indexes = append(indexes, position+i)
			}
		}
		position += count
	}
	return indexes
}

func isErrorTypeExpr(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "error"
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
	if directive := directiveViolation(fset, filename, unit); directive != "" {
		violations = append(violations, directive)
	}
	if len(unit.errorResultIndexes) == 0 {
		return violations
	}
	if exempt := directiveExempts(fset, unit); exempt {
		return violations
	}
	if logreturn := logReturnViolation(fset, filename, unit); logreturn != "" {
		violations = append(violations, logreturn)
	}
	return violations
}

func directiveViolation(fset *token.FileSet, filename string, unit functionUnit) string {
	if group := unit.doc; group != nil {
		if found, msg := commentGroupDirective(group); found {
			if msg != "" {
				return fmt.Sprintf("%s:%d: %s", filename, fset.Position(unit.pos).Line, msg)
			}
			return ""
		}
	}
	if len(unit.comments) == 0 {
		return ""
	}
	startLine := fset.Position(unit.pos).Line
	endLine := fset.Position(unit.body.End()).Line
	for _, group := range unit.comments {
		if group == nil || group == unit.doc {
			continue
		}
		line := fset.Position(group.Pos()).Line
		if line != startLine && line != endLine {
			continue
		}
		if found, msg := commentGroupDirective(group); found {
			if msg != "" {
				return fmt.Sprintf("%s:%d: %s", filename, fset.Position(unit.pos).Line, msg)
			}
			return ""
		}
	}
	return ""
}

func directiveExempts(fset *token.FileSet, unit functionUnit) bool {
	if group := unit.doc; group != nil {
		if found, msg := commentGroupDirective(group); found {
			return msg == ""
		}
	}
	if len(unit.comments) == 0 {
		return false
	}
	startLine := fset.Position(unit.pos).Line
	endLine := fset.Position(unit.body.End()).Line
	for _, group := range unit.comments {
		if group == nil || group == unit.doc {
			continue
		}
		line := fset.Position(group.Pos()).Line
		if line != startLine && line != endLine {
			continue
		}
		if found, msg := commentGroupDirective(group); found {
			return msg == ""
		}
	}
	return false
}

func commentGroupDirective(group *ast.CommentGroup) (bool, string) {
	if group == nil {
		return false, ""
	}
	for _, comment := range group.List {
		if comment == nil {
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(comment.Text), "//"))
		if !strings.HasPrefix(text, allowLogReturnPrefix) {
			continue
		}
		reason := strings.TrimSpace(strings.TrimPrefix(text, allowLogReturnPrefix))
		if reason == "" {
			return true, "allow-logreturn directive requires a reason"
		}
		return true, ""
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
	return fmt.Sprintf("%s:%d: function %q logs an error and also returns one; log at the owning boundary or return only", filename, fset.Position(unit.pos).Line, unit.name)
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
	if unit.info != nil {
		if isLoggingCallTyped(unit, call) {
			return true
		}
	}
	return isLoggingCallSyntax(call)
}

func isLoggingCallTyped(unit functionUnit, call *ast.CallExpr) bool {
	if call == nil || unit.info == nil {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if obj, ok := unit.info.Uses[sel.Sel].(*types.Func); ok && obj.Pkg() != nil && obj.Pkg().Path() == loggingPackagePath {
		switch obj.Name() {
		case "Error", "HTTPError", "Panic":
			return true
		}
	}
	if selInfo, ok := unit.info.Selections[sel]; ok {
		if isSlogLoggerPointer(selInfo.Recv()) {
			switch sel.Sel.Name {
			case "Error", "ErrorContext":
				return true
			}
		}
	}
	return false
}

func isLoggingCallSyntax(call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "Error", "HTTPError", "ErrorContext", "Panic":
		return true
	default:
		return false
	}
}

func isSlogLoggerPointer(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "log/slog" && obj.Name() == "Logger"
}
