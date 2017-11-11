// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package irgen

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	Directory   string
	PackageName string

	TypeNames struct {
		Composite string
		Consumer  string
	}

	Out io.Writer
}

func (cfg Config) Generate() error {
	gen := &generator{Config: cfg}
	return gen.run()
}

type generator struct {
	Config

	fset                *token.FileSet
	composite, consumer *ast.TypeSpec
	file                *ast.File
}

func (gen *generator) run() error {
	gen.fset = token.NewFileSet()

	err := gen.parseTypes()
	if err != nil {
		return errors.Wrap(err, "can't parse the composite/consumer type pair")
	}

	err = gen.generateAST()
	if err != nil {
		return err
	}

	return gen.dumpAST()
}

func (gen *generator) parseTypes() (err error) {
	pkgs, err := parser.ParseDir(gen.fset, gen.Directory, nil, 0)
	if err != nil {
		return errors.Errorf("can't parse package %s from dir %q: %s", gen.PackageName, gen.Directory, err)
	}

	pkg, ok := pkgs[gen.PackageName]
	if !ok {
		return errors.Errorf("package %s not in directory %q", gen.PackageName, gen.Directory)
	}

	gen.composite, err = typeSpecNamed(pkg, gen.TypeNames.Composite)
	if err != nil {
		return errors.Wrapf(err, "can't retrieve composite type %s spec", gen.TypeNames.Composite)
	}

	switch gen.composite.Type.(type) {
	case *ast.InterfaceType:
	default:
		return errors.Errorf("composite type %s is not an interface", gen.TypeNames.Composite)
	}

	gen.consumer, err = typeSpecNamed(pkg, gen.TypeNames.Consumer)
	if err != nil {
		return errors.Wrapf(err, "can't retrieve consumer type %s spec", gen.TypeNames.Consumer)
	}

	switch gen.consumer.Type.(type) {
	case *ast.InterfaceType:
	default:
		return errors.Errorf("consumer type %s is not an interface", gen.TypeNames.Consumer)
	}

	return nil
}

func (gen *generator) generateAST() error {
	typs, funs, err := gen.generateVariantTypes()
	if err != nil {
		return err
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

	gen.file = &ast.File{
		Name:  &ast.Ident{Name: gen.PackageName},
		Decls: decls,
	}
	return nil
}

func (gen *generator) dumpAST() error {
	_, err := io.WriteString(gen.Out, "// Code generated by irgen; DO NOT EDIT.\n\n")
	if err != nil {
		return err
	}

	return format.Node(gen.Out, gen.fset, gen.file)
}

func typeSpecNamed(pkg *ast.Package, name string) (*ast.TypeSpec, error) {
	specs := typeSpecsNamed(pkg, name)

	if len(specs) == 0 {
		return nil, fmt.Errorf("no type named %s in package %s", name, pkg.Name)
	}

	if len(specs) > 1 {
		return nil, fmt.Errorf("type %s declared %d times in package %s", name, len(specs), pkg.Name)
	}

	return specs[0], nil
}

func typeSpecsNamed(pkg *ast.Package, name string) []*ast.TypeSpec {
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

func (gen *generator) generateVariantTypes() ([]*ast.TypeSpec, []*ast.FuncDecl, error) {
	var (
		typs []*ast.TypeSpec
		funs []*ast.FuncDecl
	)
	compIface := gen.composite.Type.(*ast.InterfaceType)
	if compIface.Methods.NumFields() != 1 {
		return nil, nil, errors.Errorf(
			"the composite type should have 1 method (has %d)",
			compIface.Methods.NumFields())
	}
	compMethod := compIface.Methods.List[0]
	err := gen.checkDestructuringMethod(compMethod)
	if err != nil {
		return nil, nil, err
	}

	for _, method := range gen.consumer.Type.(*ast.InterfaceType).Methods.List {

		err := checkConsumerMethod(method)
		if err != nil {
			return nil, nil, err
		}

		typ, fun := gen.generateVariantType(compMethod, method)
		typs = append(typs, typ)
		funs = append(funs, fun)
	}

	return typs, funs, nil
}

func checkConsumerMethod(method *ast.Field) error {
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

func (gen *generator) generateVariantType(compositeMethod, consumerMethod *ast.Field) (*ast.TypeSpec, *ast.FuncDecl) {

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

	recvName := &ast.Ident{Name: strings.ToLower(gen.composite.Name.Name)}

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

func (gen *generator) checkDestructuringMethod(method *ast.Field) error {
	typ := method.Type.(*ast.FuncType)

	if typ.Params.NumFields() != 1 {
		return errors.Errorf(
			"composite method %s has more than one argument",
			method.Names[0].Name)
	}

	argGroup := typ.Params.List[0]
	argTyp, ok := argGroup.Type.(*ast.Ident)
	if !ok || argTyp.Name != gen.TypeNames.Consumer {
		return errors.Errorf(
			"composite method %s has wrong argument type (should be %s)",
			method.Names[0].Name, gen.TypeNames.Consumer)
	}

	if typ.Results.NumFields() > 0 {
		return errors.Errorf(
			"composite method %s has %d results (should have none)",
			method.Names[0].Name, typ.Results.NumFields())
	}

	return nil
}
