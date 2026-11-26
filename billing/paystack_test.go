package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testProvider(t *testing.T, status int, body string) *PaystackProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transaction/verify/ref_1" || r.Header.Get("Authorization") != "Bearer sk_test_x" {
			t.Errorf("unexpected request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	p := NewPaystack("sk_test_x", "GHS")
	p.baseURL = srv.URL
	return p
}

func TestVerifyTransaction(t *testing.T) {
	ok := `{"status":true,"data":{"status":"success","amount":19900,"currency":"GHS"}}`
	for _, c := range []struct {
		name    string
		status  int
		body    string
		want    bool
		wantErr bool
	}{
		{"paid", 200, ok, true, false},
		{"wrong amount", 200, `{"status":true,"data":{"status":"success","amount":100,"currency":"GHS"}}`, false, false},
		{"wrong currency", 200, `{"status":true,"data":{"status":"success","amount":19900,"currency":"NGN"}}`, false, false},
		{"abandoned", 200, `{"status":true,"data":{"status":"abandoned","amount":19900,"currency":"GHS"}}`, false, false},
		{"not found", 404, `{"status":false,"message":"Transaction reference not found"}`, false, false},
		{"not found (400)", 400, `{"status":false,"message":"Transaction reference not found"}`, false, false},
		// Paystack could not be asked: an error, so the caller says "retry", not "payment failed".
		{"paystack down", 502, `<html>bad gateway</html>`, false, true},
		{"server error with json", 500, `{"status":false,"message":"An error occurred"}`, false, true},
		{"rate limited", 429, `{"status":false}`, false, true},
		{"bad key", 401, `{"status":false,"message":"Invalid key"}`, false, true},
		{"garbled 200", 200, `not json`, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := testProvider(t, c.status, c.body).VerifyTransaction(context.Background(), "ref_1", 19900)
			if got != c.want || (err != nil) != c.wantErr {
				t.Fatalf("got %v, err %v; want %v, err %v", got, err, c.want, c.wantErr)
			}
		})
	}
}
