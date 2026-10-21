package outbound

type AccountState struct {
	Email         string
	EmailVerified bool
}

type ClaimAccount struct {
	Subject       string
	EmailVerified bool
}

type PasswordAccount struct {
	Subject       string
	PasswordHash  string
	EmailVerified bool
}

type PasswordRecoveryAccount struct {
	Subject       string
	EmailVerified bool
	HasPassword   bool
}
