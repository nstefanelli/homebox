// Package mailer provides a simple mailer for sending emails.
package mailer

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"time"
)

const defaultSendTimeout = 30 * time.Second

type Mailer struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
}

func (m *Mailer) Ready() bool {
	return m.Host != "" && m.Port != 0 && m.Username != "" && m.Password != "" && m.From != ""
}

func (m *Mailer) server() string {
	return m.Host + ":" + strconv.Itoa(m.Port)
}

func (m *Mailer) Send(msg *Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultSendTimeout)
	defer cancel()
	return m.SendContext(ctx, msg)
}

func buildMessage(msg *Message) []byte {
	header := make(map[string]string)
	header["From"] = msg.From.String()
	header["To"] = msg.To.String()
	header["Subject"] = mime.QEncoding.Encode("UTF-8", msg.Subject)
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(msg.Body))
	return []byte(message)
}

// SendContext sends a message while honoring cancellation and deadlines for
// every network operation. The standard library's smtp.SendMail has no
// context-aware variant and can otherwise block indefinitely on a stalled
// server.
func (m *Mailer) SendContext(ctx context.Context, msg *Message) (sendErr error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultSendTimeout)
		defer cancel()
	}
	defer func() {
		if sendErr != nil && ctx.Err() != nil {
			sendErr = ctx.Err()
		}
	}()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", m.server())
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}
	stopCancelWatch := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	defer stopCancelWatch()

	client, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: m.Host,
		}); err != nil {
			return err
		}
	}

	if err := client.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
		return err
	}
	if err := client.Mail(m.From); err != nil {
		return err
	}
	if err := client.Rcpt(msg.To.Address); err != nil {
		return err
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(buildMessage(msg)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return client.Quit()
}
