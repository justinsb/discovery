package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go/types"
	"io/ioutil"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
	"k8s.io/klog/v2"
)

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	klog.InitFlags(nil)
	flag.Parse()

	packageArgs := flag.Args()
	if len(packageArgs) != 1 {
		return fmt.Errorf("expected exactly one argument - package specifier")
	}

	packagePath := packageArgs[0]

	cfg := &packages.Config{Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedImports}
	pkgs, err := packages.Load(cfg, packagePath)
	if err != nil {
		return fmt.Errorf("error loading package %q: %w", packagePath, err)
	}
	if packages.PrintErrors(pkgs) != 0 {
		return fmt.Errorf("could not load packages")
	}

	imports := make(map[string]string)
	var lines []string

	for _, pkg := range pkgs {
		packageName := pkg.ID
		discoverableTypes := make(map[string]bool)
		for _, file := range pkg.Syntax {
			//klog.Infof("file %#v", file)
			for _, commentGroup := range file.Comments {
				for _, comment := range commentGroup.List {
					//klog.Infof("comment %#v", comment)
					text := comment.Text
					if strings.HasPrefix(text, "//+discovery:") {
						typeName := strings.TrimPrefix(text, "//+discovery:")
						discoverableTypes[typeName] = true
					}
				}
			}
		}
		klog.Infof("discoverableTypes %v", discoverableTypes)

		{
			scope := pkg.Types.Scope()
			klog.Infof("type %#v", scope.Names())
			for k := range discoverableTypes {
				obj := scope.Lookup(k)
				if obj == nil {
					return fmt.Errorf("object %q not found", k)
				}
				switch obj := obj.(type) {
				case *types.TypeName:
					importAlias := packageName
					importAlias = strings.ReplaceAll(importAlias, "/", "_")
					importAlias = strings.ReplaceAll(importAlias, ".", "_")
					imports[packageName] = importAlias
					lines = append(lines, fmt.Sprintf("discovery.Register(%q, %q, &%s.%v{})", packageName, k, importAlias, obj.Id()))
				default:
					return fmt.Errorf("type %T not handled for %q", obj, k)
				}
			}
			// numChildren := scope.NumChildren()
			// for i := 0; i < numChildren; i++ {
			// 	child := scope.Child(i)
			// 	klog.Infof("type %#v", child)
			// }
		}
	}

	// for _, pkg := range pkgs {
	// for _, file := range pkg.Syntax {
	// 	for _, decl := range file.Decls {
	// 		switch decl := decl.(type) {
	// 		case *ast.GenDecl:
	// 			klog.Infof("GenDecl %v", decl)
	// 		case *ast.FuncDecl:
	// 			klog.Infof("FuncDecl %v", decl)
	// 		default:
	// 			klog.Infof("unknown %T %#v", decl, decl)
	// 		}
	// 	}
	// }
	// }

	discoveryLib := "github.com/justinsb/discovery"
	var b bytes.Buffer

	w := &b
	fmt.Fprintf(w, "package main\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "import (\n")
	fmt.Fprintf(w, "  %q\n", discoveryLib)
	fmt.Fprintf(w, "\n")
	for k, v := range imports {
		fmt.Fprintf(w, "  %s %q\n", v, k)
	}
	fmt.Fprintf(w, ")\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "func RegisterForDiscovery() {\n")
	for _, line := range lines {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintf(w, "}\n")

	p := "discovery_generated.go"
	if err := ioutil.WriteFile(p, b.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write file %q: %w", p, err)
	}

	klog.Infof("wrote file %q", p)

	return nil
}
