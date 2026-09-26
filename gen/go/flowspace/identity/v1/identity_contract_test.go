package identityv1

import (
	"slices"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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

func TestSessionRPCContract(t *testing.T) {
	service := File_flowspace_identity_v1_identity_service_proto.Services().ByName("IdentityService")
	tests := []struct {
		name, route       string
		request, response []string
	}{
		{"CreatePasswordSession", "/v1/password-sessions", []string{"email", "password"}, []string{"subject", "email_verified", "access_token", "refresh_token", "access_token_expires_at", "refresh_token_expires_at", "session_expires_at"}},
		{"RefreshSession", "/v1/session-refreshes", []string{"refresh_token"}, []string{"access_token", "refresh_token", "access_token_expires_at", "refresh_token_expires_at", "session_expires_at"}},
		{"LogoutCurrentSession", "/v1/session-logouts", nil, []string{"revoked"}},
		{"LogoutAllSessions", "/v1/account-session-logouts", nil, []string{"revoked"}},
		{"CheckSession", "", []string{"subject", "session_id"}, []string{"email_verified"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			method := service.Methods().ByName(protoreflect.Name(test.name))
			if method == nil {
				t.Fatal("RPC missing")
			}
			if got := messageFields(method.Input()); !slices.Equal(got, test.request) {
				t.Fatalf("request fields = %v, want %v", got, test.request)
			}
			if got := messageFields(method.Output()); !slices.Equal(got, test.response) {
				t.Fatalf("response fields = %v, want %v", got, test.response)
			}
			if test.route == "" {
				if proto.HasExtension(method.Options(), annotations.E_Http) {
					t.Fatal("internal RPC has a public HTTP route")
				}
				return
			}
			if !proto.HasExtension(method.Options(), annotations.E_Http) {
				t.Fatal("public RPC has no HTTP route")
			}
			rule, ok := proto.GetExtension(method.Options(), annotations.E_Http).(*annotations.HttpRule)
			if !ok {
				t.Fatal("public RPC has an invalid HTTP rule")
			}
			if rule.GetPost() != test.route || rule.GetBody() != "*" {
				t.Fatalf("HTTP rule = %v, want POST %s with body *", rule, test.route)
			}
		})
	}
}

func messageFields(message protoreflect.MessageDescriptor) []string {
	fields := message.Fields()
	names := make([]string, fields.Len())
	for i := range names {
		names[i] = string(fields.Get(i).Name())
	}
	return names
}
