package hibp

import (
	"bufio"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ErrPasswordCheck = errors.New("password check unavailable")

type PasswordChecker struct{ client *http.Client }

func NewPasswordChecker(client *http.Client) *PasswordChecker {
	if client == nil {
		client = &http.Client{}
	}
	configured := *client
	configured.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &PasswordChecker{client: &configured}
}

func (c *PasswordChecker) Compromised(ctx context.Context, password string) (bool, error) {
	if !crypto.SHA1.Available() {
		return false, ErrPasswordCheck
	}
	hasher := crypto.SHA1.New()
	_, _ = hasher.Write([]byte(password))
	full := strings.ToUpper(hex.EncodeToString(hasher.Sum(nil)))
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.pwnedpasswords.com/range/"+full[:5], nil)
	if err != nil {
		return false, ErrPasswordCheck
	}
	request.Header.Set("Add-Padding", "true")
	response, err := c.client.Do(request)
	if err != nil {
		return false, ErrPasswordCheck
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return false, ErrPasswordCheck
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return false, ErrPasswordCheck
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	for scanner.Scan() {
		suffix, count, ok := strings.Cut(strings.TrimSpace(scanner.Text()), ":")
		if !ok || len(suffix) != 35 {
			return false, ErrPasswordCheck
		}
		if strings.EqualFold(suffix, full[5:]) {
			found, err := strconv.ParseUint(count, 10, 64)
			if err != nil {
				return false, ErrPasswordCheck
			}
			return found > 0, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, ErrPasswordCheck
	}
	return false, nil
}
