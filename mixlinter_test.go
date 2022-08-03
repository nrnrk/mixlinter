package mixlinter_test

import (
	"testing"

	"github.com/nrnrk/mixlinter"

	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, mixlinter.Analyzer, "a")
}
