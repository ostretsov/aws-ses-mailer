package main

import "testing"

func TestTrimFields(t *testing.T) {
	email := email{
		To:       " email1@test.com, email2@test.com ,  email3@test.com",
		Cc:       " email4@test.com, email5@test.com ,  email6@test.com",
		Subject:  "      test       subject ",
		HTMLBody: "  html body ",
		TextBody: "  text body ",
		Attaches: []emailAttach{
			{
				" file_name.pdf ",
				" file_content ",
			},
		},
	}

	email.trimFields()

}

func TestValidate(t *testing.T) {
	testCases := []struct {
		email              email
		valid              bool
		validationErrorMsg string
	}{
		{
			email{},
			false,
			"there must be at least one recipient",
		},
		{
			email{To: "something"},
			false,
			`"something" is not valid email`,
		},
		{
			email{To: "valid@email.com,invalid"},
			false,
			`"invalid" is not valid email`,
		},
		{
			email{To: "valid@email.com"},
			false,
			`subject must not be empty`,
		},
		{
			email{To: "valid@email.com", Subject: "Wow"},
			false,
			`at least text_body must be set`,
		},
		{
			email{To: "valid@email.com", Subject: "Wow", HTMLBody: "html body"},
			true,
			"",
		},
		{
			email{To: "valid@email.com", Subject: "Wow", TextBody: "text body"},
			true,
			"",
		},
		{
			email{To: "valid@email.com", Subject: "Wow", HTMLBody: "html body", TextBody: "text body"},
			true,
			"",
		},
		{
			email{To: "valid@email.com", Subject: "Wow", HTMLBody: "html body", TextBody: "text body", Cc: "invalid"},
			false,
			`"invalid" is not valid carbon copy email`,
		},
		{
			email{To: "valid@email.com", Subject: "Wow", HTMLBody: "html body", TextBody: "text body", Cc: "valid@cc.com,invalid2"},
			false,
			`"invalid2" is not valid carbon copy email`,
		},
		{
			email{To: "valid@email.com", Subject: "Wow", HTMLBody: "html body", TextBody: "text body", Cc: "valid@cc.com,invalid2@cc.com"},
			true,
			"",
		},
	}

	for _, testCase := range testCases {
		err := testCase.email.validate()
		if testCase.valid && err != nil {
			t.Fatalf("%#v must be valid, but got %s", testCase, err)
		}
		if !testCase.valid {
			if err == nil {
				t.Fatalf("%#v must not be valid", testCase)
			}
			if err.Error() != testCase.validationErrorMsg {
				t.Fatalf("%#v must emit validation error %s, got %s", testCase, testCase.validationErrorMsg, err)
			}
		}
	}
}
