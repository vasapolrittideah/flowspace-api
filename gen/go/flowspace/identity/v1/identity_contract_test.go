package identityv1

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/grpc/codes"
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

type publicRPC struct {
	name, route       string
	request, response []string
}

func TestPasswordRecoveryRPCContract(t *testing.T) {
	checkPublicRPCs(t, []publicRPC{
		{"RequestPasswordResetCode", "/v1/password-reset-codes", []string{"email"}, []string{"accepted"}},
		{"ResetPassword", "/v1/password-resets", []string{"email", "code", "new_password"}, []string{"password_changed", "sessions_revoked"}},
	})
}

func checkPublicRPCs(t *testing.T, tests []publicRPC) {
	t.Helper()
	service := File_flowspace_identity_v1_identity_service_proto.Services().ByName("IdentityService")
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
			rule, ok := proto.GetExtension(method.Options(), annotations.E_Http).(*annotations.HttpRule)
			if !ok || rule.GetPost() != test.route || rule.GetBody() != "*" {
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

func TestPasswordRecoveryRequestJSONRejectsMalformedInput(t *testing.T) {
	for _, message := range []proto.Message{&RequestPasswordResetCodeRequest{}, &ResetPasswordRequest{}} {
		for _, input := range []string{`{"email":`, `{"email":123}`} {
			if err := protojson.Unmarshal([]byte(input), message); err == nil {
				t.Fatalf("accepted malformed request %q", input)
			}
		}
	}
}

func TestPasswordRecoveryUsesCanonicalHTTPStatuses(t *testing.T) {
	for _, test := range []struct {
		code codes.Code
		want int
	}{
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.ResourceExhausted, http.StatusTooManyRequests},
		{codes.Unavailable, http.StatusServiceUnavailable},
	} {
		if got := runtime.HTTPStatusFromCode(test.code); got != test.want {
			t.Fatalf("gRPC %s maps to HTTP %d, want %d", test.code, got, test.want)
		}
	}
}

func TestProviderLoginRPCContract(t *testing.T) {
	checkPublicRPCs(t, []publicRPC{
		{"StartProviderLogin", "/v1/provider-login-attempts", []string{"provider"}, []string{"authorization_url", "attempt_token", "attempt_expires_at"}},
		{"CreateProviderSession", "/v1/provider-sessions", []string{"attempt_token", "handoff_code"}, messageFields((&CreatePasswordSessionResponse{}).ProtoReflect().Descriptor())},
	})
}

func TestProviderLoginAcceptsOnlyGoogleAndGitHub(t *testing.T) {
	values := Provider(0).Descriptor().Values()
	names := make([]string, values.Len())
	for i := range names {
		names[i] = string(values.Get(i).Name())
	}
	if want := []string{"PROVIDER_UNSPECIFIED", "PROVIDER_GOOGLE", "PROVIDER_GITHUB"}; !slices.Equal(names, want) {
		t.Fatalf("providers = %v, want %v", names, want)
	}
	var request StartProviderLoginRequest
	if err := protojson.Unmarshal([]byte(`{"provider":"PROVIDER_OKTA"}`), &request); err == nil {
		t.Fatal("accepted an unknown provider")
	}
}

func TestProviderLoginFailureUsesHTTP400(t *testing.T) {
	if got := runtime.HTTPStatusFromCode(codes.FailedPrecondition); got != http.StatusBadRequest {
		t.Fatalf("gRPC FailedPrecondition maps to HTTP %d, want 400", got)
	}
}
