package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go/ast"
	"go/format"
	"go/parser"
	"go/token"

	"github.com/pkg/errors"
)

var (
	src           = os.Getenv("GOFILE")
	pkgname       = os.Getenv("GOPACKAGE")
	dir           = filepath.Dir(src)
	consumerName  string
	compositeName string

	outputFileName string
	verbose        bool
)

func main() {
	flag.StringVar(&outputFileName, "out", "", "name for the output file (computed if \"\", stdout if \"-\")")
	flag.BoolVar(&verbose, "v", false, "if true, copy all output to stdout, besides the output file")
	flag.Parse()

	if flag.NArg() != 2 {
		log.Fatalf("two arguments wanted: CONSUMER and COMPOSITE")
	}

	if src == "" {
		log.Fatalf("missing GOFILE")
	}

	consumerName, compositeName = flag.Arg(0), flag.Arg(1)

	if outputFileName == "" {
		outputFileName = fmt.Sprintf("%s_impl.go", strings.ToLower(consumerName))
	}

	fset := token.NewFileSet()

	composite, consumer, err := ParseTypes(fset, dir, compositeName, consumerName)
	if err != nil {
		log.Fatalf("can't parse the composite/consumer type pair: %s", err)
	}

	switch composite.Type.(type) {
	case *ast.InterfaceType:
	default:
		log.Fatalf("composite type %s is not an interface", compositeName)
	}

	switch consumer.Type.(type) {
	case *ast.InterfaceType:
	default:
		log.Fatalf("composite type %s is not an interface", consumerName)
	}

	fileAST, err := File(pkgname, composite, consumer)
	if err != nil {
		log.Fatal(err)
	}

	var out io.Writer

	if outputFileName != "-" {
		file, err := os.Create(outputFileName)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		out = file
	}

	if out == nil {
		out = os.Stdout
	} else if verbose {
		out = io.MultiWriter(out, os.Stdout)
	}

	err = format.Node(out, fset, fileAST)
	if err != nil {
		log.Fatal(err)
	}
}

func ParseTypes(fset *token.FileSet, dir, consumerName, compositeName string) (composite, consumer *ast.TypeSpec, err error) {
	pkgs, err := parser.ParseDir(fset, dir, nil, 0)
	if err != nil {
		log.Fatalf("can't parse package %s from dir %q: %s", pkgname, dir, err)
	}

	pkg, ok := pkgs[pkgname]
	if !ok {
		return nil, nil, errors.Errorf("package %s not in directory %q", pkgname, dir)
	}

	composite, err = TypeSpecNamed(pkg, compositeName)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "can't retrieve composite type %s spec", compositeName)
	}

	consumer, err = TypeSpecNamed(pkg, consumerName)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "can't retrieve consumer type %s spec", consumerName)
	}
	return composite, consumer, nil
}

func TypeSpecNamed(pkg *ast.Package, name string) (*ast.TypeSpec, error) {
	specs := TypeSpecsNamed(pkg, name)

	if len(specs) == 0 {
		return nil, fmt.Errorf("no type named %s in package %s", name, pkgname)
	}

	if len(specs) > 1 {
		return nil, fmt.Errorf("type %s declared %d times in package %s", name, len(specs), pkgname)
	}

	return specs[0], nil
}

func TypeSpecsNamed(pkg *ast.Package, name string) []*ast.TypeSpec {
	var specs []*ast.TypeSpec

	for _, f := range pkg.Files {

		for _, decl := range f.Decls {

			switch decl := decl.(type) {
			case *ast.GenDecl:

				for _, spec := range decl.Specs {

					switch spec := spec.(type) {
					case (*ast.TypeSpec):

						if spec.Name.Name == name {
							specs = append(specs, spec)
						}
					}
				}
			}
		}
	}

	return specs
}

func File(pkgname string, composite, consumer *ast.TypeSpec) (*ast.File, error) {
	typs, funs, err := VariantTypes(composite, consumer)
	if err != nil {
		return nil, err
	}

	var decls []ast.Decl
	for _, typ := range typs {
		decls = append(decls, &ast.GenDecl{
			Tok:   token.TYPE,
			Specs: []ast.Spec{typ},
		})
	}
	for _, fun := range funs {
		decls = append(decls, fun)
	}

	file := &ast.File{
		Name:  &ast.Ident{Name: pkgname},
		Decls: decls,
	}
	return file, nil
}

func VariantTypes(composite, consumer *ast.TypeSpec) ([]*ast.TypeSpec, []*ast.FuncDecl, error) {
	var (
		typs []*ast.TypeSpec
		funs []*ast.FuncDecl
	)
	compIface := composite.Type.(*ast.InterfaceType)
	if compIface.Methods.NumFields() != 1 {
		return nil, nil, errors.Errorf(
			"the composite type should have 1 method (has %d)",
			compIface.Methods.NumFields())
	}
	compMethod := compIface.Methods.List[0]
	err := CheckDestructuringMethod(compMethod, consumer)
	if err != nil {
		return nil, nil, err
	}

	for _, method := range consumer.Type.(*ast.InterfaceType).Methods.List {

		err := CheckConsumerMethod(method)
		if err != nil {
			return nil, nil, err
		}

		typ, fun := VariantType(composite.Name, consumer.Name, compMethod, method)
		typs = append(typs, typ)
		funs = append(funs, fun)
	}

	return typs, funs, nil
}

func CheckConsumerMethod(method *ast.Field) error {
	typ := method.Type.(*ast.FuncType)
	for _, argGroup := range typ.Params.List {

		if len(argGroup.Names) == 0 {
			return errors.Errorf(
				"consumer method %s has unnamed arguments",
				method.Names[0].Name)
		}

		for _, name := range argGroup.Names {

			if !name.IsExported() {
				return errors.Errorf(
					"consumer method %s has argument names that can't be turned into exported field names",
					method.Names[0].Name)
			}
		}
	}

	if typ.Results.NumFields() > 0 {
		return errors.Errorf(
			"consumer method %s has %d results (should have none)",
			method.Names[0].Name, typ.Results.NumFields())
	}

	return nil
}

func VariantType(composite, consumer *ast.Ident, compositeMethod, consumerMethod *ast.Field) (*ast.TypeSpec, *ast.FuncDecl) {

	// NOTE: As we build the AST here, we're making manual copies instead of
	// reusing nodes from the original package sources. When this happens,
	// it's to avoid weird formatting -- since format.Node takes position
	// information in the nodes into account when deciding where to insert
	// whitespace.

	typName := consumerMethod.Names[0]
	funName := compositeMethod.Names[0]

	fields := consumerMethod.Type.(*ast.FuncType).Params.List

	shape := &ast.StructType{
		Fields: &ast.FieldList{
			List: fields,
		},
	}

	typ := &ast.TypeSpec{
		Name: typName,
		Type: shape,
	}

	argName := &ast.Ident{Name: "consumer"}
	funtyp := compositeMethod.Type.(*ast.FuncType)
	funtyp.Params.List[0].Names = []*ast.Ident{argName}

	recvName := &ast.Ident{Name: strings.ToLower(composite.Name)}

	// NOTE: See the note at the top of this function.
	consumerMethodName := &ast.Ident{Name: consumerMethod.Names[0].Name}
	methodLookup := &ast.SelectorExpr{X: argName, Sel: consumerMethodName}

	var args []ast.Expr
	for _, field := range fields {

		for _, name := range field.Names {

			// NOTE: See the note at the top of this function.
			fieldname := &ast.Ident{Name: name.Name}
			lookup := &ast.SelectorExpr{X: recvName, Sel: fieldname}
			args = append(args, lookup)
		}
	}

	call := &ast.CallExpr{Fun: methodLookup, Args: args}

	var body *ast.BlockStmt

	if funtyp.Results.NumFields() > 0 {
		body = &ast.BlockStmt{
			List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{call}}},
		}
	} else {
		body = &ast.BlockStmt{
			List: []ast.Stmt{&ast.ExprStmt{X: call}},
		}
	}

	recvTyp := &ast.StarExpr{X: typName}

	recv := &ast.FieldList{
		List: []*ast.Field{
			&ast.Field{
				Names: []*ast.Ident{recvName},
				Type:  recvTyp,
			},
		},
	}

	fun := &ast.FuncDecl{
		Recv: recv, Name: funName, Type: funtyp,
		Body: body,
	}

	return typ, fun
}

func CheckDestructuringMethod(method *ast.Field, consumer *ast.TypeSpec) error {
	typ := method.Type.(*ast.FuncType)

	if typ.Params.NumFields() != 1 {
		return errors.Errorf(
			"composite method %s has more than one argument",
			method.Names[0].Name)
	}

	argGroup := typ.Params.List[0]
	argTyp, ok := argGroup.Type.(*ast.Ident)
	if !ok || argTyp.Name != consumer.Name.Name {
		return errors.Errorf(
			"composite method %s has wrong argument type (should be %s)",
			method.Names[0].Name, consumer.Name.Name)
	}

	if typ.Results.NumFields() > 0 {
		return errors.Errorf(
			"composite method %s has %d results (should have none)",
			method.Names[0].Name, typ.Results.NumFields())
	}

	return nil
}
