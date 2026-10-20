package sasl

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const UTF8NULL = "\u0000"

const PlainMechanism = "PLAIN"

func PlainAuthMessage(authenticationId, password string) string {
	return fmt.Sprintf("%s%s%s%s", UTF8NULL, authenticationId, UTF8NULL, password)
}

func ParsePlainAuthMessage(encodedAuth string) (authorizationId, authenticationId, password string, err error) {
	authText, err := base64.StdEncoding.DecodeString(encodedAuth)
	if err != nil {
		return "", "", "", err
	}

	parts := strings.Split(string(authText), UTF8NULL)
	if len(parts) != 3 {
		return "", "", "", errors.New("PLAIN data MUST send authentication and password")
	}

	return parts[0], parts[1], parts[2], nil
}
