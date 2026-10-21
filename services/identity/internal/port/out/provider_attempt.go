package outbound

// ProviderAttempt binds 1 provider attempt to its proofs. The repository
// stores the attempt token and state only as keyed verifiers. A
// re-authentication attempt also binds the subject and session that
// started it. A login attempt leaves both empty.
type ProviderAttempt struct {
	Subject              string
	SessionID            string
	Provider             string
	AttemptTokenVerifier [32]byte
	StateVerifier        [32]byte
	CodeVerifier         string
	Nonce                string
	CallbackURL          string
}
