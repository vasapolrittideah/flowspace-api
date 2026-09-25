package outbound

type DeliveryProtector interface {
	Protect(challengeID, purpose, subject, email, code string) (DeliveryMaterial, error)
}

type DeliveryOpener interface {
	Open(challengeID, purpose, subject string, material DeliveryMaterial) (string, string, error)
}
