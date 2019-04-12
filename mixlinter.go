package mixlinter

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

var includeTest bool
var includeProtocolBufferGenerated bool

func init() {
	Analyzer.Flags.BoolVar(&includeTest, "test", false, "include test file or not")
	Analyzer.Flags.BoolVar(&includeProtocolBufferGenerated, "gen", false, "include generated by protocol buffer file")
}

var Analyzer = &analysis.Analyzer{
	Name: "mixlinter",
	Doc:  Doc,
	Run:  run,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
	Flags: flag.FlagSet{
		Usage: nil,
	},
}

const Doc = "mixlinter is ..."

func filterFile(fileName string) bool {
	if strings.HasPrefix(fileName, "mock_") {
		return true
	}
	if !includeTest && strings.HasSuffix(fileName, "_test.go") {
		return true
	}
	if !includeProtocolBufferGenerated && strings.HasSuffix(fileName, ".pb.go") {
		return true
	}
	return false
}

func hasNolintComment(pass *analysis.Pass, node ast.Node) bool {
	fileName := pass.Fset.File(node.Pos()).Name()
	line := pass.Fset.File(node.Pos()).Line(node.Pos())

	fp, err := os.Open(fileName)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := fp.Close(); err != nil {
			panic(err)
		}
	}()

	scanner := bufio.NewScanner(fp)
	for i := 0; i != line; i++ {
		scanner.Scan()
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	lineText := scanner.Text()

	return strings.Contains(lineText, "nolint:mixlinter")
}

func run(pass *analysis.Pass) (interface{}, error) {
	ins := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CompositeLit)(nil),
	}

	ins.Preorder(nodeFilter, func(astNode ast.Node) {
		if hasNolintComment(pass, astNode) {
			return
		}

		fileDirList := strings.Split(pass.Fset.File(astNode.Pos()).Name(), "/")
		fileName := fileDirList[len(fileDirList)-1]
		if filterFile(fileName) {
			return
		}

		var keySet bool

		compositeLit := astNode.(*ast.CompositeLit)
		var fields []string
		var setFields []string

		if reflect.ValueOf(compositeLit).IsNil() {
			return
		}

		if compositeLitType, ok := compositeLit.Type.(*ast.SelectorExpr); ok {
			nodeFile, err := parser.ParseFile(
				token.NewFileSet(),
				pass.Fset.File(astNode.Pos()).Name(),
				nil,
				0)
			if err != nil {
				fmt.Printf("pkg out Failed to parse file: %s\n", err)
				return
			}

			for _, imp := range nodeFile.Imports {
				importDirName := strings.Split(strings.Trim(imp.Path.Value, `"`), "/")
				importName := importDirName[len(importDirName)-1]

				if importName == fmt.Sprintf("%s", compositeLitType.X) {
					importDir, err := parser.ParseDir(
						token.NewFileSet(),
						filepath.Join(build.Default.GOPATH, "src", strings.Trim(imp.Path.Value, `"`)),
						nil,
						0)
					if err != nil {
						fmt.Printf("parse err:%+v\n", err)
						return
					}

					for _, importFile := range importDir {
						ast.Inspect(importFile, func(node ast.Node) bool {
							genDecl, ok := node.(*ast.GenDecl)
							if !ok {
								return true
							}
							for _, spec := range genDecl.Specs {
								typeSpec, ok := spec.(*ast.TypeSpec)
								if !ok {
									continue
								}
								if st, ok := typeSpec.Type.(*ast.StructType); ok {
									if typeSpec.Name.Name != compositeLitType.Sel.Name {
										continue
									}
									for _, f := range st.Fields.List {
										if len(f.Names) == 0 {
											switch t := f.Type.(type) {
											case *ast.SelectorExpr:
												if strings.HasPrefix(t.Sel.Name, "XXX_") {
													continue
												}
												fields = append(fields, t.Sel.Name)
											case *ast.StarExpr:
												if tse, ok := t.X.(*ast.SelectorExpr); ok {
													if strings.HasPrefix(tse.Sel.Name, "XXX_") {
														continue
													}
													fields = append(fields, tse.Sel.Name)
												}
											default:
												fmt.Printf("sf:%T\n", f.Type)
											}
										} else {
											if strings.HasPrefix(f.Names[0].Name, "XXX_") {
												continue
											}
											fields = append(fields, f.Names[0].Name)
										}
									}
								}
							}
							return true
						})
					}
				}
			}
		}

		if compositeLitType, ok := compositeLit.Type.(*ast.Ident); ok {
			if reflect.ValueOf(compositeLitType.Obj).IsNil() || reflect.ValueOf(compositeLitType.Obj.Decl).IsNil() {
				astFiles, err := parser.ParseDir(
					token.NewFileSet(),
					strings.Join(fileDirList[0:len(fileDirList)-1], "/"),
					nil,
					0)
				if err != nil {
					fmt.Printf("Failed to parse file\n")
					return
				}
				for _, astFile := range astFiles {
					ast.Inspect(astFile, func(node ast.Node) bool {
						genDecl, ok := node.(*ast.GenDecl)
						if !ok {
							return true
						}
						for _, spec := range genDecl.Specs {
							typeSpec, ok := spec.(*ast.TypeSpec)
							if !ok {
								continue
							}
							if st, ok := typeSpec.Type.(*ast.StructType); ok {
								if typeSpec.Name.Name != compositeLitType.Name {
									continue
								}
								for _, f := range st.Fields.List {
									if len(f.Names) == 0 {
										switch t := f.Type.(type) {
										case *ast.SelectorExpr:
											fields = append(fields, t.Sel.Name)
										}
									} else {
										switch f.Type.(type) {
										case *ast.Ident:
											fields = append(fields, f.Names[0].Name)
										}
									}
								}
							}
						}
						return true
					})
				}
			} else {
				if compositeLitDecl, ok := compositeLitType.Obj.Decl.(*ast.TypeSpec); ok {
					if st, ok := compositeLitDecl.Type.(*ast.StructType); ok {
						for _, f := range st.Fields.List {
							if len(f.Names) == 0 {
								switch t := f.Type.(type) {
								case *ast.SelectorExpr:
									fields = append(fields, t.Sel.Name)
								}
							} else {
								switch f.Type.(type) {
								case *ast.Ident:
									fields = append(fields, f.Names[0].Name)
								}
							}
						}
					}
				}
			}
		}

		for _, elt := range compositeLit.Elts {
			switch e := elt.(type) {
			case *ast.KeyValueExpr:
				keySet = true
				if ident, ok := e.Key.(*ast.Ident); ok {
					setFields = append(setFields, ident.Name)
				}
			default:
				setFields = append(setFields, "")
			}
		}
		if !keySet && len(setFields) != 0 {
			return
		}
		for _, field := range fields {
			if !contain(field, setFields) {
				pass.Reportf(compositeLit.Pos(), "uninitialised field found: %+v", field)
			}
		}

	})

	return nil, nil
}

func contain(s string, sl []string) bool {
	for _, v := range sl {
		if s == v {
			return true
		}
	}
	return false
}
