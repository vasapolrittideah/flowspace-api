package identityv1

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
)

func TestCreateAccountReportsUnverifiedState(t *testing.T) {
	unverified := false
	encoded, err := protojson.Marshal(&CreateAccountResponse{EmailVerified: &unverified})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"emailVerified":false`) {
		t.Fatalf("signup response = %s", encoded)
	}
}

func TestAuthenticatedRequestsCannotSelectAccount(t *testing.T) {
	if fields := (&RequestEmailVerificationCodeRequest{}).ProtoReflect().Descriptor().Fields(); fields.Len() != 0 {
		t.Fatalf("code request has %d client fields", fields.Len())
	}
	if fields := (&VerifyEmailRequest{}).ProtoReflect().Descriptor().Fields(); fields.Len() != 1 || string(fields.Get(0).Name()) != "code" {
		t.Fatalf("verification request has unexpected fields: %v", fields)
	}
}

func TestCodeFieldsPreserveLeadingZeroes(t *testing.T) {
	verification := &VerifyEmailRequest{Code: "000123"}
	encoded, err := protojson.Marshal(verification)
	if err != nil {
		t.Fatal(err)
	}
	var decoded VerifyEmailRequest
	if err := protojson.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.GetCode() != "000123" {
		t.Fatalf("verification code = %q", decoded.GetCode())
	}

	claim := &ClaimUnverifiedAccountRequest{Code: "000123"}
	encoded, err = protojson.Marshal(claim)
	if err != nil {
		t.Fatal(err)
	}
	var decodedClaim ClaimUnverifiedAccountRequest
	if err := protojson.Unmarshal(encoded, &decodedClaim); err != nil {
		t.Fatal(err)
	}
	if decodedClaim.GetCode() != "000123" {
		t.Fatalf("claim code = %q", decodedClaim.GetCode())
	}
}
