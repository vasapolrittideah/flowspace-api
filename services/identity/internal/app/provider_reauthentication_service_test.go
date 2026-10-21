package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/app"
	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	inbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/in"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

// providerReauthenticationRepository records each session that the service
// locks or re-authenticates as "subject:session".
type providerReauthenticationRepository struct {
	window                      domain.ReauthenticationWindow
	now                         time.Time
	sessionErr, createErr       error
	claimErr, linkErr, writeErr error
	commitErr                   error
	result                      outbound.ProviderResult
	claimed, linked             bool
	attempts                    []outbound.ProviderAttempt
	locks, claims, links        []string
	writes                      []string
	transactions, commits       int
	deadline                    time.Time
	claimedAttempt, claimedCode [32]byte
}

func (r *providerReauthenticationRepository) WithinProviderReauthenticationTransaction(_ context.Context, fn func(outbound.ProviderReauthenticationTransaction) error) (domain.ReauthenticationWindow, error) {
	r.transactions++
	if err := fn(r); err != nil {
		return domain.ReauthenticationWindow{}, err
	}
	if r.commitErr != nil {
		return domain.ReauthenticationWindow{}, r.commitErr
	}
	r.commits++
	return r.window, nil
}

func (r *providerReauthenticationRepository) LockSessionForReauthentication(_ context.Context, subject, sessionID string) (domain.ReauthenticationWindow, time.Time, error) {
	r.locks = append(r.locks, subject+":"+sessionID)
	return r.window, r.now, r.sessionErr
}

func (r *providerReauthenticationRepository) RequireCommitBefore(deadline time.Time) {
	r.deadline = deadline
}

func (r *providerReauthenticationRepository) CreateProviderAttempt(_ context.Context, attempt outbound.ProviderAttempt) (time.Time, error) {
	if r.createErr != nil {
		return time.Time{}, r.createErr
	}
	r.attempts = append(r.attempts, attempt)
	return time.Date(2026, 10, 1, 12, 10, 0, 0, time.UTC), nil
}

func (r *providerReauthenticationRepository) ClaimProviderReauthenticationResult(_ context.Context, subject, sessionID string, attempt, handoff [32]byte) (outbound.ProviderResult, bool, error) {
	r.claims = append(r.claims, subject+":"+sessionID)
	r.claimedAttempt, r.claimedCode = attempt, handoff
	return r.result, r.claimed, r.claimErr
}

func (r *providerReauthenticationRepository) HasProviderLink(_ context.Context, subject, provider, providerSubject string) (bool, error) {
	r.links = append(r.links, subject+":"+provider+":"+providerSubject)
	return r.linked, r.linkErr
}

func (r *providerReauthenticationRepository) ReauthenticateSession(_ context.Context, subject, sessionID string) error {
	r.writes = append(r.writes, subject+":"+sessionID)
	r.window.AuthenticatedAt = r.now
	return r.writeErr
}

// onlyFixtureSession reports whether every lock and write targets the session
// of the fixture input.
func (r *providerReauthenticationRepository) onlyFixtureSession() bool {
	return len(r.locks) != 0 && !slices.ContainsFunc(slices.Concat(r.locks, r.writes), func(target string) bool { return target != "subject-1:session-1" })
}

// checkStartLocks makes sure that a start without an identity reaches no
// store and that any other start locks only the fixture session.
func (r *providerReauthenticationRepository) checkStartLocks(t *testing.T, input inbound.StartProviderReauthenticationInput) {
	t.Helper()
	if input.Subject == "" || input.SessionID == "" {
		if r.transactions != 0 || len(r.locks) != 0 {
			t.Fatal("a missing identity reached the session store")
		}
	} else if !r.onlyFixtureSession() {
		t.Fatalf("locks = %v", r.locks)
	}
}

type providerReauthenticationFixture struct {
	attempts   *providerAttemptRepository
	repository *providerReauthenticationRepository
	started    []string
	failures   []string
	startErr   error
	failureErr error
	service    *app.ProviderReauthenticationService
	key        []byte
	input      inbound.CompleteProviderReauthenticationInput
}

func newProviderReauthenticationFixture(t *testing.T) *providerReauthenticationFixture {
	t.Helper()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	f := &providerReauthenticationFixture{
		attempts: &providerAttemptRepository{},
		repository: &providerReauthenticationRepository{
			now: now, window: domain.ReauthenticationWindow{IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)},
			result: outbound.ProviderResult{Provider: "github", Subject: "github-subject"}, claimed: true, linked: true,
		},
		key: make([]byte, 32),
		input: inbound.CompleteProviderReauthenticationInput{
			Subject: "subject-1", SessionID: "session-1", AttemptToken: strings.Repeat("A", 43), HandoffCode: strings.Repeat("B", 42) + "A", Source: "192.0.2.1",
		},
	}
	login := app.NewProviderLoginService(f.attempts, func(_ context.Context, source string) error {
		f.started = append(f.started, source)
		return f.startErr
	}, allowLimit, f.key, providerClients)
	f.service = app.NewProviderReauthenticationService(login, f.repository, f.failureLimit)
	return f
}

func (f *providerReauthenticationFixture) failureLimit(_ context.Context, source string) error {
	f.failures = append(f.failures, source)
	return f.failureErr
}

func (f *providerReauthenticationFixture) startInput(provider string) inbound.StartProviderReauthenticationInput {
	return inbound.StartProviderReauthenticationInput{Subject: "subject-1", SessionID: "session-1", Provider: provider, Source: "192.0.2.1"}
}

func TestStartProviderReauthenticationBindsAttemptToSession(t *testing.T) {
	f := newProviderReauthenticationFixture(t)

	result, err := f.service.StartProviderReauthentication(t.Context(), f.startInput("github"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.repository.attempts) != 1 || len(f.attempts.attempts) != 0 || f.repository.commits != 1 || !f.repository.onlyFixtureSession() {
		t.Fatalf("attempts in transaction = %d, outside = %d, commits = %d, locks = %v", len(f.repository.attempts), len(f.attempts.attempts), f.repository.commits, f.repository.locks)
	}
	attempt := f.repository.attempts[0]
	if attempt.Subject != "subject-1" || attempt.SessionID != "session-1" || len(f.repository.writes) != 0 {
		t.Fatalf("attempt binding = %q/%q, writes = %v", attempt.Subject, attempt.SessionID, f.repository.writes)
	}
	if len(f.started) != 1 || f.started[0] != "192.0.2.1" || !result.ExpiresAt.Equal(time.Date(2026, 10, 1, 12, 10, 0, 0, time.UTC)) {
		t.Fatalf("limits = %v, expiry = %v", f.started, result.ExpiresAt)
	}
	checkProviderAuthorization(t, result, attempt, f.key, "github", "https://github.com/login/oauth/authorize", "user:email", false)
}

func TestStartProviderReauthenticationRejectsWithoutAttempt(t *testing.T) {
	for _, test := range []struct {
		name                      string
		subject, sessionID        string
		provider                  string
		session, startErr, create error
		commit                    error
		want                      error
		wantLimit, created        bool
	}{
		{name: "missing subject", sessionID: "session-1", provider: "github", want: outbound.ErrUnauthenticated},
		{name: "missing session", subject: "subject-1", provider: "github", want: outbound.ErrUnauthenticated},
		{name: "inactive session", session: outbound.ErrUnauthenticated, want: outbound.ErrUnauthenticated},
		{name: "unknown provider", provider: "okta", want: domain.ErrInvalidProvider},
		{name: "full limit", startErr: app.ErrRateLimited, want: app.ErrRateLimited, wantLimit: true},
		{name: "limit unavailable", startErr: app.ErrLimitUnavailable, want: app.ErrLimitUnavailable, wantLimit: true},
		{name: "database unavailable", session: errors.New("private database"), want: app.ErrReauthenticationUnavailable},
		{name: "attempt write failure", create: errors.New("private insert"), want: app.ErrReauthenticationUnavailable, wantLimit: true},
		{name: "attempt write timeout", create: context.DeadlineExceeded, want: context.DeadlineExceeded, wantLimit: true},
		{name: "commit failure", commit: errors.New("private commit"), want: app.ErrReauthenticationUnavailable, wantLimit: true, created: true},
		{name: "canceled commit", commit: context.Canceled, want: context.Canceled, wantLimit: true, created: true},
		{name: "commit timeout", commit: context.DeadlineExceeded, want: context.DeadlineExceeded, wantLimit: true, created: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newProviderReauthenticationFixture(t)
			r := f.repository
			r.sessionErr, r.createErr, r.commitErr, f.startErr = test.session, test.create, test.commit, test.startErr
			input := f.startInput("github")
			if test.provider != "" {
				input.Provider = test.provider
			}
			if test.subject != "" || test.sessionID != "" {
				input.Subject, input.SessionID = test.subject, test.sessionID
			}
			result, err := f.service.StartProviderReauthentication(t.Context(), input)
			if !errors.Is(err, test.want) || result != (inbound.StartProviderLoginResult{}) || r.commits != 0 {
				t.Fatalf("result = %+v, error = %v, want %v, commits = %d", result, err, test.want, r.commits)
			}
			if (len(f.started) == 1) != test.wantLimit || (len(r.attempts) == 1) != test.created || len(f.attempts.attempts) != 0 {
				t.Fatalf("limits = %d, attempts = %d", len(f.started), len(r.attempts))
			}
			r.checkStartLocks(t, input)
		})
	}
}

func TestCompleteProviderReauthenticationOpensSessionWindow(t *testing.T) {
	f := newProviderReauthenticationFixture(t)

	result, err := f.service.CompleteProviderReauthentication(t.Context(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	r := f.repository
	if !result.ExpiresAt.Equal(r.now.Add(10*time.Minute)) || len(r.writes) != 1 || !r.onlyFixtureSession() || !r.deadline.Equal(r.now.Add(time.Hour)) {
		t.Fatalf("expiry = %v, locks = %v, writes = %v, deadline = %v", result.ExpiresAt, r.locks, r.writes, r.deadline)
	}
	wantAttempt := domain.ProviderSecretVerifier(f.key, domain.ProviderSecretAttemptToken, f.input.AttemptToken)
	wantCode := domain.ProviderSecretVerifier(f.key, domain.ProviderSecretHandoffCode, f.input.HandoffCode)
	if len(r.claims) != 1 || r.claims[0] != "subject-1:session-1" || r.claimedAttempt != wantAttempt || r.claimedCode != wantCode {
		t.Fatalf("claims = %v", r.claims)
	}
	if len(r.links) != 1 || r.links[0] != "subject-1:github:github-subject" || len(f.failures) != 0 || r.commits != 1 {
		t.Fatalf("links = %v, failures = %v, commits = %d", r.links, f.failures, r.commits)
	}
}

func TestCompleteProviderReauthenticationKeepsOpenWindow(t *testing.T) {
	f := newProviderReauthenticationFixture(t)
	opened := f.repository.now.Add(-time.Minute)
	f.repository.window.AuthenticatedAt = opened

	result, err := f.service.CompleteProviderReauthentication(t.Context(), f.input)
	if err != nil || !result.ExpiresAt.Equal(opened.Add(10*time.Minute)) || len(f.repository.writes) != 0 || !f.repository.onlyFixtureSession() {
		t.Fatalf("expiry = %v, error = %v, writes = %v", result.ExpiresAt, err, f.repository.writes)
	}
}

func TestCompleteProviderReauthenticationRejectsUnlinkedProviderAccount(t *testing.T) {
	f := newProviderReauthenticationFixture(t)
	f.repository.linked = false

	result, err := f.service.CompleteProviderReauthentication(t.Context(), f.input)
	if !errors.Is(err, app.ErrProviderAccountUnavailable) || !result.ExpiresAt.IsZero() {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	// The claim commits, so the handoff code cannot be retried.
	if len(f.repository.writes) != 0 || f.repository.commits != 1 || len(f.failures) != 0 || len(f.attempts.handoffFails) != 0 {
		t.Fatalf("writes = %v, commits = %d, failures = %v", f.repository.writes, f.repository.commits, f.failures)
	}
}

func TestCompleteProviderReauthenticationCountsRejectedHandoff(t *testing.T) {
	for _, test := range []struct {
		name                    string
		recordErr, failureErr   error
		want                    error
		wantFailure, wantRecord bool
	}{
		{name: "rejected", want: app.ErrProviderHandoffRejected, wantFailure: true},
		{name: "full failure limit", failureErr: app.ErrRateLimited, want: app.ErrRateLimited, wantFailure: true},
		{name: "failure limit unavailable", failureErr: app.ErrLimitUnavailable, want: app.ErrLimitUnavailable, wantFailure: true},
		{name: "failure record unavailable", recordErr: errors.New("private failure record"), want: app.ErrReauthenticationUnavailable},
		{name: "failure record timeout", recordErr: context.DeadlineExceeded, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newProviderReauthenticationFixture(t)
			f.repository.claimed, f.failureErr, f.attempts.handoffErr = false, test.failureErr, test.recordErr

			result, err := f.service.CompleteProviderReauthentication(t.Context(), f.input)
			if !errors.Is(err, test.want) || !result.ExpiresAt.IsZero() {
				t.Fatalf("result = %+v, error = %v, want %v", result, err, test.want)
			}
			wantAttempt := domain.ProviderSecretVerifier(f.key, domain.ProviderSecretAttemptToken, f.input.AttemptToken)
			if len(f.attempts.handoffFails) != 1 || f.attempts.handoffFails[0] != wantAttempt {
				t.Fatalf("handoff failures = %d", len(f.attempts.handoffFails))
			}
			if test.wantFailure != (len(f.failures) == 1 && f.failures[0] == "192.0.2.1") {
				t.Fatalf("failure limits = %v", f.failures)
			}
			if len(f.repository.writes) != 0 || f.repository.commits != 0 || len(f.repository.links) != 0 {
				t.Fatalf("writes = %v, commits = %d, links = %v", f.repository.writes, f.repository.commits, f.repository.links)
			}
		})
	}
}

func TestCompleteProviderReauthenticationRejectsMalformedProofsFirst(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*inbound.CompleteProviderReauthenticationInput)
		want error
	}{
		{"missing subject", func(input *inbound.CompleteProviderReauthenticationInput) { input.Subject = "" }, outbound.ErrUnauthenticated},
		{"missing session", func(input *inbound.CompleteProviderReauthenticationInput) { input.SessionID = "" }, outbound.ErrUnauthenticated},
		{"attempt token", func(input *inbound.CompleteProviderReauthenticationInput) { input.AttemptToken = "short" }, domain.ErrInvalidAttemptToken},
		{"handoff code", func(input *inbound.CompleteProviderReauthenticationInput) { input.HandoffCode = "short" }, domain.ErrInvalidHandoffCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newProviderReauthenticationFixture(t)
			test.edit(&f.input)
			if _, err := f.service.CompleteProviderReauthentication(t.Context(), f.input); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			r := f.repository
			if r.transactions != 0 || len(r.locks) != 0 || len(r.claims) != 0 || len(f.failures) != 0 || len(f.attempts.handoffFails) != 0 {
				t.Fatal("malformed input reached a store")
			}
		})
	}
}

func TestCompleteProviderReauthenticationFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name                                string
		session, claim, link, write, commit error
		want                                error
		claims, links, writes               int
	}{
		{name: "inactive session", session: outbound.ErrUnauthenticated, want: outbound.ErrUnauthenticated},
		{name: "database unavailable", session: errors.New("private database"), want: app.ErrReauthenticationUnavailable},
		{name: "session lock timeout", session: context.DeadlineExceeded, want: context.DeadlineExceeded},
		{name: "claim failure", claim: errors.New("private claim"), want: app.ErrReauthenticationUnavailable, claims: 1},
		{name: "link failure", link: errors.New("private link"), want: app.ErrReauthenticationUnavailable, claims: 1, links: 1},
		{name: "write failure", write: errors.New("private write"), want: app.ErrReauthenticationUnavailable, claims: 1, links: 1, writes: 1},
		{name: "session expired before commit", commit: outbound.ErrReauthenticationRequired, want: outbound.ErrUnauthenticated, claims: 1, links: 1, writes: 1},
		{name: "commit failure", commit: errors.New("private commit"), want: app.ErrReauthenticationUnavailable, claims: 1, links: 1, writes: 1},
		{name: "canceled commit", commit: context.Canceled, want: context.Canceled, claims: 1, links: 1, writes: 1},
		{name: "commit timeout", commit: context.DeadlineExceeded, want: context.DeadlineExceeded, claims: 1, links: 1, writes: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newProviderReauthenticationFixture(t)
			r := f.repository
			r.sessionErr, r.claimErr, r.linkErr, r.writeErr, r.commitErr = test.session, test.claim, test.link, test.write, test.commit
			result, err := f.service.CompleteProviderReauthentication(t.Context(), f.input)
			if !errors.Is(err, test.want) || !result.ExpiresAt.IsZero() || r.commits != 0 {
				t.Fatalf("result = %+v, error = %v, want %v, commits = %d", result, err, test.want, r.commits)
			}
			if len(r.claims) != test.claims || len(r.links) != test.links || len(r.writes) != test.writes {
				t.Fatalf("claims = %v, links = %v, writes = %v", r.claims, r.links, r.writes)
			}
			if len(f.failures) != 0 || len(f.attempts.handoffFails) != 0 {
				t.Fatal("a dependency failure counted as a rejected handoff")
			}
		})
	}
}

func TestProviderReauthenticationWithoutDependenciesIsUnavailable(t *testing.T) {
	f := newProviderReauthenticationFixture(t)
	login := func(attempts outbound.ProviderAttemptRepository, sourceLimit func(context.Context, string) error, key []byte) *app.ProviderLoginService {
		return app.NewProviderLoginService(attempts, sourceLimit, allowLimit, key, providerClients)
	}
	working := login(f.attempts, allowLimit, f.key)
	for _, test := range []struct {
		name       string
		service    *app.ProviderReauthenticationService
		start      bool
		completion bool
	}{
		{"login", app.NewProviderReauthenticationService(nil, f.repository, f.failureLimit), true, true},
		{"repository", app.NewProviderReauthenticationService(working, nil, f.failureLimit), true, true},
		{"failure limit", app.NewProviderReauthenticationService(working, f.repository, nil), false, true},
		{"attempt repository", app.NewProviderReauthenticationService(login(nil, allowLimit, f.key), f.repository, f.failureLimit), false, true},
		{"start limit", app.NewProviderReauthenticationService(login(f.attempts, nil, f.key), f.repository, f.failureLimit), true, false},
		{"verifier key", app.NewProviderReauthenticationService(login(f.attempts, allowLimit, nil), f.repository, f.failureLimit), true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			f.repository.transactions, f.repository.attempts = 0, nil
			started, err := test.service.StartProviderReauthentication(t.Context(), f.startInput("github"))
			if test.start && (!errors.Is(err, app.ErrReauthenticationUnavailable) || len(f.repository.attempts) != 0) ||
				!test.start && (err != nil || started.AttemptToken == "" || len(f.repository.attempts) != 1) {
				t.Fatalf("start = %+v, error = %v, attempts = %d", started, err, len(f.repository.attempts))
			}
			f.repository.transactions = 0
			f.repository.window.AuthenticatedAt = time.Time{}
			completed, err := test.service.CompleteProviderReauthentication(t.Context(), f.input)
			if test.completion && (!errors.Is(err, app.ErrReauthenticationUnavailable) || f.repository.transactions != 0) ||
				!test.completion && (err != nil || !completed.ExpiresAt.Equal(f.repository.now.Add(10*time.Minute))) {
				t.Fatalf("complete = %+v, error = %v, transactions = %d", completed, err, f.repository.transactions)
			}
		})
	}
}
