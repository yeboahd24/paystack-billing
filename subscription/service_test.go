package subscription

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/yeboahd24/paystack-billing/billing"
	"github.com/yeboahd24/paystack-billing/plan"
	"github.com/yeboahd24/paystack-billing/store/db"
	"github.com/yeboahd24/paystack-billing/store/sqlite"
)

// fakePay is a payment provider whose answers the test controls.
type fakePay struct {
	next     int
	ok       bool
	err      error
	verified []int // expected amounts passed to VerifyTransaction
}

func (f *fakePay) InitTransaction(_ context.Context, _ string, _ int, _ string, _ map[string]any) (string, string, error) {
	f.next++
	return "https://checkout.example/x", "ref-" + string(rune('a'+f.next-1)), nil
}
func (f *fakePay) VerifySignature([]byte, string) bool                { return true }
func (f *fakePay) ParseWebhook([]byte) (*billing.WebhookEvent, error) { return nil, nil }
func (f *fakePay) VerifyTransaction(_ context.Context, _ string, amount int) (bool, error) {
	f.verified = append(f.verified, amount)
	return f.ok, f.err
}

type sentMail struct{ to, subject string }

type fakeMail struct{ sent []sentMail }

func (m *fakeMail) Send(_ context.Context, to, _, subject, _ string) error {
	m.sent = append(m.sent, sentMail{to, subject})
	return nil
}

type env struct {
	t     *testing.T
	ctx   context.Context
	db    *sqlite.DB
	svc   *Service
	pay   *fakePay
	mail  *fakeMail
	plans *plan.Service
	now   time.Time
	user  int64
}

var t0 = time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	d, err := sqlite.Open(filepath.Join(t.TempDir(), "billing.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	for _, q := range []string{
		`INSERT INTO plans (name, display_name, amount, daily_limit) VALUES ('free', 'Free', 0, 20)`,
		`INSERT INTO plans (name, display_name, amount, daily_limit) VALUES ('starter', 'Starter', 10000, 500)`,
		`INSERT INTO plans (name, display_name, amount, daily_limit) VALUES ('pro', 'Pro', 50000, 5000)`,
	} {
		if _, err := d.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	plans, err := plan.NewService(ctx, sqlite.NewPlanRepository(d))
	if err != nil {
		t.Fatal(err)
	}
	users := sqlite.NewUserRepository(d)
	u, err := users.CreateUser(ctx, "ama@example.com", "Ama", "free", 20)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, ctx: ctx, db: d, pay: &fakePay{ok: true}, mail: &fakeMail{}, plans: plans, now: t0, user: u.ID}
	e.svc = NewService(sqlite.NewSubscriptionRepository(d), users, d, e.pay, plans, e.mail)
	e.svc.now = func() time.Time { return e.now }
	return e
}

// buy runs checkout + the charge.success webhook (with no metadata) for plan.
func (e *env) buy(planName string) string {
	e.t.Helper()
	_, ref, err := e.svc.InitCheckout(e.ctx, e.user, planName, "https://cb")
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.svc.HandleChargeSuccess(e.ctx, 0, "", ref); err != nil {
		e.t.Fatalf("charge %s: %v", ref, err)
	}
	return ref
}

func (e *env) tier() string {
	e.t.Helper()
	u, err := sqlite.NewUserRepository(e.db).GetUser(e.ctx, e.user)
	if err != nil {
		e.t.Fatal(err)
	}
	return u.Tier
}

func (e *env) rows() []db.Subscription {
	e.t.Helper()
	rs, err := sqlite.NewSubscriptionRepository(e.db).ListUserSubscriptions(e.ctx, e.user)
	if err != nil {
		e.t.Fatal(err)
	}
	return rs
}

func (e *env) byRef(ref string) db.Subscription {
	e.t.Helper()
	s, err := sqlite.NewSubscriptionRepository(e.db).GetSubscriptionByReference(e.ctx, ref)
	if err != nil || s == nil {
		e.t.Fatalf("ref %s: %v %v", ref, s, err)
	}
	return *s
}

func (e *env) current() *db.Subscription {
	e.t.Helper()
	s, err := sqlite.NewSubscriptionRepository(e.db).GetCurrentSubscription(e.ctx, e.user)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *env) currentCount() int {
	n := 0
	for _, r := range e.rows() {
		if r.Status == StatusActive || r.Status == StatusCancelled {
			n++
		}
	}
	return n
}

func end(s *db.Subscription) time.Time {
	t, _ := parseTime(s.CurrentPeriodEnd)
	return t
}

func TestPurchaseActivatesAndIsIdempotent(t *testing.T) {
	e := newEnv(t)
	ref := e.buy("pro")
	if e.tier() != "pro" {
		t.Fatalf("tier = %s", e.tier())
	}
	s := e.byRef(ref)
	if s.Status != StatusActive || !end(&s).Equal(time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("got %s until %v", s.Status, end(&s))
	}
	// Duplicate delivery (and with metadata this time): nothing changes.
	if err := e.svc.HandleChargeSuccess(e.ctx, e.user, "pro", ref); err != nil {
		t.Fatal(err)
	}
	if len(e.rows()) != 1 || e.currentCount() != 1 || len(e.pay.verified) != 1 {
		t.Fatalf("rows=%d current=%d verifies=%d", len(e.rows()), e.currentCount(), len(e.pay.verified))
	}
}

func TestEarlyRenewalExtendsPeriod(t *testing.T) {
	e := newEnv(t)
	first := e.buy("starter")
	e.now = t0.Add(20 * 24 * time.Hour) // 30 Jan, 11 days left
	second := e.buy("starter")

	if e.byRef(first).Status != StatusSuperseded {
		t.Fatalf("first = %s", e.byRef(first).Status)
	}
	cur := e.current()
	if cur.ProviderReference != second || e.currentCount() != 1 {
		t.Fatalf("current = %s, %d current rows", cur.ProviderReference, e.currentCount())
	}
	// Extends from 10 Feb, not from now.
	if want := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC); !end(cur).Equal(want) {
		t.Fatalf("end = %v, want %v", end(cur), want)
	}
}

func TestUpgradeCreditsUnusedTime(t *testing.T) {
	e := newEnv(t)
	e.buy("starter")                                      // until 10 Feb
	e.now = time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC) // 10 days left
	e.buy("pro")
	cur := e.current()
	// 10 days of starter (10000) is worth 2 days of pro (50000).
	want := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC).Add(2 * 24 * time.Hour)
	if cur.Plan != "pro" || !end(cur).Equal(want) || e.tier() != "pro" || e.currentCount() != 1 {
		t.Fatalf("plan=%s end=%v (want %v) tier=%s current=%d", cur.Plan, end(cur), want, e.tier(), e.currentCount())
	}
}

func TestPriceChangeDoesNotBreakPaymentInFlight(t *testing.T) {
	e := newEnv(t)
	_, ref, err := e.svc.InitCheckout(e.ctx, e.user, "starter", "https://cb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.ExecContext(e.ctx, `UPDATE plans SET amount = 12000 WHERE name = 'starter'`); err != nil {
		t.Fatal(err)
	}
	if err := e.plans.Refresh(e.ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.HandleChargeSuccess(e.ctx, 0, "", ref); err != nil {
		t.Fatal(err)
	}
	if e.pay.verified[0] != 10000 || e.tier() != "starter" {
		t.Fatalf("verified against %d, tier %s", e.pay.verified[0], e.tier())
	}
}

func TestPaymentErrors(t *testing.T) {
	e := newEnv(t)
	_, ref, _ := e.svc.InitCheckout(e.ctx, e.user, "pro", "https://cb")

	e.pay.ok = false
	err := e.svc.HandleChargeSuccess(e.ctx, 0, "", ref)
	if !errors.Is(err, ErrPaymentRejected) || !errors.Is(err, ErrVerificationFailed) || !IsPermanent(err) {
		t.Fatalf("rejected: %v", err)
	}
	e.pay.ok, e.pay.err = false, errors.New("paystack: HTTP 503")
	err = e.svc.HandleChargeSuccess(e.ctx, 0, "", ref)
	if err == nil || IsPermanent(err) {
		t.Fatalf("outage should be retryable: %v", err)
	}
	e.pay.err = nil
	if err := e.svc.HandleChargeSuccess(e.ctx, e.user+1, "", ref); !errors.Is(err, ErrReferenceMismatch) {
		t.Fatalf("other user: %v", err)
	}
	if err := e.svc.HandleChargeSuccess(e.ctx, 0, "", "never-seen"); !errors.Is(err, ErrUnknownPayment) {
		t.Fatalf("unknown: %v", err)
	}
	if e.tier() != "free" {
		t.Fatalf("tier = %s", e.tier())
	}
}

func TestPaymentWithoutCheckoutRecordUsesMetadata(t *testing.T) {
	e := newEnv(t)
	if err := e.svc.HandleChargeSuccess(e.ctx, e.user, "starter", "legacy-ref"); err != nil {
		t.Fatal(err)
	}
	if e.tier() != "starter" || e.pay.verified[0] != 10000 {
		t.Fatalf("tier %s verified %v", e.tier(), e.pay.verified)
	}
}

func TestCancelKeepsAccessUntilPeriodEndThenExpires(t *testing.T) {
	e := newEnv(t)
	e.buy("pro")
	if err := e.svc.Cancel(e.ctx, e.user); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Cancel(e.ctx, e.user); !errors.Is(err, ErrNoSubscription) {
		t.Fatalf("second cancel: %v", err)
	}
	e.now = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if n := e.svc.SweepExpired(e.ctx); n != 0 || e.tier() != "pro" {
		t.Fatalf("before end: swept %d, tier %s", n, e.tier())
	}
	e.now = time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)
	if n := e.svc.SweepExpired(e.ctx); n != 1 || e.tier() != "free" {
		t.Fatalf("at end: swept %d, tier %s", n, e.tier())
	}
	if e.current() != nil || e.rows()[0].Status != StatusExpired {
		t.Fatalf("status = %s", e.rows()[0].Status)
	}
}

func TestExpiredIsMarkedAndNotRefound(t *testing.T) {
	e := newEnv(t)
	e.buy("starter")
	e.now = t0.AddDate(0, 2, 0)
	if n := e.svc.SweepExpired(e.ctx); n != 1 {
		t.Fatalf("swept %d", n)
	}
	if n := e.svc.SweepExpired(e.ctx); n != 0 || len(e.mail.sent) != 1 {
		t.Fatalf("second sweep %d, emails %d", n, len(e.mail.sent))
	}
	// Paying again after expiry starts a fresh month from now.
	e.buy("starter")
	if want := AddMonths(e.now, 1); !end(e.current()).Equal(want) || e.currentCount() != 1 {
		t.Fatalf("end %v want %v", end(e.current()), want)
	}
}

func TestRenewalIsNotDowngradedBySweep(t *testing.T) {
	e := newEnv(t)
	e.buy("pro")
	e.now = t0.Add(25 * 24 * time.Hour)
	e.buy("pro")
	e.now = time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC) // past the first period only
	if n := e.svc.SweepExpired(e.ctx); n != 0 || e.tier() != "pro" {
		t.Fatalf("swept %d, tier %s", n, e.tier())
	}
	if ok, _ := e.svc.DowngradeExpired(e.ctx, e.user); ok {
		t.Fatal("DowngradeExpired downgraded a renewed user")
	}
}

func TestRefunds(t *testing.T) {
	t.Run("only payment", func(t *testing.T) {
		e := newEnv(t)
		ref := e.buy("pro")
		if err := e.svc.HandleRefund(e.ctx, ref, 50000); err != nil {
			t.Fatal(err)
		}
		if e.tier() != "free" || e.byRef(ref).Status != StatusRefunded || e.current() != nil {
			t.Fatalf("tier %s status %s", e.tier(), e.byRef(ref).Status)
		}
		if err := e.svc.HandleRefund(e.ctx, ref, 50000); err != nil { // redelivered
			t.Fatal(err)
		}
	})
	t.Run("current renewal restores the earlier month", func(t *testing.T) {
		e := newEnv(t)
		first := e.buy("pro") // until 10 Feb
		e.now = t0.Add(5 * 24 * time.Hour)
		second := e.buy("pro") // until 10 Mar
		if err := e.svc.HandleRefund(e.ctx, second, 0); err != nil {
			t.Fatal(err)
		}
		cur := e.current()
		if cur == nil || cur.ProviderReference != first || !end(cur).Equal(time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)) || e.tier() != "pro" {
			t.Fatalf("current %+v tier %s", cur, e.tier())
		}
	})
	t.Run("earlier payment shortens the current period", func(t *testing.T) {
		e := newEnv(t)
		first := e.buy("pro")
		e.now = t0.Add(5 * 24 * time.Hour)
		e.buy("pro")
		if err := e.svc.HandleRefund(e.ctx, first, 0); err != nil {
			t.Fatal(err)
		}
		if cur := e.current(); !end(cur).Equal(time.Date(2026, 2, 10, 12, 0, 0, 0, time.UTC)) || e.tier() != "pro" {
			t.Fatalf("end %v tier %s", end(cur), e.tier())
		}
	})
	t.Run("partial refund changes nothing", func(t *testing.T) {
		e := newEnv(t)
		ref := e.buy("pro")
		err := e.svc.HandleRefund(e.ctx, ref, 1000)
		if !errors.Is(err, ErrPartialRefund) || !IsPermanent(err) || e.tier() != "pro" {
			t.Fatalf("err %v tier %s", err, e.tier())
		}
	})
	t.Run("unknown reference", func(t *testing.T) {
		e := newEnv(t)
		if err := e.svc.HandleRefund(e.ctx, "nope", 0); !errors.Is(err, ErrUnknownPayment) {
			t.Fatal(err)
		}
	})
}

func TestAddMonths(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 9, 30, 0, 0, time.UTC) }
	for _, c := range []struct {
		in   time.Time
		n    int
		want time.Time
	}{
		{d(2026, 1, 31), 1, d(2026, 2, 28)},
		{d(2028, 1, 31), 1, d(2028, 2, 29)},
		{d(2026, 1, 15), 1, d(2026, 2, 15)},
		{d(2026, 12, 31), 2, d(2027, 2, 28)},
		{d(2026, 3, 31), -1, d(2026, 2, 28)},
		{d(2026, 5, 31), 1, d(2026, 6, 30)},
	} {
		if got := AddMonths(c.in, c.n); !got.Equal(c.want) {
			t.Errorf("AddMonths(%v, %d) = %v, want %v", c.in, c.n, got, c.want)
		}
	}
}
