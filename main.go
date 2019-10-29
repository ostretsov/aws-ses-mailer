package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
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

const (
	charSet = "UTF-8"
)

var (
	emailRegexp = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
)

type emailToSend struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body"`
	TextBody string `json:"text_body"`
	Attaches []struct {
		FileName                 string `json:"file_name"`
		Base64EncodedFileContent string `json:"base64_encoded_file_content"`
	} `json:"attaches"`
}

func (e *emailToSend) trim() {
	tos := strings.Split(e.To, ",")
	for key, to := range tos {
		tos[key] = strings.TrimSpace(to)
	}
	e.To = strings.Join(tos, ",")

	e.Subject = strings.TrimSpace(e.Subject)
	e.HTMLBody = strings.TrimSpace(e.HTMLBody)
	e.TextBody = strings.TrimSpace(e.TextBody)

	for key, attach := range e.Attaches {
		e.Attaches[key].FileName = strings.TrimSpace(attach.FileName)
		e.Attaches[key].Base64EncodedFileContent = strings.TrimSpace(attach.Base64EncodedFileContent)
	}
}

func (e *emailToSend) validate() error {
	tos := strings.Split(e.To, ",")
	if len(tos) == 0 {
		return errors.New("there must be at least one recipient")
	}

	for _, to := range tos {
		if !emailRegexp.MatchString(to) {
			return fmt.Errorf(`"%s" is not valid email`, to)
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
	amqpUrl := getEnv("AMQP_URL")
	fromAddress := getEnv("AMAZON_VERIFIED_FROM_EMAIL_ADDRESS")
	for message := range setUpRabbitMQ(amqpUrl) {
		emailToSendMessage := &emailToSend{}
		err := json.Unmarshal(message.Body, emailToSendMessage)
		if err != nil {
			message.Nack(false, true)
			log.Fatal("message could not be decoded", message.Body)
		}
		emailToSendMessage.trim()
		err = emailToSendMessage.validate()
		if err != nil {
			message.Nack(false, true)
			log.Fatal("validation error", err)
		}

		sess, err := session.NewSession(&aws.Config{
			Region: aws.String("us-west-2")},
		)
		if err != nil {
			message.Nack(false, true)
			log.Fatal("message could not be decoded", message.Body)
		}
		svc := ses.New(sess)

		email := gomail.NewMessage()
		email.SetHeader("From", fromAddress)
		email.SetHeader("To", strings.Split(emailToSendMessage.To, ",")...)
		email.SetHeader("Subject", emailToSendMessage.Subject)
		if len(emailToSendMessage.HTMLBody) > 0 {
			email.SetBody("text/html", emailToSendMessage.HTMLBody)
		}
		if len(emailToSendMessage.TextBody) > 0 {
			email.SetBody("text/plain", emailToSendMessage.TextBody)
		}
		for _, attach := range emailToSendMessage.Attaches {
			email.Attach(attach.FileName, gomail.SetCopyFunc(func(w io.Writer) error {
				fileContentDecoded, err := base64.StdEncoding.DecodeString(attach.Base64EncodedFileContent)
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
			FromArn:       aws.String(""),
			RawMessage:    &ses.RawMessage{Data: emailRaw.Bytes()},
			ReturnPathArn: aws.String(""),
			Source:        aws.String(""),
			SourceArn:     aws.String(""),
		}

		_, err = svc.SendRawEmail(input)
		if err != nil {
			log.Println(err)
			message.Nack(false, true)
			time.Sleep(5 * time.Minute)
			continue
		}

		message.Ack(false)
	}
}

func setUpRabbitMQ(amqpUrl string) <-chan amqp.Delivery {
	amqpConn, err := amqp.Dial(amqpUrl)
	if err != nil {
		log.Fatal(err)
	}
	amqpChannel, err := amqpConn.Channel()
	if err != nil {
		log.Fatal(err)
	}
	amqpChannel.Qos(1, 0, false)
	amqpQueue, err := amqpChannel.QueueDeclare("manifests_to_ftp_upload", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	messageChannel, err := amqpChannel.Consume(amqpQueue.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
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
