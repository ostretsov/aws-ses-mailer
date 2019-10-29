package main

import (
	"encoding/json"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/streadway/amqp"
	"log"
	"os"
)

const (
	charSet = "UTF-8"
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

		session, err := session.NewSession(&aws.Config{
			Region: aws.String("us-east-1")},
		)
		awsSes := ses.New(session)
		input := &ses.SendEmailInput{
			Destination: &ses.Destination{
				CcAddresses: []*string{},
				ToAddresses: []*string{
					aws.String(email),
				},
			},
			Message: &ses.Message{
				Body: &ses.Body{
					Html: &ses.Content{
						Charset: aws.String(charSet),
						Data:    aws.String(message),
					},
				},
				Subject: &ses.Content{
					Charset: aws.String(charSet),
					Data:    aws.String(fromAddress),
				},
			},
			Source: aws.String(sender),
		}
		result, err := awsSes.SendEmail(input)
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
