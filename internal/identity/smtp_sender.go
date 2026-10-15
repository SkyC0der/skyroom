package identity

import (
	"context"
	"fmt"
	"net/smtp"
)

type SMTPCodeSender struct {
	Address string
	From    string
}

func (s SMTPCodeSender) SendVerificationCode(ctx context.Context, recipient, code string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	message := []byte("To: " + recipient + "\r\n" +
		"From: " + s.From + "\r\n" +
		"Subject: Your Skyroom verification code\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"Your Skyroom verification code is " + code + ".\r\n" +
		"It expires in 10 minutes. If you did not request it, ignore this email.\r\n")
	if err := smtp.SendMail(s.Address, nil, s.From, []string{recipient}, message); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}
