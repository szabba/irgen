// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/szabba/irgen"
)

var (
	outputFileName string
	verbose        bool
)

func main() {
	flag.StringVar(&outputFileName, "out", "", "name for the output file (computed if \"\", stdout if \"-\")")
	flag.BoolVar(&verbose, "v", false, "if true, copy all output to stdout, besides the output file")
	flag.Parse()

	var config irgen.Config

	if os.Getenv("GOFILE") == "" {
		log.Fatalf("environment variable GOFILE missing or empty")
	}
	config.Directory = filepath.Dir(os.Getenv("GOFILE"))

	if os.Getenv("GOPACKAGE") == "" {
		log.Fatalf("environment variable GOPACKAGE missing or empty")
	}
	config.PackageName = os.Getenv("GOPACKAGE")

	if flag.NArg() != 2 {
		log.Fatalf("two arguments wanted: COMPOSITE and CONSUMER")
	}

	config.TypeNames.Composite = flag.Arg(0)
	config.TypeNames.Consumer = flag.Arg(1)

	var buf bytes.Buffer
	err := config.Generate(&buf)
	if err != nil {
		log.Fatal(err)
	}

	var out io.Writer

	if outputFileName == "-" {
		out = os.Stdout

	} else {
		if outputFileName == "" {
			outputFileName = fmt.Sprintf("%s_impl.go", strings.ToLower(config.TypeNames.Composite))
		}

		file, err := os.Create(outputFileName)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		if verbose {
			out = io.MultiWriter(file, os.Stdout)
		} else {
			out = file
		}
	}

	_, err = io.Copy(out, &buf)
	if err != nil {
		log.Fatal(err)
	}
}
