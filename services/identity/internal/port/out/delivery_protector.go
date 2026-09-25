package outbound

type DeliveryProtector interface {
	Protect(challengeID, purpose, subject, email, code string) (DeliveryMaterial, error)
}
