package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/streadway/amqp"
	"gopkg.in/gomail.v2"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	errAWSSessionCreation = errors.New("aws session creation error")
	emailRegexp           = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
)

type errAWSSendingEmail struct {
	err error
}

func (e errAWSSendingEmail) Error() string {
	return "aws sending email error: " + e.err.Error()
}

func (e errAWSSendingEmail) Unwrap() error {
	return e.err
}

type email struct {
	To       string        `json:"to"`
	Cc       string        `json:"cc"`
	Subject  string        `json:"subject"`
	HTMLBody string        `json:"html_body"`
	TextBody string        `json:"text_body"`
	Attaches []emailAttach `json:"attaches"`
}

type emailAttach struct {
	FileName                 string `json:"file_name"`
	FileContentBase64Encoded string `json:"file_content_base64_encoded"`
}

func (e *email) trimFields() {
	tos := strings.Split(e.To, ",")
	for i, to := range tos {
		tos[i] = strings.TrimSpace(to)
	}
	e.To = strings.Join(tos, ",")

	if len(e.Cc) > 0 {
		carbonCopies := strings.Split(e.Cc, ",")
		for i, cc := range carbonCopies {
			carbonCopies[i] = strings.TrimSpace(cc)
		}
		e.Cc = strings.Join(carbonCopies, ",")
	}

	e.Subject = strings.TrimSpace(e.Subject)
	e.HTMLBody = strings.TrimSpace(e.HTMLBody)
	e.TextBody = strings.TrimSpace(e.TextBody)

	for i, attach := range e.Attaches {
		e.Attaches[i].FileName = strings.TrimSpace(attach.FileName)
		e.Attaches[i].FileContentBase64Encoded = strings.TrimSpace(attach.FileContentBase64Encoded)
	}
}

func (e *email) validate() error {
	if len(e.To) == 0 {
		return errors.New("there must be at least one recipient")
	}
	specifiedDestEmails := map[string]bool{}
	tos := strings.Split(e.To, ",")
	for _, to := range tos {
		if !emailRegexp.MatchString(to) {
			return fmt.Errorf(`"%s" is not valid email`, to)
		}
		if _, ok := specifiedDestEmails[to]; ok {
			return fmt.Errorf(`"%s" is used twice`, to)
		}
		specifiedDestEmails[to] = true
	}

	if len(e.Cc) > 0 {
		ccopies := strings.Split(e.Cc, ",")
		for _, cc := range ccopies {
			if !emailRegexp.MatchString(cc) {
				return fmt.Errorf(`"%s" is not valid carbon copy email`, cc)
			}
			if _, ok := specifiedDestEmails[cc]; ok {
				return fmt.Errorf(`"%s" is used twice`, cc)
			}
			specifiedDestEmails[cc] = true
		}
	}

	if len(e.Subject) == 0 {
		return errors.New("subject must not be empty")
	}

	if len(e.HTMLBody) == 0 && len(e.TextBody) == 0 {
		return errors.New("at least text_body must be set")
	}

	return nil
}

func main() {
	getEnv("AMQP_URL")
	getEnv("AMQP_QUEUE")
	getEnv("AWS_VERIFIED_FROM_EMAIL_ADDRESS")

	for message := range rabbitMQMessageChan() {
		emailToSendMessage := &email{}
		err := json.Unmarshal(message.Body, emailToSendMessage)
		if err != nil {
			message.Nack(false, true)
			log.Fatal("message could not be decoded", message.Body)
		}
		log.Println("new email message:", emailToSendMessage.Subject, emailToSendMessage.To)
		emailToSendMessage.trimFields()
		err = emailToSendMessage.validate()
		if err != nil {
			message.Nack(false, true)
			log.Fatal("validation error", err)
		}

		err = sendEmail(emailToSendMessage)
		if err != nil {
			if err == errAWSSessionCreation {
				message.Nack(false, true)
				log.Fatal("message could not be decoded", message.Body)
			}
			if _, ok := err.(*errAWSSendingEmail); ok {
				log.Println(err)
				message.Nack(false, true)
				time.Sleep(5 * time.Minute)
				continue
			}
		}

		message.Ack(false)
		log.Println("email message successfully sent", emailToSendMessage.Subject, emailToSendMessage.To)
	}
}

func sendEmail(emailToSendMessage *email) error {
	fromAddress := getEnv("AWS_VERIFIED_FROM_EMAIL_ADDRESS")
	sess, err := session.NewSession()
	if err != nil {
		return errAWSSessionCreation
	}
	svc := ses.New(sess)

	email := gomail.NewMessage()
	email.SetHeader("From", fromAddress)
	email.SetHeader("To", strings.Split(emailToSendMessage.To, ",")...)
	if len(emailToSendMessage.Cc) > 0 {
		cc := strings.Split(emailToSendMessage.Cc, ",")
		email.SetHeader("Cc", cc...)
	}
	email.SetHeader("Subject", emailToSendMessage.Subject)
	if len(emailToSendMessage.HTMLBody) > 0 {
		email.SetBody("text/html", emailToSendMessage.HTMLBody)
	}
	if len(emailToSendMessage.TextBody) > 0 {
		email.SetBody("text/plain", emailToSendMessage.TextBody)
	}
	for _, attach := range emailToSendMessage.Attaches {
		base64EncodedContent := attach.FileContentBase64Encoded
		email.Attach(attach.FileName, gomail.SetCopyFunc(func(w io.Writer) error {
			fileContentDecoded, err := base64.StdEncoding.DecodeString(base64EncodedContent)
			if err != nil {
				return err
			}
			_, err = w.Write(fileContentDecoded)
			return err
		}))
	}

	var emailRaw bytes.Buffer
	email.WriteTo(&emailRaw)
	input := &ses.SendRawEmailInput{
		RawMessage: &ses.RawMessage{Data: emailRaw.Bytes()},
	}

	_, err = svc.SendRawEmail(input)

	if err != nil {
		return errAWSSendingEmail{err: err}
	}
	return nil
}

func rabbitMQMessageChan() <-chan amqp.Delivery {
	amqpUrl := getEnv("AMQP_URL")
	amqpQueueName := getEnv("AMQP_QUEUE")
	var amqpConn *amqp.Connection
	for {
		conn, err := amqp.Dial(amqpUrl)
		if err != nil {
			log.Println("dial err", err)
			time.Sleep(1 * time.Second)
			continue
		}
		amqpConn = conn
		break
	}
	amqpChannel, err := amqpConn.Channel()
	if err != nil {
		log.Fatal("channel init err", err)
	}
	amqpChannel.Qos(1, 0, false)
	amqpQueue, err := amqpChannel.QueueDeclare(amqpQueueName, true, false, false, false, nil)
	if err != nil {
		log.Fatal("queue declaration err", err)
	}

	messageChannel, err := amqpChannel.Consume(amqpQueue.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatal("message consumption err", err)
	}

	return messageChannel
}

func getEnv(k string) (v string) {
	v = os.Getenv(k)
	if v == "" {
		log.Fatalf("%v must be set\n", k)
	}
	return v
}
