Environment variables:
```
AMQP_URL: amqp://guest:guest@rabbitmq:5672/
AMQP_QUEUE: aws.ses.mailer
AWS_REGION: us-east-1
AWS_ACCESS_KEY_ID: AKIAVXXXXXXX
AWS_SECRET_ACCESS_KEY: xkM4TSomXXXXXXXXXXXXXXXXXXXXXXXX
AWS_VERIFIED_FROM_EMAIL_ADDRESS: YOURVERIFIED@EMAIL.COM
```

Expected queue message:
```json
{
  "to": "OstretsovAA@gmail.com,someone@else.com",
  "cc": "sendcopy@here.com,and@here.com",
  "html_body": "<strong>html</strong> body",
  "text_body": "text body",
  "subject": "test message",
  "attaches": [
    {
      "base64_encoded_file_content": "iVBORw0KGgoAAAANSUhEUgAAABYAAAAXCAIAAACAiijJAAAACXBIWXMAAA7EAAAOxAGVKw4bAAAAIElEQVQ4jWP8//8/A2WAiUL9o0aMGjFqxKgRo0YMlBEAiH0DK1dDnUsAAAAASUVORK5CYII=",
      "file_name": "stub.png"
    }
  ]
}
```