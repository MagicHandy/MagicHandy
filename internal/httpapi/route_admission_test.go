package httpapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mapledaemon/MagicHandy/internal/accounts"
)

// This table deliberately stops at admission. Passing it does not prove that a
// handler checks its controller generation, gateway lease, resource owner or
// response redaction. Those contracts have separate behavioral tests.
func TestRegisteredRoutesHaveReviewedAdmissionPolicy(t *testing.T) {
	want := readRouteAdmissionMatrix(t)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	registered := make(map[string]token.Position)
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			pattern, ok := registeredRoutePattern(t, fset, node)
			if !ok {
				return true
			}
			if previous, found := registered[pattern]; found {
				t.Errorf("duplicate registration %q at %s and %s", pattern, previous, fset.Position(node.Pos()))
			}
			registered[pattern] = fset.Position(node.Pos())
			if _, reviewed := want[pattern]; !reviewed {
				t.Errorf("%s: add an explicit admission policy for %q", registered[pattern], pattern)
			}
			return true
		})
	}
	for pattern := range want {
		if _, found := registered[pattern]; !found {
			t.Errorf("stale admission entry %q has no registered route", pattern)
		}
	}
}

func registeredRoutePattern(t *testing.T, fset *token.FileSet, node ast.Node) (string, bool) {
	t.Helper()
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (selector.Sel.Name != "Handle" && selector.Sel.Name != "HandleFunc") {
		return "", false
	}
	// mediaSync.Handle is the domain operation, not an HTTP registration.
	if receiver, ok := selector.X.(*ast.SelectorExpr); ok && receiver.Sel.Name == "mediaSync" {
		return "", false
	}
	if len(call.Args) != 2 {
		t.Fatalf("unreviewed registration shape at %s", fset.Position(call.Pos()))
	}
	literal, ok := call.Args[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		t.Fatalf("nonliteral route needs explicit inventory support at %s", fset.Position(call.Pos()))
	}
	pattern, err := strconv.Unquote(literal.Value)
	if err != nil {
		t.Fatal(err)
	}
	return pattern, true
}

func readRouteAdmissionMatrix(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("testdata/route_admission.tsv")
	if err != nil {
		t.Fatal(err)
	}
	policies := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		policy, pattern, ok := strings.Cut(line, "\t")
		if !ok || !strings.Contains(pattern, " /") || policies[pattern] != "" {
			t.Fatalf("invalid or duplicate admission row %q", line)
		}
		switch policy {
		case "public", "shared", "self", "gateway", "control", "host":
		default:
			t.Fatalf("unknown admission policy %q", policy)
		}
		policies[pattern] = policy
	}
	if len(policies) == 0 {
		t.Fatal("empty route admission matrix")
	}
	return policies
}

type admissionIdentity struct {
	name           string
	cookie         *http.Cookie
	authenticated  bool
	control, admin bool
}

func TestRouteAdmissionMatrixAcrossRolesAndImplicitHEAD(t *testing.T) {
	s, store, admin, cookie := newControllerSessionFixture(t)
	identities := []admissionIdentity{
		{name: "anonymous"},
		{name: "administrator", cookie: cookie, authenticated: true, control: true, admin: true},
		newAdmissionIdentity(t, store, admin.ID, "observer", false),
		newAdmissionIdentity(t, store, admin.ID, "granted-operator", true),
	}
	revoked, _, err := store.NewSession(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeSession(t.Context(), revoked); err != nil {
		t.Fatal(err)
	}
	identities = append(identities, admissionIdentity{name: "revoked-administrator", cookie: testSessionCookie(revoked)})
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := s.authenticateRequests(s.authorizeRoutes(endpoint))
	resources := strings.NewReplacer("{id}", "resource", "{seq}", "1", "{lore_id}", "lore", "{role}", "tts")
	for pattern, policy := range readRouteAdmissionMatrix(t) {
		method, route, _ := strings.Cut(pattern, " ")
		methods := []string{method}
		if method == http.MethodGet {
			methods = append(methods, http.MethodHead)
		}
		for _, method := range methods {
			for _, identity := range identities {
				t.Run(method+" "+route+"/"+identity.name, func(t *testing.T) {
					r := httptest.NewRequest(method, resources.Replace(route), strings.NewReader(`{}`))
					if identity.cookie != nil {
						r.AddCookie(identity.cookie)
					}
					w := httptest.NewRecorder()
					handler.ServeHTTP(w, r)
					if want := admissionStatus(policy, identity); w.Code != want {
						t.Fatalf("policy %s: status %d, want %d", policy, w.Code, want)
					}
				})
			}
		}
	}
}

func newAdmissionIdentity(t *testing.T, store *accounts.Store, administrator, name string, control bool) admissionIdentity {
	t.Helper()
	account, err := store.Create(t.Context(), name, "synthetic route matrix passphrase", accounts.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	if control {
		if _, err := store.GrantControl(t.Context(), administrator, account.ID, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	token, _, err := store.NewSession(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	return admissionIdentity{name: name, cookie: testSessionCookie(token), authenticated: true, control: control}
}

func admissionStatus(policy string, identity admissionIdentity) int {
	if policy == "public" {
		return http.StatusNoContent
	}
	if !identity.authenticated {
		return http.StatusUnauthorized
	}
	if (policy == "host" && !identity.admin) || (policy == "control" && !identity.control) {
		return http.StatusForbidden
	}
	return http.StatusNoContent
}
