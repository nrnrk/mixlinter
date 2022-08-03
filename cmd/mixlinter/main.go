package main

import (
	"github.com/nrnrk/mixlinter"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(mixlinter.Analyzer)
}
