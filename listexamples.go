// Copyright Mike Hughes 2018 (mike AT mikehughes DOT info)
//
// LICENSE: BSD 3-Clause License (see http://opensource.org/licenses/BSD-3-Clause)
//
// listexamples is a command line utility to search all Go source code in a path recursively and list any example code for
// each function, method or package.
package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type funcs map[string][]string

type pkg map[string]funcs

func main() {
	// Fiddly cross platform stuff for usage message.
	ps := string(os.PathSeparator)
	cmd, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	cmdSlice := strings.Split(cmd, ps)
	cmd = cmdSlice[len(cmdSlice)-1]
	// Print usage if number of arguments is incorrect.
	if len(os.Args) != 2 {
		fmt.Printf("Incorrect usage:\nPlease use \"%s path%[2]sto%[2]ssearch%[2]s\"\n", cmd, ps)
		os.Exit(1)
	}
	// Search based on absolute path.
	searchPath, err := filepath.Abs(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	// Only search in $GOPATH. This allows full package import paths to be displayed correctly.
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = build.Default.GOPATH
	}
	if !strings.HasPrefix(searchPath, gopath) {
		log.Fatal("search path is not in GOPATH")
	}

	pkgList := make(pkg)

	fset := token.NewFileSet()

	// Recursively descend directories from searchPath.
	err = filepath.Walk(searchPath, func(path string, f os.FileInfo, err error) error {
		// if not a directory, skip it.
		if !f.IsDir() {
			return filepath.SkipDir
		}
		// Parse all .go files in the current directory and add them to the FileSet. Skip directories ending in .go .
		pkgs, err := parser.ParseDir(fset, path, func(fi os.FileInfo) bool { return !fi.IsDir() }, 0)
		if err != nil {
			log.Fatalf("Could not parse files in %s: %s\n", path, err)
		}
		for _, p := range pkgs {
			pName := p.Name
			// Trim "package_test" to "package" in order to group the fuctions together.
			if strings.HasSuffix(pName, "_test") {
				pName = strings.TrimSuffix(pName, "_test")
			}
			// Remove the leading "$GOPATH/src" from all package paths.
			pPath := strings.TrimPrefix(path, gopath+ps+"src"+ps)
			pkgName := fmt.Sprintf("%s in %s", pName, pPath)
			funcList := make(funcs)
			if ast.PackageExports(p) {
				for _, f := range p.Files {
					ast.Inspect(f, func(n ast.Node) bool {
						switch x := n.(type) {
						case *ast.FuncDecl: // Only process functions and methods.
							name := x.Name.String()
							filePos := fmt.Sprintf("%s:\t", fset.Position(n.Pos()))
							if !(isTest(name, "Test") || isTest(name, "Benchmark")) { // Don't process tests and benchmarks.
								if !isExample(name) { // Set the map key for the original function.
									funcList[name] = make([]string, 0)
								}
								if isExample(name) {
									if isSubExample(name) { // process sub-examples and method examples.
										if !isMethodExample(name) { // A sub-example for a normal function.
											key := strings.Split(strings.TrimPrefix(name, "Example"), "_")[0]
											funcList[key] = append(funcList[key], filePos+name)
										}
										if isMethodExample(name) { // An example for a method on a type.
											key := strings.TrimPrefix(name, "Example")
											key = strings.Split(key, "_")[0] + "." + strings.Split(name, "_")[1]
											funcList[key] = append(funcList[key], filePos+name)
										}
									} else { // A normal example.
										key := strings.TrimPrefix(name, "Example")
										funcList[key] = append(funcList[key], filePos+name)
									}
								}
							}
						}
						return true
					})
				}
			} else {
				log.Printf("No exported identifiers in %s\n", pName)
			}
			// Union of maps if combining "package" and "package_test".
			if len(pkgList[pkgName]) != 0 {
				for k, v := range funcList {
					pkgList[pkgName][k] = append(pkgList[pkgName][k], v...)
				}
			} else {
				pkgList[pkgName] = funcList
			}
		}
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(pkgList)
}

// isTest tells whether name looks like a test, example, or benchmark.
// It is a Test (say) if there is a character after Test that is not a
// lower-case letter. (We don't want Testiness.)
//
// isTest taken from https://golang.org/src/go/doc/example.go
// Copyright 2011 The Go Authors. All rights reserved.
func isTest(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) { // "Test" is ok
		return true
	}
	rune, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(rune)
}

// isExample checks if the name is a valid Example function name.
func isExample(name string) bool {
	return isTest(name, "Example")
}

// isSubExample checks if the name contains an underscore "_" character.
func isSubExample(name string) bool {
	nSlice := strings.Split(name, "_")
	return len(nSlice) > 1
}

// isMethodExample checks if the Example function is for
// a method on a type.
func isMethodExample(name string) bool {
	rune, _ := utf8.DecodeRuneInString(strings.Split(name, "_")[1])
	return !unicode.IsLower(rune)
}

// String satisfies fmt.Stringer for formatted output.
func (p pkg) String() string {
	var out string
	for k, v := range p {
		out += fmt.Sprintf("Package %s\n", k)
		for i, j := range v {
			if i == "" { // No function or method associated with example. It's a package example.
				out += fmt.Sprint("\tPackage level example:\n")
			} else {
				out += fmt.Sprintf("\t%s\n", i)
			}
			if len(j) != 0 {
				for _, m := range j {
					out += fmt.Sprintf("\t\t%s\n", m)
				}
			} else {
				out += fmt.Sprintf("\t\tNo Examples for function %s in package %s\n", i, k) // Np examples.
			}
		}
	}
	return out
}
