package mailer

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	TestMailerConfig = "test-mailer.json"
)

func GetTestMailer() (*Mailer, error) {
	// Read JSON File
	bytes, err := os.ReadFile(TestMailerConfig)

	mailer := &Mailer{}

	if err != nil {
		return nil, err
	}

	// Unmarshal JSON
	err = json.Unmarshal(bytes, mailer)

	if err != nil {
		return nil, err
	}

	return mailer, nil
}

func Test_Mailer(t *testing.T) {
	t.Parallel()

	mailer, err := GetTestMailer()
	if err != nil {
		t.Skip("Error Reading Test Mailer Config - Skipping")
	}

	if !mailer.Ready() {
		t.Skip("Mailer not ready - Skipping")
	}

	message, err := RenderWelcome()
	if err != nil {
		t.Error(err)
	}

	mb := NewMessageBuilder().
		SetBody(message).
		SetSubject("Hello").
		SetTo("John Doe", "john@doe.com").
		SetFrom("Jane Doe", "jane@doe.com")

	msg := mb.Build()

	err = mailer.Send(msg)

	require.NoError(t, err)
}

func TestMailerSendContextHonorsDeadline(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		// Deliberately never send the SMTP greeting. SendContext must still
		// return when its context expires.
		var oneByte [1]byte
		_, _ = conn.Read(oneByte[:])
	}()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	testMailer := &Mailer{
		Host:     "127.0.0.1",
		Port:     tcpAddr.Port,
		Username: "user",
		Password: "password",
		From:     "from@example.com",
	}
	msg := NewMessageBuilder().
		SetBody("test").
		SetSubject("test").
		SetTo("Recipient", "to@example.com").
		SetFrom("Sender", testMailer.From).
		Build()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = testMailer.SendContext(ctx, msg)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("stalled SMTP connection did not close after the deadline")
	}
}
