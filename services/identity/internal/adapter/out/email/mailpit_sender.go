package email

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"time"

	"github.com/vasapolrittideah/flowspace-api/services/identity/internal/domain"
	outbound "github.com/vasapolrittideah/flowspace-api/services/identity/internal/port/out"
)

var ErrMailDelivery = errors.New("mail delivery failed")

type MailpitSender struct {
	address string
	from    string
}

var _ outbound.EmailSender = (*MailpitSender)(nil)

func NewMailpitSender(address, from string) (*MailpitSender, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return nil, ErrMailDelivery
	}
	normalized, err := domain.NormalizeEmail(from)
	if err != nil {
		return nil, ErrMailDelivery
	}
	return &MailpitSender{address: address, from: normalized}, nil
}

func (s *MailpitSender) Send(ctx context.Context, recipient, code, purpose string) error {
	address, err := domain.NormalizeEmail(recipient)
	if err != nil || len(code) != 6 {
		return ErrMailDelivery
	}
	for i := range len(code) {
		if code[i] < '0' || code[i] > '9' {
			return ErrMailDelivery
		}
	}
	var subject string
	switch purpose {
	case string(domain.PurposeVerifyEmail):
		subject = "FlowSpace email verification code"
	case string(domain.PurposeClaimAccount):
		subject = "FlowSpace account claim code"
	default:
		return ErrMailDelivery
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("%w: dial", ErrMailDelivery)
	}
	defer func() { _ = conn.Close() }()
	deadline := time.Now().Add(5 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("%w: deadline", ErrMailDelivery)
	}
	host, _, _ := net.SplitHostPort(s.address)
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("%w: greeting", ErrMailDelivery)
	}
	defer func() { _ = client.Close() }()
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("%w: sender", ErrMailDelivery)
	}
	if err := client.Rcpt(address); err != nil {
		return fmt.Errorf("%w: recipient", ErrMailDelivery)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("%w: data", ErrMailDelivery)
	}
	if _, err := fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYour FlowSpace code is %s. It expires in 10 minutes.\r\n", s.from, address, subject, code); err != nil {
		_ = writer.Close()
		return fmt.Errorf("%w: write", ErrMailDelivery)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("%w: accepted", ErrMailDelivery)
	}
	_ = client.Quit()
	return nil
}
