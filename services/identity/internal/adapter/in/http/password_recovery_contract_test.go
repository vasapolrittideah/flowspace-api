package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/vasapolrittideah/flowspace-api/gen/go/flowspace/identity/v1"
	identityhttp "github.com/vasapolrittideah/flowspace-api/services/identity/internal/adapter/in/http"
)

type recoveryContractHandler struct {
	identityv1.UnimplementedIdentityServiceServer
	codeRequest *identityv1.RequestPasswordResetCodeRequest
	reset       *identityv1.ResetPasswordRequest
	err         error
}

func (h *recoveryContractHandler) RequestPasswordResetCode(_ context.Context, request *identityv1.RequestPasswordResetCodeRequest) (*identityv1.RequestPasswordResetCodeResponse, error) {
	h.codeRequest = request
	if h.err != nil {
		return nil, h.err
	}
	return &identityv1.RequestPasswordResetCodeResponse{Accepted: true}, nil
}

func (h *recoveryContractHandler) ResetPassword(_ context.Context, request *identityv1.ResetPasswordRequest) (*identityv1.ResetPasswordResponse, error) {
	h.reset = request
	if h.err != nil {
		return nil, h.err
	}
	return &identityv1.ResetPasswordResponse{PasswordChanged: true, SessionsRevoked: true}, nil
}

func recoveryContractHTTPHandler(t *testing.T, service *recoveryContractHandler) http.Handler {
	t.Helper()
	mux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerServer(t.Context(), mux, service); err != nil {
		t.Fatal(err)
	}
	return identityhttp.NewIdentityRequestHandler(mux, nil)
}

func postRecoveryContract(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.5:1234"
	r.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, r)
	return response
}

func TestPasswordRecoveryRESTRoutes(t *testing.T) {
	service := &recoveryContractHandler{}
	handler := recoveryContractHTTPHandler(t, service)

	response := postRecoveryContract(t, handler, "/v1/password-reset-codes", `{"email":"User@example.com"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"accepted":true`) || service.codeRequest.GetEmail() != "User@example.com" {
		t.Fatalf("code request: status = %d, body = %q, request = %v", response.Code, response.Body.String(), service.codeRequest)
	}
	response = postRecoveryContract(t, handler, "/v1/password-resets", `{"email":"User@example.com","code":"000123","newPassword":"a valid new password"}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"passwordChanged":true`) || !strings.Contains(response.Body.String(), `"sessionsRevoked":true`) ||
		service.reset.GetEmail() != "User@example.com" || service.reset.GetCode() != "000123" || service.reset.GetNewPassword() != "a valid new password" {
		t.Fatalf("reset: status = %d, body = %q, request = %v", response.Code, response.Body.String(), service.reset)
	}
	if strings.Contains(response.Body.String(), "User@example.com") || strings.Contains(response.Body.String(), "000123") ||
		strings.Contains(response.Body.String(), "a valid new password") || strings.Contains(response.Body.String(), "Token") {
		t.Fatalf("reset response exposes request data: %q", response.Body.String())
	}
}

func TestPasswordRecoveryRESTRejectsMalformedJSON(t *testing.T) {
	handler := recoveryContractHTTPHandler(t, &recoveryContractHandler{})
	for _, path := range []string{"/v1/password-reset-codes", "/v1/password-resets"} {
		for _, body := range []string{`{"email":`, `{"email":123}`} {
			if response := postRecoveryContract(t, handler, path, body); response.Code != http.StatusBadRequest {
				t.Fatalf("malformed %s request %s: status = %d, body = %q", path, body, response.Code, response.Body.String())
			}
		}
	}
}

func TestPasswordRecoveryRESTMapsCanonicalErrors(t *testing.T) {
	service := &recoveryContractHandler{}
	handler := recoveryContractHTTPHandler(t, service)
	for _, test := range []struct {
		code codes.Code
		http int
	}{
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.ResourceExhausted, http.StatusTooManyRequests},
		{codes.Unavailable, http.StatusServiceUnavailable},
	} {
		service.err = status.Error(test.code, "safe recovery error")
		for _, path := range []string{"/v1/password-reset-codes", "/v1/password-resets"} {
			body := `{"email":"User@example.com","code":"000123","newPassword":"a valid new password"}`
			response := postRecoveryContract(t, handler, path, body)
			if response.Code != test.http || !strings.Contains(response.Body.String(), "safe recovery error") {
				t.Fatalf("%s error %s: status = %d, body = %q", path, test.code, response.Code, response.Body.String())
			}
		}
	}
}
