package chain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// methodsTheRepoCallsOnClient is the shadow list: every method callers reach on a *Client that comes
// from the embedded *ethclient.Client. It is the txmanager Backend interface plus the reads the
// solvers and the chain helpers make directly.
var methodsTheRepoCallsOnClient = []string{
	"NonceAt", "PendingNonceAt", "FeeHistory", "SuggestGasTipCap",
	"HeaderByNumber", "HeaderByHash", "EstimateGas", "SendTransaction", "TransactionReceipt",
	"CallContract", "BalanceAt", "CodeAt", "BlockNumber",
}

// Client embeds *ethclient.Client, so any method it does not declare itself is promoted straight
// through and never reaches the tracing wrapper in calls.go: silently untraced, and invisible in the
// trace backend on a non-HTTP endpoint, where the transport records no span either. Adding a call to
// a promoted method is easy and the omission is not visible at the call site, so this is the
// structural guard — every method on the list above must be declared on *Client in this package.
func TestClientDeclaresEveryPromotedMethodTheRepoCalls(t *testing.T) {
	declared := clientMethodsDeclaredInPackage(t)
	for _, name := range methodsTheRepoCallsOnClient {
		if !declared[name] {
			t.Errorf("*Client does not declare %s: the promoted ethclient method is untraced", name)
		}
	}
}

// clientMethodsDeclaredInPackage returns the names of the methods this package declares on Client,
// read from the sources rather than reflection, which cannot tell a declared method from a promoted
// one.
func clientMethodsDeclaredInPackage(t *testing.T) map[string]bool {
	t.Helper()
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	fset := token.NewFileSet()
	declared := make(map[string]bool)
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, source, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", source, parseErr)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			if receiverTypeName(fn.Recv.List[0].Type) == "Client" {
				declared[fn.Name.Name] = true
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("no methods on Client found; the guard is not reading the package")
	}
	return declared
}

func receiverTypeName(expr ast.Expr) string {
	if star, isPointer := expr.(*ast.StarExpr); isPointer {
		expr = star.X
	}
	if ident, isIdent := expr.(*ast.Ident); isIdent {
		return ident.Name
	}
	return ""
}
